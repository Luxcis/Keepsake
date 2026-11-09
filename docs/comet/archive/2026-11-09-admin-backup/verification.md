---
generated_from_state_version: 10
---

# 验证

## 当前结果

- 结果: **已归档**
- 验证情况: **已完成检查，验证结果已确认**
- 目标周期: 1
- 迭代: 1
- 验证器尝试次数: 1
- 完成时间: 2026-11-09T03:37:03.453Z
- 摘要: 独立只读验收：阅读 brief/spec 与前后端实现，重跑 go build/test、vue-tsc 与 vite build，并以真实服务进程 + curl/Python 端到端验证创建、状态、下载、清单一致性、SQLite 快照、失败清理、409、鉴权/CSRF、非法名称与删除；另用浏览器实测备份页生成中禁用/进度、完成后列表与删除确认。A1–A11 全部通过，verdict=pass。

## 验收

| 编号 | 结果 | 来源 | 验收项 | 原因 |
| --- | --- | --- | --- | --- |
| A1 | passed | brief.md | A1：管理员在「备份」页点击「立即备份」，页面进入生成中状态并禁用按钮；生成完成后列表出现新条目，显示文件名（`keepsake-backup-YYYYMMDD-HHMMSS.zip`）、大小与创建时间，且 `{BACKUP_DIR}` 下存在同名文件。 | AdminBackupView.vue:94-111,146-170 实现启动/轮询/禁用；真实浏览器实测点击后按钮 disabled 且显示「正在备份…」与打包进度，完成后列表出现 keepsake-backup-20261109-113355.zip、2.0 GB 与创建时间，{BACKUP_DIR} 下存在同名文件；POST 返回 202。 |
| A2 | passed | brief.md | A2：下载并解压备份包，包含 `journey.db`、`manifest.json` 和 `uploads/` 下全部上传对象；用 SQLite 打开 `journey.db` 能看到备份前创建的旅行、照片与站点设置数据；包内不包含 `.env`、Redis 数据文件或备份目录自身。 | backup.go:441-497 打包 manifest.json、DB 快照与 uploads/<key>；localstore.go:191-231 遍历存储根全部普通文件并跳过临时/系统文件。真实进程下载解压后包含 journey.db、manifest.json、uploads/.secret 与全部上传对象；SQLite 只读打开可读到备份前旅行、照片与 site_title；包内无 .env、Redis 数据或 backups/ 路径。 |
| A3 | passed | brief.md | A3：`manifest.json` 的 schema、创建时间、环境、存储驱动、数据库字节数、文件数量与总字节和实际包内内容一致，并列出全部对象 key 与字节数。 | backup.go:447-463 生成 manifest；实测 schema=keepsake.backup.v1、app_env=dev、storage_driver=local，created_at 非空，database.bytes 与 zip 内 journey.db 字节一致，files.count/bytes 与 uploads 条目数量/大小一致，objects 的 key/bytes 与实际条目逐一匹配，excludes 已记录。 |
| A4 | passed | brief.md | A4：备份生成期间站点仍可正常写入（新增旅行或照片记录不报错）；备份包中的数据库是一致快照，单独打开即可读取，不需要 `-wal`/`-shm` 伴生文件。 | snapshot.go:16 使用 VACUUM INTO 生成一致性快照；真实进程备份进行中新增旅行返回 201，备份完成后快照可单独只读打开且无 -wal/-shm，包含备份前数据、不包含备份期间新增的旅行，验证了时间点一致性与写入不受阻塞。 |
| A5 | passed | brief.md | A5：模拟对象读取失败或写包失败时，创建接口/状态返回失败原因，列表中不出现该备份，备份目录不残留半成品或临时文件。 | backup.go:358-423 在 defer 中删除临时快照与临时 zip；真实进程将上传对象设为不可读后触发备份，状态接口返回 last.status=failed 且 error 含 permission denied，列表为空，备份目录无 .zip/.tmp 残留；TestFailureLeavesNoPartialBackup 通过。 |
| A6 | passed | brief.md | A6：已有备份正在生成时再次触发备份返回 409（页面按钮禁用），不会并行产生两份备份。 | backup.go:219-238 在 Start 内互斥占用 running；handler:25-35 将 ErrRunning 映射为 409 backup_running；页面 :disabled=busy\|\|running。真实进程在备份进行中第二次 POST 返回 409，未并行生成第二份备份，且期间站点写入成功。 |
| A7 | passed | brief.md | A7：备份列表按创建时间倒序；点击下载得到 `application/zip` 附件且文件名与列表一致；删除需确认，删除后列表条目与磁盘文件同时消失；包含 `..`、绝对路径或不符合备份命名的名称被拒绝（400/404），且备份目录之外的文件不受影响。 | backup.go:278-356 实现倒序列表、名称正则、Open/Delete 普通文件校验；handler:38-66 返回 application/zip 附件与 deleted。实测下载文件名与列表一致、列表按 created_at 倒序；浏览器实测删除弹出确认，确认后列表条目与磁盘文件同时消失；bad.zip、../、绝对路径、反斜杠名称均 400，备份目录外文件未受影响。 |
| A8 | passed | brief.md | A8：未登录管理员访问备份列表、创建、下载、删除均返回 401 `auth_required`；携带跨站 `Origin` 的创建/删除请求返回 403。 | api.go:69-73 备份路由位于 withAdmin 组，router.go:52 以 csrfSameOrigin 包裹；实测未登录访问列表/创建/下载/删除均 401 auth_required，携带跨站 Origin 的创建/删除均 403 csrf_rejected。 |
| A9 | passed | brief.md | A9：存储驱动为 `cos` 时，备份包包含桶内全部对象（分页列出并逐对象下载），`manifest.json` 的 `storage_driver` 为 `cos`；任一对象读取失败时整次备份失败且不生成备份文件。 | cosutil.go:104-128 按 IsTruncated/NextMarker 分页，缺 marker 报错；cosutil_test 的 TestListAllObjectsPaginates/TestListAllObjectsRejectsTruncatedWithoutMarker 与 backup_test 的 TestManifestRecordsCosDriver 均通过；backup.go:470-496 逐对象 OpenObject，任一读取失败使整次失败且 defer 清理不发布文件。未连接真实 COS 桶，按已知限制以桩测试覆盖。 |
| A10 | passed | brief.md | A10：`cd server && go build ./... && go test ./...` 通过；`cd web && npx vue-tsc --noEmit -p tsconfig.app.json && npx vite build` 通过。 | 实测 cd server && go build ./... 退出 0；go test ./... 通过，关键包以 -count=1 重跑通过；cd web && npx vue-tsc --noEmit -p tsconfig.app.json 退出 0；npx vite build 退出 0；git status 确认未改写 tsconfig.app.tsbuildinfo 等被跟踪文件。 |
| A11 | passed | brief.md | A11：`.env.example` 与 docker-compose 提供 `BACKUP_DIR` 说明和默认值；README 记录默认备份目录、包内内容、排除项与手工恢复步骤，按文档可完成数据库与上传文件的恢复。 | .env.example:58-61 说明默认 {DATA_DIR}/backups；docker-compose.yml:34-35 默认 /data/backups；README.md:161-186 记录默认目录、包结构、包含/排除项与停机恢复步骤；实际快照与 uploads 结构可按文档覆盖恢复。 |

## 检查

| 检查 | 命令 | 工作目录 | 状态 | 退出码 | 耗时 |
| --- | --- | --- | --- | ---: | ---: |
| go build ./... | build ./... | server | passed | 0 | 527 ms |
| go test ./... | test ./... | server | passed | 0 | 412 ms |
| vue-tsc typecheck | vue-tsc --noEmit -p [REDACTED] | web | passed | 0 | 1090 ms |
| vite build | vite build | web | passed | 0 | 485 ms |

### Builder 报告的证据

以下为 Builder 报告，不等同于 Runtime 检查凭据或独立验收结果。

- go build ./...: passed — server 目录执行通过
- go vet ./...: passed — server 目录执行通过
- go test ./...: passed — 全部包通过，包含新增 backup/store/cosutil/localstore/server 测试
- npx vue-tsc --noEmit -p tsconfig.app.json: passed — web 目录类型检查通过
- npx vite build: passed — web 生产构建通过
- 真实进程冒烟: passed — 临时数据目录启动 server，登录后创建/下载/解压/删除备份均成功，包内数据库可读出 admin 账号
- 已知限制: COS 模式未连接真实桶验证：分页列举与逐对象读取以桩测试覆盖，实际读取依赖部署凭据与桶权限。
- 已知限制: 备份包不加密，且不包含 .env、部署配置与 Redis 数据（已确认范围）。
- 已知限制: 进程重启会中断进行中的备份任务；残留临时文件在下次启动时清理。
- 已知限制: 未实现多实例分布式备份锁，单任务限制只作用于同一进程。

## 阻塞项

_无。_

## 风险与跳过的工作

- COS 模式仅以桩测试覆盖分页、驱动标记与读取失败语义，未连接真实桶验证凭据、网络与大对象读取。
- 备份包未加密，包含密码哈希、站点口令哈希与私人照片；安全性依赖管理员会话与持久卷访问控制（范围内决策）。
- 单任务互斥只在同一进程内生效，多实例部署可能并行；进程重启会中断进行中任务（已知限制）。

## 之前的迭代

| 目标周期 | 迭代 | 尝试 | 结果 | 未解决项 | 摘要 | 完成时间 |
| ---: | ---: | ---: | --- | --- | --- | --- |
| 1 | 1 | 1 | pass | — | 独立只读验收：阅读 brief/spec 与前后端实现，重跑 go build/test、vue-tsc 与 vite build，并以真实服务进程 + curl/Python 端到端验证创建、状态、下载、清单一致性、SQLite 快照、失败清理、409、鉴权/CSRF、非法名称与删除；另用浏览器实测备份页生成中禁用/进度、完成后列表与删除确认。A1–A11 全部通过，verdict=pass。 | 2026-11-09T03:37:03.453Z |



## 结论

独立只读验收：阅读 brief/spec 与前后端实现，重跑 go build/test、vue-tsc 与 vite build，并以真实服务进程 + curl/Python 端到端验证创建、状态、下载、清单一致性、SQLite 快照、失败清理、409、鉴权/CSRF、非法名称与删除；另用浏览器实测备份页生成中禁用/进度、完成后列表与删除确认。A1–A11 全部通过，verdict=pass。
