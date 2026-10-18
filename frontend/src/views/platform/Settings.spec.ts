import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent, nextTick } from 'vue'
import { createPinia, setActivePinia } from 'pinia'

vi.mock('@/api/settings', () => ({
  getSettings: vi.fn(),
  updateSettings: vi.fn(async () => true),
  getPublicSettings: vi.fn(async () => ({ code: 0, data: { brand_name: '智能名片' } })),
}))
vi.mock('element-plus', () => ({
  ElMessage: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() },
}))

import { getSettings, updateSettings } from '@/api/settings'
import { useSettingsStore } from '@/store/settings'
import Settings from './Settings.vue'

// 与后端 setting.registry 一致的夹具（分组 / 类型 / 值）
const items = [
  { key: 'brand_name', group: 'general', type: 'string', value: '智能名片', description: '品牌名' },
  { key: 'miniprogram_base_url', group: 'miniprogram', type: 'string', value: '', description: '小程序基址' },
  { key: 'pinned_company_id', group: 'miniprogram', type: 'int', value: '0', description: '锁定租户' },
  { key: 'upload_max_mb', group: 'upload', type: 'int', value: '10', description: '上传上限' },
  { key: 'upload_allow_ext', group: 'upload', type: 'string_slice', value: 'png,jpg', description: '扩展名' },
  { key: 'wxacode_ttl_hours', group: 'wechat', type: 'int', value: '720', description: '小程序码 TTL' },
  { key: 'wx_appid', group: 'wechat', type: 'string', value: '', description: 'AppID' },
  { key: 'metrics_enabled', group: 'feature', type: 'bool', value: 'false', description: '指标开关' },
  { key: 'independent_lock', group: 'feature', type: 'bool', value: 'false', description: '独立版锁定' },
]

// 轻量 stub：仅渲染插槽 / 捕获 props，避免引入完整 Element Plus（jsdom 下过重易 OOM）。
const ElCard = defineComponent({
  name: 'ElCard',
  template: `<div class="el-card"><div class="el-card__header"><slot name="header" /></div><slot /></div>`,
})
const ElAlert = defineComponent({ name: 'ElAlert', template: `<div class="el-alert"><slot /></div>` })
const ElTag = defineComponent({
  name: 'ElTag',
  props: ['size', 'effect'],
  template: `<span class="el-tag"><slot /></span>`,
})
// 开关：点击在 true / false 间翻转（页面用 @change 接收布尔）
const ElSwitch = defineComponent({
  name: 'ElSwitch',
  props: ['modelValue', 'activeText', 'inactiveText'],
  emits: ['update:modelValue', 'change'],
  template: `<button class="el-switch" @click="$emit('change', !modelValue)">{{ modelValue ? '开' : '关' }}</button>`,
})
const ElInputNumber = defineComponent({
  name: 'ElInputNumber',
  props: ['modelValue', 'min', 'controlsPosition'],
  emits: ['update:modelValue', 'change'],
  template: `<input class="el-input-number" type="number" :value="modelValue" @input="$emit('change', Number($event.target.value))" />`,
})
const ElInput = defineComponent({
  name: 'ElInput',
  props: ['modelValue', 'placeholder', 'type'],
  emits: ['update:modelValue'],
  template: `<input class="el-input" :placeholder="placeholder" :value="modelValue" @input="$emit('update:modelValue', $event.target.value)" />`,
})
const ElButton = defineComponent({
  name: 'ElButton',
  props: ['type', 'loading', 'disabled'],
  template: `<button class="el-button" :disabled="disabled"><slot /></button>`,
})

const mountOpts = {
  global: {
    stubs: {
      'el-card': ElCard,
      'el-alert': ElAlert,
      'el-tag': ElTag,
      'el-switch': ElSwitch,
      'el-input-number': ElInputNumber,
      'el-input': ElInput,
      'el-button': ElButton,
    },
  },
}

async function setup() {
  localStorage.clear()
  setActivePinia(createPinia())
  const w = mount(Settings, mountOpts)
  await vi.waitFor(() => expect(getSettings).toHaveBeenCalled())
  await nextTick()
  return { w, store: useSettingsStore() }
}

// 分组顺序由前端 GROUP_ORDER 决定：通用 → 小程序 → 上传 → 微信 → 功能开关。
// 因此：
//   - 开关（仅 bool 项渲染）依次为 metrics_enabled、independent_lock
//   - 数字框（仅 int 项渲染）依次为 pinned_company_id、upload_max_mb、wxacode_ttl_hours
//   - 文本框（string / string_slice）依次为 brand_name、miniprogram_base_url、
//     upload_allow_ext、wx_appid
function saveButton(w: any) {
  return w.findAll('button.el-button').find((b: any) => b.text().includes('保存'))!
}

describe('Settings 系统设置', () => {
  beforeEach(() => {
    vi.mocked(getSettings).mockReset()
    vi.mocked(getSettings).mockImplementation(async () => ({ code: 0, data: items }) as any)
    vi.mocked(updateSettings).mockClear()
  })

  it('按分组渲染全部配置项与说明', async () => {
    const { w } = await setup()
    const text = w.text()
    // 五个分组中文名
    ;['通用', '小程序', '上传', '微信', '功能开关'].forEach((g) => expect(text).toContain(g))
    // 配置项 key 与说明都出现在界面上
    expect(text).toContain('brand_name')
    expect(text).toContain('品牌名')
    expect(text).toContain('wxacode_ttl_hours')
    expect(text).toContain('小程序码 TTL')
  })

  it('无修改时保存按钮禁用，不提交', async () => {
    const { w } = await setup()
    expect(saveButton(w).attributes('disabled')).toBeDefined()
    await saveButton(w).trigger('click')
    expect(updateSettings).not.toHaveBeenCalled()
  })

  it('改开关后只提交变更项，且布尔以 true/false 字符串提交', async () => {
    const { w } = await setup()
    const switches = w.findAll('button.el-switch')
    expect(switches).toHaveLength(2) // metrics_enabled / independent_lock

    await switches[0].trigger('click') // metrics_enabled: false -> true
    await nextTick()
    expect(saveButton(w).attributes('disabled')).toBeUndefined()

    await saveButton(w).trigger('click')
    await vi.waitFor(() => expect(updateSettings).toHaveBeenCalledTimes(1))
    // 只带变更的那一项，不把未改动的 9 项全量回传
    expect(updateSettings).toHaveBeenCalledWith({ metrics_enabled: 'true' })
  })

  it('改整数项：以字符串提交，避免后端类型校验失败', async () => {
    const { w } = await setup()
    const numbers = w.findAll('input.el-input-number')
    await numbers[1].setValue('25') // upload_max_mb
    await saveButton(w).trigger('click')
    await vi.waitFor(() => expect(updateSettings).toHaveBeenCalledTimes(1))
    expect(updateSettings).toHaveBeenCalledWith({ upload_max_mb: '25' })
  })

  it('改品牌名保存后即时同步侧边栏品牌（store）', async () => {
    const { w, store } = await setup()
    const inputs = w.findAll('input.el-input')
    await inputs[0].setValue('朗汇机电') // brand_name
    await saveButton(w).trigger('click')
    await vi.waitFor(() => expect(updateSettings).toHaveBeenCalledTimes(1))
    expect(updateSettings).toHaveBeenCalledWith({ brand_name: '朗汇机电' })
    expect(store.brandName).toBe('朗汇机电')
  })

  it('重置：把改动改回服务端原值，不再有变更', async () => {
    const { w } = await setup()
    const inputs = w.findAll('input.el-input')
    await inputs[0].setValue('临时品牌')
    await nextTick()
    expect(saveButton(w).attributes('disabled')).toBeUndefined()

    const resetBtn = w.findAll('button.el-button').find((b: any) => b.text().includes('重置'))!
    await resetBtn.trigger('click')
    await nextTick()
    expect((inputs[0].element as HTMLInputElement).value).toBe('智能名片')
    expect(saveButton(w).attributes('disabled')).toBeDefined()
  })
})
