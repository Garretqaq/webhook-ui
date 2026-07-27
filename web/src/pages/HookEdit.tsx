import { useEffect, useState } from 'react'
import { Form, Input, Select, Button, Card, message, Space, Radio } from 'antd'
import { useNavigate, useParams } from 'react-router-dom'
import { hookApi, scriptApi } from '../api/client'
import type { Script } from '../api/client'

const { TextArea } = Input

export default function HookEdit() {
  const [form] = Form.useForm()
  const [loading, setLoading] = useState(false)
  const [isNew, setIsNew] = useState(true)
  const [scripts, setScripts] = useState<Script[]>([])
  const execMode = Form.useWatch('exec_mode', form)
  const navigate = useNavigate()
  const { id } = useParams()

  useEffect(() => {
    scriptApi.list().then(res => setScripts(res.data)).catch(() => {})
    if (id && id !== 'new') {
      setIsNew(false)
      loadHook(id)
    }
  }, [id])

  const loadHook = async (hookId: string) => {
    try {
      const res = await hookApi.get(hookId)
      form.setFieldsValue({
        ...res.data,
        exec_mode: res.data.script_id ? 'script' : 'command',
        pass_arguments: res.data.pass_arguments?.join('\n') || '',
        pass_headers: res.data.pass_headers?.join('\n') || '',
      })
    } catch (error) {
      message.error('加载失败')
    }
  }

  const onFinish = async (values: any) => {
    setLoading(true)
    try {
      const useScript = values.exec_mode === 'script'
      const data = {
        ...values,
        command: useScript ? '' : values.command,
        script_id: useScript ? values.script_id : '',
        pass_arguments: values.pass_arguments?.split('\n').filter((s: string) => s.trim()) || [],
        pass_headers: values.pass_headers?.split('\n').filter((s: string) => s.trim()) || [],
      }
      delete data.exec_mode

      if (isNew) {
        await hookApi.create(data)
        message.success('创建成功')
      } else {
        await hookApi.update(id!, data)
        message.success('更新成功')
      }
      navigate('/hooks')
    } catch (error: any) {
      message.error(error.response?.data?.error || '保存失败')
    } finally {
      setLoading(false)
    }
  }

  return (
    <Card title={isNew ? '新建 Hook' : '编辑 Hook'}>
      <Form
        form={form}
        layout="vertical"
        onFinish={onFinish}
        initialValues={{
          hmac_algorithm: 'sha256',
          pass_payload_to: '',
          exec_mode: 'command',
        }}
      >
        {!isNew && (
          <Form.Item name="id" label="Hook ID">
            <Input disabled />
          </Form.Item>
        )}

        <Form.Item
          name="name"
          label="名称"
          rules={[{ required: true, message: '请输入名称' }]}
        >
          <Input placeholder="例如: 部署生产环境" />
        </Form.Item>

        <Form.Item name="exec_mode" label="执行方式">
          <Radio.Group>
            <Radio.Button value="command">命令</Radio.Button>
            <Radio.Button value="script">脚本</Radio.Button>
          </Radio.Group>
        </Form.Item>

        {execMode === 'script' ? (
          <Form.Item
            name="script_id"
            label="选择脚本"
            rules={[{ required: true, message: '请选择脚本' }]}
            extra={scripts.length === 0 ? '暂无脚本，请先到「脚本管理」创建' : undefined}
          >
            <Select
              showSearch
              optionFilterProp="label"
              placeholder="选择要执行的脚本"
              options={scripts.map(s => ({ value: s.id, label: `${s.name} (${s.interpreter})` }))}
            />
          </Form.Item>
        ) : (
          <Form.Item
            name="command"
            label="执行命令"
            rules={[{ required: true, message: '请输入命令' }]}
            extra="例如: /opt/scripts/deploy.sh 或 /usr/bin/git pull"
          >
            <Input placeholder="/path/to/command" />
          </Form.Item>
        )}

        <Form.Item name="working_dir" label="工作目录">
          <Input placeholder="/path/to/workdir (可选)" />
        </Form.Item>

        <Form.Item name="response_message" label="成功响应消息">
          <Input placeholder="OK" />
        </Form.Item>

        <Form.Item
          name="hmac_secret"
          label="HMAC 密钥"
          extra="留空则不验证签名"
        >
          <Input.Password placeholder="签名验证密钥" />
        </Form.Item>

        <Form.Item name="hmac_algorithm" label="HMAC 算法">
          <Select>
            <Select.Option value="sha1">SHA1</Select.Option>
            <Select.Option value="sha256">SHA256</Select.Option>
            <Select.Option value="sha512">SHA512</Select.Option>
          </Select>
        </Form.Item>

        <Form.Item
          name="trigger_token"
          label="固定 Token"
          extra="留空则不验证。请求需带 X-Token header 或 ?token= 参数，值相等才执行（适合不能算 HMAC 的调用方）"
        >
          <Input.Password placeholder="固定访问令牌" />
        </Form.Item>

        <Form.Item
          name="pass_arguments"
          label="Payload 字段作为参数"
          extra="每行一个字段名，从 JSON payload 中提取作为命令参数"
        >
          <TextArea rows={3} placeholder="field1&#10;field2" />
        </Form.Item>

        <Form.Item
          name="pass_headers"
          label="Header 作为环境变量"
          extra="每行一个 Header 名，转换为 HEADER_* 环境变量"
        >
          <TextArea rows={3} placeholder="X-GitHub-Event&#10;X-Custom-Header" />
        </Form.Item>

        <Form.Item name="pass_payload_to" label="传递完整 Payload">
          <Select allowClear>
            <Select.Option value="">不传递</Select.Option>
            <Select.Option value="args">作为参数</Select.Option>
            <Select.Option value="env">作为环境变量 (PAYLOAD)</Select.Option>
            <Select.Option value="both">两者都传</Select.Option>
          </Select>
        </Form.Item>

        <Form.Item>
          <Space>
            <Button type="primary" htmlType="submit" loading={loading}>
              保存
            </Button>
            <Button onClick={() => navigate('/hooks')}>取消</Button>
          </Space>
        </Form.Item>
      </Form>
    </Card>
  )
}
