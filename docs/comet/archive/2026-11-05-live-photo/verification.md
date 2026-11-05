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
- 完成时间: 2026-11-05T05:40:10.883Z
- 摘要: 独立只读验收通过 A1-A9：同名配对、合并拆分、普通图片/未合并 HEIC 回退、视频失败降级、video_key 校验、删除补偿、前台播放组件接线与构建测试均核对通过。go build/go test/npm run build 本次独立运行成功。遗留风险主要是未在真实浏览器重放播放行为、无视频响应新增空 video_src 字段、卡片徽标点击冒泡与合并文件启发式覆盖面。

## 验收

| 编号 | 结果 | 来源 | 验收项 | 原因 |
| --- | --- | --- | --- | --- |
| A1 | passed | brief.md | A1：在管理端同一批选择同名的图像文件与视频文件（如 `IMG_0001.HEIC` + `IMG_0001.MOV`）上传后，该旅行照片列表只新增 1 条照片记录，记录带实况视频，前台卡片与灯箱均可播放。 | web/src/lib/livePhoto.ts buildUploadPlans 按去扩展名同名（大小写不敏感）配对；独立临时 Node 脚本（/tmp/lpverify，编译自该 TS 文件）实测 IMG_0001.jpg+IMG_0001.mov 仅生成 1 个 live 计划，未配对 IMG_9999.mp4 为 skipped。web/src/views/admin/AdminPhotosView.vue uploadOne 先传图片再传视频，成功后 confirmPhoto 携带 video_key。server/internal/server/live_photo_test.go TestLivePhotoUploadFlow 本次独立运行 PASS：仅 1 条记录、video_src 非空且签名视频 GET 200；PhotoWallView.vue/PhotoLightbox.vue 在 video_src 存在时渲染 LivePhotoMedia（卡片/灯箱）。 |
| A2 | passed | brief.md | A2：在管理端选择一个内嵌视频的合并实况文件（JPEG+MP4 或 HEIC+MOV 合并）上传后，系统自动拆出静态图与视频两个对象，且只新增 1 条照片记录。 | web/src/lib/livePhoto.ts splitMergedLivePhoto 调用 live-photo extractFromLivePhoto 并对图片魔数与视频 ftyp+moov/mdat 二次校验。独立临时 Node 脚本实测合成的 JPEG+MP4 合并文件与 HEIC+MOV 合并文件均返回 1 个 live 计划（photo=image/jpeg 或 image/heic，video=video/mp4 或 video/quicktime），各只新增 1 条上传计划；上传/入库见 A1 的 confirm 链路。 |
| A3 | passed | brief.md | A3：选择普通 JPEG/PNG/WebP 图片（无同名视频、无可拆分视频）上传时，行为与改动前一致：1 个对象、1 条记录、无实况标识。 | 独立临时 Node 脚本实测普通 jpg/png/webp 均只生成 1 个 photo 计划（无 video 部分）；server/internal/server/live_photo_test.go TestPlainPhotoKeepsSingleObject 本次独立运行 PASS，video_src 为空且删除补偿仅 1 条；web/src/views/PhotoWallView.vue 仅在 p.video_src 为真时用实况组件，否则仍渲染 img。 |
| A4 | passed | brief.md | A4：上传单个未合并的 HEIC 图片时不会被误判为实况，仍按普通图片保存。 | web/src/lib/livePhoto.ts splitMergedLivePhoto 要求拆分后的图片部分能通过 detectImageType 且长度 >=16；独立临时 Node 脚本用 ftyp=heic、含 mdat 的未合并 HEIC 实测返回 null，buildUploadPlans 输出普通 photo 计划（不产生 video 上传计划）。 |
| A5 | passed | brief.md | A5：实况视频对象上传失败但图片上传成功时，照片仍按普通照片入库，上传任务区显示实况视频失败原因，不丢失图片。 | web/src/views/admin/AdminPhotosView.vue uploadOne 在图片 PUT 成功后 try/catch 视频 uploadObject；异常时调用 confirmPhoto(plan.name, photoKey)（不带 video_key）并返回 '实况视频上传失败，已按普通照片保存：<reason>'，onFilesChosen 把它写入 job.warning，模板在任务区优先展示 j.warning。代码路径明确保证图片已入库且失败原因可见。 |
| A6 | passed | brief.md | A6：确认请求里的 `video_key` 不属于该旅行前缀、对象不存在、内容类型不在视频白名单时，接口返回 400 且不创建照片记录。 | server/internal/handlers/photo.go confirmPhoto 对 video_key 依次校验 trips/{tripID}/ 前缀、h.cos.Head 存在性与 <=30MB、allowedVideoTypes（video/mp4、video/quicktime），全部在 CreatePhoto 之前。本次独立运行 TestVideoKeyValidation 的跨旅行前缀、图片对象冒充视频、对象不存在三个子测试均 PASS，且随后 listAdminPhotos 为 0；text/plain 签名 400、video/quicktime 签名成功也已验证。 |
| A7 | passed | brief.md | A7：删除一张实况照片后，图片对象与视频对象都各有一条删除补偿任务（`cos_delete_tasks`）。 | server/internal/store/queries_photo.go DeletePhoto 返回 cosKey/thumbKey/videoKey；server/internal/handlers/photo.go deletePhoto 对三个非空 key 逐个 EnqueueDeleteTask。本次独立运行 TestLivePhotoUploadFlow 在 DELETE 后断言 pending cos_delete_tasks=2（图片+视频）PASS；TestPlainPhotoKeepsSingleObject 断言普通照片仅 1 条 PENDING PASS。 |
| A8 | passed | brief.md | A8：前台照片墙中带视频的照片显示 LIVE 标识并在桌面悬停/触屏长按时播放，灯箱打开后自动播放；无视频照片仍显示静态图且无实况交互。 | web/src/views/PhotoWallView.vue 对 p.video_src 为真的卡片渲染 LivePhotoMedia(mode=card)，无视频仍走 img 分支；web/src/components/LivePhotoMedia.vue 卡片模式 autoplay=false、lazyLoadVideo=true，并在 host mouseenter 调用 viewer.play()/mouseleave 调用 stop()（canHover 限定桌面），触屏由 live-photo 库 overlay touchstart/touchend 长按逻辑处理；灯箱 mode=lightbox 传 autoplay=true，并设置 image/video object-fit contain。live-photo@0.2.0 库源码确认默认创建 LIVE 徽标（createBadge labels.live）并在 photo load 后按 autoplay 播放。无视频照片不创建播放器，保持静态。 |
| A9 | passed | brief.md | A9：`cd server && go build ./... && go test ./...` 与 `cd web && npm run build` 均通过。 | 本次独立执行：cd server && go build ./... 输出 GO_BUILD_OK；go test ./... 全部 ok（含 cmd/server、auth、cosutil、localstore、server）；定向 go test ./internal/server -run 'TestLivePhotoUploadFlow\|TestPlainPhotoKeepsSingleObject\|TestVideoKeyValidation\|TestQuickTimeVideoConfirmed' -v 四项全 PASS；cd web && npm run build 输出 vue-tsc -b && vite build，79 modules transformed，built in 193ms 成功。 |

## 检查

| 检查 | 命令 | 工作目录 | 状态 | 退出码 | 耗时 |
| --- | --- | --- | --- | ---: | ---: |
| go build ./... | build ./... | server | passed | 0 | 536 ms |
| go test ./... | test ./... | server | passed | 0 | 244 ms |
| npm run build | run build | web | passed | 0 | 1614 ms |

### Builder 报告的证据

以下为 Builder 报告，不等同于 Runtime 检查凭据或独立验收结果。

- go build ./...: passed — 工作区 server/，go1.26.4
- go test ./...: passed — 新增 TestLivePhotoUploadFlow / TestPlainPhotoKeepsSingleObject / TestVideoKeyValidation / TestQuickTimeVideoConfirmed
- npm run build: passed — 工作区 web/，vue-tsc -b + vite build
- 实况解析脚本校验: passed — 临时 Node 脚本覆盖配对、合并 JPEG/MP4、合并 HEIC/MOV、单 HEIC 不误判、未配对视频跳过
- 本地 E2E 上传/列表/删除: passed — local 存储后端 + Vite 代理，3 张照片（2 实况 1 普通），删除实况后 2 条补偿任务
- 浏览器 DOM/交互检查: passed — LIVE 徽标、卡片视频元素、悬停播放、灯箱实况组件与普通照片 img 分支均已核对
- 已知限制: 嵌入式浏览器在页面未聚焦时会暂停静音后台视频（Chrome power-saving）；本次实测聚焦后悬停播放与灯箱自动播放正常，真实浏览器不受此限制。
- 已知限制: HEIC 静帧与 HEVC 视频的显示/解码依赖浏览器能力，Chrome 可能不支持；本 change 不做转码。
- 已知限制: 未引入前端测试框架，解析逻辑以临时 Node 脚本校验，未随仓库提交。
- 已知限制: 合并文件拆分依赖 live-photo 的字节拆分，已加魔数校验；极端构造文件会回退为普通图片。

## 阻塞项

_无。_

## 风险与跳过的工作

- 本次验收未在真实浏览器中独立重放悬停/长按/灯箱播放，仅通过组件代码、live-photo@0.2.0 库实现与构建产物核对；实际播放仍受浏览器对 HEIC/HEVC 的解码能力限制（brief 已声明不做转码）。
- server/internal/handlers/photo.go servePhotos 对无视频照片也始终输出 video_src:""（启用/未启用存储均如此），相对改动前是响应结构新增空字段；前端按空串视为普通照片，A3 行为不受影响，但与 spec 中“无视频照片的响应结构与改动前一致”的措辞存在轻微偏差。
- PhotoWallView 的 LivePhotoMedia 位于 <button> 内，播放器自带的 LIVE 徽标点击会冒泡到卡片并触发灯箱打开，卡片上的播放器下拉菜单实际不可用；不影响 LIVE 标识、悬停/长按播放与灯箱自动播放验收。
- 合并文件识别依赖 live-photo 的“最后一个 ftyp + 前 8KB 内 moov/mdat”启发式；容器布局特殊（如 moov/mdat 均不在前 8KB）的合并文件会回退为普通图片，符合 spec 的校验约定但存在真实文件覆盖盲区。
- video_key 的视频类型校验信任 HEAD 返回的 Content-Type（本地存储按扩展名推断），不做内容嗅探；错误标注 Content-Type 的对象可能通过视频白名单。

## 之前的迭代

| 目标周期 | 迭代 | 尝试 | 结果 | 未解决项 | 摘要 | 完成时间 |
| ---: | ---: | ---: | --- | --- | --- | --- |
| 1 | 1 | 1 | pass | — | 独立只读验收通过 A1-A9：同名配对、合并拆分、普通图片/未合并 HEIC 回退、视频失败降级、video_key 校验、删除补偿、前台播放组件接线与构建测试均核对通过。go build/go test/npm run build 本次独立运行成功。遗留风险主要是未在真实浏览器重放播放行为、无视频响应新增空 video_src 字段、卡片徽标点击冒泡与合并文件启发式覆盖面。 | 2026-11-05T05:40:10.883Z |



## 结论

独立只读验收通过 A1-A9：同名配对、合并拆分、普通图片/未合并 HEIC 回退、视频失败降级、video_key 校验、删除补偿、前台播放组件接线与构建测试均核对通过。go build/go test/npm run build 本次独立运行成功。遗留风险主要是未在真实浏览器重放播放行为、无视频响应新增空 video_src 字段、卡片徽标点击冒泡与合并文件启发式覆盖面。
