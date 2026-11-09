<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { api, type SiteSettings } from '@/api/client'

// 站点设置（计划 §4.2 配置行）：白名单字段 + site_password（哈希化，留空不变）。
const fields = reactive<SiteSettings>({
  site_title: '',
  hero_kicker: '',
  hero_title: '',
  hero_subtitle: '',
  hero_cta: '',
  site_motto: '',
})
const gateEnabled = ref(false)
const sitePassword = ref('') // 仅绑新值；空串不提交
// 开放日期（YYYY-MM-DD）：设置后，日期之前前台无法解锁；空串表示不限制。
const openDate = ref('')
// 未开放提示语：留空使用默认文案；{date} 会替换为开放日期。
const notOpenMessage = ref('')
const message = ref<string | null>(null)
const loadErr = ref<string | null>(null)

const META: { key: keyof SiteSettings; label: string; multiline?: boolean }[] = [
  { key: 'site_title', label: '站点标题（浏览器标签页）' },
  { key: 'hero_kicker', label: '首屏眉题' },
  { key: 'hero_title', label: '首屏主标题' },
  { key: 'hero_subtitle', label: '首屏副标题', multiline: true },
  { key: 'hero_cta', label: '首屏按钮文案' },
  { key: 'site_motto', label: '侧边座右铭' },
]

async function load() {
  loadErr.value = null
  try {
    const data = await api.get<{ settings: SiteSettings; gate_enabled: boolean }>(
      '/api/site/settings',
    )
    for (const key of Object.keys(fields) as (keyof SiteSettings)[]) {
      fields[key] = data.settings[key] ?? ''
    }
    gateEnabled.value = data.gate_enabled
    openDate.value = data.settings.open_date ?? ''
    notOpenMessage.value = data.settings.not_open_message ?? ''
  } catch (err) {
    loadErr.value = err instanceof Error ? err.message : '加载失败'
  }
}
onMounted(load)

async function save() {
  const body: Record<string, string> = {}
  for (const key of Object.keys(fields) as (keyof SiteSettings)[]) {
    body[key] = fields[key]
  }
  if (sitePassword.value.trim() !== '') body.site_password = sitePassword.value.trim()
  body.open_date = openDate.value
  body.not_open_message = notOpenMessage.value
  try {
    await api.put('/api/admin/site/settings', body)
    message.value =
      body.site_password === undefined
        ? '已保存站点文案。'
        : '已保存站点文案并启用/更新了访问口令。'
    sitePassword.value = ''
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

    <div class="text-xs text-muted space-y-1">
      <p>门禁状态：{{ gateEnabled ? '已启用（未输口令的访客无法阅读私人内容）' : '未启用（建议尽快设置口令）' }}</p>
      <p>口令未启用时，若已过开放日期（或未设置），前台内容对知道地址的所有人开放。</p>
      <p>开放日期：{{ openDate || '未设置（随时可解锁）' }}。日期未到时即使口令正确也无法解锁，管理员登录后仍可预览。</p>
    </div>

    <form class="bg-paper border border-line rounded-sm p-6 space-y-4" @submit.prevent="save">
      <div class="grid md:grid-cols-2 gap-4">
        <label v-for="m in META" :key="m.key" class="block">
          <span class="text-[10px] tracking-[0.2em] text-muted">{{ m.label }}</span>
          <textarea
            v-if="m.multiline"
            v-model="fields[m.key]"
            rows="2"
            class="mt-1 w-full border border-line rounded-sm bg-white/70 px-3 py-2.5 text-sm"
          ></textarea>
          <input
            v-else
            v-model="fields[m.key]"
            type="text"
            maxlength="2000"
            class="mt-1 w-full border border-line rounded-sm bg-white/70 px-3 py-2.5 text-sm"
          />
        </label>
      </div>

      <label class="block max-w-md">
        <span class="text-[10px] tracking-[0.2em] text-muted">
          {{ gateEnabled ? '新访问口令（留空保持不变）' : '设置访问口令（6–128 位）' }}
        </span>
        <input
          v-model="sitePassword"
          type="password"
          autocomplete="new-password"
          class="mt-1 w-full border border-line rounded-sm bg-white/70 px-3 py-2.5 text-sm"
        />
      </label>

      <label class="block max-w-md">
        <span class="text-[10px] tracking-[0.2em] text-muted">开放日期</span>
        <input
          v-model="openDate"
          type="date"
          class="mt-1 w-full border border-line rounded-sm bg-white/70 px-3 py-2.5 text-sm"
        />
        <span class="mt-1 block text-[10px] text-muted leading-5">
          开放日当天 00:00（北京时间）起可解锁；留空表示不限制。日期之前即使口令正确也会提示尚未开放。
        </span>
      </label>

      <label class="block">
        <span class="text-[10px] tracking-[0.2em] text-muted">未开放提示语（可选）</span>
        <textarea
          v-model="notOpenMessage"
          rows="2"
          maxlength="200"
          placeholder="还没到打开的时候哦。等到 {date}，这里就会为你打开，到时候再来输入口令吧。"
          class="mt-1 w-full border border-line rounded-sm bg-white/70 px-3 py-2.5 text-sm"
        ></textarea>
        <span class="mt-1 block text-[10px] text-muted leading-5">
          仅在设置开放日期后展示。留空使用默认文案；可用 {date} 插入开放日期，例如“等到 {date}，这里就会为你打开”。
        </span>
      </label>

      <button type="submit" class="bg-ink text-paper rounded-full px-6 py-3 text-xs tracking-wider hover:bg-moss transition">
        保存设置
      </button>
    </form>
  </section>
</template>
