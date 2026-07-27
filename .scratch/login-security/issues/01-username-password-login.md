# 01 — 账号+密码登录贯通

**What to build:** 用户用用户名+密码登录管理界面。后端同时校验用户名和密码（用户名来自 `ADMIN_USERNAME` env，默认 `admin`；密码沿用 `ADMIN_PASSWORD`），前端登录页增加用户名输入框（默认填 `admin`）。凭证错误统一提示"用户名或密码错误"，不区分哪个错。

**Blocked by:** None — can start immediately

**Status:** done

- [x] 登录接口要求 username + password，两者都校验（常量时间比较）
- [x] `ADMIN_USERNAME` env 可配置，默认 `admin`
- [x] 前端登录页有用户名输入框，默认值 `admin`
- [x] 凭证错误统一返回"用户名或密码错误"
- [x] 正确的用户名+密码可登录，错误组合全部拒绝
