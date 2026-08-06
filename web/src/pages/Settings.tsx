import { useEffect, useState } from 'react'
import { Card, Button, Typography, message, Space, Alert, Popconfirm, Input } from 'antd'
import { ReloadOutlined, CopyOutlined, StopOutlined } from '@ant-design/icons'
import { settingsApi } from '../api/client'

const { Paragraph, Text } = Typography

export default function Settings() {
  const [token, setToken] = useState('')
  const [configured, setConfigured] = useState(false)
  const [loading, setLoading] = useState(false)

  const load = async () => {
    try {
      const { data } = await settingsApi.getAPIToken()
      setToken(data.token)
      setConfigured(data.configured)
    } catch {
      message.error('加载失败')
    }
  }

  useEffect(() => {
    load()
  }, [])

  const regenerate = async () => {
    setLoading(true)
    try {
      const { data } = await settingsApi.regenerateAPIToken()
      setToken(data.token)
      setConfigured(true)
      message.success('已生成新的 API Token')
    } catch {
      message.error('生成失败')
    } finally {
      setLoading(false)
    }
  }

  const disable = async () => {
    setLoading(true)
    try {
      await settingsApi.disableAPIToken()
      setToken('')
      setConfigured(false)
      message.success('已停用外部访问')
    } catch {
      message.error('操作失败')
    } finally {
      setLoading(false)
    }
  }

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(token)
      message.success('已复制')
    } catch {
      message.error('复制失败')
    }
  }

  return (
    <Card title="API Token">
      <Alert
        type="warning"
        showIcon
        style={{ marginBottom: 16 }}
        message="作用域仅限只读执行记录与日志"
        description={
          <>
            持此 token 的外部系统可访问 <Text code>GET /api/external/executions</Text> 及其子路径，
            无法读取 hooks、scripts、ssh-hosts，也无法中断或修改任何内容。
            通过请求头 <Text code>X-API-Token</Text> 传递。重新生成会使旧 token 立即失效。
          </>
        }
      />

      {configured ? (
        <>
          <Paragraph>
            <Text strong>当前 Token：</Text>
          </Paragraph>
          <Space.Compact style={{ width: 480 }}>
            <Input value={token} readOnly />
            <Button icon={<CopyOutlined />} onClick={copy} />
          </Space.Compact>
          <Paragraph style={{ marginTop: 16 }}>
            <Space>
              <Popconfirm
                title="重新生成会使旧 token 立即失效，外部调用全部中断。确定?"
                onConfirm={regenerate}
              >
                <Button icon={<ReloadOutlined />} loading={loading}>
                  重新生成
                </Button>
              </Popconfirm>
              <Popconfirm
                title="停用后外部调用全部失败，直到重新生成。确定?"
                onConfirm={disable}
              >
                <Button danger icon={<StopOutlined />} loading={loading}>
                  停用
                </Button>
              </Popconfirm>
            </Space>
          </Paragraph>
        </>
      ) : (
        <Paragraph>
          尚未配置。生成一个 token 后，外部系统即可只读拉取执行记录与日志。
          <div style={{ marginTop: 12 }}>
            <Button type="primary" icon={<ReloadOutlined />} loading={loading} onClick={regenerate}>
              生成 Token
            </Button>
          </div>
        </Paragraph>
      )}
    </Card>
  )
}
