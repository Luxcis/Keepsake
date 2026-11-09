<script setup lang="ts">
import { computed } from 'vue'
import type { Letter } from '@/api/client'

// 信纸正文（计划 §4.2）：拆信动画中的折叠信纸与最终阅读版式共用同一份内容，
// 折叠态只做裁切与位移，不另写一套排版，保证展开前后文字不会重新排版。
const props = defineProps<{ letter: Letter }>()

const paragraphs = computed<string[]>(() =>
  props.letter.body ? props.letter.body.split('\n').filter((line) => line.trim() !== '') : [],
)
</script>

<template>
  <article class="letter-paper w-full p-5 sm:p-7 md:p-10">
    <p class="eyebrow mb-4">DEAR YOU,</p>
    <h2 class="section-title text-2xl mb-5">{{ letter.title }}</h2>

    <div class="space-y-4 text-sm leading-8 text-[#68685f]">
      <p v-for="(para, i) in paragraphs" :key="i">{{ para }}</p>
      <p v-if="!paragraphs.length" class="text-muted">(信纸留白中)</p>
    </div>

    <div class="border-t border-[#ddd3c2] mt-7 pt-5 flex flex-wrap items-center justify-between gap-2 text-right">
      <span class="text-[10px] tracking-[0.18em] text-muted">
        {{ letter.updated_at.slice(0, 10).replace(/-/g, '.') }}
      </span>
      <span class="text-xs text-muted">永远爱你的那个人</span>
    </div>
  </article>
</template>
