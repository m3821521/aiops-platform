import { useEffect, useState, useCallback } from 'react'
import {
  Row, Col, Card, Statistic, Select, Button, Space, Spin, Alert, Tag, Table, Typography,
} from 'antd'
import {
  ReloadOutlined, DashboardOutlined, FundOutlined,
} from '@ant-design/icons'
import ReactECharts from 'echarts-for-react'
import { metricsApi } from '@/api/metrics'
import { clusterApi } from '@/api/cluster'
import type { Cluster } from '@/types'
import dayjs from 'dayjs'

const { Text } = Typography

const timeRanges = [
  { label: '最近 5 分钟', value: '5m', seconds: 300 },
  { label: '最近 15 分钟', value: '15m', seconds: 900 },
  { label: '最近 1 小时', value: '1h', seconds: 3600 },
  { label: '最近 6 小时', value: '6h', seconds: 21600 },
]

export default function MonitoringOverview() {
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [cluster, setCluster] = useState('')
  const [timeRange, setTimeRange] = useState('15m')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [cpuUsage, setCpuUsage] = useState<number | null>(null)
  const [memUsage, setMemUsage] = useState<number | null>(null)
  const [podCount, setPodCount] = useState<number | null>(null)
  const [nodeCount, setNodeCount] = useState<number | null>(null)
  const [cpuSeries, setCpuSeries] = useState<any[]>([])
  const [memSeries, setMemSeries] = useState<any[]>([])

  const fetchInstant = async (query: string): Promise<number | null> => {
    try {
      const res: any = await metricsApi.query(query)
      const result = res?.result || []
      if (result.length > 0) {
        return parseFloat(result[0].value[1])
      }
      return null
    } catch {
      return null
    }
  }

  const fetchRange = async (query: string, seconds: number): Promise<any[]> => {
    try {
      const end = dayjs()
      const start = end.subtract(seconds, 'second')
      const step = Math.max(15, Math.floor(seconds / 60))
      const res: any = await metricsApi.range({
        query,
        start: start.toISOString(),
        end: end.toISOString(),
        step: `${step}s`,
      })
      return res?.result?.[0]?.values || []
    } catch {
      return []
    }
  }

  const fetchData = useCallback(async () => {
    if (!cluster) return
    setLoading(true)
    setError('')
    try {
      const seconds = timeRanges.find((t) => t.value === timeRange)?.seconds || 900

      const [cpu, mem, pods, nodes, cpuHist, memHist] = await Promise.all([
        fetchInstant('100 - (avg by(instance)(rate(node_cpu_seconds_total{mode="idle"}[5m])) * 100)'),
        fetchInstant('100 * (1 - (node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes))'),
        fetchInstant('count(kube_pod_info)'),
        fetchInstant('count(kube_node_info)'),
        fetchRange('100 - (avg by(instance)(rate(node_cpu_seconds_total{mode="idle"}[5m])) * 100)', seconds),
        fetchRange('100 * (1 - (node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes))', seconds),
      ])

      setCpuUsage(cpu)
      setMemUsage(mem)
      setPodCount(pods)
      setNodeCount(nodes)
      setCpuSeries(cpuHist)
      setMemSeries(memHist)
    } catch (err: any) {
      setError(err?.message || '指标查询失败')
    } finally {
      setLoading(false)
    }
  }, [cluster, timeRange])

  useEffect(() => {
    clusterApi.list().then((res) => {
      setClusters(res || [])
      if (res && res.length > 0 && !cluster) setCluster(res[0].name)
    }).catch(() => {})
  }, [])

  useEffect(() => {
    if (cluster) fetchData()
  }, [cluster, timeRange, fetchData])

  const buildChartOption = (title: string, data: any[], color: string, unit = '%') => ({
    title: { text: title, left: 'center', textStyle: { fontSize: 14 } },
    tooltip: { trigger: 'axis' },
    grid: { left: 50, right: 20, top: 40, bottom: 30 },
    xAxis: {
      type: 'time',
      axisLabel: { fontSize: 11 },
    },
    yAxis: {
      type: 'value',
      axisLabel: { formatter: `{value}${unit}`, fontSize: 11 },
    },
    series: [
      {
        type: 'line',
        smooth: true,
        showSymbol: false,
        data: data.map((v: any) => [v[0] * 1000, parseFloat(v[1]).toFixed(2)]),
        lineStyle: { color, width: 2 },
        areaStyle: { color: `${color}20` },
      },
    ],
  })

  return (
    <div>
      <Space style={{ marginBottom: 16 }}>
        <DashboardOutlined style={{ fontSize: 20, color: '#1677ff' }} />
        <Text strong style={{ fontSize: 18 }}>监控总览</Text>
      </Space>

      {error && <Alert message={error} type="error" showIcon style={{ marginBottom: 16 }} />}

      <Card size="small" style={{ marginBottom: 16 }}>
        <Space wrap>
          <Select
            value={cluster}
            onChange={setCluster}
            style={{ width: 140 }}
            options={clusters.map((c) => ({ label: c.name, value: c.name }))}
          />
          <Select
            value={timeRange}
            onChange={setTimeRange}
            style={{ width: 140 }}
            options={timeRanges.map((t) => ({ label: t.label, value: t.value }))}
          />
          <Button icon={<ReloadOutlined />} onClick={fetchData} loading={loading}>刷新</Button>
        </Space>
      </Card>

      <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
        <Col xs={12} sm={6}>
          <Card>
            <Spin spinning={loading}>
              <Statistic
                title="CPU 使用率"
                value={cpuUsage != null ? cpuUsage.toFixed(1) : '-'}
                suffix="%"
                valueStyle={{ color: cpuUsage != null && cpuUsage > 80 ? '#ff4d4f' : '#52c41a' }}
              />
            </Spin>
          </Card>
        </Col>
        <Col xs={12} sm={6}>
          <Card>
            <Spin spinning={loading}>
              <Statistic
                title="内存使用率"
                value={memUsage != null ? memUsage.toFixed(1) : '-'}
                suffix="%"
                valueStyle={{ color: memUsage != null && memUsage > 85 ? '#ff4d4f' : '#1677ff' }}
              />
            </Spin>
          </Card>
        </Col>
        <Col xs={12} sm={6}>
          <Card>
            <Spin spinning={loading}>
              <Statistic title="Pod 总数" value={podCount ?? '-'} prefix={<FundOutlined />} />
            </Spin>
          </Card>
        </Col>
        <Col xs={12} sm={6}>
          <Card>
            <Spin spinning={loading}>
              <Statistic title="Node 总数" value={nodeCount ?? '-'} />
            </Spin>
          </Card>
        </Col>
      </Row>

      <Row gutter={[16, 16]}>
        <Col xs={24} lg={12}>
          <Card size="small">
            <Spin spinning={loading}>
              {cpuSeries.length > 0 ? (
                <ReactECharts option={buildChartOption('CPU 使用率趋势', cpuSeries, '#52c41a')} style={{ height: 280 }} />
              ) : (
                <div style={{ height: 280, display: 'flex', alignItems: 'center', justifyContent: 'center', color: '#999' }}>
                  暂无数据
                </div>
              )}
            </Spin>
          </Card>
        </Col>
        <Col xs={24} lg={12}>
          <Card size="small">
            <Spin spinning={loading}>
              {memSeries.length > 0 ? (
                <ReactECharts option={buildChartOption('内存使用率趋势', memSeries, '#1677ff')} style={{ height: 280 }} />
              ) : (
                <div style={{ height: 280, display: 'flex', alignItems: 'center', justifyContent: 'center', color: '#999' }}>
                  暂无数据
                </div>
              )}
            </Spin>
          </Card>
        </Col>
      </Row>
    </div>
  )
}
