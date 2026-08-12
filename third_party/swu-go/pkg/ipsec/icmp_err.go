package ipsec

// 这个文件只放平台无关的事件类型。真正读 Linux 错误队列（MSG_ERRQUEUE /
// IP_RECVERR）的实现在 icmp_err_linux.go，其他平台的空实现在 icmp_err_other.go。
//
// 拆分的理由：pkg/swu/deps.go 在接口里引用了 NetEvent，所以类型必须在每个平台都存在；
// 而 PMTU 探测是纯 Linux 的辅助功能，它不该决定整个模块能不能为 darwin 编译。

// NetEvent 描述从错误队列收到的网络事件
type NetEvent struct {
	Type    NetEventType
	PMTU    uint32 // 如果是 PathMTU，这里会有新的 MTU
	Reason  string
	OldPort int // NAT-T 端口漂移前的旧端口
	NewPort int // NAT-T 端口漂移后的新端口
}

type NetEventType int

const (
	EventPathMTU        NetEventType = iota // 收到了 ICMP Frag Needed / Packet Too Big
	EventNetworkDown                        // 收到了 Host / Net Unreachable (用于 DPD 欺骗预测)
	EventNATPortChanged                     // NAT-T 端口漂移：远端源端口发生了变化 (RFC 3947)
)
