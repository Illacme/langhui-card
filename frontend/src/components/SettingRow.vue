<template>
  <!-- 单行配置项：主列表与「配置向导」共用同一行渲染，保证两处形态/语义一致。
       draft 值由父级持有（v-model 拆成 :model-value + change 事件），
       避免行组件内部再复制一份状态导致两处不同步。 -->
  <div class="row" :id="id" :class="{ 'row-dim': dim }">
    <div class="row-main">
      <div class="row-label">
        <span class="label-text">{{ item.name || item.key }}</span>
        <!-- key 降级为次要灰字：界面面向运营用中文名，排查日志/环境变量时仍可对照 -->
        <span v-if="item.name" class="label-key">{{ item.key }}</span>
        <el-tag size="small" effect="plain">{{ typeLabel(item.type) }}</el-tag>
        <!-- 依赖联动：当前选项下「应该配」且还没配 → 待配置；「暂时不用管」 → 置灰说明 -->
        <el-tag v-if="focus && !item.configured" size="small" type="warning" effect="plain">待配置</el-tag>
        <el-tag v-if="dim" size="small" type="info" effect="plain">当前选项下不需要</el-tag>
        <el-tag v-if="item.restart" size="small" type="warning" effect="plain">需重启</el-tag>
        <el-tag v-if="item.readOnly" size="small" type="info" effect="plain">只读</el-tag>
        <!-- 只读项也要说清「现在到底配了没」：只写「只读」会让运营以为功能没生效。
             未配置 = 进程在用内置缺省值（如 JWT_KEY 仍是 change-me），属上线前要修的项。 -->
        <el-tag
          v-if="item.readOnly"
          size="small"
          effect="plain"
          :type="item.configured ? 'success' : 'warning'"
        >
          {{ item.configured ? '已配置' : '未配置·用缺省值' }}
        </el-tag>
        <el-tag v-if="item.secret" size="small" type="danger" effect="plain">敏感</el-tag>
      </div>
      <div class="row-desc">
        {{ item.description }}
        <span v-if="item.envVar" class="env-hint">
          部署环境配置名：{{ item.envVar }}
          <template v-if="item.readOnly">
            ——本页不可改，请到部署环境改环境变量或配置文件（默认 data/runtime.env）后重启服务
          </template>
        </span>
      </div>
    </div>

    <div class="row-control">
      <!-- 开关用三态：未设置（跟随环境变量）/ 开 / 关。
           后端以「空串 = 未在界面设置」表达回退环境变量，两态开关表达不了。 -->
      <el-select
        v-if="item.type === 'bool'"
        :model-value="modelValue"
        :disabled="item.readOnly"
        @change="(v: any) => emit('change', item.key, String(v ?? ''))"
      >
        <el-option label="未设置（跟随环境变量）" value="" />
        <el-option label="开" value="true" />
        <el-option label="关" value="false" />
      </el-select>
      <!-- 整数：上传上限 / 缓存小时 / 锁定租户 -->
      <el-input-number
        v-else-if="item.type === 'int'"
        :model-value="numVal"
        :min="minOf(item)"
        :disabled="item.readOnly"
        controls-position="right"
        @change="(v: any) => emit('change', item.key, String(v ?? 0))"
      />
      <!-- 敏感文本：可写项用密码框（只显示掩码，留空不提交，可点「清除」）。
           只读项例外：后端给的是「可辨认预览」（保留结构与首尾、遮住中段），
           用密码框会把预览变成圆点、反而什么都看不清，故用普通只读框原样显示。 -->
      <div v-if="item.secret && !item.readOnly" class="secret-control">
        <el-input
          :model-value="modelValue"
          type="password"
          show-password
          :disabled="item.readOnly"
          :placeholder="secretPlaceholder(item)"
          class="text-input"
          @update:model-value="(v: any) => emit('change', item.key, String(v ?? ''))"
        />
        <el-button
          v-if="!item.readOnly && item.configured"
          size="small"
          text
          type="danger"
          @click="emit('clear', item.key)"
        >
          清除
        </el-button>
      </div>
      <!-- 其余（string / string_slice）按文本编辑；只读敏感项也走这里，
           原样显示后端给的「可辨认预览」 -->
      <el-input
        v-else
        :model-value="modelValue"
        :disabled="item.readOnly"
        :placeholder="placeholderOf(item)"
        class="text-input"
        @update:model-value="(v: any) => emit('change', item.key, String(v ?? ''))"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { SettingItem } from '@/api/settings'

const props = defineProps<{
  item: SettingItem
  /** 当前草稿值（父级 draft[item.key]） */
  modelValue: string
  /** 行锚点 id（主列表用，供就绪面板 jumpTo 定位；向导内可不传） */
  id?: string
  /** 依赖联动：当前选项下应当配置 */
  focus?: boolean
  /** 依赖联动：当前选项下暂不需要（置灰） */
  dim?: boolean
}>()

const emit = defineEmits<{
  (e: 'change', key: string, value: string): void
  (e: 'clear', key: string): void
}>()

function typeLabel(t: string) {
  return { int: '整数', bool: '开关', string: '文本', string_slice: '列表' }[t] || t
}

// 有安全下限的整数项：界面先拦一道（后端也会拒绝，但能在选数时就提示更直观）。
// 例如上传上限填 0 会让所有上传失败，报错还指向文件而不是配置。
const INT_MIN: Record<string, number> = { upload_max_mb: 1 }
function minOf(item: SettingItem) {
  return INT_MIN[item.key] ?? 0
}
function placeholderOf(item: SettingItem) {
  if (item.readOnly) return '只读'
  return item.type === 'string_slice' ? '逗号分隔，如 png,jpg,webp' : '未设置'
}
function secretPlaceholder(item: SettingItem) {
  // 只读敏感项不走密码框（走普通只读框显示后端预览），这里只服务于可写项
  if (item.readOnly) return '只读'
  return item.configured ? '已配置，留空则不修改' : '未设置，留空则用部署环境的值'
}

// 整型控件的值：草稿是字符串，转不成数字时给 0（与旧实现一致）
const numVal = computed(() => {
  const n = Number(props.modelValue)
  return Number.isFinite(n) ? n : 0
})
</script>

<style scoped>
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
.label-key {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 11px;
  color: #c0c4cc;
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
/* 依赖联动：当前选项下暂不需要的项整行置灰（只读项另由控件 disabled 表达） */
.row-dim {
  opacity: 0.55;
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
