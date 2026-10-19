import http, { toPaged, type Paged } from './request'

// 开通申请列表（服务端分页；keyword 匹配姓名/手机/邮箱/昵称，status 为 pending/approved/rejected）
export function listOpenApply(
  params: { page?: number; size?: number; keyword?: string; status?: string } = {}
): Promise<Paged<any>> {
  return http.get('/admin/apply/open', { params }).then((r) => toPaged(r))
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

// 名片修改申请列表（服务端分页；keyword 按员工姓名匹配——申请行本身只有 staff_id
// 与变更快照，运营搜索的实际意图是「找某个人的申请」）
export function listModifyApply(
  params: { page?: number; size?: number; keyword?: string; status?: string } = {}
): Promise<Paged<any>> {
  return http.get('/admin/apply/modify', { params }).then((r) => toPaged(r))
}

// 名片修改通过 → 将 snapshot 应用到员工
export function approveModify(id: number) {
  return http.post(`/admin/apply/modify/${id}/approve`)
}

// 名片修改驳回
export function rejectModify(id: number, reason: string) {
  return http.post(`/admin/apply/modify/${id}/reject`, { reason })
}
