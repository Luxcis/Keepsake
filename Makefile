# 常用命令（详见 README）
.PHONY: dev-api dev-web up down rebuild tidy create-admin

# 本地开发：API（从仓库根目录运行，数据库落在 ./data）
dev-api:
	cd server && DATA_DIR=$(PWD)/data go run ./cmd/server

# 本地开发：Web（Vite 代理 /api -> localhost:8080）
dev-web:
	cd web && npm run dev

# Docker 全链路
up:
	docker compose up --build -d
down:
	docker compose down
rebuild:
	docker compose build --no-cache

tidy:
	cd server && go mod tidy

# 引导管理员账号（Argon2id）；可选同时启用站点访问门禁：
#   make create-admin ADMIN_USERNAME=admin ADMIN_PASSWORD=*** [ADMIN_SITE_PASSWORD=***]
create-admin:
	@test -n "$(ADMIN_USERNAME)" -a -n "$(ADMIN_PASSWORD)" || { echo '用法: make create-admin ADMIN_USERNAME=x ADMIN_PASSWORD=y [ADMIN_SITE_PASSWORD=z]'; exit 1; }
	cd server && DATA_DIR=$(PWD)/data go run ./cmd/server -create-admin

# 容器内引导（服务运行中）:
#   docker compose exec -e ADMIN_USERNAME=admin -e ADMIN_PASSWORD=*** api /usr/local/bin/server -create-admin
