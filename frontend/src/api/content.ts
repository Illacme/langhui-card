import http from './request'

// ===== 公司信息（id=1 单行，GET 空值兜底，PUT 自动建首行） =====
export function getCompanyInfo() {
  return http.get('/admin/company-info').then((r: any) => (r.data ?? { id: 1 }) as any)
}
export function saveCompanyInfo(p: {
  fullName: string
  shortName: string
  address: string
  tel: string
  logo: string
}) {
  return http.put('/admin/company-info', p).then((r: any) => r.data as any)
}

// ===== 公司简介 + 视频号 =====
export function getCompanyProfile() {
  return http.get('/admin/company-profile').then((r: any) => (r.data ?? { id: 1 }) as any)
}
export function saveCompanyProfile(p: {
  introText: string
  videoChannelId: string
  backupMp4Url: string
  cover: string
}) {
  return http.put('/admin/company-profile', p).then((r: any) => r.data as any)
}

// ===== 业绩分类 =====
export function listCategories() {
  return http.get('/admin/categories').then((r: any) => (r.data ?? []) as any[])
}
export function createCategory(p: { name: string; icon?: string; sort: number; isLocked?: boolean }) {
  return http.post('/admin/categories', p).then((r: any) => r.data as any)
}
export function updateCategory(
  id: number,
  p: { name: string; icon?: string; sort: number; isLocked?: boolean }
) {
  return http.put(`/admin/categories/${id}`, p)
}
export function deleteCategory(id: number) {
  return http.delete(`/admin/categories/${id}`)
}
// 批量保存排序（拖拽 / 上下箭头调整顺序后一次性提交）
export function sortCategories(items: { id: number; sort: number }[]) {
  return http.put('/admin/categories/sort', items)
}

// ===== 项目案例 =====
export function listProjects(categoryId?: number) {
  return http
    .get('/admin/projects', { params: categoryId ? { categoryId } : {} })
    .then((r: any) => (r.data ?? []) as any[])
}
export function createProject(p: any) {
  return http.post('/admin/projects', p).then((r: any) => r.data as any)
}
export function updateProject(id: number, p: any) {
  return http.put(`/admin/projects/${id}`, p).then((r: any) => r.data as any)
}
export function deleteProject(id: number) {
  return http.delete(`/admin/projects/${id}`)
}
// 批量保存排序（拖拽 / 上下箭头调整顺序后一次性提交）
export function sortProjects(items: { id: number; sort: number }[]) {
  return http.put('/admin/projects/sort', items)
}

// ===== 业务服务 =====
export function listServices() {
  return http.get('/admin/services').then((r: any) => (r.data ?? []) as any[])
}
export function createService(p: { title: string; icon?: string; content: string; sort: number }) {
  return http.post('/admin/services', p).then((r: any) => r.data as any)
}
export function updateService(id: number, p: { title: string; icon?: string; content: string; sort: number }) {
  return http.put(`/admin/services/${id}`, p)
}
export function deleteService(id: number) {
  return http.delete(`/admin/services/${id}`)
}
// 批量保存排序（拖拽 / 上下箭头调整顺序后一次性提交）
export function sortServices(items: { id: number; sort: number }[]) {
  return http.put('/admin/services/sort', items)
}

// ===== 首页布局（板块顺序） =====
export function getHomeLayout() {
  return http.get('/admin/home-layout').then((r: any) => (r.data?.order ?? []) as string[])
}
export function saveHomeLayout(order: string[]) {
  return http.put('/admin/home-layout', { order })
}
