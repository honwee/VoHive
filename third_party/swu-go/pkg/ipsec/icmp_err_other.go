//go:build !linux

package ipsec

import "github.com/1239t/swu-go/pkg/logger"

// startErrorListener 在非 Linux 上是空操作。
//
// 它读的是 Linux 的 socket 错误队列（MSG_ERRQUEUE + IP_RECVERR），用来提前拿到
// ICMP "Fragmentation Needed" 从而下调 PMTU，以及把 Host Unreachable 当成 DPD 的
// 先行信号。darwin/BSD 没有等价机制——ICMP 错误不会以带外数据回到发送方的 socket。
//
// 后果如实说明：隧道仍然可用，但路径 MTU 只能靠上层的固定值和重传来发现，网络中断
// 的感知也只能等 DPD 超时，比 Linux 上慢。这是功能降级，不是等价实现。
func (s *SocketManager) startErrorListener() {
	logger.Debug("当前平台无 socket 错误队列，ICMP/PMTU 事件监听已禁用")
	<-s.closeChan
}
