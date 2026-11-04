---
name: naive-ui
description: Naive UI（Vue 3 组件库）文档参考与查阅方法。任何涉及 Naive UI 的开发任务都应使用本 Skill：查询组件的 props/事件/slot/示例源码、编写或修改 n-xxx 组件代码、主题定制（n-config-provider）、按需引入、SSR/Nuxt 配置等。即使用户没有明说 "naive-ui"，只要在 Vue 项目里出现 n-button、n-data-table、n-form 等 n- 前缀组件或提到 naive 就应加载。
---

# Naive UI 文档查阅

本 Skill 提供两条查 Naive UI 文档的路径，按序选用，不要凭记忆编写组件 API 代码——props/事件/slot 必须以文档为准。

文档快照打包在本 Skill 目录内（相对路径基于本 Skill 目录）：

- `references/llms.txt` — 在线索引快照（17KB）：全部文档页 URL 及中文组件名到 slug 的映射，先查它再抓具体页面。
- `references/llms-full.txt` — 完整文档快照（约 1.1MB，116 个页面）：全部介绍、指南和组件页（含 API 与示例源码）合并版。

快照于 2026-11-03 抓取（对应 changelog `2026-08-21`）。

## 查阅路径

### 路径 1（首选）：在线抓取单个页面

需要查某个组件或某个指南时，直接抓该页的 Markdown 版本（文档站每页都有 `.md` 后缀镜像）：

```bash
curl -sL https://www.naiveui.com/zh-CN/components/data-table.md
curl -sL https://www.naiveui.com/zh-CN/docs/customize-theme.md
```

组件页 URL 规则：`https://www.naiveui.com/zh-CN/components/<slug>.md`，slug 为英文 kebab-case（如 `input-number`、`tree-select`、`auto-complete`）。中文组件名找不到对应 slug 时，先在 `references/llms.txt` 里查映射。

在线文档总能反映最新 API，除非网络不可用否则优先走本路径。

### 路径 2（离线兜底）：grep 打包快照

在线抓取失败、超时或无网络时，使用本地快照。`llms-full.txt` 里每个页面以 `<!-- Source: <原始URL> -->` 注释分隔，先定位行号再按区间读取：

```bash
# 1. 定位目标页面（及下一个页面开头，用于确定区间结束）
grep -n 'Source: https://www.naiveui.com/zh-CN/components/button.md' references/llms-full.txt
grep -n '<!-- Source:' references/llms-full.txt   # 列出全部 116 个页面起始行号
```

拿到行号后用 Read 从该 offset 读到下一个 `<!-- Source:` 行之前，不要整读全文件。

## 使用要点

- 组件页包含完整 Props/Events/Slots/Methods API 表和每个示例的源码，写代码前先抓目标组件页；组合使用多个组件时逐个抓取。
- 主题定制、国际化、discrete API（在 `setup` 外使用 `useDialog` 等）属于指南/工具组件页，不在某个具体组件页里，注意根据 `references/llms.txt` 的分类找对页面。
- 涉及新版本 API 的判断以在线文档为准（快照可能滞后于线上）。
- 英文文档同构：`https://www.naiveui.com/llms.txt`。
