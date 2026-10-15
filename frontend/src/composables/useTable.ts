import { ref, computed, watch, type Ref } from 'vue'

/**
 * 前端分页 + 关键字搜索组合式函数。
 * 适用于本后台「数据量小、运营型」的定位（见《三端联调与接口总表》§2.3）：
 * 列表已在内存中，直接做关键字筛选 + 切片分页，无需改动后端接口。
 *
 * @param source   完整数据源（通常为接口返回的原始数组 ref）
 * @param filterFn 关键字匹配谓词；不传则仅做分页
 * @param pageSize 每页条数
 */
export function useTable(
  source: Ref<any[]>,
  filterFn?: (row: any, kw: string) => boolean,
  pageSize = 10
) {
  const keyword = ref('')
  const currentPage = ref(1)
  const size = ref(pageSize)

  const filtered = computed(() => {
    const kw = keyword.value.trim().toLowerCase()
    if (!kw || !filterFn) return source.value
    return source.value.filter((r) => filterFn(r, kw))
  })

  const total = computed(() => filtered.value.length)

  const paged = computed(() => {
    const start = (currentPage.value - 1) * size.value
    return filtered.value.slice(start, start + size.value)
  })

  // 关键字变化后回到第一页，避免停留在越界页
  watch(keyword, () => {
    currentPage.value = 1
  })

  return { keyword, currentPage, size, total, paged }
}
