import { useEffect, useState } from 'react'
import {
  Drawer, Descriptions, Tag, Tabs, Button, Spin, Modal, message, Space, Empty, Typography,
} from 'antd'
import {
  ReloadOutlined, SyncOutlined, FileTextOutlined,
  ExclamationCircleOutlined, InfoCircleOutlined,
} from '@ant-design/icons'
import { automationApi } from '@/api/automation'
import type { Pod } from '@/types'
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
  involved_object?: { kind: string; name: string }
}

export default function PodDetail({ pod, cluster, open, onClose, onRestarted }: PodDetailProps) {
  const [logs, setLogs] = useState<string>('')
  const [events, setEvents] = useState<K8sEvent[]>([])
  const [logsLoading, setLogsLoading] = useState(false)
  const [eventsLoading, setEventsLoading] = useState(false)
  const [activeTab, setActiveTab] = useState('info')

  const fetchLogs = async () => {
    if (!pod) return
    setLogsLoading(true)
    try {
      const res: any = await automationApi.podLogs(pod.name, { cluster, namespace: pod.namespace, tail: 200 })
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
    }
  }, [open, pod])

  useEffect(() => {
    if (activeTab === 'logs' && open && pod && !logs) fetchLogs()
    if (activeTab === 'events' && open && pod && events.length === 0 && !eventsLoading) fetchEvents()
  }, [activeTab, open, pod])

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

  const eventTypeColor = (type: string) => {
    if (type === 'Warning') return 'error'
    return 'info'
  }

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
      width={680}
      extra={
        pod && (
          <Button danger icon={<SyncOutlined />} onClick={handleRestart}>重启 Pod</Button>
        )
      }
    >
      {pod ? (
        <Tabs
          activeKey={activeTab}
          onChange={setActiveTab}
          items={[
            {
              key: 'info',
              label: '基本信息',
              children: (
                <Descriptions column={1} bordered size="small">
                  <Descriptions.Item label="名称">{pod.name}</Descriptions.Item>
                  <Descriptions.Item label="命名空间"><Tag>{pod.namespace}</Tag></Descriptions.Item>
                  <Descriptions.Item label="状态"><Tag color={statusColor(pod.status)}>{pod.status}</Tag></Descriptions.Item>
                  <Descriptions.Item label="Node">{pod.node || '-'}</Descriptions.Item>
                  <Descriptions.Item label="Pod IP">{pod.ip || '-'}</Descriptions.Item>
                  <Descriptions.Item label="重启次数">{pod.restart_count ?? 0}</Descriptions.Item>
                  <Descriptions.Item label="创建时间">{pod.age || '-'}</Descriptions.Item>
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
                  {logs ? (
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
                      {logs}
                    </div>
                  ) : (
                    <Empty description="暂无日志" />
                  )}
                </Spin>
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
    </Drawer>
  )
}
