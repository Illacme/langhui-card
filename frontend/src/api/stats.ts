import http from './request'

// 全局名片统计（超管）：汇总 + 按员工分组。
// 与员工端 /api/employee/me/stat（仅本人）互补，见 backend/internal/handler/apply.go。
export function adminStats() {
  // 响应拦截器回传整包信封 {code,msg,data}，汇总与按员工列表均在 data 下
  return http.get('/admin/stats').then((r: any) => ({
    summary: r.data?.summary ?? { visit: 0, call: 0, save: 0 },
    byStaff: (r.data?.byStaff ?? []) as any[],
  }))
}
