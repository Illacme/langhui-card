import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick, defineComponent, h, provide, inject } from 'vue'
import Employees from './Employees.vue'

// 员工管理页依赖 api/employees（列表 / 增 / 停启用 / 重置密码）与 element-plus 的消息 / 确认框，
// 全部 mock 掉网络与 UI 反馈层；useTable（分页 + 搜索）用真实实现，顺带覆盖其与视图的联动。
// 注意：vi.mock 工厂会被提升到文件顶部，里面引用的 mock 必须经 vi.hoisted 预先创建，
// 否则访问 const 会撞 TDZ（Cannot access before initialization）。
const mocks = vi.hoisted(() => ({
  listEmployees: vi.fn(async () => [
    { staffId: 1, name: '张三', title: '工程师', phone: '13800000001', email: 'z@a.com', avatar: '', status: 'enabled', createdAt: '2026-01-01 10:00:00' },
    { staffId: 2, name: '李四', title: '销售', phone: '13900000002', email: 'l@a.com', avatar: '', status: 'disabled', createdAt: '2026-01-02 10:00:00' },
    { staffId: 3, name: '王五', title: '经理', phone: '13700000003', email: 'w@a.com', avatar: '', status: 'enabled', createdAt: '2026-01-03 10:00:00' },
  ]),
  createEmployee: vi.fn(async () => ({ staffId: 99, initialPassword: 'init123' })),
  disableEmployee: vi.fn(async () => ({ code: 0 })),
  enableEmployee: vi.fn(async () => ({ code: 0 })),
  resetPassword: vi.fn(async () => 'reset123'),
}))
const { listEmployees, createEmployee, disableEmployee, enableEmployee, resetPassword } = mocks
vi.mock('@/api/employees', () => mocks)
vi.mock('element-plus', () => ({
  ElMessage: { success: vi.fn(), error: vi.fn(), warning: vi.fn() },
  ElMessageBox: { confirm: vi.fn(async () => 'confirm') },
}))
// 图标仅作为 prop 值传递（:prefix-icon），不会真正渲染，桩成普通对象即可
vi.mock('@element-plus/icons-vue', () => ({ Refresh: {}, Plus: {}, Search: {} }))

import { ElMessageBox } from 'element-plus'

// ── 轻量 stub（jsdom 下完整 Element Plus 过重易 OOM） ──
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
const ElForm = defineComponent({
  name: 'ElForm',
  methods: {
    validate: () => Promise.resolve(true),
    clearValidate: () => {},
  },
  template: `<form class="el-form"><slot /></form>`,
})
const ElFormItem = defineComponent({ name: 'ElFormItem', template: `<div class="el-form-item"><slot /></div>` })
const ElImage = defineComponent({ name: 'ElImage', props: ['src', 'fit'], template: `<img class="el-image" />` })
const ElTag = defineComponent({ name: 'ElTag', props: ['type', 'effect'], template: `<span class="el-tag"><slot /></span>` })
const ElDialog = defineComponent({
  name: 'ElDialog',
  props: ['modelValue', 'title', 'width'],
  template: `<div v-if="modelValue" class="el-dialog"><h3>{{ title }}</h3><slot /><slot name="footer" /></div>`,
})
const ElDescriptions = defineComponent({ name: 'ElDescriptions', props: ['column', 'border'], template: `<div class="el-descriptions"><slot /></div>` })
const ElDescriptionsItem = defineComponent({ name: 'ElDescriptionsItem', props: ['label'], template: `<div class="el-descriptions-item"><slot /></div>` })
const ElPagination = defineComponent({
  name: 'ElPagination',
  props: ['currentPage', 'pageSize', 'total', 'pageSizes', 'layout', 'background'],
  emits: ['update:currentPage', 'update:pageSize'],
  template: `<div class="el-pagination" />`,
})

// 表格行上下文：ElTable 为每一行包一层 TableRow 并 provide 行数据，
// ElTableColumn 注入后转发给本列 #default 槽——复刻 Element Plus「列槽拿 { row }」的行为。
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
  props: ['prop', 'label', 'width', 'align', 'fixed', 'minWidth', 'showOverflowTooltip'],
  setup(props, { slots }) {
    const row = inject('tableRow', null) as any
    return () => {
      // 复刻 Element Plus：无作用域槽时按 prop 直接取行字段渲染（如姓名 / 手机列）
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
      'el-form': ElForm,
      'el-form-item': ElFormItem,
      'el-image': ElImage,
      'el-tag': ElTag,
      'el-dialog': ElDialog,
      'el-descriptions': ElDescriptions,
      'el-descriptions-item': ElDescriptionsItem,
      'el-pagination': ElPagination,
      'el-table': ElTable,
      'el-table-column': ElTableColumn,
    },
    // el-table 上的 v-loading 指令在 stub 下无需行为，注册个空指令消除告警
    directives: { loading: {} },
  },
}

function mountEmployees() {
  return mount(Employees, mountOpts)
}

// 等挂载后的 onMounted → load() 的微任务链走完
async function flushLoad(w: ReturnType<typeof mountEmployees>) {
  await nextTick()
  await nextTick()
  await nextTick()
}

describe('Employees', () => {
  beforeEach(() => {
    vi.mocked(listEmployees).mockClear()
    vi.mocked(createEmployee).mockClear()
    vi.mocked(disableEmployee).mockClear()
    vi.mocked(enableEmployee).mockClear()
    vi.mocked(resetPassword).mockClear()
    vi.mocked(ElMessageBox.confirm).mockClear()
    vi.mocked(ElMessageBox.confirm).mockImplementation(async () => 'confirm' as any)
  })

  it('挂载即加载列表：表格数据 / 分页总数 / 行内容正确', async () => {
    const w = mountEmployees()
    await flushLoad(w)

    const table = w.findComponent(ElTable)
    expect(vi.mocked(listEmployees)).toHaveBeenCalledTimes(1)
    expect(table.props('data')).toHaveLength(3)
    expect(w.findComponent(ElPagination).props('total')).toBe(3)
    // 行上下文已注入列槽：姓名 / 状态文案渲染出来
    expect(w.text()).toContain('张三')
    expect(w.text()).toContain('李四')
    expect(w.text()).toContain('已停用') // 李四 status=disabled
    expect(w.text()).toContain('正常') // 张三 / 王五
    // 「创建时间」列经 formatDateTime 渲染：解析与格式化都走本地时区，
    // 往返结果恒定，故此断言在任意时区（含 CI 的 UTC）下都成立。
    expect(w.text()).toContain('2026-01-01 10:00:00')
    expect(w.text()).not.toContain('T10:00') // 未把原始 RFC3339 串直接透出
  })

  it('关键字搜索按姓名 / 手机过滤表格数据', async () => {
    const w = mountEmployees()
    await flushLoad(w)

    await w.find('input.el-input').setValue('137')
    await nextTick()
    expect(w.findComponent(ElTable).props('data')).toHaveLength(1)
    expect(w.findComponent(ElTable).props('data')[0].name).toBe('王五')

    await w.find('input.el-input').setValue('李四')
    await nextTick()
    expect(w.findComponent(ElTable).props('data')).toHaveLength(1)
    expect(w.findComponent(ElTable).props('data')[0].staffId).toBe(2)
  })

  it('停用：确认后调 disableEmployee 并重新加载', async () => {
    const w = mountEmployees()
    await flushLoad(w)

    const disableBtn = w.findAll('button.el-button').find((b) => b.text() === '停用')
    expect(disableBtn).toBeTruthy()
    await disableBtn!.trigger('click')
    await vi.waitFor(() => expect(vi.mocked(ElMessageBox.confirm)).toHaveBeenCalledTimes(1))
    await vi.waitFor(() => expect(vi.mocked(disableEmployee)).toHaveBeenCalledTimes(1))
    expect(vi.mocked(disableEmployee)).toHaveBeenCalledWith(1) // 张三（第一个 enabled 行）
    await vi.waitFor(() => expect(vi.mocked(listEmployees)).toHaveBeenCalledTimes(2)) // 停用后重载
  })

  it('停用被取消时不调用接口', async () => {
    vi.mocked(ElMessageBox.confirm).mockImplementation(async () => Promise.reject(new Error('cancel')))
    const w = mountEmployees()
    await flushLoad(w)

    const disableBtn = w.findAll('button.el-button').find((b) => b.text() === '停用')
    await disableBtn!.trigger('click')
    await vi.waitFor(() => expect(vi.mocked(ElMessageBox.confirm)).toHaveBeenCalledTimes(1))
    await nextTick()
    await nextTick()
    expect(vi.mocked(disableEmployee)).not.toHaveBeenCalled()
  })

  it('启用：对已停用行调 enableEmployee 并重新加载', async () => {
    const w = mountEmployees()
    await flushLoad(w)

    const enableBtn = w.findAll('button.el-button').find((b) => b.text() === '启用')
    expect(enableBtn).toBeTruthy()
    await enableBtn!.trigger('click')
    await vi.waitFor(() => expect(vi.mocked(enableEmployee)).toHaveBeenCalledWith(2)) // 李四
    await vi.waitFor(() => expect(vi.mocked(listEmployees)).toHaveBeenCalledTimes(2))
  })

  it('重置密码：成功后弹窗展示新初始密码', async () => {
    const w = mountEmployees()
    await flushLoad(w)

    const resetBtn = w.findAll('button.el-button').find((b) => b.text() === '重置密码')
    await resetBtn!.trigger('click')
    await vi.waitFor(() => expect(vi.mocked(resetPassword)).toHaveBeenCalledWith(1))
    await vi.waitFor(() => expect(w.text()).toContain('reset123'))
    expect(w.text()).toContain('重置密码成功')
  })

  it('新增员工：校验通过后调 createEmployee、弹初始密码并重载', async () => {
    const w = mountEmployees()
    await flushLoad(w)

    // 打开新增弹窗 → 填表 → 保存
    await w.findAll('button.el-button').find((b) => b.text() === '新增员工')!.trigger('click')
    await nextTick()
    const inputs = w.findAll('.el-dialog input.el-input')
    await inputs[0].setValue('赵六')
    await inputs[1].setValue('13600000004')
    await w.findAll('.el-dialog button.el-button').find((b) => b.text() === '保存')!.trigger('click')

    await vi.waitFor(() => expect(vi.mocked(createEmployee)).toHaveBeenCalledTimes(1))
    expect(vi.mocked(createEmployee)).toHaveBeenCalledWith(
      expect.objectContaining({ name: '赵六', phone: '13600000004' })
    )
    await vi.waitFor(() => expect(w.text()).toContain('init123'))
    await vi.waitFor(() => expect(vi.mocked(listEmployees)).toHaveBeenCalledTimes(2))
  })
})
