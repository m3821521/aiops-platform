import { useEffect, useState, useRef } from 'react'
import {
  Drawer, Descriptions, Tag, Spin, Alert, Table, Typography, Space, Card, Statistic,
} from 'antd'
import type { NodeDetail as NodeDetailType, NodeCondition } from '@/types'
import { k8sApi } from '@/api/kubernetes'
import { clusterApi, type NodeMetric } from '@/api/cluster'
import dayjs from 'dayjs'
import { useDataTrust } from '@/hooks/useDataTrust'
import { extractProvenance } from '@/utils/provenance'
import { DataTrustIndicator } from '@/components/DataTrustIndicator'

const POLL_INTERVAL = 15000

const { Text } = Typography

interface Props {
  nodeName: string
  cluster: string
  open: boolean
  onClose: () => void
}

export default function NodeDetail({ nodeName, cluster, open, onClose }: Props) {
  const [loading, setLoading] = useState(false)
  const [detail, setDetail] = useState<NodeDetailType | null>(null)
  const [nodeMetric, setNodeMetric] = useState<NodeMetric | null>(null)
  const [metricError, setMetricError] = useState<string | null>(null)
  const pollingRef = useRef<number | null>(null)

  // P1-X.9: 统一 Data Trust（NodeDetail 来自 Kubernetes API）
  const trust = useDataTrust({ source: 'kubernetes' })

  const fetchMetric = async () => {
    if (!nodeName) return
    try {
      const metrics = await clusterApi.nodeMetrics()
      const found = (metrics || []).find((m: NodeMetric) => m.name === nodeName)
      setNodeMetric(found || null)
      setMetricError(null)
    } catch (err: any) {
      setMetricError(err?.message || 'Metrics 不可用')
    }
  }

  const fetchDetail = async (silent = false) => {
    if (!nodeName) return
    const seq = trust.beginFetch()
    if (!silent) setLoading(true)
    try {
      const res = await k8sApi.nodeDetail(nodeName, cluster)
      trust.markSuccess(seq, extractProvenance(res))
      setDetail(res)
    } catch (err: any) {
      // P1-X.10 Phase 3.2: API failure 不得清空数据，保留上次成功数据并标记 error/stale
      trust.markError(seq, err?.message || '加载 Node 详情失败')
    } finally {
      if (!silent) setLoading(false)
    }
    // 并行获取 metrics
    fetchMetric()
  }

  // Drawer 打开时立即加载
  useEffect(() => {
    if (open && nodeName) {
      fetchDetail()
    }
  }, [open, nodeName, cluster])

  // P1-X.10 Phase 3.2: 15s 自动轮询（仅在 Drawer 打开时）
  useEffect(() => {
    if (!open || !nodeName) return
    pollingRef.current = window.setInterval(() => {
      fetchDetail(true)
    }, POLL_INTERVAL)
    return () => {
      if (pollingRef.current) clearInterval(pollingRef.current)
    }
  }, [open, nodeName, cluster])

  const statusColor = (s: string) => {
    if (s === 'True' || s === 'Ready') return 'success'
    if (s === 'False' || s === 'NotReady') return 'error'
    return 'warning'
  }

  return (
    <Drawer
      title={`Node: ${nodeName}`}
      open={open}
      onClose={onClose}
      width={640}
      destroyOnClose
    >
      {/* P1-X.10 Phase 3.2: Data Trust 状态指示器 */}
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

      {/* P1-X.10 Phase 3.2: API failure 时明确提示，不伪装成空数据 */}
      {trust.error && trust.status === 'stale' && (
        <Alert
          message="数据刷新失败"
          description={trust.error + '（当前显示的是上次成功获取的数据）'}
          type="warning"
          showIcon
          style={{ marginBottom: 12 }}
        />
      )}
      {trust.status === 'error' && (
        <Alert
          message="加载失败"
          description={trust.error || '无法获取 Node 详情数据'}
          type="error"
          showIcon
          style={{ marginBottom: 12 }}
        />
      )}

      <Spin spinning={loading}>
        {detail && (
          <Space direction="vertical" style={{ width: '100%' }} size="large">
            <Card size="small" title="基本信息">
              <Descriptions column={1} size="small" bordered>
                <Descriptions.Item label="状态">
                  <Tag color={statusColor(detail.status)}>{detail.status}</Tag>
                </Descriptions.Item>
                <Descriptions.Item label="Internal IP">{detail.internal_ip || '-'}</Descriptions.Item>
                <Descriptions.Item label="Kubernetes 版本">{detail.version || '-'}</Descriptions.Item>
                <Descriptions.Item label="操作系统">{detail.os || '-'}</Descriptions.Item>
                <Descriptions.Item label="内核版本">{detail.kernel || '-'}</Descriptions.Item>
                <Descriptions.Item label="容器运行时">{detail.container_runtime || '-'}</Descriptions.Item>
                <Descriptions.Item label="创建时间">
                  {detail.creation_timestamp ? dayjs(detail.creation_timestamp).format('YYYY-MM-DD HH:mm:ss') : '-'}
                </Descriptions.Item>
                <Descriptions.Item label="Age">{detail.age || '-'}</Descriptions.Item>
              </Descriptions>
            </Card>

            <Card size="small" title="Conditions">
              <Table
                size="small"
                pagination={false}
                dataSource={detail.conditions || []}
                rowKey="type"
                columns={[
                  { title: 'Type', dataIndex: 'type', key: 'type' },
                  { title: 'Status', dataIndex: 'status', key: 'status', render: (s: string) => <Tag color={statusColor(s)}>{s}</Tag> },
                  { title: 'Reason', dataIndex: 'reason', key: 'reason', render: (v: string) => v || '-' },
                  { title: 'Message', dataIndex: 'message', key: 'message', ellipsis: true, render: (v: string) => v || '-' },
                ]}
              />
            </Card>

            {detail.taints && detail.taints.length > 0 && (
              <Card size="small" title="Taints">
                <Space wrap>
                  {detail.taints.map((t, i) => (
                    <Tag key={i} color="red">{t.key}={t.value || ''}:{t.effect}</Tag>
                  ))}
                </Space>
              </Card>
            )}

            {detail.labels && Object.keys(detail.labels).length > 0 && (
              <Card size="small" title="Labels">
                <Space wrap>
                  {Object.entries(detail.labels).map(([k, v]) => (
                    <Tag key={k}>{k}={v}</Tag>
                  ))}
                </Space>
              </Card>
            )}

            <Card size="small" title="资源监控（Metrics Server 实时）">
              {nodeMetric ? (
                <Space size="large">
                  <Statistic title="CPU 使用率" value={nodeMetric.cpu_percent} precision={1} suffix="%" />
                  <Statistic title="CPU 使用量" value={nodeMetric.cpu_cores} />
                  <Statistic title="内存使用率" value={nodeMetric.memory_percent} precision={1} suffix="%" />
                  <Statistic title="内存使用量" value={nodeMetric.memory_bytes} />
                  <div>
                    <Text type="secondary" style={{ fontSize: 12 }}>
                      采集时间: {nodeMetric.timestamp ? dayjs(nodeMetric.timestamp).format('HH:mm:ss') : '-'}
                    </Text>
                  </div>
                </Space>
              ) : (
                <Alert
                  message={metricError ? 'Metrics Server 不可用' : '暂无 Metrics 数据'}
                  description={metricError || '等待 metrics-server 采集数据...'}
                  type="info"
                  showIcon
                />
              )}
            </Card>
          </Space>
        )}
      </Spin>
    </Drawer>
  )
}
