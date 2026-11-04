// 动效基础设施（计划 §2.1）：
// - GSAP/ScrollTrigger 动态加载，只用于有明确叙事价值的动效；
// - prefers-reduced-motion 或模块加载失败 → 返回 null，页面保持纯 CSS 静态可见；
// - 只有调用方拿到非 null 时才给根节点加 .motion-ready（此时 CSS 才隐藏待入场元素，
//   交由 GSAP 接管），确保「无动画或动画加载失败时主要功能仍可用」。
// - 视图卸载用 gsap.context(...).revert() 清理 ScrollTrigger。

import type { Ref } from 'vue'
import type { gsap as GSAP } from 'gsap'
import type { ScrollTrigger } from 'gsap/ScrollTrigger'

export type MotionModules = { gsap: typeof GSAP; ScrollTrigger: typeof ScrollTrigger }

let cached: MotionModules | null | undefined

export function prefersReducedMotion(): boolean {
  return window.matchMedia('(prefers-reduced-motion: reduce)').matches
}

// loadMotion 加载并注册插件；返回 null 表示本轮禁用动效。
export async function loadMotion(): Promise<MotionModules | null> {
  if (prefersReducedMotion()) return null
  if (cached !== undefined) return cached
  cached = null
  try {
    const [g, st] = await Promise.all([import('gsap'), import('gsap/ScrollTrigger')])
    const gsap = (g as { default: typeof GSAP }).default
    // Vite interop 下 default/named 视打包模式而定，做双保险解析。
    const stMod = st as { default?: typeof ScrollTrigger; ScrollTrigger?: typeof ScrollTrigger }
    const trigger = stMod.default ?? stMod.ScrollTrigger
    if (typeof trigger !== 'function') {
      throw new Error('gsap ScrollTrigger module shape unexpected')
    }
    gsap.registerPlugin(trigger)
    cached = { gsap, ScrollTrigger: trigger }
  } catch {
    cached = null // 计划 §2.1：动画加载失败完全降级
  }
  return cached
}

type RootArg = HTMLElement | null | Ref<HTMLElement | null>

// bindMotion：确认动效可用后给根元素打 motion-ready 标记，
// 返回 modules 与已解析的根元素（保证非空，方便 gsap.context(fn, el) 直接使用）。
export interface BoundMotion {
  modules: MotionModules
  el: HTMLElement
}

function resolveEl(root: RootArg): HTMLElement | null {
  if (!root) return null
  if ((root as Ref<HTMLElement | null>).value !== undefined) {
    const v = (root as Ref<HTMLElement | null>).value
    if (v instanceof HTMLElement) return v
    return null
  }
  return root instanceof HTMLElement ? root : null
}

export async function bindMotion(root: RootArg): Promise<BoundMotion | null> {
  const modules = await loadMotion()
  if (!modules) return null
  const el = resolveEl(root)
  if (!el) return null
  el.classList.add('motion-ready')
  return { modules, el }
}
