import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick, defineComponent } from 'vue'
import UploadField from './UploadField.vue'

// 轻量 stub：仅捕获 el-upload 透传的 props，避免引入完整 Element Plus（jsdom 下过重易 OOM）。
const ElUpload = defineComponent({
  name: 'ElUpload',
  props: ['action', 'limit', 'fileList', 'multiple', 'accept', 'listType'],
  template: `<div class="el-upload"><slot /><slot name="tip" /></div>`,
})

const mountOpts = { global: { stubs: { 'el-upload': ElUpload } } }

describe('UploadField', () => {
  it('渲染选择文件按钮与提示文案', () => {
    const w = mount(UploadField, { props: { modelValue: '' }, ...mountOpts })
    expect(w.text()).toContain('选择文件')
    expect(w.text()).toContain('支持图片')
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
})
