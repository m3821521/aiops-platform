import { useState } from 'react'
import {
  Card, Input, Button, Space, Select, Table, Spin, Alert, Tabs, Tag, Typography, Empty,
} from 'antd'
import { PlayCircleOutlined, CodeOutlined } from '@ant-design/icons'
import ReactECharts from 'echarts-for-react'
import { metricsApi } from '@/api/metrics'
import dayjs from 'dayjs'

const { TextArea } = Input
const { Text } = Typography

const examples = [
  'up',
  'rate(node_cpu_seconds_total{mode="idle"}[5m])',
  'node_memory_MemAvailable_bytes',
  'kube_pod_info',
  'kube_deployment_status_replicas',
  'count by(namespace)(kube_pod_info)',
]

const timeRanges = [
  { label: '最近 5 分钟', value: '5m', seconds: 300 },
  { label: '最近 15 分钟', value: '15m', seconds: 900 },
  { label: '最近 1 小时', value: '1h', seconds: 3600 },
  { label: '最近 6 小时', value: '6h', seconds: 21600 },
]

interface QueryResult {
  metric: Record<string, string>
  value?: [number, string]
  values?: [number, string][]
}

export default function PromQL() {
  const [query, setQuery] = useState('up')
  const [timeRange, setTimeRange] = useState('15m')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [result, setResult] = useState<QueryResult[]>([])
  const [resultType, setResultType] = useState('')
  const [mode, setMode] = useState<'instant' | 'range'>('instant')

  const executeQuery = async () => {
    if (!query.trim()) return
    setLoading(true)
    setError('')
    setResult([])
    try {
      if (mode === 'instant') {
        const res: any = await metricsApi.query(query)
        setResultType(res?.resultType || 'vector')
        setResult(res?.result || [])
      } else {
        const seconds = timeRanges.find((t) => t.value === timeRange)?.seconds || 900
        const end = dayjs()
        const start = end.subtract(seconds, 'second')
        const step = Math.max(15, Math.floor(seconds / 60))
        const res: any = await metricsApi.range({
          query,
          start: start.toISOString(),
          end: end.toISOString(),
          step: `${step}s`,
        })
        setResultType(res?.resultType || 'matrix')
        setResult(res?.result || [])
      }
    } catch (err: any) {
      setError(err?.message || '查询失败')
    } finally {
      setLoading(false)
    }
  }

  const instantColumns = [
    {
      title: 'Metric',
      key: 'metric',
      render: (_: any, record: QueryResult) => (
        <Space wrap>
          {Object.entries(record.metric).map(([k, v]) => (
            <Tag key={k}>{k}={v}</Tag>
          ))}
        </Space>
      ),
    },
    {
      title: 'Value',
      dataIndex: ['value', 1],
      key: 'value',
      width: 150,
      render: (v: string) => <Text code>{v}</Text>,
    },
    {
      title: 'Time',
      dataIndex: ['value', 0],
      key: 'time',
      width: 180,
      render: (t: number) => dayjs(t * 1000).format('YYYY-MM-DD HH:mm:ss'),
    },
  ]

  const buildRangeChart = () => {
    if (result.length === 0) return null
    const colors = ['#1677ff', '#52c41a', '#faad14', '#ff4d4f', '#722ed1', '#eb2f96']
    return {
      tooltip: { trigger: 'axis' },
      legend: { type: 'scroll', bottom: 0 },
      grid: { left: 50, right: 20, top: 20, bottom: 40 },
      xAxis: { type: 'time', axisLabel: { fontSize: 11 } },
      yAxis: { type: 'value', axisLabel: { fontSize: 11 } },
      series: result.slice(0, 6).map((r, i) => {
        const name = r.metric.__name__ || Object.entries(r.metric).filter(([k]) => k !== '__name__').map(([k, v]) => `${k}=${v}`).join(',') || `series-${i}`
        return {
          name,
          type: 'line',
          smooth: true,
          showSymbol: false,
          data: (r.values || []).map((v) => [v[0] * 1000, parseFloat(v[1])]),
          lineStyle: { color: colors[i % colors.length], width: 2 },
        }
      }),
    }
  }

  return (
    <div>
      <Space style={{ marginBottom: 16 }}>
        <CodeOutlined style={{ fontSize: 20, color: '#722ed1' }} />
        <Text strong style={{ fontSize: 18 }}>PromQL 查询</Text>
      </Space>

      <Card size="small" style={{ marginBottom: 16 }}>
        <Space direction="vertical" style={{ width: '100%' }}>
          <TextArea
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="输入 PromQL 查询语句..."
            rows={2}
            style={{ fontFamily: 'monospace' }}
          />
          <Space wrap>
            <Select
              value={mode}
              onChange={(v) => setMode(v)}
              style={{ width: 120 }}
              options={[
                { label: '瞬时查询', value: 'instant' },
                { label: '范围查询', value: 'range' },
              ]}
            />
            {mode === 'range' && (
              <Select
                value={timeRange}
                onChange={setTimeRange}
                style={{ width: 140 }}
                options={timeRanges.map((t) => ({ label: t.label, value: t.value }))}
              />
            )}
            <Button type="primary" icon={<PlayCircleOutlined />} onClick={executeQuery} loading={loading}>
              执行
            </Button>
          </Space>
          <Space wrap size={[4, 4]}>
            <Text type="secondary" style={{ fontSize: 12 }}>示例：</Text>
            {examples.map((ex) => (
              <Tag key={ex} color="blue" style={{ cursor: 'pointer' }} onClick={() => setQuery(ex)}>
                {ex}
              </Tag>
            ))}
          </Space>
        </Space>
      </Card>

      {error && <Alert message={error} type="error" showIcon style={{ marginBottom: 16 }} />}

      <Card size="small">
        <Spin spinning={loading}>
          {result.length > 0 ? (
            <Tabs
              items={[
                {
                  key: 'table',
                  label: `表格 (${result.length})`,
                  children: mode === 'instant' ? (
                    <Table columns={instantColumns} dataSource={result} rowKey={(_, i) => String(i)} pagination={{ pageSize: 10 }} size="small" />
                  ) : (
                    <Table
                      columns={[
                        {
                          title: 'Series',
                          key: 'metric',
                          render: (_: any, record: QueryResult) => (
                            <Space wrap>
                              {Object.entries(record.metric).map(([k, v]) => (
                                <Tag key={k}>{k}={v}</Tag>
                              ))}
                            </Space>
                          ),
                        },
                        { title: '数据点', key: 'points', render: (_: any, r: QueryResult) => r.values?.length || 0 },
                      ]}
                      dataSource={result}
                      rowKey={(_, i) => String(i)}
                      pagination={{ pageSize: 10 }}
                      size="small"
                    />
                  ),
                },
                mode === 'range' && {
                  key: 'graph',
                  label: '图表',
                  children: <ReactECharts option={buildRangeChart() || {}} style={{ height: 400 }} />,
                },
              ].filter(Boolean) as any}
            />
          ) : (
            <Empty description={loading ? '查询中...' : '输入 PromQL 并执行查询'} />
          )}
        </Spin>
      </Card>
    </div>
  )
}
