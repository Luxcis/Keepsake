<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import {
  isGateRequired,
  api,
  type LightboxPhoto,
  type PhotoItem,
  type Trip,
  type TripLocation,
} from '@/api/client'
import PageState from '@/components/PageState.vue'
import GatePrompt from '@/components/GatePrompt.vue'
import PhotoLightbox from '@/components/PhotoLightbox.vue'
import LivePhotoMedia from '@/components/LivePhotoMedia.vue'
import { bindMotion } from '@/compositions/motion'

// 旅行详情（计划 §2）：故事、地点列表与照片；空字段优雅隐藏；门禁降级。
// 照片与单独上传的视频来自同一接口，混排网格 + 灯箱（实况照片与视频可播放）。
const route = useRoute()
const trip = ref<Trip | null>(null)
const locations = ref<TripLocation[]>([])
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
    const id = encodeURIComponent(String(route.params.id))
    const [detail, photoData] = await Promise.all([
      api.get<{ trip: Trip; locations: TripLocation[] }>(`/api/trips/${id}`),
      api.get<{ enabled: boolean; photos: PhotoItem[] }>(`/api/trips/${id}/photos`),
    ])
    trip.value = detail.trip
    locations.value = detail.locations
    cosEnabled.value = photoData.enabled
    photos.value = photoData.photos
  } catch (err) {
    if (isGateRequired(err)) {
      gate.value = true
      return
    }
    error.value = err instanceof Error ? err.message : '加载失败'
  } finally {
    loading.value = false
  }
  // 先解除 loading（网格 v-if 依赖）再等一帧 DOM，动效才绑得到卡片
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
      { y: 0, opacity: 1, duration: 0.7, ease: 'power2.out', stagger: 0.06 },
    )
  }, bound.el)
}

const showLocations = computed(() => locations.value.length > 0)
const showStory = computed(() => !!trip.value?.summary)

// 可放大查看的条目：照片优先用显示尺寸签名（原图可达 30MB，灯箱直连会让切换长时间停在上一张），
// 独立视频直接用签名后的视频地址。过滤掉无可用地址的条目后下标会与网格错位，
// 所以点击时按 id 反查灯箱下标。
const viewable = computed<LightboxPhoto[]>(() =>
  photos.value.flatMap((p): LightboxPhoto[] => {
    if (p.media_kind === 'video') {
      return p.video_src
        ? [{ id: p.id, src: p.video_src, mediaKind: 'video', caption: p.title ?? undefined }]
        : []
    }
    const display = p.preview_src || p.src
    return display
      ? [{ id: p.id, src: display, videoSrc: p.video_src || undefined, caption: p.title ?? undefined }]
      : []
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
    <div class="max-w-5xl mx-auto px-6 md:px-12 py-20">
      <p class="mb-6">
        <RouterLink to="/trips" class="text-xs tracking-widest text-muted hover:text-ink"
          >← 返回足迹</RouterLink
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

          <div class="mt-12">
            <p class="eyebrow mb-4">PHOTOGRAPHS</p>

            <p v-if="!loading && !error && photos.length === 0" class="text-xs text-muted">
              {{
                !cosEnabled
                  ? '照片存储尚未配置（服务端环境变量 COS_*），照片上传后即可在这里展示。'
                  : '这一次旅行还没有精选照片。'
              }}
            </p>

            <template v-else-if="photos.length">
              <div ref="gridRoot" class="grid grid-cols-2 md:grid-cols-3 gap-3 md:gap-5">
                <button
                  v-for="p in photos"
                  :key="p.id"
                  class="photo-card aspect-[4/5] rounded-sm group reveal"
                  :disabled="!p.src && !p.video_src"
                  @click="(p.src || p.video_src) && openPhoto(p)"
                >
                  <video
                    v-if="p.media_kind === 'video' && p.video_src"
                    :src="p.video_src"
                    preload="metadata"
                    muted
                    playsinline
                    class="w-full h-full object-cover"
                  ></video>
                  <LivePhotoMedia
                    v-else-if="p.video_src && p.thumb_src"
                    :photo-src="p.thumb_src"
                    :video-src="p.video_src"
                    mode="card"
                  />
                  <img
                    v-else-if="p.thumb_src"
                    :src="p.thumb_src"
                    :alt="p.title"
                    loading="lazy"
                    class="w-full h-full object-cover transition duration-700 group-hover:scale-[1.03]"
                  />
                  <div
                    v-else
                    class="w-full h-full flex items-center justify-center text-[10px] text-muted"
                  >
                    {{ p.media_kind === 'video' ? '视频地址签名失败' : '签名生成失败' }}
                  </div>
                  <span
                    v-if="p.media_kind === 'video'"
                    class="absolute top-3 left-3 z-20 flex h-8 w-8 items-center justify-center rounded-full bg-black/55 text-[11px] text-white"
                    aria-hidden="true"
                    >▶</span
                  >
                  <span
                    v-if="p.title"
                    class="absolute bottom-0 inset-x-0 z-20 p-4 text-left text-white bg-gradient-to-t from-black/55 to-transparent text-xs"
                    >{{ p.title }}</span
                  >
                </button>
              </div>

              <p class="text-center text-[10px] text-muted tracking-wider mt-8" aria-hidden="true">
                点击任意照片放大查看 · ESC 返回 · ← → 切换
              </p>
            </template>
          </div>
        </article>
      </template>
    </div>

    <PhotoLightbox v-model="lightboxIndex" :photos="viewable" />
  </section>
</template>
