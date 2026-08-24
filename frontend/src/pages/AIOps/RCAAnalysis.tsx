import { useState, useEffect, useCallback } from 'react'
import {
  Card,
  Row,
  Col,
  Tag,
  Button,
  Space,
  Select,
  Empty,
  Spin,
  Alert,
  Timeline,
  Collapse,
  Table,
  Badge,
  Tooltip,
  Divider,
  Typography,
  Statistic,
  Progress,
  List,
  message,
} from 'antd'
import {
  ReloadOutlined,
  PlayCircleOutlined,
  EyeOutlined,
  AlertOutlined,
  ThunderboltOutlined,
  BarChartOutlined,
  FileTextOutlined,
  ClusterOutlined,
  RobotOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined,
  ClockCircleOutlined,
  ExclamationCircleOutlined,
  LinkOutlined,
} from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { rcaApi } from '@/api/rca'
import { incidentApi } from '@/api/incident'
import type { EvidenceBundle } from '@/api/incident'
import { aiAnalysisApi } from '@/api/aiAnalysis'
import { topologyApi } from '@/api/topology'
import type {
  Incident,
  RCAResult,
  RCACandidate,
  RCAEvidence,
  RCATimelineItem,
  RCAStatus,
  AIAnalysisResult,
} from '@/types'

const { Title, Text, Paragraph } = Typography
const { Panel } = Collapse

// ============== Helpers ==============

const severityColor = (s: string) => {
  switch (s) {
    case 'critical':
    case 'p1':
      return 'red'
    case 'warning':
    case 'p2':
      return 'orange'
    case 'info':
    case 'p3':
      return 'blue'
    default:
      return 'default'
  }
}

const statusColor = (s: string) => {
  switch (s) {
    case 'open':
      return 'red'
    case 'acknowledged':
      return 'orange'
    case 'resolved':
      return 'green'
    case 'closed':
      return 'default'
    default:
      return 'default'
  }
}

const rcaStatusMeta = (s: RCAStatus) => {
  switch (s) {
    case 'completed':
      return { color: 'green', icon: <CheckCircleOutlined />, text: '已完成' }
    case 'analyzing':
      return { color: 'blue', icon: <Spin size="small" />, text: '分析中' }
    case 'insufficient_evidence':
      return { color: 'orange', icon: <ExclamationCircleOutlined />, text: '证据不足' }
    case 'failed':
      return { color: 'red', icon: <CloseCircleOutlined />, text: '失败' }
    default:
      return { color: 'default', icon: <ClockCircleOutlined />, text: '未知' }
  }
}

const evidenceTypeMeta = (t: string) => {
  switch (t) {
    case 'alert':
      return { color: 'red', icon: <AlertOutlined />, label: '告警' }
    case 'anomaly':
      return { color: 'orange', icon: <ThunderboltOutlined />, label: '异常' }
    case 'metric':
      return { color: 'blue', icon: <BarChartOutlined />, label: '指标' }
    case 'log':
      return { color: 'purple', icon: <FileTextOutlined />, label: '日志' }
    case 'event':
      return { color: 'cyan', icon: <ClusterOutlined />, label: 'K8s 事件' }
    case 'topology':
      return { color: 'geekblue', icon: <ClusterOutlined />, label: '拓扑' }
    default:
      return { color: 'default', icon: <FileTextOutlined />, label: t }
  }
}

const formatTime = (t: string) => {
  if (!t) return '-'
  try {
    const d = new Date(t)
    return d.toLocaleString('zh-CN', { hour12: false })
  } catch {
    return t
  }
}

const formatDuration = (start: string, end?: string | null) => {
  if (!start) return '-'
  const s = new Date(start).getTime()
  const e = end ? new Date(end).getTime() : Date.now()
  const diff = Math.max(0, Math.floor((e - s) / 1000))
  const h = Math.floor(diff / 3600)
  const m = Math.floor((diff % 3600) / 60)
  const sec = diff % 60
  if (h > 0) return `${h}h ${m}m`
  if (m > 0) return `${m}m ${sec}s`
  return `${sec}s`
}

// ============== Main Component ==============

export default function RCAAnalysis() {
  const navigate = useNavigate()
  const [incidents, setIncidents] = useState<Incident[]>([])
  const [selectedIncidentId, setSelectedIncidentId] = useState<number | null>(null)
  const [incident, setIncident] = useState<Incident | null>(null)
  const [rcaResult, setRcaResult] = useState<RCAResult | null>(null)
  const [aiResult, setAiResult] = useState<AIAnalysisResult | null>(null)
  const [evidenceBundle, setEvidenceBundle] = useState<EvidenceBundle | null>(null)
  const [timeline, setTimeline] = useState<any[]>([])
  const [loading, setLoading] = useState(false)
  const [rcaRunning, setRcaRunning] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // 加载 Incident 列表
  useEffect(() => {
    incidentApi
      .list({ page: 1, page_size: 50 })
      .then((res) => {
        setIncidents(res.items || [])
        if (res.items && res.items.length > 0) {
          setSelectedIncidentId(res.items[0].id)
        }
      })
      .catch(() => {
        setError('加载事件列表失败')
      })
  }, [])

  // 加载选中 Incident 的所有数据
  const loadIncidentData = useCallback(async (id: number) => {
    setLoading(true)
    setError(null)
    try {
      const [inc, rca, ai, ev, tl] = await Promise.all([
        incidentApi.get(id).catch(() => null),
        rcaApi.getLatest(id).catch(() => null),
        aiAnalysisApi.getLatest(id).catch(() => null),
        incidentApi.evidence(id).catch(() => null),
        incidentApi.timeline(id).catch(() => null),
      ])
      setIncident(inc)
      setRcaResult(rca)
      setAiResult(ai)
      setEvidenceBundle(ev)
      setTimeline((tl?.items || []).map((s: any) => ({ timestamp: s.timestamp, type: s.signal_type || s.type || "signal", description: s.title || s.description || "", severity: s.severity, resource: s.resource_name })))
    } catch (e: any) {
      setError(e?.message || '加载数据失败')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    if (selectedIncidentId) {
      loadIncidentData(selectedIncidentId)
    }
  }, [selectedIncidentId, loadIncidentData])

  // 运行 RCA
  const handleRunRCA = async () => {
    if (!selectedIncidentId) return
    setRcaRunning(true)
    setError(null)
    try {
      const result = await rcaApi.analyze(selectedIncidentId)
      setRcaResult(result)
      message.success('RCA 分析完成')
      // 刷新证据和时间线
      const [ev, tl] = await Promise.all([
        incidentApi.evidence(selectedIncidentId).catch(() => null),
        incidentApi.timeline(selectedIncidentId).catch(() => null),
      ])
      setEvidenceBundle(ev)
      setTimeline((tl?.items || []).map((s: any) => ({ timestamp: s.timestamp, type: s.signal_type || s.type || "signal", description: s.title || s.description || "", severity: s.severity, resource: s.resource_name })))
    } catch (e: any) {
      setError(e?.message || 'RCA 分析失败')
      message.error('RCA 分析失败')
    } finally {
      setRcaRunning(false)
    }
  }

  // 刷新
  const handleRefresh = () => {
    if (selectedIncidentId) {
      loadIncidentData(selectedIncidentId)
    }
  }

  // 加载 Impact
  const [impactData, setImpactData] = useState<any>(null)
  const loadImpact = useCallback(async () => {
    if (!incident?.resource_type || !incident?.resource_name) return
    try {
      const data = await topologyApi.getImpact(
        incident.cluster || 'default',
        incident.resource_type as any,
        incident.namespace || 'default',
        incident.resource_name,
      )
      setImpactData(data)
    } catch {
      // ignore
    }
  }, [incident])

  useEffect(() => {
    if (incident) {
      loadImpact()
    }
  }, [incident, loadImpact])

  const hasRCA = !!rcaResult && rcaResult.status !== 'analyzing'
  const hasAI = !!aiResult && !!aiResult.summary
  const rcaMeta = rcaResult ? rcaStatusMeta(rcaResult.status) : null

  return (
    <div style={{ padding: 16 }}>
      {/* ===== Header ===== */}
      <Card
        style={{ marginBottom: 16 }}
        bodyStyle={{ padding: '16px 24px' }}
      >
        <Row align="middle" gutter={16}>
          <Col flex="auto">
            <Space direction="vertical" size={4} style={{ width: '100%' }}>
              <Space wrap>
                <Title level={4} style={{ margin: 0 }}>
                  根因分析 (RCA)
                </Title>
                {incident && (
                  <>
                    <Tag color={severityColor(incident.severity)}>
                      {incident.severity?.toUpperCase()}
                    </Tag>
                    <Tag color={statusColor(incident.status)}>{incident.status}</Tag>
                  </>
                )}
              </Space>
              {incident && (
                <Space size="large" wrap>
                  <Text type="secondary">
                    事件: <Text strong>{incident.title}</Text>
                  </Text>
                  {incident.service && (
                    <Text type="secondary">
                      服务: <Text strong>{incident.service}</Text>
                    </Text>
                  )}
                  {incident.cluster && (
                    <Text type="secondary">
                      集群: <Text strong>{incident.cluster}</Text>
                    </Text>
                  )}
                  {incident.namespace && (
                    <Text type="secondary">
                      命名空间: <Text strong>{incident.namespace}</Text>
                    </Text>
                  )}
                  {incident.resource_name && (
                    <Text type="secondary">
                      资源: <Text strong>{incident.resource_name}</Text>
                    </Text>
                  )}
                  <Text type="secondary">
                    持续: <Text strong>{formatDuration(incident.start_time, incident.end_time)}</Text>
                  </Text>
                </Space>
              )}
            </Space>
          </Col>
          <Col>
            <Space>
              <Select
                style={{ width: 320 }}
                placeholder="选择事件"
                value={selectedIncidentId}
                onChange={(v) => setSelectedIncidentId(v)}
                showSearch
                optionFilterProp="label"
                options={incidents.map((i) => ({
                  value: i.id,
                  label: `#${i.id} ${i.title}`,
                }))}
              />
              <Button
                type="primary"
                icon={<PlayCircleOutlined />}
                loading={rcaRunning}
                onClick={handleRunRCA}
                disabled={!selectedIncidentId}
              >
                运行 RCA
              </Button>
              <Button icon={<ReloadOutlined />} onClick={handleRefresh} disabled={loading}>
                刷新
              </Button>
              <Button
                icon={<EyeOutlined />}
                onClick={() => incident && navigate(`/aiops/incidents/${incident.id}`)}
                disabled={!incident}
              >
                查看事件
              </Button>
            </Space>
          </Col>
        </Row>
      </Card>

      {error && (
        <Alert
          message="错误"
          description={error}
          type="error"
          showIcon
          closable
          style={{ marginBottom: 16 }}
        />
      )}

      {loading && (
        <div style={{ textAlign: 'center', padding: 60 }}>
          <Spin size="large" tip="加载中..." />
        </div>
      )}

      {!loading && !selectedIncidentId && (
        <Card>
          <Empty description="请选择一个事件进行根因分析" />
        </Card>
      )}

      {!loading && selectedIncidentId && (
        <>
          {/* ===== RCA Summary ===== */}
          <Row gutter={16} style={{ marginBottom: 16 }}>
            <Col xs={24} lg={16}>
              <Card
                title={
                  <Space>
                    <ThunderboltOutlined style={{ color: '#722ed1' }} />
                    <span>RCA 引擎分析结果</span>
                    {rcaMeta && <Tag color={rcaMeta.color}>{rcaMeta.icon} {rcaMeta.text}</Tag>}
                  </Space>
                }
                style={{ height: '100%' }}
              >
                {!rcaResult && (
                  <Empty
                    description={
                      <Space direction="vertical">
                        <span>暂无 RCA 分析结果</span>
                        <Button type="primary" icon={<PlayCircleOutlined />} onClick={handleRunRCA}>
                          立即运行 RCA
                        </Button>
                      </Space>
                    }
                  />
                )}
                {rcaResult && (
                  <Space direction="vertical" size="middle" style={{ width: '100%' }}>
                    <div>
                      <Text type="secondary">根因:</Text>
                      <div style={{ marginTop: 4 }}>
                        <Title level={4} style={{ margin: 0, color: '#cf1322' }}>
                          {rcaResult.root_cause || '未确定'}
                        </Title>
                      </div>
                    </div>
                    <Row gutter={24}>
                      <Col span={8}>
                        <Statistic
                          title="置信度"
                          value={Math.round((rcaResult.confidence || 0) * 100)}
                          suffix="%"
                          valueStyle={{
                            color:
                              (rcaResult.confidence || 0) >= 0.7
                                ? '#3f8600'
                                : (rcaResult.confidence || 0) >= 0.4
                                  ? '#d48806'
                                  : '#cf1322',
                          }}
                        />
                        <Progress
                          percent={Math.round((rcaResult.confidence || 0) * 100)}
                          size="small"
                          showInfo={false}
                          strokeColor={
                            (rcaResult.confidence || 0) >= 0.7
                              ? '#52c41a'
                              : (rcaResult.confidence || 0) >= 0.4
                                ? '#faad14'
                                : '#ff4d4f'
                          }
                        />
                      </Col>
                      <Col span={8}>
                        <Statistic
                          title="证据数量"
                          value={rcaResult.evidence?.length || 0}
                          prefix={<FileTextOutlined />}
                        />
                      </Col>
                      <Col span={8}>
                        <Statistic
                          title="候选根因"
                          value={rcaResult.candidates?.length || 0}
                          prefix={<ExclamationCircleOutlined />}
                        />
                      </Col>
                    </Row>
                    {rcaResult.explanation && (
                      <div>
                        <Text type="secondary">分析说明:</Text>
                        <Paragraph
                          style={{
                            marginTop: 4,
                            padding: 12,
                            background: '#fafafa',
                            borderRadius: 4,
                            marginBottom: 0,
                          }}
                        >
                          {rcaResult.explanation}
                        </Paragraph>
                      </div>
                    )}
                    {rcaResult.impact && rcaResult.impact.length > 0 && (
                      <div>
                        <Text type="secondary">影响范围:</Text>
                        <div style={{ marginTop: 4 }}>
                          {rcaResult.impact.map((imp, i) => (
                            <Tag key={i} color="red" icon={<AlertOutlined />}>
                              {imp}
                            </Tag>
                          ))}
                        </div>
                      </div>
                    )}
                    {rcaResult.generated_at && (
                      <Text type="secondary" style={{ fontSize: 12 }}>
                        分析时间: {formatTime(rcaResult.generated_at)}
                      </Text>
                    )}
                  </Space>
                )}
              </Card>
            </Col>

            {/* ===== AI Analysis Summary ===== */}
            <Col xs={24} lg={8}>
              <Card
                title={
                  <Space>
                    <RobotOutlined style={{ color: '#1890ff' }} />
                    <span>AI 分析假设</span>
                  </Space>
                }
                style={{ height: '100%' }}
                extra={
                  hasAI && (
                    <Tag color="blue">AI Hypothesis</Tag>
                  )
                }
              >
                {!hasAI && (
                  <Empty
                    description={
                      <Space direction="vertical" size="small">
                        <span>暂无 AI 分析结果</span>
                        <Text type="secondary" style={{ fontSize: 12 }}>
                          AI 分析与 RCA 引擎相互独立，可在事件详情页触发
                        </Text>
                      </Space>
                    }
                  />
                )}
                {hasAI && aiResult && (
                  <Space direction="vertical" size="small" style={{ width: '100%' }}>
                    <Text type="secondary">AI 摘要:</Text>
                    <Paragraph
                      ellipsis={{ rows: 4, expandable: true, symbol: '展开' }}
                      style={{ marginBottom: 0 }}
                    >
                      {aiResult.summary}
                    </Paragraph>
                    <Divider style={{ margin: '8px 0' }} />
                    <Row gutter={8}>
                      <Col span={12}>
                        <Statistic
                          title="AI 置信度"
                          value={Math.round((aiResult.confidence || 0) * 100)}
                          suffix="%"
                          valueStyle={{ fontSize: 18 }}
                        />
                      </Col>
                      <Col span={12}>
                        <Statistic
                          title="AI 证据"
                          value={aiResult.evidence?.length || 0}
                          valueStyle={{ fontSize: 18 }}
                        />
                      </Col>
                    </Row>
                    {aiResult.recommendations && aiResult.recommendations.length > 0 && (
                      <>
                        <Divider style={{ margin: '8px 0' }} />
                        <Text type="secondary">AI 建议:</Text>
                        <List
                          size="small"
                          dataSource={aiResult.recommendations.slice(0, 3)}
                          renderItem={(item: any) => (
                            <List.Item>
                              <Text ellipsis style={{ fontSize: 12 }}>
                                {item.action || item.description || JSON.stringify(item)}
                              </Text>
                            </List.Item>
                          )}
                        />
                      </>
                    )}
                  </Space>
                )}
              </Card>
            </Col>
          </Row>

          {/* ===== Root Cause Candidates ===== */}
          {rcaResult && rcaResult.candidates && rcaResult.candidates.length > 0 && (
            <Card
              title={
                <Space>
                  <ExclamationCircleOutlined style={{ color: '#fa8c16' }} />
                  <span>根因候选 ({rcaResult.candidates.length})</span>
                </Space>
              }
              style={{ marginBottom: 16 }}
            >
              <Collapse defaultActiveKey={['0']}>
                {rcaResult.candidates.map((cand: RCACandidate, idx: number) => (
                  <Panel
                    key={idx}
                    header={
                      <Space wrap>
                        <Tag color="geekblue">#{idx + 1}</Tag>
                        <Text strong>{cand.root_cause}</Text>
                        <Tag color={cand.confidence >= 0.7 ? 'green' : cand.confidence >= 0.4 ? 'orange' : 'red'}>
                          置信度 {Math.round(cand.confidence * 100)}%
                        </Tag>
                        <Tag>
                          评分 {cand.score?.toFixed(2)}
                        </Tag>
                        <Tag icon={<FileTextOutlined />}>
                          {cand.evidence?.length || 0} 证据
                        </Tag>
                        {cand.resource_name && (
                          <Tag color="purple">
                            {cand.resource_type}/{cand.resource_name}
                          </Tag>
                        )}
                      </Space>
                    }
                  >
                    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
                      {cand.explanation && (
                        <div>
                          <Text type="secondary">为什么是这个根因:</Text>
                          <Paragraph style={{ marginTop: 4, marginBottom: 0 }}>
                            {cand.explanation}
                          </Paragraph>
                        </div>
                      )}
                      {cand.evidence && cand.evidence.length > 0 && (
                        <div>
                          <Text type="secondary">关联证据:</Text>
                          <Table
                            size="small"
                            style={{ marginTop: 8 }}
                            dataSource={cand.evidence}
                            rowKey="id"
                            pagination={false}
                            columns={[
                              {
                                title: '类型',
                                dataIndex: 'type',
                                width: 100,
                                render: (t: string) => {
                                  const m = evidenceTypeMeta(t)
                                  return (
                                    <Tag color={m.color} icon={m.icon}>
                                      {m.label}
                                    </Tag>
                                  )
                                },
                              },
                              { title: '来源', dataIndex: 'source', width: 150 },
                              {
                                title: '时间',
                                dataIndex: 'timestamp',
                                width: 180,
                                render: (t: string) => formatTime(t),
                              },
                              { title: '描述', dataIndex: 'description', ellipsis: true },
                              {
                                title: '相关度',
                                dataIndex: 'score',
                                width: 100,
                                render: (s: number) => (
                                  <Progress
                                    percent={Math.round((s || 0) * 100)}
                                    size="small"
                                    showInfo={false}
                                  />
                                ),
                              },
                            ]}
                          />
                        </div>
                      )}
                      {cand.impact && cand.impact.length > 0 && (
                        <div>
                          <Text type="secondary">影响:</Text>
                          <div style={{ marginTop: 4 }}>
                            {cand.impact.map((imp, i) => (
                              <Tag key={i} color="orange">
                                {imp}
                              </Tag>
                            ))}
                          </div>
                        </div>
                      )}
                    </Space>
                  </Panel>
                ))}
              </Collapse>
            </Card>
          )}

          {/* ===== Evidence + Timeline + Impact ===== */}
          <Row gutter={16} style={{ marginBottom: 16 }}>
            {/* Evidence */}
            <Col xs={24} lg={14}>
              <Card
                title={
                  <Space>
                    <FileTextOutlined style={{ color: '#13c2c2' }} />
                    <span>证据链</span>
                    <Badge
                      count={rcaResult?.evidence?.length || evidenceBundle ? 'loaded' : 0}
                      style={{ backgroundColor: '#13c2c2' }}
                    />
                  </Space>
                }
                style={{ height: '100%' }}
              >
                {(!rcaResult?.evidence || rcaResult.evidence.length === 0) &&
                  (!evidenceBundle ||
                    (evidenceBundle.alerts?.length === 0 &&
                      evidenceBundle.anomalies?.length === 0 &&
                      evidenceBundle.events?.length === 0 &&
                      evidenceBundle.metrics?.length === 0)) && (
                    <Empty description="暂无证据数据，运行 RCA 后将自动收集证据" />
                  )}

                {/* RCA Evidence */}
                {rcaResult?.evidence && rcaResult.evidence.length > 0 && (
                  <Table
                    size="small"
                    dataSource={rcaResult.evidence}
                    rowKey="id"
                    pagination={{ pageSize: 10, size: 'small' }}
                    scroll={{ x: 800 }}
                    columns={[
                      {
                        title: '类型',
                        dataIndex: 'type',
                        width: 90,
                        fixed: 'left',
                        render: (t: string) => {
                          const m = evidenceTypeMeta(t)
                          return (
                            <Tag color={m.color} icon={m.icon} style={{ margin: 0 }}>
                              {m.label}
                            </Tag>
                          )
                        },
                      },
                      { title: '来源', dataIndex: 'source', width: 130, ellipsis: true },
                      {
                        title: '资源',
                        width: 150,
                        ellipsis: true,
                        render: (_: any, r: RCAEvidence) =>
                          r.resource_name ? `${r.resource_type}/${r.resource_name}` : '-',
                      },
                      {
                        title: '时间',
                        dataIndex: 'timestamp',
                        width: 170,
                        render: (t: string) => formatTime(t),
                      },
                      { title: '描述', dataIndex: 'description', ellipsis: true },
                      {
                        title: '相关度',
                        dataIndex: 'score',
                        width: 90,
                        render: (s: number) => (
                          <Tooltip title={`相关度 ${Math.round((s || 0) * 100)}%`}>
                            <Progress
                              percent={Math.round((s || 0) * 100)}
                              size="small"
                              showInfo={false}
                            />
                          </Tooltip>
                        ),
                      },
                    ]}
                  />
                )}

                {/* Evidence Bundle Summary (from /evidence API) */}
                {evidenceBundle &&
                  (!rcaResult?.evidence || rcaResult.evidence.length === 0) && (
                    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
                      <Row gutter={8}>
                        <Col span={4}>
                          <Statistic title="告警" value={evidenceBundle.alerts?.length || 0} valueStyle={{ color: '#cf1322', fontSize: 18 }} />
                        </Col>
                        <Col span={4}>
                          <Statistic title="异常" value={evidenceBundle.anomalies?.length || 0} valueStyle={{ color: '#d48806', fontSize: 18 }} />
                        </Col>
                        <Col span={4}>
                          <Statistic title="事件" value={evidenceBundle.events?.length || 0} valueStyle={{ color: '#08979c', fontSize: 18 }} />
                        </Col>
                        <Col span={4}>
                          <Statistic title="指标" value={evidenceBundle.metrics?.length || 0} valueStyle={{ color: '#1d39c4', fontSize: 18 }} />
                        </Col>
                        <Col span={4}>
                          <Statistic title="日志" value={evidenceBundle.logs?.length || 0} valueStyle={{ color: '#531dab', fontSize: 18 }} />
                        </Col>
                        <Col span={4}>
                          <Statistic title="时间线" value={evidenceBundle.timeline?.length || 0} valueStyle={{ fontSize: 18 }} />
                        </Col>
                      </Row>
                      <Text type="secondary" style={{ fontSize: 12 }}>
                        证据已收集，运行 RCA 后将展示详细证据链
                      </Text>
                    </Space>
                  )}
              </Card>
            </Col>

            {/* Timeline */}
            <Col xs={24} lg={10}>
              <Card
                title={
                  <Space>
                    <ClockCircleOutlined style={{ color: '#722ed1' }} />
                    <span>事件时间线</span>
                  </Space>
                }
                style={{ height: '100%' }}
              >
                {(!timeline || timeline.length === 0) &&
                  (!rcaResult?.timeline || rcaResult.timeline.length === 0) && (
                    <Empty description="暂无时间线数据" />
                  )}
                <div style={{ maxHeight: 480, overflowY: 'auto' }}>
                  <Timeline
                    items={[
                      ...(rcaResult?.timeline || []),
                      ...(timeline || []),
                    ]
                      .sort((a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime())
                      .map((item: RCATimelineItem, idx: number) => ({
                        key: idx,
                        color:
                          item.severity === 'critical' || item.severity === 'high'
                            ? 'red'
                            : item.severity === 'warning'
                              ? 'orange'
                              : item.severity === 'info'
                                ? 'blue'
                                : 'gray',
                        children: (
                          <div>
                            <div style={{ fontSize: 12, color: '#999' }}>
                              {formatTime(item.timestamp)}
                            </div>
                            <div style={{ fontSize: 13, marginTop: 2 }}>
                              <Tag color="geekblue" style={{ fontSize: 11 }}>
                                {item.type}
                              </Tag>
                              {item.description}
                            </div>
                            {item.resource && (
                              <div style={{ fontSize: 11, color: '#999', marginTop: 2 }}>
                                资源: {item.resource}
                              </div>
                            )}
                          </div>
                        ),
                      }))}
                  />
                </div>
              </Card>
            </Col>
          </Row>

          {/* ===== Impact Analysis ===== */}
          <Card
            title={
              <Space>
                <ClusterOutlined style={{ color: '#eb2f96' }} />
                <span>影响分析</span>
                {incident?.resource_name && (
                  <Tag color="purple">
                    {incident.resource_type}/{incident.resource_name}
                  </Tag>
                )}
              </Space>
            }
            style={{ marginBottom: 16 }}
          >
            {!impactData && (
              <Empty description="暂无影响分析数据（需要资源关联拓扑）" />
            )}
            {impactData && (
              <Row gutter={16}>
                <Col span={12}>
                  <div>
                    <Text type="secondary">受影响的上游服务:</Text>
                    <div style={{ marginTop: 8 }}>
                      {(impactData.upstream || impactData.affected_upstream || []).map(
                        (node: any, i: number) => (
                          <Tag key={i} color="red" icon={<LinkOutlined />} style={{ marginBottom: 4 }}>
                            {node.name || node.service || JSON.stringify(node)}
                          </Tag>
                        ),
                      )}
                      {(!impactData.upstream || impactData.upstream.length === 0) &&
                        (!impactData.affected_upstream || impactData.affected_upstream.length === 0) && (
                          <Text type="secondary">无</Text>
                        )}
                    </div>
                  </div>
                </Col>
                <Col span={12}>
                  <div>
                    <Text type="secondary">受影响的下游服务:</Text>
                    <div style={{ marginTop: 8 }}>
                      {(impactData.downstream || impactData.affected_downstream || []).map(
                        (node: any, i: number) => (
                          <Tag key={i} color="orange" icon={<LinkOutlined />} style={{ marginBottom: 4 }}>
                            {node.name || node.service || JSON.stringify(node)}
                          </Tag>
                        ),
                      )}
                      {(!impactData.downstream || impactData.downstream.length === 0) &&
                        (!impactData.affected_downstream ||
                          impactData.affected_downstream.length === 0) && (
                          <Text type="secondary">无</Text>
                        )}
                    </div>
                  </div>
                </Col>
              </Row>
            )}
          </Card>

          {/* ===== AI Analysis Detail ===== */}
          {hasAI && aiResult && (
            <Card
              title={
                <Space>
                  <RobotOutlined style={{ color: '#1890ff' }} />
                  <span>AI 分析详情</span>
                  <Tag color="blue">AI Hypothesis</Tag>
                </Space>
              }
              style={{ marginBottom: 16 }}
            >
              <Row gutter={16}>
                <Col span={12}>
                  <div>
                    <Text type="secondary">AI 根因解释:</Text>
                    <Paragraph
                      style={{
                        marginTop: 4,
                        padding: 12,
                        background: '#e6f7ff',
                        borderRadius: 4,
                        borderLeft: '3px solid #1890ff',
                      }}
                    >
                      {aiResult.root_cause_explanation}
                    </Paragraph>
                  </div>
                </Col>
                <Col span={12}>
                  <div>
                    <Text type="secondary">AI 证据引用:</Text>
                    {aiResult.evidence && aiResult.evidence.length > 0 ? (
                      <List
                        size="small"
                        style={{ marginTop: 8 }}
                        dataSource={aiResult.evidence.slice(0, 5)}
                        renderItem={(item: any) => (
                          <List.Item>
                            <Space>
                              <Tag color={evidenceTypeMeta(item.type).color}>
                                {evidenceTypeMeta(item.type).label}
                              </Tag>
                              <Text ellipsis style={{ maxWidth: 300, fontSize: 12 }}>
                                {item.description}
                              </Text>
                            </Space>
                          </List.Item>
                        )}
                      />
                    ) : (
                      <Text type="secondary">无</Text>
                    )}
                  </div>
                </Col>
              </Row>
              {aiResult.recommendations && aiResult.recommendations.length > 0 && (
                <>
                  <Divider />
                  <Text type="secondary">AI 推荐操作:</Text>
                  <List
                    size="small"
                    style={{ marginTop: 8 }}
                    dataSource={aiResult.recommendations}
                    renderItem={(item: any, idx: number) => (
                      <List.Item>
                        <Space>
                          <Tag color="geekblue">#{idx + 1}</Tag>
                          <Text strong>{item.action || item.type || '操作'}</Text>
                          <Text style={{ fontSize: 12 }}>{item.description || item.reason}</Text>
                          {item.risk && <Tag color={item.risk === 'high' ? 'red' : 'orange'}>{item.risk}</Tag>}
                        </Space>
                      </List.Item>
                    )}
                  />
                </>
              )}
              {aiResult.generated_at && (
                <div style={{ marginTop: 12, textAlign: 'right' }}>
                  <Text type="secondary" style={{ fontSize: 12 }}>
                    AI 分析时间: {formatTime(aiResult.generated_at)}
                    {aiResult.model && ` | 模型: ${aiResult.model}`}
                  </Text>
                </div>
              )}
            </Card>
          )}

          {/* ===== Footer Note ===== */}
          <Card bodyStyle={{ padding: 12 }}>
            <Space size="large" wrap>
              <Text type="secondary" style={{ fontSize: 12 }}>
                <ThunderboltOutlined /> RCA 引擎: 基于规则 + 证据评分的确定性根因分析
              </Text>
              <Text type="secondary" style={{ fontSize: 12 }}>
                <RobotOutlined /> AI 分析: 大模型生成的假设性解释，需结合 RCA 证据判断
              </Text>
              <Text type="secondary" style={{ fontSize: 12 }}>
                <FileTextOutlined /> 所有结论均基于真实采集的 Evidence，禁止编造
              </Text>
            </Space>
          </Card>
        </>
      )}
    </div>
  )
}
