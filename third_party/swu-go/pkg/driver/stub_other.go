//go:build !linux

package driver

import (
	"errors"
	"net"
)

// 非 Linux 平台上的内核网络桩。
//
// 这个包做的三件事——XFRM SA/SP、netlink 地址与路由、TUN 设备——全部是 Linux 内核
// 特性，在 darwin/BSD 上没有等价物。它们之所以还需要在这些平台上存在，是因为
// pkg/swu 的 Session 引用了这些类型；而 Session 真正在 macOS 上要走的是 netstack
// 路径，那条路径把 TUN 和地址配置都交给用户态，根本不碰这里。
//
// 每个桩都返回错误而不是静默成功。一个假装配好了路由的 no-op，会把"这条路在这个
// 平台上不通"变成"隧道建起来了但没有流量"——后者要花几个小时才能查到。
var ErrUnsupportedPlatform = errors.New("driver: 该功能仅在 Linux 上可用（XFRM/netlink/TUN）")

// --- NetTools ---

// NetTools 在非 Linux 上没有实现。走 netstack 的调用方应当通过 swu.Config.NetTools
// 注入自己的实现；不注入而直接使用，就会拿到 ErrUnsupportedPlatform。
type NetTools struct{}

func NewNetTools() *NetTools { return &NetTools{} }

func (n *NetTools) SetLinkUp(iface string) error                { return ErrUnsupportedPlatform }
func (n *NetTools) SetMTU(iface string, mtu int) error          { return ErrUnsupportedPlatform }
func (n *NetTools) AddAddress(iface string, cidr string) error  { return ErrUnsupportedPlatform }
func (n *NetTools) AddAddress6(iface string, cidr string) error { return ErrUnsupportedPlatform }
func (n *NetTools) AddRoute(cidr, gw, iface string) error       { return ErrUnsupportedPlatform }
func (n *NetTools) AddRoute6(cidr, gw, iface string) error      { return ErrUnsupportedPlatform }
func (n *NetTools) AddRouteTable(cidr, iface string, table int) error {
	return ErrUnsupportedPlatform
}

// 回滚方向的操作（transaction.go 用）。同样返回错误：一个假装删掉了路由的 no-op，
// 会让失败回滚看起来干净，而实际留下一堆残留配置。
func (n *NetTools) SetLinkDown(iface string) error              { return ErrUnsupportedPlatform }
func (n *NetTools) DelAddress(iface string, cidr string) error  { return ErrUnsupportedPlatform }
func (n *NetTools) DelAddress6(iface string, cidr string) error { return ErrUnsupportedPlatform }
func (n *NetTools) DelRoute(cidr, gw, iface string) error       { return ErrUnsupportedPlatform }
func (n *NetTools) DelRoute6(cidr, gw, iface string) error      { return ErrUnsupportedPlatform }

// --- TUN ---

// TUNDevice 在非 Linux 上无法创建。netstack 模式通过 swu.Config.TUNFactory 提供自己的
// 实现，那条路径不会走到这里。
type TUNDevice struct{}

func NewTUNDevice(name string) (*TUNDevice, error) { return nil, ErrUnsupportedPlatform }

func (t *TUNDevice) DeviceName() string          { return "" }
func (t *TUNDevice) Read(p []byte) (int, error)  { return 0, ErrUnsupportedPlatform }
func (t *TUNDevice) Write(p []byte) (int, error) { return 0, ErrUnsupportedPlatform }
func (t *TUNDevice) Close() error                { return nil }

// --- XFRM ---

// XFRMManager 在非 Linux 上是空壳。内核 IPsec 不可用时，ESP 只能在用户态做——这正是
// netstack 路径的做法。
type XFRMManager struct{}

func NewXFRMManager() *XFRMManager { return &XFRMManager{} }

func (x *XFRMManager) FlushAll()                       {}
func (x *XFRMManager) AddSA(cfg XFRMSAConfig) error    { return ErrUnsupportedPlatform }
func (x *XFRMManager) UpdateSA(cfg XFRMSAConfig) error { return ErrUnsupportedPlatform }
func (x *XFRMManager) DelSA(spi uint32, src, dst net.IP, proto Proto) error {
	return ErrUnsupportedPlatform
}
func (x *XFRMManager) AddSP(cfg XFRMSPConfig) error    { return ErrUnsupportedPlatform }
func (x *XFRMManager) UpdateSP(cfg XFRMSPConfig) error { return ErrUnsupportedPlatform }
func (x *XFRMManager) DelSP(cfg XFRMSPConfig) error    { return ErrUnsupportedPlatform }
func (x *XFRMManager) FlushByIP(ip net.IP)             {}
func (x *XFRMManager) GetSALastUsed(spi uint32, src, dst net.IP, proto Proto) (uint64, error) {
	return 0, ErrUnsupportedPlatform
}
func (x *XFRMManager) AddXFRMInterface(name string, ifID uint32, underlyingIdx int) error {
	return ErrUnsupportedPlatform
}
func (x *XFRMManager) DelXFRMInterface(name string) error { return ErrUnsupportedPlatform }
