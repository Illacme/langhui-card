<template>
  <div>
    <el-card>
      <div class="page-bar">
        <h3>企业管理</h3>
        <div class="bar-right">
          <span class="bar-tip">平台超级管理员：开通/停用企业、管理企业管理员</span>
          <el-button :icon="Refresh" :loading="loading" @click="load">刷新</el-button>
          <el-button type="primary" :icon="Plus" :disabled="locked" @click="openCreate"
            >开通企业</el-button
          >
        </div>
      </div>

      <!-- 独立版企业锁定：整体只读，并说明在哪里能解除 -->
      <el-alert
        v-if="locked"
        type="warning"
        :closable="false"
        show-icon
        title="已开启独立版企业锁定，当前为只读"
        description="不能新建/编辑企业、启停企业或添加企业管理员。如需修改，请到「平台管理 → 系统设置 → 功能开关」关闭 independent_lock。"
        style="margin-bottom: 14px"
      />

      <el-table :data="rows" v-loading="loading" border stripe>
        <el-table-column prop="id" label="编号" width="70" align="center" v-if="!isPhone" />
        <el-table-column prop="name" label="企业名称" min-width="220" show-overflow-tooltip />
        <el-table-column label="简称" min-width="130" show-overflow-tooltip>
          <template #default="{ row }">{{ row.shortName || '智能名片' }}</template>
        </el-table-column>
        <el-table-column label="状态" width="90" align="center">
          <template #default="{ row }">
            <el-tag :type="row.status === 'enabled' ? 'success' : 'info'" effect="light">
              {{ row.status === 'enabled' ? '运营中' : '已停用' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="employeeCount" label="员工数" width="90" align="center" v-if="!isPhone" />
        <el-table-column prop="adminCount" label="管理员数" width="100" align="center" v-if="!isPhone" />
        <el-table-column label="创建时间" width="180" show-overflow-tooltip v-if="!isPhone">
          <template #default="{ row }">{{ formatDateTime(row.createdAt) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="260" fixed="right" align="center">
          <template #default="{ row }">
            <el-button type="primary" link @click="openAdmins(row)">管理员</el-button>
            <el-button type="primary" link :disabled="locked" @click="openEdit(row)">编辑</el-button>
            <el-button
              v-if="row.status === 'enabled'"
              type="warning"
              link
              :disabled="locked"
              :loading="actingId === row.id"
              @click="toggleStatus(row)"
              >停用</el-button
            >
            <el-button
              v-else
              type="success"
              link
              :disabled="locked"
              :loading="actingId === row.id"
              @click="toggleStatus(row)"
              >启用</el-button
            >
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 开通企业 -->
    <el-dialog v-model="createVisible" title="开通企业" width="480px" @closed="resetCreate">
      <el-form ref="createFormRef" :model="createForm" :rules="createRules" label-width="110px">
        <el-form-item label="企业名称" prop="name">
          <el-input v-model="createForm.name" placeholder="如：廊坊市XX科技有限公司" clearable />
        </el-form-item>
        <el-form-item label="企业简称">
          <el-input
            v-model="createForm.shortName"
            placeholder="侧边栏「简称 · 管理后台」品牌名，留空默认「智能名片」"
            clearable
          />
        </el-form-item>
        <el-divider content-position="left">
          <span class="divider-hint">企业管理员（选填，留空稍后添加）</span>
        </el-divider>
        <el-form-item label="管理员用户名" prop="adminUsername">
          <el-input v-model="createForm.adminUsername" placeholder="登录账号" clearable />
        </el-form-item>
        <el-form-item label="管理员密码" prop="adminPassword">
          <el-input v-model="createForm.adminPassword" placeholder="登录密码" clearable />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="createVisible = false">取消</el-button>
        <el-button type="primary" :loading="creating" @click="submitCreate">开通</el-button>
      </template>
    </el-dialog>

    <!-- 编辑企业（名称 / 简称） -->
    <el-dialog v-model="editVisible" title="编辑企业" width="480px" @closed="resetEdit">
      <el-form ref="editFormRef" :model="editForm" :rules="editRules" label-width="110px">
        <el-form-item label="企业名称" prop="name">
          <el-input v-model="editForm.name" placeholder="如：廊坊市XX科技有限公司" clearable />
        </el-form-item>
        <el-form-item label="企业简称">
          <el-input
            v-model="editForm.shortName"
            placeholder="侧边栏「简称 · 管理后台」品牌名，留空默认「智能名片」"
            clearable
          />
        </el-form-item>
        <div class="edit-tip">
          简称修改后同步后台侧边栏品牌名（留空默认「智能名片」）；公司信息里的简称（仅小程序展示）不受影响，由企业管理员在「公司信息」页维护。
        </div>
      </el-form>
      <template #footer>
        <el-button @click="editVisible = false">取消</el-button>
        <el-button type="primary" :loading="editing" @click="submitEdit">保存</el-button>
      </template>
    </el-dialog>

    <!-- 企业管理员管理 -->
    <el-dialog v-model="adminsVisible" :title="`管理员管理 - ${currentCompany?.name || ''}`" width="560px">
      <div class="admin-add-bar">
        <el-input v-model="newAdmin.username" placeholder="用户名" style="width: 180px" clearable />
        <el-input v-model="newAdmin.password" placeholder="密码" style="width: 180px" clearable />
        <el-button
          type="primary"
          :icon="Plus"
          :disabled="locked"
          :loading="addingAdmin"
          @click="submitAddAdmin"
          >添加</el-button
        >
      </div>
      <el-table :data="admins" v-loading="adminsLoading" border stripe max-height="360">
        <el-table-column prop="id" label="编号" width="70" align="center" />
        <el-table-column prop="username" label="用户名" min-width="160" show-overflow-tooltip />
        <el-table-column label="创建时间" width="180" show-overflow-tooltip>
          <template #default="{ row }">{{ formatDateTime(row.createdAt) }}</template>
        </el-table-column>
      </el-table>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
const { isPhone } = useBreakpoint()
import { computed, onMounted, reactive, ref } from 'vue'
import { Plus, Refresh } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox, type FormInstance, type FormRules } from 'element-plus'
import {
  createCompany,
  createCompanyAdmin,
  listCompanies,
  listCompanyAdmins,
  setCompanyStatus,
  updateCompany,
  type CompanyAdminItem,
  type CompanyItem,
} from '@/api/tenants'
import { formatDateTime } from '@/utils/datetime'
import { useAuthStore } from '@/store/auth'
import { useBreakpoint } from '@/composables/useBreakpoint'

const auth = useAuthStore()
// 独立版企业锁定（后台「系统设置 → independent_lock」）：企业管理页整体只读。
// 写按钮已 disabled，这里再加一道入口守卫——防止快捷键 / 残留弹窗绕过界面禁用。
// 后端 writeLocked 才是最终防线，前端只是避免「点了才报错」。
const locked = computed(() => auth.independentLock)

const rows = ref<any[]>([])
const loading = ref(false)
const actingId = ref<number | null>(null)

async function load() {
  loading.value = true
  try {
    const resp = await listCompanies()
    rows.value = resp.data || []
  } catch {
    // 拦截器已提示
  } finally {
    loading.value = false
  }
}

onMounted(load)

// ===== 开通企业 =====
const createVisible = ref(false)
const creating = ref(false)
const createFormRef = ref<FormInstance>()
const createForm = reactive({ name: '', shortName: '', adminUsername: '', adminPassword: '' })

const createRules: FormRules = {
  name: [{ required: true, message: '请输入企业名称', trigger: 'blur' }],
}

function openCreate() {
  if (locked.value) return
  createVisible.value = true
}

function resetCreate() {
  createForm.name = ''
  createForm.shortName = ''
  createForm.adminUsername = ''
  createForm.adminPassword = ''
  createFormRef.value?.clearValidate()
}

async function submitCreate() {
  if (!createFormRef.value) return
  const ok = await createFormRef.value.validate().catch(() => false)
  if (!ok) return
  creating.value = true
  try {
    await createCompany({
      name: createForm.name,
      shortName: createForm.shortName || undefined,
      adminUsername: createForm.adminUsername || undefined,
      adminPassword: createForm.adminPassword || undefined,
    })
    ElMessage.success('企业已开通')
    createVisible.value = false
    await load()
  } catch {
    // 拦截器已提示（如用户名已存在）
  } finally {
    creating.value = false
  }
}

// ===== 编辑企业（名称 / 简称） =====
const editVisible = ref(false)
const editing = ref(false)
const editFormRef = ref<FormInstance>()
const editForm = reactive({ id: 0, name: '', shortName: '' })

const editRules: FormRules = {
  name: [{ required: true, message: '请输入企业名称', trigger: 'blur' }],
}

function openEdit(row: any) {
  if (locked.value) return
  editForm.id = row.id
  editForm.name = row.name
  editForm.shortName = row.shortName || ''
  editVisible.value = true
}

function resetEdit() {
  editForm.id = 0
  editForm.name = ''
  editForm.shortName = ''
  editFormRef.value?.clearValidate()
}

async function submitEdit() {
  if (!editFormRef.value) return
  const ok = await editFormRef.value.validate().catch(() => false)
  if (!ok) return
  editing.value = true
  try {
    await updateCompany(editForm.id, { name: editForm.name, shortName: editForm.shortName })
    ElMessage.success('已保存')
    editVisible.value = false
    await load()
  } catch {
    // 拦截器已提示
  } finally {
    editing.value = false
  }
}

// ===== 停用 / 启用 =====
async function toggleStatus(row: any) {
  if (locked.value) return
  const disabling = row.status === 'enabled'
  try {
    await ElMessageBox.confirm(
      disabling
        ? `停用后该企业的管理员与员工将不能登录，确定停用「${row.name}」？`
        : `确定恢复启用「${row.name}」？`,
      '确认操作',
      { type: 'warning' }
    )
  } catch {
    return // 用户取消
  }
  actingId.value = row.id
  try {
    await setCompanyStatus(row.id, disabling ? 'disabled' : 'enabled')
    ElMessage.success(disabling ? '已停用' : '已启用')
    await load()
  } catch {
    // 拦截器已提示
  } finally {
    actingId.value = null
  }
}

// ===== 企业管理员管理 =====
const adminsVisible = ref(false)
const adminsLoading = ref(false)
const addingAdmin = ref(false)
const currentCompany = ref<CompanyItem | null>(null)
const admins = ref<any[]>([])
const newAdmin = reactive({ username: '', password: '' })

async function openAdmins(row: any) {
  currentCompany.value = row
  adminsVisible.value = true
  newAdmin.username = ''
  newAdmin.password = ''
  await reloadAdmins()
}

async function reloadAdmins() {
  if (!currentCompany.value) return
  adminsLoading.value = true
  try {
    const resp = await listCompanyAdmins(currentCompany.value.id)
    admins.value = resp.data || []
  } catch {
    // 拦截器已提示
  } finally {
    adminsLoading.value = false
  }
}

async function submitAddAdmin() {
  if (locked.value) return
  if (!currentCompany.value) return
  if (!newAdmin.username || !newAdmin.password) {
    ElMessage.warning('请输入用户名与密码')
    return
  }
  addingAdmin.value = true
  try {
    await createCompanyAdmin(currentCompany.value.id, newAdmin.username, newAdmin.password)
    ElMessage.success('管理员已添加')
    newAdmin.username = ''
    newAdmin.password = ''
    await reloadAdmins()
    await load()
  } catch {
    // 拦截器已提示（如用户名已存在）
  } finally {
    addingAdmin.value = false
  }
}
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
  font-size: 16px;
}
.bar-right {
  display: flex;
  align-items: center;
  gap: 8px;
}
.bar-tip {
  font-size: 12px;
  color: #909399;
  margin-right: 4px;
}
.divider-hint {
  font-size: 12px;
  color: #909399;
}
.edit-tip {
  font-size: 12px;
  color: #909399;
  line-height: 1.6;
  padding: 0 4px;
}
.admin-add-bar {
  display: flex;
  gap: 8px;
  margin-bottom: 12px;
}
</style>
