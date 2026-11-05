---
generated_from_state_version: 8
---

# 验证

## 当前结果

- 结果: **已归档**
- 验证情况: **已完成检查，验证结果已确认**
- 目标周期: 1
- 迭代: 1
- 验证器尝试次数: 1
- 完成时间: 2026-11-05T08:19:40.044Z
- 摘要: 独立核验通过：前台文案统一为「足迹/我们的足迹」，未配对 mp4/mov 以视频为主对象经签名、直传、确认写入 media_kind=video 并在任务区显示进度；旅行详情按接口顺序混排照片与视频，视频卡片带播放标记并可在灯箱原生播放、Esc/方向键切换；400MB/30MB 在签名与确认两处按类型生效，前端前置拦截；实况 video_key 同步放宽到 400MB 且配对/合并/降级逻辑保持不变；删除视频条目同步清理对象并在失败时入补偿队列。独立重跑 11 个后端用例全部 PASS，vue-tsc --noEmit 退出码 0，工作区未被验证命令改写。

## 验收

| 编号 | 结果 | 来源 | 验收项 | 原因 |
| --- | --- | --- | --- | --- |
| A1 | passed | brief.md | 前台不再出现「旅行足迹」：桌面主导航项、`/trips` 的路由标题与页面主标题、旅行详情返回链接均显示「足迹」，页面主标题为「我们的足迹」。 | 桌面与移动端导航均为「足迹」(web/src/components/AppHeader.vue:23,38)；/trips 路由标题「足迹」(web/src/router/index.ts:7)；页面主标题「我们的足迹」(web/src/views/TripsView.vue:59)；详情返回链接「← 返回足迹」(web/src/views/TripDetailView.vue:105)。grep -rn 旅行足迹 web/src 无结果（仅 docs/demo.html 与历史/规格文档残留，非前台应用产物）。 |
| A2 | passed | brief.md | 管理端只选择一个 mp4/mov 视频（没有同名照片）时，视频完成签名与直传并创建一条视频媒体记录，上传任务区显示该视频的进度与完成状态，不再出现「视频不会单独上传」的跳过提示。 | 未配对视频现在生成 kind='video' 上传计划(web/src/lib/livePhoto.ts:275-282)，任务区展示「视频」徽标与进度/完成(web/src/views/admin/AdminPhotosView.vue:257-279)；上传以视频对象本身作为 cos_key 签名直传并确认(AdminPhotosView.vue:122-127,151-159,161-168)，服务端确认按 HEAD 视频类型写入 media_kind=video(server/internal/handlers/photo.go:160-168,221-225)。旧「视频不会单独上传/未找到同名照片」文案 grep 无结果。独立执行 go test -run TestStandaloneVideoUploadFlow 通过，实测签名→PUT→confirm→列表 media_kind=video→video_src 可读取。 |
| A3 | passed | brief.md | 公开旅行详情中，视频条目与照片按接口返回顺序混排在同一网格；视频卡片显示播放标记，点击后在灯箱内播放，Esc 可关闭，左右方向键可切换到相邻照片或视频。 | 网格直接按 photos 数组（接口 sort_order, created_at 顺序）v-for 混排(web/src/views/TripDetailView.vue:167-212)；视频卡片用 video 元素并以 ▶ 显示播放标记(:174-181,:201-206)；viewable 将 video_src 映射为 mediaKind='video' 进入灯箱(:77-89)；灯箱渲染原生 video(controls/autoplay/muted)(web/src/components/PhotoLightbox.vue:167-178)，Esc 关闭与 ArrowLeft/ArrowRight 在 props.photos（照片与视频混合数组）上循环切换(:101-120)，视频因 :key 切换/关闭而卸载停止播放。npx vue-tsc --noEmit -p tsconfig.app.json 独立执行退出码 0。 |
| A4 | passed | brief.md | 视频对象上限为 400MB、图片上限保持 30MB：不超过 400MB 的 mp4/mov 可完成签名与确认入库；超过 400MB 的视频在签名接口返回 400，前端在上传前跳过并提示；超过 30MB 的图片仍被签名接口拒绝。 | 上限常量为视频 400<<20、图片 30<<20(server/internal/config/config.go:15-18)；签名接口按 content_type 选择上限并在 max_bytes 返回对应值(photo.go:46-53,91-96,105-111)；确认阶段按 HEAD 内容类型分别校验 400MB/30MB(photo.go:160-178)，实况 video_key 亦用 400MB(photo.go:200-203)；本地存储单对象放行到 400MB(server/cmd/server/main.go:164-170)、nginx client_max_body_size 410m 与 900s 超时(deploy/nginx/default.conf:14-20,52-60)；前端上传前跳过 >400MB 视频与 >30MB 图片并给出原因(web/src/lib/livePhoto.ts:211-231,255-271)。独立重跑 TestVideoSignLimits（400MB 可签名、400MB+1 拒绝、图片 30MB+1 拒绝）与 TestConfirmObjectSizeByMediaKind（31MB 视频入库、31MB 图片与 400MB+1 视频拒绝）均 PASS。 |
| A5 | passed | brief.md | 实况照片关联的视频同样按 400MB 校验，不超过 400MB 的 `video_key` 可以确认入库；成对/合并识别与视频失败降级为普通照片的行为保持不变。 | 确认阶段 video_key 校验改为 MaxVideoUploadBytes(server/internal/handlers/photo.go:195-207)，并保留前缀、存在性、类型白名单校验；成对/合并识别逻辑未被改写，仅新增大小上限拦截(web/src/lib/livePhoto.ts:233-273)，视频失败降级仍为 confirm 不带 video_key 的普通照片(web/src/views/admin/AdminPhotosView.vue:137-148)。独立重跑 TestLivePhotoVideoSizeLimit（31MB video_key 入库、400MB+1 拒绝）、TestLivePhotoUploadFlow、TestQuickTimeVideoConfirmed、TestVideoKeyValidation、TestPlainPhotoKeepsSingleObject 均 PASS。 |
| A6 | passed | brief.md | 删除视频媒体条目时，接口返回前同步删除该视频对象，删除失败的对象进入补偿任务队列。 | 删除接口在返回前对 cos_key/thumbnail_key/video_key 去重后同步调用存储删除，失败对象逐个 EnqueueDeleteTask(server/internal/handlers/photo.go:468-489)；视频条目 cos_key 即视频对象、video_key 为空(store/queries_photo.go:12-27,161-179)。独立重跑 TestStandaloneVideoUploadFlow：DELETE 返回 200 后视频对象已不存在且 cos_delete_tasks 无待处理项；TestDeletePhotoEnqueuesWhenStorageDeleteFails 验证同步删除失败时以相同处理分支写入补偿任务（该用例使用图片 key，视频主对象复用同一循环）。 |

## 检查

| 检查 | 命令 | 工作目录 | 状态 | 退出码 | 耗时 |
| --- | --- | --- | --- | ---: | ---: |
| Go 构建 | build ./... | server | passed | 0 | 1383 ms |
| Go 全量测试 | test -count=1 ./... | server | passed | 0 | 2465 ms |
| 前端类型检查 | vue-tsc --noEmit -p [REDACTED] | web | passed | 0 | 1378 ms |
| 前端构建 | vite build | web | passed | 0 | 658 ms |

### Builder 报告的证据

以下为 Builder 报告，不等同于 Runtime 检查凭据或独立验收结果。

- go build ./...: passed — server/ 下执行，退出码 0
- go vet ./...: passed — server/ 下执行，退出码 0
- go test -count=1 ./...: passed — server/ 下全部包通过，含新增 media_video_test.go
- npx vue-tsc --noEmit -p tsconfig.app.json: passed — web/ 下执行，退出码 0，未改写 tsbuildinfo
- npx vite build: passed — web/ 下执行，退出码 0，仅写入被忽略的 dist/
- 已知限制: 视频不做服务端转码或海报生成，网格依赖浏览器 preload=metadata 显示首帧
- 已知限制: 400MB 上限在本地存储模式依赖 nginx client_max_body_size 410m 与 900s 超时；COS 模式仍受 Bucket 侧策略约束
- 已知限制: 本地存储模式 HTTP 读写超时提高到 15 分钟，慢速上行仍可能超时
- 已知限制: 视频条目不作为旅行封面，封面选择列表只列照片
- 已知限制: 历史 photos 记录经迁移默认 media_kind=photo，无需回填

## 阻塞项

_无。_

## 风险与跳过的工作

- 前端无自动化测试（web 下无 *.test.ts/*.spec.ts）：独立视频上传与灯箱视频交互仅由源码审查 + vue-tsc 类型检查覆盖，未做浏览器端到端验证。
- 网格视频首帧依赖浏览器 preload="metadata"，部分浏览器可能先显示黑帧/空白直至解码（brief 已知限制：无服务端转码与海报生成）。
- 400MB 本地存储模式依赖 nginx client_max_body_size 410m、900s 代理超时与 Go 15 分钟读写超时；COS 模式仍受 Bucket 侧策略约束，未在本环境验证。
- A6 的删除失败补偿用例使用图片 key 触发；视频主对象走同一 deletePhoto 循环但无独立失败注入测试，属间接覆盖。
- docs/demo.html 与 docs/comet/specs/timeline/spec.md 仍含「旅行足迹」字样，属设计稿/内部规格文档，不在前台应用产物内；web/dist 构建产物已无该文案。

## 之前的迭代

| 目标周期 | 迭代 | 尝试 | 结果 | 未解决项 | 摘要 | 完成时间 |
| ---: | ---: | ---: | --- | --- | --- | --- |
| 1 | 1 | 1 | pass | — | 独立核验通过：前台文案统一为「足迹/我们的足迹」，未配对 mp4/mov 以视频为主对象经签名、直传、确认写入 media_kind=video 并在任务区显示进度；旅行详情按接口顺序混排照片与视频，视频卡片带播放标记并可在灯箱原生播放、Esc/方向键切换；400MB/30MB 在签名与确认两处按类型生效，前端前置拦截；实况 video_key 同步放宽到 400MB 且配对/合并/降级逻辑保持不变；删除视频条目同步清理对象并在失败时入补偿队列。独立重跑 11 个后端用例全部 PASS，vue-tsc --noEmit 退出码 0，工作区未被验证命令改写。 | 2026-11-05T08:19:40.044Z |



## 结论

独立核验通过：前台文案统一为「足迹/我们的足迹」，未配对 mp4/mov 以视频为主对象经签名、直传、确认写入 media_kind=video 并在任务区显示进度；旅行详情按接口顺序混排照片与视频，视频卡片带播放标记并可在灯箱原生播放、Esc/方向键切换；400MB/30MB 在签名与确认两处按类型生效，前端前置拦截；实况 video_key 同步放宽到 400MB 且配对/合并/降级逻辑保持不变；删除视频条目同步清理对象并在失败时入补偿队列。独立重跑 11 个后端用例全部 PASS，vue-tsc --noEmit 退出码 0，工作区未被验证命令改写。
