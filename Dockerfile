# Build frontend
FROM node:20-alpine AS frontend-builder
WORKDIR /app/web
COPY web/package.json web/package-lock.json* ./
RUN npm ci
COPY web/ ./
RUN npm run build

# Build backend
FROM golang:1.25-alpine AS backend-builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend-builder /app/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -ldflags="-w -s" -o webhook-ui .

# Runtime
FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app

RUN mkdir -p /app/data

COPY --from=backend-builder /app/webhook-ui .

EXPOSE 9000

ENV PORT=9000
ENV DATA_DIR=/app/data

CMD ["./webhook-ui"]
