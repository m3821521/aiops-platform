import { Layout, ConfigProvider, theme as antdTheme } from 'antd'
import zhCN from 'antd/locale/zh_CN'
import { Outlet } from 'react-router-dom'
import Sidebar from '@/components/Sidebar'
import AppHeader from '@/components/Header'
import { useAppStore } from '@/stores/app'

const { Content } = Layout

export default function MainLayout() {
  const themeMode = useAppStore((s) => s.theme)

  return (
    <ConfigProvider
      locale={zhCN}
      theme={{
        algorithm: themeMode === 'dark' ? antdTheme.darkAlgorithm : antdTheme.defaultAlgorithm,
        token: {
          colorPrimary: '#1677ff',
          borderRadius: 6,
        },
      }}
    >
      <Layout style={{ minHeight: '100vh' }}>
        <Sidebar />
        <Layout>
          <AppHeader />
          <Content
            style={{
              margin: 16,
              padding: 16,
              minHeight: 280,
              overflow: 'auto',
            }}
          >
            <Outlet />
          </Content>
        </Layout>
      </Layout>
    </ConfigProvider>
  )
}
