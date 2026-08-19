import { useState, useEffect, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  Table, Card, Tag, Button, Space, Input, Select, Modal, message, Drawer,
  Descriptions, Timeline, Alert, Typography, Badge, Spin,
} from 'antd'
import {
  PlayCircleOutlined, CheckCircleOutlined, CloseCircleOutlined,
  ThunderboltOutlined, HistoryOutlined, ExperimentOutlined,
} from '@ant-design/icons'
import { automationApi, type AutomationAction, type DryRunResult } from '@/api/automation'

const { Text } = Typography

const statusColor: Record<string, string> = {
  proposed: 'default',
  pending_approval: 'orange',
  approved: 'blue',
  rejected: 'red',
  running: 'cyan',
  success: 'green',
  failed: 'red',
  timeout: 'orange',
  cancelled: 'default',
}

const riskColor: Record<string, string> = {
  low: 'green',
  medium: 'orange',
  high: 'red',
  critical: 'volcano',
}

const actionTypeLabel: Record<string, string> = {
  restart_pod: '重启 Pod',
  scale_deployment: '扩容 Deployment',
  jenkins_build: 'Jenkins 构建',
  argocd_sync: 'ArgoCD 同步',
}

export default function AutomationActions() {
  const navigate = useNavigate()
  const [actions, setActions] = useState<AutomationAction[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [statusFilter, setStatusFilter] = useState<string>()
  const [riskFilter, setRiskFilter] = useState<string>()
  const [typeFilter, setTypeFilter] = useState<string>()
  const [detail, setDetail] = useState<AutomationAction | null>(null)
  const [detailOpen, setDetailOpen] = useState(false)
  const [dryRunResult, setDryRunResult] = useState<DryRunResult | null>(null)
  const [dryRunLoading, setDryRunLoading] = useState(false)
  const [executing, setExecuting] = useState(false)
  const [executions, setExecutions] = useState<any[]>([])
  const [executionsLoading, setExecutionsLoading] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await automationApi.list({
        status: statusFilter,
        risk: riskFilter,
        action_type: typeFilter,
        page,
        page_size: pageSize,
      })
      setActions(res.items)
      setTotal(res.total)
    } catch (e: any) {
      message.error('加载操作列表失败: ' + (e?.message || ''))
    } finally {
      setLoading(false)
    }
  }, [page, pageSize, statusFilter, riskFilter, typeFilter])

  useEffect(() => { load() }, [load])

  const handleApprove = async (id: number) => {
    Modal.confirm({
      title: '确认审批',
      content: '审批通过后该操作可以被执行。确认审批？',
      okText: '确认审批',
      okType: 'primary',
      cancelText: '取消',
      onOk: async () => {
        try {
          await automationApi.approve(id)
          message.success('审批通过')
          load()
          if (detail?.id === id) setDetail({ ...detail, status: 'approved' })
        } catch (e: any) {
          message.error('审批失败: ' + (e?.message || ''))
        }
      },
    })
  }

  const handleReject = async (id: number) => {
    let reason = ''
    Modal.confirm({
      title: '拒绝操作',
      content: (
        <Input.TextArea
          rows={3}
          placeholder="请输入拒绝原因"
          onChange={(e) => { reason = e.target.value }}
        />
      ),
      okText: '确认拒绝',
      okButtonProps: { danger: true },
      cancelText: '取消',
      onOk: async () => {
        try {
          await automationApi.reject(id, reason)
          message.success('已拒绝')
          load()
        } catch (e: any) {
          message.error('拒绝失败: ' + (e?.message || ''))
        }
      },
    })
  }

  const handleDryRun = async (id: number) => {
    setDryRunLoading(true)
    setDryRunResult(null)
    try {
      const res = await automationApi.dryRun(id)
      setDryRunResult(res)
    } catch (e: any) {
      message.error('Dry Run 失败: ' + (e?.message || ''))
    } finally {
      setDryRunLoading(false)
    }
  }

  const handleExecute = async (id: number) => {
    Modal.confirm({
      title: '确认执行',
      content: '此操作将真正修改 Kubernetes/Jenkins/ArgoCD 资源。确认执行？',
      okText: '确认执行',
      okType: 'danger',
      cancelText: '取消',
      onOk: async () => {
        setExecuting(true)
        try {
          const res = await automationApi.execute(id)
          if (res.success) {
            message.success('执行成功: ' + res.message)
          } else {
            message.error('执行失败: ' + (res.error || '未知错误'))
          }
          load()
        } catch (e: any) {
          message.error('执行失败: ' + (e?.message || ''))
        } finally {
          setExecuting(false)
        }
      },
    })
  }

  const openDetail = async (action: AutomationAction) => {
    setDetail(action)
    setDetailOpen(true)
    setDryRunResult(null)
    fetchExecutions(action.id)
  }

  const fetchExecutions = async (actionId: number) => {
    setExecutionsLoading(true)
    try {
      const res = await automationApi.executions(actionId)
      setExecutions(res as any[])
    } catch {
      setExecutions([])
    } finally {
      setExecutionsLoading(false)
    }
  }

  // running 状态时 5s 自动刷新
  useEffect(() => {
    if (!detailOpen || !detail || detail.status !== 'running') return
    const timer = setInterval(() => {
      fetchExecutions(detail.id)
      load()
    }, 5000)
    return () => clearInterval(timer)
  }, [detailOpen, detail])

  const columns = [
    {
      title: 'ID',
      dataIndex: 'id',
      width: 70,
      render: (id: number) => <Text code>#{id}</Text>,
    },
    {
      title: '操作类型',
      dataIndex: 'action_type',
      width: 140,
      render: (t: string) => actionTypeLabel[t] || t,
    },
    {
      title: '目标',
      dataIndex: 'target_name',
      render: (name: string, record: AutomationAction) => (
        <Space size={4}>
          <Text strong>{name}</Text>
          {record.namespace && <Tag style={{ fontSize: 10 }}>{record.namespace}</Tag>}
        </Space>
      ),
    },
    {
      title: '风险',
      dataIndex: 'risk',
      width: 80,
      render: (r: string) => <Tag color={riskColor[r]}>{r}</Tag>,
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 130,
      render: (s: string) => <Badge color={statusColor[s]} text={s} />,
    },
    {
      title: '关联事件',
      dataIndex: 'incident_id',
      width: 90,
      render: (id: number) => id ? <a onClick={() => navigate(`/aiops/incidents/${id}`)}>#{id}</a> : '-',
    },
    {
      title: '原因',
      dataIndex: 'reason',
      ellipsis: true,
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      width: 170,
      render: (t: string) => new Date(t).toLocaleString(),
    },
    {
      title: '操作',
      width: 200,
      render: (_: any, record: AutomationAction) => (
        <Space size={4}>
          <Button size="small" type="link" onClick={() => openDetail(record)}>详情</Button>
          {record.status === 'pending_approval' && (
            <>
              <Button size="small" type="link" onClick={() => handleApprove(record.id)}>审批</Button>
              <Button size="small" type="link" danger onClick={() => handleReject(record.id)}>拒绝</Button>
            </>
          )}
          {record.status === 'approved' && (
            <Button size="small" type="link" danger onClick={() => handleExecute(record.id)}>执行</Button>
          )}
        </Space>
      ),
    },
  ]

  return (
    <div>
      <Card
        title={<Space><ThunderboltOutlined /> 自动化操作</Space>}
        extra={
          <Space>
            <Select
              placeholder="状态"
              allowClear
              style={{ width: 130 }}
              value={statusFilter}
              onChange={(v) => { setStatusFilter(v); setPage(1) }}
              options={[
                { value: 'pending_approval', label: '待审批' },
                { value: 'approved', label: '已审批' },
                { value: 'running', label: '执行中' },
                { value: 'success', label: '成功' },
                { value: 'failed', label: '失败' },
                { value: 'rejected', label: '已拒绝' },
              ]}
            />
            <Select
              placeholder="风险"
              allowClear
              style={{ width: 100 }}
              value={riskFilter}
              onChange={(v) => { setRiskFilter(v); setPage(1) }}
              options={[
                { value: 'low', label: 'Low' },
                { value: 'medium', label: 'Medium' },
                { value: 'high', label: 'High' },
                { value: 'critical', label: 'Critical' },
              ]}
            />
            <Select
              placeholder="类型"
              allowClear
              style={{ width: 150 }}
              value={typeFilter}
              onChange={(v) => { setTypeFilter(v); setPage(1) }}
              options={Object.entries(actionTypeLabel).map(([v, l]) => ({ value: v, label: l }))}
            />
            <Button onClick={load}>刷新</Button>
          </Space>
        }
      >
        <Table
          rowKey="id"
          columns={columns}
          dataSource={actions}
          loading={loading}
          pagination={{
            current: page,
            pageSize,
            total,
            showSizeChanger: true,
            onChange: (p, ps) => { setPage(p); setPageSize(ps) },
          }}
          size="small"
        />
      </Card>

      <Drawer
        title={detail ? `操作 #${detail.id}: ${actionTypeLabel[detail.action_type] || detail.action_type}` : ''}
        width={560}
        open={detailOpen}
        onClose={() => setDetailOpen(false)}
        destroyOnClose
      >
        {detail && (
          <div>
            <Descriptions column={1} size="small" bordered>
              <Descriptions.Item label="状态">
                <Badge color={statusColor[detail.status]} text={detail.status} />
              </Descriptions.Item>
              <Descriptions.Item label="风险">
                <Tag color={riskColor[detail.risk]}>{detail.risk}</Tag>
              </Descriptions.Item>
              <Descriptions.Item label="目标">{detail.target_name}</Descriptions.Item>
              <Descriptions.Item label="命名空间">{detail.namespace || '-'}</Descriptions.Item>
              <Descriptions.Item label="集群">{detail.cluster}</Descriptions.Item>
              <Descriptions.Item label="原因">{detail.reason || '-'}</Descriptions.Item>
              <Descriptions.Item label="关联事件">
                {detail.incident_id ? `#${detail.incident_id}` : '-'}
              </Descriptions.Item>
              <Descriptions.Item label="创建时间">
                {new Date(detail.created_at).toLocaleString()}
              </Descriptions.Item>
              {detail.approved_at && (
                <Descriptions.Item label="审批时间">
                  {new Date(detail.approved_at).toLocaleString()}
                </Descriptions.Item>
              )}
              {detail.reject_reason && (
                <Descriptions.Item label="拒绝原因">{detail.reject_reason}</Descriptions.Item>
              )}
            </Descriptions>

            <div style={{ marginTop: 16, marginBottom: 16 }}>
              <Space>
                {detail.status === 'pending_approval' && (
                  <>
                    <Button type="primary" icon={<CheckCircleOutlined />} onClick={() => handleApprove(detail.id)}>
                      审批通过
                    </Button>
                    <Button danger icon={<CloseCircleOutlined />} onClick={() => handleReject(detail.id)}>
                      拒绝
                    </Button>
                  </>
                )}
                {detail.status === 'approved' && (
                  <>
                    <Button icon={<ExperimentOutlined />} loading={dryRunLoading} onClick={() => handleDryRun(detail.id)}>
                      Dry Run
                    </Button>
                    <Button type="primary" danger icon={<PlayCircleOutlined />} loading={executing} onClick={() => handleExecute(detail.id)}>
                      执行
                    </Button>
                  </>
                )}
                {(detail.status === 'pending_approval' || detail.status === 'approved') && (
                  <Button onClick={() => {
                    Modal.confirm({
                      title: '取消操作',
                      content: '确认取消该操作？',
                      onOk: async () => {
                        await automationApi.cancel(detail.id)
                        message.success('已取消')
                        load()
                        setDetailOpen(false)
                      },
                    })
                  }}>
                    取消
                  </Button>
                )}
              </Space>
            </div>

            {dryRunResult && (
              <Alert
                type="info"
                showIcon
                message="Dry Run 结果"
                description={
                  <div>
                    <p><strong>当前状态:</strong> {dryRunResult.current_state}</p>
                    <p><strong>预期操作:</strong> {dryRunResult.expected_operation}</p>
                    <p><strong>潜在影响:</strong> {dryRunResult.potential_impact}</p>
                  </div>
                }
                style={{ marginBottom: 16 }}
              />
            )}

            <Card size="small" title={<Space><HistoryOutlined /> 状态时间线</Space>}>
              <Timeline
                items={[
                  { color: 'blue', children: `创建 - ${new Date(detail.created_at).toLocaleString()}` },
                  detail.approved_at ? { color: 'green', children: `审批通过 - ${new Date(detail.approved_at).toLocaleString()}` } : null,
                  detail.rejected_at ? { color: 'red', children: `已拒绝 - ${new Date(detail.rejected_at).toLocaleString()}` } : null,
                  (detail.status === 'running' || detail.status === 'success' || detail.status === 'failed') ? { color: 'cyan', children: '执行中' } : null,
                  detail.status === 'success' ? { color: 'green', children: '执行成功' } : null,
                  detail.status === 'failed' ? { color: 'red', children: '执行失败' } : null,
                ].filter(Boolean) as any}
              />
            </Card>

            <Card
              size="small"
              title={<Space><ThunderboltOutlined /> 执行历史 {detail.status === 'running' && <Tag color="processing">实时刷新中</Tag>}</Space>}
              style={{ marginTop: 12 }}
            >
              {executionsLoading ? (
                <div style={{ textAlign: 'center', padding: 16 }}><Spin size="small" /></div>
              ) : executions.length === 0 ? (
                <div style={{ color: '#999', textAlign: 'center', padding: 16 }}>暂无执行记录</div>
              ) : (
                <Space direction="vertical" style={{ width: '100%' }} size="small">
                  {executions.map((exec) => (
                    <div key={exec.id} style={{ padding: 8, border: '1px solid #f0f0f0', borderRadius: 6 }}>
                      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                        <Space>
                          <Tag color={exec.status === 'success' ? 'green' : exec.status === 'failed' ? 'red' : 'cyan'}>
                            {exec.status}
                          </Tag>
                          <span style={{ fontSize: 12 }}>{exec.executor}</span>
                          {exec.external_id && <Tag style={{ fontSize: 10 }}>#{exec.external_id}</Tag>}
                        </Space>
                        <span style={{ fontSize: 11, color: '#999' }}>{exec.duration_ms}ms</span>
                      </div>
                      {exec.message && <div style={{ fontSize: 12, color: '#666', marginTop: 4 }}>{exec.message}</div>}
                      {exec.error && (
                        <div style={{ fontSize: 12, color: '#ff4d4f', marginTop: 4, background: '#fff2f0', padding: 6, borderRadius: 4 }}>
                          错误: {exec.error}
                        </div>
                      )}
                      <div style={{ fontSize: 11, color: '#bbb', marginTop: 4 }}>
                        {new Date(exec.started_at).toLocaleString()}
                      </div>
                    </div>
                  ))}
                </Space>
              )}
            </Card>
          </div>
        )}
      </Drawer>
    </div>
  )
}
