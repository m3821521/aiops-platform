# AIOps Platform

企业级智能运维平台，基于 Go + React 构建，支持 Kubernetes 监控、Prometheus 指标、告警管理、异常检测、根因分析（RCA）、ELK 日志分析、AI 运维助手、自动化运维、Jenkins/ArgoCD 集成等完整运维闭环。

## 系统架构

```
                    Internet
                        │
                        ▼
                    Ingress / Nginx
                    /          \
                   /            \
                  ▼              ▼
             Frontend         Backend (Go/Gin)
             (React/TS)            │
                                   ├── Kubernetes (client-go)
                                   ├── Prometheus
                                   ├── Alertmanager
                                   ├── Elasticsearch (ELK)
                                   ├── MySQL (GORM)
                                   ├── Redis
                                   ├── Jenkins
                                   └── ArgoCD
```

**运维闭环：**

```
发现问题 → 查看告警 → 查看指标 → 查看日志 → 查看 K8s
    → 服务拓扑 → RCA 分析 → AI 辅助 → 人工确认 → 自动化处理 → 审计
```

## 技术栈

### 后端
- **语言**: Go 1.25+
- **框架**: Gin
- **ORM**: GORM
- **缓存**: go-redis
- **监控**: Prometheus client_golang
- **K8s**: client-go
- **认证**: JWT + RBAC + bcrypt
- **日志**: slog
- **API 文档**: Swagger/OpenAPI

### 前端
- **框架**: React 18 + TypeScript
- **构建**: Vite 5
- **UI**: Ant Design 5
- **路由**: React Router 6
- **状态**: Zustand
- **HTTP**: Axios
- **图表**: ECharts
- **时间**: Day.js

### 基础设施
- **数据库**: MySQL 8.0
- **缓存**: Redis 7
- **监控**: Prometheus + Grafana + Alertmanager
- **日志**: Elasticsearch + Kibana
- **容器**: Docker + Docker Compose
- **编排**: Kubernetes + Helm
- **CI/CD**: Jenkins + ArgoCD

## 目录结构

```
aiops-platform/
├── backend/                    # Go 后端
│   ├── cmd/server/             # 入口
│   ├── internal/               # 业务代码
│   │   ├── api/                # 路由
│   │   ├── handler/            # HTTP Handler
│   │   ├── service/            # 业务逻辑
│   │   ├── repository/         # 数据访问
│   │   ├── model/              # 数据模型
│   │   ├── middleware/         # 中间件
│   │   ├── config/             # 配置
│   │   ├── cluster/            # K8s 集群管理
│   │   ├── monitoring/         # Prometheus
│   │   ├── alert/              # 告警系统
│   │   ├── anomaly/            # 异常检测
│   │   ├── rca/                # 根因分析
│   │   ├── logging/            # ELK 日志
│   │   ├── ai/                 # AI 助手
│   │   ├── automation/         # 自动化运维
│   │   ├── auth/               # 认证授权
│   │   ├── audit/              # 审计日志
│   │   └── redisutil/          # Redis 工具
│   ├── pkg/                    # 公共包
│   ├── migrations/             # 数据库迁移
│   ├── configs/                # 配置文件
│   ├── docs/                   # Swagger 文档
│   ├── tests/                  # 测试
│   ├── scripts/                # 脚本
│   ├── go.mod
│   ├── Dockerfile
│   └── Makefile
│
├── frontend/                   # React 前端
│   ├── src/
│   │   ├── api/                # API 层
│   │   ├── components/         # 通用组件
│   │   ├── layouts/            # 布局
│   │   ├── pages/              # 页面
│   │   ├── router/             # 路由
│   │   ├── stores/             # Zustand 状态
│   │   ├── types/              # TypeScript 类型
│   │   ├── utils/              # 工具函数
│   │   ├── hooks/              # 自定义 Hooks
│   │   └── styles/             # 全局样式
│   ├── public/
│   ├── Dockerfile
│   ├── nginx.conf
│   ├── package.json
│   └── vite.config.ts
│
├── deployments/                # 部署配置
│   ├── docker/                 # Docker 配置
│   ├── kubernetes/             # K8s Manifest
│   ├── helm/aiops/             # Helm Chart
│   └── argocd/                 # ArgoCD Application
│
├── docs/                       # 项目文档
├── scripts/                    # 根目录脚本
├── .env.example                # 环境变量模板
├── .gitignore
├── Makefile                    # 统一构建入口
├── docker-compose.yaml         # 本地开发环境
├── Jenkinsfile                 # CI/CD Pipeline
└── README.md
```

## 环境要求

- Go 1.25+
- Node.js 20+
- Docker & Docker Compose
- MySQL 8.0
- Redis 7
- (可选) Kubernetes 集群
- (可选) Prometheus / Grafana / ELK

## 快速开始

### 方式一：Docker Compose（推荐）

```bash
# 克隆项目
git clone <repository>
cd aiops-platform

# 复制环境变量
cp .env.example .env

# 启动全部服务
docker compose up -d

# 查看状态
docker compose ps
```

访问：
- 前端: http://localhost
- 后端 API: http://localhost:8080
- 健康检查: http://localhost:8080/health
- Prometheus: http://localhost:9090
- Grafana: http://localhost:3000 (admin/admin123)
- Kibana: http://localhost:5601

### 方式二：本地开发

```bash
# 1. 启动依赖服务（MySQL + Redis）
docker compose up -d mysql redis

# 2. 执行数据库迁移
cd backend
mysql -h 127.0.0.1 -u aiops -paiops123 aiops < migrations/001_init.sql
mysql -h 127.0.0.1 -u aiops -paiops123 aiops < migrations/002_alerts.sql
mysql -h 127.0.0.1 -u aiops -paiops123 aiops < migrations/003_auth.sql
mysql -h 127.0.0.1 -u aiops -paiops123 aiops < migrations/004_audit_logs.sql
cd ..

# 3. 启动后端
make dev-backend
# 或: cd backend && go run ./cmd/server

# 4. 启动前端（新终端）
make dev-frontend
# 或: cd frontend && npm install && npm run dev
```

访问：
- 前端: http://localhost:5173
- 后端: http://localhost:8080

默认账号: `admin / admin123`

## Makefile 命令

```bash
make dev              # 同时启动前后端
make dev-backend      # 仅启动后端
make dev-frontend     # 仅启动前端

make test             # 运行全部测试
make test-backend     # 后端测试
make test-frontend    # 前端测试

make lint             # 代码检查
make lint-backend     # go fmt + go vet
make lint-frontend    # npm run lint

make build            # 构建前后端
make build-backend    # go build
make build-frontend   # npm run build

make docker-build     # 构建 Docker 镜像
make docker-up        # 启动 docker compose
make docker-down      # 停止 docker compose

make clean            # 清理构建产物
```

## API 文档

后端启动后访问 Swagger UI:
- http://localhost:8080/swagger/index.html

主要 API 模块：

| 模块 | 前缀 | 说明 |
|------|------|------|
| 健康检查 | `/health`, `/ready`, `/metrics` | 存活/就绪/指标 |
| Kubernetes | `/api/v1/clusters`, `/nodes`, `/pods`, ... | K8s 资源查询 |
| 指标 | `/api/v1/metrics/query`, `/range`, `/nodes`, `/pods` | Prometheus 查询 |
| 告警 | `/api/v1/alerts`, `/aggregate`, `/noise` | 告警管理 |
| 异常检测 | `/api/v1/anomaly/detect` | 异常检测 |
| RCA | `/api/v1/rca/analyze` | 根因分析 |
| 日志 | `/api/v1/logs/search`, `/analyze` | ELK 日志 |
| AI | `/api/v1/ai/ask` | AI 助手 |
| 自动化 | `/api/v1/automation/pods/...`, `/deployments/...` | 运维操作 |
| Jenkins | `/api/v1/jenkins/jobs`, `/builds`, `/build` | CI/CD |
| ArgoCD | `/api/v1/argocd/apps`, `/sync`, `/refresh` | GitOps |
| 认证 | `/api/v1/auth/login`, `/me`, `/logout` | JWT 认证 |
| 用户 | `/api/v1/users`, `/roles` | 用户角色管理 |
| 审计 | `/api/v1/audit-logs` | 审计日志 |

## 开发规范

### 后端
- 代码格式: `go fmt ./...`
- 静态检查: `go vet ./...`
- 测试: `go test ./...`
- 错误处理: 所有 error 必须处理
- Context: 所有外部调用使用 context.Context
- 配置: 通过 configs/config.yaml 管理，不硬编码

### 前端
- 代码检查: `npm run lint`
- 构建: `npm run build`
- API 调用: 统一通过 src/api/ 层，禁止在组件中直接写 axios
- 状态管理: 全局状态用 Zustand，页面数据局部管理
- 类型: 所有 API 响应定义 TypeScript 类型

## 安全说明

- 所有密码、Token、API Key 通过环境变量注入，不提交到 Git
- JWT Secret 生产环境必须修改
- 数据库密码生产环境必须修改
- Kubernetes kubeconfig / Token 不提交到 Git
- 所有写操作（重启 Pod、扩容、Jenkins 构建、ArgoCD Sync）需要人工确认
- AI 助手默认不执行危险操作
- API 限流基于 Redis，防止滥用
- 审计日志记录所有重要操作

## 部署

### Kubernetes (Helm)

```bash
cd deployments/helm/aiops
helm install aiops . -n aiops --create-namespace
```

### ArgoCD

```bash
kubectl apply -f deployments/argocd/application.yaml
```

## 测试

### 后端测试

```bash
cd backend
go test ./... -v -cover
```

测试覆盖：
- 告警聚合/降噪
- 异常检测算法（静态阈值/MA/EWMA/Z-Score）
- RCA 规则引擎
- 日志分析器
- AI 助手
- 自动化引擎
- 认证/JWT
- 审计日志
- Redis 分布式锁/限流

### 前端测试

```bash
cd frontend
npm run lint
npm run build
```

## Git 工作规范

- Commit Message 格式: `feat:`, `fix:`, `refactor:`, `test:`, `docs:`, `chore:`
- 不直接 push 到 main，通过 PR 合并
- 每个功能独立分支
- 合并前确保测试通过

## License

MIT
