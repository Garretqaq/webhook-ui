import { useState } from 'react'
import { Form, Input, Button, Card, message } from 'antd'
import { authApi } from '../api/client'

export default function Login({ onLogin }: { onLogin: () => void }) {
  const [loading, setLoading] = useState(false)

  const onFinish = async (values: { password: string }) => {
    setLoading(true)
    try {
      await authApi.login(values.password)
      message.success('登录成功')
      onLogin()
    } catch (error: any) {
      message.error(error.response?.data?.error || '登录失败')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div style={{
      display: 'flex',
      justifyContent: 'center',
      alignItems: 'center',
      minHeight: '100vh',
      background: '#f0f2f5'
    }}>
      <Card title="Webhook UI 登录" style={{ width: 400 }}>
        <Form onFinish={onFinish} layout="vertical">
          <Form.Item
            name="password"
            label="管理员密码"
            rules={[{ required: true, message: '请输入密码' }]}
          >
            <Input.Password placeholder="请输入管理员密码" />
          </Form.Item>
          <Form.Item>
            <Button type="primary" htmlType="submit" loading={loading} block>
              登录
            </Button>
          </Form.Item>
        </Form>
      </Card>
    </div>
  )
}
