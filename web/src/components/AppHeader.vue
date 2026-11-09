<script setup lang="ts">
// 头部导航状态由 router-link-active 提供；旅行详情是 /trips 的兄弟路由，
// 需要手动把「足迹」标记为当前板块，避免子页无选中态。
import { computed } from 'vue'
import { useRoute } from 'vue-router'

const route = useRoute()
const tripsActive = computed(
  () => route.path === '/trips' || route.path.startsWith('/trips/'),
)
</script>

<template>
  <header
    class="fixed top-0 left-0 right-0 z-20 bg-paper/90 backdrop-blur-xl border-b border-line/70 pt-[env(safe-area-inset-top,0px)]"
  >
    <div
      class="edge-safe max-w-7xl mx-auto h-[var(--header-row-h)] flex items-center justify-between gap-3"
    >
      <RouterLink to="/" class="flex min-h-[44px] items-center gap-2.5 sm:gap-3 min-w-0" aria-label="回到首页">
        <span
          class="w-8 h-8 sm:w-9 sm:h-9 shrink-0 rounded-full border border-[#c8b99f] flex items-center justify-center text-clay text-sm sm:text-base"
          aria-hidden="true"
          >✳</span
        >
        <span class="min-w-0">
          <b class="block text-[10px] sm:text-[11px] tracking-[0.16em] sm:tracking-[0.2em] truncate"
            >KEEPSAKE · 拾光集</b
          >
          <small class="block text-[9px] sm:text-[10px] text-muted tracking-wider truncate"
            >A MEMORY BOOK · VOL. 01</small
          >
        </span>
      </RouterLink>

      <nav class="hidden md:flex items-center gap-6 lg:gap-8 text-xs tracking-wide" aria-label="主导航">
        <RouterLink class="nav-link" :class="{ 'router-link-active': tripsActive }" to="/trips">足迹</RouterLink>
        <RouterLink class="nav-link" to="/timeline">时光轴</RouterLink>
        <RouterLink class="nav-link" to="/letter">生日信</RouterLink>
      </nav>

      <RouterLink
        to="/letter"
        class="hidden md:inline-flex shrink-0 text-xs border border-[#c9b99f] px-4 py-2 rounded-full hover:bg-ink hover:text-paper transition"
        >写给你的信 ↗</RouterLink
      >
    </div>

    <!-- 移动端导航：四项等宽，整块可点（高 44px），选中态贴行底 -->
    <nav
      class="md:hidden grid grid-cols-4 items-stretch h-[var(--header-nav-h)] border-t border-line/60 text-xs"
      aria-label="移动端导航"
    >
      <RouterLink class="nav-tab" to="/">首页</RouterLink>
      <RouterLink class="nav-tab" :class="{ 'router-link-active': tripsActive }" to="/trips">足迹</RouterLink>
      <RouterLink class="nav-tab" to="/timeline">时光轴</RouterLink>
      <RouterLink class="nav-tab" to="/letter">生日信</RouterLink>
    </nav>
  </header>
</template>
