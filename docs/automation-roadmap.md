# Paily ON NAS —— 自动化路线

记录当前的手动部分、可行的自动化方向，以及何时值得动手。

---

## 一、现状：哪些自动，哪些手动

| 环节 | 状态 | 说明 |
| --- | --- | --- |
| 抓取订阅 | ✅ 自动 | `paily-fetch` 每 3 小时一轮 |
| 解析去重 | ✅ 自动 | 按协议+地址+端口+认证去重 |
| 测活评分 | ✅ 自动 | `paily-check` 每 2 小时一轮 |
| 死节点剔除 | ✅ 自动 | `node_dead_window_days=3`，3 天无存活记录即连带源判死 |
| 死源判定 | ✅ 自动 | `fetch_dead_window_days=7`，窗口内抓取全失败即标 `dead` |
| 清理节点关联 | ✅ 自动 | `MarkDeadAndClean` 级联删除孤儿节点 |
| **发现新源** | ❌ **手动** | 需要人往源库里加 |
| 删除长期零产出的源 | ❌ 手动 | 系统只标 `dead`，不删记录 |

**关键事实**：`paily-fetch` 的源码里**没有任何 discovery / crawl / scrape 逻辑**。它只按源库里已登记的清单去下载。原项目的设计就是「有人喂源，平台负责后续」。

---

## 二、当前基线（2026-10-22）

19 个源，节点池 1337，存活 162。

### 存活产出排行

| 源 | 节点数 | 存活 | 存活率 |
| --- | --- | --- | --- |
| ssrsub-ssr | 189 | 72 | **38.1%** |
| zhangkaiitugithub/passcro | 77 | 25 | 32.5% |
| ermaozi/get_subscribe | 489 | 56 | 11.5% |
| peasoft/NoMoreWalls | 515 | 48 | 9.3% |
| kooker-jp | 747 | 47 | 6.3% |
| zhuhaiuk-nodes | 26 | 19 | **73.1%** |
| zhuhaiuk-nodes-txt | 24 | 17 | 70.8% |
| ermaozi01/free_clash_vpn | 10 | 7 | 70.0% |
| oversizexl-all | 66 | 17 | 25.8% |
| oversizexl-mihomo | 90 | 17 | 18.9% |
| snakem982-meta | 63 | 11 | 17.5% |
| kooker-mihomo | 71 | 11 | 15.5% |
| snakem982-v2ray | 62 | 5 | 8.1% |
| v2raynnodes-nodev2ray | 26 | 1 | **3.8%** ← 可考虑删 |

**观察**：存活率和仓库体量**不成正比**。`kooker-jp` 有 747 个节点但只有 6.3% 存活，而 `zhuhaiuk` 只有 26 个节点却有 73% 存活。小仓库反而质量更稳定。

---

## 三、自动化方向（按投入产出排序）

### 方案 A：自动清理（最容易，收益明确）

**做什么**：定期删除「连续 N 天存活率为 0」的源。

**为什么值得**：
- 直接减少测活负担（当前 1337 节点，删掉 `v2raynnodes-nodev2ray` 这类低质源能省 26 个）
- 无需外部依赖，纯数据库查询 + API 调用
- 风险低（删源不删节点历史，可追溯）

**实现**：

```python
# 逻辑示意
for source in sources:
    total, alive = stats(source.id)
    if total > 0 and alive == 0:
        zero_days[source.id] += 1
        if zero_days[source.id] >= 7:   # 连续 7 轮都没存活
            delete(source.id)
```

**频率**：每周一次足够。

---

### 方案 B：自动发现新源（中等难度，收益最大）

**参考实现已存在**：上游归档前留下了 [`openpaily/paicatch2`](https://github.com/openpaily/paicatch2)
（未归档）——Telethon 抓 TG 频道 + WordPress 解析 + `dsrules/` 规则模块 +
Source API 上报，即本方案的完整实现。**但其 dsrules 里登记的源站已全部
死亡**（2026-10-24 实测：freenode.me 站没了、wefound.cc 404、
clashgithub.com 只剩 16 字节空壳、sharkDoor 停在 2023），复用需要重写规则。
TG 频道采集方向仍成立（免费节点真正集散地），成本 = TG 账号 + api_id + MySQL。

**做什么**：定期搜索 GitHub 上新增/活跃的节点仓库，验证后自动加入。

**技术路径**：

1. **GitHub Search API** 按关键词搜（`free nodes clash`、`免费节点 订阅` 等）
2. 过滤：`pushed_at` 在 7 天内、star > 某阈值、排除已用过的
3. **用 GitHub Trees API 拿文件列表**（不要猜路径，这是关键——我这几轮踩过，猜的文件名 90% 是 404）
4. 用 jsdelivr / gh-proxy 测可达性
5. **先小规模试抓**（加入后观察一轮，存活率 > 阈值才保留）

**难点**：
- 源质量参差不齐，需要「先试后留」机制
- GitHub API 限流（未认证 10 次/分钟搜索）
- 有些仓库会挂羊头卖狗肉（名字像节点库，实际是推广页）

**实现成本**：约 200 行 Python，一周内可完成并稳定。

---

### 方案 C：利用 `extract:` 指令跟踪导航页（最省事，覆盖有限）

`paily-fetch` 原生支持：

```text
extract: https://example.com/landing-page
```

它会下载页面，从正文提取**所有 HTTP 地址和代理分享链接**，然后逐个跟进。

**优点**：完全不需要额外代码，登记一个导航页就能自动跟进页面上新出现的订阅链接。

**缺点**：
- 需要先找到「持续更新且列出可用订阅」的导航页，这类页面不多
- 提取出的 URL 质量不可控
- 无法过滤广告链接

**适合**：作为方案 B 的补充，不是替代。

---

### 方案 D：健康度告警（辅助）

当节点池总量或存活数跌破阈值时通知（邮件 / Telegram / Webhook）。

**为什么有用**：让你知道「该补源了」，而不是靠记忆定期检查。

**实现**：一个定时脚本，调 `/api/v1/stats`，对比阈值。

---

## 四、建议的实施顺序

| 阶段 | 内容 | 何时做 |
| --- | --- | --- |
| 1 | **方案 A**（自动清理） | 想省事时，随时可做 |
| 2 | **方案 D**（健康告警） | 和 A 一起做，代码量小 |
| 3 | **方案 B**（自动发现） | 节点池明显萎缩时 |

**当前是否急需**：不急需。19 个源、1337 节点、162 存活，能稳定跑很久。免费源的波动主要影响「存活数量」，而存活数量的下限由源的质量决定，不是由源的**数量**决定——加更多垃圾源不会提升存活数。

---

## 五、几个必须知道的坑

### 0. 手动加源 SOP（2026-10-24 实操验证）

搜索 → Trees API 看真实文件 → NAS 上 jsdelivr 验证 → core API 入库：

```bash
# 1. 验证（在 NAS 上跑，确认格式与节点量，避开链接农场/HTML）
curl -sL https://fastly.jsdelivr.net/gh/<owner>/<repo>@main/<file> | head -c 300

# 2. 入库（secret 在 /vol1/1000/paily/.env 的 PAILY_SERVICE_SECRET）
SECRET=$(grep '^PAILY_SERVICE_SECRET=' /vol1/1000/paily/.env | cut -d= -f2)
curl -X POST -H "Authorization: Bearer $SECRET" -H 'Content-Type: application/json' \
  -d '{"type":"subscribe","identifier":"<唯一名>","info":"<备注>","content":"<jsdelivr URL>"}' \
  http://127.0.0.1:21380/api/v1/sources        # 201=新建 200=已存在

# 3. 立即抓一轮（不等 3h 定时）：docker compose restart fetch
```

**选源口径**（本节实测沉淀）：文件 5~100KB、节点 20~200 个为宜；
节点名带实测速度的仓库（如 shaoyouvip/free）质量更高；
`stable/verified` 类精选目录 > 全量聚合；**每天轮换外部链接的仓库不要收**
（如 bq2015/FreeProxies 实为 blue2sea.com 引流，URL 每日变，抓不到）。

本轮新增 4 源（+315 节点）：pawdroid/Free-servers、shaoyouvip/free
（mihomo+all）、Au1rxx/free-vpn-subscriptions 的 stable 分片。
第二轮 +1：gslege/CloudflareIP Vless.txt（CF 优选 vless，20 节点；
其 All/JP/SG.txt 是裸优选 IP 清单，无协议凭证，不可收）。
第三轮 +2（**两跳策略**：不自己爬 TG，消费已爬 TG 的聚合器，
免费继承其采集能力）：mahdibland/V2RayAggregator 的
`sub/airport_sub_merge.txt`（9.6KB 机场过滤子集，61 链接；全量
sub_merge 1MB 红线不碰）、bin1site1/V2rayFree `config.txt`
（17.5KB，15 分钟一更；raw 直链 NAS 不通，走 jsdelivr 有缓存延迟）。
另：原项目组的三网优化订阅与其爬虫源站已全部死亡（见方案 B 注），不可复现。
mahdibland 节点入口可见 `gzdx.`（广州电信）前缀——三网优化的活样本，
从移动线路测活即自动完成三网择优。


### 1. 不要用「猜路径」的方式找源

我这几轮实测：凭经验猜 `clash.yaml`、`sub.txt` 这类路径，**命中率不到 20%**。正确做法是先用 GitHub Trees API 拿真实文件列表：

```bash
curl -s "https://api.github.com/repos/<owner>/<repo>/git/trees/<branch>?recursive=1" \
  | jq -r '.tree[] | select(.type=="blob" and .size>2000) | .path'
```

### 2. 镜像选择：jsdelivr > gh-proxy

实测 `fastly.jsdelivr.net` 比 `gh-proxy.com` 更稳定，且 **gh-proxy 会截断大文件**（`anaer/Sub` 经 gh-proxy 只有 169KB，jsdelivr 有 452KB）。

### 3. 大文件不等于高质量

`0xRadikal/Free-v2ray-Configs` 单文件 4.3MB，加入 9155 个节点，**存活产出 6 个**（0.07%），还把测活时间从 3.8 分钟拖到 25 分钟。

**教训**：加入前先用样本估算存活率，别一次性灌进来。

### 4. 不要给容器指定自定义 `dns:`

会导致容器无法解析 `core` 这类服务名。Docker 内嵌 DNS（127.0.0.11）必须保留。详见 README 故障排查第 1 条。

---

## 六、如果以后想动手

把这个文档直接给我，说「按方案 A 做」即可。需要的上下文（源列表 API、数据库结构、当前阈值）都在这个会话里，不用重新摸索。
