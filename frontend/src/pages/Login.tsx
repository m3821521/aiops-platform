import { Form, Input, Button, Card, message, ConfigProvider, theme as antdTheme } from 'antd'
import { UserOutlined, LockOutlined } from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { useAuthStore } from '@/stores/auth'
import { useState } from 'react'

export default function Login() {
  const navigate = useNavigate()
  const login = useAuthStore((s) => s.login)
  const [loading, setLoading] = useState(false)

  const onFinish = async (values: { username: string; password: string }) => {
    setLoading(true)
    try {
      await login(values.username, values.password)
      message.success('登录成功')
      navigate('/')
    } catch {
      // 错误已在拦截器处理
    } finally {
      setLoading(false)
    }
  }

  return (
    <ConfigProvider theme={{ algorithm: antdTheme.darkAlgorithm }}>
      <div className="login-bg">
        <Card
          style={{ width: 400, boxShadow: '0 8px 32px rgba(0,0,0,0.3)' }}
          styles={{ body: { padding: '32px 24px' } }}
        >
          <div style={{ textAlign: 'center', marginBottom: 32 }}>
            <h1 style={{ color: '#fff', fontSize: 24, marginBottom: 0 }}>AIOps 智能运维平台</h1>
          </div>
          <Form
            name="login"
            onFinish={onFinish}
            autoComplete="off"
            size="large"
            initialValues={{ username: 'admin' }}
          >
            <Form.Item
              name="username"
              rules={[{ required: true, message: '请输入用户名' }]}
            >
              <Input prefix={<UserOutlined />} placeholder="用户名" />
            </Form.Item>
            <Form.Item
              name="password"
              rules={[{ required: true, message: '请输入密码' }]}
            >
              <Input.Password prefix={<LockOutlined />} placeholder="密码" />
            </Form.Item>
            <Form.Item>
              <Button type="primary" htmlType="submit" block loading={loading}>
                登录
              </Button>
            </Form.Item>
          </Form>
        </Card>
      </div>
    </ConfigProvider>
  )
}
