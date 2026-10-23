import axios from 'axios'
import http from './request'

// 全局名片统计（超管）：汇总 + 按员工 + 按天趋势。
// 与员工端 /api/employee/me/stat（仅本人）互补，见 backend/internal/handler/apply.go。
//
// from/to 为 YYYY-MM-DD（闭区间），均可缺省——缺省即「全部时间」口径。
export interface StatRange {
  from?: string
  to?: string
}

// 清掉空值再进 params：缺省日期不应变成 from=&to= 传给后端
// （后端虽也按空串处理，但请求 URL 干净些，日志与排查都少一层歧义）。
function cleanParams(range: StatRange): Record<string, string> {
  const out: Record<string, string> = {}
  if (range.from) out.from = range.from
  if (range.to) out.to = range.to
  return out
}

export function adminStats(range: StatRange = {}) {
  // 响应拦截器回传整包信封 {code,msg,data}，汇总/按员工/趋势均在 data 下
  return http.get('/admin/stats', { params: cleanParams(range) }).then((r: any) => ({
    summary: r.data?.summary ?? { visit: 0, call: 0, save: 0 },
    byStaff: (r.data?.byStaff ?? []) as any[],
    trend: (r.data?.trend ?? []) as any[],
  }))
}

// 导出走独立 axios 实例（无业务拦截器）：拦截器按 {code,msg} 信封解析，
// 而 CSV 响应体不是 JSON——走拦截器会被误判成「业务失败」弹红条并 reject。
// withCredentials 照旧带上（超管是 httpOnly Cookie 鉴权）。
const rawHttp = axios.create({
  baseURL: import.meta.env.VITE_API_BASE || '/api',
  withCredentials: true,
})

/** 导出统计 CSV 并触发浏览器下载（文件名取自 Content-Disposition）。 */
export async function exportStatsCsv(range: StatRange = {}): Promise<void> {
  const resp = await rawHttp.get('/admin/stats/export', {
    params: cleanParams(range),
    responseType: 'blob',
  })
  // 后端业务失败（code:1 / 401 等）走的是 HTTP 200 + JSON 信封，
  // 在 responseType:'blob' 下会变成一个装着 JSON 文本的 Blob——
  // 不拦就会把错误信封当 CSV 下载给用户（打开是一串 JSON，还以 .csv 命名）。
  const ct = String(resp.headers['content-type'] || '')
  if (ct.includes('json')) {
    const text = await (resp.data as Blob).text()
    let msg = '导出失败'
    try {
      msg = JSON.parse(text).msg || msg
    } catch {
      /* 非标准信封：用兜底文案 */
    }
    throw new Error(msg)
  }
  const filename = parseCsvFilename(resp.headers['content-disposition'])
  const url = URL.createObjectURL(new Blob([resp.data], { type: 'text/csv;charset=utf-8' }))
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  a.remove()
  // 立即 revoke 在部分浏览器会取消尚未开始的下载，故延后一拍再回收
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}

/**
 * 从 Content-Disposition 解析下载文件名。
 * 优先 filename*（RFC 5987，服务端中文名走这里，百分号编码），
 * 回落 filename，再兜底 stats.csv。解析失败不抛——下载本身已成功，
 * 文件名只是个体验问题，抛错反而会让调用方以为导出失败。
 */
export function parseCsvFilename(disposition: string | undefined | null): string {
  if (!disposition) return 'stats.csv'
  const star = /filename\*=UTF-8''([^;]+)/i.exec(disposition)
  if (star) {
    try {
      return decodeURIComponent(star[1])
    } catch {
      return 'stats.csv'
    }
  }
  const plain = /filename="?([^";]+)"?/i.exec(disposition)
  return plain ? plain[1] : 'stats.csv'
}
