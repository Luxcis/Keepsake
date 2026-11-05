# 目标

后台管理端上传照片时，如果用户一次选择了「照片 + 同名视频」或「单个内嵌视频的合并实况文件」，系统自动识别为一张实况照片：在浏览器中拆分/配对出静态图与视频，分别上传并在入库时关联；前台照片墙卡片与灯箱可以直接播放实况视频。没有视频的普通照片保持现有行为完全不变。

# 范围

- 只覆盖管理端上传链路与公开照片展示：`AdminPhotosView`（上传）、`PhotoWallView` + `PhotoLightbox`（展示）。
- 解析在浏览器完成，服务端只接收拆分后的独立图片/视频对象，不新增 Go 媒体依赖。
- 新增前端运行时依赖 `live-photo@^0.2.0`，使用其 `LivePhotoViewer` 播放、`extractFromLivePhoto` 拆分合并文件。
- 服务端扩展：`photos.video_key` 可空列、视频类型上传签名、确认阶段校验、`video_src` 签名输出、删除时补偿清理视频对象。
- 单对象大小上限沿用现有 30MB（`config.MaxUploadBytes`），不新增转码或压缩。

## 目录结构

### 新建

- web/src/lib/livePhoto.ts — 实况图输入分组、同名配对、合并文件拆分、JPEG/HEIC 与 MP4/MOV 类型嗅探（纯 TS，可独立验证）。
- web/src/components/LivePhotoMedia.vue — 封装 `LivePhotoViewer` 的挂载/销毁、尺寸与 object-fit 适配，供照片墙卡片与灯箱复用。
- server/internal/store/migrations/0004_photo_video_key.sql — `photos` 表新增可空 `video_key` 列。

### 修改

- web/package.json、web/package-lock.json — 添加 `live-photo@^0.2.0` 运行时依赖。
- web/src/views/admin/AdminPhotosView.vue — 选择文件后先分组解析；实况照片上传两个对象并按字节汇总进度；实况标记、视频失败降级与未配对视频提示。
- web/src/views/PhotoWallView.vue — 有视频的照片用实况组件渲染卡片（懒加载视频、悬停/长按播放、LIVE 徽标），其余保持缩略图。
- web/src/components/PhotoLightbox.vue — 有视频的当前照片改用实况组件并自动播放；切换/关闭时销毁播放器，无视频时保持现有 `<img>` 加载态。
- web/src/api/client.ts — `PhotoItem` 增加 `video_src`，`LightboxPhoto` 增加 `videoSrc`，确认请求类型允许 `video_key`。
- server/internal/handlers/photo.go — 上传签名接受视频类型；确认接口接收并校验 `video_key`（前缀/类型/大小）；照片列表输出签名后的 `video_src`；删除时返回视频 key。
- server/internal/store/queries_photo.go — `Photo`/`PhotoInput` 增加 `VideoKey`，插入、查询、删除同步该列。
- server/internal/localstore/localstore.go — `.mp4`/`.mov` 的 Content-Type 映射，保证本地存储 HEAD/GET 与视频类型一致。
- server/internal/server/router_test.go — 覆盖实况签名、确认、列表输出与删除补偿的端到端测试。

### 明确不建

- 服务端媒体解析/转码模块 — 解析依赖浏览器与 `live-photo`，服务端只做类型与归属校验，不引入 Go 解码库。
- 前端测试运行器（vitest 等） — 仓库当前没有前端测试框架，本 change 以 `vue-tsc` 类型检查、`vite build` 与浏览器端到端手测作为验证手段。
- 独立的实况元数据表/布尔字段 — 单个可空 `video_key` 足以表达“是否有实况视频”，避免冗余状态不一致。

# 非目标

- 视频转码、压缩、封面截帧，以及 HEIC → JPEG 转码。
- 为已上传的普通照片事后补挂实况视频。
- 时间轴、首页、信纸等其他页面的实况展示。
- 在服务端解析合并实况文件。
- 实况视频的编辑、裁剪或作为旅行封面。

# 验收示例

- A1：在管理端同一批选择同名的图像文件与视频文件（如 `IMG_0001.HEIC` + `IMG_0001.MOV`）上传后，该旅行照片列表只新增 1 条照片记录，记录带实况视频，前台卡片与灯箱均可播放。
- A2：在管理端选择一个内嵌视频的合并实况文件（JPEG+MP4 或 HEIC+MOV 合并）上传后，系统自动拆出静态图与视频两个对象，且只新增 1 条照片记录。
- A3：选择普通 JPEG/PNG/WebP 图片（无同名视频、无可拆分视频）上传时，行为与改动前一致：1 个对象、1 条记录、无实况标识。
- A4：上传单个未合并的 HEIC 图片时不会被误判为实况，仍按普通图片保存。
- A5：实况视频对象上传失败但图片上传成功时，照片仍按普通照片入库，上传任务区显示实况视频失败原因，不丢失图片。
- A6：确认请求里的 `video_key` 不属于该旅行前缀、对象不存在、内容类型不在视频白名单时，接口返回 400 且不创建照片记录。
- A7：删除一张实况照片后，图片对象与视频对象都各有一条删除补偿任务（`cos_delete_tasks`）。
- A8：前台照片墙中带视频的照片显示 LIVE 标识并在桌面悬停/触屏长按时播放，灯箱打开后自动播放；无视频照片仍显示静态图且无实况交互。
- A9：`cd server && go build ./... && go test ./...` 与 `cd web && npm run build` 均通过。

## Constraints and invariants

存储链路保持现状：`POST /api/admin/uploads/sign` 签发、浏览器 PUT 直传、`POST /api/admin/photos/confirm` HEAD 校验后入库；单对象上限沿用 30MB。`video_key` 必须与 `cos_key` 同属 `trips/{tripID}/` 前缀，且只能以视频白名单（`video/mp4`、`video/quicktime`）出现。合并文件只有在拆分结果同时通过图片魔数（JPEG SOI 或 HEIC/HEIF `ftyp`）与视频结构（`ftyp` + `moov`/`mdat`）校验时才按实况处理，否则按普通图片上传。

解析只发生在浏览器，服务端不下载、不读取对象内容做识别。删除照片必须同时把 `video_key` 入队清理；无视频时行为不变。无视频照片的请求、响应与页面渲染保持现有行为，不改动既有字段语义。同一批次中未配对的视频不单独上传，只在上传任务区提示“未找到配对照片”。HEIC 静帧与 HEVC 视频的浏览器兼容性沿用系统能力；Chrome 可能无法显示 HEIC 或解码 HEVC，本 change 不提供转码兜底。

## Decisions

依赖固定为 `live-photo@^0.2.0`（npm 当前 latest，含 viewer 与 extract 导出）。实况状态由 `video_src` 是否存在派生，不新增数据库布尔字段。上传顺序固定为「先图片、后视频」；视频失败不阻断照片入库，只降级为普通照片。

播放策略：照片墙卡片 `lazyLoadVideo: true`、悬停/长按播放；灯箱 `autoplay: true` + 静音（库默认）避免自动播放被浏览器拦截。灯箱中通过 `imageCustomization`/`videoCustomization` 覆盖 `object-fit: contain`，卡片保持库默认 `cover`。

## Open questions

（无）

## Verification expectations

Go 侧以 `go build ./... && go test ./...` 为门禁，新增测试使用本地存储后端走完整签名→PUT→confirm→列表→删除链路。前端以 `npm run build`（`vue-tsc -b` 类型检查 + Vite 构建）为门禁。实现完成后用 ffmpeg 合成 JPEG+MP4 合并文件与 HEIC+MOV 成对文件，验证上传后列表、卡片、灯箱与删除补偿。
