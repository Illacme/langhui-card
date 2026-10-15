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
})
