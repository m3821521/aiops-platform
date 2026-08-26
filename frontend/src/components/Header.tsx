import { Layout, Dropdown, Avatar, Button, Select, Space, Tooltip, Badge, Popover, List, Tag, message } from 'antd'
import {
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  UserOutlined,
  LogoutOutlined,
  BulbOutlined,
  BulbFilled,
  CloudServerOutlined,
  BellOutlined,
} from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { useEffect, useState, useRef } from 'react'
import { clusterApi } from '@/api/cluster'
import { alertsApi } from '@/api/alerts'
import { automationApi } from '@/api/automation'
import type { Cluster, Alert } from '@/types'
import type { AutomationAction } from '@/api/automation'

const { Header } = Layout

export default function AppHeader() {
  const navigate = useNavigate()
  const { collapsed, toggleCollapsed, theme, toggleTheme, currentCluster, setCluster } =
    useAppStore()
  const { user, logout } = useAuthStore()
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [firingAlerts, setFiringAlerts] = useState<Alert[]>([])
  const [pendingActions, setPendingActions] = useState<AutomationAction[]>([])
  // P1-X.11: 防 toast storm — 仅在首次失败时提示，成功后重置
  const clusterErrorNotified = useRef(false)
  const alertsErrorNotified = useRef(false)
  const actionsErrorNotified = useRef(false)

  useEffect(() => {
    clusterApi.list()
      .then((res) => { setClusters(res || []); clusterErrorNotified.current = false })
      .catch(() => {
        if (!clusterErrorNotified.current) {
          message.warning('集群列表加载失败，部分功能可能不可用')
          clusterErrorNotified.current = true
        }
      })
  }, [])

  const fetchAlerts = () => {
    alertsApi.list({ page: 1, page_size: 5, status: 'firing' })
      .then((res) => { setFiringAlerts(res?.items || []); alertsErrorNotified.current = false })
      .catch(() => {
        if (!alertsErrorNotified.current) {
          message.warning('活动告警加载失败')
          alertsErrorNotified.current = true
        }
      })
  }

  const fetchPendingActions = () => {
    automationApi.pendingApproval({ page: 1, page_size: 5 })
      .then((res) => { setPendingActions(res?.items || []); actionsErrorNotified.current = false })
      .catch(() => {
        if (!actionsErrorNotified.current) {
          message.warning('待审批操作加载失败')
          actionsErrorNotified.current = true
        }
      })
  }

  useEffect(() => {
    fetchAlerts()
    fetchPendingActions()
    const timer = setInterval(() => { fetchAlerts(); fetchPendingActions() }, 30000)
    return () => clearInterval(timer)
  }, [])

  const userMenu = {
    items: [
      { key: 'profile', icon: <UserOutlined />, label: '个人信息' },
      { type: 'divider' as const },
      { key: 'logout', icon: <LogoutOutlined />, label: '退出登录' },
    ],
    onClick: ({ key }: { key: string }) => {
      if (key === 'logout') {
        logout()
        navigate('/login')
      }
    },
  }

  return (
    <Header
      style={{
        padding: '0 16px',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        background: 'var(--bg-header)',
        borderBottom: '1px solid var(--border-color)',
      }}
    >
      <Space>
        <Button
          type="text"
          icon={collapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
          onClick={toggleCollapsed}
          style={{ fontSize: 16 }}
        />
        {clusters.length > 0 && (
          <Select
            size="small"
            value={currentCluster || clusters[0]?.name}
            onChange={setCluster}
            style={{ width: 160 }}
            prefix={<CloudServerOutlined />}
            options={clusters.map((c) => ({ label: c.name, value: c.name }))}
          />
        )}
      </Space>

      <Space>
        <Popover
          content={
            <div style={{ width: 320 }}>
              <div style={{ fontWeight: 600, marginBottom: 8 }}>待审批操作 ({pendingActions.length})</div>
              {pendingActions.length > 0 ? (
                <List
                  size="small"
                  dataSource={pendingActions}
                  renderItem={(item) => (
                    <List.Item>
                      <Space>
                        <Tag color="orange">{item.risk}</Tag>
                        <span style={{ fontSize: 13 }}>{item.target_name}</span>
                        <span style={{ fontSize: 11, color: '#999' }}>{item.action_type}</span>
                      </Space>
                    </List.Item>
                  )}
                />
              ) : (
                <div style={{ color: '#999', textAlign: 'center', padding: '8px 0' }}>暂无待审批操作</div>
              )}
              <div style={{ textAlign: 'center', marginBottom: 8 }}>
                <Button type="link" size="small" onClick={() => navigate('/automation/actions')}>查看全部</Button>
              </div>
              <div style={{ borderTop: '1px solid #f0f0f0', paddingTop: 8 }}>
                <div style={{ fontWeight: 600, marginBottom: 8 }}>活动告警 ({firingAlerts.length})</div>
                {firingAlerts.length > 0 ? (
                  <List
                    size="small"
                    dataSource={firingAlerts}
                    renderItem={(item) => (
                      <List.Item>
                        <Space>
                          <Tag color={item.severity === 'critical' ? 'error' : item.severity === 'warning' ? 'warning' : 'info'}>
                            {item.severity}
                          </Tag>
                          <span style={{ fontSize: 13 }}>{item.alertname}</span>
                        </Space>
                      </List.Item>
                    )}
                  />
                ) : (
                  <div style={{ color: '#999', textAlign: 'center', padding: '8px 0' }}>暂无活动告警</div>
                )}
                <div style={{ textAlign: 'center', marginTop: 8 }}>
                  <Button type="link" size="small" onClick={() => navigate('/alerts/realtime')}>查看全部</Button>
                </div>
              </div>
            </div>
          }
          title={null}
          trigger="click"
        >
          <Badge count={firingAlerts.length + pendingActions.length} size="small">
            <Button type="text" icon={<BellOutlined />} />
          </Badge>
        </Popover>
        <Tooltip title={theme === 'dark' ? '切换浅色' : '切换深色'}>
          <Button
            type="text"
            icon={theme === 'dark' ? <BulbFilled /> : <BulbOutlined />}
            onClick={toggleTheme}
          />
        </Tooltip>
        <Dropdown menu={userMenu} placement="bottomRight">
          <Space style={{ cursor: 'pointer' }}>
            <Avatar size="small" icon={<UserOutlined />} />
            <span>{user?.username || 'admin'}</span>
          </Space>
        </Dropdown>
      </Space>
    </Header>
  )
}
