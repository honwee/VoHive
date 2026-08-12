//go:build linux

package device

import (
	"errors"
	"time"

	"github.com/iniwex5/netlink/nl"
	"golang.org/x/sys/unix"

	"github.com/1239t/vohive/pkg/logger"
)

func (w *UdevWatcher) loop() {
	// 创建 netlink 连接监听内核 uevent
	conn, err := nl.Subscribe(unix.NETLINK_KOBJECT_UEVENT)
	if err != nil {
		logger.Warn("udev 监听器启动失败，热插拔功能不可用", "err", err)
		return
	}
	defer conn.Close()

	logger.Info("udev 设备热插拔监听器已启动")

	for {
		select {
		case <-w.stop:
			logger.Info("udev 监听器已停止")
			return
		default:
		}

		// 设置读取超时，以便定期检查 stop 信号
		tv := unix.NsecToTimeval((1 * time.Second).Nanoseconds())
		_ = conn.SetReceiveTimeout(&tv)

		msgs, _, err := conn.Receive()
		if err != nil {
			// 超时错误是正常的
			if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK) {
				continue
			}
			// 其他错误记录但继续
			continue
		}

		for _, msg := range msgs {
			if w.isModemEvent(msg.Data) {
				w.scheduleRescan()
				break // 一批事件只触发一次扫描
			}
		}
	}
}
