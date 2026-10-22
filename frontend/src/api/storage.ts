import http from './request'

// CDN 刷新（覆盖写使边缘旧缓存失效），需后端启用刷新并配好厂商与该厂商要求的凭据
// （界面设置或环境变量，五家之一）；未配置时接口返回 code:1 由拦截器提示
export function cdnRefresh(urls: string[]) {
  return http.post('/admin/cdn/refresh', { urls })
}

// 私有桶：把已存的公网 URL 列表换成限时签名 URL（key 由 BaseURL 前缀反推）
export function signUrls(urls: string[]) {
  return http.post('/admin/storage/sign', { urls }).then((r: any) => (r.data?.signed ?? {}) as Record<string, string>)
}
