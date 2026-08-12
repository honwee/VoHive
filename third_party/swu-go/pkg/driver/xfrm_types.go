package driver

import "net"

// XFRM 的配置类型放在这里、不带 build tag，因为 pkg/swu 的 Session 和 MOBIKE 逻辑
// 引用它们，而那些文件是平台无关的。字段类型用本包的别名而不是 netlink 的，别名在
// xfrm_netlink_linux.go / xfrm_netlink_other.go 里按平台给出——netlink 只有 Linux
// 才有，让协议层的结构体直接依赖它，等于把整个模块钉死在 Linux 上。

// XFRMSAConfig XFRM Security Association 配置
type XFRMSAConfig struct {
	Src   net.IP // 本机 IP
	Dst   net.IP // 对端 IP (ePDG)
	SPI   uint32
	Proto Proto // 通常为 XFRM_PROTO_ESP

	// 算法配置 (AEAD 和 Crypt/Auth 互斥)
	IsAEAD bool

	// AEAD 模式 (如 AES-GCM)
	AeadAlgoName string
	AeadKey      []byte // encKey + salt
	AeadICVLen   int    // ICV 位数 (如 128)

	// 非 AEAD 模式 (如 AES-CBC + HMAC)
	CryptAlgoName string
	CryptKey      []byte
	AuthAlgoName  string
	AuthKey       []byte
	AuthTruncLen  int // 截断位数 (如 128)

	// ESP-in-UDP 封装 (NAT-T)
	EncapType    EncapType // XFRM_ENCAP_ESPINUDP
	EncapSrcPort int
	EncapDstPort int

	// XFRM Interface 关联
	Ifid int

	// Tunnel 模式 (VoWiFi 使用 tunnel 模式)
	Mode Mode // XFRM_MODE_TUNNEL

	// SA 生命周期（秒），用于触发内核 XFRM_MSG_EXPIRE 事件
	// Soft: 触发 rekey（默认 3300s = 55分钟）
	// Hard: 强制删除（默认 3600s = 60分钟）
	TimeLimitSoft uint64
	TimeLimitHard uint64

	// 抗重放窗口大小（0 = 使用默认值 32）
	ReplayWindow int

	// SA 方向标记（Linux 6.x+, XFRMA_SA_DIR）
	// 0 = 不设置, netlink.XFRM_SA_DIR_IN = 入站, netlink.XFRM_SA_DIR_OUT = 出站
	SADir SADir

	// 扩展序列号（ESN, RFC 4303 §2.2.1）
	// 64 位序列号，防止高速网络下 32 位 SN 溢出
	ESN bool
}

// XFRMSPConfig XFRM Security Policy 配置
type XFRMSPConfig struct {
	Src *net.IPNet // 源地址范围
	Dst *net.IPNet // 目标地址范围
	Dir Dir        // XFRM_DIR_IN / XFRM_DIR_OUT / XFRM_DIR_FWD

	// 模板参数
	TmplSrc   net.IP
	TmplDst   net.IP
	TmplProto Proto
	TmplMode  Mode
	TmplSPI   int

	// XFRM Interface 关联
	Ifid int
}
