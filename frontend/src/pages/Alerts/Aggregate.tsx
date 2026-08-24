import { useEffect, useState, useCallback, useRef } from 'react'
import {
  Table, Tag, Card, Button, Spin, Space, Select, Row, Col, Statistic, Empty, Alert, Badge, Tooltip, Typography,
} from 'antd'
import {
  ReloadOutlined, AlertOutlined, FireOutlined, NodeIndexOutlined,
  AppstoreOutlined, ClockCircleOutlined,
} from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { alertsApi } from '@/api/alerts'
import type { AlertGroup } from '@/types'
import IncidentDetail from '@/pages/AIOps/IncidentDetail'
import dayjs from 'dayjs'

const { Title, Text } = Typography

const severityColor: Record<string, string> = {
  critical: 'error',
  warning: 'warning',
  info: 'info',
}

const severityLabel: Record<string, string> = {
  critical: '严重',
  warning: '警告',
  info: '信息',
}

type Dimension = 'service' | 'node' | 'namespace'

export default function AlertAggregate() {
  const navigate = useNavigate()
  const [dimension, setDimension] = useState<Dimension>('service')
  const [groups, setGroups] = useState<AlertGroup[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const requestTokenRef = useRef(0)
  // IncidentDetail Drawer
  const [incidentDetailId, setIncidentDetailId] = useState<number | null>(null)
  const [incidentDetailOpen, setIncidentDetailOpen] = useState(false)

  const fetchData = useCallback(async (dim: Dimension) => {
    const token = ++requestTokenRef.current
    setLoading(true)
    setError('')
    try {
      const res = await alertsApi.aggregate(dim)
      if (token !== requestTokenRef.current) return
      setGroups(Array.isArray(res) ? res : [])
    } catch (err: any) {
      if (token !== requestTokenRef.current) return
      setError(err?.message || '聚合数据加载失败')
      setGroups([])
    } finally {
      if (token === requestTokenRef.current) setLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchData(dimension)
  }, [dimension, fetchData])

  const handleDimensionChange = (val: Dimension) => {
    setDimension(val)
  }

  const handleRefresh = () => {
    fetchData(dimension)
  }

  // Summary 统计
  const totalAlerts = groups.reduce((sum, g) => sum + g.count, 0)
  const criticalCount = groups.filter((g) => g.severity === 'critical').length
  const warningCount = groups.filter((g) => g.severity === 'warning').length

  const dimensionLabel: Record<Dimension, string> = {
    service: '服务',
    node: '节点',
    namespace: '命名空间',
  }

  const columns = [
    {
      title: dimensionLabel[dimension], dataIndex: 'key', width: 200, ellipsis: true,
      render: (v: string, record: AlertGroup) => (
        <Tooltip title={v}>
          <Space>
            {dimension === 'service' && <AppstoreOutlined style={{ color: '#3b82f6' }} />}
            {dimension === 'node' && <NodeIndexOutlined style={{ color: '#3b82f6' }} />}
            {dimension === 'namespace' && <AlertOutlined style={{ color: '#3b82f6' }} />}
            <Text strong>{v || '-'}</Text>
          </Space>
        </Tooltip>
      ),
    },
    {
      title: '告警数', dataIndex: 'count', width: 100,
      render: (v: number) => <Tag color={v >= 5 ? 'error' : v >= 3 ? 'warning' : 'default'} style={{ margin: 0 }}>{v}</Tag>,
      sorter: (a: AlertGroup, b: AlertGroup) => a.count - b.count,
      defaultSortOrder: 'descend' as const,
    },
    {
      title: '严重度', dataIndex: 'severity', width: 100,
      render: (v: string) => <Tag color={severityColor[v] || 'default'} style={{ margin: 0 }}>{severityLabel[v] || v}</Tag>,
    },
    {
      title: '告警名称', dataIndex: 'alertnames', width: 250, ellipsis: true,
      render: (v: string[]) => (
        <Tooltip title={v?.join(', ')}>
          <Text type="secondary" style={{ fontSize: 12 }}>{v?.slice(0, 3).join(', ')}{v?.length > 3 ? ` 等${v.length}个` : ''}</Text>
        </Tooltip>
      ),
    },
    {
      title: '首次出现', dataIndex: 'starts_at', width: 160,
      render: (v: string) => <Text type="secondary" style={{ fontSize: 12 }}>{dayjs(v).format('YYYY-MM-DD HH:mm:ss')}</Text>,
    },
    {
      title: '最近出现', dataIndex: 'ends_at', width: 160,
      render: (v?: string) => v ? <Text type="secondary" style={{ fontSize: 12 }}>{dayjs(v).format('YYYY-MM-DD HH:mm:ss')}</Text> : <Text type="secondary">-</Text>,
    },
    {
      title: '关联 Incident', width: 140,
      render: (_: any, record: AlertGroup) => {
        // 从 group.Alerts 中收集所有 incident_ids（去重）
        const idSet = new Set<number>()
        record.alerts?.forEach((a) => {
          a.incident_ids?.forEach((id) => idSet.add(id))
        })
        const ids = Array.from(idSet)
        if (ids.length === 0) {
          return <Tag color="default" style={{ margin: 0 }}>未关联</Tag>
        }
        return (
          <Space size={4} wrap>
            {ids.map((id) => (
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
      title: '操作', width: 120,
      render: (_: any, record: AlertGroup) => (
        <Space size={4}>
          <Button type="link" size="small" onClick={() => navigate('/alerts/realtime')}>查看告警</Button>
        </Space>
      ),
    },
  ]

  return (
    <div style={{ padding: '16px 24px' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
        <div>
          <Title level={4} style={{ margin: 0 }}>
            <FireOutlined style={{ color: '#f59e0b', marginRight: 8 }} />
            告警聚合
          </Title>
          <Text type="secondary" style={{ fontSize: 12 }}>按维度聚合同类告警，快速识别影响范围</Text>
        </div>
        <Space>
          <Select value={dimension} onChange={handleDimensionChange} style={{ width: 140 }}>
            <Select.Option value="service">按服务聚合</Select.Option>
            <Select.Option value="node">按节点聚合</Select.Option>
            <Select.Option value="namespace">按命名空间聚合</Select.Option>
          </Select>
          <Button icon={<ReloadOutlined />} onClick={handleRefresh} loading={loading}>刷新</Button>
        </Space>
      </div>

      {/* Summary */}
      <Row gutter={[12, 12]} style={{ marginBottom: 12 }}>
        <Col xs={12} sm={6}>
          <Card size="small"><Statistic title="告警总数" value={totalAlerts} prefix={<AlertOutlined />} /></Card>
        </Col>
        <Col xs={12} sm={6}>
          <Card size="small"><Statistic title="聚合组数" value={groups.length} prefix={<AppstoreOutlined />} /></Card>
        </Col>
        <Col xs={12} sm={6}>
          <Card size="small"><Statistic title="严重组" value={criticalCount} valueStyle={{ color: '#dc2626' }} prefix={<FireOutlined />} /></Card>
        </Col>
        <Col xs={12} sm={6}>
          <Card size="small"><Statistic title="警告组" value={warningCount} valueStyle={{ color: '#d97706' }} prefix={<AlertOutlined />} /></Card>
        </Col>
      </Row>

      {/* Table */}
      <Card size="small">
        {error ? (
          <Alert type="error" message={error} showIcon action={<Button size="small" onClick={handleRefresh}>重试</Button>} />
        ) : loading && groups.length === 0 ? (
          <div style={{ textAlign: 'center', padding: 48 }}><Spin size="large" /></div>
        ) : groups.length === 0 ? (
          <Empty description="暂无聚合告警数据" image={Empty.PRESENTED_IMAGE_SIMPLE} />
        ) : (
          <Table
            dataSource={groups}
            columns={columns}
            rowKey="key"
            size="small"
            loading={loading}
            pagination={{ pageSize: 20, showSizeChanger: true, showTotal: (t) => `共 ${t} 组` }}
            scroll={{ x: 1100 }}
          />
        )}
      </Card>

      <div style={{ marginTop: 12 }}>
        <Alert
          type="info"
          showIcon
          message="Alert Intelligence 导航"
          description={
            <Space>
              <Button type="link" size="small" onClick={() => navigate('/alerts/realtime')}>实时告警</Button>
              <Button type="link" size="small" onClick={() => navigate('/alerts/history')}>告警历史</Button>
              <Button type="link" size="small" onClick={() => navigate('/alerts/noise')}>告警降噪</Button>
            </Space>
          }
        />
      </div>

      {/* IncidentDetail Drawer */}
      {incidentDetailId !== null && (
        <IncidentDetail
          id={incidentDetailId}
          open={incidentDetailOpen}
          onClose={() => setIncidentDetailOpen(false)}
          onChanged={() => fetchData(dimension)}
        />
      )}
    </div>
  )
}
