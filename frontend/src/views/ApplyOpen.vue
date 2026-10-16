<template>
  <div>
    <el-card>
      <div class="page-bar">
        <h3>开通审核</h3>
        <div class="bar-right">
          <el-input
            v-model="keyword"
            placeholder="搜索姓名/手机/邮箱"
            clearable
            :prefix-icon="Search"
            style="width: 220px"
          />
          <el-button :icon="Refresh" :loading="loading" @click="load">刷新</el-button>
        </div>
      </div>

      <el-table :data="paged" v-loading="loading" border stripe>
        <el-table-column prop="name" label="姓名" width="110" />
        <el-table-column prop="phone" label="手机" width="140" />
        <el-table-column prop="email" label="邮箱" min-width="160" show-overflow-tooltip />
        <el-table-column prop="title" label="岗位" width="120" />
        <el-table-column label="头像" width="80" align="center">
          <template #default="{ row }">
            <el-image
              v-if="row.avatar"
              :src="row.avatar"
              style="width: 40px; height: 40px; border-radius: 4px"
              fit="cover"
            />
            <span v-else class="muted">—</span>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="100" align="center">
          <template #default="{ row }">
            <el-tag :type="statusTag(row.status)" effect="light">{{ statusText(row.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="rejectReason" label="驳回原因" min-width="160" show-overflow-tooltip />
        <el-table-column prop="createdAt" label="提交时间" width="180" />
        <el-table-column label="操作" width="150" fixed="right" align="center">
          <template #default="{ row }">
            <template v-if="row.status === 'pending'">
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

    <!-- 驳回原因 -->
    <el-dialog v-model="rejectVisible" title="驳回申请" width="440px" class="resp-dialog">
      <el-input
        v-model="rejectReason"
        type="textarea"
        :rows="3"
        placeholder="请填写驳回原因（必填）"
      />
      <template #footer>
        <el-button @click="rejectVisible = false">取消</el-button>
        <el-button type="primary" :disabled="!rejectReason.trim()" @click="submitReject">确定驳回</el-button>
      </template>
    </el-dialog>

    <!-- 通过结果：一次性初始密码 -->
    <el-dialog v-model="okVisible" title="开通成功" width="420px" class="resp-dialog">
      <el-descriptions :column="1" border>
        <el-descriptions-item label="员工编号">{{ okResult.staffId }}</el-descriptions-item>
        <el-descriptions-item label="初始密码">
          <span class="pwd">{{ okResult.initialPassword }}</span>
        </el-descriptions-item>
      </el-descriptions>
      <p class="tip">初始密码仅展示一次，请线下通知员工登录后尽快修改。</p>
      <template #footer>
        <el-button type="primary" @click="okVisible = false">知道了</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { Refresh, Search } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { listOpenApply, approveOpen, rejectOpen } from '@/api/apply'
import { useTable } from '@/composables/useTable'

const rows = ref<any[]>([])
const { keyword, currentPage, size, total, paged } = useTable(
  rows,
  (r, kw) =>
    (r.name || '').toLowerCase().includes(kw) ||
    (r.phone || '').includes(kw) ||
    (r.email || '').toLowerCase().includes(kw)
)
const loading = ref(false)
const actingId = ref<number | null>(null)

const rejectVisible = ref(false)
const rejectReason = ref('')
const rejectId = ref<number | null>(null)

const okVisible = ref(false)
const okResult = ref({ staffId: 0, initialPassword: '' })

function statusText(s: string) {
  return s === 'pending' ? '待审核' : s === 'approved' ? '已通过' : '已驳回'
}
function statusTag(s: string) {
  return s === 'pending' ? 'success' : s === 'approved' ? 'primary' : 'danger'
}

async function load() {
  loading.value = true
  try {
    rows.value = await listOpenApply()
  } catch {
    /* 拦截器已提示 */
  } finally {
    loading.value = false
  }
}

async function approve(row: any) {
  actingId.value = row.id
  try {
    const r = await approveOpen(row.id)
    okResult.value = { staffId: r.staffId, initialPassword: r.initialPassword }
    okVisible.value = true
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
  if (!rejectId.value) return
  try {
    await rejectOpen(rejectId.value, rejectReason.value.trim())
    ElMessage.success('已驳回')
    rejectVisible.value = false
    await load()
  } catch {
    /* 拦截器已提示 */
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
.muted {
  color: #c0c4cc;
}
.pwd {
  font-family: monospace;
  font-size: 16px;
  letter-spacing: 1px;
  color: #e6a23c;
}
.tip {
  color: #909399;
  font-size: 12px;
  margin: 12px 0 0;
}
</style>
