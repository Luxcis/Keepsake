<script setup lang="ts">
// 照片灯箱（计划 §2 照片墙验收点：点击全屏、前后切换、Esc 与方向键支持）。
// COS 签名链路上线后由照片墙/旅行详情直接使用；本组件不关心图片来源。
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import type { LightboxPhoto } from '@/api/client'
import LivePhotoMedia from '@/components/LivePhotoMedia.vue'

const props = defineProps<{
  photos: LightboxPhoto[]
  // 未选中任何一张时传 null（关闭态）
  modelValue: number | null
}>()

const emit = defineEmits<{ 'update:modelValue': [value: number | null] }>()

const open = computed(() => props.modelValue !== null)
const current = computed(() =>
  props.modelValue !== null ? props.photos[props.modelValue] ?? null : null,
)

const loading = ref(false)
const failed = ref(false)
// 当前是否为独立视频（src 即视频地址）；单独收敛类型，模板里直接用非空字段。
const videoCurrent = computed(() => {
  const p = current.value
  return p?.mediaKind === 'video' ? p : null
})
// 当前照片是否为实况（有视频地址）；单独收敛类型，模板里直接用非空字段。
const liveCurrent = computed(() => {
  const p = current.value
  return p?.mediaKind !== 'video' && p?.videoSrc
    ? { id: p.id, src: p.src, videoSrc: p.videoSrc }
    : null
})
// 已预加载的 URL（相邻图各预取一次即可，切回来时走浏览器缓存）
const prefetched = new Set<string>()

// 快速连按时上一张图可能才刚失败/完成，迟到的事件不能污染当前这张的状态。
function isStale(e: Event) {
  const el = e.currentTarget as HTMLImageElement | null
  return !el || el.getAttribute('src') !== current.value?.src
}

function onLoad(e: Event) {
  if (isStale(e)) return
  loading.value = false
}

function onError(e: Event) {
  if (isStale(e)) return
  loading.value = false
  failed.value = true
}

// 实况照片：图片加载成功即撤掉“加载中”；只有图片本身失败才展示失败提示，
// 视频失败由库回退到静态图，不阻断浏览。
function onLivePhotoLoad() {
  loading.value = false
}

function onLiveError(type: string) {
  if (type === 'PHOTO_LOAD_ERROR' || type === 'VALIDATION_ERROR') {
    loading.value = false
    failed.value = true
  }
}

// 切换照片后立刻清掉上一张的残影：浏览器会保留旧帧直到新图解码完成，
// 原图较大时看起来就成了“只换了名字、照片没换”。配合 img 的 :key 换掉整个元素，
// 加载期间明确显示“加载中”。
watch(
  () => current.value?.src,
  (src) => {
    loading.value = !!src
    failed.value = false
    if (src) prefetchNeighbors()
  },
  { immediate: true },
)

// 预取左右相邻的照片，让方向键切换命中缓存而不是重新下载。
function prefetchNeighbors() {
  const total = props.photos.length
  const at = props.modelValue
  if (total < 2 || at === null) return
  for (const delta of [1, -1]) {
    const neighbor = props.photos[(at + delta + total) % total]
    // 视频体积远大于图片，不预取，切换时按需加载。
    if (!neighbor?.src || neighbor.mediaKind === 'video' || prefetched.has(neighbor.src)) continue
    prefetched.add(neighbor.src)
    const img = new Image()
    img.decoding = 'async'
    img.src = neighbor.src
  }
}

function close() {
  emit('update:modelValue', null)
}

function step(delta: 1 | -1) {
  if (!props.photos.length || props.modelValue === null) return
  const next = (props.modelValue + delta + props.photos.length) % props.photos.length
  emit('update:modelValue', next)
}

function onKey(e: KeyboardEvent) {
  if (!open.value) return
  switch (e.key) {
    case 'Escape':
      close()
      break
    case 'ArrowRight':
      step(1)
      break
    case 'ArrowLeft':
      step(-1)
      break
  }
}

watch(
  open,
  (isOpen) => {
    if (isOpen) {
      document.body.style.overflow = 'hidden'
      document.addEventListener('keydown', onKey)
    } else {
      document.body.style.overflow = ''
      document.removeEventListener('keydown', onKey)
    }
  },
  { immediate: true },
)

onBeforeUnmount(() => {
  document.body.style.overflow = ''
  document.removeEventListener('keydown', onKey)
})

// prefers-reduced-motion：wrapper 上已经有全局降级，动效自动失效
</script>

<template>
  <Teleport to="body">
    <Transition name="lb">
      <div
        v-if="open"
        class="lightbox-safe fixed inset-0 z-50 bg-[#101715]/95 flex items-center justify-center p-4 sm:p-5"
        role="dialog"
        aria-modal="true"
        aria-label="照片预览"
        @click.self="close"
      >
      <button class="lb-close" aria-label="关闭" @click="close">×</button>
      <button class="lb-nav lb-prev" aria-label="上一张" @click="step(-1)">‹</button>
      <div class="max-w-5xl w-full text-center">
        <div class="min-h-[40vh] flex flex-col items-center justify-center">
          <video
            v-if="videoCurrent && !failed"
            :key="videoCurrent.id ?? videoCurrent.src"
            :src="videoCurrent.src"
            controls
            autoplay
            muted
            playsinline
            class="lb-media max-w-full mx-auto object-contain"
            @loadeddata="onLoad"
            @error="onError"
          ></video>
          <div v-else-if="liveCurrent && !failed" class="lb-live w-full">
            <LivePhotoMedia
              :key="liveCurrent.id ?? liveCurrent.src"
              :photo-src="liveCurrent.src"
              :video-src="liveCurrent.videoSrc"
              mode="lightbox"
              @photo-load="onLivePhotoLoad"
              @error="onLiveError"
            />
          </div>
          <img
            v-else-if="current && !failed"
            :key="current.src"
            :src="current.src"
            :alt="current.caption ?? '照片预览'"
            class="lb-media max-w-full mx-auto object-contain"
            @load="onLoad"
            @error="onError"
          />
          <p v-if="loading" class="text-white/55 text-xs tracking-widest mt-4">加载中…</p>
          <p v-else-if="failed" class="text-white/70 text-xs tracking-wider mt-4">
            内容加载失败，刷新页面后再试一次。
          </p>
        </div>
        <p v-if="current?.caption" class="text-white/75 text-xs tracking-wider mt-4 sm:mt-5 px-2">
          {{ current.caption }}
        </p>
      </div>
      <button class="lb-nav lb-next" aria-label="下一张" @click="step(1)">›</button>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
/* 手机端：内容按可用视口高度缩放，左右按钮贴边、不压住画面中心。 */
.lb-media {
  max-height: calc(100vh - 9rem);
  max-height: calc(100dvh - 9rem);
}
.lb-live {
  height: calc(100vh - 9rem);
  height: calc(100dvh - 9rem);
}
@media (min-width: 640px) {
  .lb-media {
    max-height: 78vh;
  }
  .lb-live {
    height: 78vh;
  }
}

.lightbox-safe {
  padding-top: max(1rem, env(safe-area-inset-top, 0px));
  padding-bottom: max(1rem, env(safe-area-inset-bottom, 0px));
}

.lb-close,
.lb-nav {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 44px;
  height: 44px;
  border-radius: 9999px;
  background: rgba(0, 0, 0, 0.42);
  color: rgba(255, 255, 255, 0.9);
  font-size: 1.5rem;
  line-height: 1;
  transition: background 0.2s ease;
}
.lb-close:hover,
.lb-nav:hover {
  background: rgba(0, 0, 0, 0.62);
}
.lb-close {
  position: absolute;
  top: calc(env(safe-area-inset-top, 0px) + 0.75rem);
  right: calc(env(safe-area-inset-right, 0px) + 0.75rem);
  font-size: 1.75rem;
}
.lb-nav {
  position: absolute;
  top: 50%;
  transform: translateY(-50%);
}
.lb-prev {
  left: calc(env(safe-area-inset-left, 0px) + 0.5rem);
}
.lb-next {
  right: calc(env(safe-area-inset-right, 0px) + 0.5rem);
}
@media (min-width: 640px) {
  .lb-close {
    top: calc(env(safe-area-inset-top, 0px) + 1.5rem);
    right: calc(env(safe-area-inset-right, 0px) + 1.75rem);
  }
  .lb-prev {
    left: calc(env(safe-area-inset-left, 0px) + 1rem);
  }
  .lb-next {
    right: calc(env(safe-area-inset-right, 0px) + 1rem);
  }
}
@media (min-width: 768px) {
  .lb-prev {
    left: calc(env(safe-area-inset-left, 0px) + 2.5rem);
  }
  .lb-next {
    right: calc(env(safe-area-inset-right, 0px) + 2.5rem);
  }
}
</style>
