<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { api, isGateRequired, type Trip } from '@/api/client'
import PageState from '@/components/PageState.vue'
import GatePrompt from '@/components/GatePrompt.vue'
import { bindMotion } from '@/compositions/motion'

// 旅行足迹（计划 §2）：旅行卡片列表；门禁降级；照片卡错落入场（数据到达后触发）。
const trips = ref<Trip[]>([])
const loading = ref(true)
const error = ref<string | null>(null)
const gate = ref(false)
const gridRoot = ref<HTMLElement | null>(null)
let ctx: { revert(): void } | null = null

async function load() {
  loading.value = true
  error.value = null
  try {
    trips.value = await api.get<Trip[]>('/api/trips')
  } catch (err) {
    if (isGateRequired(err)) {
      gate.value = true
    } else {
      error.value = err instanceof Error ? err.message : '加载失败'
    }
  }
  loading.value = false
  if (!gate.value && !error.value) await nextTick(runEntrance)
}

async function runEntrance() {
  const bound = await bindMotion(gridRoot)
  if (!bound) return
  ctx?.revert()
  ctx = bound.modules.gsap.context(() => {
    bound.modules.gsap.fromTo(
      '.reveal',
      { y: 30, opacity: 0 },
      { y: 0, opacity: 1, duration: 0.75, ease: 'power2.out', stagger: 0.09 },
    )
  }, bound.el)
}

function formatDate(t: Trip): string {
  return t.start_date.replace(/-/g, '.')
}

onMounted(load)
onBeforeUnmount(() => ctx?.revert())
</script>

<template>
  <section class="pt-[74px]">
    <div class="max-w-7xl mx-auto px-6 md:px-12 py-20 md:py-24">
      <div class="flex flex-col md:flex-row md:items-end justify-between gap-6 mb-12">
        <div>
          <p class="eyebrow mb-4">PLACES WE'VE BEEN</p>
          <h1 class="section-title text-4xl md:text-5xl">我们的旅行足迹</h1>
        </div>
        <p class="text-sm text-ink-soft leading-7 max-w-sm">
          每一次出发，都成为了我们故事的一部分。<br />
          选一段旅程，再走一遍。
        </p>
      </div>

      <GatePrompt v-if="gate" hint="旅行是你们的小秘密，解锁后自动回到这一页。" @unlocked="gate = false;load()" />

      <template v-else>
        <PageState
          :loading="loading"
          :error="error"
          :empty="!loading && !error && trips.length === 0"
          empty-title="第一段旅程还没有被记录下来。"
          empty-hint="在管理后台录入你们的第一次旅行，这里就会出现一张张旅程卡片。"
          @retry="load"
        />

        <div v-if="!loading && !error && trips.length" ref="gridRoot" class="grid md:grid-cols-3 gap-5">
          <RouterLink
            v-for="(trip, i) in trips"
            :key="trip.id"
            :to="`/trips/${trip.id}`"
            class="group reveal"
          >
            <div class="photo-card aspect-[5/4] mb-5 rounded-sm">
              <div class="w-full h-full flex items-end p-5 text-xs tracking-widest">
                <span class="bg-paper/90 text-ink px-3 py-1.5">
                  {{ String(i + 1).padStart(2, '0') }}
                </span>
              </div>
            </div>
            <div class="flex justify-between items-start gap-4">
              <div>
                <p class="text-[10px] text-clay tracking-widest mb-2">{{ formatDate(trip) }}</p>
                <h2 class="section-title text-2xl">{{ trip.title }}</h2>
                <p class="text-xs text-muted mt-2 line-clamp-2">{{ trip.summary || '一段旅程' }}</p>
              </div>
              <span class="text-xl transition group-hover:translate-x-1" aria-hidden="true">↗</span>
            </div>
          </RouterLink>
        </div>
      </template>
    </div>
  </section>
</template>

<style scoped>
.photo-card > div {
  background:
    linear-gradient(150deg, rgba(38, 60, 59, 0.4), rgba(38, 60, 59, 0.05)),
    linear-gradient(130deg, #93a896, #cfc9ba 55%, #c4a578 90%);
}
</style>
