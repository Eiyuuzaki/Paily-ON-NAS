# Paily ON NAS

> **把一台闲置的 NAS，变成你自己的节点聚合中枢。**

在 NAS 上自建节点聚合与订阅分发平台：抓取公开订阅源 → 去重 → 测活 → 评分 → 生成 Clash / Sing-box / Base64 订阅。

![Platform](https://img.shields.io/badge/platform-linux%2Farm64-blue)
![Go](https://img.shields.io/badge/Go-1.24%2B-00ADD8)
![License](https://img.shields.io/badge/license-AGPL--3.0-orange)

---

## ⚠️ 免责声明（请务必阅读）

**本项目仅供个人学习、技术研究与自用实验，严禁用于任何商业用途或对外提供服务。**

1. **本项目不提供、不托管、不分发任何代理节点。** 所有节点均来自互联网上公开可访问的第三方订阅源，
   本项目仅提供聚合、去重、测活与格式转换的技术实现。节点的合法性、安全性、可用性由原始提供方负责，
   本项目作者不拥有、不控制、不背书任何节点。

2. **严禁将本项目用于任何违反所在国家或地区法律法规的用途。** 使用者应自行了解并遵守当地关于
   网络访问、电信业务经营、计算机信息系统安全等方面的全部法律法规。因使用本项目产生的一切后果，
   **由使用者自行承担全部责任**，作者不承担任何直接或间接责任。

3. **严禁对外公开提供服务。** 本项目的订阅接口默认**无任何认证**，任何能访问到端口的人都能获取
   全部订阅内容。请务必仅在**本地内网或受控的私有网络**中使用，**不要做公网端口映射**。
   将本服务暴露到公网属于高风险行为，可能导致服务被滥用、数据泄露，并可能触及法律红线。

4. **本项目按「现状」提供，不附带任何明示或暗示的担保**，包括但不限于对适销性、特定用途适用性、
   无侵权、无缺陷、服务不中断的担保。使用本软件可能造成数据丢失、服务异常、设备负载升高、
   账号被封禁等风险，作者不承担任何责任。

5. **本项目不是上游项目的官方分支。** 它基于 [openpaily](https://github.com/openpaily) 已归档的
   开源代码改造而来，与原作者无任何隶属关系，作者不对上游代码的质量问题负责。

6. 如果你**不同意**上述任何一条，请**立即停止使用并删除本项目全部文件**。

> 简言之：**这是技术学习项目，不是翻墙工具，更不是可以拿来运营的服务。**
> 用它研究 Go 服务端架构、代理协议解析、并发测活调度，是可以的；拿它做生意或对外提供服务，不行。

---

## 项目特色

基于上游 `paily-core` 做了针对性改造，让它真正能在 NAS 上长期跑起来。

### 🔧 解除了上游的"死依赖"

上游 `go.mod` 硬依赖另一个**已归档**的仓库 `paily-fetch`，其中 7 个包被 `paily-core` 直接引用（约 3200 行）：

| 包 | 用途 | 行数 |
| --- | --- | --- |
| `format` | 订阅格式接口 | 2003 |
| `format/base64` | Base64 订阅生成 | 40 |
| `format/clash` | Clash 配置生成 | 132 |
| `format/singbox` | Sing-box 配置生成 | 784 |
| `format/links` | 分享链接输出 | ~800 |
| `internal/format` | 格式注册表 | ~80 |
| `node` | 节点模型与哈希 | 196 |

**这个问题很隐蔽**：只 fork `paily-core` 时构建是能成功的——因为 Go 会去 module proxy 拉 `paily-fetch`。
但一旦 proxy 缓存被清理（Go 官方 proxy 对归档/删除的仓库会失去缓存），或者你换台机器、用 CI 构建，
就会**直接失败**，而你无法修复，因为那个仓库你也没有控制权。

> 实测验证：把 `GOPROXY` 指向无效地址 + 全新 `GOMODCACHE`，原版构建立刻失败。

**本项目的做法**：把这 7 个包内联到 `third_party/paily-fetch/`，改写 import 路径，从 `go.mod` 移除依赖。

| 方案 | 体积 | 文件数 |
| --- | --- | --- |
| 原样 fork | 0.50 MB | 100 |
| `go mod vendor` | **82 MB** | **6146** |
| **本方案（内联 7 个包）** | **0.57 MB** | **101** |

只多 7 万字节，换来彻底的自包含。

### ⏱️ 新增 `Latency` 过滤字段

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

数据来源是 `node_initial_checks` 表中每个节点**最新一次**的初筛延迟。改动涉及 4 个文件：

| 文件 | 改动 |
| --- | --- |
| `internal/filter/env.go` | 新增 `Latency` 字段 |
| `internal/cache/rebuild.go` | 新增 A4b 查询，取最新初筛延迟 |
| `internal/api/handler/filter.go` | 预览接口补上该字段 |
| `internal/api/handler/search.go` | 搜索接口 + `loadNodeMaps` 补上该字段 |

现在可以这样用了：

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

没有延迟过滤，你只能二选一：全都要（一堆几百毫秒到几秒的混合），或者不要（自己做不了筛选）。

### 🐳 修正了无法使用的 Dockerfile

上游 `Dockerfile` 有三个问题，直接 `docker build` 必定失败：

| 问题 | 说明 |
| --- | --- |
| 路径假设错误 | `COPY paily-core ./paily-core` 假设你在 monorepo 父目录构建 |
| 强制 cgo | `CGO_ENABLED=1` + 安装 `gcc musl-dev`，镜像臃肿且不必要 |
| `--tags="noclash"` | 关闭 clash 支持，但静态编译下这个 tag 并非必需 |

本版本改为标准形式 + `CGO_ENABLED=0` 纯静态编译，镜像更小、ARM 上构建更快。

### 🚀 内置 CI，多架构自动出包

`.github/workflows/build.yml`：

- **每次 push / PR**：`go build ./...` + `go vet ./...`，坏了立刻知道
- **构建 5 个平台**：`linux-arm64`、`linux-amd64`、`linux-armv7`、`darwin-arm64`、`darwin-amd64`
- **打 `v*` 标签**：自动创建 Release，附带全部二进制、`config.example.yaml`、源码快照和 `SHA256SUMS.txt`
- **同时编译配套服务**：`paily-fetch` / `paily-check` 也一并构建发布，下载即用

全程跑在 GitHub 的服务器上，**本地不需要装 Go、不需要设备参与**。

### 📦 明确只支持 PostgreSQL

上游文档写"sqlite 支持尚不完整"，实际是**完全不能用**：

```
go-sqlite3 requires cgo to work. This is a stub
```

用 `CGO_ENABLED=0` 编译（NAS 场景必然如此）后，配 sqlite 直接启动失败。
本项目如实注明，不做误导。

---

## 平台支持：**主要面向 ARM NAS**

| 平台 | 支持 | 说明 |
| --- | --- | --- |
| **`linux/arm64`** | ✅ **主要目标** | 群晖、威联通、fnOS、极空间等主流 NAS；树莓派 4/5、ARM 云服务器 |
| `linux/armv7` | ✅ 支持 | 32 位 ARM 老设备（性能有限，测活并发要调低） |
| `linux/amd64` | ✅ 支持 | x86 NAS / 服务器 / 软路由 |
| `darwin-arm64` / `darwin-amd64` | ✅ 支持 | macOS（Apple Silicon / Intel），适合本地调试 |

### ⚠️ x86 用户请注意

**本项目的配置与调优参数是按 ARM NAS（8 核 / 7.6 GB 内存 / 无独立显卡）设定的**，
包括：

| 参数 | 本项目值 | x86 建议 |
| --- | --- | --- |
| `INITIAL_CONCURRENCY` | 64 | x86 可提到 128~256 |
| `DEEP_PING_CONCURRENCY` | 24 | 可提到 48~96 |
| `FETCH_CONCURRENCY` | 16 | 可提到 32 |
| `ulimits.nofile` | 65536 | 保持不变（这是硬性要求）|
| `FETCH_INTERVAL` | 10800（3 小时） | 可缩到 1800~3600 |
| `CHECK_CRON` | 每 2 小时 | 视 CPU 可缩到 30~60 分钟 |

> **为什么抓取默认 3 小时、测活默认 2 小时？**
> 公开免费源的更新频率本就是几小时到一天，抓太勤没有收益。而测活才是真正的连接大头——
> 每轮要对上千个节点发起 TCP 连接，在家宽上过于频繁可能引起 ISP 侧异常。
> 按 1790 个节点估算：每 2 小时一轮 ≈ 每天 2.1 万次连接；若改成 30 分钟一轮则是 8.6 万次。

**你需要自己修改 `docker-compose.yml` 和 `.env` 中的并发参数。** 具体做法：

```bash
# 编辑 .env
INITIAL_CONCURRENCY=128
DEEP_PING_CONCURRENCY=96
FETCH_CONCURRENCY=32

# 然后重建
docker compose up -d
```

> **重要提醒**：`ulimits.nofile` 千万别调低，也别删。容器默认只有 1024，
> 并发一高就会耗尽文件描述符，表现为**测活进度卡死、CPU 掉到 0%**。
> 本项目在 ARM 上实测过这个坑，调到 128 并发就会触发。

另外，二进制是**交叉编译**的，x86 用户直接用 Release 里的 `linux-amd64` 版本即可，
不需要自己重新编译。

---

## 平台组成

完整平台由三个服务协作，它们之间**通过 HTTP API 通信，没有代码级依赖**：

| 服务 | 职责 | 本仓库 |
| --- | --- | --- |
| **Paily 主服务**（`paily-core`） | 维护源库与节点池，负责去重、存活判定、评分、分发和管理 API | ← **就是本仓库** |
| **Paily Fetch**（`paily-fetch`） | 从源库获取来源，下载订阅、解析内容、校验节点，并上报结果 | 库代码已内联；抓取服务需另行运行 |
| **Paily 测活后端**（`paily-check`） | 从节点池获取节点，执行初筛与复筛，并上报检测结果 | 需另行运行 |
| **管理前端**（`paily-frontend`） | Web 管理界面 | 可选，需另行部署 |

> **重要**：`third_party/` 里内联的是 `paily-fetch` 的**库代码**（格式生成、节点模型）。
> 真正执行抓取的 `paily-fetch` **服务和二进制不在本仓库**，你需要单独运行它。
> 编译期不依赖它，但运行期仍然需要。

所有二进制（含 `paily-fetch` / `paily-check`）都在 [Release](../../releases) 页面提供，按架构下载即可。

---

## 工作流程

```text
  订阅源 / 节点
        │
        ▼
   Paily Fetch        下载 · 识别 · 解析 · 校验
        │
        ▼
   Paily 主服务  ◄────── 管理界面 / 来源提交器
   源库 · 节点池 · 去重 · 历史
        │
        ▼
   Paily 测活后端        初筛 · 复筛（延迟 · 地区 · 流媒体 · 速度）
        │
        ▼
   Paily 主服务         存活判定 · 评分 · 缓存 · 筛选
        │
        ▼
 /clash  /singbox  /base64
```

四个阶段：

1. **提交来源** —— 通过管理界面或 API，把订阅源（`subscribe`）或节点（`node`）写入源库，按类型和内容去重。
2. **抓取入库** —— `paily-fetch` 定期拉取启用的来源，下载、解析、校验后上报；主服务合并重复节点，保留节点与来源的多对多关联。
3. **测活评分** —— `paily-check` 从节点池拉取节点，初筛判断存活，再复筛测量详细延迟、抖动、出口地区、流媒体解锁和可选下载速度；主服务据此更新存活状态并综合评分。
4. **筛选分发** —— 测活结果入库后构建分发缓存，终端用户请求 `/clash` 等接口时按地区策略和评分选节点、重命名、生成订阅。

## 主要能力

- **源库**：区分订阅源和节点，按类型和内容去重，按历史自动裁掉长期失效的来源。
- **节点池**：按协议、地址、端口和密码去重，保留节点与多个来源的对应关系。
- **存活判定与评分**：接收测活结果，判断存活，根据延迟、抖动、稳定性等综合打分。
- **表达式筛选**：按地区、存活状态、协议、来源、评分、**延迟**、流媒体解锁能力筛选。
- **自动命名**：支持 Sponsor 名称与表达式模板生成节点名（含国旗、等级、地区序号）。
- **三种订阅格式**：Clash、Sing-box、Base64，支持按测活视角、地区策略、流媒体能力分发。

---

## 快速开始（Docker Compose，推荐）

**三条命令跑起来**：

```bash
# 1. 准备配置
cp .env.example .env
cp etc/core.yaml.example  etc/core.yaml
cp etc/check.yaml.example etc/check.yaml

# 2. 生成密钥并填入 .env（把输出贴进去，替换 CHANGE_ME_*）
echo "POSTGRES_PASSWORD=$(openssl rand -hex 16)"
echo "PAILY_ADMIN_PASSWORD=$(openssl rand -hex 16)"
echo "PAILY_SERVICE_SECRET=$(openssl rand -hex 24)"
echo "PAILY_JWT_SECRET=$(openssl rand -hex 32)"
echo "CHECKER_SECRET=$(openssl rand -hex 24)"

# 3. 下载配套服务的二进制（Release 页面，按架构选）
#    例如 ARM NAS：
mkdir -p bin
# 从 https://github.com/Eiyuuzaki/Paily-ON-NAS/releases 下载后放入：
#   bin/paily-fetch     (linux-arm64)
#   bin/paily-check     (linux-arm64)
chmod +x bin/paily-fetch bin/paily-check

# 4. 启动
docker compose up -d
```

> **三处密钥必须一致**
> `.env` 的 `CHECKER_SECRET`、`etc/core.yaml` 的 `checkers[0].secret`、
> `etc/check.yaml` 的 `core.secret` —— 这三个值要完全相同。
> 因为 `checkers` 是列表，viper 无法用环境变量覆盖，只能写死在配置文件里。
> 不一致的表现是测活服务一直 401，日志里能看到认证失败。

### 完整目录结构

```text
paily-on-nas/
├── docker-compose.yml           # 四服务编排
├── .env.example                 # 环境变量模板 → 复制为 .env
├── etc/
│   ├── core.yaml.example        # 主服务配置 → 复制为 core.yaml
│   └── check.yaml.example       # 测活配置   → 复制为 check.yaml
├── bin/                         # 配套服务二进制（从 Release 下载）
│   ├── paily-fetch
│   └── paily-check
├── scripts/                     # 解锁检测脚本（可选）
├── ui/
│   ├── dist/                    # 管理界面构建产物（可选）
│   └── nginx.conf               # 管理界面反代配置
└── data/postgres/               # 数据库存储（自动创建）
```

### 各架构的启动方式

```bash
# ARM NAS（群晖 / 威联通 / fnOS / 极空间）—— 默认
docker compose up -d

# x86 NAS / 服务器
TARGETARCH=amd64 docker compose up -d
# 同时把 .env 里的并发参数调高（见「平台支持」一节）

# 带管理界面
docker compose --profile ui up -d      # 访问 http://<NAS-IP>:21381
```

### 常用运维命令

```bash
docker compose ps                      # 状态
docker compose logs -f core            # 主服务日志
docker compose logs -f fetch           # 抓取日志（每轮打印各源结果）
docker compose logs -f check           # 测活日志（有进度条）

docker compose restart fetch           # 立刻触发一轮抓取
docker compose restart check           # 立刻触发一轮测活
docker compose restart core            # 改了运行时配置后需重启重建缓存

# 备份数据库
docker compose exec postgres pg_dump -U paily paily | gzip > paily-$(date +%F).sql.gz

# 完全停止（保留数据）
docker compose down

# 彻底重置（连数据库一起删）
docker compose down && rm -rf data/
```

### 客户端订阅地址

| 地址 | 用途 |
| --- | --- |
| `http://<NAS-IP>:21380/clash` | Clash / mihomo 客户端 |
| `http://<NAS-IP>:21380/singbox` | Sing-box 客户端 |
| `http://<NAS-IP>:21380/base64` | 通用 Base64 订阅 |

可选参数：`?preset=common\|all\|other`（地区策略）、`?tag=local`（测活视角）。

> ⚠️ **这些地址不需要任何认证**，任何能访问到端口的人都能拿到全部订阅。
> 请只在内网使用，**不要做公网端口映射**。见文首免责声明。

---

## 手动构建与运行

需要 **Go 1.24+**。

```bash
go build -o paily-core ./cmd/server
```

交叉编译（NAS / ARM）：

```bash
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 \
  go build -trimpath -ldflags="-s -w" -o paily-core-arm64 ./cmd/server
```

已验证 `go build ./...` 与 `go vet ./...` 均无错误。由于依赖已内联，
**构建不再需要访问 `openpaily/paily-fetch`**，只需能拉取通用第三方库（gin、gorm 等）。

单独构建镜像：

```bash
docker build --build-arg TARGETARCH=arm64 -t paily-on-nas/core:local .
```

> **`docker-compose.yml` 默认使用现成镜像，不自动构建**
>
> `core` 服务配置的是 `image: paily-on-nas/core:local` 而非 `build:`。原因是 compose 的
> `build.context` 需要**完整仓库源码**（`go.mod`、`internal/`、`third_party/` 等），
> 而实际部署目录里通常只有二进制和配置文件。
>
> 两种用法任选：
>
> | 场景 | 做法 |
> | --- | --- |
> | 已有镜像（默认） | `docker build -t paily-on-nas/core:local .` 先构建好，再 `docker compose up -d` |
> | 想用 compose 直接构建 | 取消 `docker-compose.yml` 里 `build:` 段的注释，并确保仓库源码在 context 内 |

> **必须使用 PostgreSQL**
> sqlite 驱动（`mattn/go-sqlite3`）需要 cgo。用 `CGO_ENABLED=0` 编译后若配置
> `database.type: sqlite`，启动会报 `go-sqlite3 requires cgo to work. This is a stub`。
> 建议数据库放在 NVMe、至少 2 核 4G 的机器上。

```bash
cp config.example.yaml config.yaml
# 编辑 config.yaml：数据库 DSN、管理员密码、服务密钥、JWT 密钥、checkers
./paily-core -config ./config.yaml
```

### 关键配置

| 配置项 | 环境变量 | 说明 |
| --- | --- | --- |
| `database.type` | `PAILY_DATABASE_TYPE` | 固定为 `postgres` |
| `database.dsn` | `PAILY_DATABASE_DSN` | PostgreSQL 连接串 |
| `auth.admin_password` | `PAILY_AUTH_ADMIN_PASSWORD` | 管理端登录密码 |
| `auth.service_secret` | `PAILY_AUTH_SERVICE_SECRET` | fetch / check 上报用的共享密钥 |
| `auth.jwt_secret` | `PAILY_AUTH_JWT_SECRET` | JWT 签名密钥（≥32 字符） |
| `checkers[].secret` | — | 每个测活后端的独立密钥（**列表，无法用环境变量覆盖**） |

环境变量优先级高于配置文件，前缀 `PAILY_`，嵌套键的 `.` 替换为 `_`。

### 运行时配置（数据库内，改完需重启生效）

通过管理界面或 `PUT /api/v1/config` 修改。常用项：

| 键 | 默认 | 说明 |
| --- | --- | --- |
| `node_distribute_filter` | 空 | 分发过滤表达式，**返回 `true` 的节点被排除** |
| `node_dead_window_days` | 4 | 节点测活失效判定窗口（天） |
| `node_alive_check_window_minutes` | 240 | 最新初筛的存活窗口（分钟） |
| `fetch_dead_window_days` | 7 | 来源抓取失效判定窗口（天） |
| `max_distribute` | 200 | 单次订阅最多输出节点数 |
| `grade_a_min` / `grade_b_min` / `grade_c_min` | 0.75 / 0.50 / 0.25 | 等级阈值 |

---

## 筛选表达式

基于 [expr-lang](https://expr-lang.org/)，用于节点/来源搜索、过滤预览、分发缓存过滤和命名模板。
布尔表达式应返回 `true` 或 `false`。

**节点可用字段**：

```text
Server, Protocol, Password, Hash, Region, Alive, Score, Latency,
SourceIDs, Sources, RawConfig, Streaming
```

> `Latency` 是本仓库新增的字段（毫秒，-1 表示不可达）。

常用写法：

| 目的 | 表达式 |
| --- | --- |
| 排除中国大陆和未知地区 | `Region == "CN" \|\| Region == "UN"` |
| 只要延迟 ≤800ms | `Latency > 800` |
| 排除低分节点 | `Score < 0.3` |
| 只看这几个地区 | `!(["HK","TW","SG","JP","US"] contains Region)` |
| 组合 | `Region == "CN" \|\| Region == "UN" \|\| Latency > 800` |

**来源可用字段**：`ID, Identifier, Info, Status, Type`

预览表达式（不改动配置）：

```bash
curl -X POST http://127.0.0.1:21380/api/v1/filter/preview \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"expr":"Latency > 800","target":"node"}'
```

修改分发过滤器：

```bash
# 注意 body 是扁平 KV 映射，不是 {"key":..,"value":..}
curl -X PUT http://127.0.0.1:21380/api/v1/config \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"node_distribute_filter":"Region == \"CN\" || Latency > 800"}'
# 之后需重启 core 触发缓存重建
```

---

## 管理界面

`paily-frontend` 提供 Web 控制台（需单独部署）：仪表盘、来源管理、节点管理、
Sponsor、筛选引擎预览、运行时配置、格式配置、日志。

---

## 开发

接口采用 **spec-first**：`api/openapi/spec.yaml` 是 API 契约的唯一来源，服务端代码由 `oapi-codegen` 生成。

```bash
go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.6.0
oapi-codegen -config oapi-codegen.yaml api/openapi/spec.yaml
```

输出到 `internal/api/generated/server.gen.go`（Gin 路由注册、请求模型、`ServerInterface`）。
当前生成文件由 `v2.6.0` 生成，建议使用相同版本以避免无关差异。

约定：

- 不要手动修改 `internal/api/generated/` 下的生成文件。
- 业务实现在 `internal/api/handler/`，实现生成的 `ServerInterface`。
- 变更接口时先改 `spec.yaml`，再生成，最后补齐 handler 并通过 `go build ./...`。

### CI

`.github/workflows/build.yml`：

- 每次 push / PR：`go build ./...` + `go vet ./...`
- 构建 5 个平台：`linux-arm64`、`linux-amd64`、`linux-armv7`、`darwin-arm64`、`darwin-amd64`
- 打 `v*` 标签：自动创建 Release，附带二进制、`config.example.yaml`、源码快照和 `SHA256SUMS.txt`

## 常见故障排查

以下是实际部署中踩过的坑，按出现频率排序。

### 1. `paily-fetch` 报 `lookup core: no such host` / `connection refused`

**症状**：抓取日志显示 `dial tcp: lookup core on 127.0.0.11:53: no such host`，
或 `dial tcp 172.20.0.3:21380: connect: connection refused`。

**原因是两个容器不在同一 Docker 网络**。常见于修改过 compose 的 `name:` 字段（项目名）
之后——compose 会用新项目名创建新网络，但旧容器还挂在旧网络上。

**诊断**：

```bash
for c in paily-core paily-fetch paily-check paily-postgres; do
  printf "%-16s " $c
  docker inspect $c --format '{{range $k,$v := .NetworkSettings.Networks}}{{$k}}={{\$v.IPAddress}} {{end}}'
done
```

所有容器应该在**同一个网络名**下（例如都在 `paily-on-nas_paily`）。

**修复**：彻底删除后重建

```bash
docker rm -f paily-core paily-fetch paily-check paily-postgres paily-ui
docker network rm <旧网络名>
docker compose up -d
```

> **不要给容器指定自定义 `dns:`**。Docker 内嵌 DNS（`127.0.0.11`）负责解析服务名，
> 一旦在 compose 里写死外部 DNS，容器就**无法解析 `core` 这类服务名**，抓取直接失败。
> 如果确实有 DNS 问题，应该修宿主机的 `/etc/resolv.conf`，而不是给容器加 `dns:`。

### 2. 订阅能打开但一个节点都没有

依次排查：

```bash
# a) 确认有存活节点
docker compose exec -T postgres psql -U paily -d paily \
  -tAc "select count(*) from nodes where alive"

# b) 看当前过滤规则
curl -s http://127.0.0.1:21380/api/v1/config \
  -H "Authorization: Bearer $TOKEN" | grep node_distribute_filter

# c) 看缓存重建结果（重启 core 后观察日志）
docker compose logs core | grep "rebuild complete"
```

**最常见原因是 `node_distribute_filter` 里的延迟阈值太严**。公开免费节点的延迟普遍在
1~5 秒，设 `Latency > 800` 会把几乎所有节点滤掉。参考实测分布（1790 节点、51 个存活）：

| 阈值 | 剩余节点 |
| --- | --- |
| ≤1000ms | 3 |
| ≤2000ms | 9 |
| ≤3000ms | 14 |
| ≤5000ms | 24 |

建议先用 `Latency > 5000` 或干脆去掉延迟条件，确认链路通了再逐步收紧。

> 注意：`/clash` 在无候选节点时**仍返回 HTTP 200 和空内容**，不能只看状态码。

### 3. 客户端导入订阅报 `FormatException: Scheme not starting with alphabetic character`

**复制地址时混入了不可见字符**（空格、零宽空格 U+200B、BOM）。报错里的 `^` 会指向
那个字符的位置。手动重新输入一遍地址即可，或先粘到备忘录清掉首尾空白再复制。

### 4. 手机连不上但 PC 正常

先确认手机和 NAS 在同一网段（比较 IP 前三位）。若相同仍不通，检查路由器的
**AP 隔离 / 客户端隔离 / 2.4G-5G 隔离**是否开启——这些选项会阻断同网段设备互访。

**快速验证**：用手机浏览器直接访问订阅地址。能下载文件说明网络通，问题在客户端配置；
打不开则是网络隔离，需在路由器侧关闭隔离。

### 5. 测活进度卡住不动、CPU 掉到 0%

**容器文件描述符耗尽**。默认 `ulimit -n` 只有 1024，高并发时会把 fd 用光。
本项目的 `docker-compose.yml` 里已给 `check` 服务加上 `ulimits.nofile: 65536`，**不要删**。

若仍需提高并发，同时观察 `INITIAL_CONCURRENCY` 和 fd 上限，两者要匹配。

## 文档

| 文档 | 内容 |
| --- | --- |
| [源库与来源管理](docs/source-management.md) | 订阅源与节点、源库去重、死源判定、管理界面操作 |
| [节点池](docs/node-pool.md) | 节点去重、节点与来源的关联、存活判定 |
| [评分机制](docs/scoring.md) | 如何根据节点表现计算评分 |
| [分发](docs/distribution.md) | 订阅地址、查询参数、地区策略与选节点算法 |
| [订阅模板](docs/templates.md) | 分发时的订阅模板配置与制作方法 |
| [配置与 API](docs/configuration-and-api.md) | 启动配置、动态参数、筛选引擎、命名与 Sponsor、API 权限 |
| [自动化路线](docs/automation-roadmap.md) | 哪些自动、哪些手动，以及未来可做的自动化方向 |

## 致谢与来源

- 上游项目：[openpaily/paily-core](https://github.com/openpaily/paily-core) / [paily-fetch](https://github.com/openpaily/paily-fetch) / [paily-check](https://github.com/openpaily/paily-check) / [paily-frontend](https://github.com/openpaily/paily-frontend)（作者 OpenPailyProject）
- 测活引擎部分代码源自 [miaospeed](https://github.com/miaokobot/miaospeed)
- 代理协议解析与内核依赖 [mihomo](https://github.com/MetaCubeX/mihomo)

## 许可证

本项目基于 **GNU Affero General Public License v3.0（AGPL-3.0）** 发布，详见 [LICENSE](LICENSE)。

上游作者为 OpenPailyProject；`third_party/paily-fetch/` 中的代码版权归原作者，同样遵循 AGPL-3.0。
按 AGPL-3.0 要求，若你将修改后的版本作为网络服务对外提供，需要公开修改后的源码。

---

> **再次强调**：本项目仅供学习研究，请勿用于商业用途或对外提供服务。
> 使用者须自行承担全部法律与使用风险。