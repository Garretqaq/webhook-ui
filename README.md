# Webhook UI

基于 Go 的 Webhook 管理工具，带 Web 控制台。

## 功能

- Webhook 接收并执行 shell 命令
- 脚本管理：脚本存数据库，支持 bash/sh/python3，页内试运行
- SSH 远程执行：脚本可在配置的远端主机上运行（TOFU host key 校验）
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
| `ALLOWED_COMMANDS` | 允许的命令前缀，逗号分隔 | `/usr/bin/git,/usr/bin/curl,/bin/bash,/bin/sh,/usr/bin/python3` |

**注意**：默认值包含 bash/sh/python3 解释器（脚本管理功能需要）。这意味着能登录控制台的管理员本质上可以执行任意命令——这符合本工具的定位，但请勿将控制台暴露给不可信网络。如不需要脚本功能，可用 `ALLOWED_COMMANDS` 收紧。

## 脚本管理

「脚本管理」页维护可复用的脚本（名称、解释器 bash/sh/python3、内容），无需登录服务器放置文件。编辑页可直接试运行当前内容（无需保存）。

- Hook 配置时二选一：自由命令 或 绑定脚本，互斥
- 本地执行：脚本写入 `DATA_DIR/tmp` 临时文件（0700）运行，跑完删除；解释器必须命中 `ALLOWED_COMMANDS` 白名单
- 传参与自由命令一致：`$1/$2...`（Payload 字段）、`QUERY_*`/`HEADER_*` 环境变量、`PAYLOAD`；参数判断逻辑写在脚本内
- 被 Hook 引用的脚本不可删除

## SSH 远程执行

脚本的「执行位置」可选 SSH 主机，脚本内容通过 stdin 在远端执行（`bash -s` 等），不写入远端磁盘。

「SSH 主机」页维护连接信息（host/port/user，私钥或密码认证），配完可点「测试连接」验证。

**安全说明**：

- 凭据（私钥/密码）存储在服务端本地 SQLite（`DATA_DIR`），请保护好数据目录
- Host Key 采用 TOFU：首次连接自动记录服务器公钥，之后强校验，公钥变更会拒绝连接（可能是劫持或服务器重装）。可在主机编辑页查看/清除/手动预填
- **公网环境建议**：先用 `ssh-keyscan 主机` 获取公钥并手动预填，避免首次连接被中间人截获
- 远程执行不支持 Hook 的「工作目录」设置（仅本地生效）
- 被脚本引用的主机不可删除

## Docker 中控制宿主机

脚本/命令默认运行在容器内，碰不到宿主机。三种方案：

1. **挂载目录**（推荐）：`docker run -v /宿主机/目录:/workspace ...`，脚本操作 `/workspace` 即操作宿主机文件
2. **挂 docker.sock**：`docker run -v /var/run/docker.sock:/var/run/docker.sock ...`，脚本内可执行 `docker` 命令（容器内需有 docker CLI）。等价于宿主机 root 权限，风险自负
3. **SSH 回连**：用「SSH 远程执行」功能把宿主机配为一台 SSH 主机，脚本在宿主机上跑——推荐此方式，隔离不破、权限可控
4. **不用 Docker**：单二进制直接跑在宿主机上，脚本天然就是宿主机命令

## API

### Webhook 触发

```
POST /hooks/:id
```

支持两种访问校验（可单独或同时使用）:

**固定 Token**：Hook 配置固定 Token 后，请求需带 `X-Token` header 或 `?token=` 查询参数，值相等才执行。适合不能计算 HMAC 的调用方。

**HMAC 签名**：
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
- `GET/POST/PUT/DELETE /api/scripts` - 脚本管理
- `POST /api/scripts/test` - 试运行脚本
- `GET/POST/PUT/DELETE /api/ssh-hosts` - SSH 主机管理
- `POST /api/ssh-hosts/test` - 测试连接

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
