<template>
  <div>
    <el-card>
      <div class="page-bar">
        <h3>项目案例</h3>
        <div class="bar-right">
          <el-select v-model="catFilter" placeholder="全部分类" clearable style="width: 160px">
            <el-option v-for="c in categories" :key="c.id" :label="c.name" :value="c.id" />
          </el-select>
          <el-input
            v-model="keyword"
            placeholder="搜索案例名"
            clearable
            :prefix-icon="Search"
            style="width: 200px"
          />
          <el-button :icon="Refresh" :loading="loading" @click="load">刷新</el-button>
          <el-button type="primary" :icon="Plus" @click="openCreate">新增案例</el-button>
        </div>
      </div>

      <el-table :data="paged" v-loading="loading" border stripe>
        <el-table-column prop="name" label="名称" min-width="180" />
        <el-table-column label="分类" width="130">
          <template #default="{ row }">{{ categoryName(row.categoryId) }}</template>
        </el-table-column>
        <el-table-column label="封面" width="80" align="center">
          <template #default="{ row }">
            <el-image v-if="row.cover" :src="row.cover" style="width: 40px; height: 40px" fit="cover" />
            <span v-else class="muted">—</span>
          </template>
        </el-table-column>
        <el-table-column label="精选" width="80" align="center">
          <template #default="{ row }">
            <el-tag :type="row.isHomeFeatured ? 'danger' : 'info'" effect="plain">
              {{ row.isHomeFeatured ? '精选' : '普通' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="上架" width="80" align="center">
          <template #default="{ row }">
            <el-tag :type="row.enabled ? 'success' : 'info'" effect="plain">
              {{ row.enabled ? '上架' : '下架' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="sort" label="排序" width="80" align="center" />
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

    <el-dialog v-model="dialogVisible" :title="editingId ? '编辑案例' : '新增案例'" width="720px" @closed="resetForm">
      <el-form ref="formRef" :model="form" :rules="rules" label-width="90px">
        <el-form-item label="分类" prop="categoryId">
          <el-select v-model="form.categoryId" placeholder="选择业绩分类" style="width: 240px">
            <el-option v-for="c in categories" :key="c.id" :label="c.name" :value="c.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="名称" prop="name">
          <el-input v-model="form.name" />
        </el-form-item>
        <el-form-item label="简介">
          <el-input v-model="form.summary" type="textarea" :rows="2" />
        </el-form-item>
        <el-form-item label="详情">
          <RichEditor v-model="form.detail" :height="360" />
        </el-form-item>
        <el-form-item label="封面">
          <UploadField v-model="form.cover" :multiple="false" accept="image/*" />
        </el-form-item>
        <el-form-item label="相册">
          <UploadField v-model="form.album" :multiple="true" :limit="9" accept="image/*" />
        </el-form-item>
        <el-form-item label="首页精选">
          <el-switch v-model="form.isHomeFeatured" />
        </el-form-item>
        <el-form-item label="上架">
          <el-switch v-model="form.enabled" />
        </el-form-item>
        <el-form-item label="排序">
          <el-input-number v-model="form.sort" :min="0" />
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
import { ref, reactive, computed, watch, onMounted } from 'vue'
import { Refresh, Plus, Search } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox, type FormInstance, type FormRules } from 'element-plus'
import {
  listProjects,
  createProject,
  updateProject,
  deleteProject,
  listCategories,
} from '@/api/content'
import UploadField from '@/components/UploadField.vue'
import RichEditor from '@/components/RichEditor.vue'
import { useTable } from '@/composables/useTable'

const rows = ref<any[]>([])
const categories = ref<any[]>([])
const catFilter = ref<number | undefined>(undefined)
const baseList = computed(() =>
  catFilter.value ? rows.value.filter((r) => r.categoryId === catFilter.value) : rows.value
)
const { keyword, currentPage, size, total, paged } = useTable(
  baseList,
  (r, kw) => (r.name || '').toLowerCase().includes(kw)
)
// 切换分类时回到第一页，避免停留在越界页
watch(catFilter, () => (currentPage.value = 1))
const loading = ref(false)
const dialogVisible = ref(false)
const editingId = ref<number | null>(null)
const saving = ref(false)
const formRef = ref<FormInstance>()
const form = reactive({
  categoryId: undefined as number | undefined,
  name: '',
  summary: '',
  detail: '',
  cover: '',
  album: [] as string[],
  isHomeFeatured: false,
  enabled: true,
  sort: 1,
})
const rules: FormRules = {
  name: [{ required: true, message: '请输入案例名称', trigger: 'blur' }],
  categoryId: [{ required: true, message: '请选择分类', trigger: 'change' }],
}

function categoryName(id: number) {
  const c = categories.value.find((x) => x.id === id)
  return c ? c.name : '—'
}

async function load() {
  loading.value = true
  try {
    const [ps, cs] = await Promise.all([listProjects(), listCategories()])
    rows.value = ps
    categories.value = cs
  } catch {
    /* 拦截器已提示 */
  } finally {
    loading.value = false
  }
}

function resetForm() {
  form.categoryId = undefined
  form.name = ''
  form.summary = ''
  form.detail = ''
  form.cover = ''
  form.album = []
  form.isHomeFeatured = false
  form.enabled = true
  form.sort = 1
  editingId.value = null
  formRef.value?.clearValidate()
}

function openCreate() {
  resetForm()
  dialogVisible.value = true
}

function openEdit(row: any) {
  editingId.value = row.id
  form.categoryId = row.categoryId
  form.name = row.name
  form.summary = row.summary
  form.detail = row.detail
  form.cover = row.cover
  form.album = row.album ? [...row.album] : []
  form.isHomeFeatured = row.isHomeFeatured
  form.enabled = row.enabled
  form.sort = row.sort
  dialogVisible.value = true
}

async function submit() {
  if (!formRef.value) return
  const ok = await formRef.value.validate().catch(() => false)
  if (!ok) return
  saving.value = true
  try {
    const payload = {
      categoryId: form.categoryId,
      name: form.name,
      summary: form.summary,
      detail: form.detail,
      cover: form.cover,
      album: form.album,
      isHomeFeatured: form.isHomeFeatured,
      enabled: form.enabled,
      sort: form.sort,
    }
    if (editingId.value) {
      await updateProject(editingId.value, payload)
      ElMessage.success('已更新')
    } else {
      await createProject(payload)
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
  const confirmed = await ElMessageBox.confirm(`确认删除案例「${row.name}」？`, '提示', {
    type: 'warning',
  })
    .then(() => true)
    .catch(() => false)
  if (!confirmed) return
  try {
    await deleteProject(row.id)
    ElMessage.success('已删除')
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
  flex-wrap: wrap;
}
.table-pager {
  margin-top: 14px;
  justify-content: flex-end;
}
.muted {
  color: #c0c4cc;
}
</style>
