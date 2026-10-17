import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick, defineComponent } from 'vue'
import UploadField from './UploadField.vue'

// UploadField 内的素材库弹窗（AssetPicker）有自己的单测；
// 这里把它桩成只透传 props / 派发事件的壳，专注验证入口与 picked 回填逻辑。
// element-plus 的 ElMessage 同样 mock 掉（限位提示等分支只断言调用，不渲染真实消息）。
vi.mock('element-plus', () => ({
  ElMessage: { success: vi.fn(), error: vi.fn(), warning: vi.fn() },
}))

import { ElMessage } from 'element-plus'

// 轻量 stub：仅捕获 el-upload 透传的 props，避免引入完整 Element Plus（jsdom 下过重易 OOM）。
const ElUpload = defineComponent({
  name: 'ElUpload',
  props: ['action', 'limit', 'fileList', 'multiple', 'accept', 'listType'],
  template: `<div class="el-upload"><slot /><slot name="tip" /></div>`,
})
const AssetPickerStub = defineComponent({
  name: 'AssetPicker',
  props: ['modelValue', 'multiple', 'limit', 'defaultCategory'],
  emits: ['update:modelValue', 'picked'],
  template: `<div class="asset-picker-stub" />`,
})

const mountOpts = {
  global: { stubs: { 'el-upload': ElUpload, AssetPicker: AssetPickerStub } },
}

describe('UploadField', () => {
  beforeEach(() => {
    vi.mocked(ElMessage.warning).mockClear()
  })

  it('渲染选择文件按钮与提示文案', () => {
    const w = mount(UploadField, { props: { modelValue: '' }, ...mountOpts })
    expect(w.text()).toContain('选择文件')
    expect(w.text()).toContain('支持图片')
  })

  it('渲染「从素材库选择」入口', () => {
    const w = mount(UploadField, { props: { modelValue: '' }, ...mountOpts })
    expect(w.text()).toContain('从素材库选择')
  })

  it('单选 modelValue 回显为文件列表（文件名取 URL 末段）', async () => {
    const w = mount(UploadField, {
      props: { modelValue: 'http://cdn.example.com/path/abc.png' },
      ...mountOpts,
    })
    await nextTick()
    const fl = w.findComponent(ElUpload).props('fileList') as Array<{ name: string }>
    expect(fl).toHaveLength(1)
    expect(fl[0].name).toBe('abc.png')
  })

  it('多选 modelValue 数组回显多个文件', async () => {
    const w = mount(UploadField, {
      props: { modelValue: ['http://x/a.png', 'http://x/b.jpg'], multiple: true },
      ...mountOpts,
    })
    await nextTick()
    const fl = w.findComponent(ElUpload).props('fileList') as Array<{ name: string }>
    expect(fl.map((f) => f.name)).toEqual(['a.png', 'b.jpg'])
  })

  it('普通上传 action 与 limit 透传给 el-upload', async () => {
    const w = mount(UploadField, {
      props: { modelValue: '', action: '/api/admin/upload' },
      ...mountOpts,
    })
    await nextTick()
    const up = w.findComponent(ElUpload)
    expect(up.props('action')).toBe('/api/admin/upload')
    expect(up.props('limit')).toBe(9)
  })

  it('library-type 透传给素材选择器（定位默认分类）', () => {
    const w = mount(UploadField, { props: { modelValue: '', libraryType: 'icon' }, ...mountOpts })
    expect(w.findComponent(AssetPickerStub).props('defaultCategory')).toBe('icon')
    const w2 = mount(UploadField, { props: { modelValue: '', multiple: true, limit: 3 }, ...mountOpts })
    const picker = w2.findComponent(AssetPickerStub)
    expect(picker.props('multiple')).toBe(true)
    expect(picker.props('limit')).toBe(3)
  })

  it('素材库单选：picked 直接替换 modelValue', async () => {
    const w = mount(UploadField, { props: { modelValue: 'http://x/old.png' }, ...mountOpts })
    w.findComponent(AssetPickerStub).vm.$emit('picked', 'http://x/lib.png')
    await nextTick()
    expect(w.emitted('update:modelValue')![0]).toEqual(['http://x/lib.png'])
  })

  it('素材库多选：picked 数组追加到已有值', async () => {
    const w = mount(UploadField, {
      props: { modelValue: ['http://x/a.png'], multiple: true, limit: 9 },
      ...mountOpts,
    })
    w.findComponent(AssetPickerStub).vm.$emit('picked', ['http://x/b.png', 'http://x/c.png'])
    await nextTick()
    expect(w.emitted('update:modelValue')![0]).toEqual([['http://x/a.png', 'http://x/b.png', 'http://x/c.png']])
  })

  it('素材库多选超限：截断到 limit 并提示', async () => {
    const w = mount(UploadField, {
      props: { modelValue: ['http://x/a.png'], multiple: true, limit: 2 },
      ...mountOpts,
    })
    w.findComponent(AssetPickerStub).vm.$emit('picked', ['http://x/b.png', 'http://x/c.png', 'http://x/d.png'])
    await nextTick()
    expect(w.emitted('update:modelValue')![0]).toEqual([['http://x/a.png', 'http://x/b.png']])
    expect(ElMessage.warning).toHaveBeenCalledWith('最多上传 2 个文件')
  })
})
