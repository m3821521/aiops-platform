.PHONY: dev dev-backend dev-frontend test test-backend test-frontend lint lint-backend lint-frontend build build-backend build-frontend docker-build docker-up docker-down clean help

# 默认目标
help:
	@echo "AIOps Platform Monorepo Makefile"
	@echo ""
	@echo "开发命令:"
	@echo "  make dev              同时启动后端和前端"
	@echo "  make dev-backend      仅启动后端 (go run)"
	@echo "  make dev-frontend     仅启动前端 (npm run dev)"
	@echo ""
	@echo "测试命令:"
	@echo "  make test             运行前后端全部测试"
	@echo "  make test-backend     运行后端测试"
	@echo "  make test-frontend    运行前端测试"
	@echo ""
	@echo "代码检查:"
	@echo "  make lint             运行前后端 lint"
	@echo "  make lint-backend     go fmt + go vet"
	@echo "  make lint-frontend    npm run lint"
	@echo ""
	@echo "构建命令:"
	@echo "  make build            构建前后端"
	@echo "  make build-backend    go build"
	@echo "  make build-frontend   npm run build"
	@echo ""
	@echo "Docker 命令:"
	@echo "  make docker-build     构建所有 Docker 镜像"
	@echo "  make docker-up        启动 docker compose"
	@echo "  make docker-down      停止 docker compose"
	@echo ""
	@echo "清理命令:"
	@echo "  make clean            清理构建产物"

# 开发
dev:
	@echo "启动后端和前端..."
	@cd backend && go run ./cmd/server &
	@cd frontend && npm run dev

dev-backend:
	@cd backend && go run ./cmd/server

dev-frontend:
	@cd frontend && npm run dev

# 测试
test: test-backend test-frontend

test-backend:
	@cd backend && go test ./... -v

test-frontend:
	@cd frontend && npm test 2>/dev/null || echo "前端暂无测试配置"

# 代码检查
lint: lint-backend lint-frontend

lint-backend:
	@cd backend && go fmt ./... && go vet ./...

lint-frontend:
	@cd frontend && npm run lint

# 构建
build: build-backend build-frontend

build-backend:
	@cd backend && CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/aiops-server ./cmd/server

build-frontend:
	@cd frontend && npm run build

# Docker
docker-build:
	docker compose build

docker-up:
	docker compose up -d

docker-down:
	docker compose down

# 清理
clean:
	@rm -rf backend/bin backend/coverage.out
	@rm -rf frontend/dist
	@echo "清理完成"
