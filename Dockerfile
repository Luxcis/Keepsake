# 统一镜像：同一容器内由 Nginx(80) 提供 Vue dist，并把 /api 反代到同容器 Go API(8080)。
# 构建上下文为仓库根目录（compose: build.context = .）。
# ---------- Stage 1: 前端构建 ----------
FROM node:24-alpine AS web-build
WORKDIR /src
COPY web/package.json web/package-lock.json* ./
RUN npm install --no-audit --no-fund
COPY web/ .
RUN npm run build

# ---------- Stage 2: 后端构建（纯 Go 驱动，CGO_ENABLED=0 保证跨平台） ----------
FROM golang:1.26-alpine AS go-build
WORKDIR /src
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# ---------- Stage 3: 运行时 ----------
FROM nginx:1.28-alpine
RUN apk add --no-cache ca-certificates tzdata su-exec \
  && adduser -D -H -u 10001 app
COPY --from=web-build /src/dist /usr/share/nginx/html
COPY --from=go-build /out/server /usr/local/bin/server
# 站点配置镜像内自带 else 默认；compose 挂载 deploy/nginx/default.conf（单一事实来源）可覆盖。
COPY deploy/nginx/default.conf /etc/nginx/conf.d/default.conf
COPY deploy/entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod 755 /usr/local/bin/entrypoint.sh
EXPOSE 80
# Volume 挂载 /data（SQLite），宿主目录需对 uid 10001 可写（README 提示不变）。
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
