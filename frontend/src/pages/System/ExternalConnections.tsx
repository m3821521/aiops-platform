import { useState, useEffect } from 'react';
import {
  Table, Button, Modal, Form, Input, Select, Tag, Space, Popconfirm,
  message, Card, Row, Col, Statistic, Tooltip, Badge, Switch, Alert
} from 'antd';
import {
  PlusOutlined, ReloadOutlined, DeleteOutlined, EditOutlined,
  PlayCircleOutlined, StopOutlined, ApiOutlined, KeyOutlined,
  CheckCircleOutlined, CloseCircleOutlined, MinusCircleOutlined
} from '@ant-design/icons';
import { connectionApi, credentialApi } from '../../api/connection';
import type { ConnectionView, Credential, TestConnectionResult } from '../../api/connection';

const { Option } = Select;
const { TextArea } = Input;

// 使用从 api/connection 导入的类型: Connection, ConnectionView, Credential, TestConnectionResult

const CONNECTION_TYPES = [
  { value: 'kubernetes', label: 'Kubernetes', color: 'blue' },
  { value: 'prometheus', label: 'Prometheus', color: 'orange' },
  { value: 'elasticsearch', label: 'Elasticsearch', color: 'green' },
  { value: 'jenkins', label: 'Jenkins', color: 'red' },
  { value: 'argocd', label: 'ArgoCD', color: 'purple' },
  { value: 'grafana', label: 'Grafana', color: 'geekblue' },
  { value: 'mysql', label: 'MySQL', color: 'cyan' },
  { value: 'redis', label: 'Redis', color: 'magenta' },
];

const ENVIRONMENTS = [
  { value: 'dev', label: '开发' },
  { value: 'test', label: '测试' },
  { value: 'staging', label: '预发布' },
  { value: 'prod', label: '生产' },
];

const CREDENTIAL_TYPES = [
  { value: 'username_password', label: '用户名/密码' },
  { value: 'token', label: 'Token' },
  { value: 'api_key', label: 'API Key' },
  { value: 'tls', label: 'TLS 证书' },
  { value: 'kubeconfig', label: 'Kubeconfig' },
];

export default function ExternalConnections() {
  const [connections, setConnections] = useState<ConnectionView[]>([]);
  const [credentials, setCredentials] = useState<Credential[]>([]);
  const [loading, setLoading] = useState(false);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);

  const [connectionModalVisible, setConnectionModalVisible] = useState(false);
  const [credentialModalVisible, setCredentialModalVisible] = useState(false);
  const [testModalVisible, setTestModalVisible] = useState(false);
  const [editingConnection, setEditingConnection] = useState<ConnectionView | null>(null);
  const [testResult, setTestResult] = useState<TestConnectionResult | null>(null);
  const [testingId, setTestingId] = useState<number | null>(null);

  const [connectionForm] = Form.useForm();
  const [credentialForm] = Form.useForm();

  useEffect(() => {
    loadConnections();
    loadCredentials();
  }, [page, pageSize]);

  const loadConnections = async () => {
    setLoading(true);
    try {
      const res = await connectionApi.list({ page, page_size: pageSize });
      setConnections(res.items || []);
      setTotal(res.total || 0);
    } catch (err: any) {
      message.error('加载连接列表失败: ' + (err.response?.data?.message || err.message));
    } finally {
      setLoading(false);
    }
  };

  const loadCredentials = async () => {
    try {
      const res = await credentialApi.list({ page: 1, page_size: 100 });
      setCredentials(res.items || []);
    } catch (err: any) {
      console.error('加载凭证列表失败:', err);
    }
  };

  const handleCreateConnection = () => {
    setEditingConnection(null);
    connectionForm.resetFields();
    setConnectionModalVisible(true);
  };

  const handleEditConnection = (record: ConnectionView) => {
    setEditingConnection(record);
    connectionForm.setFieldsValue({
      name: record.name,
      type: record.type,
      environment: record.environment,
      endpoint: record.endpoint,
      credential_id: record.credential_id,
      enabled: record.enabled,
      description: record.description,
    });
    setConnectionModalVisible(true);
  };

  const handleSaveConnection = async (values: any) => {
    try {
      if (editingConnection) {
        await connectionApi.update(editingConnection.id, values);
        message.success('连接更新成功');
      } else {
        await connectionApi.create(values);
        message.success('连接创建成功');
      }
      setConnectionModalVisible(false);
      loadConnections();
    } catch (err: any) {
      message.error('保存连接失败: ' + (err.response?.data?.message || err.message));
    }
  };

  const handleDeleteConnection = async (id: number) => {
    try {
      await connectionApi.delete(id);
      message.success('连接删除成功');
      loadConnections();
    } catch (err: any) {
      message.error('删除连接失败: ' + (err.response?.data?.message || err.message));
    }
  };

  const handleToggleConnection = async (record: ConnectionView, enabled: boolean) => {
    try {
      if (enabled) {
        await connectionApi.enable(record.id);
      } else {
        await connectionApi.disable(record.id);
      }
      message.success(enabled ? '连接已启用' : '连接已禁用');
      loadConnections();
    } catch (err: any) {
      message.error('操作失败: ' + (err.response?.data?.message || err.message));
    }
  };

  const handleTestConnection = async (record: ConnectionView) => {
    setTestingId(record.id);
    try {
      const res = await connectionApi.test(record.id);
      setTestResult(res);
      setTestModalVisible(true);
      loadConnections();
    } catch (err: any) {
      message.error('测试连接失败: ' + (err.response?.data?.message || err.message));
    } finally {
      setTestingId(null);
    }
  };

  const handleCreateCredential = () => {
    credentialForm.resetFields();
    setCredentialModalVisible(true);
  };

  const handleSaveCredential = async (values: any) => {
    try {
      const data: Record<string, string> = {};
      if (values.credential_type === 'username_password') {
        data.username = values.username;
        data.password = values.password;
      } else if (values.credential_type === 'token') {
        data.token = values.token;
      } else if (values.credential_type === 'api_key') {
        data.api_key = values.api_key;
      } else if (values.credential_type === 'kubeconfig') {
        data.kubeconfig = values.kubeconfig;
      } else if (values.credential_type === 'tls') {
        data.certificate = values.certificate;
        data.private_key = values.private_key;
      }

      await credentialApi.create({
        name: values.name,
        type: values.credential_type,
        description: values.description,
        data,
      });
      message.success('凭证创建成功');
      setCredentialModalVisible(false);
      loadCredentials();
    } catch (err: any) {
      message.error('保存凭证失败: ' + (err.response?.data?.message || err.message));
    }
  };

  const getTypeColor = (type: string) => {
    const found = CONNECTION_TYPES.find(t => t.value === type);
    return found?.color || 'default';
  };

  const getStatusTag = (status: string) => {
    switch (status) {
      case 'available':
        return <Tag icon={<CheckCircleOutlined />} color="success">可用</Tag>;
      case 'unavailable':
        return <Tag icon={<CloseCircleOutlined />} color="error">不可用</Tag>;
      default:
        return <Tag icon={<MinusCircleOutlined />} color="default">未知</Tag>;
    }
  };

  const columns = [
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
      render: (text: string, record: ConnectionView) => (
        <Space>
          <span style={{ fontWeight: 500 }}>{text}</span>
          {record.is_system_default && <Tag color="blue" style={{ fontSize: 11 }}>系统默认</Tag>}
        </Space>
      ),
    },
    {
      title: '类型',
      dataIndex: 'type',
      key: 'type',
      render: (type: string) => (
        <Tag color={getTypeColor(type)}>
          {CONNECTION_TYPES.find(t => t.value === type)?.label || type}
        </Tag>
      ),
    },
    {
      title: '环境',
      dataIndex: 'environment',
      key: 'environment',
      render: (env: string) => ENVIRONMENTS.find(e => e.value === env)?.label || env,
    },
    {
      title: '端点',
      dataIndex: 'endpoint',
      key: 'endpoint',
      ellipsis: true,
      render: (text: string) => <code style={{ fontSize: 12 }}>{text}</code>,
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status: string) => getStatusTag(status),
    },
    {
      title: '启用',
      dataIndex: 'enabled',
      key: 'enabled',
      render: (enabled: boolean, record: ConnectionView) => (
        <Switch
          checked={enabled}
          onChange={(checked: boolean) => handleToggleConnection(record, checked)}
          disabled={record.is_system_default}
        />
      ),
    },
    {
      title: '最后检查',
      dataIndex: 'last_check_at',
      key: 'last_check_at',
      render: (time?: string) => time ? new Date(time).toLocaleString('zh-CN') : '-',
    },
    {
      title: '操作',
      key: 'actions',
      width: 200,
      render: (_: any, record: ConnectionView) => (
        <Space size="small">
          <Tooltip title="测试连接">
            <Button
              type="link"
              size="small"
              icon={<ApiOutlined />}
              loading={testingId === record.id}
              onClick={() => handleTestConnection(record)}
            />
          </Tooltip>
          <Tooltip title="编辑">
            <Button
              type="link"
              size="small"
              icon={<EditOutlined />}
              onClick={() => handleEditConnection(record)}
              disabled={record.is_system_default}
            />
          </Tooltip>
          <Popconfirm
            title="确定删除此连接？"
            onConfirm={() => handleDeleteConnection(record.id)}
            okText="删除"
            cancelText="取消"
            disabled={record.is_system_default}
          >
            <Button
              type="link"
              size="small"
              danger
              icon={<DeleteOutlined />}
              disabled={record.is_system_default}
            />
          </Popconfirm>
        </Space>
      ),
    },
  ];

  const stats = {
    total: connections.length,
    available: connections.filter(c => c.status === 'available').length,
    unavailable: connections.filter(c => c.status === 'unavailable').length,
    enabled: connections.filter(c => c.enabled).length,
  };

  return (
    <div style={{ padding: 24 }}>
      <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
        <Col span={6}>
          <Card>
            <Statistic title="连接总数" value={stats.total} prefix={<ApiOutlined />} />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic title="可用" value={stats.available} valueStyle={{ color: '#52c41a' }} prefix={<CheckCircleOutlined />} />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic title="不可用" value={stats.unavailable} valueStyle={{ color: '#ff4d4f' }} prefix={<CloseCircleOutlined />} />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic title="已启用" value={stats.enabled} prefix={<PlayCircleOutlined />} />
          </Card>
        </Col>
      </Row>

      <Card
        title="外部连接管理"
        extra={
          <Space>
            <Button icon={<KeyOutlined />} onClick={handleCreateCredential}>
              管理凭证
            </Button>
            <Button type="primary" icon={<PlusOutlined />} onClick={handleCreateConnection}>
              新建连接
            </Button>
            <Button icon={<ReloadOutlined />} onClick={loadConnections}>
              刷新
            </Button>
          </Space>
        }
      >
        <Table
          columns={columns}
          dataSource={connections}
          rowKey="id"
          loading={loading}
          pagination={{
            current: page,
            pageSize,
            total,
            showSizeChanger: true,
            showQuickJumper: true,
            showTotal: (total) => `共 ${total} 条`,
            onChange: (p, ps) => { setPage(p); setPageSize(ps); },
          }}
        />
      </Card>

      {/* 连接编辑弹窗 */}
      <Modal
        title={editingConnection ? '编辑连接' : '新建连接'}
        open={connectionModalVisible}
        onCancel={() => setConnectionModalVisible(false)}
        footer={null}
        width={600}
      >
        <Form
          form={connectionForm}
          layout="vertical"
          onFinish={handleSaveConnection}
          initialValues={{ enabled: true, environment: 'dev' }}
        >
          <Form.Item name="name" label="连接名称" rules={[{ required: true, message: '请输入连接名称' }]}>
            <Input placeholder="例如: prod-kubernetes" />
          </Form.Item>
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="type" label="连接类型" rules={[{ required: true }]}>
                <Select placeholder="选择类型">
                  {CONNECTION_TYPES.map(t => (
                    <Option key={t.value} value={t.value}>{t.label}</Option>
                  ))}
                </Select>
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="environment" label="环境" rules={[{ required: true }]}>
                <Select placeholder="选择环境">
                  {ENVIRONMENTS.map(e => (
                    <Option key={e.value} value={e.value}>{e.label}</Option>
                  ))}
                </Select>
              </Form.Item>
            </Col>
          </Row>
          <Form.Item name="endpoint" label="端点地址" rules={[{ required: true, message: '请输入端点地址' }]}>
            <Input placeholder="例如: https://192.168.1.100:6443" />
          </Form.Item>
          <Form.Item name="credential_id" label="关联凭证">
            <Select placeholder="选择凭证（可选）" allowClear>
              {credentials.map(c => (
                <Option key={c.id} value={c.id}>
                  {c.name} ({CREDENTIAL_TYPES.find(t => t.value === c.type)?.label || c.type})
                </Option>
              ))}
            </Select>
          </Form.Item>
          <Form.Item name="enabled" label="启用状态" valuePropName="checked">
            <Switch />
          </Form.Item>
          <Form.Item name="description" label="描述">
            <TextArea rows={2} placeholder="连接描述（可选）" />
          </Form.Item>
          <Form.Item>
            <Space>
              <Button type="primary" htmlType="submit">保存</Button>
              <Button onClick={() => setConnectionModalVisible(false)}>取消</Button>
            </Space>
          </Form.Item>
        </Form>
      </Modal>

      {/* 凭证创建弹窗 */}
      <Modal
        title="新建凭证"
        open={credentialModalVisible}
        onCancel={() => setCredentialModalVisible(false)}
        footer={null}
        width={600}
      >
        <Form
          form={credentialForm}
          layout="vertical"
          onFinish={handleSaveCredential}
        >
          <Form.Item name="name" label="凭证名称" rules={[{ required: true }]}>
            <Input placeholder="例如: prod-k8s-token" />
          </Form.Item>
          <Form.Item name="credential_type" label="凭证类型" rules={[{ required: true }]}>
            <Select placeholder="选择凭证类型">
              {CREDENTIAL_TYPES.map(t => (
                <Option key={t.value} value={t.value}>{t.label}</Option>
              ))}
            </Select>
          </Form.Item>
          <Form.Item noStyle shouldUpdate={(prev, cur) => prev.credential_type !== cur.credential_type}>
            {({ getFieldValue }) => {
              const type = getFieldValue('credential_type');
              return (
                <>
                  {type === 'username_password' && (
                    <Row gutter={16}>
                      <Col span={12}>
                        <Form.Item name="username" label="用户名" rules={[{ required: true }]}>
                          <Input />
                        </Form.Item>
                      </Col>
                      <Col span={12}>
                        <Form.Item name="password" label="密码" rules={[{ required: true }]}>
                          <Input.Password />
                        </Form.Item>
                      </Col>
                    </Row>
                  )}
                  {type === 'token' && (
                    <Form.Item name="token" label="Token" rules={[{ required: true }]}>
                      <Input.Password placeholder="Bearer Token" />
                    </Form.Item>
                  )}
                  {type === 'api_key' && (
                    <Form.Item name="api_key" label="API Key" rules={[{ required: true }]}>
                      <Input.Password placeholder="API Key" />
                    </Form.Item>
                  )}
                  {type === 'kubeconfig' && (
                    <Form.Item name="kubeconfig" label="Kubeconfig 内容" rules={[{ required: true }]}>
                      <TextArea rows={6} placeholder="粘贴 kubeconfig YAML 内容（或 Base64 编码）" />
                    </Form.Item>
                  )}
                  {type === 'tls' && (
                    <>
                      <Form.Item name="certificate" label="证书" rules={[{ required: true }]}>
                        <TextArea rows={3} placeholder="TLS 证书内容" />
                      </Form.Item>
                      <Form.Item name="private_key" label="私钥" rules={[{ required: true }]}>
                        <TextArea rows={3} placeholder="TLS 私钥内容" />
                      </Form.Item>
                    </>
                  )}
                </>
              );
            }}
          </Form.Item>
          <Form.Item name="description" label="描述">
            <Input placeholder="凭证描述（可选）" />
          </Form.Item>
          <Form.Item>
            <Space>
              <Button type="primary" htmlType="submit">创建</Button>
              <Button onClick={() => setCredentialModalVisible(false)}>取消</Button>
            </Space>
          </Form.Item>
        </Form>
      </Modal>

      {/* 测试结果弹窗 */}
      <Modal
        title="连接测试结果"
        open={testModalVisible}
        onCancel={() => setTestModalVisible(false)}
        footer={[
          <Button key="close" onClick={() => setTestModalVisible(false)}>关闭</Button>,
        ]}
      >
        {testResult && (
          <div>
            <Row gutter={16} style={{ marginBottom: 16 }}>
              <Col span={8}>
                <Statistic
                  title="状态"
                  value={testResult.status === 'available' ? '可用' : '不可用'}
                  valueStyle={{ color: testResult.status === 'available' ? '#52c41a' : '#ff4d4f' }}
                />
              </Col>
              <Col span={8}>
                <Statistic title="延迟" value={testResult.latency_ms} suffix="ms" />
              </Col>
              <Col span={8}>
                <Statistic title="检查时间" value={new Date(testResult.checked_at).toLocaleTimeString('zh-CN')} />
              </Col>
            </Row>
            {testResult.error_code && (
              <Alert
                message={`错误码: ${testResult.error_code}`}
                description={testResult.error_message}
                type="error"
                showIcon
              />
            )}
            {!testResult.error_code && testResult.error_message && (
              <Alert
                message="测试详情"
                description={testResult.error_message}
                type="success"
                showIcon
              />
            )}
          </div>
        )}
      </Modal>
    </div>
  );
}
