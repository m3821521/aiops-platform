import { Layout, Dropdown, Avatar, Button, Select, Space, Tooltip } from 'antd'
import {
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  UserOutlined,
  LogoutOutlined,
  BulbOutlined,
  BulbFilled,
  CloudServerOutlined,
} from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { useEffect, useState } from 'react'
import { clusterApi } from '@/api/cluster'
import type { Cluster } from '@/types'

const { Header } = Layout

export default function AppHeader() {
  const navigate = useNavigate()
  const { collapsed, toggleCollapsed, theme, toggleTheme, currentCluster, setCluster } =
    useAppStore()
  const { user, logout } = useAuthStore()
  const [clusters, setClusters] = useState<Cluster[]>([])

  useEffect(() => {
    clusterApi.list().then(setClusters).catch(() => {})
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
        background: theme === 'dark' ? '#001529' : '#fff',
        borderBottom: '1px solid rgba(0,0,0,0.06)',
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
