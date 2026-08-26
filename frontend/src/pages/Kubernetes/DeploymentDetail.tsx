import { useEffect, useState, useRef } from 'react'
import {
  Drawer, Descriptions, Tag, Spin, Alert, Space, Card, Button, InputNumber, Modal, message,
} from 'antd'
import type { Deployment } from '@/types'
import { k8sApi } from '@/api/kubernetes'
import { automationApi } from '@/api/automation'
import { usePermission } from '@/utils/permission'
import dayjs from 'dayjs'
import { useDataTrust } from '@/hooks/useDataTrust'
import { extractProvenance } from '@/utils/provenance'
import { DataTrustIndicator } from '@/components/DataTrustIndicator'

const POLL_INTERVAL = 15000

interface Props {
  deployment: Deployment | null
  cluster: string
  open: boolean
  onClose: () => void
  onScaled?: () => void
}

export default function DeploymentDetail({ deployment, cluster, open, onClose, onScaled }: Props) {
  const canWrite = usePermission('cluster', 'write')
  const [loading, setLoading] = useState(false)
  const [detail, setDetail] = useState<Deployment | null>(null)
  const [scaleOpen, setScaleOpen] = useState(false)
  const [replicas, setReplicas] = useState(1)
  const [scaling, setScaling] = useState(false)
  const pollingRef = useRef<number | null>(null)

  // P1-X.9: 统一 Data Trust（DeploymentDetail 来自 Kubernetes API）
  const trust = useDataTrust({ source: 'kubernetes' })

  const fetchDetail = async (silent = false) => {
    if (!deployment) return
    const seq = trust.beginFetch()
    if (!silent) setLoading(true)
    try {
      const res = await k8sApi.deploymentDetail(deployment.name, { cluster, namespace: deployment.namespace })
      trust.markSuccess(seq, extractProvenance(res))
      setDetail(res)
      setReplicas(res.replicas || 1)
    } catch (err: any) {
      // P1-X.10 Phase 3.2: API failure 不得清空数据，保留上次成功数据并标记 error/stale
      trust.markError(seq, err?.message || '加载 Deployment 详情失败')
    } finally {
      if (!silent) setLoading(false)
    }
  }

  // Drawer 打开时立即加载
  useEffect(() => {
    if (open && deployment) {
      setDetail(deployment)
      setReplicas(deployment.replicas || 1)
      fetchDetail()
    }
  }, [open, deployment, cluster])

  // P1-X.10 Phase 3.2: 15s 自动轮询（仅在 Drawer 打开时）
  useEffect(() => {
    if (!open || !deployment) return
    pollingRef.current = window.setInterval(() => {
      fetchDetail(true)
    }, POLL_INTERVAL)
    return () => {
      if (pollingRef.current) clearInterval(pollingRef.current)
    }
  }, [open, deployment, cluster])

  const handleScale = async () => {
    if (!detail) return
    setScaling(true)
    try {
      await automationApi.scaleDeployment(detail.name, { cluster, namespace: detail.namespace, replicas, confirm: true })
      message.success(`已将 ${detail.name} 扩容到 ${replicas} 副本`)
      setScaleOpen(false)
      onScaled?.()
      // 扩容后立即刷新
      fetchDetail()
    } catch (err: any) {
      message.error(err?.message || '扩容失败')
    } finally {
      setScaling(false)
    }
  }

  return (
    <Drawer
      title={`Deployment: ${deployment?.name}`}
      open={open}
      onClose={onClose}
      width={560}
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
          description={trust.error || '无法获取 Deployment 详情数据'}
          type="error"
          showIcon
          style={{ marginBottom: 12 }}
        />
      )}

      <Spin spinning={loading}>
        {detail && (
          <Space direction="vertical" style={{ width: '100%' }} size="large">
            <Card size="small" title="基本信息" extra={
              canWrite && <Button type="primary" size="small" onClick={() => setScaleOpen(true)}>扩容/缩容</Button>
            }>
              <Descriptions column={1} size="small" bordered>
                <Descriptions.Item label="Namespace"><Tag>{detail.namespace}</Tag></Descriptions.Item>
                <Descriptions.Item label="Ready">{detail.ready || '-'}</Descriptions.Item>
                <Descriptions.Item label="期望副本数">{detail.replicas}</Descriptions.Item>
                <Descriptions.Item label="可用副本数">{detail.available ?? '-'}</Descriptions.Item>
                <Descriptions.Item label="已更新副本数">{detail.updated ?? '-'}</Descriptions.Item>
                <Descriptions.Item label="更新策略">{detail.strategy || '-'}</Descriptions.Item>
                <Descriptions.Item label="创建时间">
                  {detail.creation_timestamp ? dayjs(detail.creation_timestamp).format('YYYY-MM-DD HH:mm:ss') : '-'}
                </Descriptions.Item>
                <Descriptions.Item label="Age">{detail.age || '-'}</Descriptions.Item>
              </Descriptions>
            </Card>

            {detail.images && detail.images.length > 0 && (
              <Card size="small" title="容器镜像">
                <Space direction="vertical">
                  {detail.images.map((img, i) => (
                    <Tag key={i} style={{ fontFamily: 'monospace', fontSize: 12 }}>{img}</Tag>
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
          </Space>
        )}
      </Spin>

      <Modal
        title="扩容/缩容 Deployment"
        open={scaleOpen}
        onOk={handleScale}
        onCancel={() => setScaleOpen(false)}
        confirmLoading={scaling}
        okText="确认执行"
        okButtonProps={{ danger: true }}
      >
        <p>当前副本数：<strong>{detail?.replicas}</strong></p>
        <p>目标副本数：</p>
        <InputNumber min={0} max={100} value={replicas} onChange={(v) => setReplicas(v || 0)} style={{ width: '100%' }} />
        <Alert
          message="危险操作"
          description="扩容/缩容会直接修改 Kubernetes 集群中的 Deployment，请确认后执行。"
          type="warning"
          showIcon
          style={{ marginTop: 16 }}
        />
      </Modal>
    </Drawer>
  )
}
