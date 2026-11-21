# 分发

本文档介绍公开订阅接口、查询参数、鉴权方式与选节点算法。

## 工作流程

终端用户请求订阅时，Paily 主服务按以下顺序处理：

1. 校验订阅 token（见下节「鉴权」）。
2. 根据 `tag` 参数选择对应测活视角的存活节点缓存；未指定时使用全局缓存。
3. 若提供 `filter_unlock`，按 `filter_mode` 指定的语义过滤节点。
4. 按「分区分源配额」选点（每地区 CF 每 Worker 与非 CF 节点各自限流）。
5. 对选中节点重新命名，并交由对应格式生成器输出。

整个过程读取内存缓存，不会为每次请求扫描完整数据库。缓存会在测活结果入库后按需重建。

## 订阅地址

```text
https://your-domain.example/clash
https://your-domain.example/singbox
https://your-domain.example/base64
```

| 地址 | 输出 | 默认下载文件名 |
| --- | --- | --- |
| `/clash` | Clash YAML | `Paily Connect.yaml` |
| `/singbox` | Sing-box JSON | `Paily Connect.json` |
| `/base64` | Base64 文本 | `Paily Connect.txt` |

## 鉴权

订阅正文包含可直接使用的节点凭据，因此三个地址**都需要订阅 token**（`internal/auth/bearer.go` 的
`SubscribeTokenMiddleware`）。两种传法等价：

```text
/clash?token=<token>                       # 查询参数，Clash 客户端可直接用完整 URL
Authorization: Bearer <token>              # 或放在请求头
```

token 存在数据库 `configs` 表的 `subscribe_token` 键（可在管理界面轮换，无需重启 core），
启动配置 `auth.subscribe_token` 仅作为首次启动的初始值。**该键为空时不设防**，core 启动时会打印
warning 提示订阅已暴露。基础链接可在管理界面的仪表盘和格式配置页面复制。

## 查询参数

| 参数 | 可选值 | 说明 |
| --- | --- | --- |
| `token` | 订阅 token | 见上节「鉴权」；也可改用 `Authorization: Bearer`。 |
| `preset` | `common`、`all`、`other` | **保留但不再影响选点**。地区策略已由「分区分源配额」取代，该参数只传给命名模板。 |
| `tag` | 已配置的测活后端 tag | 仅使用该测活视角最近一次初筛成功的节点。 |
| `filter_unlock` | 以 `\|` 分隔的服务名 | 例如 `netflix\|disney`。 |
| `filter_mode` | `select`、`mark` | 指定流媒体筛选语义，应与 `filter_unlock` 一起显式传入。 |

示例：

```text
/clash?token=<token>
/singbox?token=<token>&tag=hk
/base64?token=<token>&filter_unlock=netflix|disney&filter_mode=select
/clash?token=<token>&filter_unlock=netflix|youtube&filter_mode=mark
```

## 选点算法：分区分源配额

实现见 `internal/distribution/regionquota.go` 的 `SelectRegionQuota`。每次请求都按当前存活池实时计算：

| 步骤 | 规则 |
| --- | --- |
| 1. 丢弃 | 无地区或地区为 `UN` 的节点排除（保证每条都有国旗）；明文 HTTP/SOCKS 默认排除（`dist_allow_plaintext`）。 |
| 2. 分桶 | 每个地区内分成三桶：CF 节点、安全非 CF、自签非 CF。 |
| 3. CF 配额 | 同一 CF Worker 最多 `cf_per_worker_max`（默认 4）个入口——多入口共用一个 Worker 日配额，刷屏只会挤爆 429。 |
| 4. 路径配额 | 其余节点按「同一路径」最多 `dist_per_path_max`（默认 2）个——一台机器的多个端口/UUID 不算冗余。 |
| 5. 非 CF 上限 | 每地区最多 `region_other_max`（默认 16）个非 CF 安全节点。 |
| 6. 自签兜底 | 按 `dist_selfsigned_fallback`：`short`=补满到 `region_other_max`，`empty`=仅地区为空时补，`none`=不补。付费/最高来源的自签由 `dist_allow_paid_selfsigned` 直接算作安全节点。 |
| 7. 排序 | 地区成块输出，地区按「入选节点分数之和」降序；区内按分数降序编号。 |

以上参数均可在管理界面「配置」页实时调整，无需重启。

### Worker 熔断对分发的影响

CF 节点共用同一个 Worker，整体劣化时相对排名不会动，因此由 `worker_health` 表在分发时介入：

| 状态 | 分发行为 |
| --- | --- |
| `open` | 本轮剔除全部 CF 节点（非 CF 不受影响），节点分数不改。 |
| `degraded` / `half_open` | CF 每 Worker 配额降为 1，保留每地区一个代表以便观察恢复。 |
| `healthy` | 正常按 `cf_per_worker_max` 出点。 |

> 旧的 `?preset=` 地区策略（common/all/other）与全局 `priority_ratio` 名额分配已停用：
> 前者只影响命名模板，后者读入后即丢弃。来源优先级改为在地区内部按
> 「付费/最高 > 公益 > 一手/聚合」排序。

## 流媒体筛选

服务名不区分大小写。

| 模式 | 语义 |
| --- | --- |
| `select` | 节点必须解锁全部指定服务，即交集。 |
| `mark` | 节点至少解锁一个指定服务，即并集；节点名称会显示实际解锁的标记。 |

建议始终显式传入 `filter_mode`。若未传入，服务端不会自动应用 `select` 过滤。

## 输出数量与排序

`max_distribute` 决定单次订阅最多输出的节点数，默认 `200`。若存活节点总数不足该值，则全部输出。

输出结果按地区展示顺序排列，同一地区内按评分降序编号。节点名称由命名模板生成，包含解锁标记、等级、国旗、地区、序号和出口标识，详见 [配置与 API](configuration-and-api.md)。

当前实现下，若没有候选节点，接口仍可能返回 HTTP `200` 及空的格式内容。订阅客户端不应仅依据 HTTP 状态码判断订阅是否包含节点。
