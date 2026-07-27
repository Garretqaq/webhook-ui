# webhook-ui

Go + React 的 Webhook 管理工具。前端构建产物通过 `go:embed` 打进单个二进制。

## 打包发版约定

用户说"打包"时，执行以下流程：

1. **升级版本号**：修改根目录 `VERSION` 文件（语义化版本，如 `0.1.0` → `0.2.0`）。默认升 minor，用户指定则按用户要求。
2. **提交版本变更**：`git add VERSION && git commit -m "chore: bump version to x.y.z"`，推送到 main。
3. **打 tag 并推送**：`git tag vx.y.z && git push origin vx.y.z`（tag 前缀必须带 `v`）。

推送 tag 后 GitHub Actions 自动完成：

- `docker-build-push.yml`：构建并推送 Docker 镜像到阿里云 ACR（`registry.cn-hangzhou.aliyuncs.com/dato/webhook-ui`），tag 同时作为镜像标签。
- `release.yml`：前端构建 + 交叉编译二进制（linux/darwin/windows，amd64/arm64），创建 GitHub Release 并上传产物。

版本号通过 `-ldflags "-X main.version=x.y.z"` 注入，启动日志可见。

## 本地构建

```bash
cd web && npm ci && npm run build && cd ..
go build -ldflags "-X main.version=$(cat VERSION)" -o webhook-ui .
```
