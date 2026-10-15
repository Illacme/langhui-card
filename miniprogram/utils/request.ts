// 统一的 API 请求封装：自动注入员工 token、统一错误码处理。
// 401 时先用 refresh 令牌静默换发新 access 并重试原请求一次（单飞锁），
// 无 refreshToken 或刷新失败则清 token 由调用方引导重新登录。
const app = getApp() as any

function baseUrl(): string {
  return (app && app.globalData && app.globalData.baseUrl) || 'http://127.0.0.1:8080'
}

// apiBaseUrl 暴露给文件上传等需要直接拼 URL 的场景（如 wx.uploadFile）
export function apiBaseUrl(): string {
  return baseUrl()
}

export interface ApiResult<T> {
  code: number
  msg?: string
  data?: T
  // 登录接口在根级返回 token / refreshToken（{ code:0, token, refreshToken }）
  token?: string
  refreshToken?: string
}

// 单飞锁：并发 401 只触发一次刷新，其余复用同一次结果
let refreshing: Promise<string | null> | null = null

// 裸请求（不经过本封装，避免递归）：用 refresh 令牌换新的 access
function rawRefresh(refreshToken: string): Promise<ApiResult<any>> {
  return new Promise((resolve, reject) => {
    wx.request({
      url: baseUrl() + '/api/refresh',
      method: 'POST',
      data: { refreshToken },
      header: { 'content-type': 'application/json' },
      success: (res) => resolve(res.data as ApiResult<any>),
      fail: (err) => reject(err),
    })
  })
}

async function tryRefresh(): Promise<string | null> {
  const refreshToken = wx.getStorageSync('empRefreshToken')
  if (!refreshToken) return null
  try {
    const r = await rawRefresh(refreshToken)
    if (r && r.code === 0 && r.token) {
      wx.setStorageSync('empToken', r.token)
      return r.token
    }
  } catch {
    // 刷新失败：按失效处理
  }
  return null
}

function clearAuth() {
  wx.removeStorageSync('empToken')
  wx.removeStorageSync('empRefreshToken')
}

// method 支持 GET/POST/PUT；GET 的 data 会拼成 query string
export function request<T = any>(method: 'GET' | 'POST' | 'PUT', path: string, data?: any): Promise<ApiResult<T>> {
  return doRequest<T>(method, path, data, wx.getStorageSync('empToken'), false)
}

function doRequest<T>(
  method: 'GET' | 'POST' | 'PUT',
  path: string,
  data: any,
  token: string,
  retried: boolean
): Promise<ApiResult<T>> {
  return new Promise((resolve, reject) => {
    let url = baseUrl() + path
    let body: any = data
    if (method === 'GET' && data) {
      const qs = Object.keys(data)
        .filter((k) => data[k] !== undefined && data[k] !== '')
        .map((k) => `${encodeURIComponent(k)}=${encodeURIComponent(data[k])}`)
        .join('&')
      if (qs) url += (url.indexOf('?') >= 0 ? '&' : '?') + qs
      body = undefined
    }
    wx.request({
      url,
      method,
      data: body,
      header: {
        'content-type': 'application/json',
        ...(token ? { Authorization: 'Bearer ' + token } : {}),
      },
      success: (res) => {
        const result = res.data as ApiResult<T>
        if (result && result.code === 401 && !retried) {
          // 先用 refresh 令牌换新 access，再重试一次
          if (!refreshing) {
            refreshing = tryRefresh().finally(() => {
              refreshing = null
            })
          }
          refreshing.then((newToken) => {
            if (newToken) {
              doRequest<T>(method, path, data, newToken, true).then(resolve, reject)
            } else {
              clearAuth()
              resolve(result)
            }
          })
          return
        }
        if (result && result.code === 401) {
          // 已重试仍 401：彻底清理
          clearAuth()
        }
        resolve(result)
      },
      fail: (err) => reject(err),
    })
  })
}
