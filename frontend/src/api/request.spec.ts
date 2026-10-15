import { describe, it, expect, vi, beforeEach } from 'vitest'

// 统一响应契约见 request.ts 注释：
//   成功 code=0；业务失败 code=1；特殊状态 code=2；鉴权失败 HTTP 200 + code 401/403。
// 拦截器以 res.code 为准。PC 后台为 httpOnly Cookie 鉴权：令牌由浏览器携带，JS 不接触，
// 故无 Bearer 请求拦截；401 时用 refresh Cookie 静默换发并重试一次。
// 这里直接驱动已注册的拦截器函数，并用 vi.spyOn 控制刷新/重试的内部请求，不真正发网络请求。
//
// mock 两个重依赖：
//   - element-plus 的 ElMessage（避免真实 toast）
//   - @/router 的 default 导出（避免加载 vue-router 全链路 + 真实跳转）
const { elMessage, routerReplace } = vi.hoisted(() => ({
  elMessage: { error: vi.fn(), warning: vi.fn(), success: vi.fn(), info: vi.fn() },
  routerReplace: vi.fn(),
}))

vi.mock('element-plus', () => ({ ElMessage: elMessage }))
vi.mock('@/router', () => ({
  default: {
    currentRoute: { value: { path: '/apply/open' } },
    replace: routerReplace,
  },
}))

import http from '@/api/request'

// 取出已注册的响应拦截 fulfilled（axios 把 fulfilled/rejected 存在 handlers 数组）
function respFulfilled() {
  return (http.interceptors.response as any).handlers[0].fulfilled
}

describe('request 响应拦截：鉴权契约（httpOnly Cookie 流）', () => {
  beforeEach(() => {
    localStorage.clear()
    routerReplace.mockClear()
    elMessage.error.mockClear()
    elMessage.warning.mockClear()
    vi.restoreAllMocks()
  })

  it('code=0 成功：回传整包（交由各 api 函数取字段）', async () => {
    const data = await respFulfilled()({ data: { code: 0, msg: 'ok', data: { a: 1 } }, config: {} })
    expect(data).toEqual({ code: 0, msg: 'ok', data: { a: 1 } })
  })

  it('code=401 且刷新失败：清登录态、跳登录并 reject', async () => {
    localStorage.setItem('admin_user', 'admin')
    localStorage.setItem('admin_loggedin', '1')
    // /api/refresh 返回非 0 → 刷新失败
    vi.spyOn(http, 'post').mockResolvedValue({ code: 1 } as any)
    const p = respFulfilled()({ data: { code: 401, msg: '登录失效，请重新登录' }, config: {} })
    await expect(p).rejects.toThrow('登录失效，请重新登录')
    expect(localStorage.getItem('admin_user')).toBeNull()
    expect(localStorage.getItem('admin_loggedin')).toBeNull()
    expect(elMessage.error).toHaveBeenCalledWith('登录失效，请重新登录')
    expect(routerReplace).toHaveBeenCalledWith('/login')
  })

  it('code=401 且刷新成功：静默重试原请求并返回其结果，不跳登录', async () => {
    // 刷新成功：服务端轮换下发新 Cookie，返回 code:0
    vi.spyOn(http, 'post').mockResolvedValue({ code: 0 } as any)
    // 重试原请求成功
    vi.spyOn(http, 'request').mockResolvedValue({ code: 0, data: { ok: true } } as any)
    const cfg = { url: '/api/admin/employees', method: 'GET' }
    const result = await respFulfilled()({ data: { code: 401, msg: 'x' }, config: cfg })
    expect(result).toEqual({ code: 0, data: { ok: true } })
    expect(http.post).toHaveBeenCalledWith('/refresh', {}, expect.objectContaining({ __skipRefresh: true }))
    expect(routerReplace).not.toHaveBeenCalled()
  })

  it('code=401 刷新请求自身（__skipRefresh）不再递归刷新', async () => {
    // 刷新端点自身返回 401 时，应直接走清登录态，不再调用 http.post 去刷新
    const postSpy = vi.spyOn(http, 'post')
    localStorage.setItem('admin_user', 'admin')
    const p = respFulfilled()({
      data: { code: 401, msg: '刷新令牌无效' },
      config: { __skipRefresh: true },
    })
    await expect(p).rejects.toThrow('刷新令牌无效')
    expect(postSpy).not.toHaveBeenCalled()
    expect(localStorage.getItem('admin_user')).toBeNull()
    expect(routerReplace).toHaveBeenCalledWith('/login')
  })

  it('code=403：警告并 reject', async () => {
    const p = respFulfilled()({ data: { code: 403, msg: '无权限' }, config: {} })
    await expect(p).rejects.toThrow('无权限')
    expect(elMessage.warning).toHaveBeenCalledWith('无权限')
  })

  it('code!=0（业务失败）：报错并 reject', async () => {
    const p = respFulfilled()({ data: { code: 1, msg: '参数错误' }, config: {} })
    await expect(p).rejects.toThrow('参数错误')
    expect(elMessage.error).toHaveBeenCalledWith('参数错误')
  })
})
