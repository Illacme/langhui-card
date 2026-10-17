// 小程序端多租户解析纯逻辑测试（utils/tenant.ts）。
// 覆盖两种部署形态：
//  1. 平台模式（pinnedCompanyId=0）：扫码 scene / 分享 query 绑定租户并粘滞，
//     无来源时不传（走后端缺省朗汇）；
//  2. 企业独立版（pinnedCompanyId>0）：恒返回锁定值，且拒绝一切外部绑定
//     （scene / query / Storage 残留都不能改写）。
import { describe, it, expect, beforeEach, vi } from 'vitest'

// 内存版 wx.Storage：tenant 模块只在调用时触达 wx.*，装桩后再动态 import
function installWx() {
  const store: Record<string, string> = {}
  vi.stubGlobal('wx', {
    getStorageSync: (k: string) => (k in store ? store[k] : ''),
    setStorageSync: (k: string, v: string) => {
      store[k] = v
    },
    removeStorageSync: (k: string) => {
      delete store[k]
    },
  })
  return store
}

async function loadTenant() {
  vi.resetModules()
  return import('../utils/tenant')
}

beforeEach(() => {
  installWx()
})

describe('平台模式（未配置 companyId）', () => {
  it('无任何来源时返回 0（不传，后端缺省落朗汇）', async () => {
    const t = await loadTenant()
    expect(t.getCompanyId()).toBe(0)
  })

  it('scene 携带 companyId 时绑定并粘滞', async () => {
    const t = await loadTenant()
    t.bindCompanyFromScene('companyId=3&staffId=9')
    expect(t.getCompanyId()).toBe(3)
    // 粘滞：再次无参进入仍落该企业
    t.bindCompanyFromScene(undefined)
    expect(t.getCompanyId()).toBe(3)
  })

  it('scene 整体 URL-encoded 也能解析', async () => {
    const t = await loadTenant()
    t.bindCompanyFromScene('companyId%3D4%26staffId%3D1')
    expect(t.getCompanyId()).toBe(4)
  })

  it('页面 query 的 companyId 同样生效', async () => {
    const t = await loadTenant()
    t.bindCompanyFromScene(undefined, 6)
    expect(t.getCompanyId()).toBe(6)
  })

  it('scene 无 companyId（只有 staffId）时不改写已有绑定', async () => {
    const t = await loadTenant()
    t.bindCompanyFromScene('companyId=5')
    t.bindCompanyFromScene('staffId=9')
    expect(t.getCompanyId()).toBe(5)
  })

  it('非法 / 零值 companyId 返回 0', async () => {
    const t = await loadTenant()
    expect(t.parseCompanyId('')).toBe(0)
    expect(t.parseCompanyId(undefined)).toBe(0)
    expect(t.parseCompanyId('companyId=0')).toBe(0)
    expect(t.parseCompanyId('companyId=abc')).toBe(0)
    expect(t.parseCompanyId('staffId=9')).toBe(0)
  })

  it('resetCompanyBinding 清掉粘滞，恢复缺省行为', async () => {
    const t = await loadTenant()
    t.bindCompanyFromScene('companyId=2')
    expect(t.getCompanyId()).toBe(2)
    t.resetCompanyBinding()
    expect(t.getCompanyId()).toBe(0)
  })
})

describe('企业独立版（配置了 companyId）', () => {
  it('恒返回锁定企业，不读 Storage 残留', async () => {
    const t = await loadTenant()
    t.tenantConfig.pinnedCompanyId = 7
    // 先造一个平台模式的残留绑定，独立版必须无视它
    t.tenantConfig.pinnedCompanyId = 0
    t.bindCompanyFromScene('companyId=3')
    t.tenantConfig.pinnedCompanyId = 7
    expect(t.isPinned()).toBe(true)
    expect(t.getCompanyId()).toBe(7)
  })

  it('拒绝 scene / query 绑定（无切换多租户行为）', async () => {
    const t = await loadTenant()
    t.tenantConfig.pinnedCompanyId = 7
    t.bindCompanyFromScene('companyId=3&staffId=9')
    t.bindCompanyFromScene(undefined, 8)
    expect(t.getCompanyId()).toBe(7)
  })
})
