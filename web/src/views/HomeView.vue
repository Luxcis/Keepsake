<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { useSiteStore } from '@/stores/site'
import { bindMotion } from '@/compositions/motion'

// 首页（计划 §2）：主视觉 + 祝福 + 通往足迹的 CTA。
// 动效：首屏文案渐入（GSAP 动态加载；reduced-motion/加载失败 → CSS 静态呈现）。
const site = useSiteStore()
const root = ref<HTMLElement | null>(null)
let ctx: { revert(): void } | null = null

onMounted(async () => {
  const bound = await bindMotion(root)
  if (!bound) return
  ctx = bound.modules.gsap.context(() => {
    bound.modules.gsap.fromTo(
      '.hero-anim',
      { y: 26, opacity: 0 },
      {
        y: 0,
        opacity: 1,
        duration: 0.9,
        stagger: 0.12,
        ease: 'power3.out',
        delay: 0.1,
      },
    )
    bound.modules.gsap.fromTo(
      '.deco-anim',
      { y: 26, opacity: 0 },
      {
        y: 0,
        opacity: 1,
        duration: 0.8,
        stagger: 0.12,
        ease: 'power2.out',
        scrollTrigger: { trigger: '.deco-wrap', start: 'top 86%', once: true },
      },
    )
  }, bound.el)
})

onBeforeUnmount(() => ctx?.revert())
</script>

<template>
  <section ref="root" class="pt-[74px]">
    <div class="hero relative min-h-[620px] md:min-h-[680px] flex items-end">
      <div class="hero-canvas absolute inset-0" aria-hidden="true"></div>
      <div class="max-w-7xl mx-auto w-full px-6 md:px-12 pb-16 md:pb-24 text-[#f8f4eb] relative">
        <div class="max-w-2xl">
          <p class="eyebrow !text-[#e5c9a4] mb-6 hero-anim">{{ site.state.settings.hero_kicker }}</p>
          <h1
            class="section-title text-[clamp(2rem,9vw,2.85rem)] md:text-6xl lg:text-7xl leading-[1.15] md:leading-[1.12] mb-6 hero-anim"
          >
            {{ site.state.settings.hero_title }}
          </h1>
          <p class="text-sm md:text-base text-white/80 leading-8 max-w-lg hero-anim">
            {{ site.state.settings.hero_subtitle }}
          </p>
          <div class="mt-10 flex flex-wrap items-center gap-5 hero-anim">
            <RouterLink
              to="/trips"
              class="bg-[#f4efe5] text-ink px-6 py-3 rounded-full text-xs tracking-wider hover:bg-white transition"
            >
              {{ site.state.settings.hero_cta }} <span class="ml-3" aria-hidden="true">↓</span>
            </RouterLink>
            <span class="text-[10px] tracking-[0.22em] text-white/60" aria-hidden="true">
              PLACES · MOMENTS · MEMORIES
            </span>
          </div>
        </div>
      </div>
      <div
        class="absolute right-8 bottom-10 hidden md:block text-white/70 text-[10px] tracking-[0.2em] [writing-mode:vertical-rl]"
        aria-hidden="true"
      >
        {{ site.state.settings.site_motto }}
      </div>
    </div>

    <div
      class="max-w-7xl mx-auto px-6 md:px-12 py-7 flex justify-between items-center text-[10px] tracking-[0.18em] text-muted"
    >
      <span>OUR STORY · 202X — 2026</span>
      <span aria-hidden="true">SCROLL TO EXPLORE ↓</span>
    </div>

    <!-- 简介 + 足迹入口（脚手架静态版） -->
    <section class="deco-wrap max-w-7xl mx-auto px-6 md:px-12 py-20 grid md:grid-cols-2 gap-12 items-center">
      <div class="deco-anim">
        <p class="eyebrow mb-5">A NOTE FOR YOU</p>
        <h2 class="section-title text-3xl md:text-5xl leading-tight mb-7">
          把一起走过的<br />
          <span class="text-clay italic">日子，收藏起来。</span>
        </h2>
        <p class="text-sm text-ink-soft leading-8">
          我把一起看过的风景、走过的路、拍下的照片，都整理在这里。希望你打开它的时候，能重新感受到那些快乐的瞬间。
        </p>
      </div>
      <div class="deco-anim grid grid-cols-5 gap-3 items-center">
        <div class="col-span-3 photo-card rounded-sm aspect-[4/5] hero-deco-1"></div>
        <div class="col-span-2 space-y-3">
          <div class="photo-card aspect-[4/5] rounded-sm hero-deco-2"></div>
          <div class="bg-[#e8e3d8] p-4 md:p-5">
            <span class="eyebrow">LITTLE MOMENTS</span>
            <p class="section-title text-xl mt-2">平凡日子<br />也闪闪发光。</p>
          </div>
        </div>
      </div>
    </section>
  </section>
</template>

<style scoped>
.hero-canvas {
  background:
    linear-gradient(160deg, rgba(19, 35, 33, 0.72), rgba(19, 35, 33, 0.08) 65%),
    linear-gradient(135deg, #3d5348 0%, #4f6a5d 38%, #b58d62 78%, #d9c097 100%);
}
.photo-card.hero-deco-1 {
  background-image: linear-gradient(150deg, rgba(38, 60, 59, 0.35), rgba(38, 60, 59, 0.08)),
    linear-gradient(120deg, #9db3a4, #d8d3c4 70%, #cbb08a);
}
.photo-card.hero-deco-2 {
  background-image: linear-gradient(120deg, rgba(255, 253, 248, 0.35), rgba(181, 141, 98, 0.4)),
    linear-gradient(160deg, #b9a487, #8ba08f 80%);
}
</style>
