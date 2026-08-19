import { useEffect, useState } from 'react'
import {
  Drawer, Descriptions, Tag, Spin, Alert, Space, Card, Button, InputNumber, Modal, message,
} from 'antd'
import type { Deployment } from '@/types'
import { k8sApi } from '@/api/kubernetes'
import { automationApi } from '@/api/automation'
import dayjs from 'dayjs'

interface Props {
  deployment: Deployment | null
  cluster: string
  open: boolean
  onClose: () => void
  onScaled?: () => void
}

export default function DeploymentDetail({ deployment, cluster, open, onClose, onScaled }: Props) {
  const [loading, setLoading] = useState(false)
  const [detail, setDetail] = useState<Deployment | null>(null)
  const [error, setError] = useState('')
  const [scaleOpen, setScaleOpen] = useState(false)
  const [replicas, setReplicas] = useState(1)
  const [scaling, setScaling] = useState(false)

  useEffect(() => {
    if (open && deployment) {
      setLoading(true)
      setError('')
      setDetail(deployment)
      setReplicas(deployment.replicas || 1)
      // 尝试获取详情
      k8sApi.deploymentDetail(deployment.name, { cluster, namespace: deployment.namespace })
        .then((res) => setDetail(res))
        .catch(() => { /* 列表数据已足够 */ })
        .finally(() => setLoading(false))
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
      <Spin spinning={loading}>
        {error && <Alert message={error} type="error" showIcon style={{ marginBottom: 16 }} />}
        {detail && (
          <Space direction="vertical" style={{ width: '100%' }} size="large">
            <Card size="small" title="基本信息" extra={
              <Button type="primary" size="small" onClick={() => setScaleOpen(true)}>扩容/缩容</Button>
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
