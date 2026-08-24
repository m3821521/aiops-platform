import { useEffect, useState, useRef } from 'react'
import { Table, Tag, Card, Button, Spin, Select, Space, Alert } from 'antd'
import { ReloadOutlined, EyeOutlined } from '@ant-design/icons'
import { k8sApi } from '@/api/kubernetes'
import { clusterApi } from '@/api/cluster'
import type { Node, Cluster } from '@/types'
import NodeDetail from './NodeDetail'

const POLL_INTERVAL = 15000

export default function Nodes() {
  const [data, setData] = useState<Node[]>([])
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [cluster, setCluster] = useState<string>('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [lastUpdated, setLastUpdated] = useState<Date | null>(null)
  const [selectedNode, setSelectedNode] = useState('')
  const [detailOpen, setDetailOpen] = useState(false)
  const pollingRef = useRef<number | null>(null)
  const requestTokenRef = useRef(0)

  const fetchClusters = async () => {
    try {
      const res = await clusterApi.list()
      setClusters(res || [])
      if (res && res.length > 0 && !cluster) {
        setCluster(res[0].name)
      }
    } catch {
      // ignore
    }
  }

  const fetchData = async (silent = false) => {
    if (!cluster) return
    const token = ++requestTokenRef.current
    if (!silent) setLoading(true)
    try {
      const res = await k8sApi.nodes(cluster)
      if (token !== requestTokenRef.current) return
      setData(res || [])
      setError(null)
      setLastUpdated(new Date())
    } catch (err: any) {
      if (token !== requestTokenRef.current) return
      setError(err?.message || '加载 Node 列表失败')
    } finally {
      if (token === requestTokenRef.current && !silent) setLoading(false)
    }
  }

  useEffect(() => {
    fetchClusters()
  }, [])

  useEffect(() => {
    if (cluster) fetchData()
  }, [cluster])

  useEffect(() => {
    if (!cluster) return
    pollingRef.current = window.setInterval(() => {
      fetchData(true)
    }, POLL_INTERVAL)
    return () => {
      if (pollingRef.current) clearInterval(pollingRef.current)
    }
  }, [cluster])

  const statusColor = (status: string) => {
    if (status === 'Ready') return 'success'
    if (status === 'NotReady') return 'error'
    return 'default'
  }

  const openDetail = (name: string) => {
    setSelectedNode(name)
    setDetailOpen(true)
  }

  const formatLastUpdated = () => {
    if (!lastUpdated) return '未加载'
    const diff = Date.now() - lastUpdated.getTime()
    if (diff < 5000) return '刚刚更新'
    if (diff < 60000) return `${Math.floor(diff / 1000)} 秒前更新`
    return `${Math.floor(diff / 60000)} 分钟前更新`
  }

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
            <Select
              value={cluster}
              onChange={setCluster}
              style={{ width: 160 }}
              placeholder="选择集群"
              options={clusters.map((c) => ({ label: c.name, value: c.name }))}
            />
            <Button icon={<ReloadOutlined />} onClick={() => fetchData()} loading={loading}>刷新</Button>
          </Space>
        }
      >
        <div style={{ marginBottom: 8, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <span style={{ fontSize: 12, color: '#8c8c8c' }}>{formatLastUpdated()} · 自动刷新 15s</span>
        </div>
        {error && (
          <Alert
            message="数据刷新失败"
            description={error + '（当前显示的是上次成功获取的数据）'}
            type="warning"
            showIcon
            style={{ marginBottom: 12 }}
          />
        )}
        <Spin spinning={loading}>
          <Table columns={columns} dataSource={data} rowKey="name" pagination={{ pageSize: 20 }} size="middle" />
        </Spin>
      </Card>

      <NodeDetail nodeName={selectedNode} cluster={cluster} open={detailOpen} onClose={() => setDetailOpen(false)} />
    </>
  )
}
