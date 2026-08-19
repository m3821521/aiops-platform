import { useEffect, useState } from 'react'
import {
  Card,
  Table,
  Tag,
  Space,
  Input,
  Select,
  Button,
  Row,
  Col,
  Badge,
  Empty,
} from 'antd'
import { ReloadOutlined } from '@ant-design/icons'
import { anomalyApi } from '../../api/anomaly'
import type { AnomalyRecord, AnomalyListFilter } from '../../types'
import AnomalyDetail from './AnomalyDetail'

const severityColor: Record<string, string> = {
  critical: 'red',
  warning: 'orange',
  info: 'blue',
}

const statusColor: Record<string, string> = {
  active: 'red',
  detected: 'orange',
  resolved: 'green',
}

export default function Anomaly() {
  const [data, setData] = useState<AnomalyRecord[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [filter, setFilter] = useState<AnomalyListFilter>({})
  const [keyword, setKeyword] = useState('')
  const [detail, setDetail] = useState<AnomalyRecord | null>(null)

  const fetchData = async () => {
    setLoading(true)
    try {
      const res = await anomalyApi.list({ ...filter, metric: keyword || undefined, page, page_size: pageSize })
      setData(res.items || [])
      setTotal(res.total || 0)
    } catch (e) {
      console.error('fetch anomaly failed', e)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchData()
  }, [page, pageSize, filter])

  // 30 秒自动刷新
  useEffect(() => {
    const timer = setInterval(fetchData, 30000)
    return () => clearInterval(timer)
  }, [filter, keyword])

  const columns = [
    {
      title: '时间',
      dataIndex: 'timestamp',
      width: 170,
      render: (v: string) => new Date(v).toLocaleString(),
    },
    {
      title: '指标',
      dataIndex: 'metric',
      ellipsis: true,
      width: 200,
    },
    {
      title: '资源',
      width: 160,
      render: (_: any, r: AnomalyRecord) => (
        <Space size={4}>
          {r.resource_type && <Tag>{r.resource_type}</Tag>}
          <span style={{ fontSize: 12 }}>{r.resource_name || '-'}</span>
        </Space>
      ),
    },
    {
      title: 'Namespace',
      dataIndex: 'namespace',
      width: 120,
      render: (v: string) => v || '-',
    },
    {
      title: '当前值',
      dataIndex: 'value',
      width: 90,
      render: (v: number) => v.toFixed(2),
    },
    {
      title: '基线',
      dataIndex: 'baseline',
      width: 90,
      render: (v?: number) => (v != null ? v.toFixed(2) : '-'),
    },
    {
      title: '异常分',
      dataIndex: 'anomaly_score',
      width: 90,
      render: (v: number) => (
        <Badge
          color={v >= 0.8 ? 'red' : v >= 0.5 ? 'orange' : 'blue'}
          text={`${(v * 100).toFixed(0)}%`}
        />
      ),
    },
    {
      title: '严重度',
      dataIndex: 'severity',
      width: 90,
      render: (v: string) => <Tag color={severityColor[v]}>{v}</Tag>,
    },
    {
      title: '算法',
      dataIndex: 'algorithm',
      width: 120,
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 90,
      render: (v: string) => <Tag color={statusColor[v]}>{v}</Tag>,
    },
    {
      title: '操作',
      width: 80,
      render: (_: any, r: AnomalyRecord) => (
        <Button type="link" size="small" onClick={() => setDetail(r)}>
          详情
        </Button>
      ),
    },
  ]

  return (
    <div>
      <Card
        title="异常检测"
        extra={
          <Button icon={<ReloadOutlined />} onClick={fetchData} loading={loading}>
            刷新
          </Button>
        }
      >
        <Row gutter={[12, 12]} style={{ marginBottom: 16 }}>
          <Col span={5}>
            <Input
              placeholder="搜索指标"
              value={keyword}
              onChange={(e) => setKeyword(e.target.value)}
              onPressEnter={() => {
                setPage(1)
                fetchData()
              }}
              allowClear
            />
          </Col>
          <Col span={3}>
            <Select
              placeholder="严重度"
              allowClear
              style={{ width: '100%' }}
              value={filter.severity}
              onChange={(v) => {
                setFilter({ ...filter, severity: v })
                setPage(1)
              }}
              options={[
                { label: 'Critical', value: 'critical' },
                { label: 'Warning', value: 'warning' },
                { label: 'Info', value: 'info' },
              ]}
            />
          </Col>
          <Col span={3}>
            <Select
              placeholder="状态"
              allowClear
              style={{ width: '100%' }}
              value={filter.status}
              onChange={(v) => {
                setFilter({ ...filter, status: v })
                setPage(1)
              }}
              options={[
                { label: 'Active', value: 'active' },
                { label: 'Detected', value: 'detected' },
                { label: 'Resolved', value: 'resolved' },
              ]}
            />
          </Col>
          <Col span={3}>
            <Select
              placeholder="资源类型"
              allowClear
              style={{ width: '100%' }}
              value={filter.resource_type}
              onChange={(v) => {
                setFilter({ ...filter, resource_type: v })
                setPage(1)
              }}
              options={[
                { label: 'Node', value: 'node' },
                { label: 'Pod', value: 'pod' },
                { label: 'Deployment', value: 'deployment' },
                { label: 'Service', value: 'service' },
              ]}
            />
          </Col>
          <Col span={3}>
            <Select
              placeholder="算法"
              allowClear
              style={{ width: '100%' }}
              value={filter.algorithm}
              onChange={(v) => {
                setFilter({ ...filter, algorithm: v })
                setPage(1)
              }}
              options={[
                { label: '静态阈值', value: 'static_threshold' },
                { label: '移动平均', value: 'moving_average' },
                { label: 'EWMA', value: 'ewma' },
                { label: 'Z-Score', value: 'z_score' },
              ]}
            />
          </Col>
        </Row>

        <Table
          rowKey="id"
          columns={columns}
          dataSource={data}
          loading={loading}
          pagination={{
            current: page,
            pageSize,
            total,
            showSizeChanger: true,
            onChange: (p, ps) => {
              setPage(p)
              setPageSize(ps)
            },
          }}
          locale={{
            emptyText: <Empty description="暂无异常数据" />,
          }}
          size="small"
        />
      </Card>

      <AnomalyDetail record={detail} onClose={() => setDetail(null)} onRefresh={fetchData} />
    </div>
  )
}
