import { useState, useRef, useEffect } from 'react'
import { useSearchParams, useNavigate } from 'react-router-dom'
import {
  Card, Input, Button, Space, Typography, Spin, Alert, Tag, List, Avatar, Divider,
  Timeline, Badge, Drawer, Empty, Popconfirm, Modal, Form, message,
} from 'antd'
import {
  RobotOutlined, UserOutlined, SendOutlined, ThunderboltOutlined,
  BulbOutlined, CheckCircleOutlined, WarningOutlined, CloseCircleOutlined,
  ExperimentOutlined, DatabaseOutlined, CloudOutlined, AlertOutlined,
  BarChartOutlined, FileTextOutlined, ApartmentOutlined, PlusOutlined,
  DeleteOutlined, HistoryOutlined, MessageOutlined, SettingOutlined,
} from '@ant-design/icons'
import { aiApi, type AIAskResponse, type AIToolCall, type AIConversation, type AIConfigResponse, type AIAskRecommendation } from '@/api/ai'
import { automationApi } from '@/api/automation'

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
  recommendations?: AIAskRecommendation[]
  tool_calls?: AIToolCall[]
  timestamp: number
  loading?: boolean
  error?: string
}

const quickQuestions = [
  '现在系统有什么异常？',
  '生产环境有哪些严重告警？',
  '最近 30 分钟有哪些异常 Pod？',
  '最可能的根因是什么？',
  '下一步应该怎么处理？',
]

/**
 * Recommendation Router - AI Recommendation 类型路由。
 *
 * 明确区分：
 * - Investigation / Analysis Action (investigate, network_check)
 * - Monitoring / Observation (observe)
 * - Automation Action (restart → restart_pod, scale → scale_deployment)
 * - Unsupported (rollback, config_change)
 *
 * 禁止将 investigate/observe 等非 Automation 类型直接传给 automationApi.create()。
 */
type RecommendationRoute =
  | { kind: 'automation'; actionType: string; targetType: string }
  | { kind: 'investigation'; message: string }
  | { kind: 'monitoring'; message: string }
  | { kind: 'unsupported'; message: string }

function routeRecommendation(rec: AIAskRecommendation): RecommendationRoute {
  const actionType = rec.action_type || ''

  // Automation 类型映射
  // AI 使用短名 (restart, scale)，Automation 使用长名 (restart_pod, scale_deployment)
  if (actionType === 'restart') {
    return { kind: 'automation', actionType: 'restart_pod', targetType: 'pod' }
  }
  if (actionType === 'scale' || actionType.includes('deployment')) {
    return { kind: 'automation', actionType: 'scale_deployment', targetType: 'deployment' }
  }

  // jenkins_build 和 argocd_sync 直接支持
  if (actionType === 'jenkins_build' || actionType === 'argocd_sync') {
    return { kind: 'automation', actionType, targetType: 'service' }
  }

  // Investigation 类型 - 不创建 Automation Action
  if (actionType === 'investigate') {
    return {
      kind: 'investigation',
      message: '该建议属于调查分析类型，请进入 Incident Detail 查看 RCA / Investigation',
    }
  }
  if (actionType === 'network_check') {
    return {
      kind: 'investigation',
      message: '该建议属于网络调查类型，暂不支持自动执行，请手动排查',
    }
  }

  // Monitoring 类型 - 不创建 Automation Action
  if (actionType === 'observe') {
    return {
      kind: 'monitoring',
      message: '该建议属于监控观察类型，请进入 Monitoring 页面查看指标',
    }
  }

  // 暂不支持的类型
  if (actionType === 'rollback') {
    return {
      kind: 'unsupported',
      message: '回滚操作暂不支持自动执行，请通过 Jenkins / ArgoCD 手动执行',
    }
  }
  if (actionType === 'config_change') {
    return {
      kind: 'unsupported',
      message: '配置变更暂不支持自动执行，请手动修改配置',
    }
  }

  // 未知类型 - 默认不支持
  return {
    kind: 'unsupported',
    message: `不支持的操作类型: ${actionType}，支持的 Automation 类型: restart_pod, scale_deployment, jenkins_build, argocd_sync`,
  }
}

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

const riskColorMap: Record<string, string> = {
  low: 'green',
  medium: 'gold',
  high: 'orange',
  critical: 'red',
}

const priorityColorMap: Record<string, string> = {
  P0: 'red',
  P1: 'orange',
  P2: 'gold',
  P3: 'blue',
}

export default function AIAssistant() {
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()
  const incidentId = searchParams.get('incident_id') ? Number(searchParams.get('incident_id')) : undefined

  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [input, setInput] = useState('')
  const [loading, setLoading] = useState(false)
  const [conversationId, setConversationId] = useState<number | undefined>(undefined)
  const [conversations, setConversations] = useState<AIConversation[]>([])
  const [drawerOpen, setDrawerOpen] = useState(false)
  const [aiEnabled, setAiEnabled] = useState<boolean | null>(null)
  const [configModalOpen, setConfigModalOpen] = useState(false)
  const [configLoading, setConfigLoading] = useState(false)
  const [aiConfig, setAiConfig] = useState<AIConfigResponse | null>(null)
  const [configForm] = Form.useForm()
  const scrollRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    loadConversations()
    loadAIConfig()
    // 检查 AI 是否启用
    aiApi.ask('ping').then(() => setAiEnabled(true)).catch(() => setAiEnabled(false))
  }, [])

  const loadAIConfig = async () => {
    try {
      const res = await aiApi.getConfig()
      setAiConfig(res)
    } catch (e) {
      // 静默失败
    }
  }

  const handleSaveConfig = async () => {
    try {
      const values = await configForm.validateFields()
      setConfigLoading(true)
      const res = await aiApi.updateConfig({
        api_key: values.api_key,
        model: values.model,
        base_url: values.base_url,
        provider: values.provider || 'openai',
      })
      setAiConfig(res)
      setAiEnabled(true)
      message.success('AI 配置已保存')
      setConfigModalOpen(false)
      configForm.resetFields()
      // 重新检查 AI 是否可用
      aiApi.ask('ping').then(() => setAiEnabled(true)).catch(() => setAiEnabled(false))
    } catch (e: any) {
      if (e.errorFields) return // 表单验证错误
      message.error('保存配置失败: ' + (e?.response?.data?.message || e.message))
    } finally {
      setConfigLoading(false)
    }
  }

  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight
    }
  }, [messages])

  // 从 AI 推荐创建 Automation Action
  // 使用 routeRecommendation 进行类型路由，禁止将 investigate/observe 等非 Automation 类型直接创建 Action
  const handleCreateActionFromRecommendation = async (rec: AIAskRecommendation) => {
    try {
      // 类型路由：区分 Automation / Investigation / Monitoring / Unsupported
      const route = routeRecommendation(rec)

      if (route.kind === 'automation') {
        // Automation 类型：创建 Action
        const action = await automationApi.create({
          action_type: route.actionType,
          target_type: route.targetType,
          target_name: rec.target || '',
          cluster: 'local',
          namespace: rec.namespace,
          parameters: rec.parameters || {},
          reason: rec.reason || rec.title,
          risk: rec.risk || 'medium',
        })

        message.success(`Action #${action.id} 已创建，等待审批`)
        // 跳转到 Automation 页面
        navigate('/automation/actions')
        return
      }

      if (route.kind === 'investigation') {
        // Investigation 类型：不创建 Action，提示用户进入 Incident Detail
        message.info(route.message)
        // 如果有 incident_id，跳转到 Incident Detail
        if (rec.incident_id) {
          navigate('/aiops/incidents')
        }
        return
      }

      if (route.kind === 'monitoring') {
        // Monitoring 类型：不创建 Action，跳转到 Monitoring 页面
        message.info(route.message)
        navigate('/observability/metrics')
        return
      }

      // Unsupported 类型：显示提示，不创建 Action
      message.warning(route.message)
    } catch (e: any) {
      message.error('创建 Action 失败: ' + (e?.response?.data?.message || e.message))
    }
  }

  const loadConversations = async () => {
    try {
      const res = await aiApi.listConversations(1, 50)
      setConversations(res.items || [])
    } catch (e) {
      // 静默失败
    }
  }

  const newConversation = () => {
    setMessages([])
    setConversationId(undefined)
    setDrawerOpen(false)
  }

  const loadConversation = async (id: number) => {
    try {
      const res = await aiApi.getConversation(id)
      setConversationId(id)
      const msgs: ChatMessage[] = (res.messages || []).map((m) => ({
        id: m.id.toString(),
        role: m.role,
        content: m.content,
        summary: m.summary,
        root_cause: m.root_cause,
        confidence: m.confidence,
        timestamp: new Date(m.created_at).getTime(),
      }))
      setMessages(msgs)
      setDrawerOpen(false)
    } catch (e) {
      // 静默失败
    }
  }

  const deleteConversation = async (id: number) => {
    try {
      await aiApi.deleteConversation(id)
      if (conversationId === id) {
        newConversation()
      }
      loadConversations()
    } catch (e) {
      // 静默失败
    }
  }

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
      const res = await aiApi.ask(question, incidentId, conversationId)
      if (res.conversation_id) {
        setConversationId(res.conversation_id)
        loadConversations()
      }
      setMessages((prev) =>
        prev.map((m) =>
          m.id === assistantMsg.id
            ? {
                ...m,
                loading: false,
                content: res.answer,
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
      const errorMsg = err?.response?.data?.message || err?.message || 'AI 请求失败'
      setMessages((prev) =>
        prev.map((m) =>
          m.id === assistantMsg.id
            ? { ...m, loading: false, error: errorMsg }
            : m,
        ),
      )
    } finally {
      setLoading(false)
    }
  }

  const renderEvidence = (evidence: ChatMessage['evidence']) => {
    if (!evidence || evidence.length === 0) return null
    return (
      <div style={{ marginTop: 12 }}>
        <Text strong style={{ fontSize: 13, color: 'var(--text-secondary)' }}>
          <DatabaseOutlined style={{ marginRight: 6 }} />证据
        </Text>
        <div style={{ marginTop: 8, display: 'flex', flexDirection: 'column', gap: 6 }}>
          {evidence.map((e, i) => (
            <div
              key={i}
              style={{
                padding: '8px 12px',
                background: 'var(--bg-surface-hover)',
                borderRadius: 6,
                borderLeft: '3px solid var(--color-primary)',
                fontSize: 13,
              }}
            >
              <Tag color="blue" style={{ marginRight: 8 }}>{e.source}</Tag>
              <span>{e.description}</span>
              {e.resource && <Text type="secondary" style={{ marginLeft: 8 }}>({e.resource})</Text>}
            </div>
          ))}
        </div>
      </div>
    )
  }

  const renderRecommendations = (recs: ChatMessage['recommendations']) => {
    if (!recs || recs.length === 0) return null
    return (
      <div style={{ marginTop: 12 }}>
        <Text strong style={{ fontSize: 13, color: 'var(--text-secondary)' }}>
          <BulbOutlined style={{ marginRight: 6 }} />建议操作
        </Text>
        <div style={{ marginTop: 8, display: 'flex', flexDirection: 'column', gap: 8 }}>
          {recs.map((r, i) => {
            // 使用 routeRecommendation 进行类型路由
            const route = routeRecommendation(r)
            // 按钮文字和图标根据类型显示
            let buttonText = ''
            let buttonIcon: React.ReactNode = null
            let showButton = false

            if (route.kind === 'automation') {
              buttonText = '创建 Action'
              buttonIcon = <PlusOutlined />
              showButton = true
            } else if (route.kind === 'investigation') {
              buttonText = '查看分析'
              buttonIcon = <FileTextOutlined />
              showButton = true
            } else if (route.kind === 'monitoring') {
              buttonText = '查看监控'
              buttonIcon = <BarChartOutlined />
              showButton = true
            }
            // unsupported 类型不显示按钮

            return (
              <div
                key={i}
                style={{
                  padding: '10px 12px',
                  background: 'var(--color-primary-light)',
                  borderRadius: 6,
                  border: '1px solid var(--color-primary-border)',
                  fontSize: 13,
                }}
              >
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: r.description ? 4 : 0 }}>
                  <span>
                    <Tag color={priorityColorMap[r.priority] || 'default'} style={{ marginRight: 8 }}>
                      {r.priority}
                    </Tag>
                    <strong>{r.title}</strong>
                    {r.target && (
                      <Tag color="blue" style={{ marginLeft: 8 }}>
                        {r.target}
                      </Tag>
                    )}
                    {/* 显示操作类型标签 */}
                    <Tag
                      color={
                        route.kind === 'automation' ? 'green' :
                        route.kind === 'investigation' ? 'orange' :
                        route.kind === 'monitoring' ? 'blue' : 'default'
                      }
                      style={{ marginLeft: 8 }}
                    >
                      {route.kind === 'automation' ? '自动化' :
                       route.kind === 'investigation' ? '调查' :
                       route.kind === 'monitoring' ? '监控' : '暂不支持'}
                    </Tag>
                  </span>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                    <Tag color={riskColorMap[r.risk] || 'default'}>风险: {r.risk}</Tag>
                    {showButton && (
                      <Button
                        type="primary"
                        size="small"
                        icon={buttonIcon}
                        onClick={() => handleCreateActionFromRecommendation(r)}
                      >
                        {buttonText}
                      </Button>
                    )}
                  </div>
                </div>
                {r.description && (
                  <div style={{ color: 'var(--text-secondary)', fontSize: 12, marginTop: 4 }}>
                    {r.description}
                  </div>
                )}
                {r.reason && (
                  <div style={{ color: 'var(--text-muted)', fontSize: 11, marginTop: 2 }}>
                    原因: {r.reason}
                  </div>
                )}
              </div>
            )
          })}
        </div>
      </div>
    )
  }

  const renderToolCalls = (calls: AIToolCall[]) => {
    if (!calls || calls.length === 0) return null
    return (
      <div style={{ marginTop: 12 }}>
        <Text strong style={{ fontSize: 13, color: 'var(--text-secondary)' }}>
          <ThunderboltOutlined style={{ marginRight: 6 }} />工具调用
        </Text>
        <div style={{ marginTop: 8, display: 'flex', flexWrap: 'wrap', gap: 6 }}>
          {calls.map((c, i) => {
            const Icon = toolIconMap[c.tool_name] || DatabaseOutlined
            return (
              <Tag
                key={i}
                icon={<Icon />}
                color={c.result.success ? 'success' : 'error'}
                style={{ fontSize: 12 }}
              >
                {toolLabelMap[c.tool_name] || c.tool_name}
                {c.result.available === false && ' (不可用)'}
              </Tag>
            )
          })}
        </div>
      </div>
    )
  }

  const renderMessage = (msg: ChatMessage) => {
    if (msg.role === 'user') {
      return (
        <div key={msg.id} style={{ display: 'flex', justifyContent: 'flex-end', marginBottom: 16 }}>
          <div style={{ maxWidth: '70%', display: 'flex', alignItems: 'flex-start', gap: 8 }}>
            <div
              style={{
                background: 'var(--color-primary)',
                color: '#fff',
                padding: '10px 14px',
                borderRadius: '12px 12px 2px 12px',
                fontSize: 14,
                lineHeight: 1.6,
              }}
            >
              {msg.content}
            </div>
            <Avatar icon={<UserOutlined />} size="small" style={{ background: 'var(--color-primary)' }} />
          </div>
        </div>
      )
    }

    return (
      <div key={msg.id} style={{ display: 'flex', justifyContent: 'flex-start', marginBottom: 16 }}>
        <div style={{ maxWidth: '85%', display: 'flex', alignItems: 'flex-start', gap: 8 }}>
          <Avatar icon={<RobotOutlined />} size="small" style={{ background: 'var(--color-ai)' }} />
          <div
            style={{
              background: 'var(--bg-surface)',
              border: '1px solid var(--border-color)',
              padding: '12px 16px',
              borderRadius: '2px 12px 12px 12px',
              fontSize: 14,
              lineHeight: 1.7,
              minWidth: 200,
            }}
          >
            {msg.loading ? (
              <div style={{ display: 'flex', alignItems: 'center', gap: 8, color: 'var(--text-muted)' }}>
                <Spin size="small" />
                <span>AI 正在分析...</span>
              </div>
            ) : msg.error ? (
              <Alert message={msg.error} type="error" showIcon style={{ marginTop: 0 }} />
            ) : (
              <>
                {msg.summary && (
                  <div style={{ marginBottom: 8, padding: '8px 12px', background: 'var(--color-ai-light)', borderRadius: 6, borderLeft: '3px solid var(--color-ai)' }}>
                    <Text strong style={{ color: 'var(--color-ai)' }}>摘要</Text>
                    <div style={{ marginTop: 4, fontSize: 13 }}>{msg.summary}</div>
                  </div>
                )}
                {msg.root_cause && (
                  <div style={{ marginBottom: 8 }}>
                    <Text strong style={{ color: 'var(--color-danger)' }}>根因: </Text>
                    <span>{msg.root_cause}</span>
                    {msg.confidence != null && (
                      <Tag color="blue" style={{ marginLeft: 8 }}>置信度 {Math.round(msg.confidence * 100)}%</Tag>
                    )}
                  </div>
                )}
                <div style={{ whiteSpace: 'pre-wrap' }}>{msg.content}</div>
                {renderToolCalls(msg.tool_calls || [])}
                {renderEvidence(msg.evidence)}
                {renderRecommendations(msg.recommendations)}
              </>
            )}
          </div>
        </div>
      </div>
    )
  }

  return (
    <div style={{ display: 'flex', height: 'calc(100vh - 64px)', background: 'var(--bg-app)' }}>
      {/* 侧边栏 - 历史对话 */}
      <div
        style={{
          width: 260,
          background: 'var(--bg-sidebar)',
          borderRight: '1px solid var(--border-sidebar)',
          display: 'flex',
          flexDirection: 'column',
          flexShrink: 0,
        }}
      >
        <div style={{ padding: 16, borderBottom: '1px solid var(--border-color)' }}>
          <Button
            type="primary"
            icon={<PlusOutlined />}
            block
            onClick={newConversation}
            style={{ marginBottom: 12 }}
          >
            新建对话
          </Button>
          <Button
            icon={<HistoryOutlined />}
            block
            onClick={() => setDrawerOpen(true)}
          >
            历史对话 ({conversations.length})
          </Button>
        </div>
        <div style={{ flex: 1, overflow: 'auto', padding: '8px 0' }}>
          {conversations.length === 0 ? (
            <Empty description="暂无对话" style={{ marginTop: 40 }} />
          ) : (
            conversations.map((conv) => (
              <div
                key={conv.id}
                onClick={() => loadConversation(conv.id)}
                style={{
                  padding: '10px 16px',
                  cursor: 'pointer',
                  background: conversationId === conv.id ? 'var(--color-primary-light)' : 'transparent',
                  borderLeft: conversationId === conv.id ? '3px solid var(--color-primary)' : '3px solid transparent',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'space-between',
                }}
                onMouseEnter={(e) => (e.currentTarget.style.background = 'var(--bg-sidebar-hover)')}
                onMouseLeave={(e) => (e.currentTarget.style.background = conversationId === conv.id ? 'var(--color-primary-light)' : 'transparent')}
              >
                <div style={{ flex: 1, minWidth: 0 }}>
                  <div style={{ fontSize: 13, fontWeight: 500, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                    <MessageOutlined style={{ marginRight: 6, color: 'var(--text-muted)' }} />
                    {conv.title || '新对话'}
                  </div>
                  <div style={{ fontSize: 11, color: 'var(--text-muted)', marginTop: 2 }}>
                    {conv.message_count} 条消息 · {new Date(conv.updated_at).toLocaleDateString()}
                  </div>
                </div>
                <Popconfirm
                  title="删除此对话？"
                  onConfirm={(e) => { e?.stopPropagation(); deleteConversation(conv.id) }}
                  onCancel={(e) => e?.stopPropagation()}
                  okText="删除"
                  cancelText="取消"
                >
                  <DeleteOutlined
                    style={{ color: 'var(--text-muted)', fontSize: 14 }}
                    onClick={(e) => e.stopPropagation()}
                  />
                </Popconfirm>
              </div>
            ))
          )}
        </div>
      </div>

      {/* 主聊天区域 */}
      <div style={{ flex: 1, display: 'flex', flexDirection: 'column', minWidth: 0 }}>
        {/* 头部 */}
        <div
          style={{
            padding: '16px 24px',
            background: 'var(--bg-header)',
            borderBottom: '1px solid var(--border-color)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
          }}
        >
          <div>
            <Title level={4} style={{ margin: 0, fontSize: 18 }}>
              <RobotOutlined style={{ color: 'var(--color-ai)', marginRight: 8 }} />
              AI 运维助手
            </Title>
            <Text type="secondary" style={{ fontSize: 12 }}>
              询问关于 Incident、服务、Pod、指标、日志等问题
            </Text>
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            {aiConfig && (
              <Tag color={aiConfig.api_key_set ? 'success' : 'warning'} style={{ margin: 0 }}>
                {aiConfig.api_key_set ? 'API Key 已配置' : '未配置 API Key'}
              </Tag>
            )}
            <Button
              type="text"
              icon={<SettingOutlined />}
              onClick={() => {
                configForm.setFieldsValue({
                  provider: aiConfig?.provider || 'openai',
                  base_url: aiConfig?.base_url || 'https://api.openai.com/v1',
                  model: aiConfig?.model || 'gpt-4o-mini',
                })
                setConfigModalOpen(true)
              }}
            >
              配置
            </Button>
            {incidentId && (
              <Tag color="blue" icon={<AlertOutlined />}>
                Incident #{incidentId}
              </Tag>
            )}
          </div>
        </div>

        {/* 消息区域 */}
        <div ref={scrollRef} style={{ flex: 1, overflow: 'auto', padding: '24px', background: 'var(--bg-app)' }}>
          {messages.length === 0 ? (
            <div style={{ textAlign: 'center', marginTop: 60 }}>
              <Avatar size={64} icon={<RobotOutlined />} style={{ background: 'var(--color-ai)', marginBottom: 16 }} />
              <Title level={4} style={{ marginBottom: 8 }}>AI 运维助手</Title>
              <Text type="secondary" style={{ display: 'block', marginBottom: 24 }}>
                我可以帮你分析系统异常、查询指标、检索日志、解释根因
              </Text>
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8, justifyContent: 'center', maxWidth: 600, margin: '0 auto' }}>
                {quickQuestions.map((q) => (
                  <Tag
                    key={q}
                    color="blue"
                    style={{ cursor: 'pointer', padding: '6px 12px', fontSize: 13 }}
                    onClick={() => sendMessage(q)}
                  >
                    {q}
                  </Tag>
                ))}
              </div>
            </div>
          ) : (
            messages.map(renderMessage)
          )}
        </div>

        {/* 输入区域 */}
        <div style={{ padding: '16px 24px', background: 'var(--bg-header)', borderTop: '1px solid var(--border-color)' }}>
          <div style={{ display: 'flex', gap: 12, alignItems: 'flex-end' }}>
            <TextArea
              value={input}
              onChange={(e) => setInput(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' && !e.shiftKey) {
                  e.preventDefault()
                  sendMessage(input)
                }
              }}
              placeholder="询问关于 Incident、服务、Pod、指标、日志等问题... (Enter 发送, Shift+Enter 换行)"
              autoSize={{ minRows: 1, maxRows: 4 }}
              style={{ flex: 1, borderRadius: 8 }}
              disabled={loading}
            />
            <Button
              type="primary"
              icon={<SendOutlined />}
              onClick={() => sendMessage(input)}
              loading={loading}
              style={{ height: 40, borderRadius: 8 }}
            >
              发送
            </Button>
          </div>
          <div style={{ marginTop: 8, textAlign: 'center' }}>
            <Text type="secondary" style={{ fontSize: 11 }}>
              AI 仅供参考，所有操作需人工确认。AI 不会自动执行任何生产操作。
            </Text>
          </div>
        </div>
      </div>

      {/* 历史对话 Drawer（移动端备用） */}
      <Drawer
        title="历史对话"
        placement="left"
        open={drawerOpen}
        onClose={() => setDrawerOpen(false)}
        width={300}
      >
        {conversations.length === 0 ? (
          <Empty description="暂无对话" />
        ) : (
          <List
            dataSource={conversations}
            renderItem={(item) => (
              <List.Item
                onClick={() => loadConversation(item.id)}
                style={{ cursor: 'pointer' }}
                actions={[
                  <Popconfirm
                    key="delete"
                    title="删除此对话？"
                    onConfirm={() => deleteConversation(item.id)}
                    okText="删除"
                    cancelText="取消"
                  >
                    <DeleteOutlined />
                  </Popconfirm>,
                ]}
              >
                <List.Item.Meta
                  avatar={<Avatar icon={<MessageOutlined />} size="small" />}
                  title={item.title || '新对话'}
                  description={`${item.message_count} 条消息`}
                />
              </List.Item>
            )}
          />
        )}
      </Drawer>

      {/* AI 配置弹窗 */}
      <Modal
        title="AI 配置"
        open={configModalOpen}
        onCancel={() => setConfigModalOpen(false)}
        onOk={handleSaveConfig}
        confirmLoading={configLoading}
        okText="保存"
        cancelText="取消"
        width={520}
      >
        <Alert
          message="API Key 会加密存储在数据库中，不会明文保存或返回给前端。"
          type="info"
          showIcon
          style={{ marginBottom: 16 }}
        />
        <Form form={configForm} layout="vertical">
          <Form.Item
            name="provider"
            label="AI 提供商"
            initialValue="openai"
          >
            <Input placeholder="openai / azure / ollama / custom" />
          </Form.Item>
          <Form.Item
            name="base_url"
            label="API 地址"
            initialValue="https://api.openai.com/v1"
            rules={[{ required: true, message: '请输入 API 地址' }]}
          >
            <Input placeholder="https://api.openai.com/v1" />
          </Form.Item>
          <Form.Item
            name="model"
            label="模型名称"
            initialValue="gpt-4o-mini"
            rules={[{ required: true, message: '请输入模型名称' }]}
          >
            <Input placeholder="gpt-4o-mini / gpt-4o / claude-3-5-sonnet" />
          </Form.Item>
          <Form.Item
            name="api_key"
            label="API Key"
            extra={aiConfig?.api_key_set ? `当前已配置: ${aiConfig.api_key_masked}（留空则不修改）` : '请输入 API Key（sk-xxxxxxxxxx）'}
            rules={[
              { min: 10, message: 'API Key 长度至少 10 位' },
            ]}
          >
            <Input.Password placeholder="sk-xxxxxxxxxxxxxxxxxxxxxxxx" autoComplete="off" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
