# syntax=docker/dockerfile:1
# P1-X.10 Phase 2.1: Unified Single-Port Production Image
# Multi-stage build: frontend build + backend build → single runtime image
# Production: Go server on :8080 serves both API and React SPA.
# Build context must be project root (./), not ./backend.

# ============================================================================
# Stage 1: Frontend build
# ============================================================================
FROM node:20-alpine AS frontend-build
WORKDIR /app/frontend

# 先复制依赖文件，利用 Docker 缓存
COPY frontend/package.json frontend/package-lock.json* ./
RUN npm ci

# 复制源码并构建
COPY frontend/ .
RUN npm run build

# ============================================================================
# Stage 2: Backend build
# ============================================================================
FROM golang:1.25-alpine AS backend-build
WORKDIR /src/backend

# 安装 git（go mod download 可能需要）
RUN apk add --no-cache git ca-certificates tzdata

# 先复制依赖文件，利用 Docker 缓存
COPY backend/go.mod backend/go.sum ./
RUN go mod download

# 复制源码并构建
COPY backend/ .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/aiops-server ./cmd/server

# ============================================================================
# Stage 3: Production runtime (single port :8080)
# ============================================================================
FROM alpine:3.20
WORKDIR /app

# 安装运行时依赖
RUN apk add --no-cache ca-certificates tzdata && \
    adduser -D -u 10001 aiops

# 复制后端二进制和配置
COPY --from=backend-build /out/aiops-server /app/aiops-server
COPY backend/docs/swagger.json /app/docs/swagger.json
COPY backend/configs/config.example.yaml /app/configs/config.example.yaml
COPY backend/migrations /app/migrations

# P1-X.10 Phase 2: 复制 frontend dist，由 Go server 托管
COPY --from=frontend-build /app/frontend/dist /app/frontend/dist

# 设置 FRONTEND_DIST_DIR，Go server 的 resolveFrontendDistDir() 会读取此环境变量
ENV FRONTEND_DIST_DIR=/app/frontend/dist

# 非 root 用户运行
USER aiops

# 唯一 HTTP 端口
EXPOSE 8080

# 健康检查
HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
    CMD wget --spider -q http://localhost:8080/health || exit 1

ENTRYPOINT ["/app/aiops-server"]
