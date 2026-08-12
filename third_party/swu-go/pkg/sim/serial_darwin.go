//go:build darwin

package sim

import "golang.org/x/sys/unix"

const (
	ioctlReadTermios  = unix.TIOCGETA
	ioctlWriteTermios = unix.TIOCSETA
)

// darwin 的 Ispeed/Ospeed 是 uint64。
func setTermiosSpeed(t *unix.Termios, speed uint32) {
	t.Ispeed = uint64(speed)
	t.Ospeed = uint64(speed)
}
