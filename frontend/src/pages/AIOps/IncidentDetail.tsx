import { useEffect, useState, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  Drawer, Tabs, Descriptions, Tag, Spin, Timeline, Empty, Button, Space,
  Modal, message, Badge, Card,
} from 'antd'
import {
  CheckCircleOutlined, StopOutlined, CloseCircleOutlined,
  AlertOutlined, FireOutlined, InfoCircleOutlined,
} from '@ant-design/icons'
import { incidentApi } from '@/api/incident'
import { topologyApi } from '@/api/topology'
import { rcaApi } from '@/api/rca'
import { aiAnalysisApi } from '@/api/aiAnalysis'
import type { Incident, IncidentSignal, TopologyNode, RCAResult, AIAnalysisResult } from '@/types'
import dayjs from 'dayjs'

const severityColor: Record<string, string> = {
  critical: 'error',
  warning: 'warning',
  info: 'info',
}

const statusColor: Record<string, string> = {
  open: 'error',
  acknowledged: 'warning',
  resolved: 'success',
  closed: 'default',
}

const statusLabel: Record<string, string> = {
  open: '进行中',
  acknowledged: '已确认',
  resolved: '已解决',
  closed: '已关闭',
}

const signalTypeIcon: Record<string, any> = {
  alert: <AlertOutlined />,
  anomaly: <FireOutlined />,
  log: <InfoCircleOutlined />,
  k8s_event: <InfoCircleOutlined />,
  metric: <InfoCircleOutlined />,
}

const signalTypeLabel: Record<string, string> = {
  alert: '告警',
  anomaly: '异常',
  log: '日志',
  k8s_event: 'K8s事件',
  metric: '指标',
}

interface Props {
  id: number | null
  open: boolean
  onClose: () => void
  onChanged: () => void
}

export default function IncidentDetail({ id, open, onClose, onChanged }: Props) {
  const navigate = useNavigate()
  const [incident, setIncident] = useState<Incident | null>(null)
  const [signals, setSignals] = useState<IncidentSignal[]>([])
  const [loading, setLoading] = useState(false)
  const [tab, setTab] = useState('overview')
  const [topologyNodes, setTopologyNodes] = useState<TopologyNode[]>([])
  const [topologyLoading, setTopologyLoading] = useState(false)
  const [rcaResult, setRcaResult] = useState<RCAResult | null>(null)
  const [rcaLoading, setRcaLoading] = useState(false)
  const [aiResult, setAiResult] = useState<AIAnalysisResult | null>(null)
  const [aiLoading, setAiLoading] = useState(false)

  const fetchDetail = useCallback(async () => {
    if (!id) return
    setLoading(true)
    try {
      const [incRes, sigRes] = await Promise.all([
        incidentApi.get(id),
        incidentApi.signals(id),
      ])
      setIncident(incRes)
      setSignals(sigRes.items || [])
    } finally {
      setLoading(false)
    }
  }, [id])

  useEffect(() => {
    if (open && id) {
      setTab('overview')
      fetchDetail()
    }
  }, [open, id, fetchDetail])

  // 加载拓扑数据（受影响资源的上下游）
  useEffect(() => {
    if (tab !== 'topology' || !incident) return
    setTopologyLoading(true)
    const load = async () => {
      try {
        // 用 incident 的 resource_name 查询影响范围
        if (incident.resource_name && incident.resource_type) {
          const res = await topologyApi.getImpact(
            incident.cluster || 'local',
            incident.resource_type as any,
            incident.namespace || '',
            incident.resource_name
          )
          setTopologyNodes(res.affected_nodes || [])
        } else {
          setTopologyNodes([])
        }
      } catch {
        setTopologyNodes([])
      } finally {
        setTopologyLoading(false)
      }
    }
    load()
  }, [tab, incident])

  // 加载 RCA 结果
  useEffect(() => {
    if (tab !== 'rca' || !id) return
    setRcaLoading(true)
    const load = async () => {
      try {
        const res = await rcaApi.getLatest(id)
        setRcaResult(res)
      } catch {
        setRcaResult(null)
      } finally {
        setRcaLoading(false)
      }
    }
    load()
  }, [tab, id])

  const handleRunRCA = async () => {
    if (!id) return
    setRcaLoading(true)
    try {
      const res = await rcaApi.analyze(id)
      setRcaResult(res)
      message.success('RCA 分析完成')
    } catch (e: any) {
      message.error('RCA 分析失败: ' + (e?.message || '未知错误'))
    } finally {
      setRcaLoading(false)
    }
  }

  // 加载 AI 分析结果
  useEffect(() => {
    if (tab !== 'ai' || !id) return
    setAiLoading(true)
    const load = async () => {
      try {
        const res = await aiAnalysisApi.getLatest(id)
        setAiResult(res)
      } catch {
        setAiResult(null)
      } finally {
        setAiLoading(false)
      }
    }
    load()
  }, [tab, id])

  const handleRunAI = async () => {
    if (!id) return
    setAiLoading(true)
    try {
      const res = await aiAnalysisApi.analyze(id)
      setAiResult(res)
      message.success('AI 分析完成')
    } catch (e: any) {
      message.error('AI 分析失败: ' + (e?.message || '未知错误'))
    } finally {
      setAiLoading(false)
    }
  }

  const handleAcknowledge = () => {
    if (!id) return
    Modal.confirm({
      title: '确认事件',
      content: `确认事件 #${id} ？确认后状态变为"已确认"。`,
      onOk: async () => {
        await incidentApi.acknowledge(id)
        message.success('事件已确认')
        fetchDetail()
        onChanged()
      },
    })
  }

  const handleResolve = () => {
    if (!id) return
    Modal.confirm({
      title: '解决事件',
      content: `将事件 #${id} 标记为已解决？`,
      onOk: async () => {
        await incidentApi.resolve(id)
        message.success('事件已解决')
        fetchDetail()
        onChanged()
      },
    })
  }

  const handleClose = () => {
    if (!id) return
    Modal.confirm({
      title: '关闭事件',
      content: `关闭事件 #${id} ？关闭后不可重新打开。`,
      okType: 'danger',
      onOk: async () => {
        await incidentApi.close(id)
        message.success('事件已关闭')
        fetchDetail()
        onChanged()
      },
    })
  }

  const renderOverview = () => {
    if (!incident) return null
    return (
      <Space direction="vertical" style={{ width: '100%' }} size="middle">
        <Descriptions column={1} bordered size="small">
          <Descriptions.Item label="事件 ID">#{incident.id}</Descriptions.Item>
          <Descriptions.Item label="标题">{incident.title}</Descriptions.Item>
          <Descriptions.Item label="严重度">
            <Tag color={severityColor[incident.severity]}>{incident.severity?.toUpperCase()}</Tag>
          </Descriptions.Item>
          <Descriptions.Item label="状态">
            <Badge color={statusColor[incident.status]} text={statusLabel[incident.status]} />
          </Descriptions.Item>
          <Descriptions.Item label="集群">{incident.cluster || '-'}</Descriptions.Item>
          <Descriptions.Item label="命名空间">{incident.namespace || '-'}</Descriptions.Item>
          <Descriptions.Item label="服务">{incident.service || '-'}</Descriptions.Item>
          <Descriptions.Item label="资源">
            {incident.resource_type && incident.resource_name
              ? `${incident.resource_type}/${incident.resource_name}`
              : '-'}
          </Descriptions.Item>
          <Descriptions.Item label="信号数量">{incident.signal_count}</Descriptions.Item>
          <Descriptions.Item label="开始时间">
            {dayjs(incident.start_time).format('YYYY-MM-DD HH:mm:ss')}
          </Descriptions.Item>
          <Descriptions.Item label="结束时间">
            {incident.end_time ? dayjs(incident.end_time).format('YYYY-MM-DD HH:mm:ss') : '-'}
          </Descriptions.Item>
          <Descriptions.Item label="持续时间">
            {incident.end_time
              ? dayjs(incident.end_time).diff(dayjs(incident.start_time), 'minute') + ' 分钟'
              : dayjs().diff(dayjs(incident.start_time), 'minute') + ' 分钟（进行中）'}
          </Descriptions.Item>
          {incident.root_cause && (
            <Descriptions.Item label="根因">{incident.root_cause}</Descriptions.Item>
          )}
          {incident.confidence !== undefined && incident.confidence > 0 && (
            <Descriptions.Item label="置信度">{(incident.confidence * 100).toFixed(0)}%</Descriptions.Item>
          )}
        </Descriptions>

        {incident.status === 'open' && (
          <Space>
            <Button type="primary" icon={<CheckCircleOutlined />} onClick={handleAcknowledge}>
              确认
            </Button>
            <Button icon={<StopOutlined />} onClick={handleResolve}>
              标记解决
            </Button>
          </Space>
        )}
        {incident.status === 'acknowledged' && (
          <Space>
            <Button icon={<StopOutlined />} onClick={handleResolve}>
              标记解决
            </Button>
          </Space>
        )}
        {(incident.status === 'resolved') && (
          <Button danger icon={<CloseCircleOutlined />} onClick={handleClose}>
            关闭事件
          </Button>
        )}
      </Space>
    )
  }

  const renderTimeline = () => {
    if (signals.length === 0) {
      return <Empty description="暂无时间线数据" />
    }
    const sorted = [...signals].sort(
      (a, b) => dayjs(a.timestamp).valueOf() - dayjs(b.timestamp).valueOf(),
    )
    return (
      <Timeline
        items={sorted.map((s) => ({
          color: s.resolved ? 'green' : severityColor[s.severity] || 'blue',
          children: (
            <div>
              <div style={{ fontWeight: 500 }}>
                {signalTypeIcon[s.signal_type]} {signalTypeLabel[s.signal_type] || s.signal_type}: {s.title}
                {s.resolved && <Tag color="success" style={{ marginLeft: 8 }}>已恢复</Tag>}
              </div>
              <div style={{ color: '#999', fontSize: 12, marginTop: 4 }}>
                {dayjs(s.timestamp).format('HH:mm:ss')}
                {s.service && ` · ${s.service}`}
                {s.namespace && ` · ${s.namespace}`}
              </div>
            </div>
          ),
        }))}
      />
    )
  }

  const renderSignals = () => {
    if (signals.length === 0) {
      return <Empty description="暂无关联信号" />
    }
    return (
      <Space direction="vertical" style={{ width: '100%' }} size="small">
        {signals.map((s) => (
          <Card key={s.id} size="small" styles={{ body: { padding: 12 } }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <div>
                <Tag color={severityColor[s.severity]} style={{ marginRight: 8 }}>
                  {s.severity?.toUpperCase()}
                </Tag>
                <span style={{ fontWeight: 500 }}>{s.title}</span>
                <Tag style={{ marginLeft: 8 }}>{signalTypeLabel[s.signal_type]}</Tag>
                {s.resolved && <Tag color="success">已恢复</Tag>}
              </div>
              <span style={{ color: '#999', fontSize: 12 }}>
                {dayjs(s.timestamp).format('MM-DD HH:mm:ss')}
              </span>
            </div>
            {(s.service || s.namespace || s.resource_name) && (
              <div style={{ color: '#666', fontSize: 12, marginTop: 6 }}>
                {s.namespace && `ns: ${s.namespace}`}
                {s.service && ` · svc: ${s.service}`}
                {s.resource_name && ` · ${s.resource_type}: ${s.resource_name}`}
              </div>
            )}
            {s.signal_type === 'anomaly' && s.metadata && (
              <div style={{ color: '#666', fontSize: 12, marginTop: 4 }}>
                {s.metadata.metric && `指标: ${s.metadata.metric}`}
                {s.metadata.value != null && ` · 值: ${Number(s.metadata.value).toFixed(2)}`}
                {s.metadata.anomaly_score != null && ` · 异常分: ${(Number(s.metadata.anomaly_score) * 100).toFixed(0)}%`}
              </div>
            )}
          </Card>
        ))}
      </Space>
    )
  }

  const renderTopology = () => {
    if (topologyLoading) return <Spin />
    if (!incident?.resource_name) {
      return <Empty description="此事件未关联具体资源，无法显示拓扑" />
    }
    if (topologyNodes.length === 0) {
      return <Empty description="未找到上下游资源" />
    }
    return (
      <div>
        <Card size="small" style={{ marginBottom: 12 }}>
          <Space>
            <span style={{ fontWeight: 500 }}>受影响资源：</span>
            <Tag>{incident.resource_type}</Tag>
            <span>{incident.resource_name}</span>
          </Space>
        </Card>
        <Space direction="vertical" style={{ width: '100%' }} size="small">
          {topologyNodes.map((n) => (
            <Card key={n.id} size="small" styles={{ body: { padding: 10 } }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <Space>
                  <Tag>{n.type}</Tag>
                  <span style={{ fontWeight: 500 }}>{n.name}</span>
                  {n.namespace && <span style={{ fontSize: 12, color: '#999' }}>{n.namespace}</span>}
                </Space>
                <Tag color={n.status === 'critical' ? 'red' : n.status === 'warning' ? 'orange' : 'green'}>
                  {n.status}
                </Tag>
              </div>
            </Card>
          ))}
        </Space>
      </div>
    )
  }

  const renderRCA = () => {
    if (rcaLoading) return <Spin />
    if (!rcaResult) {
      return (
        <div style={{ textAlign: 'center', padding: 40 }}>
          <Empty description="尚未执行根因分析" />
          <Button type="primary" onClick={handleRunRCA} style={{ marginTop: 16 }}>
            开始 RCA 分析
          </Button>
        </div>
      )
    }

    const statusMap: Record<string, { color: string; label: string }> = {
      completed: { color: 'green', label: '已完成' },
      analyzing: { color: 'blue', label: '分析中' },
      insufficient_evidence: { color: 'orange', label: '证据不足' },
      failed: { color: 'red', label: '失败' },
    }
    const status = statusMap[rcaResult.status] || { color: 'default', label: rcaResult.status }

    return (
      <div>
        <Card size="small" style={{ marginBottom: 12 }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 12 }}>
            <Space>
              <Tag color={status.color}>{status.label}</Tag>
              <span style={{ fontWeight: 500 }}>{rcaResult.root_cause}</span>
            </Space>
            <Button size="small" onClick={handleRunRCA}>重新分析</Button>
          </div>
          <Descriptions column={2} size="small">
            <Descriptions.Item label="置信度">
              <span style={{ fontWeight: 600, color: rcaResult.confidence > 0.7 ? '#52c41a' : rcaResult.confidence > 0.4 ? '#faad14' : '#ff4d4f' }}>
                {(rcaResult.confidence * 100).toFixed(0)}%
              </span>
            </Descriptions.Item>
            <Descriptions.Item label="分析时间">
              {dayjs(rcaResult.generated_at).format('MM-DD HH:mm:ss')}
            </Descriptions.Item>
          </Descriptions>
          {rcaResult.explanation && (
            <div style={{ marginTop: 8, fontSize: 12, color: '#666' }}>{rcaResult.explanation}</div>
          )}
        </Card>

        {/* 候选根因 */}
        {rcaResult.candidates && rcaResult.candidates.length > 0 && (
          <Card title="候选根因" size="small" style={{ marginBottom: 12 }}>
            <Space direction="vertical" style={{ width: '100%' }} size="small">
              {rcaResult.candidates.map((c, i) => (
                <div key={i} style={{ padding: 8, border: '1px solid #f0f0f0', borderRadius: 4 }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                    <Space>
                      <Tag>{c.resource_type}</Tag>
                      <span style={{ fontWeight: 500 }}>{c.resource_name}</span>
                    </Space>
                    <span style={{ fontWeight: 600 }}>{(c.confidence * 100).toFixed(0)}%</span>
                  </div>
                  <div style={{ fontSize: 12, color: '#666', marginTop: 4 }}>{c.root_cause}</div>
                  <div style={{ fontSize: 11, color: '#999', marginTop: 2 }}>
                    证据: {c.evidence?.length || 0} 条
                  </div>
                </div>
              ))}
            </Space>
          </Card>
        )}

        {/* 证据链 */}
        {rcaResult.evidence && rcaResult.evidence.length > 0 && (
          <Card title="证据链" size="small" style={{ marginBottom: 12 }}>
            <Space direction="vertical" style={{ width: '100%' }} size="small">
              {rcaResult.evidence.map((e, i) => (
                <div key={i} style={{ padding: 6, borderLeft: '3px solid #1890ff', paddingLeft: 8 }}>
                  <div style={{ fontSize: 12 }}>
                    <Tag color="blue">{e.type}</Tag>
                    <span>{e.description}</span>
                  </div>
                  <div style={{ fontSize: 11, color: '#999', marginTop: 2 }}>
                    {dayjs(e.timestamp).format('HH:mm:ss')}
                    {e.resource_name && ` · ${e.resource_name}`}
                    {e.metric && ` · ${e.metric}`}
                  </div>
                </div>
              ))}
            </Space>
          </Card>
        )}

        {/* 时间线 */}
        {rcaResult.timeline && rcaResult.timeline.length > 0 && (
          <Card title="时间线" size="small">
            <Timeline
              items={rcaResult.timeline.map((t) => ({
                color: t.severity === 'critical' ? 'red' : t.severity === 'warning' ? 'orange' : 'blue',
                children: (
                  <div>
                    <div style={{ fontSize: 12 }}>{t.description}</div>
                    <div style={{ fontSize: 11, color: '#999' }}>
                      {dayjs(t.timestamp).format('HH:mm:ss')} · {t.type}
                    </div>
                  </div>
                ),
              }))}
            />
          </Card>
        )}
      </div>
    )
  }

  const renderAI = () => {
    if (aiLoading) return <Spin />
    if (!aiResult) {
      return (
        <div style={{ textAlign: 'center', padding: 40 }}>
          <Empty description="尚未执行 AI 分析" />
          <Button type="primary" onClick={handleRunAI} style={{ marginTop: 16 }}>
            开始 AI 分析
          </Button>
        </div>
      )
    }

    const riskColor: Record<string, string> = {
      low: 'green', medium: 'orange', high: 'red', critical: 'volcano',
    }
    const priorityColor: Record<string, string> = {
      P0: 'red', P1: 'orange', P2: 'blue', P3: 'default',
    }

    return (
      <div>
        {/* Summary */}
        <Card size="small" style={{ marginBottom: 12 }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
            <span style={{ fontWeight: 500 }}>AI 分析摘要</span>
            <Button size="small" onClick={handleRunAI}>重新分析</Button>
          </div>
          <div style={{ fontSize: 13, lineHeight: 1.6 }}>{aiResult.summary}</div>
          <div style={{ marginTop: 8, fontSize: 12, color: '#666' }}>
            置信度: <span style={{ fontWeight: 600, color: aiResult.confidence > 0.7 ? '#52c41a' : aiResult.confidence > 0.4 ? '#faad14' : '#ff4d4f' }}>
              {(aiResult.confidence * 100).toFixed(0)}%
            </span>
            {aiResult.model && <span style={{ marginLeft: 12 }}>模型: {aiResult.model}</span>}
          </div>
        </Card>

        {/* Root Cause Explanation */}
        <Card title="根因解释" size="small" style={{ marginBottom: 12 }}>
          <div style={{ fontSize: 13, lineHeight: 1.6 }}>{aiResult.root_cause_explanation}</div>
        </Card>

        {/* Evidence */}
        {aiResult.evidence && aiResult.evidence.length > 0 && (
          <Card title={`证据 (${aiResult.evidence.length})`} size="small" style={{ marginBottom: 12 }}>
            <Space direction="vertical" style={{ width: '100%' }} size="small">
              {aiResult.evidence.map((e, i) => (
                <div key={i} style={{ padding: 6, borderLeft: '3px solid #1890ff', paddingLeft: 8 }}>
                  <div style={{ fontSize: 12 }}>
                    <Tag color="blue">{e.type}</Tag>
                    <Tag color={e.importance === 'high' ? 'red' : e.importance === 'medium' ? 'orange' : 'default'}>
                      {e.importance}
                    </Tag>
                    <span>{e.description}</span>
                  </div>
                  {e.resource && <div style={{ fontSize: 11, color: '#999', marginTop: 2 }}>资源: {e.resource}</div>}
                </div>
              ))}
            </Space>
          </Card>
        )}

        {/* Impact */}
        {aiResult.impact && aiResult.impact.length > 0 && (
          <Card title="影响范围" size="small" style={{ marginBottom: 12 }}>
            <Space size="small" wrap>
              {aiResult.impact.map((item, i) => (
                <Tag key={i} color={riskColor[item.impact_level] || 'default'}>
                  {item.resource_type}: {item.resource_name}
                </Tag>
              ))}
            </Space>
          </Card>
        )}

        {/* Recommendations */}
        {aiResult.recommendations && aiResult.recommendations.length > 0 && (
          <Card title="处理建议" size="small" style={{ marginBottom: 12 }}>
            <Space direction="vertical" style={{ width: '100%' }} size="small">
              {aiResult.recommendations.map((r, i) => (
                <div key={i} style={{ padding: 8, border: '1px solid #f0f0f0', borderRadius: 4 }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                    <Space>
                      <Tag color={priorityColor[r.priority]}>{r.priority}</Tag>
                      <span style={{ fontWeight: 500 }}>{r.title}</span>
                    </Space>
                    <Tag color={riskColor[r.risk]}>风险: {r.risk}</Tag>
                  </div>
                  <div style={{ fontSize: 12, color: '#666', marginTop: 4 }}>{r.description}</div>
                  <div style={{ fontSize: 11, color: '#999', marginTop: 2 }}>原因: {r.reason}</div>
                </div>
              ))}
            </Space>
          </Card>
        )}

        {/* Risks */}
        {aiResult.risks && aiResult.risks.length > 0 && (
          <Card title="风险评估" size="small" style={{ marginBottom: 12 }}>
            <Space direction="vertical" style={{ width: '100%' }} size="small">
              {aiResult.risks.map((r, i) => (
                <div key={i}>
                  <Tag color={riskColor[r.level]}>{r.level}</Tag>
                  <span style={{ fontSize: 12 }}>{r.description}</span>
                </div>
              ))}
            </Space>
          </Card>
        )}

        {/* Next Actions */}
        {aiResult.next_actions && aiResult.next_actions.length > 0 && (
          <Card title="下一步操作" size="small">
            <Timeline
              items={aiResult.next_actions.map((a) => ({
                children: (
                  <div>
                    <div style={{ fontSize: 12, fontWeight: 500 }}>{a.title}</div>
                    <div style={{ fontSize: 11, color: '#666' }}>{a.description}</div>
                    <div style={{ fontSize: 11, color: '#999' }}>原因: {a.reason}</div>
                  </div>
                ),
              }))}
            />
          </Card>
        )}

        {/* Data Sources */}
        <Card title="数据源状态" size="small" style={{ marginTop: 12 }}>
          <Space size="small" wrap>
            <Tag color={aiResult.data_sources.alerts_available ? 'green' : 'default'}>Alerts</Tag>
            <Tag color={aiResult.data_sources.anomalies_available ? 'green' : 'default'}>Anomalies</Tag>
            <Tag color={aiResult.data_sources.metrics_available ? 'green' : 'default'}>Metrics</Tag>
            <Tag color={aiResult.data_sources.logs_available ? 'green' : 'default'}>Logs</Tag>
            <Tag color={aiResult.data_sources.events_available ? 'green' : 'default'}>Events</Tag>
            <Tag color={aiResult.data_sources.topology_available ? 'green' : 'default'}>Topology</Tag>
            <Tag color={aiResult.data_sources.rca_available ? 'green' : 'default'}>RCA</Tag>
          </Space>
        </Card>
      </div>
    )
  }

  return (
    <Drawer
      title={incident ? `事件 #${incident.id}: ${incident.title}` : '事件详情'}
      width={640}
      open={open}
      onClose={onClose}
      destroyOnClose
      extra={
        <Button
          type="primary"
          size="small"
          icon={<span>🤖</span>}
          onClick={() => {
            onClose()
            navigate(`/ai/assistant?incident_id=${id}`)
          }}
        >
          Ask AI
        </Button>
      }
    >
      <Spin spinning={loading}>
        <Tabs
          activeKey={tab}
          onChange={setTab}
          items={[
            { key: 'overview', label: '概览', children: renderOverview() },
            { key: 'timeline', label: `时间线 (${signals.length})`, children: renderTimeline() },
            { key: 'signals', label: `关联信号 (${signals.length})`, children: renderSignals() },
            { key: 'topology', label: '拓扑影响', children: renderTopology() },
            { key: 'rca', label: '根因分析', children: renderRCA() },
            { key: 'ai', label: 'AI 分析', children: renderAI() },
          ]}
        />
      </Spin>
    </Drawer>
  )
}
