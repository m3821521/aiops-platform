import { useEffect, useState, useCallback } from 'react'
import {
  Row, Col, Card, Statistic, Select, Button, Space, Spin, Alert, Tag, Table, Typography,
  Tooltip, Empty, message,
} from 'antd'
import {
  ReloadOutlined, DashboardOutlined, FundOutlined, WarningOutlined, CheckCircleOutlined,
  CloseCircleOutlined, ApiOutlined,
} from '@ant-design/icons'
import ReactECharts from 'echarts-for-react'
import { metricsApi } from '@/api/metrics'
import { clusterApi, type NodeMetric, type PodMetric } from '@/api/cluster'
import type { Cluster } from '@/types'
import dayjs from 'dayjs'

const { Text } = Typography

const timeRanges = [
  { label: '最近 5 分钟', value: '5m', seconds: 300 },
  { label: '最近 15 分钟', value: '15m', seconds: 900 },
  { label: '最近 1 小时', value: '1h', seconds: 3600 },
  { label: '最近 6 小时', value: '6h', seconds: 21600 },
]

interface DataSourceStatus {
  prometheus: 'checking' | 'available' | 'unavailable'
  message: string
}

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
  const [dataSource, setDataSource] = useState<DataSourceStatus>({
    prometheus: 'checking',
    message: '正在检查 Prometheus 数据源...',
  })
  const [hasMetrics, setHasMetrics] = useState(false)

  // 检查 Prometheus 数据源状态
  const checkDataSource = useCallback(async () => {
    try {
      // 尝试查询一个简单的指标来判断 Prometheus 是否可用
      const res: any = await metricsApi.query('up')
      const result = res?.result || []
      if (result.length > 0) {
        setDataSource({
          prometheus: 'available',
          message: `Prometheus 数据源正常，共 ${result.length} 个采集目标`,
        })
      } else {
        setDataSource({
          prometheus: 'available',
          message: 'Prometheus 已连接，但暂无采集目标数据',
        })
      }
    } catch (err: any) {
      setDataSource({
        prometheus: 'unavailable',
        message: `Prometheus 数据源不可用: ${err?.message || '连接失败'}`,
      })
    }
  }, [])

  const fetchInstant = async (query: string): Promise<{ value: number | null; error?: string }> => {
    try {
      const res: any = await metricsApi.query(query)
      const result = res?.result || []
      if (result.length > 0) {
        return { value: parseFloat(result[0].value[1]) }
      }
      return { value: null }
    } catch (err: any) {
      return { value: null, error: err?.message || '查询失败' }
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

      // 先尝试从 Prometheus 获取指标
      const [cpu, mem, pods, nodes, cpuHist, memHist] = await Promise.all([
        fetchInstant('100 - (avg by(instance)(rate(node_cpu_seconds_total{mode="idle"}[5m])) * 100)'),
        fetchInstant('100 * (1 - (node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes))'),
        fetchInstant('count(kube_pod_info)'),
        fetchInstant('count(kube_node_info)'),
        fetchRange('100 - (avg by(instance)(rate(node_cpu_seconds_total{mode="idle"}[5m])) * 100)', seconds),
        fetchRange('100 * (1 - (node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes))', seconds),
      ])

      let finalCpu = cpu.value
      let finalMem = mem.value
      let finalPods = pods.value
      let finalNodes = nodes.value
      let usingK8sMetrics = false

      // 如果 Prometheus 没有节点指标，使用 Kubernetes Metrics API（metrics-server）
      if (finalCpu === null || finalMem === null) {
        try {
          const nodeMetrics: NodeMetric[] = await clusterApi.nodeMetrics()
          if (nodeMetrics && nodeMetrics.length > 0) {
            // 计算所有节点的平均 CPU 和内存使用率
            const avgCpu = nodeMetrics.reduce((sum, n) => sum + n.cpu_percent, 0) / nodeMetrics.length
            const avgMem = nodeMetrics.reduce((sum, n) => sum + n.memory_percent, 0) / nodeMetrics.length
            finalCpu = avgCpu
            finalMem = avgMem
            finalNodes = nodeMetrics.length
            usingK8sMetrics = true
          }
        } catch {
          // Kubernetes Metrics API 不可用，保持 null
        }
      }

      // 如果 Prometheus 没有 Pod 数量，使用 Kubernetes Metrics API
      if (finalPods === null) {
        try {
          const podMetrics: PodMetric[] = await clusterApi.podMetrics()
          if (podMetrics) {
            finalPods = podMetrics.length
            usingK8sMetrics = true
          }
        } catch {
          // Kubernetes Metrics API 不可用，保持 null
        }
      }

      setCpuUsage(finalCpu)
      setMemUsage(finalMem)
      setPodCount(finalPods)
      setNodeCount(finalNodes)
      setCpuSeries(cpuHist)
      setMemSeries(memHist)

      // 判断是否有监控数据
      const hasAnyData = finalCpu !== null || finalMem !== null || finalPods !== null || finalNodes !== null || cpuHist.length > 0 || memHist.length > 0
      setHasMetrics(hasAnyData)

      // 如果使用了 Kubernetes Metrics API，更新数据源状态消息
      if (usingK8sMetrics && dataSource.prometheus === 'available') {
        setDataSource({
          prometheus: 'available',
          message: 'Prometheus 已连接，节点指标来自 Kubernetes metrics-server（未配置 node-exporter）',
        })
      }
    } catch (err: any) {
      setError(err?.message || '指标查询失败')
    } finally {
      setLoading(false)
    }
  }, [cluster, timeRange, dataSource.prometheus])

  useEffect(() => {
    checkDataSource()
    clusterApi.list().then((res) => {
      setClusters(res || [])
      if (res && res.length > 0 && !cluster) setCluster(res[0].name)
    }).catch(() => {
      message.warning('集群列表加载失败，监控数据可能不完整')
    })
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

  const renderDataSourceStatus = () => {
    if (dataSource.prometheus === 'checking') {
      return (
        <Alert
          icon={<Spin size="small" />}
          message="正在检查 Prometheus 数据源..."
          type="info"
          showIcon
          style={{ marginBottom: 16 }}
        />
      )
    }
    if (dataSource.prometheus === 'unavailable') {
      return (
        <Alert
          message="Prometheus 数据源不可用"
          description={dataSource.message}
          type="error"
          showIcon
          action={<Button size="small" icon={<ReloadOutlined />} onClick={checkDataSource}>重试</Button>}
          style={{ marginBottom: 16 }}
        />
      )
    }
    if (!hasMetrics && !loading) {
      return (
        <Alert
          message="监控指标暂无数据"
          description={
            <div>
              <p style={{ margin: '0 0 8px 0' }}>
                Prometheus 已连接，但未采集到节点 CPU/内存、Pod 数量等指标。这通常是因为未配置以下采集器：
              </p>
              <Space wrap>
                <Tag color="blue">node-exporter（节点指标）</Tag>
                <Tag color="cyan">kube-state-metrics（Kubernetes 资源指标）</Tag>
                <Tag color="purple">cAdvisor（容器指标）</Tag>
              </Space>
              <p style={{ margin: '8px 0 0 0', fontSize: 12, color: '#888' }}>
                请在 Prometheus 配置中添加对应的 scrape job，或启用 Kubernetes metrics-server。
              </p>
            </div>
          }
          type="warning"
          showIcon
          style={{ marginBottom: 16 }}
        />
      )
    }
    return (
      <Alert
        message={dataSource.message}
        type="success"
        showIcon
        icon={<CheckCircleOutlined />}
        style={{ marginBottom: 16 }}
      />
    )
  }

  const renderKpiCard = (title: string, value: number | null, suffix: string, color: string, icon?: React.ReactNode) => {
    const isEmpty = value === null
    return (
      <Card>
        <Spin spinning={loading}>
          <Statistic
            title={title}
            value={isEmpty ? '-' : value.toFixed(1)}
            suffix={isEmpty ? '' : suffix}
            prefix={icon}
            valueStyle={{ color: isEmpty ? '#ccc' : color }}
          />
          {isEmpty && !loading && (
            <Tooltip title="未采集到该指标，请检查 node-exporter / kube-state-metrics 配置">
              <Text type="secondary" style={{ fontSize: 11, cursor: 'help' }}>
                <WarningOutlined style={{ marginRight: 4 }} />暂无数据
              </Text>
            </Tooltip>
          )}
        </Spin>
      </Card>
    )
  }

  return (
    <div>
      <Space style={{ marginBottom: 16 }}>
        <DashboardOutlined style={{ fontSize: 20, color: '#1677ff' }} />
        <Text strong style={{ fontSize: 18 }}>监控总览</Text>
        <Tag icon={<ApiOutlined />} color={dataSource.prometheus === 'available' ? 'success' : 'default'}>
          Prometheus: {dataSource.prometheus}
        </Tag>
      </Space>

      {error && <Alert message={error} type="error" showIcon style={{ marginBottom: 16 }} />}

      {renderDataSourceStatus()}

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
          <Button icon={<ReloadOutlined />} onClick={() => { checkDataSource(); fetchData() }} loading={loading}>刷新</Button>
        </Space>
      </Card>

      <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
        <Col xs={12} sm={6}>
          {renderKpiCard('CPU 使用率', cpuUsage, '%', cpuUsage !== null && cpuUsage > 80 ? '#ff4d4f' : '#52c41a')}
        </Col>
        <Col xs={12} sm={6}>
          {renderKpiCard('内存使用率', memUsage, '%', memUsage !== null && memUsage > 85 ? '#ff4d4f' : '#1677ff')}
        </Col>
        <Col xs={12} sm={6}>
          {renderKpiCard('Pod 总数', podCount, '', '#722ed1', <FundOutlined />)}
        </Col>
        <Col xs={12} sm={6}>
          {renderKpiCard('Node 总数', nodeCount, '', '#13c2c2')}
        </Col>
      </Row>

      <Row gutter={[16, 16]}>
        <Col xs={24} lg={12}>
          <Card size="small">
            <Spin spinning={loading}>
              {cpuSeries.length > 0 ? (
                <ReactECharts option={buildChartOption('CPU 使用率趋势', cpuSeries, '#52c41a')} style={{ height: 280 }} />
              ) : (
                <div style={{ height: 280, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                  <Empty description="暂无 CPU 监控数据" image={Empty.PRESENTED_IMAGE_SIMPLE} />
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
                <div style={{ height: 280, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                  <Empty description="暂无内存监控数据" image={Empty.PRESENTED_IMAGE_SIMPLE} />
                </div>
              )}
            </Spin>
          </Card>
        </Col>
      </Row>
    </div>
  )
}
