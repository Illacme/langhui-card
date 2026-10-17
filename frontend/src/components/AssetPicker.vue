<template>
  <!-- 常用素材库选择器：点一下即采用并填入（单选），或多选后批量添加。
       素材由后端内嵌（Lucide 图标 + 通用封面/LOGO），采用后与普通上传同构。 -->
  <el-dialog
    v-model="visible"
    title="素材库"
    width="880px"
    top="6vh"
    :close-on-click-modal="false"
    @closed="onClosed"
  >
    <div class="asset-picker">
      <div class="picker-bar">
        <el-radio-group v-model="tab" size="default">
          <el-radio-button label="all">全部</el-radio-button>
          <el-radio-button v-for="c in categories" :key="c.key" :label="c.key">{{ c.name }}</el-radio-button>
        </el-radio-group>
        <el-input
          v-model="keyword"
          placeholder="搜索素材名称"
          :prefix-icon="Search"
          clearable
          class="picker-search"
        />
      </div>

      <div v-loading="loading" class="picker-grid">
        <div
          v-for="it in filteredItems"
          :key="it.id"
          class="asset-card"
          :class="{ selected: isSelected(it.id) }"
          @click="onCardClick(it)"
        >
          <img class="asset-img" :src="libraryFileUrl(it.thumb)" :alt="it.name" loading="lazy" />
          <div class="asset-name">{{ it.name }}</div>
          <div v-if="adoptingId === it.id" class="asset-mask">
            <el-icon class="is-loading"><Loading /></el-icon>
          </div>
          <div v-else-if="isSelected(it.id)" class="asset-check"><el-icon><Check /></el-icon></div>
        </div>
      </div>
      <el-empty v-if="!loading && !filteredItems.length" description="没有匹配的素材" :image-size="60" />
    </div>

    <template #footer>
      <span class="picker-count">共 {{ filteredItems.length }} 项{{ multiple ? `，已选 ${selected.length} 个` : '' }}</span>
      <span class="picker-actions">
        <el-button @click="visible = false">取消</el-button>
        <el-button
          v-if="multiple"
          type="primary"
          :disabled="!selected.length"
          :loading="batchAdopting"
          @click="confirmMultiple"
        >
          添加所选{{ selected.length ? ` (${selected.length})` : '' }}
        </el-button>
      </span>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { Check, Loading, Search } from '@element-plus/icons-vue'
import {
  listLibrary,
  adoptAsset,
  libraryFileUrl,
  type LibraryCategory,
  type LibraryItem,
} from '@/api/assets'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  multiple: { type: Boolean, default: false },
  limit: { type: Number, default: 9 },
  // 打开时定位到的分类（icon 图标 / cover 封面 / logo LOGO），由调用方按字段语义传入
  defaultCategory: { type: String, default: '' },
})
const emit = defineEmits(['update:modelValue', 'picked'])

const visible = computed({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', v),
})

const loading = ref(false)
const loaded = ref(false)
const categories = ref<LibraryCategory[]>([])
const items = ref<LibraryItem[]>([])
const tab = ref('all')
const keyword = ref('')
const selected = ref<string[]>([])
const adoptingId = ref('')
const batchAdopting = ref(false)

const filteredItems = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  return items.value.filter((it) => {
    if (tab.value !== 'all' && it.category !== tab.value) return false
    if (kw && !(it.name.toLowerCase().includes(kw) || it.tags.some((t) => t.toLowerCase().includes(kw))))
      return false
    return true
  })
})

// 清单只拉一次（素材静态内嵌，不会变），之后每次打开复用
async function load() {
  if (loaded.value) return
  loading.value = true
  try {
    const resp = await listLibrary()
    if (resp.code === 0 && resp.data) {
      categories.value = resp.data.categories || []
      items.value = resp.data.items || []
      loaded.value = true
    }
  } catch {
    // 拦截器已提示
  } finally {
    loading.value = false
  }
}

watch(
  visible,
  (v) => {
    if (!v) return
    tab.value = props.defaultCategory || 'all'
    keyword.value = ''
    selected.value = []
    load()
  },
  { immediate: true }
)

function isSelected(id: string) {
  return selected.value.includes(id)
}

// 单选：点卡片 = 采用 + 填入 + 关闭（一步到位）；多选：点卡片累计选择
async function onCardClick(it: LibraryItem) {
  if (props.multiple) {
    if (isSelected(it.id)) {
      selected.value = selected.value.filter((x) => x !== it.id)
    } else if (selected.value.length >= props.limit) {
      ElMessage.warning(`最多选择 ${props.limit} 个`)
    } else {
      selected.value.push(it.id)
    }
    return
  }
  adoptingId.value = it.id
  try {
    const resp = await adoptAsset(it.id)
    if (resp.code === 0 && resp.data?.url) {
      emit('picked', resp.data.url)
      visible.value = false
    }
  } catch {
    // 拦截器已提示
  } finally {
    adoptingId.value = ''
  }
}

// 多选：逐项采用（顺序执行，避免并发写同一存储），全部成功后回传 URL 数组
async function confirmMultiple() {
  if (!selected.value.length) return
  batchAdopting.value = true
  const urls: string[] = []
  try {
    for (const id of selected.value) {
      const resp = await adoptAsset(id)
      if (resp.code === 0 && resp.data?.url) urls.push(resp.data.url)
    }
    if (urls.length) {
      emit('picked', urls)
      visible.value = false
    }
  } catch {
    // 拦截器已提示
  } finally {
    batchAdopting.value = false
  }
}

function onClosed() {
  selected.value = []
}
</script>

<style scoped>
.asset-picker {
  min-height: 300px;
}
.picker-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 14px;
  flex-wrap: wrap;
}
.picker-search {
  width: 220px;
}
.picker-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
  gap: 12px;
  max-height: 56vh;
  overflow-y: auto;
  padding: 2px;
}
.asset-card {
  position: relative;
  border: 1px solid #ebeef5;
  border-radius: 8px;
  overflow: hidden;
  cursor: pointer;
  background: #fff;
  transition: border-color 0.15s, box-shadow 0.15s;
}
.asset-card:hover {
  border-color: #409eff;
  box-shadow: 0 2px 8px rgba(64, 158, 255, 0.15);
}
.asset-card.selected {
  border-color: #409eff;
  box-shadow: 0 0 0 1px #409eff inset;
}
.asset-img {
  display: block;
  width: 100%;
  height: 110px;
  object-fit: contain;
  background: #f7f8fa;
}
.asset-name {
  padding: 6px 8px;
  font-size: 12px;
  color: #606266;
  text-align: center;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.asset-mask {
  position: absolute;
  inset: 0;
  background: rgba(255, 255, 255, 0.75);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 22px;
  color: #409eff;
}
.asset-check {
  position: absolute;
  top: 6px;
  right: 6px;
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: #409eff;
  color: #fff;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 14px;
}
.picker-count {
  font-size: 13px;
  color: #909399;
  margin-right: 12px;
}
.picker-actions {
  float: right;
}
</style>
