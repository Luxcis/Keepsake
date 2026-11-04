<script setup lang="ts">
// 头部为纯展示组件：导航状态由 router-link-active 提供。
</script>

<template>
  <header
    class="fixed top-0 left-0 right-0 z-20 bg-paper/90 backdrop-blur-xl border-b border-line/70"
  >
    <div class="max-w-7xl mx-auto px-5 md:px-10 h-[74px] flex items-center justify-between">
      <RouterLink to="/" class="flex items-center gap-3" aria-label="回到首页">
        <span
          class="w-9 h-9 rounded-full border border-[#c8b99f] flex items-center justify-center text-clay"
          aria-hidden="true"
          >✳</span
        >
        <span>
          <b class="block text-[11px] tracking-[0.2em]">KEEPSAKE · 拾光集</b>
          <small class="text-[10px] text-muted tracking-wider">A MEMORY BOOK · VOL. 01</small>
        </span>
      </RouterLink>

      <nav class="hidden md:flex items-center gap-8 text-xs tracking-wide" aria-label="主导航">
        <RouterLink class="nav-link" to="/trips">旅行足迹</RouterLink>
        <RouterLink class="nav-link" to="/photos">照片收藏</RouterLink>
        <RouterLink class="nav-link" to="/timeline">时光轴</RouterLink>
        <RouterLink class="nav-link" to="/letter">生日信</RouterLink>
      </nav>

      <RouterLink
        to="/letter"
        class="hidden md:inline-flex text-xs border border-[#c9b99f] px-4 py-2 rounded-full hover:bg-ink hover:text-paper transition"
        >写给你的信 ↗</RouterLink
      >
    </div>

    <!-- 移动端导航 -->
    <nav class="md:hidden flex justify-around pb-3 text-xs" aria-label="移动端导航">
      <RouterLink class="nav-link" to="/">首页</RouterLink>
      <RouterLink class="nav-link" to="/trips">足迹</RouterLink>
      <RouterLink class="nav-link" to="/photos">照片</RouterLink>
      <RouterLink class="nav-link" to="/timeline">时光轴</RouterLink>
      <RouterLink class="nav-link" to="/letter">生日信</RouterLink>
    </nav>
  </header>
</template>
