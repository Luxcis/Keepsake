import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api, type SiteSettings } from '@/api/client'

// 站点级状态：从 /api/site/settings 拉取白名单配置。
// 默认值与后端种子数据（0001_init.sql）保持一致，接口异常时页面仍可呈现。
const DEFAULTS: SiteSettings = {
  site_title: '我们的旅行纪念册',
  hero_kicker: 'TO MY FAVORITE PERSON · 2026.11.22',
  hero_title: '和你走过的每一站。',
  hero_subtitle:
    '有些风景值得收藏，有些瞬间，因为有你才变得特别。这一次，想邀请你重新走一遍我们的故事。',
  hero_cta: '开启我们的故事',
  site_motto: 'A JOURNEY THROUGH US',
}

interface SiteState {
  settings: SiteSettings
  loaded: boolean
  loadError: string | null
}

export const useSiteStore = defineStore('site', () => {
  const state = ref<SiteState>({
    settings: { ...DEFAULTS },
    loaded: false,
    loadError: null,
  })

  async function load(force = false) {
    if (state.value.loaded && !force) return
    try {
      // 响应形如 { data: { settings: {...}, gate_enabled: bool } }
      const payload = await api.get<{ settings: SiteSettings; gate_enabled: boolean }>(
        '/api/site/settings',
      )
      state.value = { settings: { ...DEFAULTS, ...payload.settings }, loaded: true, loadError: null }
      void payload.gate_enabled // 首页不消费；解锁体验由页面级行为决定
    } catch (err) {
      // 公开配置失败不阻断浏览（计划 §2.1：主要功能必须在异常时仍可用）
      state.value = { ...state.value, loadError: err instanceof Error ? err.message : '加载失败' }
    }
  }

  function reset() {
    state.value = { settings: { ...DEFAULTS }, loaded: false, loadError: null }
  }

  return { state, load, reset }
})
