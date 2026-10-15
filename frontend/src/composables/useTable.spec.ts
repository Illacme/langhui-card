import { describe, it, expect, afterEach, vi } from 'vitest'
import { ref, nextTick } from 'vue'
import { useTable } from '@/composables/useTable'

// useTable 是纯前端分页 + 关键字筛选逻辑（见 useTable.ts 注释），不依赖后端，
// 直接驱动 Vue 响应式即可测试。
const rows = ref([
  { name: 'Alice', dept: 'Sales' },
  { name: 'Bob', dept: 'Eng' },
  { name: 'Carol', dept: 'sales' },
  { name: 'Dave', dept: 'Eng' },
  { name: 'Eve', dept: 'eng' },
])

afterEach(() => {
  vi.restoreAllMocks()
})

describe('useTable 分页与筛选', () => {
  it('按 pageSize 切片，total 等于总行数', () => {
    const t = useTable(rows, undefined, 2)
    expect(t.total.value).toBe(5)
    expect(t.paged.value.map((r) => r.name)).toEqual(['Alice', 'Bob'])
    t.currentPage.value = 2
    expect(t.paged.value.map((r) => r.name)).toEqual(['Carol', 'Dave'])
    t.currentPage.value = 3
    expect(t.paged.value.map((r) => r.name)).toEqual(['Eve'])
  })

  it('关键字筛选大小写不敏感（命中 name 或 dept）', () => {
    const t = useTable(
      rows,
      (r, kw) => r.name.toLowerCase().includes(kw) || r.dept.toLowerCase().includes(kw)
    )
    t.keyword.value = 'SALES'
    expect(t.total.value).toBe(2) // Alice(Sales) + Carol(sales)
    expect(t.paged.value.map((r) => r.name)).toEqual(['Alice', 'Carol'])
  })

  it('关键字变化后回到第一页，避免停留在越界页', async () => {
    const t = useTable(rows, (r, kw) => r.name.toLowerCase().includes(kw), 2)
    t.currentPage.value = 3 // 越界页
    t.keyword.value = 'e' // 命中 Alice、Dave、Eve
    await nextTick()
    expect(t.currentPage.value).toBe(1)
    expect(t.total.value).toBe(3)
    expect(t.paged.value.map((r) => r.name)).toEqual(['Alice', 'Dave'])
  })

  it('纯空白关键字视为「不过滤」', () => {
    const t = useTable(rows, (r, kw) => r.name.includes(kw))
    t.keyword.value = '   '
    expect(t.total.value).toBe(5)
  })
})
