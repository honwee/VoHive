package ipsec

import (
	"syscall"
	"time"
)

// durationToTimeval 把超时转成 syscall.Timeval。
//
// 原本是 timeval_32.go / timeval_64.go 两个按**架构**打 tag 的文件，各自手写字段。
// 那个划分是错的：Timeval 的字段宽度取决于**操作系统**而不只是架构——linux/arm64
// 的 Sec 和 Usec 都是 int64，darwin/arm64 的 Usec 却是 int32（__darwin_suseconds_t），
// 于是 arm64 这一档在 darwin 上编不过。
//
// 标准库自己就有平台正确的转换，直接用它，两个文件和整张架构清单都不需要了。
func durationToTimeval(d time.Duration) syscall.Timeval {
	return syscall.NsecToTimeval(int64(d))
}
