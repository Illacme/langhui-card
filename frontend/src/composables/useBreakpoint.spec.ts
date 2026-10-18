import { describe, it, expect } from 'vitest'
import { deviceOf, TABLET_MAX_WIDTH, PHONE_MAX_WIDTH } from './useBreakpoint'

describe('deviceOf 断点映射', () => {
  it('手机：<= 767', () => {
    expect(deviceOf(320)).toBe('phone')
    expect(deviceOf(375)).toBe('phone')
    expect(deviceOf(PHONE_MAX_WIDTH)).toBe('phone')
  })

  it('平板：768 - 1199（含上下界）', () => {
    expect(deviceOf(PHONE_MAX_WIDTH + 1)).toBe('tablet')
    expect(deviceOf(768)).toBe('tablet')
    expect(deviceOf(900)).toBe('tablet')
    expect(deviceOf(TABLET_MAX_WIDTH)).toBe('tablet')
  })

  it('桌面：>= 1200', () => {
    expect(deviceOf(TABLET_MAX_WIDTH + 1)).toBe('desktop')
    expect(deviceOf(1280)).toBe('desktop')
    expect(deviceOf(1920)).toBe('desktop')
  })
})
