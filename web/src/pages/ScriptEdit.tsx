import { useEffect, useState } from 'react'
import { Form, Input, Select, Button, Card, message, Space, Tag, Radio, Alert } from 'antd'
import { PlayCircleOutlined } from '@ant-design/icons'
import { useNavigate, useParams } from 'react-router-dom'
import { scriptApi, scriptTestRunApi, sshHostApi } from '../api/client'
import type { SSHHost } from '../api/client'
import LogStreamView from '../components/LogStreamView'

const { TextArea } = Input

const testRunStatus: Record<string, { color: string; label: string }> = {
  running: { color: 'blue', label: '运行中' },
  success: { color: 'green', label: '执行成功' },
  failed: { color: 'red', label: '执行失败' },
  timeout: { color: 'orange', label: '执行超时' },
  canceled: { color: 'default', label: '已中断' },
}

export default function ScriptEdit() {
  const [form] = Form.useForm()
  const [loading, setLoading] = useState(false)
  const [testing, setTesting] = useState(false)
  const [runId, setRunId] = useState<string | null>(null)
  const [status, setStatus] = useState('running')
  const [isNew, setIsNew] = useState(true)
  const [sshHosts, setSSHHosts] = useState<SSHHost[]>([])
  const testLocation = Form.useWatch('test_exec_location', form)
  const interpreter = Form.useWatch('interpreter', form)
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
      form.setFieldsValue(res.data)
    } catch (error) {
      message.error('加载失败')
    }
  }

  const onFinish = async (values: any) => {
    setLoading(true)
    try {
      const data = {
        name: values.name,
        interpreter: values.interpreter,
        description: values.description,
        content: values.content,
      }

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
    setRunId(null)
    try {
      const args = values.test_args?.split('\n').map((s: string) => s.trim()).filter(Boolean) || []
      const res = await scriptTestRunApi.start({
        interpreter: values.interpreter,
        content: values.content || '',
        args,
        ssh_host_id: values.test_exec_location === 'ssh' ? values.test_ssh_host_id : undefined,
      })
      setStatus(res.data.status)
      setRunId(res.data.run_id)
    } catch (error: any) {
      message.error(error.response?.data?.error || '试运行失败')
      setTesting(false)
    }
  }

  return (
    <Card title={isNew ? '新建脚本' : '编辑脚本'}>
      <Form
        form={form}
        layout="vertical"
        onFinish={onFinish}
        initialValues={{ interpreter: 'bash', test_exec_location: 'local' }}
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
          extra="本地执行时解释器必须在服务端的 ALLOWED_COMMANDS 白名单内。powershell 只能配合目标系统为 Windows 的 SSH 主机使用，其余解释器只能用于 Linux"
        >
          <Select>
            <Select.Option value="bash">bash</Select.Option>
            <Select.Option value="sh">sh</Select.Option>
            <Select.Option value="python3">python3</Select.Option>
            <Select.Option value="powershell">powershell (Windows)</Select.Option>
          </Select>
        </Form.Item>

        {interpreter === 'powershell' && (
          <Alert
            type="warning"
            showIcon
            style={{ marginBottom: 24 }}
            message="调用外部程序必须走管道"
            description={
              <>
                <div>
                  脚本通过 stdin 交给远端 <code>powershell -Command -</code> 执行。裸调 <code>&amp; npm run start</code> 会被子进程抢走 stdin，
                  输出看不见，后续语句也不会执行。固定写法：
                </div>
                <pre style={{ fontFamily: 'monospace', margin: '8px 0 0' }}>
{`& <命令> <参数> 2>&1 | Out-Host
$code = $LASTEXITCODE
if ($code -ne 0) { exit $code }`}
                </pre>
                <div style={{ marginTop: 8 }}>
                  工作目录填在 Hook 上，不要自己写 <code>Set-Location</code>；不要设 <code>$ErrorActionPreference = 'Stop'</code>
                  （<code>2&gt;&amp;1</code> 会把 stderr 变成 ErrorRecord 炸掉执行）；不会自己退出的进程要在 Hook 上勾选「异步执行」并调大超时。
                </div>
              </>
            }
          />
        )}

        <Form.Item name="description" label="描述">
          <Input placeholder="脚本用途说明 (可选)" />
        </Form.Item>

        <Form.Item
          name="content"
          label="脚本内容"
          rules={[{ required: true, message: '请输入脚本内容' }]}
          extra="通过位置参数和环境变量 (QUERY_*, HEADER_*, PAYLOAD) 获取 webhook 传入的数据。bash/sh 用 $1 $2，python3 用 sys.argv，powershell 用 $args[0] $args[1]（stdin 执行模式不支持 param() 块）"
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

        <Form.Item
          name="test_exec_location"
          label="试运行位置"
          extra="仅用于本次试运行，不会保存到脚本。正式执行位置在 Hook 中设置"
        >
          <Radio.Group>
            <Radio.Button value="local">本地</Radio.Button>
            <Radio.Button value="ssh">SSH 主机</Radio.Button>
          </Radio.Group>
        </Form.Item>

        {testLocation === 'ssh' && (
          <Form.Item
            name="test_ssh_host_id"
            label="试运行主机"
            rules={[{ required: true, message: '请选择 SSH 主机' }]}
            extra={sshHosts.length === 0 ? '暂无主机，请先到「SSH 主机」创建' : '脚本内容通过 stdin 在远端执行，不写入远端磁盘'}
          >
            <Select
              showSearch
              optionFilterProp="label"
              placeholder="选择试运行的主机"
              options={sshHosts.map(h => ({ value: h.id, label: `${h.name} (${h.user}@${h.host}:${h.port})` }))}
            />
          </Form.Item>
        )}

        <Form.Item>
          <Space>
            <Button type="primary" htmlType="submit" loading={loading}>
              保存
            </Button>
            <Button
              icon={<PlayCircleOutlined />}
              loading={testing}
              disabled={testing}
              onClick={handleTest}
            >
              试运行
            </Button>
            <Button onClick={() => navigate('/scripts')}>取消</Button>
          </Space>
        </Form.Item>

        {runId && (
          <Card
            size="small"
            title={
              <Tag color={testRunStatus[status]?.color || 'default'}>
                {testRunStatus[status]?.label || status}
              </Tag>
            }
          >
            <LogStreamView
              sourceKey={runId}
              initiallyFinished={false}
              fetchPage={async afterSeq => (await scriptTestRunApi.logs(runId, afterSeq)).data}
              onStatus={(runStatus, finished) => {
                setStatus(runStatus)
                if (finished) setTesting(false)
              }}
              renderEmpty={() => '(无输出)'}
            />
          </Card>
        )}
      </Form>
    </Card>
  )
}
