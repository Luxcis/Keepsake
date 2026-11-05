// 实况照片（Live Photo）输入解析：把管理端一次选择的文件转换成上传计划。
//
// 支持两种来源：
// 1. 成对文件：同名（忽略扩展名）的图像 + 视频（iOS 相册“导出原片”得到 HEIC/JPG + MOV）；
// 2. 合并文件：视频内嵌在图片尾部（Android/Google 动态照片、部分导出工具），
//    用 live-photo 的 extractFromLivePhoto 拆分，并用魔数校验避免把单个 HEIC/JPEG 误判为实况。
//
// 解析只发生在浏览器；这里不做解码或转码，只识别类型与归属。

import { extractFromLivePhoto } from 'live-photo'

export interface MediaPart {
  blob: Blob
  // 服务端上传签名使用的 Content-Type（按魔数或扩展名归一）。
  contentType: string
  fileName: string
}

export interface LiveUploadPlan {
  kind: 'live'
  name: string
  photo: MediaPart
  video: MediaPart
}

export interface PhotoUploadPlan {
  kind: 'photo'
  name: string
  photo: MediaPart
}

export interface SkippedUploadPlan {
  kind: 'skipped'
  name: string
  reason: string
}

export type UploadPlan = LiveUploadPlan | PhotoUploadPlan | SkippedUploadPlan

const IMAGE_EXT_TYPES: Record<string, string> = {
  jpg: 'image/jpeg',
  jpeg: 'image/jpeg',
  png: 'image/png',
  webp: 'image/webp',
  heic: 'image/heic',
  heif: 'image/heif',
}

const VIDEO_EXT_TYPES: Record<string, string> = {
  mp4: 'video/mp4',
  mov: 'video/quicktime',
}

const TYPE_EXT: Record<string, string> = {
  'image/jpeg': 'jpg',
  'image/png': 'png',
  'image/webp': 'webp',
  'image/heic': 'heic',
  'image/heif': 'heif',
  'video/mp4': 'mp4',
  'video/quicktime': 'mov',
}

// 只有这两种图片可能把视频追加在尾部；PNG/WebP 直接按普通图片处理。
const SPLIT_EXT = new Set(['jpg', 'jpeg', 'heic', 'heif'])

function fileExt(name: string): string {
  const i = name.lastIndexOf('.')
  return i >= 0 ? name.slice(i + 1).toLowerCase() : ''
}

function baseName(name: string): string {
  const i = name.lastIndexOf('.')
  return i > 0 ? name.slice(0, i) : name
}

function replaceExt(name: string, ext: string): string {
  return baseName(name) + '.' + ext
}

function normalizeType(raw: string): string {
  return raw.split(';')[0].trim().toLowerCase()
}

export function imageContentType(file: File): string {
  const type = normalizeType(file.type)
  if (type === 'image/jpeg' || type === 'image/png' || type === 'image/webp' ||
    type === 'image/heic' || type === 'image/heif') {
    return type
  }
  return IMAGE_EXT_TYPES[fileExt(file.name)] ?? ''
}

export function videoContentType(file: File): string {
  const type = normalizeType(file.type)
  if (type === 'video/mp4' || type === 'video/quicktime') return type
  return VIDEO_EXT_TYPES[fileExt(file.name)] ?? ''
}

function isImageFile(file: File): boolean {
  return imageContentType(file) !== ''
}

function isVideoFile(file: File): boolean {
  return videoContentType(file) !== ''
}

function partOf(file: File, contentType: string): MediaPart {
  const ext = TYPE_EXT[contentType] ?? fileExt(file.name)
  return { blob: file, contentType, fileName: replaceExt(file.name, ext) }
}

function ascii(bytes: Uint8Array, start: number, len: number): string {
  let out = ''
  for (let i = start; i < start + len && i < bytes.length; i++) out += String.fromCharCode(bytes[i])
  return out
}

/** 从字节判断图片真实类型；不认识的返回 null。 */
export function detectImageType(bytes: Uint8Array): string | null {
  if (bytes.length >= 3 && bytes[0] === 0xff && bytes[1] === 0xd8 && bytes[2] === 0xff) {
    return 'image/jpeg'
  }
  if (bytes.length >= 12 && ascii(bytes, 4, 4) === 'ftyp') {
    const brand = ascii(bytes, 8, 4)
    // heic/heix/hevc/hevx 均为 HEIF 的 HEVC 静态图品牌；mif1/msf1 为通用 HEIF。
    if (['heic', 'heix', 'hevc', 'hevx', 'heim', 'heis'].includes(brand)) return 'image/heic'
    if (['mif1', 'msf1'].includes(brand)) return 'image/heif'
  }
  return null
}

/** 从字节判断视频容器类型；不认识的返回 null。 */
export function detectVideoType(bytes: Uint8Array): string | null {
  if (bytes.length >= 12 && ascii(bytes, 4, 4) === 'ftyp') {
    return ascii(bytes, 8, 4) === 'qt  ' ? 'video/quicktime' : 'video/mp4'
  }
  return null
}

/** 视频是否具备可播放的基本结构（ftyp + moov/mdat）。 */
export function isPlayableVideo(bytes: Uint8Array): boolean {
  if (detectVideoType(bytes) === null) return false
  const limit = Math.min(bytes.length - 3, 8192)
  for (let i = 0; i < limit; i++) {
    const tag = ascii(bytes, i, 4)
    if (tag === 'moov' || tag === 'mdat') return true
  }
  return false
}

/**
 * 尝试从单个合并实况文件拆出图片与视频。
 * 拆分结果必须通过独立校验，否则返回 null（按普通图片上传）。
 */
export async function splitMergedLivePhoto(file: File): Promise<{ photo: MediaPart; video: MediaPart } | null> {
  if (!SPLIT_EXT.has(fileExt(file.name))) return null
  const result = await extractFromLivePhoto(file)
  if (!result) return null
  try {
    const photoBytes = new Uint8Array(await result.photoBlob.arrayBuffer())
    const videoBytes = new Uint8Array(await result.videoBlob.arrayBuffer())
    const photoType = detectImageType(photoBytes)
    // 单个 HEIC 会在这里被拦下：它的 ftyp 位于文件开头，拆分后图片部分为空。
    if (!photoType || photoBytes.length < 16) return null
    if (!isPlayableVideo(videoBytes)) return null

    const videoType = detectVideoType(videoBytes) ?? 'video/mp4'
    return {
      photo: {
        blob: new Blob([photoBytes], { type: photoType }),
        contentType: photoType,
        fileName: replaceExt(file.name, TYPE_EXT[photoType] ?? 'jpg'),
      },
      video: {
        blob: new Blob([videoBytes], { type: videoType }),
        contentType: videoType,
        fileName: replaceExt(file.name, TYPE_EXT[videoType] ?? 'mp4'),
      },
    }
  } catch {
    return null
  } finally {
    result.revoke()
  }
}

/**
 * 把一次选择的文件整理成上传计划：
 * - 同名图像 + 视频 → 一张实况照片；
 * - 可拆分的合并文件 → 一张实况照片；
 * - 其余图像 → 普通照片；
 * - 未配对的视频/未知文件 → 跳过并给出原因。
 */
export async function buildUploadPlans(files: File[]): Promise<UploadPlan[]> {
  const images: File[] = []
  const videos: File[] = []
  const skipped: SkippedUploadPlan[] = []

  for (const file of files) {
    if (isImageFile(file)) {
      images.push(file)
    } else if (isVideoFile(file)) {
      videos.push(file)
    } else {
      skipped.push({ kind: 'skipped', name: file.name, reason: '不支持的文件类型，已跳过' })
    }
  }

  const usedVideos = new Set<File>()
  const plans: UploadPlan[] = []

  for (const image of images) {
    const base = baseName(image.name).toLowerCase()
    const paired = videos.find((v) => !usedVideos.has(v) && baseName(v.name).toLowerCase() === base)
    if (paired) {
      usedVideos.add(paired)
      plans.push({
        kind: 'live',
        name: baseName(image.name),
        photo: partOf(image, imageContentType(image)),
        video: partOf(paired, videoContentType(paired)),
      })
      continue
    }

    if (SPLIT_EXT.has(fileExt(image.name))) {
      const split = await splitMergedLivePhoto(image)
      if (split) {
        plans.push({ kind: 'live', name: baseName(image.name), ...split })
        continue
      }
    }

    plans.push({ kind: 'photo', name: baseName(image.name), photo: partOf(image, imageContentType(image)) })
  }

  for (const video of videos) {
    if (usedVideos.has(video)) continue
    plans.push({
      kind: 'skipped',
      name: video.name,
      reason: '未找到同名照片，视频不会单独上传',
    })
  }

  return [...plans, ...skipped]
}

/** 上传计划的总字节数，用于把两个对象的进度合成一个百分比。 */
export function planBytes(plan: UploadPlan): number {
  if (plan.kind === 'photo') return plan.photo.blob.size
  if (plan.kind === 'live') return plan.photo.blob.size + plan.video.blob.size
  return 0
}
