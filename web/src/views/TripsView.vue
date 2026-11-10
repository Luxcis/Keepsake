<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { api, isGateRequired, type Trip } from '@/api/client'
import PageState from '@/components/PageState.vue'
import GatePrompt from '@/components/GatePrompt.vue'
import { bindMotion } from '@/compositions/motion'

// 足迹（计划 §2）：旅行卡片列表；门禁降级；照片卡错落入场（数据到达后触发）。
const trips = ref<Trip[]>([])
// 置顶旅行只在时光轴顶部展示，不进入足迹卡片列表。
const visibleTrips = computed(() => trips.value.filter((t) => !t.pinned))
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
  <section>
    <div class="max-w-7xl mx-auto px-5 sm:px-6 md:px-12 py-16 sm:py-20 md:py-24">
      <div class="flex flex-col md:flex-row md:items-end justify-between gap-6 mb-12">
        <div>
          <p class="eyebrow mb-4">PLACES WE'VE BEEN</p>
          <h1 class="section-title text-3xl sm:text-4xl md:text-5xl">我们的足迹</h1>
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
          :empty="!loading && !error && visibleTrips.length === 0"
          empty-title="第一段旅程还没有被记录下来。"
          empty-hint="在管理后台录入你们的第一次旅行，这里就会出现一张张旅程卡片。"
          @retry="load"
        />

        <div v-if="!loading && !error && visibleTrips.length" ref="gridRoot" class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-5 md:gap-6">
          <RouterLink
            v-for="(trip, i) in visibleTrips"
            :key="trip.id"
            :to="`/trips/${trip.id}`"
            class="group reveal"
          >
            <div class="photo-card aspect-[5/4] mb-5 rounded-sm">
              <!-- 封面：签名的缩略图 URL（后台选择后返回）；无封面回退渐变占位 -->
              <img
                v-if="trip.cover_thumb_src"
                :src="trip.cover_thumb_src"
                :alt="trip.title"
                loading="lazy"
                class="w-full h-full object-cover transition duration-700 group-hover:scale-[1.03]"
              />
              <div v-else class="placeholder absolute inset-0" aria-hidden="true"></div>
              <div class="absolute inset-0 flex items-end p-5 text-xs tracking-widest pointer-events-none">
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
.placeholder {
  background:
    linear-gradient(150deg, rgba(38, 60, 59, 0.4), rgba(38, 60, 59, 0.05)),
    linear-gradient(130deg, #93a896, #cfc9ba 55%, #c4a578 90%);
}
</style>
