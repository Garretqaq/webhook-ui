# 04 — 登录 IP 限速

**What to build:** `/api/login` 按客户端 IP 限速，每 IP 每分钟最多 10 次登录尝试，超限返回 429，前端正常展示提示。只罩登录接口，webhook trigger 等其他端点不受影响。限速状态存内存，重启清零。

**Blocked by:** 02（依赖真实客户端 IP）

**Status:** done

- [x] 每 IP 每分钟最多 10 次 `/api/login` 请求，`LOGIN_RATE_LIMIT_PER_MIN` env 可覆盖
- [x] 超限返回 429，前端有可读的提示
- [x] 仅作用于 `/api/login`，webhook trigger 等端点不受影响
