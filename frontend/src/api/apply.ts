import http from './request'

// 开通申请列表
export function listOpenApply() {
  return http.get('/admin/apply/open').then((r: any) => (r.data ?? []) as any[])
}

// 开通申请通过 → 自动建员工账号，返回 staffId + 一次性初始密码
export function approveOpen(id: number) {
  return http.post(`/admin/apply/open/${id}/approve`).then((r: any) => ({
    staffId: r.staffId as number,
    initialPassword: r.initialPassword as string,
  }))
}

// 开通申请驳回
export function rejectOpen(id: number, reason: string) {
  return http.post(`/admin/apply/open/${id}/reject`, { reason })
}

// 名片修改申请列表（snapshot 为 JSON 字符串）
export function listModifyApply() {
  return http.get('/admin/apply/modify').then((r: any) => (r.data ?? []) as any[])
}

// 名片修改通过 → 将 snapshot 应用到员工
export function approveModify(id: number) {
  return http.post(`/admin/apply/modify/${id}/approve`)
}

// 名片修改驳回
export function rejectModify(id: number, reason: string) {
  return http.post(`/admin/apply/modify/${id}/reject`, { reason })
}
