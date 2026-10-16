// 时间格式化：后端 Go time.Time 序列化为 RFC3339（2026-10-16T15:34:12.123+08:00），
// 直接绑到 wxml 会把 T／纳秒／时区后缀一起显示出来。
// 与 PC 后台 frontend/src/utils/datetime.ts 同一套口径，两端展示一致。
// 空值返回「—」；非法时间原样回显，不静默吞掉，便于暴露脏数据。
export function formatDateTime(value: string | number | Date | null | undefined): string {
  if (value === null || value === undefined || value === '') return '—'
  const d = value instanceof Date ? value : new Date(value)
  if (Number.isNaN(d.getTime())) return String(value)
  const pad = (n: number) => String(n).padStart(2, '0')
  return (
    `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}` +
    ` ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
  )
}
