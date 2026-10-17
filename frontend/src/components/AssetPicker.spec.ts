import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick, defineComponent } from 'vue'
import AssetPicker from './AssetPicker.vue'

// 素材选择器：mock 掉素材 API（清单 / 采用）与 UI 反馈层；
// 预览图 URL 拼接逻辑原地保留（返回可断言的路由）。
const mocks = vi.hoisted(() => ({
  listLibrary: vi.fn(),
  adoptAsset: vi.fn(),
}))
const { listLibrary, adoptAsset } = mocks

vi.mock('@/api/assets', () => ({
  listLibrary: mocks.listLibrary,
  adoptAsset: mocks.adoptAsset,
  libraryFileUrl: (rel: string) => '/api/assets/file/' + rel,
}))
vi.mock('element-plus', () => ({
  ElMessage: { success: vi.fn(), error: vi.fn(), warning: vi.fn() },
}))
// 图标仅作为 prop 值传递（:prefix-icon / 卡片角标），不会真正渲染，桩成普通对象即可
vi.mock('@element-plus/icons-vue', () => ({ Check: {}, Loading: {}, Search: {} }))

import { ElMessage } from 'element-plus'

// ── 轻量 stub（jsdom 下完整 Element Plus 过重易 OOM） ──
const ElDialog = defineComponent({
  name: 'ElDialog',
  props: ['modelValue', 'title', 'width', 'top', 'closeOnClickModal'],
  emits: ['update:modelValue'],
  template: `<div v-if="modelValue" class="el-dialog"><slot /><slot name="footer" /></div>`,
})
const ElRadioGroup = defineComponent({
  name: 'ElRadioGroup',
  props: ['modelValue'],
  template: `<div class="el-radio-group"><slot /></div>`,
})
const ElRadioButton = defineComponent({
  name: 'ElRadioButton',
  props: ['label'],
  template: `<button class="el-radio-button"><slot /></button>`,
})
const ElInput = defineComponent({
  name: 'ElInput',
  props: ['modelValue', 'placeholder', 'clearable'],
  emits: ['update:modelValue'],
  template: `<input class="el-input" :value="modelValue" @input="$emit('update:modelValue', $event.target.value)" />`,
})
const ElButton = defineComponent({
  name: 'ElButton',
  props: ['type', 'loading', 'disabled'],
  template: `<button class="el-button"><slot /></button>`,
})
const ElEmpty = defineComponent({
  name: 'ElEmpty',
  props: ['description', 'imageSize'],
  template: `<div class="el-empty" />`,
})

const mountOpts = {
  global: {
    stubs: {
      'el-dialog': ElDialog,
      'el-radio-group': ElRadioGroup,
      'el-radio-button': ElRadioButton,
      'el-input': ElInput,
      'el-button': ElButton,
      'el-empty': ElEmpty,
    },
    directives: { loading: {} },
  },
}

const LIB_ITEMS = [
  { id: 'icon-house', name: '居住住宅', category: 'icon', file: 'icons/icon-house.png', thumb: 'icons/icon-house.png', width: 256, height: 256, tags: ['住宅', 'home'] },
  { id: 'icon-wrench', name: '维修服务', category: 'icon', file: 'icons/icon-wrench.png', thumb: 'icons/icon-wrench.png', width: 256, height: 256, tags: ['维修'] },
  { id: 'cover-skyline', name: '天际线', category: 'cover', file: 'covers/cover-skyline.jpg', thumb: 'thumbs/covers/cover-skyline.jpg', width: 960, height: 504, tags: ['封面'] },
  { id: 'logo-building', name: '建筑', category: 'logo', file: 'logos/logo-building.png', thumb: 'logos/logo-building.png', width: 512, height: 512, tags: ['LOGO'] },
]

function mockList(items = LIB_ITEMS) {
  listLibrary.mockResolvedValue({
    code: 0,
    data: {
      version: 1,
      categories: [
        { key: 'icon', name: '图标' },
        { key: 'cover', name: '封面' },
        { key: 'logo', name: 'LOGO' },
      ],
      items,
    },
  })
}

describe('AssetPicker', () => {
  beforeEach(() => {
    mockList()
    adoptAsset.mockResolvedValue({ code: 0, data: { url: 'http://cdn/uploads/library/icon-house.png' } })
  })

  it('打开时拉取清单并渲染素材卡片', async () => {
    const w = mount(AssetPicker, { props: { modelValue: true }, ...mountOpts })
    await vi.waitFor(() => expect(listLibrary).toHaveBeenCalledTimes(1))
    await nextTick()
    await nextTick()
    expect(w.findAll('.asset-card')).toHaveLength(4)
    expect(w.text()).toContain('居住住宅')
    expect(w.text()).toContain('天际线')
  })

  it('defaultCategory 定位到对应分类（只渲染该分类素材）', async () => {
    const w = mount(AssetPicker, { props: { modelValue: true, defaultCategory: 'cover' }, ...mountOpts })
    await vi.waitFor(() => expect(listLibrary).toHaveBeenCalled())
    await nextTick()
    await nextTick()
    expect(w.findAll('.asset-card')).toHaveLength(1)
    expect(w.text()).toContain('天际线')
  })

  it('单选：点卡片即采用并回传 URL、关闭弹窗', async () => {
    const w = mount(AssetPicker, { props: { modelValue: true }, ...mountOpts })
    await vi.waitFor(() => expect(listLibrary).toHaveBeenCalled())
    await nextTick()
    await nextTick()
    await w.findAll('.asset-card')[0].trigger('click')
    await vi.waitFor(() => expect(adoptAsset).toHaveBeenCalledWith('icon-house'))
    expect(w.emitted('picked')).toBeTruthy()
    expect(w.emitted('picked')![0]).toEqual(['http://cdn/uploads/library/icon-house.png'])
    // 采用后自动关闭
    expect(w.emitted('update:modelValue')).toBeTruthy()
    expect(w.emitted('update:modelValue')![0]).toEqual([false])
  })

  it('多选：点卡片累计选择，footer 批量采用后回传 URL 数组', async () => {
    adoptAsset
      .mockResolvedValueOnce({ code: 0, data: { url: 'http://cdn/a.png' } })
      .mockResolvedValueOnce({ code: 0, data: { url: 'http://cdn/b.png' } })
    const w = mount(AssetPicker, { props: { modelValue: true, multiple: true }, ...mountOpts })
    await vi.waitFor(() => expect(listLibrary).toHaveBeenCalled())
    await nextTick()
    await nextTick()
    await w.findAll('.asset-card')[0].trigger('click')
    await w.findAll('.asset-card')[1].trigger('click')
    await nextTick()
    // 两张卡进入选中态（含角标），未触发采用
    expect(w.findAll('.asset-card.selected')).toHaveLength(2)
    expect(adoptAsset).not.toHaveBeenCalled()
    // footer 按钮批量采用
    const addBtn = w.findAll('.el-button').find((b) => b.text().includes('添加所选'))
    expect(addBtn).toBeTruthy()
    await addBtn!.trigger('click')
    await vi.waitFor(() => expect(adoptAsset).toHaveBeenCalledTimes(2))
    expect(adoptAsset).toHaveBeenCalledWith('icon-house')
    expect(adoptAsset).toHaveBeenCalledWith('icon-wrench')
    expect(w.emitted('picked')![0]).toEqual([['http://cdn/a.png', 'http://cdn/b.png']])
  })

  it('多选超限：第 limit+1 次点选被拦截并提示', async () => {
    const w = mount(AssetPicker, { props: { modelValue: true, multiple: true, limit: 1 }, ...mountOpts })
    await vi.waitFor(() => expect(listLibrary).toHaveBeenCalled())
    await nextTick()
    await nextTick()
    await w.findAll('.asset-card')[0].trigger('click')
    await w.findAll('.asset-card')[1].trigger('click')
    await nextTick()
    expect(w.findAll('.asset-card.selected')).toHaveLength(1)
    expect(ElMessage.warning).toHaveBeenCalledWith('最多选择 1 个')
  })

  it('搜索关键字按名称/标签过滤', async () => {
    const w = mount(AssetPicker, { props: { modelValue: true }, ...mountOpts })
    await vi.waitFor(() => expect(listLibrary).toHaveBeenCalled())
    await nextTick()
    await nextTick()
    expect(w.findAll('.asset-card')).toHaveLength(4)
    await w.find('.el-input').setValue('维修')
    await nextTick()
    expect(w.findAll('.asset-card')).toHaveLength(1)
    expect(w.text()).toContain('维修服务')
    // 标签也可命中（home 是 icon-house 的标签）
    await w.find('.el-input').setValue('home')
    await nextTick()
    expect(w.findAll('.asset-card')).toHaveLength(1)
    expect(w.text()).toContain('居住住宅')
  })
})
