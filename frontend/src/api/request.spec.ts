import { describe, it, expect, vi, beforeEach } from 'vitest'

// 统一响应契约见 request.ts 注释：
//   成功 code=0；业务失败 code=1；特殊状态 code=2；鉴权失败 HTTP 200 + code 401/403。
// 拦截器以 res.code 为准。这里直接驱动已注册的拦截器函数，不真正发请求。
//
// 需要 mock 两个重依赖：
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

// 取出已注册的拦截器函数（axios 把 fulfilled/rejected 存在 handlers 数组）
function respFulfilled() {
  return (http.interceptors.response as any).handlers[0].fulfilled
}
function reqFulfilled() {
  return (http.interceptors.request as any).handlers[0].fulfilled
}

describe('request 响应拦截：鉴权契约', () => {
  beforeEach(() => {
    localStorage.clear()
    routerReplace.mockClear()
  })

  it('code=0 成功：回传整包（交由各 api 函数取字段）', async () => {
    const data = await respFulfilled()({ data: { code: 0, msg: 'ok', data: { a: 1 } } })
    expect(data).toEqual({ code: 0, msg: 'ok', data: { a: 1 } })
  })

  it('code=401：清 token、提示、跳登录并 reject', async () => {
    localStorage.setItem('token', 'old')
    localStorage.setItem('admin_user', 'admin')
    const p = respFulfilled()({ data: { code: 401, msg: '登录失效，请重新登录' } })
    await expect(p).rejects.toThrow('登录失效，请重新登录')
    expect(localStorage.getItem('token')).toBeNull()
    expect(localStorage.getItem('admin_user')).toBeNull()
    expect(elMessage.error).toHaveBeenCalledWith('登录失效，请重新登录')
    expect(routerReplace).toHaveBeenCalledWith('/login')
  })

  it('code=403：警告并 reject', async () => {
    const p = respFulfilled()({ data: { code: 403, msg: '无权限' } })
    await expect(p).rejects.toThrow('无权限')
    expect(elMessage.warning).toHaveBeenCalledWith('无权限')
  })

  it('code!=0（业务失败）：报错并 reject', async () => {
    const p = respFulfilled()({ data: { code: 1, msg: '参数错误' } })
    await expect(p).rejects.toThrow('参数错误')
    expect(elMessage.error).toHaveBeenCalledWith('参数错误')
  })
})

describe('request 请求拦截：注入鉴权头', () => {
  beforeEach(() => localStorage.clear())

  it('有 token 时注入 Bearer', () => {
    localStorage.setItem('token', 'abc')
    const cfg: any = reqFulfilled()({ headers: {} })
    expect(cfg.headers.Authorization).toBe('Bearer abc')
  })

  it('无 token 时不注入', () => {
    const cfg: any = reqFulfilled()({ headers: {} })
    expect(cfg.headers.Authorization).toBeUndefined()
  })
})
