import { Layout, Menu } from 'antd'
import { useNavigate, useLocation } from 'react-router-dom'
import {
  DashboardOutlined,
  CloudOutlined,
  MonitorOutlined,
  AlertOutlined,
  ThunderboltOutlined,
  FileTextOutlined,
  RobotOutlined,
  SettingOutlined,
  ToolOutlined,
  ApartmentOutlined,
} from '@ant-design/icons'
import { useAppStore } from '@/stores/app'

const { Sider } = Layout

const menuItems = [
  { key: '/', icon: <DashboardOutlined />, label: 'Overview' },
  {
    key: '/kubernetes',
    icon: <CloudOutlined />,
    label: 'Infrastructure',
    children: [
      { key: '/kubernetes/clusters', label: 'Clusters' },
      { key: '/kubernetes/nodes', label: 'Nodes' },
      { key: '/kubernetes/namespaces', label: 'Namespaces' },
      { key: '/kubernetes/pods', label: 'Pods' },
      { key: '/kubernetes/deployments', label: 'Deployments' },
      { key: '/kubernetes/services', label: 'Services' },
    ],
  },
  {
    key: '/monitoring',
    icon: <MonitorOutlined />,
    label: 'Observability',
    children: [
      { key: '/monitoring/overview', label: 'Metrics' },
      { key: '/monitoring/promql', label: 'PromQL' },
    ],
  },
  {
    key: '/alerts',
    icon: <AlertOutlined />,
    label: 'Alerts',
    children: [
      { key: '/alerts/realtime', label: 'Active Alerts' },
      { key: '/alerts/history', label: 'History' },
    ],
  },
  {
    key: '/aiops',
    icon: <ThunderboltOutlined />,
    label: 'AIOps',
    children: [
      { key: '/aiops/incidents', label: 'Incidents' },
      { key: '/aiops/anomaly', label: 'Anomaly Detection' },
      { key: '/aiops/topology', label: 'Service Topology' },
    ],
  },
  {
    key: '/logs',
    icon: <FileTextOutlined />,
    label: 'Logs',
    children: [
      { key: '/logs/search', label: 'Log Search' },
    ],
  },
  {
    key: '/ai',
    icon: <RobotOutlined />,
    label: 'AI Assistant',
  },
  {
    key: '/automation',
    icon: <ToolOutlined />,
    label: 'Automation',
    children: [
      { key: '/automation/actions', label: 'Actions' },
      { key: '/automation/workflows', label: 'Workflows' },
      { key: '/automation/jenkins', label: 'Jenkins' },
      { key: '/automation/argocd', label: 'ArgoCD' },
    ],
  },
  {
    key: '/system',
    icon: <SettingOutlined />,
    label: 'Administration',
    children: [
      { key: '/system/users', label: 'Users' },
      { key: '/system/audit', label: 'Audit Logs' },
    ],
  },
]

export default function Sidebar() {
  const navigate = useNavigate()
  const location = useLocation()
  const collapsed = useAppStore((s) => s.collapsed)
  const theme = useAppStore((s) => s.theme)
  const isDark = theme === 'dark'

  const selectedKey = location.pathname
  const openKey = '/' + location.pathname.split('/')[1]

  return (
    <Sider
      trigger={null}
      collapsible
      collapsed={collapsed}
      width={220}
      theme="light"
      style={{
        overflow: 'auto',
        height: '100vh',
        position: 'sticky',
        top: 0,
        left: 0,
        background: isDark ? '#1e293b' : '#ffffff',
        borderRight: `1px solid ${isDark ? '#334155' : '#e5e7eb'}`,
        boxShadow: isDark ? 'none' : '0 1px 3px rgba(0,0,0,0.04)',
      }}
    >
      <div
        style={{
          height: 56,
          display: 'flex',
          alignItems: 'center',
          justifyContent: collapsed ? 'center' : 'flex-start',
          paddingLeft: collapsed ? 0 : 20,
          color: isDark ? '#f1f5f9' : '#111827',
          fontSize: collapsed ? 14 : 15,
          fontWeight: 700,
          borderBottom: `1px solid ${isDark ? '#334155' : '#f3f4f6'}`,
          letterSpacing: -0.2,
        }}
      >
        {collapsed ? (
          <ApartmentOutlined style={{ fontSize: 20, color: '#2563eb' }} />
        ) : (
          <span style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <ApartmentOutlined style={{ color: '#2563eb', fontSize: 18 }} />
            <span>AIOps Platform</span>
          </span>
        )}
      </div>
      <Menu
        theme="light"
        mode="inline"
        selectedKeys={[selectedKey]}
        defaultOpenKeys={[openKey]}
        items={menuItems}
        onClick={({ key }) => navigate(key)}
        style={{
          borderRight: 'none',
          background: 'transparent',
          padding: '8px 0',
        }}
      />
    </Sider>
  )
}
