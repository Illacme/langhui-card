import http from './request'

// 常用开源素材库（后端内嵌，见 backend/internal/assets）：
// 所有需要图片的位置都能直接选内置素材，免去四处找图、按尺寸处理图。
// 采用（adopt）= 后端把素材复制进上传存储，返回与普通上传完全同构的 URL，
// 因此下游（小程序 <image> / CDN）零感知。

export interface LibraryCategory {
  key: string
  name: string
}

export interface LibraryItem {
  id: string
  name: string
  category: string // icon / cover / logo
  file: string
  thumb: string // 预览图相对路径（封面为缩略版）
  width: number
  height: number
  tags: string[]
}

export interface LibraryData {
  version: number
  categories: LibraryCategory[]
  items: LibraryItem[]
}

// 素材清单（管理员登录态）
export function listLibrary() {
  return http.get<{ code: number; data?: LibraryData }>('/admin/assets/library')
}

// 采用素材：返回可直接填入表单的 URL
export function adoptAsset(id: string) {
  return http.post<{
    code: number
    data?: { url: string; item?: { id: string; name: string; category: string } }
  }>('/admin/assets/adopt', { id })
}

// 预览图 URL：公开端点（/api/assets/file/...），长缓存，与登录态无关
export function libraryFileUrl(rel: string) {
  const base = import.meta.env.VITE_API_BASE || '/api'
  return `${base.replace(/\/$/, '')}/assets/file/${rel}`
}
