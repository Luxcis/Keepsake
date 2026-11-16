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
- 完成时间: 2026-11-16T01:47:40.128Z
- 摘要: 独立验收全部 5 项通过。复用 Runtime 已执行的 go-build、go-test、web-typecheck、web-build 记录：其 candidateId 与本次候选一致，inputBefore==inputAfter 且 exitCode 均为 0；另独立执行 go test ./internal/server -run 'TestSiteOpenDate|TestUnlockRateLimitedWhileClosed' -count=1 复核服务端顺序、未开放弹窗数据、自定义 {date} 文案、已开放解锁与未开放期间限速语义，均通过。前端弹窗结构、关闭方式与内联错误分支经 GatePrompt.vue 源码核对。

## 验收

| 编号 | 结果 | 来源 | 验收项 | 原因 |
| --- | --- | --- | --- | --- |
| A1 | passed | brief.md | 站点已设置未来开放日期时，访客在门禁卡片输入错误口令：卡片内显示「访问口令不正确。」，不出现未开放弹窗。 | server/internal/handlers/auth.go:26-32 先执行 svc.Unlock（限速+口令校验），口令错误经 unlockFail 返回 401 gate_mismatch「访问口令不正确」；web/src/components/GatePrompt.vue:38-41 仅设置卡片内联 message，不设置 notOpenMessage，故不会渲染 dialog；server/internal/server/router_test.go:225-233 断言未开放时错误口令返回 401 gate_mismatch 且响应不含开放日期文案。独立执行 go test ./internal/server -run 'TestSiteOpenDate\|TestUnlockRateLimitedWhileClosed' -count=1 通过。 |
| A2 | passed | brief.md | 站点已设置未来开放日期时，访客输入正确口令：出现未开放弹窗，弹窗内提示包含具体开放日期；关闭弹窗后仍停留在门禁卡片，私人内容不可见。 | auth.go:34-42 口令通过后才判断开放日期，未开放返回 403 not_open_yet（携带渲染后提示语）且不发放 Cookie；server/internal/handlers/site.go:15、35-43 默认文案含 {date} 并替换为「2099 年 1 月 1 日」。GatePrompt.vue:48-54 将 err.message 写入 notOpenMessage，97-122 以 Teleport 渲染 role=dialog 弹层（「我知道了」按钮、遮罩 @click.self、window Esc 监听），是前端 dialog 而非浏览器 alert；closeNotOpen 仅清空 notOpenMessage、不触发 unlocked，父组件（web/src/views/TripsView.vue:69 等 @unlocked="gate = false;load()"）仍保持 gate=true，故关闭后停留门禁卡片且私人内容不可见。router_test.go:235-243 断言 403 not_open_yet 且响应含具体开放日期。弹窗交互由代码审查确认。 |
| A3 | passed | brief.md | 管理端配置了自定义未开放提示语（含 `{date}` 占位符）时，口令正确但未开放的弹窗显示替换日期后的完整自定义文案。 | site.go:37-43 renderNotOpenMessage 将自定义文案中的 {date} 替换为中文日期标签；auth.go:40 将渲染后的完整文案作为 error.message 返回；client.ts 解析 error.message，GatePrompt.vue:111 在弹窗中直接插值显示。router_test.go:245-253 配置自定义文案「先别急，{date} 再见。」后断言响应含「先别急，2099 年 1 月 1 日 再见。」；独立定向测试通过。 |
| A4 | passed | brief.md | 未设置开放日期或开放日期已过时，访客输入正确口令直接解锁并加载页面内容，不出现未开放弹窗。 | site.go:19-33 open_date 为空、非法或已过时均视为已开放；auth.go:34-50 已开放路径跳过 not_open_yet，IssueGate 并设置 HttpOnly 门禁 Cookie 返回 200；GatePrompt.vue:33-35 成功后 emit('unlocked')，私有页（TimelineView.vue:107、LetterView.vue:129、TripsView.vue:69、TripDetailView.vue:109）以 @unlocked="gate = false;load()" 加载内容，不出现未开放弹窗。router_test.go:208-214（过去日期）与 275-281（清空日期）断言解锁成功并可读内容；独立定向测试通过。 |
| A5 | passed | brief.md | 站点未开放期间，错误口令尝试计入限速；连续失败达到上限后提示「尝试次数过多，请稍后再试」，而不是继续提示口令不正确。 | server/internal/auth/service.go:79-86 限速计数在口令校验之前执行，站点未开放期间的每次错误口令尝试都计入同一 IP 的 15 分钟固定窗口（unlockLimitPerWindow=10、unlockLimitWindow=15m，service.go:29-30）；超限后返回 ErrRateLimited，auth.go:55-57 返回 429 rate_limited「尝试次数过多，请稍后再试」，GatePrompt.vue:42-44 内联显示「尝试次数过多，请稍后再试。」而非口令错误文案。router_test.go:284-309 TestUnlockRateLimitedWhileClosed：10 次错误口令均为 401 gate_mismatch，第 11 次即使口令正确也返回 429 rate_limited；独立定向测试通过。 |

## 检查

| 检查 | 命令 | 工作目录 | 状态 | 退出码 | 耗时 |
| --- | --- | --- | --- | ---: | ---: |
| Go 构建 | build ./... | server | passed | 0 | 453 ms |
| Go 测试 | test ./... | server | passed | 0 | 156 ms |
| 前端类型检查 | vue-tsc --noEmit -p [REDACTED] | web | passed | 0 | 1042 ms |
| 前端构建 | vite build | web | passed | 0 | 526 ms |

### Builder 报告的证据

以下为 Builder 报告，不等同于 Runtime 检查凭据或独立验收结果。

- gofmt: passed — gofmt -l 对修改的两个 Go 文件无输出
- go build: passed — server/ 下 go build ./... 成功
- go test: passed — server/ 下 go test ./... 全部包通过，含新增 TestUnlockRateLimitedWhileClosed
- vue-tsc: passed — web/ 下 npx vue-tsc --noEmit -p tsconfig.app.json 无错误
- vite build: passed — web/ 下 npx vite build 成功，dist 未被 Git 跟踪
- browser-smoke: passed — 隔离临时数据库 + 本地 API + Vite 代理预览：错误口令内联提示、正确口令未开放弹窗、自定义 {date} 文案、清空日期后解锁、Esc 关闭弹窗均按预期；预览数据与进程已清理
- 已知限制: 前端没有单元测试框架，弹窗行为以本地隔离预览的浏览器观察和代码审查作为证据，未形成自动化回归测试。
- 已知限制: 闭站期间会暴露口令是否正确并消耗限速次数，这是本次明确接受的取舍，已在 brief 关键决定中记录。
- 已知限制: Esc 关闭通过页面 keydown 事件验证监听器生效，遮罩点击关闭仅由实现保证，未单独做自动化点击验证。

## 阻塞项

_无。_

## 风险与跳过的工作

- 前端弹窗与内联提示的渲染行为由代码审查确认；本轮未独立运行浏览器做端到端观察，项目也无前端自动化测试框架。服务端契约与错误文案已由独立定向测试复核。
- 未开放弹窗未实现焦点陷阱与背景滚动锁定（规格未要求，属可访问性增强项，不影响本次验收行为）。

## 之前的迭代

| 目标周期 | 迭代 | 尝试 | 结果 | 未解决项 | 摘要 | 完成时间 |
| ---: | ---: | ---: | --- | --- | --- | --- |
| 1 | 1 | 1 | pass | — | 独立验收全部 5 项通过。复用 Runtime 已执行的 go-build、go-test、web-typecheck、web-build 记录：其 candidateId 与本次候选一致，inputBefore==inputAfter 且 exitCode 均为 0；另独立执行 go test ./internal/server -run 'TestSiteOpenDate\|TestUnlockRateLimitedWhileClosed' -count=1 复核服务端顺序、未开放弹窗数据、自定义 {date} 文案、已开放解锁与未开放期间限速语义，均通过。前端弹窗结构、关闭方式与内联错误分支经 GatePrompt.vue 源码核对。 | 2026-11-16T01:47:40.128Z |



## 结论

独立验收全部 5 项通过。复用 Runtime 已执行的 go-build、go-test、web-typecheck、web-build 记录：其 candidateId 与本次候选一致，inputBefore==inputAfter 且 exitCode 均为 0；另独立执行 go test ./internal/server -run 'TestSiteOpenDate|TestUnlockRateLimitedWhileClosed' -count=1 复核服务端顺序、未开放弹窗数据、自定义 {date} 文案、已开放解锁与未开放期间限速语义，均通过。前端弹窗结构、关闭方式与内联错误分支经 GatePrompt.vue 源码核对。
