<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { isGateRequired, api, type Trip, type TripLocation } from '@/api/client'
import PageState from '@/components/PageState.vue'
import GatePrompt from '@/components/GatePrompt.vue'

// 旅行详情（计划 §2）：故事、地点列表与照片入口；空字段优雅隐藏；门禁降级。
const route = useRoute()
const trip = ref<Trip | null>(null)
const locations = ref<TripLocation[]>([])
const loading = ref(true)
const error = ref<string | null>(null)
const gate = ref(false)

async function load() {
  loading.value = true
  error.value = null
  try {
    const data = await api.get<{ trip: Trip; locations: TripLocation[] }>(
      `/api/trips/${encodeURIComponent(String(route.params.id))}`,
    )
    trip.value = data.trip
    locations.value = data.locations
  } catch (err) {
    if (isGateRequired(err)) {
      gate.value = true
      return
    }
    error.value = err instanceof Error ? err.message : '加载失败'
  } finally {
    loading.value = false
  }
}

const showLocations = computed(() => locations.value.length > 0)
const showStory = computed(() => !!trip.value?.summary)

onMounted(load)
</script>

<template>
  <section class="pt-[74px]">
    <div class="max-w-5xl mx-auto px-6 md:px-12 py-20">
      <p class="mb-6">
        <RouterLink to="/trips" class="text-xs tracking-widest text-muted hover:text-ink"
          >← 返回旅行足迹</RouterLink
        >
      </p>

      <GatePrompt v-if="gate" hint="解锁后即可查看旅程、地点和照片。" @unlocked="gate = false;load()" />

      <template v-else>
        <PageState
          :loading="loading"
          :error="error"
          :empty="!loading && !error && !trip"
          empty-title="没有找到这段旅程。"
          empty-hint="旅程可能还没有发布，或者地址已经改变。"
          @retry="load"
        />

        <article v-if="trip" class="bg-paper rounded-sm shadow-soft p-6 md:p-10">
          <p class="eyebrow mb-4">SELECTED JOURNEY · {{ trip.start_date }}</p>
          <h1 class="section-title text-3xl md:text-5xl mb-6">{{ trip.title }}</h1>

          <p v-if="showStory" class="text-sm leading-8 text-ink-soft mb-10">{{ trip.summary }}</p>

          <div>
            <p class="eyebrow mb-4">PLACES</p>
            <ol v-if="showLocations" class="space-y-4">
              <li
                v-for="(loc, i) in locations"
                :key="loc.id"
                class="flex items-start gap-4 border-b border-line pb-4"
              >
                <span class="text-clay section-title text-lg">{{
                  String(i + 1).padStart(2, '0')
                }}</span>
                <div class="flex-1">
                  <h2 class="section-title text-lg">{{ loc.name }}</h2>
                  <p v-if="loc.note" class="text-xs text-muted mt-1 leading-6">{{ loc.note }}</p>
                  <p class="text-[10px] text-muted mt-1 tracking-wider">
                    <template v-if="loc.latitude !== null && loc.longitude !== null">
                      {{ loc.latitude.toFixed(4) }} / {{ loc.longitude.toFixed(4) }}
                    </template>
                    <template v-else-if="loc.address">{{ loc.address }}</template>
                    <template v-else>位置待补充</template>
                  </p>
                </div>
              </li>
            </ol>
            <p v-else class="text-xs text-muted">这一站的地点信息正在整理。</p>
          </div>

          <div class="mt-12 flex gap-4">
            <RouterLink
              to="/photos"
              class="text-xs border border-line rounded-full px-5 py-3 hover:bg-ink hover:text-paper transition"
              >查看这次旅行的照片 ↗</RouterLink
            >
          </div>
        </article>
      </template>
    </div>
  </section>
</template>
