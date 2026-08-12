//go:build !linux

package qmicore

// 非 Linux 上无法枚举控制设备的占用者：这个探测靠遍历 /proc/<pid>/fd 并比对 rdev，
// 而 darwin 没有 /proc。
//
// 返回 Unknown=true 而不是"没人占用"。调用方对 Unknown 的处理是强制走 qmi-proxy，
// 这正是不确定时该有的保守选择——直连一个已被别人打开的控制设备，表现是两边的请求
// 互相踩，比多一层 proxy 糟得多。
var detectQMIControlDeviceHolders = func(controlDevice string) (qmiControlDeviceHolders, error) {
	return qmiControlDeviceHolders{Unknown: true}, nil
}
