import { useEffect, useState } from 'react'
import {
  Drawer, Descriptions, Tag, Spin, Alert, Table, Typography, Space, Card,
} from 'antd'
import type { NodeDetail as NodeDetailType, NodeCondition } from '@/types'
import { k8sApi } from '@/api/kubernetes'
import dayjs from 'dayjs'

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
  const [error, setError] = useState('')

  useEffect(() => {
    if (open && nodeName) {
      setLoading(true)
      setError('')
      k8sApi.nodeDetail(nodeName, cluster)
        .then((res) => setDetail(res))
        .catch((err) => setError(err?.message || '加载失败'))
        .finally(() => setLoading(false))
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
      <Spin spinning={loading}>
        {error && <Alert message={error} type="error" showIcon style={{ marginBottom: 16 }} />}
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

            <Card size="small" title="资源监控">
              <Alert
                message="Prometheus 监控数据"
                description="Node CPU / Memory / Disk 趋势图将在监控模块中展示。"
                type="info"
                showIcon
              />
            </Card>
          </Space>
        )}
      </Spin>
    </Drawer>
  )
}
