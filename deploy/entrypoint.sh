#!/bin/sh
set -e
# 统一镜像入口：Go API 以 uid 10001(非 root) 后台运行，PID1 交给 Nginx 前台。
# API 崩溃时 compose healthcheck（wget 127.0.0.1:8080/healthz）失败，由 restart 策略重启容器。
su-exec 10001:10001 /usr/local/bin/server &
exec nginx -g 'daemon off;'
