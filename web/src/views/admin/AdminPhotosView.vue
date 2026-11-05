<script setup lang="ts">
import { onMounted, ref } from 'vue'
import {
  api,
  putToCos,
  type PhotoItem,
  type Trip,
  type UploadSignResponse,
} from '@/api/client'
import PageState from '@/components/PageState.vue'

// 相册与照片（计划 §4.3 + §2 管理行）：
// 选择文件 → 签名 → 浏览器直传 COS（进度） → 确认入库 → 维护（说明/拍摄时间/排序/删除）。

interface UploadJob {
  name: string
  pct: number
  done: boolean
  failed?: string
}

const trips = ref<Trip[]>([])
const tripId = ref('')
const loadErr = ref<string | null>(null)
const notice = ref<string | null>(null)
const cosEnabled = ref(true)
const photos = ref<PhotoItem[]>([])
const jobs = ref<UploadJob[]>([])
const uploading = ref(false)
const confirmDelete = ref('')

async function loadTrips() {
  try {
    trips.value = await api.get<Trip[]>('/api/admin/trips')
    if (trips.value.length && tripId.value === '') tripId.value = trips.value[0].id
  } catch (err) {
    loadErr.value = err instanceof Error ? err.message : '加载失败'
  }
}

async function loadPhotos() {
  notice.value = null
  if (tripId.value === '') {
    photos.value = []
    return
  }
  try {
    const data = await api.get<{ enabled: boolean; photos: PhotoItem[] }>(
      `/api/admin/trips/${tripId.value}/photos`,
    )
    cosEnabled.value = data.enabled
    photos.value = data.photos
  } catch (err) {
    loadErr.value = err instanceof Error ? err.message : '读取照片失败'
  }
}

function pickTrip(id: string) {
  tripId.value = id
  void loadPhotos()
}

async function onFilesChosen(ev: Event) {
  const input = ev.target as HTMLInputElement
  const files = Array.from(input.files ?? [])
  input.value = ''
  if (!files.length || uploading.value) return
  if (tripId.value === '') {
    notice.value = '先选择一次旅行，再上传照片。'
    return
  }
  uploading.value = true
  try {
    for (const file of files) {
      const job: UploadJob = { name: file.name, pct: 0, done: false }
      jobs.value.push(job)
      try {
        await uploadOne(file, job)
        job.done = true
        job.pct = 100
      } catch (err) {
        job.failed = err instanceof Error ? err.message : '上传失败'
      }
    }
    await loadPhotos()
  } finally {
    uploading.value = false
    jobs.value = []
  }
}

async function uploadOne(file: File, job: UploadJob) {
  const contentType = file.type || guessType(file.name)
  const sign = await api.post<UploadSignResponse>('/api/admin/uploads/sign', {
    trip_id: tripId.value,
    content_type: contentType,
    size: file.size,
  })
  await putToCos(sign.upload_url, sign.content_type, file, (pct) => {
    job.pct = pct
  })
  await api.post('/api/admin/photos/confirm', {
    trip_id: tripId.value,
    cos_key: sign.cos_key,
    title: file.name.replace(/\.[^.]+$/, ''),
  })
}

function guessType(name: string): string {
  const ext = (name.split('.').pop() ?? '').toLowerCase()
  const map: Record<string, string> = {
    jpg: 'image/jpeg', jpeg: 'image/jpeg', png: 'image/png',
    webp: 'image/webp', heic: 'image/heic', heif: 'image/heif',
  }
  return map[ext] ?? 'image/jpeg'
}

async function removePhoto(p: PhotoItem) {
  if (confirmDelete.value !== p.id) {
    confirmDelete.value = p.id
    return
  }
  confirmDelete.value = ''
  try {
    await api.delete(`/api/admin/photos/${p.id}`)
    await loadPhotos()
  } catch (err) {
    notice.value = err instanceof Error ? err.message : '删除失败'
  }
}

async function movePhoto(p: PhotoItem, delta: -1 | 1) {
  const idx = photos.value.findIndex((x) => x.id === p.id)
  const target = idx + delta
  if (idx < 0 || target < 0 || target >= photos.value.length) return
  const ids = photos.value.map((x) => x.id)
  const tmp = ids[idx]
  ids[idx] = ids[target]
  ids[target] = tmp
  try {
    await api.put('/api/admin/photos/reorder', { ids })
    await loadPhotos()
  } catch (err) {
    notice.value = err instanceof Error ? err.message : '排序保存失败'
  }
}

async function savePhoto(p: PhotoItem) {
  try {
    await api.put(`/api/admin/photos/${p.id}`, {
      title: p.title ?? '',
      description: p.description ?? '',
      taken_at: p.taken_at ?? null,
      album_id: p.album_id ?? '',
    })
    notice.value = '已保存。'
  } catch (err) {
    notice.value = err instanceof Error ? err.message : '保存失败'
  }
}

onMounted(async () => {
  await loadTrips()
  await loadPhotos()
})
</script>

<template>
  <section class="space-y-8">
    <p v-if="loadErr" class="text-xs text-clay">{{ loadErr }}</p>
    <p v-if="notice" class="text-xs text-moss">{{ notice }}</p>

    <!-- 旅行选择 + 上传 -->
    <div class="bg-paper border border-line rounded-sm p-6 space-y-4">
      <div class="flex flex-wrap items-center gap-3">
        <label class="text-xs text-muted">
          旅行
          <select
            :value="tripId"
            class="ml-2 border border-line rounded-sm bg-white/70 px-3 py-2 text-sm"
            @change="pickTrip(($event.target as HTMLSelectElement).value)"
          >
            <option v-if="!trips.length" value="">（尚无旅行，请先在“旅行管理”创建）</option>
            <option v-for="t in trips" :key="t.id" :value="t.id">{{ t.title }}</option>
          </select>
        </label>

        <label class="ml-auto flex items-center gap-2 text-xs text-muted">
          选择文件（可多选，单张 ≤30MB）
          <input
            type="file"
            accept="image/jpeg,image/png,image/webp,image/heic,image/heif"
            multiple
            :disabled="uploading || !tripId"
            class="text-xs"
            @change="onFilesChosen"
          />
        </label>
      </div>

      <p v-if="!cosEnabled" class="text-xs text-clay leading-6">
        对象存储尚未配置（COS_SECRET_ID/KEY/REGION/BUCKET），上传暂不可用；文本内容维护不受影响。
      </p>

      <div v-if="jobs.length" class="space-y-2">
        <div v-for="j in jobs" :key="j.name" class="text-xs">
          <div class="flex justify-between mb-1">
            <span>{{ j.name }}</span>
            <span :class="j.failed ? 'text-clay' : 'text-muted'">{{ j.failed ?? `${j.pct}%` }}</span>
          </div>
          <div class="h-1.5 bg-paper-deep rounded-full overflow-hidden">
            <div class="h-full bg-moss transition-all" :style="{ width: `${j.pct}%` }"></div>
          </div>
        </div>
      </div>
    </div>

    <PageState
      :loading="false"
      :error="null"
      :empty="tripId !== '' && photos.length === 0"
      empty-title="这一次旅行还没有照片。"
      empty-hint="建议每次安排 8–20 张精选照片。"
    />

    <div v-if="photos.length" class="grid sm:grid-cols-2 lg:grid-cols-3 gap-5">
      <article
        v-for="(p, i) in photos"
        :key="p.id"
        class="border border-line rounded-sm bg-paper p-3 space-y-2"
      >
        <div class="photo-card aspect-[4/3] rounded-sm">
          <img
            v-if="p.thumb_src"
            :src="p.thumb_src"
            :alt="p.title"
            loading="lazy"
            class="w-full h-full object-cover"
          />
          <div v-else class="w-full h-full flex items-center justify-center text-[10px] text-muted">
            待 COS 配置后可见
          </div>
        </div>

        <input
          v-model="p.title"
          type="text"
          class="w-full border border-line rounded-sm bg-white/70 px-2 py-1.5 text-xs"
          @change="savePhoto(p)"
        />
        <input
          v-model="p.description"
          type="text"
          placeholder="一句话注释"
          class="w-full border border-line rounded-sm bg-white/70 px-2 py-1.5 text-xs"
          @change="savePhoto(p)"
        />

        <div class="flex items-center justify-between text-[10px] text-muted">
          <span>#{{ i + 1 }}{{ p.taken_at ? ` · ${p.taken_at.slice(0, 10)}` : '' }}</span>
          <span class="flex items-center gap-1">
            <button aria-label="上移" @click="movePhoto(p, -1)">↑</button>
            <button aria-label="下移" @click="movePhoto(p, 1)">↓</button>
            <button
              class="rounded-full px-2 py-0.5 border"
              :class="confirmDelete === p.id ? 'bg-clay text-white border-clay' : 'border-line'"
              @click="removePhoto(p)"
            >
              {{ confirmDelete === p.id ? '确认删除？' : '删除' }}
            </button>
          </span>
        </div>
      </article>
    </div>
  </section>
</template>
