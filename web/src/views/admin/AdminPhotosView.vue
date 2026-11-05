<script setup lang="ts">
import { onMounted, ref } from 'vue'
import {
  api,
  putToCos,
  type PhotoItem,
  type Trip,
  type UploadSignResponse,
} from '@/api/client'
import {
  buildUploadPlans,
  planBytes,
  type LiveUploadPlan,
  type MediaPart,
  type PhotoUploadPlan,
} from '@/lib/livePhoto'
import PageState from '@/components/PageState.vue'

// 相册与照片（计划 §4.3 + §2 管理行）：
// 选择文件 → 实况识别/拆分 → 签名 → 浏览器直传（图片 + 视频） → 确认入库 → 维护说明/排序/删除。

interface UploadJob {
  name: string
  pct: number
  done: boolean
  kind: 'live' | 'photo' | 'skipped'
  failed?: string
  warning?: string
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
  jobs.value = []
  try {
    const plans = await buildUploadPlans(files)
    for (const plan of plans) {
      const job: UploadJob = { name: plan.name, pct: 0, done: false, kind: plan.kind }
      jobs.value.push(job)
      if (plan.kind === 'skipped') {
        job.failed = plan.reason
        continue
      }
      try {
        job.warning = await uploadOne(plan, job)
        job.done = true
        job.pct = 100
      } catch (err) {
        job.failed = err instanceof Error ? err.message : '上传失败'
      }
    }
    await loadPhotos()
  } catch (err) {
    notice.value = err instanceof Error ? err.message : '解析所选文件失败'
  } finally {
    uploading.value = false
  }
}

// uploadOne 上传一张照片（实况照片含视频）；视频失败时降级为普通照片并返回提示。
async function uploadOne(
  plan: PhotoUploadPlan | LiveUploadPlan,
  job: UploadJob,
): Promise<string | undefined> {
  const total = planBytes(plan)
  let uploaded = 0
  const track = (size: number) => (pct: number) => {
    const doneBytes = (size * pct) / 100
    job.pct = total > 0 ? Math.min(99, Math.round(((uploaded + doneBytes) / total) * 100)) : 0
  }

  const photoKey = await uploadObject(plan.photo, track(plan.photo.blob.size))
  uploaded += plan.photo.blob.size

  if (plan.kind === 'photo') {
    await confirmPhoto(plan.name, photoKey)
    return undefined
  }

  let videoKey: string
  try {
    videoKey = await uploadObject(plan.video, track(plan.video.blob.size))
  } catch (err) {
    const reason = err instanceof Error ? err.message : '上传失败'
    // 图片已经上传成功，先保证照片入库，再提示实况视频失败。
    await confirmPhoto(plan.name, photoKey)
    return `实况视频上传失败，已按普通照片保存：${reason}`
  }
  uploaded += plan.video.blob.size
  await confirmPhoto(plan.name, photoKey, videoKey)
  return undefined
}

async function uploadObject(part: MediaPart, onProgress: (pct: number) => void): Promise<string> {
  const sign = await api.post<UploadSignResponse>('/api/admin/uploads/sign', {
    trip_id: tripId.value,
    content_type: part.contentType,
    size: part.blob.size,
  })
  await putToCos(sign.upload_url, sign.content_type, part.blob, onProgress)
  return sign.cos_key
}

async function confirmPhoto(name: string, cosKey: string, videoKey?: string) {
  await api.post('/api/admin/photos/confirm', {
    trip_id: tripId.value,
    cos_key: cosKey,
    ...(videoKey ? { video_key: videoKey } : {}),
    title: name,
  })
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
          选择文件（可多选，单张 ≤30MB；照片 + 同名视频会自动识别为实况照片）
          <input
            type="file"
            accept="image/jpeg,image/png,image/webp,image/heic,image/heif,video/mp4,video/quicktime"
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
          <div class="flex justify-between mb-1 gap-3">
            <span class="truncate">
              <span
                v-if="j.kind === 'live'"
                class="mr-1 rounded-full bg-moss/15 px-1.5 py-0.5 text-[10px] text-moss"
                >实况</span
              >
              {{ j.name }}
            </span>
            <span
              class="shrink-0"
              :class="j.failed || j.warning ? 'text-clay' : 'text-muted'"
            >
              {{ j.failed ?? j.warning ?? (j.done ? '完成' : `${j.pct}%`) }}
            </span>
          </div>
          <div v-if="j.kind !== 'skipped'" class="h-1.5 bg-paper-deep rounded-full overflow-hidden">
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
          <span
            v-if="p.video_src"
            class="absolute top-2 left-2 rounded-full bg-black/55 px-2 py-0.5 text-[10px] text-white"
            >实况</span
          >
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
