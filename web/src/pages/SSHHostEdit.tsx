import { useEffect, useState } from 'react'
import { Form, Input, InputNumber, Select, Button, Card, message, Space, Alert, Popconfirm } from 'antd'
import { ApiOutlined } from '@ant-design/icons'
import { useNavigate, useParams } from 'react-router-dom'
import { sshHostApi } from '../api/client'

const { TextArea } = Input

export default function SSHHostEdit() {
  const [form] = Form.useForm()
  const [loading, setLoading] = useState(false)
  const [testing, setTesting] = useState(false)
  const [isNew, setIsNew] = useState(true)
  const [hostKey, setHostKey] = useState('')
  const authType = Form.useWatch('auth_type', form)
  const navigate = useNavigate()
  const { id } = useParams()

  useEffect(() => {
    if (id && id !== 'new') {
      setIsNew(false)
      loadHost(id)
    }
  }, [id])

  const loadHost = async (hostId: string) => {
    try {
      const res = await sshHostApi.get(hostId)
      form.setFieldsValue(res.data)
      setHostKey(res.data.host_key || '')
    } catch (error) {
      message.error('加载失败')
    }
  }

  const onFinish = async (values: any) => {
    setLoading(true)
    try {
      const data = { ...values, host_key: hostKey }
      if (isNew) {
        await sshHostApi.create(data)
        message.success('创建成功')
      } else {
        await sshHostApi.update(id!, data)
        message.success('更新成功')
      }
      navigate('/ssh-hosts')
    } catch (error: any) {
      message.error(error.response?.data?.error || '保存失败')
    } finally {
      setLoading(false)
    }
  }

  const handleTest = async () => {
    const values = form.getFieldsValue()
    setTesting(true)
    try {
      const res = await sshHostApi.test({ ...values, id: isNew ? undefined : id, host_key: hostKey })
      if (res.data.success) {
        if (res.data.learned_host_key) {
          setHostKey(res.data.learned_host_key)
          message.success('连接成功，已记录服务器 Host Key')
        } else {
          message.success('连接成功')
        }
      } else {
        message.error(`连接失败: ${res.data.error}`)
      }
    } catch (error: any) {
      message.error(error.response?.data?.error || '测试失败')
    } finally {
      setTesting(false)
    }
  }

  return (
    <Card title={isNew ? '新建 SSH 主机' : '编辑 SSH 主机'}>
      <Form
        form={form}
        layout="vertical"
        onFinish={onFinish}
        initialValues={{ port: 22, auth_type: 'key' }}
      >
        <Form.Item
          name="name"
          label="名称"
          rules={[{ required: true, message: '请输入名称' }]}
        >
          <Input placeholder="例如: 生产服务器" />
        </Form.Item>

        <Space size="large" style={{ display: 'flex' }}>
          <Form.Item
            name="host"
            label="主机地址"
            rules={[{ required: true, message: '请输入主机地址' }]}
            style={{ width: 320 }}
          >
            <Input placeholder="192.168.1.10 或 example.com" />
          </Form.Item>

          <Form.Item
            name="port"
            label="端口"
            rules={[{ required: true, message: '请输入端口' }]}
          >
            <InputNumber min={1} max={65535} />
          </Form.Item>

          <Form.Item
            name="user"
            label="用户名"
            rules={[{ required: true, message: '请输入用户名' }]}
          >
            <Input placeholder="root" />
          </Form.Item>
        </Space>

        <Form.Item
          name="auth_type"
          label="认证方式"
          rules={[{ required: true }]}
        >
          <Select style={{ width: 200 }}>
            <Select.Option value="key">私钥</Select.Option>
            <Select.Option value="password">密码</Select.Option>
          </Select>
        </Form.Item>

        {authType === 'password' ? (
          <Form.Item
            name="credential"
            label="密码"
            rules={[{ required: true, message: '请输入密码' }]}
          >
            <Input.Password placeholder="SSH 登录密码" />
          </Form.Item>
        ) : (
          <Form.Item
            name="credential"
            label="私钥"
            rules={[{ required: true, message: '请粘贴私钥' }]}
            extra="粘贴 PEM 格式私钥全文（含 BEGIN/END 行），存储在服务端本地数据库中"
          >
            <TextArea
              rows={6}
              style={{ fontFamily: 'monospace' }}
              placeholder="-----BEGIN OPENSSH PRIVATE KEY-----"
            />
          </Form.Item>
        )}

        <Form.Item label="服务器 Host Key">
          {hostKey ? (
            <Space direction="vertical" style={{ width: '100%' }}>
              <code style={{ wordBreak: 'break-all' }}>{hostKey}</code>
              <Popconfirm
                title="清除后下次连接将重新记录，确定?"
                onConfirm={() => setHostKey('')}
              >
                <Button size="small" danger>清除</Button>
              </Popconfirm>
            </Space>
          ) : (
            <Alert
              type="info"
              showIcon
              message="未固定。首次连接时将自动记录服务器公钥（TOFU）；公网环境建议用 ssh-keyscan 获取后粘贴到下方"
            />
          )}
          <TextArea
            rows={2}
            style={{ fontFamily: 'monospace', marginTop: 8 }}
            placeholder="手动预填: ssh-ed25519 AAAA... (可用 ssh-keyscan 主机 获取)"
            value={hostKey}
            onChange={(e) => setHostKey(e.target.value.trim())}
          />
        </Form.Item>

        <Form.Item>
          <Space>
            <Button type="primary" htmlType="submit" loading={loading}>
              保存
            </Button>
            <Button
              icon={<ApiOutlined />}
              loading={testing}
              onClick={handleTest}
            >
              测试连接
            </Button>
            <Button onClick={() => navigate('/ssh-hosts')}>取消</Button>
          </Space>
        </Form.Item>
      </Form>
    </Card>
  )
}
