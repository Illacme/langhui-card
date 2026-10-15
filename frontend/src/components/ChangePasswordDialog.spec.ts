import { describe, it, expect, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick, defineComponent } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import { createRouter, createMemoryHistory } from 'vue-router'
import ChangePasswordDialog from './ChangePasswordDialog.vue'

// 组件 setup 内使用了 useAuthStore() 与 useRouter()（改密成功后登出回登录），
// 故挂载前需提供 Pinia 与一个内存 Router。
const router = createRouter({ history: createMemoryHistory(), routes: [] })

// 轻量 stub：仅渲染插槽，避免引入完整 Element Plus（jsdom 下过重易 OOM）。
const ElDialog = defineComponent({ name: 'ElDialog', template: `<div class="el-dialog"><slot /><slot name="footer" /></div>` })
const ElForm = defineComponent({ name: 'ElForm', template: `<form class="el-form"><slot /></form>` })
const ElFormItem = defineComponent({ name: 'ElFormItem', template: `<div class="el-form-item"><slot /></div>` })
const ElInput = defineComponent({
  name: 'ElInput',
  props: ['type'],
  template: `<input class="el-input" :type="type || 'text'" />`,
})
const ElButton = defineComponent({ name: 'ElButton', template: `<button class="el-button"><slot /></button>` })

const mountOpts = {
  global: {
    plugins: [router],
    stubs: {
      'el-dialog': ElDialog,
      'el-form': ElForm,
      'el-form-item': ElFormItem,
      'el-input': ElInput,
      'el-button': ElButton,
    },
  },
}

describe('ChangePasswordDialog', () => {
  beforeEach(() => setActivePinia(createPinia()))

  it('modelValue=true 时显示三个密码输入框', async () => {
    const w = mount(ChangePasswordDialog, { props: { modelValue: true }, ...mountOpts })
    await nextTick()
    expect(w.findAll('input[type="password"]').length).toBe(3)
  })

  it('点击取消关闭弹窗（emit update:modelValue=false）', async () => {
    const w = mount(ChangePasswordDialog, { props: { modelValue: true }, ...mountOpts })
    await nextTick()
    const cancel = w.findAll('button').find((b) => b.text() === '取消')
    expect(cancel).toBeTruthy()
    await cancel!.trigger('click')
    const ev = w.emitted('update:modelValue')
    expect(ev).toBeTruthy()
    expect(ev![ev!.length - 1]).toEqual([false])
  })
})
