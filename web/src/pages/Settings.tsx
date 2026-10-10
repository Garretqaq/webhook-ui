import { useEffect, useState } from 'react'
import { Card, Button, Typography, message, Space, Alert, Popconfirm, Input, InputNumber, Switch } from 'antd'
import { ReloadOutlined, CopyOutlined } from '@ant-design/icons'
import { settingsApi } from '../api/client'

const { Paragraph, Text } = Typography

export default function Settings() {
  const [token, setToken] = useState('')
  const [configured, setConfigured] = useState(false)
  const [loading, setLoading] = useState(false)

  const [retentionEnabled, setRetentionEnabled] = useState(false)
  const [retentionDays, setRetentionDays] = useState<number | null>(30)
  const [savingRetention, setSavingRetention] = useState(false)

  const load = async () => {
    try {
      const { data } = await settingsApi.getAPIToken()
      setToken(data.token)
      setConfigured(data.configured)
    } catch {
      message.error('加载失败')
    }
  }

  const loadRetention = async () => {
    try {
      const { data } = await settingsApi.getRetention()
      setRetentionEnabled(data.enabled)
      setRetentionDays(data.days)
    } catch {
      message.error('加载自动清理设置失败')
    }
  }

  useEffect(() => {
    load()
    loadRetention()
  }, [])

  const saveRetention = async () => {
    if (!retentionDays || retentionDays < 1 || retentionDays > 3650) {
      message.error('保留天数需在 1 ~ 3650 之间')
      return
    }
    setSavingRetention(true)
    try {
      const { data } = await settingsApi.updateRetention({
        enabled: retentionEnabled,
        days: retentionDays,
      })
      setRetentionEnabled(data.enabled)
      setRetentionDays(data.days)
      message.success('已保存')
    } catch {
      message.error('保存失败')
    } finally {
      setSavingRetention(false)
    }
  }

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


  const copy = async () => {
    try {
      await navigator.clipboard.writeText(token)
      message.success('已复制')
    } catch {
      message.error('复制失败')
    }
  }

  return (
    <Space direction="vertical" size="large" style={{ width: '100%' }}>
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

    <Card title="执行日志自动清理">
      <Paragraph type="secondary">
        开启后每天自动清理超过 {retentionDays ?? '-'} 天的已结束执行记录及日志，运行中的记录不受影响。
        保存后立即生效并执行一次清理。
      </Paragraph>
      <Space size="large" wrap>
        <Space>
          <Text>自动清理</Text>
          <Switch checked={retentionEnabled} onChange={setRetentionEnabled} />
        </Space>
        <Space>
          <Text>保留天数</Text>
          <InputNumber
            min={1}
            max={3650}
            precision={0}
            value={retentionDays}
            onChange={(v) => setRetentionDays(v)}
            disabled={!retentionEnabled}
            addonAfter="天"
          />
        </Space>
        <Button type="primary" loading={savingRetention} onClick={saveRetention}>
          保存
        </Button>
      </Space>
    </Card>
    </Space>
  )
}
