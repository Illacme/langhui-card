import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent, nextTick } from 'vue'
import { createRouter, createMemoryHistory } from 'vue-router'

vi.mock('@/api/settings', () => ({ getSettings: vi.fn() }))
vi.mock('@/api/storage', () => ({ cdnRefresh: vi.fn() }))
vi.mock('element-plus', () => ({
  ElMessage: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() },
}))

import { ElMessage } from 'element-plus'
import { getSettings } from '@/api/settings'
import { cdnRefresh } from '@/api/storage'
import CdnRefresh from './CdnRefresh.vue'
import type { SettingItem } from '@/api/settings'

const ElCard = defineComponent({ name: 'ElCard', template: `<div class="el-card"><slot /></div>` })
// alert 拍平：标题与默认插槽（按钮）都进 DOM，便于断言「提示 + 去设置」一体
const ElAlert = defineComponent({
  name: 'ElAlert',
  props: ['title', 'type', 'closable'],
  template: `<div class="el-alert">{{ title }}<slot /></div>`,
})
const ElTag = defineComponent({
  name: 'ElTag',
  props: ['size', 'type', 'effect'],
  template: `<span class="el-tag"><slot /></span>`,
})
const ElInput = defineComponent({
  name: 'ElInput',
  props: ['modelValue', 'placeholder', 'type', 'rows'],
  emits: ['update:modelValue'],
  template: `<textarea class="el-textarea" :placeholder="placeholder" :value="modelValue" @input="$emit('update:modelValue', $event.target.value)"></textarea>`,
})
const ElButton = defineComponent({
  name: 'ElButton',
  props: ['type', 'loading', 'size'],
  template: `<button class="el-button" @click="$emit('click')"><slot /></button>`,
})

function item(key: string, over: Partial<SettingItem> = {}): SettingItem {
  return {
    key,
    name: key,
    group: 'storage',
    type: 'string',
    value: '',
    description: key,
    secret: false,
    restart: true,
    readOnly: false,
    envVar: '',
    configured: false,
    ...over,
  }
}

// 五家厂商的界面侧事实（谁要资源标识、谁不要 AK）由 utils/cdn 与后端同源维护，
// 这里用三家覆盖三种形态：aliyun（AK+SK）、cloudfront（AK+SK+资源标识）、cloudflare（仅 SK+资源标识）
const ON_ALIYUN = [
  item('cdn_enabled', { name: '启用 CDN 刷新', type: 'bool', value: 'true', envVar: 'CDN_ENABLED', configured: true }),
  item('cdn_provider', { name: 'CDN 厂商', value: 'aliyun', envVar: 'CDN_PROVIDER', configured: true }),
  item('cdn_access_key_id', { name: 'CDN AccessKeyId', envVar: 'CDN_ACCESS_KEY_ID', configured: true }),
  item('cdn_access_key_secret', { name: 'CDN AccessKeySecret', secret: true, envVar: 'CDN_ACCESS_KEY_SECRET', configured: true }),
  item('cdn_resource_id', { name: 'CDN 资源标识', envVar: 'CDN_RESOURCE_ID', configured: false }),
  item('cdn_endpoint', { name: 'CDN 服务端点', envVar: 'CDN_ENDPOINT', configured: false }),
]

async function setup(items: SettingItem[]) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', component: { template: '<div />' } },
      { path: '/platform/settings', component: { template: '<div>settings</div>' } },
    ],
  })
  await router.push('/')
  await router.isReady()
  vi.mocked(getSettings).mockResolvedValue({ code: 0, data: items } as never)
  const w = mount(CdnRefresh, {
    global: {
      plugins: [router],
      stubs: { 'el-card': ElCard, 'el-alert': ElAlert, 'el-tag': ElTag, 'el-input': ElInput, 'el-button': ElButton },
    },
  })
  await vi.waitFor(() => expect(getSettings).toHaveBeenCalled())
  await nextTick()
  return { w, router }
}

function submitButton(w: any) {
  return w.findAll('button.el-button').find((b: any) => b.text().includes('提交刷新'))!
}

describe('CdnRefresh 平台管理 · CDN 刷新', () => {
  beforeEach(() => {
    vi.mocked(getSettings).mockReset()
    vi.mocked(cdnRefresh).mockReset()
    vi.mocked(cdnRefresh).mockResolvedValue({ code: 0 } as never)
    vi.mocked(ElMessage.success).mockClear()
    vi.mocked(ElMessage.warning).mockClear()
  })

  it('配置齐全：只给通用说明，不挡提交，并显示当前厂商', async () => {
    const { w } = await setup(ON_ALIYUN)
    const text = w.text()
    expect(text).toContain('aliyun（阿里云）')
    // 厂商确实取到了（不是「没拉到设置」时的回落显示）
    expect(text).not.toContain('未设置，按默认')
    expect(text).not.toContain('未启用')
    expect(text).not.toContain('还缺')
    // 说明里点明支持五家 + 自动刷新触发点（不再是「填好阿里云凭据」的旧说法）
    expect(text).toContain('CloudFront')
    expect(text).toContain('自动刷新')
  })

  it('未启用：先点明缺口并给去设置入口，省一次无效提交', async () => {
    const items = ON_ALIYUN.map((it) =>
      it.key === 'cdn_enabled' ? { ...it, value: '', configured: false } : it,
    )
    const { w, router } = await setup(items)
    expect(w.text()).toContain('CDN 刷新当前未启用')
    await w.findAll('button.el-button').find((b: any) => b.text().includes('去设置'))!.trigger('click')
    await nextTick()
    // 路由跳转是异步的：给它一个微任务周期再断言
    await vi.waitFor(() => expect(router.currentRoute.value.path).toBe('/platform/settings'))
  })

  it('CloudFront 缺资源标识：点名缺的那一项', async () => {
    const items = ON_ALIYUN.map((it) => {
      if (it.key === 'cdn_provider') return { ...it, value: 'cloudfront' }
      if (it.key === 'cdn_resource_id') return { ...it, configured: false }
      return it
    })
    const { w } = await setup(items)
    expect(w.text()).toContain('cloudfront')
    expect(w.text()).toContain('还缺 CDN 资源标识')
    expect(w.text()).not.toContain('AccessKeyId')
  })

  it('Cloudflare：用不到 AccessKeyId，只提示缺 AccessKeySecret', async () => {
    const items = ON_ALIYUN.map((it) => {
      if (it.key === 'cdn_provider') return { ...it, value: 'cloudflare' }
      if (it.key === 'cdn_access_key_secret') return { ...it, configured: false }
      if (it.key === 'cdn_resource_id') return { ...it, configured: true }
      return it
    })
    const { w } = await setup(items)
    expect(w.text()).toContain('还缺 CDN AccessKeySecret')
    expect(w.text()).not.toContain('CDN AccessKeyId')
  })

  it('设置接口拿不到时不下判断：宁可不提示，也不误报未启用', async () => {
    vi.mocked(getSettings).mockRejectedValue(new Error('网络错误') as never)
    const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/', component: { template: '<div />' } }] })
    await router.push('/')
    await router.isReady()
    const w = mount(CdnRefresh, {
      global: {
        plugins: [router],
        stubs: { 'el-card': ElCard, 'el-alert': ElAlert, 'el-tag': ElTag, 'el-input': ElInput, 'el-button': ElButton },
      },
    })
    await nextTick()
    expect(w.text()).not.toContain('未启用')
    expect(submitButton(w).exists()).toBe(true)
  })

  it('提交：按行拆成 URL 数组，空行与首尾空格忽略', async () => {
    const { w } = await setup(ON_ALIYUN)
    await w.find('textarea').setValue('https://cdn.example.com/a.png\n\n  https://cdn.example.com/b.jpg  \n')
    expect(w.text()).toContain('共 2 条')
    await submitButton(w).trigger('click')
    await nextTick()
    expect(vi.mocked(cdnRefresh)).toHaveBeenCalledWith([
      'https://cdn.example.com/a.png',
      'https://cdn.example.com/b.jpg',
    ])
    expect(vi.mocked(ElMessage.success)).toHaveBeenCalled()
  })

  it('空输入不发请求：先提示，避免一次必然失败的往返', async () => {
    const { w } = await setup(ON_ALIYUN)
    await submitButton(w).trigger('click')
    await nextTick()
    expect(vi.mocked(cdnRefresh)).not.toHaveBeenCalled()
    expect(vi.mocked(ElMessage.warning)).toHaveBeenCalledWith('请至少填写一个 URL')
  })
})
