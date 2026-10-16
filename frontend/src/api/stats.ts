import http from './request'

// 全局名片统计（超管）：汇总 + 按员工分组。
// 与员工端 /api/employee/me/stat（仅本人）互补，见 backend/internal/handler/apply.go。
export function adminStats() {
  return http.get('/admin/stats').then((r: any) => ({
    summary: r.summary ?? { visit: 0, call: 0, save: 0 },
    byStaff: (r.byStaff ?? []) as any[],
  }))
}
