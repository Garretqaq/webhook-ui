# 02 — 可信代理配置

**What to build:** 服务通过 `SetTrustedProxies` 只信任配置的代理，`TRUSTED_PROXIES` env 可覆盖（默认 `127.0.0.1`）。反代部署后 `ClientIP()` 返回真实客户端 IP，攻击者无法靠伪造 `X-Forwarded-For` 头伪装来源 IP。这是后续 IP 限速与锁定的基础。

**Blocked by:** None — can start immediately

**Status:** done

- [x] 启动时调用 `SetTrustedProxies`，代理列表来自 `TRUSTED_PROXIES` env（逗号分隔），默认 `127.0.0.1`
- [x] 反代（nginx 等）后面 `ClientIP()` 取到真实客户端 IP
- [x] 直连部署时不信任任何转发的 `X-Forwarded-For`
