import { useEffect, useState, useCallback, useRef } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  Drawer, Tabs, Descriptions, Tag, Spin, Timeline, Empty, Button, Space,
  Modal, message, Badge, Card, Alert, Table,
} from 'antd'
import {
  CheckCircleOutlined, StopOutlined, CloseCircleOutlined,
  AlertOutlined, FireOutlined, InfoCircleOutlined,
} from '@ant-design/icons'
import { incidentApi, type EvidenceBundle } from '@/api/incident'
import { topologyApi } from '@/api/topology'
import { rcaApi } from '@/api/rca'
import { aiAnalysisApi } from '@/api/aiAnalysis'
import { automationApi, type AutomationAction, type ActionExecution } from '@/api/automation'
import { workflowApi, type Workflow, type WorkflowExecution, type WorkflowStepExecution } from '@/api/workflow'
import type { Incident, IncidentSignal, TopologyNode, RCAResult, AIAnalysisResult } from '@/types'
import dayjs from 'dayjs'
import { useDataTrust } from '@/hooks/useDataTrust'
import { extractProvenance } from '@/utils/provenance'
import { DataTrustIndicator } from '@/components/DataTrustIndicator'

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

  // P1-X.9: 统一 Data Trust（IncidentDetail 来自 MySQL + 多 Provider）
  const trust = useDataTrust({ source: 'mysql' })
  const [topologyNodes, setTopologyNodes] = useState<TopologyNode[]>([])
  const [topologyLoading, setTopologyLoading] = useState(false)
  const [rcaResult, setRcaResult] = useState<RCAResult | null>(null)
  const [rcaLoading, setRcaLoading] = useState(false)
  const [aiResult, setAiResult] = useState<AIAnalysisResult | null>(null)
  const [aiLoading, setAiLoading] = useState(false)
  const [evidence, setEvidence] = useState<EvidenceBundle | null>(null)
  const [evidenceLoading, setEvidenceLoading] = useState(false)

  // P0-PRODUCT-02: Incident Action Execution Closure
  const [incidentActions, setIncidentActions] = useState<AutomationAction[]>([])
  const [actionsLoading, setActionsLoading] = useState(false)
  const [selectedActionId, setSelectedActionId] = useState<number | null>(null)
  const [actionExecutions, setActionExecutions] = useState<ActionExecution[]>([])
  const [executionsLoading, setExecutionsLoading] = useState(false)

  // P0-PRODUCT-03: Incident Workflow Orchestration
  const [incidentWorkflows, setIncidentWorkflows] = useState<Workflow[]>([])
  const [workflowsLoading, setWorkflowsLoading] = useState(false)
  const [selectedWorkflowId, setSelectedWorkflowId] = useState<number | null>(null)
  const [workflowExecutions, setWorkflowExecutions] = useState<WorkflowExecution[]>([])
  const [workflowExecLoading, setWorkflowExecLoading] = useState(false)
  const [createWorkflowOpen, setCreateWorkflowOpen] = useState(false)
  const [newWorkflowName, setNewWorkflowName] = useState('')
  const [newWorkflowDesc, setNewWorkflowDesc] = useState('')
  const pollingRef = useRef<number | null>(null)
  const currentIdRef = useRef<number | null>(null)

  const actionStatusColor: Record<string, string> = {
    proposed: 'default',
    pending_approval: 'warning',
    approved: 'blue',
    rejected: 'default',
    running: 'processing',
    success: 'success',
    failed: 'error',
    timeout: 'error',
    cancelled: 'default',
  }

  const actionStatusLabel: Record<string, string> = {
    proposed: '待提交',
    pending_approval: '待审批',
    approved: '已审批',
    rejected: '已拒绝',
    running: '执行中',
    success: '执行成功',
    failed: '执行失败',
    timeout: '执行超时',
    cancelled: '已取消',
  }

  const workflowStatusColor: Record<string, string> = {
    draft: 'default',
    pending_approval: 'warning',
    approved: 'blue',
    running: 'processing',
    success: 'success',
    failed: 'error',
    cancelled: 'default',
  }

  const workflowStatusLabel: Record<string, string> = {
    draft: '草稿',
    pending_approval: '待审批',
    approved: '已审批',
    running: '执行中',
    success: '执行成功',
    failed: '执行失败',
    cancelled: '已取消',
  }

  const stepStatusColor: Record<string, string> = {
    pending: 'default',
    running: 'processing',
    success: 'success',
    failed: 'error',
    skipped: 'default',
    timeout: 'warning',
  }

  const stepTypeLabel: Record<string, string> = {
    observation: '观察',
    investigation: '调查',
    automation: '自动化',
    verification: '验证',
  }

  const execStatusColor: Record<string, string> = {
    pending: 'default',
    running: 'processing',
    success: 'success',
    failed: 'error',
    timeout: 'warning',
    cancelled: 'default',
  }

  const parseVerification = (resultJson?: string) => {
    if (!resultJson) return null
    try {
      const parsed = JSON.parse(resultJson)
      return parsed?.data?.verification || parsed?.verification || null
    } catch {
      return null
    }
  }

  const hasRunningAction = incidentActions.some((a) => a.status === 'running' || a.status === 'approved')

  const loadIncidentActions = useCallback(async () => {
    if (!id) return
    if (currentIdRef.current !== id) return
    setActionsLoading(true)
    try {
      const res = await automationApi.list({ incident_id: id, page_size: 50 })
      if (currentIdRef.current !== id) return
      setIncidentActions(res.items || [])
      // 自动加载 success Action 的 Execution，确保 Verification 数据可用
      const successActions = (res.items || []).filter((a) => a.status === 'success')
      for (const action of successActions) {
        if (currentIdRef.current !== id) return
        try {
          const execs = await automationApi.executions(action.id)
          if (currentIdRef.current !== id) return
          setActionExecutions((prev) => {
            const filtered = prev.filter((e) => e.action_id !== action.id)
            return [...filtered, ...(execs || [])]
          })
        } catch {
          // ignore individual execution load failure
        }
      }
    } catch {
      if (currentIdRef.current === id) setIncidentActions([])
    } finally {
      if (currentIdRef.current === id) setActionsLoading(false)
    }
  }, [id])

  const loadIncidentWorkflows = useCallback(async () => {
    if (!id) return
    if (currentIdRef.current !== id) return
    setWorkflowsLoading(true)
    try {
      const res = await workflowApi.list({ incident_id: id, page_size: 50 })
      if (currentIdRef.current !== id) return
      setIncidentWorkflows(res.items || [])
      // 自动加载 success Workflow 的 Execution，确保 Verification 数据可用
      const successWfs = (res.items || []).filter((w) => w.status === 'success')
      for (const wf of successWfs) {
        if (currentIdRef.current !== id) return
        try {
          const execRes = await workflowApi.listExecutions(wf.id, { page_size: 5 })
          if (currentIdRef.current !== id) return
          setWorkflowExecutions((prev) => {
            const filtered = prev.filter((e) => e.workflow_id !== wf.id)
            return [...filtered, ...(execRes.items || [])]
          })
        } catch {
          // ignore individual execution load failure
        }
      }
    } catch {
      if (currentIdRef.current === id) setIncidentWorkflows([])
    } finally {
      if (currentIdRef.current === id) setWorkflowsLoading(false)
    }
  }, [id])

  const hasRunningWorkflow = incidentWorkflows.some((w) => w.status === 'running' || w.status === 'approved')
  const hasRunningAny = hasRunningAction || hasRunningWorkflow

  const stopPolling = useCallback(() => {
    if (pollingRef.current !== null) {
      clearInterval(pollingRef.current)
      pollingRef.current = null
    }
  }, [])

  const startPolling = useCallback(() => {
    stopPolling()
    pollingRef.current = window.setInterval(() => {
      loadIncidentActions()
      loadIncidentWorkflows()
    }, 4000)
  }, [loadIncidentActions, loadIncidentWorkflows, stopPolling])

  // 加载关联 Action 列表
  useEffect(() => {
    if (!open || !id) return
    currentIdRef.current = id
    loadIncidentActions()
    return () => {
      stopPolling()
    }
  }, [open, id, loadIncidentActions, stopPolling])

  // 有 running/approved Action 或 Workflow 时启动轮询
  useEffect(() => {
    if (hasRunningAny && open) {
      startPolling()
    } else {
      stopPolling()
    }
  }, [hasRunningAny, open, startPolling, stopPolling])

  // 选中 Action 时加载执行记录
  useEffect(() => {
    if (!selectedActionId) {
      setActionExecutions([])
      return
    }
    setExecutionsLoading(true)
    automationApi.executions(selectedActionId)
      .then((data) => setActionExecutions(data || []))
      .catch(() => setActionExecutions([]))
      .finally(() => setExecutionsLoading(false))
  }, [selectedActionId, incidentActions])

  const handleApprove = async (actionId: number) => {
    try {
      await automationApi.approve(actionId)
      message.success('操作已审批通过')
      loadIncidentActions()
    } catch (e: any) {
      if (e?.response?.status === 409) {
        message.warning('操作状态已变更，正在刷新...')
        loadIncidentActions()
      } else {
        message.error('审批失败: ' + (e?.message || ''))
      }
    }
  }

  const handleReject = (actionId: number) => {
    Modal.confirm({
      title: '拒绝操作',
      content: '请输入拒绝原因：',
      onOk: async () => {
        try {
          await automationApi.reject(actionId, '用户拒绝')
          message.success('操作已拒绝')
          loadIncidentActions()
        } catch (e: any) {
          message.error('拒绝失败: ' + (e?.message || ''))
        }
      },
    })
  }

  const handleExecute = async (actionId: number) => {
    try {
      const result = await automationApi.execute(actionId)
      if (result.success) {
        message.success('操作执行完成')
      } else {
        message.error('操作执行失败: ' + (result.message || result.error || ''))
      }
      loadIncidentActions()
      if (selectedActionId === actionId) {
        setSelectedActionId(null)
        setTimeout(() => setSelectedActionId(actionId), 300)
      }
    } catch (e: any) {
      if (e?.response?.status === 409) {
        message.warning('操作状态已变更，正在刷新...')
        loadIncidentActions()
      } else {
        message.error('执行失败: ' + (e?.message || ''))
      }
    }
  }

  const handleResolveIncident = async () => {
    if (!id) return
    Modal.confirm({
      title: '解决事件',
      content: '验证已通过，确认将此事件标记为已解决？',
      onOk: async () => {
        try {
          await incidentApi.resolve(id)
          message.success('事件已解决')
          fetchDetail()
          onChanged()
        } catch (e: any) {
          message.error('解决事件失败: ' + (e?.message || ''))
        }
      },
    })
  }

  // ===== P0-PRODUCT-03: Workflow Orchestration =====

  // 加载关联 Workflow 列表
  useEffect(() => {
    if (!open || !id) return
    loadIncidentWorkflows()
  }, [open, id, loadIncidentWorkflows])

  // 选中 Workflow 时加载执行记录
  useEffect(() => {
    if (!selectedWorkflowId) {
      setWorkflowExecutions([])
      return
    }
    setWorkflowExecLoading(true)
    workflowApi.listExecutions(selectedWorkflowId, { page_size: 10 })
      .then((data) => setWorkflowExecutions(data.items || []))
      .catch(() => setWorkflowExecutions([]))
      .finally(() => setWorkflowExecLoading(false))
  }, [selectedWorkflowId, incidentWorkflows])

  const handleCreateWorkflow = async () => {
    if (!id || !incident) return
    if (!newWorkflowName.trim()) {
      message.warning('请输入 Workflow 名称')
      return
    }
    try {
      const wf = await workflowApi.create({
        name: newWorkflowName.trim(),
        description: newWorkflowDesc.trim() || `基于事件 #${id}: ${incident.title}`,
        incident_id: id,
        risk: incident.severity === 'critical' ? 'high' : incident.severity === 'warning' ? 'medium' : 'low',
        steps: [
          { order: 1, name: '系统观察', action_type: 'observation', status: 'pending' },
          { order: 2, name: '故障调查', action_type: 'investigation', status: 'pending' },
          { order: 3, name: '自动化处置', action_type: 'automation', status: 'pending' },
          { order: 4, name: '结果验证', action_type: 'verification', status: 'pending' },
        ],
      })
      message.success(`Workflow #${wf.id} 已创建`)
      setCreateWorkflowOpen(false)
      setNewWorkflowName('')
      setNewWorkflowDesc('')
      loadIncidentWorkflows()
    } catch (e: any) {
      message.error('创建 Workflow 失败: ' + (e?.message || ''))
    }
  }

  const handleWorkflowSubmit = async (wfId: number) => {
    try {
      await workflowApi.submit(wfId)
      message.success('Workflow 已提交审批')
      loadIncidentWorkflows()
    } catch (e: any) {
      message.error('提交失败: ' + (e?.message || ''))
    }
  }

  const handleWorkflowApprove = async (wfId: number) => {
    try {
      await workflowApi.approve(wfId)
      message.success('Workflow 已审批通过')
      loadIncidentWorkflows()
    } catch (e: any) {
      if (e?.response?.status === 409) {
        message.warning('状态已变更，正在刷新...')
        loadIncidentWorkflows()
      } else {
        message.error('审批失败: ' + (e?.message || ''))
      }
    }
  }

  const handleWorkflowExecute = async (wfId: number) => {
    try {
      await workflowApi.execute(wfId)
      message.success('Workflow 执行已启动')
      loadIncidentWorkflows()
      if (selectedWorkflowId === wfId) {
        setSelectedWorkflowId(null)
        setTimeout(() => setSelectedWorkflowId(wfId), 300)
      }
    } catch (e: any) {
      if (e?.response?.status === 409) {
        message.warning('状态已变更，正在刷新...')
        loadIncidentWorkflows()
      } else {
        message.error('执行失败: ' + (e?.message || ''))
      }
    }
  }

  const getWorkflowVerification = (wf: Workflow) => {
    const latestExec = workflowExecutions.find((e) => e.workflow_id === wf.id)
    if (!latestExec?.step_executions) return null
    const verifyStep = latestExec.step_executions.find(
      (s) => s.step_type === 'verification' || s.action_type === 'verification'
    )
    if (!verifyStep) return null
    return {
      verified: verifyStep.status === 'success',
      status: verifyStep.status,
      message: verifyStep.result || verifyStep.error || '',
    }
  }

  const canResolveFromWorkflow = () => {
    if (!incident || incident.status === 'resolved' || incident.status === 'closed') return false
    return incidentWorkflows.some((w) => {
      if (w.status !== 'success') return false
      const v = getWorkflowVerification(w)
      // 安全策略：必须有真实 Verification 数据且 verified=true 才允许 Resolve
      // 无 Verification Step / 未加载 Execution 时一律禁止 Resolve
      return v ? v.verified === true : false
    })
  }

  const fetchDetail = useCallback(async () => {
    if (!id) return
    const seq = trust.beginFetch()
    setLoading(true)
    try {
      const [incRes, sigRes] = await Promise.all([
        incidentApi.get(id),
        incidentApi.signals(id),
      ])
      trust.markSuccess(seq, extractProvenance(incRes))
      setIncident(incRes)
      setSignals(sigRes.items || [])
    } catch (err: any) {
      trust.markError(seq, err?.message || '加载事件详情失败')
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

  // 加载 Evidence（切换到 evidence tab 时）
  useEffect(() => {
    if (tab !== 'evidence' || !id) return
    setEvidenceLoading(true)
    const load = async () => {
      try {
        const res = await incidentApi.evidence(id)
        setEvidence(res)
      } catch {
        setEvidence(null)
      } finally {
        setEvidenceLoading(false)
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

  const renderRecommendedActions = () => {
    const priorityColor: Record<string, string> = { P1: 'red', P2: 'orange', P3: 'blue', P4: 'default' }
    const riskColor: Record<string, string> = { low: 'green', medium: 'orange', high: 'red', critical: 'volcano' }
    const recs: any[] = []
    if (aiResult?.recommendations) {
      aiResult.recommendations.forEach((r: any, i: number) => {
        if (r.action_type) {
          recs.push({ ...r, source: 'AI', key: `ai-${i}` })
        }
      })
    }

    if (recs.length === 0) {
      return <Empty description="暂无推荐操作" />
    }

    const handleCreateAction = async (rec: any) => {
      try {
        const target = rec.target || rec.resource_name || incident?.resource_name || ''
        const action = await automationApi.createFromIncident(Number(id), {
          action_type: rec.action_type,
          target_type: rec.target_type || rec.resource_type || 'pod',
          target_name: target,
          cluster: incident?.cluster || 'local',
          namespace: incident?.namespace,
          reason: rec.reason || rec.title || rec.description,
          risk: rec.risk,
        })
        message.success(`操作 #${action.id} 已创建，等待审批`)
        loadIncidentActions()
      } catch (e: any) {
        message.error('创建操作失败: ' + (e?.message || ''))
      }
    }

    const latestExecution = (actionId: number) => {
      const execs = actionExecutions.filter((e) => e.action_id === actionId)
      return execs.length > 0 ? execs[execs.length - 1] : null
    }

    const getActionVerification = (action: AutomationAction) => {
      const exec = latestExecution(action.id)
      return parseVerification(exec?.result_json)
    }

    const canResolve = () => {
      if (!incident || incident.status === 'resolved' || incident.status === 'closed') return false
      // Action Verification
      const successActions = incidentActions.filter((a) => a.status === 'success')
      const actionVerified = successActions.some((a) => {
        const v = getActionVerification(a)
        return v && v.verified === true
      })
      if (actionVerified) return true
      // Workflow Verification
      return canResolveFromWorkflow()
    }

    const renderActionRow = (action: AutomationAction) => {
      const isSelected = selectedActionId === action.id
      const verification = getActionVerification(action)
      const exec = latestExecution(action.id)

      return (
        <div key={action.id} style={{ marginBottom: 8 }}>
          <Card
            size="small"
            style={{ borderLeft: `3px solid ${isSelected ? '#1890ff' : '#e8e8e8'}` }}
            onClick={() => setSelectedActionId(isSelected ? null : action.id)}
            hoverable
          >
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <Space>
                <Badge color={actionStatusColor[action.status] || 'default'} text={actionStatusLabel[action.status] || action.status} />
                <Tag color="blue">#{action.id}</Tag>
                <span style={{ fontWeight: 500 }}>{action.action_type}</span>
                <Tag>{action.target_type}/{action.target_name}</Tag>
                <Tag color={action.risk === 'high' || action.risk === 'critical' ? 'red' : action.risk === 'medium' ? 'orange' : 'green'}>
                  风险: {action.risk}
                </Tag>
              </Space>
              <Space>
                {action.status === 'running' && <Spin size="small" />}
                {action.status === 'pending_approval' && (
                  <>
                    <Button size="small" type="primary" onClick={(e) => { e.stopPropagation(); handleApprove(action.id) }}>审批</Button>
                    <Button size="small" danger onClick={(e) => { e.stopPropagation(); handleReject(action.id) }}>拒绝</Button>
                  </>
                )}
                {action.status === 'approved' && (
                  <Button size="small" type="primary" danger onClick={(e) => { e.stopPropagation(); handleExecute(action.id) }}>执行</Button>
                )}
                {action.status === 'success' && verification && (
                  verification.verified
                    ? <Tag color="success">✓ 验证通过</Tag>
                    : <Tag color="warning">⚠ 验证未通过</Tag>
                )}
                {action.status === 'failed' && exec?.error && (
                  <Tag color="error">失败: {exec.error.slice(0, 30)}...</Tag>
                )}
              </Space>
            </div>
            {action.reason && <div style={{ fontSize: 11, color: '#999', marginTop: 4 }}>原因: {action.reason}</div>}
            <div style={{ fontSize: 11, color: '#bbb', marginTop: 2 }}>
              创建: {dayjs(action.created_at).format('MM-DD HH:mm:ss')}
              {action.approved_at && ` · 审批: ${dayjs(action.approved_at).format('MM-DD HH:mm:ss')}`}
            </div>
          </Card>

          {isSelected && (
            <Card size="small" style={{ marginTop: 4, background: '#fafafa' }}>
              <div style={{ fontWeight: 500, marginBottom: 8 }}>执行记录</div>
              {executionsLoading ? (
                <Spin />
              ) : actionExecutions.filter((e) => e.action_id === action.id).length === 0 ? (
                <Empty description="暂无执行记录" image={Empty.PRESENTED_IMAGE_SIMPLE} />
              ) : (
                <Table
                  size="small"
                  pagination={false}
                  dataSource={actionExecutions.filter((e) => e.action_id === action.id)}
                  rowKey="id"
                  columns={[
                    { title: 'ID', dataIndex: 'id', width: 60 },
                    { title: '执行器', dataIndex: 'executor', width: 120 },
                    { title: '状态', dataIndex: 'status', width: 100, render: (s: string) => <Badge color={s === 'success' ? 'success' : s === 'failed' || s === 'timeout' ? 'error' : 'processing'} text={s} /> },
                    { title: '开始时间', dataIndex: 'started_at', width: 150, render: (t: string) => dayjs(t).format('MM-DD HH:mm:ss') },
                    { title: '耗时', dataIndex: 'duration_ms', width: 80, render: (ms: number) => ms ? `${(ms / 1000).toFixed(1)}s` : '-' },
                    { title: '结果', dataIndex: 'result_json', render: (json: string) => {
                      const v = parseVerification(json)
                      if (v) return v.verified ? <Tag color="success">验证通过</Tag> : <Tag color="warning">验证未通过: {v.message}</Tag>
                      return <span style={{ color: '#999' }}>-</span>
                    }},
                    { title: '错误', dataIndex: 'error', render: (e: string) => e ? <span style={{ color: '#ff4d4f', fontSize: 11 }}>{e.slice(0, 50)}</span> : '-' },
                  ]}
                />
              )}
            </Card>
          )}
        </div>
      )
    }

    return (
      <Space direction="vertical" style={{ width: '100%' }} size="small">
        {/* 推荐操作 */}
        {recs.length > 0 && (
          <div>
            <div style={{ fontWeight: 500, marginBottom: 8, color: '#666' }}>AI 推荐操作</div>
            {recs.map((rec) => (
              <Card key={rec.key} size="small" style={{ marginBottom: 8 }} title={
                <Space>
                  <Tag color={priorityColor[rec.priority] || 'default'}>{rec.priority}</Tag>
                  <span>{rec.title}</span>
                  <Tag>来源: {rec.source}</Tag>
                  <Tag color={riskColor[rec.risk]}>风险: {rec.risk}</Tag>
                </Space>
              } extra={
                <Button type="primary" size="small" onClick={() => handleCreateAction(rec)}>
                  创建操作
                </Button>
              }>
                <div style={{ fontSize: 12, color: '#666' }}>{rec.description}</div>
                {rec.reason && <div style={{ fontSize: 11, color: '#999', marginTop: 4 }}>原因: {rec.reason}</div>}
                <div style={{ fontSize: 11, color: '#888', marginTop: 4 }}>
                  目标: {rec.target || rec.resource_name || '-'} | 类型: {rec.action_type}
                </div>
              </Card>
            ))}
          </div>
        )}

        {/* 关联操作列表 - P0-PRODUCT-02 */}
        <div>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
            <span style={{ fontWeight: 500, color: '#666' }}>
              关联操作 ({incidentActions.length})
              {hasRunningAction && <Spin size="small" style={{ marginLeft: 8 }} />}
            </span>
            <Button size="small" onClick={loadIncidentActions}>刷新</Button>
          </div>
          {actionsLoading && incidentActions.length === 0 ? (
            <Spin />
          ) : incidentActions.length === 0 ? (
            <Empty description="暂无关联操作" image={Empty.PRESENTED_IMAGE_SIMPLE} />
          ) : (
            incidentActions.map(renderActionRow)
          )}
        </div>

        {/* P0-PRODUCT-03: 关联 Workflow 区域 */}
        <div>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8, marginTop: 16 }}>
            <span style={{ fontWeight: 500, color: '#666' }}>
              编排 Workflow ({incidentWorkflows.length})
              {hasRunningWorkflow && <Spin size="small" style={{ marginLeft: 8 }} />}
            </span>
            <Space>
              <Button size="small" onClick={loadIncidentWorkflows}>刷新</Button>
              <Button size="small" type="primary" onClick={() => setCreateWorkflowOpen(true)}>创建 Workflow</Button>
            </Space>
          </div>
          {workflowsLoading && incidentWorkflows.length === 0 ? (
            <Spin />
          ) : incidentWorkflows.length === 0 ? (
            <Empty description="暂无关联 Workflow，可创建多步骤编排处置流程" image={Empty.PRESENTED_IMAGE_SIMPLE} />
          ) : (
            incidentWorkflows.map((wf) => {
              const isWfSelected = selectedWorkflowId === wf.id
              const wfVerification = getWorkflowVerification(wf)
              const steps = wf.steps || []
              const completedSteps = steps.filter((s) => s.status === 'success').length
              const currentStep = steps.find((s) => s.status === 'running') || steps.find((s) => s.status === 'pending')
              return (
                <div key={wf.id} style={{ marginBottom: 8 }}>
                  <Card
                    size="small"
                    style={{ borderLeft: `3px solid ${isWfSelected ? '#1890ff' : '#e8e8e8'}` }}
                    onClick={() => setSelectedWorkflowId(isWfSelected ? null : wf.id)}
                    hoverable
                  >
                    <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                      <Space>
                        <Badge color={workflowStatusColor[wf.status] || 'default'} text={workflowStatusLabel[wf.status] || wf.status} />
                        <Tag color="purple">#{wf.id}</Tag>
                        <span style={{ fontWeight: 500 }}>{wf.name}</span>
                        <Tag>{steps.length} 步骤</Tag>
                        {wf.status === 'running' && currentStep && (
                          <Tag color="cyan">当前: {currentStep.name}</Tag>
                        )}
                        {wf.status === 'success' && (
                          wfVerification?.verified
                            ? <Tag color="success">✓ 验证通过</Tag>
                            : <Tag color="warning">⚠ 验证未通过</Tag>
                        )}
                      </Space>
                      <Space>
                        {wf.status === 'draft' && (
                          <Button size="small" type="primary" onClick={(e) => { e.stopPropagation(); handleWorkflowSubmit(wf.id) }}>提交审批</Button>
                        )}
                        {wf.status === 'pending_approval' && (
                          <Button size="small" type="primary" onClick={(e) => { e.stopPropagation(); handleWorkflowApprove(wf.id) }}>审批</Button>
                        )}
                        {wf.status === 'approved' && (
                          <Button size="small" type="primary" danger onClick={(e) => { e.stopPropagation(); handleWorkflowExecute(wf.id) }}>执行</Button>
                        )}
                        {wf.status === 'running' && <Spin size="small" />}
                      </Space>
                    </div>
                    {wf.description && <div style={{ fontSize: 11, color: '#999', marginTop: 4 }}>{wf.description}</div>}
                    <div style={{ fontSize: 11, color: '#bbb', marginTop: 2 }}>
                      创建: {dayjs(wf.created_at).format('MM-DD HH:mm:ss')}
                      {wf.approved_at && ` · 审批: ${dayjs(wf.approved_at).format('MM-DD HH:mm:ss')}`}
                      {wf.started_at && ` · 开始: ${dayjs(wf.started_at).format('MM-DD HH:mm:ss')}`}
                      {wf.finished_at && ` · 完成: ${dayjs(wf.finished_at).format('MM-DD HH:mm:ss')}`}
                      {wf.duration_ms && ` · 耗时: ${(wf.duration_ms / 1000).toFixed(1)}s`}
                    </div>
                    {/* 步骤进度条 */}
                    {steps.length > 0 && (
                      <div style={{ marginTop: 8, display: 'flex', alignItems: 'center', gap: 4 }}>
                        <span style={{ fontSize: 11, color: '#999' }}>进度:</span>
                        <div style={{ flex: 1, height: 6, background: '#f0f0f0', borderRadius: 3, overflow: 'hidden' }}>
                          <div
                            style={{
                              height: '100%',
                              width: `${(completedSteps / steps.length) * 100}%`,
                              background: wf.status === 'failed' ? '#ff4d4f' : '#52c41a',
                              transition: 'width 0.3s',
                            }}
                          />
                        </div>
                        <span style={{ fontSize: 11, color: '#666' }}>{completedSteps}/{steps.length}</span>
                      </div>
                    )}
                  </Card>

                  {/* Workflow Execution 详情 */}
                  {isWfSelected && (
                    <Card size="small" style={{ marginTop: 4, background: '#fafafa' }}>
                      <div style={{ fontWeight: 500, marginBottom: 8 }}>执行记录</div>
                      {workflowExecLoading ? (
                        <Spin />
                      ) : workflowExecutions.filter((e) => e.workflow_id === wf.id).length === 0 ? (
                        <Empty description="暂无执行记录" image={Empty.PRESENTED_IMAGE_SIMPLE} />
                      ) : (
                        workflowExecutions.filter((e) => e.workflow_id === wf.id).map((exec) => (
                          <div key={exec.id} style={{ marginBottom: 12 }}>
                            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 6 }}>
                              <Space>
                                <Badge color={execStatusColor[exec.status] || 'default'} text={exec.status} />
                                <span style={{ fontSize: 12 }}>Execution #{exec.id}</span>
                                {exec.trigger_type && <Tag>{exec.trigger_type}</Tag>}
                              </Space>
                              <span style={{ fontSize: 11, color: '#999' }}>
                                {exec.started_at && dayjs(exec.started_at).format('MM-DD HH:mm:ss')}
                                {exec.duration_ms && ` · ${(exec.duration_ms / 1000).toFixed(1)}s`}
                              </span>
                            </div>
                            {/* Step Executions */}
                            {exec.step_executions && exec.step_executions.length > 0 && (
                              <Timeline
                                items={exec.step_executions.map((se) => ({
                                  color: se.status === 'success' ? 'green' : se.status === 'failed' ? 'red' : se.status === 'running' ? 'blue' : 'gray',
                                  children: (
                                    <div>
                                      <div style={{ fontSize: 12, fontWeight: 500 }}>
                                        {se.step_name}
                                        <Tag style={{ marginLeft: 8 }}>{stepTypeLabel[se.step_type] || se.step_type}</Tag>
                                        {se.attempt > 1 && <Tag color="orange">重试 #{se.attempt}</Tag>}
                                      </div>
                                      <div style={{ fontSize: 11, color: '#666', marginTop: 2 }}>
                                        状态: {se.status}
                                        {se.started_at && ` · 开始: ${dayjs(se.started_at).format('HH:mm:ss')}`}
                                        {se.finished_at && ` · 完成: ${dayjs(se.finished_at).format('HH:mm:ss')}`}
                                        {se.duration_ms && ` · ${(se.duration_ms / 1000).toFixed(1)}s`}
                                      </div>
                                      {se.result && <div style={{ fontSize: 11, color: '#52c41a', marginTop: 2 }}>结果: {typeof se.result === 'string' ? se.result.slice(0, 100) : JSON.stringify(se.result).slice(0, 100)}</div>}
                                      {se.error && <div style={{ fontSize: 11, color: '#ff4d4f', marginTop: 2 }}>错误: {se.error.slice(0, 100)}</div>}
                                    </div>
                                  ),
                                }))}
                              />
                            )}
                            {exec.error && (
                              <Alert type="error" showIcon message="执行错误" description={exec.error} style={{ marginTop: 4 }} />
                            )}
                          </div>
                        ))
                      )}
                    </Card>
                  )}
                </div>
              )
            })
          )}
        </div>

        {/* 创建 Workflow Modal */}
        <Modal
          title="创建编排 Workflow"
          open={createWorkflowOpen}
          onCancel={() => setCreateWorkflowOpen(false)}
          onOk={handleCreateWorkflow}
          okText="创建"
          cancelText="取消"
        >
          <div style={{ marginBottom: 12 }}>
            <div style={{ marginBottom: 4, fontSize: 13 }}>Workflow 名称 *</div>
            <input
              style={{ width: '100%', padding: '6px 8px', border: '1px solid #d9d9d9', borderRadius: 4 }}
              placeholder="例如: Pod 恢复编排流程"
              value={newWorkflowName}
              onChange={(e) => setNewWorkflowName(e.target.value)}
            />
          </div>
          <div style={{ marginBottom: 12 }}>
            <div style={{ marginBottom: 4, fontSize: 13 }}>描述</div>
            <textarea
              style={{ width: '100%', padding: '6px 8px', border: '1px solid #d9d9d9', borderRadius: 4, minHeight: 60 }}
              placeholder="可选"
              value={newWorkflowDesc}
              onChange={(e) => setNewWorkflowDesc(e.target.value)}
            />
          </div>
          <Alert
            type="info"
            showIcon
            message="默认步骤"
            description="将自动创建 4 个步骤：系统观察 → 故障调查 → 自动化处置 → 结果验证。可在 Workflow 页面编辑步骤详情。"
          />
        </Modal>

        {/* 验证通过后一键解决 */}
        {canResolve() && (
          <Alert
            type="success"
            showIcon
            message="操作执行成功且验证通过"
            description="所有关联操作均已执行成功并通过验证，可以将此事件标记为已解决。"
            action={
              <Button type="primary" size="small" onClick={handleResolveIncident}>
                标记事件已解决
              </Button>
            }
          />
        )}

        {/* 执行成功但验证未通过 */}
        {incidentActions.some((a) => a.status === 'success') && !canResolve() && incidentActions.some((a) => {
          const v = getActionVerification(a)
          return v && v.verified === false
        }) && (
          <Alert
            type="warning"
            showIcon
            message="操作执行成功，但验证未通过"
            description="操作已执行，但验证结果表明问题可能尚未解决。请检查执行详情和验证信息。"
          />
        )}

        <Alert
          type="info"
          showIcon
          message="安全说明"
          description="所有操作必须经过人工审批后才能执行。AI/RCA 仅提供建议，不会自动执行。执行成功后需通过验证才能标记事件已解决。"
        />
      </Space>
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
          <Card title={`支撑证据 (${aiResult.evidence.length})`} size="small" style={{ marginBottom: 12 }}>
            <Space direction="vertical" style={{ width: '100%' }} size="small">
              {aiResult.evidence.map((e, i) => (
                <div key={i} style={{ padding: 8, borderLeft: '3px solid #1890ff', paddingLeft: 10, background: '#fafafa', borderRadius: 4 }}>
                  <div style={{ fontSize: 12, display: 'flex', alignItems: 'center', gap: 6, flexWrap: 'wrap' }}>
                    {e.id && (
                      <Button
                        type="link"
                        size="small"
                        style={{ padding: 0, fontWeight: 600, color: '#1890ff' }}
                        onClick={() => {
                          setTab('evidence')
                          setTimeout(() => {
                            const el = document.getElementById(`evidence-${e.id}`)
                            if (el) {
                              el.scrollIntoView({ behavior: 'smooth', block: 'center' })
                              el.style.background = '#fffbe6'
                              setTimeout(() => { el.style.background = '' }, 2000)
                            }
                          }, 300)
                        }}
                      >
                        [{e.id}]
                      </Button>
                    )}
                    <Tag color="blue">{e.type}</Tag>
                    <Tag color={e.importance === 'high' ? 'red' : e.importance === 'medium' ? 'orange' : 'default'}>
                      {e.importance}
                    </Tag>
                    <span style={{ fontSize: 13 }}>{e.description}</span>
                  </div>
                  {e.resource && <div style={{ fontSize: 11, color: '#999', marginTop: 4 }}>资源: {e.resource}</div>}
                </div>
              ))}
            </Space>
          </Card>
        )}

        {/* Missing Sources */}
        {aiResult.data_sources && (
          <Card title="数据源状态" size="small" style={{ marginBottom: 12 }}>
            <Space size="small" wrap>
              {[
                { key: 'alerts_available', label: '告警' },
                { key: 'anomalies_available', label: '异常' },
                { key: 'metrics_available', label: '指标' },
                { key: 'logs_available', label: '日志' },
                { key: 'events_available', label: '事件' },
                { key: 'topology_available', label: '拓扑' },
              ].map(({ key, label }) => (
                <Tag key={key} color={(aiResult.data_sources as any)[key] ? 'green' : 'default'}>
                  {label}: {(aiResult.data_sources as any)[key] ? '可用' : '无数据'}
                </Tag>
              ))}
            </Space>
            {aiResult.confidence < 0.5 && (
              <div style={{ marginTop: 8, fontSize: 12, color: '#faad14' }}>
                置信度较低，部分数据源缺失，根因分析仅供参考。
              </div>
            )}
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

  const renderEvidence = () => {
    if (evidenceLoading) return <div style={{ textAlign: 'center', padding: 40 }}><Spin /></div>
    if (!evidence) return <Empty description="暂无 Evidence 数据" />

    // 兼容 API 返回 null 的情况，统一转换为空数组
    const alerts = evidence.alerts || []
    const anomalies = evidence.anomalies || []
    const events = evidence.events || []
    const metrics = evidence.metrics || []
    const logs = evidence.logs || []
    const timeline = evidence.timeline || []
    const podState = evidence.pod_resource_state || null

    const totalCount = alerts.length + anomalies.length + events.length + metrics.length + logs.length

    return (
      <div>
        {/* 数据源状态 */}
        <Card size="small" title="数据源状态" style={{ marginBottom: 12 }}>
          <Space wrap>
            {Object.entries(evidence.sources).map(([key, status]) => (
              <Tag key={key} color={status === 'success' ? 'green' : 'default'}>
                {key}: {status === 'success' ? `${status}` : '无数据'}
              </Tag>
            ))}
          </Space>
        </Card>

        {/* Pod Resource State */}
        {podState && (
          <Card size="small" title="Pod 资源状态" style={{ marginBottom: 12 }}>
            <Descriptions size="small" column={2} bordered>
              <Descriptions.Item label="Phase">
                <Tag color={podState.phase === 'Running' ? 'green' : 'orange'}>
                  {podState.phase}
                </Tag>
              </Descriptions.Item>
              <Descriptions.Item label="Ready">
                <Tag color={podState.ready ? 'green' : 'red'}>
                  {podState.ready ? 'Ready' : 'Not Ready'}
                </Tag>
              </Descriptions.Item>
              <Descriptions.Item label="Restart Count">
                <span style={{ color: podState.restart_count > 3 ? '#ff4d4f' : '#333', fontWeight: 'bold' }}>
                  {podState.restart_count}
                </span>
              </Descriptions.Item>
              <Descriptions.Item label="Node">{podState.node_name || '-'}</Descriptions.Item>
              <Descriptions.Item label="Pod IP">{podState.pod_ip || '-'}</Descriptions.Item>
              <Descriptions.Item label="Host IP">{podState.host_ip || '-'}</Descriptions.Item>
              <Descriptions.Item label="Start Time" span={2}>
                {podState.start_time ? new Date(podState.start_time).toLocaleString() : '-'}
              </Descriptions.Item>
            </Descriptions>

            {/* Container 状态 */}
            <div style={{ marginTop: 12 }}>
              <div style={{ fontSize: 13, fontWeight: 'bold', marginBottom: 8 }}>Containers</div>
              <Table
                size="small"
                dataSource={podState.containers}
                rowKey="name"
                pagination={false}
                columns={[
                  { title: 'Name', dataIndex: 'name', width: 120 },
                  { title: 'Ready', dataIndex: 'ready', width: 70, render: (v: boolean) => <Tag color={v ? 'green' : 'red'}>{v ? 'Yes' : 'No'}</Tag> },
                  { title: 'Restarts', dataIndex: 'restart_count', width: 70 },
                  {
                    title: 'State', dataIndex: 'state', width: 90,
                    render: (v: string) => {
                      const color = v === 'running' ? 'green' : v === 'waiting' ? 'orange' : v === 'terminated' ? 'red' : 'default'
                      return <Tag color={color}>{v}</Tag>
                    },
                  },
                  { title: 'Reason', dataIndex: 'reason', width: 120, render: (v: string) => v ? <Tag color="red">{v}</Tag> : '-' },
                  { title: 'Exit Code', dataIndex: 'exit_code', width: 80, render: (v: number | null) => v !== null && v !== undefined ? v : '-' },
                  { title: 'Last State', dataIndex: 'last_state', width: 90 },
                  { title: 'Last Reason', dataIndex: 'last_reason', width: 120, render: (v: string) => v || '-' },
                ]}
              />
            </div>

            {/* Conditions */}
            {podState.conditions?.length > 0 && (
              <div style={{ marginTop: 12 }}>
                <div style={{ fontSize: 13, fontWeight: 'bold', marginBottom: 8 }}>Conditions</div>
                <Table
                  size="small"
                  dataSource={podState.conditions || []}
                  rowKey="type"
                  pagination={false}
                  columns={[
                    { title: 'Type', dataIndex: 'type', width: 150 },
                    { title: 'Status', dataIndex: 'status', width: 80, render: (v: string) => <Tag color={v === 'True' ? 'green' : v === 'False' ? 'red' : 'orange'}>{v}</Tag> },
                    { title: 'Reason', dataIndex: 'reason', render: (v: string) => v || '-' },
                    { title: 'Message', dataIndex: 'message', ellipsis: true, render: (v: string) => v || '-' },
                  ]}
                />
              </div>
            )}
          </Card>
        )}

        {/* Evidence 时间线 */}
        {timeline && timeline.length > 0 && (
          <Card size="small" title={`Evidence 时间线 (${timeline.length})`} style={{ marginBottom: 12 }}>
            <Timeline
              items={timeline.slice(0, 50).map((t) => ({
                color: t.severity === 'critical' ? 'red' : t.severity === 'warning' ? 'orange' : 'blue',
                children: (
                  <div>
                    <Space>
                      <Tag>{t.type}</Tag>
                      {t.severity && <Tag color={t.severity === 'critical' ? 'red' : 'orange'}>{t.severity}</Tag>}
                      {t.resource && <span style={{ fontSize: 12, color: '#666' }}>{t.resource}</span>}
                    </Space>
                    <div style={{ fontSize: 13, marginTop: 2 }}>{t.description}</div>
                    <div style={{ fontSize: 11, color: '#999', marginTop: 2 }}>{new Date(t.timestamp).toLocaleString()}</div>
                  </div>
                ),
              }))}
            />
          </Card>
        )}

        {/* Kubernetes Events */}
        {events.length > 0 && (
          <Card size="small" title={`Kubernetes Events (${events.length})`} style={{ marginBottom: 12 }}>
            {events.slice(0, 20).map((e, i) => (
              <div key={i} style={{ padding: '6px 0', borderBottom: '1px solid #f0f0f0' }}>
                <Space>
                  <Tag color={e.type === 'Warning' ? 'orange' : 'blue'}>{e.type}</Tag>
                  <Tag>{e.reason}</Tag>
                  {e.count > 1 && <Tag style={{ fontSize: 10 }}>x{e.count}</Tag>}
                </Space>
                <div style={{ fontSize: 12, color: '#333', marginTop: 2 }}>{e.message}</div>
                <div style={{ fontSize: 11, color: '#999', marginTop: 1 }}>{e.namespace}/{e.resource_name} · {new Date(e.timestamp).toLocaleString()}</div>
              </div>
            ))}
          </Card>
        )}

        {/* Metrics */}
        {metrics.length > 0 && (
          <Card size="small" title={`指标 (${metrics.length})`} style={{ marginBottom: 12 }}>
            <Table
              size="small"
              dataSource={metrics}
              rowKey={(m, i) => `${m.metric}-${i}`}
              pagination={{ pageSize: 10, size: 'small' }}
              columns={[
                { title: '指标', dataIndex: 'metric', width: 150 },
                { title: '值', dataIndex: 'value', width: 120, render: (v: number) => v != null ? Number(v).toFixed(2) : '-' },
                { title: '资源', dataIndex: 'resource', width: 150 },
                { title: '时间', dataIndex: 'timestamp', render: (t: string) => t ? new Date(t).toLocaleTimeString() : '-' },
              ]}
            />
          </Card>
        )}

        {/* Logs */}
        {logs.length > 0 && (
          <Card size="small" title={`错误日志 (${logs.length})`} style={{ marginBottom: 12 }}>
            {logs.slice(0, 20).map((l, i) => (
              <div key={i} style={{ padding: '4px 0', borderBottom: '1px solid #f5f5f5' }}>
                <Space>
                  <Tag color={l.level === 'ERROR' ? 'red' : l.level === 'WARN' ? 'orange' : 'blue'}>{l.level}</Tag>
                  <span style={{ fontSize: 11, color: '#999' }}>{l.pod}</span>
                </Space>
                <div style={{ fontSize: 12, fontFamily: 'monospace', marginTop: 2 }}>{l.message}</div>
              </div>
            ))}
          </Card>
        )}

        {/* Alerts */}
        {alerts.length > 0 && (
          <Card size="small" title={`关联告警 (${alerts.length})`} style={{ marginBottom: 12 }}>
            {alerts.map((a, i) => (
              <div key={i} style={{ padding: '6px 0', borderBottom: '1px solid #f0f0f0' }}>
                <Space>
                  <Tag color={a.severity === 'critical' ? 'red' : 'orange'}>{a.severity}</Tag>
                  <strong>{a.alertname}</strong>
                </Space>
                <div style={{ fontSize: 12, color: '#666', marginTop: 2 }}>
                  {a.namespace && `${a.namespace}/`}{a.pod || a.node || a.service || '-'} · {new Date(a.starts_at).toLocaleString()}
                </div>
              </div>
            ))}
          </Card>
        )}

        {/* Anomalies */}
        {anomalies.length > 0 && (
          <Card size="small" title={`异常 (${anomalies.length})`}>
            <Table
              size="small"
              dataSource={anomalies}
              rowKey="id"
              pagination={{ pageSize: 10, size: 'small' }}
              columns={[
                { title: '指标', dataIndex: 'metric', width: 150 },
                { title: '值', dataIndex: 'value', width: 80, render: (v: number) => v != null ? Number(v).toFixed(2) : '-' },
                { title: '基线', dataIndex: 'baseline', width: 80, render: (v: number) => v != null ? Number(v).toFixed(2) : '-' },
                { title: '级别', dataIndex: 'severity', width: 80, render: (s: string) => <Tag color={s === 'critical' ? 'red' : 'orange'}>{s}</Tag> },
                { title: '原因', dataIndex: 'reason', ellipsis: true },
              ]}
            />
          </Card>
        )}

        {totalCount === 0 && (
          <Alert
            type="warning"
            showIcon
            message="未收集到 Evidence"
            description="所有数据源均无数据。请检查 Prometheus、Elasticsearch、Kubernetes 连接是否正常。"
          />
        )}
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
            navigate(`/ai?incident_id=${id}`)
          }}
        >
          Ask AI
        </Button>
      }
    >
      <div style={{ marginBottom: 12 }}>
        <DataTrustIndicator
          status={trust.status}
          lastSuccessfulAt={trust.lastSuccessfulAt}
          fetchAgeSeconds={trust.fetchAgeSeconds}
          sourceLabel={trust.sourceLabel}
          error={trust.error}
          formatFetchAge={trust.formatFetchAge}
          formatLastSuccessful={trust.formatLastSuccessful}
            dataAgeSeconds={trust.dataAgeSeconds}
            dataTimestampAvailable={trust.dataTimestampAvailable}
            provenance={trust.provenance}
        />
      </div>
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
            { key: 'evidence', label: '证据链', children: renderEvidence() },
            { key: 'ai', label: 'AI 分析', children: renderAI() },
            { key: 'actions', label: '推荐操作', children: renderRecommendedActions() },
          ]}
        />
      </Spin>
    </Drawer>
  )
}
