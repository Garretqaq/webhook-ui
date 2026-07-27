#!/usr/bin/env bash
# 重新部署 docker compose 应用 (配合 webhook-ui 使用)
# 触发: POST /hooks/<hook-id>?app=aaa
#   webhook-ui 会把 query 参数注入为 QUERY_APP=aaa,脚本读取该变量,
#   进入 $APP_BASE_DIR/aaa (默认 /root/app/aaa),执行 compose down/pull/up,
#   清理旧镜像,成功后发邮件通知
#
# 配置见下方"默认配置"块,可用同名环境变量覆盖:
#   RESEND_API_KEY / EMAIL_FROM / EMAIL_TO / APP_BASE_DIR

set -euo pipefail

# ===== 默认配置 (环境变量可覆盖) =====
DEFAULT_RESEND_API_KEY=""          # Resend API key,留空则不发邮件
DEFAULT_EMAIL_FROM="提醒 <noreply@hanhandato.top>"
DEFAULT_EMAIL_TO="952430164@qq.com"
DEFAULT_APP_BASE_DIR="/root/app"   # 应用目录前缀

RESEND_API_KEY="${RESEND_API_KEY:-$DEFAULT_RESEND_API_KEY}"
EMAIL_FROM="${EMAIL_FROM:-$DEFAULT_EMAIL_FROM}"
EMAIL_TO="${EMAIL_TO:-$DEFAULT_EMAIL_TO}"
APP_BASE_DIR="${APP_BASE_DIR:-$DEFAULT_APP_BASE_DIR}"
# ====================================

# 应用名: 优先 webhook query (?app=aaa -> QUERY_APP),兜底位置参数 $1
APP="${QUERY_APP:-${1:-}}"
if [ -z "$APP" ]; then
  echo "缺少应用名: 通过 ?app=<应用名> query 参数传入" >&2
  exit 1
fi

APP_DIR="$APP_BASE_DIR/$APP"
if [ ! -d "$APP_DIR" ]; then
  echo "目录不存在: $APP_DIR" >&2
  exit 1
fi

cd "$APP_DIR"
echo "==> 部署 $APP ($APP_DIR)"

docker compose down
docker compose pull
docker compose up -d

echo "==> 清理旧镜像"
docker image prune -f

echo "==> $APP 部署完成"

# 发送成功邮件
if [ -n "$RESEND_API_KEY" ]; then
  HOST="$(hostname)"
  TIME="$(date '+%Y-%m-%d %H:%M:%S')"
  curl -sS -X POST https://api.resend.com/emails \
    -H "Authorization: Bearer $RESEND_API_KEY" \
    -H "Content-Type: application/json" \
    -d "{
      \"from\": \"$EMAIL_FROM\",
      \"to\": [\"$EMAIL_TO\"],
      \"subject\": \"[部署成功] $APP @ $HOST\",
      \"text\": \"应用 $APP 已于 $TIME 在 $HOST 重新部署完成。\n目录: $APP_DIR\"
    }" > /dev/null
  echo "==> 通知邮件已发送至 $EMAIL_TO"
fi
