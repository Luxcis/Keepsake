# 拾光集 Keepsake · 我们的旅行纪念册

私人旅行纪念网站（V1.0）：以旅行地点与时间为主线，把照片、旅行故事、重要日子与生日信组织成一段可浏览、可持续补充的数字纪念册。首个交付目标：**2026-11-22 生日当天可稳定访问**。

- 设计/开发计划全文：[docs/plan.docx](docs/plan.docx)
- UI 视觉方向（已确认稿）：[docs/demo.html](docs/demo.html)

## 技术栈

| 层级 | 选型 |
| --- | --- |
| 前端 | Vue 3 + TypeScript + Vite + Tailwind CSS 4 + GSAP |
| 后端 | Go 1.26 + chi（路由）+ `modernc.org/sqlite`（纯 Go 驱动）|
| 缓存/会话 | Redis 8（go-redis/v9），可缺失运行（降级模式）|
| 文件存储 | 本地磁盘（开发默认）或腾讯 COS（私有桶直传；读取支持短时效签名或 CDN 域名），`STORAGE_DRIVER` 切换 |
| 部署 | Docker Compose：单镜像 app（Nginx + Go API 同容器, dist + 反代 127.0.0.1:8080）+ 可选 redis（profiles） |

## 目录结构

```
├─ docs/plan.docx    设计与实施计划（含 8 周期排期与验收清单）
├─ docs/demo.html    首页视觉方向（配色/排版动效参考稿）
├─ server/           Go API
│  ├─ cmd/server/            启动入口（配置 → 迁移 → 路由 → 优雅退出）
│  ├─ internal/config/       环境变量配置
│  ├─ internal/store/        SQLite 连接 + 迁移器 + 显式 SQL 查询
│  │  └─ migrations/0001_init.sql   核心八表 + 索引 + 站点设置种子
│  ├─ internal/handlers/     JSON 接口（统一 envelope / 501 占位策略）
│  ├─ internal/localstore/   本地文件存储（开发默认；签名直传/读取）
│  ├─ internal/cachex/       Redis 封装（探活语义）
│  └─ internal/server/       chi 路由与中间件（slog 日志 / CORS / Recoverer）
├─ Dockerfile        统一镜像（三阶段：Vue dist + Go server + Nginx 运行时）
├─ web/              Vue 3 SPA（页面路由即计划 §2 的七大页面）
├─ deploy/
│  ├─ nginx/         Nginx 站点配置（dist + /api 反代 127.0.0.1:8080）
│  └─ entrypoint.sh  统一镜像入口（uid 10001 起 Go API，PID1 交给 Nginx）
├─ data/             SQLite 持久化目录（挂载进 app 容器，勿放共享盘）
└─ docker-compose.yml
```

## 页面路由（阶段交付：路由骨架）

| 路由 | 页面 |
| --- | --- |
| `/` | 首页（主视觉 + CTA） |
| `/trips` · `/trips/:id` | 足迹 · 旅行详情（含该旅行的照片与视频列表、灯箱） |
| `/photos` | 旧照片墙链接，重定向到 `/trips` |
| `/timeline` | 时光轴 |
| `/letter` | 生日信（门禁阶段接入数据） |
| `/admin/login` · `/admin` | 管理登录 · 后台（认证阶段接入） |

## 本地开发

```bash
# 1. 后端（数据库自动建表在 ./data/journey.db）
cd server && DATA_DIR=$PWD/data go run ./cmd/server

# 2. 前端（另一终端）
cd web && npm run dev # http://localhost:5173，/api 自动代理到 :8080
```

接口自检：`curl http://localhost:8080/healthz`

照片存储：开发环境未配置 COS 时自动使用**本地磁盘**（`server/data/uploads`，即 `DATA_DIR/uploads`），无需任何云服务即可完成上传/浏览/删除。可用 `STORAGE_DRIVER=local|cos` 显式指定；COS 四项配置齐全时自动优先使用 COS。

## 引导：管理员与站点访问门禁

```bash
# 创建管理员（Argon2id 哈希入库）
cd server && DATA_DIR=$PWD/data \
  ADMIN_USERNAME=admin ADMIN_PASSWORD='你的管理密码' \
  ADMIN_SITE_PASSWORD='给TA的访问口令' \
  go run ./cmd/server -create-admin

# 或在容器运行中引导：
docker compose exec -e ADMIN_USERNAME=admin -e ADMIN_PASSWORD='***' -e ADMIN_SITE_PASSWORD='***' \
  app /usr/local/bin/server -create-admin
```

- `ADMIN_SITE_PASSWORD` 未设置时，门禁处于“开放模式”，`/api/site/settings` 中 `gate_enabled: false`，日志有明确告警；随时可通过 `PUT /api/admin/site/settings {"site_password":"..."}` 启用。
- 管理员会话 24h，门禁会话 30 天，均为 HttpOnly + SameSite=Lax Cookie；错误请求体大小受限，登录失败 5 次/15 分钟即限速（Redis 固定窗口）。

### Docker 自动引导

在 `.env` 填好 `ADMIN_USERNAME`/`ADMIN_PASSWORD`（可选 `ADMIN_SITE_PASSWORD`）后，`docker compose up -d` 时 app 容器启动会自动创建首个管理员，无需手动执行引导命令：

- 仅当该用户名**不存在**时创建；同名已存在则跳过日志提示，**不重置密码**（重启容器不会覆盖现有账号）。
- 变量只填其一会在日志告警并跳过；全部留空则行为与之前完全一致。
- 自动引导与 CLI 使用同一套校验（用户名 2..64、密码 ≥8），引导失败只记日志不阻断启动，修正 `.env` 后重启即可重试。
- 安全提示：配置后会以明文进入容器环境变量（`docker inspect` 可见）；介意可把 `.env` 中 `ADMIN_PASSWORD` 留空，首次建号改用上面 CLI 方式。

## Docker 本地跑通

```bash
cp .env.example .env   # 按需修改（本地 http 联调保持 COOKIE_SECURE=false）
# 不启用 Redis（默认，仅 app 一个服务）：
docker compose up --build -d
# 启用 Redis（.env 放开 REDIS_ADDR=redis:6379 后）：
docker compose --profile redis up --build -d
# 连接服务器已有 Redis（不需要本仓 redis 服务，无需 --profile）：
#   .env 配 REDIS_ADDR=<host>:6379；Redis 开启了 requirepass 时再加 REDIS_PASSWORD=<密码>
# 国内网络构建（npm 走 npmmirror、Go 模块走 goproxy.cn；或在 .env 放开 BUILD_DOCKERFILE）：
BUILD_DOCKERFILE=Dockerfile-cn docker compose build
#   打开 http://localhost:8088 （Nginx → SPA + /api 反代 127.0.0.1:8080）
curl http://localhost:8088/api/site/settings
docker compose down
```

> Linux 服务器上如遇 SQLite 写入权限问题：`sudo chown -R 10001 ./data`（容器内 API 以 uid 10001 运行）。

## 已实现接口（后端基础阶段，11/05–11/08）

| 方法 | 路径 | 权限 | 说明 |
| --- | --- | --- | --- |
| GET | `/healthz` | 公开 | db / redis 探活 |
| GET | `/api/site/settings` | 公开 | 白名单字段 + `gate_enabled` |
| POST | `/api/site/unlock` | 公开（限速） | 访问口令 → 门禁会话 |
| POST | `/api/auth/login`·`/logout` | 公开（限速） | Argon2id 校验 + 会话 |
| GET | `/api/auth/me` | 管理员 | 当前账号 |
| GET | `/api/trips`·`/trips/{id}` | 门禁 | 发布内容读取（列表按出发日期倒序） |
| GET | `/api/letter` | 门禁 | 生日信 |
| POST/PUT/DELETE | `/api/admin/trips`（含 `{id}/locations`、`{id}/albums`） | 管理员 | 旅行/地点/相册 CRUD |
| PUT | `/api/admin/letter` | 管理员 | 信件 upsert |
| PUT | `/api/admin/site/settings` | 管理员 | 白名单字段 + 口令哈希化 |
| — | `/api/trips/{id}/photos`、`/api/admin/uploads/sign`、`/api/admin/photos/confirm` | — | **501**：COS 与后台阶段（11/13–11/15）实现 |

### 安全语义（计划 §3.2 / §5.1）

- **失败关闭**：`REDIS_ADDR` 已配置但不可达时，登录/解锁/管理端会话读取返回 `503 backend_unavailable`，不放行。
- **可选降级**：未配置 Redis 时使用进程内存会话与限速（本站为单实例部署的合法形态；容器重启后需重新登录，且限速计数清零。多实例部署才必须启用 Redis）。
- **CSRF**：SameSite=Lax Cookie + “不安全方法携带 Origin/Referer 时必须同源（感知 X-Forwarded-Proto）”双层防护。

## 照片上传链路（本地存储 / COS）

```text
选择文件 → POST /api/admin/uploads/sign（校验类型/大小，服务端生成 Key）
        → 浏览器 XHR 直传（PUT，进度条）→ POST /api/admin/photos/confirm
        （服务端 HEAD 校验真实存在/大小/类型后才入库）
删除：先删数据库关联 → 同步删除原图/缩略图/视频对象；仅删除失败的对象入队
      → 后台 janitor 60s 轮询补偿（404 幂等）
展示：GET /api/trips/{id}/photos 输出读取 URL（CDN 无签名或短时效签名）
```

存储后端由 `STORAGE_DRIVER` 切换，前端链路完全一致：

- **local（开发默认）**：签名 URL 指向本服务 `/api/storage/upload/*`（PUT）与 `/api/storage/files/*`（GET），
  文件落在 `{DATA_DIR}/uploads`；URL 带 HMAC 能力签名与有效期，不依赖 Cookie。
  本地模式缩略图/灯箱直接读取原图（浏览器按显示尺寸缩放，同一 URL 可复用缓存）。
- **cos**：直传腾讯 COS 私有桶，缩略图 URL 签入 imageMogr2 处理参数（私有桶下签名后追加参数会被 COS 拒绝 403）。

选择规则：显式 `STORAGE_DRIVER=local|cos|none` 优先；留空时 COS 四项齐全用 COS，否则开发环境用本地磁盘、生产环境禁用上传（`503 cos_not_configured`）。单机生产可直接 `STORAGE_DRIVER=local`（`./data:/data` 已挂载持久化，也可用 `STORAGE_SECRET` 固定签名密钥）。

启用 COS 前必须在腾讯云 COS 控制台完成（计划 §6.2 风险项）：

1. Bucket 权限默认设为**私有**（只允许本服务密钥与预签名访问）；若配置 `COS_CDN_DOMAIN` 由 CDN 读取，需在 CDN 开启回源鉴权，或把 Bucket 设为公共读（照片可被直接访问）。
2. Bucket **CORS** 允许：来源 `http://<站点域名>`；方法 `PUT, GET, HEAD`；Headers `Content-Type`。上传对象上限为图片 30MB、视频 400MB，由服务端与 Nginx 同步限制。
3. 建议开启 COS 版本控制/生命周期策略（照片不可再生，备份 §5.3）。

之后在 `.env` 填入 `COS_SECRET_ID/KEY/REGION/BUCKET` 并 `docker compose up -d --build`，照片上传立刻可用。如需通过 CDN 读取照片，设置 `COS_CDN_DOMAIN=photos.example.com`（可带 `https://`，不能带路径）：原图/缩略图/灯箱 URL 直接指向该域名，不带 COS 签名与有效期（需在 CDN 开启回源鉴权，或把 Bucket 设为公共读；缩略图/灯箱沿用 imageMogr2 参数，CDN 侧需开启图片处理），上传 PUT、HEAD 校验与删除仍走 `{bucket}.cos.{region}.myqcloud.com`；未配置时读取继续使用短时效签名 URL。只想用本地磁盘时无需 COS：开发环境自动启用，容器部署在 `.env` 设置 `STORAGE_DRIVER=local`（文件写入 `./data/uploads`）；两者都未配置时上传接口返回语义化的 `503 cos_not_configured`。

## 一键备份与恢复

管理端「备份」页提供一键备份：点击后服务端在后台生成 zip，页面轮询进度；同一时间只允许一个任务，离开页面不会中断。备份文件保存在 `{BACKUP_DIR}`（默认 `{DATA_DIR}/backups`，容器内 `/data/backups`，随 `./data:/data` 挂载保留），可下载到本机或删除。

备份包结构：

```text
keepsake-backup-YYYYMMDD-HHMMSS.zip
├─ journey.db          # SQLite VACUUM INTO 一致性快照（不依赖 -wal/-shm）
├─ manifest.json       # schema、创建时间、环境、存储驱动、数据库/文件统计、对象列表、排除项
└─ uploads/<key>       # 本地存储全部普通文件；COS 模式下为桶内全部对象
```

- 包含：全部数据库表（管理员、旅行、照片、站点设置等）与全部上传对象。
- 不包含：Redis 会话/限流数据、`.env` 与部署配置、备份目录自身。
- 本地存储跳过上传临时文件（`.upload-*`）与系统文件（`.DS_Store`）；`.secret` 正常收录。
- 生成失败不发布半成品：先写 `.tmp`，成功后原子重命名；进程重启会中断进行中的任务并清理临时文件。

恢复（需停机）：

1. `docker compose stop app` 停止服务，并另外备份当前 `data/` 目录。
2. 解压备份包，将 `journey.db` 覆盖到 `data/`（先删除同目录的 `journey.db-wal`/`journey.db-shm`）。
3. 将包内 `uploads/` 覆盖到 `data/uploads/`（本地存储模式）。
4. `docker compose start app`，用管理员账号登录确认数据与照片。

COS 模式：备份会分页列出桶内全部对象并逐个下载，耗时与流量随媒体总量增长；对象读取失败会导致整次备份失败，不会生成缺少文件的包。

## 阶段路线（→ 计划 §6 排期）

- [x] **11/03–11/04 脚手架**：仓库骨架、配置样例、页面路由、Docker/Nginx 本地跑通
- [x] **11/05–11/08 后端基础**：门禁（site/unlock）、Argon2id 认证 + 限流 + 会话、旅行/地点/相册/时光轴 CRUD、管理端写接口
- [x] **11/09–12 前端核心**：七页对接真实数据、门禁解锁卡片、信件揭晓交互、灯箱组件、admin 会话态
- [x] **11/13–15 COS 与后台**：预签名直传（XHR 进度）→ HEAD 确认 → 照片排序/说明/删除（COS 对象补偿清理）、管理端五个维护页、照片墙按旅行分组 + 灯箱
- [x] **11/16–18 视觉与动效**：GSAP 动态加载（motion-ready 防闪 / fromTo 终态 / reduced-motion 与加载失败全降级）、首屏入场、卡片错落、时间轴 scrub、信封展开、灯箱过渡、360px 防溢出
- [ ] 11/19–11/20 部署与测试：HTTPS、备份恢复演练、权限异常测试
- [ ] 11/21 内容冻结：录入真实照片与旅行信息
- [ ] 11/22 正式交付

## 工程约定（摘自计划）

- 应用层记录 **UTC**（SQLite 中 TEXT RFC3339 / YYYY-MM-DD），展示层按 Asia/Shanghai 格式化。
- SQLite 开启 WAL + busy_timeout，连接池保守；备份使用 Online Backup API / `.backup`，勿复制裸 `.db`。
- 对象 Key 服务端生成；COS SecretId/Key 严禁出服务端；未配置 CDN 时读取 URL 短时效签名。
- 日志（slog JSON）不记录密码、Cookie、签名 URL。
- 当前脚手架运行在可选 Redis 形态，检查点见 `/healthz`：`redis: disabled`=未启用（预期内）、`unreachable`=已配置但连不上（此时认证写入失败关闭）。
