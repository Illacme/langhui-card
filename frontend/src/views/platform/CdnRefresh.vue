<template>
  <el-card>
    <div class="page-bar">
      <h3>CDN 刷新</h3>
      <!-- 当前生效厂商：值来自「系统设置 → 文件存储与 CDN」。库里没存（走环境变量配的）
           时后端按 aliyun 回落，这里如实标注，避免「没配过怎么会是阿里云」的困惑 -->
      <el-tag v-if="providerTag" size="small" effect="plain">{{ providerTag }}</el-tag>
    </div>

    <!-- 配不全就提交只会得到一句「CDN 刷新未配置」——先在这儿点名缺口，省一次往返 -->
    <el-alert
      v-if="blockReason"
      type="warning"
      :closable="false"
      :title="blockReason"
      style="margin-bottom: 14px"
    >
      <el-button size="small" @click="goSettings">去设置</el-button>
    </el-alert>

    <el-alert
      type="info"
      :closable="false"
      title="覆盖同名文件的操作（素材采用、小程序码重生成）会自动刷新，这里是手动兜底：直接在对象存储里换了文件、或想让边缘立刻失效时用。支持阿里云 / 腾讯云 / 七牛云 / Cloudflare / AWS CloudFront，厂商与凭据在「系统设置 → 文件存储与 CDN」里配（也可用环境变量），改完需重启服务。条数超过厂商单次上限时会自动分批提交。"
      style="margin-bottom: 14px"
    />

    <el-input
      v-model="urls"
      type="textarea"
      :rows="8"
      placeholder="每行一个待刷新的 URL（文件或目录），例如：&#10;https://cdn.example.com/uploads/abc.png&#10;https://cdn.example.com/uploads/def.jpg"
    />

    <div class="actions">
      <el-button type="primary" :loading="loading" @click="refresh">提交刷新</el-button>
      <span class="count">共 {{ urlList.length }} 条</span>
    </div>
  </el-card>
</template>

<script setup lang="ts">
import { ElMessage } from 'element-plus'
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { cdnRefresh } from '@/api/storage'
import { getSettings, type SettingItem } from '@/api/settings'
import {
  CDN_DEFAULT_PROVIDER,
  CDN_NO_AK_PROVIDERS,
  CDN_RESOURCE_PROVIDERS,
  cdnProviderLabel,
} from '@/utils/cdn'

const router = useRouter()
const urls = ref('')
const loading = ref(false)
const items = ref<SettingItem[]>([])

onMounted(async () => {
  try {
    const r = await getSettings()
    // 与设置页同构：响应体就是信封，data 才是设置项列表
    items.value = r.data ?? []
  } catch {
    // 状态条是锦上添花：拿不到设置就只留通用说明，不能因此挡住刷新入口
  }
})

function itemOf(key: string): SettingItem | undefined {
  return items.value.find((it) => it.key === key)
}
// 缺项提示用后端下发的中文名：新增/改名时界面自动跟上，不用再维护一份标签
function nameOf(key: string): string {
  return itemOf(key)?.name || key
}

// 未选厂商时后端按 aliyun 处理（storage.normalizeCDNProvider），界面同步显示
const provider = computed(() => (itemOf('cdn_provider')?.value || '').trim() || CDN_DEFAULT_PROVIDER)
const providerSet = computed(() => !!(itemOf('cdn_provider')?.value || '').trim())
const providerTag = computed(() => {
  // 设置还没拉到时不显示：否则会先闪一下「按默认（aliyun）」，像是配过
  if (!items.value.length) return ''
  const p = provider.value
  const tail = providerSet.value ? '' : ' · 未设置，按默认'
  return `${p}（${cdnProviderLabel(p)}）${tail}`
})
// 开关：库值优先；库里没值时 configured 表示环境变量里有（缺省关的开关 configured=false）
const enabled = computed(() => {
  const it = itemOf('cdn_enabled')
  if (!it) return false
  return it.configured && it.value !== 'false'
})
// 还缺哪几项：与后端 storage.CDNProviderNeeds* 同源（utils/cdn 是界面侧同一份事实）
const missingKeys = computed(() => {
  const need = ['cdn_access_key_secret']
  if (!CDN_NO_AK_PROVIDERS.includes(provider.value)) need.push('cdn_access_key_id')
  if (CDN_RESOURCE_PROVIDERS.includes(provider.value)) need.push('cdn_resource_id')
  return need.filter((k) => !itemOf(k)?.configured)
})
const blockReason = computed(() => {
  // 设置还没拉到（或拉取失败）时不下判断：宁可不提示，也不误报「未启用」
  if (!items.value.length) return ''
  if (!enabled.value) {
    return 'CDN 刷新当前未启用，提交会被拒绝。请到「系统设置 → 文件存储与 CDN」开启「启用 CDN 刷新」，选好厂商并填全它要求的凭据，保存后重启服务。'
  }
  if (missingKeys.value.length) {
    return `当前厂商 ${provider.value} 还缺 ${missingKeys.value.map(nameOf).join('、')}，提交会被拒绝。请到「系统设置 → 文件存储与 CDN」补齐后重启服务。`
  }
  return ''
})

function goSettings() {
  router.push('/platform/settings')
}

const urlList = computed(() =>
  urls.value
    .split('\n')
    .map((s) => s.trim())
    .filter(Boolean)
)

async function refresh() {
  if (!urlList.value.length) {
    ElMessage.warning('请至少填写一个 URL')
    return
  }
  loading.value = true
  try {
    await cdnRefresh(urlList.value)
    ElMessage.success(`已提交刷新 ${urlList.value.length} 条`)
  } catch {
    // 拦截器已提示（如 CDN 未配置）
  } finally {
    loading.value = false
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
}
.actions {
  margin-top: 14px;
  display: flex;
  align-items: center;
  gap: 12px;
}
.count {
  color: #909399;
  font-size: 13px;
}
</style>
