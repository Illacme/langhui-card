import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick, defineComponent, h } from 'vue'
import CompanyProfile from './CompanyProfile.vue'

// 公司简介页：视频来源五档 + 动态表单 + 存储能力探测。
// 这页的价值全在「按来源切换的字段」和「保存前的三道拦截」——
// 漏一条拦截就会存下一条永远播不出视频的配置，而界面上看不出任何异常。
const mocks = vi.hoisted(() => ({
  getCompanyProfile: vi.fn(async (): Promise<any> => ({
    id: 1,
    introText: '朗汇机电成立于 2003 年',
    videoSource: 'none',
    videoChannelId: '',
    videoFeedId: '',
    videoFeedToken: '',
    videoVid: '',
    videoUrl: '',
    videoFallbackUrl: false,
    cover: '/uploads/cover.png',
  })),
  saveCompanyProfile: vi.fn(async (_payload: any) => ({ code: 0 })),
  getUploadCapability: vi.fn(async (): Promise<any> => ({
    driver: 'oss',
    presign: true,
    videoUpload: true,
    videoMaxMb: 200,
    reason: '',
  })),
}))

vi.mock('@/api/content', () => mocks)
vi.mock('element-plus', () => ({
  ElMessage: { success: vi.fn(), error: vi.fn(), warning: vi.fn() },
}))
vi.mock('@element-plus/icons-vue', () => ({ Refresh: {} }))

import { ElMessage } from 'element-plus'

const ElCard = defineComponent({ name: 'ElCard', template: `<div class="el-card"><slot /></div>` })
const ElButton = defineComponent({
  name: 'ElButton',
  props: ['type', 'loading', 'icon'],
  template: `<button class="el-button" v-bind="$attrs"><slot /></button>`,
  inheritAttrs: true,
})
const ElForm = defineComponent({
  name: 'ElForm',
  setup(_p, { slots }) {
    return () => h('form', { class: 'el-form' }, slots.default ? slots.default() : [])
  },
})
const ElFormItem = defineComponent({
  name: 'ElFormItem',
  props: ['label'],
  template: `<div class="el-form-item"><label>{{ label }}</label><slot /></div>`,
})
const ElInput = defineComponent({
  name: 'ElInput',
  props: ['modelValue', 'placeholder', 'type', 'rows'],
  emits: ['update:modelValue'],
  template: `<input class="el-input" :placeholder="placeholder" :value="modelValue"
    @input="$emit('update:modelValue', $event.target.value)" />`,
})
const ElSelect = defineComponent({
  name: 'ElSelect',
  props: ['modelValue'],
  emits: ['update:modelValue'],
  template: `<select class="el-select" :value="modelValue"
    @change="$emit('update:modelValue', $event.target.value)"><slot /></select>`,
})
// disabled 不是声明的 prop，会落到 $attrs 上——正是「存储不支持时禁用上传档」的断言依据
const ElOption = defineComponent({
  name: 'ElOption',
  props: ['label', 'value'],
  template: `<option :value="value" v-bind="$attrs">{{ label }}</option>`,
  inheritAttrs: true,
})
const ElCheckbox = defineComponent({
  name: 'ElCheckbox',
  props: ['modelValue'],
  emits: ['update:modelValue'],
  template: `<label class="el-checkbox"><input type="checkbox" :checked="modelValue"
    @change="$emit('update:modelValue', $event.target.checked)" /><slot /></label>`,
})
const ElAlert = defineComponent({
  name: 'ElAlert',
  props: ['type', 'closable', 'showIcon'],
  template: `<div class="el-alert"><slot name="title" /></div>`,
})
const UploadField = defineComponent({
  name: 'UploadField',
  props: ['modelValue', 'multiple', 'accept', 'listType', 'hideLibrary', 'maxMB', 'tip', 'libraryType'],
  emits: ['update:modelValue'],
  template: `<button class="upload-field" :data-tip="tip"
    @click="$emit('update:modelValue', '/uploads/new-cover.png')" />`,
})

const mountOpts = {
  global: {
    stubs: {
      'el-card': ElCard,
      'el-button': ElButton,
      'el-form': ElForm,
      'el-form-item': ElFormItem,
      'el-input': ElInput,
      'el-select': ElSelect,
      'el-option': ElOption,
      'el-checkbox': ElCheckbox,
      'el-alert': ElAlert,
      UploadField,
    },
    directives: { loading: {} },
  },
}

async function flush() {
  await nextTick()
  await nextTick()
  await nextTick()
}

async function mountPage(profile?: any, cap?: any) {
  if (profile) mocks.getCompanyProfile.mockResolvedValueOnce(profile)
  if (cap) mocks.getUploadCapability.mockResolvedValueOnce(cap)
  const w = mount(CompanyProfile, mountOpts)
  await flush()
  return w
}

function byPlaceholder(w: any, placeholder: string) {
  const el = w.findAll('.el-input').find((i: any) => i.attributes('placeholder') === placeholder)
  if (!el) throw new Error(`找不到 placeholder=${placeholder} 的输入框`)
  return el
}

function saveButton(w: any) {
  const btn = w.findAll('.el-button').find((b: any) => b.text() === '保存')
  if (!btn) throw new Error('找不到保存按钮')
  return btn
}

async function pickSource(w: any, value: string) {
  await w.find('.el-select').setValue(value)
  await flush()
}

describe('content/CompanyProfile', () => {
  beforeEach(() => {
    mocks.getCompanyProfile.mockClear()
    mocks.saveCompanyProfile.mockClear()
    mocks.getUploadCapability.mockClear()
    ;(ElMessage.success as any).mockClear()
    ;(ElMessage.warning as any).mockClear()
  })

  it('挂载即拉取简介与存储能力', async () => {
    const w = await mountPage()

    expect(mocks.getCompanyProfile).toHaveBeenCalledTimes(1)
    expect(mocks.getUploadCapability).toHaveBeenCalledTimes(1)
    expect(byPlaceholder(w, '首页简介区文字').element.value).toBe('朗汇机电成立于 2003 年')
  })

  // 该列是后加的，历史行可能是空串：空串必须归一到 none，否则下拉显示空白
  it('videoSource 为空串时归一到「不展示视频」', async () => {
    const w = await mountPage({ ...(await mocks.getCompanyProfile()), videoSource: '' })

    expect((w.find('.el-select').element as HTMLSelectElement).value).toBe('none')
  })

  it('视频号档：展开 channelId / feedId / feed-token 三项', async () => {
    const w = await mountPage()
    await pickSource(w, 'channels')

    expect(byPlaceholder(w, 'sph 开头，视频号助手首页可查看').exists()).toBe(true)
    expect(byPlaceholder(w, '视频号助手 → 动态管理 → 复制 feedId').exists()).toBe(true)
    expect(byPlaceholder(w, '跨主体才填：MP 平台开权限后移动端复制').exists()).toBe(true)
  })

  it('云存储档：显示视频上传与容量提示', async () => {
    const w = await mountPage()
    await pickSource(w, 'upload')

    const upload = w.findAll('.upload-field').find((u: any) => u.attributes('data-tip')?.includes('mp4'))
    expect(upload).toBeTruthy()
    expect(upload!.attributes('data-tip')).toContain('200MB')
  })

  // 存储不支持视频时「云存储上传」档必须禁用并给出原因，不能让用户选了才报错
  it('存储不支持视频：上传档禁用并显示原因', async () => {
    const w = await mountPage(undefined, {
      driver: 'local',
      presign: false,
      videoUpload: false,
      videoMaxMb: 0,
      reason: '本地存储不支持视频直传，请改用对象存储',
    })

    const opt = w.findAll('option').find((o: any) => o.text() === '云存储上传')
    expect(opt!.attributes('disabled')).toBeDefined()

    await pickSource(w, 'upload')
    expect(w.find('.el-alert').text()).toContain('本地存储不支持视频直传')
  })

  it('外部直链档：显示 MP4 地址输入框', async () => {
    const w = await mountPage()
    await pickSource(w, 'url')

    expect(byPlaceholder(w, 'https:// 开头的 mp4 地址').exists()).toBe(true)
  })

  // 兜底只对「主源可能播不出来」的档位有意义：MP4 档不需要兜底
  it('兜底开关只在视频号档出现', async () => {
    const w = await mountPage()
    expect(w.find('.el-checkbox').exists()).toBe(false)

    await pickSource(w, 'channels')
    expect(w.find('.el-checkbox').exists()).toBe(true)

    await pickSource(w, 'upload')
    expect(w.find('.el-checkbox').exists()).toBe(false)
  })

  // ---- 保存前的三道拦截 ----

  it('视频号档两个标识都没填：拦下且不提交', async () => {
    const w = await mountPage()
    await pickSource(w, 'channels')

    await saveButton(w).trigger('click')
    await flush()

    expect(ElMessage.warning).toHaveBeenCalledWith(
      '视频号档需要填写 feedId（同主体）或 feed-token（跨主体）'
    )
    expect(mocks.saveCompanyProfile).not.toHaveBeenCalled()
  })

  it('填了 feedId 即可通过校验', async () => {
    const w = await mountPage()
    await pickSource(w, 'channels')
    await byPlaceholder(w, '视频号助手 → 动态管理 → 复制 feedId').setValue('feed-123')
    await flush()

    await saveButton(w).trigger('click')
    await flush()

    expect(mocks.saveCompanyProfile).toHaveBeenCalledTimes(1)
    expect(mocks.saveCompanyProfile.mock.calls[0][0]).toMatchObject({
      videoSource: 'channels',
      videoFeedId: 'feed-123',
    })
  })

  it('外部直链档没填地址：拦下且不提交', async () => {
    const w = await mountPage()
    await pickSource(w, 'url')

    await saveButton(w).trigger('click')
    await flush()

    expect(ElMessage.warning).toHaveBeenCalledWith('请填写 MP4 地址')
    expect(mocks.saveCompanyProfile).not.toHaveBeenCalled()
  })

  // 开了兜底却不填地址 = 存了一个永远不生效的开关，必须在保存时拦住
  it('开了兜底却没填地址：拦下且不提交', async () => {
    const w = await mountPage()
    await pickSource(w, 'channels')
    await byPlaceholder(w, '视频号助手 → 动态管理 → 复制 feedId').setValue('feed-1')
    await w.find('.el-checkbox input').setValue(true)
    await flush()

    await saveButton(w).trigger('click')
    await flush()

    expect(ElMessage.warning).toHaveBeenCalledWith('已开启兜底，请填写兜底 MP4 地址')
    expect(mocks.saveCompanyProfile).not.toHaveBeenCalled()
  })

  it('兜底开启并填了地址：正常提交', async () => {
    const w = await mountPage()
    await pickSource(w, 'channels')
    await byPlaceholder(w, '视频号助手 → 动态管理 → 复制 feedId').setValue('feed-1')
    await w.find('.el-checkbox input').setValue(true)
    await flush()
    await byPlaceholder(w, '兜底用的 mp4 地址（云存储或任意 CDN 均可）').setValue('https://x/a.mp4')
    await flush()

    await saveButton(w).trigger('click')
    await flush()

    expect(mocks.saveCompanyProfile).toHaveBeenCalledWith(
      expect.objectContaining({ videoFallbackUrl: true, videoUrl: 'https://x/a.mp4' })
    )
  })

  // ---- 保存 payload ----

  it('保存提交九个字段，不带 id', async () => {
    const w = await mountPage()

    await saveButton(w).trigger('click')
    await flush()

    const payload = mocks.saveCompanyProfile.mock.calls[0][0]
    expect(Object.keys(payload).sort()).toEqual([
      'cover',
      'introText',
      'videoChannelId',
      'videoFallbackUrl',
      'videoFeedId',
      'videoFeedToken',
      'videoSource',
      'videoUrl',
      'videoVid',
    ])
    expect(payload).not.toHaveProperty('id')
    expect(ElMessage.success).toHaveBeenCalledWith('已保存')
  })

  it('选中封面随保存提交', async () => {
    const w = await mountPage()
    const covers = w.findAll('.upload-field')
    await covers[covers.length - 1].trigger('click') // 最后一个是「封面 / 占位图」
    await flush()

    await saveButton(w).trigger('click')
    await flush()

    expect(mocks.saveCompanyProfile).toHaveBeenCalledWith(
      expect.objectContaining({ cover: '/uploads/new-cover.png' })
    )
  })

  it('保存失败：不提示成功', async () => {
    mocks.saveCompanyProfile.mockRejectedValueOnce(new Error('500'))
    const w = await mountPage()

    await saveButton(w).trigger('click')
    await flush()

    expect(ElMessage.success).not.toHaveBeenCalled()
  })

  // 存储能力探测失败不能把整页拖垮，也不能把 rejection 漏成 Uncaught (in promise)：
  // 页面降级为「不可用」并给出可读原因，其余字段照常可编辑
  it('能力探测失败：页面仍可用，且上传档给出可读原因', async () => {
    mocks.getUploadCapability.mockRejectedValueOnce(new Error('500'))
    const w = await mountPage()

    await pickSource(w, 'upload')

    expect(w.find('.el-alert').text()).toContain('存储能力探测失败')
    expect(byPlaceholder(w, '首页简介区文字').exists()).toBe(true)
    expect(saveButton(w).exists()).toBe(true)
  })
})
