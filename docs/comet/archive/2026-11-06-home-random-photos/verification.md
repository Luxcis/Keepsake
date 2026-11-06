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
- 完成时间: 2026-11-06T01:18:15.164Z
- 摘要: 独立只读验收完成：逐文件核对服务端查询/路由/处理器与前端 HomeView/API 类型，复核 7 项验收并额外运行 go test ./internal/server -run TestRandomPhotos（GateRequired、PoolFiltersAndLimit、StorageDisabled 全部 PASS，工作区文件未变化）。A1–A7 均有代码与运行/截图证据支持，未发现功能差异；仅记录两项不影响验收的观察性风险。

## 验收

| 编号 | 结果 | 来源 | 验收项 | 原因 |
| --- | --- | --- | --- | --- |
| A1 | passed | brief.md | 随机照片接口只返回 `status = published` 旅行下的照片条目：草稿旅行的照片、单独上传的视频条目不会出现，返回条数不超过请求的 `limit`。 | queries_photo.go:157-166 的 SQL 仅 JOIN trips 且 WHERE t.status='published' AND p.media_kind='photo'，ORDER BY RANDOM() 后 LIMIT ?；photo.go:336-345 将 limit 限定为 1–12（默认 3）。photos_random_test.go:140-196 以已发布旅行 5 张照片 + 1 个独立视频 + 草稿旅行 2 张照片验证：limit=2 返回 2 条且全部属于已发布照片、limit=12 仅返回 5 条且视频缺席、limit=abc/0/99 与缺省回落为 3 条。独立执行 go test ./internal/server -run TestRandomPhotos -v 三个测试均 PASS。 |
| A2 | passed | brief.md | 未解锁门禁时请求随机照片接口返回 401 `gate_required`；解锁后返回照片数组，条目包含所属旅行 ID、标题以及非空的缩略图地址。 | api.go:40-47 将 GET /photos/random 挂在 withGate 路由组内；middleware.go:40-43 在无 gate cookie 时调用 gateRequired，respond.go:57-58 返回 401 且 code=gate_required。photos_random_test.go:99-138 使用独立的无 Cookie visitorClient（注释明确避免管理员绕过门禁造成假阳性）验证未解锁 401 且响应体不含 /api/storage/ 地址；解锁后返回 trip_id、trip_title、thumb_src、preview_src、src 均非空，且 trip_title 正确。独立执行该测试 PASS。 |
| A3 | passed | brief.md | 首页首屏主视觉显示随机照片作为背景，并保留深色渐变遮罩，首屏标题、副标题与按钮清晰可读，不再显示纯渐变背景。 | HomeView.vue:90-99 首屏在 hero-canvas 内渲染第 1 张随机照片（heroSrc: preview_src 优先、回退 src，见 34-36），img 以 absolute inset-0 + object-cover 铺满；同一层叠顺序中 hero-photo-scrim 覆盖在图片之上（98 行），191-195 定义底部 0.88 至顶部 0.15 的深色渐变，仅在有照片时出现；无照片时保留 184-188 的原有渐变。Builder 截图（亮色演唱会照片）确认标题、副标题与 CTA 按钮在照片上清晰可读。 |
| A4 | passed | brief.md | 首页简介拼贴区的两张照片卡片显示随机照片，保持现有三列/两列拼贴布局、圆角与悬停缩放效果。 | HomeView.vue:151-177 两张 photo-card 分别使用第 2、3 张照片（decoPhotoA/decoPhotoB，29-31），cardSrc 按 thumb_src→preview_src→src 回退（37-39）；布局仍为 151 行 grid-cols-5 与 152/162 行 col-span-3/col-span-2，类名保留 rounded-sm 与 photo-card（全局 main.css:61-65 提供 overflow-hidden），196-201 保留 .photo-card:hover img 的 scale(1.045) 悬停缩放。Builder 拼贴截图确认三列/两列布局与圆角、两张照片正常显示。 |
| A5 | passed | brief.md | 每次打开或刷新首页时照片组合重新随机；同一次页面停留期间照片不变化，不会在页面内自动轮换。 | HomeView.vue:41-53 仅由 onMounted 调用一次 loadRandomPhotos 请求 /api/photos/random?limit=3，之后无定时器或轮换逻辑，页面停留期间照片不变；web/src/router/index.ts:6 为普通路由且全项目无 keep-alive，离开再进入或刷新都会重新挂载并重新请求。服务端 queries_photo.go:166 使用 ORDER BY RANDOM()，photo.go:379 对成功响应设置 Cache-Control: no-store，保证每次请求重新抽取。 |
| A6 | passed | brief.md | 门禁未解锁、存储未配置、可用照片少于 3 张或图片地址签名失败时，无照片的位置保持现有渐变占位，页面不出现破图、错误提示或空白区块，其它首页内容正常显示。 | HomeView.vue:41-48 对 401/网络/其它异常一律 catch 后把 photos 置空，不弹解锁提示、不显示错误信息；91-97 与 153-171 仅当地址非空才渲染 img，地址为空时保留 .hero-canvas（184-188）与 .hero-deco-1/.hero-deco-2（202-209）的渐变占位；18-26 的 onImageError 把加载失败的照片加入 failedIds，usable()（23-26）使 v-if 立即移除该 img，不留下破图，其余区块正常；照片少于 3 张时按 index 顺序填充（29-31），缺位保持占位。服务端 photo.go:356-376 在存储未配置时返回 enabled=false 且三个地址为空串，photos_random_test.go:198-229 覆盖该路径并通过。 |
| A7 | passed | brief.md | 首页随机照片为纯展示：点击主视觉或拼贴照片不会打开灯箱、不会跳转页面，光标不呈现可点击样式。 | HomeView.vue 的主视觉与两张拼贴照片均为纯 img，全文未绑定 @click/onClick，无 Lightbox 组件、无 router.push/跳转调用（唯一 RouterLink 是 112-117 行既有的「开启我们的故事」CTA，与照片元素无关）；首屏照片容器带 aria-hidden=true（90 行），照片无 role=button；web/src 全量 grep 无 cursor 样式规则，.photo-card（main.css:61-65）不含 cursor:pointer，因此悬停不出现可点击光标。 |

## 检查

| 检查 | 命令 | 工作目录 | 状态 | 退出码 | 耗时 |
| --- | --- | --- | --- | ---: | ---: |
| go build ./... | build ./... | server | passed | 0 | 516 ms |
| go test ./... | test ./... | server | passed | 0 | 233 ms |
| vue-tsc --noEmit | vue-tsc --noEmit -p [REDACTED] | web | passed | 0 | 1103 ms |
| vite build | vite build | web | passed | 0 | 541 ms |

### Builder 报告的证据

以下为 Builder 报告，不等同于 Runtime 检查凭据或独立验收结果。

- gofmt: passed — cd server && gofmt -l . 无输出
- go build: passed — cd server && go build ./... 成功
- go test: passed — cd server && go test ./... 全部通过，含新增 TestRandomPhotosGateRequired / TestRandomPhotosPoolFiltersAndLimit / TestRandomPhotosStorageDisabled
- vue-tsc: passed — cd web && npx vue-tsc --noEmit -p tsconfig.app.json 通过，tsbuildinfo 未被改写
- vite build: passed — cd web && npx vite build 通过，HomeView chunk 正常产出
- browser visual check: passed — 本地临时实例（复制开发数据）5 张照片：首屏照片+遮罩、拼贴两张照片、刷新重新随机、启用门禁后回退渐变且无破图
- 已知限制: 本地视觉验证使用复制自开发库的临时实例（/tmp/hrp-visual，含 5 张照片），未改动用户数据，验证后已停止临时服务。
- 已知限制: 仅截图验证桌面 1280x720；移动端沿用原响应式 class，未单独截图。
- 已知限制: 门禁启用且访客未解锁时，首页按已确认方案静默显示渐变占位，不提供解锁入口。

## 阻塞项

_无。_

## 风险与跳过的工作

- photo.go:365-374 在 src 签名失败时跳过 thumb/preview 签名，与 spec「单个签名失败只让该字段为空串」的逐字段独立签名措辞不完全一致；前端仍把空地址回退为渐变占位，不影响 A1–A7 的验收结论，建议后续对齐逐字段签名。
- 候选工作区还包含与功能无关的 .gitignore 变更（Comet 托管块从文件头移动到文件末尾，内容仅顺序变化），提交时需确认是否属于本 change。

## 之前的迭代

| 目标周期 | 迭代 | 尝试 | 结果 | 未解决项 | 摘要 | 完成时间 |
| ---: | ---: | ---: | --- | --- | --- | --- |
| 1 | 1 | 1 | pass | — | 独立只读验收完成：逐文件核对服务端查询/路由/处理器与前端 HomeView/API 类型，复核 7 项验收并额外运行 go test ./internal/server -run TestRandomPhotos（GateRequired、PoolFiltersAndLimit、StorageDisabled 全部 PASS，工作区文件未变化）。A1–A7 均有代码与运行/截图证据支持，未发现功能差异；仅记录两项不影响验收的观察性风险。 | 2026-11-06T01:18:15.164Z |



## 结论

独立只读验收完成：逐文件核对服务端查询/路由/处理器与前端 HomeView/API 类型，复核 7 项验收并额外运行 go test ./internal/server -run TestRandomPhotos（GateRequired、PoolFiltersAndLimit、StorageDisabled 全部 PASS，工作区文件未变化）。A1–A7 均有代码与运行/截图证据支持，未发现功能差异；仅记录两项不影响验收的观察性风险。
