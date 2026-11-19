<script setup lang="ts">
// 未开放提示弹窗：生日信在开放日之前展示，沿用原门禁卡片的弹窗样式。
// 支持点击遮罩、按 Esc 或「我知道了」关闭；关闭后由使用方决定页面占位。
import { onBeforeUnmount, onMounted, ref } from 'vue'

defineProps<{ message: string }>()
const emit = defineEmits<{ close: [] }>()
const closeButton = ref<HTMLButtonElement | null>(null)

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') emit('close')
}

onMounted(() => {
  window.addEventListener('keydown', onKeydown)
  closeButton.value?.focus()
})
onBeforeUnmount(() => window.removeEventListener('keydown', onKeydown))
</script>

<template>
  <Teleport to="body">
    <div
      class="fixed inset-0 z-50 flex items-center justify-center bg-[#101715]/60 p-4 sm:p-5"
      @click.self="emit('close')"
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="not-open-title"
        class="w-full max-w-sm bg-paper rounded-sm shadow-soft p-6 sm:p-8 text-center"
      >
        <p class="eyebrow mb-3">NOT YET</p>
        <h2 id="not-open-title" class="section-title text-xl mb-3">还没到打开的时候</h2>
        <p class="text-xs text-muted leading-6">{{ message }}</p>
        <button
          ref="closeButton"
          type="button"
          class="mt-6 min-h-[44px] rounded-full bg-ink px-6 py-3 text-xs tracking-wider text-paper transition hover:bg-moss"
          @click="emit('close')"
        >
          我知道了
        </button>
      </div>
    </div>
  </Teleport>
</template>
