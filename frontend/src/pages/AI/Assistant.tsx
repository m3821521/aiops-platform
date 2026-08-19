import { useState, useRef, useEffect } from 'react'
import {
  Card, Input, Button, Space, Typography, Spin, Alert, Tag, List, Avatar, Divider,
} from 'antd'
import {
  RobotOutlined, UserOutlined, SendOutlined, ThunderboltOutlined,
  BulbOutlined, CheckCircleOutlined,
} from '@ant-design/icons'
import { aiApi } from '@/api/ai'

const { Text, Paragraph, Title } = Typography
const { TextArea } = Input

interface ChatMessage {
  id: string
  role: 'user' | 'assistant'
  content: string
  evidence?: { description: string; metric?: string; value?: string }[]
  suggestions?: string[]
  timestamp: number
  loading?: boolean
  error?: string
}

const quickQuestions = [
  '当前集群有哪些异常？',
  '为什么 order-service 错误率升高？',
  '分析最近 10 分钟的告警',
  '哪个 Pod 重启次数最多？',
]

export default function AIAssistant() {
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
      const res = await aiApi.ask(question)
      setAiEnabled(true)
      setMessages((prev) =>
        prev.map((m) =>
          m.id === assistantMsg.id
            ? {
                ...m,
                loading: false,
                content: res.answer || '分析完成',
                evidence: res.evidence,
                suggestions: res.suggestions,
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
        <div style={{ maxWidth: '80%', flex: 1 }}>
          {msg.loading ? (
            <Card size="small" style={{ borderRadius: '2px 12px 12px 12px' }}>
              <Spin size="small" /> <Text type="secondary">AI 正在分析...</Text>
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
              <Paragraph style={{ marginBottom: 8, whiteSpace: 'pre-wrap' }}>{msg.content}</Paragraph>

              {msg.evidence && msg.evidence.length > 0 && (
                <div style={{ marginBottom: 8 }}>
                  <Divider style={{ margin: '8px 0' }} />
                  <Text strong><CheckCircleOutlined style={{ color: '#52c41a' }} /> 证据</Text>
                  <List
                    size="small"
                    dataSource={msg.evidence}
                    renderItem={(item, i) => (
                      <List.Item>
                        <Text>{i + 1}. {item.description}</Text>
                        {item.metric && <Tag style={{ marginLeft: 8 }}>{item.metric}: {item.value}</Tag>}
                      </List.Item>
                    )}
                  />
                </div>
              )}

              {msg.suggestions && msg.suggestions.length > 0 && (
                <div>
                  <Divider style={{ margin: '8px 0' }} />
                  <Text strong><BulbOutlined style={{ color: '#faad14' }} /> 建议</Text>
                  <List
                    size="small"
                    dataSource={msg.suggestions}
                    renderItem={(item, i) => (
                      <List.Item>
                        <Text>{i + 1}. {item}</Text>
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
      <Space style={{ marginBottom: 16 }}>
        <ThunderboltOutlined style={{ fontSize: 20, color: '#722ed1' }} />
        <Text strong style={{ fontSize: 18 }}>AI 运维助手</Text>
        {aiEnabled === false && <Tag color="warning">未启用</Tag>}
        {aiEnabled === true && <Tag color="success">已连接</Tag>}
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
          <div style={{ textAlign: 'center', paddingTop: 60 }}>
            <RobotOutlined style={{ fontSize: 48, color: '#722ed1', marginBottom: 16 }} />
            <Title level={4}>AIOps 智能运维助手</Title>
            <Text type="secondary">输入运维问题，AI 将结合监控、日志、告警进行分析</Text>
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
            placeholder="输入运维问题，例如：为什么 order-service 最近错误率升高？"
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
          Enter 发送，Shift+Enter 换行
        </Text>
      </Card>
    </div>
  )
}
