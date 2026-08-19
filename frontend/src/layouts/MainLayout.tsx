import { Layout, ConfigProvider, theme as antdTheme, Breadcrumb } from 'antd'
import zhCN from 'antd/locale/zh_CN'
import { Outlet, useLocation } from 'react-router-dom'
import { useEffect } from 'react'
import Sidebar from '@/components/Sidebar'
import AppHeader from '@/components/Header'
import { useAppStore } from '@/stores/app'

const { Content } = Layout

// 路由标题映射
const routeTitles: Record<string, string> = {
  '/dashboard': '首页',
  '/kubernetes/clusters': '集群',
  '/kubernetes/nodes': 'Node',
  '/kubernetes/namespaces': 'Namespace',
  '/kubernetes/pods': 'Pod',
  '/kubernetes/deployments': 'Deployment',
  '/kubernetes/statefulsets': 'StatefulSet',
  '/kubernetes/daemonsets': 'DaemonSet',
  '/kubernetes/services': 'Service',
  '/monitoring/host': '主机监控',
  '/monitoring/k8s': 'Kubernetes 监控',
  '/monitoring/promql': 'PromQL',
  '/alerts/realtime': '实时告警',
  '/alerts/history': '告警历史',
  '/alerts/aggregate': '告警聚合',
  '/logs/search': '日志搜索',
  '/logs/analyze': '日志分析',
  '/aiops/incidents': '事件中心',
  '/aiops/anomaly': '异常检测',
  '/aiops/rca': 'RCA 根因分析',
  '/aiops/topology': '服务拓扑',
  '/ai': 'AI 运维助手',
  '/automation/tasks': '自动化任务',
  '/automation/jenkins': 'Jenkins',
  '/automation/argocd': 'ArgoCD',
  '/system/users': '用户管理',
  '/system/roles': '角色管理',
  '/system/audit': '审计日志',
}

// 面包屑映射
const breadcrumbMap: Record<string, string[]> = {
  '/dashboard': ['首页'],
  '/kubernetes/clusters': ['资源', 'Kubernetes', '集群'],
  '/kubernetes/nodes': ['资源', 'Kubernetes', 'Node'],
  '/kubernetes/namespaces': ['资源', 'Kubernetes', 'Namespace'],
  '/kubernetes/pods': ['资源', 'Kubernetes', 'Pod'],
  '/kubernetes/deployments': ['资源', 'Kubernetes', 'Deployment'],
  '/kubernetes/statefulsets': ['资源', 'Kubernetes', 'StatefulSet'],
  '/kubernetes/daemonsets': ['资源', 'Kubernetes', 'DaemonSet'],
  '/kubernetes/services': ['资源', 'Kubernetes', 'Service'],
  '/monitoring/host': ['监控', '主机监控'],
  '/monitoring/k8s': ['监控', 'Kubernetes 监控'],
  '/monitoring/promql': ['监控', 'PromQL'],
  '/alerts/realtime': ['告警', '实时告警'],
  '/alerts/history': ['告警', '告警历史'],
  '/alerts/aggregate': ['告警', '告警聚合'],
  '/logs/search': ['日志', '日志搜索'],
  '/logs/analyze': ['日志', '日志分析'],
  '/aiops/incidents': ['AIOps', '事件中心'],
  '/aiops/anomaly': ['AIOps', '异常检测'],
  '/aiops/rca': ['AIOps', 'RCA 根因分析'],
  '/aiops/topology': ['AIOps', '服务拓扑'],
  '/ai': ['AI', 'AI 运维助手'],
  '/automation/tasks': ['自动化', '运维任务'],
  '/automation/jenkins': ['自动化', 'Jenkins'],
  '/automation/argocd': ['自动化', 'ArgoCD'],
  '/system/users': ['系统', '用户管理'],
  '/system/roles': ['系统', '角色管理'],
  '/system/audit': ['系统', '审计日志'],
}

export default function MainLayout() {
  const themeMode = useAppStore((s) => s.theme)
  const location = useLocation()

  // 动态浏览器标题
  useEffect(() => {
    const title = routeTitles[location.pathname] || 'AIOps Platform'
    document.title = `${title} - AIOps Platform`
  }, [location.pathname])

  const breadcrumbItems = breadcrumbMap[location.pathname] || ['首页']

  return (
    <ConfigProvider
      locale={zhCN}
      theme={{
        algorithm: themeMode === 'dark' ? antdTheme.darkAlgorithm : antdTheme.defaultAlgorithm,
        token: {
          colorPrimary: '#2563eb',
          colorInfo: '#2563eb',
          colorSuccess: '#16a34a',
          colorWarning: '#d97706',
          colorError: '#dc2626',
          borderRadius: 6,
          fontSize: 13,
          controlHeight: 32,
        },
      }}
    >
      <Layout style={{ minHeight: '100vh', background: 'var(--bg-app)' }}>
        <Sidebar />
        <Layout style={{ background: 'var(--bg-app)' }}>
          <AppHeader />
          <div style={{ padding: '4px 24px 0', background: 'var(--bg-app)' }}>
            <Breadcrumb items={breadcrumbItems.map((item) => ({ title: item }))} />
          </div>
          <Content
            style={{
              margin: '8px 16px 16px',
              padding: 0,
              minHeight: 280,
              overflow: 'auto',
              background: 'transparent',
            }}
          >
            <Outlet />
          </Content>
        </Layout>
      </Layout>
    </ConfigProvider>
  )
}
