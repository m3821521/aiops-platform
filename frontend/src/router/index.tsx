import { createBrowserRouter, Navigate } from 'react-router-dom'
import MainLayout from '@/layouts/MainLayout'
import Login from '@/pages/Login'
import Dashboard from '@/pages/Dashboard'
import Placeholder from '@/pages/Placeholder'
import RequireAuth from './RequireAuth'
import Clusters from '@/pages/Kubernetes/Clusters'
import Nodes from '@/pages/Kubernetes/Nodes'
import Namespaces from '@/pages/Kubernetes/Namespaces'
import Pods from '@/pages/Kubernetes/Pods'
import Deployments from '@/pages/Kubernetes/Deployments'
import Services from '@/pages/Kubernetes/Services'
import AlertRealtime from '@/pages/Alerts/Realtime'

export const router = createBrowserRouter([
  {
    path: '/login',
    element: <Login />,
  },
  {
    path: '/',
    element: (
      <RequireAuth>
        <MainLayout />
      </RequireAuth>
    ),
    children: [
      { index: true, element: <Dashboard /> },
      // Kubernetes
      { path: 'kubernetes/clusters', element: <Clusters /> },
      { path: 'kubernetes/nodes', element: <Nodes /> },
      { path: 'kubernetes/namespaces', element: <Namespaces /> },
      { path: 'kubernetes/pods', element: <Pods /> },
      { path: 'kubernetes/deployments', element: <Deployments /> },
      { path: 'kubernetes/services', element: <Services /> },
      // Monitoring
      { path: 'monitoring/host', element: <Placeholder title="主机监控" phase="Phase 4" /> },
      { path: 'monitoring/k8s', element: <Placeholder title="Kubernetes 监控" phase="Phase 4" /> },
      { path: 'monitoring/pod', element: <Placeholder title="Pod 监控" phase="Phase 4" /> },
      { path: 'monitoring/promql', element: <Placeholder title="PromQL 查询" phase="Phase 4" /> },
      // Alerts
      { path: 'alerts/realtime', element: <AlertRealtime /> },
      { path: 'alerts/history', element: <Placeholder title="告警历史" phase="Phase 5" /> },
      { path: 'alerts/aggregate', element: <Placeholder title="告警聚合" phase="Phase 5" /> },
      { path: 'alerts/noise', element: <Placeholder title="告警降噪" phase="Phase 5" /> },
      // AIOps
      { path: 'aiops/anomaly', element: <Placeholder title="异常检测" phase="Phase 7" /> },
      { path: 'aiops/rca', element: <Placeholder title="根因分析" phase="Phase 7" /> },
      { path: 'aiops/topology', element: <Placeholder title="服务拓扑" phase="Phase 7" /> },
      // Logs
      { path: 'logs/search', element: <Placeholder title="日志搜索" phase="Phase 6" /> },
      { path: 'logs/analyze', element: <Placeholder title="日志分析" phase="Phase 6" /> },
      // AI
      { path: 'ai', element: <Placeholder title="AI 运维助手" phase="Phase 8" /> },
      // Automation
      { path: 'automation/k8s', element: <Placeholder title="Kubernetes 运维" phase="Phase 9" /> },
      { path: 'automation/jenkins', element: <Placeholder title="Jenkins" phase="Phase 10" /> },
      { path: 'automation/argocd', element: <Placeholder title="ArgoCD" phase="Phase 10" /> },
      // System
      { path: 'system/users', element: <Placeholder title="用户管理" phase="Phase 11" /> },
      { path: 'system/roles', element: <Placeholder title="角色权限" phase="Phase 11" /> },
      { path: 'system/audit', element: <Placeholder title="审计日志" phase="Phase 11" /> },
    ],
  },
  { path: '*', element: <Navigate to="/" replace /> },
])
