import { useState, useEffect } from 'react'
import {
  Card, Table, Button, Space, Tag, Input, Select, Drawer, Descriptions, Badge, message,
} from 'antd'
import {
  ReloadOutlined, FileTextOutlined, SearchOutlined, EyeOutlined,
} from '@ant-design/icons'
import { userApi } from '@/api/user'
import type { AuditLog } from '@/types'

const { Option } = Select

export default function AuditLogs() {
  const [logs, setLogs] = useState<AuditLog[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [detailOpen, setDetailOpen] = useState(false)
  const [currentLog, setCurrentLog] = useState<AuditLog | null>(null)

  // 筛选条件
  const [username, setUsername] = useState('')
  const [action, setAction] = useState<string | undefined>()
  const [resource, setResource] = useState<string | undefined>()
  const [result, setResult] = useState<string | undefined>()

  const loadLogs = async () => {
    setLoading(true)
    try {
      const res = await userApi.auditLogs({
        page,
        page_size: pageSize,
        username: username || undefined,
        action,
        resource,
        result,
      })
      setLogs(res.items || [])
      setTotal(res.total || 0)
    } catch (e: any) {
      message.error('加载审计日志失败: ' + (e?.response?.data?.message || e.message))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadLogs()
  }, [page, pageSize])

  const handleSearch = () => {
    setPage(1)
    loadLogs()
  }

  const handleReset = () => {
    setUsername('')
    setAction(undefined)
    setResource(undefined)
    setResult(undefined)
    setPage(1)
    setTimeout(loadLogs, 0)
  }

  const handleViewDetail = (log: AuditLog) => {
    setCurrentLog(log)
    setDetailOpen(true)
  }

  const actionColorMap: Record<string, string> = {
    GET: 'blue',
    POST: 'green',
    PUT: 'orange',
    PATCH: 'cyan',
    DELETE: 'red',
    LOGIN: 'purple',
    LOGOUT: 'geekblue',
  }

  const resultColorMap: Record<string, string> = {
    success: 'success',
    failed: 'error',
    denied: 'warning',
  }

  const columns = [
    {
      title: 'ID',
      dataIndex: 'id',
      width: 80,
    },
    {
      title: '时间',
      dataIndex: 'created_at',
      width: 170,
      render: (text: string) => new Date(text).toLocaleString('zh-CN'),
    },
    {
      title: '用户',
      dataIndex: 'username',
      width: 120,
      render: (text: string) => text || 'system',
    },
    {
      title: '操作',
      dataIndex: 'action',
      width: 100,
      render: (text: string) => (
        <Tag color={actionColorMap[text] || 'default'}>{text}</Tag>
      ),
    },
    {
      title: '资源',
      dataIndex: 'resource',
      width: 150,
      render: (text: string) => text || '-',
    },
    {
      title: '资源ID',
      dataIndex: 'resource_id',
      width: 120,
      render: (text: string) => text || '-',
    },
    {
      title: '结果',
      dataIndex: 'result',
      width: 100,
      render: (text: string) => (
        <Badge status={resultColorMap[text] as any || 'default'} text={text} />
      ),
    },
    {
      title: 'IP地址',
      dataIndex: 'ip',
      width: 140,
      render: (text: string) => text || '-',
    },
    {
      title: '操作',
      key: 'action',
      width: 100,
      render: (_: any, record: AuditLog) => (
        <Button
          size="small"
          type="link"
          icon={<EyeOutlined />}
          onClick={() => handleViewDetail(record)}
        >
          详情
        </Button>
      ),
    },
  ]

  return (
    <div>
      <Card
        title={
          <Space>
            <FileTextOutlined style={{ color: 'var(--color-primary)' }} />
            <span>审计日志</span>
            <Tag color="blue">{total} 条记录</Tag>
          </Space>
        }
        extra={
          <Button icon={<ReloadOutlined />} onClick={loadLogs}>刷新</Button>
        }
      >
        {/* 筛选栏 */}
        <Card size="small" style={{ marginBottom: 16, background: 'var(--color-bg-1)' }}>
          <Space wrap>
            <Input
              placeholder="用户名"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              style={{ width: 150 }}
              allowClear
            />
            <Select
              placeholder="操作类型"
              value={action}
              onChange={setAction}
              style={{ width: 130 }}
              allowClear
            >
              <Option value="GET">GET</Option>
              <Option value="POST">POST</Option>
              <Option value="PUT">PUT</Option>
              <Option value="DELETE">DELETE</Option>
              <Option value="LOGIN">LOGIN</Option>
              <Option value="LOGOUT">LOGOUT</Option>
            </Select>
            <Select
              placeholder="资源类型"
              value={resource}
              onChange={setResource}
              style={{ width: 130 }}
              allowClear
            >
              <Option value="users">users</Option>
              <Option value="roles">roles</Option>
              <Option value="incidents">incidents</Option>
              <Option value="alerts">alerts</Option>
              <Option value="actions">actions</Option>
              <Option value="workflows">workflows</Option>
              <Option value="pods">pods</Option>
            </Select>
            <Select
              placeholder="结果"
              value={result}
              onChange={setResult}
              style={{ width: 120 }}
              allowClear
            >
              <Option value="success">成功</Option>
              <Option value="failed">失败</Option>
              <Option value="denied">拒绝</Option>
            </Select>
            <Button type="primary" icon={<SearchOutlined />} onClick={handleSearch}>搜索</Button>
            <Button onClick={handleReset}>重置</Button>
          </Space>
        </Card>

        <Table
          columns={columns}
          dataSource={logs}
          rowKey="id"
          loading={loading}
          size="small"
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

      <Drawer
        title={
          <Space>
            <FileTextOutlined />
            <span>审计日志详情 #{currentLog?.id}</span>
          </Space>
        }
        open={detailOpen}
        onClose={() => setDetailOpen(false)}
        width={600}
      >
        {currentLog && (
          <div>
            <Descriptions column={1} bordered size="small">
              <Descriptions.Item label="日志ID">{currentLog.id}</Descriptions.Item>
              <Descriptions.Item label="时间">
                {new Date(currentLog.created_at).toLocaleString('zh-CN')}
              </Descriptions.Item>
              <Descriptions.Item label="用户">{currentLog.username || 'system'}</Descriptions.Item>
              <Descriptions.Item label="操作">
                <Tag color={actionColorMap[currentLog.action] || 'default'}>
                  {currentLog.action}
                </Tag>
              </Descriptions.Item>
              <Descriptions.Item label="资源类型">{currentLog.resource || '-'}</Descriptions.Item>
              <Descriptions.Item label="资源ID">{currentLog.resource_id || '-'}</Descriptions.Item>
              <Descriptions.Item label="结果">
                <Badge status={resultColorMap[currentLog.result] as any || 'default'} text={currentLog.result} />
              </Descriptions.Item>
              <Descriptions.Item label="IP地址">{currentLog.ip || '-'}</Descriptions.Item>
            </Descriptions>

            {currentLog.request && (
              <div style={{ marginTop: 24 }}>
                <div style={{ marginBottom: 8 }}>
                  <strong>请求内容</strong>
                </div>
                <pre style={{
                  background: 'var(--color-bg-1)',
                  padding: 12,
                  borderRadius: 6,
                  fontSize: 12,
                  overflow: 'auto',
                  maxHeight: 300,
                }}>
                  {currentLog.request}
                </pre>
              </div>
            )}
          </div>
        )}
      </Drawer>
    </div>
  )
}
