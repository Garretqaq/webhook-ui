import { useEffect, useState } from 'react'
import { Form, Input, Select, Button, Card, message, Space, Tag, Radio } from 'antd'
import { PlayCircleOutlined } from '@ant-design/icons'
import { useNavigate, useParams } from 'react-router-dom'
import { scriptApi, sshHostApi } from '../api/client'
import type { ScriptTestResult, SSHHost } from '../api/client'

const { TextArea } = Input

export default function ScriptEdit() {
  const [form] = Form.useForm()
  const [loading, setLoading] = useState(false)
  const [testing, setTesting] = useState(false)
  const [testResult, setTestResult] = useState<ScriptTestResult | null>(null)
  const [isNew, setIsNew] = useState(true)
  const [sshHosts, setSSHHosts] = useState<SSHHost[]>([])
  const execLocation = Form.useWatch('exec_location', form)
  const navigate = useNavigate()
  const { id } = useParams()

  useEffect(() => {
    sshHostApi.list().then(res => setSSHHosts(res.data)).catch(() => {})
    if (id && id !== 'new') {
      setIsNew(false)
      loadScript(id)
    }
  }, [id])

  const loadScript = async (scriptId: string) => {
    try {
      const res = await scriptApi.get(scriptId)
      form.setFieldsValue({
        ...res.data,
        exec_location: res.data.ssh_host_id ? 'ssh' : 'local',
      })
    } catch (error) {
      message.error('加载失败')
    }
  }

  const onFinish = async (values: any) => {
    setLoading(true)
    try {
      const data = {
        ...values,
        ssh_host_id: values.exec_location === 'ssh' ? values.ssh_host_id : '',
      }
      delete data.exec_location

      if (isNew) {
        await scriptApi.create(data)
        message.success('创建成功')
      } else {
        await scriptApi.update(id!, data)
        message.success('更新成功')
      }
      navigate('/scripts')
    } catch (error: any) {
      message.error(error.response?.data?.error || '保存失败')
    } finally {
      setLoading(false)
    }
  }

  const handleTest = async () => {
    const values = form.getFieldsValue()
    setTesting(true)
    setTestResult(null)
    try {
      const args = values.test_args?.split('\n').map((s: string) => s.trim()).filter(Boolean) || []
      const res = await scriptApi.test({
        interpreter: values.interpreter,
        content: values.content || '',
        args,
        ssh_host_id: values.exec_location === 'ssh' ? values.ssh_host_id : undefined,
      })
      setTestResult(res.data)
    } catch (error: any) {
      message.error(error.response?.data?.error || '试运行失败')
    } finally {
      setTesting(false)
    }
  }

  return (
    <Card title={isNew ? '新建脚本' : '编辑脚本'}>
      <Form
        form={form}
        layout="vertical"
        onFinish={onFinish}
        initialValues={{ interpreter: 'bash', exec_location: 'local' }}
      >
        <Form.Item
          name="name"
          label="名称"
          rules={[{ required: true, message: '请输入名称' }]}
        >
          <Input placeholder="例如: 部署生产环境" />
        </Form.Item>

        <Form.Item
          name="interpreter"
          label="解释器"
          rules={[{ required: true, message: '请选择解释器' }]}
          extra="本地执行时解释器必须在服务端的 ALLOWED_COMMANDS 白名单内"
        >
          <Select>
            <Select.Option value="bash">bash</Select.Option>
            <Select.Option value="sh">sh</Select.Option>
            <Select.Option value="python3">python3</Select.Option>
          </Select>
        </Form.Item>

        <Form.Item name="exec_location" label="执行位置">
          <Radio.Group>
            <Radio.Button value="local">本地</Radio.Button>
            <Radio.Button value="ssh">SSH 主机</Radio.Button>
          </Radio.Group>
        </Form.Item>

        {execLocation === 'ssh' && (
          <Form.Item
            name="ssh_host_id"
            label="SSH 主机"
            rules={[{ required: true, message: '请选择 SSH 主机' }]}
            extra={sshHosts.length === 0 ? '暂无主机，请先到「SSH 主机」创建' : '脚本内容通过 stdin 在远端执行，不写入远端磁盘'}
          >
            <Select
              showSearch
              optionFilterProp="label"
              placeholder="选择执行脚本的主机"
              options={sshHosts.map(h => ({ value: h.id, label: `${h.name} (${h.user}@${h.host}:${h.port})` }))}
            />
          </Form.Item>
        )}

        <Form.Item name="description" label="描述">
          <Input placeholder="脚本用途说明 (可选)" />
        </Form.Item>

        <Form.Item
          name="content"
          label="脚本内容"
          rules={[{ required: true, message: '请输入脚本内容' }]}
          extra="通过位置参数 ($1, $2...) 和环境变量 (QUERY_*, HEADER_*, PAYLOAD) 获取 webhook 传入的数据"
        >
          <TextArea
            rows={14}
            style={{ fontFamily: 'monospace' }}
            placeholder="#!/bin/bash&#10;echo &quot;branch: $1&quot;"
          />
        </Form.Item>

        <Form.Item
          name="test_args"
          label="试运行参数"
          extra="每行一个参数，依次作为 $1, $2... 传入"
        >
          <TextArea rows={3} placeholder="arg1&#10;arg2" />
        </Form.Item>

        <Form.Item>
          <Space>
            <Button type="primary" htmlType="submit" loading={loading}>
              保存
            </Button>
            <Button
              icon={<PlayCircleOutlined />}
              loading={testing}
              onClick={handleTest}
            >
              试运行
            </Button>
            <Button onClick={() => navigate('/scripts')}>取消</Button>
          </Space>
        </Form.Item>

        {testResult && (
          <Card
            size="small"
            title={
              testResult.success
                ? <Tag color="green">执行成功</Tag>
                : <Tag color="red">执行失败</Tag>
            }
          >
            {testResult.output && (
              <pre style={{ margin: 0, whiteSpace: 'pre-wrap' }}>{testResult.output}</pre>
            )}
            {testResult.error && (
              <pre style={{ margin: 0, whiteSpace: 'pre-wrap', color: '#cf1322' }}>{testResult.error}</pre>
            )}
            {!testResult.output && !testResult.error && <span>(无输出)</span>}
          </Card>
        )}
      </Form>
    </Card>
  )
}
