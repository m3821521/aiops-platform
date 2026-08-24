import { useEffect, useState, useRef } from 'react'
import { Table, Tag, Card, Button, Spin, Select, Space, Input, Alert } from 'antd'
import { ReloadOutlined, EyeOutlined } from '@ant-design/icons'
import { k8sApi } from '@/api/kubernetes'
import type { Namespace } from '@/api/kubernetes'
import { clusterApi } from '@/api/cluster'
import type { Pod, Cluster } from '@/types'
import PodDetail from './PodDetail'

const POLL_INTERVAL = 15000

export default function Pods() {
  const [data, setData] = useState<Pod[]>([])
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [namespaces, setNamespaces] = useState<Namespace[]>([])
  const [cluster, setCluster] = useState<string>('')
  const [namespace, setNamespace] = useState<string>('')
  const [keyword, setKeyword] = useState<string>('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [lastUpdated, setLastUpdated] = useState<Date | null>(null)
  const [selectedPod, setSelectedPod] = useState<Pod | null>(null)
  const [detailOpen, setDetailOpen] = useState(false)
  const pollingRef = useRef<number | null>(null)
  const requestTokenRef = useRef(0)

  const fetchClusters = async () => {
    try {
      const res = await clusterApi.list()
      setClusters(res || [])
      if (res && res.length > 0 && !cluster) setCluster(res[0].name)
    } catch {
      // ignore
    }
  }

  const fetchNamespaces = async (c: string) => {
    try {
      const res = await k8sApi.namespaces(c)
      setNamespaces(res || [])
    } catch {
      // ignore
    }
  }

  const fetchData = async (silent = false) => {
    if (!cluster) return
    const token = ++requestTokenRef.current
    if (!silent) setLoading(true)
    try {
      const res = await k8sApi.pods({ cluster, namespace: namespace || undefined })
      if (token !== requestTokenRef.current) return // race condition guard
      setData(res || [])
      setError(null)
      setLastUpdated(new Date())
    } catch (err: any) {
      if (token !== requestTokenRef.current) return
      setError(err?.message || '加载 Pod 列表失败')
    } finally {
      if (token === requestTokenRef.current && !silent) setLoading(false)
    }
  }

  useEffect(() => { fetchClusters() }, [])

  // cluster 或 namespace 变化时重新请求
  useEffect(() => {
    if (cluster) {
      fetchNamespaces(cluster)
      fetchData()
    }
  }, [cluster, namespace])

  // 15s 自动轮询
  useEffect(() => {
    if (!cluster) return
    pollingRef.current = window.setInterval(() => {
      fetchData(true) // silent refresh
    }, POLL_INTERVAL)
    return () => {
      if (pollingRef.current) clearInterval(pollingRef.current)
    }
  }, [cluster, namespace])

  const statusColor = (status: string) => {
    if (status === 'Running') return 'success'
    if (status === 'Pending') return 'warning'
    if (status === 'Failed' || status === 'Unknown') return 'error'
    return 'default'
  }

  const filtered = data.filter((p) =>
    !keyword || p.name.toLowerCase().includes(keyword.toLowerCase())
  )

  const openDetail = (pod: Pod) => {
    setSelectedPod(pod)
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
    {
      title: 'Pod',
      dataIndex: 'name',
      key: 'name',
      render: (t: string, record: Pod) => (
        <a onClick={() => openDetail(record)} style={{ fontWeight: 500 }}>{t}</a>
      ),
    },
    { title: 'Namespace', dataIndex: 'namespace', key: 'namespace', render: (t: string) => <Tag>{t}</Tag> },
    { title: '状态', dataIndex: 'status', key: 'status', render: (s: string) => <Tag color={statusColor(s)}>{s}</Tag> },
    { title: 'Node', dataIndex: 'node', key: 'node', render: (v: string) => v || '-' },
    { title: 'IP', dataIndex: 'ip', key: 'ip', render: (v: string) => v || '-' },
    { title: '重启次数', dataIndex: 'restart_count', key: 'restart', render: (v: number) => v ?? 0 },
    { title: 'Age', dataIndex: 'age', key: 'age', render: (v: string) => v || '-' },
    {
      title: '操作',
      key: 'action',
      width: 80,
      render: (_: any, record: Pod) => (
        <Button type="link" size="small" icon={<EyeOutlined />} onClick={() => openDetail(record)}>详情</Button>
      ),
    },
  ]

  return (
    <>
      <Card
        title="Pod 管理"
        extra={
          <Space wrap>
            <Select value={cluster} onChange={setCluster} style={{ width: 140 }} placeholder="选择集群"
              options={clusters.map((c) => ({ label: c.name, value: c.name }))} />
            <Select value={namespace} onChange={setNamespace} style={{ width: 160 }} placeholder="所有命名空间" allowClear
              options={namespaces.map((n) => ({ label: n.name, value: n.name }))} />
            <Input.Search placeholder="搜索 Pod" value={keyword} onChange={(e) => setKeyword(e.target.value)} style={{ width: 180 }} allowClear />
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
          <Table columns={columns} dataSource={filtered} rowKey="name" pagination={{ pageSize: 20 }} size="middle" />
        </Spin>
      </Card>

      <PodDetail
        pod={selectedPod}
        cluster={cluster}
        open={detailOpen}
        onClose={() => setDetailOpen(false)}
        onRestarted={() => fetchData()}
      />
    </>
  )
}
