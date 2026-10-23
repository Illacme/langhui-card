<template>
  <el-drawer
    :model-value="open"
    title="数据明细"
    size="720px"
    @update:model-value="$emit('update:open', $event)"
  >
    <div class="detail-bar">
      <el-select
        :model-value="staffFilter"
        placeholder="全部员工"
        clearable
        style="width: 200px"
        @change="onStaffChange"
      >
        <el-option
          v-for="s in staffs"
          :key="s.staffId"
          :label="`${s.name}（${s.staffId}）`"
          :value="s.staffId"
        />
      </el-select>
      <el-select
        :model-value="typeFilter"
        placeholder="全部事件"
        clearable
        style="width: 150px"
        @change="onTypeChange"
      >
        <el-option v-for="t in eventTypes" :key="t.value" :label="t.label" :value="t.value" />
      </el-select>
      <!-- 日期范围继承主页选择器（改范围请在上方调）：明细与汇总是同一个口径，
           两边各选各的日期就会出现「汇总 100 条、明细列出 105 条」的对账事故 -->
      <span class="detail-range">日期范围：{{ rangeLabel }}</span>
    </div>

    <el-table :data="list" v-loading="loading" border stripe>
      <el-table-column prop="createdAt" label="时间" width="180" />
      <el-table-column prop="staffId" label="编号" width="80" align="center" />
      <el-table-column prop="name" label="姓名" width="120" />
      <el-table-column label="事件" min-width="90">
        <template #default="{ row }">
          <el-tag :type="tagType(row.eventType)" size="small">{{ eventLabel(row.eventType) }}</el-tag>
        </template>
      </el-table-column>
    </el-table>
    <p v-if="!loading && !list.length" class="detail-empty">当前条件下暂无埋点记录</p>

    <el-pagination
      class="table-pager"
      v-model:current-page="page"
      v-model:page-size="size"
      :total="total"
      :page-sizes="[10, 20, 50]"
      layout="total, sizes, prev, pager, next"
      background
      @current-change="onPageChange"
      @size-change="onSizeChange"
    />
  </el-drawer>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { adminStatsDetail, type StatDetailItem, type StatRange } from '@/api/stats'

const props = defineProps<{
  open: boolean
  range: StatRange
  staffs: { staffId: number; name: string }[]
  /** 初始员工筛选：点员工行的「明细」时由父组件给定；0/缺省 = 全部 */
  staffId?: number
}>()
const emit = defineEmits<{ (e: 'update:open', v: boolean): void }>()

const list = ref<StatDetailItem[]>([])
const total = ref(0)
const page = ref(1)
const size = ref(20)
const loading = ref(false)
const staffFilter = ref(0)
const typeFilter = ref('')

// 事件类型枚举：标签与汇总页三卡同源，避免同一个 visit 在两处叫两个名字
const eventTypes = [
  { value: 'visit', label: '访问' },
  { value: 'call', label: '拨号' },
  { value: 'save', label: '保存' },
]
function eventLabel(t: string): string {
  return eventTypes.find((e) => e.value === t)?.label ?? t
}
// 配色与趋势图三系列对齐（访问蓝 / 拨号绿 / 保存橙），同一指标全站一个色
function tagType(t: string): 'success' | 'warning' | undefined {
  if (t === 'call') return 'success'
  if (t === 'save') return 'warning'
  return undefined
}

const rangeLabel = computed(() =>
  props.range.from || props.range.to
    ? `${props.range.from || '上线起'} 至 ${props.range.to || '今天'}`
    : '全部时间'
)

async function load() {
  loading.value = true
  try {
    const r = await adminStatsDetail({
      from: props.range.from,
      to: props.range.to,
      staffId: staffFilter.value || undefined,
      eventType: typeFilter.value || undefined,
      page: page.value,
      size: size.value,
    })
    list.value = r.list
    total.value = r.total
    // 越界页码（如翻到第 5 页后数据被删）会被后端夹到末页，界面同步过来，
    // 否则分页栏显示第 5 页而表格是第 1 页的内容
    page.value = r.page
  } catch {
    /* 拦截器已提示 */
  } finally {
    loading.value = false
  }
}

// 打开 / 换员工 / 改日期范围 → 重查。
// 用多源 watch（数组里每个都是**原始值** getter，元素级比较）而不是
// watch(() => [a, b])：后者每次求值都返回新数组，父组件重渲染就会被误触发。
// 「趋势图点某天」会同时改范围与打开状态——多源 watch 只回调一次，不会双请求。
// immediate：抽屉可能带着 open=true 直接挂载（复用/测试场景），也要查一次。
watch(
  [
    () => props.open,
    () => props.staffId || 0,
    () => props.range.from || '',
    () => props.range.to || '',
  ],
  ([open, staff]) => {
    if (!open) return
    staffFilter.value = staff as number
    typeFilter.value = ''
    page.value = 1
    void load()
  },
  { immediate: true }
)

function onPageChange(p: number) {
  page.value = p
  void load()
}

function onSizeChange(s: number) {
  size.value = s
  page.value = 1
  void load()
}

// 员工 / 事件筛选：刻意不走 v-model + watch。多源 watch 在打开抽屉时会
// **程序性**回填筛选值（staffFilter.value = staff），若再挂一个 watch 监听
// staffFilter，每次打开都会多跑一次重复查询。统一由 change 事件驱动——
// Element Plus 只在**用户操作**时发 change，程序性赋值不发，天然区分两种来源。
// 改筛选一律页码回 1：否则会停在「第 2 页的张三」上看李四（大概率空白页）。
function onStaffChange(v: number) {
  staffFilter.value = v || 0
  page.value = 1
  void load()
}

function onTypeChange(v: string) {
  typeFilter.value = v || ''
  page.value = 1
  void load()
}
</script>

<style scoped>
.detail-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin-bottom: 12px;
}
.detail-range {
  margin-left: auto;
  color: #909399;
  font-size: 12px;
}
.detail-empty {
  margin: 12px 0 0;
  padding: 24px 0;
  text-align: center;
  color: #909399;
  font-size: 13px;
  background: #fafcff;
  border: 1px dashed #ebeef5;
  border-radius: 6px;
}
</style>
