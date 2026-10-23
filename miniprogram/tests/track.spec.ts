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
