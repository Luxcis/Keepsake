<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { api, isGateRequired, type LightboxPhoto, type PhotoItem, type Trip } from '@/api/client'
import PhotoLightbox from '@/components/PhotoLightbox.vue'
import PageState from '@/components/PageState.vue'
import GatePrompt from '@/components/GatePrompt.vue'
import { bindMotion } from '@/compositions/motion'

// 照片墙（计划 §2）：按旅行分组的照片网格；点击全屏灯箱，Esc/方向键切换。
// 网格用缩略图（imageMogr2 处理参数），灯箱才载原图（§6.1 性能验收点）；
// 照片错落出现，数据到达后由 GSAP 触发，降级场景以静态网格直接呈现。
const trips = ref<Trip[]>([])
const tripId = ref('')
const photos = ref<PhotoItem[]>([])
const cosEnabled = ref(true)
const loading = ref(true)
const error = ref<string | null>(null)
const gate = ref(false)
const lightboxIndex = ref<number | null>(null)
const gridRoot = ref<HTMLElement | null>(null)
let ctx: { revert(): void } | null = null

async function load() {
  loading.value = true
  error.value = null
  try {
    if (trips.value.length === 0) {
      trips.value = await api.get<Trip[]>('/api/trips')
      tripId.value = trips.value[0]?.id ?? ''
    }
    if (tripId.value !== '') {
      const data = await api.get<{ enabled: boolean; photos: PhotoItem[] }>(
        `/api/trips/${tripId.value}/photos`,
      )
      cosEnabled.value = data.enabled
      photos.value = data.photos
    }
  } catch (err) {
    if (isGateRequired(err)) {
      gate.value = true
    } else {
      error.value = err instanceof Error ? err.message : '加载失败'
    }
  }
  // 先解除 loading（网格 v-if 依赖）再等一帧 DOM，动效才绑得到卡片
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
      { y: 26, opacity: 0 },
      {
        y: 0,
        opacity: 1,
        duration: 0.7,
        ease: 'power2.out',
        stagger: 0.06,
        scrollTrigger: { trigger: gridRoot.value ?? undefined, start: 'top 88%', once: true },
      },
    )
  }, bound.el)
}

function pickTrip(id: string) {
  tripId.value = id
  lightboxIndex.value = null
  void load()
}

// 可放大查看的照片：优先用显示尺寸签名（原图可达 30MB，灯箱直连会让切换长时间停在上一张）。
// 过滤掉无原图的条目后下标会与网格错位，所以点击时按 id 反查灯箱下标。
const viewable = computed<LightboxPhoto[]>(() =>
  photos.value.flatMap((p) => {
    const display = p.preview_src || p.src
    return display ? [{ id: p.id, src: display, caption: p.title ?? undefined }] : []
  }),
)

function openPhoto(p: PhotoItem) {
  const i = viewable.value.findIndex((x) => x.id === p.id)
  if (i >= 0) lightboxIndex.value = i
}

onMounted(load)
onBeforeUnmount(() => ctx?.revert())
</script>

<template>
  <section class="pt-[74px]">
    <div class="max-w-7xl mx-auto px-6 md:px-12 py-20 md:py-24">
      <div class="text-center mb-10">
        <p class="eyebrow mb-4">PHOTOGRAPHS &amp; MEMORIES</p>
        <h1 class="section-title text-4xl md:text-5xl">把瞬间，留在这里。</h1>
        <p class="text-sm text-muted mt-5">每一张照片，都藏着一段故事。</p>
      </div>

      <GatePrompt v-if="gate" hint="照片是私密的，解锁后即可翻阅。" @unlocked="gate = false;load()" />

      <template v-else>
        <PageState
          :loading="loading"
          :error="error"
          :empty="!loading && !error && trips.length === 0"
          empty-title="还没有可以翻阅的旅行。"
          empty-hint="在后台创建旅行并上传精选照片后，这里会出现照片墙。"
          @retry="load"
        />

        <template v-if="trips.length && !loading && !error">
          <div class="flex justify-center flex-wrap gap-2 mb-9">
            <button
              v-for="t in trips"
              :key="t.id"
              class="pill"
              :class="{ active: t.id === tripId }"
              @click="pickTrip(t.id)"
            >
              {{ t.title }}
            </button>
          </div>

          <p v-if="!cosEnabled && photos.length === 0" class="text-center text-xs text-muted py-10">
            对象存储尚未配置（服务端环境变量 COS_*），照片上传后即可在这里展示。
          </p>
          <p v-else-if="photos.length === 0" class="text-center text-xs text-muted py-10">
            这一次旅行还没有精选照片，去后台挑几张吧。
          </p>

          <div v-else ref="gridRoot" class="grid grid-cols-2 md:grid-cols-3 gap-3 md:gap-5">
            <button
              v-for="p in photos"
              :key="p.id"
              class="photo-card aspect-[4/5] rounded-sm group reveal"
              :disabled="!p.src"
              @click="p.src && openPhoto(p)"
            >
              <img
                v-if="p.thumb_src"
                :src="p.thumb_src"
                :alt="p.title"
                loading="lazy"
                class="w-full h-full object-cover transition duration-700 group-hover:scale-[1.03]"
              />
              <div v-else class="w-full h-full flex items-center justify-center text-[10px] text-muted">
                签名生成失败
              </div>
              <span
                v-if="p.title"
                class="absolute bottom-0 inset-x-0 p-4 text-left text-white bg-gradient-to-t from-black/55 to-transparent text-xs"
                >{{ p.title }}</span
              >
            </button>
          </div>

          <p class="text-center text-[10px] text-muted tracking-wider mt-8" aria-hidden="true">
            点击任意照片放大查看 · ESC 返回 · ← → 切换
          </p>
        </template>
      </template>
    </div>

    <PhotoLightbox v-model="lightboxIndex" :photos="viewable" />
  </section>
</template>
