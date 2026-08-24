import { useEffect, useState, useMemo, useRef } from 'react'
import {
  Drawer, Descriptions, Tag, Tabs, Button, Spin, Modal, message, Space, Empty, Typography, Select, InputNumber, Input, Alert,
} from 'antd'
import {
  ReloadOutlined, SyncOutlined, FileTextOutlined,
  ExclamationCircleOutlined, InfoCircleOutlined, CodeOutlined,
  ApiOutlined, PlayCircleOutlined,
} from '@ant-design/icons'
import { automationApi } from '@/api/automation'
import { k8sApi } from '@/api/kubernetes'
import { usePermission } from '@/utils/permission'
import type { Pod, PodDetail as PodDetailType, Container } from '@/types'
import dayjs from 'dayjs'
import { useDataTrust } from '@/hooks/useDataTrust'
import { DataTrustIndicator } from '@/components/DataTrustIndicator'

const { Text, Paragraph } = Typography

interface PodDetailProps {
  pod: Pod | null
  cluster: string
  open: boolean
  onClose: () => void
  onRestarted?: () => void
}

interface K8sEvent {
  type: string
  reason: string
  message: string
  count?: number
  first_timestamp?: string
  last_timestamp?: string
}

export default function PodDetail({ pod, cluster, open, onClose, onRestarted }: PodDetailProps) {
  const canWrite = usePermission('cluster', 'write')
  const [detail, setDetail] = useState<PodDetailType | null>(null)
  const [logs, setLogs] = useState<string>('')
  const [events, setEvents] = useState<K8sEvent[]>([])
  const [logsLoading, setLogsLoading] = useState(false)
  const [eventsLoading, setEventsLoading] = useState(false)
  const [detailLoading, setDetailLoading] = useState(false)

  // P1-X.9: 统一 Data Trust（PodDetail 来自 Kubernetes API）
  const trust = useDataTrust({ source: 'kubernetes' })
  const [activeTab, setActiveTab] = useState('info')
  const [selectedContainer, setSelectedContainer] = useState<string>('')
  const [tailLines, setTailLines] = useState(200)
  const [logFilter, setLogFilter] = useState('')
  const [autoRefresh, setAutoRefresh] = useState(true)
  const [showTimestamp, setShowTimestamp] = useState(true)
  const [logLevel, setLogLevel] = useState<string>('all')
  const [logsError, setLogsError] = useState<string>('')
  const logEndRef = useRef<HTMLDivElement>(null)

  // Resizable Drawer 相关
  const DEFAULT_DRAWER_WIDTH = 720
  const MIN_DRAWER_WIDTH = 480
  const [drawerWidth, setDrawerWidth] = useState(DEFAULT_DRAWER_WIDTH)
  const isResizingRef = useRef(false)
  const resizeStartXRef = useRef(0)
  const resizeStartWidthRef = useRef(0)

  // 终端相关 state
  const [terminalCmd, setTerminalCmd] = useState('sh')
  const [terminalArgs, setTerminalArgs] = useState('-c')
  const [terminalInput, setTerminalInput] = useState('ls -la /')
  const [terminalOutput, setTerminalOutput] = useState('')
  const [terminalLoading, setTerminalLoading] = useState(false)
  const [terminalHistory, setTerminalHistory] = useState<Array<{ cmd: string; output: string; exitCode: number }>>([])

  const fetchDetail = async () => {
    if (!pod) return
    const seq = trust.beginFetch()
    setDetailLoading(true)
    try {
      const res = await k8sApi.podDetail(pod.name, { cluster, namespace: pod.namespace })
      trust.markSuccess(seq)
      setDetail(res)
      if (res.containers && res.containers.length > 0) {
        setSelectedContainer(res.containers[0].name)
      }
    } catch (err: any) {
      trust.markError(seq, err?.message || '加载 Pod 详情失败')
      setDetail(null)
    } finally {
      setDetailLoading(false)
    }
  }

  const fetchLogs = async () => {
    if (!pod) return
    setLogsLoading(true)
    setLogsError('')
    try {
      const res: any = await automationApi.podLogs(pod.name, {
        cluster,
        namespace: pod.namespace,
        container: selectedContainer || undefined,
        tail: tailLines,
        timestamps: true,
      })
      setLogs(res?.logs || '')
    } catch (err: any) {
      setLogs('')
      setLogsError(err?.message || '获取日志失败')
    } finally {
      setLogsLoading(false)
    }
  }

  const fetchEvents = async () => {
    if (!pod) return
    setEventsLoading(true)
    try {
      const res: any = await automationApi.podEvents(pod.name, { cluster, namespace: pod.namespace })
      setEvents(res?.events || res || [])
    } catch {
      setEvents([])
    } finally {
      setEventsLoading(false)
    }
  }

  useEffect(() => {
    if (open && pod) {
      setActiveTab('info')
      setLogs('')
      setEvents([])
      setDetail(null)
      fetchDetail()
    }
  }, [open, pod])

  useEffect(() => {
    if (activeTab === 'logs' && open && pod) fetchLogs()
    if (activeTab === 'events' && open && pod && events.length === 0 && !eventsLoading) fetchEvents()
  }, [activeTab, open, pod])

  // 容器切换时自动刷新日志
  useEffect(() => {
    if (activeTab === 'logs' && open && pod && selectedContainer) {
      fetchLogs()
    }
  }, [selectedContainer])

  // 日志自动轮询（在 logs tab 且开启自动刷新时，每5秒刷新一次）
  useEffect(() => {
    if (!open || !pod || activeTab !== 'logs' || !autoRefresh) return
    const timer = setInterval(() => {
      fetchLogs()
    }, 5000)
    return () => clearInterval(timer)
  }, [open, pod, activeTab, autoRefresh, selectedContainer, tailLines])

  // 解析日志行：Kubernetes timestamps=true 格式 "2026-08-21T06:09:55.479950Z 日志内容"
  const parsedLogs = useMemo(() => {
    if (!logs) return []
    const lines = logs.split('\n').filter((l) => l.trim() !== '')
    const tsRegex = /^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d+Z)\s+(.*)$/
    // 优先匹配带方括号的级别 [error]，其次匹配行首/空格后的级别，避免普通文本误判
    const bracketLevelRegex = /\[(ERROR|ERR|FATAL|PANIC|WARN(?:ING)?|NOTICE|INFO|DEBUG|TRACE)\]/i
    const plainLevelRegex = /(?:^|\s)(ERROR|ERR|FATAL|PANIC|WARN(?:ING)?|NOTICE|INFO|DEBUG|TRACE)(?:\s|$)/i
    // 级别归一化
    const levelNormalize: Record<string, string> = {
      ERR: 'ERROR', ERROR: 'ERROR',
      WARN: 'WARN', WARNING: 'WARN',
      NOTICE: 'NOTICE',
      INFO: 'INFO',
      DEBUG: 'DEBUG', TRACE: 'TRACE',
      FATAL: 'FATAL', PANIC: 'PANIC',
    }
    return lines.map((line) => {
      const m = line.match(tsRegex)
      let timestamp = ''
      let message = line
      if (m) {
        timestamp = m[1]
        message = m[2]
      }
      // 优先匹配方括号格式 [error]，其次匹配普通格式
      let lm = message.match(bracketLevelRegex)
      if (!lm) lm = message.match(plainLevelRegex)
      const rawLevel = lm ? lm[1].toUpperCase() : ''
      const level = rawLevel ? (levelNormalize[rawLevel] || rawLevel) : ''
      return { timestamp, level, message, raw: line }
    })
  }, [logs])

  // 过滤日志：级别 + 关键词
  const filteredLogLines = useMemo(() => {
    let result = parsedLogs
    if (logLevel !== 'all') {
      result = result.filter((l) => l.level === logLevel)
    }
    if (logFilter.trim()) {
      const kw = logFilter.toLowerCase()
      result = result.filter((l) => l.message.toLowerCase().includes(kw))
    }
    return result
  }, [parsedLogs, logLevel, logFilter])

  // 兼容旧的 filteredLogs 字符串（用于自动滚动依赖）
  const filteredLogs = useMemo(() => filteredLogLines.map((l) => l.raw).join('\n'), [filteredLogLines])

  // 日志自动滚动到底部
  useEffect(() => {
    if (logEndRef.current) {
      logEndRef.current.scrollIntoView({ behavior: 'smooth' })
    }
  }, [filteredLogs])

  // Drawer 打开时重置为默认宽度
  useEffect(() => {
    if (open) {
      setDrawerWidth(DEFAULT_DRAWER_WIDTH)
    }
  }, [open])

  // Resize Handle 指针事件
  const handleResizeStart = (e: React.PointerEvent) => {
    e.preventDefault()
    isResizingRef.current = true
    resizeStartXRef.current = e.clientX
    resizeStartWidthRef.current = drawerWidth
    document.body.style.userSelect = 'none'
    document.body.style.cursor = 'col-resize'
    document.addEventListener('pointermove', handleResizeMove)
    document.addEventListener('pointerup', handleResizeEnd)
  }

  const handleResizeMove = (e: PointerEvent) => {
    if (!isResizingRef.current) return
    // Drawer 位于右侧：向左拖变宽，向右拖变窄
    const deltaX = resizeStartXRef.current - e.clientX
    let newWidth = resizeStartWidthRef.current + deltaX
    // 最大宽度：min(1200px, 100vw - 24px)
    const maxWidth = Math.min(1200, window.innerWidth - 24)
    newWidth = Math.max(MIN_DRAWER_WIDTH, Math.min(newWidth, maxWidth))
    setDrawerWidth(newWidth)
  }

  const handleResizeEnd = () => {
    isResizingRef.current = false
    document.body.style.userSelect = ''
    document.body.style.cursor = ''
    document.removeEventListener('pointermove', handleResizeMove)
    document.removeEventListener('pointerup', handleResizeEnd)
  }

  // 组件卸载时清理 resize 事件
  useEffect(() => {
    return () => {
      document.removeEventListener('pointermove', handleResizeMove)
      document.removeEventListener('pointerup', handleResizeEnd)
      document.body.style.userSelect = ''
      document.body.style.cursor = ''
    }
  }, [])

  const handleRestart = () => {
    if (!pod) return
    Modal.confirm({
      title: '重启 Pod',
      content: `确认重启 Pod "${pod.namespace}/${pod.name}"？这将删除当前 Pod 并由控制器重建。`,
      okText: '确认重启',
      okType: 'danger',
      cancelText: '取消',
      onOk: async () => {
        try {
          await automationApi.restartPod(pod.name, { cluster, namespace: pod.namespace, confirm: true })
          message.success('Pod 重启指令已发送')
          onRestarted?.()
          onClose()
        } catch (err: any) {
          message.error(err?.message || '重启失败')
        }
      },
    })
  }

  const handleExec = async () => {
    if (!pod || !terminalInput.trim()) {
      message.warning('请输入要执行的命令')
      return
    }
    setTerminalLoading(true)
    try {
      const command = [terminalCmd]
      if (terminalArgs) command.push(terminalArgs)
      command.push(terminalInput)
      const res = await automationApi.podExec(pod.name, {
        cluster,
        namespace: pod.namespace,
        container: selectedContainer || undefined,
        command,
        confirm: true,
      })
      const output = res.stdout + (res.stderr ? `\n[stderr]\n${res.stderr}` : '')
      setTerminalOutput(output)
      setTerminalHistory((prev) => [...prev, { cmd: terminalInput, output, exitCode: res.exit_code }].slice(-20))
    } catch (err: any) {
      const errMsg = err?.message || '执行失败'
      setTerminalOutput(`[ERROR] ${errMsg}`)
      message.error(errMsg)
    } finally {
      setTerminalLoading(false)
    }
  }

  const statusColor = (status: string) => {
    if (status === 'Running') return 'success'
    if (status === 'Pending') return 'warning'
    if (status === 'Failed' || status === 'Unknown') return 'error'
    return 'default'
  }

  const containerStateColor = (state: string) => {
    if (state.startsWith('Running')) return 'success'
    if (state.startsWith('Waiting')) return 'warning'
    if (state.startsWith('Terminated')) return 'error'
    return 'default'
  }

  const eventTypeColor = (type: string) => {
    if (type === 'Warning') return 'error'
    return 'info'
  }

  const containers: Container[] = detail?.containers || pod?.containers || []

  return (
    <Drawer
      title={
        <Space>
          <InfoCircleOutlined />
          <span>Pod 详情</span>
          {pod && <Tag color={statusColor(pod.status)}>{pod.status}</Tag>}
        </Space>
      }
      open={open}
      onClose={onClose}
      width={drawerWidth}
      destroyOnClose
      styles={{ body: { position: 'relative', padding: 16 } }}
      extra={
        pod && canWrite && (
          <Button danger icon={<SyncOutlined />} onClick={handleRestart}>重启 Pod</Button>
        )
      }
    >
      <div style={{ marginBottom: 12 }}>
        <DataTrustIndicator
          status={trust.status}
          lastSuccessfulAt={trust.lastSuccessfulAt}
          ageSeconds={trust.ageSeconds}
          sourceLabel={trust.sourceLabel}
          error={trust.error}
          formatAge={trust.formatAge}
          formatLastSuccessful={trust.formatLastSuccessful}
        />
      </div>
      {/* Resize Handle：Drawer 左侧边缘，鼠标拖拽调整宽度 */}
      <div
        onPointerDown={handleResizeStart}
        style={{
          position: 'absolute',
          left: 0,
          top: 0,
          bottom: 0,
          width: 6,
          cursor: 'col-resize',
          zIndex: 1000,
          background: 'transparent',
          transition: 'background 0.2s',
        }}
        onMouseEnter={(e) => { e.currentTarget.style.background = 'rgba(24, 144, 255, 0.2)' }}
        onMouseLeave={(e) => { e.currentTarget.style.background = 'transparent' }}
        title="拖拽调整宽度"
      />
      <Spin spinning={detailLoading}>
        {pod ? (
          <Tabs
            activeKey={activeTab}
            onChange={setActiveTab}
            items={[
              {
                key: 'info',
                label: '基本信息',
                children: (
                  <Space direction="vertical" style={{ width: '100%' }} size="large">
                    <Descriptions column={1} bordered size="small">
                      <Descriptions.Item label="名称">{pod.name}</Descriptions.Item>
                      <Descriptions.Item label="命名空间"><Tag>{pod.namespace}</Tag></Descriptions.Item>
                      <Descriptions.Item label="状态"><Tag color={statusColor(pod.status)}>{pod.status}</Tag></Descriptions.Item>
                      <Descriptions.Item label="Node">{pod.node || '-'}</Descriptions.Item>
                      <Descriptions.Item label="Pod IP">{pod.ip || '-'}</Descriptions.Item>
                      <Descriptions.Item label="重启次数">{pod.restart_count ?? 0}</Descriptions.Item>
                      <Descriptions.Item label="Age">{pod.age || '-'}</Descriptions.Item>
                      {pod.creation_timestamp && (
                        <Descriptions.Item label="创建时间">
                          {dayjs(pod.creation_timestamp).format('YYYY-MM-DD HH:mm:ss')}
                        </Descriptions.Item>
                      )}
                      {pod.labels && Object.keys(pod.labels).length > 0 && (
                        <Descriptions.Item label="Labels">
                          <Space wrap>
                            {Object.entries(pod.labels).map(([k, v]) => (
                              <Tag key={k}>{k}={v}</Tag>
                            ))}
                          </Space>
                        </Descriptions.Item>
                      )}
                    </Descriptions>

                    {containers.length > 0 && (
                      <div>
                        <Text strong style={{ display: 'block', marginBottom: 8 }}>Containers ({containers.length})</Text>
                        {containers.map((c, i) => (
                          <div key={i} style={{ padding: 8, border: '1px solid #f0f0f0', borderRadius: 4, marginBottom: 8 }}>
                            <Space wrap>
                              <Text strong>{c.name}</Text>
                              <Tag color={containerStateColor(c.state)}>{c.state}</Tag>
                              <Tag color={c.ready ? 'success' : 'default'}>{c.ready ? 'Ready' : 'Not Ready'}</Tag>
                              <Text type="secondary" style={{ fontSize: 12 }}>重启: {c.restart_count}</Text>
                            </Space>
                            <div style={{ fontFamily: 'monospace', fontSize: 12, color: '#666', marginTop: 4 }}>{c.image}</div>
                          </div>
                        ))}
                      </div>
                    )}
                  </Space>
                ),
              },
              {
                key: 'logs',
                label: (
                  <Space>
                    <FileTextOutlined />日志
                    <Button size="small" type="text" icon={<ReloadOutlined />} onClick={fetchLogs} loading={logsLoading} />
                  </Space>
                ),
                children: (
                  <Spin spinning={logsLoading}>
                    <Space style={{ marginBottom: 12 }} wrap size="small">
                      {containers.length > 0 && (
                        <Select
                          value={selectedContainer}
                          onChange={setSelectedContainer}
                          style={{ width: 160 }}
                          placeholder="选择容器"
                          options={containers.map((c) => ({ label: c.name, value: c.name }))}
                        />
                      )}
                      <span style={{ fontSize: 12 }}>Tail:</span>
                      <InputNumber min={10} max={5000} value={tailLines} onChange={(v) => setTailLines(v || 200)} style={{ width: 80 }} />
                      <Select
                        value={logLevel}
                        onChange={setLogLevel}
                        style={{ width: 110 }}
                        options={[
                          { label: '全部级别', value: 'all' },
                          { label: 'ERROR', value: 'ERROR' },
                          { label: 'WARN', value: 'WARN' },
                          { label: 'NOTICE', value: 'NOTICE' },
                          { label: 'INFO', value: 'INFO' },
                          { label: 'DEBUG', value: 'DEBUG' },
                          { label: 'TRACE', value: 'TRACE' },
                          { label: 'FATAL', value: 'FATAL' },
                          { label: 'PANIC', value: 'PANIC' },
                        ]}
                      />
                      <Input.Search
                        placeholder="过滤关键词"
                        value={logFilter}
                        onChange={(e) => setLogFilter(e.target.value)}
                        style={{ width: 160 }}
                        allowClear
                      />
                      <Button
                        size="small"
                        type={showTimestamp ? 'primary' : 'default'}
                        onClick={() => setShowTimestamp(!showTimestamp)}
                      >
                        {showTimestamp ? '时间戳开' : '时间戳关'}
                      </Button>
                      <Button
                        size="small"
                        type={autoRefresh ? 'primary' : 'default'}
                        icon={<SyncOutlined spin={autoRefresh} />}
                        onClick={() => setAutoRefresh(!autoRefresh)}
                      >
                        {autoRefresh ? '刷新中' : '自动刷新'}
                      </Button>
                      <span style={{ fontSize: 12, color: '#888' }}>{filteredLogLines.length} 行</span>
                    </Space>
                    {logsError ? (
                      <Alert
                        message="获取日志失败"
                        description={logsError}
                        type="error"
                        showIcon
                        action={<Button size="small" onClick={fetchLogs}>重试</Button>}
                      />
                    ) : filteredLogLines.length > 0 ? (
                      <div
                        style={{
                          background: '#1e1e1e',
                          color: '#d4d4d4',
                          padding: 12,
                          borderRadius: 4,
                          fontFamily: 'monospace',
                          fontSize: 12,
                          maxHeight: 'calc(100vh - 320px)',
                          minHeight: 300,
                          overflow: 'auto',
                        }}
                      >
                        {filteredLogLines.map((line, idx) => (
                          <div key={idx} style={{ display: 'flex', gap: 8, lineHeight: 1.6, whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}>
                            {showTimestamp && line.timestamp && (
                              <span style={{ color: '#6a9955', flexShrink: 0, userSelect: 'none' }}>
                                {line.timestamp.replace('T', ' ').replace('Z', '')}
                              </span>
                            )}
                            {line.level && (
                              <span style={{
                                color: line.level === 'ERROR' || line.level === 'FATAL' || line.level === 'PANIC' ? '#f48771' :
                                       line.level === 'WARN' ? '#dcdcaa' :
                                       line.level === 'NOTICE' ? '#4ec9b0' :
                                       line.level === 'INFO' ? '#4fc1ff' :
                                       line.level === 'TRACE' ? '#606060' : '#808080',
                                flexShrink: 0,
                                fontWeight: 'bold',
                                minWidth: 50,
                                userSelect: 'none',
                              }}>
                                {line.level}
                              </span>
                            )}
                            <span style={{ flex: 1 }}>{line.message}</span>
                          </div>
                        ))}
                        <div ref={logEndRef} />
                      </div>
                    ) : (
                      <Empty description={logs ? '无匹配日志' : '暂无日志'} />
                    )}
                  </Spin>
                ),
              },
              {
                key: 'yaml',
                label: (
                  <Space>
                    <CodeOutlined />YAML
                  </Space>
                ),
                children: detail?.yaml ? (
                  <div
                    style={{
                      background: '#1e1e1e',
                      color: '#d4d4d4',
                      padding: 12,
                      borderRadius: 4,
                      fontFamily: 'monospace',
                      fontSize: 12,
                      maxHeight: 500,
                      overflow: 'auto',
                      whiteSpace: 'pre',
                    }}
                  >
                    {detail.yaml}
                  </div>
                ) : (
                  <Alert message="YAML 加载中或不可用" type="info" />
                ),
              },
              {
                key: 'terminal',
                label: (
                  <Space>
                    <ApiOutlined />终端
                  </Space>
                ),
                children: (
                  <Space direction="vertical" style={{ width: '100%' }} size="middle">
                    <Alert
                      message="在 Pod 容器中执行命令"
                      description="通过 Kubernetes API 在指定容器中一次性执行命令并返回输出。非交互式终端，不支持持续会话。"
                      type="info"
                      showIcon
                    />
                    <Space wrap>
                      {containers.length > 0 && (
                        <Select
                          value={selectedContainer}
                          onChange={setSelectedContainer}
                          style={{ width: 180 }}
                          placeholder="选择容器"
                          options={containers.map((c) => ({ label: c.name, value: c.name }))}
                        />
                      )}
                      <Input
                        value={terminalCmd}
                        onChange={(e) => setTerminalCmd(e.target.value)}
                        style={{ width: 80 }}
                        placeholder="命令"
                        addonBefore="Shell"
                      />
                      <Input
                        value={terminalArgs}
                        onChange={(e) => setTerminalArgs(e.target.value)}
                        style={{ width: 80 }}
                        placeholder="参数"
                      />
                      <Input.Search
                        value={terminalInput}
                        onChange={(e) => setTerminalInput(e.target.value)}
                        onSearch={handleExec}
                        placeholder="输入要执行的命令，如: ls -la /"
                        style={{ width: 300 }}
                        enterButton={<Button type="primary" icon={<PlayCircleOutlined />} loading={terminalLoading}>执行</Button>}
                      />
                    </Space>
                    {terminalOutput ? (
                      <div
                        style={{
                          background: '#1e1e1e',
                          color: '#d4d4d4',
                          padding: 12,
                          borderRadius: 4,
                          fontFamily: 'monospace',
                          fontSize: 12,
                          maxHeight: 400,
                          overflow: 'auto',
                          whiteSpace: 'pre-wrap',
                          wordBreak: 'break-all',
                        }}
                      >
                        {terminalOutput}
                      </div>
                    ) : (
                      <Empty description="尚未执行命令" />
                    )}
                    {terminalHistory.length > 0 && (
                      <div>
                        <Text strong style={{ fontSize: 12 }}>执行历史（最近 {terminalHistory.length} 条）</Text>
                        <div style={{ maxHeight: 150, overflow: 'auto', marginTop: 8 }}>
                          {terminalHistory.map((h, i) => (
                            <div key={i} style={{ padding: '4px 0', borderBottom: '1px solid #f0f0f0', fontSize: 12 }}>
                              <Text code>$ {h.cmd}</Text>
                              <Tag color={h.exitCode === 0 ? 'success' : 'error'} style={{ marginLeft: 8 }}>exit {h.exitCode}</Tag>
                            </div>
                          ))}
                        </div>
                      </div>
                    )}
                  </Space>
                ),
              },
              {
                key: 'events',
                label: (
                  <Space>
                    <ExclamationCircleOutlined />事件
                    <Button size="small" type="text" icon={<ReloadOutlined />} onClick={fetchEvents} loading={eventsLoading} />
                  </Space>
                ),
                children: (
                  <Spin spinning={eventsLoading}>
                    {events && events.length > 0 ? (
                      <div style={{ maxHeight: 500, overflow: 'auto' }}>
                        {events.map((e, i) => (
                          <div key={i} style={{ padding: '8px 0', borderBottom: '1px solid rgba(0,0,0,0.06)' }}>
                            <Space>
                              <Tag color={eventTypeColor(e.type)}>{e.type}</Tag>
                              <Text strong>{e.reason}</Text>
                              {e.count && <Text type="secondary" style={{ fontSize: 12 }}>x{e.count}</Text>}
                            </Space>
                            <Paragraph style={{ margin: '4px 0 0', fontSize: 13 }}>{e.message}</Paragraph>
                            <Text type="secondary" style={{ fontSize: 12 }}>
                              {e.last_timestamp ? dayjs(e.last_timestamp).format('YYYY-MM-DD HH:mm:ss') : ''}
                            </Text>
                          </div>
                        ))}
                      </div>
                    ) : (
                      <Empty description="暂无事件" />
                    )}
                  </Spin>
                ),
              },
            ]}
          />
        ) : null}
      </Spin>
    </Drawer>
  )
}
