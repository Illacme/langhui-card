import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick, defineComponent, h, provide, inject } from 'vue'
import type { StatDetailItem } from '@/api/stats'
import StatDetail from './StatDetail.vue'

// 明细抽屉只依赖 api/stats 与 Element Plus 的抽屉/选择器/标签/分页/表格，全部桩掉。
const mocks = vi.hoisted(() => ({
  adminStatsDetail: vi.fn(
    async (): Promise<{ list: StatDetailItem[]; total: number; page: number; size: number }> => ({
      list: [],
      total: 0,
      page: 1,
      size: 20,
    })
  ),
}))
vi.mock('@/api/stats', () => mocks)
const { adminStatsDetail } = mocks

const ElDrawer = defineComponent({
  name: 'ElDrawer',
  props: ['modelValue', 'title', 'size'],
  emits: ['update:modelValue'],
  template: `<div class="el-drawer" v-if="modelValue"><slot /></div>`,
})
const ElSelect = defineComponent({
  name: 'ElSelect',
  props: ['modelValue', 'placeholder', 'clearable'],
  emits: ['change'],
  template: `<div class="el-select" />`,
})
const ElOption = defineComponent({
  name: 'ElOption',
  props: ['label', 'value'],
  template: `<div class="el-option" />`,
})
const ElTag = defineComponent({
  name: 'ElTag',
  props: ['type', 'size'],
  template: `<span class="el-tag"><slot /></span>`,
})
const ElPagination = defineComponent({
  name: 'ElPagination',
  props: ['currentPage', 'pageSize', 'total', 'pageSizes', 'layout', 'background'],
  emits: ['update:currentPage', 'update:pageSize', 'current-change', 'size-change'],
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
  props: ['prop', 'label', 'width', 'align'],
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
      'el-drawer': ElDrawer,
      'el-select': ElSelect,
      'el-option': ElOption,
      'el-tag': ElTag,
      'el-pagination': ElPagination,
      'el-table': ElTable,
      'el-table-column': ElTableColumn,
    },
    directives: { loading: {} },
  },
}

function mountDetail(props: Record<string, unknown> = {}) {
  return mount(StatDetail, {
    props: { open: true, range: {}, staffs: [], ...props },
    ...mountOpts,
  })
}

// 等挂载后 watch(immediate) → load() 的微任务链走完
async function flush(w: ReturnType<typeof mount>) {
  await nextTick()
  await nextTick()
  await nextTick()
}

describe('StatDetail', () => {
  beforeEach(() => {
    vi.mocked(adminStatsDetail).mockClear()
    vi.mocked(adminStatsDetail).mockResolvedValue({ list: [], total: 0, page: 1, size: 20 })
  })

  it('打开即查询：日期范围与初始员工一起下传给接口', async () => {
    const w = mountDetail({
      range: { from: '2026-10-01', to: '2026-10-07' },
      staffs: [{ staffId: 1, name: '张三' }],
      staffId: 1,
    })
    await flush(w)

    expect(vi.mocked(adminStatsDetail)).toHaveBeenCalledTimes(1)
    expect(vi.mocked(adminStatsDetail)).toHaveBeenCalledWith({
      from: '2026-10-01',
      to: '2026-10-07',
      staffId: 1,
      eventType: undefined,
      page: 1,
      size: 20,
    })
    // 范围文案：两端都有值时显示「起 至 止」
    expect(w.text()).toContain('2026-10-01 至 2026-10-07')
  })

  it('关闭状态不查询（抽屉常驻 DOM，查询只发生在打开时）', async () => {
    const w = mountDetail({ open: false })
    await flush(w)
    expect(vi.mocked(adminStatsDetail)).not.toHaveBeenCalled()
  })

  it('切换员工筛选：带 staffId 重查，且页码回 1', async () => {
    const w = mountDetail({
      staffs: [
        { staffId: 1, name: '张三' },
        { staffId: 2, name: '李四' },
      ],
    })
    await flush(w)

    // 先翻到第 2 页，再切员工——必须回到第 1 页，
    // 否则「第 2 页的张三数据」会变成「第 2 页的李四数据」（大概率空白）
    await w.findComponent(ElPagination).vm.$emit('current-change', 2)
    await flush(w)
    expect(vi.mocked(adminStatsDetail)).toHaveBeenLastCalledWith(
      expect.objectContaining({ page: 2 })
    )

    // el-select 的真实契约：用户选择时发 change（不是 update:modelValue）
    await w.findAllComponents(ElSelect)[0].vm.$emit('change', 2)
    await flush(w)
    expect(vi.mocked(adminStatsDetail)).toHaveBeenLastCalledWith(
      expect.objectContaining({ staffId: 2, page: 1 })
    )
  })

  it('切换事件类型筛选：带 eventType 重查', async () => {
    const w = mountDetail()
    await flush(w)

    await w.findAllComponents(ElSelect)[1].vm.$emit('change', 'call')
    await flush(w)
    expect(vi.mocked(adminStatsDetail)).toHaveBeenLastCalledWith(
      expect.objectContaining({ eventType: 'call' })
    )
  })

  it('清空筛选发 change 空值：按不过滤处理且不传 undefined 字符串', async () => {
    const w = mountDetail()
    await flush(w)

    // el-select 清空（×）时 change 载荷是 '' / undefined
    await w.findAllComponents(ElSelect)[0].vm.$emit('change', '')
    await flush(w)
    expect(vi.mocked(adminStatsDetail)).toHaveBeenLastCalledWith(
      expect.objectContaining({ staffId: undefined })
    )
  })

  it('翻页与改每页条数各自带正确的 page/size', async () => {
    const w = mountDetail()
    await flush(w)

    await w.findComponent(ElPagination).vm.$emit('current-change', 3)
    await flush(w)
    expect(vi.mocked(adminStatsDetail)).toHaveBeenLastCalledWith(
      expect.objectContaining({ page: 3 })
    )

    await w.findComponent(ElPagination).vm.$emit('size-change', 50)
    await flush(w)
    expect(vi.mocked(adminStatsDetail)).toHaveBeenLastCalledWith(
      expect.objectContaining({ size: 50, page: 1 })
    )
  })

  it('回包页码被后端夹到末页时，分页栏同步（不停在第 5 页显示第 1 页内容）', async () => {
    const w = mountDetail()
    await flush(w)

    vi.mocked(adminStatsDetail).mockResolvedValueOnce({ list: [], total: 3, page: 1, size: 20 })
    await w.findComponent(ElPagination).vm.$emit('current-change', 5)
    await flush(w)

    expect(w.findComponent(ElPagination).props('currentPage')).toBe(1)
  })

  it('正常渲染明细行：时间 / 编号 / 姓名 / 事件标签', async () => {
    vi.mocked(adminStatsDetail).mockResolvedValueOnce({
      list: [
        {
          id: 7,
          staffId: 1,
          name: '张三',
          eventType: 'visit',
          statDate: '2026-10-22',
          createdAt: '2026-10-22 14:30:00',
        },
        {
          id: 8,
          staffId: 2,
          name: '李四',
          eventType: 'call',
          statDate: '2026-10-22',
          createdAt: '2026-10-22 15:05:12',
        },
      ],
      total: 2,
      page: 1,
      size: 20,
    })
    const w = mountDetail()
    await flush(w)

    expect(w.text()).toContain('2026-10-22 14:30:00')
    expect(w.text()).toContain('张三')
    expect(w.text()).toContain('访问')
    expect(w.text()).toContain('李四')
    expect(w.text()).toContain('拨号')
  })

  it('空结果显示空态文案（不当成加载失败）', async () => {
    const w = mountDetail()
    await flush(w)
    expect(w.text()).toContain('当前条件下暂无埋点记录')
  })

  it('只有单边日期时范围文案给出明确口径（不显示 undefined）', async () => {
    const w = mountDetail({ range: { from: '2026-10-01' } })
    await flush(w)
    expect(w.text()).toContain('2026-10-01 至 今天')
    expect(w.text()).not.toContain('undefined')
  })
})
