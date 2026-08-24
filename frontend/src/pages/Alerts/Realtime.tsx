import { useEffect, useState, useCallback } from 'react'
import {
  Table, Tag, Card, Button, Spin, Space, Select, Input, Drawer, Descriptions,
  Modal, message, Badge, Empty, Tooltip, Alert as AlertBanner,
} from 'antd'
import {
  ReloadOutlined, CheckCircleOutlined, StopOutlined,
  ExclamationCircleOutlined, SearchOutlined, FireOutlined, ThunderboltOutlined,
} from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { alertsApi } from '@/api/alerts'
import type { Alert, PageResult } from '@/types'
import IncidentDetail from '@/pages/AIOps/IncidentDetail'
import dayjs from 'dayjs'
import { useDataTrust } from '@/hooks/useDataTrust'
import { DataTrustIndicator } from '@/components/DataTrustIndicator'

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

export default function AlertList() {
  const navigate = useNavigate()
  const [data, setData] = useState<PageResult<Alert> | null>(null)
  const [loading, setLoading] = useState(false)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [statusFilter, setStatusFilter] = useState<string>('firing')
  const [severityFilter, setSeverityFilter] = useState<string>('')
  const [keyword, setKeyword] = useState('')
  const [detail, setDetail] = useState<Alert | null>(null)
  const [detailOpen, setDetailOpen] = useState(false)
  const [detailLoading, setDetailLoading] = useState(false)
  // IncidentDetail Drawer
  const [incidentDetailId, setIncidentDetailId] = useState<number | null>(null)
  const [incidentDetailOpen, setIncidentDetailOpen] = useState(false)

  // P1-X.9: 统一 Data Trust（Alert 来自 Alertmanager）
  const trust = useDataTrust({ source: 'alertmanager' })

  const fetchData = useCallback(async () => {
    const seq = trust.beginFetch()
    setLoading(true)
    try {
      const params: any = { page, page_size: pageSize }
      if (statusFilter) params.status = statusFilter
      if (severityFilter) params.severity = severityFilter
      const res = await alertsApi.list(params)
      trust.markSuccess(seq)
      setData(res)
    } catch (err: any) {
      trust.markError(seq, err?.message || '加载告警列表失败')
    } finally {
      setLoading(false)
    }
  }, [page, pageSize, statusFilter, severityFilter])

  useEffect(() => {
    fetchData()
    const timer = setInterval(fetchData, 30000)
    return () => clearInterval(timer)
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

  const handleAcknowledge = (record: Alert) => {
    Modal.confirm({
      title: '确认告警',
      content: `确认告警 "${record.alertname}" ？确认后状态变为已确认。`,
      okText: '确认',
      cancelText: '取消',
      onOk: async () => {
        await alertsApi.acknowledge(record.id)
        message.success('告警已确认')
        fetchData()
        if (detail?.id === record.id) setDetail({ ...detail, status: 'acknowledged' })
      },
    })
  }

  const handleResolve = (record: Alert) => {
    Modal.confirm({
      title: '解决告警',
      content: `标记告警 "${record.alertname}" 为已解决？`,
      okText: '解决',
      okType: 'danger',
      cancelText: '取消',
      onOk: async () => {
        await alertsApi.resolve(record.id)
        message.success('告警已解决')
        fetchData()
        if (detail?.id === record.id) setDetail({ ...detail, status: 'resolved' })
      },
    })
  }

  const filtered = data?.items?.filter((a) =>
    !keyword ||
    a.alertname.toLowerCase().includes(keyword.toLowerCase()) ||
    (a.service && a.service.toLowerCase().includes(keyword.toLowerCase())) ||
    (a.pod && a.pod.toLowerCase().includes(keyword.toLowerCase()))
  ) || []

  const columns = [
    {
      title: '告警名称',
      dataIndex: 'alertname',
      key: 'alertname',
      render: (t: string, r: Alert) => (
        <a onClick={() => handleViewDetail(r)} style={{ fontWeight: 500 }}>{t}</a>
      ),
    },
    {
      title: '级别',
      dataIndex: 'severity',
      key: 'severity',
      width: 100,
      render: (s: string) => <Tag color={severityColor[s] || 'default'}>{s?.toUpperCase()}</Tag>,
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 110,
      render: (s: string) => <Badge status={statusColor[s] as any} text={s} />,
    },
    { title: '服务', dataIndex: 'service', key: 'service', width: 140, render: (v: string) => v || '-' },
    { title: 'Pod', dataIndex: 'pod', key: 'pod', width: 180, render: (v: string) => v || '-' },
    { title: 'Namespace', dataIndex: 'namespace', key: 'namespace', width: 140, render: (v: string) => v ? <Tag>{v}</Tag> : '-' },
    { title: 'Node', dataIndex: 'node', key: 'node', width: 120, render: (v: string) => v || '-' },
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
    {
      title: '开始时间',
      dataIndex: 'starts_at',
      key: 'starts_at',
      width: 170,
      render: (v: string) => v ? dayjs(v).format('YYYY-MM-DD HH:mm:ss') : '-',
    },
    {
      title: '操作',
      key: 'action',
      width: 160,
      render: (_: any, r: Alert) => (
        <Space size="small">
          {r.status === 'firing' && (
            <Tooltip title="确认告警">
              <Button type="link" size="small" icon={<CheckCircleOutlined />} onClick={() => handleAcknowledge(r)}>确认</Button>
            </Tooltip>
          )}
          {r.status !== 'resolved' && (
            <Tooltip title="标记解决">
              <Button type="link" size="small" danger icon={<StopOutlined />} onClick={() => handleResolve(r)}>解决</Button>
            </Tooltip>
          )}
        </Space>
      ),
    },
  ]

  return (
    <div style={{ padding: '16px 24px' }}>
      <AlertBanner
        type="info"
        showIcon
        message="Alert Intelligence"
        description={
          <Space wrap>
            <Button type="primary" size="small">实时告警</Button>
            <Button size="small" icon={<FireOutlined />} onClick={() => navigate('/alerts/aggregate')}>告警聚合</Button>
            <Button size="small" icon={<ThunderboltOutlined />} onClick={() => navigate('/alerts/noise')}>告警降噪</Button>
            <Button size="small" onClick={() => navigate('/alerts/history')}>告警历史</Button>
          </Space>
        }
        style={{ marginBottom: 12 }}
      />
    <Card
      title={
        <Space>
          <ExclamationCircleOutlined style={{ color: '#faad14' }} />
          实时告警
          {data && <Tag color="blue">共 {data.total} 条</Tag>}
        </Space>
      }
      extra={
        <Space wrap>
          <Select
            value={statusFilter}
            onChange={setStatusFilter}
            style={{ width: 120 }}
            allowClear
            placeholder="状态"
            options={[
              { label: 'Firing', value: 'firing' },
              { label: '已确认', value: 'acknowledged' },
              { label: '已解决', value: 'resolved' },
            ]}
          />
          <Select
            value={severityFilter}
            onChange={setSeverityFilter}
            style={{ width: 120 }}
            allowClear
            placeholder="级别"
            options={[
              { label: 'Critical', value: 'critical' },
              { label: 'Warning', value: 'warning' },
              { label: 'Info', value: 'info' },
            ]}
          />
          <Input.Search
            placeholder="搜索告警/服务/Pod"
            value={keyword}
            onChange={(e) => setKeyword(e.target.value)}
            style={{ width: 220 }}
            allowClear
            prefix={<SearchOutlined />}
          />
          <Button icon={<ReloadOutlined />} onClick={fetchData} loading={loading}>刷新</Button>
        </Space>
      }
    >
      <div style={{ marginBottom: 12 }}>
        <DataTrustIndicator
          status={trust.status}
          lastSuccessfulAt={trust.lastSuccessfulAt}
          ageSeconds={trust.ageSeconds}
          sourceLabel={trust.sourceLabel}
          error={trust.error}
          formatAge={trust.formatAge}
          formatLastSuccessful={trust.formatLastSuccessful}
        />
      </div>
      {trust.error && trust.status === 'stale' && (
        <AlertBanner message="数据刷新失败" description={trust.error + '（当前显示的是上次成功获取的数据）'} type="warning" showIcon style={{ marginBottom: 12 }} />
      )}
      <Spin spinning={loading}>
        {filtered.length > 0 || loading ? (
          <Table
            scroll={{ x: 'max-content' }}
            columns={columns}
            dataSource={filtered}
            rowKey="id"
            pagination={{
              current: page,
              pageSize,
              total: data?.total || 0,
              showSizeChanger: true,
              showTotal: (t) => `共 ${t} 条`,
              onChange: (p, ps) => { setPage(p); setPageSize(ps) },
            }}
            size="middle"
          />
        ) : (
          <Empty description={statusFilter === 'firing' ? '暂无活动告警' : '暂无告警数据'} />
        )}
      </Spin>

      <Drawer
        title="告警详情"
        open={detailOpen}
        onClose={() => setDetailOpen(false)}
        width={560}
        loading={detailLoading}
        extra={
          detail && (
            <Space>
              {detail.status === 'firing' && (
                <Button icon={<CheckCircleOutlined />} onClick={() => handleAcknowledge(detail)}>确认</Button>
              )}
              {detail.status !== 'resolved' && (
                <Button danger icon={<StopOutlined />} onClick={() => handleResolve(detail)}>解决</Button>
              )}
            </Space>
          )
        }
      >
        {detail && (
          <Space direction="vertical" style={{ width: '100%' }} size="large">
            <Descriptions column={1} bordered size="small">
              <Descriptions.Item label="告警名称">{detail.alertname}</Descriptions.Item>
              <Descriptions.Item label="级别">
                <Tag color={severityColor[detail.severity] || 'default'}>{detail.severity?.toUpperCase()}</Tag>
              </Descriptions.Item>
              <Descriptions.Item label="状态">
                <Badge status={statusColor[detail.status] as any} text={detail.status} />
              </Descriptions.Item>
              <Descriptions.Item label="Fingerprint">{detail.fingerprint || '-'}</Descriptions.Item>
              <Descriptions.Item label="服务">{detail.service || '-'}</Descriptions.Item>
              <Descriptions.Item label="Pod">{detail.pod || '-'}</Descriptions.Item>
              <Descriptions.Item label="Namespace">{detail.namespace || '-'}</Descriptions.Item>
              <Descriptions.Item label="Node">{detail.node || '-'}</Descriptions.Item>
              <Descriptions.Item label="Instance">{detail.instance || '-'}</Descriptions.Item>
              <Descriptions.Item label="开始时间">{detail.starts_at ? dayjs(detail.starts_at).format('YYYY-MM-DD HH:mm:ss') : '-'}</Descriptions.Item>
              <Descriptions.Item label="结束时间">{detail.ends_at ? dayjs(detail.ends_at).format('YYYY-MM-DD HH:mm:ss') : '-'}</Descriptions.Item>
            </Descriptions>

            {detail.labels && Object.keys(detail.labels).length > 0 && (
              <div>
                <div style={{ fontWeight: 600, marginBottom: 8 }}>Labels</div>
                <Space wrap>
                  {Object.entries(detail.labels).map(([k, v]) => (
                    <Tag key={k}>{k}={v}</Tag>
                  ))}
                </Space>
              </div>
            )}

            {detail.annotations && Object.keys(detail.annotations).length > 0 && (
              <div>
                <div style={{ fontWeight: 600, marginBottom: 8 }}>Annotations</div>
                <Descriptions column={1} size="small" bordered>
                  {Object.entries(detail.annotations).map(([k, v]) => (
                    <Descriptions.Item key={k} label={k}>{String(v)}</Descriptions.Item>
                  ))}
                </Descriptions>
              </div>
            )}
          </Space>
        )}
      </Drawer>
    </Card>

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
