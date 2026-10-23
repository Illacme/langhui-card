import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import StatTrend, { type TrendPoint } from './StatTrend.vue'

// StatTrend 是纯 SVG 组件（无 Element Plus 依赖），直接挂真实组件验几何与文案。

const pts: TrendPoint[] = [
  { date: '2026-10-21', visit: 4, call: 1, save: 2 },
  { date: '2026-10-22', visit: 8, call: 2, save: 3 },
  { date: '2026-10-23', visit: 2, call: 0, save: 5 },
]

function mountTrend(points: TrendPoint[]) {
  return mount(StatTrend, { props: { points } })
}

describe('StatTrend', () => {
  it('三条折线各连全部数据点', () => {
    const w = mountTrend(pts)
    const lines = w.findAll('polyline')
    expect(lines).toHaveLength(3)
    for (const line of lines) {
      // 每个点一对 "x,y"，三点共 6 个数字（按空白与逗号切）
      expect(line.attributes('points')!.trim().split(/[\s,]+/)).toHaveLength(6)
    }
  })

  it('X 轴只打首/中/尾三个日期标签（月-日）', () => {
    const w = mountTrend(pts)
    const labels = w.findAll('text').map((t) => t.text())
    expect(labels).toContain('10-21')
    expect(labels).toContain('10-22')
    expect(labels).toContain('10-23')
  })

  it('Y 轴上限取三系列的最大值（8），并给出 0 刻度', () => {
    const w = mountTrend(pts)
    const labels = w.findAll('text').map((t) => t.text())
    expect(labels).toContain('8')
    expect(labels).toContain('4') // 中值
    expect(labels).toContain('0')
  })

  it('图例含访问/拨号/保存三项与区间文字', () => {
    const w = mountTrend(pts)
    expect(w.text()).toContain('访问')
    expect(w.text()).toContain('拨号')
    expect(w.text()).toContain('保存')
    expect(w.text()).toContain('2026-10-21 ~ 2026-10-23')
  })

  it('数据点 tooltip 带日期 + 指标名 + 数值', () => {
    const w = mountTrend(pts)
    const titles = w.findAll('title').map((t) => t.text())
    expect(titles).toContain('2026-10-22 访问 8')
    expect(titles).toContain('2026-10-23 保存 5')
  })

  it('全零数据仍画线（纵轴兜底为 1，不除零）', () => {
    const w = mountTrend([
      { date: '2026-10-21', visit: 0, call: 0, save: 0 },
      { date: '2026-10-22', visit: 0, call: 0, save: 0 },
    ])
    expect(w.findAll('polyline')).toHaveLength(3)
    expect(w.findAll('text').map((t) => t.text())).toContain('1')
  })

  it('单点只画圆点不画面积与折线路径', () => {
    const w = mountTrend([{ date: '2026-10-23', visit: 3, call: 1, save: 1 }])
    expect(w.findAll('polygon')).toHaveLength(0)
    // 三个系列数据点 + 一个点击热区（透明大圆，下钻当天明细用）
    expect(w.findAll('circle:not(.hit-dot)')).toHaveLength(3)
    expect(w.findAll('circle.hit-dot')).toHaveLength(1)
    expect(w.text()).toContain('2026-10-23')
  })

  it('点数据点热区发出 pick 事件（下钻当天的明细）', async () => {
    const w = mountTrend(pts)
    const hits = w.findAll('circle.hit-dot')
    expect(hits).toHaveLength(3)
    await hits[1].trigger('click')
    expect(w.emitted('pick')).toEqual([['2026-10-22']])
  })

  it('无数据走空态文案，不渲染 SVG', () => {
    const w = mountTrend([])
    expect(w.find('svg').exists()).toBe(false)
    expect(w.text()).toContain('所选时段暂无数据')
  })
})
