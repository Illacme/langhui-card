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
        // stub 要渲染 placeholder：示例占位符是「填什么格式」的引导，断言它就得看得见
        'el-input': { template: '<input type="text" :placeholder="placeholder" />', props: ['placeholder'] },
        'el-button': { template: '<button><slot /></button>' },
        'el-tag': { template: '<span><slot /></span>' },
        // tooltip 拍平：触发徽标 + 说明文本一起渲染，便于断言说明仍在 DOM
        'el-tooltip': {
          template: '<span class="el-tooltip"><slot /><span class="el-tooltip__content">{{ content }}</span></span>',
          props: ['content', 'placement'],
        },
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

  it('格式易错项带悬浮说明徽标，普通项不带', () => {
    // 地址类项填错后果当场发现不了（图片打不开 / 小程序连不上），必须有说明徽标
    const w = mountRow({ key: 'upload_base_url', type: 'string' })
    expect(w.find('.row-hint').exists()).toBe(true)
    expect(w.text()).toContain('图片全部打不开')
    // 无提示的项不挂徽标：徽标一多就等于没有
    const plain = mountRow({ key: 'brand_name', type: 'string' })
    expect(plain.find('.row-hint').exists()).toBe(false)
  })

  it('格式关键项给示例占位，其余保持未设置', () => {
    const w = mountRow({ key: 'miniprogram_base_url', type: 'string' })
    expect(w.find('.row-control input').attributes('placeholder')).toContain('https://')
    const plain = mountRow({ key: 'brand_name', type: 'string' })
    expect(plain.find('.row-control input').attributes('placeholder')).toBe('未设置')
  })

  it('行内校验错误：传了才显示，不传不占版面', () => {
    // 配置向导在「下一步」前校验地址类项，错误显示在行内（主列表不传，靠后端拦）
    const w = mountRow({ key: 'upload_base_url', type: 'string' }, 'cdn.example.com')
    expect(w.find('.row-error').exists()).toBe(false)
    const w2 = mount(SettingRow, {
      props: {
        item: { ...base, key: 'upload_base_url', type: 'string' },
        modelValue: 'cdn.example.com',
        error: '不是完整地址，应类似 https://cdn.example.com',
      },
      global: {
        stubs: {
          'el-select': { template: '<select><slot /></select>' },
          'el-option': { template: '<option />' },
          'el-input-number': { template: '<input type="number" />' },
          'el-input': { template: '<input type="text" :placeholder="placeholder" />', props: ['placeholder'] },
          'el-button': { template: '<button><slot /></button>' },
          'el-tag': { template: '<span><slot /></span>' },
          'el-tooltip': {
            template: '<span class="el-tooltip"><slot /><span class="el-tooltip__content">{{ content }}</span></span>',
            props: ['content', 'placement'],
          },
        },
      },
    })
    expect(w2.find('.row-error').text()).toContain('不是完整地址')
  })
})
