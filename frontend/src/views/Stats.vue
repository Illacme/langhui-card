<template>
  <div>
    <el-card>
      <div class="page-bar">
        <h3>名片统计</h3>
        <div class="bar-right">
          <el-input
            v-model="keyword"
            placeholder="搜索姓名/职位"
            clearable
            :prefix-icon="Search"
            style="width: 200px"
          />
          <!-- 日期范围：value-format 直接给 YYYY-MM-DD 字符串，与接口参数同形；
               清空（×）= 全部时间，与后端 from/to 缺省语义一致 -->
          <el-date-picker
            v-model="range"
            type="daterange"
            unlink-panels
            range-separator="至"
            start-placeholder="开始日期"
            end-placeholder="结束日期"
            :shortcuts="rangeShortcuts"
            value-format="YYYY-MM-DD"
            clearable
            style="width: 260px"
          />
          <el-button :icon="Download" @click="onExport">导出</el-button>
          <el-button :icon="Tickets" @click="openDetail()">数据明细</el-button>
          <el-button :icon="Refresh" :loading="loading" @click="load">刷新</el-button>
        </div>
      </div>

      <!-- 汇总三卡：访问 / 拨号 / 保存（口径见《首页组件与接口字段映射》§4：只记点击，不校验真实结果） -->
      <div class="stat-cards">
        <div class="stat-card">
          <div class="stat-label">总访问</div>
          <div class="stat-value">{{ summary.visit }}</div>
        </div>
        <div class="stat-card">
          <div class="stat-label">拨号点击</div>
          <div class="stat-value">{{ summary.call }}</div>
        </div>
        <div class="stat-card">
          <div class="stat-label">保存点击</div>
          <div class="stat-value">{{ summary.save }}</div>
        </div>
      </div>
      <p class="range-tip">统计范围：{{ rangeLabel }}（日期按浏览器本地时区与后端 TZ 一致计算）</p>

      <!-- 按天趋势：只含所选范围内有事件的日期；点某一天可下钻当天明细 -->
      <StatTrend :points="trend" @pick="onTrendPick" />

      <el-table :data="paged" v-loading="loading" border stripe>
        <el-table-column prop="staffId" label="编号" width="80" align="center" />
        <el-table-column prop="name" label="姓名" width="120" />
        <el-table-column prop="title" label="职位" width="120" />
        <el-table-column prop="visit" label="访问" width="110" align="right" sortable />
        <el-table-column prop="call" label="拨号" width="110" align="right" sortable />
        <el-table-column prop="save" label="保存" width="110" align="right" sortable />
        <el-table-column label="操作" width="80" align="center">
          <template #default="{ row }">
            <el-button link type="primary" @click="openDetail(row.staffId)">明细</el-button>
          </template>
        </el-table-column>
      </el-table>
      <p class="tip">
        口径：只记前端点击、不校验真实结果（需求 §6）。想看某员工的名片实际效果，在小程序首页输入其编号即可预览。
      </p>

      <el-pagination
        class="table-pager"
        v-model:current-page="currentPage"
        v-model:page-size="size"
        :total="total"
        :page-sizes="[10, 20, 50]"
        layout="total, sizes, prev, pager, next"
        background
      />
    </el-card>

    <!-- 明细抽屉：员工行「明细」与趋势图点某天都会打开它（分别带上 staffId / 收窄日期） -->
    <StatDetail
      v-model:open="detailOpen"
      :range="rangeParam"
      :staffs="staffOptions"
      :staff-id="detailStaffId"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { Download, Refresh, Search, Tickets } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { adminStats, exportStatsCsv, type StatRange } from '@/api/stats'
import StatTrend from '@/components/StatTrend.vue'
import StatDetail from '@/components/StatDetail.vue'
import { useTable } from '@/composables/useTable'

const rows = ref<any[]>([])
const { keyword, currentPage, size, total, paged } = useTable(
  rows,
  (r, kw) =>
    (r.name || '').toLowerCase().includes(kw) || (r.title || '').toLowerCase().includes(kw)
)
const loading = ref(false)
const summary = ref({ visit: 0, call: 0, save: 0 })
const trend = ref<any[]>([])

// null = 全部时间（不发 from/to）。日期快捷选项不放「全部」——
// 清空输入框（×）即是全部，与后端缺省语义一致。
const range = ref<[string, string] | null>(null)
const rangeShortcuts = [
  { text: '今天', value: () => [dayOffset(0), dayOffset(0)] },
  { text: '昨天', value: () => [dayOffset(-1), dayOffset(-1)] },
  { text: '近 7 天', value: () => [dayOffset(-6), dayOffset(0)] },
  { text: '近 30 天', value: () => [dayOffset(-29), dayOffset(0)] },
]

// 本地时区取日期：与「后端按 TZ（默认 Asia/Shanghai）落库」配合，
// 同处东八区时两边「今天」是同一天；跨时区使用以各自本地日期为准。
function dayOffset(days: number): string {
  const d = new Date()
  d.setDate(d.getDate() + days)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

const rangeLabel = computed(() =>
  range.value ? `${range.value[0]} 至 ${range.value[1]}` : '全部时间（自上线起累计）'
)

// 选择器的 [from, to] 元组 → 接口的 { from, to } 对象（直接传元组会被
// axios 序列化成 from[]=... 的数组参数，后端收不到日期）。
// 取 computed 而非函数：作为 prop 传给明细抽屉时引用稳定，
// 它的多源 watch 才不会在父组件每次重渲染时被误触发。
const rangeParam = computed<StatRange>(() =>
  range.value ? { from: range.value[0], to: range.value[1] } : {}
)

// 范围变化即重查（不含首次：首次由 onMounted 触发，避免双请求）
watch(range, () => {
  load()
})

// 明细抽屉：员工行「明细」/「数据明细」按钮/趋势图点某天三个入口
const detailOpen = ref(false)
const detailStaffId = ref(0)
// 员工下拉选项取全量 rows（不受关键字/分页影响）——筛明细时要能选到任何人
const staffOptions = computed(() => rows.value.map((r) => ({ staffId: r.staffId, name: r.name })))

function openDetail(staffId = 0) {
  detailStaffId.value = staffId
  detailOpen.value = true
}

// 点趋势图某天：范围收窄到当天 + 打开明细抽屉（一次点击完成「看某天有什么」）
function onTrendPick(date: string) {
  range.value = [date, date] // watch(range) 触发汇总重查；抽屉经 range 变化同步重查
  openDetail()
}

async function load() {
  loading.value = true
  try {
    const r = await adminStats(rangeParam.value)
    summary.value = r.summary
    rows.value = r.byStaff
    trend.value = r.trend
  } catch {
    /* 拦截器已提示 */
  } finally {
    loading.value = false
  }
}

async function onExport() {
  try {
    await exportStatsCsv(rangeParam.value)
  } catch (e) {
    // 业务失败（如登录失效）在 api 层已解析出 msg；网络异常走通用文案
    ElMessage.error((e as Error)?.message || '导出失败')
  }
}

// 埋点口径见《首页组件与接口字段映射》§4：只记前端点击、不校验真实结果；
// 本页无写操作，故不需要「查看名片」之类的跳转（PC 端无法直接打开小程序，
// 运营可在小程序首页输入编号预览）。

onMounted(load)
</script>

<style scoped>
.page-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}
.page-bar h3 {
  margin: 0;
}
.bar-right {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.stat-cards {
  display: flex;
  gap: 16px;
  margin-bottom: 16px;
}
.stat-card {
  flex: 1;
  padding: 16px 20px;
  border: 1px solid #ebeef5;
  border-radius: 6px;
  background: #fafcff;
}
.stat-label {
  color: #909399;
  font-size: 13px;
}
.stat-value {
  margin-top: 6px;
  font-size: 28px;
  font-weight: 600;
  color: #303133;
}
.range-tip {
  margin: 0 0 4px;
  color: #909399;
  font-size: 12px;
}
.tip {
  margin: 12px 0 0;
  color: #909399;
  font-size: 12px;
}
/* 手机端：三张汇总卡由横排改纵向堆叠，避免窄屏被压扁 */
@media (max-width: 767px) {
  .stat-cards {
    flex-direction: column;
  }
  .stat-card {
    padding: 14px 16px;
  }
  .stat-value {
    font-size: 24px;
  }
}
</style>
