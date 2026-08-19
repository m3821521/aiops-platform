# AIOps Automation Architecture

## Overview

AIOps Platform Automation 模块提供企业级安全自动化框架，支持从 AI 建议到人工审批再到安全执行的完整闭环。

## Core Principles

1. **AI 不能直接执行** — AI 只能 Recommendation，必须经过 Human Approval
2. **四眼原则** — Requester != Approver
3. **Dry Run 前置** — Execute 前必须通过 Dry Run 验证
4. **全链路审计** — 所有操作记录 Audit Log
5. **敏感数据脱敏** — password/token/secret/kubeconfig 自动脱敏

## Action Lifecycle

```
proposed → pending_approval → approved → running → success/failed/timeout
                ↓                  ↓
            rejected          cancelled
```

## Components

### Action
- 单个自动化操作
- 支持: restart_pod, scale_deployment, jenkins_build, argocd_sync

### Workflow
- 多步骤编排
- Step 依赖执行
- 失败跳过后续步骤
- 一次审批，逐步执行

### Policy Engine
- 环境感知策略
- 状态跳转验证
- 并发控制（同一资源同时只能一个 running）

### Executors
- **KubernetesExecutor**: restart_pod, scale_deployment (30s timeout)
- **JenkinsExecutor**: trigger build (60s timeout)
- **ArgoCDExecutor**: sync application (120s timeout)

### RBAC
- `automation:create` — 创建 Action
- `automation:approve` — 审批
- `automation:execute` — 执行
- `automation:cancel` — 取消
- `automation:audit` — 查看审计

## API Endpoints

### Actions
| Method | Path | Description | Permission |
|--------|------|-------------|------------|
| POST | /api/v1/actions | Create Action | create |
| GET | /api/v1/actions | List Actions | - |
| GET | /api/v1/actions/pending-approval | Pending Approvals | - |
| GET | /api/v1/actions/:id | Action Detail | - |
| POST | /api/v1/actions/:id/approve | Approve | approve |
| POST | /api/v1/actions/:id/reject | Reject | approve |
| POST | /api/v1/actions/:id/dry-run | Dry Run | - |
| POST | /api/v1/actions/:id/execute | Execute | execute |
| POST | /api/v1/actions/:id/cancel | Cancel | cancel |
| GET | /api/v1/actions/:id/executions | Execution History | - |
| GET | /api/v1/automation/audit | Audit Log | audit |

### Workflows
| Method | Path | Description |
|--------|------|-------------|
| POST | /api/v1/workflows | Create Workflow |
| GET | /api/v1/workflows | List Workflows |
| GET | /api/v1/workflows/:id | Workflow Detail |
| POST | /api/v1/workflows/:id/submit | Submit for Approval |
| POST | /api/v1/workflows/:id/approve | Approve |
| POST | /api/v1/workflows/:id/execute | Execute |
| POST | /api/v1/workflows/:id/cancel | Cancel |

### Incident Integration
| Method | Path | Description |
|--------|------|-------------|
| POST | /api/v1/incidents/:id/actions | Create Action from Incident |

## Database Tables

- `actions` — Action 记录
- `action_executions` — 执行历史
- `automation_audit` — 审计日志（Immutable）
- `workflows` — 工作流
- `workflow_steps` — 工作流步骤

## Incident Timeline Integration

Action 执行完成后自动写入 Incident Timeline:
- SignalType: `automation`
- 成功: Resolved=true
- 失败: Resolved=false, Error 记录在 Metadata
