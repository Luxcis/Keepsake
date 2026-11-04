# Naive UI

> 一个 Vue 3 组件库。比较完整，主题可调，使用 TypeScript，快。

下面每个链接都是文档站（https://www.naiveui.com）一个页面的 Markdown 版本。组件页包含 API 和每个示例的完整源码。所有页面合在一个文件里：https://www.naiveui.com/zh-CN/llms-full.txt。English: https://www.naiveui.com/llms.txt.

## 介绍

- [Naive UI](https://www.naiveui.com/zh-CN/docs/introduction.md): Naive UI 是一个 Vue3 的组件库。

## 快速上手

- [安装](https://www.naiveui.com/zh-CN/docs/installation.md): 使用 npm 安装。
- [在 SFC 中使用](https://www.naiveui.com/zh-CN/docs/usage-sfc.md): 如果你想使用单文件组件风格（SFC - Single File Component），可以选择直接引入或全局安装在 Vue App 中。
- [使用 UMD](https://www.naiveui.com/zh-CN/docs/umd.md): 在 `2.30.3` 版本后，你可以使用 UMD 版本的 naive。
- [配置字体](https://www.naiveui.com/zh-CN/docs/fonts.md): Naive UI 可以和 [vfonts](https://github.com/07akioni/vfonts) 配合，你可以简单的引入 `vfonts` 中的字体，包含常规字体和等宽字体。
- [按需引入（Tree Shaking）](https://www.naiveui.com/zh-CN/docs/import-on-demand.md): Naive UI 支持 tree shaking，组件、语言、主题均可 tree-shaking。
- [支持的平台](https://www.naiveui.com/zh-CN/docs/supported-platforms.md): 不支持 IE 浏览器。
- [常见问题](https://www.naiveui.com/zh-CN/docs/common-issues.md): 如果你没有在同一个组件上同时使用 `v-model:xxx` 和 `on-update:xxx`，`@update:xxx` 和 `on-update:xxx` 在模版中使用时没有任何区别。
- [受控模式与非受控模式](https://www.naiveui.com/zh-CN/docs/controlled-uncontrolled.md): 一个组件的行为可以分为受控模式和非受控模式两种。非受控模式指的是只监听组件的变化，而不去控制组件的 value，受控模式指的是控制组件的值。
- [LLMs](https://www.naiveui.com/zh-CN/docs/llms.md): 文档也提供给大模型和 Agent 使用的 Markdown 版本，遵循 [llms.txt](https://llmstxt.org) 规范。

## 指南

- [JSX & TSX](https://www.naiveui.com/zh-CN/docs/jsx.md): 关于启用 JSX 和 TSX，请参考你使用的工具链的相关文档。
- [服务端渲染 Server-Sider Rendering](https://www.naiveui.com/zh-CN/docs/ssr.md): 由于 naive-ui 在使用 CSS in JS，在 SSR 的情况下需要一些额外的配置。
- [Nuxt.js](https://www.naiveui.com/zh-CN/docs/nuxtjs.md): 本文档涉及到 SSR，请先了解[SSR 的注意事项](https://www.naiveui.com/zh-CN/docs/ssr.md#%E6%B3%A8%E6%84%8F)。
- [Vitepress](https://www.naiveui.com/zh-CN/docs/vitepress.md): 本文档涉及到 SSR，请先了解[SSR 的注意事项](https://www.naiveui.com/zh-CN/docs/ssr.md#%E6%B3%A8%E6%84%8F)。
- [Vite SSG/SSE](https://www.naiveui.com/zh-CN/docs/vite-ssge.md): 本文档涉及到 SSR，请先了解[SSR 的注意事项](https://www.naiveui.com/zh-CN/docs/ssr.md#%E6%B3%A8%E6%84%8F)。
- [调整主题](https://www.naiveui.com/zh-CN/docs/customize-theme.md): Naive UI 通过使用 `n-config-provider` 调整主题。
- [国际化](https://www.naiveui.com/zh-CN/docs/i18n.md): Naive-ui 通过使用 `n-config-provider` 调整语言，默认情况下所有组件均为英语。
- [创建适配主题的组件](https://www.naiveui.com/zh-CN/docs/theme.md): 你可能觉得只用内置的组件不够爽，想自己也写适配主题的组件。
- [潜在的样式冲突](https://www.naiveui.com/zh-CN/docs/style-conflict.md): 有时你可能希望控制样式元素插入的位置。
- [社区精选资源](https://www.naiveui.com/zh-CN/docs/community.md): Naive UI 是统一设计规范的高质量 Vue 组件库，我们倾向于只提供符合统一规范且带有视觉展现的 UI 组件。所以我们推荐以下社区已有的优秀资源，与 Naive UI 形成互补。

## 通用组件

- [头像 Avatar](https://www.naiveui.com/zh-CN/components/avatar.md): 在互联网上，没有人知道你是 \*\*\* 。
- [按钮 Button](https://www.naiveui.com/zh-CN/components/button.md): 按钮用来触发一些操作。
- [卡片 Card](https://www.naiveui.com/zh-CN/components/card.md): 放点东西进去。
- [轮播图 Carousel](https://www.naiveui.com/zh-CN/components/carousel.md): 一般用来播放一些好消息。
- [折叠面板 Collapse](https://www.naiveui.com/zh-CN/components/collapse.md): 我看到它总被用在界面边栏的控制面板上。
- [分割线 Divider](https://www.naiveui.com/zh-CN/components/divider.md): 分割一些内容。
- [下拉菜单 Dropdown](https://www.naiveui.com/zh-CN/components/dropdown.md): 当你想触发一些操作的时候。
- [文本省略 Ellipsis](https://www.naiveui.com/zh-CN/components/ellipsis.md): 复杂度不会消失，只会转移。
- [渐变文字 Gradient Text](https://www.naiveui.com/zh-CN/components/gradient-text.md): 这个东西看起来没啥用，实际上确实没啥用。
- [图标 Icon](https://www.naiveui.com/zh-CN/components/icon.md): naive-ui 推荐使用 [xicons](https://xicons.org) 作为图标库。
- [页头 PageHeader](https://www.naiveui.com/zh-CN/components/page-header.md): 我感觉你很可能用不上这个组件，因为根据你拿到的设计稿很可能是重写一个简单。
- [标签 Tag](https://www.naiveui.com/zh-CN/components/tag.md): 通常用来展示一些属性，偶尔也用来当一些备选的属性。
- [排印 Typography](https://www.naiveui.com/zh-CN/components/typography.md): Naive UI 提供了常用 HTML 标签的一些基本样式，以及对文本渲染提供帮助的组件。
- [水印 Watermark](https://www.naiveui.com/zh-CN/components/watermark.md): 出现在钱上的时候比较可爱。
- [浮动按钮 Float Button](https://www.naiveui.com/zh-CN/components/float-button.md): 跟 `Back Top` 很像，多一份颜值，多一份互动。

## 数据录入组件

- [自动填充 Auto Complete](https://www.naiveui.com/zh-CN/components/auto-complete.md): 用来当搜索提示或者类似的东西。
- [级联选择 Cascader](https://www.naiveui.com/zh-CN/components/cascader.md): 用来选一些树型信息。
- [颜色选择器 Color Picker](https://www.naiveui.com/zh-CN/components/color-picker.md): 和真实世界比起来，它的空间是不连续的。
- [复选框 Checkbox](https://www.naiveui.com/zh-CN/components/checkbox.md): 哟，哟，Check it out。
- [日期选择器 Date Picker](https://www.naiveui.com/zh-CN/components/date-picker.md): 关于如何设定时间，大家总有很多种想法。
- [动态录入 Dynamic Input](https://www.naiveui.com/zh-CN/components/dynamic-input.md): 这个组件的名字改过很多次。
- [动态标签 Dynamic Tags](https://www.naiveui.com/zh-CN/components/dynamic-tags.md): 把标签变得可以输入。
- [表单 Form](https://www.naiveui.com/zh-CN/components/form.md): 收集、验证信息。
- [文本输入 Input](https://www.naiveui.com/zh-CN/components/input.md): 很多年前，人们还在用打孔纸卡输入。
- [数字输入 Input Number](https://www.naiveui.com/zh-CN/components/input-number.md): 输入数字就用它。
- [验证码 Input OTP](https://www.naiveui.com/zh-CN/components/input-otp.md): 输入一次性密码。
- [提及 Mention](https://www.naiveui.com/zh-CN/components/mention.md): 一年多之前产品经理问我能不能搞这个东西，当时我让他们用多选凑活一下。
- [单选 Radio](https://www.naiveui.com/zh-CN/components/radio.md): 我还小的时候，很喜欢听收音机，比如 FM 106.8 或者 FM 92.1。
- [评分 Rate](https://www.naiveui.com/zh-CN/components/rate.md): 没有把握的话不要给星星换颜色，那会是个灾难。
- [选择器 Select](https://www.naiveui.com/zh-CN/components/select.md): 选点啥！
- [滑动选择 Slider](https://www.naiveui.com/zh-CN/components/slider.md): 就我所知，这东西一般就用来控制音量。
- [开关 Switch](https://www.naiveui.com/zh-CN/components/switch.md): 我有一个 Switch，不过没时间玩。每每这个时候，我都会回忆起童年玩 NDS 的快乐时光。
- [时间选择器 Time Picker](https://www.naiveui.com/zh-CN/components/time-picker.md): 就像一块数码表。
- [穿梭框 Transfer](https://www.naiveui.com/zh-CN/components/transfer.md): 一个更高效的穿梭框。
- [树型选择 Tree Select](https://www.naiveui.com/zh-CN/components/tree-select.md): 据说 99% 的人分不清它和 Cascader 的区别。
- [上传 Upload](https://www.naiveui.com/zh-CN/components/upload.md): 如果不在意延迟，我更想用卡车和硬盘。

## 数据展示组件

- [日历 Calendar](https://www.naiveui.com/zh-CN/components/calendar.md): 时间过的快。
- [倒计时 Countdown](https://www.naiveui.com/zh-CN/components/countdown.md): 每过一秒，就会少一秒。
- [代码 Code](https://www.naiveui.com/zh-CN/components/code.md): 下面的代码展示了如何为 Code 设定 hljs。比较推荐的方式是按需引入，因为它可以极大地减小打包尺寸。
- [数据表格 Data Table](https://www.naiveui.com/zh-CN/components/data-table.md): 数据表格用来显示一些格式化信息。
- [描述 Descriptions](https://www.naiveui.com/zh-CN/components/descriptions.md): 简单的列出信息。
- [无内容 Empty](https://www.naiveui.com/zh-CN/components/empty.md): 空<span style="opacity: 0;">空如</span>也。
- [公式 Equation](https://www.naiveui.com/zh-CN/components/equation.md): 没有人会想到一个组件库会有这种组件，但是作者的一位朋友需要这个。
- [图像 Image](https://www.naiveui.com/zh-CN/components/image.md): 预览一下。
- [列表 List](https://www.naiveui.com/zh-CN/components/list.md): 想让列表看起来优雅真的挺难的。但是也不能不做这个组件啊。
- [日志 Log](https://www.naiveui.com/zh-CN/components/log.md): 如果你有一些日志要展示，可以使用 Log。
- [数值动画 Number Animation](https://www.naiveui.com/zh-CN/components/number-animation.md): 这个动画，那么有科技感，快请到我们的大屏上来。
- [二维码 QR Code](https://www.naiveui.com/zh-CN/components/qr-code.md): 总是即将过期。
- [统计数据 Statistic](https://www.naiveui.com/zh-CN/components/statistic.md): 和它看起来一样简单。
- [表格 Table](https://www.naiveui.com/zh-CN/components/table.md): 如果你只想画点简单的表格，用它。需要渲染数据请看[数据表格 Data Table](https://www.naiveui.com/zh-CN/components/data-table.md)。
- [东西 Thing](https://www.naiveui.com/zh-CN/components/thing.md): 当你想要描述一个东西，使用 Thing。如果你觉得这个组件不能满足你的需求，可以自己写一个。
- [时间 Time](https://www.naiveui.com/zh-CN/components/time.md): Time 提供一些基本的时间格式化功能。
- [时间线 Timeline](https://www.naiveui.com/zh-CN/components/timeline.md): 这个世界有两个纬度：时间、事件。
- [树 Tree](https://www.naiveui.com/zh-CN/components/tree.md): 老实说我生物不好，认不出几种树来。
- [无限滚动 Infinite Scroll](https://www.naiveui.com/zh-CN/components/infinite-scroll.md): 滚雪球，滚啊滚，内容越来越多，停不下来。
- [高亮文本 Highlight](https://www.naiveui.com/zh-CN/components/highlight.md): 用过搜索引擎的都知道这个是做什么的。
- [热力图 Heatmap](https://www.naiveui.com/zh-CN/components/heatmap.md): 数字民工有属于自己的墙要刷。

## 导航组件

- [固钉 Affix](https://www.naiveui.com/zh-CN/components/affix.md): Affix 可以让内容在页面滚动的时候固定在一个位置，它和 `position: sticky` 有那么点像不过可以做更多事。
- [侧边导航 Anchor](https://www.naiveui.com/zh-CN/components/anchor.md): 你永远的指路明灯。
- [回到顶部 Back Top](https://www.naiveui.com/zh-CN/components/back-top.md): 它可以帮你回到你曾经到过的地方。不过时间是回不去了。
- [面包屑 Breadcrumb](https://www.naiveui.com/zh-CN/components/breadcrumb.md): 这东西长得和它名字不怎么像。
- [加载条 Loading Bar](https://www.naiveui.com/zh-CN/components/loading-bar.md): 焦虑的安慰剂，疗效尚可。
- [菜单 Menu](https://www.naiveui.com/zh-CN/components/menu.md): 没有吃的。
- [分页 Pagination](https://www.naiveui.com/zh-CN/components/pagination.md): 长数据之友。
- [步骤 Steps](https://www.naiveui.com/zh-CN/components/steps.md): 1、2、3...成了！
- [标签页 Tabs](https://www.naiveui.com/zh-CN/components/tabs.md): 在同一块区域切换内容。

## 反馈组件

- [警示信息 Alert](https://www.naiveui.com/zh-CN/components/alert.md): 根据我的经验，这东西使用最频繁的场景是让你关掉 AdBlocks。
- [标记 Badge](https://www.naiveui.com/zh-CN/components/badge.md): 我敢说你肯定见过这玩意而且知道怎么用它。
- [对话框 Dialog](https://www.naiveui.com/zh-CN/components/dialog.md): 执行之前，请确认。
- [抽屉 Drawer](https://www.naiveui.com/zh-CN/components/drawer.md): 我感觉和 Modal 功能差不太多，位置有点差别。
- [跑马灯 Marquee](https://www.naiveui.com/zh-CN/components/marquee.md): 我有一个高中同学，当时他的口头禅是：“滚滚滚。”
- [信息 Message](https://www.naiveui.com/zh-CN/components/message.md): （一般是）从浏览器顶部降下来的神谕。
- [模态框 Modal](https://www.naiveui.com/zh-CN/components/modal.md): 它会弹出来，然后给你看点东西。
- [通知 Notification](https://www.naiveui.com/zh-CN/components/notification.md): 通知某人。
- [弹出确认 Popconfirm](https://www.naiveui.com/zh-CN/components/popconfirm.md): 一个确认，弹出来的。
- [弹出信息 Popover](https://www.naiveui.com/zh-CN/components/popover.md): Popover 在内容周围弹出一些隐藏的信息。Popover 里面没什么内置样式，在里面填什么主要靠你了。
- [弹出选择 Popselect](https://www.naiveui.com/zh-CN/components/popselect.md): 如果你想选择一些数据，还不想看到那个框子，可以使用 Popselect。
- [进度 Progress](https://www.naiveui.com/zh-CN/components/progress.md): 不知道说点啥，但是还是得写点东西，让这个页面看起来舒展一点。
- [结果页 Result](https://www.naiveui.com/zh-CN/components/result.md): 结果页是展示结果的。
- [骨架屏 Skeleton](https://www.naiveui.com/zh-CN/components/skeleton.md): 可以闪的占位符。
- [加载 Spin](https://www.naiveui.com/zh-CN/components/spin.md): 按理说它叫做 Loading 也可以，但是为啥叫 Spin 呢？因为还有一个属性更少的内部组件叫 Loading。
- [弹出提示 Tooltip](https://www.naiveui.com/zh-CN/components/tooltip.md): 它走起来像一个 Popover，叫起来像一个 Popover，但是长得有点区别。

## 布局组件

- [弹性布局 Flex](https://www.naiveui.com/zh-CN/components/flex.md): 自 `2.37.0` 开始提供。
- [布局 Layout](https://www.naiveui.com/zh-CN/components/layout.md): 布局是用来布局的。
- [旧版栅格 Legacy Grid](https://www.naiveui.com/zh-CN/components/legacy-grid.md): 一个基本的栅格系统。
- [栅格 Grid](https://www.naiveui.com/zh-CN/components/grid.md): 基于 CSS Grid，响应式，远离 IE。
- [间距 Space](https://www.naiveui.com/zh-CN/components/space.md): 这是个伟大的发明（不是我发明的）。
- [面板分割 Split](https://www.naiveui.com/zh-CN/components/split.md): 灵活的布局工具，提供了用户自定义界面布局的可能性。

## 工具组件

- [折叠渐变 Collapse Transition](https://www.naiveui.com/zh-CN/components/collapse-transition.md): 一个没什么封装的 collapse item。
- [脱离上下文的 API（v2.29.0）](https://www.naiveui.com/zh-CN/components/discrete.md): 如果你想在 `setup` 外使用 `useDialog`、`useMessage`、`useNotification`、`useLoadingBar`，`useModal`，可以通过 `createDiscreteApi` 来构建对应的 API。
- [滚动条 Scrollbar](https://www.naiveui.com/zh-CN/components/scrollbar.md): 看起来好看点，但是我能保证这个没有原生滚动条可靠。
- [虚拟列表 Virtual List](https://www.naiveui.com/zh-CN/components/virtual-list.md): 当谈到虚拟列表时，能让你感觉列表像是无限长的，但实际上它只是在偷偷隐藏那些不可见的元素。

## 配置组件

- [全局化配置 Config Provider](https://www.naiveui.com/zh-CN/components/config-provider.md): 全局化配置设置内部组件的主题、语言和组件卸载于其他位置的 DOM 的类名。
- [元素 Element](https://www.naiveui.com/zh-CN/components/element.md): Element 上面有很多 Naive UI 提供的主题变量。
- [全局样式 Global Style](https://www.naiveui.com/zh-CN/components/global-style.md): 出于以下原因，你可能需要将某些样式设定在 `document.body` 上。

## 废弃的组件

- [旧版穿梭框 Legacy Transfer](https://www.naiveui.com/zh-CN/components/legacy-transfer.md): 左、右、左、右...像我这么无聊的人能玩一整天。

## Optional

- [CHANGELOG](https://www.naiveui.com/zh-CN/docs/changelog.md): `2026-08-21`
