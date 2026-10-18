<template>
  <el-card>
    <div class="page-bar">
      <h3>系统设置</h3>
      <div class="bar-actions">
        <el-button :disabled="!dirty || saving" @click="reset">重置</el-button>
        <el-button type="primary" :loading="saving" :disabled="!dirty" @click="save">
          保存{{ dirty ? `（${changedCount} 项）` : '' }}
        </el-button>
      </div>
    </div>

    <el-alert
      type="info"
      :closable="false"
      title="在此维护产品运行参数，保存后即时生效、无需重启。"
      description="密钥与基础设施类配置（数据库地址、JWT 密钥、云存储/CDN 凭据、微信 AppSecret）出于安全考虑始终走环境变量，不在此处编辑。"
      style="margin-bottom: 16px"
    />

    <div v-if="loading" class="loading">加载中…</div>

    <template v-else>
      <el-card v-for="g in groups" :key="g.key" shadow="never" class="group-card">
        <template #header>
          <div class="group-header">
            <span class="group-title">{{ g.label }}</span>
            <span class="group-count">{{ g.items.length }} 项</span>
          </div>
        </template>

        <div v-for="item in g.items" :key="item.key" class="row">
          <div class="row-main">
            <div class="row-label">
              <span class="label-text">{{ item.key }}</span>
              <el-tag size="small" effect="plain">{{ typeLabel(item.type) }}</el-tag>
            </div>
            <div class="row-desc">{{ item.description }}</div>
          </div>

          <div class="row-control">
            <!-- 开关：后端按 true/false 字符串存储 -->
            <el-switch
              v-if="item.type === 'bool'"
              :model-value="isTrue(item.key)"
              active-text="开"
              inactive-text="关"
              @change="(v: any) => setVal(item.key, v ? 'true' : 'false')"
            />
            <!-- 整数：上传上限 / 缓存小时 / 锁定租户 -->
            <el-input-number
              v-else-if="item.type === 'int'"
              :model-value="numVal(item.key)"
              :min="minOf(item.key)"
              controls-position="right"
              @change="(v: any) => setVal(item.key, String(v ?? 0))"
            />
            <!-- 其余（string / string_slice）按文本编辑 -->
            <el-input
              v-else
              v-model="draft[item.key]"
              :placeholder="placeholderOf(item)"
              class="text-input"
            />
          </div>
        </div>
      </el-card>
    </template>
  </el-card>
</template>

<script setup lang="ts">
import { ElMessage } from 'element-plus'
import { computed, onMounted, reactive, ref } from 'vue'
import { getSettings, updateSettings, type SettingItem } from '@/api/settings'
import { useSettingsStore } from '@/store/settings'

// 分组展示顺序与中文名；与后端 setting.registry 的 group 字段对应。
// 未登记的分组兜底显示在最后，避免后端新增分组后前端丢项。
const GROUP_LABELS: Record<string, string> = {
  general: '通用',
  miniprogram: '小程序',
  upload: '上传',
  wechat: '微信',
  feature: '功能开关',
}
const GROUP_ORDER = ['general', 'miniprogram', 'upload', 'wechat', 'feature']

const settings = useSettingsStore()

const items = ref<SettingItem[]>([])
const original = reactive<Record<string, string>>({})
const draft = reactive<Record<string, string>>({})
const loading = ref(true)
const saving = ref(false)

// 按分组聚合（已按后端返回顺序 stable，这里再套一层分组顺序）
const groups = computed(() => {
  const map = new Map<string, SettingItem[]>()
  for (const it of items.value) {
    if (!map.has(it.group)) map.set(it.group, [])
    map.get(it.group)!.push(it)
  }
  const keys = [...map.keys()].sort((a, b) => {
    const ia = GROUP_ORDER.indexOf(a)
    const ib = GROUP_ORDER.indexOf(b)
    return (ia < 0 ? 99 : ia) - (ib < 0 ? 99 : ib)
  })
  return keys.map((k) => ({
    key: k,
    label: GROUP_LABELS[k] || k,
    items: map.get(k)!,
  }))
})

const changedKeys = computed(() =>
  Object.keys(draft).filter((k) => draft[k] !== original[k])
)
const dirty = computed(() => changedKeys.value.length > 0)
const changedCount = computed(() => changedKeys.value.length)

function typeLabel(t: string) {
  return { int: '整数', bool: '开关', string: '文本', string_slice: '列表' }[t] || t
}
function placeholderOf(item: SettingItem) {
  return item.type === 'string_slice' ? '逗号分隔，如 png,jpg,webp' : '未设置'
}
function isTrue(k: string) {
  const v = String(draft[k] ?? '').toLowerCase()
  return v === 'true' || v === '1' || v === 'yes' || v === 'on'
}
function numVal(k: string) {
  const n = Number(draft[k])
  return Number.isFinite(n) ? n : 0
}
// 租户 id 允许 0（平台模式）；其余数值项不允许负数
function minOf(k: string) {
  return k === 'pinned_company_id' ? 0 : 0
}
function setVal(k: string, v: string) {
  draft[k] = v
}

function reset() {
  Object.keys(draft).forEach((k) => {
    draft[k] = original[k]
  })
}

async function load() {
  loading.value = true
  try {
    const r = await getSettings()
    if (r.code === 0 && r.data) {
      items.value = r.data
      r.data.forEach((it) => {
        original[it.key] = it.value
        draft[it.key] = it.value
      })
    }
  } catch {
    // 拦截器已提示（如非平台超管越权）
  } finally {
    loading.value = false
  }
}

async function save() {
  if (!dirty.value) return
  saving.value = true
  const payload: Record<string, string> = {}
  changedKeys.value.forEach((k) => {
    payload[k] = draft[k]
  })
  try {
    // 后端逐条校验类型，任一非法即返回 code:1（拦截器已弹错误提示并 reject）
    await updateSettings(payload)
    // 成功后把「原值」推进到新值，按钮回到禁用态
    Object.keys(payload).forEach((k) => {
      original[k] = payload[k]
    })
    // 品牌名变了要立刻同步侧边栏展示，避免用户以为没生效
    if (payload.brand_name !== undefined) {
      settings.setBrandName(payload.brand_name || '')
    }
    ElMessage.success(`已保存 ${changedCount.value} 项设置`)
  } catch {
    // 失败保留草稿，方便用户修正后重试
  } finally {
    saving.value = false
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
.bar-actions {
  display: flex;
  gap: 8px;
}
.loading {
  padding: 24px 0;
  text-align: center;
  color: #909399;
}
.group-card {
  margin-bottom: 14px;
  border-radius: 8px;
}
.group-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.group-title {
  font-size: 15px;
  font-weight: 600;
  color: #303133;
}
.group-count {
  font-size: 12px;
  color: #909399;
}
.row {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 24px;
  padding: 12px 0;
  border-bottom: 1px solid #f2f6fc;
}
.row:last-child {
  border-bottom: none;
}
.row-main {
  min-width: 0;
  flex: 1;
}
.row-label {
  display: flex;
  align-items: center;
  gap: 8px;
}
.label-text {
  font-weight: 600;
  color: #303133;
  font-size: 14px;
}
.row-desc {
  margin-top: 4px;
  color: #909399;
  font-size: 12px;
  line-height: 1.6;
}
.row-control {
  flex: none;
  width: 260px;
  display: flex;
  justify-content: flex-end;
}
.text-input {
  width: 100%;
}
/* 手机端：控制区改为独占一行，避免标签被挤压 */
@media (max-width: 767px) {
  .row {
    flex-direction: column;
    gap: 10px;
  }
  .row-control {
    width: 100%;
    justify-content: flex-start;
  }
}
</style>
