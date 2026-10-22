import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import SettingRow from './SettingRow.vue'

// 守门测试：SettingRow 的控件分支是一条 v-if / v-else-if 链
// （bool → int → 可写敏感 → 其余文本）。历史上把「可写敏感」从 v-else-if
// 误写成 v-if，导致开关/整数行额外多渲染一个文本框（界面出现「60 / 60」两个框）。
// 这里对每种类型断言：控制区只出现一个输入类控件，防止分支链再次断裂。

const base = {
  key: 'k',
  name: '配置项',
  key_type: '',
  type: 'string',
  description: '',
  value: '',
  configured: false,
  restart: false,
  readOnly: false,
  secret: false,
} as any

function mountRow(item: Record<string, unknown>, modelValue = '') {
  return mount(SettingRow, {
    props: { item: { ...base, ...item }, modelValue },
    global: {
      stubs: {
        'el-select': { template: '<select><slot /></select>' },
        'el-option': { template: '<option />' },
        'el-input-number': { template: '<input type="number" />' },
        'el-input': { template: '<input type="text" />' },
        'el-button': { template: '<button><slot /></button>' },
        'el-tag': { template: '<span><slot /></span>' },
      },
    },
  })
}

describe('SettingRow 控件唯一性（防 v-if 链断裂回归）', () => {
  it('bool 行只渲染一个控件（三态下拉）', () => {
    const w = mountRow({ type: 'bool' })
    expect(w.find('.row-control').findAll('select, input')).toHaveLength(1)
  })

  it('int 行只渲染一个控件（数字框）', () => {
    const w = mountRow({ type: 'int' }, '60')
    expect(w.find('.row-control').findAll('select, input')).toHaveLength(1)
  })

  it('string 行只渲染一个控件（文本框）', () => {
    const w = mountRow({ type: 'string' }, 'x')
    expect(w.find('.row-control').findAll('select, input')).toHaveLength(1)
  })

  it('可写敏感行：一个输入框 + 清除按钮，但仍只有一个输入控件', () => {
    const w = mountRow({ type: 'string', secret: true, configured: true }, 'x')
    expect(w.find('.row-control').findAll('input')).toHaveLength(1)
  })

  it('只读敏感行：走普通只读框显示预览，也只有一个输入控件', () => {
    const w = mountRow({ type: 'string', secret: true, readOnly: true }, 'preview')
    expect(w.find('.row-control').findAll('input')).toHaveLength(1)
  })
})
