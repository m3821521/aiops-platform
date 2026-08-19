import { useState, useRef, useEffect } from 'react'
import { useSearchParams, useNavigate } from 'react-router-dom'
import {
  Card, Input, Button, Space, Typography, Spin, Alert, Tag, List, Avatar, Divider,
  Timeline, Badge,
} from 'antd'
import {
  RobotOutlined, UserOutlined, SendOutlined, ThunderboltOutlined,
  BulbOutlined, CheckCircleOutlined, WarningOutlined, CloseCircleOutlined,
  ExperimentOutlined, DatabaseOutlined, CloudOutlined, AlertOutlined,
  BarChartOutlined, FileTextOutlined, ApartmentOutlined,
} from '@ant-design/icons'
import { aiApi, type AIAskResponse, type AIToolCall } from '@/api/ai'

const { Text, Paragraph, Title } = Typography
const { TextArea } = Input

interface ChatMessage {
  id: string
  role: 'user' | 'assistant'
  content: string
  summary?: string
  root_cause?: string
  confidence?: number
  evidence?: { source: string; description: string; resource?: string }[]
  recommendations?: { priority: string; title: string; risk: string }[]
  tool_calls?: AIToolCall[]
  timestamp: number
  loading?: boolean
  error?: string
}

const quickQuestions = [
  '分析这个 Incident',
  '为什么这个 Pod 异常？',
  '最可能的根因是什么？',
  '哪些证据支持这个结论？',
  '影响了哪些服务？',
  '下一步应该怎么处理？',
]

const toolIconMap: Record<string, any> = {
  get_incident: AlertOutlined,
  get_rca: ExperimentOutlined,
  get_alerts: WarningOutlined,
  get_anomalies: BarChartOutlined,
  query_metrics: BarChartOutlined,
  search_logs: FileTextOutlined,
  get_k8s_resource: CloudOutlined,
  get_k8s_events: AlertOutlined,
  get_topology: ApartmentOutlined,
}

const toolLabelMap: Record<string, string> = {
  get_incident: '查询 Incident',
  get_rca: '查询 RCA',
  get_alerts: '查询告警',
  get_anomalies: '查询异常',
  query_metrics: '查询指标',
  search_logs: '检索日志',
  get_k8s_resource: '查询 Kubernetes',
  get_k8s_events: '查询事件',
  get_topology: '查询拓扑',
}

export default function AIAssistant() {
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()
  const incidentId = searchParams.get('incident_id') ? Number(searchParams.get('incident_id')) : undefined

  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [input, setInput] = useState('')
  const [loading, setLoading] = useState(false)
  const [aiEnabled, setAiEnabled] = useState<boolean | null>(null)
  const scrollRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight
    }
  }, [messages])

  const sendMessage = async (question: string) => {
    if (!question.trim() || loading) return

    const userMsg: ChatMessage = {
      id: Date.now().toString(),
      role: 'user',
      content: question,
      timestamp: Date.now(),
    }
    const assistantMsg: ChatMessage = {
      id: (Date.now() + 1).toString(),
      role: 'assistant',
      content: '',
      timestamp: Date.now(),
      loading: true,
    }
    setMessages((prev) => [...prev, userMsg, assistantMsg])
    setInput('')
    setLoading(true)

    try {
      const res: AIAskResponse = await aiApi.ask(question, incidentId)
      setAiEnabled(true)
      setMessages((prev) =>
        prev.map((m) =>
          m.id === assistantMsg.id
            ? {
                ...m,
                loading: false,
                content: res.answer || '分析完成',
                summary: res.summary,
                root_cause: res.root_cause,
                confidence: res.confidence,
                evidence: res.evidence,
                recommendations: res.recommendations,
                tool_calls: res.tool_calls,
              }
            : m,
        ),
      )
    } catch (err: any) {
      const msg = err?.message || 'AI 助手请求失败'
      if (msg.includes('未启用') || msg.includes('disabled')) {
        setAiEnabled(false)
      }
      setMessages((prev) =>
        prev.map((m) =>
          m.id === assistantMsg.id
            ? { ...m, loading: false, error: msg, content: '' }
            : m,
        ),
      )
    } finally {
      setLoading(false)
    }
  }

  const renderToolActivity = (toolCalls?: AIToolCall[]) => {
    if (!toolCalls || toolCalls.length === 0) return null
    return (
      <div style={{ marginBottom: 8 }}>
        <Divider style={{ margin: '8px 0' }} />
        <Text strong><DatabaseOutlined style={{ color: '#1890ff' }} /> 数据查询</Text>
        <Timeline
          style={{ marginTop: 8 }}
          items={toolCalls.map((tc) => {
            const Icon = toolIconMap[tc.tool_name] || DatabaseOutlined
            const label = toolLabelMap[tc.tool_name] || tc.tool_name
            let color = 'green'
            let dot = <CheckCircleOutlined style={{ color: '#52c41a' }} />
            if (!tc.result.success) {
              color = 'red'
              dot = <CloseCircleOutlined style={{ color: '#ff4d4f' }} />
            } else if (!tc.result.available) {
              color = 'orange'
              dot = <WarningOutlined style={{ color: '#faad14' }} />
            }
            return {
              color,
              dot,
              children: (
                <div>
                  <Space size={4}>
                    <Icon />
                    <Text style={{ fontSize: 12 }}>{label}</Text>
                    <Tag style={{ fontSize: 10 }}>{tc.result.source}</Tag>
                    {tc.duration_ms > 0 && <Text type="secondary" style={{ fontSize: 11 }}>{tc.duration_ms}ms</Text>}
                  </Space>
                  {!tc.result.available && tc.result.error && (
                    <div style={{ fontSize: 11, color: '#faad14', marginTop: 2 }}>{tc.result.error}</div>
                  )}
                </div>
              ),
            }
          })}
        />
      </div>
    )
  }

  const renderMessage = (msg: ChatMessage) => {
    if (msg.role === 'user') {
      return (
        <div style={{ display: 'flex', justifyContent: 'flex-end', marginBottom: 16 }}>
          <div style={{ maxWidth: '70%' }}>
            <div style={{ background: '#1677ff', color: '#fff', padding: '10px 14px', borderRadius: '12px 12px 2px 12px' }}>
              <Text style={{ color: '#fff' }}>{msg.content}</Text>
            </div>
          </div>
          <Avatar icon={<UserOutlined />} style={{ marginLeft: 8, backgroundColor: '#1677ff' }} />
        </div>
      )
    }

    return (
      <div style={{ display: 'flex', marginBottom: 16 }}>
        <Avatar icon={<RobotOutlined />} style={{ marginRight: 8, backgroundColor: '#722ed1' }} />
        <div style={{ maxWidth: '85%', flex: 1 }}>
          {msg.loading ? (
            <Card size="small" style={{ borderRadius: '2px 12px 12px 12px' }}>
              <Spin size="small" /> <Text type="secondary">AI 正在分析，正在查询数据...</Text>
            </Card>
          ) : msg.error ? (
            <Alert
              message="AI 助手不可用"
              description={msg.error}
              type="warning"
              showIcon
              style={{ borderRadius: '2px 12px 12px 12px' }}
            />
          ) : (
            <Card size="small" style={{ borderRadius: '2px 12px 12px 12px' }}>
              {renderToolActivity(msg.tool_calls)}

              {msg.summary && (
                <div style={{ marginBottom: 8 }}>
                  <Text strong><ThunderboltOutlined style={{ color: '#722ed1' }} /> 摘要</Text>
                  <div style={{ fontSize: 13, marginTop: 4 }}>{msg.summary}</div>
                </div>
              )}

              <Paragraph style={{ marginBottom: 8, whiteSpace: 'pre-wrap' }}>{msg.content}</Paragraph>

              {msg.root_cause && (
                <div style={{ marginBottom: 8, padding: 8, background: '#fff7e6', borderRadius: 4 }}>
                  <Text strong style={{ color: '#d46b08' }}>根因: </Text>
                  <Text>{msg.root_cause}</Text>
                  {msg.confidence !== undefined && (
                    <Tag color={msg.confidence > 0.7 ? 'green' : msg.confidence > 0.4 ? 'orange' : 'red'} style={{ marginLeft: 8 }}>
                      置信度 {(msg.confidence * 100).toFixed(0)}%
                    </Tag>
                  )}
                </div>
              )}

              {msg.evidence && msg.evidence.length > 0 && (
                <div style={{ marginBottom: 8 }}>
                  <Divider style={{ margin: '8px 0' }} />
                  <Text strong><CheckCircleOutlined style={{ color: '#52c41a' }} /> 证据 ({msg.evidence.length})</Text>
                  <List
                    size="small"
                    style={{ marginTop: 4 }}
                    dataSource={msg.evidence}
                    renderItem={(item, i) => (
                      <List.Item>
                        <Space size={4}>
                          <Tag color="blue" style={{ fontSize: 10 }}>{item.source}</Tag>
                          <Text style={{ fontSize: 12 }}>{i + 1}. {item.description}</Text>
                        </Space>
                      </List.Item>
                    )}
                  />
                </div>
              )}

              {msg.recommendations && msg.recommendations.length > 0 && (
                <div>
                  <Divider style={{ margin: '8px 0' }} />
                  <Text strong><BulbOutlined style={{ color: '#faad14' }} /> 建议</Text>
                  <List
                    size="small"
                    style={{ marginTop: 4 }}
                    dataSource={msg.recommendations}
                    renderItem={(item, i) => (
                      <List.Item>
                        <Space size={4}>
                          <Tag color={item.priority === 'P0' ? 'red' : item.priority === 'P1' ? 'orange' : 'blue'} style={{ fontSize: 10 }}>
                            {item.priority}
                          </Tag>
                          <Text style={{ fontSize: 12 }}>{i + 1}. {item.title}</Text>
                          <Tag color={item.risk === 'high' || item.risk === 'critical' ? 'red' : item.risk === 'medium' ? 'orange' : 'green'} style={{ fontSize: 10 }}>
                            风险: {item.risk}
                          </Tag>
                        </Space>
                      </List.Item>
                    )}
                  />
                </div>
              )}
            </Card>
          )}
        </div>
      </div>
    )
  }

  return (
    <div style={{ height: 'calc(100vh - 140px)', display: 'flex', flexDirection: 'column' }}>
      <Space style={{ marginBottom: 16 }} align="center">
        <ThunderboltOutlined style={{ fontSize: 20, color: '#722ed1' }} />
        <Text strong style={{ fontSize: 18 }}>AI 运维助手</Text>
        {aiEnabled === false && <Tag color="warning">未启用</Tag>}
        {aiEnabled === true && <Tag color="success">已连接</Tag>}
        {incidentId && (
          <Badge status="processing" color="#722ed1" text={`Incident #${incidentId}`} />
        )}
        {incidentId && (
          <Button size="small" type="link" onClick={() => navigate('/aiops/incidents')}>
            返回事件列表
          </Button>
        )}
      </Space>

      {aiEnabled === false && (
        <Alert
          message="AI 助手未启用"
          description="请在后端配置中设置 ai.enabled=true 并配置 LLM Provider（OpenAI / Azure / Ollama 等）。当前可查看界面交互，提问将返回错误提示。"
          type="info"
          showIcon
          style={{ marginBottom: 16 }}
        />
      )}

      <div
        ref={scrollRef}
        style={{
          flex: 1,
          overflow: 'auto',
          padding: 16,
          background: 'rgba(0,0,0,0.02)',
          borderRadius: 8,
          marginBottom: 16,
        }}
      >
        {messages.length === 0 ? (
          <div style={{ textAlign: 'center', paddingTop: 40 }}>
            <RobotOutlined style={{ fontSize: 48, color: '#722ed1', marginBottom: 16 }} />
            <Title level={4}>AIOps 智能运维助手</Title>
            <Text type="secondary">
              {incidentId
                ? `正在分析 Incident #${incidentId}，AI 将自动查询相关数据`
                : '输入运维问题，AI 将结合监控、日志、告警、Kubernetes 进行分析'}
            </Text>
            <div style={{ marginTop: 24 }}>
              <Text type="secondary" style={{ display: 'block', marginBottom: 8 }}>快速提问：</Text>
              <Space wrap>
                {quickQuestions.map((q) => (
                  <Tag
                    key={q}
                    color="blue"
                    style={{ cursor: 'pointer', padding: '4px 12px' }}
                    onClick={() => sendMessage(q)}
                  >
                    {q}
                  </Tag>
                ))}
              </Space>
            </div>
          </div>
        ) : (
          messages.map(renderMessage)
        )}
      </div>

      <Card size="small" style={{ borderRadius: 8 }}>
        <Space.Compact style={{ width: '100%' }}>
          <TextArea
            value={input}
            onChange={(e) => setInput(e.target.value)}
            placeholder={incidentId ? `针对 Incident #${incidentId} 提问...` : '输入运维问题，例如：为什么 order-service 最近错误率升高？'}
            rows={2}
            onPressEnter={(e) => {
              if (!e.shiftKey) {
                e.preventDefault()
                sendMessage(input)
              }
            }}
            style={{ borderRadius: '6px 0 0 6px' }}
          />
          <Button
            type="primary"
            icon={<SendOutlined />}
            onClick={() => sendMessage(input)}
            loading={loading}
            disabled={!input.trim()}
            style={{ height: '100%', borderRadius: '0 6px 6px 0' }}
          >
            发送
          </Button>
        </Space.Compact>
        <Text type="secondary" style={{ fontSize: 12 }}>
          Enter 发送，Shift+Enter 换行 | AI 仅执行只读查询，不自动执行任何写操作
        </Text>
      </Card>
    </div>
  )
}
