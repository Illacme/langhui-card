import http, { toPaged, type Paged } from './request'

// 企业管理（多租户第一期，仅平台超级管理员可用）：
// 开通/停用企业、企业管理员管理。企业管理员调用会被后端 403 拒绝。
// 列表走服务端分页（page/size/keyword/status），见 api/request.ts 的 Paged 约定。

export interface CompanyItem {
  id: number
  name: string
  shortName: string
  status: 'enabled' | 'disabled'
  createdAt: string
  employeeCount: number
  adminCount: number
}

export interface CompanyAdminItem {
  id: number
  username: string
  createdAt: string
}

// 企业列表（含员工/管理员数量），服务端分页 + 关键字（名称/简称）+ 状态筛选
export function listCompanies(params: {
  page?: number
  size?: number
  keyword?: string
  status?: string
} = {}): Promise<Paged<CompanyItem>> {
  return http.get('/admin/companies', { params }).then((r) => toPaged<CompanyItem>(r))
}

// 开通企业：name 必填，shortName 为企业简称（侧边栏品牌名 + 公司信息简称初始值）；
// adminUsername + adminPassword 要么都填（一并建企业管理员），要么都不填
export function createCompany(payload: {
  name: string
  shortName?: string
  adminUsername?: string
  adminPassword?: string
}) {
  return http.post<{ code: number; data?: { id: number; name: string; shortName: string } }>(
    '/admin/companies',
    payload
  )
}

// 编辑企业（名称 / 简称）：简称改后同步侧边栏品牌名，但不回写公司信息里的简称
// （公司信息简称仅用于小程序展示，由企业管理员在「公司信息」页自行维护）
export function updateCompany(id: number, payload: { name: string; shortName?: string }) {
  return http.put(`/admin/companies/${id}`, payload)
}

// 停用 / 启用企业（停用后该企业的管理员与员工均不能登录）
export function setCompanyStatus(id: number, status: 'enabled' | 'disabled') {
  return http.post(`/admin/companies/${id}/status`, { status })
}

// 企业管理员列表：单个企业下的管理员数量天然有界（几个），不做分页，
// 保持返回数组——分页只为「会随时间增长」的列表服务，别为了统一而统一。
export function listCompanyAdmins(id: number) {
  return http.get<{ code: number; data?: CompanyAdminItem[] }>(`/admin/companies/${id}/admins`)
}

// 新增企业管理员
export function createCompanyAdmin(id: number, username: string, password: string) {
  return http.post(`/admin/companies/${id}/admins`, { username, password })
}
