import { ref, nextTick, onMounted } from 'vue'

/**
 * 列表顺序调整：拖拽 + 上下箭头（项目案例 / 业绩分类 / 业务服务共用）。
 *
 * 设计要点：
 * - rows 为全量数据（后端按 sort 升序返回），重排直接改写该数组并重算 sort，
 *   保证「所见顺序」与「落库顺序」一致，避免筛选/分页下的中间态错乱。
 * - paged 为当前页展示数据，仅用于把拖拽落点 <tr> 的序号映射回行对象。
 * - 拖拽事件用事件委托挂在 el-table 根节点上（表格行会随数据重建，
 *   逐行绑定会在重排后失效）。
 *
 * @param rows    全量有序数据
 * @param paged   当前页数据（只读使用）
 * @param persist 批量持久化回调，接收 [{ id, sort }]
 */
export function useSortableRows(
  rows: { value: any[] },
  paged: { value: any[] },
  persist: (items: { id: number; sort: number }[]) => Promise<unknown>
) {
  const tableRef = ref<any>()
  let dragId: number | null = null

  function globalIndex(row: any) {
    return rows.value.findIndex((r) => r.id === row.id)
  }
  function isFirstRow(row: any) {
    return globalIndex(row) <= 0
  }
  function isLastRow(row: any) {
    const i = globalIndex(row)
    return i < 0 || i === rows.value.length - 1
  }

  // 重排后按新顺序重算 sort 并一次性批量持久化
  async function applyAndPersist() {
    rows.value.forEach((r, i) => (r.sort = i + 1))
    try {
      await persist(rows.value.map((r) => ({ id: r.id, sort: r.sort })))
    } catch {
      /* 拦截器已提示 */
    }
  }

  async function moveUp(row: any) {
    const i = globalIndex(row)
    if (i <= 0) return
    const arr = [...rows.value]
    ;[arr[i - 1], arr[i]] = [arr[i], arr[i - 1]]
    rows.value = arr
    await applyAndPersist()
  }

  async function moveDown(row: any) {
    const i = globalIndex(row)
    if (i < 0 || i >= rows.value.length - 1) return
    const arr = [...rows.value]
    ;[arr[i + 1], arr[i]] = [arr[i], arr[i + 1]]
    rows.value = arr
    await applyAndPersist()
  }

  function trToRow(tr: Element | null) {
    if (!tr || !tr.closest('.el-table__body-wrapper')) return null
    const tbody = tr.parentElement
    if (!tbody) return null
    const idx = [...tbody.children].indexOf(tr)
    return idx >= 0 ? paged.value[idx] : null
  }

  function onDragStart(e: Event) {
    const de = e as DragEvent
    const tr = (de.target as HTMLElement)?.closest?.('tr')
    const row = trToRow(tr)
    if (!row) return
    dragId = row.id
    de.dataTransfer?.setData('text/plain', String(row.id))
    if (de.dataTransfer) de.dataTransfer.effectAllowed = 'move'
  }

  function onDragOver(e: Event) {
    const de = e as DragEvent
    if (!(de.target as HTMLElement)?.closest?.('.el-table__body-wrapper')) return
    de.preventDefault()
    if (de.dataTransfer) de.dataTransfer.dropEffect = 'move'
  }

  async function onDrop(e: Event) {
    const de = e as DragEvent
    de.preventDefault()
    const tr = (de.target as HTMLElement)?.closest?.('tr')
    const target = trToRow(tr)
    if (!target || dragId == null || target.id === dragId) {
      dragId = null
      return
    }
    const from = rows.value.findIndex((r) => r.id === dragId)
    const to = rows.value.findIndex((r) => r.id === target.id)
    dragId = null
    if (from < 0 || to < 0) return
    const arr = [...rows.value]
    const [moved] = arr.splice(from, 1)
    arr.splice(to, 0, moved)
    rows.value = arr
    await applyAndPersist()
  }

  function onDragEnd() {
    dragId = null
  }

  onMounted(() => {
    nextTick(() => {
      const root = tableRef.value?.$el
      if (!root) return
      root.addEventListener('dragstart', onDragStart)
      root.addEventListener('dragover', onDragOver)
      root.addEventListener('drop', onDrop)
      root.addEventListener('dragend', onDragEnd)
    })
  })

  return { tableRef, isFirstRow, isLastRow, moveUp, moveDown }
}
