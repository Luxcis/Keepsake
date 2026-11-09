<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import {
  api,
  ApiError,
  backupDownloadUrl,
  type BackupItem,
  type BackupProgress,
  type BackupStatus,
} from '@/api/client'

// 管理端一键备份：启动后台任务 → 轮询状态 → 下载/删除。
// 备份包含数据库与全部上传文件，但不含 .env、部署配置与 Redis 会话数据。
const status = ref<BackupStatus | null>(null)
const loadErr = ref<string | null>(null)
const actionErr = ref<string | null>(null)
const notice = ref<string | null>(null)
const busy = ref(false)

let pollTimer: number | null = null
let stopped = false

const running = computed(() => status.value?.running ?? null)
const items = computed(() => status.value?.items ?? [])
const lastFailed = computed(() =>
  status.value?.last?.status === 'failed' ? status.value.last : null,
)

const stageLabel: Record<string, string> = {
  snapshot: '正在生成数据库快照',
  listing: '正在枚举上传文件',
  packing: '正在打包备份文件',
}

function formatBytes(n: number): string {
  if (!Number.isFinite(n) || n <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let v = n
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v >= 10 || i === 0 ? Math.round(v) : v.toFixed(1)} ${units[i]}`
}

function formatTime(iso: string): string {
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleString('zh-CN', { hour12: false })
}

function progressText(p: BackupProgress): string {
  const stage = stageLabel[p.stage] ?? '正在生成备份'
  if (p.total_bytes > 0) {
    return `${stage} · ${formatBytes(p.processed_bytes)} / ${formatBytes(p.total_bytes)}`
  }
  if (p.processed_bytes > 0) return `${stage} · 已处理 ${formatBytes(p.processed_bytes)}`
  return stage
}

function percent(p: BackupProgress): number {
  if (p.total_bytes <= 0) return 0
  return Math.min(100, Math.round((p.processed_bytes / p.total_bytes) * 100))
}

function describeError(err: unknown): string {
  if (err instanceof ApiError) return err.message
  return err instanceof Error ? err.message : '操作失败'
}

function schedule() {
  if (pollTimer !== null) {
    window.clearTimeout(pollTimer)
    pollTimer = null
  }
  if (!stopped && status.value?.running) {
    pollTimer = window.setTimeout(() => {
      void load()
    }, 1200)
  }
}

async function load() {
  try {
    status.value = await api.get<BackupStatus>('/api/admin/backups')
    loadErr.value = null
  } catch (err) {
    loadErr.value = describeError(err)
  } finally {
    schedule()
  }
}

async function startBackup() {
  actionErr.value = null
  notice.value = null
  busy.value = true
  try {
    await api.post<{ started: boolean }>('/api/admin/backups')
    notice.value = '备份任务已开始，完成后会出现在下方列表。'
  } catch (err) {
    if (err instanceof ApiError && err.status === 409) {
      notice.value = '已有备份任务正在生成，请稍候。'
    } else {
      actionErr.value = describeError(err)
    }
  } finally {
    busy.value = false
    await load()
  }
}

async function removeBackup(item: BackupItem) {
  if (!window.confirm(`确定删除备份「${item.name}」？删除后无法从服务器恢复。`)) return
  actionErr.value = null
  notice.value = null
  try {
    await api.delete(`/api/admin/backups/${encodeURIComponent(item.name)}`)
    notice.value = '备份已删除。'
  } catch (err) {
    actionErr.value = describeError(err)
  } finally {
    await load()
  }
}

onMounted(load)
onUnmounted(() => {
  stopped = true
  if (pollTimer !== null) window.clearTimeout(pollTimer)
})
</script>

<template>
  <section class="space-y-6">
    <p v-if="loadErr" class="text-xs text-clay">{{ loadErr }}</p>
    <p v-if="actionErr" class="text-xs text-clay">{{ actionErr }}</p>
    <p v-if="notice" class="text-xs text-moss">{{ notice }}</p>

    <div class="bg-paper border border-line rounded-sm p-6 space-y-4">
      <div class="flex flex-wrap items-start justify-between gap-4">
        <div class="text-xs text-muted space-y-1 max-w-xl">
          <p>备份包含 SQLite 数据库与全部上传文件（照片、视频），生成在当前服务器。</p>
          <p>不包含 .env、部署配置与 Redis 会话数据；建议生成后下载到本机留存。</p>
        </div>
        <button
          type="button"
          class="bg-ink text-paper rounded-full px-6 py-3 text-xs tracking-wider hover:bg-moss transition disabled:opacity-50 disabled:cursor-not-allowed"
          :disabled="busy || !!running"
          @click="startBackup"
        >
          {{ running ? '正在备份…' : '立即备份' }}
        </button>
      </div>

      <div v-if="running" class="space-y-2">
        <div class="flex items-center justify-between text-xs text-muted">
          <span>{{ progressText(running) }}</span>
          <span v-if="running.total_bytes > 0">{{ percent(running) }}%</span>
        </div>
        <div class="h-1.5 bg-line/60 rounded-full overflow-hidden">
          <div
            class="h-full bg-moss transition-all duration-500"
            :style="{ width: (running.total_bytes > 0 ? percent(running) : 8) + '%' }"
          ></div>
        </div>
        <p class="text-[10px] text-muted">
          任务在服务器后台执行，离开页面不会中断；重新进入可继续查看进度。
        </p>
      </div>

      <p v-if="lastFailed" class="text-xs text-clay">
        最近一次备份失败：{{ lastFailed.error || '未知原因' }}
      </p>
    </div>

    <div class="bg-paper border border-line rounded-sm">
      <div class="flex items-center justify-between px-6 py-4 border-b border-line">
        <h2 class="text-sm">已有备份</h2>
        <button type="button" class="text-xs text-muted underline hover:text-ink" @click="load">
          刷新
        </button>
      </div>

      <p v-if="items.length === 0" class="px-6 py-8 text-xs text-muted">
        还没有备份。点击「立即备份」生成第一份。
      </p>
      <ul v-else class="divide-y divide-line">
        <li
          v-for="item in items"
          :key="item.name"
          class="px-6 py-4 flex flex-wrap items-center justify-between gap-3"
        >
          <div class="min-w-0">
            <p class="text-sm truncate">{{ item.name }}</p>
            <p class="text-[11px] text-muted mt-1">
              {{ formatTime(item.created_at) }} · {{ formatBytes(item.size_bytes) }}
            </p>
          </div>
          <div class="flex items-center gap-4 text-xs">
            <a :href="backupDownloadUrl(item.name)" class="underline hover:text-ink">下载</a>
            <button type="button" class="text-clay hover:underline" @click="removeBackup(item)">
              删除
            </button>
          </div>
        </li>
      </ul>
    </div>
  </section>
</template>
