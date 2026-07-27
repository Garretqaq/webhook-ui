# 04 — SSH 远程执行脚本

**What to build:** 脚本增加"执行位置"属性：本地 或 某台 SSH 主机。选 SSH 主机时，执行经 golang.org/x/crypto/ssh 建立连接，远端起 `解释器 -s`（如 bash -s），脚本内容写 stdin 执行，env/args 经 shell 转义拼入，不落远端盘。试运行按钮同样支持远程路径。连接复用 ticket 01 的 TOFU host key 校验与 10 秒连接超时，执行超时复用 5 分钟。删除被脚本引用的 SSH 主机时阻止，报错列出引用它的脚本。

**Blocked by:** 01 — SSH 主机管理；02 — 脚本管理（本地执行）

**Status:** done (commit 8e37212)

- [x] scripts 表加执行位置字段（本地 / ssh_host_id）
- [x] 脚本编辑页：执行位置选择 UI
- [x] SSH stdin 远程执行路径（env/args shell 转义）
- [x] 试运行支持远程执行
- [x] SSH 主机删除保护：被引用时报错并列出引用脚本
- [x] 远程执行复用 TOFU 校验与超时设置
