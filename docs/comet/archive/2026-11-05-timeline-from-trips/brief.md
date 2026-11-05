# 目标

管理面板不再提供时光轴事件维护；前台时光轴页面改为根据已发布的旅行足迹自动生成，每个旅行对应一个时间节点；前台旅行足迹列表按旅行时间排序展示，最新的旅行在前。

# 范围

- 管理端：移除「时光轴」标签页、`AdminTimelineView` 页面及其路由，并移除对应管理 API（`/api/admin/timeline` 的增删改）。
- 前台时光轴：不再读取手工事件，改为在客户端依据公开旅行列表（`/api/trips`）派生节点；沿用现有页面结构、门禁降级与滚动动效。
- 前台旅行足迹：公开旅行列表改为根据出发日期排序；`sort_order` 仅作为同一天出发时的次级排序依据。
- 服务端：移除 `timeline_events` 相关查询、写入、JSON 序列化与公开 `/api/timeline` 接口；公开旅行列表查询改为按出发日期排序。
- 数据：新增迁移删除 `timeline_events` 表，手工事件数据一并移除。
- 门禁与错误降级保持现状：未解锁时仍显示 `GatePrompt`。

## 目录结构

### 新建

- server/internal/store/migrations/0005_drop_timeline_events.sql — 删除手工时光轴事件表 `timeline_events`（DROP TABLE 同时移除其索引）。

### 修改

- web/src/views/TimelineView.vue — 数据来源改为 `/api/trips`，按旅行生成节点（日期、标题、简介、封面、跳转），保持门禁与滚动动效。
- web/src/views/TripsView.vue — 适配按出发日期倒序后的卡片顺序与编号；排序由后端查询完成。
- web/src/views/admin/AdminView.vue — 移除「时光轴」标签。
- web/src/router/index.ts — 移除 `admin-timeline` 路由。
- web/src/api/client.ts — 移除 `TimelineEvent` 类型；时光轴复用现有 `Trip` 类型。
- server/internal/handlers/api.go — 移除 `/api/timeline` 与 `/api/admin/timeline` 路由及 `listTimeline` handler。
- server/internal/handlers/admin.go — 移除时光轴校验与创建/更新/删除 handler。
- server/internal/store/queries.go — 移除 `ListTimeline`；`ListTrips` 改为按出发日期倒序、同日按 `sort_order` 升序。
- server/internal/store/queries_admin.go — 移除 `TimelineEventInput` 与时光轴 CRUD。
- server/internal/store/marshal.go — 移除 `TimelineEvent` 的 `MarshalJSON`。
- server/internal/server/router_test.go — 移除时光轴接口测试；补充旅行时间排序与时光轴接口已移除的覆盖。
- README.md — 从「已实现接口」表移除 `/api/timeline` 与 `/api/admin/timeline` 两行。

### 删除

- web/src/views/admin/AdminTimelineView.vue — 管理端时光轴维护页。

# 非目标

- 不改变旅行详情、照片墙、生日信等其他页面与接口的行为。
- 不新增“相识、生日”等非旅行类时间节点，也不保留任何手工录入时光轴的入口。
- 不改变门禁策略、登录态或管理端权限模型。
- 不做旧手工事件到旅行节点的自动迁移；旧事件按 Q3 选择直接删除。
- 不调整旅行管理中的 `sort_order` 字段编辑能力，后台仍可维护该字段。

# 验收示例

- A1：管理后台不再显示「时光轴」标签和管理页；访问 `/admin/timeline` 落到页面不存在；`GET /api/timeline` 与 `/api/admin/timeline` 的增删改请求均返回 404。
- A2：发布一段旅行后打开 `/timeline`，该旅行自动成为一个节点，节点按出发日期从早到晚排列；草稿旅行不产生节点。
- A3：时光轴节点显示出发日期（有结束日期时显示起止范围）、旅行标题、旅行简介和封面缩略图；没有封面时显示占位；点击节点进入 `/trips/:id`。
- A4：`/trips` 页面的已发布旅行按出发日期从近到远排列（最新在前），同一天出发的旅行按 `sort_order` 升序；草稿旅行不出现。
- A5：迁移执行后数据库不再存在 `timeline_events` 表，原有手工时光轴记录不再出现在任何页面或接口。
- A6：未解锁时 `/timeline` 显示门禁卡片；没有已发布旅行时显示空态提示且文案不再指引去后台补录；加载失败时显示错误与重试入口。
- A7：`cd server && go build ./... && go test ./...` 与 `cd web && npm run build` 均通过。

# 关键决定

- 时光轴节点在客户端由 `/api/trips` 数据派生，不保留独立的 `/api/timeline` 接口；公开旅行列表是唯一数据源，避免两套数据语义。
- 时光轴节点按出发日期从早到晚排列（沿用现有叙事方向）；旅行足迹列表按出发日期最新在前排列。
- 节点日期取 `start_date`，`end_date` 存在且与出发日期不同时显示为起止范围。
- 节点整块可点击，跳转到对应旅行详情 `/trips/:id`。
- 封面使用 `/api/trips` 已签名的 `cover_thumb_src`；缺失或签名为空时沿用现有渐变占位样式。
- 管理端 `sort_order` 字段保留，但公开列表不再以其为主排序依据。
- 删除 `timeline_events` 表和已有手工事件数据（用户明确选择删除）。

# Verification expectations

- Go：`cd server && go build ./... && go test ./...` 通过；测试覆盖公开旅行列表的时间排序，以及 `/api/timeline` 与 `/api/admin/timeline` 返回 404。
- 前端：`cd web && npm run build`（`vue-tsc -b` 类型检查 + Vite 构建）通过。
- 迁移：测试库执行迁移后不存在 `timeline_events` 表。
