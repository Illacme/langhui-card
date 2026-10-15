import { describe, it, expect, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useAuthStore } from '@/store/auth'

// 超管登录态：state 来自 localStorage，动作同步写回 localStorage。
// jsdom 提供 localStorage，每个用例前清空，保证隔离。
describe('useAuthStore 登录态', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    localStorage.clear()
  })

  it('初始态：无 token / 未登录', () => {
    const s = useAuthStore()
    expect(s.token).toBe('')
    expect(s.username).toBe('')
    expect(s.isLoggedIn).toBe(false)
  })

  it('setAuth：state 与 localStorage 同步写入', () => {
    const s = useAuthStore()
    s.setAuth('tok-123', 'admin')
    expect(s.token).toBe('tok-123')
    expect(s.isLoggedIn).toBe(true)
    expect(localStorage.getItem('token')).toBe('tok-123')
    expect(localStorage.getItem('admin_user')).toBe('admin')
  })

  it('logout：state 与 localStorage 同步清空', () => {
    const s = useAuthStore()
    s.setAuth('tok-123', 'admin')
    s.logout()
    expect(s.token).toBe('')
    expect(s.isLoggedIn).toBe(false)
    expect(localStorage.getItem('token')).toBeNull()
    expect(localStorage.getItem('admin_user')).toBeNull()
  })
})
