//go:build linux

package driver

import "github.com/iniwex5/netlink"

// Linux 上这些就是 netlink 的类型与常量，别名零成本。
type (
	Proto     = netlink.Proto
	EncapType = netlink.EncapType
	Mode      = netlink.Mode
	SADir     = netlink.SADir
	Dir       = netlink.Dir
)

const (
	XFRMProtoESP      = netlink.XFRM_PROTO_ESP
	XFRMModeTunnel    = netlink.XFRM_MODE_TUNNEL
	XFRMEncapESPinUDP = netlink.XFRM_ENCAP_ESPINUDP
	XFRMDirIn         = netlink.XFRM_DIR_IN
	XFRMDirOut        = netlink.XFRM_DIR_OUT
	XFRMDirFwd        = netlink.XFRM_DIR_FWD
	XFRMSADirIn       = netlink.XFRM_SA_DIR_IN
	XFRMSADirOut      = netlink.XFRM_SA_DIR_OUT
)
