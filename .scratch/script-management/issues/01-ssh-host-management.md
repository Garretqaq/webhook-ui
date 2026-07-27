# 01 — SSH 主机管理

**What to build:** 新增 SSH 主机管理页，管理员可增删改查 SSH 主机（名称、host、port、user、认证方式为私钥或密码二选一）。"测试连接"按钮可验证当前表单内容（无需先保存）：建立连接并跑一个轻量命令确认可用。host key 采用 TOFU：首次连接自动记录服务器公钥，之后每次连接强校验，公钥变更时拒绝并提示；host_key 字段支持手动预填、查看、清除。实现基于 golang.org/x/crypto/ssh，不依赖容器内 ssh 客户端。

**Blocked by:** None — can start immediately

**Status:** done (commit b9f528e)

- [x] ssh_hosts 表迁移（name/host/port/user/auth 类型/凭据/host_key/时间戳）
- [x] CRUD API + 管理页（列表 + 编辑表单）
- [x] 测试连接接口（测表单当前内容）+ 页面按钮
- [x] TOFU host key：首连自动记录、后续校验、变更拒绝提示
- [x] host_key 手动预填/查看/清除
- [x] 连接超时 10 秒
