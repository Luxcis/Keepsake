<script setup lang="ts">
// 实况照片播放组件：封装 live-photo 的 LivePhotoViewer 生命周期。
// 卡片模式：不自动播放（避免网格里同时加载多条视频），桌面悬停播放、触屏长按播放；
// 灯箱模式：加载后自动静音播放，并用 object-fit: contain 保持完整画面。
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { LivePhotoViewer, type LivePhotoError } from 'live-photo'

const props = withDefaults(
  defineProps<{
    photoSrc: string
    videoSrc: string
    mode?: 'card' | 'lightbox'
  }>(),
  { mode: 'card' },
)

const emit = defineEmits<{
  'photo-load': []
  error: [type: string, message: string]
}>()

const host = ref<HTMLElement | null>(null)
let viewer: LivePhotoViewer | null = null

function destroy() {
  viewer?.destroy()
  viewer = null
}

// 卡片悬停播放：只在真正支持悬停的设备上启用，触屏继续走库自带的长按逻辑。
function canHover() {
  return window.matchMedia('(hover: hover) and (pointer: fine)').matches
}

function onHostEnter() {
  if (props.mode !== 'card' || !canHover()) return
  void viewer?.play().catch(() => {})
}

function onHostLeave() {
  if (props.mode !== 'card' || !canHover()) return
  viewer?.stop()
}

function mount() {
  destroy()
  const el = host.value
  if (!el) return
  const lightbox = props.mode === 'lightbox'
  try {
    viewer = new LivePhotoViewer({
      photoSrc: props.photoSrc,
      videoSrc: props.videoSrc,
      container: el,
      width: '100%',
      height: '100%',
      // 卡片交给悬停/长按触发；灯箱加载完立即播放。
      autoplay: lightbox,
      lazyLoadVideo: !lightbox,
      muted: true,
      locale: 'zh-CN',
      labels: { live: 'LIVE' },
      borderRadius: 0,
      imageCustomization: lightbox ? { styles: { objectFit: 'contain' } } : undefined,
      videoCustomization: lightbox ? { styles: { objectFit: 'contain' } } : undefined,
      onPhotoLoad: () => emit('photo-load'),
      onError: (err: LivePhotoError) => emit('error', err.type, err.message),
    })
  } catch (err) {
    emit('error', 'VALIDATION_ERROR', err instanceof Error ? err.message : '实况照片初始化失败')
  }
}

onMounted(mount)
watch(
  () => [props.photoSrc, props.videoSrc, props.mode],
  () => mount(),
)
onBeforeUnmount(destroy)
</script>

<template>
  <div
    ref="host"
    :class="mode === 'lightbox' ? 'relative w-full h-full' : 'absolute inset-0'"
    @mouseenter="onHostEnter"
    @mouseleave="onHostLeave"
  ></div>
</template>
