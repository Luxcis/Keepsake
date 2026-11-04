<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { api, type Letter } from '@/api/client'

// 生日信编辑（计划 §2 生日信行）：标题/正文/揭晓开关；语音彩蛋的 COS 对象 Key 留待 COS 上传链路。
const letter = reactive({
  title: '',
  body: '',
  reveal_enabled: true,
  audio_key: '',
})
const updated = ref<string | null>(null)
const loadErr = ref<string | null>(null)
const message = ref<string | null>(null)

async function load() {
  loadErr.value = null
  try {
    const data = await api.get<Letter>('/api/letter')
    letter.title = data.title
    letter.body = data.body
    letter.reveal_enabled = data.reveal_enabled
    letter.audio_key = data.audio_key ?? ''
    updated.value = data.updated_at
  } catch (err) {
    // 404 = 信件尚未写（后台可写），其余照常呈现
    loadErr.value = null
  }
}
onMounted(load)

async function save() {
  if (!letter.title) {
    message.value = '请填写信件标题。'
    return
  }
  try {
    await api.put('/api/admin/letter', { ...letter })
    message.value = '已保存。'
    await load()
  } catch (err) {
    message.value = err instanceof Error ? err.message : '保存失败'
  }
}
</script>

<template>
  <section class="space-y-6">
    <p v-if="loadErr" class="text-xs text-clay">{{ loadErr }}</p>
    <p v-if="message" class="text-xs text-moss">{{ message }}</p>
    <p v-if="updated" class="text-[10px] tracking-[0.18em] text-muted">
      上次更新：{{ updated.slice(0, 10).replace(/-/g, '.') }}
    </p>

    <form class="bg-paper border border-line rounded-sm p-6 space-y-4" @submit.prevent="save">
      <label class="block">
        <span class="text-[10px] tracking-[0.2em] text-muted">标题</span>
        <input
          v-model="letter.title"
          type="text"
          maxlength="200"
          class="mt-1 w-full border border-line rounded-sm bg-white/70 px-3 py-2.5 text-sm"
        />
      </label>

      <label class="block">
        <span class="text-[10px] tracking-[0.2em] text-muted">正文（空行分段，前台按空行保留）</span>
        <textarea
          v-model="letter.body"
          rows="12"
          class="mt-1 w-full border border-line rounded-sm bg-white/70 px-3 py-2.5 text-sm leading-7"
        ></textarea>
      </label>

      <label class="flex items-center gap-2 text-xs text-muted">
        <input v-model="letter.reveal_enabled" type="checkbox" class="accent-clay" />
        前台先呈现信封，由收件人点击「拆开这封信」揭晓（关闭则直接展示正文）
      </label>

      <label class="block">
        <span class="text-[10px] tracking-[0.2em] text-muted">语音彩蛋（COS 对象 Key，可留空）</span>
        <input
          v-model="letter.audio_key"
          type="text"
          class="mt-1 w-full border border-line rounded-sm bg-white/70 px-3 py-2.5 text-sm"
        />
      </label>

      <button type="submit" class="bg-ink text-paper rounded-full px-6 py-3 text-xs tracking-wider hover:bg-moss transition">
        保存生日信
      </button>
    </form>
  </section>
</template>
