import { useEffect, useState, useCallback } from 'react'
import {
  Table, Tag, Card, Button, Spin, Space, Select, Input, Badge, Empty, Tooltip,
} from 'antd'
import { ReloadOutlined, SearchOutlined, EyeOutlined } from '@ant-design/icons'
import { incidentApi } from '@/api/incident'
import type { Incident, PageResult } from '@/types'
import dayjs from 'dayjs'
import IncidentDetail from './IncidentDetail'

const severityColor: Record<string, string> = {
  critical: 'error',
  warning: 'warning',
  info: 'info',
}

const statusColor: Record<string, string> = {
  open: 'error',
  acknowledged: 'warning',
  resolved: 'success',
  closed: 'default',
}

const statusLabel: Record<string, string> = {
  open: '进行中',
  acknowledged: '已确认',
  resolved: '已解决',
  closed: '已关闭',
}

export default function Incidents() {
  const [data, setData] = useState<PageResult<Incident> | null>(null)
  const [loading, setLoading] = useState(false)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [statusFilter, setStatusFilter] = useState<string>('')
  const [severityFilter, setSeverityFilter] = useState<string>('')
  const [namespaceFilter, setNamespaceFilter] = useState<string>('')
  const [keyword, setKeyword] = useState('')
  const [detailId, setDetailId] = useState<number | null>(null)
  const [detailOpen, setDetailOpen] = useState(false)

  const fetchData = useCallback(async () => {
    setLoading(true)
    try {
      const params: any = { page, page_size: pageSize }
      if (statusFilter) params.status = statusFilter
      if (severityFilter) params.severity = severityFilter
      if (namespaceFilter) params.namespace = namespaceFilter
      if (keyword) params.keyword = keyword
      const res = await incidentApi.list(params)
      setData(res)
    } finally {
      setLoading(false)
    }
  }, [page, pageSize, statusFilter, severityFilter, namespaceFilter, keyword])

  useEffect(() => {
    fetchData()
    const timer = setInterval(fetchData, 30000)
    return () => clearInterval(timer)
  }, [fetchData])

  const handleViewDetail = (record: Incident) => {
    setDetailId(record.id)
    setDetailOpen(true)
  }

  const columns = [
    {
      title: 'ID',
      dataIndex: 'id',
      width: 70,
      render: (v: number) => <span style={{ color: '#999' }}>#{v}</span>,
    },
    {
      title: '标题',
      dataIndex: 'title',
      ellipsis: true,
      render: (v: string, record: Incident) => (
        <a onClick={() => handleViewDetail(record)} style={{ fontWeight: 500 }}>
          {v}
        </a>
      ),
    },
    {
      title: '严重度',
      dataIndex: 'severity',
      width: 90,
      render: (v: string) => (
        <Tag color={severityColor[v] || 'default'} style={{ margin: 0 }}>
          {v?.toUpperCase()}
        </Tag>
      ),
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 100,
      render: (v: string) => (
        <Badge color={statusColor[v] || 'default'} text={statusLabel[v] || v} />
      ),
    },
    {
      title: '服务',
      dataIndex: 'service',
      width: 140,
      render: (v: string) => v || <span style={{ color: '#999' }}>-</span>,
    },
    {
      title: '命名空间',
      dataIndex: 'namespace',
      width: 120,
      render: (v: string) => v || <span style={{ color: '#999' }}>-</span>,
    },
    {
      title: '信号数',
      dataIndex: 'signal_count',
      width: 80,
      render: (v: number) => (
        <Tag color={v > 0 ? 'blue' : 'default'}>{v}</Tag>
      ),
    },
    {
      title: '开始时间',
      dataIndex: 'start_time',
      width: 170,
      render: (v: string) => dayjs(v).format('YYYY-MM-DD HH:mm:ss'),
    },
    {
      title: '持续时间',
      dataIndex: 'duration',
      width: 90,
      render: (v: string, record: Incident) => {
        if (record.status === 'resolved' || record.status === 'closed') {
          return record.end_time
            ? dayjs(record.end_time).diff(dayjs(record.start_time), 'minute') + 'm'
            : '-'
        }
        return dayjs().diff(dayjs(record.start_time), 'minute') + 'm'
      },
    },
    {
      title: '操作',
      width: 80,
      render: (_: any, record: Incident) => (
        <Tooltip title="查看详情">
          <Button type="link" size="small" icon={<EyeOutlined />} onClick={() => handleViewDetail(record)} />
        </Tooltip>
      ),
    },
  ]

  return (
    <div>
      <Card
        title="事件中心"
        extra={
          <Space>
            <Input
              placeholder="搜索标题/摘要"
              prefix={<SearchOutlined />}
              value={keyword}
              onChange={(e) => { setKeyword(e.target.value); setPage(1) }}
              style={{ width: 200 }}
              allowClear
            />
            <Select
              placeholder="状态"
              value={statusFilter || undefined}
              onChange={(v) => { setStatusFilter(v || ''); setPage(1) }}
              style={{ width: 110 }}
              allowClear
              options={[
                { value: 'open', label: '进行中' },
                { value: 'acknowledged', label: '已确认' },
                { value: 'resolved', label: '已解决' },
                { value: 'closed', label: '已关闭' },
              ]}
            />
            <Select
              placeholder="严重度"
              value={severityFilter || undefined}
              onChange={(v) => { setSeverityFilter(v || ''); setPage(1) }}
              style={{ width: 100 }}
              allowClear
              options={[
                { value: 'critical', label: 'Critical' },
                { value: 'warning', label: 'Warning' },
                { value: 'info', label: 'Info' },
              ]}
            />
            <Select
              placeholder="命名空间"
              value={namespaceFilter || undefined}
              onChange={(v) => { setNamespaceFilter(v || ''); setPage(1) }}
              style={{ width: 130 }}
              allowClear
              options={[
                { value: 'default', label: 'default' },
                { value: 'monitoring', label: 'monitoring' },
                { value: 'kube-system', label: 'kube-system' },
              ]}
            />
            <Button icon={<ReloadOutlined />} onClick={fetchData}>刷新</Button>
          </Space>
        }
      >
        <Spin spinning={loading}>
          {data && data.items.length > 0 ? (
            <Table
              scroll={{ x: 'max-content' }}
              dataSource={data.items}
              columns={columns}
              rowKey="id"
              pagination={{
                current: page,
                pageSize,
                total: data.total,
                showSizeChanger: true,
                showTotal: (t) => `共 ${t} 条`,
                onChange: (p, ps) => { setPage(p); setPageSize(ps) },
              }}
              size="middle"
            />
          ) : (
            <Empty description={loading ? '加载中...' : '暂无事件'} />
          )}
        </Spin>
      </Card>

      <IncidentDetail
        id={detailId}
        open={detailOpen}
        onClose={() => setDetailOpen(false)}
        onChanged={fetchData}
      />
    </div>
  )
}
