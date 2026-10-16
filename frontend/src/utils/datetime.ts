/**
 * 时间格式化工具。
 *
 * 背景：后端 model 的 CreatedAt/UpdatedAt 是 Go 的 time.Time，经 encoding/json
 * 序列化为 RFC3339（例：`2026-10-16T15:34:12.123456789+08:00`）。直接绑到表格列上
 * 会连 T、纳秒和时区后缀一起显示，因此后台列表统一转成 `YYYY-MM-DD HH:mm:ss`。
 *
 * 实现说明：用本地时区分量（getFullYear 等）而非 toISOString()，这样展示的是
 * 「浏览器所在时区的时间」，与用户直觉一致，也避免 UTC 换算后跨日。
 */

export function formatDateTime(value: string | number | Date | null | undefined): string {
  if (value === null || value === undefined || value === '') return '—'
  const d = value instanceof Date ? value : new Date(value)
  // 非法时间（例如后端字段缺失给到 'undefined' 字符串）不静默吞掉，原样回显便于排查
  if (Number.isNaN(d.getTime())) return String(value)
  const pad = (n: number) => String(n).padStart(2, '0')
  return (
    `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}` +
    ` ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
  )
}
