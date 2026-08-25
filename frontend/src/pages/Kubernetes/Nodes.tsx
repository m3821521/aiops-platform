import { useEffect, useState, useRef } from 'react'
import { Table, Tag, Card, Button, Spin, Select, Space, Alert } from 'antd'
import { ReloadOutlined, EyeOutlined } from '@ant-design/icons'
import { k8sApi } from '@/api/kubernetes'
import { clusterApi } from '@/api/cluster'
import type { Node, Cluster } from '@/types'
import NodeDetail from './NodeDetail'
import { useDataTrust } from '@/hooks/useDataTrust'
import { extractProvenance } from '@/utils/provenance'
import { DataTrustIndicator } from '@/components/DataTrustIndicator'

const POLL_INTERVAL = 15000

export default function Nodes() {
  const [data, setData] = useState<Node[]>([])
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [cluster, setCluster] = useState<string>('')
  const [loading, setLoading] = useState(false)
  const [selectedNode, setSelectedNode] = useState('')
  const [detailOpen, setDetailOpen] = useState(false)
  const pollingRef = useRef<number | null>(null)

  const trust = useDataTrust({ source: 'kubernetes' })

  const fetchClusters = async () => {
    try {
      const res = await clusterApi.list()
      setClusters(res || [])
      if (res && res.length > 0 && !cluster) setCluster(res[0].name)
    } catch { /* ignore */ }
  }

  const fetchData = async (silent = false) => {
    if (!cluster) return
    const seq = trust.beginFetch()
    if (!silent) setLoading(true)
    try {
      const res = await k8sApi.nodes(cluster)
      trust.markSuccess(seq, extractProvenance(res))
      setData(res || [])
    } catch (err: any) {
      trust.markError(seq, err?.message || '加载 Node 列表失败')
    } finally {
      if (!silent) setLoading(false)
    }
  }

  useEffect(() => { fetchClusters() }, [])

  useEffect(() => { if (cluster) fetchData() }, [cluster])

  useEffect(() => {
    if (!cluster) return
    pollingRef.current = window.setInterval(() => { fetchData(true) }, POLL_INTERVAL)
    return () => { if (pollingRef.current) clearInterval(pollingRef.current) }
  }, [cluster])

  const statusColor = (status: string) => {
    if (status === 'Ready') return 'success'
    if (status === 'NotReady') return 'error'
    return 'default'
  }

  const openDetail = (name: string) => { setSelectedNode(name); setDetailOpen(true) }

  const columns = [
    { title: 'Node', dataIndex: 'name', key: 'name', render: (t: string) => <a onClick={() => openDetail(t)} style={{ fontWeight: 500 }}>{t}</a> },
    { title: '状态', dataIndex: 'status', key: 'status', render: (s: string) => <Tag color={statusColor(s)}>{s}</Tag> },
    { title: 'K8s 版本', dataIndex: 'version', key: 'version', render: (v: string) => v || '-' },
    { title: '内部 IP', dataIndex: 'internal_ip', key: 'ip', render: (v: string) => v || '-' },
    { title: 'OS', dataIndex: 'os', key: 'os', render: (v: string) => v ? v.split(' ')[0] : '-' },
    { title: '内核', dataIndex: 'kernel', key: 'kernel', render: (v: string) => v ? v.split('-')[0] : '-' },
    { title: '容器运行时', dataIndex: 'container_runtime', key: 'runtime', render: (v: string) => v ? v.split('://')[0] : '-' },
    { title: 'Age', dataIndex: 'age', key: 'age', render: (v: string) => v || '-' },
    {
      title: '操作', key: 'action', width: 80,
      render: (_: any, record: Node) => (
        <Button type="link" size="small" icon={<EyeOutlined />} onClick={() => openDetail(record.name)}>详情</Button>
      ),
    },
  ]

  return (
    <>
      <Card
        title="Node 管理"
        extra={
          <Space>
            <Select value={cluster} onChange={setCluster} style={{ width: 160 }} placeholder="选择集群"
              options={clusters.map((c) => ({ label: c.name, value: c.name }))} />
            <Button icon={<ReloadOutlined />} onClick={() => fetchData()} loading={loading}>刷新</Button>
          </Space>
        }
      >
        <div style={{ marginBottom: 8 }}>
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
        {trust.error && trust.status === 'stale' && (
          <Alert message="数据刷新失败" description={trust.error + '（当前显示的是上次成功获取的数据）'} type="warning" showIcon style={{ marginBottom: 12 }} />
        )}
        <Spin spinning={loading}>
          <Table columns={columns} dataSource={data} rowKey="name" pagination={{ pageSize: 20 }} size="middle" />
        </Spin>
      </Card>

      <NodeDetail nodeName={selectedNode} cluster={cluster} open={detailOpen} onClose={() => setDetailOpen(false)} />
    </>
  )
}
