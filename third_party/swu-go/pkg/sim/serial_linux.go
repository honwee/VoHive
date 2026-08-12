//go:build linux

package sim

import "golang.org/x/sys/unix"

// termios 的 ioctl 号与速度字段在各 unix 之间不一致，只有这两处需要按平台分。
const (
	ioctlReadTermios  = unix.TCGETS
	ioctlWriteTermios = unix.TCSETS
)

// Linux 的 Termios 有独立的 Ispeed/Ospeed 字段（uint32）。
func setTermiosSpeed(t *unix.Termios, speed uint32) {
	t.Ispeed = speed
	t.Ospeed = speed
}
