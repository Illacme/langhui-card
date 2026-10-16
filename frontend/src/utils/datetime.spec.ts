import { describe, it, expect } from 'vitest'
import { formatDateTime } from './datetime'

// 断言只依赖「本地时区分量」，不写死 UTC 期望值，因此在任意时区下都稳定
// （CI 与开发者机器时区不同也不会飘红）。
describe('formatDateTime', () => {
  it('Date 输入按本地分量输出 YYYY-MM-DD HH:mm:ss', () => {
    expect(formatDateTime(new Date(2026, 9, 16, 15, 34, 12))).toBe('2026-10-16 15:34:12')
  })

  it('月/日/时/分/秒均补零到两位', () => {
    expect(formatDateTime(new Date(2026, 0, 2, 3, 4, 5))).toBe('2026-01-02 03:04:05')
  })

  it('RFC3339 字符串剥掉 T / 纳秒 / 时区后缀', () => {
    const out = formatDateTime('2026-10-16T15:34:12.123456789+08:00')
    expect(out).toMatch(/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/)
    expect(out).not.toContain('T')
    expect(out).not.toContain('Z')
    expect(out).not.toContain('.')
  })

  it('UTC 后缀（Z）同样被格式化而非原样透出', () => {
    expect(formatDateTime('2026-10-16T07:34:12Z')).toMatch(
      /^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/
    )
  })

  it('空值返回破折号占位', () => {
    expect(formatDateTime(null)).toBe('—')
    expect(formatDateTime(undefined)).toBe('—')
    expect(formatDateTime('')).toBe('—')
  })

  it('非法时间原样返回，便于暴露脏数据', () => {
    expect(formatDateTime('not-a-date')).toBe('not-a-date')
  })
})
