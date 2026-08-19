import { useEffect, useState, useCallback } from 'react'
import { Row, Col, Card, Statistic, Typography, Space, Tag, Spin, Alert, Table, Badge, Button, Select } from 'antd'
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
  ExclamationCircleOutlined,
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
    const end = dayjs()
    const start = end.subtract(seconds, 'second')
    const step = Math.max(30, Math.floor(seconds / 120))
    try {
      const [cpuRes, memRes] = await Promise.all([
        metricsApi.range({
          query: '100 - (avg by(instance)(rate(node_cpu_seconds_total{mode="idle"}[5m])) * 100)',
          start: start.toISOString(),
          end: end.toISOString(),
          step: `${step}s`,
        }).catch(() => ({ result: [] })),
        metricsApi.range({
          query: '100 * (1 - (node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes))',
          start: start.toISOString(),
          end: end.toISOString(),
          step: `${step}s`,
        }).catch(() => ({ result: [] })),
      ])
      setCpuSeries((cpuRes as any)?.result?.[0]?.values || [])
      setMemSeries((memRes as any)?.result?.[0]?.values || [])
    } catch {
      setMetricsError('Prometheus 监控数据源不可用')
    }
  }, [clusters.length, timeRange])

  useEffect(() => {
    if (clusters.length > 0) fetchMetrics()
  }, [clusters.length, timeRange, fetchMetrics])

  const runningPods = pods.filter((p) => p.status === 'Running').length
  const abnormalPods = pods.filter((p) => p.status !== 'Running' && p.status !== 'Succeeded')
  const criticalAlerts = alerts?.items?.filter((a) => a.severity === 'critical') || []
  const warningAlerts = alerts?.items?.filter((a) => a.severity === 'warning') || []
  const firingCount = alerts?.total || 0

  // 系统健康状态判定
  const healthStatus = criticalAlerts.length > 0
    ? { label: '异常', color: 'error', icon: <ExclamationCircleOutlined /> }
    : abnormalPods.length > 0
    ? { label: '警告', color: 'warning', icon: <WarningOutlined /> }
    : { label: '正常', color: 'success', icon: <CheckCircleOutlined /> }

  const podColumns = [
    { title: 'Pod', dataIndex: 'name', key: 'name', render: (t: string) => <Text strong>{t}</Text> },
    { title: 'Namespace', dataIndex: 'namespace', key: 'namespace', render: (t: string) => <Tag>{t}</Tag> },
    { title: '状态', dataIndex: 'status', key: 'status', render: (s: string) => <Tag color="error">{s}</Tag> },
    { title: 'Node', dataIndex: 'node', key: 'node', render: (v: string) => v || '-' },
    {
      title: '操作',
      key: 'action',
      render: (_: any, record: Pod) => (
        <Button type="link" size="small" onClick={() => navigate(`/kubernetes/pods?name=${record.name}`)}>
          查看
        </Button>
      ),
    },
  ]

  return (
    <div>
      <Space style={{ marginBottom: 24 }} align="center">
        <Title level={4} style={{ margin: 0 }}>运维总览</Title>
        <Badge status={healthStatus.color as any} text={<Tag color={healthStatus.color} icon={healthStatus.icon}>{healthStatus.label}</Tag>} />
        <Button icon={<ReloadOutlined />} size="small" onClick={fetchData} loading={loading}>刷新</Button>
      </Space>

      {error && (
        <Alert message="数据加载失败" description={error} type="error" showIcon style={{ marginBottom: 16 }} />
      )}

      {/* 资源统计 */}
      <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
        <Col xs={12} sm={8} md={6} lg={4}>
          <Card className="stat-card">
            <Spin spinning={loading}>
              <Statistic title="集群" value={clusters.length} prefix={<CloudOutlined style={{ color: '#1677ff' }} />} />
            </Spin>
          </Card>
        </Col>
        <Col xs={12} sm={8} md={6} lg={4}>
          <Card className="stat-card">
            <Spin spinning={loading}>
              <Statistic title="Node" value={nodes.length} prefix={<NodeIndexOutlined style={{ color: '#52c41a' }} />} />
            </Spin>
          </Card>
        </Col>
        <Col xs={12} sm={8} md={6} lg={4}>
          <Card className="stat-card">
            <Spin spinning={loading}>
              <Statistic
                title="Pod"
                value={pods.length}
                prefix={<AppstoreOutlined style={{ color: '#722ed1' }} />}
                suffix={pods.length > 0 ? <span style={{ fontSize: 14, color: '#999' }}>({runningPods} Running)</span> : null}
              />
            </Spin>
          </Card>
        </Col>
        <Col xs={12} sm={8} md={6} lg={4}>
          <Card className="stat-card">
            <Spin spinning={loading}>
              <Statistic
                title="活动告警"
                value={firingCount}
                prefix={<AlertOutlined style={{ color: '#faad14' }} />}
                valueStyle={{ color: firingCount > 0 ? '#faad14' : undefined }}
              />
            </Spin>
          </Card>
        </Col>
        <Col xs={12} sm={8} md={6} lg={4}>
          <Card className="stat-card">
            <Spin spinning={loading}>
              <Statistic
                title="严重告警"
                value={criticalAlerts.length}
                prefix={<WarningOutlined style={{ color: '#ff4d4f' }} />}
                valueStyle={{ color: criticalAlerts.length > 0 ? '#ff4d4f' : undefined }}
              />
            </Spin>
          </Card>
        </Col>
        <Col xs={12} sm={8} md={6} lg={4}>
          <Card className="stat-card">
            <Spin spinning={loading}>
              <Statistic
                title="异常 Pod"
                value={abnormalPods.length}
                prefix={<ThunderboltOutlined style={{ color: '#eb2f96' }} />}
                valueStyle={{ color: abnormalPods.length > 0 ? '#eb2f96' : undefined }}
              />
            </Spin>
          </Card>
        </Col>
        <Col xs={12} sm={8} md={6} lg={4}>
          <Card className="stat-card" onClick={() => navigate('/aiops/anomaly')} style={{ cursor: 'pointer' }}>
            <Spin spinning={loading}>
              <Statistic
                title="活跃异常"
                value={activeAnomalyCount}
                prefix={<FireOutlined style={{ color: '#fa541c' }} />}
                valueStyle={{ color: activeAnomalyCount > 0 ? '#fa541c' : undefined }}
              />
            </Spin>
          </Card>
        </Col>
      </Row>

      {/* 集群状态 */}
      {clusters.length > 0 && (
        <Card title="集群状态" style={{ marginBottom: 24 }} size="small">
          <Row gutter={[16, 16]}>
            {clusters.map((c) => (
              <Col xs={24} sm={12} md={8} key={c.name}>
                <Card size="small" type="inner">
                  <Space direction="vertical" style={{ width: '100%' }}>
                    <Space>
                      <CloudOutlined style={{ color: '#1677ff' }} />
                      <Text strong>{c.name}</Text>
                      <Tag color={c.enabled ? 'success' : 'default'}>{c.enabled ? '已启用' : '已禁用'}</Tag>
                    </Space>
                    <Text type="secondary" style={{ fontSize: 13 }}>{c.description || '-'}</Text>
                    <div style={{ fontSize: 12, color: '#999' }}>
                      认证: {c.auth_type} | Nodes: {nodes.length} | Pods: {pods.length} | 告警: {firingCount}
                    </div>
                  </Space>
                </Card>
              </Col>
            ))}
          </Row>
        </Card>
      )}

      {/* 异常 Pod 列表 */}
      <Row gutter={[16, 16]}>
        <Col xs={24} lg={14}>
          <Card
            title={<Space><ThunderboltOutlined style={{ color: '#eb2f96' }} />异常 Pod</Space>}
            size="small"
            extra={<Button type="link" size="small" onClick={() => navigate('/kubernetes/pods')}>查看全部</Button>}
          >
            <Spin spinning={loading}>
              {abnormalPods.length > 0 ? (
                <Table
                  columns={podColumns}
                  dataSource={abnormalPods}
                  rowKey="name"
                  pagination={false}
                  size="small"
                />
              ) : (
                <div style={{ color: '#999', textAlign: 'center', padding: '24px 0' }}>
                  <CheckCircleOutlined style={{ fontSize: 24, color: '#52c41a', marginBottom: 8 }} />
                  <div>无异常 Pod</div>
                </div>
              )}
            </Spin>
          </Card>
        </Col>
        <Col xs={24} lg={10}>
          <Card
            title={<Space><AlertOutlined style={{ color: '#faad14' }} />活动告警</Space>}
            size="small"
            extra={<Button type="link" size="small" onClick={() => navigate('/alerts/realtime')}>查看全部</Button>}
          >
            <Spin spinning={loading}>
              {alerts && alerts.items && alerts.items.length > 0 ? (
                <Table
                  columns={[
                    { title: '告警', dataIndex: 'alertname', key: 'alertname', render: (t: string) => <Text strong>{t}</Text> },
                    { title: '级别', dataIndex: 'severity', key: 'severity', render: (s: string) => <Tag color={s === 'critical' ? 'error' : s === 'warning' ? 'warning' : 'info'}>{s}</Tag> },
                    { title: '服务', dataIndex: 'service', key: 'service', render: (v: string) => v || '-' },
                  ]}
                  dataSource={alerts.items.slice(0, 5)}
                  rowKey="id"
                  pagination={false}
                  size="small"
                />
              ) : (
                <div style={{ color: '#999', textAlign: 'center', padding: '24px 0' }}>
                  <CheckCircleOutlined style={{ fontSize: 24, color: '#52c41a', marginBottom: 8 }} />
                  <div>暂无活动告警</div>
                </div>
              )}
            </Spin>
          </Card>
        </Col>
      </Row>

      {/* 资源趋势图 */}
      <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
        <Col xs={24} lg={16}>
          <Card
            title="资源趋势"
            size="small"
            extra={
              <Select
                value={timeRange}
                onChange={setTimeRange}
                size="small"
                style={{ width: 100 }}
                options={[
                  { label: '1 小时', value: '1h' },
                  { label: '6 小时', value: '6h' },
                  { label: '24 小时', value: '24h' },
                ]}
              />
            }
          >
            {metricsError ? (
              <Alert message={metricsError} type="warning" showIcon style={{ marginTop: 8 }} />
            ) : cpuSeries.length > 0 || memSeries.length > 0 ? (
              <ReactECharts
                option={{
                  tooltip: { trigger: 'axis' },
                  legend: { data: ['CPU 使用率', '内存使用率'], bottom: 0 },
                  grid: { left: 50, right: 20, top: 20, bottom: 40 },
                  xAxis: { type: 'time', axisLabel: { fontSize: 11 } },
                  yAxis: { type: 'value', axisLabel: { formatter: '{value}%', fontSize: 11 }, max: 100 },
                  series: [
                    {
                      name: 'CPU 使用率',
                      type: 'line',
                      smooth: true,
                      showSymbol: false,
                      data: cpuSeries.map((v: any) => [v[0] * 1000, parseFloat(v[1]).toFixed(2)]),
                      lineStyle: { color: '#52c41a', width: 2 },
                      areaStyle: { color: '#52c41a20' },
                    },
                    {
                      name: '内存使用率',
                      type: 'line',
                      smooth: true,
                      showSymbol: false,
                      data: memSeries.map((v: any) => [v[0] * 1000, parseFloat(v[1]).toFixed(2)]),
                      lineStyle: { color: '#1677ff', width: 2 },
                      areaStyle: { color: '#1677ff20' },
                    },
                  ],
                }}
                style={{ height: 240 }}
              />
            ) : (
              <div style={{ color: '#999', textAlign: 'center', paddingTop: 80 }}>暂无监控数据</div>
            )}
          </Card>
        </Col>
        <Col xs={24} lg={8}>
          <Card title="告警分布" size="small">
            <Row gutter={[8, 8]}>
              <Col span={12}>
                <Statistic title="Critical" value={criticalAlerts.length} valueStyle={{ color: '#ff4d4f' }} />
              </Col>
              <Col span={12}>
                <Statistic title="Warning" value={warningAlerts.length} valueStyle={{ color: '#faad14' }} />
              </Col>
              <Col span={12}>
                <Statistic title="Info" value={(alerts?.items?.filter((a) => a.severity === 'info') || []).length} />
              </Col>
              <Col span={12}>
                <Statistic title="总计" value={firingCount} />
              </Col>
            </Row>
          </Card>
        </Col>
      </Row>
    </div>
  )
}
