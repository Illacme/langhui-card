import http from './request'

export interface EmployeePayload {
  name: string
  phone: string
  email?: string
  title?: string
  landline?: string
  avatar?: string
}

// 员工列表（按 staffId 升序）
export function listEmployees() {
  return http.get('/admin/employees').then((r: any) => (r.data ?? []) as any[])
}

// 新增员工 → 返回 staffId + 一次性初始密码
export function createEmployee(payload: EmployeePayload) {
  return http.post('/admin/employees', payload).then((r: any) => ({
    staffId: r.staffId as number,
    initialPassword: r.initialPassword as string,
  }))
}

// 超管编辑员工资料（即时生效，不经过员工端审核流）
export function updateEmployee(id: number, payload: EmployeePayload) {
  return http.put(`/admin/employees/${id}`, payload)
}

export function disableEmployee(id: number) {
  return http.post(`/admin/employees/${id}/disable`)
}

export function enableEmployee(id: number) {
  return http.post(`/admin/employees/${id}/enable`)
}

// 重置密码 → 返回一次性初始密码
export function resetPassword(id: number) {
  return http.post(`/admin/employees/${id}/reset-password`).then((r: any) => r.initialPassword as string)
}
