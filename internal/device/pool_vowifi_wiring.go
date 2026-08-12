package device

import (
	"fmt"

	"github.com/1239t/vohive/internal/sipgw"
	"github.com/1239t/vowifi-go/runtimehost/voicehost"

	"github.com/1239t/vohive/pkg/logger"
)

// SetVoiceGateway 注入 VoWiFi 语音网关，用于优先走 IMS 外呼/挂断路径。
func (p *Pool) SetVoiceGateway(g *voicehost.Gateway) {
	p.mu.Lock()
	p.voiceGateway = g
	p.mu.Unlock()
	p.voWiFiHost().ConfigureRuntimeDependencies(g, vowifiDeliveryStore{}, poolVoWiFiRuntimeDispatcher{pool: p}, vowifiInboundSMS{pool: p})
}

// GetVoiceGateway 返回绑定的 VoiceGateway 实例
func (p *Pool) GetVoiceGateway() *voicehost.Gateway {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.voiceGateway
}

// SetSIPRegistrar 注入 SIP 注册器
// 由于此方法通常在 Worker 初始化之后才被调用，
// 需要回扫已有 Worker，给有 AudioDevice 但还没有 CSCallMgr 的 Worker 补创建
//
// 它**不再**注册 OnInvite/OnBye/OnCancel。cmd/vohive/main.go 在调用本方法之前
// 已经装好了同名回调，而且那套严格更好：它按 CSCallMgr.HasCall(callID) 判断这通
// 归谁，本方法原来的版本只看 "CSCallMgr != nil"——于是任何配了 AudioDevice 的设备，
// VoWiFi 的 BYE/CANCEL 全被送去 CS 管理器，IMS 腿永远拆不掉，只能等网络超时。
// 而且覆盖动作还顺手把 main.go 装的 SetOnPrack/SetOnAck 丢掉了。
func (p *Pool) SetSIPRegistrar(r *sipgw.Registrar) {
	p.mu.Lock()
	p.sipRegistrar = r
	for _, w := range p.workers {
		logger.Debug(fmt.Sprintf("[%s] SetSIPRegistrar 回扫: AudioDevice=%q, CSCallMgr=%v, Modem=%v", w.ID, w.Config.AudioDevice, w.CSCallMgr != nil, w.Modem != nil))
		if w.Config.AudioDevice != "" && w.CSCallMgr == nil {
			w.CSCallMgr = newCSCallManagerForWorker(w, r)
			if w.CSCallMgr != nil {
				logger.Info(fmt.Sprintf("[%s] 已启用 CS 域语音桥接 (AudioDev: %s)", w.ID, w.Config.AudioDevice))
			}
		}
	}
	p.mu.Unlock()
}
