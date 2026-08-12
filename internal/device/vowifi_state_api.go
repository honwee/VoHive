package device

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/1239t/vohive/pkg/smscodec"
	"github.com/1239t/vowifi-go/runtimehost"
	"github.com/1239t/vowifi-go/runtimehost/eventhost"
	"github.com/1239t/vowifi-go/runtimehost/messaging"
)

// nextRPMR allocates RP-Message-Reference values for outgoing VoWiFi SMS
// parts. A single wrapping byte counter is sufficient: the delivery-report
// correlation path (messaging.DeliveryStore.MarkSMSDeliveryPartReport) also
// keys on deviceID and a time window, so this only needs to avoid
// collisions within that window, not be globally unique. There's no
// pre-existing allocator to reuse here -- SendSMS via SIP MESSAGE is new;
// vohive's legacy AT/QMI SMS path never called smscodec.BuildRPData.
var rpmrCounter uint32

func nextRPMR() byte {
	return byte(atomic.AddUint32(&rpmrCounter, 1))
}

func (p *Pool) GetVoWiFiApp() *runtimehost.Instance {
	return p.GetVoWiFiAppForDevice()
}

// FirstVoWiFiDeviceID returns the device ID of an arbitrary active VoWiFi
// instance (matching GetVoWiFiAppForDevice's no-argument behavior), or ""
// if none is active. Exists so callers that only had a *runtimehost.Instance
// via GetVoWiFiApp() (with no device ID of their own) can still reach the
// device-scoped Pool methods, e.g. SendVoWiFiSMSWithOptions.
func (p *Pool) FirstVoWiFiDeviceID() string {
	for devID := range p.voWiFiHost().Instances() {
		return devID
	}
	return ""
}

func (p *Pool) GetVoWiFiAppForDevice(deviceID ...string) *runtimehost.Instance {
	if len(deviceID) > 0 && deviceID[0] != "" {
		return p.voWiFiHost().Instance(deviceID[0])
	} else {
		for _, app := range p.voWiFiHost().Instances() {
			return app
		}
	}

	return nil
}

func (p *Pool) GetAllVoWiFiApps() map[string]*runtimehost.Instance {
	return p.voWiFiHost().Instances()
}

func (p *Pool) SendVoWiFiSMS(ctx context.Context, deviceID, to, text string) error {
	_, err := p.SendVoWiFiSMSWithResult(ctx, deviceID, to, text)
	return err
}

// SimulateVoWiFiCall places a signalling-only outbound call over VoWiFi.
//
// It answers the one question terminating-call tests cannot: whether the network
// accepts an originating INVITE from this registration at all. Media points at a
// black hole on purpose -- a call that reaches 180/200 has proved the carrier
// routes IMS voice for this line, and a 403/603/404 has proved it does not.
func (p *Pool) SimulateVoWiFiCall(ctx context.Context, deviceID, callee string, hold time.Duration) (string, int, error) {
	inst := p.voWiFiHost().Instance(deviceID)
	if inst == nil {
		return "", 0, fmt.Errorf("设备 %s 的 VoWiFi 未启动", deviceID)
	}
	return inst.SimulateCall(ctx, callee, hold)
}

func (p *Pool) SendVoWiFiSMSWithResult(ctx context.Context, deviceID, to, text string) (messaging.SendOutcome, error) {
	return p.SendVoWiFiSMSWithOptions(ctx, deviceID, to, text, smscodec.SubmitOptions{})
}

// SendVoWiFiSMSWithOptions encodes to/text into RP-DATA(SUBMIT) parts using
// vohive's own pkg/smscodec, then hands the already-encoded bytes to the
// vowifi-go IMS engine. That engine (vowifi-go/runtimehost) must not import
// vohive (the dependency runs the other way), so encoding responsibility
// -- TPDU construction, GSM7/UCS2 choice, RP-DATA framing, RP-MR allocation
// -- lives here, on vohive's side, per messaging.SMSPart's contract.
func (p *Pool) SendVoWiFiSMSWithOptions(ctx context.Context, deviceID, to, text string, opts smscodec.SubmitOptions) (messaging.SendOutcome, error) {
	inst := p.voWiFiHost().Instance(deviceID)
	if inst == nil {
		return messaging.SendOutcome{}, fmt.Errorf("设备 %s 的 VoWiFi 未启动", deviceID)
	}
	svc := inst.Service()
	if svc == nil {
		return messaging.SendOutcome{}, fmt.Errorf("设备 %s 的 VoWiFi IMS 服务未就绪", deviceID)
	}

	worker := p.GetWorker(deviceID)
	if worker == nil {
		return messaging.SendOutcome{}, fmt.Errorf("设备 %s 不存在", deviceID)
	}
	smscCtx, smscCancel := context.WithTimeout(ctx, 5*time.Second)
	smsc, err := worker.getSMSCWithContext(smscCtx)
	smscCancel()
	if err != nil {
		return messaging.SendOutcome{}, fmt.Errorf("获取 SMSC 失败: %w", err)
	}
	if smsc == "" {
		return messaging.SendOutcome{}, fmt.Errorf("设备 %s 未配置 SMSC，无法构建 RP-DATA", deviceID)
	}

	tpdus, _, err := smscodec.BuildSubmitTPDUsWithOptions(to, text, opts)
	if err != nil {
		return messaging.SendOutcome{}, fmt.Errorf("编码 SMS TPDU 失败: %w", err)
	}

	parts := make([]messaging.SMSPart, 0, len(tpdus))
	for _, tpdu := range tpdus {
		mr := nextRPMR()
		parts = append(parts, messaging.SMSPart{
			RPMR: mr,
			Body: smscodec.BuildRPData(mr, tpdu, smsc),
			SMSC: smsc,
		})
	}

	outcome, err := svc.SendSMS(ctx, to, text, parts)
	if err != nil {
		return outcome, err
	}

	// 出站短信入库只能在这里做：API 层的 VoWiFi 分支刻意不调 SaveSMS，改为等一条
	// eventhost.SMSSent —— 但 vowifi-go 从来没有发过这个事件（SendSMS 只返回
	// SendOutcome），于是发送成功的短信在历史里根本不存在，前端只看得到收到的。
	// 事件在这里补发，而不是在 vowifi-go 里 emit：TargetURI 要的是收件人号码，
	// 而引擎侧的 Request-URI 是 SMSC，两者不是一回事。
	poolVoWiFiRuntimeDispatcher{pool: p}.Dispatch(ctx, eventhost.SMSSent{
		DevID:      deviceID,
		TargetURI:  to,
		Content:    text,
		Time:       time.Now(),
		TotalParts: outcome.PartsTotal,
	})
	return outcome, nil
}

func (p *Pool) IsVoWiFiActive(deviceID string) bool {
	return p.voWiFiHost().Active(deviceID)
}

// SendVoWiFiUSSD 通过 VoWiFi 发送 USSD 请求（首轮）。
func (p *Pool) SendVoWiFiUSSD(ctx context.Context, deviceID, command string) (*messaging.USSDResult, error) {
	if inst := p.voWiFiHost().Instance(deviceID); inst != nil {
		svc := inst.Service()
		if svc == nil {
			return nil, fmt.Errorf("设备 %s 的 VoWiFi IMS 服务未就绪", deviceID)
		}
		return svc.SendUSSD(ctx, command)
	}
	return nil, fmt.Errorf("设备 %s 的 VoWiFi 未启动", deviceID)
}

// ContinueVoWiFiUSSD 在已有 VoWiFi USSD 会话中发送后续输入。
func (p *Pool) ContinueVoWiFiUSSD(ctx context.Context, deviceID, sessionID, input string) (*messaging.USSDResult, error) {
	if inst := p.voWiFiHost().Instance(deviceID); inst != nil {
		svc := inst.Service()
		if svc == nil {
			return nil, fmt.Errorf("设备 %s 的 VoWiFi IMS 服务未就绪", deviceID)
		}
		return svc.ContinueUSSD(ctx, sessionID, input)
	}
	return nil, fmt.Errorf("设备 %s 的 VoWiFi 未启动", deviceID)
}

// CancelVoWiFiUSSD 取消 VoWiFi USSD 会话。
func (p *Pool) CancelVoWiFiUSSD(ctx context.Context, deviceID, sessionID string) error {
	if inst := p.voWiFiHost().Instance(deviceID); inst != nil {
		svc := inst.Service()
		if svc == nil {
			return fmt.Errorf("设备 %s 的 VoWiFi IMS 服务未就绪", deviceID)
		}
		return svc.CancelUSSD(ctx, sessionID)
	}
	return fmt.Errorf("设备 %s 的 VoWiFi 未启动", deviceID)
}

func (p *Pool) GetVoWiFiStatus() (enabled bool, deviceID string, status string) {
	for devID, inst := range p.voWiFiHost().Instances() {
		if inst == nil {
			return true, devID, "VoWiFi: STOPPED"
		}
		return true, devID, inst.Status()
	}
	return false, "", "未初始化"
}

func (p *Pool) GetVoWiFiStatusAll() map[string]string {
	result := make(map[string]string)
	for devID, inst := range p.voWiFiHost().Instances() {
		result[devID] = inst.Status()
	}
	return result
}

func (p *Pool) GetVoWiFiObs(deviceID string) map[string]interface{} {
	if inst := p.voWiFiHost().Instance(deviceID); inst != nil {
		return inst.Obs()
	}
	return nil
}

func (p *Pool) GetVoWiFiRuntimeState(deviceID string) (runtimehost.State, bool) {
	return p.voWiFiHost().State(deviceID)
}

func (p *Pool) SubscribeVoWiFiState(deviceID string) (<-chan struct{}, func()) {
	return p.voWiFiHost().SubscribeState(deviceID)
}

func (p *Pool) recordVoWiFiStartupState(deviceID string, state runtimehost.State) {
	p.voWiFiHost().RecordStartupState(deviceID, state)
}

func (p *Pool) clearVoWiFiStartupState(deviceID string) {
	p.voWiFiHost().ClearStartupState(deviceID)
}

func (p *Pool) clearVoWiFiStartupStateAndBroadcast(deviceID string) {
	p.voWiFiHost().ClearStartupStateAndBroadcast(deviceID)
}

func (p *Pool) broadcastVoWiFiStateChange(deviceID string) {
	p.voWiFiHost().BroadcastState(deviceID)
}
