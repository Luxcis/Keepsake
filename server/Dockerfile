# Go API 生产镜像：多阶段构建，纯 Go 驱动（CGO_ENABLED=0）保证跨平台。
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata \
  && adduser -D -H -u 10001 app
USER app
COPY --from=build /out/server /usr/local/bin/server
EXPOSE 8080
# SQLite 数据库位于挂载卷（DATA_DIR），需保证宿主机目录对 uid 10001 可写。
ENTRYPOINT ["/usr/local/bin/server"]
