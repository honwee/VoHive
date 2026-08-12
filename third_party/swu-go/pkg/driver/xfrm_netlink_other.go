//go:build !linux

package driver

// 非 Linux 上没有 XFRM，这些类型只是为了让引用它们的平台无关代码能编译。取值与
// Linux 的 uapi 保持一致纯粹是为了日志里打印出来时不误导——**没有任何代码会消费
// 它们**，因为所有真正操作 XFRM 的函数在这些平台上都直接返回 ErrXFRMUnsupported。
type (
	Proto     uint8
	EncapType int8
	Mode      uint8
	SADir     uint8
	Dir       uint8
)

const (
	XFRMProtoESP      Proto     = 50 // IPPROTO_ESP
	XFRMModeTunnel    Mode      = 1
	XFRMEncapESPinUDP EncapType = 2
	XFRMDirIn         Dir       = 0
	XFRMDirOut        Dir       = 1
	XFRMDirFwd        Dir       = 2
	XFRMSADirIn       SADir     = 1
	XFRMSADirOut      SADir     = 2
)
