# syntax=docker/dockerfile:1
# 多阶段构建：构建阶段
FROM golang:1.25-alpine AS build
WORKDIR /src

# 安装 git（go mod download 可能需要）
RUN apk add --no-cache git ca-certificates tzdata

# 先复制依赖文件，利用 Docker 缓存
COPY go.mod go.sum ./
RUN go mod download

# 复制源码并构建
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/aiops-server ./cmd/server

# 运行阶段：最小化镜像
FROM alpine:3.20
WORKDIR /app

# 安装运行时依赖
RUN apk add --no-cache ca-certificates tzdata && \
    adduser -D -u 10001 aiops

# 复制二进制和配置
COPY --from=build /out/aiops-server /app/aiops-server
COPY docs/swagger.json /app/docs/swagger.json
COPY configs/config.example.yaml /app/configs/config.example.yaml
COPY migrations /app/migrations

# 非 root 用户运行
USER aiops

EXPOSE 8080

# 健康检查
HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
    CMD wget --spider -q http://localhost:8080/health || exit 1

ENTRYPOINT ["/app/aiops-server"]
