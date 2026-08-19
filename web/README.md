# AIOps 前端

企业级 AIOps 智能运维平台前端，基于 React + TypeScript + Vite + Ant Design。

## 技术栈

- React 18 + TypeScript
- Vite 5
- Ant Design 5
- React Router 6
- Axios
- Zustand（状态管理）
- ECharts（图表）
- Day.js（时间处理）

## 快速开始

```bash
# 安装依赖
npm install

# 启动开发服务器（默认 5173 端口，代理 /api 到后端 8080）
npm run dev

# 构建
npm run build

# 代码检查
npm run lint

# 预览构建产物
npm run preview
```

## 目录结构

```
src/
├── api/          # API 层（按模块拆分）
├── components/   # 通用组件
├── layouts/      # 布局组件
├── pages/        # 页面
├── router/       # 路由配置 + 权限守卫
├── stores/       # Zustand 状态管理
├── styles/       # 全局样式
├── types/        # TypeScript 类型定义
├── utils/        # 工具函数
├── hooks/        # 自定义 Hooks
├── App.tsx
└── main.tsx
```

## 开发阶段

- Phase 1: 前端基础框架（当前）
- Phase 2: Dashboard
- Phase 3: Kubernetes
- Phase 4: Prometheus Monitoring
- Phase 5: Alert
- Phase 6: Logs
- Phase 7: AIOps（Anomaly / RCA / Topology）
- Phase 8: AI Assistant
- Phase 9: Automation
- Phase 10: Jenkins / ArgoCD
- Phase 11: RBAC / Audit
- Phase 12: 整体优化

## 后端代理

Vite 开发服务器已配置代理：

- `/api` → `http://localhost:8080`
- `/health` → `http://localhost:8080`
- `/ready` → `http://localhost:8080`
- `/metrics` → `http://localhost:8080`

确保后端服务在 8080 端口运行。
