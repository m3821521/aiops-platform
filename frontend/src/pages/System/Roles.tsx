import { useState, useEffect } from 'react'
import {
  Card, Table, Button, Space, Tag, Drawer, Descriptions, Badge, message,
} from 'antd'
import {
  ReloadOutlined, SafetyCertificateOutlined, KeyOutlined, EyeOutlined,
} from '@ant-design/icons'
import { userApi } from '@/api/user'
import type { Role, Permission } from '@/types'

export default function RoleManagement() {
  const [roles, setRoles] = useState<Role[]>([])
  const [loading, setLoading] = useState(false)
  const [detailOpen, setDetailOpen] = useState(false)
  const [currentRole, setCurrentRole] = useState<Role | null>(null)

  const loadRoles = async () => {
    setLoading(true)
    try {
      const res = await userApi.roles()
      setRoles(res || [])
    } catch (e: any) {
      message.error('加载角色列表失败: ' + (e?.response?.data?.message || e.message))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadRoles()
  }, [])

  const handleViewDetail = (role: Role) => {
    setCurrentRole(role)
    setDetailOpen(true)
  }

  // 按资源分组权限
  const groupPermissions = (perms: Permission[]) => {
    const groups: Record<string, Permission[]> = {}
    perms.forEach((p) => {
      const key = p.resource || 'other'
      if (!groups[key]) groups[key] = []
      groups[key].push(p)
    })
    return groups
  }

  const actionColorMap: Record<string, string> = {
    read: 'blue',
    write: 'orange',
    create: 'green',
    update: 'cyan',
    delete: 'red',
    approve: 'purple',
    execute: 'magenta',
    audit: 'geekblue',
  }

  const columns = [
    {
      title: 'ID',
      dataIndex: 'id',
      width: 80,
    },
    {
      title: '角色名称',
      dataIndex: 'name',
      width: 180,
      render: (text: string) => (
        <Space>
          <SafetyCertificateOutlined style={{ color: 'var(--color-primary)' }} />
          <strong>{text}</strong>
        </Space>
      ),
    },
    {
      title: '描述',
      dataIndex: 'description',
      render: (text: string) => text || '-',
    },
    {
      title: '权限数量',
      dataIndex: 'permissions',
      width: 120,
      render: (perms: Permission[]) => (
        <Tag color={perms?.length > 10 ? 'red' : perms?.length > 5 ? 'orange' : 'blue'}>
          {perms?.length || 0} 个权限
        </Tag>
      ),
    },
    {
      title: '操作',
      key: 'action',
      width: 120,
      render: (_: any, record: Role) => (
        <Button
          size="small"
          type="link"
          icon={<EyeOutlined />}
          onClick={() => handleViewDetail(record)}
        >
          查看权限
        </Button>
      ),
    },
  ]

  return (
    <div>
      <Card
        title={
          <Space>
            <SafetyCertificateOutlined style={{ color: 'var(--color-primary)' }} />
            <span>角色管理</span>
            <Tag color="blue">{roles.length} 个角色</Tag>
          </Space>
        }
        extra={
          <Button icon={<ReloadOutlined />} onClick={loadRoles}>刷新</Button>
        }
      >
        <Table
          columns={columns}
          dataSource={roles}
          rowKey="id"
          loading={loading}
          pagination={false}
        />
      </Card>

      <Drawer
        title={
          <Space>
            <KeyOutlined />
            <span>角色详情 - {currentRole?.name}</span>
          </Space>
        }
        open={detailOpen}
        onClose={() => setDetailOpen(false)}
        width={600}
      >
        {currentRole && (
          <div>
            <Descriptions column={1} bordered size="small" style={{ marginBottom: 24 }}>
              <Descriptions.Item label="角色ID">{currentRole.id}</Descriptions.Item>
              <Descriptions.Item label="角色名称">{currentRole.name}</Descriptions.Item>
              <Descriptions.Item label="描述">{currentRole.description || '-'}</Descriptions.Item>
              <Descriptions.Item label="权限数量">
                <Badge count={currentRole.permissions?.length || 0} style={{ backgroundColor: 'var(--color-primary)' }} />
              </Descriptions.Item>
            </Descriptions>

            <div style={{ marginBottom: 12 }}>
              <strong style={{ fontSize: 14 }}>权限列表</strong>
            </div>

            {Object.entries(groupPermissions(currentRole.permissions || [])).map(([resource, perms]) => (
              <Card
                key={resource}
                size="small"
                title={
                  <Space>
                    <Tag color="blue">{resource}</Tag>
                    <span style={{ fontSize: 12, color: 'var(--text-secondary)' }}>
                      {perms.length} 个权限
                    </span>
                  </Space>
                }
                style={{ marginBottom: 12 }}
              >
                <Space wrap>
                  {perms.map((p) => (
                    <Tag
                      key={p.id}
                      color={actionColorMap[p.action] || 'default'}
                      style={{ marginBottom: 4 }}
                    >
                      {p.action}: {p.name || p.code}
                    </Tag>
                  ))}
                </Space>
              </Card>
            ))}

            {(!currentRole.permissions || currentRole.permissions.length === 0) && (
              <div style={{ textAlign: 'center', padding: 40, color: 'var(--text-muted)' }}>
                该角色暂无权限配置
              </div>
            )}
          </div>
        )}
      </Drawer>
    </div>
  )
}
