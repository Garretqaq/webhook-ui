# Webhook UI

基于 Go 的 Webhook 管理工具，带 Web 控制台。

## 功能

- Webhook 接收并执行 shell 命令
- HMAC 签名验证 (SHA1/SHA256/SHA512)
- 参数传递 (query/header/payload)
- 执行日志查看
- 管理员登录认证
- 命令白名单安全控制

## 快速开始

### 本地运行

```bash
# 构建前端
cd web && npm install && npm run build && cd ..

# 运行
export ADMIN_PASSWORD=your-password
go run main.go
```

访问 http://localhost:9000

### Docker 运行

```bash
docker run -d \
  -p 9000:9000 \
  -e ADMIN_PASSWORD=your-password \
  -v webhook-data:/app/data \
  registry.cn-hangzhou.aliyuncs.com/dato/webhook-ui:latest
```

## 环境变量

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `PORT` | 服务端口 | `9000` |
| `DATA_DIR` | 数据目录 | `./data` |
| `ADMIN_PASSWORD` | 管理员密码 | (必填) |
| `SESSION_SECRET` | Session 密钥 | 自动生成 |
| `ALLOWED_COMMANDS` | 允许的命令前缀，逗号分隔 | `/usr/bin/git,/usr/bin/curl,/bin/bash,/bin/sh` |

## API

### Webhook 触发

```
POST /hooks/:id
```

支持 HMAC 签名验证:
- GitHub: `X-Hub-Signature-256` header
- GitLab: `X-Gitlab-Token` header
- 通用: `X-Signature` header

### 管理 API

需要登录:
- `POST /api/auth/login` - 登录
- `POST /api/auth/logout` - 登出
- `GET /api/hooks` - Hook 列表
- `POST /api/hooks` - 创建 Hook
- `GET /api/hooks/:id` - Hook 详情
- `PUT /api/hooks/:id` - 更新 Hook
- `DELETE /api/hooks/:id` - 删除 Hook
- `GET /api/executions` - 执行日志
- `GET /api/executions/:id` - 执行详情

## 开发

### 后端

```bash
go run main.go
```

### 前端 (开发模式)

```bash
cd web
npm run dev
```

前端开发服务器会自动代理 `/api` 和 `/hooks` 到 `localhost:9000`。
