import { useState, useEffect, useCallback, useRef } from 'react'
import {
  Drawer, Descriptions, Tag, Button, Space, Card, Timeline, Badge, Empty,
  Collapse, Alert, Spin, Tooltip,
} from 'antd'
import {
  CheckCircleOutlined, CloseCircleOutlined, ClockCircleOutlined,
  SyncOutlined, ReloadOutlined, ExclamationCircleOutlined,
} from '@ant-design/icons'
import { workflowApi, type WorkflowExecution, type WorkflowStepExecution } from '@/api/workflow'

interface Props {
  executionId: number | null
  workflowName?: string
  onClose: () => void
  onRefresh?: () => void
}

const execStatusColor: Record<string, string> = {
  pending: 'default',
  running: 'cyan',
  success: 'green',
  failed: 'red',
  timeout: 'orange',
  cancelled: 'default',
}

const stepStatusColor: Record<string, string> = {
  pending: 'default',
  running: 'cyan',
  success: 'green',
  failed: 'red',
  skipped: 'default',
  timeout: 'orange',
}

const stepStatusIcon: Record<string, React.ReactNode> = {
  success: <CheckCircleOutlined style={{ color: '#52c41a' }} />,
  failed: <CloseCircleOutlined style={{ color: '#ff4d4f' }} />,
  running: <SyncOutlined spin style={{ color: '#1890ff' }} />,
  timeout: <ClockCircleOutlined style={{ color: '#fa8c16' }} />,
  skipped: <ClockCircleOutlined style={{ color: '#999' }} />,
  pending: <ClockCircleOutlined style={{ color: '#bfbfbf' }} />,
}

function formatDuration(ms: number): string {
  if (!ms || ms <= 0) return '-'
  if (ms < 1000) return `${ms}ms`
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`
  return `${Math.floor(ms / 60000)}m ${Math.floor((ms % 60000) / 1000)}s`
}

function formatTime(t?: string): string {
  if (!t) return '-'
  return new Date(t).toLocaleString()
}

// 按 step_name 分组，展示每个 Step 的所有 Attempt
function groupStepsByAttempt(steps: WorkflowStepExecution[]): Map<string, WorkflowStepExecution[]> {
  const groups = new Map<string, WorkflowStepExecution[]>()
  for (const s of steps) {
    const key = s.step_name || `step-${s.workflow_step_id}`
    if (!groups.has(key)) groups.set(key, [])
    groups.get(key)!.push(s)
  }
  return groups
}

export default function WorkflowExecutionDetail({ executionId, workflowName, onClose, onRefresh }: Props) {
  const [execution, setExecution] = useState<WorkflowExecution | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null)

  const load = useCallback(async () => {
    if (!executionId) return
    setLoading(true)
    setError(null)
    try {
      const exec = await workflowApi.getExecution(executionId)
      setExecution(exec)
    } catch (e: any) {
      setError(e?.message || '加载执行详情失败')
    } finally {
      setLoading(false)
    }
  }, [executionId])

  useEffect(() => {
    if (executionId) {
      load()
    } else {
      setExecution(null)
      setError(null)
    }
  }, [executionId, load])

  // RUNNING 状态时每 3 秒轮询
  useEffect(() => {
    if (pollRef.current) {
      clearInterval(pollRef.current)
      pollRef.current = null
    }
    if (execution?.status === 'running') {
      pollRef.current = setInterval(() => { load() }, 3000)
    }
    return () => {
      if (pollRef.current) clearInterval(pollRef.current)
    }
  }, [execution?.status, load])

  if (!executionId) return null

  const stepGroups = execution?.step_executions ? groupStepsByAttempt(execution.step_executions) : new Map()
  const isRunning = execution?.status === 'running'

  // 构建 Timeline 数据
  const timelineItems = execution?.step_executions
    ?.slice()
    .sort((a, b) => new Date(a.started_at || a.created_at).getTime() - new Date(b.started_at || b.created_at).getTime())
    .map((step) => ({
      color: step.status === 'success' ? 'green' : step.status === 'failed' ? 'red' : step.status === 'running' ? 'blue' : 'gray',
      children: (
        <div>
          <Space>
            <strong>{step.step_name}</strong>
            <Tag color={stepStatusColor[step.status]}>{step.status}</Tag>
            {step.attempt > 1 && <Tag color="orange">Attempt {step.attempt}</Tag>}
          </Space>
          <div style={{ fontSize: 12, color: '#666', marginTop: 2 }}>
            {formatTime(step.started_at)} → {formatTime(step.finished_at)} · {formatDuration(step.duration_ms)}
          </div>
          {step.error && <div style={{ fontSize: 12, color: '#ff4d4f', marginTop: 2 }}>{step.error}</div>}
        </div>
      ),
    })) || []

  return (
    <Drawer
      title={
        <Space>
          {execution && <Badge color={execStatusColor[execution.status]} text={execution.status} />}
          <span style={{ fontSize: 14 }}>
            执行 #{executionId}{workflowName ? ` · ${workflowName}` : ''}
          </span>
          {isRunning && <Spin size="small" />}
        </Space>
      }
      width={720}
      open={!!executionId}
      onClose={onClose}
      destroyOnClose
      extra={
        <Space>
          <Button size="small" icon={<ReloadOutlined />} onClick={load} loading={loading}>刷新</Button>
        </Space>
      }
    >
      {loading && !execution && <div style={{ textAlign: 'center', padding: 40 }}><Spin /></div>}

      {error && (
        <Alert
          type="error"
          message="加载失败"
          description={error}
          showIcon
          action={<Button size="small" onClick={load}>重试</Button>}
        />
      )}

      {execution && !error && (
        <div>
          {/* Execution Overview */}
          <Descriptions column={2} size="small" bordered style={{ marginBottom: 16 }}>
            <Descriptions.Item label="状态">
              <Badge color={execStatusColor[execution.status]} text={execution.status} />
            </Descriptions.Item>
            <Descriptions.Item label="触发方式">{execution.trigger_type || 'manual'}</Descriptions.Item>
            <Descriptions.Item label="开始时间">{formatTime(execution.started_at)}</Descriptions.Item>
            <Descriptions.Item label="结束时间">{formatTime(execution.finished_at)}</Descriptions.Item>
            <Descriptions.Item label="耗时">{formatDuration(execution.duration_ms)}</Descriptions.Item>
            <Descriptions.Item label="步骤数">{execution.step_executions?.length || 0}</Descriptions.Item>
            {execution.error && (
              <Descriptions.Item label="错误" span={2}>
                <span style={{ color: '#ff4d4f' }}>{execution.error}</span>
              </Descriptions.Item>
            )}
          </Descriptions>

          {/* Step Execution 详情（按 Step 分组，展示 Attempt 历史） */}
          <Card
            size="small"
            title={<Space><ExclamationCircleOutlined /> 步骤执行详情</Space>}
            style={{ marginBottom: 16 }}
          >
            {stepGroups.size === 0 ? (
              <Empty description="暂无步骤执行记录" />
            ) : (
              <Collapse
                size="small"
                defaultActiveKey={Array.from(stepGroups.keys())}
                items={Array.from(stepGroups.entries()).map(([stepName, attempts]) => {
                  const latest = attempts[attempts.length - 1]
                  const finalStatus = latest.status
                  const hasRetry = attempts.length > 1
                  return {
                    key: stepName,
                    label: (
                      <Space>
                        {stepStatusIcon[finalStatus] || <ClockCircleOutlined />}
                        <strong>{stepName}</strong>
                        <Tag color={stepStatusColor[finalStatus]}>{finalStatus}</Tag>
                        {hasRetry && <Tag color="orange">重试 {attempts.length - 1} 次</Tag>}
                        <span style={{ fontSize: 12, color: '#999' }}>{formatDuration(latest.duration_ms)}</span>
                      </Space>
                    ),
                    children: (
                      <div>
                        {/* 每个 Attempt 的详情 */}
                        {attempts.map((att: WorkflowStepExecution, idx: number) => (
                          <div
                            key={att.id}
                            style={{
                              padding: '8px 12px',
                              marginBottom: 8,
                              border: '1px solid #f0f0f0',
                              borderRadius: 4,
                              backgroundColor: att.status === 'failed' ? '#fff2f0' : '#fafafa',
                            }}
                          >
                            <Space style={{ marginBottom: 4 }}>
                              <Tag color={att.attempt > 1 ? 'orange' : 'blue'}>Attempt {att.attempt}</Tag>
                              <Tag color={stepStatusColor[att.status]}>{att.status}</Tag>
                              {att.step_type && <Tag style={{ fontSize: 10 }}>{att.step_type}</Tag>}
                              {att.action_type && <Tag style={{ fontSize: 10 }}>{att.action_type}</Tag>}
                            </Space>
                            <Descriptions column={2} size="small">
                              <Descriptions.Item label="开始">{formatTime(att.started_at)}</Descriptions.Item>
                              <Descriptions.Item label="结束">{formatTime(att.finished_at)}</Descriptions.Item>
                              <Descriptions.Item label="耗时">{formatDuration(att.duration_ms)}</Descriptions.Item>
                              <Descriptions.Item label="Step ID">#{att.workflow_step_id}</Descriptions.Item>
                            </Descriptions>
                            {att.result && (
                              <div style={{ marginTop: 4 }}>
                                <div style={{ fontSize: 12, color: '#666', marginBottom: 2 }}>结果:</div>
                                <pre style={{
                                  fontSize: 11,
                                  background: '#f5f5f5',
                                  padding: 8,
                                  borderRadius: 4,
                                  maxHeight: 150,
                                  overflow: 'auto',
                                  margin: 0,
                                }}>{att.result}</pre>
                              </div>
                            )}
                            {att.error && (
                              <Alert
                                type="error"
                                message="执行错误"
                                description={att.error}
                                showIcon
                                style={{ marginTop: 4, fontSize: 12 }}
                              />
                            )}
                          </div>
                        ))}
                      </div>
                    ),
                  }
                })}
              />
            )}
          </Card>

          {/* Execution Timeline */}
          <Card size="small" title={<Space><SyncOutlined /> 执行时间线</Space>}>
            {timelineItems.length === 0 ? (
              <Empty description="暂无时间线数据" />
            ) : (
              <Timeline items={timelineItems} />
            )}
          </Card>

          {isRunning && (
            <Alert
              type="info"
              showIcon
              message="工作流正在执行中"
              description="页面每 3 秒自动刷新，执行完成后停止轮询。"
              style={{ marginTop: 16 }}
            />
          )}
        </div>
      )}
    </Drawer>
  )
}
