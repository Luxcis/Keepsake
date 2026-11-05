# 目标

前台把「旅行足迹」统一改称「足迹」；管理端可以单独上传 mp4/mov 视频（不再要求同名照片配对），视频作为独立媒体条目与照片一起在旅行详情中混排浏览并在灯箱播放；视频对象上传上限从 30MB 放宽到 400MB，图片上限保持 30MB。

# 范围

- 前台文案：桌面主导航、`/trips` 页面主标题、路由标题、旅行详情返回链接统一显示「足迹」。
- 管理端上传：未被同名图片配对的 mp4/mov 视频上传为独立视频条目；同名配对与合并文件仍识别为实况照片。
- 服务端：照片记录区分照片与视频媒体；视频对象按 400MB 校验，图片仍 30MB；本地存储单对象上限、nginx 请求体上限与本地存储模式的 Go 读写超时同步放宽。
- 前台展示：旅行详情网格中视频与照片按同一排序混排，视频卡片显示播放标记；点击后灯箱内播放，支持 Esc 关闭与左右方向键切换。
- 删除：视频条目删除时同步清理其存储对象，失败对象沿用补偿任务重试。
- 封面：视频条目不作为旅行封面候选，封面缩略图签名跳过视频条目。

## 目录结构

### 新建

- server/internal/store/migrations/0006_photo_media_kind.sql — 为 photos 增加 media_kind 列（photo/video），历史记录默认 photo。
- server/internal/server/media_video_test.go — 覆盖视频签名与确认上限、独立视频入库与列表输出、删除清理；沿用现有集成测试脚手架。

### 修改

- server/internal/config/config.go — 新增视频上限常量（400MB），保留图片上限 30MB 并明确注释。
- server/internal/handlers/photo.go — 签名与确认按媒体类型选择上限；确认接口接受视频主对象并写入 media_kind；列表输出 media_kind 与视频签名地址。
- server/internal/store/queries_photo.go — Photo/PhotoInput 增加 MediaKind，插入与查询携带该列。
- server/cmd/server/main.go — 本地存储单对象上限取媒体上限；本地存储模式读写超时覆盖 400MB 上传。
- deploy/nginx/default.conf — 调整 client_max_body_size 与代读超时以覆盖 400MB 视频直传。
- web/src/lib/livePhoto.ts — 未配对视频生成独立视频上传计划；增加图片/视频大小上限的前置校验。
- web/src/views/admin/AdminPhotosView.vue — 上传提示与任务徽标支持视频，视频条目使用视频预览卡片。
- web/src/views/admin/AdminTripsView.vue — 封面选择排除视频条目。
- web/src/views/TripDetailView.vue — 视频条目渲染为可点击的视频卡片并进入灯箱。
- web/src/components/PhotoLightbox.vue — 灯箱支持视频播放与跨照片/视频切换。
- web/src/components/AppHeader.vue、web/src/views/TripsView.vue、web/src/router/index.ts — 「旅行足迹」文案改为「足迹」。
- web/src/api/client.ts — PhotoItem/LightboxPhoto 增加媒体类型字段。
- README.md — 路由说明同步为「足迹」。

### 明确不建

- 服务端视频转码与海报生成 — 不引入 FFmpeg，浏览器直接播放原视频，网格显示视频首帧。
- 独立视频表或独立视频接口 — 复用 photos 记录与现有上传签名、确认、删除和补偿链路。
- 视频分片上传 — 400MB 以内沿用单次 PUT 直传。

# 非目标

- 不改管理端「旅行管理」「相册与照片」等既有命名，后台不使用「足迹」文案。
- 不放宽图片上限（仍 30MB），不改变实况照片的成对/合并识别规则与视频失败降级行为。
- 不改变旅行、时光轴、生日信的数据结构或排序。
- 不支持 mp4/mov 之外的视频容器（如 webm、avi），也不做历史对象回溯或转码。
- 不新增前台独立视频页或视频收藏入口。

# 约束与不变量

- 视频白名单仍为 `video/mp4` 与 `video/quicktime`；图片白名单仍为 jpeg/png/webp/heic/heif。
- 上传对象 Key 必须由服务端生成且位于 `trips/{tripID}/` 前缀；确认阶段必须 HEAD 校验对象存在、大小与内容类型。
- 存储后端未配置时，上传接口保持现有 503 `cos_not_configured` 语义与前台提示。
- 前台门禁、空态、错误重试、滚动动效与灯箱键盘交互保持现有行为。
- 视频条目与照片条目共用现有排序（`sort_order`、`created_at`）与删除补偿语义。

# 关键决定

- 视频复用 photos 表：新增 `media_kind` 列（`photo`/`video`），视频记录的 `cos_key` 指向视频对象，不新增表、接口或权限链路。
- 公共照片列表新增 `media_kind` 字段；视频条目的 `src`、`preview_src`、`thumb_src` 为空，`video_src` 为签名后的视频地址。
- 视频条目不参与旅行封面选择，`fillCoverThumb` 遇到视频条目时保持封面缩略图为空。
- 400MB 上限同时适用于独立视频与实况照片视频；图片仍 30MB。前端、服务端、本地存储、nginx 与 Go 超时同步调整。
- 视频网格不生成服务端海报：浏览器以 `preload="metadata"` 展示首帧并叠加播放标记，灯箱使用带控件的原生视频播放。

# 验收示例

1. 前台不再出现「旅行足迹」：桌面主导航项、`/trips` 的路由标题与页面主标题、旅行详情返回链接均显示「足迹」，页面主标题为「我们的足迹」。
2. 管理端只选择一个 mp4/mov 视频（没有同名照片）时，视频完成签名与直传并创建一条视频媒体记录，上传任务区显示该视频的进度与完成状态，不再出现「视频不会单独上传」的跳过提示。
3. 公开旅行详情中，视频条目与照片按接口返回顺序混排在同一网格；视频卡片显示播放标记，点击后在灯箱内播放，Esc 可关闭，左右方向键可切换到相邻照片或视频。
4. 视频对象上限为 400MB、图片上限保持 30MB：不超过 400MB 的 mp4/mov 可完成签名与确认入库；超过 400MB 的视频在签名接口返回 400，前端在上传前跳过并提示；超过 30MB 的图片仍被签名接口拒绝。
5. 实况照片关联的视频同样按 400MB 校验，不超过 400MB 的 `video_key` 可以确认入库；成对/合并识别与视频失败降级为普通照片的行为保持不变。
6. 删除视频媒体条目时，接口返回前同步删除该视频对象，删除失败的对象进入补偿任务队列。

# 验证要求

- 服务端：`go build ./...` 与 `go test ./...` 通过；新增测试覆盖视频签名上限、超限拒绝、独立视频确认入库、列表 `media_kind` 输出、图片 30MB 拒绝与删除清理。
- 前端：在 `web/` 执行 `npx vue-tsc --noEmit -p tsconfig.app.json` 与 `npx vite build` 通过；不使用 `npm run build`，避免改写被跟踪的 `tsconfig.app.tsbuildinfo`。
- 文案：前台源码中不再有用户可见的「旅行足迹」文案。
