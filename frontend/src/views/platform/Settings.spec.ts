import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent, nextTick } from 'vue'
import { createPinia, setActivePinia } from 'pinia'

vi.mock('@/api/settings', () => ({
  getSettings: vi.fn(),
  updateSettings: vi.fn(async () => ({ code: 0, data: { restartKeys: [] } })),
  getPublicSettings: vi.fn(async () => ({ code: 0, data: { brand_name: '智能名片' } })),
}))
vi.mock('element-plus', () => ({
  ElMessage: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() },
}))

import { ElMessage } from 'element-plus'
import { getSettings, updateSettings } from '@/api/settings'
import { useSettingsStore } from '@/store/settings'
import Settings from './Settings.vue'
import type { SettingItem } from '@/api/settings'

// 与后端 setting.registry 一致的夹具（分组 / 类型 / 值 / 生效方式元数据）
const items: SettingItem[] = [
  { key: 'brand_name', group: 'general', type: 'string', value: '智能名片', description: '品牌名', secret: false, restart: false, readOnly: false, envVar: '', configured: true },
  { key: 'miniprogram_base_url', group: 'miniprogram', type: 'string', value: '', description: '小程序基址', secret: false, restart: false, readOnly: false, envVar: '', configured: false },
  { key: 'pinned_company_id', group: 'miniprogram', type: 'int', value: '0', description: '锁定租户', secret: false, restart: false, readOnly: false, envVar: '', configured: true },
  { key: 'upload_allow_ext', group: 'upload', type: 'string_slice', value: 'png,jpg', description: '扩展名', secret: false, restart: false, readOnly: false, envVar: '', configured: true },
  { key: 'upload_max_mb', group: 'upload', type: 'int', value: '10', description: '上传上限', secret: false, restart: false, readOnly: false, envVar: '', configured: true },
  { key: 'wx_appid', group: 'wechat', type: 'string', value: '', description: 'AppID', secret: false, restart: false, readOnly: false, envVar: 'WX_APPID', configured: false },
  { key: 'wx_appsecret', group: 'wechat', type: 'string', value: '******', description: 'AppSecret（敏感）', secret: true, restart: false, readOnly: false, envVar: 'WX_APPSECRET', configured: true },
  { key: 'wxacode_ttl_hours', group: 'wechat', type: 'int', value: '720', description: '小程序码 TTL', secret: false, restart: false, readOnly: false, envVar: '', configured: true },
  { key: 'cdn_enabled', group: 'storage', type: 'bool', value: '', description: 'CDN 刷新', secret: false, restart: true, readOnly: false, envVar: 'CDN_ENABLED', configured: false },
  { key: 'storage_driver', group: 'storage', type: 'string', value: 'local', description: '存储驱动', secret: false, restart: true, readOnly: false, envVar: 'STORAGE_DRIVER', configured: true },
  { key: 'storage_secret_key', group: 'storage', type: 'string', value: '', description: '存储 SecretKey', secret: true, restart: true, readOnly: false, envVar: 'MINIO_SECRET_KEY', configured: false },
  { key: 'cookie_domain', group: 'security', type: 'string', value: '', description: 'Cookie 作用域', secret: false, restart: true, readOnly: true, envVar: 'COOKIE_DOMAIN', configured: false },
  { key: 'cookie_secure', group: 'security', type: 'bool', value: 'false', description: 'Cookie Secure', secret: false, restart: true, readOnly: true, envVar: 'COOKIE_SECURE', configured: true },
  { key: 'db_dsn', group: 'security', type: 'string', value: '******', description: '数据库连接串', secret: true, restart: true, readOnly: true, envVar: 'DB_DSN', configured: true },
  { key: 'jwt_key', group: 'security', type: 'string', value: '******', description: 'JWT 密钥', secret: true, restart: true, readOnly: true, envVar: 'JWT_KEY', configured: true },
  { key: 'independent_lock', group: 'feature', type: 'bool', value: 'false', description: '独立版锁定', secret: false, restart: false, readOnly: false, envVar: '', configured: true },
  { key: 'metrics_enabled', group: 'feature', type: 'bool', value: '', description: '指标开关', secret: false, restart: false, readOnly: false, envVar: 'METRICS_ENABLED', configured: false },
]

// 轻量 stub：仅渲染插槽 / 捕获 props，避免引入完整 Element Plus（jsdom 下过重易 OOM）。
const ElCard = defineComponent({
  name: 'ElCard',
  template: `<div class="el-card"><div class="el-card__header"><slot name="header" /></div><slot /></div>`,
})
const ElAlert = defineComponent({ name: 'ElAlert', template: `<div class="el-alert"><slot /></div>` })
const ElTag = defineComponent({
  name: 'ElTag',
  props: ['size', 'effect', 'type'],
  template: `<span class="el-tag"><slot /></span>`,
})
// 开关改三态下拉：未设置（空串=回退环境变量）/ 开 / 关
const ElSelect = defineComponent({
  name: 'ElSelect',
  props: ['modelValue', 'disabled'],
  emits: ['update:modelValue', 'change'],
  template: `<select class="el-select" :disabled="disabled" :value="modelValue" @change="$emit('change', $event.target.value)"><slot /></select>`,
})
const ElOption = defineComponent({
  name: 'ElOption',
  props: ['label', 'value'],
  template: `<option :value="value">{{ label }}</option>`,
})
const ElInputNumber = defineComponent({
  name: 'ElInputNumber',
  props: ['modelValue', 'min', 'disabled', 'controlsPosition'],
  emits: ['update:modelValue', 'change'],
  template: `<input class="el-input-number" type="number" :disabled="disabled" :value="modelValue" @input="$emit('change', Number($event.target.value))" />`,
})
const ElInput = defineComponent({
  name: 'ElInput',
  props: ['modelValue', 'placeholder', 'type', 'showPassword', 'disabled'],
  emits: ['update:modelValue'],
  template: `<input class="el-input" :type="type || 'text'" :disabled="disabled" :placeholder="placeholder" :value="modelValue" @input="$emit('update:modelValue', $event.target.value)" />`,
})
const ElButton = defineComponent({
  name: 'ElButton',
  props: ['type', 'loading', 'disabled', 'text', 'size'],
  template: `<button class="el-button" :disabled="disabled"><slot /></button>`,
})

const mountOpts = {
  global: {
    stubs: {
      'el-card': ElCard,
      'el-alert': ElAlert,
      'el-tag': ElTag,
      'el-select': ElSelect,
      'el-option': ElOption,
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

// 定位辅助：一律「先找 key 所在的行，再找行内控件」。
// 早先按控件类型全局索引断言打偏过（分组顺序一变就错位），按 key 定位对顺序不敏感。
function rowOf(w: any, key: string) {
  const r = w.findAll('.row').find((x: any) => x.text().includes(key))
  expect(r, `未找到配置项行: ${key}`).toBeTruthy()
  return r!
}
function textInputOf(w: any, key: string) {
  return rowOf(w, key).find('input.el-input')
}
function numberInputOf(w: any, key: string) {
  return rowOf(w, key).find('input.el-input-number')
}
function selectOf(w: any, key: string) {
  return rowOf(w, key).find('select.el-select')
}
function saveButton(w: any) {
  return w.findAll('button.el-button').find((b: any) => b.text().includes('保存'))!
}

describe('Settings 系统设置', () => {
  beforeEach(() => {
    vi.mocked(getSettings).mockReset()
    vi.mocked(getSettings).mockImplementation(async () => ({ code: 0, data: items }) as any)
    vi.mocked(updateSettings).mockClear()
    vi.mocked(updateSettings).mockImplementation(async () => ({ code: 0, data: { restartKeys: [] } }))
    vi.mocked(ElMessage.success).mockClear()
    vi.mocked(ElMessage.warning).mockClear()
  })

  it('按分组渲染全部配置项与说明（含存储与只读分组）', async () => {
    const { w } = await setup()
    const text = w.text()
    ;['通用', '小程序', '上传', '微信', '文件存储与 CDN', '安全与基础设施', '功能开关'].forEach((g) =>
      expect(text).toContain(g),
    )
    expect(text).toContain('brand_name')
    expect(text).toContain('品牌名')
    expect(text).toContain('wxacode_ttl_hours')
    expect(text).toContain('小程序码 TTL')
    // 需重启 / 只读 / 敏感 标记都渲染出来
    expect(rowOf(w, 'storage_driver').text()).toContain('需重启')
    expect(rowOf(w, 'db_dsn').text()).toContain('只读')
    expect(rowOf(w, 'jwt_key').text()).toContain('敏感')
    // 环境变量名提示（只读项只告诉改法）
    expect(rowOf(w, 'db_dsn').text()).toContain('DB_DSN')
  })

  it('无修改时保存按钮禁用，不提交（含敏感项的掩码不被回传）', async () => {
    const { w } = await setup()
    expect(saveButton(w).attributes('disabled')).toBeDefined()
    await saveButton(w).trigger('click')
    expect(updateSettings).not.toHaveBeenCalled()
  })

  it('敏感项输入框留空：已配置的显示占位提示，不把掩码当值提交', async () => {
    const { w } = await setup()
    const input = textInputOf(w, 'wx_appsecret')
    // 后端只给掩码，界面输入框必须是空的（否则一保存就把 '******' 写进库）
    expect((input.element as HTMLInputElement).value).toBe('')
    expect(input.attributes('placeholder')).toContain('已配置')
    expect(input.attributes('type')).toBe('password')

    // 只改别的项，敏感项不应出现在载荷里
    await textInputOf(w, 'brand_name').setValue('朗汇机电')
    await saveButton(w).trigger('click')
    await vi.waitFor(() => expect(updateSettings).toHaveBeenCalledTimes(1))
    expect(updateSettings).toHaveBeenCalledWith({ brand_name: '朗汇机电' })
  })

  it('敏感项填新值才会提交；点「清除」则提交空串回退环境变量', async () => {
    const { w } = await setup()
    await textInputOf(w, 'wx_appsecret').setValue('new-secret')
    await saveButton(w).trigger('click')
    await vi.waitFor(() => expect(updateSettings).toHaveBeenCalledTimes(1))
    expect(updateSettings).toHaveBeenCalledWith({ wx_appsecret: 'new-secret' })

    vi.mocked(updateSettings).mockClear()
    const clearBtn = rowOf(w, 'db_dsn').findAll('button.el-button')
    // db_dsn 是只读项，没有「清除」按钮；storage_secret_key 未配置，也没有；用 wx_appsecret 验证
    expect(clearBtn).toHaveLength(0)
    const secretClear = rowOf(w, 'wx_appsecret').findAll('button.el-button')
    expect(secretClear).toHaveLength(1)
    await secretClear[0].trigger('click')
    await nextTick()
    expect(saveButton(w).attributes('disabled')).toBeUndefined()
    await saveButton(w).trigger('click')
    await vi.waitFor(() => expect(updateSettings).toHaveBeenCalledTimes(1))
    expect(updateSettings).toHaveBeenCalledWith({ wx_appsecret: '' })
  })

  it('只读项渲染为禁用控件，即使被改也不进提交载荷', async () => {
    const { w } = await setup()
    expect(textInputOf(w, 'db_dsn').attributes('disabled')).toBeDefined()
    expect(textInputOf(w, 'cookie_domain').attributes('disabled')).toBeDefined()
    expect(selectOf(w, 'cookie_secure').attributes('disabled')).toBeDefined()
    // 只读项仍展示当前值（敏感项展示掩码而非真实值）
    expect((textInputOf(w, 'db_dsn').element as HTMLInputElement).value).toBe('******')
    expect(selectOf(w, 'cookie_secure').element.value).toBe('false')

    // 绕过禁用直接触发 change，也不应产生变更
    await selectOf(w, 'cookie_secure').setValue('true')
    await nextTick()
    expect(saveButton(w).attributes('disabled')).toBeDefined()
  })

  it('开关三态：未设置 → 开，以 true/false 字符串提交', async () => {
    const { w } = await setup()
    const sel = selectOf(w, 'metrics_enabled')
    expect(sel.element.value).toBe('') // 未设置（跟随环境变量）
    await sel.setValue('true')
    await nextTick()
    expect(saveButton(w).attributes('disabled')).toBeUndefined()

    await saveButton(w).trigger('click')
    await vi.waitFor(() => expect(updateSettings).toHaveBeenCalledTimes(1))
    expect(updateSettings).toHaveBeenCalledWith({ metrics_enabled: 'true' })
  })

  it('改整数项：以字符串提交，避免后端类型校验失败', async () => {
    const { w } = await setup()
    await numberInputOf(w, 'upload_max_mb').setValue('25')
    await saveButton(w).trigger('click')
    await vi.waitFor(() => expect(updateSettings).toHaveBeenCalledTimes(1))
    expect(updateSettings).toHaveBeenCalledWith({ upload_max_mb: '25' })
  })

  it('改品牌名保存后即时同步侧边栏品牌（store）', async () => {
    const { w, store } = await setup()
    await textInputOf(w, 'brand_name').setValue('朗汇机电')
    await saveButton(w).trigger('click')
    await vi.waitFor(() => expect(updateSettings).toHaveBeenCalledTimes(1))
    expect(updateSettings).toHaveBeenCalledWith({ brand_name: '朗汇机电' })
    expect(store.brandName).toBe('朗汇机电')
  })

  it('保存返回需重启项时给出告警提示，避免误以为没生效', async () => {
    vi.mocked(updateSettings).mockImplementation(async () => ({
      code: 0,
      msg: '保存成功；其中 1 项需重启服务后生效：storage_driver',
      data: { restartKeys: ['storage_driver'] },
    }))
    const { w } = await setup()
    await textInputOf(w, 'storage_driver').setValue('oss')
    await saveButton(w).trigger('click')
    await vi.waitFor(() => expect(updateSettings).toHaveBeenCalledTimes(1))
    expect(vi.mocked(ElMessage.warning).mock.calls[0][0]).toContain('storage_driver')
    expect(vi.mocked(ElMessage.success)).not.toHaveBeenCalled()
  })

  it('重置：把改动改回服务端原值，不再有变更', async () => {
    const { w } = await setup()
    const input = textInputOf(w, 'brand_name')
    await input.setValue('临时品牌')
    await nextTick()
    expect(saveButton(w).attributes('disabled')).toBeUndefined()

    const resetBtn = w.findAll('button.el-button').find((b: any) => b.text().includes('重置'))!
    await resetBtn.trigger('click')
    await nextTick()
    expect((input.element as HTMLInputElement).value).toBe('智能名片')
    expect(saveButton(w).attributes('disabled')).toBeDefined()
  })
})
