<template>
  <div>
    <el-card>
      <div class="page-bar">
        <h3>员工管理</h3>
        <div class="bar-right">
          <el-input
            v-model="keyword"
            placeholder="搜索姓名/手机/职位/座机/昵称/openid"
            clearable
            :prefix-icon="Search"
            style="width: 240px"
          />
          <el-button :icon="Refresh" :loading="loading" @click="load">刷新</el-button>
          <el-button type="primary" :icon="Plus" @click="openCreate">新增员工</el-button>
        </div>
      </div>

      <!--
        列宽是「算」出来的，不是拍的：固定 width 之和 + min-width 之和 ≈ 1116px，
        需小于后台内容区宽度（约 1170px）。历史上操作列曾设 280px，加上其余列
        合计 1380px 超出容器，fixed="right" 的操作列直接压住「创建时间」列。
        改动列宽时请按同一口径重算，宁紧勿松。
      -->
      <el-table :data="paged" v-loading="loading" border stripe>
        <el-table-column prop="staffId" label="编号" width="62" align="center" />
        <el-table-column prop="name" label="姓名" min-width="80" />
        <el-table-column prop="title" label="职位" min-width="80" />
        <el-table-column prop="phone" label="手机" width="120" show-overflow-tooltip />
        <el-table-column label="座机" width="132" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="landline-cell" :class="{ muted: !row.landline }">{{ row.landline || '—' }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="email" label="邮箱" min-width="120" show-overflow-tooltip />
        <el-table-column prop="nickname" label="微信昵称" width="100" show-overflow-tooltip>
          <template #default="{ row }">
            <span :class="{ muted: !row.nickname }">{{ row.nickname || '—' }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="openid" label="openid" width="210" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="openid-cell" :class="{ muted: !row.openid }">{{ row.openid || '—' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="头像" width="62" align="center">
          <template #default="{ row }">
            <el-image
              v-if="row.avatar"
              :src="row.avatar"
              style="width: 36px; height: 36px; border-radius: 4px"
              fit="cover"
            />
            <span v-else class="muted">—</span>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="72" align="center">
          <template #default="{ row }">
            <el-tag :type="row.status === 'enabled' ? 'success' : 'info'" effect="light">
              {{ row.status === 'enabled' ? '正常' : '已停用' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="创建时间" width="180" show-overflow-tooltip>
          <template #default="{ row }">{{ formatDateTime(row.createdAt) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="190" fixed="right" align="center">
          <template #default="{ row }">
            <el-button type="primary" link @click="preview(row)">预览</el-button>
            <el-button type="primary" link @click="openEdit(row)">编辑</el-button>
            <el-button
              v-if="row.status === 'disabled'"
              type="success"
              link
              :loading="actingId === row.staffId"
              @click="enable(row)"
              >启用</el-button
            >
            <el-button v-else type="warning" link :loading="actingId === row.staffId" @click="disable(row)"
              >停用</el-button
            >
            <el-button
              type="primary"
              link
              :loading="actingId === row.staffId"
              @click="reset(row)"
              >重置密码</el-button
            >
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

    <!-- 新增 / 编辑员工（同一表单：新增走创建接口并弹初始密码，编辑直接覆盖资料） -->
    <el-dialog
      v-model="dialogVisible"
      :title="editingId ? '编辑员工' : '新增员工'"
      width="520px"
      class="resp-dialog"
      @closed="resetForm"
    >
      <el-form ref="formRef" :model="form" :rules="rules" label-width="80px">
        <el-form-item label="姓名" prop="name">
          <el-input v-model="form.name" placeholder="必填" />
        </el-form-item>
        <el-form-item label="手机" prop="phone">
          <el-input v-model="form.phone" placeholder="必填" />
        </el-form-item>
        <el-form-item label="职位">
          <el-input v-model="form.title" />
        </el-form-item>
        <el-form-item label="座机">
          <el-input v-model="form.landline" placeholder="选填，如 021-88888888" />
        </el-form-item>
        <el-form-item label="邮箱">
          <el-input v-model="form.email" />
        </el-form-item>
        <el-form-item label="头像">
          <UploadField v-model="form.avatar" :multiple="false" accept="image/*" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="submit">保存</el-button>
      </template>
    </el-dialog>

    <!-- 名片预览（C2 要求「预览」）：PC 端打不开小程序，用名片摘要 + 小程序码近似。
         小程序码拉取失败（典型：本地开发未配 WX_APPID/WX_APPSECRET）时静默降级——
         api/wechat.ts 走 __silent，这里只把二维码位置换成文字提示，不弹红条。 -->
    <el-dialog v-model="previewVisible" title="名片预览" width="440px" class="resp-dialog">
      <div class="preview-card" v-if="previewRow">
        <div class="preview-head">
          <el-image v-if="previewRow.avatar" :src="previewRow.avatar" class="preview-avatar" fit="cover" />
          <div class="preview-avatar ph" v-else>—</div>
          <div>
            <div class="preview-name">{{ previewRow.name }}</div>
            <div class="preview-title">{{ previewRow.title || '未填写职位' }} · 编号 {{ previewRow.staffId }}</div>
          </div>
        </div>
        <el-descriptions :column="1" border>
          <el-descriptions-item label="手机">{{ previewRow.phone }}</el-descriptions-item>
          <el-descriptions-item label="座机">{{ previewRow.landline || '—' }}</el-descriptions-item>
          <el-descriptions-item label="邮箱">{{ previewRow.email || '—' }}</el-descriptions-item>
          <el-descriptions-item label="微信昵称">
            <span :class="{ muted: !previewRow.nickname }">{{ previewRow.nickname || '—' }}</span>
          </el-descriptions-item>
          <el-descriptions-item label="openid">
            <span class="openid-cell" :class="{ muted: !previewRow.openid }">{{ previewRow.openid || '—' }}</span>
          </el-descriptions-item>
          <el-descriptions-item label="创建时间">{{ formatDateTime(previewRow.createdAt) }}</el-descriptions-item>
          <el-descriptions-item label="状态">
            <el-tag :type="previewRow.status === 'enabled' ? 'success' : 'info'" effect="light">
              {{ previewRow.status === 'enabled' ? '正常' : '已停用' }}
            </el-tag>
          </el-descriptions-item>
        </el-descriptions>
        <div class="preview-qr">
          <el-image v-if="previewQr" :src="previewQr" class="qr-img" fit="contain" />
          <span v-else class="muted">小程序码暂不可用（需配置微信凭据）</span>
        </div>
        <p class="tip">扫码，或在微信小程序首页输入编号 {{ previewRow.staffId }}，可查看访客视角的实际效果。</p>
      </div>
      <template #footer>
        <el-button type="primary" @click="previewVisible = false">知道了</el-button>
      </template>
    </el-dialog>

    <!-- 重置密码结果 -->
    <el-dialog v-model="pwdVisible" title="重置密码成功" width="420px" class="resp-dialog">
      <el-descriptions :column="1" border>
        <el-descriptions-item label="员工编号">{{ pwdResult.staffId }}</el-descriptions-item>
        <el-descriptions-item label="新初始密码">
          <span class="pwd">{{ pwdResult.initialPassword }}</span>
        </el-descriptions-item>
      </el-descriptions>
      <p class="tip">初始密码仅展示一次，请通知员工登录后尽快修改。</p>
      <template #footer>
        <el-button type="primary" @click="pwdVisible = false">知道了</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { Refresh, Plus, Search } from '@element-plus/icons-vue'
import type { FormInstance, FormRules } from 'element-plus'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  listEmployees,
  createEmployee,
  updateEmployee,
  disableEmployee,
  enableEmployee,
  resetPassword,
} from '@/api/employees'
import { getWxacode } from '@/api/wechat'
import UploadField from '@/components/UploadField.vue'
import { useTable } from '@/composables/useTable'
import { formatDateTime } from '@/utils/datetime'

const rows = ref<any[]>([])
const { keyword, currentPage, size, total, paged } = useTable(
  rows,
  (r, kw) =>
    (r.name || '').toLowerCase().includes(kw) ||
    (r.phone || '').includes(kw) ||
    (r.title || '').toLowerCase().includes(kw) ||
    (r.landline || '').includes(kw) ||
    (r.nickname || '').toLowerCase().includes(kw) ||
    (r.openid || '').includes(kw)
)
const loading = ref(false)
const actingId = ref<number | null>(null)

const dialogVisible = ref(false)
const editingId = ref<number | null>(null)
const saving = ref(false)
const formRef = ref<FormInstance>()
const form = reactive({ name: '', phone: '', title: '', landline: '', email: '', avatar: '' })
const rules: FormRules = {
  name: [{ required: true, message: '请输入姓名', trigger: 'blur' }],
  phone: [{ required: true, message: '请输入手机', trigger: 'blur' }],
}

const pwdVisible = ref(false)
const pwdResult = ref({ staffId: 0, initialPassword: '' })

// 名片预览：先开弹窗展示资料（不等二维码），再异步拉小程序码
const previewVisible = ref(false)
const previewRow = ref<any>(null)
const previewQr = ref('')

async function preview(row: any) {
  previewRow.value = row
  previewQr.value = ''
  previewVisible.value = true
  previewQr.value = await getWxacode(row.staffId)
}

async function load() {
  loading.value = true
  try {
    rows.value = await listEmployees()
  } catch {
    /* 拦截器已提示 */
  } finally {
    loading.value = false
  }
}

function openCreate() {
  resetForm()
  dialogVisible.value = true
}

function openEdit(row: any) {
  editingId.value = row.staffId
  form.name = row.name
  form.phone = row.phone
  form.title = row.title
  form.landline = row.landline || ''
  form.email = row.email
  form.avatar = row.avatar
  formRef.value?.clearValidate()
  dialogVisible.value = true
}

function resetForm() {
  form.name = ''
  form.phone = ''
  form.title = ''
  form.landline = ''
  form.email = ''
  form.avatar = ''
  editingId.value = null
  formRef.value?.clearValidate()
}

async function submit() {
  if (!formRef.value) return
  const ok = await formRef.value.validate().catch(() => false)
  if (!ok) return
  saving.value = true
  try {
    if (editingId.value) {
      // 超管编辑即时生效，不经过员工端审核流
      await updateEmployee(editingId.value, { ...form })
      ElMessage.success('已更新')
      dialogVisible.value = false
      await load()
    } else {
      const r = await createEmployee({ ...form })
      pwdResult.value = { staffId: r.staffId, initialPassword: r.initialPassword }
      pwdVisible.value = true
      dialogVisible.value = false
      await load()
    }
  } catch {
    /* 拦截器已提示 */
  } finally {
    saving.value = false
  }
}

async function disable(row: any) {
  const confirmed = await ElMessageBox.confirm(
    `确认停用「${row.name}」？停用后其小程序名片将整页提示已停用。`,
    '提示',
    { type: 'warning' }
  )
    .then(() => true)
    .catch(() => false)
  if (!confirmed) return
  actingId.value = row.staffId
  try {
    await disableEmployee(row.staffId)
    ElMessage.success('已停用')
    await load()
  } catch {
    /* 拦截器已提示 */
  } finally {
    actingId.value = null
  }
}

async function enable(row: any) {
  actingId.value = row.staffId
  try {
    await enableEmployee(row.staffId)
    ElMessage.success('已启用')
    await load()
  } catch {
    /* 拦截器已提示 */
  } finally {
    actingId.value = null
  }
}

async function reset(row: any) {
  const confirmed = await ElMessageBox.confirm(
    `确认重置「${row.name}」的登录密码？重置后该员工旧密码立即失效，需使用新初始密码重新登录。`,
    '提示',
    { type: 'warning' }
  )
    .then(() => true)
    .catch(() => false)
  if (!confirmed) return
  actingId.value = row.staffId
  try {
    const pwd = await resetPassword(row.staffId)
    pwdResult.value = { staffId: row.staffId, initialPassword: pwd }
    pwdVisible.value = true
  } catch {
    /* 拦截器已提示 */
  } finally {
    actingId.value = null
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
.openid-cell {
  font-family: monospace;
  font-size: 12px;
}
.landline-cell {
  white-space: nowrap;
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

/* 名片预览弹窗 */
.preview-card .preview-head {
  display: flex;
  align-items: center;
  margin-bottom: 16px;
}
.preview-avatar {
  width: 56px;
  height: 56px;
  border-radius: 50%;
  margin-right: 14px;
  flex-shrink: 0;
}
.preview-avatar.ph {
  background: #f0f0f0;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #c0c4cc;
}
.preview-name {
  font-size: 16px;
  font-weight: 600;
}
.preview-title {
  color: #909399;
  font-size: 13px;
  margin-top: 4px;
}
.preview-qr {
  margin-top: 16px;
  text-align: center;
}
.qr-img {
  width: 180px;
  height: 180px;
}
</style>
