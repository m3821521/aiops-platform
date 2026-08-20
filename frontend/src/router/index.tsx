import { createBrowserRouter, Navigate } from 'react-router-dom'
import { lazy, Suspense } from 'react'
import MainLayout from '@/layouts/MainLayout'
import Login from '@/pages/Login'
import RequireAuth from './RequireAuth'

// 懒加载大页面（包含 ECharts 等重依赖）
const Dashboard = lazy(() => import('@/pages/Dashboard'))
const Clusters = lazy(() => import('@/pages/Kubernetes/Clusters'))
const Nodes = lazy(() => import('@/pages/Kubernetes/Nodes'))
const Namespaces = lazy(() => import('@/pages/Kubernetes/Namespaces'))
const Pods = lazy(() => import('@/pages/Kubernetes/Pods'))
const Deployments = lazy(() => import('@/pages/Kubernetes/Deployments'))
const Services = lazy(() => import('@/pages/Kubernetes/Services'))
const AlertRealtime = lazy(() => import('@/pages/Alerts/Realtime'))
const AlertHistory = lazy(() => import('@/pages/Alerts/History'))
const MonitoringOverview = lazy(() => import('@/pages/Monitoring/Overview'))
const PromQL = lazy(() => import('@/pages/Monitoring/PromQL'))
const LogSearch = lazy(() => import('@/pages/Logs/Search'))
const AIAssistant = lazy(() => import('@/pages/AI/Assistant'))
const AutomationActions = lazy(() => import('@/pages/Automation/Actions'))
const Workflows = lazy(() => import('@/pages/Automation/Workflows'))
const Incidents = lazy(() => import('@/pages/AIOps/Incidents'))
const Anomaly = lazy(() => import('@/pages/AIOps/Anomaly'))
const Topology = lazy(() => import('@/pages/AIOps/Topology'))
const SystemUsers = lazy(() => import('@/pages/System/Users'))
const SystemRoles = lazy(() => import('@/pages/System/Roles'))
const SystemAuditLogs = lazy(() => import('@/pages/System/AuditLogs'))
const ExternalConnections = lazy(() => import('@/pages/System/ExternalConnections'))
const AgentOrchestration = lazy(() => import('@/pages/Agent/Orchestration'))
const Placeholder = lazy(() => import('@/pages/Placeholder'))

// 懒加载页面的 Suspense 包装
const LazyPage = ({ children }: { children: React.ReactNode }) => (
  <Suspense fallback={
    <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '60vh' }}>
      <div style={{ fontSize: 14, color: 'var(--text-muted)' }}>加载中...</div>
    </div>
  }>
    {children}
  </Suspense>
)

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
      { index: true, element: <LazyPage><Dashboard /></LazyPage> },
      // Kubernetes
      { path: 'kubernetes/clusters', element: <LazyPage><Clusters /></LazyPage> },
      { path: 'kubernetes/nodes', element: <LazyPage><Nodes /></LazyPage> },
      { path: 'kubernetes/namespaces', element: <LazyPage><Namespaces /></LazyPage> },
      { path: 'kubernetes/pods', element: <LazyPage><Pods /></LazyPage> },
      { path: 'kubernetes/deployments', element: <LazyPage><Deployments /></LazyPage> },
      { path: 'kubernetes/services', element: <LazyPage><Services /></LazyPage> },
      // Monitoring
      { path: 'monitoring/host', element: <LazyPage><MonitoringOverview /></LazyPage> },
      { path: 'monitoring/k8s', element: <LazyPage><MonitoringOverview /></LazyPage> },
      { path: 'monitoring/pod', element: <LazyPage><Placeholder title="Pod 监控" phase="Phase 4" /></LazyPage> },
      { path: 'monitoring/promql', element: <LazyPage><PromQL /></LazyPage> },
      // Alerts
      { path: 'alerts/realtime', element: <LazyPage><AlertRealtime /></LazyPage> },
      { path: 'alerts/history', element: <LazyPage><AlertHistory /></LazyPage> },
      { path: 'alerts/aggregate', element: <LazyPage><Placeholder title="告警聚合" phase="Phase 5" /></LazyPage> },
      { path: 'alerts/noise', element: <LazyPage><Placeholder title="告警降噪" phase="Phase 5" /></LazyPage> },
      // AIOps
      { path: 'aiops/incidents', element: <LazyPage><Incidents /></LazyPage> },
      { path: 'aiops/anomaly', element: <LazyPage><Anomaly /></LazyPage> },
      { path: 'aiops/rca', element: <LazyPage><Placeholder title="根因分析" phase="Phase 7" /></LazyPage> },
      { path: 'aiops/topology', element: <LazyPage><Topology /></LazyPage> },
      // Logs
      { path: 'logs/search', element: <LazyPage><LogSearch /></LazyPage> },
      { path: 'logs/analyze', element: <LazyPage><Placeholder title="日志分析" phase="Phase 6" /></LazyPage> },
      // AI
      { path: 'ai', element: <LazyPage><AIAssistant /></LazyPage> },
      // Automation
      { path: 'automation/actions', element: <LazyPage><AutomationActions /></LazyPage> },
      { path: 'automation/workflows', element: <LazyPage><Workflows /></LazyPage> },
      { path: 'automation/k8s', element: <LazyPage><Placeholder title="Kubernetes 运维" phase="Phase 9" /></LazyPage> },
      { path: 'automation/jenkins', element: <LazyPage><Placeholder title="Jenkins" phase="Phase 10" /></LazyPage> },
      { path: 'automation/argocd', element: <LazyPage><Placeholder title="ArgoCD" phase="Phase 10" /></LazyPage> },
      // System
      { path: 'system/users', element: <LazyPage><SystemUsers /></LazyPage> },
      { path: 'system/roles', element: <LazyPage><SystemRoles /></LazyPage> },
      { path: 'system/audit', element: <LazyPage><SystemAuditLogs /></LazyPage> },
      { path: 'system/connections', element: <LazyPage><ExternalConnections /></LazyPage> },
      { path: 'agent/orchestration', element: <LazyPage><AgentOrchestration /></LazyPage> },
    ],
  },
  { path: '*', element: <Navigate to="/" replace /> },
])
