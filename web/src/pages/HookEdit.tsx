import { useEffect, useState } from 'react'
import { Form, Input, InputNumber, Select, Button, Card, message, Space, Radio, Switch } from 'antd'
import { useNavigate, useParams } from 'react-router-dom'
import { hookApi, scriptApi, sshHostApi } from '../api/client'
import type { Script, SSHHost } from '../api/client'

const { TextArea } = Input

export default function HookEdit() {
  const [form] = Form.useForm()
  const [loading, setLoading] = useState(false)
  const [isNew, setIsNew] = useState(true)
  const [scripts, setScripts] = useState<Script[]>([])
  const [sshHosts, setSSHHosts] = useState<SSHHost[]>([])
  const execMode = Form.useWatch('exec_mode', form)
  const execLocation = Form.useWatch('exec_location', form)
  const authMode = Form.useWatch('auth_mode', form)
  const navigate = useNavigate()
  const { id } = useParams()

  useEffect(() => {
    scriptApi.list().then(res => setScripts(res.data)).catch(() => {})
    sshHostApi.list().then(res => setSSHHosts(res.data)).catch(() => {})
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
        exec_location: res.data.ssh_host_id ? 'ssh' : 'local',
        auth_mode: res.data.hmac_secret ? 'hmac' : res.data.trigger_token ? 'token' : 'none',
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
      // Only the chosen auth method's credential is sent, so the backend's
      // mutual-exclusion rule can never trip on a form submission.
      const data = {
        ...values,
        command: useScript ? '' : values.command,
        script_id: useScript ? values.script_id : '',
        ssh_host_id: values.exec_location === 'ssh' ? values.ssh_host_id : '',
        hmac_secret: values.auth_mode === 'hmac' ? values.hmac_secret : '',
        trigger_token: values.auth_mode === 'token' ? values.trigger_token : '',
        pass_arguments: values.pass_arguments?.split('\n').filter((s: string) => s.trim()) || [],
        pass_headers: values.pass_headers?.split('\n').filter((s: string) => s.trim()) || [],
      }
      delete data.exec_mode
      delete data.exec_location
      delete data.auth_mode

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
          exec_location: 'local',
          auth_mode: 'none',
          async: false,
          timeout_seconds: 300,
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
            extra={
              execLocation === 'ssh'
                ? '在远端主机上执行，不受服务端 ALLOWED_COMMANDS 白名单限制'
                : '例如: /opt/scripts/deploy.sh 或 /usr/bin/git pull，必须在服务端 ALLOWED_COMMANDS 白名单内'
            }
          >
            <Input placeholder="/path/to/command" />
          </Form.Item>
        )}

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
            extra={
              sshHosts.length === 0
                ? '暂无主机，请先到「SSH 主机」创建'
                : execMode === 'script'
                  ? '脚本内容通过 stdin 在远端执行，不写入远端磁盘'
                  : '命令在远端主机上执行'
            }
          >
            <Select
              showSearch
              optionFilterProp="label"
              placeholder="选择执行的主机"
              options={sshHosts.map(h => ({ value: h.id, label: `${h.name} (${h.user}@${h.host}:${h.port})` }))}
            />
          </Form.Item>
        )}

        <Form.Item
          name="working_dir"
          label="工作目录"
          extra="留空则使用默认目录。SSH 执行时会先 cd 到该目录，目录不存在则执行失败"
        >
          <Input placeholder="/path/to/workdir (可选)" />
        </Form.Item>

        <Form.Item
          name="async"
          label="异步执行"
          valuePropName="checked"
          extra="开启后触发立即返回 202 和 execution_id，不等脚本跑完；日志在执行日志页实时查看。同一 Hook 上一次未结束时再次触发会返回 409"
        >
          <Switch />
        </Form.Item>

        <Form.Item
          noStyle
          shouldUpdate={(prev, cur) => prev.async !== cur.async}
        >
          {({ getFieldValue }) => (
            <Form.Item
              name="timeout_seconds"
              label="超时（秒）"
              rules={[
                { required: true, message: '请输入超时秒数' },
                {
                  validator: (_, value) =>
                    getFieldValue('async') || value > 0
                      ? Promise.resolve()
                      : Promise.reject(new Error('同步 Hook 会一直占住 HTTP 连接，不能设为不限时')),
                },
              ]}
              extra={
                getFieldValue('async')
                  ? '0 表示不限时。长任务填实际需要的秒数，例如 2 小时填 7200'
                  : '同步 Hook 必须有上限，否则请求会被一直挂住'
              }
            >
              <InputNumber min={0} style={{ width: 200 }} />
            </Form.Item>
          )}
        </Form.Item>

        <Form.Item name="response_message" label="成功响应消息">
          {/* Chrome autofills a saved username into any plain text input that
              precedes a password field. The bogus autocomplete value is what
              actually stops it — "off" is ignored. */}
          <Input placeholder="OK" autoComplete="nope" />
        </Form.Item>

        <Form.Item name="auth_mode" label="鉴权方式" extra="与固定 Token 二选一；都不选则任何人都可触发">
          <Radio.Group
            onChange={e => {
              const mode = e.target.value
              // Switching methods drops the other credential so the backend
              // never sees both at once.
              if (mode !== 'hmac') form.setFieldValue('hmac_secret', '')
              if (mode !== 'token') form.setFieldValue('trigger_token', '')
            }}
          >
            <Radio.Button value="none">无鉴权</Radio.Button>
            <Radio.Button value="hmac">HMAC 签名</Radio.Button>
            <Radio.Button value="token">固定 Token</Radio.Button>
          </Radio.Group>
        </Form.Item>

        {authMode === 'hmac' && (
          <>
            <Form.Item
              name="hmac_secret"
              label="HMAC 密钥"
              extra="留空则不验证签名"
            >
              <Input.Password placeholder="签名验证密钥" autoComplete="new-password" />
            </Form.Item>

            <Form.Item name="hmac_algorithm" label="HMAC 算法">
              <Select>
                <Select.Option value="sha1">SHA1</Select.Option>
                <Select.Option value="sha256">SHA256</Select.Option>
                <Select.Option value="sha512">SHA512</Select.Option>
              </Select>
            </Form.Item>
          </>
        )}

        {authMode === 'token' && (
          <Form.Item
            name="trigger_token"
            label="固定 Token"
            extra="请求需带 X-Token / X-Gitlab-Token header 或 ?token= 参数，值相等才执行（适合不能算 HMAC 的调用方，如 GitLab Webhook）"
          >
            <Input.Password placeholder="固定访问令牌" autoComplete="new-password" />
          </Form.Item>
        )}

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
