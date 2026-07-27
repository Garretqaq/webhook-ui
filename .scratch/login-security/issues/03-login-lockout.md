# 03 — 登录失败锁定（username+IP）

**What to build:** 同一 username+IP 组合连续登录失败 5 次后锁定 15 分钟，期间该组合的登录请求直接返回 429 和剩余秒数，前端显示"已锁定，请 X 分钟后重试"。登录成功清零该组合计数；失败记录超过锁定时长未再尝试则惰性过期（无后台 goroutine）。锁定状态存内存，重启清零。失败尝试与锁定触发打 WARN 日志（记 username+IP，不记密码）。

**Blocked by:** 01（锁定计数挂在 username 上）、02（依赖真实客户端 IP）

**Status:** done

- [x] 失败计数按 username+IP 组合维护（内存 map）
- [x] 阈值 5 次、锁定时长 15 分钟，`LOGIN_MAX_FAILURES`、`LOGIN_LOCKOUT_MINUTES` env 可覆盖
- [x] 锁定期间返回 429 + 剩余秒数，前端展示剩余等待时间
- [x] 登录成功清零该组合计数
- [x] 失败记录惰性过期，无后台清理 goroutine
- [x] 失败与锁定事件打 WARN 日志（username+IP，不含密码）
- [x] 攻击者从 IP-X 爆破只锁 IP-X，管理员从其他 IP 仍可登录
