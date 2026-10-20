# ── 构建阶段 ──────────────────────────────────────────────────────────────────
# 本仓库已内联 paily-fetch，构建上下文就是仓库根目录：
#   docker build -t paily-core .
#
# 使用 CGO_ENABLED=0 静态编译，因此只支持 PostgreSQL（sqlite 驱动需要 cgo）。
FROM golang:1.24-alpine AS builder

# git 用于部分模块解析；ca-certificates 用于 HTTPS 拉取依赖
RUN apk add --no-cache git ca-certificates

WORKDIR /src

# 先复制依赖清单，利用层缓存
COPY go.mod go.sum ./
RUN go mod download

# 复制源码（含 third_party）
COPY . .

# 静态编译；架构由 --build-arg 决定，默认 arm64
ARG TARGETOS=linux
ARG TARGETARCH=arm64
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/paily-core ./cmd/server

# ── 运行阶段 ──────────────────────────────────────────────────────────────────
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /out/paily-core /app/paily-core

# 配置需挂载：-v $(pwd)/config.yaml:/app/config.yaml:ro
EXPOSE 21380

ENTRYPOINT ["/app/paily-core", "-config", "/app/config.yaml"]
