<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { api, type TimelineEvent } from '@/api/client'

// 时光轴事件维护（计划 §4.2 管理端时光轴）。
const events = ref<TimelineEvent[]>([])
const loadErr = ref<string | null>(null)
const message = ref<string | null>(null)
const confirmDelete = ref('')

const form = reactive({ id: '', event_date: '', title: '', description: '', sort_order: 0 })

async function load() {
  loadErr.value = null
  try {
    events.value = await api.get<TimelineEvent[]>('/api/timeline')
  } catch (err) {
    loadErr.value = err instanceof Error ? err.message : '加载失败'
  }
}
onMounted(load)

function startCreate() {
  const today = new Date().toISOString().slice(0, 10)
  form.id = ''
  form.event_date = today
  form.title = ''
  form.description = ''
  form.sort_order = events.value.length
  message.value = null
}

function startEdit(e: TimelineEvent) {
  form.id = e.id
  form.event_date = e.event_date
  form.title = e.title
  form.description = e.description
  form.sort_order = e.sort_order
  message.value = null
}

async function save() {
  if (!form.title || !form.event_date) {
    message.value = '请补齐日期与标题。'
    return
  }
  try {
    if (form.id === '') {
      await api.post('/api/admin/timeline', { ...form })
      message.value = '已创建。'
    } else {
      await api.put(`/api/admin/timeline/${form.id}`, { ...form })
      message.value = '已保存。'
    }
    await load()
  } catch (err) {
    message.value = err instanceof Error ? err.message : '保存失败'
  }
}

async function remove(id: string) {
  if (confirmDelete.value !== id) {
    confirmDelete.value = id
    return
  }
  confirmDelete.value = ''
  try {
    await api.delete(`/api/admin/timeline/${id}`)
    if (form.id === id) startCreate()
    await load()
  } catch (err) {
    message.value = err instanceof Error ? err.message : '删除失败'
  }
}
</script>

<template>
  <section class="space-y-8">
    <p v-if="loadErr" class="text-xs text-clay">{{ loadErr }}</p>
    <p v-if="message" class="text-xs text-moss">{{ message }}</p>

    <form class="bg-paper border border-line rounded-sm p-6 space-y-4" @submit.prevent="save">
      <div class="flex items-center justify-between">
        <h2 class="section-title text-xl">{{ form.id === '' ? '添加重要时刻' : '编辑时刻' }}</h2>
        <button v-if="form.id" type="button" class="text-xs text-muted underline hover:text-ink" @click="startCreate">
          改为新增
        </button>
      </div>

      <div class="grid md:grid-cols-[1fr,2fr,auto] gap-4 items-end">
        <label><span class="text-[10px] tracking-[0.2em] text-muted">日期 *</span>
          <input v-model="form.event_date" type="date" class="mt-1 w-full border border-line rounded-sm bg-white/70 px-3 py-2.5 text-sm" /></label>
        <label><span class="text-[10px] tracking-[0.2em] text-muted">标题 *</span>
          <input v-model="form.title" type="text" maxlength="200" class="mt-1 w-full border border-line rounded-sm bg-white/70 px-3 py-2.5 text-sm" /></label>
        <label class="text-xs text-muted">排序 {{ form.sort_order }}</label>
      </div>
      <label class="block"><span class="text-[10px] tracking-[0.2em] text-muted">描述</span>
        <textarea v-model="form.description" rows="2" maxlength="2000" class="mt-1 w-full border border-line rounded-sm bg-white/70 px-3 py-2.5 text-sm"></textarea></label>

      <button type="submit" class="bg-ink text-paper rounded-full px-6 py-3 text-xs tracking-wider hover:bg-moss transition">
        {{ form.id === '' ? '添加' : '保存' }}
      </button>
    </form>

    <div class="space-y-3">
      <h2 class="section-title text-xl">全部时刻（{{ events.length }}）· 日期升序</h2>
      <p v-if="!events.length" class="text-xs text-muted">还没有记录，写下第一个重要时刻。</p>
      <article
        v-for="e in events"
        :key="e.id"
        class="border border-line rounded-sm bg-paper p-4 flex items-center justify-between gap-4"
      >
        <div>
          <p class="text-[10px] text-clay tracking-widest">{{ e.event_date.replace(/-/g, '.') }}</p>
          <h3 class="section-title text-lg">{{ e.title }}</h3>
          <p v-if="e.description" class="text-xs text-muted mt-1">{{ e.description }}</p>
        </div>
        <div class="flex items-center gap-2 text-xs shrink-0">
          <button class="border border-line rounded-full px-4 py-2 hover:bg-ink hover:text-paper transition" @click="startEdit(e)">编辑</button>
          <button
            class="rounded-full px-4 py-2 border transition"
            :class="confirmDelete === e.id ? 'bg-clay text-white border-clay' : 'border-line text-muted hover:text-clay'"
            @click="remove(e.id)"
          >
            {{ confirmDelete === e.id ? '确认删除？' : '删除' }}
          </button>
        </div>
      </article>
    </div>
  </section>
</template>
