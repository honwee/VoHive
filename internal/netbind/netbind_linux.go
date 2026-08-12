//go:build linux

package netbind

import "syscall"

// Linux 直接按网卡名绑定。
func bindToDevice(fd int, iface string) error {
	return syscall.SetsockoptString(fd, syscall.SOL_SOCKET, syscall.SO_BINDTODEVICE, iface)
}
