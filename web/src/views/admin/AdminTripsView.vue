<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { api, type PhotoItem, type Trip, type TripLocation } from '@/api/client'

// 旅行管理：创建/编辑/发布樖墙 + 地点维护 + 封面选择（整组替换后端 API）。
const trips = ref<Trip[]>([])
const loadErr = ref<string | null>(null)
const message = ref<string | null>(null)

const form = reactive({
  id: '', // 空 = 创建
  title: '',
  summary: '',
  start_date: '',
  end_date: '',
  cover_photo_id: '', // '' = 未选（保存时清除）；保存后生效
  sort_order: 0,
  status: 'draft' as 'draft' | 'published',
})

const locations = reactive({
  tripId: '', // 绑定的旅行
  rows: [] as { name: string; note: string; latitude: string; longitude: string }[],
})

// 封面选择：编辑模式从本旅行相册选一张作卡片封面；缩略图来自管理端照片列表。
const coverPhotos = ref<PhotoItem[]>([])
const coverPickerOpen = ref(false)
const coverLoading = ref(false)
// 当前旅行已保存封面的缩略图（进编辑时从列表带出，保存后随列表刷新）
const savedCoverPreview = ref('')

const coverPreview = computed(() => {
  if (!form.cover_photo_id) return ''
  const fromPicker = coverPhotos.value.find((p) => p.id === form.cover_photo_id)
  return fromPicker?.thumb_src ?? savedCoverPreview.value
})

async function load() {
  loadErr.value = null
  try {
    trips.value = await api.get<Trip[]>('/api/admin/trips')
  } catch (err) {
    loadErr.value = err instanceof Error ? err.message : '加载失败'
  }
}
onMounted(load)

function startCreate() {
  const today = new Date().toISOString().slice(0, 10)
  form.id = ''
  form.title = ''
  form.summary = ''
  form.start_date = today
  form.end_date = ''
  form.cover_photo_id = ''
  form.sort_order = trips.value.length
  form.status = 'draft'
  locations.tripId = ''
  locations.rows = []
  coverPhotos.value = []
  coverPickerOpen.value = false
  savedCoverPreview.value = ''
  message.value = null
}

function startEdit(t: Trip) {
  form.id = t.id
  form.title = t.title
  form.summary = t.summary
  form.start_date = t.start_date
  form.end_date = t.end_date ?? ''
  form.cover_photo_id = t.cover_photo_id ?? ''
  form.sort_order = t.sort_order
  form.status = t.status as 'draft' | 'published'
  locations.tripId = ''
  locations.rows = []
  coverPhotos.value = []
  coverPickerOpen.value = false
  savedCoverPreview.value = t.cover_thumb_src ?? ''
  message.value = null
  void editLocations(t.id)
}

async function loadCoverPhotos() {
  if (coverLoading.value || form.id === '') return
  coverLoading.value = true
  try {
    const data = await api.get<{ enabled: boolean; photos: PhotoItem[] }>(
      `/api/admin/trips/${form.id}/photos`,
    )
    coverPhotos.value = data.photos
  } catch (err) {
    message.value = err instanceof Error ? err.message : '照片列表加载失败'
  }
  coverLoading.value = false
}

async function toggleCoverPicker() {
  coverPickerOpen.value = !coverPickerOpen.value
  if (coverPickerOpen.value) await loadCoverPhotos()
}

// 再点一次选中的照片即取消选择（= 清除）。
function pickCover(p: PhotoItem) {
  form.cover_photo_id = form.cover_photo_id === p.id ? '' : p.id
}

async function save() {
  if (!form.title || !form.start_date) {
    message.value = '请补齐标题与开始日期。'
    return
  }
  const base = {
    title: form.title,
    summary: form.summary,
    start_date: form.start_date,
    end_date: form.end_date === '' ? null : form.end_date,
    sort_order: form.sort_order,
    status: form.status,
  }
  try {
    if (form.id === '') {
      // 创建分支不发 cover（封面照片必须先属于本旅行）
      const created = await api.post<{ id: string }>('/api/admin/trips', base)
      form.id = created.id
      message.value = '已创建，可继续编辑地点或照片。'
    } else {
      await api.put(`/api/admin/trips/${form.id}`, { ...base, cover_photo_id: form.cover_photo_id })
      message.value = '已保存。'
      // 保存后刷新预览缓存（下方 load() 会重拉列表）
      const fresh = trips.value.find((x) => x.id === form.id)
      savedCoverPreview.value = fresh?.cover_thumb_src ?? ''
    }
    await load()
  } catch (err) {
    message.value = err instanceof Error ? err.message : '保存失败'
  }
}

async function toggleStatus(t: Trip) {
  const next = t.status === 'published' ? 'draft' : 'published'
  await api.put(`/api/admin/trips/${t.id}`, {
    title: t.title, summary: t.summary, start_date: t.start_date,
    end_date: t.end_date, sort_order: t.sort_order, status: next,
  })
  await load()
}

// 两步确认删除（计划：删除二次确认）
const confirmDelete = ref('')

async function removeTrip(id: string) {
  if (confirmDelete.value !== id) {
    confirmDelete.value = id
    return
  }
  confirmDelete.value = ''
  try {
    await api.delete(`/api/admin/trips/${id}`)
    if (form.id === id) startCreate()
    message.value = '已删除（关联的地点/相册/照片记录一并移除）。'
    await load()
  } catch (err) {
    message.value = err instanceof Error ? err.message : '删除失败'
  }
}

async function editLocations(tripId: string) {
  // 管理端专用读取：草稿同样可见（公开 GET 只放已发布）。
  const data = await api.get<{ trip: Trip; locations: TripLocation[] }>(
    `/api/admin/trips/${tripId}`,
  )
  locations.tripId = tripId
  locations.rows = data.locations.map((l) => ({
    name: l.name,
    note: l.note ?? '',
    latitude: l.latitude === null ? '' : String(l.latitude),
    longitude: l.longitude === null ? '' : String(l.longitude),
  }))
}

function addLocationRow() {
  locations.rows.push({ name: '', note: '', latitude: '', longitude: '' })
}

function removeLocationRow(i: number) {
  locations.rows.splice(i, 1)
}

async function saveLocations() {
  if (locations.tripId === '') return
  const payload = locations.rows
    .filter((r) => r.name.trim() !== '')
    .map((r) => ({
      name: r.name.trim(),
      note: r.note === '' ? null : r.note,
      latitude: r.latitude === '' ? null : Number(r.latitude),
      longitude: r.longitude === '' ? null : Number(r.longitude),
    }))
  try {
    await api.put(`/api/admin/trips/${locations.tripId}/locations`, { locations: payload })
    message.value = '地点顺序已保存。'
  } catch (err) {
    message.value = err instanceof Error ? err.message : '保存失败'
  }
}

const editingExisting = computed(() => form.id !== '')
</script>

<template>
  <section class="space-y-8">
    <p v-if="loadErr" class="text-xs text-clay">{{ loadErr }}</p>
    <p v-if="message" class="text-xs text-moss">{{ message }}</p>

    <!-- 创建 / 编辑表单 -->
    <form class="bg-paper border border-line rounded-sm p-6 space-y-4" @submit.prevent="save">
      <div class="flex items-center justify-between">
        <h2 class="section-title text-xl">{{ editingExisting ? '编辑旅行' : '新建旅行' }}</h2>
        <button
          v-if="editingExisting"
          type="button"
          class="text-xs text-muted underline hover:text-ink"
          @click="startCreate"
        >
          改为新建
        </button>
      </div>

      <div class="grid md:grid-cols-2 gap-4">
        <label class="block">
          <span class="text-[10px] tracking-[0.2em] text-muted">标题 *</span>
          <input v-model="form.title" type="text" class="mt-1 w-full border border-line rounded-sm bg-white/70 px-3 py-2.5 text-sm outline-none focus:border-clay" />
        </label>
        <label class="block">
          <span class="text-[10px] tracking-[0.2em] text-muted">状态</span>
          <select v-model="form.status" class="mt-1 w-full border border-line rounded-sm bg-white/70 px-3 py-2.5 text-sm outline-none focus:border-clay">
            <option value="draft">草稿（前台不可见）</option>
            <option value="published">已发布</option>
          </select>
        </label>
        <label class="block">
          <span class="text-[10px] tracking-[0.2em] text-muted">开始日期 *</span>
          <input v-model="form.start_date" type="date" class="mt-1 w-full border border-line rounded-sm bg-white/70 px-3 py-2.5 text-sm outline-none focus:border-clay" />
        </label>
        <label class="block">
          <span class="text-[10px] tracking-[0.2em] text-muted">结束日期</span>
          <input v-model="form.end_date" type="date" class="mt-1 w-full border border-line rounded-sm bg-white/70 px-3 py-2.5 text-sm outline-none focus:border-clay" />
        </label>
      </div>

      <label class="block">
        <span class="text-[10px] tracking-[0.2em] text-muted">旅程故事（100 字以内）</span>
        <textarea
          v-model="form.summary"
          rows="3"
          maxlength="2000"
          class="mt-1 w-full border border-line rounded-sm bg-white/70 px-3 py-2.5 text-sm outline-none focus:border-clay"
        ></textarea>
      </label>

      <div class="flex items-center justify-between">
        <span class="text-[10px] tracking-[0.2em] text-muted">排序：{{ form.sort_order }}</span>
        <button type="submit" class="bg-ink text-paper rounded-full px-6 py-3 text-xs tracking-wider hover:bg-moss transition">
          {{ editingExisting ? '保存修改' : '创建旅行' }}
        </button>
      </div>
    </form>

    <!-- 地点维护（编辑中的旅行） -->
    <div v-if="editingExisting" class="bg-paper border border-line rounded-sm p-6 space-y-3">
      <div class="flex items-center justify-between">
        <h3 class="section-title text-lg">地点（可多选，自上而下顺序即展示顺序）</h3>
        <button type="button" class="text-xs underline text-muted hover:text-ink" @click="addLocationRow">+ 添加地点</button>
      </div>

      <p v-if="!locations.rows.length" class="text-xs text-muted">这一站还没有地点，点击右上角添加。</p>

      <div
        v-for="(row, i) in locations.rows"
        :key="i"
        class="grid grid-cols-2 md:grid-cols-[2fr,2fr,1fr,1fr,auto] gap-2 items-end"
      >
        <label><span class="text-[10px] text-muted">名称</span>
          <input v-model="row.name" type="text" class="mt-1 w-full border border-line rounded-sm bg-white/70 px-2.5 py-2 text-sm" /></label>
        <label><span class="text-[10px] text-muted">备注</span>
          <input v-model="row.note" type="text" class="mt-1 w-full border border-line rounded-sm bg-white/70 px-2.5 py-2 text-sm" /></label>
        <label><span class="text-[10px] text-muted">纬度</span>
          <input v-model="row.latitude" type="number" step="any" class="mt-1 w-full border border-line rounded-sm bg-white/70 px-2.5 py-2 text-sm" /></label>
        <label><span class="text-[10px] text-muted">经度</span>
          <input v-model="row.longitude" type="number" step="any" class="mt-1 w-full border border-line rounded-sm bg-white/70 px-2.5 py-2 text-sm" /></label>
        <button type="button" class="text-muted hover:text-clay" aria-label="删除地点" @click="removeLocationRow(i)">✕</button>
      </div>

      <button
        v-if="locations.rows.length"
        type="button"
        class="bg-ink text-paper rounded-full px-6 py-2.5 text-xs tracking-wider hover:bg-moss transition"
        @click="saveLocations"
      >
        保存地点顺序
      </button>
    </div>

    <!-- 封面选择（编辑中的旅行）：从本旅行相册挑一张作前台卡片封面 -->
    <div v-if="editingExisting" class="bg-paper border border-line rounded-sm p-6 space-y-3">
      <div class="flex items-center justify-between gap-3 flex-wrap">
        <h3 class="section-title text-lg">封面照片（前台旅行卡展示）</h3>
        <div class="flex items-center gap-4 text-xs">
          <button type="button" class="underline text-muted hover:text-ink" @click="toggleCoverPicker">
            {{ coverPickerOpen ? '收起照片列表' : '从相册选择' }}
          </button>
          <button
            v-if="form.cover_photo_id"
            type="button"
            class="underline text-muted hover:text-clay"
            @click="form.cover_photo_id = ''"
          >
            清除封面
          </button>
        </div>
      </div>

      <div v-if="coverPreview" class="flex items-center gap-3">
        <img :src="coverPreview" alt="封面预览" class="w-28 h-20 object-cover rounded-sm border border-line" />
        <p class="text-xs text-muted">当前选择，点击「保存修改」生效。</p>
      </div>
      <p v-else class="text-xs text-muted">未选择封面，前台将显示渐变占位色块。</p>

      <div v-if="coverPickerOpen" class="grid grid-cols-3 md:grid-cols-5 gap-2 max-h-72 overflow-y-auto">
        <p v-if="coverLoading" class="text-xs text-muted">照片加载中…</p>
        <p v-else-if="!coverPhotos.length" class="text-xs text-muted">
          这个旅行还没有照片，先到「照片管理」上传后再来选封面。
        </p>
        <button
          v-for="p in coverPhotos"
          :key="p.id"
          type="button"
          class="photo-card aspect-[4/3] rounded-sm border transition overflow-hidden"
          :class="form.cover_photo_id === p.id ? 'border-moss ring-2 ring-moss' : 'border-transparent hover:border-clay'"
          :title="p.title"
          @click="pickCover(p)"
        >
          <img v-if="p.thumb_src" :src="p.thumb_src" :alt="p.title" loading="lazy" class="w-full h-full object-cover" />
          <span v-else class="text-[10px] text-muted px-1 line-clamp-2">{{ p.title || '无预览' }}</span>
        </button>
      </div>
    </div>

    <!-- 旅行列表 -->
    <div class="space-y-3">
      <h2 class="section-title text-xl">全部旅行（{{ trips.length }}）</h2>
      <p v-if="!trips.length" class="text-xs text-muted">还没有旅行，先在上方创建。</p>

      <article
        v-for="t in trips"
        :key="t.id"
        class="border border-line rounded-sm bg-paper p-5 flex flex-wrap items-center gap-3 justify-between"
      >
        <div class="flex items-center gap-3">
          <img
            v-if="t.cover_thumb_src"
            :src="t.cover_thumb_src"
            alt=""
            class="w-14 h-11 object-cover rounded-sm border border-line"
          />
          <div>
            <p class="text-[10px] text-clay tracking-widest">{{ t.start_date }} · {{ t.status === 'published' ? '已发布' : '草稿' }}</p>
            <h3 class="section-title text-lg">{{ t.title }}</h3>
            <p class="text-xs text-muted mt-1 max-w-xl line-clamp-2">{{ t.summary }}</p>
          </div>
        </div>
        <div class="flex items-center gap-2 text-xs">
          <button class="border border-line rounded-full px-4 py-2 hover:bg-ink hover:text-paper transition" @click="startEdit(t)">编辑</button>
          <button class="border border-line rounded-full px-4 py-2 hover:bg-ink hover:text-paper transition" @click="toggleStatus(t)">
            {{ t.status === 'published' ? '转草稿' : '发布' }}
          </button>
          <button
            class="rounded-full px-4 py-2 border transition"
            :class="confirmDelete === t.id ? 'bg-clay text-white border-clay' : 'border-line text-muted hover:text-clay'"
            @click="removeTrip(t.id)"
          >
            {{ confirmDelete === t.id ? '确认删除？' : '删除' }}
          </button>
        </div>
      </article>
    </div>
  </section>
</template>
