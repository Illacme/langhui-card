import { computed, getCurrentInstance, onBeforeUnmount, ref } from 'vue'

/**
 * 响应式断点：模块级单例，所有组件共享同一 width 源，全程只挂一个 resize 监听。
 *
 * 断点定义（与 responsive.css 保持一致）：
 * - desktop 桌面：>= 1200px（当前主战场，保持原样）
 * - tablet  平板：768 - 1199px（侧栏抽屉、表格横滚、弹窗限宽）
 * - phone   手机：< 768px（侧栏抽屉、弹窗全屏、表格横滚、仅保证不破版）
 */
export type DeviceType = 'desktop' | 'tablet' | 'phone'

export const TABLET_MAX_WIDTH = 1199
export const PHONE_MAX_WIDTH = 767

/** 纯函数：把视口宽度映射到设备档位（导出便于单测，避免依赖 window）。 */
export function deviceOf(width: number): DeviceType {
  if (width <= PHONE_MAX_WIDTH) return 'phone'
  if (width <= TABLET_MAX_WIDTH) return 'tablet'
  return 'desktop'
}

const getInitialWidth = (): number =>
  typeof window !== 'undefined' && window.innerWidth ? window.innerWidth : 1280

const width = ref<number>(getInitialWidth())
let refCount = 0
let cleanup: (() => void) | null = null

function attach() {
  if (typeof window === 'undefined' || cleanup) return
  const onResize = () => {
    width.value = window.innerWidth
  }
  window.addEventListener('resize', onResize)
  cleanup = () => {
    window.removeEventListener('resize', onResize)
    cleanup = null
  }
}

function detach() {
  refCount--
  if (refCount <= 0 && cleanup) {
    cleanup()
    cleanup = null
    refCount = 0
  }
}

/**
 * 返回响应式断点信息。组件卸载时自动释放监听（无活跃组件实例时跳过，
 * 便于在测试里直接调用纯逻辑）。
 */
export function useBreakpoint() {
  attach()
  refCount++
  if (getCurrentInstance()) onBeforeUnmount(detach)

  const device = computed(() => deviceOf(width.value))
  return {
    width,
    device,
    isDesktop: computed(() => device.value === 'desktop'),
    isTabletDown: computed(() => device.value !== 'desktop'),
    isPhone: computed(() => device.value === 'phone'),
  }
}
