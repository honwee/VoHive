package device

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/1239t/vohive/pkg/logger"
	"github.com/1239t/vohive/pkg/smscodec"
	"github.com/1239t/vowifi-go/runtimehost/eventhost"
)

// vowifiInboundReassembler 拼接 IMS 下发的长短信分片。做成包级变量而不是
// vowifiInboundSMS 的字段：ConfigureRuntimeDependencies 会被重复调用（Pool 初始化
// 一次、注入 VoiceGateway 时再一次），每次新建 reassembler 会把已收到的分片丢掉。
var vowifiInboundReassembler = smscodec.NewReassembler()

// vowifiInboundSMS 是 vowifi-go 的 messaging.InboundSMSHandler 实现：IMS 侧收到
// 网络下发的 RP-DATA（MT 短信）后交到这里。解码留在 vohive 一侧的原因和出站编码
// 一样——TPDU codec 在 pkg/smscodec，而 vowifi-go 不能反向依赖 vohive。
type vowifiInboundSMS struct {
	pool *Pool
}

func (h vowifiInboundSMS) HandleInboundSMS(ctx context.Context, deviceID string, rpData []byte, at time.Time) ([]byte, error) {
	rpMR, oa, _, tpdu, err := smscodec.ParseRPDataWithAddresses(rpData)
	if err != nil {
		// RP-MR 都取不到就没法构造 RP-ACK，只能让 SMSC 重投。
		return nil, fmt.Errorf("解析 RP-DATA 失败: %w", err)
	}
	// 无论 TPDU 能否解开都要回 RP-ACK：解不开是我们这边的问题，让 SMSC 反复重投
	// 这一条没有意义，只会阻塞后续短信。
	ack := smscodec.BuildRPAck(rpMR)

	sender, content, msgTime, concat, err := smscodec.DecodeDeliverTPDU(tpdu)
	if err != nil {
		return ack, fmt.Errorf("解码 DELIVER TPDU 失败(SC=%s): %w", oa, err)
	}

	ts := at
	if !msgTime.IsZero() {
		ts = msgTime
	}
	if ts.IsZero() {
		ts = time.Now()
	}

	if concat.IsConcat {
		complete, full := vowifiInboundReassembler.Add(sender, concat, content)
		if !complete {
			// Info, not Debug: an incomplete fragment is indistinguishable from
			// "nothing arrived" at the app layer, and that is exactly the state a
			// failing RP-ACK produces -- the SC keeps resending the same part
			// forever and the message never assembles. Without this line the
			// symptom is a silent missing SMS.
			logger.Info("VoWiFi 收到长短信分片，等待后续", "device", deviceID,
				"ref", concat.Ref, "seq", concat.Seq, "total", concat.Total)
			return ack, nil
		}
		content = full
		logger.Info("VoWiFi 长短信重组完成", "device", deviceID, "total", concat.Total)
	}
	vowifiInboundReassembler.Cleanup(10 * time.Minute)

	if strings.TrimSpace(content) == "" {
		logger.Info("VoWiFi 收到空短信，未入库", "device", deviceID, "sender", sender)
		return ack, nil
	}

	logger.Info("VoWiFi 收到短信", "device", deviceID, "sender", sender, "len", len(content))
	poolVoWiFiRuntimeDispatcher{pool: h.pool}.Dispatch(ctx, eventhost.SMSReceived{
		DevID:   deviceID,
		Sender:  strings.TrimSpace(sender),
		Content: content,
		Time:    ts,
	})
	return ack, nil
}
