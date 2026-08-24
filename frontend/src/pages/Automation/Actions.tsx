import { useState, useEffect, useCallback } from 'react'
import {
  Table, Card, Tag, Button, Space, Input, Select, Modal, message, Drawer,
  Descriptions, Timeline, Alert, Typography, Badge, Spin, Form,
} from 'antd'
import {
  PlayCircleOutlined, CheckCircleOutlined, CloseCircleOutlined,
  ThunderboltOutlined, HistoryOutlined, ExperimentOutlined, PlusOutlined,
} from '@ant-design/icons'
import { automationApi, type AutomationAction, type DryRunResult } from '@/api/automation'
import { connectionApi, type ConnectionView } from '@/api/connection'
import { jenkinsApi } from '@/api/jenkins'
import { argocdApi } from '@/api/argocd'
import IncidentDetail from '@/pages/AIOps/IncidentDetail'

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

// 生成操作对应的执行命令，供审批者查看
const generateActionCommand = (action: AutomationAction): string => {
  const ns = action.namespace ? `-n ${action.namespace}` : ''
  switch (action.action_type) {
    case 'restart_pod':
      return `kubectl delete pod ${action.target_name} ${ns}`.trim()
    case 'scale_deployment': {
      let replicas = ''
      try {
        const params = typeof action.parameters === 'string' ? JSON.parse(action.parameters) : action.parameters
        if (params?.replicas !== undefined) replicas = `--replicas=${params.replicas}`
      } catch { /* ignore */ }
      return `kubectl scale deployment ${action.target_name} ${replicas} ${ns}`.replace(/\s+/g, ' ').trim()
    }
    case 'jenkins_build':
      return `jenkins build ${action.target_name}`
    case 'argocd_sync':
      return `argocd app sync ${action.target_name}`
    default:
      return action.action_type
  }
}

export default function AutomationActions() {
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
  // IncidentDetail Drawer
  const [incidentDetailId, setIncidentDetailId] = useState<number | null>(null)
  const [incidentDetailOpen, setIncidentDetailOpen] = useState(false)
  const [dryRunResult, setDryRunResult] = useState<DryRunResult | null>(null)
  const [dryRunLoading, setDryRunLoading] = useState(false)
  const [executing, setExecuting] = useState(false)
  const [executions, setExecutions] = useState<any[]>([])
  const [executionsLoading, setExecutionsLoading] = useState(false)

  // 创建 Action
  const [createOpen, setCreateOpen] = useState(false)
  const [createForm] = Form.useForm()
  const [creating, setCreating] = useState(false)
  const [jenkinsConnections, setJenkinsConnections] = useState<ConnectionView[]>([])
  const [argocdConnections, setArgoCDConnections] = useState<ConnectionView[]>([])
  const [jenkinsJobs, setJenkinsJobs] = useState<any[]>([])
  const [argoApps, setArgoApps] = useState<any[]>([])
  const [loadingJobs, setLoadingJobs] = useState(false)
  const [loadingApps, setLoadingApps] = useState(false)
  const [createActionType, setCreateActionType] = useState<string>('')
  const [paramList, setParamList] = useState<{ key: string; value: string }[]>([])

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

  // 打开创建 Modal
  const openCreate = async () => {
    createForm.resetFields()
    setCreateActionType('')
    setParamList([])
    setJenkinsJobs([])
    setArgoApps([])
    setCreateOpen(true)
    // 加载 Jenkins 和 ArgoCD Connection 列表
    try {
      const [jenkinsRes, argoRes] = await Promise.all([
        connectionApi.list({ type: 'jenkins', enabled: true, page_size: 100 }),
        connectionApi.list({ type: 'argocd', enabled: true, page_size: 100 }),
      ])
      setJenkinsConnections(jenkinsRes.items)
      setArgoCDConnections(argoRes.items)
    } catch (e: any) {
      message.error('加载 Connection 列表失败: ' + (e?.message || ''))
    }
  }

  // action_type 变化时处理
  const handleActionTypeChange = (type: string) => {
    setCreateActionType(type)
    setJenkinsJobs([])
    setArgoApps([])
    createForm.setFieldsValue({ connection_id: undefined, target_name: undefined })
  }

  // 加载 Jenkins Jobs
  const loadJenkinsJobs = async (connectionId: number) => {
    setLoadingJobs(true)
    try {
      const jobs = await jenkinsApi.jobs(connectionId)
      setJenkinsJobs(jobs as any[])
    } catch (e: any) {
      message.error('加载 Jenkins Jobs 失败: ' + (e?.message || ''))
      setJenkinsJobs([])
    } finally {
      setLoadingJobs(false)
    }
  }

  // 加载 ArgoCD Applications
  const loadArgoApps = async (connectionId: number) => {
    setLoadingApps(true)
    try {
      const apps = await argocdApi.apps(connectionId)
      setArgoApps(apps as any[])
    } catch (e: any) {
      message.error('加载 ArgoCD Applications 失败: ' + (e?.message || ''))
      setArgoApps([])
    } finally {
      setLoadingApps(false)
    }
  }

  // Connection 变化时加载 Jobs/Apps
  const handleConnectionChange = (connectionId: number) => {
    if (createActionType === 'jenkins_build') {
      loadJenkinsJobs(connectionId)
    } else if (createActionType === 'argocd_sync') {
      loadArgoApps(connectionId)
    }
  }

  // 提交创建
  const handleCreateSubmit = async () => {
    try {
      const values = await createForm.validateFields()
      setCreating(true)

      // 构建 parameters
      const parameters: Record<string, any> = {}
      if (values.action_type === 'scale_deployment' && values.replicas) {
        parameters.replicas = Number(values.replicas)
      }
      paramList.forEach((p) => {
        if (p.key) parameters[p.key] = p.value
      })

      const data: any = {
        action_type: values.action_type,
        target_type: values.target_type || '',
        target_name: values.target_name,
        cluster: values.cluster || '',
        namespace: values.namespace || '',
        reason: values.reason || '',
        risk: values.risk || 'medium',
      }
      if (values.connection_id) data.connection_id = values.connection_id
      if (Object.keys(parameters).length > 0) data.parameters = parameters

      await automationApi.create(data)
      message.success('操作创建成功，等待审批')
      setCreateOpen(false)
      load()
    } catch (e: any) {
      if (e?.errorFields) return // 表单验证错误
      message.error('创建失败: ' + (e?.message || ''))
    } finally {
      setCreating(false)
    }
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
      width: 240,
      ellipsis: true,
      render: (name: string, record: AutomationAction) => (
        <Space size={4} wrap>
          <Text strong style={{ fontSize: 12 }}>{name}</Text>
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
      width: 100,
      render: (id: number) => {
        if (!id) return '-'
        return (
          <Tag
            color="blue"
            style={{ margin: 0, cursor: 'pointer' }}
            onClick={() => {
              setIncidentDetailId(id)
              setIncidentDetailOpen(true)
            }}
          >
            #{id}
          </Tag>
        )
      },
    },
    {
      title: '原因',
      dataIndex: 'reason',
      width: 200,
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
            <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>创建操作</Button>
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
            {/* 执行命令预览 - 审批者重点关注 */}
            <Alert
              type="warning"
              showIcon
              icon={<ThunderboltOutlined />}
              message="即将执行的命令"
              description={
                <div>
                  <Text code copyable style={{ fontSize: 13, background: 'rgba(0,0,0,0.06)', padding: '4px 8px', borderRadius: 4, display: 'block', marginTop: 4 }}>
                    {generateActionCommand(detail)}
                  </Text>
                  <div style={{ marginTop: 8, fontSize: 12, color: '#666' }}>
                    操作类型: <Tag color="blue" style={{ fontSize: 10 }}>{actionTypeLabel[detail.action_type] || detail.action_type}</Tag>
                    目标: <Text strong style={{ fontSize: 12 }}>{detail.target_name}</Text>
                    {detail.namespace && <span style={{ marginLeft: 8 }}>命名空间: <Text code style={{ fontSize: 12 }}>{detail.namespace}</Text></span>}
                  </div>
                </div>
              }
              style={{ marginBottom: 16 }}
            />

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
              <Descriptions.Item label="执行命令">
                <Text code copyable style={{ fontSize: 12 }}>{generateActionCommand(detail)}</Text>
              </Descriptions.Item>
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

      {/* 创建 Action Modal */}
      <Modal
        title="创建自动化操作"
        open={createOpen}
        onCancel={() => setCreateOpen(false)}
        onOk={handleCreateSubmit}
        confirmLoading={creating}
        okText="创建"
        cancelText="取消"
        width={640}
        destroyOnClose
      >
        <Form form={createForm} layout="vertical" size="small">
          <Form.Item
            name="action_type"
            label="操作类型"
            rules={[{ required: true, message: '请选择操作类型' }]}
          >
            <Select
              placeholder="请选择操作类型"
              onChange={handleActionTypeChange}
              options={[
                { value: 'restart_pod', label: '重启 Pod' },
                { value: 'scale_deployment', label: '扩容 Deployment' },
                { value: 'jenkins_build', label: 'Jenkins 构建' },
                { value: 'argocd_sync', label: 'ArgoCD 同步' },
              ]}
            />
          </Form.Item>

          {/* Kubernetes 操作字段 */}
          {(createActionType === 'restart_pod' || createActionType === 'scale_deployment') && (
            <>
              <Form.Item name="cluster" label="集群" rules={[{ required: true, message: '请输入集群名称' }]}>
                <Input placeholder="例如: local" />
              </Form.Item>
              <Form.Item name="namespace" label="命名空间" rules={[{ required: true, message: '请输入命名空间' }]}>
                <Input placeholder="例如: default" />
              </Form.Item>
              <Form.Item name="target_name" label={createActionType === 'restart_pod' ? 'Pod 名称' : 'Deployment 名称'} rules={[{ required: true, message: '请输入名称' }]}>
                <Input placeholder="请输入名称" />
              </Form.Item>
              {createActionType === 'scale_deployment' && (
                <Form.Item name="replicas" label="目标副本数" rules={[{ required: true, message: '请输入副本数' }]}>
                  <Input type="number" placeholder="例如: 2" />
                </Form.Item>
              )}
            </>
          )}

          {/* Jenkins 操作字段 */}
          {createActionType === 'jenkins_build' && (
            <>
              <Form.Item name="connection_id" label="Jenkins Connection" rules={[{ required: true, message: '请选择 Connection' }]}>
                <Select
                  placeholder="选择 Jenkins Connection"
                  onChange={handleConnectionChange}
                  options={jenkinsConnections.map((c) => ({ value: c.id, label: c.name }))}
                />
              </Form.Item>
              <Form.Item name="target_name" label="Job" rules={[{ required: true, message: '请选择 Job' }]}>
                <Select
                  placeholder={loadingJobs ? '加载中...' : '选择 Jenkins Job'}
                  loading={loadingJobs}
                  showSearch
                  options={jenkinsJobs.map((j: any) => ({ value: j.name, label: j.name }))}
                />
              </Form.Item>
            </>
          )}

          {/* ArgoCD 操作字段 */}
          {createActionType === 'argocd_sync' && (
            <>
              <Form.Item name="connection_id" label="ArgoCD Connection" rules={[{ required: true, message: '请选择 Connection' }]}>
                <Select
                  placeholder="选择 ArgoCD Connection"
                  onChange={handleConnectionChange}
                  options={argocdConnections.map((c) => ({ value: c.id, label: c.name }))}
                />
              </Form.Item>
              <Form.Item name="target_name" label="Application" rules={[{ required: true, message: '请选择 Application' }]}>
                <Select
                  placeholder={loadingApps ? '加载中...' : '选择 ArgoCD Application'}
                  loading={loadingApps}
                  showSearch
                  options={argoApps.map((a: any) => ({ value: a.metadata?.name || a.name, label: a.metadata?.name || a.name }))}
                />
              </Form.Item>
            </>
          )}

          {/* 通用参数配置（Jenkins/ArgoCD） */}
          {(createActionType === 'jenkins_build' || createActionType === 'argocd_sync') && (
            <Form.Item label="参数配置">
              <Space direction="vertical" style={{ width: '100%' }}>
                {paramList.map((p, idx) => (
                  <Space key={idx}>
                    <Input
                      placeholder="参数名"
                      value={p.key}
                      onChange={(e) => {
                        const newList = [...paramList]
                        newList[idx].key = e.target.value
                        setParamList(newList)
                      }}
                      style={{ width: 150 }}
                    />
                    <Input
                      placeholder="参数值"
                      value={p.value}
                      onChange={(e) => {
                        const newList = [...paramList]
                        newList[idx].value = e.target.value
                        setParamList(newList)
                      }}
                      style={{ width: 200 }}
                    />
                    <Button type="link" danger onClick={() => setParamList(paramList.filter((_, i) => i !== idx))}>删除</Button>
                  </Space>
                ))}
                <Button type="dashed" onClick={() => setParamList([...paramList, { key: '', value: '' }])}>+ 添加参数</Button>
              </Space>
            </Form.Item>
          )}

          <Form.Item name="reason" label="原因">
            <Input.TextArea rows={2} placeholder="请输入操作原因" />
          </Form.Item>

          <Form.Item name="risk" label="风险等级" initialValue="medium">
            <Select
              options={[
                { value: 'low', label: 'Low' },
                { value: 'medium', label: 'Medium' },
                { value: 'high', label: 'High' },
                { value: 'critical', label: 'Critical' },
              ]}
            />
          </Form.Item>
        </Form>
      </Modal>

      {/* IncidentDetail Drawer */}
      {incidentDetailId !== null && (
        <IncidentDetail
          id={incidentDetailId}
          open={incidentDetailOpen}
          onClose={() => setIncidentDetailOpen(false)}
          onChanged={load}
        />
      )}
    </div>
  )
}
