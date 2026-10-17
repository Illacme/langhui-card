import http from './request'

// 企业管理（多租户第一期，仅平台超级管理员可用）：
// 开通/停用企业、企业管理员管理。企业管理员调用会被后端 403 拒绝。

export interface CompanyItem {
  id: number
  name: string
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

// 企业列表（含员工/管理员数量）
export function listCompanies() {
  return http.get<{ code: number; data?: CompanyItem[] }>('/admin/companies')
}

// 开通企业：name 必填；adminUsername + adminPassword 要么都填（一并建企业管理员），要么都不填
export function createCompany(payload: {
  name: string
  adminUsername?: string
  adminPassword?: string
}) {
  return http.post<{ code: number; data?: { id: number; name: string } }>('/admin/companies', payload)
}

// 停用 / 启用企业（停用后该企业的管理员与员工均不能登录）
export function setCompanyStatus(id: number, status: 'enabled' | 'disabled') {
  return http.post(`/admin/companies/${id}/status`, { status })
}

// 企业管理员列表
export function listCompanyAdmins(id: number) {
  return http.get<{ code: number; data?: CompanyAdminItem[] }>(`/admin/companies/${id}/admins`)
}

// 新增企业管理员
export function createCompanyAdmin(id: number, username: string, password: string) {
  return http.post(`/admin/companies/${id}/admins`, { username, password })
}
