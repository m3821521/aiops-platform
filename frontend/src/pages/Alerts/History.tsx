import { useEffect, useState, useCallback } from 'react'
import {
  Table, Tag, Card, Button, Spin, Space, Select, Input, Drawer, Descriptions,
  message, Empty, Tooltip, Alert as AlertBanner,
} from 'antd'
import {
  ReloadOutlined, SearchOutlined, FireOutlined, ThunderboltOutlined, ExclamationCircleOutlined,
} from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { alertsApi } from '@/api/alerts'
import type { Alert, PageResult } from '@/types'
import IncidentDetail from '@/pages/AIOps/IncidentDetail'
import dayjs from 'dayjs'

const severityColor: Record<string, string> = {
  critical: 'error',
  warning: 'warning',
  info: 'info',
}

const statusColor: Record<string, string> = {
  firing: 'error',
  acknowledged: 'warning',
  resolved: 'success',
}

const statusLabel: Record<string, string> = {
  firing: '触发中',
  acknowledged: '已确认',
  resolved: '已恢复',
}

const severityLabel: Record<string, string> = {
  critical: '严重',
  warning: '警告',
  info: '信息',
}

export default function AlertHistory() {
  const navigate = useNavigate()
  const [data, setData] = useState<PageResult<Alert> | null>(null)
  const [loading, setLoading] = useState(false)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [statusFilter, setStatusFilter] = useState<string>('')
  const [severityFilter, setSeverityFilter] = useState<string>('')
  const [keyword, setKeyword] = useState('')
  const [detail, setDetail] = useState<Alert | null>(null)
  const [detailOpen, setDetailOpen] = useState(false)
  const [detailLoading, setDetailLoading] = useState(false)
  // IncidentDetail Drawer
  const [incidentDetailId, setIncidentDetailId] = useState<number | null>(null)
  const [incidentDetailOpen, setIncidentDetailOpen] = useState(false)

  const fetchData = useCallback(async () => {
    setLoading(true)
    try {
      const params: any = { page, page_size: pageSize }
      if (statusFilter) params.status = statusFilter
      if (severityFilter) params.severity = severityFilter
      const res = await alertsApi.list(params)
      setData(res)
    } catch (e: any) {
      message.error('加载告警历史失败: ' + (e?.response?.data?.message || e.message))
    } finally {
      setLoading(false)
    }
  }, [page, pageSize, statusFilter, severityFilter])

  useEffect(() => {
    fetchData()
  }, [fetchData])

  const handleViewDetail = async (record: Alert) => {
    setDetailOpen(true)
    setDetailLoading(true)
    try {
      const res = await alertsApi.get(record.id)
      setDetail(res)
    } catch {
      setDetail(record)
    } finally {
      setDetailLoading(false)
    }
  }

  const handleSearch = () => {
    setPage(1)
    fetchData()
  }

  const handleReset = () => {
    setStatusFilter('')
    setSeverityFilter('')
    setKeyword('')
    setPage(1)
    setTimeout(fetchData, 0)
  }

  const columns = [
    {
      title: '告警名称',
      dataIndex: 'alertname',
      key: 'alertname',
      width: 200,
      render: (text: string, record: Alert) => (
        <a onClick={() => handleViewDetail(record)}>{text || '-'}</a>
      ),
    },
    {
      title: '级别',
      dataIndex: 'severity',
      key: 'severity',
      width: 100,
      render: (severity: string) => (
        <Tag color={severityColor[severity] || 'default'}>
          {severityLabel[severity] || severity}
        </Tag>
      ),
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 100,
      render: (status: string) => (
        <Tag color={statusColor[status] || 'default'}>
          {statusLabel[status] || status}
        </Tag>
      ),
    },
    {
      title: '命名空间',
      dataIndex: 'namespace',
      key: 'namespace',
      width: 120,
      render: (text: string) => text || '-',
    },
    {
      title: '服务',
      dataIndex: 'service',
      key: 'service',
      width: 150,
      render: (text: string) => text || '-',
    },
    {
      title: '实例',
      dataIndex: 'instance',
      key: 'instance',
      width: 150,
      render: (text: string) => text || '-',
    },
    {
      title: '触发时间',
      dataIndex: 'starts_at',
      key: 'starts_at',
      width: 180,
      render: (text: string) => text ? dayjs(text).format('YYYY-MM-DD HH:mm:ss') : '-',
    },
    {
      title: '恢复时间',
      dataIndex: 'ends_at',
      key: 'ends_at',
      width: 180,
      render: (text: string) => text ? dayjs(text).format('YYYY-MM-DD HH:mm:ss') : '-',
    },
    {
      title: '持续时间',
      key: 'duration',
      width: 120,
      render: (_: any, record: Alert) => {
        if (!record.starts_at) return '-'
        const start = dayjs(record.starts_at)
        const end = record.ends_at ? dayjs(record.ends_at) : dayjs()
        const diff = end.diff(start, 'minute')
        if (diff < 60) return `${diff} 分钟`
        const hours = Math.floor(diff / 60)
        const mins = diff % 60
        return `${hours} 小时 ${mins} 分`
      },
    },
    {
      title: '关联 Incident',
      key: 'incident',
      width: 140,
      render: (_: any, r: Alert) => {
        if (!r.incident_ids || r.incident_ids.length === 0) {
          return <Tag color="default" style={{ margin: 0 }}>未关联</Tag>
        }
        return (
          <Space size={4} wrap>
            {r.incident_ids.map((id) => (
              <Tag
                key={id}
                color="blue"
                style={{ margin: 0, cursor: 'pointer' }}
                onClick={() => {
                  setIncidentDetailId(id)
                  setIncidentDetailOpen(true)
                }}
              >
                #{id}
              </Tag>
            ))}
          </Space>
        )
      },
    },
  ]

  return (
    <div style={{ padding: 16 }}>
      <AlertBanner
        type="info"
        showIcon
        message="Alert Intelligence"
        description={
          <Space wrap>
            <Button size="small" icon={<ExclamationCircleOutlined />} onClick={() => navigate('/alerts/realtime')}>实时告警</Button>
            <Button type="primary" size="small">告警历史</Button>
            <Button size="small" icon={<FireOutlined />} onClick={() => navigate('/alerts/aggregate')}>告警聚合</Button>
            <Button size="small" icon={<ThunderboltOutlined />} onClick={() => navigate('/alerts/noise')}>告警降噪</Button>
          </Space>
        }
        style={{ marginBottom: 12 }}
      />
      <Card
        title="告警历史"
        extra={
          <Space>
            <Button icon={<ReloadOutlined />} onClick={fetchData}>刷新</Button>
          </Space>
        }
      >
        <Space style={{ marginBottom: 16 }} wrap>
          <Select
            placeholder="状态"
            allowClear
            style={{ width: 120 }}
            value={statusFilter || undefined}
            onChange={(v) => setStatusFilter(v || '')}
            options={[
              { value: 'firing', label: '触发中' },
              { value: 'acknowledged', label: '已确认' },
              { value: 'resolved', label: '已恢复' },
            ]}
          />
          <Select
            placeholder="级别"
            allowClear
            style={{ width: 120 }}
            value={severityFilter || undefined}
            onChange={(v) => setSeverityFilter(v || '')}
            options={[
              { value: 'critical', label: '严重' },
              { value: 'warning', label: '警告' },
              { value: 'info', label: '信息' },
            ]}
          />
          <Input
            placeholder="搜索告警名称"
            prefix={<SearchOutlined />}
            style={{ width: 200 }}
            value={keyword}
            onChange={(e) => setKeyword(e.target.value)}
            onPressEnter={handleSearch}
            allowClear
          />
          <Button type="primary" onClick={handleSearch}>查询</Button>
          <Button onClick={handleReset}>重置</Button>
        </Space>

        <Spin spinning={loading}>
          <Table
            scroll={{ x: 'max-content' }}
            rowKey="id"
            columns={columns}
            dataSource={data?.items || []}
            pagination={{
              current: page,
              pageSize,
              total: data?.total || 0,
              showSizeChanger: true,
              showQuickJumper: true,
              showTotal: (total) => `共 ${total} 条`,
              onChange: (p, ps) => {
                setPage(p)
                setPageSize(ps)
              },
            }}
            locale={{
              emptyText: <Empty description="暂无告警历史记录" />,
            }}
            size="middle"
          />
        </Spin>
      </Card>

      <Drawer
        title="告警详情"
        width={600}
        open={detailOpen}
        onClose={() => setDetailOpen(false)}
        loading={detailLoading}
      >
        {detail && (
          <Descriptions column={1} bordered size="small">
            <Descriptions.Item label="告警名称">{detail.alertname || '-'}</Descriptions.Item>
            <Descriptions.Item label="级别">
              <Tag color={severityColor[detail.severity] || 'default'}>
                {severityLabel[detail.severity] || detail.severity}
              </Tag>
            </Descriptions.Item>
            <Descriptions.Item label="状态">
              <Tag color={statusColor[detail.status] || 'default'}>
                {statusLabel[detail.status] || detail.status}
              </Tag>
            </Descriptions.Item>
            <Descriptions.Item label="命名空间">{detail.namespace || '-'}</Descriptions.Item>
            <Descriptions.Item label="服务">{detail.service || '-'}</Descriptions.Item>
            <Descriptions.Item label="实例">{detail.instance || '-'}</Descriptions.Item>
            <Descriptions.Item label="触发时间">
              {detail.starts_at ? dayjs(detail.starts_at).format('YYYY-MM-DD HH:mm:ss') : '-'}
            </Descriptions.Item>
            <Descriptions.Item label="恢复时间">
              {detail.ends_at ? dayjs(detail.ends_at).format('YYYY-MM-DD HH:mm:ss') : '-'}
            </Descriptions.Item>
            <Descriptions.Item label="描述">{detail.annotations?.description || '-'}</Descriptions.Item>
            <Descriptions.Item label="摘要">{detail.annotations?.summary || '-'}</Descriptions.Item>
            <Descriptions.Item label="指纹">{detail.fingerprint || '-'}</Descriptions.Item>
            <Descriptions.Item label="Labels">
              <pre style={{ margin: 0, whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}>
                {detail.labels ? JSON.stringify(detail.labels, null, 2) : '-'}
              </pre>
            </Descriptions.Item>
            <Descriptions.Item label="Annotations">
              <pre style={{ margin: 0, whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}>
                {detail.annotations ? JSON.stringify(detail.annotations, null, 2) : '-'}
              </pre>
            </Descriptions.Item>
          </Descriptions>
        )}
      </Drawer>

      {/* IncidentDetail Drawer */}
      {incidentDetailId !== null && (
        <IncidentDetail
          id={incidentDetailId}
          open={incidentDetailOpen}
          onClose={() => setIncidentDetailOpen(false)}
          onChanged={fetchData}
        />
      )}
    </div>
  )
}
