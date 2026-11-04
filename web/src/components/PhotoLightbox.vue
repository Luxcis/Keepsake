<script setup lang="ts">
// 照片灯箱（计划 §2 照片墙验收点：点击全屏、前后切换、Esc 与方向键支持）。
// COS 签名链路上线后由照片墙/旅行详情直接使用；本组件不关心图片来源。
import { computed, onBeforeUnmount, watch } from 'vue'
import type { LightboxPhoto } from '@/api/client'

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
        class="fixed inset-0 z-50 bg-[#101715]/95 flex items-center justify-center p-5"
        role="dialog"
        aria-modal="true"
        aria-label="照片预览"
        @click.self="close"
      >
      <button class="absolute top-6 right-7 text-white text-3xl" aria-label="关闭" @click="close">
        ×
      </button>
      <button
        class="absolute left-4 md:left-10 text-white/80 text-3xl"
        aria-label="上一张"
        @click="step(-1)"
      >
        ‹
      </button>
      <div class="max-w-5xl w-full text-center">
        <img
          v-if="current"
          :src="current.src"
          :alt="current.caption ?? '照片预览'"
          class="max-h-[78vh] max-w-full mx-auto object-contain"
        />
        <p v-if="current?.caption" class="text-white/75 text-xs tracking-wider mt-5">
          {{ current.caption }}
        </p>
      </div>
      <button
        class="absolute right-4 md:right-10 text-white/80 text-3xl"
        aria-label="下一张"
        @click="step(1)"
      >
        ›
      </button>
      </div>
    </Transition>
  </Teleport>
</template>
