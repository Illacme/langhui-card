// 统一的 API 请求封装：自动注入员工 token、统一错误码处理
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
  // 登录接口在根级返回 token（{ code:0, token }），与 data 下的资源区分开
  token?: string
}

// method 支持 GET/POST/PUT；GET 的 data 会拼成 query string
export function request<T = any>(method: 'GET' | 'POST' | 'PUT', path: string, data?: any): Promise<ApiResult<T>> {
  return new Promise((resolve, reject) => {
    const token = wx.getStorageSync('empToken')
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
        const body = res.data as ApiResult<T>
        if (body && body.code === 401) {
          // 登录失效：清 token（员工端）
          wx.removeStorageSync('empToken')
        }
        resolve(body)
      },
      fail: (err) => reject(err),
    })
  })
}
