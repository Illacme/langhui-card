import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick, defineComponent, h, provide, inject } from 'vue'
import Stats from './Stats.vue'

// 名片统计页依赖 api/stats（汇总 + 按员工）与 element-plus 的消息/确认框，全部 mock 掉。
// 表格复用 Employees.spec.ts 那套「行上下文」轻量 stub，避免 jsdom 下挂完整 Element Plus。
const mocks = vi.hoisted(() => ({
  adminStats: vi.fn(async () => ({
    summary: { visit: 12, call: 3, save: 5 },
    byStaff: [
      { staffId: 1, name: '张三', title: '工程师', visit: 7, call: 2, save: 3 },
      { staffId: 2, name: '李四', title: '销售', visit: 5, call: 1, save: 2 },
    ],
  })),
}))
const { adminStats } = mocks
vi.mock('@/api/stats', () => mocks)

const ElCard = defineComponent({ name: 'ElCard', template: `<div class="el-card"><slot /></div>` })
const ElInput = defineComponent({
  name: 'ElInput',
  props: ['modelValue', 'placeholder', 'clearable'],
  emits: ['update:modelValue'],
  template: `<input class="el-input" :placeholder="placeholder" :value="modelValue"
    @input="$emit('update:modelValue', $event.target.value)" />`,
})
const ElButton = defineComponent({
  name: 'ElButton',
  props: ['type', 'loading', 'link'],
  template: `<button class="el-button"><slot /></button>`,
})
const ElPagination = defineComponent({
  name: 'ElPagination',
  props: ['currentPage', 'pageSize', 'total', 'pageSizes', 'layout', 'background'],
  template: `<div class="el-pagination" />`,
})

// 行上下文：ElTable 为每行包一层 TableRow 并 provide 行数据，ElTableColumn 注入后转发给 #default 槽
const TableRow = defineComponent({
  name: 'TableRow',
  props: ['row'],
  setup(props, { slots }) {
    provide('tableRow', props.row)
    return () => h('div', { class: 'el-table-row' }, slots.default ? slots.default() : [])
  },
})
const ElTable = defineComponent({
  name: 'ElTable',
  props: ['data', 'border', 'stripe'],
  setup(props, { slots }) {
    return () =>
      h(
        'div',
        { class: 'el-table' },
        (props.data || []).map((row: any, i: number) =>
          h(TableRow, { key: i, row }, () => (slots.default ? slots.default() : null))
        )
      )
  },
})
const ElTableColumn = defineComponent({
  name: 'ElTableColumn',
  props: ['prop', 'label', 'width', 'align', 'sortable'],
  setup(props, { slots }) {
    const row = inject('tableRow', null) as any
    return () => {
      const content = slots.default
        ? slots.default({ row })
        : props.prop && row
          ? String(row[props.prop] ?? '')
          : ''
      return h('div', { class: 'el-table-column' }, content)
    }
  },
})

const mountOpts = {
  global: {
    stubs: {
      'el-card': ElCard,
      'el-input': ElInput,
      'el-button': ElButton,
      'el-table': ElTable,
      'el-table-column': ElTableColumn,
      'el-pagination': ElPagination,
    },
    directives: { loading: {} },
  },
}

// 等挂载后 onMounted → load() 的微任务链走完
async function flushLoad(w: ReturnType<typeof mount>) {
  await nextTick()
  await nextTick()
  await nextTick()
}

describe('Stats', () => {
  beforeEach(() => {
    vi.mocked(adminStats).mockClear()
  })

  it('挂载即加载：汇总三卡 + 按员工表格都渲染', async () => {
    const w = mount(Stats, mountOpts)
    await flushLoad(w)

    expect(vi.mocked(adminStats)).toHaveBeenCalledTimes(1)
    // 汇总
    expect(w.text()).toContain('12') // 总访问
    expect(w.text()).toContain('3') // 拨号点击
    expect(w.text()).toContain('5') // 保存点击
    // 按员工明细（编号 / 姓名 / 三个计数）
    expect(w.text()).toContain('张三')
    expect(w.text()).toContain('李四')
    expect(w.text()).toContain('7')
  })

  it('关键字按姓名 / 职位过滤表格数据', async () => {
    const w = mount(Stats, mountOpts)
    await flushLoad(w)

    await w.find('input.el-input').setValue('销售')
    await nextTick()
    expect(w.findComponent(ElTable).props('data')).toHaveLength(1)
    expect(w.findComponent(ElTable).props('data')[0].name).toBe('李四')
    // 汇总卡片不受筛选影响（汇总来自接口，不是前端累加当前页）
    expect(w.text()).toContain('12')
  })

  it('列定义含编号/姓名/职位/访问/拨号/保存六个统计列', async () => {
    const w = mount(Stats, mountOpts)
    await flushLoad(w)

    // 注意：stub 的 ElTable 会为每一行各渲染一次列，故先去重再比对顺序
    const labels = [
      ...new Set(w.findAllComponents(ElTableColumn).map((c) => c.props('label'))),
    ]
    expect(labels).toEqual(['编号', '姓名', '职位', '访问', '拨号', '保存'])
  })
})
