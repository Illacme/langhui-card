import { describe, it, expect, beforeEach } from 'vitest'
import {
  DEFAULT_BASE_URL,
  baseUrlOf,
  pinnedCompanyOf,
  cachedSettings,
  cacheSettings,
} from '../utils/config'

// 小程序端「后台下发配置」的纯逻辑测试：
//   - 基址 / 锁定租户的归一化（去斜杠、容错非法值）
//   - Storage 缓存读写（冷启动首帧同步生效的关键）
// config.ts 只在函数内部使用 wx，模块顶层无副作用，故可直接导入；
// 这里按需给全局 wx 打桩即可覆盖缓存路径。

function stubWx(store: Record<string, any> = {}) {
  ;(globalThis as any).wx = {
    getStorageSync: (k: string) => store[k] ?? '',
    setStorageSync: (k: string, v: any) => {
      store[k] = v
    },
  }
}

describe('config', () => {
  beforeEach(() => {
    stubWx()
  })

  it('内置引导种子地址可用（换部署环境改这一处）', () => {
    expect(DEFAULT_BASE_URL).toBe('http://127.0.0.1:8080')
  })

  describe('baseUrlOf', () => {
    it('未拿到配置时返回空串（由调用方保持原值，不误覆盖）', () => {
      expect(baseUrlOf(null)).toBe('')
      expect(baseUrlOf({})).toBe('')
      expect(baseUrlOf({ miniprogram_base_url: '   ' })).toBe('')
    })

    it('去掉末尾斜杠与首尾空白，避免拼出双斜杠', () => {
      expect(baseUrlOf({ miniprogram_base_url: 'https://api.example.com///' })).toBe(
        'https://api.example.com'
      )
      expect(baseUrlOf({ miniprogram_base_url: '  https://api.example.com  ' })).toBe(
        'https://api.example.com'
      )
    })
  })

  describe('pinnedCompanyOf', () => {
    it('未配置 / 非法值一律回落 0（平台模式）', () => {
      expect(pinnedCompanyOf(null)).toBe(0)
      expect(pinnedCompanyOf({})).toBe(0)
      expect(pinnedCompanyOf({ pinned_company_id: 'abc' })).toBe(0)
      expect(pinnedCompanyOf({ pinned_company_id: '-1' })).toBe(0)
      expect(pinnedCompanyOf({ pinned_company_id: '0' })).toBe(0)
    })

    it('正整数解析为锁定租户', () => {
      expect(pinnedCompanyOf({ pinned_company_id: '7' })).toBe(7)
    })
  })

  describe('Storage 缓存', () => {
    it('空缓存 / 非法 JSON 返回 null，不抛异常', () => {
      expect(cachedSettings()).toBeNull()
      stubWx({ publicSettingsCache: '{ not json' })
      expect(cachedSettings()).toBeNull()
    })

    it('写入后可原样读回（下次冷启动首帧即用上次值）', () => {
      cacheSettings({ miniprogram_base_url: 'https://api.example.com', pinned_company_id: '5' })
      const got = cachedSettings()
      expect(got?.miniprogram_base_url).toBe('https://api.example.com')
      expect(pinnedCompanyOf(got)).toBe(5)
    })
  })
})
