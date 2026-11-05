<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { api, isGateRequired, type Trip } from '@/api/client'
import PageState from '@/components/PageState.vue'
import GatePrompt from '@/components/GatePrompt.vue'
import { bindMotion } from '@/compositions/motion'

// 时光轴：由已发布旅行足迹自动生成，节点按出发日期从早到晚推进。
// 滚动叙事 —— 中轴线随滚动生长，交替节拍滑入（门禁降级优先；reduced-motion/加载失败 → 静态呈现）。
const trips = ref<Trip[]>([])
const loading = ref(true)
const error = ref<string | null>(null)
const gate = ref(false)
const root = ref<HTMLElement | null>(null)
let ctx: { revert(): void } | null = null

async function load() {
  loading.value = true
  error.value = null
  try {
    const list = await api.get<Trip[]>('/api/trips')
    // 公开列表按出发日期倒序；时光轴改为从早到晚，同日沿用后台维护顺序。
    trips.value = [...list].sort(
      (a, b) =>
        a.start_date.localeCompare(b.start_date) ||
        a.sort_order - b.sort_order ||
        a.id.localeCompare(b.id),
    )
  } catch (err) {
    if (isGateRequired(err)) {
      gate.value = true
    } else {
      error.value = err instanceof Error ? err.message : '加载失败'
    }
  }
  // 先解除 loading（事件块 v-if 依赖），DOM 出现后才绑动效
  loading.value = false
  if (!gate.value && !error.value) await nextTick(runMotion)
}

async function runMotion() {
  try {
    const bound = await bindMotion(root)
    if (!bound) return
    ctx?.revert()
    ctx = bound.modules.gsap.context(() => {
      // 中轴线：随滚动的 scrub 生长
      bound.modules.gsap.fromTo(
        '.timeline-line',
        { scaleY: 0, transformOrigin: 'top center' },
        {
          scaleY: 1,
          ease: 'none',
          scrollTrigger: { trigger: '.timeline-wrap', start: 'top 78%', end: 'bottom 70%', scrub: true },
        },
      )
      // 节点：进入视口时滑入
      bound.modules.gsap.utils.toArray<HTMLElement>('.reveal').forEach((el) => {
        bound.modules.gsap.fromTo(
          el,
          { y: 30, opacity: 0 },
          {
            y: 0,
            opacity: 1,
            duration: 0.8,
            ease: 'power2.out',
            scrollTrigger: { trigger: el, start: 'top 87%', once: true },
          },
        )
      })
    }, bound.el)
  } catch {
    // 计划 §2.1：动画编排失败不影响静态可用性
  }
}

function formatDate(date: string): string {
  return date.replace(/-/g, '.')
}

function formatDateRange(trip: Trip): string {
  const start = formatDate(trip.start_date)
  if (!trip.end_date) return start
  const end = formatDate(trip.end_date)
  return end === start ? start : `${start} – ${end}`
}

onMounted(load)
onBeforeUnmount(() => ctx?.revert())
</script>

<template>
  <section ref="root" class="pt-[74px]">
    <div class="max-w-5xl mx-auto px-6 md:px-12 py-20 md:py-24">
      <div class="text-center mb-16">
        <p class="eyebrow mb-4">OUR MEMORY TIMELINE</p>
        <h1 class="section-title text-3xl md:text-5xl">一起走过的日子</h1>
        <p class="text-sm text-muted mt-5">那些看似平凡，却一直记得的瞬间。</p>
      </div>

      <GatePrompt v-if="gate" hint="解锁后即可回看这些重要的日子。" @unlocked="gate = false;load()" />

      <template v-else>
        <PageState
          :loading="loading"
          :error="error"
          :empty="!loading && !error && trips.length === 0"
          empty-title="时光轴还在等第一段旅程。"
          empty-hint="记录并发布第一段旅行后，这里会自动出现对应的节点。"
          @retry="load"
        />

        <div v-if="!loading && !error && trips.length" class="timeline-wrap relative">
          <div
            class="timeline-line absolute left-[15px] md:left-1/2 top-0 bottom-0 w-px"
            aria-hidden="true"
          ></div>
          <div class="space-y-14">
            <RouterLink
              v-for="(trip, i) in trips"
              :key="trip.id"
              :to="`/trips/${trip.id}`"
              class="reveal group relative grid md:grid-cols-2 gap-7 md:gap-16 items-center"
            >
              <span
                class="absolute left-[9px] md:left-[calc(50%-6px)] top-2 w-3 h-3 rounded-full bg-clay ring-4 ring-paper-deep"
                aria-hidden="true"
              ></span>
              <div class="pl-12 md:pl-0" :class="i % 2 === 1 ? 'md:order-2' : 'md:text-right'">
                <p class="eyebrow mb-2">{{ formatDateRange(trip) }}</p>
                <h2 class="section-title text-2xl">{{ trip.title }}</h2>
                <p v-if="trip.summary" class="text-sm text-muted leading-7 mt-3">
                  {{ trip.summary }}
                </p>
                <span class="inline-flex items-center gap-1 text-xs tracking-wider text-clay mt-4">
                  查看这段旅程
                  <span aria-hidden="true" class="transition group-hover:translate-x-1">↗</span>
                </span>
              </div>
              <div class="pl-12 md:pl-0" :class="i % 2 === 1 ? 'md:order-1' : ''">
                <div class="polaroid max-w-[330px] mx-auto md:mx-0">
                  <div class="overflow-hidden">
                    <img
                      v-if="trip.cover_thumb_src"
                      :src="trip.cover_thumb_src"
                      :alt="trip.title"
                      loading="lazy"
                      class="w-full aspect-[4/3] object-cover transition duration-700 group-hover:scale-[1.03]"
                    />
                    <div v-else class="w-full aspect-[4/3] photo-card" aria-hidden="true"></div>
                  </div>
                </div>
              </div>
            </RouterLink>
          </div>
        </div>
      </template>
    </div>
  </section>
</template>

<style scoped>
.timeline-line {
  background: linear-gradient(to bottom, #d3c2a9, #d9ddd5, transparent);
}
.photo-card {
  background-image: linear-gradient(120deg, rgba(38, 60, 59, 0.18), rgba(181, 141, 98, 0.25)),
    linear-gradient(140deg, #a6b8a9, #d6d0c1 60%, #cbb08a);
}
</style>
