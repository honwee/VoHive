//go:build !linux

package device

import "github.com/1239t/vohive/pkg/logger"

// 非 Linux 上没有内核 uevent 的 netlink 广播，热插拔监听不可用。
//
// 这不是新的降级路径：Linux 上 nl.Subscribe 失败时走的也是同一条——记一条 warn 然后
// 返回，设备变化改由周期性重扫发现。所以这里的行为与"Linux 上没有权限订阅 uevent"
// 完全一致，只是原因不同。
func (w *UdevWatcher) loop() {
	logger.Warn("当前平台无内核 uevent 订阅，热插拔监听不可用，设备变化将依赖周期性重扫")
	<-w.stop
}
