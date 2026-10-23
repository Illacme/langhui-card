// 员工数据统计页（B10）：总访问 / 拨号点击 / 保存点击。
// 关键约束是「未登录或令牌失效时必须回登录页」——这页没有游客态，
// 卡在空页对员工来说就是「数据没了」。
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { installPageCapture, capturedPage, makeInstance, installWx, flush } from './helpers/harness'

const mocks = vi.hoisted(() => ({
  request: vi.fn(async (..._args: any[]): Promise<any> => ({ code: 0, data: {} })),
}))

vi.mock('../utils/request', () => ({
  request: mocks.request,
  assetUrl: (u: string) => u || '',
  apiBaseUrl: () => 'https://api.test',
}))

let wxStub: any
let storage: Map<string, any>

beforeEach(() => {
  vi.resetModules()
  installPageCapture()
  const installed = installWx()
  wxStub = installed.wx
  storage = installed.storage
  mocks.request.mockClear()
})

async function page() {
  await import('../pages/employee/stat/stat')
  return makeInstance(capturedPage())
}

function lastToast(): string {
  const calls = wxStub.showToast.mock.calls
  return calls.length ? calls[calls.length - 1][0].title : ''
}

describe('employee/stat 页', () => {
  it('无令牌：直接跳登录页，不发请求（未登录没有数据可看）', async () => {
    const p = await page()

    await p.onShow()

    expect(wxStub.redirectTo).toHaveBeenCalledWith({ url: '/pages/employee/login/login' })
    expect(mocks.request).not.toHaveBeenCalled()
  })

  // onShow 而非 onLoad：从后台切回来要刷新数字，挂在 onLoad 上只会显示进页面那一次
  it('有令牌：onShow 拉取统计数据并落位', async () => {
    storage.set('empToken', 'tk-1')
    mocks.request.mockResolvedValueOnce({ code: 0, data: { visit: 12, call: 3, save: 5 } })
    const p = await page()

    await p.onShow()
    await flush()

    expect(mocks.request).toHaveBeenCalledWith('GET', '/api/employee/me/stat')
    expect(p.data.stat).toEqual({ visit: 12, call: 3, save: 5 })
    expect(p.data.loading).toBe(false)
  })

  // 令牌过期时 request 封装已清 storage，这里必须把人送回登录页，
  // 否则页面停在 loading 空白，用户以为「没数据」
  it('接口返回 401：跳登录页', async () => {
    storage.set('empToken', 'tk-1')
    mocks.request.mockResolvedValueOnce({ code: 401, msg: '登录失效' })
    const p = await page()

    await p.onShow()
    await flush()

    expect(wxStub.redirectTo).toHaveBeenCalledWith({ url: '/pages/employee/login/login' })
    expect(p.data.stat).toBeNull()
  })

  it('其它错误：提示后端 msg，loading 必须落下', async () => {
    storage.set('empToken', 'tk-1')
    mocks.request.mockResolvedValueOnce({ code: 1, msg: '统计服务不可用' })
    const p = await page()

    await p.onShow()
    await flush()

    expect(lastToast()).toBe('统计服务不可用')
    expect(p.data.loading).toBe(false)
  })

  it('错误且无 msg：回落「加载失败」', async () => {
    storage.set('empToken', 'tk-1')
    mocks.request.mockResolvedValueOnce({ code: 500 })
    const p = await page()

    await p.onShow()
    await flush()

    expect(lastToast()).toBe('加载失败')
  })

  it('加载中先置 loading=true（数字区显示骨架而不是 0）', async () => {
    storage.set('empToken', 'tk-1')
    let release: any
    mocks.request.mockImplementationOnce(() => new Promise((r) => (release = r)))
    const p = await page()

    p.onShow() // onShow 内部才 async，这里同步返回，靠后续 flush 推进
    expect(p.data.loading).toBe(true)

    release({ code: 0, data: { visit: 1 } })
    await flush()
    expect(p.data.loading).toBe(false)
  })
})
