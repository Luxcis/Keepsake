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
- 完成时间: 2026-11-09T08:47:25.325Z
- 摘要: 独立只读验收：静态阅读 LetterView.vue、LetterCard.vue、motion.ts 与样式，并在 /tmp/letter-preview 临时预览（静态站 4173 + Go API 18080，未触碰项目 data/）用浏览器实测四段动画、抽纸遮挡、展开、降级、重复点击和三视口。因 ZCode 窗口处于后台、原生 rAF 被挂起且 CUA/Playwright 输入注入未送达页面，点击使用真实 DOM click 事件触发，并用可控虚拟时钟 + 手动帧泵按时间线逐段采样与截图；web-typecheck / web-build 复用已记录 passed，源文件 mtime(16:27) 早于 dist 构建(16:29)。A1-A7 全部通过。

## 验收

| 编号 | 结果 | 来源 | 验收项 | 原因 |
| --- | --- | --- | --- | --- |
| A1 | passed | brief.md | A1：点击「拆开这封信 ✉」后，先看到信封封口向上翻开，信封本体仍留在原位；随后一部分折起的空白信纸（纸面无文字）从信封内向上露出，信封此时仍完整可见。 | 0-420ms 仅封口 rotateX 由 0 翻到 -180 度（transform m33: 1→0.694→-0.694→-1），信封正面 opacity=1、translateY=0 始终留在原位；420-920ms 信纸层 translateY 由 157px 升到 -58px（折叠高 115px），信封上沿露出 58px（约折叠高度的 50%）；此时 paper-inner opacity=0，纸面无文字，信封正面 opacity=1、y=0 完整可见。截图 10-a1-flap-200ms / 11-a1-paper-revealed-920ms 与之吻合。 |
| A2 | passed | brief.md | A2：信纸继续向上完全抽出（信纸下沿高于信封上沿），信封同时向下移动并逐渐淡出至完全不可见；抽出过程中信纸始终显示在信封之上，不被信封遮挡。 | 980-1580ms 信纸层 y 由 -58px 升到 -135px；1580ms 时信纸下沿（-17.5px）已高于信封上沿，1630ms 时下沿高于信封上沿约 9px 且信封已下移 56px；信封各层 980-1630ms translateY 0→56px、opacity 1→0（1630ms front/flap/label 均为 0）。信纸层 z-index=1，封口在 250ms 翻到侧面时切到 z-index=0，露出的纸面始终绘制在信封与封口之上（1250ms 截图可见纸带覆盖在信封与翻盖之上）；仍在信封内部的下半部分被信封正面遮住，符合“从信封内抽出”的物理层次。 |
| A3 | passed | brief.md | A3：信纸完全抽出后，折叠的空白信纸向下展开为完整信纸，展开过程中随纸面展开显示出正文，展开完成后标题、全部正文段落、日期与落款完整可见，文字与纸面背景均未被裁切，折痕消失。 | 1500-2350ms clip 高度由 115px 增长到 739px，内层正文 opacity 0→1（1680ms 起淡入，2100ms 达 1），折痕 opacity 在 2030ms 降为 0；1900ms 截图中正文随纸面展开逐渐显现。完成后切换为真实 article：标题「写给你的第七封信」、7 段正文、日期 2026.11.09、落款「永远爱你的那个人」全部可见；最终 article 高 740px、无 overflow 裁切，纸面背景完整。 |
| A4 | passed | brief.md | A4：动画结束后页面稳定为与改动前一致的阅读版式（信纸宽度等于父容器宽度且不超过 448px，留白与行距不变），没有残留的信封元素或空白占位，信纸不出现位置突跳。 | 完成前 2300ms 动画纸面 top=473.89px、left=416、w=448、h=739，页面 scrollHeight=1496；完成后 2350ms 最终 article top=474、left=416、w=448、h=740，scrollHeight 仍为 1496，无位置/高度突跳。场景、按钮与 spacer 全部移除（stagePresent=false、btnPresent=false、articleCount=1）。最终信纸宽度 448px≤448px（375/768 视口下为 335/448px，等于父容器内容宽），padding 40px（375 下 20px）、正文 line-height 32px，与原 article 的类名和内容一致。 |
| A5 | passed | brief.md | A5：系统开启「减少动效」或 GSAP 模块加载失败时，点击「拆开这封信」后直接显示完整信纸，不播放任何动画阶段，信件可正常阅读。 | 覆盖 matchMedia 令 (prefers-reduced-motion: reduce) 命中后点击：仅触发 1 次 reduce 查询，无 motion-ready、无 GSAP 内联样式，信封与按钮立即移除，直接显示完整信纸（7 段正文、title、日期、落款；top 474、w 448、h 740），无动画阶段。另将 web/dist 复制到 /tmp 并移除 gsap 分块（该请求返回 text/html 导致模块加载失败），点击后同样直接显示完整信纸、无动画、无未捕获错误。 |
| A6 | passed | brief.md | A6：动画进行期间重复点击「拆开这封信」不会重启动画或产生跳帧；动画完成后信封与按钮不再出现，页面可正常滚动阅读。 | openLetter 在 await 前同步置 phase=animating；同一 tick 连续 2 次 click 事件只产生一条时间线（500ms 时 layerY=69.431，与单次运行基准一致）。在 500ms 处强制移除 disabled 属性后再点按钮 2 次并依次点击信封、纸层、信封正面，状态仍为 69.431 不变；继续推进到 920ms 得到 layerY=-58、front opacity=1、frontY=0，与单次基准完全一致，无重启或跳帧。完成后场景与按钮不存在；scrollHeight=1496>900，scrollTo(0,500) 在下一帧生效（scrollY=500，截图可见滚动条），页面可正常滚动阅读。 |
| A7 | passed | brief.md | A7：在手机（375px）、平板（768px）与桌面（1280px）视口下，开信动画各阶段与展开后的信纸都完整显示，无横向溢出、无两侧空隙。 | 在 375/768/1280 三种视口完整跑完动画：各阶段 documentElement.scrollWidth 均等于 innerWidth（375/768/1280）；信封、纸层与最终信纸同宽（分别为 335/448/448px）并左右居中（375: 左右各 20px；768: 各 160px；1280: 各 416px），无横向溢出、无两侧空隙；A1/A2 阶段纸面均在视口内，最终 article 宽度为 min(448, 父内容宽)，375 下 335px 恰好填满父内容区。 |

## 检查

| 检查 | 命令 | 工作目录 | 状态 | 退出码 | 耗时 |
| --- | --- | --- | --- | ---: | ---: |
| 前端类型检查 | vue-tsc --noEmit -p [REDACTED] | web | passed | 0 | 1267 ms |
| 前端构建 | vite build | web | passed | 0 | 678 ms |

### Builder 报告的证据

以下为 Builder 报告，不等同于 Runtime 检查凭据或独立验收结果。

- 前端类型检查: passed — cd web && npx vue-tsc --noEmit -p tsconfig.app.json 通过
- 前端构建: passed — cd web && npx vite build 通过
- 浏览器分阶段采样: passed — 用临时复制的 journey.db + 本地存储启动 Go API（18080）与静态预览（4173），1280px 下采样各阶段几何/透明度并截图
- 减少动效降级: passed — 覆盖 matchMedia 模拟 reduce，点击后直接显示完整信纸
- 关闭揭晓保持原行为: passed — reveal_enabled=0 时直接显示信纸与语音彩蛋区块，无信封
- 已知限制: 浏览器窗口处于后台时 requestAnimationFrame 与定时器被系统节流，动画无法播放；768px 视口只核对了最终版式与无横向溢出，动画阶段证据来自 1280px 与 375px 视口。
- 已知限制: 动画缓动与节奏属于视觉判断，验收以采样到的位移/透明度/高度数据和阶段截图为准。

## 阻塞项

_无。_

## 风险与跳过的工作

- 开封时翻盖向上翻到信封卡片上沿之上，会遮住页面引导段落的部分文字（375/768/1280 截图均可见），因为 .envelope-stage 去掉了 overflow-hidden 以便信纸升起；该遮挡不属于 A1-A7 明确验收项，但属于视觉副作用。
- .envelope-button 带有 Tailwind transition，GSAP 对 opacity 的逐帧写入被 CSS transition 延迟约 150ms；冻结时钟测量在 1630ms 时按钮计算 opacity 仍约 0.83，等待 500ms 后为 0，实际播放中按钮淡出略滞后于信封本体（最终仍会消失并在完成时移除）。
- 时间线实际结束于约 2.35s（clip 展开段 1.50-2.35s），比 brief 的“约 2.2 秒”略长，仍属约数范围。

## 之前的迭代

| 目标周期 | 迭代 | 尝试 | 结果 | 未解决项 | 摘要 | 完成时间 |
| ---: | ---: | ---: | --- | --- | --- | --- |
| 1 | 1 | 1 | pass | — | 独立只读验收：静态阅读 LetterView.vue、LetterCard.vue、motion.ts 与样式，并在 /tmp/letter-preview 临时预览（静态站 4173 + Go API 18080，未触碰项目 data/）用浏览器实测四段动画、抽纸遮挡、展开、降级、重复点击和三视口。因 ZCode 窗口处于后台、原生 rAF 被挂起且 CUA/Playwright 输入注入未送达页面，点击使用真实 DOM click 事件触发，并用可控虚拟时钟 + 手动帧泵按时间线逐段采样与截图；web-typecheck / web-build 复用已记录 passed，源文件 mtime(16:27) 早于 dist 构建(16:29)。A1-A7 全部通过。 | 2026-11-09T08:47:25.325Z |



## 结论

独立只读验收：静态阅读 LetterView.vue、LetterCard.vue、motion.ts 与样式，并在 /tmp/letter-preview 临时预览（静态站 4173 + Go API 18080，未触碰项目 data/）用浏览器实测四段动画、抽纸遮挡、展开、降级、重复点击和三视口。因 ZCode 窗口处于后台、原生 rAF 被挂起且 CUA/Playwright 输入注入未送达页面，点击使用真实 DOM click 事件触发，并用可控虚拟时钟 + 手动帧泵按时间线逐段采样与截图；web-typecheck / web-build 复用已记录 passed，源文件 mtime(16:27) 早于 dist 构建(16:29)。A1-A7 全部通过。
