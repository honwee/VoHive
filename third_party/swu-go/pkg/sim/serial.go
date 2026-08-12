package sim

import (
	"os"

	"golang.org/x/sys/unix"
)

// 串口参数设置。
//
// 原实现直接用 syscall.Syscall6 打 TCGETS/TCSETS，那两个常量只有 Linux 有；而
// Termios 的字段宽度也随平台变（darwin 的 Ispeed/Ospeed 是 uint64，Linux 是 uint32），
// 于是整个包把 swu-go 钉死在 Linux 上。
//
// x/sys/unix 的 IoctlGetTermios / IoctlSetTermios 在 linux 与 darwin 上都存在，并且
// 各自用正确的 ioctl 号和结构体。换过来之后这段不但能跨平台，还比原来的手搓 ioctl
// 更短——原来那段注释写着"这可能不容易交叉编译到 Windows，但用户是在 Linux 上"，
// 现在这个前提不成立了。
func setSerialParam(fd uintptr, baudRate int) error {
	termios, err := unix.IoctlGetTermios(int(fd), ioctlReadTermios)
	if err != nil {
		return err
	}

	speed := baudRateConstant(baudRate)

	// 原始模式 8N1：无奇偶校验、1 位停止位、8 位数据位，允许接收，忽略调制解调器
	// 控制线（模组的 AT 口没有真正的 DCD）。
	termios.Cflag &^= unix.PARENB | unix.CSTOPB | unix.CSIZE
	termios.Cflag |= unix.CS8 | unix.CREAD | unix.CLOCAL

	// 关掉所有流控与字符转换：AT 命令与 APDU 都是字节透明的，任何 CR/LF 改写都会
	// 破坏十六进制载荷。
	termios.Iflag &^= unix.IXON | unix.IXOFF | unix.IXANY |
		unix.ISTRIP | unix.INLCR | unix.IGNCR | unix.ICRNL

	// 非规范模式：不按行缓冲、不回显、不产生信号。
	termios.Lflag &^= unix.ICANON | unix.ECHO | unix.ECHOE | unix.ISIG

	// 输出不做任何后处理。
	termios.Oflag &^= unix.OPOST

	setTermiosSpeed(termios, speed)

	// 至少读到 1 字节才返回，不设字符间超时——超时由上层的 context 控制。
	termios.Cc[unix.VMIN] = 1
	termios.Cc[unix.VTIME] = 0

	return unix.IoctlSetTermios(int(fd), ioctlWriteTermios, termios)
}

func baudRateConstant(baudRate int) uint32 {
	switch baudRate {
	case 9600:
		return unix.B9600
	case 19200:
		return unix.B19200
	case 38400:
		return unix.B38400
	case 57600:
		return unix.B57600
	case 115200:
		return unix.B115200
	default:
		return unix.B115200
	}
}

func OpenSerial(path string, baudRate int) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_RDWR|unix.O_NOCTTY|unix.O_NONBLOCK, 0666)
	if err != nil {
		return nil, err
	}

	// 打开时用 O_NONBLOCK 是为了不被没有载波的串口卡住；配置完就切回阻塞，
	// 让读写按 VMIN/VTIME 的语义走。
	if err := unix.SetNonblock(int(f.Fd()), false); err != nil {
		f.Close()
		return nil, err
	}

	if err := setSerialParam(f.Fd(), baudRate); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}
