// utils/track：埋点统一入口的载荷与边界纪律。
// 这些是「静默约定」——写错不报错，只会在后台统计里表现为对不上账，
// 所以用测试钉住。
import { describe, it, expect, beforeEach, vi } from 'vitest'

const mocks = vi.hoisted(() => ({
  request: vi.fn(async (..._args: any[]): Promise<any> => ({})),
}))
vi.mock('../utils/request', () => ({ request: mocks.request }))

beforeEach(() => {
  vi.resetModules()
  mocks.request.mockClear()
})

async function loadTrack() {
  return await import('../utils/track')
}

describe('utils/track', () => {
  it('targetId>0 时带上该字段（TOP 榜聚合的依据）', async () => {
    const { track } = await loadTrack()

    track('project_view', 42, 7)

    expect(mocks.request).toHaveBeenCalledWith('POST', '/api/stat/track', {
      staffId: 42,
      eventType: 'project_view',
      targetId: 7,
    })
  })

  // 老载荷形状不变：早期抓包与既有 visit 断言的字段集一致，
  // 后端对缺省 targetId 按 0 处理
  it('targetId 为 0 时省略该字段', async () => {
    const { track } = await loadTrack()

    track('visit', 42)

    expect(mocks.request).toHaveBeenCalledWith('POST', '/api/stat/track', {
      staffId: 42,
      eventType: 'visit',
    })
  })

  // 统计按员工聚合：staffId=0 的事件没有归属，进了后台只是对不上账的噪声
  it('staffId 为 0：丢弃不发请求', async () => {
    const { track } = await loadTrack()

    track('visit', 0)

    expect(mocks.request).not.toHaveBeenCalled()
  })

  // 后端白名单是权威，端上就近拦掉：少一次注定被拒的无用请求
  it('白名单外事件类型：就近丢弃', async () => {
    const { track } = await loadTrack()

    track('hack' as any, 42)

    expect(mocks.request).not.toHaveBeenCalled()
  })

  // 纪律 1：失败静默——不 throw，rejection 也被吃掉（不会变成
  // unhandledrejection 在真机上刷错误日志）
  it('上报失败：静默吞掉，不向调用方抛', async () => {
    mocks.request.mockRejectedValueOnce(new Error('offline'))
    const { track } = await loadTrack()

    expect(() => track('visit', 42)).not.toThrow()
    await new Promise((r) => setTimeout(r, 0)) // 放行微任务，确认没有未决 rejection
  })
})

// 流量来源（source）：端上推导的语义轻易写错——写错不报错，只表现为后台
// 来源分布悄悄失真（比如把扫码全算成转发、海报渠道永远统计不到），故用测试钉住。
describe('utils/track 来源推导', () => {
  it('转发卡片场景 → share_card（裂变榜的数据来源）', async () => {
    const { sceneToSource } = await loadTrack()
    expect(sceneToSource(1007)).toBe('share_card')
    expect(sceneToSource(1008)).toBe('share_card')
    expect(sceneToSource(1036)).toBe('share_card')
  })

  it('扫码场景 → scan；搜索入口 → search', async () => {
    const { sceneToSource } = await loadTrack()
    expect(sceneToSource(1011)).toBe('scan')
    expect(sceneToSource(1047)).toBe('scan')
    expect(sceneToSource(1001)).toBe('search')
  })

  // 识别不了必须回落空串（而不是 'unknown'）：空串让 track() 省略该字段，
  // 保持老载荷形状；后端对空串同样归一为 unknown，统计口径一致。
  it('识别不了的场景 → 空串（不携带 source，后端归一为 unknown）', async () => {
    const { sceneToSource } = await loadTrack()
    expect(sceneToSource(0)).toBe('')
    expect(sceneToSource(9999)).toBe('')
  })

  // 海报标记优先于 scene：扫海报码与扫普通码的 scene 同在扫码区间，
  // 只有 from=poster 能区分（标记由后端埋进小程序码的 scene 串）。
  it('deriveSource 优先认海报标记（query.from / query.scene 里的 from=poster）', async () => {
    const { deriveSource } = await loadTrack()
    expect(deriveSource(1011, { from: 'poster' })).toBe('poster')
    expect(deriveSource(1011, { scene: encodeURIComponent('staffId=42&from=poster') })).toBe('poster')
    // 同是扫码场景、无标记 → 就是普通扫码
    expect(deriveSource(1011, { scene: encodeURIComponent('staffId=42') })).toBe('scan')
    expect(deriveSource(1011)).toBe('scan')
  })

  it('确实有来源时才带 source 字段；空串省略（老载荷形状不变）', async () => {
    const { track } = await loadTrack()

    track('visit', 42, 0, 'share_card')
    expect(mocks.request).toHaveBeenLastCalledWith('POST', '/api/stat/track', {
      staffId: 42,
      eventType: 'visit',
      source: 'share_card',
    })

    track('visit', 42, 0, '')
    expect(mocks.request).toHaveBeenLastCalledWith('POST', '/api/stat/track', {
      staffId: 42,
      eventType: 'visit',
    })
  })
})
