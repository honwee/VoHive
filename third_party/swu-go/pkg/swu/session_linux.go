//go:build linux

package swu

import (
	"fmt"
	"net"
	"time"

	"github.com/1239t/swu-go/pkg/driver"
	"github.com/1239t/swu-go/pkg/ikev2"
	"github.com/1239t/swu-go/pkg/logger"
	"github.com/iniwex5/netlink"
	"github.com/iniwex5/netlink/nl"
)

// 这两个函数直接操作 Linux 内核：一个订阅 XFRM 的 SA 过期事件（netlink 广播组），
// 一个用 netlink 给内核 TUN 配地址、路由和策略路由表。两者在 darwin/BSD 上都没有
// 等价物，而 netstack 路径两个都不走——它在用户态自己做 ESP 和路由。

// startXFRMExpireMonitor 监听内核 XFRM_MSG_EXPIRE 事件
// Soft Expire: SA 接近过期，触发主动 Child SA Rekey
// Hard Expire: SA 已过期，触发隧道重建
func (s *Session) startXFRMExpireMonitor() {
	if s.xfrmMgr == nil {
		return
	}

	ch := make(chan netlink.XfrmMsg)
	done := make(chan struct{})
	errCh := make(chan error, 1)

	if err := netlink.XfrmMonitor(ch, done, errCh, nl.XFRM_MSG_EXPIRE); err != nil {
		s.Logger.Warn("启动 XFRM Expire 监听失败", logger.Err(err))
		return
	}

	s.Logger.Info("XFRM SA Expire 监听已启动")

	go func() {
		defer close(done)

		for {
			select {
			case <-s.ctx.Done():
				return
			case err := <-errCh:
				s.Logger.Warn("XFRM 监听错误", logger.Err(err))
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				expire, ok := msg.(*netlink.XfrmMsgExpire)
				if !ok || expire.XfrmState == nil {
					continue
				}

				// 过滤：只处理本 session 的 SA
				spi := uint32(expire.XfrmState.Spi)
				isOurSA := false
				if s.ChildSAOut != nil && s.ChildSAOut.SPI == spi {
					isOurSA = true
				}
				if s.ChildSAIn != nil && s.ChildSAIn.SPI == spi {
					isOurSA = true
				}
				if !isOurSA {
					continue
				}

				if expire.Hard {
					s.Logger.Warn("XFRM SA Hard Expire，触发隧道重建",
						logger.Uint32("spi", spi))
					if s.OnSessionDown != nil {
						go s.OnSessionDown()
					} else if s.cancel != nil {
						s.cancel()
					}
				} else {
					// Soft Expire 触发主动 Child SA Rekey
					s.Logger.Info("XFRM SA Soft Expire，触发主动 Child SA Rekey",
						logger.Uint32("spi", spi))
					go func() {
						if err := s.RekeyChildSA(); err != nil {
							s.Logger.Warn("Soft Expire 触发 Rekey 失败", logger.Err(err))
						}
					}()
				}
			}
		}
	}()
}

func (s *Session) applyNetworkConfigOnTUN(iface string) error {
	s.Logger.Debug("Applying network config on TUN", logger.String("iface", iface), logger.Bool("has_driver", s.net != nil))

	if s.net == nil {
		return nil
	}
	deleter, _ := s.net.(netToolsDeleter)

	if s.cpConfig != nil {
		if len(s.cpConfig.IPv4Addresses) > 0 {
			ip := s.cpConfig.IPv4Addresses[0].To4()
			if ip != nil {
				cidr := fmt.Sprintf("%s/32", ip.String())
				if err := s.net.AddAddress(iface, cidr); err != nil {
					return err
				}
				// 优化: 删除接口时 IP 地址会自动被内核回收，不再记录 O(N) 的 DelAddress
			}
		}
		if len(s.cpConfig.IPv6Addresses) > 0 {
			ip := s.cpConfig.IPv6Addresses[0].To16()
			if ip != nil {
				cidr := fmt.Sprintf("%s/128", ip.String())
				if err := s.net.AddAddress6(iface, cidr); err != nil {
					return err
				}
				// 优化: IPv6 同样随接口销毁
			}
		}
	}

	var routes []string
	var routes6 []string
	if s.cpConfig != nil {
		for _, ip := range s.cpConfig.IPv4PCSCF {
			if v4 := ip.To4(); v4 != nil {
				routes = append(routes, fmt.Sprintf("%s/32", v4.String()))
			}
		}
		for _, ip := range s.cpConfig.IPv6PCSCF {
			if v6 := ip.To16(); v6 != nil {
				routes6 = append(routes6, fmt.Sprintf("%s/128", v6.String()))
			}
		}
	}

	// 检查是否支持策略路由
	// 如果支持，我们允许添加 0.0.0.0/0 默认路由（因为它会被隔离在独立的路由表中）
	// 如果不支持，我们需要跳过默认路由，防止覆盖宿主机的默认网关
	type policyRouter interface {
		AddRouteTable(cidr string, iface string, table int) error
		DelRouteTable(cidr string, iface string, table int) error
		AddRule(srcCIDR string, table int) error
		DelRule(srcCIDR string, table int) error
		AddInputRule(iface string, table int) error
		DelInputRule(iface string, table int) error
		FlushRules(table int, iface string) error
		CleanConflictRoutes(cidrs []string, keepIface string, family int)
		SetSysctl(key, value string) error
	}
	_, enablePolicyRouting := s.net.(policyRouter)

	for _, ts := range s.tsr {
		if ts.TSType != ikev2.TS_IPV4_ADDR_RANGE && ts.TSType != ikev2.TS_IPV6_ADDR_RANGE {
			continue
		}

		// IPv4 处理
		if ts.TSType == ikev2.TS_IPV4_ADDR_RANGE {
			// 如果不支持策略路由，且是全网段，则跳过 (保护宿主机)
			if !enablePolicyRouting && isFullIPv4Range(ts) {
				s.Logger.Debug("Skipping full range IPv4 TS to protect host default gateway", logger.String("start", net.IP(ts.StartAddr).String()))
				continue
			}

			// 如果是全网段，直接添加 0.0.0.0/0
			if isFullIPv4Range(ts) {
				s.Logger.Debug("PolicyRouting: Adding default IPv4 route (0.0.0.0/0)", logger.Int("table", 0)) // table ID not avail here, just info
				routes = append(routes, "0.0.0.0/0")
				continue
			}

			start := net.IP(ts.StartAddr)
			end := net.IP(ts.EndAddr)
			cidrs, err := ipv4RangeToCIDRs(start, end)
			if err != nil {
				continue
			}
			routes = append(routes, cidrs...)
		}

		// IPv6 处理
		if ts.TSType == ikev2.TS_IPV6_ADDR_RANGE {
			// 如果不支持策略路由，且是全网段，则跳过
			if !enablePolicyRouting && isFullIPv6Range(ts) {
				s.Logger.Warn("Skipping full range IPv6 TS to protect host default gateway")
				continue
			}

			// 如果是全网段，直接添加 ::/0
			if isFullIPv6Range(ts) {
				s.Logger.Debug("PolicyRouting: Adding default IPv6 route (::/0)")
				routes6 = append(routes6, "::/0")
				continue
			}

			// 简单处理：如果是单个 IP
			if len(ts.StartAddr) == 16 && len(ts.EndAddr) == 16 {
				start := net.IP(ts.StartAddr)
				end := net.IP(ts.EndAddr)
				if start.Equal(end) {
					routes6 = append(routes6, fmt.Sprintf("%s/128", start.String()))
				} else {
					// TODO: 完整的 IPv6 范围转 CIDR 比较复杂，暂时只支持全网段或单IP
					// 如果不是全网段，我们暂不添加详细路由，或者等待后续完善
					s.Logger.Warn("Skipping complex IPv6 range", logger.String("start", start.String()), logger.String("end", end.String()))
				}
			}
		}
	}

	// 尝试使用策略路由（独立路由表），避免多设备共享 P-CSCF 等场景下路由冲突
	if pr, ok := s.net.(policyRouter); ok {
		enablePolicyRouting = true
		s.Logger.Info("Policy routing supported by driver", logger.String("iface", iface))
		// 使用 TUN 接口的 link index 作为路由表 ID（避免与系统表冲突，加偏移 1000）
		link, err := s.net.(*driver.NetTools).GetLink(iface)
		if err == nil {
			tableID := link.Attrs().Index + 1000

			// O(1) 清理: 只注册一次 FlushRules 把与该设备(table/iface)相关的所有 rule 清除
			s.netUndos = append(s.netUndos, func() error { return pr.FlushRules(tableID, iface) })

			// 1. 添加基于入站接口 (iif) 的策略路由规则：iif <iface> lookup <tableID>
			// 这解决了 RPF (反向路径过滤) 问题：确保入站包能匹配到正确的路由表
			if err := pr.AddInputRule(iface, tableID); err != nil {
				return err
			}

			// 2. 添加基于源地址的策略路由规则：from <设备IP> lookup <tableID>
			var srcCIDRs []string
			if s.cpConfig != nil {
				for _, ip := range s.cpConfig.IPv4Addresses {
					if v4 := ip.To4(); v4 != nil {
						srcCIDRs = append(srcCIDRs, fmt.Sprintf("%s/32", v4.String()))
					}
				}
				for _, ip := range s.cpConfig.IPv6Addresses {
					if v6 := ip.To16(); v6 != nil {
						srcCIDRs = append(srcCIDRs, fmt.Sprintf("%s/128", v6.String()))
					}
				}
			}

			// 先添加 ip rule
			for _, src := range srcCIDRs {
				if err := pr.AddRule(src, tableID); err != nil {
					return err
				}
			}

			// 再添加路由到独立路由表 (路由表随接口 LinkDown 而内核自动隐式销毁)
			for _, cidr := range routes {
				if err := pr.AddRouteTable(cidr, iface, tableID); err != nil {
					return err
				}
			}
			for _, cidr := range routes6 {
				// Revert: StrongSwan uses direct routes. Let's try direct routes again with ARP enabled.
				if err := pr.AddRouteTable(cidr, iface, tableID); err != nil {
					return err
				}
			}

			// [清理 main 表冲突路由]
			// 其他设备或旧 session 可能在 main 表中留下到 P-CSCF 的路由 (dev ens2)，
			// 这些路由会抢占策略路由，导致 Go dial tcp 走物理接口而非 XFRM 隧道
			pr.CleanConflictRoutes(routes6, iface, netlink.FAMILY_V6)
			pr.CleanConflictRoutes(routes, iface, netlink.FAMILY_V4)

			// XFRM 接口初始化：确保 IPv6 可用
			go func() {
				time.Sleep(500 * time.Millisecond)
				// 确保接口 UP
				if nt, ok := s.net.(*driver.NetTools); ok {
					_ = nt.SetLinkUp(iface)
					// 添加 Link-Local 地址（XFRM 接口无 ARP 可能不会自动生成）
					_ = nt.AddAddress6(iface, "fe80::1/64")
				}
				// 确保 IPv6 启用且禁用 DAD（XFRM 接口无需邻居发现）
				_ = pr.SetSysctl(fmt.Sprintf("net.ipv6.conf.%s.disable_ipv6", iface), "0")
				_ = pr.SetSysctl(fmt.Sprintf("net.ipv6.conf.%s.accept_dad", iface), "0")
			}()

			return nil
		}
	}

	// 回退：使用默认路由表（单设备场景或不支持策略路由时）
	for _, cidr := range routes {
		if err := s.net.AddRoute(cidr, "", iface); err != nil {
			return err
		}
		if deleter != nil {
			c := cidr
			s.netUndos = append(s.netUndos, func() error { return deleter.DelRoute(c, "", iface) })
		}
	}
	for _, cidr := range routes6 {
		if err := s.net.AddRoute6(cidr, "", iface); err != nil {
			return err
		}
		if deleter != nil {
			c := cidr
			s.netUndos = append(s.netUndos, func() error { return deleter.DelRoute6(c, "", iface) })
		}
	}
	return nil
}
