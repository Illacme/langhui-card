import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import StatTrend, { type TrendPoint } from './StatTrend.vue'

// StatTrend 是纯 SVG 组件（无 Element Plus 依赖），直接挂真实组件验几何与文案。

const pts: TrendPoint[] = [
  { date: '2026-10-21', visit: 4, call: 1, save: 2, projectView: 2, serviceView: 1 },
  { date: '2026-10-22', visit: 8, call: 2, save: 3, projectView: 6, serviceView: 3 },
  { date: '2026-10-23', visit: 2, call: 0, save: 5, projectView: 1, serviceView: 0 },
]

function mountTrend(points: TrendPoint[]) {
  return mount(StatTrend, { props: { points } })
}

// 图例项按文案定位（八项里点名取用，比按下标稳）
function legend(w: ReturnType<typeof mountTrend>, label: string) {
  const item = w.findAll('.legend-item').find((el) => el.text().includes(label))
  if (!item) throw new Error(`未找到图例项：${label}`)
  return item
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

  it('图例含八个指标（转化三 + 意向五）与区间文字', () => {
    const w = mountTrend(pts)
    expect(w.findAll('.legend-item')).toHaveLength(8)
    expect(w.text()).toContain('访问')
    expect(w.text()).toContain('拨号')
    expect(w.text()).toContain('保存')
    expect(w.text()).toContain('案例浏览')
    expect(w.text()).toContain('服务浏览')
    expect(w.text()).toContain('视频播放')
    expect(w.text()).toContain('转发')
    expect(w.text()).toContain('海报保存')
    expect(w.text()).toContain('2026-10-21 ~ 2026-10-23')
  })

  it('默认只画转化三线（八条全开会糊成一团）', () => {
    const w = mountTrend(pts)
    expect(w.findAll('polyline')).toHaveLength(3)
    // 未勾选的意向项压暗但仍在图例里（让用户知道还能勾）
    expect(legend(w, '案例浏览').classes()).toContain('legend-off')
    expect(legend(w, '访问').classes()).not.toContain('legend-off')
  })

  it('勾选意向指标后多画一条线，纵轴按激活系列重算', async () => {
    const w = mountTrend(pts)
    expect(w.findAll('polyline')).toHaveLength(3)

    await legend(w, '案例浏览').trigger('click')
    expect(w.findAll('polyline')).toHaveLength(4)
    expect(legend(w, '案例浏览').classes()).not.toContain('legend-off')
    // 案例浏览最大值 6 < 访问 8：纵轴上限仍是 8（取所有激活系列的全局最大）
    expect(w.findAll('text').map((t) => t.text())).toContain('8')

    // 只留案例浏览：纵轴上限应贴合它（6），否则线会被压成贴底直线
    await legend(w, '访问').trigger('click')
    await legend(w, '拨号').trigger('click')
    await legend(w, '保存').trigger('click')
    expect(w.findAll('polyline')).toHaveLength(1)
    const labels = w.findAll('text').map((t) => t.text())
    expect(labels).toContain('6')
    expect(labels).not.toContain('8')
  })

  it('最后一个激活项点不掉（不允许出现空图）', async () => {
    const w = mountTrend(pts)
    await legend(w, '拨号').trigger('click')
    await legend(w, '保存').trigger('click')
    expect(w.findAll('polyline')).toHaveLength(1)
    // 再点唯一剩下的「访问」应无效，图仍在
    await legend(w, '访问').trigger('click')
    expect(w.findAll('polyline')).toHaveLength(1)
    expect(legend(w, '访问').classes()).not.toContain('legend-off')
    // 点击热区也仍然可用（从前锚死在 visit 上，勾掉访问就点不动了）
    await w.findAll('circle.hit-dot')[0].trigger('click')
    expect(w.emitted('pick')).toEqual([['2026-10-21']])
  })

  it('老副本不回意向项时按 0 画，不出现 NaN 坐标', async () => {
    // 只回三项（后端未升级时不带意向列）：勾出案例浏览应得到贴底的 0 线
    const w = mountTrend([
      { date: '2026-10-21', visit: 4, call: 1, save: 2 },
      { date: '2026-10-22', visit: 8, call: 2, save: 3 },
    ])
    await legend(w, '案例浏览').trigger('click')
    expect(w.findAll('polyline')).toHaveLength(4)
    expect(w.html()).not.toContain('NaN')
    expect(w.html()).not.toContain('undefined')
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
