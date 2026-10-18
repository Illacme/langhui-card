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
      title="在此维护产品运行参数。常规项保存后即时生效；标注「需重启」的项要重启服务后才生效；标注「只读」的项只能改环境变量后重启。"
      description="敏感项（密钥 / 连接串）不会回传真实值，界面只显示「已配置」；输入框留空表示不修改，填新值即覆盖。"
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
              <el-tag v-if="item.restart" size="small" type="warning" effect="plain">需重启</el-tag>
              <el-tag v-if="item.readOnly" size="small" type="info" effect="plain">只读</el-tag>
              <el-tag v-if="item.secret" size="small" type="danger" effect="plain">敏感</el-tag>
            </div>
            <div class="row-desc">
              {{ item.description }}
              <span v-if="item.envVar" class="env-hint">环境变量：{{ item.envVar }}</span>
            </div>
          </div>

          <div class="row-control">
            <!-- 开关用三态：未设置（跟随环境变量）/ 开 / 关。
                 后端以「空串 = 未在界面设置」表达回退环境变量，两态开关表达不了。 -->
            <el-select
              v-if="item.type === 'bool'"
              :model-value="draft[item.key]"
              :disabled="item.readOnly"
              @change="(v: any) => setVal(item.key, String(v ?? ''))"
            >
              <el-option label="未设置（跟随环境变量）" value="" />
              <el-option label="开" value="true" />
              <el-option label="关" value="false" />
            </el-select>
            <!-- 整数：上传上限 / 缓存小时 / 锁定租户 -->
            <el-input-number
              v-else-if="item.type === 'int'"
              :model-value="numVal(item.key)"
              :min="0"
              :disabled="item.readOnly"
              controls-position="right"
              @change="(v: any) => setVal(item.key, String(v ?? 0))"
            />
            <!-- 敏感文本：只显示掩码，留空不提交；点「清除」可显式置空回退环境变量 -->
            <div v-else-if="item.secret" class="secret-control">
              <el-input
                v-model="draft[item.key]"
                type="password"
                show-password
                :disabled="item.readOnly"
                :placeholder="secretPlaceholder(item)"
                class="text-input"
              />
              <el-button
                v-if="!item.readOnly && item.configured"
                size="small"
                text
                type="danger"
                @click="clearSecret(item.key)"
              >
                清除
              </el-button>
            </div>
            <!-- 其余（string / string_slice）按文本编辑 -->
            <el-input
              v-else
              v-model="draft[item.key]"
              :disabled="item.readOnly"
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
  auth: '登录与令牌',
  storage: '文件存储与 CDN',
  security: '安全与基础设施（只读）',
  feature: '功能开关',
}
const GROUP_ORDER = ['general', 'miniprogram', 'upload', 'wechat', 'auth', 'storage', 'security', 'feature']

const settings = useSettingsStore()

const items = ref<SettingItem[]>([])
const original = reactive<Record<string, string>>({})
const draft = reactive<Record<string, string>>({})
// 敏感项点「清除」= 显式提交空串（回退环境变量）。它与「没动过」不是一回事，
// 因为两者在输入框里都是空值，必须单独记一笔才能正确生成提交载荷。
const cleared = reactive<Record<string, boolean>>({})
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
  items.value
    .filter((it) => !it.readOnly)
    .filter((it) => cleared[it.key] || draft[it.key] !== original[it.key])
    .map((it) => it.key),
)
const dirty = computed(() => changedKeys.value.length > 0)
const changedCount = computed(() => changedKeys.value.length)

function typeLabel(t: string) {
  return { int: '整数', bool: '开关', string: '文本', string_slice: '列表' }[t] || t
}
function placeholderOf(item: SettingItem) {
  if (item.readOnly) return '只读'
  return item.type === 'string_slice' ? '逗号分隔，如 png,jpg,webp' : '未设置'
}
function secretPlaceholder(item: SettingItem) {
  if (item.readOnly) return '已配置（只读）'
  return item.configured ? '已配置，留空则不修改' : '未设置，留空则回退环境变量'
}
function numVal(k: string) {
  const n = Number(draft[k])
  return Number.isFinite(n) ? n : 0
}
function setVal(k: string, v: string) {
  draft[k] = v
  cleared[k] = false
}
// 清除敏感值：置空并标记，提交时会带上空串让后端回退环境变量
function clearSecret(k: string) {
  draft[k] = ''
  cleared[k] = true
}

function reset() {
  items.value.forEach((it) => {
    draft[it.key] = original[it.key]
    cleared[it.key] = false
  })
}

async function load() {
  loading.value = true
  try {
    const r = await getSettings()
    if (r.code === 0 && r.data) {
      items.value = r.data
      r.data.forEach((it) => {
        // 敏感项后端只给掩码，界面输入框一律留空：留空 = 不修改，避免把掩码回写成真实值。
        // 只读的敏感项例外：它本来就不能提交，直接把掩码显示出来，管理员才看得到「当前已配置」。
        original[it.key] = it.secret && !it.readOnly ? '' : it.value
        draft[it.key] = original[it.key]
        cleared[it.key] = false
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
    payload[k] = cleared[k] ? '' : draft[k]
  })
  try {
    // 后端逐条校验类型，任一非法即返回 code:1（拦截器已弹错误提示并 reject）
    const r = await updateSettings(payload)
    // 成功后把「原值」推进到新值，按钮回到禁用态
    Object.keys(payload).forEach((k) => {
      cleared[k] = false
      // 敏感项保存后输入框回到空（下发的仍是掩码），原值也保持空，避免显示成「已修改」
      original[k] = items.value.find((it) => it.key === k)?.secret ? '' : payload[k]
      draft[k] = original[k]
    })
    // 品牌名变了要立刻同步侧边栏展示，避免用户以为没生效
    if (payload.brand_name !== undefined) {
      settings.setBrandName(payload.brand_name || '')
    }
    const restart = r?.data?.restartKeys || []
    if (restart.length > 0) {
      ElMessage.warning(`已保存 ${changedCount.value} 项；${restart.length} 项需重启服务后生效：${restart.join('、')}`)
    } else {
      ElMessage.success(`已保存 ${changedCount.value} 项设置`)
    }
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
  flex-wrap: wrap;
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
.env-hint {
  margin-left: 6px;
  color: #c0c4cc;
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
.secret-control {
  display: flex;
  align-items: center;
  gap: 6px;
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
