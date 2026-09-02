import { Card, Typography, Table, Tag, Tabs } from 'antd'

const { Title, Paragraph, Text } = Typography

const envTableData = [
  { key: '1', source: 'Query 参数 ?foo=bar', env: 'QUERY_FOO=bar', remark: '所有 query 参数自动注入' },
  { key: '2', source: 'Header X-My-Token', env: 'HEADER_X_MY_TOKEN', remark: '需在"Header 作为环境变量"中声明' },
  { key: '3', source: '完整 Payload', env: 'PAYLOAD', remark: '"传递完整 Payload"选 env 或 both' },
]

const hmacHeaders = [
  { key: '1', header: 'X-Hub-Signature-256', format: 'sha256=<hex>', remark: 'GitHub 风格，优先匹配' },
  { key: '2', header: 'X-Gitlab-Token', format: '<hex>', remark: 'GitLab 风格；启用 HMAC 时按签名比对，启用固定 Token 时按明文比对' },
  { key: '3', header: 'X-Signature', format: '<hex>', remark: '通用格式，可带 sha256= 前缀' },
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
            与固定 Token 二选一，同时配置会被拒绝。
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
            简单明文令牌验证，适合不能算 HMAC 的调用方（如只能配固定 header 的 SaaS、GitLab Webhook）。
            请求带 <Text code>X-Token</Text> header、<Text code>?token=</Text> query 参数
            或 <Text code>X-Gitlab-Token</Text> header（GitLab 原生格式），
            值与配置相等才执行。留空则不验证。与 HMAC 二选一，同时配置会被拒绝。
          </Paragraph>
          <Paragraph>
            <pre style={{ background: '#f6f8fa', padding: 12, borderRadius: 6 }}>{`curl -X POST http://localhost:9000/hooks/<hook-id> \\
  -H "X-Token: my-fixed-token" \\
  -d '{}'
# 或
curl -X POST "http://localhost:9000/hooks/<hook-id>?token=my-fixed-token" -d '{}'
# GitLab Webhook 直接在 GitLab 侧填 Secret Token 即可，它会以 X-Gitlab-Token 头发送`}</pre>
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

          <Title level={4}>脚本试运行</Title>
          <Paragraph>
            脚本编辑页的「试运行」用来验证正在编辑的脚本，
            <Text strong>它不是一次正式执行</Text>，和 Hook 触发有几处刻意的区别：
          </Paragraph>
          <ul>
            <li>不产生执行记录，不会出现在「执行日志」里；日志只存在服务内存中，<Text strong>服务重启即丢</Text></li>
            <li>日志按尾部保留（上限同 <Text code>LOG_TAIL_BYTES</Text>），超出会滚删最旧的部分。日志框上方出现断档提示时，表示中间确实丢了一段</li>
            <li>超时固定 5 分钟且不可配置。需要跑更久的任务，请配成 Hook 并勾选「异步执行」，在那里调超时</li>
            <li>同时最多 3 个试运行，占满后新的试运行直接被拒绝（不排队）——试运行是要立刻看到输出的动作，排队没有意义</li>
            <li>关闭页面或刷新只是停止观看，脚本仍在服务端继续跑；但页面拿不回原来的 run，日志框会空掉</li>
          </ul>
          <Paragraph>
            运行期间「试运行」按钮变成「中断」。中断后本地执行连同脚本派生的子进程一起终止，
            远端执行关闭 SSH 会话，状态记为「已中断」并在日志尾部留下标记——
            这样事后能分清是脚本自己挂了还是人工停的。
            <Text strong>已脱离 SSH 会话的远端进程中断不到</Text>（例如 Windows 上 fire-and-forget 起的进程），
            「已中断」只代表本服务不再跟踪它。
          </Paragraph>
        </Typography>
  </>
)

const responseColumns = [
  { title: '状态码', dataIndex: 'code', width: 90, render: (v: string) => <Text code>{v}</Text> },
  { title: '场景', dataIndex: 'scene' },
  { title: 'Body', dataIndex: 'body' },
]

const paramColumns = [
  { title: '参数', dataIndex: 'name', width: 140, render: (v: string) => <Text code>{v}</Text> },
  { title: '位置', dataIndex: 'where', width: 90 },
  { title: '说明', dataIndex: 'desc' },
]

const fieldColumns = [
  { title: '字段', dataIndex: 'name', width: 180, render: (v: string) => <Text code>{v}</Text> },
  { title: '说明', dataIndex: 'desc' },
]

const triggerResponses = [
  { key: '200', code: '200', scene: '同步执行成功', body: '{"message", "output"}' },
  { key: '202', code: '202', scene: '异步执行已受理，凭 execution_id 轮询日志', body: '{"message", "execution_id", "status": "queued"}' },
  { key: '401', code: '401', scene: 'Token / 签名校验失败', body: '{"error"}' },
  { key: '404', code: '404', scene: 'Hook 不存在', body: '{"error"}' },
  { key: '409', code: '409', scene: '同一异步 Hook 上次执行未结束', body: '{"error", "running_execution_id"}' },
  { key: '429', code: '429', scene: '异步并发与队列均满', body: '{"error"}' },
  { key: '500', code: '500', scene: '同步执行失败', body: '{"error", "output"}' },
]

const execListParams = [
  { key: 'limit', name: 'limit', where: 'query', desc: '每页条数，默认 50' },
  { key: 'offset', name: 'offset', where: 'query', desc: '偏移量，默认 0' },
  { key: 'hook_id', name: 'hook_id', where: 'query', desc: '按 Hook ID 过滤' },
]

const execDetailParams = [
  { key: 'id', name: 'id', where: 'path', desc: '执行 ID' },
]

const logParams = [
  { key: 'id', name: 'id', where: 'path', desc: '执行 ID' },
  { key: 'after_seq', name: 'after_seq', where: 'query', desc: '游标，默认 0；之后带上次响应的 next_seq' },
]

const execListResponses = [
  { key: '200', code: '200', scene: '成功', body: 'Execution 数组（不含 output / error，字段见下表）' },
]

const execDetailResponses = [
  { key: '200', code: '200', scene: '成功', body: '完整 Execution（含 output / error）' },
  { key: '404', code: '404', scene: '执行记录不存在', body: '{"error"}' },
]

const logResponses = [
  { key: '200', code: '200', scene: '成功', body: '分页日志对象（结构见下）' },
  { key: '404', code: '404', scene: '执行记录不存在', body: '{"error"}' },
]

const execFields = [
  { key: 'id', name: 'id', desc: '执行 ID（整数）' },
  { key: 'hook_id', name: 'hook_id', desc: '触发的 Hook' },
  { key: 'trigger_source', name: 'trigger_source', desc: '调用方 IP' },
  { key: 'exec_target', name: 'exec_target', desc: 'local，或 SSH 主机的 user@host:port' },
  { key: 'status', name: 'status', desc: 'queued / running / success / failed / timeout / canceled / interrupted' },
  { key: 'output', name: 'output / error', desc: '输出与错误；仅详情接口返回，列表省略，按 LOG_TAIL_BYTES 截尾' },
  { key: 'started_at', name: 'started_at / finished_at', desc: '起止时间（RFC3339），未结束时 finished_at 缺省' },
]

const logPageFields = [
  { key: 'chunks', name: 'chunks', desc: '日志块数组，每块带 seq 与 stream（stdout/stderr）；单页最多 500 块' },
  { key: 'next_seq', name: 'next_seq', desc: '下次轮询的 after_seq 起点' },
  { key: 'oldest_seq', name: 'oldest_seq', desc: '仍在的最老序号；游标比它小说明中间有一段已被滚删' },
  { key: 'has_more', name: 'has_more', desc: '还有积压未拉完，应立即续拉' },
  { key: 'status', name: 'status / finished', desc: '执行状态与是否结束' },
]

const apiContent = (
  <>
        <Typography>
          <Title level={3}>API 接口</Title>
          <Paragraph>
            面向外部系统（CI、监控等）。分两类入口：触发执行按 Hook 配置的 Token / HMAC 校验（无需登录）；
            查询类接口凭 <Text code>X-API-Token</Text> 请求头访问，作用域仅限只读执行记录与日志。
          </Paragraph>

          <Title level={4}>通用约定</Title>
          <ul>
            <li>所有响应均为 JSON；错误统一返回 <Text code>{'{"error": "原因"}'}</Text></li>
            <li>时间戳为 RFC3339 格式</li>
            <li>查询类接口：token 不符返回 <Text code>401</Text>；未生成 token 一律返回 <Text code>403</Text></li>
          </ul>

          <Title level={4}>获取 Token</Title>
          <Paragraph>
            在「设置」页生成，通过请求头传递：<Text code>X-API-Token: &lt;token&gt;</Text>。
            重新生成会使旧 token 立即失效，外部调用全部中断。
          </Paragraph>

          <Title level={4}>触发执行</Title>
          <Paragraph code copyable>
            POST /hooks/&lt;hook-id&gt;
          </Paragraph>
          <Paragraph>
            也支持 <Text code>GET</Text>。认证按 Hook 配置：固定 Token（<Text code>X-Token</Text> /
            <Text code> X-Gitlab-Token</Text> 请求头或 <Text code>?token=</Text> 查询参数）
            或 HMAC 签名（<Text code>X-Signature</Text> / <Text code>X-Hub-Signature-256</Text>），配置方式见「字段说明」tab。
          </Paragraph>
          <Paragraph>响应：</Paragraph>
          <Table size="small" pagination={false} columns={responseColumns} dataSource={triggerResponses} />
          <Paragraph style={{ marginTop: 12 }}>
            同步 Hook 跑完才返回（200 / 500）；异步 Hook 立即返回 202，凭 <Text code>execution_id</Text> 轮询下方日志接口。
          </Paragraph>

          <Title level={4}>查询执行记录列表</Title>
          <Paragraph code copyable>
            GET /api/external/executions
          </Paragraph>
          <Table size="small" pagination={false} columns={paramColumns} dataSource={execListParams} />
          <Paragraph style={{ marginTop: 12 }}>响应：</Paragraph>
          <Table size="small" pagination={false} columns={responseColumns} dataSource={execListResponses} />
          <Paragraph style={{ marginTop: 12 }}>Execution 字段：</Paragraph>
          <Table size="small" pagination={false} columns={fieldColumns} dataSource={execFields} />

          <Title level={4}>查询执行详情</Title>
          <Paragraph code copyable>
            GET /api/external/executions/&lt;id&gt;
          </Paragraph>
          <Table size="small" pagination={false} columns={paramColumns} dataSource={execDetailParams} />
          <Paragraph style={{ marginTop: 12 }}>响应：</Paragraph>
          <Table size="small" pagination={false} columns={responseColumns} dataSource={execDetailResponses} />

          <Title level={4}>轮询执行日志</Title>
          <Paragraph code copyable>
            GET /api/external/executions/&lt;id&gt;/logs?after_seq=0
          </Paragraph>
          <Table size="small" pagination={false} columns={paramColumns} dataSource={logParams} />
          <Paragraph style={{ marginTop: 12 }}>响应：</Paragraph>
          <Table size="small" pagination={false} columns={responseColumns} dataSource={logResponses} />
          <Paragraph style={{ marginTop: 12 }}>200 时的 body：</Paragraph>
          <pre style={{ background: '#f6f8fa', padding: 12, borderRadius: 6 }}>{`{
  "chunks": [{ "seq": 1, "stream": "stdout", "text": "..." }],
  "next_seq": 42,
  "oldest_seq": 3,
  "has_more": false,
  "status": "success",
  "finished": true
}`}</pre>
          <Table size="small" pagination={false} columns={fieldColumns} dataSource={logPageFields} />
          <Paragraph style={{ marginTop: 12 }}>
            轮询策略：执行中每 2 秒拉一次；<Text code>has_more</Text> 为真时不停顿续拉；
            <Text code>finished</Text> 为真且无积压后停止。
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
