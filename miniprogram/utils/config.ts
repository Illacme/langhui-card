// 小程序端「可由后台下发」的运行时配置。
//
// 背景：接口基址与「企业独立版锁定租户」原本分别写死在 app.ts 与 tenant.ts，
// 改一次就得重新打包提审。现在收敛为后台「系统设置 → 小程序」维护，
// 冷启动拉一次 /api/public/settings（公开、无需登录）覆盖：
//   - miniprogram_base_url  接口基址
//   - pinned_company_id     企业独立版锁定租户（>0 锁定，0 = 平台模式）
//
// 内置 DEFAULT_BASE_URL 只是「还没拿到后台配置时」的引导种子：
// 种子需能连通一次，之后即可完全由后台控制（目标域名须在微信后台配为合法域名）。
// 拿到过的配置会缓存在 Storage，下次冷启动首帧同步生效，不必等网络。

const CACHE_KEY = 'publicSettingsCache'

// 引导种子地址：换部署环境改这一处即可（模拟器用 127.0.0.1，真机用电脑局域网 IP）。
export const DEFAULT_BASE_URL = 'http://127.0.0.1:8080'

// 后台「系统设置」公开下发的非敏感项（与后端 setting.publicKeys 一致）
export interface PublicSettings {
  brand_name?: string
  miniprogram_base_url?: string
  pinned_company_id?: string
}

// 取上次缓存的后台配置（同步）：冷启动首帧先用它，避免首屏请求打到旧地址。
export function cachedSettings(): PublicSettings | null {
  try {
    const raw = wx.getStorageSync(CACHE_KEY)
    if (!raw) return null
    return JSON.parse(raw) as PublicSettings
  } catch {
    return null
  }
}

// 缓存后台配置供下次冷启动同步使用（存储不可用时仅影响首帧，不阻塞启动）。
export function cacheSettings(data: PublicSettings): void {
  try {
    wx.setStorageSync(CACHE_KEY, JSON.stringify(data))
  } catch {
    // 忽略：隐私模式 / 空间不足等
  }
}

// 拉取后台公开配置。fromUrl 缺省用引导种子；失败静默返回 null，调用方保持当前值。
export function fetchPublicSettings(fromUrl?: string): Promise<PublicSettings | null> {
  return new Promise((resolve) => {
    wx.request({
      url: (fromUrl || DEFAULT_BASE_URL) + '/api/public/settings',
      method: 'GET',
      timeout: 5000,
      success: (res) => {
        const d = res.data as { code?: number; data?: PublicSettings } | null
        if (d && d.code === 0 && d.data) resolve(d.data)
        else resolve(null)
      },
      fail: () => resolve(null),
    })
  })
}

// 归一化的接口基址（去首尾空白与末尾斜杠）；未配置返回空串，由调用方保持原值。
export function baseUrlOf(data: PublicSettings | null): string {
  if (!data) return ''
  return String(data.miniprogram_base_url || '')
    .trim()
    .replace(/\/+$/, '')
}

// 独立版锁定租户；未配置 / 非法返回 0（平台模式）。
export function pinnedCompanyOf(data: PublicSettings | null): number {
  if (!data) return 0
  const n = parseInt(String(data.pinned_company_id ?? '0'), 10)
  return n > 0 ? n : 0
}
