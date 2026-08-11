# 关于这个 fork

上游：[1239t/vohive](https://github.com/1239t/vohive)。这个 fork 只做一件事——
让开源版在 **CTExcel UK（MCC/MNC 234-33）** 卡上把 VoWiFi 的 IMS REGISTER
打通到 200 OK，并让短信双向可用。已在真机实测通过。

## 两个仓库，必须并排 clone

引擎侧的改动全部在 [honwee/vowifi-go](https://github.com/honwee/vowifi-go)，
本仓库通过 `go.mod` 里的本地 `replace` 指过去：

```
replace github.com/1239t/vowifi-go => ../vowifi-go-source
```

所以目录名是有要求的——`vowifi-go` 必须 clone 成同级目录 `vowifi-go-source`：

```sh
git clone https://github.com/honwee/VoHive.git            vohive-source
git clone https://github.com/honwee/vowifi-go.git         vowifi-go-source
cd vohive-source && go build ./cmd/vohive
```

之所以保留本地路径 `replace` 而不改成模块路径替换：后者要求被替换模块的
`go.mod` 声明 `module github.com/honwee/vowifi-go`，那意味着把两个仓库里所有
import 路径全部重写。为了少动上游代码，这里选了目录约定。

## 本 fork 相对上游改了什么

分两层，可分别回滚：

**引擎层（在 vowifi-go 仓库）**

- ESP 封装层次搞错：此前 ESP 被套在 TCP 流上，且 IPv6 next-header 遍历把
  ESP 当成可跳过的扩展头。这是注册不上的真正根因。
- 注册后的请求补 `Security-Verify` 与 `Require/Proxy-Require: sec-agree`，
  否则 P-CSCF 回 494。
- 注册 UA 不再关闭借来的受保护连接。
- 入站 port-s（TS 33.203 §6.3）交给 sipgo 服务，trace 只旁路不抢读。
- MT 短信：入站 RP-DATA 按 TS 24.341 §5.3.2.4 以**新的** MESSAGE 回 RP-ACK，
  Request-URI 取自收到的 `P-Asserted-Identity`。
- 传输探活：30s 的 RFC 5626 §4.4.1 双 CRLF ping，写失败即上报宿主重建隧道；
  拆隧道不再死锁（此前 `Close()` 关错了监听器，`Accept()` 收不到取消）。
- 运营商预设改为数据驱动，`234-33` 解析为专属条目而非通用 `ee-uk`。

**宿主层（本仓库）**

- SMS TPDU 的编解码留在 `pkg/smscodec`：vowifi-go 不能反向依赖 vohive，
  所以 RP-DATA 构造/解析、GSM7 与 UCS2 选择、长短信重组都在这边。
- 出站短信补发 `eventhost.SMSSent` 事件后入库（此前该事件无人 emit，
  发送成功的短信在历史里根本不存在）。
- `UpsertSMSDeliveryPart` 的 part_no 与唯一约束修复。
- 收到引擎的传输已死通知后自动重启隧道。

## 已知限制

- 探活抓的是 socket 级死亡（写失败）。隧道还开着但被黑洞的情况探不出来——
  读侧归 sipgo 所有，看不到 ping 有没有回应。
- 16 条运营商预设中只有 CTExcel UK 那条有实测验证，其余为数据移植。
- 入站 port-s 监听至今没有真正承载过流量：网络侧复用了我们拨出去的连接。
