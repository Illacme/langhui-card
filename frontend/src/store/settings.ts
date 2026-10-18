import { defineStore } from 'pinia'
import { getPublicSettings } from '@/api/settings'

// 系统设置的「公开子集」缓存：品牌名等展示位从后端取，避免把品牌名写死在前端。
//
// 为什么缓存到 localStorage：每次进入后台都等接口返回会让侧边栏品牌名先闪一下占位。
// 这里用「上次已知值先渲染 + 后台静默刷新」的策略，既即时又最终一致；
// 接口不可用（未登录 / 后端未升级）时回落到内置默认「智能名片」，不阻塞界面。
export const useSettingsStore = defineStore('settings', {
  state: () => ({
    brandName: localStorage.getItem('sys_brand_name') || '',
    loaded: false,
  }),
  getters: {
    // 品牌名：未配置 / 接口不可用回落内置默认
    displayBrandName: (s): string => s.brandName || '智能名片',
  },
  actions: {
    // 拉取公开设置并刷新本地缓存（force=true 用于「系统设置」保存后立刻同步品牌名）
    async load(force = false) {
      if (this.loaded && !force) return
      this.loaded = true
      try {
        const r = await getPublicSettings()
        if (r.code === 0 && r.data) {
          this.brandName = r.data.brand_name || ''
          localStorage.setItem('sys_brand_name', this.brandName)
        }
      } catch {
        // 网络异常 / 后端未升级：保留本地缓存或默认值，不阻塞界面
      }
    },
    // 「系统设置」页保存品牌名后即时更新侧边栏，无需刷新页面
    setBrandName(v: string) {
      this.brandName = v
      localStorage.setItem('sys_brand_name', v)
    },
  },
})
