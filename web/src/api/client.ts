// 轻量 API 客户端：统一 envelope 与错误语义。
// 认证阶段落地后，logout/401 处理与 CSRF 头在此扩展（计划 §5.1）。

export class ApiError extends Error {
  status: number
  code: string

  constructor(status: number, code: string, message: string) {
    super(message)
    this.status = status
    this.code = code
  }
}

interface ErrorEnvelope {
  error?: { code?: string; message?: string }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    credentials: 'same-origin',
    headers: { Accept: 'application/json', ...(init?.headers ?? {}) },
    ...init,
  })

  const body = (await res.json().catch(() => null)) as
    | { data?: T }
    | (ErrorEnvelope & { data?: T })
    | null

  if (!res.ok || (body && 'error' in body && body.error)) {
    const e = (body as ErrorEnvelope | null)?.error
    throw new ApiError(res.status, e?.code ?? `http_${res.status}`, e?.message ?? res.statusText)
  }

  return (body as { data?: T })?.data as T
}

export const api = {
  get: <T>(path: string) => request<T>(path),
  post: <T>(path: string, json?: unknown) =>
    request<T>(path, {
      method: 'POST',
      headers: json !== undefined ? { 'Content-Type': 'application/json' } : undefined,
      body: json === undefined ? undefined : JSON.stringify(json),
    }),
  put: <T>(path: string, json: unknown) =>
    request<T>(path, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(json),
    }),
  delete: <T>(path: string) => request<T>(path, { method: 'DELETE' }),
}

// 私密内容的统一门禁判定：后端 401 gate_required 时，页面渲染 <GatePrompt>。
export function isGateRequired(err: unknown): boolean {
  return err instanceof ApiError && err.code === 'gate_required'
}

// ---------- 领域类型（计划 §4.1） ----------

export interface Trip {
  id: string
  title: string
  summary: string
  start_date: string
  end_date: string | null
  cover_photo_id: string | null
  // 服务端按封面照片签名的缩略图 URL（COS 未配置/签名失败时为空串 → 前端回退占位）
  cover_thumb_src?: string
  sort_order: number
  status: string
  created_at: string
  updated_at: string
}

export interface TripLocation {
  id: string
  trip_id: string
  name: string
  address: string | null
  latitude: number | null
  longitude: number | null
  note: string | null
  sort_order: number
}

export type SiteSettings = Record<string, string>

export interface Letter {
  title: string
  body: string
  audio_key: string
  reveal_enabled: boolean
  updated_at: string
}

// 媒体条目类型：photo 为照片（可带实况视频），video 为单独上传的视频。
export type MediaKind = 'photo' | 'video'

export interface PhotoItem {
  id: string
  album_id?: string
  title: string
  description?: string
  taken_at?: string | null
  width?: number | null
  height?: number | null
  sort_order: number
  // 服务端返回的 media_kind；旧数据缺省时按照片处理。
  media_kind?: MediaKind
  src?: string
  // 灯箱显示尺寸（长边 2400、不放大）；签名失败时为空，前端回退 src。
  preview_src?: string
  thumb_src?: string
  // 实况照片的视频地址或独立视频的播放地址；为空表示无视频。
  video_src?: string
}

// 首页随机照片条目：跨全部已发布旅行随机挑选，只用于展示。
export interface RandomPhoto {
  id: string
  trip_id: string
  trip_title: string
  title: string
  taken_at?: string | null
  // 缩略图、灯箱尺寸与原图地址；存储未配置或签名失败时为空串。
  thumb_src: string
  preview_src: string
  src: string
}

export interface RandomPhotoResponse {
  enabled: boolean
  photos: RandomPhoto[]
}

// 灯箱条目（与 PhotoLightbox 组件共享）。
export interface LightboxPhoto {
  id?: string
  src: string
  // 实况照片的视频地址；存在时灯箱改用实况播放器。
  videoSrc?: string
  // video 表示 src 是视频播放地址，灯箱使用原生视频播放器。
  mediaKind?: MediaKind
  caption?: string
}

export interface UploadSignResponse {
  cos_key: string
  upload_url: string
  content_type: string
  max_bytes: number
  expires_at: string
  album_id?: string
}

// 直传 COS：fetch 无上传进度回调，使用 XHR 获取百分比。
export function putToCos(
  uploadUrl: string,
  contentType: string,
  file: Blob,
  onProgress?: (pct: number) => void,
): Promise<void> {
  return new Promise((resolve, reject) => {
    const xhr = new window.XMLHttpRequest()
    xhr.open('PUT', uploadUrl, true)
    xhr.setRequestHeader('Content-Type', contentType)
    xhr.upload.addEventListener('progress', (e) => {
      if (e.lengthComputable && onProgress) onProgress(Math.round((e.loaded / e.total) * 100))
    })
    xhr.addEventListener('load', () => {
      if (xhr.status >= 200 && xhr.status < 300) resolve()
      else reject(new Error(`直传失败 HTTP ${xhr.status}`))
    })
    xhr.addEventListener('error', () => reject(new Error('直传网络错误')))
    xhr.addEventListener('abort', () => reject(new Error('直传已取消')))
    xhr.send(file)
  })
}
