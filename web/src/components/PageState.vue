<script setup lang="ts">
// 页面级加载/空态/错误提示，保证“优雅降级”（计划 §2 验收点：空字段优雅隐藏）。
defineProps<{
  loading?: boolean
  error?: string | null
  empty?: boolean
  emptyTitle?: string
  emptyHint?: string
}>()

const emit = defineEmits<{ retry: [] }>()
</script>

<template>
  <div v-if="loading" class="py-24 text-center text-xs tracking-[0.2em] text-muted">
    整理回忆中…
  </div>

  <div v-else-if="error" class="py-20 text-center">
    <p class="eyebrow mb-3">OH NO</p>
    <p class="section-title text-xl mb-2">这里的回忆暂时打不开了。</p>
    <p class="text-xs text-muted mb-5">{{ error }}</p>
    <button
      class="text-xs border border-line rounded-full px-5 py-2.5 hover:bg-ink hover:text-paper transition"
      @click="emit('retry')"
    >
      再试一次
    </button>
  </div>

  <div v-else-if="empty" class="py-20 text-center max-w-md mx-auto">
    <p class="eyebrow mb-3">COMING SOON</p>
    <p class="section-title text-xl mb-2">{{ emptyTitle ?? '这一页还空着。' }}</p>
    <p class="text-xs text-muted leading-6">{{ emptyHint ?? '内容正在准备中，请稍后再来看看。' }}</p>
  </div>
</template>
