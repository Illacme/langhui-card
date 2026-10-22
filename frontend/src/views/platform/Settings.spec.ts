import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent, nextTick } from 'vue'
import { createPinia, setActivePinia } from 'pinia'

vi.mock('@/api/settings', () => ({
  getSettings: vi.fn(),
  updateSettings: vi.fn(async () => ({ code: 0, data: { restartKeys: [] } })),
  getPublicSettings: vi.fn(async () => ({ code: 0, data: { brand_name: '微传名片' } })),
}))
vi.mock('element-plus', () => ({
  ElMessage: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() },
  ElMessageBox: { confirm: vi.fn() },
}))

import { ElMessage, ElMessageBox } from 'element-plus'
import { getSettings, updateSettings } from '@/api/settings'
import { useSettingsStore } from '@/store/settings'
import Settings from './Settings.vue'
import type { SettingItem } from '@/api/settings'

// 与后端 setting.registry 一致的夹具（分组 / 类型 / 值 / 生效方式元数据 / 中文名）
const items: SettingItem[] = [
  { key: 'brand_name', name: '品牌名称', group: 'general', type: 'string', value: '微传名片', description: '品牌名', secret: false, restart: false, readOnly: false, envVar: '', configured: true },
  { key: 'miniprogram_base_url', name: '小程序接口基址', group: 'miniprogram', type: 'string', value: '', description: '小程序基址', secret: false, restart: false, readOnly: false, envVar: '', configured: false },
  { key: 'pinned_company_id', name: '独立版锁定企业', group: 'miniprogram', type: 'int', value: '0', description: '锁定租户', secret: false, restart: false, readOnly: false, envVar: '', configured: true },
  { key: 'upload_allow_ext', name: '允许上传的文件类型', group: 'upload', type: 'string_slice', value: 'png,jpg', description: '扩展名', secret: false, restart: false, readOnly: false, envVar: '', configured: true },
  { key: 'upload_max_mb', name: '单张图片大小上限', group: 'upload', type: 'int', value: '10', description: '上传上限', secret: false, restart: false, readOnly: false, envVar: '', configured: true },
  { key: 'wx_appid', name: '小程序 AppID', group: 'wechat', type: 'string', value: '', description: 'AppID', secret: false, restart: false, readOnly: false, envVar: 'WX_APPID', configured: false },
  { key: 'wx_appsecret', name: '小程序 AppSecret', group: 'wechat', type: 'string', value: '******', description: 'AppSecret（敏感）', secret: true, restart: false, readOnly: false, envVar: 'WX_APPSECRET', configured: true },
  { key: 'wxacode_ttl_hours', name: '小程序码缓存时长', group: 'wechat', type: 'int', value: '720', description: '小程序码 TTL', secret: false, restart: false, readOnly: false, envVar: '', configured: true },
  { key: 'access_token_ttl_minutes', name: '登录凭证有效期', group: 'auth', type: 'int', value: '15', description: 'access 有效期', secret: false, restart: false, readOnly: false, envVar: '', configured: true },
  { key: 'login_rate_limit_per_min', name: '登录尝试频率限制', group: 'auth', type: 'int', value: '10', description: '登录限流', secret: false, restart: false, readOnly: false, envVar: '', configured: true },
  { key: 'cdn_enabled', name: '启用 CDN 刷新', group: 'storage', type: 'bool', value: '', description: 'CDN 刷新', secret: false, restart: true, readOnly: false, envVar: 'CDN_ENABLED', configured: false },
  { key: 'cdn_access_key_id', name: 'CDN AccessKeyId', group: 'storage', type: 'string', value: '', description: 'CDN AK', secret: false, restart: true, readOnly: false, envVar: 'CDN_ACCESS_KEY_ID', configured: false },
  { key: 'storage_driver', name: '存储方式', group: 'storage', type: 'string', value: 'local', description: '存储驱动', secret: false, restart: true, readOnly: false, envVar: 'STORAGE_DRIVER', configured: true },
  { key: 'storage_endpoint', name: '存储服务端点', group: 'storage', type: 'string', value: '', description: '对象存储端点', secret: false, restart: true, readOnly: false, envVar: 'MINIO_ENDPOINT', configured: false },
  { key: 'storage_bucket', name: '存储空间名称', group: 'storage', type: 'string', value: '', description: '存储桶', secret: false, restart: true, readOnly: false, envVar: 'MINIO_BUCKET', configured: false },
  { key: 'storage_access_key', name: '存储 AccessKey', group: 'storage', type: 'string', value: '', description: '存储 AK', secret: false, restart: true, readOnly: false, envVar: 'MINIO_ACCESS_KEY', configured: false },
  { key: 'storage_secret_key', name: '存储 SecretKey', group: 'storage', type: 'string', value: '', description: '存储 SecretKey', secret: true, restart: true, readOnly: false, envVar: 'MINIO_SECRET_KEY', configured: false },
  { key: 'storage_use_ssl', name: '存储启用 HTTPS', group: 'storage', type: 'bool', value: '', description: '存储 SSL', secret: false, restart: true, readOnly: false, envVar: 'MINIO_USE_SSL', configured: false },
  { key: 'upload_dir', name: '本地上传目录', group: 'storage', type: 'string', value: '', description: '本地落盘目录', secret: false, restart: true, readOnly: false, envVar: 'UPLOAD_DIR', configured: false },
  { key: 'upload_base_url', name: '文件访问基址', group: 'storage', type: 'string', value: '', description: '文件公开访问地址', secret: false, restart: true, readOnly: false, envVar: 'UPLOAD_BASE_URL', configured: false },
  { key: 'cookie_domain', name: '登录 Cookie 作用域', group: 'security', type: 'string', value: '', description: 'Cookie 作用域', secret: false, restart: true, readOnly: true, envVar: 'COOKIE_DOMAIN', configured: false },
  { key: 'cookie_secure', name: '登录 Cookie 强制 HTTPS', group: 'security', type: 'bool', value: 'false', description: 'Cookie Secure', secret: false, restart: true, readOnly: true, envVar: 'COOKIE_SECURE', configured: true },
  { key: 'db_dsn', name: '数据库连接串', group: 'security', type: 'string', value: '******', description: '数据库连接串', secret: true, restart: true, readOnly: true, envVar: 'DB_DSN', configured: true },
  { key: 'jwt_key', name: '登录签名密钥', group: 'security', type: 'string', value: '******', description: 'JWT 密钥', secret: true, restart: true, readOnly: true, envVar: 'JWT_KEY', configured: true },
  { key: 'independent_lock', name: '独立版企业锁定', group: 'feature', type: 'bool', value: 'false', description: '独立版锁定', secret: false, restart: false, readOnly: false, envVar: '', configured: true },
  { key: 'metrics_enabled', name: '开放监控指标', group: 'feature', type: 'bool', value: '', description: '指标开关', secret: false, restart: false, readOnly: false, envVar: 'METRICS_ENABLED', configured: false },
]

// 轻量 stub：仅渲染插槽 / 捕获 props，避免引入完整 Element Plus（jsdom 下过重易 OOM）。
const ElCard = defineComponent({
  name: 'ElCard',
  template: `<div class="el-card"><div class="el-card__header"><slot name="header" /></div><slot /></div>`,
})
// 渲染 title 与关闭按钮：真实 el-alert 会消费这两个 prop，
// stub 不渲染的话「待重启提示条」这类靠 title 传达的信息就断言不到。
const ElAlert = defineComponent({
  name: 'ElAlert',
  props: {
    type: String,
    title: String,
    description: String,
    // 必须是 Boolean：模板里写无值属性 `closable` 时，只有声明为 Boolean 才会
    // 得到 true，字符串类型会拿到空串、v-if 判假，关闭按钮就渲染不出来
    closable: Boolean,
    showIcon: Boolean,
  },
  emits: ['close'],
  template: `<div class="el-alert"><div class="el-alert__title">{{ title }}</div><slot /><button v-if="closable" class="el-alert__close" @click="$emit('close')">×</button></div>`,
})
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
// 配置向导：对话框 + 步骤条。stub 不传送门，直接内联渲染，便于在 wrapper 里断言。
const ElDialog = defineComponent({
  name: 'ElDialog',
  props: ['modelValue', 'title', 'width', 'closeOnClickModal'],
  template: `<div class="el-dialog" v-if="modelValue"><div class="el-dialog__title">{{ title }}</div><slot /><div class="el-dialog__footer"><slot name="footer" /></div></div>`,
})
const ElSteps = defineComponent({ name: 'ElSteps', props: ['active'], template: `<div class="el-steps"><slot /></div>` })
const ElStep = defineComponent({
  name: 'ElStep',
  props: ['title', 'status'],
  template: `<div class="el-step"><span class="el-step__title">{{ title }}</span><slot /></div>`,
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
      'el-dialog': ElDialog,
      'el-steps': ElSteps,
      'el-step': ElStep,
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
// 非抛出版本：断言某行「不在场」（被搜索过滤隐藏时用）
function rowExists(w: any, key: string) {
  return w.findAll('.row').some((x: any) => x.text().includes(key))
}
function textInputOf(w: any, key: string) {
  return rowOf(w, key).find('input.el-input')
}
function numberInputOf(w: any, key: string) {
  return rowOf(w, key).find('input.el-input-number')
}
// 断言组件 props（min 只存在于组件上，DOM 上没有对应属性）
function numberComponentOf(w: any, key: string) {
  return rowOf(w, key).findComponent({ name: 'ElInputNumber' })
}
function selectOf(w: any, key: string) {
  return rowOf(w, key).find('select.el-select')
}
function saveButton(w: any) {
  return w.findAll('button.el-button').find((b: any) => b.text().includes('保存'))!
}
// 向导对话框内的定位：主列表与向导共用 SettingRow，不限定作用域会一行两处打偏
function dialogOf(w: any) {
  const dlg = w.find('.el-dialog')
  expect(dlg.exists(), '配置向导对话框未打开').toBe(true)
  return dlg
}
function dialogRow(w: any, key: string) {
  const r = dialogOf(w)
    .findAll('.row')
    .find((x: any) => x.text().includes(key))
  expect(r, `向导中未找到配置项行: ${key}`).toBeTruthy()
  return r!
}
function dialogButton(w: any, text: string) {
  const b = dialogOf(w)
    .findAll('button.el-button')
    .find((x: any) => x.text() === text)
  expect(b, `向导中未找到按钮: ${text}`).toBeTruthy()
  return b!
}
function wizardEntryButton(w: any) {
  return w.findAll('button.el-button').find((b: any) => b.text().includes('配置向导'))!
}

describe('Settings 系统设置', () => {
  beforeEach(() => {
    vi.mocked(getSettings).mockReset()
    // 必须返回深拷贝：save() 的乐观推进会就地改 item.configured，
    // 直接返回共享 fixture 会把改动泄漏到后续测试（清空敏感项的测试就中过招）。
    vi.mocked(getSettings).mockImplementation(async () => ({ code: 0, data: items.map((it) => ({ ...it })) }) as any)
    vi.mocked(updateSettings).mockClear()
    vi.mocked(updateSettings).mockImplementation(async () => ({ code: 0, data: { restartKeys: [] } }))
    vi.mocked(ElMessage.success).mockClear()
    vi.mocked(ElMessage.warning).mockClear()
    // 破坏性确认默认「同意」：只有专门测取消行为的用例才改写为拒绝
    vi.mocked(ElMessageBox.confirm).mockReset()
    vi.mocked(ElMessageBox.confirm).mockResolvedValue('confirm' as any)
    // localStorage 每个用例清空：就绪清单展开态与向导「已看过」都是持久化状态，
    // 不清会在用例间互相污染（前一个用例点开过，后一个用例的默认值就变了）
    try {
      localStorage.clear()
    } catch {
      // jsdom 一定能读写；真出异常时至少不影响其它断言
    }
  })

  it('按分组渲染全部配置项与说明（含存储与只读分组）', async () => {
    const { w } = await setup()
    const text = w.text()
    ;['通用', '小程序', '上传', '微信', '登录与令牌', '文件存储与 CDN', '安全与基础设施', '功能开关'].forEach(
      (g) => expect(text).toContain(g),
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

  it('保存含「需重启」项后给出常驻待重启提示（含重启命令），可手动关闭', async () => {
    // 后端会点名哪些项需重启；这里模拟改了存储方式
    vi.mocked(updateSettings).mockImplementationOnce(async () => ({
      code: 0,
      data: { restartKeys: ['storage_driver'] },
    }) as any)
    const { w } = await setup()
    await textInputOf(w, 'storage_driver').setValue('minio')
    await saveButton(w).trigger('click')
    await vi.waitFor(() => expect(w.find('.restart-alert').exists()).toBe(true))
    const t = w.find('.restart-alert').text()
    // 显示中文名而不是 key，且给出可直接照做的重启命令
    expect(t).toContain('存储方式')
    expect(t).not.toContain('storage_driver')
    expect(t).toContain('docker compose restart backend')
    // 关掉后不再占用注意力
    await w.find('.restart-alert .el-alert__close').trigger('click')
    await nextTick()
    expect(w.find('.restart-alert').exists()).toBe(false)
  })

  it('没有需重启项时不显示待重启提示', async () => {
    const { w } = await setup()
    // 必须改成与当前值不同的内容，否则 dirty=false、保存按钮是禁用的
    await textInputOf(w, 'brand_name').setValue('朗汇机电')
    await saveButton(w).trigger('click')
    await vi.waitFor(() => expect(updateSettings).toHaveBeenCalledTimes(1))
    expect(w.find('.restart-alert').exists()).toBe(false)
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
    expect(vi.mocked(ElMessage.warning).mock.calls[0][0]).toContain('1 项需重启服务后才生效')
    expect(vi.mocked(ElMessage.success)).not.toHaveBeenCalled()
    // 具体是哪几项、怎么重启交给常驻提示条（toast 几秒就消失，说不完）
    await nextTick()
    const bar = w.find('.restart-alert')
    expect(bar.exists()).toBe(true)
    expect(bar.text()).toContain('存储方式')
    expect(bar.text()).toContain('docker compose restart backend')
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
    expect((input.element as HTMLInputElement).value).toBe('微传名片')
    expect(saveButton(w).attributes('disabled')).toBeDefined()
  })

  // 只写「只读」不够：运营会以为功能没生效。必须同时说清「现在配了没」和「去哪改」。
  it('只读项标明当前是否已配置，并给出部署环境的改法', async () => {
    const { w } = await setup()
    expect(rowOf(w, 'db_dsn').text()).toContain('已配置')
    // cookie_domain 未配置 = 进程在用内置缺省值，属上线前要修的项
    expect(rowOf(w, 'cookie_domain').text()).toContain('未配置')
    expect(rowOf(w, 'cookie_domain').text()).toContain('COOKIE_DOMAIN')
    // 改法要指向部署环境，而不是让人在本页白找
    expect(rowOf(w, 'db_dsn').text()).toContain('部署环境')
    expect(rowOf(w, 'db_dsn').text()).toContain('重启')
  })

  it('有安全下限的整数项：上传上限最小为 1，界面就选不到 0', async () => {
    const { w } = await setup()
    expect(numberComponentOf(w, 'upload_max_mb').props('min')).toBe(1)
    // 其它整数项（如小程序码 TTL）仍允许 0：后端有回落逻辑，不该在界面硬拦
    expect(numberComponentOf(w, 'wxacode_ttl_hours').props('min')).toBe(0)
  })

  // ---- P0：商业产品可读性（中文名 / 分组导语 / 上线就绪面板）----
  it('配置项以中文名为主标签，key 降级为灰字仍可对照', async () => {
    const { w } = await setup()
    const row = rowOf(w, 'brand_name')
    expect(row.find('.label-text').text()).toBe('品牌名称')
    expect(row.find('.label-key').text()).toBe('brand_name')
    // 只读的中文名同样渲染（数据库连接串）
    expect(rowOf(w, 'db_dsn').find('.label-text').text()).toBe('数据库连接串')
  })

  it('旧部署响应缺 name：标签回落显示 key，不留空白', async () => {
    vi.mocked(getSettings).mockImplementationOnce(async () => ({
      code: 0,
      data: items.map((it) => ({ ...it, name: undefined })),
    }) as any)
    const { w } = await setup()
    expect(rowOf(w, 'brand_name').find('.label-text').text()).toBe('brand_name')
    expect(rowOf(w, 'brand_name').find('.label-key').exists()).toBe(false)
  })

  it('每个分组展示导语：一句话说清管什么、什么时候动', async () => {
    const { w } = await setup()
    expect(w.text()).toContain('产品的基础展示信息') // general
    expect(w.text()).toContain('图片 / 视频 / 小程序码存哪') // storage
    expect(w.text()).toContain('首次部署必配') // miniprogram
    // 8 个分组各一条导语
    expect(w.findAll('.group-blurb')).toHaveLength(8)
  })

  it('上线就绪清单默认收起为摘要条：点「查看详情」才展开，并记住选择', async () => {
    const { w } = await setup()
    // 常驻摘要条：一眼给出「还差几项」及其归属（本页可配 / 环境变量）
    expect(w.find('.ready-bar').exists()).toBe(true)
    expect(w.text()).toContain('3 / 7 项就绪')
    expect(w.text()).toContain('还有 4 项待处理')
    // 清单本身默认不展开：本页主任务是改配置，不能被 7 项清单推到屏幕外
    expect(w.find('.ready-card').exists()).toBe(false)
    await w.find('.ready-bar-toggle').trigger('click')
    await nextTick()
    expect(w.find('.ready-card').exists()).toBe(true)
    expect(w.find('.ready-bar-toggle').text()).toBe('收起')
    await w.find('.ready-bar-toggle').trigger('click')
    await nextTick()
    expect(w.find('.ready-card').exists()).toBe(false)
  })

  it('全新部署（0 项就绪）时清单默认展开：这时它就是首配的导航图', async () => {
    vi.mocked(getSettings).mockImplementationOnce(async () => ({
      code: 0,
      data: items.map((it) => ({ ...it, value: '', configured: false })),
    }) as any)
    const { w } = await setup()
    // 0 就绪会自动拉起向导（stub 不传送门，页面本体仍在同一棵树上，可直接断言）
    expect(w.find('.el-dialog').exists()).toBe(true)
    expect(w.text()).toContain('0 / 7 项就绪')
    expect(w.find('.ready-card').exists()).toBe(true)
  })

  it('上线就绪面板：按「本页可配置 / 环境变量 / 配置文件」分组；只读项给可复制写法而非死路入口', async () => {
    const { w } = await setup()
    // 夹具已配置 jwt_key / db_dsn / storage_driver → 3/7；
    // cookie_secure=false、wx_appid 未配、upload_base_url 夹具缺省、miniprogram_base_url 未配
    expect(w.text()).toContain('上线就绪检查')
    expect(w.text()).toContain('3 / 7 项就绪')
    // 清单默认收起：点「查看详情」展开后再断言明细
    if (!w.find('.ready-card').exists()) {
      await w.find('.ready-bar-toggle').trigger('click')
      await nextTick()
    }
    const readyRows = w.findAll('.ready-row')
    expect(readyRows).toHaveLength(7)

    // 分两组呈现：本页可配 4 项（1 就绪） / 部署环境变量 3 项（2 就绪）
    expect(w.findAll('.ready-section-title')).toHaveLength(2)
    expect(w.text()).toContain('本页可配置')
    expect(w.text()).toContain('环境变量 / 配置文件')

    // 已就绪项：显示「已就绪」且无按钮
    const jwtRow = readyRows.find((r: any) => r.text().includes('登录签名密钥'))!
    expect(jwtRow.text()).toContain('已就绪')
    expect(jwtRow.findAll('button.el-button')).toHaveLength(0)

    // 未就绪的可配置项：「去配置」按钮 + 原因提示；点击走 jumpTo（jsdom 无 scrollIntoView，不应抛错）
    const wxRow = readyRows.find((r: any) => r.text().includes('微信 AppID'))!
    expect(wxRow.text()).toContain('小程序码与微信登录依赖')
    const goBtn = wxRow.findAll('button.el-button')
    expect(goBtn).toHaveLength(1)
    expect(goBtn[0].text()).toBe('去配置')
    await goBtn[0].trigger('click')

    // 未就绪的只读项：不再给「查看」（跳过去只是一行改不了的灰行，等于死路），
    // 改为直接给出可复制的环境变量示例 + 重启提示
    const cookieRow = readyRows.find((r: any) => r.text().includes('登录 Cookie 强制 HTTPS'))!
    expect(cookieRow.text()).toContain('COOKIE_SECURE=true')
    // 改法要同时给出环境变量与配置文件两条路（多数部署是安装向导装的，值落在
    // data/runtime.env 而非环境变量），只提环境变量会让人找不到地方改
    expect(cookieRow.text()).toContain('设为环境变量，或写进配置文件 data/runtime.env，重启后生效')
    const cookieBtns = cookieRow.findAll('button.el-button')
    expect(cookieBtns).toHaveLength(1)
    expect(cookieBtns[0].text()).toBe('复制')
  })

  it('cookie_secure 置为 true 后该项就绪（生产 HTTPS 标准）', async () => {
    vi.mocked(getSettings).mockImplementationOnce(async () => ({
      code: 0,
      data: items.map((it) => (it.key === 'cookie_secure' ? { ...it, value: 'true' } : it)),
    }) as any)
    const { w } = await setup()
    expect(w.text()).toContain('4 / 7 项就绪')
  })

  // ---- P1：依赖联动 + 首次配置向导 ----
  it('依赖联动：local 只亮本地上传目录，云存储凭据置灰；切 minio 后反之', async () => {
    const { w } = await setup()
    // 夹具默认 local：upload_dir 未配置 → 待配置；云凭据「当前选项下不需要」且不出现待配置
    expect(rowOf(w, 'upload_dir').text()).toContain('待配置')
    ;['storage_endpoint', 'storage_bucket', 'storage_access_key', 'storage_secret_key'].forEach((k) => {
      expect(rowOf(w, k).text()).toContain('当前选项下不需要')
      expect(rowOf(w, k).text()).not.toContain('待配置')
    })
    // CDN 未开：CDN 凭据同样置灰
    expect(rowOf(w, 'cdn_access_key_id').text()).toContain('当前选项下不需要')

    // 切到 minio：云凭据转亮（待配置），本地上传目录转为置灰
    await textInputOf(w, 'storage_driver').setValue('minio')
    await nextTick()
    expect(rowOf(w, 'storage_endpoint').text()).toContain('待配置')
    expect(rowOf(w, 'storage_endpoint').text()).not.toContain('当前选项下不需要')
    expect(rowOf(w, 'upload_dir').text()).toContain('当前选项下不需要')
    expect(rowOf(w, 'upload_dir').text()).not.toContain('待配置')
  })

  it('配置向导：默认落在第一个未完成步骤，条件显隐与主列表一致', async () => {
    const { w } = await setup()
    await wizardEntryButton(w).trigger('click')
    await nextTick()
    // 品牌名已配置 → 跳过；存储与上传有未配置项（upload_dir / upload_base_url）→ 落在第 2 步
    expect(dialogOf(w).text()).toContain('第 2 步 / 共 6 步')
    expect(dialogOf(w).text()).toContain('存储与上传')
    // local 驱动：向导里出现本地上传目录，不出现云凭据
    expect(dialogOf(w).text()).toContain('upload_dir')
    expect(dialogOf(w).text()).not.toContain('storage_endpoint')

    // 向导内切 minio：端点转亮出现、目录隐藏——联动在向导里同样生效
    await dialogRow(w, 'storage_driver').find('input.el-input').setValue('minio')
    await nextTick()
    expect(dialogRow(w, 'storage_endpoint').text()).toContain('待配置')
    expect(dialogOf(w).text()).not.toContain('upload_dir')
  })

  it('向导「稍后再说」：恢复打开前的草稿，不产生任何提交', async () => {
    const { w } = await setup()
    await wizardEntryButton(w).trigger('click')
    await nextTick()
    await dialogRow(w, 'upload_dir').find('input.el-input').setValue('/data/uploads')
    await nextTick()
    // 草稿已脏（主列表保存按钮可用）
    expect(saveButton(w).attributes('disabled')).toBeUndefined()

    await dialogButton(w, '稍后再说').trigger('click')
    await nextTick()
    // 对话框关闭、草稿复原、不提交
    expect(w.find('.el-dialog').exists()).toBe(false)
    expect((textInputOf(w, 'upload_dir').element as HTMLInputElement).value).toBe('')
    expect(saveButton(w).attributes('disabled')).toBeDefined()
    expect(updateSettings).not.toHaveBeenCalled()
  })

  it('向导无改动时「完成」直接关闭，不发起空保存', async () => {
    const { w } = await setup()
    await wizardEntryButton(w).trigger('click')
    await nextTick()
    // 从第 2 步一路点到安全确认（共 6 步）
    for (let i = 0; i < 4; i++) {
      await dialogButton(w, '下一步').trigger('click')
      await nextTick()
    }
    expect(dialogOf(w).text()).toContain('第 6 步 / 共 6 步')
    expect(dialogOf(w).text()).toContain('安全确认')
    const finish = dialogButton(w, '完成')
    expect(finish.exists()).toBe(true)
    await finish.trigger('click')
    await nextTick()
    expect(w.find('.el-dialog').exists()).toBe(false)
    expect(updateSettings).not.toHaveBeenCalled()
  })

  it('向导有改动时「保存并完成」：提交变更、成功后关闭，且就绪统计即时推进', async () => {
    const { w } = await setup()
    await wizardEntryButton(w).trigger('click')
    await nextTick()
    await dialogRow(w, 'upload_dir').find('input.el-input').setValue('/data/uploads')
    await nextTick()
    for (let i = 0; i < 4; i++) {
      await dialogButton(w, '下一步').trigger('click')
      await nextTick()
    }
    // 最后一步点名「本次改动中需重启的项」（upload_dir 是 Restart 项）
    expect(dialogOf(w).text()).toContain('需重启服务后生效')
    const finish = dialogButton(w, '保存并完成')
    await finish.trigger('click')
    await vi.waitFor(() => expect(updateSettings).toHaveBeenCalledTimes(1))
    expect(updateSettings).toHaveBeenCalledWith({ upload_dir: '/data/uploads' })
    // 保存成功 → 对话框关闭；upload_dir 不属于上线就绪检查的 7 个 key，
    // 就绪数保持 3/7（不伪造进度），但该行「待配置」标记应随之消失
    expect(w.find('.el-dialog').exists()).toBe(false)
    expect(w.text()).toContain('3 / 7 项就绪')
    expect(rowOf(w, 'upload_dir').text()).not.toContain('待配置')
  })

  it('全新部署（一项都没就绪）进入页面自动拉起配置向导', async () => {
    vi.mocked(getSettings).mockImplementationOnce(async () => ({
      code: 0,
      data: items.map((it) => ({ ...it, value: '', configured: false })),
    }) as any)
    const { w } = await setup()
    await vi.waitFor(() => expect(w.find('.el-dialog').exists()).toBe(true))
    // 品牌名也未配置 → 落在第 1 步
    expect(dialogOf(w).text()).toContain('第 1 步 / 共 6 步')
    expect(dialogOf(w).text()).toContain('品牌与身份')
  })

  // ---- P2：搜索过滤 + 破坏性更改二次确认 ----
  it('搜索过滤：环境变量名可搜，未命中分组整组隐藏，无匹配给空态提示', async () => {
    const { w } = await setup()
    const search = w.find('input.head-search')
    // 环境变量名搜索（大小写不敏感）：MINIO_ENDPOINT → 只命中存储服务端点
    await search.setValue('minio_endpoint')
    await nextTick()
    expect(w.text()).toContain('匹配 1 /')
    expect(rowExists(w, 'storage_endpoint')).toBe(true)
    expect(rowExists(w, 'brand_name')).toBe(false)
    // 分组中文名也参与匹配：搜「存储」出整个存储组
    await search.setValue('存储')
    await nextTick()
    expect(rowExists(w, 'storage_endpoint')).toBe(true)
    expect(rowExists(w, 'upload_dir')).toBe(true)
    expect(rowExists(w, 'brand_name')).toBe(false)
    // 无匹配：空态提示 + 分组全部隐藏
    await search.setValue('zzz-不存在')
    await nextTick()
    expect(w.find('.filter-empty').exists()).toBe(true)
    expect(w.findAll('.group-card')).toHaveLength(0)
    // 清空搜索恢复全量
    await search.setValue('')
    await nextTick()
    expect(w.findAll('.group-card')).toHaveLength(8)
    expect(rowExists(w, 'brand_name')).toBe(true)
  })

  it('过滤不影响草稿：被搜掉的行保存时仍进载荷（草稿按 key 持有）', async () => {
    const { w } = await setup()
    await textInputOf(w, 'brand_name').setValue('朗汇机电')
    await w.find('input.head-search').setValue('minio_endpoint')
    await nextTick()
    expect(rowExists(w, 'brand_name')).toBe(false)
    await saveButton(w).trigger('click')
    await vi.waitFor(() => expect(updateSettings).toHaveBeenCalledTimes(1))
    expect(updateSettings).toHaveBeenCalledWith({ brand_name: '朗汇机电' })
  })

  it('破坏性更改二次确认：存储方式变更弹确认，取消则不提交、草稿保留', async () => {
    vi.mocked(ElMessageBox.confirm).mockRejectedValueOnce('cancel')
    const { w } = await setup()
    await textInputOf(w, 'storage_driver').setValue('minio')
    await saveButton(w).trigger('click')
    await vi.waitFor(() => expect(ElMessageBox.confirm).toHaveBeenCalledTimes(1))
    const [msg, title] = vi.mocked(ElMessageBox.confirm).mock.calls[0]
    expect(title).toBe('破坏性更改确认')
    expect(msg).toContain('存储方式')
    expect(msg).toContain('历史文件')
    // 未确认前不提交；草稿保留（保存按钮仍可用，可改回去或再次确认）
    expect(updateSettings).not.toHaveBeenCalled()
    expect(saveButton(w).attributes('disabled')).toBeUndefined()
  })

  it('破坏性确认通过后正常提交（确认即保存）', async () => {
    const { w } = await setup()
    await textInputOf(w, 'storage_driver').setValue('oss')
    await saveButton(w).trigger('click')
    await vi.waitFor(() => expect(updateSettings).toHaveBeenCalledTimes(1))
    expect(updateSettings).toHaveBeenCalledWith({ storage_driver: 'oss' })
    expect(ElMessageBox.confirm).toHaveBeenCalledTimes(1)
  })

  it('无害改动不弹确认：品牌名随时可改，不该被二次确认骚扰', async () => {
    const { w } = await setup()
    await textInputOf(w, 'brand_name').setValue('朗汇机电')
    await saveButton(w).trigger('click')
    await vi.waitFor(() => expect(updateSettings).toHaveBeenCalledWith({ brand_name: '朗汇机电' }))
    expect(ElMessageBox.confirm).not.toHaveBeenCalled()
  })

  it('清空敏感项同样需确认：消息讲清回退环境变量的后果', async () => {
    const { w } = await setup()
    const secretClear = rowOf(w, 'wx_appsecret').findAll('button.el-button')
    await secretClear[0].trigger('click')
    await nextTick()
    await saveButton(w).trigger('click')
    await vi.waitFor(() => expect(ElMessageBox.confirm).toHaveBeenCalledTimes(1))
    expect(vi.mocked(ElMessageBox.confirm).mock.calls[0][0]).toContain('回退环境变量')
    // 默认确认 → 正常提交空串
    expect(updateSettings).toHaveBeenCalledWith({ wx_appsecret: '' })
  })

  it('向导内破坏性改动：确认被取消则对话框不关、不提交', async () => {
    vi.mocked(ElMessageBox.confirm).mockRejectedValueOnce('cancel')
    const { w } = await setup()
    await wizardEntryButton(w).trigger('click')
    await nextTick()
    await dialogRow(w, 'storage_driver').find('input.el-input').setValue('minio')
    await nextTick()
    for (let i = 0; i < 4; i++) {
      await dialogButton(w, '下一步').trigger('click')
      await nextTick()
    }
    await dialogButton(w, '保存并完成').trigger('click')
    await vi.waitFor(() => expect(ElMessageBox.confirm).toHaveBeenCalledTimes(1))
    expect(updateSettings).not.toHaveBeenCalled()
    // 对话框保持打开，用户可改回来或重新确认
    expect(dialogOf(w).exists()).toBe(true)
  })
})
