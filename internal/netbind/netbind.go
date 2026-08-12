// Package netbind 把"让这个 socket 只从某张网卡出去"这件事做成跨平台的。
//
// 这在多模组场景里是必需的，不是优化：机器上同时有好几条 WWAN 链路，路由表里谁的
// 度量值小就走谁，而探测和代理必须能指定"我就要从这一张卡出去"。绑错网卡的表现是
// 数据从另一张卡发出去、看起来完全正常，只是用错了 SIM 的流量和 IP。
package netbind

import (
	"strings"
	"syscall"
)

// Control 返回可直接赋给 net.Dialer.Control 的函数，把 socket 绑定到 iface。
// iface 为空时返回 nil，调用方无需自己判空。
func Control(iface string) func(network, address string, c syscall.RawConn) error {
	iface = strings.TrimSpace(iface)
	if iface == "" {
		return nil
	}
	return func(_, _ string, c syscall.RawConn) error {
		var serr error
		if err := c.Control(func(fd uintptr) {
			serr = bindToDevice(int(fd), iface)
		}); err != nil {
			return err
		}
		return serr
	}
}
