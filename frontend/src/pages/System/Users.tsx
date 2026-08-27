import { useState, useEffect } from 'react'
import {
  Card, Table, Button, Space, Tag, Modal, Form, Input, message, Popconfirm, Badge, Select, Checkbox,
} from 'antd'
import {
  PlusOutlined, ReloadOutlined, UserOutlined, LockOutlined, SafetyCertificateOutlined,
} from '@ant-design/icons'
import { userApi } from '@/api/user'
import type { User, Role } from '@/types'

export default function UserManagement() {
  const [users, setUsers] = useState<User[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [createOpen, setCreateOpen] = useState(false)
  const [createLoading, setCreateLoading] = useState(false)
  const [form] = Form.useForm()
  const [roles, setRoles] = useState<Role[]>([])
  const [assignOpen, setAssignOpen] = useState(false)
  const [assignLoading, setAssignLoading] = useState(false)
  const [currentUser, setCurrentUser] = useState<User | null>(null)
  const [selectedRoleIds, setSelectedRoleIds] = useState<number[]>([])
  const [editOpen, setEditOpen] = useState(false)
  const [editLoading, setEditLoading] = useState(false)
  const [editForm] = Form.useForm()
  const [passwordOpen, setPasswordOpen] = useState(false)
  const [passwordLoading, setPasswordLoading] = useState(false)
  const [passwordForm] = Form.useForm()

  const loadUsers = async () => {
    setLoading(true)
    try {
      const res = await userApi.list({ page, page_size: pageSize })
      setUsers(res.items || [])
      setTotal(res.total || 0)
    } catch (e: any) {
      message.error('加载用户列表失败: ' + (e?.response?.data?.message || e.message))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadUsers()
  }, [page, pageSize])

  const handleCreate = async () => {
    try {
      const values = await form.validateFields()
      setCreateLoading(true)
      const { role_ids, ...userData } = values
      const newUser = await userApi.create(userData)
      // 如果选择了角色，分配角色
      if (role_ids && role_ids.length > 0 && newUser?.id) {
        await userApi.assignRoles(newUser.id, role_ids)
      }
      message.success('用户创建成功')
      setCreateOpen(false)
      form.resetFields()
      loadUsers()
    } catch (e: any) {
      if (e?.errorFields) return
      message.error('创建用户失败: ' + (e?.response?.data?.message || e.message))
    } finally {
      setCreateLoading(false)
    }
  }

  // 加载角色列表
  const loadRoles = async () => {
    try {
      const res = await userApi.roles()
      setRoles(res || [])
    } catch (e: any) {
      message.error('加载角色列表失败')
    }
  }

  useEffect(() => {
    loadRoles()
  }, [])

  // 打开分配角色弹窗
  const handleOpenAssign = (user: User) => {
    setCurrentUser(user)
    setSelectedRoleIds((user.roles || []).map((r) => r.id))
    setAssignOpen(true)
  }

  // 确认分配角色
  const handleAssignRoles = async () => {
    if (!currentUser) return
    setAssignLoading(true)
    try {
      await userApi.assignRoles(currentUser.id, selectedRoleIds)
      message.success('角色分配成功')
      setAssignOpen(false)
      loadUsers()
    } catch (e: any) {
      message.error('分配角色失败: ' + (e?.response?.data?.message || e.message))
    } finally {
      setAssignLoading(false)
    }
  }

  // 切换用户状态（禁用/启用）
  const handleToggleStatus = async (user: User) => {
    const newStatus = user.status === 'active' ? 'disabled' : 'active'
    const actionText = newStatus === 'disabled' ? '禁用' : '启用'
    try {
      await userApi.updateStatus(user.id, newStatus)
      message.success(`用户${actionText}成功`)
      loadUsers()
    } catch (e: any) {
      message.error(`${actionText}用户失败: ` + (e?.response?.data?.message || e.message))
    }
  }

  // 打开编辑用户弹窗
  const handleOpenEdit = (user: User) => {
    setCurrentUser(user)
    editForm.setFieldsValue({
      full_name: user.full_name,
      email: user.email,
      status: user.status,
    })
    setEditOpen(true)
  }

  // 确认编辑用户
  const handleEditUser = async () => {
    if (!currentUser) return
    try {
      const values = await editForm.validateFields()
      setEditLoading(true)
      await userApi.update(currentUser.id, values)
      message.success('用户信息更新成功')
      setEditOpen(false)
      loadUsers()
    } catch (e: any) {
      if (e?.errorFields) return
      message.error('更新用户失败: ' + (e?.response?.data?.message || e.message))
    } finally {
      setEditLoading(false)
    }
  }

  // 打开重置密码弹窗
  const handleOpenResetPassword = (user: User) => {
    setCurrentUser(user)
    passwordForm.resetFields()
    setPasswordOpen(true)
  }

  // 确认重置密码
  const handleResetPassword = async () => {
    if (!currentUser) return
    try {
      const values = await passwordForm.validateFields()
      setPasswordLoading(true)
      await userApi.resetPassword(currentUser.id, values.password)
      message.success('密码重置成功')
      setPasswordOpen(false)
    } catch (e: any) {
      if (e?.errorFields) return
      message.error('重置密码失败: ' + (e?.response?.data?.message || e.message))
    } finally {
      setPasswordLoading(false)
    }
  }

  const columns = [
    {
      title: 'ID',
      dataIndex: 'id',
      width: 80,
    },
    {
      title: '用户名',
      dataIndex: 'username',
      width: 150,
      render: (text: string) => (
        <Space>
          <UserOutlined style={{ color: 'var(--color-primary)' }} />
          <strong>{text}</strong>
        </Space>
      ),
    },
    {
      title: '姓名',
      dataIndex: 'full_name',
      width: 150,
      render: (text: string) => text || '-',
    },
    {
      title: '邮箱',
      dataIndex: 'email',
      width: 200,
      render: (text: string) => text || '-',
    },
    {
      title: '角色',
      dataIndex: 'roles',
      width: 200,
      render: (roles: any[]) => (
        <Space wrap>
          {(roles || []).map((r: any) => (
            <Tag key={r.id} color="blue">{r.name}</Tag>
          ))}
          {(!roles || roles.length === 0) && <span style={{ color: 'var(--text-muted)' }}>无角色</span>}
        </Space>
      ),
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 100,
      render: (status: string) => (
        status === 'active'
          ? <Badge status="success" text="正常" />
          : <Badge status="error" text="禁用" />
      ),
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      width: 180,
      render: (text: string) => new Date(text).toLocaleString('zh-CN'),
    },
    {
      title: '操作',
      key: 'action',
      width: 280,
      render: (_: any, record: User) => (
        <Space>
          <Button size="small" type="link" onClick={() => handleOpenEdit(record)}>
            编辑
          </Button>
          <Button
            size="small"
            type="link"
            icon={<SafetyCertificateOutlined />}
            onClick={() => handleOpenAssign(record)}
          >
            分配角色
          </Button>
          <Popconfirm
            title={record.status === 'active' ? '确定要禁用该用户吗？' : '确定要启用该用户吗？'}
            onConfirm={() => handleToggleStatus(record)}
          >
            <Button size="small" type="link" danger={record.status === 'active'}>
              {record.status === 'active' ? '禁用' : '启用'}
            </Button>
          </Popconfirm>
          <Button
            size="small"
            type="link"
            icon={<LockOutlined />}
            onClick={() => handleOpenResetPassword(record)}
          >
            重置密码
          </Button>
        </Space>
      ),
    },
  ]

  return (
    <div>
      <Card
        title={
          <Space>
            <UserOutlined style={{ color: 'var(--color-primary)' }} />
            <span>用户管理</span>
            <Tag color="blue">{total} 个用户</Tag>
          </Space>
        }
        extra={
          <Space>
            <Button icon={<ReloadOutlined />} onClick={loadUsers}>刷新</Button>
            <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateOpen(true)}>
              创建用户
            </Button>
          </Space>
        }
      >
        <Table
          scroll={{ x: 'max-content' }}
          columns={columns}
          dataSource={users}
          rowKey="id"
          loading={loading}
          pagination={{
            current: page,
            pageSize,
            total,
            showSizeChanger: true,
            showQuickJumper: true,
            showTotal: (t) => `共 ${t} 条`,
            onChange: (p, ps) => { setPage(p); setPageSize(ps) },
          }}
        />
      </Card>

      <Modal
        title="创建用户"
        open={createOpen}
        onCancel={() => { setCreateOpen(false); form.resetFields() }}
        onOk={handleCreate}
        confirmLoading={createLoading}
        width={500}
      >
        <Form form={form} layout="vertical" style={{ marginTop: 16 }}>
          <Form.Item
            name="username"
            label="用户名"
            rules={[{ required: true, message: '请输入用户名' }]}
          >
            <Input placeholder="请输入用户名" />
          </Form.Item>
          <Form.Item
            name="password"
            label="密码"
            rules={[{ required: true, message: '请输入密码' }, { min: 6, message: '密码至少6位' }]}
          >
            <Input.Password placeholder="请输入密码" />
          </Form.Item>
          <Form.Item name="full_name" label="姓名">
            <Input placeholder="请输入姓名" />
          </Form.Item>
          <Form.Item name="email" label="邮箱">
            <Input placeholder="请输入邮箱" />
          </Form.Item>
          <Form.Item name="role_ids" label="分配角色">
            <Checkbox.Group style={{ width: '100%' }}>
              <Space direction="vertical" style={{ width: '100%' }}>
                {roles.map((role) => (
                  <Checkbox key={role.id} value={role.id}>
                    <Space>
                      <strong>{role.name}</strong>
                      <span style={{ color: 'var(--text-secondary)', fontSize: 12 }}>
                        {role.description || `${role.permissions?.length || 0} 个权限`}
                      </span>
                    </Space>
                  </Checkbox>
                ))}
              </Space>
            </Checkbox.Group>
          </Form.Item>
        </Form>
      </Modal>

      {/* 分配角色弹窗 */}
      <Modal
        title={
          <Space>
            <SafetyCertificateOutlined />
            <span>分配角色 - {currentUser?.username}</span>
          </Space>
        }
        open={assignOpen}
        onCancel={() => setAssignOpen(false)}
        onOk={handleAssignRoles}
        confirmLoading={assignLoading}
        width={500}
      >
        <div style={{ marginTop: 16, marginBottom: 16 }}>
          <div style={{ marginBottom: 12, color: 'var(--text-secondary)' }}>
            为用户 <strong>{currentUser?.username}</strong> 分配角色，用户将获得所选角色的所有权限。
          </div>
          <Checkbox.Group
            style={{ width: '100%' }}
            value={selectedRoleIds}
            onChange={(vals) => setSelectedRoleIds(vals as number[])}
          >
            <Space direction="vertical" style={{ width: '100%' }}>
              {roles.map((role) => (
                <Checkbox key={role.id} value={role.id}>
                  <Space>
                    <strong>{role.name}</strong>
                    <Tag color="blue" style={{ marginLeft: 8 }}>
                      {role.permissions?.length || 0} 权限
                    </Tag>
                    <span style={{ color: 'var(--text-secondary)', fontSize: 12 }}>
                      {role.description}
                    </span>
                  </Space>
                </Checkbox>
              ))}
            </Space>
          </Checkbox.Group>
        </div>
      </Modal>

      {/* 编辑用户弹窗 */}
      <Modal
        title={
          <Space>
            <UserOutlined />
            <span>编辑用户 - {currentUser?.username}</span>
          </Space>
        }
        open={editOpen}
        onCancel={() => setEditOpen(false)}
        onOk={handleEditUser}
        confirmLoading={editLoading}
        width={500}
      >
        <Form form={editForm} layout="vertical" style={{ marginTop: 16 }}>
          <Form.Item name="full_name" label="姓名">
            <Input placeholder="请输入姓名" />
          </Form.Item>
          <Form.Item name="email" label="邮箱">
            <Input placeholder="请输入邮箱" />
          </Form.Item>
          <Form.Item name="status" label="状态">
            <Select>
              <Select.Option value="active">正常</Select.Option>
              <Select.Option value="disabled">禁用</Select.Option>
            </Select>
          </Form.Item>
        </Form>
      </Modal>

      {/* 重置密码弹窗 */}
      <Modal
        title={
          <Space>
            <LockOutlined />
            <span>重置密码 - {currentUser?.username}</span>
          </Space>
        }
        open={passwordOpen}
        onCancel={() => setPasswordOpen(false)}
        onOk={handleResetPassword}
        confirmLoading={passwordLoading}
        width={450}
      >
        <div style={{ marginTop: 16, marginBottom: 12, color: 'var(--text-secondary)' }}>
          为用户 <strong>{currentUser?.username}</strong> 设置新密码，密码至少 6 位。
        </div>
        <Form form={passwordForm} layout="vertical">
          <Form.Item
            name="password"
            label="新密码"
            rules={[{ required: true, message: '请输入新密码' }, { min: 6, message: '密码至少6位' }]}
          >
            <Input.Password placeholder="请输入新密码" />
          </Form.Item>
          <Form.Item
            name="confirmPassword"
            label="确认密码"
            dependencies={['password']}
            rules={[
              { required: true, message: '请确认新密码' },
              ({ getFieldValue }) => ({
                validator(_, value) {
                  if (!value || getFieldValue('password') === value) {
                    return Promise.resolve()
                  }
                  return Promise.reject(new Error('两次输入的密码不一致'))
                },
              }),
            ]}
          >
            <Input.Password placeholder="请再次输入新密码" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
