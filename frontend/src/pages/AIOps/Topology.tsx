import { useEffect, useRef, useState, useCallback } from 'react'
import {
  Card,
  Row,
  Col,
  Select,
  Input,
  Button,
  Space,
  Drawer,
  Descriptions,
  Tag,
  Empty,
  Spin,
  Badge,
  Typography,
} from 'antd'
import { ReloadOutlined, ApartmentOutlined } from '@ant-design/icons'
import * as echarts from 'echarts'
import { topologyApi } from '../../api/topology'
import { clusterApi } from '../../api/cluster'
import type { TopologyGraph, TopologyNode, TopologyNodeType, TopologyNodeStatus } from '../../types'

const { Text } = Typography

import { useDataTrust } from '@/hooks/useDataTrust'
import { DataTrustIndicator } from '@/components/DataTrustIndicator'

const nodeColor: Record<TopologyNodeType, string> = {
  node: '#722ed1',
  pod: '#1890ff',
  deployment: '#52c41a',
  service: '#faad14',
  ingress: '#eb2f96',
}

const statusColor: Record<TopologyNodeStatus, string> = {
  healthy: '#52c41a',
  warning: '#faad14',
  critical: '#ff4d4f',
  unknown: '#8c8c8c',
}

const nodeSize: Record<TopologyNodeType, number> = {
  node: 50,
  pod: 30,
  deployment: 40,
  service: 35,
  ingress: 45,
}

export default function Topology() {
  const chartRef = useRef<HTMLDivElement>(null)
  const chartInstance = useRef<echarts.ECharts | null>(null)
  const [clusters, setClusters] = useState<string[]>([])
  const [cluster, setCluster] = useState('local')
  const [namespace, setNamespace] = useState('')
  const [resourceType, setResourceType] = useState<TopologyNodeType | ''>('')
  const [keyword, setKeyword] = useState('')
  const [graph, setGraph] = useState<TopologyGraph | null>(null)
  const [loading, setLoading] = useState(false)
  const [selectedNode, setSelectedNode] = useState<TopologyNode | null>(null)

  // P1-X.9: 统一 Data Trust（Topology: K8s + Prometheus，60s Redis cache）
  const trust = useDataTrust({ source: 'topology' })

  const fetchClusters = useCallback(async () => {
    try {
      const list = await clusterApi.list()
      const names = list?.map((c: any) => c.name) || []
      setClusters(names.length > 0 ? names : ['local'])
      // 自动选择第一个有效集群（如果当前 cluster 不在列表中）
      if (names.length > 0 && !names.includes(cluster)) {
        setCluster(names[0])
      }
    } catch {
      setClusters(['local'])
    }
  }, [cluster])

  const fetchGraph = useCallback(async () => {
    const seq = trust.beginFetch()
    setLoading(true)
    try {
      const data = await topologyApi.getGraph({ cluster, namespace: namespace || undefined, refresh: true })
      trust.markSuccess(seq)
      setGraph(data)
    } catch (e: any) {
      trust.markError(seq, e?.message || '获取拓扑数据失败')
    } finally {
      setLoading(false)
    }
  }, [cluster, namespace])

  useEffect(() => {
    fetchClusters()
  }, [fetchClusters])

  useEffect(() => {
    fetchGraph()
  }, [fetchGraph])

  // P1-PRODUCT-06: 30s 自动轮询（Topology 缓存 60s，轮询 30s 可接受）
  useEffect(() => {
    const timer = window.setInterval(() => {
      fetchGraph()
    }, 30000)
    return () => clearInterval(timer)
  }, [fetchGraph])

  // 渲染 ECharts Graph
  useEffect(() => {
    if (!graph || !chartRef.current) return

    if (!chartInstance.current) {
      chartInstance.current = echarts.init(chartRef.current)
      chartInstance.current.on('click', (params: any) => {
        if (params.dataType === 'node') {
          const node = graph.nodes.find((n) => n.id === params.data.id)
          if (node) setSelectedNode(node)
        }
      })
    }

    // 过滤节点
    let nodes = graph.nodes
    if (resourceType) {
      nodes = nodes.filter((n) => n.type === resourceType)
    }
    if (keyword) {
      const kw = keyword.toLowerCase()
      nodes = nodes.filter(
        (n) => n.name.toLowerCase().includes(kw) || (n.namespace || '').toLowerCase().includes(kw)
      )
    }
    const nodeIds = new Set(nodes.map((n) => n.id))
    const edges = graph.edges.filter((e) => nodeIds.has(e.source) && nodeIds.has(e.target))

    const option: echarts.EChartsOption = {
      tooltip: {
        formatter: (params: any) => {
          if (params.dataType === 'node') {
            const n = params.data
            return `<b>${n.name}</b><br/>type: ${n.category}<br/>namespace: ${n.namespace || '-'}<br/>status: ${n.status}`
          }
          return params.data.relation
        },
      },
      legend: [
        {
          data: ['node', 'pod', 'deployment', 'service', 'ingress'],
          top: 10,
          textStyle: { fontSize: 11 },
        },
      ],
      series: [
        {
          type: 'graph',
          layout: 'force',
          roam: true,
          draggable: true,
          force: {
            repulsion: 300,
            edgeLength: [80, 150],
            gravity: 0.1,
          },
          label: {
            show: true,
            position: 'bottom',
            fontSize: 10,
            formatter: (p: any) => p.data.name.slice(0, 20),
          },
          edgeSymbol: ['none', 'arrow'],
          edgeSymbolSize: 8,
          lineStyle: {
            color: '#8c8c8c',
            width: 1.5,
            curveness: 0.1,
          },
          emphasis: {
            focus: 'adjacency',
            lineStyle: { width: 3 },
          },
          categories: [
            { name: 'node', itemStyle: { color: nodeColor.node } },
            { name: 'pod', itemStyle: { color: nodeColor.pod } },
            { name: 'deployment', itemStyle: { color: nodeColor.deployment } },
            { name: 'service', itemStyle: { color: nodeColor.service } },
            { name: 'ingress', itemStyle: { color: nodeColor.ingress } },
          ],
          data: nodes.map((n) => ({
            id: n.id,
            name: n.name,
            category: n.type,
            symbolSize: nodeSize[n.type] || 30,
            status: n.status,
            namespace: n.namespace,
            itemStyle: {
              borderColor: statusColor[n.status],
              borderWidth: n.status === 'healthy' ? 0 : 3,
            },
          })),
          links: edges.map((e) => ({
            source: e.source,
            target: e.target,
            relation: e.relation,
            label: { show: true, formatter: e.relation, fontSize: 9 },
          })),
        },
      ],
    }

    chartInstance.current.setOption(option, true)

    const handleResize = () => chartInstance.current?.resize()
    window.addEventListener('resize', handleResize)
    return () => {
      window.removeEventListener('resize', handleResize)
    }
  }, [graph, resourceType, keyword])

  useEffect(() => {
    return () => {
      chartInstance.current?.dispose()
      chartInstance.current = null
    }
  }, [])

  return (
    <div>
      <Card
        title={
          <Space>
            <ApartmentOutlined />
            <span>服务拓扑</span>
            {graph && (
              <Text type="secondary" style={{ fontSize: 12 }}>
                {graph.nodes.length} 节点 · {graph.edges.length} 关系
              </Text>
            )}
            {trust.lastSuccessfulAt && (
              <Text type="secondary" style={{ fontSize: 12 }}>
                · {trust.formatAge()} · 自动刷新 30s
              </Text>
            )}
          </Space>
        }
        extra={
          <Button icon={<ReloadOutlined />} onClick={fetchGraph} loading={loading}>
            刷新
          </Button>
        }
      >
        <div style={{ marginBottom: 12 }}>
          <DataTrustIndicator
            status={trust.status}
            lastSuccessfulAt={trust.lastSuccessfulAt}
            ageSeconds={trust.ageSeconds}
            sourceLabel={trust.sourceLabel}
            error={trust.error}
            formatAge={trust.formatAge}
            formatLastSuccessful={trust.formatLastSuccessful}
          />
        </div>
        <Row gutter={[12, 12]} style={{ marginBottom: 16 }}>
          <Col span={4}>
            <Select
              style={{ width: '100%' }}
              value={cluster}
              onChange={setCluster}
              options={clusters.map((c) => ({ label: c, value: c }))}
            />
          </Col>
          <Col span={4}>
            <Select
              style={{ width: '100%' }}
              placeholder="Namespace"
              allowClear
              value={namespace || undefined}
              onChange={(v) => setNamespace(v || '')}
              options={[
                { label: '全部', value: '' },
                ...(graph
                  ? Array.from(new Set(graph.nodes.map((n) => n.namespace).filter(Boolean))).map((ns) => ({
                      label: ns,
                      value: ns,
                    }))
                  : []),
              ]}
            />
          </Col>
          <Col span={4}>
            <Select
              style={{ width: '100%' }}
              placeholder="资源类型"
              allowClear
              value={resourceType || undefined}
              onChange={(v) => setResourceType(v || '')}
              options={[
                { label: 'Node', value: 'node' },
                { label: 'Pod', value: 'pod' },
                { label: 'Deployment', value: 'deployment' },
                { label: 'Service', value: 'service' },
                { label: 'Ingress', value: 'ingress' },
              ]}
            />
          </Col>
          <Col span={6}>
            <Input
              placeholder="搜索名称/Namespace"
              value={keyword}
              onChange={(e) => setKeyword(e.target.value)}
              allowClear
            />
          </Col>
          <Col span={6}>
            <Space size="small" wrap>
              <Badge color={nodeColor.node} text="Node" />
              <Badge color={nodeColor.pod} text="Pod" />
              <Badge color={nodeColor.deployment} text="Deployment" />
              <Badge color={nodeColor.service} text="Service" />
              <Badge color={nodeColor.ingress} text="Ingress" />
              <span style={{ marginLeft: 8 }}>|</span>
              <Badge color={statusColor.critical} text="Critical" />
              <Badge color={statusColor.warning} text="Warning" />
              <Badge color={statusColor.healthy} text="Healthy" />
            </Space>
          </Col>
        </Row>

        <Spin spinning={loading}>
          {graph && graph.nodes.length > 0 ? (
            <div ref={chartRef} style={{ height: 560, width: '100%' }} />
          ) : (
            <Empty description="暂无拓扑数据" style={{ padding: 80 }} />
          )}
        </Spin>
      </Card>

      <Drawer
        title={
          <Space>
            <Tag color={nodeColor[selectedNode?.type || 'pod']}>{selectedNode?.type}</Tag>
            <span>{selectedNode?.name}</span>
          </Space>
        }
        width={480}
        open={!!selectedNode}
        onClose={() => setSelectedNode(null)}
      >
        {selectedNode && (
          <div>
            <Descriptions column={1} size="small" bordered style={{ marginBottom: 16 }}>
              <Descriptions.Item label="类型">{selectedNode.type}</Descriptions.Item>
              <Descriptions.Item label="名称">{selectedNode.name}</Descriptions.Item>
              <Descriptions.Item label="Namespace">{selectedNode.namespace || '-'}</Descriptions.Item>
              <Descriptions.Item label="集群">{selectedNode.cluster}</Descriptions.Item>
              <Descriptions.Item label="状态">
                <Tag color={statusColor[selectedNode.status]}>{selectedNode.status}</Tag>
              </Descriptions.Item>
              {selectedNode.anomaly_count ? (
                <Descriptions.Item label="异常数">{selectedNode.anomaly_count}</Descriptions.Item>
              ) : null}
              {selectedNode.incident_ids && selectedNode.incident_ids.length > 0 ? (
                <Descriptions.Item label="关联事件">
                  {selectedNode.incident_ids.map((id) => (
                    <Tag key={id} color="purple">
                      #{id}
                    </Tag>
                  ))}
                </Descriptions.Item>
              ) : null}
            </Descriptions>

            {selectedNode.labels && Object.keys(selectedNode.labels).length > 0 && (
              <Card title="Labels" size="small" style={{ marginBottom: 16 }}>
                <Space size={[4, 4]} wrap>
                  {Object.entries(selectedNode.labels).map(([k, v]) => (
                    <Tag key={k}>
                      {k}={v}
                    </Tag>
                  ))}
                </Space>
              </Card>
            )}

            {selectedNode.metadata && Object.keys(selectedNode.metadata).length > 0 && (
              <Card title="元数据" size="small">
                <Descriptions column={1} size="small">
                  {Object.entries(selectedNode.metadata).map(([k, v]) => (
                    <Descriptions.Item key={k} label={k}>
                      {String(v)}
                    </Descriptions.Item>
                  ))}
                </Descriptions>
              </Card>
            )}
          </div>
        )}
      </Drawer>
    </div>
  )
}
