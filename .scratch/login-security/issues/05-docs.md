# 05 — 文档更新

**What to build:** README 记录本次新增的全部环境变量及其默认值：`ADMIN_USERNAME`、`LOGIN_MAX_FAILURES`、`LOGIN_LOCKOUT_MINUTES`、`LOGIN_RATE_LIMIT_PER_MIN`、`TRUSTED_PROXIES`，并简述登录锁定与限速行为。

**Blocked by:** 01、03、04（文档描述的行为以这些 ticket 落地为准）

**Status:** done

- [x] README 包含全部新 env 的名称、默认值、含义
- [x] 简述锁定与限速行为（5 次锁 15 分钟、每 IP 每分钟 10 次）
