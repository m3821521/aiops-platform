import { Row, Col, Card, Statistic, Typography, Space, Tag } from 'antd'
import {
  CloudOutlined,
  NodeIndexOutlined,
  AppstoreOutlined,
  AlertOutlined,
  WarningOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons'

const { Title } = Typography

export default function Dashboard() {
  return (
    <div>
      <Space style={{ marginBottom: 24 }} align="center">
        <Title level={4} style={{ margin: 0 }}>运维总览</Title>
        <Tag color="green">系统正常</Tag>
      </Space>

      {/* 资源统计 */}
      <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
        <Col xs={12} sm={8} md={6} lg={4}>
          <Card className="stat-card">
            <Statistic
              title="集群"
              value={0}
              prefix={<CloudOutlined style={{ color: '#1677ff' }} />}
            />
          </Card>
        </Col>
        <Col xs={12} sm={8} md={6} lg={4}>
          <Card className="stat-card">
            <Statistic
              title="Node"
              value={0}
              prefix={<NodeIndexOutlined style={{ color: '#52c41a' }} />}
            />
          </Card>
        </Col>
        <Col xs={12} sm={8} md={6} lg={4}>
          <Card className="stat-card">
            <Statistic
              title="Pod"
              value={0}
              prefix={<AppstoreOutlined style={{ color: '#722ed1' }} />}
            />
          </Card>
        </Col>
        <Col xs={12} sm={8} md={6} lg={4}>
          <Card className="stat-card">
            <Statistic
              title="当前告警"
              value={0}
              prefix={<AlertOutlined style={{ color: '#faad14' }} />}
            />
          </Card>
        </Col>
        <Col xs={12} sm={8} md={6} lg={4}>
          <Card className="stat-card">
            <Statistic
              title="严重告警"
              value={0}
              prefix={<WarningOutlined style={{ color: '#ff4d4f' }} />}
            />
          </Card>
        </Col>
        <Col xs={12} sm={8} md={6} lg={4}>
          <Card className="stat-card">
            <Statistic
              title="异常服务"
              value={0}
              prefix={<ThunderboltOutlined style={{ color: '#eb2f96' }} />}
            />
          </Card>
        </Col>
      </Row>

      {/* 占位区域，Phase 2 填充图表 */}
      <Row gutter={[16, 16]}>
        <Col xs={24} lg={16}>
          <Card title="资源趋势" style={{ height: 320 }}>
            <div style={{ color: '#999', textAlign: 'center', paddingTop: 100 }}>
              CPU / Memory / Network 趋势图表（Phase 2 实现）
            </div>
          </Card>
        </Col>
        <Col xs={24} lg={8}>
          <Card title="Top 告警" style={{ height: 320 }}>
            <div style={{ color: '#999', textAlign: 'center', paddingTop: 100 }}>
              告警排行（Phase 2 实现）
            </div>
          </Card>
        </Col>
      </Row>
    </div>
  )
}
