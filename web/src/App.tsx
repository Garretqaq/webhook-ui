import { useEffect, useState } from 'react'
import { BrowserRouter, Routes, Route, Navigate, useNavigate, useLocation } from 'react-router-dom'
import { Layout, Menu, Spin } from 'antd'
import {
  ApiOutlined,
  CodeOutlined,
  CloudServerOutlined,
  FileTextOutlined,
  LogoutOutlined,
  QuestionCircleOutlined,
} from '@ant-design/icons'
import { authApi } from './api/client'
import Login from './pages/Login'
import HookList from './pages/HookList'
import HookEdit from './pages/HookEdit'
import ScriptList from './pages/ScriptList'
import ScriptEdit from './pages/ScriptEdit'
import SSHHostList from './pages/SSHHostList'
import SSHHostEdit from './pages/SSHHostEdit'
import ExecutionLogs from './pages/ExecutionLogs'
import UsageGuide from './pages/UsageGuide'

const { Header, Content, Sider } = Layout

function AppLayout({ children, onLogout }: { children: React.ReactNode; onLogout: () => void }) {
  const navigate = useNavigate()
  const location = useLocation()

  const handleLogout = async () => {
    await authApi.logout()
    onLogout()
  }

  const menuItems = [
    {
      key: '/hooks',
      icon: <ApiOutlined />,
      label: 'Webhook 管理',
    },
    {
      key: '/scripts',
      icon: <CodeOutlined />,
      label: '脚本管理',
    },
    {
      key: '/ssh-hosts',
      icon: <CloudServerOutlined />,
      label: 'SSH 主机',
    },
    {
      key: '/executions',
      icon: <FileTextOutlined />,
      label: '执行日志',
    },
    {
      key: '/guide',
      icon: <QuestionCircleOutlined />,
      label: '使用说明',
    },
  ]

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Header style={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        background: '#001529'
      }}>
        <div style={{ color: 'white', fontSize: 18, fontWeight: 'bold' }}>
          Webhook UI
        </div>
        <LogoutOutlined
          style={{ color: 'white', fontSize: 16, cursor: 'pointer' }}
          onClick={handleLogout}
        />
      </Header>
      <Layout>
        <Sider width={200} theme="light">
          <Menu
            mode="inline"
            selectedKeys={[location.pathname]}
            items={menuItems}
            onClick={({ key }) => navigate(key)}
            style={{ height: '100%', borderRight: 0 }}
          />
        </Sider>
        <Content style={{ padding: 24, background: '#f0f2f5' }}>
          {children}
        </Content>
      </Layout>
    </Layout>
  )
}

function App() {
  const [authenticated, setAuthenticated] = useState<boolean | null>(null)

  useEffect(() => {
    checkAuth()
  }, [])

  const checkAuth = async () => {
    try {
      const res = await authApi.check()
      setAuthenticated(res.data.authenticated)
    } catch {
      setAuthenticated(false)
    }
  }

  const handleLogin = () => setAuthenticated(true)
  const handleLogout = () => setAuthenticated(false)

  if (authenticated === null) {
    return (
      <div style={{
        display: 'flex',
        justifyContent: 'center',
        alignItems: 'center',
        height: '100vh'
      }}>
        <Spin size="large" />
      </div>
    )
  }

  return (
    <BrowserRouter>
      <Routes>
        <Route
          path="/login"
          element={authenticated ? <Navigate to="/hooks" /> : <Login onLogin={handleLogin} />}
        />
        <Route
          path="/hooks"
          element={
            authenticated ? (
              <AppLayout onLogout={handleLogout}>
                <HookList />
              </AppLayout>
            ) : (
              <Navigate to="/login" />
            )
          }
        />
        <Route
          path="/hooks/:id"
          element={
            authenticated ? (
              <AppLayout onLogout={handleLogout}>
                <HookEdit />
              </AppLayout>
            ) : (
              <Navigate to="/login" />
            )
          }
        />
        <Route
          path="/scripts"
          element={
            authenticated ? (
              <AppLayout onLogout={handleLogout}>
                <ScriptList />
              </AppLayout>
            ) : (
              <Navigate to="/login" />
            )
          }
        />
        <Route
          path="/scripts/:id"
          element={
            authenticated ? (
              <AppLayout onLogout={handleLogout}>
                <ScriptEdit />
              </AppLayout>
            ) : (
              <Navigate to="/login" />
            )
          }
        />
        <Route
          path="/ssh-hosts"
          element={
            authenticated ? (
              <AppLayout onLogout={handleLogout}>
                <SSHHostList />
              </AppLayout>
            ) : (
              <Navigate to="/login" />
            )
          }
        />
        <Route
          path="/ssh-hosts/:id"
          element={
            authenticated ? (
              <AppLayout onLogout={handleLogout}>
                <SSHHostEdit />
              </AppLayout>
            ) : (
              <Navigate to="/login" />
            )
          }
        />
        <Route
          path="/executions"
          element={
            authenticated ? (
              <AppLayout onLogout={handleLogout}>
                <ExecutionLogs />
              </AppLayout>
            ) : (
              <Navigate to="/login" />
            )
          }
        />
        <Route
          path="/guide"
          element={
            authenticated ? (
              <AppLayout onLogout={handleLogout}>
                <UsageGuide />
              </AppLayout>
            ) : (
              <Navigate to="/login" />
            )
          }
        />
        <Route path="/" element={<Navigate to="/hooks" />} />
      </Routes>
    </BrowserRouter>
  )
}

export default App
