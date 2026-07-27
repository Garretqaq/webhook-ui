# 02 — 脚本管理（本地执行）

**What to build:** 新增脚本管理页，管理员可增删改查脚本（名称、解释器下拉 bash/sh/python3、内容、描述），内容编辑器用 antd Input.TextArea。页内"试运行"按钮直接测试编辑器当前内容（解释器+内容+参数，参数一行一个），无需先保存，返回 stdout/stderr/成败。执行方式：内容写入 DATA_DIR/tmp 下临时文件（权限 0700），跑 `解释器 文件 参数...`，结束删除。解释器二进制必须通过 ALLOWED_COMMANDS 白名单校验；白名单默认值加 /bin/bash、/bin/sh、/usr/bin/python3。执行超时复用现有 5 分钟。

**Blocked by:** None — can start immediately

**Status:** done (commit 03e04a7)

- [x] scripts 表迁移（name/interpreter/content/description/时间戳）
- [x] 解释器枚举校验 bash/sh/python3，执行前过白名单
- [x] CRUD API + 管理页（列表 + 编辑页，TextArea 编辑器）
- [x] 试运行接口（测编辑器当前内容，不落库）+ 页面按钮与结果展示
- [x] 本地临时文件执行（0700，跑完删除，5 分钟超时）
- [x] ALLOWED_COMMANDS 默认值加解释器路径
