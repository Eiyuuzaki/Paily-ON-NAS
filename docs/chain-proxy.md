# 链式代理（CN 入口 → 境外出口）

## 为什么需要它

免费节点从国内移动线路直连时，境外端点经常超时（实测境外入口存活率 ~0.5%，
CN 段入口高数倍）。CN 节点实测延迟 224–267ms，境外直连节点 2.2–4.3s，
差一个数量级。所以 CN 节点不是"出口"，而是**入口/中转**：
设备先连 CN 节点，再由 CN 节点拨到境外落地。

## 自动做了什么

分发时如果本轮有 CN 存活节点，core 会：

1. 把 CN 节点作为**入口**排在最前面
2. 给每个非 CN 出口节点注入 `dialer-proxy: "<CN 入口的节点名>"`
3. CN 节点之间不互相嵌套（避免绕圈）
4. 多个 CN 入口时轮询分配，不让一个入口扛所有流量

本轮没有 CN 存活节点时，输出退化为原来的扁平列表——功能不报错、不影响可用性。

## 客户端怎么用

**mihomo / Clash Verge / ClashMetaForAndroid**
```yaml
proxies:
  - {name: "[vmess] D - 🇨🇳 CN 01", type: vmess, server: ..., port: 10086, ...}
  - {name: "[vless] D - 🇸🇬 SG 01", type: vless, server: ..., port: 443, ...,
     dialer-proxy: "[vmess] D - 🇨🇳 CN 01"}   # ← 关键字段
```
直接导入订阅即可，`dialer-proxy` 会被 mihomo 识别。**不需要手动改配置**。

**sing-box**
```json
{
  "type": "vless", "tag": "SG 01", "server": "...", "server_port": 443,
  "detour": "CN 01"        // ← sing-box 的等价字段
}
```
注意：sing-box 生成器目前只对部分协议输出 `detour`，若订阅里缺少该字段，
说明当前实现还没覆盖到那个协议类型（见下方"已知限制"）。

## 验证方法

```bash
curl -s http://<NAS>:21380/clash | grep -E 'name:|dialer-proxy'
```
应该看到 CN 节点名 + 后面出口节点带 `dialer-proxy: "<CN 节点名>"`。

## 已知限制

1. **只覆盖 clash 格式的 `dialer-proxy`**；sing-box 的 `detour` 尚未在生成器里全协议实现
2. **CN 节点必须真的能出网**——如果 CN 入口本身只能访问国内，链式仍然失败
3. **入口是单点**：CN 节点死了，挂在它上面的出口全部失效（轮询分配可缓解）
4. 链式会增加一跳延迟（CN 入口 RTT），但相比直连超时仍是净收益

## 相关代码

- `internal/distribution/chain.go` — 入口选择与配对逻辑
- `internal/distribution/handler.go` — 分发时注入
- `internal/distribution/chain_test.go` — 5 个单测（含"不改缓存"回归）
