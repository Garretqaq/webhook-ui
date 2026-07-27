# 05 — 文档更新

**What to build:** README 补充三块内容：(1) 脚本管理与 SSH 功能使用说明，含安全含义——SSH 凭据存于本地 SQLite、TOFU host key 机制、公网部署建议手动预填 host_key；(2) "Docker 中控制宿主机"章节：挂载目录、挂 docker.sock（标注风险）、二进制直跑宿主机三种方案；(3) ALLOWED_COMMANDS 默认值变更说明（新增 bash/sh/python3）及其安全含义。

**Blocked by:** 01 — SSH 主机管理；02 — 脚本管理（本地执行）；03 — Hook 关联脚本；04 — SSH 远程执行脚本

**Status:** done (commit 1f50e92)

- [x] 脚本管理 + SSH 功能使用文档
- [x] 安全说明：凭据存储、TOFU、公网预填 host_key 建议
- [x] Docker 控制宿主机三方案章节
- [x] ALLOWED_COMMANDS 默认值变更说明
