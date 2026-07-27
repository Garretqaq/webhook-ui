# 03 — Hook 关联脚本

**What to build:** Hook 编辑页改为二选一模式：选择已建脚本，或保留现有自由 command，两者互斥，老数据不受影响。Webhook 触发时若 hook 绑了脚本，解析脚本内容执行；传参机制完全复用现有 buildCommandInput（QUERY_*/HEADER_* 环境变量、PassArguments 位置参数、PAYLOAD 传递），脚本内自行做参数判断。删除被 hook 引用的脚本时阻止，报错列出引用它的 hook。执行记录结构不变。

**Blocked by:** 02 — 脚本管理（本地执行）

**Status:** done (commit cc7feab)

- [x] hooks 表加 script_id 列（可空，关联 scripts）
- [x] Hook 编辑页：脚本/command 二选一互斥 UI，老数据正常展示
- [x] 触发流程：绑脚本时解析脚本走脚本执行路径，传参复用现有机制
- [x] 脚本删除保护：被引用时报错并列出引用 hook
- [x] 执行记录沿用现有结构
