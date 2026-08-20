import { useState, useEffect } from 'react'
import {
  Card, Table, Button, Space, Tag, Modal, Form, Input, Select, message, Timeline, Badge,
  Descriptions, Row, Col, Progress, Empty, Spin, Alert,
} from 'antd'
import {
  RobotOutlined, PlayCircleOutlined, ReloadOutlined, EyeOutlined, CheckCircleOutlined,
  CloseCircleOutlined, ClockCircleOutlined, WarningOutlined, ThunderboltOutlined,
  ApiOutlined, SafetyCertificateOutlined,
} from '@ant-design/icons'
import request from '@/api/client'

const { TextArea } = Input
const { Option } = Select

interface AgentInfo {
  name: string
  type: string
  description: string
  capabilities: string[]
}

interface AgentTask {
  id: string
  title: string
  description: string
  agent_type: string
  agent_name: string
  status: string
  result?: any
  error?: string
  started_at?: string
  finished_at?: string
  duration_ms?: number
}

interface OrchestrationResult {
  task_id: string
  title: string
  status: string
  tasks: AgentTask[]
  summary: string
  final_report: string
  risk_assessment?: {
    level: string
    score: number
    description: string
    factors: string[]
    requires_approval: boolean
  }
  recommendations: any[]
  findings: any[]
  evidence: any[]
  created_at: string
  updated_at: string
  completed_at?: string
  duration_ms?: number
}

const agentTypeColorMap: Record<string, string> = {
  monitor: 'blue',
  diagnosis: 'cyan',
  executor: 'orange',
  verifier: 'green',
  reporter: 'purple',
  risk: 'red',
}

const statusColorMap: Record<string, string> = {
  pending: 'default',
  running: 'processing',
  success: 'success',
  failed: 'error',
  skipped: 'warning',
  cancelled: 'default',
}

const taskStatusColorMap: Record<string, string> = {
  pending: 'default',
  decomposed: 'blue',
  in_progress: 'processing',
  awaiting_approval: 'warning',
  approved: 'cyan',
  rejected: 'error',
  executing: 'processing',
  verifying: 'processing',
  completed: 'success',
  failed: 'error',
  cancelled: 'default',
}

const riskLevelColorMap: Record<string, string> = {
  low: 'success',
  medium: 'warning',
  high: 'orange',
  critical: 'error',
}

export default function MultiAgentOrchestration() {
  const [agents, setAgents] = useState<AgentInfo[]>([])
  const [results, setResults] = useState<OrchestrationResult[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const [createOpen, setCreateOpen] = useState(false)
  const [createLoading, setCreateLoading] = useState(false)
  const [detailOpen, setDetailOpen] = useState(false)
  const [currentResult, setCurrentResult] = useState<OrchestrationResult | null>(null)
  const [detailLoading, setDetailLoading] = useState(false)
  const [form] = Form.useForm()

  const loadAgents = async () => {
    try {
      const res = await request.get<any, AgentInfo[]>('/api/v1/agents')
      setAgents(res || [])
    } catch (e: any) {
      message.error('加载 Agent 列表失败')
    }
  }

  const loadResults = async () => {
    setLoading(true)
    try {
      const res = await request.get<any, { items: OrchestrationResult[]; total: number; page: number; page_size: number }>(
        '/api/v1/agent/results',
        { params: { page, page_size: pageSize } },
      )
      setResults(res.items || [])
      setTotal(res.total || 0)
    } catch (e: any) {
      message.error('加载编排任务失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadAgents()
    loadResults()
  }, [page, pageSize])

  const handleCreate = async () => {
    try {
      const values = await form.validateFields()
      setCreateLoading(true)
      const res = await request.post<any, OrchestrationResult>('/api/v1/agent/orchestrate', values)
      message.success('多 Agent 编排任务创建成功')
      setCreateOpen(false)
      form.resetFields()
      setCurrentResult(res)
      setDetailOpen(true)
      loadResults()
    } catch (e: any) {
      message.error('创建编排任务失败: ' + (e?.response?.data?.message || e.message))
    } finally {
      setCreateLoading(false)
    }
  }

  const handleViewDetail = async (result: OrchestrationResult) => {
    setDetailLoading(true)
    try {
      const res = await request.get<any, OrchestrationResult>(`/api/v1/agent/results/${result.task_id}`)
      setCurrentResult(res)
      setDetailOpen(true)
    } catch (e: any) {
      message.error('加载任务详情失败')
    } finally {
      setDetailLoading(false)
    }
  }

  const columns = [
    {
      title: '任务ID',
      dataIndex: 'task_id',
      width: 280,
      render: (text: string) => <code style={{ fontSize: 12 }}>{text?.substring(0, 12)}...</code>,
    },
    {
      title: '标题',
      dataIndex: 'title',
      width: 200,
      render: (text: string) => <strong>{text}</strong>,
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 140,
      render: (status: string) => (
        <Badge status={taskStatusColorMap[status] as any || 'default'} text={status} />
      ),
    },
    {
      title: '子任务',
      dataIndex: 'tasks',
      width: 100,
      render: (tasks: AgentTask[]) => {
        const success = tasks?.filter((t) => t.status === 'success').length || 0
        const total = tasks?.length || 0
        return <Tag color={success === total ? 'success' : 'processing'}>{success}/{total}</Tag>
      },
    },
    {
      title: '风险等级',
      dataIndex: 'risk_assessment',
      width: 120,
      render: (ra: any) => ra ? <Tag color={riskLevelColorMap[ra.level] || 'default'}>{ra.level}</Tag> : '-',
    },
    {
      title: '耗时',
      dataIndex: 'duration_ms',
      width: 100,
      render: (ms: number) => ms ? `${ms}ms` : '-',
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      width: 180,
      render: (text: string) => new Date(text).toLocaleString('zh-CN'),
    },
    {
      title: '操作',
      key: 'action',
      width: 100,
      render: (_: any, record: OrchestrationResult) => (
        <Button type="link" icon={<EyeOutlined />} onClick={() => handleViewDetail(record)}>
          详情
        </Button>
      ),
    },
  ]

  return (
    <div>
      <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
        <Col span={6}>
          <Card size="small">
            <Space>
              <ApiOutlined style={{ fontSize: 24, color: 'var(--color-primary)' }} />
              <div>
                <div style={{ fontSize: 12, color: 'var(--text-secondary)' }}>可用 Agent</div>
                <div style={{ fontSize: 20, fontWeight: 'bold' }}>{agents.length}</div>
              </div>
            </Space>
          </Card>
        </Col>
        <Col span={6}>
          <Card size="small">
            <Space>
              <ThunderboltOutlined style={{ fontSize: 24, color: '#16a34a' }} />
              <div>
                <div style={{ fontSize: 12, color: 'var(--text-secondary)' }}>编排任务</div>
                <div style={{ fontSize: 20, fontWeight: 'bold' }}>{total}</div>
              </div>
            </Space>
          </Card>
        </Col>
        <Col span={6}>
          <Card size="small">
            <Space>
              <CheckCircleOutlined style={{ fontSize: 24, color: '#16a34a' }} />
              <div>
                <div style={{ fontSize: 12, color: 'var(--text-secondary)' }}>已完成</div>
                <div style={{ fontSize: 20, fontWeight: 'bold' }}>
                  {results.filter((r) => r.status === 'completed').length}
                </div>
              </div>
            </Space>
          </Card>
        </Col>
        <Col span={6}>
          <Card size="small">
            <Space>
              <WarningOutlined style={{ fontSize: 24, color: '#d97706' }} />
              <div>
                <div style={{ fontSize: 12, color: 'var(--text-secondary)' }}>待审批</div>
                <div style={{ fontSize: 20, fontWeight: 'bold' }}>
                  {results.filter((r) => r.status === 'awaiting_approval').length}
                </div>
              </div>
            </Space>
          </Card>
        </Col>
      </Row>

      <Card
        title={
          <Space>
            <RobotOutlined style={{ color: 'var(--color-primary)' }} />
            <span>多 Agent 编排任务</span>
          </Space>
        }
        extra={
          <Space>
            <Button icon={<ReloadOutlined />} onClick={loadResults}>刷新</Button>
            <Button type="primary" icon={<PlayCircleOutlined />} onClick={() => setCreateOpen(true)}>
              新建编排任务
            </Button>
          </Space>
        }
      >
        <Table
          columns={columns}
          dataSource={results}
          rowKey="task_id"
          loading={loading}
          pagination={{
            current: page,
            pageSize,
            total,
            showSizeChanger: true,
            showQuickJumper: true,
            showTotal: (t) => `共 ${t} 条`,
            onChange: (p, ps) => { setPage(p); setPageSize(ps) },
          }}
        />
      </Card>

      {/* 新建编排任务弹窗 */}
      <Modal
        title={
          <Space>
            <RobotOutlined />
            <span>新建多 Agent 编排任务</span>
          </Space>
        }
        open={createOpen}
        onCancel={() => { setCreateOpen(false); form.resetFields() }}
        onOk={handleCreate}
        confirmLoading={createLoading}
        width={600}
      >
        <Form form={form} layout="vertical" style={{ marginTop: 16 }}>
          <Form.Item
            name="title"
            label="任务标题"
            rules={[{ required: true, message: '请输入任务标题' }]}
          >
            <Input placeholder="例如：系统健康检查、故障根因分析" />
          </Form.Item>
          <Form.Item name="description" label="任务描述">
            <TextArea rows={3} placeholder="详细描述需要分析的问题" />
          </Form.Item>
          <Form.Item name="agent_types" label="指定 Agent 类型（可选）">
            <Select mode="multiple" placeholder="不指定则使用默认流程：监控→诊断→风险→报告">
              <Option value="monitor">监控 Agent</Option>
              <Option value="diagnosis">诊断 Agent</Option>
              <Option value="risk">风险评估 Agent</Option>
              <Option value="executor">执行 Agent</Option>
              <Option value="verifier">验证 Agent</Option>
              <Option value="reporter">报告 Agent</Option>
            </Select>
          </Form.Item>
          <Form.Item name="auto_approve" label="自动批准低风险操作">
            <Select defaultValue={false}>
              <Option value={false}>否（需要人工审批）</Option>
              <Option value={true}>是（低风险自动批准）</Option>
            </Select>
          </Form.Item>
        </Form>

        <div style={{ marginTop: 16, padding: 12, background: 'var(--color-bg-1)', borderRadius: 6 }}>
          <div style={{ marginBottom: 8, fontWeight: 'bold' }}>
            <ApiOutlined style={{ marginRight: 6 }} />
            可用 Agent ({agents.length})
          </div>
          <Space wrap>
            {agents.map((a) => (
              <Tag key={a.name} color={agentTypeColorMap[a.type] || 'default'}>
                {a.name} ({a.type})
              </Tag>
            ))}
          </Space>
        </div>
      </Modal>

      {/* 任务详情弹窗 */}
      <Modal
        title={
          <Space>
            <RobotOutlined />
            <span>编排任务详情 - {currentResult?.title}</span>
            <Badge status={taskStatusColorMap[currentResult?.status || ''] as any} text={currentResult?.status} />
          </Space>
        }
        open={detailOpen}
        onCancel={() => setDetailOpen(false)}
        footer={null}
        width={900}
      >
        {detailLoading ? (
          <div style={{ textAlign: 'center', padding: 40 }}>
            <Spin size="large" />
          </div>
        ) : currentResult ? (
          <div>
            {/* 概览 */}
            <Descriptions column={3} size="small" bordered style={{ marginBottom: 16 }}>
              <Descriptions.Item label="任务ID">
                <code style={{ fontSize: 11 }}>{currentResult.task_id}</code>
              </Descriptions.Item>
              <Descriptions.Item label="状态">
                <Badge status={taskStatusColorMap[currentResult.status] as any} text={currentResult.status} />
              </Descriptions.Item>
              <Descriptions.Item label="耗时">{currentResult.duration_ms}ms</Descriptions.Item>
              <Descriptions.Item label="子任务数">{currentResult.tasks?.length}</Descriptions.Item>
              <Descriptions.Item label="发现数">{currentResult.findings?.length}</Descriptions.Item>
              <Descriptions.Item label="建议数">{currentResult.recommendations?.length}</Descriptions.Item>
            </Descriptions>

            {/* 风险评估 */}
            {currentResult.risk_assessment && (
              <Alert
                message={
                  <Space>
                    <SafetyCertificateOutlined />
                    <span>风险评估：{currentResult.risk_assessment.level}</span>
                    <Progress
                      type="circle"
                      size={40}
                      percent={currentResult.risk_assessment.score}
                      strokeColor={riskLevelColorMap[currentResult.risk_assessment.level] === 'error' ? '#dc2626' : riskLevelColorMap[currentResult.risk_assessment.level] === 'warning' ? '#d97706' : '#16a34a'}
                    />
                    <span style={{ fontSize: 12, color: 'var(--text-secondary)' }}>
                      {currentResult.risk_assessment.description}
                    </span>
                    {currentResult.risk_assessment.requires_approval && (
                      <Tag color="warning">需要审批</Tag>
                    )}
                  </Space>
                }
                type={currentResult.risk_assessment.level === 'critical' ? 'error' : currentResult.risk_assessment.level === 'high' ? 'warning' : 'success'}
                showIcon
                style={{ marginBottom: 16 }}
              />
            )}

            {/* 摘要 */}
            <Card size="small" title="分析摘要" style={{ marginBottom: 16 }}>
              <p style={{ margin: 0 }}>{currentResult.summary}</p>
            </Card>

            {/* 子任务执行时间线 */}
            <Card size="small" title="Agent 执行流程" style={{ marginBottom: 16 }}>
              <Timeline
                items={currentResult.tasks?.map((task) => ({
                  color: task.status === 'success' ? 'green' : task.status === 'failed' ? 'red' : task.status === 'running' ? 'blue' : 'gray',
                  dot: task.status === 'success' ? <CheckCircleOutlined /> : task.status === 'failed' ? <CloseCircleOutlined /> : <ClockCircleOutlined />,
                  children: (
                    <div>
                      <Space>
                        <strong>{task.agent_name}</strong>
                        <Tag color={agentTypeColorMap[task.agent_type] || 'default'}>{task.agent_type}</Tag>
                        <Badge status={statusColorMap[task.status] as any} text={task.status} />
                        {task.duration_ms ? <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>{task.duration_ms}ms</span> : null}
                      </Space>
                      {task.result?.summary && (
                        <div style={{ marginTop: 4, fontSize: 13, color: 'var(--text-secondary)' }}>
                          {task.result.summary}
                        </div>
                      )}
                      {task.error && (
                        <div style={{ marginTop: 4, fontSize: 12, color: '#dc2626' }}>
                          错误: {task.error}
                        </div>
                      )}
                    </div>
                  ),
                }))}
              />
            </Card>

            {/* 发现 */}
            {currentResult.findings?.length > 0 && (
              <Card size="small" title="主要发现" style={{ marginBottom: 16 }}>
                {currentResult.findings.map((f: any, i: number) => (
                  <div key={i} style={{ padding: '8px 0', borderBottom: i < currentResult.findings.length - 1 ? '1px solid var(--color-border)' : 'none' }}>
                    <Space>
                      <Tag color={f.severity === 'critical' ? 'error' : f.severity === 'warning' ? 'warning' : 'info'}>
                        {f.severity}
                      </Tag>
                      <strong>{f.title}</strong>
                      {f.resource && <Tag>{f.resource}</Tag>}
                    </Space>
                    <div style={{ marginTop: 4, fontSize: 13, color: 'var(--text-secondary)' }}>
                      {f.description}
                    </div>
                  </div>
                ))}
              </Card>
            )}

            {/* 建议 */}
            {currentResult.recommendations?.length > 0 && (
              <Card size="small" title="建议操作">
                {currentResult.recommendations.map((r: any, i: number) => (
                  <div key={i} style={{ padding: '8px 0', borderBottom: i < currentResult.recommendations.length - 1 ? '1px solid var(--color-border)' : 'none' }}>
                    <Space>
                      <Tag color={r.priority === 'P0' ? 'error' : r.priority === 'P1' ? 'warning' : 'info'}>
                        {r.priority}
                      </Tag>
                      <strong>{r.title}</strong>
                      <Tag color={riskLevelColorMap[r.risk] || 'default'}>风险: {r.risk}</Tag>
                    </Space>
                    <div style={{ marginTop: 4, fontSize: 13, color: 'var(--text-secondary)' }}>
                      {r.description}
                    </div>
                    {r.reason && (
                      <div style={{ marginTop: 2, fontSize: 12, color: 'var(--text-muted)' }}>
                        原因: {r.reason}
                      </div>
                    )}
                  </div>
                ))}
              </Card>
            )}

            {/* 最终报告 */}
            {currentResult.final_report && (
              <Card size="small" title="最终报告" style={{ marginTop: 16 }}>
                <div style={{ whiteSpace: 'pre-wrap', fontSize: 13, lineHeight: 1.8 }}>
                  {currentResult.final_report}
                </div>
              </Card>
            )}
          </div>
        ) : (
          <Empty description="暂无数据" />
        )}
      </Modal>
    </div>
  )
}
