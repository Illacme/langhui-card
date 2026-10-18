<template>
  <div>
    <el-card>
      <div class="page-bar">
        <h3>名片修改审核</h3>
        <div class="bar-right">
          <el-input
            v-model="keyword"
            placeholder="搜索员工ID/字段变更"
            clearable
            :prefix-icon="Search"
            style="width: 240px"
          />
          <el-button :icon="Refresh" :loading="loading" @click="load">刷新</el-button>
        </div>
      </div>

      <el-table :data="paged" v-loading="loading" border stripe>
        <el-table-column prop="staffId" label="员工ID" width="90" align="center" v-if="!isPhone" />
        <el-table-column label="变更摘要" min-width="240">
          <template #default="{ row }">
            <el-tag v-for="f in summary(row.snapshot)" :key="f.key" class="sum-tag" size="small" type="info">
              {{ f.label }}→{{ f.new }}
            </el-tag>
            <span v-if="!summary(row.snapshot).length" class="muted">无</span>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="100" align="center">
          <template #default="{ row }">
            <el-tag :type="statusTag(row.status)" effect="light">{{ statusText(row.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="rejectReason" label="驳回原因" min-width="160" show-overflow-tooltip />
        <el-table-column label="提交时间" width="180" v-if="!isPhone">
          <template #default="{ row }">{{ formatDateTime(row.createdAt) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="190" fixed="right" align="center">
          <template #default="{ row }">
            <template v-if="row.status === 'pending'">
              <el-button type="primary" link @click="openDetail(row)">明细</el-button>
              <el-button type="success" link :loading="actingId === row.id" @click="approve(row)">通过</el-button>
              <el-button type="danger" link @click="openReject(row)">驳回</el-button>
            </template>
            <span v-else class="muted">已处理</span>
          </template>
        </el-table-column>
      </el-table>

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

    <!-- 变更明细：字段级 旧值 → 新值 -->
    <el-dialog v-model="detailVisible" title="变更明细" width="520px" class="resp-dialog">
      <el-descriptions :column="1" border v-if="detailRows.length">
        <el-descriptions-item v-for="f in detailRows" :key="f.key" :label="f.label">
          <span class="old">{{ f.old || '（空）' }}</span>
          <el-icon class="arrow"><Right /></el-icon>
          <span class="new">{{ f.new }}</span>
        </el-descriptions-item>
      </el-descriptions>
      <template #footer>
        <el-button @click="detailVisible = false">关闭</el-button>
        <el-button type="success" :loading="actingId === detailRow.id" @click="approve(detailRow)">通过此申请</el-button>
      </template>
    </el-dialog>

    <!-- 驳回 -->
    <el-dialog v-model="rejectVisible" title="驳回申请" width="440px" class="resp-dialog">
      <el-input v-model="rejectReason" type="textarea" :rows="3" placeholder="请填写驳回原因（必填）" />
      <template #footer>
        <el-button @click="rejectVisible = false">取消</el-button>
        <el-button type="primary" :loading="rejecting" :disabled="!rejectReason.trim()" @click="submitReject">确定驳回</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
const { isPhone } = useBreakpoint()
import { ElMessage } from 'element-plus'
import { ref, onMounted } from 'vue'
import { Refresh, Right, Search } from '@element-plus/icons-vue'
import { listModifyApply, approveModify, rejectModify } from '@/api/apply'
import { listEmployees } from '@/api/employees'
import { useTable } from '@/composables/useTable'
import { formatDateTime } from '@/utils/datetime'
import { useBreakpoint } from '@/composables/useBreakpoint'

const FIELD_LABELS: Record<string, string> = {
  avatar: '头像',
  name: '姓名',
  title: '职位',
  phone: '手机',
  landline: '座机',
  email: '邮箱',
}

const rows = ref<any[]>([])
const { keyword, currentPage, size, total, paged } = useTable(
  rows,
  (r, kw) => JSON.stringify(r).toLowerCase().includes(kw)
)
const employeesMap = ref<Record<number, any>>({})
const loading = ref(false)
const actingId = ref<number | null>(null)

const rejectVisible = ref(false)
const rejectReason = ref('')
const rejectId = ref<number | null>(null)
const rejecting = ref(false)

const detailVisible = ref(false)
const detailRow = ref<any>({})
const detailRows = ref<any[]>([])

function parseSnapshot(snap?: string): Record<string, string> {
  if (!snap) return {}
  try {
    return JSON.parse(snap)
  } catch {
    return {}
  }
}

function summary(snap?: string) {
  const obj = parseSnapshot(snap)
  return Object.keys(obj).map((k) => ({ key: k, label: FIELD_LABELS[k] || k, new: obj[k] }))
}

function statusText(s: string) {
  return s === 'pending' ? '待审核' : s === 'approved' ? '已通过' : '已驳回'
}
function statusTag(s: string) {
  return s === 'pending' ? 'success' : s === 'approved' ? 'primary' : 'danger'
}

async function load() {
  loading.value = true
  try {
    const [applies, emps] = await Promise.all([listModifyApply(), listEmployees()])
    rows.value = applies
    const map: Record<number, any> = {}
    emps.forEach((e: any) => (map[e.staffId] = e))
    employeesMap.value = map
  } catch {
    /* 拦截器已提示 */
  } finally {
    loading.value = false
  }
}

function openDetail(row: any) {
  detailRow.value = row
  const obj = parseSnapshot(row.snapshot)
  const emp = employeesMap.value[row.staffId] || {}
  detailRows.value = Object.keys(obj).map((k) => ({
    key: k,
    label: FIELD_LABELS[k] || k,
    old: emp[k] ?? '',
    new: obj[k],
  }))
  detailVisible.value = true
}

async function approve(row: any) {
  actingId.value = row.id
  try {
    await approveModify(row.id)
    ElMessage.success('已通过，变更已应用到该员工')
    detailVisible.value = false
    await load()
  } catch {
    /* 拦截器已提示（已处理等） */
  } finally {
    actingId.value = null
  }
}

function openReject(row: any) {
  rejectId.value = row.id
  rejectReason.value = ''
  rejectVisible.value = true
}

async function submitReject() {
  if (!rejectId.value || rejecting.value) return
  rejecting.value = true
  try {
    await rejectModify(rejectId.value, rejectReason.value.trim())
    ElMessage.success('已驳回')
    rejectVisible.value = false
    await load()
  } catch {
    /* 拦截器已提示 */
  } finally {
    rejecting.value = false
  }
}

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
}
.table-pager {
  margin-top: 14px;
  justify-content: flex-end;
}
.sum-tag {
  margin: 2px 4px 2px 0;
}
.muted {
  color: #c0c4cc;
}
.old {
  color: #909399;
  text-decoration: line-through;
}
.new {
  color: #67c23a;
  font-weight: 600;
}
.arrow {
  margin: 0 8px;
  color: #c0c4cc;
  vertical-align: middle;
}
</style>
