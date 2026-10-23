# 相对上游的改动

本仓库不是上游 `paily-core` 的简单 fork。上游已归档，且有几处问题让它无法直接用于
NAS 长期运行。下面按重要性列出改动内容与原因。

---

## 1. 解除了上游的「死依赖」

上游 `go.mod` 硬依赖另一个**已归档**的仓库 `paily-fetch`，其中 7 个包被
`paily-core` 直接引用（约 3200 行）：

| 包 | 用途 | 行数 |
| --- | --- | --- |
| `format` | 订阅格式接口 | 2003 |
| `format/base64` | Base64 订阅生成 | 40 |
| `format/clash` | Clash 配置生成 | 132 |
| `format/singbox` | Sing-box 配置生成 | 784 |
| `format/links` | 分享链接输出 | ~800 |
| `internal/format` | 格式注册表 | ~80 |
| `node` | 节点模型与哈希 | 196 |

**这个问题很隐蔽。** 只 fork `paily-core` 时构建能成功——因为 Go 会去 module proxy
拉 `paily-fetch`。但一旦 proxy 缓存被清理（Go 官方 proxy 对归档/删除的仓库会失去
缓存），或者你换台机器、用 CI 构建，就会**直接失败**，而你无法修复，因为那个仓库
你也没有控制权。

> 实测验证：把 `GOPROXY` 指向无效地址 + 全新 `GOMODCACHE`，原版构建立刻失败。

**本项目的做法**：把这 7 个包内联到 `third_party/paily-fetch/`，改写 import 路径，
从 `go.mod` 移除依赖。

| 方案 | 体积 | 文件数 |
| --- | --- | --- |
| 原样 fork | 0.50 MB | 100 |
| `go mod vendor` | **82 MB** | **6146** |
| **本方案（内联 7 个包）** | **0.57 MB** | **101** |

只多 7 万字节，换来彻底的自包含。

---

## 2. 新增 `Latency` 过滤字段

上游的分发过滤器**不支持按延迟筛选**——`NodeFilterEnv` 里根本没有延迟字段，
写 `Latency > 800` 会直接编译报错。

本版本补上了：

```go
// internal/filter/env.go
type NodeFilterEnv struct {
    ...
    Latency int // 最新一次初筛的延迟（毫秒），-1 = 不可达
}
```

数据来源是 `node_initial_checks` 表中每个节点**最新一次**的初筛延迟：

| 文件 | 改动 |
| --- | --- |
| `internal/filter/env.go` | 新增 `Latency` 字段 |
| `internal/cache/rebuild.go` | 新增查询，取最新初筛延迟 |
| `internal/api/handler/filter.go` | 预览接口补上该字段 |
| `internal/api/handler/search.go` | 搜索接口 + `loadNodeMaps` 补上该字段 |

现在可以这样用：

```text
Region == "CN" || Region == "UN" || Latency > 800
```

**为什么需要它**：公开免费节点的延迟中位数在 1000ms 以上。实测 93 个存活节点的分布：

| 阈值 | 剩余节点 |
| --- | --- |
| ≤300ms | 4 个 |
| ≤500ms | 8 个 |
| ≤800ms | 17 个 |
| ≤1000ms | 26 个 |
| 不限 | 93 个 |

没有延迟过滤，你只能二选一：全都要（一堆几百毫秒到几秒的混合），或者不要。

---

## 3. 修正了无法使用的 Dockerfile

上游 `Dockerfile` 有三个问题，直接 `docker build` 必定失败：

| 问题 | 说明 |
| --- | --- |
| 路径假设错误 | `COPY paily-core ./paily-core` 假设你在 monorepo 父目录构建 |
| 强制 cgo | `CGO_ENABLED=1` + 安装 `gcc musl-dev`，镜像臃肿且不必要 |
| `--tags="noclash"` | 关闭 clash 支持，但静态编译下这个 tag 并非必需 |

本版本改为标准形式 + `CGO_ENABLED=0` 纯静态编译，镜像更小、ARM 上构建更快。

---

## 4. 内置 CI，多架构自动出包

`.github/workflows/build.yml`：

- **每次 push / PR**：`go build ./...` + `go vet ./...`，坏了立刻知道
- **构建 5 个平台**：`linux-arm64`、`linux-amd64`、`linux-armv7`、`darwin-arm64`、`darwin-amd64`
- **打 `v*` 标签**：自动创建 Release，附带全部二进制、`config.example.yaml`、
  源码快照和 `SHA256SUMS.txt`
- **同时编译配套服务**：`paily-fetch` / `paily-check` 也一并构建发布，下载即用

全程跑在 GitHub 的服务器上，本地不需要装 Go。

---

## 5. 明确只支持 PostgreSQL

上游文档写"sqlite 支持尚不完整"，实际是**完全不能用**：

```
go-sqlite3 requires cgo to work. This is a stub
```

用 `CGO_ENABLED=0` 编译（NAS 场景必然如此）后，配 sqlite 直接启动失败。
本项目如实注明，不做误导。

---

## 6. 重新设计的 Web 管理界面（Material Design 3）

原版前端用的是 Element Plus，界面比较朴素。本版本整体改写为
**Vuetify 4 + Material Design 3**：

- **明暗双主题**，一键切换，偏好存 localStorage
- **响应式布局**：桌面常驻侧边栏（可折叠为 navigation rail），手机自动折叠为抽屉
- **图标零负担**：用 `@mdi/js` 的 SVG path 按需引入，替代 3.5MB 的 MDI 字体
  （构建产物从 4.82 MB 降到 1.08 MB）
- **完整的 MD3 色阶系统**：surface-container 五级分层、tonal 容器、on-* 前景色，
  暗色模式带独立的 tonal palette
- 仪表盘增加**节点存活率环形图**与**实际流量分布条**（机场 vs 公益）
- 首页新增「关于」页，包含项目介绍、技术栈与完整免责声明

### 踩过的坑（供参考）

| 现象 | 原因 |
| --- | --- |
| 所有图标在暗色模式下是黑块 | 自定义 SVG 图标集的 `<path>` 未设 `fill="currentColor"`，浏览器默认填黑 |
| 部分图标消失 | `v-app-bar-nav-icon` 等组件用 `$menu` 等别名，需在 `icons.aliases` 显式映射 |
| `color="on-surface-variant"` 无效 | Vuetify 的 `color` prop 只认白名单 token，`on-*` 不在其中，需用内联 CSS 变量 |
| 折叠成 rail 后选中项是拉伸椭圆 | `rounded="full"` 不存在，药丸圆角是 `pill`；且 rail 需固定 56×32 + 16px 圆角 |

---

## 7. 来源优先级 + 机场额度

这是相对上游最实用的两个新增能力。

### 来源优先级

每个来源可设 `priority`（0 公益 / 1 机场 / 2 最高）。分发时按 `priority_ratio`
（默认 0.6）分配名额——高优先级来源先占 60% 的位置，不足或超出的部分再由公益节点
补齐。机场节点不再被海量免费节点稀释。

```
priority_ratio = 0.6  →  订阅中约 60% 是机场节点，40% 是公益节点
```

### 机场额度

抓取订阅时解析 HTTP 响应头的 `subscription-userinfo`（sing-box 标准）：

```
subscription-userinfo: upload=0; download=0; total=58339622912; expire=2108019779
```

解析出的已用 / 总量 / 到期时间会存进来源表，在管理界面的「额度」页与列表进度条中
展示。

> 这只能读取订阅头里携带的信息；大多数机场接口不返回额度，此时该字段为空，属正常。

### 机场自动识别

`subscription-userinfo` 是付费机场的事实标准，公益订阅基本不会返回它。因此
**只要抓取时收到该头，就自动把该源的 priority 提升到 1**。

判定条件很保守：

```sql
UPDATE sources SET priority = 1 WHERE id = ? AND priority = 0
```

`priority = 0` 这个 guard 是核心——**用户手动标记过的源永不被覆盖**（包括手动设为
2 的"最高"），重复抓取也是幂等的。

> **局限**：不带额度头的机场不会被识别，需要导入后在前端手动切换优先级。
> 这是有意的取舍——误判的代价比漏判高。

### 节点存活可视化

来源列表展示「存活 / 抓取」数量（如 `15 / 747`），存活数加粗着色，0 存活显示为
红色。详情抽屉里有存活率进度条。

---

## 8. 实际流量统计（可选）

内置 `traffic reporter`，定期轮询本机 mihomo 的 `/proxies` 接口，把每个节点的
累计上传/下载按来源聚合，在仪表盘展示"机场 vs 公益"的**实际**流量占比。

默认关闭，需在 `.env` 里指定：

```bash
MIHOMO_API_URL=http://172.17.0.1:9090
MIHOMO_API_SECRET=你的mihomo密钥
```

`172.17.0.1` 是 Docker 网桥对宿主机的地址，适用于 mihomo 跑在 NAS 宿主机的场景。

### 为什么默认没有数据

只有**经过这个 mihomo 的流量**才会被统计。如果手机/电脑用「订阅模式」
（直连节点，不经 NAS），流量不经过 mihomo，仪表盘就只会显示"暂无流量数据"。
这是正常现象，不是故障。

### 让它开始统计

**方式一：设备把 NAS 当代理（推荐，统计最准）**

在 mihomo 配置里开启混合端口，然后让设备的代理指向 NAS：

```yaml
# mihomo config.yaml
mixed-port: 7890
allow-lan: true          # 允许局域网设备连进来
external-controller: 0.0.0.0:9090   # traffic reporter 要读这个
```

设备侧（以手机为例）：Wi-Fi 高级设置 → 手动代理 → 填 `NAS-IP` + `7890`。

**方式二：旁路由模式**

把 NAS mihomo 设为网关，DNS 也指向它，所有流量强制过网。适合想让全家设备都走统计
的场景，但配置成本更高。

### 检查是否生效

```bash
# 1. mihomo 的 API 通不通（把 secret 换成你的）
curl -H "Authorization: Bearer <secret>" http://127.0.0.1:9090/version

# 2. core 的 traffic reporter 有没有起来
docker compose logs core | grep "traffic reporter"
# 期望看到：traffic reporter: starting url=http://172.17.0.1:9090
```

看到 reporter 启动日志但仪表盘仍无数据，说明 mihomo 正常、只是还没流量经过。

> mihomo 的 `secret` 必须和 `MIHOMO_API_SECRET` 一致，否则 reporter 会一直 401，
> 仪表盘永远为空。没有设 secret 就两边都留空。

界面上也能看：仪表盘的流量卡右上角有 ⓘ 按钮（窄屏是折叠面板），点开就是同样的
接入指引，且会自动区分「未配置」和「已配置但无流量」两种情况。
