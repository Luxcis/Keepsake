# 前端镜像：构建 dist 后由 Nginx 提供，静态资源 + /api 反向代理。
FROM node:24-alpine AS build
WORKDIR /src
COPY package.json package-lock.json* ./
RUN npm install --no-audit --no-fund
COPY . .
RUN npm run build

FROM nginx:1.28-alpine
COPY --from=build /src/dist /usr/share/nginx/html
EXPOSE 80
# Nginx 站点配置由 compose 挂载 deploy/nginx/default.conf（单一事实来源）。
