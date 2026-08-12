//go:build darwin

package netbind

import (
	"fmt"
	"net"

	"golang.org/x/sys/unix"
)

// darwin 没有 SO_BINDTODEVICE，对应物是 IP_BOUND_IF / IPV6_BOUND_IF，而且它们要的是
// **接口索引**而不是名字，所以要先查一次。
//
// 两个选项都设：socket 建立时还不知道最终会走 v4 还是 v6（net.Dialer 的 Control 在
// 连接之前调用），只设一个的话另一族会静默走默认路由——正是这类绑定要防的情况。
// 对不适用的那一族，内核会返回错误，忽略即可。
func bindToDevice(fd int, iface string) error {
	ifi, err := net.InterfaceByName(iface)
	if err != nil {
		return fmt.Errorf("netbind: 找不到网卡 %s: %w", iface, err)
	}

	errV4 := unix.SetsockoptInt(fd, unix.IPPROTO_IP, unix.IP_BOUND_IF, ifi.Index)
	errV6 := unix.SetsockoptInt(fd, unix.IPPROTO_IPV6, unix.IPV6_BOUND_IF, ifi.Index)
	if errV4 != nil && errV6 != nil {
		// 两族都失败才算真失败：单族失败通常只是这个 socket 不是那一族的。
		return fmt.Errorf("netbind: 绑定 %s(index=%d) 失败: v4=%v v6=%v", iface, ifi.Index, errV4, errV6)
	}
	return nil
}
