//go:build !linux

package swu

// 非 Linux 上的空实现。见 session_linux.go 的说明：这两条路径都属于内核 IPsec +
// 内核 TUN 模式，而这些平台上只有 netstack 模式可用。

// startXFRMExpireMonitor 在没有 XFRM 的平台上无事可做。Child SA 的 rekey 仍由
// session 自己的定时器驱动，只是少了内核提前通知这一路。
func (s *Session) startXFRMExpireMonitor() {}

// applyNetworkConfigOnTUN 只在内核 TUN 模式下被调用。netstack 模式自己持有地址和
// 路由，不经过这里；真走到这里说明调用方选错了模式，所以返回错误而不是假装配好。
func (s *Session) applyNetworkConfigOnTUN(iface string) error {
	return errKernelTUNUnsupported
}

var errKernelTUNUnsupported = errUnsupported("内核 TUN 网络配置仅在 Linux 上可用；此平台请使用 netstack 模式")

type errUnsupported string

func (e errUnsupported) Error() string { return string(e) }
