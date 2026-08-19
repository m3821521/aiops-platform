import { useEffect, useState } from 'react'
import { Row, Col, Card, Statistic, Typography, Space, Tag, Spin, Alert } from 'antd'
import {
  CloudOutlined,
  NodeIndexOutlined,
  AppstoreOutlined,
  AlertOutlined,
  WarningOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons'
import { clusterApi } from '@/api/cluster'
import { k8sApi } from '@/api/kubernetes'
import type { Cluster, Node, Pod } from '@/types'

const { Title } = Typography

export default function Dashboard() {
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [nodes, setNodes] = useState<Node[]>([])
  const [pods, setPods] = useState<Pod[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string>('')

  const fetchData = async () => {
    setLoading(true)
    setError('')
    try {
      const clusterList = await clusterApi.list()
      setClusters(clusterList || [])

      if (clusterList && clusterList.length > 0) {
        const firstCluster = clusterList[0].name
        const [nodeList, podList] = await Promise.all([
          k8sApi.nodes(firstCluster).catch(() => [] as Node[]),
          k8sApi.pods({ cluster: firstCluster }).catch(() => [] as Pod[]),
        ])
        setNodes(nodeList || [])
        setPods(podList || [])
      } else {
        setNodes([])
        setPods([])
      }
    } catch (err: any) {
      setError(err?.message || '数据加载失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchData()
  }, [])

  const runningPods = pods.filter((p) => p.status === 'Running').length

  return (
    <div>
      <Space style={{ marginBottom: 24 }} align="center">
        <Title level={4} style={{ margin: 0 }}>运维总览</Title>
        <Tag color="green">系统正常</Tag>
      </Space>

      {error && (
        <Alert message="数据加载失败" description={error} type="error" showIcon style={{ marginBottom: 16 }} />
      )}

      <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
        <Col xs={12} sm={8} md={6} lg={4}>
          <Card className="stat-card">
            <Spin spinning={loading}>
              <Statistic title="集群" value={clusters.length} prefix={<CloudOutlined style={{ color: '#1677ff' }} />} />
            </Spin>
          </Card>
        </Col>
        <Col xs={12} sm={8} md={6} lg={4}>
          <Card className="stat-card">
            <Spin spinning={loading}>
              <Statistic title="Node" value={nodes.length} prefix={<NodeIndexOutlined style={{ color: '#52c41a' }} />} />
            </Spin>
          </Card>
        </Col>
        <Col xs={12} sm={8} md={6} lg={4}>
          <Card className="stat-card">
            <Spin spinning={loading}>
              <Statistic
                title="Pod"
                value={pods.length}
                prefix={<AppstoreOutlined style={{ color: '#722ed1' }} />}
                suffix={pods.length > 0 ? <span style={{ fontSize: 14, color: '#999' }}>({runningPods} Running)</span> : null}
              />
            </Spin>
          </Card>
        </Col>
        <Col xs={12} sm={8} md={6} lg={4}>
          <Card className="stat-card">
            <Statistic title="当前告警" value={0} prefix={<AlertOutlined style={{ color: '#faad14' }} />} />
          </Card>
        </Col>
        <Col xs={12} sm={8} md={6} lg={4}>
          <Card className="stat-card">
            <Statistic title="严重告警" value={0} prefix={<WarningOutlined style={{ color: '#ff4d4f' }} />} />
          </Card>
        </Col>
        <Col xs={12} sm={8} md={6} lg={4}>
          <Card className="stat-card">
            <Statistic title="异常服务" value={0} prefix={<ThunderboltOutlined style={{ color: '#eb2f96' }} />} />
          </Card>
        </Col>
      </Row>

      {clusters.length > 0 && (
        <Card title="集群状态" style={{ marginBottom: 24 }}>
          <Row gutter={[16, 16]}>
            {clusters.map((c) => (
              <Col xs={24} sm={12} md={8} key={c.name}>
                <Card size="small" type="inner">
                  <Space direction="vertical" style={{ width: '100%' }}>
                    <Space>
                      <CloudOutlined style={{ color: '#1677ff' }} />
                      <span style={{ fontWeight: 600 }}>{c.name}</span>
                      <Tag color={c.enabled ? 'success' : 'default'}>{c.enabled ? '已启用' : '已禁用'}</Tag>
                    </Space>
                    <div style={{ color: '#666', fontSize: 13 }}>{c.description || '-'}</div>
                    <div style={{ fontSize: 12, color: '#999' }}>
                      认证: {c.auth_type} | Nodes: {nodes.length} | Pods: {pods.length}
                    </div>
                  </Space>
                </Card>
              </Col>
            ))}
          </Row>
        </Card>
      )}

      <Row gutter={[16, 16]}>
        <Col xs={24} lg={16}>
          <Card title="资源趋势" style={{ height: 320 }}>
            <div style={{ color: '#999', textAlign: 'center', paddingTop: 100 }}>
              CPU / Memory / Network 趋势图表（待实现）
            </div>
          </Card>
        </Col>
        <Col xs={24} lg={8}>
          <Card title="Top 告警" style={{ height: 320 }}>
            <div style={{ color: '#999', textAlign: 'center', paddingTop: 100 }}>
              告警排行（待实现）
            </div>
          </Card>
        </Col>
      </Row>
    </div>
  )
}
