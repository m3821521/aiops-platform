import { useEffect, useState, useRef } from 'react'
import { Table, Tag, Card, Button, Spin, Select, Space, Input, Alert, Tooltip } from 'antd'
import { ReloadOutlined, EyeOutlined } from '@ant-design/icons'
import { k8sApi } from '@/api/kubernetes'
import type { Namespace } from '@/api/kubernetes'
import { clusterApi, type PodMetric } from '@/api/cluster'
import type { Pod, Cluster } from '@/types'
import PodDetail from './PodDetail'
import { useDataTrust } from '@/hooks/useDataTrust'
import { extractProvenance } from '@/utils/provenance'
import { DataTrustIndicator } from '@/components/DataTrustIndicator'

const POLL_INTERVAL = 15000

export default function Pods() {
  const [data, setData] = useState<Pod[]>([])
  const [podMetrics, setPodMetrics] = useState<Map<string, PodMetric>>(new Map())
  const [metricsError, setMetricsError] = useState<string | null>(null)
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [namespaces, setNamespaces] = useState<Namespace[]>([])
  const [cluster, setCluster] = useState<string>('')
  const [namespace, setNamespace] = useState<string>('')
  const [keyword, setKeyword] = useState<string>('')
  const [loading, setLoading] = useState(false)
  const [selectedPod, setSelectedPod] = useState<Pod | null>(null)
  const [detailOpen, setDetailOpen] = useState(false)
  const pollingRef = useRef<number | null>(null)

  // P1-X.9: 统一 Data Trust
  const trust = useDataTrust({ source: 'kubernetes' })

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

  const fetchMetrics = async () => {
    try {
      const metrics = await clusterApi.podMetrics(namespace || undefined)
      const map = new Map<string, PodMetric>()
      ;(metrics || []).forEach((m: PodMetric) => {
        map.set(`${m.namespace}/${m.name}`, m)
      })
      setPodMetrics(map)
      setMetricsError(null)
    } catch (err: any) {
      setMetricsError(err?.message || 'Metrics 不可用')
      // 不清除已有 metrics
    }
  }

  const fetchData = async (silent = false) => {
    if (!cluster) return
    const seq = trust.beginFetch()
    if (!silent) setLoading(true)
    try {
      const res = await k8sApi.pods({ cluster, namespace: namespace || undefined })
      // Race protection: useDataTrust.markSuccess 内部检查 seq
      trust.markSuccess(seq, extractProvenance(res))
      setData(res || [])
    } catch (err: any) {
      trust.markError(seq, err?.message || '加载 Pod 列表失败')
    } finally {
      if (!silent) setLoading(false)
    }
    // 并行获取 metrics（不阻塞主列表加载）
    fetchMetrics()
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

  const renderPodMetric = (pod: Pod, type: 'cpu' | 'memory') => {
    const key = `${pod.namespace}/${pod.name}`
    const m = podMetrics.get(key)
    if (!m) {
      return <span style={{ color: '#999' }}>—</span>
    }
    if (type === 'cpu') {
      return (
        <Tooltip title={`CPU: ${m.cpu_cores}`}>
          <span>{m.cpu_cores}</span>
        </Tooltip>
      )
    }
    return (
      <Tooltip title={`Memory: ${m.memory_bytes}`}>
        <span>{m.memory_bytes}</span>
      </Tooltip>
    )
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
    {
      title: 'CPU',
      key: 'cpu',
      width: 80,
      render: (_: any, record: Pod) => renderPodMetric(record, 'cpu'),
    },
    {
      title: 'Memory',
      key: 'memory',
      width: 90,
      render: (_: any, record: Pod) => renderPodMetric(record, 'memory'),
    },
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
          <Alert
            message="数据刷新失败"
            description={trust.error + '（当前显示的是上次成功获取的数据）'}
            type="warning"
            showIcon
            style={{ marginBottom: 12 }}
          />
        )}
        {metricsError && podMetrics.size === 0 && (
          <Alert message="Metrics Server 不可用" description={metricsError + '（CPU/Memory 指标暂不可用）'} type="info" showIcon style={{ marginBottom: 12 }} />
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
