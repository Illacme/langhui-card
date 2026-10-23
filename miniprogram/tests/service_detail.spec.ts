// 业务服务详情页：与项目详情同构，但正文取自 content 字段、空态文案与默认分享标题不同。
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { installPageCapture, capturedPage, makeInstance, installWx, flush } from './helpers/harness'

const mocks = vi.hoisted(() => ({
  request: vi.fn(async (..._args: any[]): Promise<any> => ({})),
  companyId: 0,
}))

vi.mock('../utils/request', () => ({
  request: mocks.request,
  assetUrl: (u: string) =>
    u ? (/^https?:\/\//i.test(u) ? u : `https://cdn.test${u.startsWith('/') ? '' : '/'}${u}`) : '',
  apiBaseUrl: () => 'https://api.test',
}))
vi.mock('../utils/tenant', () => ({
  getCompanyId: () => mocks.companyId,
  bindCompanyFromScene: vi.fn(),
}))

let wxStub: any

beforeEach(() => {
  vi.resetModules()
  installPageCapture()
  wxStub = installWx().wx
  mocks.request.mockClear()
  mocks.companyId = 0
})

async function page(query: any = {}) {
  await import('../pages/service/detail/detail')
  const p = makeInstance(capturedPage())
  p.onLoad(query)
  await flush()
  return p
}

const service = (extra: any = {}) => ({
  id: 3,
  title: '中央空调维保',
  content: '<p>季度保养</p>',
  updatedAt: '2026-10-16T15:34:12',
  ...extra,
})

describe('service/detail 页', () => {
  it('onLoad 解析 id / staffId 并按 id 拉详情', async () => {
    mocks.request.mockResolvedValueOnce({ code: 0, data: service() })
    const p = await page({ id: '3', staffId: '42' })

    expect(p.data.id).toBe(3)
    expect(p.data.staffId).toBe(42)
    expect(mocks.request).toHaveBeenCalledWith('GET', '/api/services/3')
  })

  it('正文取自 content 字段并解析为片段', async () => {
    mocks.request.mockResolvedValueOnce({ code: 0, data: service() })
    const p = await page({ id: '3' })

    expect(p.data.service.title).toBe('中央空调维保')
    expect(p.data.segments).toEqual([{ type: 'html', content: '<p>季度保养</p>' }])
  })

  it('content 为空：片段为空数组（不产出 [undefined]）', async () => {
    mocks.request.mockResolvedValueOnce({ code: 0, data: service({ content: '' }) })
    const p = await page({ id: '3' })

    expect(p.data.segments).toEqual([])
  })

  it('更新时间格式化（不含 T）', async () => {
    mocks.request.mockResolvedValueOnce({ code: 0, data: service() })
    const p = await page({ id: '3' })

    expect(p.data.updatedAtText).toBe('2026-10-16 15:34:12')
  })

  it('服务不存在：提示并结束加载', async () => {
    mocks.request.mockResolvedValueOnce({ code: 1 })
    const p = await page({ id: '3' })

    expect(wxStub.showToast).toHaveBeenCalledWith({ title: '服务不存在', icon: 'none' })
    expect(p.data.service).toBeNull()
    expect(p.data.loading).toBe(false)
  })

  it('请求异常：error 置位且 loading 落下', async () => {
    mocks.request.mockRejectedValueOnce(new Error('offline'))
    const p = await page({ id: '3' })

    expect(p.data.error).toBe(true)
    expect(p.data.loading).toBe(false)
  })

  it('分享：标题用服务名，路径回带 id / staffId / companyId', async () => {
    mocks.request.mockResolvedValueOnce({ code: 0, data: service() })
    mocks.companyId = 5
    const p = await page({ id: '3', staffId: '42' })

    const share = p.onShareAppMessage()

    expect(share.title).toBe('中央空调维保')
    expect(share.path).toBe('/pages/service/detail/detail?id=3&staffId=42&companyId=5')
  })

  it('详情未加载时分享用默认标题「业务服务」', async () => {
    mocks.request.mockRejectedValueOnce(new Error('offline'))
    const p = await page({ id: '3' })

    expect(p.onShareAppMessage().title).toBe('业务服务')
  })
})
