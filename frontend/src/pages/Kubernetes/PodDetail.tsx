import { useEffect, useState, useMemo } from 'react'
import {
  Drawer, Descriptions, Tag, Tabs, Button, Spin, Modal, message, Space, Empty, Typography, Select, InputNumber, Input, Alert,
} from 'antd'
import {
  ReloadOutlined, SyncOutlined, FileTextOutlined,
  ExclamationCircleOutlined, InfoCircleOutlined, CodeOutlined,
} from '@ant-design/icons'
import { automationApi } from '@/api/automation'
import { k8sApi } from '@/api/kubernetes'
import type { Pod, PodDetail as PodDetailType, Container } from '@/types'
import dayjs from 'dayjs'

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
  const [detail, setDetail] = useState<PodDetailType | null>(null)
  const [logs, setLogs] = useState<string>('')
  const [events, setEvents] = useState<K8sEvent[]>([])
  const [logsLoading, setLogsLoading] = useState(false)
  const [eventsLoading, setEventsLoading] = useState(false)
  const [detailLoading, setDetailLoading] = useState(false)
  const [activeTab, setActiveTab] = useState('info')
  const [selectedContainer, setSelectedContainer] = useState<string>('')
  const [tailLines, setTailLines] = useState(200)
  const [logFilter, setLogFilter] = useState('')

  const fetchDetail = async () => {
    if (!pod) return
    setDetailLoading(true)
    try {
      const res = await k8sApi.podDetail(pod.name, { cluster, namespace: pod.namespace })
      setDetail(res)
      if (res.containers && res.containers.length > 0) {
        setSelectedContainer(res.containers[0].name)
      }
    } catch {
      setDetail(null)
    } finally {
      setDetailLoading(false)
    }
  }

  const fetchLogs = async () => {
    if (!pod) return
    setLogsLoading(true)
    try {
      const res: any = await automationApi.podLogs(pod.name, {
        cluster,
        namespace: pod.namespace,
        container: selectedContainer || undefined,
        tail: tailLines,
      })
      setLogs(res?.logs || '')
    } catch {
      setLogs('')
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

  const filteredLogs = useMemo(() => {
    if (!logs) return ''
    if (!logFilter.trim()) return logs
    return logs.split('\n').filter((l) => l.toLowerCase().includes(logFilter.toLowerCase())).join('\n')
  }, [logs, logFilter])

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
      width={720}
      extra={
        pod && (
          <Button danger icon={<SyncOutlined />} onClick={handleRestart}>重启 Pod</Button>
        )
      }
    >
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
                    <Space style={{ marginBottom: 12 }} wrap>
                      {containers.length > 0 && (
                        <Select
                          value={selectedContainer}
                          onChange={setSelectedContainer}
                          style={{ width: 180 }}
                          placeholder="选择容器"
                          options={containers.map((c) => ({ label: c.name, value: c.name }))}
                        />
                      )}
                      <span style={{ fontSize: 12 }}>Tail:</span>
                      <InputNumber min={10} max={5000} value={tailLines} onChange={(v) => setTailLines(v || 200)} style={{ width: 90 }} />
                      <Input.Search
                        placeholder="过滤关键词"
                        value={logFilter}
                        onChange={(e) => setLogFilter(e.target.value)}
                        style={{ width: 180 }}
                        allowClear
                      />
                    </Space>
                    {filteredLogs ? (
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
                          whiteSpace: 'pre-wrap',
                          wordBreak: 'break-all',
                        }}
                      >
                        {filteredLogs}
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
