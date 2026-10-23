# 构建与部署

## 手动构建

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
**构建不再需要访问 `openpaily/paily-fetch`**，只需能拉取通用第三方库
（gin、gorm 等）。

### 单独构建镜像

```bash
docker build --build-arg TARGETARCH=arm64 -t paily-on-nas/core:local .
```

> **`docker-compose.yml` 默认使用现成镜像，不自动构建**
>
> `core` 服务配置的是 `image: paily-on-nas/core:local` 而非 `build:`。
> 原因是 compose 的 `build.context` 需要**完整仓库源码**（`go.mod`、`internal/`、
> `third_party/` 等），而实际部署目录里通常只有二进制和配置文件。
>
> | 场景 | 做法 |
> | --- | --- |
> | 已有镜像（默认） | `docker build -t paily-on-nas/core:local .` 先构建好，再 `docker compose up -d` |
> | 想用 compose 直接构建 | 取消 `docker-compose.yml` 里 `build:` 段的注释，并确保仓库源码在 context 内 |

### 不用 Docker 直接跑

```bash
cp config.example.yaml config.yaml
# 编辑 config.yaml：数据库 DSN、管理员密码、服务密钥、JWT 密钥、checkers
./paily-core -config ./config.yaml
```

> **必须使用 PostgreSQL**
>
> sqlite 驱动（`mattn/go-sqlite3`）需要 cgo。用 `CGO_ENABLED=0` 编译后若配置
> `database.type: sqlite`，启动会报
> `go-sqlite3 requires cgo to work. This is a stub`。
>
> 建议数据库放在 NVMe、至少 2 核 4G 的机器上。

---

## 完整目录结构

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

---

## 各架构的启动方式

二进制是**交叉编译**的，直接用 Release 里对应架构的版本即可，不需要自己重新编译。

```bash
# ARM64（主流 NAS）
docker compose --profile arm64 up -d

# 或者手动指定
docker compose up -d
```

---

## 关键配置速查

| 配置项 | 环境变量 | 说明 |
| --- | --- | --- |
| `database.type` | `PAILY_DATABASE_TYPE` | 固定为 `postgres` |
| `database.dsn` | `PAILY_DATABASE_DSN` | PostgreSQL 连接串 |
| `auth.admin_password` | `PAILY_AUTH_ADMIN_PASSWORD` | 管理端登录密码 |
| `auth.service_secret` | `PAILY_AUTH_SERVICE_SECRET` | fetch / check 上报用的共享密钥 |
| `auth.jwt_secret` | `PAILY_AUTH_JWT_SECRET` | JWT 签名密钥（≥32 字符） |
| `checkers[].secret` | — | 每个测活后端的独立密钥（**列表，无法用环境变量覆盖**） |

环境变量优先级高于配置文件，前缀 `PAILY_`，嵌套键的 `.` 替换为 `_`。

完整的启动配置与运行时配置列表见
[configuration-and-api.md](configuration-and-api.md)。

---

## 部署踩坑记录

按真实部署（ARM NAS + Docker）遇到问题的顺序整理。

### 1. 容器网络分裂

改了 `docker-compose.yml` 里的 `name:` 字段后，新旧容器会落在不同网络，
表现为 `lookup core: no such host`。

```bash
docker compose down
docker rm -f $(docker ps -aq --filter name=paily-) 2>/dev/null
docker network ls | grep paily   # 删除残留网络
docker compose up -d
```

### 2. 容器自定义 DNS 破坏服务名解析

给容器配 `dns:` 会绕过 Docker 内嵌 DNS（`127.0.0.11`），导致容器间
`lookup core: NXDOMAIN`。**不要给容器配自定义 dns**。

### 3. 替换二进制报 `Text file busy`

容器正在运行该二进制时无法覆盖。

```bash
docker compose stop core
cp new-binary /path/to/bin/paily-core
docker compose up -d core
```

### 4. 大文件上传到 NAS

SFTP 常规 `UploadFile` 在 28~36 MB 的二进制上会报 "Failure"。可行做法是
分块写入：`Create()` 打开流 → 512 KB~1 MB 分块写入 → 先写临时文件
`/tmp/xxx.tmp` → 手动 `mv` 到目标位置。

### 5. 管理界面容器

nginx 配置做了同源反代（`/api/` → core，`/clash`、`/singbox`、`/base64` 同步代理），
所以界面里复制的订阅链接是相对路径，不需要额外配域名。

前端构建产物放 `ui/dist/`，用 `docker compose --profile ui up -d` 启动。
