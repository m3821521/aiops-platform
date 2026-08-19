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
} from '@ant-design/icons'
import { useAppStore } from '@/stores/app'

const { Sider } = Layout

const menuItems = [
  { key: '/', icon: <DashboardOutlined />, label: '首页' },
  {
    key: '/kubernetes',
    icon: <CloudOutlined />,
    label: 'Kubernetes',
    children: [
      { key: '/kubernetes/clusters', label: '集群' },
      { key: '/kubernetes/nodes', label: 'Node' },
      { key: '/kubernetes/namespaces', label: 'Namespace' },
      { key: '/kubernetes/pods', label: 'Pod' },
      { key: '/kubernetes/deployments', label: 'Deployment' },
      { key: '/kubernetes/services', label: 'Service' },
    ],
  },
  {
    key: '/monitoring',
    icon: <MonitorOutlined />,
    label: '监控',
    children: [
      { key: '/monitoring/host', label: '主机监控' },
      { key: '/monitoring/k8s', label: 'Kubernetes 监控' },
      { key: '/monitoring/pod', label: 'Pod 监控' },
      { key: '/monitoring/promql', label: 'PromQL' },
    ],
  },
  {
    key: '/alerts',
    icon: <AlertOutlined />,
    label: '告警中心',
    children: [
      { key: '/alerts/realtime', label: '实时告警' },
      { key: '/alerts/history', label: '告警历史' },
      { key: '/alerts/aggregate', label: '告警聚合' },
      { key: '/alerts/noise', label: '告警降噪' },
    ],
  },
  {
    key: '/aiops',
    icon: <ThunderboltOutlined />,
    label: 'AIOps',
    children: [
      { key: '/aiops/anomaly', label: '异常检测' },
      { key: '/aiops/rca', label: '根因分析' },
      { key: '/aiops/topology', label: '服务拓扑' },
    ],
  },
  {
    key: '/logs',
    icon: <FileTextOutlined />,
    label: '日志中心',
    children: [
      { key: '/logs/search', label: '日志搜索' },
      { key: '/logs/analyze', label: '日志分析' },
    ],
  },
  {
    key: '/ai',
    icon: <RobotOutlined />,
    label: 'AI 助手',
  },
  {
    key: '/automation',
    icon: <ToolOutlined />,
    label: '自动化',
    children: [
      { key: '/automation/k8s', label: 'Kubernetes' },
      { key: '/automation/jenkins', label: 'Jenkins' },
      { key: '/automation/argocd', label: 'ArgoCD' },
    ],
  },
  {
    key: '/system',
    icon: <SettingOutlined />,
    label: '系统管理',
    children: [
      { key: '/system/users', label: '用户管理' },
      { key: '/system/roles', label: '角色权限' },
      { key: '/system/audit', label: '审计日志' },
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
      theme={isDark ? 'dark' : 'light'}
      style={{
        overflow: 'auto',
        height: '100vh',
        position: 'sticky',
        top: 0,
        left: 0,
        borderRight: isDark ? 'none' : '1px solid rgba(0,0,0,0.06)',
      }}
    >
      <div
        style={{
          height: 56,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          color: isDark ? '#fff' : '#001529',
          fontSize: collapsed ? 14 : 18,
          fontWeight: 600,
          borderBottom: isDark ? '1px solid rgba(255,255,255,0.1)' : '1px solid rgba(0,0,0,0.06)',
        }}
      >
        {collapsed ? 'AIO' : 'AIOps 平台'}
      </div>
      <Menu
        theme={isDark ? 'dark' : 'light'}
        mode="inline"
        selectedKeys={[selectedKey]}
        defaultOpenKeys={[openKey]}
        items={menuItems}
        onClick={({ key }) => navigate(key)}
      />
    </Sider>
  )
}
