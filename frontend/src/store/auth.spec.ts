import { describe, it, expect, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useAuthStore } from '@/store/auth'

// PC 后台改用 httpOnly Cookie 鉴权后，store 只保存非敏感信息：用户名 + 登录标记。
// 令牌由浏览器持有、JS 不可读，故不再出现在 store / localStorage。
describe('useAuthStore 登录态', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    localStorage.clear()
  })

  it('初始态：未登录', () => {
    const s = useAuthStore()
    expect(s.username).toBe('')
    expect(s.loggedIn).toBe(false)
    expect(s.isLoggedIn).toBe(false)
  })

  it('setAuth：写入非敏感登录态，且不存令牌', () => {
    const s = useAuthStore()
    s.setAuth('admin')
    expect(s.username).toBe('admin')
    expect(s.loggedIn).toBe(true)
    expect(s.isLoggedIn).toBe(true)
    expect(localStorage.getItem('admin_user')).toBe('admin')
    expect(localStorage.getItem('admin_loggedin')).toBe('1')
    // 令牌不在 JS 可读存储中（httpOnly Cookie 由浏览器管理）
    expect(localStorage.getItem('token')).toBeNull()
    expect(localStorage.getItem('refreshToken')).toBeNull()
  })

  it('logout：清空登录态', () => {
    const s = useAuthStore()
    s.setAuth('admin')
    s.logout()
    expect(s.username).toBe('')
    expect(s.loggedIn).toBe(false)
    expect(s.isLoggedIn).toBe(false)
    expect(localStorage.getItem('admin_user')).toBeNull()
    expect(localStorage.getItem('admin_loggedin')).toBeNull()
  })

  it('setAuth：记录企业简称（侧边栏品牌名）并持久化', () => {
    const s = useAuthStore()
    s.setAuth('comp2', {
      companyId: 2,
      companyName: '测试乙公司',
      companyShortName: '乙司',
      isPlatform: false,
    })
    expect(s.companyId).toBe(2)
    expect(s.companyShortName).toBe('乙司')
    expect(localStorage.getItem('admin_company_short_name')).toBe('乙司')
  })

  it('hydrate：以服务端身份覆盖本地缓存（含企业简称）', () => {
    const s = useAuthStore()
    s.setAuth('comp2', {
      companyId: 2,
      companyName: '旧公司名',
      companyShortName: '旧简称',
      isPlatform: false,
    })
    s.hydrate({
      adminId: 9,
      username: 'comp2',
      companyId: 2,
      companyName: '测试乙公司',
      companyShortName: '新简称',
      isPlatform: false,
    })
    expect(s.companyName).toBe('测试乙公司')
    expect(s.companyShortName).toBe('新简称')
    expect(localStorage.getItem('admin_company_short_name')).toBe('新简称')
  })

  it('logout：清空企业简称', () => {
    const s = useAuthStore()
    s.setAuth('comp2', {
      companyId: 2,
      companyName: '测试乙公司',
      companyShortName: '乙司',
      isPlatform: false,
    })
    s.logout()
    expect(s.companyShortName).toBe('')
    expect(localStorage.getItem('admin_company_short_name')).toBeNull()
  })

  it('logout：清空系统设置 / 配置向导的未保存草稿', () => {
    // 换人登录不该看到上一个人留在输入框里的半截值（可能含未保存的密钥）
    localStorage.setItem(
      'settings_wizard_draft',
      JSON.stringify({ v: 1, draft: { brand_name: '上个人的草稿' }, cleared: {} }),
    )
    const s = useAuthStore()
    s.setAuth('admin')
    s.logout()
    expect(localStorage.getItem('settings_wizard_draft')).toBeNull()
  })
})
