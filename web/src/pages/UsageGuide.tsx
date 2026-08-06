import { Card, Typography, Table, Tag, Tabs } from 'antd'

const { Title, Paragraph, Text } = Typography

const envTableData = [
  { key: '1', source: 'Query 参数 ?foo=bar', env: 'QUERY_FOO=bar', remark: '所有 query 参数自动注入' },
  { key: '2', source: 'Header X-My-Token', env: 'HEADER_X_MY_TOKEN', remark: '需在"Header 作为环境变量"中声明' },
  { key: '3', source: '完整 Payload', env: 'PAYLOAD', remark: '"传递完整 Payload"选 env 或 both' },
]

const hmacHeaders = [
  { key: '1', header: 'X-Hub-Signature-256', format: 'sha256=<hex>', remark: 'GitHub 风格，优先匹配' },
  { key: '2', header: 'X-Signature', format: '<hex>', remark: '通用格式，可带 sha256= 前缀' },
  { key: '3', header: 'X-Gitlab-Token', format: '<token>', remark: '明文 token 直接比对' },
]

const guideContent = (
  <>
        <Typography>
          <Title level={3}>字段使用说明</Title>

          <Title level={4}>触发 Webhook</Title>
          <Paragraph>
            每个 Hook 有唯一 ID，触发地址：
          </Paragraph>
          <Paragraph code copyable>
            POST http://localhost:9000/hooks/&lt;hook-id&gt;
          </Paragraph>

          <Title level={4}>表单字段</Title>

          <Title level={5}>名称</Title>
          <Paragraph>仅用于界面展示，方便识别。</Paragraph>

          <Title level={5}>执行命令</Title>
          <Paragraph>
            收到 Webhook 后执行的 shell 命令。本地执行时必须是 <Text code>ALLOWED_COMMANDS</Text> 白名单前缀内的命令
            （默认 <Text code>/usr/bin/git,/usr/bin/curl,/bin/bash,/bin/sh</Text>）。
            例如 <Text code>/bin/bash /opt/scripts/deploy.sh</Text>。
          </Paragraph>

          <Title level={5}>执行位置</Title>
          <Paragraph>
            选「本地」则在 webhook 服务所在机器执行；选「SSH 主机」则在远端主机执行。
            位置属于 Hook 而非脚本，同一个脚本可以被不同 Hook 派到不同主机上跑。
            <Text strong> SSH 执行不受 ALLOWED_COMMANDS 白名单限制</Text>
            ——白名单描述的是本机可执行文件，对远端无意义。
          </Paragraph>

          <Title level={5}>工作目录</Title>
          <Paragraph>
            命令执行时的 cwd，留空则为服务进程当前目录（SSH 执行时为登录目录）。
            SSH 执行会先 <Text code>cd</Text> 到该目录，目录不存在则整次执行失败。
          </Paragraph>

          <Title level={5}>成功响应消息</Title>
          <Paragraph>命令执行成功后 HTTP 响应里的 message 字段内容。</Paragraph>

          <Title level={5}>HMAC 密钥 / 算法</Title>
          <Paragraph>
            密钥只存服务端，请求方用同一密钥对 <Text strong>原始请求体字节</Text> 计算
            <Text code> hex(HMAC(secret, body)) </Text>
            ，结果放签名 header 传入。留空密钥则不验签。
          </Paragraph>
          <Table
            size="small"
            pagination={false}
            columns={[
              { title: 'Header', dataIndex: 'header', render: (v: string) => <Text code>{v}</Text> },
              { title: '格式', dataIndex: 'format', render: (v: string) => <Text code>{v}</Text> },
              { title: '说明', dataIndex: 'remark' },
            ]}
            dataSource={hmacHeaders}
          />
          <Paragraph style={{ marginTop: 12 }}>
            示例（密钥 <Text code>mysecret</Text>，算法 sha256）：
          </Paragraph>
          <Paragraph>
            <pre style={{ background: '#f6f8fa', padding: 12, borderRadius: 6 }}>{`SECRET=mysecret
BODY='{"msg":"hello"}'
SIG=$(printf '%s' "$BODY" | openssl dgst -sha256 -hmac "$SECRET" | awk '{print $2}')

curl -X POST http://localhost:9000/hooks/<hook-id> \\
  -H "Content-Type: application/json" \\
  -H "X-Hub-Signature-256: sha256=$SIG" \\
  -d "$BODY"`}</pre>
          </Paragraph>
          <Paragraph type="warning">
            注意：签名对原始 body 计算，不要重新 JSON 序列化后再发（空格/换行差异会导致验签失败）。
          </Paragraph>

          <Title level={5}>固定 Token</Title>
          <Paragraph>
            简单明文令牌验证，适合不能算 HMAC 的调用方（如只能配固定 header 的 SaaS）。
            请求带 <Text code>X-Token</Text> header 或 <Text code>?token=</Text> query 参数，
            值与配置相等才执行。留空则不验证。可与 HMAC 同时启用（两者都通过才行）。
          </Paragraph>
          <Paragraph>
            <pre style={{ background: '#f6f8fa', padding: 12, borderRadius: 6 }}>{`curl -X POST http://localhost:9000/hooks/<hook-id> \\
  -H "X-Token: my-fixed-token" \\
  -d '{}'
# 或
curl -X POST "http://localhost:9000/hooks/<hook-id>?token=my-fixed-token" -d '{}'`}</pre>
          </Paragraph>

          <Title level={5}>Payload 字段作为参数</Title>
          <Paragraph>
            每行一个 JSON 字段名。从请求 body 中提取对应值，按声明顺序追加为命令行参数。
          </Paragraph>
          <Paragraph>
            例：body 为 <Text code>{'{"branch":"main","version":"1.2.0"}'}</Text>，
            字段填 <Text code>branch</Text> 和 <Text code>version</Text>，
            则命令执行为 <Text code>/bin/bash deploy.sh main 1.2.0</Text>。
          </Paragraph>

          <Title level={5}>Header 作为环境变量</Title>
          <Paragraph>
            每行一个请求 Header 名。值注入为环境变量，命名规则：
            <Text code>HEADER_</Text> + 全大写 + <Text code>-</Text> 转 <Text code>_</Text>。
          </Paragraph>
          <Table
            size="small"
            pagination={false}
            columns={[
              { title: '来源', dataIndex: 'source' },
              { title: '环境变量', dataIndex: 'env', render: (v: string) => <Text code>{v}</Text> },
              { title: '说明', dataIndex: 'remark' },
            ]}
            dataSource={envTableData}
          />

          <Title level={5}>传递完整 Payload</Title>
          <Paragraph>
            <Tag>不传递</Tag>body 不传给命令<br />
            <Tag>作为参数</Tag>原始 body 字符串作为最后一个命令行参数<br />
            <Tag>作为环境变量</Tag>原始 body 存入 <Text code>PAYLOAD</Text> 环境变量<br />
            <Tag>两者都传</Tag>同时做以上两种
          </Paragraph>

          <Title level={4}>脚本内使用示例</Title>
          <Paragraph>
            <pre style={{ background: '#f6f8fa', padding: 12, borderRadius: 6 }}>{`#!/bin/bash
# deploy.sh
BRANCH=$1              # pass_arguments 提取的第一个字段
echo "分支: $BRANCH"
echo "事件: $HEADER_X_GITHUB_EVENT"   # pass_headers 声明的 header
echo "来源IP参数: $QUERY_ENV"          # ?env=prod
echo "完整body: $PAYLOAD"              # pass_payload_to 选 env/both`}</pre>
          </Paragraph>

          <Title level={4}>执行日志</Title>
          <Paragraph>
            每次触发（含验签失败）都会记录到"执行日志"菜单，可查看命令 stdout/stderr 和耗时。
          </Paragraph>
        </Typography>
  </>
)

const apiContent = (
  <>
        <Typography>
          <Title level={3}>API 接口</Title>
          <Paragraph>
            面向外部系统（CI、监控等）。无需登录，凭 <Text code>X-API-Token</Text> 请求头访问，
            作用域仅限只读执行记录与日志。
          </Paragraph>

          <Title level={4}>获取 Token</Title>
          <Paragraph>
            在「设置」页生成。重新生成会使旧 token 立即失效，外部调用全部中断。
            通过请求头传递：<Text code>X-API-Token: &lt;token&gt;</Text>。
            未配置时外部访问一律 403。
          </Paragraph>

          <Title level={4}>查询执行记录</Title>
          <Paragraph code copyable>
            GET /api/external/executions
          </Paragraph>
          <Paragraph>
            支持 <Text code>limit</Text>、<Text code>offset</Text>、<Text code>hook_id</Text> 过滤。
            单条：<Text code>GET /api/external/executions/&lt;id&gt;</Text>。
          </Paragraph>

          <Title level={4}>轮询日志</Title>
          <Paragraph code copyable>
            GET /api/external/executions/&lt;id&gt;/logs?after_seq=0
          </Paragraph>
          <Paragraph>
            用响应里的 <Text code>next_seq</Text> 作为下一轮的 <Text code>after_seq</Text> 继续拉。
            响应字段：
          </Paragraph>
          <ul>
            <li><Text code>chunks</Text> — 增量日志块，每块带 <Text code>seq</Text> 与 <Text code>stream</Text>（stdout/stderr）</li>
            <li><Text code>next_seq</Text> — 下次轮询的起点</li>
            <li><Text code>oldest_seq</Text> — 当前仍在的最老序号。日志按尾部保留，旧块会滚删；若你的游标小于它，说明中间丢了一段</li>
            <li><Text code>has_more</Text> — 还有积压未拉完，立即续拉（单次响应封顶 500 块）</li>
            <li><Text code>status</Text> / <Text code>finished</Text> — 执行状态与是否结束</li>
          </ul>
          <Paragraph>
            轮询策略：执行中每 2 秒拉一次；<Text code>has_more</Text> 为真时不停顿续拉；
            <Text code>finished</Text> 为真且无积压后停止。
          </Paragraph>

          <Title level={4}>异步触发</Title>
          <Paragraph>
            Hook 勾选「异步执行」后，触发接口立即返回 <Text code>202</Text>，
            body 带 <Text code>execution_id</Text> 与 <Text code>status</Text>（<Text code>queued</Text>），
            不等脚本跑完。同步 Hook 仍返回 <Text code>200</Text> 并在 body 里带完整输出——两种模式从状态码即可区分。
          </Paragraph>
          <Paragraph>
            同一异步 Hook 上一次未结束时再次触发，返回 <Text code>409</Text>，
            body 带 <Text code>running_execution_id</Text> 指向正在跑的那次。并发槽与队列均满时返回 <Text code>429</Text>。
          </Paragraph>

          <Title level={4}>执行状态机</Title>
          <Paragraph>
            <Text code>queued</Text>（排队中）→ <Text code>running</Text>（运行中）→ 终态：
          </Paragraph>
          <ul>
            <li><Text code>success</Text> — 正常跑完</li>
            <li><Text code>failed</Text> — 脚本非零退出、超时或执行器错误</li>
            <li><Text code>canceled</Text> — 被手动中断</li>
            <li><Text code>interrupted</Text> — 服务重启，本服务不再跟踪它（远端进程可能仍在运行）</li>
          </ul>
          <Paragraph>
            <Text code>canceled</Text> 与 <Text code>interrupted</Text> 都只代表「本服务不再跟踪」，
            不保证远端进程已终止。中断接口（<Text code>POST /api/executions/&lt;id&gt;/cancel</Text>）
            仅登录会话可用，API token 调不了。
          </Paragraph>

          <Title level={4}>已知边界</Title>
          <ul>
            <li>已脱离 SSH 会话的远端进程（例如 Windows 上用 Start-Process 启动的看门狗）中断不到</li>
            <li>stdout/stderr 的交错顺序只保证「读到的顺序」，极近的两行可能因缓冲有轻微不确定性</li>
            <li>执行记录按 <Text code>RETENTION_DAYS</Text>（默认 30 天）保留，过期连同日志删除</li>
          </ul>
        </Typography>
  </>
)

export default function UsageGuide() {
  return (
    <div style={{ maxWidth: 900 }}>
      <Card>
        <Tabs
          defaultActiveKey="guide"
          items={[
            { key: 'guide', label: '字段说明', children: guideContent },
            { key: 'api', label: 'API 接口', children: apiContent },
          ]}
        />
      </Card>
    </div>
  )
}
