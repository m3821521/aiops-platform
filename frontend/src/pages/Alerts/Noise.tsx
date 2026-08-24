import { useEffect, useState, useCallback, useRef } from 'react'
import {
  Table, Tag, Card, Button, Spin, Space, Select, Row, Col, Statistic, Empty, Alert, Tooltip, Typography, Progress,
} from 'antd'
import {
  ReloadOutlined, AlertOutlined, FireOutlined, NodeIndexOutlined,
  AppstoreOutlined, ThunderboltOutlined, CheckCircleOutlined,
} from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { alertsApi } from '@/api/alerts'
import type { AlertGroup, NoiseResult } from '@/types'
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

export default function AlertNoise() {
  const navigate = useNavigate()
  const [dimension, setDimension] = useState<Dimension>('service')
  const [result, setResult] = useState<NoiseResult | null>(null)
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
      const res = await alertsApi.noise(dim)
      if (token !== requestTokenRef.current) return
      setResult(res)
    } catch (err: any) {
      if (token !== requestTokenRef.current) return
      setError(err?.message || '降噪数据加载失败')
      setResult(null)
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

  const groups = result?.groups || []
  const totalBefore = result?.total_before || 0
  const totalAfter = result?.total_after || 0
  const noiseReduced = totalBefore - totalAfter
  const reductionRate = totalBefore > 0 ? (noiseReduced / totalBefore) * 100 : 0
  const isStorm = result?.is_storm || false
  const stormReason = result?.storm_reason || ''

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
      title: '降噪后告警数', dataIndex: 'count', width: 120,
      render: (v: number) => <Tag color={v >= 5 ? 'error' : v >= 3 ? 'warning' : 'success'} style={{ margin: 0 }}>{v}</Tag>,
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
      render: () => (
        <Button type="link" size="small" onClick={() => navigate('/alerts/realtime')}>查看告警</Button>
      ),
    },
  ]

  return (
    <div style={{ padding: '16px 24px' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
        <div>
          <Title level={4} style={{ margin: 0 }}>
            <ThunderboltOutlined style={{ color: '#8b5cf6', marginRight: 8 }} />
            告警降噪
          </Title>
          <Text type="secondary" style={{ fontSize: 12 }}>识别低价值告警和告警风暴，减少干扰</Text>
        </div>
        <Space>
          <Select value={dimension} onChange={handleDimensionChange} style={{ width: 140 }}>
            <Select.Option value="service">按服务降噪</Select.Option>
            <Select.Option value="node">按节点降噪</Select.Option>
            <Select.Option value="namespace">按命名空间降噪</Select.Option>
          </Select>
          <Button icon={<ReloadOutlined />} onClick={handleRefresh} loading={loading}>刷新</Button>
        </Space>
      </div>

      {/* Storm Warning */}
      {isStorm && (
        <Alert
          type="warning"
          showIcon
          icon={<FireOutlined />}
          message="检测到告警风暴"
          description={stormReason || '当前时间窗口内告警数量异常，建议关注'}
          style={{ marginBottom: 12 }}
          action={<Button size="small" type="primary" onClick={() => navigate('/alerts/realtime')}>查看实时告警</Button>}
        />
      )}

      {/* Summary */}
      <Row gutter={[12, 12]} style={{ marginBottom: 12 }}>
        <Col xs={12} sm={6}>
          <Card size="small">
            <Statistic title="降噪前告警数" value={totalBefore} prefix={<AlertOutlined />} />
          </Card>
        </Col>
        <Col xs={12} sm={6}>
          <Card size="small">
            <Statistic title="降噪后告警数" value={totalAfter} prefix={<CheckCircleOutlined />} valueStyle={{ color: '#16a34a' }} />
          </Card>
        </Col>
        <Col xs={12} sm={6}>
          <Card size="small">
            <Statistic title="已降噪告警数" value={noiseReduced} prefix={<ThunderboltOutlined />} valueStyle={{ color: '#8b5cf6' }} />
          </Card>
        </Col>
        <Col xs={12} sm={6}>
          <Card size="small">
            <div style={{ marginBottom: 4 }}><Text type="secondary" style={{ fontSize: 13 }}>降噪率</Text></div>
            <Progress percent={Math.round(reductionRate)} status={reductionRate > 50 ? 'success' : 'normal'} />
          </Card>
        </Col>
      </Row>

      {/* Table */}
      <Card size="small" title={<Space><AppstoreOutlined />降噪后告警组 ({groups.length})</Space>}>
        {error ? (
          <Alert type="error" message={error} showIcon action={<Button size="small" onClick={handleRefresh}>重试</Button>} />
        ) : loading && groups.length === 0 ? (
          <div style={{ textAlign: 'center', padding: 48 }}><Spin size="large" /></div>
        ) : groups.length === 0 ? (
          <Empty description="暂无降噪告警数据" image={Empty.PRESENTED_IMAGE_SIMPLE} />
        ) : (
          <Table
            dataSource={groups}
            columns={columns}
            rowKey="key"
            size="small"
            loading={loading}
            pagination={{ pageSize: 20, showSizeChanger: true, showTotal: (t) => `共 ${t} 组` }}
            scroll={{ x: 1000 }}
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
              <Button type="link" size="small" onClick={() => navigate('/alerts/aggregate')}>告警聚合</Button>
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
