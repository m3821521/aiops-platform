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
  { key: '/', icon: <DashboardOutlined />, label: '总览' },
  {
    key: '/kubernetes',
    icon: <CloudOutlined />,
    label: '基础设施',
    children: [
      { key: '/kubernetes/clusters', label: '集群' },
      { key: '/kubernetes/nodes', label: '节点' },
      { key: '/kubernetes/namespaces', label: '命名空间' },
      { key: '/kubernetes/pods', label: '容器组' },
      { key: '/kubernetes/deployments', label: '部署' },
      { key: '/kubernetes/services', label: '服务' },
    ],
  },
  {
    key: '/monitoring',
    icon: <MonitorOutlined />,
    label: '可观测性',
    children: [
      { key: '/monitoring/host', label: '指标监控' },
      { key: '/monitoring/promql', label: 'PromQL 查询' },
    ],
  },
  {
    key: '/alerts',
    icon: <AlertOutlined />,
    label: '告警中心',
    children: [
      { key: '/alerts/realtime', label: '实时告警' },
      { key: '/alerts/history', label: '告警历史' },
    ],
  },
  {
    key: '/aiops',
    icon: <ThunderboltOutlined />,
    label: '智能运维',
    children: [
      { key: '/aiops/incidents', label: '事件中心' },
      { key: '/aiops/anomaly', label: '异常检测' },
      { key: '/aiops/topology', label: '服务拓扑' },
    ],
  },
  {
    key: '/logs',
    icon: <FileTextOutlined />,
    label: '日志中心',
    children: [
      { key: '/logs/search', label: '日志检索' },
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
      { key: '/automation/actions', label: '操作审批' },
      { key: '/automation/workflows', label: '工作流' },
      { key: '/automation/jenkins', label: 'Jenkins' },
      { key: '/automation/argocd', label: 'ArgoCD' },
    ],
  },
  {
    key: '/agent',
    icon: <RobotOutlined />,
    label: 'AI Agent',
    children: [
      { key: '/agent/orchestration', label: '多 Agent 编排' },
    ],
  },
  {
    key: '/system',
    icon: <SettingOutlined />,
    label: '系统管理',
    children: [
      { key: '/system/users', label: '用户管理' },
      { key: '/system/roles', label: '角色管理' },
      { key: '/system/audit', label: '审计日志' },
      { key: '/system/connections', label: '外部连接' },
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
        background: 'var(--bg-sidebar)',
        borderRight: '1px solid var(--border-sidebar)',
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
          color: 'var(--text-primary)',
          fontSize: collapsed ? 14 : 15,
          fontWeight: 700,
          borderBottom: '1px solid var(--border-light)',
          letterSpacing: -0.2,
        }}
      >
        {collapsed ? (
          <ApartmentOutlined style={{ fontSize: 20, color: 'var(--color-primary)' }} />
        ) : (
          <span style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <ApartmentOutlined style={{ color: 'var(--color-primary)', fontSize: 18 }} />
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
