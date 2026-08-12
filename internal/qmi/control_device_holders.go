package qmicore

import "strings"

// 谁正占用着 QMI 控制设备——用来决定要不要强制走 qmi-proxy。类型放在这里（无平台
// 后缀），因为 client_options.go 是平台无关的；探测实现只有 Linux 有，见
// control_device_holders_linux.go 与 _other.go。

type qmiControlDeviceHolder struct {
	PID     int
	Command string
}

type qmiControlDeviceHolders struct {
	Holders []qmiControlDeviceHolder
	Unknown bool
}

func (h qmiControlDeviceHolders) onlyQMIProxy() bool {
	if len(h.Holders) == 0 {
		return false
	}
	for _, holder := range h.Holders {
		cmd := strings.ToLower(strings.TrimSpace(holder.Command))
		if !strings.Contains(cmd, "qmi-proxy") {
			return false
		}
	}
	return true
}
