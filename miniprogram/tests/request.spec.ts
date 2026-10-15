// 小程序端请求封装纯逻辑测试（401 静默刷新重试 + 单飞锁）。
// 说明：
//  1. request.ts 在模块顶层调用 getApp()、运行时依赖 wx.* 全局，因此每个用例都先
//     vi.stubGlobal 打桩，再 resetModules + 动态 import，保证拿到「带桩环境的新模块实例」
//     （refreshing 单飞锁状态也随之重置）。
//  2. 刷新链跨多轮微任务（401 → 刷新 → 重试），用「定时器脚本应答」容易因设置时机
//     错拍而丢应答。故 wx.request 桩改为「同步挂起 + respond() 显式应答」：
//     调用发生即按 index 挂起，测试在合适的链路节点用 respond(index, data) 精确投递，
//     之后用 tick()（等一个 macrotask，附带排空微任务）推进到下一节点。
import { describe, it, expect, vi, beforeEach } from 'vitest'

interface WxRequestOpts {
  url: string
  method: string
  data?: any
  header: Record<string, string>
  success: (res: { data: any }) => void
  fail: (err: any) => void
}

// 等一个 macrotask：排空期间产生的微任务链（刷新 → 重试的推进全靠它）
const tick = () => new Promise((r) => setTimeout(r, 0))

// 安装 wx / getApp 桩。respond(i, v)：应答第 i 个 wx.request——
// 对象 → success({data:v})；'fail' → fail。调用未发生时会 recorded，发生时立即补投。
function installWx() {
  const store: Record<string, string> = {}
  const calls: WxRequestOpts[] = []
  const script = new Map<number, any | 'fail'>()
  const waiting = new Map<number, WxRequestOpts>()

  const deliver = (idx: number, opts: WxRequestOpts) => {
    const v = script.get(idx)
    if (v === undefined) {
      waiting.set(idx, opts) // 尚未预设应答：挂起等 respond
      return
    }
    waiting.delete(idx)
    if (v === 'fail') opts.fail(new Error('network'))
    else opts.success({ data: v })
  }

  vi.stubGlobal('getApp', () => ({ globalData: { baseUrl: 'http://test' } }))
  vi.stubGlobal('wx', {
    request: vi.fn((opts: WxRequestOpts) => {
      const idx = calls.length
      calls.push(opts)
      deliver(idx, opts)
    }),
    getStorageSync: (k: string) => (k in store ? store[k] : ''),
    setStorageSync: (k: string, v: string) => {
      store[k] = v
    },
    removeStorageSync: (k: string) => {
      delete store[k]
    },
  })

  const respond = (idx: number, v: any | 'fail') => {
    script.set(idx, v)
    const opts = waiting.get(idx)
    if (opts) deliver(idx, opts)
  }

  return { store, calls, respond }
}

async function loadRequest() {
  vi.resetModules()
  return import('../utils/request')
}

beforeEach(() => {
  vi.unstubAllGlobals()
})

describe('request 基础行为', () => {
  it('GET 的 data 拼成 query string，并剔除 undefined / 空串', async () => {
    const { calls } = installWx()
    const { request } = await loadRequest()

    request('GET', '/api/card', { staffId: 1, keyword: '', extra: undefined })
    expect(calls).toHaveLength(1)
    expect(calls[0].url).toBe('http://test/api/card?staffId=1')
    expect(calls[0].method).toBe('GET')
    expect(calls[0].data).toBeUndefined()
  })

  it('带上 empToken 时注入 Bearer 头', async () => {
    const { store, calls } = installWx()
    store.empToken = 't-123'
    const { request } = await loadRequest()

    request('GET', '/api/employee/me')
    expect(calls[0].header.Authorization).toBe('Bearer t-123')
  })

  it('wx.request fail → Promise reject', async () => {
    const { respond } = installWx()
    const { request } = await loadRequest()

    const p = request('POST', '/api/stat/track', { a: 1 })
    respond(0, 'fail')
    await expect(p).rejects.toThrow('network')
  })
})

describe('401 静默刷新与重试', () => {
  it('401 → 刷新成功 → 用新 token 重试原请求一次', async () => {
    const { store, calls, respond } = installWx()
    store.empToken = 'old'
    store.empRefreshToken = 'r-1'
    const { request } = await loadRequest()

    const p = request('GET', '/api/employee/me')
    respond(0, { code: 401, msg: '登录失效' }) // 401 → 同步触发刷新
    expect(calls).toHaveLength(2)
    expect(calls[1].url).toBe('http://test/api/refresh')

    respond(1, { code: 0, token: 'new' }) // 刷新成功 → 微任务链推进到重试
    await tick()
    expect(calls).toHaveLength(3)
    expect(calls[2].header.Authorization).toBe('Bearer new')

    respond(2, { code: 0, data: { name: 'x' } })
    const r = await p

    expect(store.empToken).toBe('new') // 新 access 已落库
    expect(r.code).toBe(0)
  })

  it('无 refreshToken 时不发起刷新、直接清理并返回 401 结果', async () => {
    const { store, calls, respond } = installWx()
    store.empToken = 'old'
    const { request } = await loadRequest()

    const p = request('GET', '/api/employee/me')
    respond(0, { code: 401 })
    const r = await p

    expect(calls).toHaveLength(1) // 没有发起刷新
    expect(store.empToken).toBeUndefined() // 已清理
    expect(r.code).toBe(401)
  })

  it('刷新失败（网络错误）→ 清理登录态并返回原 401 结果', async () => {
    const { store, calls, respond } = installWx()
    store.empToken = 'old'
    store.empRefreshToken = 'r-1'
    const { request } = await loadRequest()

    const p = request('GET', '/api/employee/me')
    respond(0, { code: 401 })
    respond(1, 'fail')
    const r = await p

    expect(calls).toHaveLength(2)
    expect(store.empToken).toBeUndefined()
    expect(store.empRefreshToken).toBeUndefined()
    expect(r.code).toBe(401)
  })

  it('刷新返回非 0 → 同样清理', async () => {
    const { store, respond } = installWx()
    store.empToken = 'old'
    store.empRefreshToken = 'r-1'
    const { request } = await loadRequest()

    const p = request('GET', '/api/employee/me')
    respond(0, { code: 401 })
    respond(1, { code: 401 })
    const r = await p

    expect(store.empToken).toBeUndefined()
    expect(r.code).toBe(401)
  })

  it('重试后仍 401 → 彻底清理并返回 401', async () => {
    const { store, calls, respond } = installWx()
    store.empToken = 'old'
    store.empRefreshToken = 'r-1'
    const { request } = await loadRequest()

    const p = request('GET', '/api/employee/me')
    respond(0, { code: 401 })
    respond(1, { code: 0, token: 'new2' })
    await tick() // 重试发起
    expect(calls).toHaveLength(3)
    respond(2, { code: 401 })
    const r = await p

    expect(store.empToken).toBeUndefined() // retried 后仍失败 → 清理
    expect(r.code).toBe(401)
  })

  it('并发 401 只触发一次刷新（单飞锁），两个请求都用新 token 重试', async () => {
    const { store, calls, respond } = installWx()
    store.empToken = 'old'
    store.empRefreshToken = 'r-1'
    const { request } = await loadRequest()

    const p1 = request('GET', '/api/employee/me')
    const p2 = request('GET', '/api/employee/me/stat')
    respond(0, { code: 401 })
    respond(1, { code: 401 }) // 两个 401 都到达；只发起一次刷新
    expect(calls).toHaveLength(3)
    expect(calls.filter((c) => c.url === 'http://test/api/refresh')).toHaveLength(1)

    respond(2, { code: 0, token: 'shared' })
    await tick() // 两个请求各自重试
    expect(calls).toHaveLength(5)
    expect(calls[3].header.Authorization).toBe('Bearer shared')
    expect(calls[4].header.Authorization).toBe('Bearer shared')

    respond(3, { code: 0, data: 1 })
    respond(4, { code: 0, data: 2 })
    const [r1, r2] = await Promise.all([p1, p2])

    expect(r1.code).toBe(0)
    expect(r2.code).toBe(0)
    expect(store.empToken).toBe('shared')
  })
})
