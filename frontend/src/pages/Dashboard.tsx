import { useEffect, useState, useCallback } from 'react'
import { Row, Col, Typography, Space, Tag, Spin, Table, Button, Select, Progress, Empty } from 'antd'
import {
  CloudOutlined,
  NodeIndexOutlined,
  AppstoreOutlined,
  AlertOutlined,
  WarningOutlined,
  ThunderboltOutlined,
  FireOutlined,
  CheckCircleOutlined,
  ReloadOutlined,
  ClockCircleOutlined,
  RocketOutlined,
  HeartOutlined,
  ArrowUpOutlined,
  ArrowDownOutlined,
} from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import ReactECharts from 'echarts-for-react'
import { clusterApi } from '@/api/cluster'
import { k8sApi } from '@/api/kubernetes'
import { alertsApi } from '@/api/alerts'
import { metricsApi } from '@/api/metrics'
import { anomalyApi } from '@/api/anomaly'
import type { Cluster, Node, Pod, Alert as AlertType, PageResult } from '@/types'
import dayjs from 'dayjs'

const { Title, Text } = Typography

export default function Dashboard() {
  const navigate = useNavigate()
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [nodes, setNodes] = useState<Node[]>([])
  const [pods, setPods] = useState<Pod[]>([])
  const [alerts, setAlerts] = useState<PageResult<AlertType> | null>(null)
  const [activeAnomalyCount, setActiveAnomalyCount] = useState(0)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string>('')
  const [timeRange, setTimeRange] = useState('1h')
  const [cpuSeries, setCpuSeries] = useState<any[]>([])
  const [memSeries, setMemSeries] = useState<any[]>([])
  const [metricsError, setMetricsError] = useState('')

  const fetchData = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const clusterList = await clusterApi.list()
      setClusters(clusterList || [])

      if (clusterList && clusterList.length > 0) {
        const firstCluster = clusterList[0].name
        const [nodeList, podList, alertData, anomalyData] = await Promise.all([
          k8sApi.nodes(firstCluster).catch(() => [] as Node[]),
          k8sApi.pods({ cluster: firstCluster }).catch(() => [] as Pod[]),
          alertsApi.list({ page: 1, page_size: 100, status: 'firing' }).catch(() => ({ items: [], total: 0, page: 1, page_size: 100 } as PageResult<AlertType>)),
          anomalyApi.activeCount().catch(() => ({ count: 0 })),
        ])
        setNodes(nodeList || [])
        setPods(podList || [])
        setAlerts(alertData)
        setActiveAnomalyCount(anomalyData?.count || 0)
      } else {
        setNodes([])
        setPods([])
        setAlerts({ items: [], total: 0, page: 1, page_size: 100 })
        setActiveAnomalyCount(0)
      }
    } catch (err: any) {
      setError(err?.message || '数据加载失败')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchData()
  }, [fetchData])

  const fetchMetrics = useCallback(async () => {
    if (clusters.length === 0) return
    setMetricsError('')
    const seconds = timeRange === '1h' ? 3600 : timeRange === '6h' ? 21600 : 86400
    const end = Math.floor(Date.now() / 1000)
    const start = end - seconds
    try {
      const [cpuData, memData] = await Promise.all([
        metricsApi.range({ query: '100 - (avg by(instance) (rate(node_cpu_seconds_total{mode="idle"}[5m])) * 100)', start: String(start), end: String(end), step: '60' }).catch(() => null),
        metricsApi.range({ query: '(1 - node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes) * 100', start: String(start), end: String(end), step: '60' }).catch(() => null),
      ])
      if (cpuData?.data?.result?.[0]?.values) setCpuSeries(cpuData.data.result[0].values)
      if (memData?.data?.result?.[0]?.values) setMemSeries(memData.data.result[0].values)
    } catch {
      setMetricsError('监控数据不可用')
    }
  }, [clusters, timeRange])

  useEffect(() => {
    fetchMetrics()
  }, [fetchMetrics])

  // 计算派生指标
  const runningPods = pods.filter((p) => p.status === 'Running').length
  const failedPods = pods.filter((p) => p.status === 'Failed' || p.status === 'CrashLoopBackOff' || p.status === 'Error').length
  const firingCount = alerts?.total || 0
  const criticalAlerts = alerts?.items?.filter((a) => a.severity === 'critical') || []
  const warningAlerts = alerts?.items?.filter((a) => a.severity === 'warning') || []
  const abnormalPods = pods.filter((p) => p.status !== 'Running' && p.status !== 'Succeeded' && p.status !== 'Completed')

  // System Health: 基于 Pod running 比例 + 无 critical alert
  const totalPods = pods.length || 1
  const podHealthRatio = (runningPods / totalPods) * 100
  const alertPenalty = criticalAlerts.length > 0 ? 10 : warningAlerts.length > 0 ? 5 : 0
  const systemHealth = Math.max(0, Math.min(100, podHealthRatio - alertPenalty))
  const healthStatus = systemHealth >= 95 ? 'healthy' : systemHealth >= 80 ? 'warning' : 'critical'
  const healthColor = healthStatus === 'healthy' ? '#16a34a' : healthStatus === 'warning' ? '#d97706' : '#dc2626'

  const podColumns = [
    { title: 'Pod', dataIndex: 'name', key: 'name', render: (t: string) => <Text strong style={{ fontSize: 13 }}>{t}</Text> },
    { title: 'Namespace', dataIndex: 'namespace', key: 'namespace', render: (v: string) => v || '-' },
    { title: 'Status', dataIndex: 'status', key: 'status', render: (s: string) => (
      <span className={`status-badge ${s === 'Running' ? 'success' : s === 'Failed' ? 'danger' : 'warning'}`}>{s || 'Unknown'}</span>
    )},
    { title: 'Restarts', dataIndex: 'restartCount', key: 'restartCount', render: (v: number) => v || 0 },
  ]

  const alertColumns = [
    { title: 'Alert', dataIndex: 'alertname', key: 'alertname', render: (t: string) => <Text strong style={{ fontSize: 13 }}>{t}</Text> },
    { title: 'Severity', dataIndex: 'severity', key: 'severity', render: (s: string) => (
      <span className={`status-badge ${s === 'critical' ? 'critical' : s === 'warning' ? 'warning' : 'info'}`}>{s}</span>
    )},
    { title: 'Service', dataIndex: 'service', key: 'service', render: (v: string) => v || '-' },
  ]

  const chartOption = {
    tooltip: { trigger: 'axis', backgroundColor: '#fff', borderColor: '#e5e7eb', textStyle: { color: '#111827', fontSize: 12 } },
    legend: { data: ['CPU', 'Memory'], bottom: 0, textStyle: { fontSize: 12, color: '#6b7280' }, itemWidth: 12, itemHeight: 8 },
    grid: { left: 45, right: 16, top: 16, bottom: 36 },
    xAxis: { type: 'time', axisLabel: { fontSize: 11, color: '#9ca3af' }, axisLine: { lineStyle: { color: '#e5e7eb' } } },
    yAxis: { type: 'value', axisLabel: { formatter: '{value}%', fontSize: 11, color: '#9ca3af' }, max: 100, splitLine: { lineStyle: { color: '#f3f4f6' } } },
    series: [
      {
        name: 'CPU',
        type: 'line',
        smooth: true,
        showSymbol: false,
        data: cpuSeries.map((v: any) => [v[0] * 1000, parseFloat(v[1]).toFixed(1)]),
        lineStyle: { color: '#2563eb', width: 2 },
        areaStyle: { color: { type: 'linear', x: 0, y: 0, x2: 0, y2: 1, colorStops: [{ offset: 0, color: 'rgba(37,99,235,0.15)' }, { offset: 1, color: 'rgba(37,99,235,0.01)' }] } },
      },
      {
        name: 'Memory',
        type: 'line',
        smooth: true,
        showSymbol: false,
        data: memSeries.map((v: any) => [v[0] * 1000, parseFloat(v[1]).toFixed(1)]),
        lineStyle: { color: '#7c3aed', width: 2 },
        areaStyle: { color: { type: 'linear', x: 0, y: 0, x2: 0, y2: 1, colorStops: [{ offset: 0, color: 'rgba(124,58,237,0.12)' }, { offset: 1, color: 'rgba(124,58,237,0.01)' }] } },
      },
    ],
  }

  return (
    <div className="aiops-page">
      {/* Page Header */}
      <div className="aiops-page-header">
        <div>
          <div className="aiops-page-title">Operations Overview</div>
          <div className="aiops-page-subtitle">
            {dayjs().format('YYYY-MM-DD HH:mm')} · {clusters.length} cluster{clusters.length !== 1 ? 's' : ''} · {nodes.length} node{nodes.length !== 1 ? 's' : ''}
          </div>
        </div>
        <Space>
          <Select
            value={timeRange}
            onChange={setTimeRange}
            size="middle"
            style={{ width: 110 }}
            options={[
              { label: 'Last 1h', value: '1h' },
              { label: 'Last 6h', value: '6h' },
              { label: 'Last 24h', value: '24h' },
            ]}
          />
          <Button icon={<ReloadOutlined />} onClick={fetchData} loading={loading}>Refresh</Button>
        </Space>
      </div>

      {/* KPI Row — Compact */}
      <Row gutter={[12, 12]} style={{ marginBottom: 16 }}>
        <Col xs={12} sm={12} md={6}>
          <div className={`kpi-card accent-${healthStatus}`} onClick={() => navigate('/kubernetes/nodes')} style={{ cursor: 'pointer' }}>
            <HeartOutlined className="kpi-icon" style={{ color: healthColor }} />
            <div className="kpi-label"><HeartOutlined style={{ fontSize: 11 }} /> System Health</div>
            <div>
              <div className="kpi-value" style={{ color: healthColor, fontSize: 28 }}>{systemHealth.toFixed(1)}%</div>
              <div className="kpi-sub" style={{ color: healthColor }}>
                {healthStatus === 'healthy' ? 'All systems operational' : healthStatus === 'warning' ? 'Minor issues detected' : 'Critical issues require attention'}
              </div>
            </div>
          </div>
        </Col>
        <Col xs={12} sm={12} md={6}>
          <div className={`kpi-card ${firingCount > 0 ? 'accent-warning' : ''}`} onClick={() => navigate('/aiops/incidents')} style={{ cursor: 'pointer' }}>
            <AlertOutlined className="kpi-icon" style={{ color: '#d97706' }} />
            <div className="kpi-label"><AlertOutlined style={{ fontSize: 11 }} /> Active Alerts</div>
            <div>
              <div className="kpi-value" style={{ fontSize: 28 }}>{firingCount}</div>
              <div className="kpi-sub">
                {criticalAlerts.length > 0 && <span style={{ color: '#dc2626', fontWeight: 600 }}>{criticalAlerts.length} Critical</span>}
                {criticalAlerts.length > 0 && warningAlerts.length > 0 && ' · '}
                {warningAlerts.length > 0 && <span style={{ color: '#d97706' }}>{warningAlerts.length} Warning</span>}
                {firingCount === 0 && <span style={{ color: '#16a34a' }}>No active alerts</span>}
              </div>
            </div>
          </div>
        </Col>
        <Col xs={12} sm={12} md={6}>
          <div className="kpi-card" onClick={() => navigate('/kubernetes/pods')} style={{ cursor: 'pointer' }}>
            <AppstoreOutlined className="kpi-icon" style={{ color: '#7c3aed' }} />
            <div className="kpi-label"><AppstoreOutlined style={{ fontSize: 11 }} /> Infrastructure</div>
            <div>
              <div className="kpi-value" style={{ fontSize: 28 }}>{pods.length} <span style={{ fontSize: 16, color: '#9ca3af', fontWeight: 400 }}>Pods</span></div>
              <div className="kpi-sub">
                <span style={{ color: '#16a34a' }}>{runningPods} Running</span>
                {failedPods > 0 && <span style={{ color: '#dc2626' }}> · {failedPods} Failed</span>}
                {abnormalPods.length > 0 && <span style={{ color: '#d97706' }}> · {abnormalPods.length} Abnormal</span>}
              </div>
            </div>
          </div>
        </Col>
        <Col xs={12} sm={12} md={6}>
          <div className="kpi-card" onClick={() => navigate('/aiops/anomaly')} style={{ cursor: 'pointer' }}>
            <FireOutlined className="kpi-icon" style={{ color: '#ea580c' }} />
            <div className="kpi-label"><FireOutlined style={{ fontSize: 11 }} /> Anomalies</div>
            <div>
              <div className="kpi-value" style={{ fontSize: 28, color: activeAnomalyCount > 0 ? '#ea580c' : undefined }}>{activeAnomalyCount}</div>
              <div className="kpi-sub">{activeAnomalyCount > 0 ? 'Active anomalies detected' : 'No anomalies detected'}</div>
            </div>
          </div>
        </Col>
      </Row>

      {/* Charts Row */}
      <Row gutter={[12, 12]} style={{ marginBottom: 16 }}>
        <Col xs={24} lg={16}>
          <div className="aiops-card">
            <div className="aiops-card-header">
              <div className="aiops-card-title">Resource Trend</div>
              <Text type="secondary" style={{ fontSize: 12 }}>Cluster-wide CPU & Memory</Text>
            </div>
            <div className="aiops-card-body" style={{ paddingTop: 8 }}>
              {metricsError ? (
                <div style={{ textAlign: 'center', padding: '40px 0' }}>
                  <Text type="secondary">{metricsError}</Text>
                </div>
              ) : cpuSeries.length > 0 || memSeries.length > 0 ? (
                <ReactECharts option={chartOption} style={{ height: 220 }} notMerge />
              ) : (
                <Empty description="No monitoring data" image={Empty.PRESENTED_IMAGE_SIMPLE} style={{ padding: '32px 0' }} />
              )}
            </div>
          </div>
        </Col>
        <Col xs={24} lg={8}>
          <div className="aiops-card" style={{ height: '100%' }}>
            <div className="aiops-card-header">
              <div className="aiops-card-title">Alert Summary</div>
              <Button type="link" size="small" onClick={() => navigate('/alerts/realtime')}>View all</Button>
            </div>
            <div className="aiops-card-body">
              <Row gutter={[8, 16]}>
                <Col span={12}>
                  <div style={{ textAlign: 'center' }}>
                    <div style={{ fontSize: 28, fontWeight: 700, color: '#dc2626' }}>{criticalAlerts.length}</div>
                    <div style={{ fontSize: 11, color: '#9ca3af', textTransform: 'uppercase', letterSpacing: 0.5 }}>Critical</div>
                  </div>
                </Col>
                <Col span={12}>
                  <div style={{ textAlign: 'center' }}>
                    <div style={{ fontSize: 28, fontWeight: 700, color: '#d97706' }}>{warningAlerts.length}</div>
                    <div style={{ fontSize: 11, color: '#9ca3af', textTransform: 'uppercase', letterSpacing: 0.5 }}>Warning</div>
                  </div>
                </Col>
              </Row>
              <div style={{ marginTop: 16, paddingTop: 16, borderTop: '1px solid #f3f4f6' }}>
                <div style={{ fontSize: 12, color: '#6b7280', marginBottom: 8 }}>Cluster Health</div>
                <Progress percent={Math.round(systemHealth)} strokeColor={healthColor} showInfo={false} size="small" />
                <div style={{ fontSize: 12, color: healthColor, marginTop: 4, fontWeight: 500 }}>
                  {systemHealth.toFixed(1)}% — {healthStatus === 'healthy' ? 'Healthy' : healthStatus === 'warning' ? 'Degraded' : 'Critical'}
                </div>
              </div>
            </div>
          </div>
        </Col>
      </Row>

      {/* Active Alerts + Abnormal Pods */}
      <Row gutter={[12, 12]}>
        <Col xs={24} lg={14}>
          <div className="aiops-card">
            <div className="aiops-card-header">
              <div className="aiops-card-title">Active Alerts</div>
              <Button type="link" size="small" onClick={() => navigate('/alerts/realtime')}>View all →</Button>
            </div>
            <div className="aiops-card-body" style={{ padding: 0 }}>
              <Spin spinning={loading}>
                {alerts && alerts.items && alerts.items.length > 0 ? (
                  <Table
                    columns={alertColumns}
                    dataSource={alerts.items.slice(0, 6)}
                    rowKey="id"
                    pagination={false}
                    size="small"
                    showHeader={true}
                  />
                ) : (
                  <Empty description="No active alerts" image={Empty.PRESENTED_IMAGE_SIMPLE} style={{ padding: '32px 0' }} />
                )}
              </Spin>
            </div>
          </div>
        </Col>
        <Col xs={24} lg={10}>
          <div className="aiops-card">
            <div className="aiops-card-header">
              <div className="aiops-card-title">Infrastructure Status</div>
              <Button type="link" size="small" onClick={() => navigate('/kubernetes/pods')}>Pods →</Button>
            </div>
            <div className="aiops-card-body" style={{ padding: 0 }}>
              <Spin spinning={loading}>
                {abnormalPods.length > 0 ? (
                  <Table
                    columns={podColumns}
                    dataSource={abnormalPods.slice(0, 6)}
                    rowKey="name"
                    pagination={false}
                    size="small"
                  />
                ) : (
                  <div style={{ textAlign: 'center', padding: '32px 0' }}>
                    <CheckCircleOutlined style={{ fontSize: 28, color: '#16a34a', marginBottom: 8 }} />
                    <div style={{ fontSize: 13, color: '#6b7280' }}>All pods healthy</div>
                    <div style={{ fontSize: 12, color: '#9ca3af', marginTop: 4 }}>{runningPods}/{pods.length} pods running</div>
                  </div>
                )}
              </Spin>
            </div>
          </div>
        </Col>
      </Row>
    </div>
  )
}
