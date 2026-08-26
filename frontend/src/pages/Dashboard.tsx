import { useEffect, useState, useCallback, useRef } from 'react'
import {
  Row, Col, Typography, Space, Tag, Spin, Table, Button, Progress, Empty, Alert, Card, Badge, Tooltip, Statistic, message,
} from 'antd'
import {
  ReloadOutlined, WarningOutlined, CheckCircleOutlined, ClockCircleOutlined,
  ThunderboltOutlined, FireOutlined, AlertOutlined, AppstoreOutlined,
  NodeIndexOutlined, CloudOutlined, HeartOutlined, RocketOutlined,
  ExclamationCircleOutlined, PlayCircleOutlined,
} from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import ReactECharts from 'echarts-for-react'
import { clusterApi } from '@/api/cluster'
import { k8sApi } from '@/api/kubernetes'
import { alertsApi } from '@/api/alerts'
import { metricsApi } from '@/api/metrics'
import { anomalyApi } from '@/api/anomaly'
import { incidentApi } from '@/api/incident'
import { automationApi } from '@/api/automation'
import type { AutomationAction, ActionExecution } from '@/api/automation'
import { workflowApi } from '@/api/workflow'
import type { Workflow } from '@/api/workflow'
import IncidentDetail from '@/pages/AIOps/IncidentDetail'
import type {
  Cluster, Node, Pod, Alert as AlertType, PageResult,
  Incident,
} from '@/types'
import dayjs from 'dayjs'
import { useDataTrust } from '@/hooks/useDataTrust'
import { DataTrustIndicator } from '@/components/DataTrustIndicator'
import { extractProvenance } from '@/utils/provenance'

const { Title, Text } = Typography

// Severity 排序权重
const severityWeight: Record<string, number> = { critical: 3, warning: 2, info: 1 }
const severityColor: Record<string, string> = {
  critical: '#dc2626', warning: '#d97706', info: '#2563eb',
}
const severityLabel: Record<string, string> = {
  critical: '严重', warning: '警告', info: '信息',
}
const statusColor: Record<string, string> = {
  open: '#dc2626', acknowledged: '#d97706', resolved: '#16a34a', closed: '#6b7280',
}
const statusLabel: Record<string, string> = {
  open: '待处理', acknowledged: '已确认', resolved: '已解决', closed: '已关闭',
}

// Action 状态颜色
const actionStatusColor: Record<string, string> = {
  proposed: 'default', pending_approval: 'warning', approved: 'blue',
  rejected: 'default', running: 'processing', success: 'success',
  failed: 'error', timeout: 'error', cancelled: 'default',
}

// Workflow 状态颜色
const workflowStatusColor: Record<string, string> = {
  draft: 'default', pending_approval: 'warning', approved: 'blue',
  running: 'processing', success: 'success', failed: 'error',
  cancelled: 'default', timeout: 'error',
}

export default function Dashboard() {
  const navigate = useNavigate()
  const refreshTokenRef = useRef(0)
  const pollingRef = useRef<number | null>(null)
  // P1-X.11: 防 toast storm — execution 加载失败仅提示一次
  const execErrorNotified = useRef(false)

  // === P1-X.9: Multi-Source Data Trust ===
  // 每个核心数据源独立跟踪 success/failure/lastSuccessfulAt
  const incidentTrust = useDataTrust({ source: 'mysql' })
  const actionTrust = useDataTrust({ source: 'mysql' })
  const workflowTrust = useDataTrust({ source: 'mysql' })
  const alertTrust = useDataTrust({ source: 'alertmanager' })
  const metricsTrust = useDataTrust({ source: 'prometheus' })
  const kpiTrust = useDataTrust({ source: 'mysql' })

  // 组合整体状态：ALL_SUCCESS / PARTIAL_FAILURE / ALL_FAILURE
  const allTrusts = [incidentTrust, actionTrust, workflowTrust, alertTrust, metricsTrust, kpiTrust]
  const anyFetching = allTrusts.some(t => t.status === 'fetching')
  const failedSources = allTrusts.filter(t => t.status === 'stale' || t.status === 'error')
  const successfulSources = allTrusts.filter(t => t.status === 'fresh')
  const hasAnySuccess = successfulSources.length > 0
  const dashboardTrustStatus: 'fresh' | 'fetching' | 'stale' | 'error' =
    anyFetching ? 'fetching'
    : failedSources.length === 0 && hasAnySuccess ? 'fresh'
    : hasAnySuccess ? 'stale'
    : 'error'
  const dashboardLastSuccessful = allTrusts
    .map(t => t.lastSuccessfulAt)
    .filter(Boolean)
    .sort((a, b) => b!.getTime() - a!.getTime())[0]
  const dashboardAge = dashboardLastSuccessful
    ? Math.floor((Date.now() - dashboardLastSuccessful.getTime()) / 1000)
    : undefined
  const dashboardError = failedSources.length > 0
    ? `${failedSources.length} 个数据源刷新失败: ${failedSources.map(t => t.sourceLabel).join(', ')}`
    : undefined

  // === Operations Data ===
  const [incidents, setIncidents] = useState<Incident[]>([])
  const [incidentsLoading, setIncidentsLoading] = useState(false)
  const [incidentsError, setIncidentsError] = useState('')

  const [actions, setActions] = useState<AutomationAction[]>([])
  const [actionsLoading, setActionsLoading] = useState(false)
  const [actionExecutions, setActionExecutions] = useState<ActionExecution[]>([])

  const [workflows, setWorkflows] = useState<Workflow[]>([])
  const [workflowsLoading, setWorkflowsLoading] = useState(false)

  // === KPI Totals（全量统计，使用 API total，不受 page_size 限制）===
  const [kpiTotals, setKpiTotals] = useState({
    // Incident: status × severity
    openCritical: 0, openWarning: 0, openInfo: 0,
    ackCritical: 0, ackWarning: 0, ackInfo: 0,
    // Action
    runningActions: 0, failedActions: 0,
    // Workflow
    runningWorkflows: 0, failedWorkflows: 0,
    // Alert
    criticalAlerts: 0,
  })

  // === Infrastructure Data (保留现有) ===
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [nodes, setNodes] = useState<Node[]>([])
  const [pods, setPods] = useState<Pod[]>([])
  const [alerts, setAlerts] = useState<PageResult<AlertType> | null>(null)
  const [activeAnomalyCount, setActiveAnomalyCount] = useState(0)
  const [infraLoading, setInfraLoading] = useState(false)
  const [infraError, setInfraError] = useState('')
  const [timeRange, setTimeRange] = useState('1h')
  const [cpuSeries, setCpuSeries] = useState<any[]>([])
  const [memSeries, setMemSeries] = useState<any[]>([])
  const [metricsError, setMetricsError] = useState('')

  // === UI State ===
  const [lastUpdated, setLastUpdated] = useState<dayjs.Dayjs | null>(null)
  const [selectedIncidentId, setSelectedIncidentId] = useState<number | null>(null)
  const [incidentDetailOpen, setIncidentDetailOpen] = useState(false)
  const [showInfra, setShowInfra] = useState(true)

  // === 解析 Verification ===
  const parseVerification = (resultJson?: string) => {
    if (!resultJson) return null
    try {
      const parsed = JSON.parse(resultJson)
      return parsed?.data?.verification || parsed?.verification || null
    } catch {
      return null
    }
  }

  // === 加载 Operations 数据 ===
  const loadOperationsData = useCallback(async () => {
    const token = ++refreshTokenRef.current
    setIncidentsLoading(true)
    setIncidentsError('')
    setActionsLoading(true)
    setWorkflowsLoading(true)

    // P1-X.9: 每个数据源独立 beginFetch
    const incSeq = incidentTrust.beginFetch()
    const actSeq = actionTrust.beginFetch()
    const wfSeq = workflowTrust.beginFetch()
    const kpiSeq = kpiTrust.beginFetch()
    const alertSeq = alertTrust.beginFetch()

    // 独立请求，互不影响
    const incidentPromise = incidentApi.list({ page: 1, page_size: 200 })
      .then((res) => {
        if (token === refreshTokenRef.current) {
          setIncidents(res.items || [])
          incidentTrust.markSuccess(incSeq, extractProvenance(res))
        }
      })
      .catch((err) => {
        if (token === refreshTokenRef.current) {
          setIncidentsError(err?.message || 'Incident 加载失败')
          incidentTrust.markError(incSeq, err?.message || 'Incident 加载失败')
        }
      })
      .finally(() => { if (token === refreshTokenRef.current) setIncidentsLoading(false) })

    const actionPromise = automationApi.list({ page: 1, page_size: 100 })
      .then(async (res) => {
        if (token !== refreshTokenRef.current) return
        const actionList = res.items || []
        setActions(actionList)
        actionTrust.markSuccess(actSeq, extractProvenance(res))
        // 对 success Action 加载 executions 获取 Verification（限制数量，避免 N+1 限流）
        const successActions = actionList.filter((a) => a.status === 'success').slice(0, 5)
        // P1-X.11: Error → null（不是 []），区分 API Error 与 Success+Empty
        const execResults = await Promise.allSettled(
          successActions.map((a) => automationApi.executions(a.id).catch(() => null))
        )
        if (token !== refreshTokenRef.current) return
        const allExecs: ActionExecution[] = []
        let hasExecError = false
        execResults.forEach((r) => {
          if (r.status === 'fulfilled' && Array.isArray(r.value)) {
            // Success（可能为空数组）→ 正常合并
            allExecs.push(...r.value)
          } else {
            // Error（null 或 rejected）→ 不伪造空数据，标记错误
            hasExecError = true
          }
        })
        if (hasExecError && !execErrorNotified.current) {
          message.warning('部分操作执行记录加载失败，验证状态可能不完整')
          execErrorNotified.current = true
        } else if (!hasExecError) {
          execErrorNotified.current = false
        }
        setActionExecutions(allExecs)
      })
      .catch((err: any) => {
        if (token === refreshTokenRef.current) {
          actionTrust.markError(actSeq, err?.message || 'Action 加载失败')
        }
      })
      .finally(() => { if (token === refreshTokenRef.current) setActionsLoading(false) })

    const workflowPromise = workflowApi.list({ page: 1, page_size: 100 })
      .then((res) => {
        if (token === refreshTokenRef.current) {
          setWorkflows(res.items || [])
          workflowTrust.markSuccess(wfSeq, extractProvenance(res))
        }
      })
      .catch((err: any) => {
        if (token === refreshTokenRef.current) {
          workflowTrust.markError(wfSeq, err?.message || 'Workflow 加载失败')
        }
      })
      .finally(() => { if (token === refreshTokenRef.current) setWorkflowsLoading(false) })

    await Promise.allSettled([incidentPromise, actionPromise, workflowPromise])

    // === KPI 全量统计（分批串行，避免瞬时并发触发限流）===
    // 使用 status/severity filter + page_size=1，读取 total
    const kpiRequestDefs = [
      // Incident: status=open × severity
      () => incidentApi.list({ page: 1, page_size: 1, status: 'open', severity: 'critical' }),
      () => incidentApi.list({ page: 1, page_size: 1, status: 'open', severity: 'warning' }),
      () => incidentApi.list({ page: 1, page_size: 1, status: 'open', severity: 'info' }),
      // Incident: status=acknowledged × severity
      () => incidentApi.list({ page: 1, page_size: 1, status: 'acknowledged', severity: 'critical' }),
      () => incidentApi.list({ page: 1, page_size: 1, status: 'acknowledged', severity: 'warning' }),
      () => incidentApi.list({ page: 1, page_size: 1, status: 'acknowledged', severity: 'info' }),
      // Action
      () => automationApi.list({ page: 1, page_size: 1, status: 'running' }),
      () => automationApi.list({ page: 1, page_size: 1, status: 'failed' }),
      // Workflow
      () => workflowApi.list({ page: 1, page_size: 1, status: 'running' }),
      () => workflowApi.list({ page: 1, page_size: 1, status: 'failed' }),
      // Alert critical firing
      () => alertsApi.list({ page: 1, page_size: 1, status: 'firing', severity: 'critical' }),
    ]

    // 分批串行执行，每批 3 个，间隔 150ms，避免触发限流
    const kpiResults: any[] = []
    const BATCH_SIZE = 3
    const BATCH_DELAY = 150
    for (let i = 0; i < kpiRequestDefs.length; i += BATCH_SIZE) {
      if (token !== refreshTokenRef.current) break
      const batch = kpiRequestDefs.slice(i, i + BATCH_SIZE)
      const batchResults = await Promise.allSettled(batch.map((fn) => fn()))
      kpiResults.push(...batchResults)
      // 不是最后一批时延迟
      if (i + BATCH_SIZE < kpiRequestDefs.length) {
        await new Promise((resolve) => setTimeout(resolve, BATCH_DELAY))
      }
    }

    if (token === refreshTokenRef.current) {
      const getTotal = (idx: number) => {
        const r = kpiResults[idx]
        return r && r.status === 'fulfilled' ? (r.value?.total || 0) : 0
      }
      setKpiTotals({
        openCritical: getTotal(0),
        openWarning: getTotal(1),
        openInfo: getTotal(2),
        ackCritical: getTotal(3),
        ackWarning: getTotal(4),
        ackInfo: getTotal(5),
        runningActions: getTotal(6),
        failedActions: getTotal(7),
        runningWorkflows: getTotal(8),
        failedWorkflows: getTotal(9),
        criticalAlerts: getTotal(10),
      })
      // P1-X.9: KPI 整体成功/失败判断
      const kpiFailed = kpiResults.some((r: any) => r && r.status === 'rejected')
      if (kpiFailed) {
        kpiTrust.markError(kpiSeq, '部分 KPI 统计请求失败')
      } else {
        kpiTrust.markSuccess(kpiSeq)
      }
      // Alert critical 单独跟踪（KPI 最后一个请求）
      const alertResult = kpiResults[10]
      if (alertResult && alertResult.status === 'fulfilled') {
        alertTrust.markSuccess(alertSeq)
      } else {
        alertTrust.markError(alertSeq, 'Alert critical 统计请求失败')
      }
    }

    if (token === refreshTokenRef.current) setLastUpdated(dayjs())
  }, [])

  // === 加载 Infrastructure 数据（P1-X.10 Phase 3.1: 错误不得转换为空数据）===
  const loadInfraData = useCallback(async () => {
    setInfraLoading(true)
    setInfraError('')
    try {
      const clusterList = await clusterApi.list()
      setClusters(clusterList || [])
      if (clusterList && clusterList.length > 0) {
        const firstCluster = clusterList[0].name
        // P1-X.10 Phase 3.1: 使用 Promise.allSettled 分别追踪每个 API 的成功/失败
        // 禁止 .catch(() => []) 将 API 失败转换为空数据
        const [nodeResult, podResult, alertResult, anomalyResult] = await Promise.allSettled([
          k8sApi.nodes(firstCluster),
          k8sApi.pods({ cluster: firstCluster }),
          alertsApi.list({ page: 1, page_size: 100, status: 'firing' }),
          anomalyApi.activeCount(),
        ])
        const failedSources: string[] = []
        if (nodeResult.status === 'fulfilled') {
          setNodes(nodeResult.value || [])
        } else {
          failedSources.push('Nodes')
        }
        if (podResult.status === 'fulfilled') {
          setPods(podResult.value || [])
        } else {
          failedSources.push('Pods')
        }
        if (alertResult.status === 'fulfilled') {
          setAlerts(alertResult.value)
        } else {
          failedSources.push('Alerts')
        }
        if (anomalyResult.status === 'fulfilled') {
          setActiveAnomalyCount(anomalyResult.value?.count || 0)
        } else {
          failedSources.push('Anomalies')
        }
        if (failedSources.length > 0) {
          setInfraError(`基础设施数据加载失败: ${failedSources.join(', ')}。当前显示的可能是上次成功获取的数据。`)
        }
      }
    } catch (err: any) {
      setInfraError(err?.message || '基础设施数据加载失败')
    } finally {
      setInfraLoading(false)
    }
  }, [])

  // === 加载 Metrics（P1-X.10 Phase 3.1: 部分失败不得标记为全部成功）===
  const loadMetrics = useCallback(async () => {
    if (clusters.length === 0) return
    const mSeq = metricsTrust.beginFetch()
    setMetricsError('')
    const seconds = timeRange === '1h' ? 3600 : timeRange === '6h' ? 21600 : 86400
    const end = Math.floor(Date.now() / 1000)
    const start = end - seconds
    const startStr = new Date(start * 1000).toISOString()
    const endStr = new Date(end * 1000).toISOString()
    try {
      // P1-X.10 Phase 3.1: 使用 Promise.allSettled，部分失败时标记为 error 而非全部成功
      const [cpuResult, memResult] = await Promise.allSettled([
        metricsApi.range({ query: '100 - (avg by(instance) (rate(node_cpu_seconds_total{mode="idle"}[5m])) * 100)', start: startStr, end: endStr, step: '60s' }),
        metricsApi.range({ query: '(1 - node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes) * 100', start: startStr, end: endStr, step: '60s' }),
      ])
      let hasSuccess = false
      const failedMetrics: string[] = []
      if (cpuResult.status === 'fulfilled' && cpuResult.value?.data?.result?.[0]?.values) {
        setCpuSeries(cpuResult.value.data.result[0].values)
        hasSuccess = true
      } else {
        failedMetrics.push('CPU')
      }
      if (memResult.status === 'fulfilled' && memResult.value?.data?.result?.[0]?.values) {
        setMemSeries(memResult.value.data.result[0].values)
        hasSuccess = true
      } else {
        failedMetrics.push('Memory')
      }
      if (hasSuccess && failedMetrics.length === 0) {
        metricsTrust.markSuccess(mSeq)
      } else if (hasSuccess) {
        // 部分成功：保留成功数据，标记为 stale/error
        setMetricsError(`部分监控数据加载失败: ${failedMetrics.join(', ')}`)
        metricsTrust.markError(mSeq, `部分监控数据加载失败: ${failedMetrics.join(', ')}`)
      } else {
        setMetricsError('监控数据不可用')
        metricsTrust.markError(mSeq, '监控数据不可用')
      }
    } catch (err: any) {
      setMetricsError('监控数据不可用')
      metricsTrust.markError(mSeq, err?.message || '监控数据不可用')
    }
  }, [clusters, timeRange])

  // === 初始加载 + 30s 轮询 ===
  useEffect(() => {
    loadOperationsData()
    loadInfraData()
    pollingRef.current = window.setInterval(() => {
      loadOperationsData()
    }, 30000)
    return () => {
      if (pollingRef.current) clearInterval(pollingRef.current)
    }
  }, [loadOperationsData, loadInfraData])

  useEffect(() => {
    loadMetrics()
  }, [loadMetrics])

  const handleRefresh = () => {
    loadOperationsData()
    loadInfraData()
  }

  // === 派生指标 ===
  // KPI 使用全量统计（kpiTotals），不受 page_size 限制
  const activeIncidentsTotal = kpiTotals.openCritical + kpiTotals.openWarning + kpiTotals.openInfo +
    kpiTotals.ackCritical + kpiTotals.ackWarning + kpiTotals.ackInfo
  const criticalIncidentsTotal = kpiTotals.openCritical + kpiTotals.ackCritical
  const warningIncidentsTotal = kpiTotals.openWarning + kpiTotals.ackWarning
  const infoIncidentsTotal = kpiTotals.openInfo + kpiTotals.ackInfo
  const openIncidentsTotal = kpiTotals.openCritical + kpiTotals.openWarning + kpiTotals.openInfo
  const acknowledgedIncidentsTotal = kpiTotals.ackCritical + kpiTotals.ackWarning + kpiTotals.ackInfo

  // items 仍用于 Priority List、Affected Services 等需要详情的场景
  const activeIncidents = incidents.filter((i) => i.status === 'open' || i.status === 'acknowledged')
  const criticalIncidents = activeIncidents.filter((i) => i.severity === 'critical')
  const warningIncidents = activeIncidents.filter((i) => i.severity === 'warning')
  const infoIncidents = activeIncidents.filter((i) => i.severity === 'info')
  const openIncidents = activeIncidents.filter((i) => i.status === 'open')
  const acknowledgedIncidents = activeIncidents.filter((i) => i.status === 'acknowledged')

  // New Today
  const today = dayjs().startOf('day')
  const newToday = incidents.filter((i) => dayjs(i.created_at).isAfter(today))

  // Resolved Today + MTTR（基于 end_time，如果可靠）
  const resolvedIncidents = incidents.filter((i) => (i.status === 'resolved' || i.status === 'closed') && i.end_time)
  const resolvedToday = resolvedIncidents.filter((i) => dayjs(i.end_time).isAfter(today))
  const hasReliableResolutionTime = resolvedIncidents.length > 0 && resolvedIncidents.every((i) => i.start_time && i.end_time)
  const mttrMinutes = hasReliableResolutionTime
    ? resolvedIncidents.reduce((sum, i) => sum + dayjs(i.end_time).diff(dayjs(i.start_time), 'minute'), 0) / resolvedIncidents.length
    : null

  // Priority Incidents: 按 severity + duration 排序，Top 10
  const priorityIncidents = [...activeIncidents].sort((a, b) => {
    const sw = severityWeight[b.severity] - severityWeight[a.severity]
    if (sw !== 0) return sw
    return dayjs(a.start_time).valueOf() - dayjs(b.start_time).valueOf()
  }).slice(0, 10)

  // Action 统计（KPI 使用全量 total，items 仍用于 Priority List 详情）
  const runningActionsCount = kpiTotals.runningActions
  const failedActionsCount = kpiTotals.failedActions
  const runningActions = actions.filter((a) => a.status === 'running')
  const failedActions = actions.filter((a) => a.status === 'failed' || a.status === 'timeout')
  const successActions = actions.filter((a) => a.status === 'success')
  const pendingActions = actions.filter((a) => a.status === 'pending_approval' || a.status === 'approved')

  // Verification Failed（来自真实 Execution result_json）
  const verificationFailed = actionExecutions.filter((e) => {
    const v = parseVerification(e.result_json)
    return v && v.verified === false
  })

  // Workflow 统计（KPI 使用全量 total）
  const runningWorkflowsCount = kpiTotals.runningWorkflows
  const failedWorkflowsCount = kpiTotals.failedWorkflows
  const runningWorkflows = workflows.filter((w) => w.status === 'running')
  const failedWorkflows = workflows.filter((w) => w.status === 'failed' || w.status === 'timeout')
  const successWorkflows = workflows.filter((w) => w.status === 'success')
  const pendingWorkflows = workflows.filter((w) => w.status === 'pending_approval' || w.status === 'approved')

  // Affected Services / Namespaces
  const serviceCount: Record<string, number> = {}
  const namespaceCount: Record<string, number> = {}
  activeIncidents.forEach((i) => {
    if (i.service) serviceCount[i.service] = (serviceCount[i.service] || 0) + 1
    if (i.namespace) namespaceCount[i.namespace] = (namespaceCount[i.namespace] || 0) + 1
  })
  const topServices = Object.entries(serviceCount).sort((a, b) => b[1] - a[1]).slice(0, 8)
  const topNamespaces = Object.entries(namespaceCount).sort((a, b) => b[1] - a[1]).slice(0, 8)

  // 获取 Incident 关联的 Action 状态（用于 Priority List）
  const getIncidentActions = (incidentId: number) => actions.filter((a) => a.incident_id === incidentId)
  const getIncidentActionStatus = (incidentId: number) => {
    const ia = getIncidentActions(incidentId)
    if (ia.some((a) => a.status === 'running')) return { text: '执行中', color: 'processing' }
    if (ia.some((a) => a.status === 'failed' || a.status === 'timeout')) return { text: '执行失败', color: 'error' }
    if (ia.some((a) => a.status === 'success')) {
      // 检查 Verification
      const successAction = ia.find((a) => a.status === 'success')
      if (successAction) {
        const exec = actionExecutions.find((e) => e.action_id === successAction.id)
        const v = parseVerification(exec?.result_json)
        if (v && v.verified === true) return { text: '已验证', color: 'success' }
        if (v && v.verified === false) return { text: '验证失败', color: 'error' }
      }
      return { text: '执行成功', color: 'success' }
    }
    if (ia.some((a) => a.status === 'pending_approval')) return { text: '待审批', color: 'warning' }
    if (ia.length > 0) return { text: '已创建', color: 'default' }
    return { text: '-', color: 'default' }
  }

  // RCA 状态（基于 Incident.root_cause + confidence）
  const getRcaStatus = (incident: Incident) => {
    if (incident.root_cause && incident.confidence && incident.confidence > 0) {
      return { text: `RCA ${Math.round(incident.confidence * 100)}%`, color: 'success' }
    }
    return { text: '-', color: 'default' }
  }

  // Duration 显示
  const formatDuration = (startTime: string) => {
    const diff = dayjs().diff(dayjs(startTime), 'minute')
    if (diff < 60) return `${diff}m`
    if (diff < 1440) return `${Math.floor(diff / 60)}h ${diff % 60}m`
    return `${Math.floor(diff / 1440)}d ${Math.floor((diff % 1440) / 60)}h`
  }

  // === Infrastructure 派生指标（保留现有） ===
  const runningPods = pods.filter((p) => p.status === 'Running').length
  const failedPods = pods.filter((p) => p.status === 'Failed' || p.status === 'CrashLoopBackOff' || p.status === 'Error').length
  const firingCount = alerts?.total || 0
  const criticalAlerts = alerts?.items?.filter((a) => a.severity === 'critical') || []
  const totalPods = pods.length || 1
  const podHealthRatio = (runningPods / totalPods) * 100
  const alertPenalty = criticalAlerts.length > 0 ? 10 : 0
  const systemHealth = Math.max(0, Math.min(100, podHealthRatio - alertPenalty))
  const healthColor = systemHealth >= 95 ? '#16a34a' : systemHealth >= 80 ? '#d97706' : '#dc2626'

  // CPU/Memory 图表配置
  const chartOption = {
    tooltip: { trigger: 'axis', backgroundColor: '#fff', borderColor: '#e5e7eb', textStyle: { color: '#111827', fontSize: 12 } },
    legend: { data: ['CPU %', 'Memory %'], textStyle: { color: '#6b7280', fontSize: 11 }, top: 0 },
    grid: { left: 40, right: 16, top: 28, bottom: 24 },
    xAxis: { type: 'category', data: cpuSeries.map((p) => dayjs(p[0] * 1000).format('HH:mm')), axisLine: { lineStyle: { color: '#e5e7eb' } }, axisLabel: { color: '#6b7280', fontSize: 10 } },
    yAxis: { type: 'value', max: 100, axisLine: { show: false }, splitLine: { lineStyle: { color: '#f3f4f6' } }, axisLabel: { color: '#6b7280', fontSize: 10, formatter: '{value}%' } },
    series: [
      { name: 'CPU %', type: 'line', smooth: true, data: cpuSeries.map((p) => parseFloat(p[1]).toFixed(1)), lineStyle: { color: '#3b82f6', width: 2 }, areaStyle: { color: 'rgba(59,130,246,0.1)' }, showSymbol: false },
      { name: 'Memory %', type: 'line', smooth: true, data: memSeries.map((p) => parseFloat(p[1]).toFixed(1)), lineStyle: { color: '#8b5cf6', width: 2 }, areaStyle: { color: 'rgba(139,92,246,0.1)' }, showSymbol: false },
    ],
  }

  // Priority Incident 表格列
  const priorityColumns = [
    {
      title: '严重度', dataIndex: 'severity', width: 80,
      render: (v: string) => <Tag color={severityColor[v]} style={{ margin: 0 }}>{severityLabel[v] || v}</Tag>,
    },
    {
      title: '事件', dataIndex: 'title', width: 200, ellipsis: true,
      render: (v: string, record: Incident) => (
        <a onClick={() => { setSelectedIncidentId(record.id); setIncidentDetailOpen(true) }} style={{ color: '#111827' }}>
          {v}
        </a>
      ),
    },
    {
      title: '状态', dataIndex: 'status', width: 80,
      render: (v: string) => <Badge color={statusColor[v]} text={statusLabel[v] || v} />,
    },
    {
      title: '服务', dataIndex: 'service', width: 120, ellipsis: true,
      render: (v: string) => v || <Text type="secondary">-</Text>,
    },
    {
      title: '集群/命名空间', width: 140, ellipsis: true,
      render: (_: any, record: Incident) => (
        <Text type="secondary" style={{ fontSize: 12 }}>{record.cluster || '-'} / {record.namespace || '-'}</Text>
      ),
    },
    {
      title: '持续时间', dataIndex: 'start_time', width: 90,
      render: (v: string) => <Text strong>{formatDuration(v)}</Text>,
    },
    {
      title: 'RCA', width: 90,
      render: (_: any, record: Incident) => {
        const r = getRcaStatus(record)
        return <Tag color={r.color} style={{ margin: 0 }}>{r.text}</Tag>
      },
    },
    {
      title: '自动化', width: 90,
      render: (_: any, record: Incident) => {
        const s = getIncidentActionStatus(record.id)
        return <Tag color={s.color} style={{ margin: 0 }}>{s.text}</Tag>
      },
    },
  ]

  return (
    <div style={{ padding: '16px 24px', maxWidth: '100%' }}>
      {/* Header */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
        <div>
          <Title level={4} style={{ margin: 0 }}>
            <ThunderboltOutlined style={{ color: '#3b82f6', marginRight: 8 }} />
            Operations Command Center
          </Title>
          <Text type="secondary" style={{ fontSize: 12 }}>
            当前系统运维态势 {lastUpdated && `· 最后更新 ${lastUpdated.format('HH:mm:ss')}`}
          </Text>
        </div>
        <Button icon={<ReloadOutlined />} onClick={handleRefresh} loading={incidentsLoading || actionsLoading}>
          刷新
        </Button>
      </div>

      {/* P1-X.9: Multi-Source Data Trust Indicator */}
      <div style={{ marginBottom: 12 }}>
        <DataTrustIndicator
          status={dashboardTrustStatus}
          lastSuccessfulAt={dashboardLastSuccessful}
          fetchAgeSeconds={dashboardAge}
          sourceLabel="Multiple Sources"
          error={dashboardError}
          formatFetchAge={() => dashboardAge !== undefined ? (dashboardAge < 60 ? `${dashboardAge}s` : `${Math.floor(dashboardAge / 60)}m ${dashboardAge % 60}s`) : 'N/A'}
          formatLastSuccessful={() => dashboardLastSuccessful ? dashboardLastSuccessful.toLocaleTimeString('zh-CN', { hour12: false }) : 'Never'}
          dataTimestampAvailable={false}
        />
      </div>

      {/* Partial Failure Alert */}
      {dashboardTrustStatus === 'stale' && dashboardError && (
        <Alert
          type="warning"
          showIcon
          message="部分数据源刷新失败"
          description={`${dashboardError}。当前显示的是最近一次成功获取的数据，可能已过期。`}
          style={{ marginBottom: 12 }}
        />
      )}
      {dashboardTrustStatus === 'error' && (
        <Alert
          type="error"
          showIcon
          message="所有数据源刷新失败"
          description="无法获取当前运维态势数据，请检查后端服务和外部连接状态。"
          style={{ marginBottom: 12 }}
        />
      )}

      {/* KPI Row 1 */}
      <Row gutter={[12, 12]} style={{ marginBottom: 12 }}>
        <Col xs={12} sm={12} md={6}>
          <Card size="small" style={{ borderLeft: `3px solid ${criticalIncidentsTotal > 0 ? '#dc2626' : '#16a34a'}` }}>
            <Statistic
              title={<span><AlertOutlined style={{ marginRight: 4 }} />活跃事件</span>}
              value={activeIncidentsTotal}
              valueStyle={{ fontSize: 28, color: criticalIncidentsTotal > 0 ? '#dc2626' : '#111827' }}
            />
            <div style={{ marginTop: 4, fontSize: 11 }}>
              <Tag color="red" style={{ margin: 0, fontSize: 10 }}>严重 {criticalIncidentsTotal}</Tag>
              <Tag color="orange" style={{ margin: '0 4px', fontSize: 10 }}>警告 {warningIncidentsTotal}</Tag>
              <Tag color="blue" style={{ margin: 0, fontSize: 10 }}>信息 {infoIncidentsTotal}</Tag>
            </div>
          </Card>
        </Col>
        <Col xs={12} sm={12} md={6}>
          <Card size="small" style={{ borderLeft: '3px solid #d97706' }}>
            <Statistic
              title={<span><ClockCircleOutlined style={{ marginRight: 4 }} />待处理 / 已确认</span>}
              value={openIncidentsTotal}
              suffix={`/ ${acknowledgedIncidentsTotal}`}
              valueStyle={{ fontSize: 28, color: '#d97706' }}
            />
            <div style={{ marginTop: 4, fontSize: 11, color: '#6b7280' }}>
              待处理 {openIncidentsTotal} · 已确认 {acknowledgedIncidentsTotal}
            </div>
          </Card>
        </Col>
        <Col xs={12} sm={12} md={6}>
          <Card size="small" style={{ borderLeft: '3px solid #3b82f6' }}>
            <Statistic
              title={<span><PlayCircleOutlined style={{ marginRight: 4 }} />今日新增</span>}
              value={newToday.length}
              valueStyle={{ fontSize: 28, color: '#3b82f6' }}
            />
            <div style={{ marginTop: 4, fontSize: 11, color: '#6b7280' }}>
              今日已解决 {resolvedToday.length > 0 ? resolvedToday.length : 'N/A'}
            </div>
          </Card>
        </Col>
        <Col xs={12} sm={12} md={6}>
          <Card size="small" style={{ borderLeft: `3px solid ${verificationFailed.length > 0 ? '#dc2626' : '#16a34a'}` }}>
            <Statistic
              title={<span><RocketOutlined style={{ marginRight: 4 }} />自动化运行中</span>}
              value={runningActionsCount + runningWorkflowsCount}
              valueStyle={{ fontSize: 28, color: '#2563eb' }}
            />
            <div style={{ marginTop: 4, fontSize: 11 }}>
              <Tag color="error" style={{ margin: 0, fontSize: 10 }}>失败 {failedActionsCount + failedWorkflowsCount}</Tag>
              {verificationFailed.length > 0 && (
                <Tag color="error" style={{ marginLeft: 4, fontSize: 10 }}>验证失败 {verificationFailed.length}</Tag>
              )}
            </div>
          </Card>
        </Col>
      </Row>

      {/* MTTR Row */}
      {mttrMinutes !== null && (
        <Row gutter={[12, 12]} style={{ marginBottom: 12 }}>
          <Col xs={24} sm={12} md={6}>
            <Card size="small">
              <Statistic
                title="平均恢复时间 (MTTR)"
                value={mttrMinutes.toFixed(1)}
                suffix="分钟"
                valueStyle={{ fontSize: 20 }}
              />
              <div style={{ fontSize: 11, color: '#6b7280' }}>基于 {resolvedIncidents.length} 个已解决事件</div>
            </Card>
          </Col>
        </Row>
      )}

      {/* Priority Incidents */}
      <Card
        size="small"
        style={{ marginBottom: 12 }}
        title={
          <Space>
            <FireOutlined style={{ color: '#dc2626' }} />
            <span>优先级事件 (Top {priorityIncidents.length})</span>
          </Space>
        }
        extra={
          <Button type="link" size="small" onClick={() => navigate('/aiops/incidents')}>查看全部</Button>
        }
      >
        {incidentsLoading && priorityIncidents.length === 0 ? (
          <div style={{ textAlign: 'center', padding: 24 }}><Spin /></div>
        ) : incidentsError ? (
          <Alert type="error" message={incidentsError} showIcon />
        ) : priorityIncidents.length === 0 ? (
          <Empty description="暂无活跃事件" image={Empty.PRESENTED_IMAGE_SIMPLE} />
        ) : (
          <Table
            dataSource={priorityIncidents}
            columns={priorityColumns}
            rowKey="id"
            size="small"
            pagination={false}
            scroll={{ x: 900 }}
          />
        )}
      </Card>

      {/* Automation + Workflow Health */}
      <Row gutter={[12, 12]} style={{ marginBottom: 12 }}>
        <Col xs={24} lg={12}>
          <Card
            size="small"
            title={<Space><AppstoreOutlined />自动化健康</Space>}
            extra={<Button type="link" size="small" onClick={() => navigate('/automation/actions')}>管理</Button>}
          >
            {actionsLoading ? <Spin /> : (
              <Row gutter={[8, 8]}>
                <Col span={6}><Statistic title="运行中" value={runningActionsCount} valueStyle={{ fontSize: 20, color: '#2563eb' }} /></Col>
                <Col span={6}><Statistic title="失败" value={failedActionsCount} valueStyle={{ fontSize: 20, color: '#dc2626' }} /></Col>
                <Col span={6}><Statistic title="成功" value={successActions.length} valueStyle={{ fontSize: 20, color: '#16a34a' }} /></Col>
                <Col span={6}><Statistic title="待审批" value={pendingActions.length} valueStyle={{ fontSize: 20, color: '#d97706' }} /></Col>
              </Row>
            )}
            {verificationFailed.length > 0 && (
              <Alert
                type="error"
                showIcon
                style={{ marginTop: 8 }}
                message={`${verificationFailed.length} 个操作执行成功但验证失败`}
              />
            )}
          </Card>
        </Col>
        <Col xs={24} lg={12}>
          <Card
            size="small"
            title={<Space><NodeIndexOutlined />工作流健康</Space>}
            extra={<Button type="link" size="small" onClick={() => navigate('/automation/workflows')}>管理</Button>}
          >
            {workflowsLoading ? <Spin /> : (
              <Row gutter={[8, 8]}>
                <Col span={6}><Statistic title="运行中" value={runningWorkflowsCount} valueStyle={{ fontSize: 20, color: '#2563eb' }} /></Col>
                <Col span={6}><Statistic title="失败" value={failedWorkflowsCount} valueStyle={{ fontSize: 20, color: '#dc2626' }} /></Col>
                <Col span={6}><Statistic title="成功" value={successWorkflows.length} valueStyle={{ fontSize: 20, color: '#16a34a' }} /></Col>
                <Col span={6}><Statistic title="待审批" value={pendingWorkflows.length} valueStyle={{ fontSize: 20, color: '#d97706' }} /></Col>
              </Row>
            )}
          </Card>
        </Col>
      </Row>

      {/* Affected Services / Namespaces */}
      {topServices.length > 0 && (
        <Card size="small" style={{ marginBottom: 12 }} title={<Space><CloudOutlined />受影响服务 / 命名空间</Space>}>
          <Row gutter={[24, 12]}>
            <Col xs={24} md={12}>
              <Text strong style={{ fontSize: 12 }}>服务</Text>
              <div style={{ marginTop: 8 }}>
                {topServices.map(([svc, count]) => (
                  <div key={svc} style={{ display: 'flex', justifyContent: 'space-between', padding: '4px 0', borderBottom: '1px solid #f3f4f6' }}>
                    <Text style={{ fontSize: 12 }}>{svc}</Text>
                    <Tag color={count >= 3 ? 'red' : count >= 2 ? 'orange' : 'blue'} style={{ margin: 0 }}>{count} 事件</Tag>
                  </div>
                ))}
              </div>
            </Col>
            <Col xs={24} md={12}>
              <Text strong style={{ fontSize: 12 }}>命名空间</Text>
              <div style={{ marginTop: 8 }}>
                {topNamespaces.map(([ns, count]) => (
                  <div key={ns} style={{ display: 'flex', justifyContent: 'space-between', padding: '4px 0', borderBottom: '1px solid #f3f4f6' }}>
                    <Text style={{ fontSize: 12 }}>{ns}</Text>
                    <Tag color={count >= 3 ? 'red' : count >= 2 ? 'orange' : 'blue'} style={{ margin: 0 }}>{count} 事件</Tag>
                  </div>
                ))}
              </div>
            </Col>
          </Row>
        </Card>
      )}

      {/* Infrastructure Monitoring (次要区域) */}
      <Card
        size="small"
        title={
          <Space>
            <HeartOutlined style={{ color: healthColor }} />
            基础设施监控
            <Tag color={healthColor === '#16a34a' ? 'success' : healthColor === '#d97706' ? 'warning' : 'error'} style={{ marginLeft: 8 }}>
              系统健康 {systemHealth.toFixed(0)}%
            </Tag>
          </Space>
        }
        extra={
          <Button type="link" size="small" onClick={() => setShowInfra(!showInfra)}>
            {showInfra ? '收起' : '展开'}
          </Button>
        }
      >
        {showInfra && (
          <>
            {infraError && (
              <Alert type="warning" message={infraError} showIcon style={{ marginBottom: 12 }} />
            )}
            <Row gutter={[12, 12]} style={{ marginBottom: 12 }}>
              <Col xs={12} sm={6}><Statistic title="节点" value={nodes.length} prefix={<NodeIndexOutlined />} /></Col>
              <Col xs={12} sm={6}><Statistic title="Pod" value={pods.length} prefix={<AppstoreOutlined />} suffix={`运行 ${runningPods}`} /></Col>
              <Col xs={12} sm={6}><Statistic title="Firing 告警" value={firingCount} prefix={<AlertOutlined />} valueStyle={{ color: firingCount > 0 ? '#dc2626' : undefined }} /></Col>
              <Col xs={12} sm={6}><Statistic title="活跃异常" value={activeAnomalyCount} prefix={<WarningOutlined />} /></Col>
            </Row>
            <Row gutter={[12, 12]}>
              <Col xs={24} lg={16}>
                <div style={{ background: '#fff', borderRadius: 6, padding: 8 }}>
                  {metricsError ? (
                    <Alert type="warning" message={metricsError} showIcon />
                  ) : (
                    <ReactECharts option={chartOption} style={{ height: 200 }} notMerge />
                  )}
                </div>
              </Col>
              <Col xs={24} lg={8}>
                <Text strong style={{ fontSize: 12 }}>不健康 Pod ({failedPods})</Text>
                <div style={{ maxHeight: 180, overflowY: 'auto', marginTop: 4 }}>
                  {failedPods === 0 ? (
                    <Empty description="全部正常" image={Empty.PRESENTED_IMAGE_SIMPLE} style={{ padding: 16 }} />
                  ) : (
                    pods.filter((p) => p.status !== 'Running' && p.status !== 'Succeeded' && p.status !== 'Completed').slice(0, 10).map((p) => (
                      <div key={p.name} style={{ display: 'flex', justifyContent: 'space-between', padding: '4px 0', borderBottom: '1px solid #f3f4f6', fontSize: 12 }}>
                        <Text ellipsis style={{ maxWidth: 140 }}>{p.name}</Text>
                        <Tag color="error" style={{ margin: 0, fontSize: 10 }}>{p.status}</Tag>
                      </div>
                    ))
                  )}
                </div>
              </Col>
            </Row>
          </>
        )}
      </Card>

      {/* Incident Detail Drawer */}
      <IncidentDetail
        id={selectedIncidentId}
        open={incidentDetailOpen}
        onClose={() => setIncidentDetailOpen(false)}
        onChanged={() => { loadOperationsData(); }}
      />
    </div>
  )
}
