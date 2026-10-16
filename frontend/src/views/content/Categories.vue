<template>
  <div>
    <el-card>
      <div class="page-bar">
        <h3>业绩分类</h3>
        <div class="bar-right">
          <el-input
            v-model="keyword"
            placeholder="搜索分类名"
            clearable
            :prefix-icon="Search"
            style="width: 200px"
          />
          <el-button :icon="Refresh" :loading="loading" @click="load">刷新</el-button>
          <el-button type="primary" :icon="Plus" @click="openCreate">新增分类</el-button>
        </div>
      </div>

      <el-table ref="tableRef" :data="paged" v-loading="loading" border stripe>
        <el-table-column prop="name" label="名称" min-width="140" />
        <el-table-column label="图标" width="90" align="center">
          <template #default="{ row }">
            <el-image
              v-if="row.icon"
              :src="row.icon"
              style="width: 36px; height: 36px"
              fit="cover"
            />
            <span v-else class="muted">—</span>
          </template>
        </el-table-column>
        <el-table-column label="排序" width="120" align="center">
          <template #default="{ row }">
            <div class="sort-cell">
              <span class="drag-handle" draggable="true" title="按住拖拽调整顺序">
                <el-icon><Rank /></el-icon>
              </span>
              <el-button link :disabled="isFirstRow(row)" title="上移" @click="moveUp(row)">
                <el-icon><ArrowUp /></el-icon>
              </el-button>
              <el-button link :disabled="isLastRow(row)" title="下移" @click="moveDown(row)">
                <el-icon><ArrowDown /></el-icon>
              </el-button>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="锁定" width="90" align="center">
          <template #default="{ row }">
            <el-tag :type="row.isLocked ? 'warning' : 'info'" effect="light">
              {{ row.isLocked ? '已锁定' : '未锁' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="160" fixed="right" align="center">
          <template #default="{ row }">
            <el-button type="primary" link @click="openEdit(row)">编辑</el-button>
            <el-button type="danger" link @click="remove(row)">删除</el-button>
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

    <el-dialog
      v-model="dialogVisible"
      :title="editingId ? '编辑分类' : '新增分类'"
      width="460px"
      class="resp-dialog"
      @closed="resetForm"
    >
      <el-form ref="formRef" :model="form" :rules="rules" label-width="80px">
        <el-form-item label="名称" prop="name">
          <el-input v-model="form.name" />
        </el-form-item>
        <el-form-item label="图标">
          <UploadField v-model="form.icon" :multiple="false" accept="image/*" />
        </el-form-item>
        <el-form-item label="排序">
          <el-input-number v-model="form.sort" :min="0" />
        </el-form-item>
        <el-form-item label="锁定">
          <el-switch v-model="form.isLocked" />
          <span class="hint">锁定后禁止删除（如已确认的 7 类）</span>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="submit">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { Refresh, Plus, Search, Rank, ArrowUp, ArrowDown } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox, type FormInstance, type FormRules } from 'element-plus'
import {
  listCategories,
  createCategory,
  updateCategory,
  deleteCategory,
  sortCategories,
} from '@/api/content'
import UploadField from '@/components/UploadField.vue'
import { useTable } from '@/composables/useTable'
import { useSortableRows } from '@/composables/useSortableRows'

const rows = ref<any[]>([])
const { keyword, currentPage, size, total, paged } = useTable(
  rows,
  (r, kw) => (r.name || '').toLowerCase().includes(kw)
)
// 列表顺序调整：拖拽 + 上下箭头（与项目案例/业务服务共用同一 composable）
const { tableRef, isFirstRow, isLastRow, moveUp, moveDown } = useSortableRows(
  rows,
  paged,
  sortCategories
)
const loading = ref(false)
const dialogVisible = ref(false)
const editingId = ref<number | null>(null)
const saving = ref(false)
const formRef = ref<FormInstance>()
const form = reactive({ name: '', icon: '', sort: 1, isLocked: false })
const rules: FormRules = {
  name: [{ required: true, message: '请输入分类名', trigger: 'blur' }],
}

async function load() {
  loading.value = true
  try {
    rows.value = await listCategories()
  } catch {
    /* 拦截器已提示 */
  } finally {
    loading.value = false
  }
}

function resetForm() {
  form.name = ''
  form.icon = ''
  form.sort = 1
  form.isLocked = false
  editingId.value = null
  formRef.value?.clearValidate()
}

function openCreate() {
  resetForm()
  dialogVisible.value = true
}

function openEdit(row: any) {
  editingId.value = row.id
  form.name = row.name
  form.icon = row.icon
  form.sort = row.sort
  form.isLocked = row.isLocked
  dialogVisible.value = true
}

async function submit() {
  if (!formRef.value) return
  const ok = await formRef.value.validate().catch(() => false)
  if (!ok) return
  saving.value = true
  try {
    if (editingId.value) {
      await updateCategory(editingId.value, { ...form })
      ElMessage.success('已更新')
    } else {
      await createCategory({ ...form })
      ElMessage.success('已新增')
    }
    dialogVisible.value = false
    await load()
  } catch {
    /* 拦截器已提示 */
  } finally {
    saving.value = false
  }
}

async function remove(row: any) {
  const confirmed = await ElMessageBox.confirm(`确认删除分类「${row.name}」？`, '提示', {
    type: 'warning',
  })
    .then(() => true)
    .catch(() => false)
  if (!confirmed) return
  try {
    await deleteCategory(row.id)
    ElMessage.success('已删除')
    await load()
  } catch {
    // 后端强校验：锁定分类或存在关联项目会返回 code=1（拦截器已提示原因）
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
.hint {
  color: #909399;
  font-size: 12px;
  margin-left: 8px;
}
</style>
