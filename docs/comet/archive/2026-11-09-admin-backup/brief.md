# 目标

在管理控制台新增「备份」模块：管理员点击一次即可把站点当前的全部业务数据与上传文件打包成一个备份文件，保存在服务器持久卷上，并能在页面中查看、下载、删除已经生成的备份，用于本地留存或灾难恢复。

# 范围

- 备份内容：SQLite 业务数据库全部表（`users`、`trips`、`photos`、`site_settings` 等）、当前存储后端中的全部上传对象、以及一份 `manifest.json` 清单。
- 备份包为 zip，固定结构：`journey.db`（或配置的 `DB_FILE` 名）、`manifest.json`、`uploads/<对象 key>`。
- 备份包保存在 `{DATA_DIR}/backups`（可通过 `BACKUP_DIR` 覆盖），只保留在本地持久卷，不自动清理。
- 管理端新增「备份」导航与页面：一键备份、生成状态、备份列表、下载、删除。
- 服务端新增仅管理员可用的备份接口：创建（异步）、状态/列表、下载、删除。
- 存储后端为本地磁盘时遍历 `STORAGE_DIR` 收录全部文件；为腾讯 COS 时列出桶内全部对象并逐个下载收录；存储未启用时只备份数据库与清单。
- 本 change 只提供备份能力；恢复按 README 记录的手工步骤执行。

## 目录结构

### 新建

- server/internal/backup/ — 备份服务：单任务并发控制、SQLite 快照、对象枚举与打包、zip 写入、清单生成、列表/删除/下载路径校验、启动时清理残留临时文件。
- server/internal/backup/backup_test.go — 覆盖包结构、清单一致性、失败清理、并发保护、路径校验与对象源异常。
- server/internal/store/snapshot.go — `SnapshotTo`：用 `VACUUM INTO` 生成一致性数据库快照，供备份服务调用。
- server/internal/store/snapshot_test.go — 覆盖快照可独立打开、包含全部表与数据、不依赖 WAL 伴生文件。
- server/internal/handlers/backup.go — 管理端备份接口：创建、状态与列表、下载、删除；名称校验与 HTTP 语义。
- web/src/views/admin/AdminBackupView.vue — 备份页面：一键备份、生成进度、列表、下载与删除。

### 修改

- server/internal/config/config.go — 新增 `BackupDir`（默认 `{DATA_DIR}/backups`）。
- server/cmd/server/main.go — 创建备份服务并注入路由器；按存储驱动装配对象源。
- server/internal/cosutil/cosutil.go — 新增分页列出全部对象、按 key 打开对象读取流的方法。
- server/internal/localstore/localstore.go — 新增遍历存储根目录、按 key 打开文件的方法；跳过上传临时文件与系统文件。
- server/internal/handlers/api.go — 注册 `/api/admin/backups` 路由（沿用 `withAdmin` 与 CSRF 中间件）。
- server/internal/server/router_test.go — 端到端覆盖备份创建、列表、下载、删除、鉴权与非法名称。
- web/src/router/index.ts — 新增 `/admin/backup` 子路由与标题。
- web/src/views/admin/AdminView.vue — 新增「备份」导航项。
- web/src/api/client.ts — 备份状态与条目类型、列表/创建/删除调用。
- .env.example、docker-compose.yml — 补充 `BACKUP_DIR` 说明与默认值。
- README.md — 备份使用说明、包内容、手工恢复步骤与注意事项。

### 明确不建

- 独立备份数据库/任务表 — 备份文件即事实来源，状态只在进程内存，重建列表靠扫描备份目录，避免多一套持久状态。
- 备份加密、密码保护或分卷压缩 — 本次要求是一键备份，加密与恢复密钥管理超出范围。
- 定时任务、增量备份、异地/对象存储上传 — 非本次需求。

# 非目标

- 不备份 Redis 中的会话与限流数据（重启后可重建）。
- 不备份 `.env`、docker-compose、Nginx 配置与任何部署密钥。
- 不做定时/自动备份、增量备份、异地同步或把备份包再上传到 COS。
- 不提供页面上的恢复/导入功能；恢复只写入手工步骤文档。
- 不为多实例部署做分布式备份锁；同一进程内只允许一个备份任务。

# 验收示例

- A1：管理员在「备份」页点击「立即备份」，页面进入生成中状态并禁用按钮；生成完成后列表出现新条目，显示文件名（`keepsake-backup-YYYYMMDD-HHMMSS.zip`）、大小与创建时间，且 `{BACKUP_DIR}` 下存在同名文件。
- A2：下载并解压备份包，包含 `journey.db`、`manifest.json` 和 `uploads/` 下全部上传对象；用 SQLite 打开 `journey.db` 能看到备份前创建的旅行、照片与站点设置数据；包内不包含 `.env`、Redis 数据文件或备份目录自身。
- A3：`manifest.json` 的 schema、创建时间、环境、存储驱动、数据库字节数、文件数量与总字节和实际包内内容一致，并列出全部对象 key 与字节数。
- A4：备份生成期间站点仍可正常写入（新增旅行或照片记录不报错）；备份包中的数据库是一致快照，单独打开即可读取，不需要 `-wal`/`-shm` 伴生文件。
- A5：模拟对象读取失败或写包失败时，创建接口/状态返回失败原因，列表中不出现该备份，备份目录不残留半成品或临时文件。
- A6：已有备份正在生成时再次触发备份返回 409（页面按钮禁用），不会并行产生两份备份。
- A7：备份列表按创建时间倒序；点击下载得到 `application/zip` 附件且文件名与列表一致；删除需确认，删除后列表条目与磁盘文件同时消失；包含 `..`、绝对路径或不符合备份命名的名称被拒绝（400/404），且备份目录之外的文件不受影响。
- A8：未登录管理员访问备份列表、创建、下载、删除均返回 401 `auth_required`；携带跨站 `Origin` 的创建/删除请求返回 403。
- A9：存储驱动为 `cos` 时，备份包包含桶内全部对象（分页列出并逐对象下载），`manifest.json` 的 `storage_driver` 为 `cos`；任一对象读取失败时整次备份失败且不生成备份文件。
- A10：`cd server && go build ./... && go test ./...` 通过；`cd web && npx vue-tsc --noEmit -p tsconfig.app.json && npx vite build` 通过。
- A11：`.env.example` 与 docker-compose 提供 `BACKUP_DIR` 说明和默认值；README 记录默认备份目录、包内内容、排除项与手工恢复步骤，按文档可完成数据库与上传文件的恢复。

## Constraints and invariants

备份目录必须位于持久卷上：默认 `{DATA_DIR}/backups`，容器内即 `/data/backups`，随现有 `./data:/data` 挂载保留。数据库快照必须通过 SQLite 在线快照能力（`VACUUM INTO`）生成，禁止直接复制 `.db`、`-wal`、`-shm`。同一时刻只允许一个备份任务；备份在后台 goroutine 中执行，HTTP 请求返回后继续运行，进程重启会中断进行中的任务并清理临时文件。存储对象路径必须复用现有 key 语义：本地 `STORAGE_DIR` 相对路径、COS 对象 key；包内统一放在 `uploads/` 前缀下，禁止目录穿越。本地文件枚举跳过 `.upload-*` 上传临时文件、`.DS_Store` 与非普通文件；`.secret` 作为存储根文件正常收录。备份包包含密码哈希、站点口令哈希与全部私人照片，接口仅管理员可访问，日志不得输出包内容或敏感配置。备份生成失败时不得发布半成品：先写入 `.tmp` 文件，成功后原子重命名，失败与启动时清理。COS 模式必须处理分页与空桶；空桶时备份仍成功，清单文件数量为 0。

## Decisions

生成方式采用异步任务加前端轮询状态：大备份或 COS 下载可能持续数分钟，同步请求会撞上 HTTP 写超时；异步也让用户离开页面后任务继续。留存位置与策略为服务端 `{BACKUP_DIR}` 留存、手动删除，不自动保留最近 N 份，由用户在页面上管理。包格式为 zip：跨平台可直接解压查看，媒体文件本身已压缩，按存储方式写入以节省 CPU。对象来源抽象只依赖「列出对象 + 打开读取流」两个能力，本地磁盘与 COS 各实现一份，未启用存储时为空实现，便于用假对象源覆盖异常路径。清单 `manifest.json` 记录 schema、创建时间、运行环境、存储驱动、数据库统计、对象列表与排除项，供人工核对与后续恢复工具使用。恢复本次只写 README 手工步骤（停服务 → 解压覆盖数据库与上传目录 → 启动），不做页面导入。

## Open questions

（无）

## Verification expectations

Go 门禁为 `cd server && go build ./... && go test ./...`，新增单测覆盖 `store.SnapshotTo`、备份服务打包/清单/失败清理/并发/路径校验，以及 COS 对象分页列出的桩测试。前端门禁为 `cd web && npx vue-tsc --noEmit -p tsconfig.app.json && npx vite build`；不使用 `npm run build`，避免 `vue-tsc -b` 改写被 Git 跟踪的 `tsconfig.app.tsbuildinfo`。端到端使用本地存储后端与临时数据目录，通过 API 创建备份、轮询完成、下载 zip、解开核对数据库与文件、删除并确认磁盘清理，同时覆盖未登录 401、跨站 Origin 403 与非法备份名。浏览器手测登录管理端进入「备份」页，验证生成中状态、完成刷新、下载文件名、删除确认与错误提示。
