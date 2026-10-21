<template>
  <el-card class="settings-shell" :style="{ '--toolbar-h': toolbarH + 'px' }">
    <div class="page-bar">
      <h3>系统设置</h3>
      <div class="bar-actions">
        <!-- 配置向导：给第一次部署的人一条明确顺序（品牌→存储→上传→微信→小程序→确认），
             不必在 35 个输入框里自己猜该从哪开始、按什么顺序配 -->
        <el-button :disabled="loading" @click="openWizard">配置向导</el-button>
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
      description="敏感项（密钥 / 连接串）不会回传真实值，界面只显示「已配置」；输入框留空表示不修改，填新值即覆盖。带范围 / 格式约束的项（上传上限、接口基址、存储驱动、锁定租户）保存时会校验，非法值直接拒绝并说明原因——与其让你保存后排查半天，不如当场拦住。"
      style="margin-bottom: 16px"
    />

    <!-- 上线就绪检查：把「上线前必须配好的项」从全量里挑出来集中呈现。
         分两类，避免误导：
         - 本页可配置：能点「去配置」直接跳到对应行改；
         - 部署环境变量：只读项，界面改不了，直接给出要设的环境变量（可复制）+ 重启提示。
           （旧版只读项也跳「查看」，落到一个改不了的灰行，等于给了一条死路） -->
    <el-card v-if="!loading && checks.length" shadow="never" class="ready-card">
      <template #header>
        <div class="group-header">
          <div class="group-title-wrap">
            <span class="group-accent" />
            <span class="group-title">上线就绪检查</span>
          </div>
          <span class="group-count">{{ readyCount }} / {{ checks.length }} 项就绪</span>
        </div>
      </template>

      <template v-if="uiChecks.length">
        <div class="ready-section-title">
          本页可配置
          <span class="ready-section-count">{{ uiReadyCount }} / {{ uiChecks.length }}</span>
        </div>
        <div v-for="c in uiChecks" :key="c.label" class="ready-row">
          <span class="ready-badge" :class="c.ok ? 'ok' : 'warn'">{{ c.ok ? '✓' : '!' }}</span>
          <div class="ready-main">
            <div class="ready-label">{{ c.label }}</div>
            <div class="ready-desc">{{ c.ok ? '已就绪' : c.hint || '待配置' }}</div>
          </div>
          <el-button v-if="!c.ok" size="small" text type="primary" @click="jumpTo(c.keys[0])">
            去配置
          </el-button>
        </div>
      </template>

      <template v-if="envChecks.length">
        <div class="ready-section-title">
          部署环境变量
          <span class="ready-section-count">{{ envReadyCount }} / {{ envChecks.length }}</span>
        </div>
        <div v-for="c in envChecks" :key="c.label" class="ready-row">
          <span class="ready-badge" :class="c.ok ? 'ok' : 'warn'">{{ c.ok ? '✓' : '!' }}</span>
          <div class="ready-main">
            <div class="ready-label">{{ c.label }}</div>
            <div class="ready-desc">{{ c.ok ? '已就绪' : c.hint || '待配置' }}</div>
            <div v-if="!c.ok && c.envSample" class="ready-env">
              <code class="ready-env-code">{{ c.envSample }}</code>
              <el-button size="small" text type="primary" @click="copyEnv(c)">复制</el-button>
              <span class="ready-env-tip">改环境变量后重启生效</span>
            </div>
          </div>
        </div>
      </template>

      <!-- 底部入口：向导只覆盖「本页可配置」项，所以只在还有本页项时才给入口 -->
      <div class="ready-foot">
        <span v-if="readyCount < checks.length">
          还有 {{ checks.length - readyCount }} 项上线前要处理：本页可配 {{ uiPending }} 项（点「去配置」直接改）<template
            v-if="envPending"
          >，部署环境变量 {{ envPending }} 项（需在服务器 / 容器设置后重启）</template>。
        </span>
        <span v-else>上线必配项已全部就绪。</span>
        <el-button v-if="uiPending > 0" size="small" type="primary" text @click="openWizard">
          开始配置向导
        </el-button>
      </div>
    </el-card>

    <div v-if="loading" class="loading">加载中…</div>

    <template v-else>
      <!-- 搜索过滤：35 个配置项平铺时「找一个项」本身就是成本。
           中文名 / key / 环境变量都能搜；分组整体命中（如搜「存储」出整组），
           未命中分组整组隐藏。草稿按 key 持有，过滤掉的行不会丢改动。 -->
      <!-- 顶部工具条吸顶：搜索 + 模块锚点导航。滚动时始终可见，
           既保留搜索，又随时给出「当前在哪个模块 / 一键跳到某模块」的入口，
           长列表翻找不再靠肉眼扫标题 -->
      <div ref="toolbarRef" class="toolbar">
        <div class="filter-bar">
          <el-input
            v-model="keyword"
            clearable
            placeholder="搜索配置项：中文名 / key / 环境变量（如「存储」「minio」「JWT_KEY」）"
            class="filter-input"
          />
          <span v-if="keyword.trim()" class="filter-count">匹配 {{ matchCount }} / {{ items.length }} 项</span>
        </div>
        <div v-if="!keyword.trim() || matchCount > 0" class="module-nav">
          <button
            v-for="g in filteredGroups"
            :key="g.key"
            type="button"
            class="module-pill"
            :class="{ active: g.key === activeKey }"
            @click="jumpToGroup(g.key)"
          >{{ g.label }}</button>
        </div>
      </div>
      <div v-if="keyword.trim() && matchCount === 0" class="filter-empty">
        没有匹配「{{ keyword.trim() }}」的配置项——换个关键词，或清空搜索查看全部。
      </div>

      <el-card v-for="g in filteredGroups" :key="g.key" shadow="never" class="group-card" :id="'grp-' + g.key">
        <template #header>
          <div class="group-header">
            <div class="group-title-wrap">
              <span class="group-accent" />
              <span class="group-title">{{ g.label }}</span>
            </div>
            <span class="group-count">{{ g.items.length }} 项</span>
          </div>
          <!-- 分组导语：一句话说清「这组管什么、什么时候该动」，
               商业产品的配置页不能只甩一堆输入框给运营 -->
          <div v-if="g.blurb" class="group-blurb">{{ g.blurb }}</div>
        </template>

        <!-- 单行渲染抽成 SettingRow 组件：主列表与配置向导共用同一形态，
             draft 仍由本页持有（:model-value + @change），避免两处各存一份值 -->
        <SettingRow
          v-for="item in g.items"
          :key="item.key"
          :id="'set-' + item.key"
          :item="item"
          :model-value="draft[item.key]"
          :focus="depState(item.key).focus"
          :dim="depState(item.key).dim"
          @change="setVal"
          @clear="clearSecret"
        />
      </el-card>
    </template>

    <!-- 配置向导：按依赖顺序分步，每步只显示当前相关的项（复用 depState 过滤），
         默认落在「第一个有未配置项的步骤」；草稿与主列表共享，「稍后再说」可原样恢复 -->
    <el-dialog
      v-model="wizardOpen"
      title="配置向导"
      width="720px"
      class="wizard-dialog"
      :close-on-click-modal="false"
    >
      <el-steps :active="wizardStep" align-center class="wizard-steps">
        <el-step v-for="(s, i) in WIZARD_STEPS" :key="s.key" :title="s.title" :status="stepStatus(i)" />
      </el-steps>
      <div class="wizard-progress">第 {{ wizardStep + 1 }} 步 / 共 {{ WIZARD_STEPS.length }} 步</div>
      <div class="wizard-blurb">{{ currentStep.blurb }}</div>
      <div class="wizard-body">
        <SettingRow
          v-for="item in currentVisible"
          :key="item.key"
          :item="item"
          :model-value="draft[item.key]"
          :focus="depState(item.key).focus"
          :dim="depState(item.key).dim"
          @change="setVal"
          @clear="clearSecret"
        />
      </div>
      <div v-if="currentStep.key === 'final'" class="wizard-final">
        <p class="wizard-final-line">
          当前上线就绪 <b>{{ readyCount }} / {{ checks.length }}</b> 项。
          <template v-if="readyCount < checks.length">
            未就绪的多是只读项（数据库连接串 / 签名密钥 / Cookie HTTPS），只能到部署环境改环境变量后重启——界面不开放，是为了防止把「改回来的入口」一起弄丢。
          </template>
          <template v-else>必配项已全部就绪。</template>
        </p>
        <p v-if="wizardRestartKeys.length" class="wizard-final-line">
          本次改动中 <b>{{ wizardRestartKeys.length }}</b> 项需重启服务后生效：{{ wizardRestartKeys.join('、') }}
        </p>
        <p class="wizard-final-line">
          保存后请按部署方式重启进程 / 容器；重启前新存储配置不生效，属预期行为。
        </p>
      </div>
      <template #footer>
        <div class="wizard-foot">
          <el-button text @click="wizardCancel">稍后再说</el-button>
          <div class="wizard-foot-right">
            <el-button v-if="wizardStep > 0" @click="prevStep">上一步</el-button>
            <el-button v-if="!isLastStep" type="primary" @click="nextStep">下一步</el-button>
            <el-button v-else type="primary" :loading="saving" @click="wizardFinish">
              {{ dirty ? '保存并完成' : '完成' }}
            </el-button>
          </div>
        </div>
      </template>
    </el-dialog>
  </el-card>
</template>

<script setup lang="ts">
import { ElMessage, ElMessageBox } from 'element-plus'
import { computed, nextTick, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { getSettings, updateSettings, type SettingItem } from '@/api/settings'
import { useSettingsStore } from '@/store/settings'
import SettingRow from '@/components/SettingRow.vue'

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

// 分组导语：每组一句话「管什么、什么时候该动」，降低首配心智负担。
const GROUP_BLURBS: Record<string, string> = {
  general: '产品的基础展示信息，随时可改、即时生效。',
  miniprogram: '小程序端如何找到后端、是否锁定为单一企业版。首次部署必配「小程序接口基址」。',
  upload: '限制文件大小、类型与临时链接有效期，防止撑爆存储或链接过早失效。',
  wechat: '微信凭据，用于生成小程序码与微信登录；未配置则小程序码相关功能不可用。',
  auth: '后台登录凭证能用多久、防爆破的尝试次数。',
  storage:
    '图片 / 视频 / 小程序码存哪、怎么访问。⚠️ 本组任一项改动都需重启服务；更换存储方式前请确认历史文件仍可访问。',
  security: '数据库连接、签名密钥等关键项，界面只展示，必须到部署环境改环境变量后重启——改错会把自己锁在系统外。',
  feature: '运营期功能开关，按需开启。',
}

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
    blurb: GROUP_BLURBS[k] || '',
    items: map.get(k)!,
  }))
})

// ---- 上线就绪检查 ----
// 把「上线前必须配好的项」从全量里挑出来集中呈现（规则与 scripts/preflight.sh 对齐）。
// 每项默认以「相关 key 均已 configured」为就绪标准；cookie_secure 例外：
// 它是只读开关，就绪标准是值为 true/1（生产 HTTPS）。
interface ReadyCheck {
  label: string
  keys: string[]
  hint?: string
  ok: boolean
  readOnly: boolean
  /** 只读项对应的环境变量名（派生自后端 registry，前端不写死） */
  envVar?: string
  /** 只读项的环境变量示例：界面改不了，必须把这条交给部署的人去设 */
  envSample?: string
}
const checks = computed<ReadyCheck[]>(() => {
  const find = (k: string) => items.value.find((i) => i.key === k)
  const defs: Array<Omit<ReadyCheck, 'ok' | 'readOnly' | 'envVar'>> = [
    {
      label: '登录签名密钥',
      keys: ['jwt_key'],
      hint: '未配置 = 仍用内置缺省值，上线前必须改',
      envSample: 'JWT_KEY=<至少 32 位随机字符串>',
    },
    {
      label: '数据库连接串',
      keys: ['db_dsn'],
      hint: '在部署环境配置后重启',
      envSample: 'DB_DSN=用户名:密码@tcp(主机:3306)/库名?parseTime=true',
    },
    {
      label: '登录 Cookie 强制 HTTPS',
      keys: ['cookie_secure'],
      hint: '生产 HTTPS 环境应为「开」',
      envSample: 'COOKIE_SECURE=true',
    },
    { label: '存储方式与凭据', keys: ['storage_driver'], hint: '演示可用 local；生产建议 minio / oss' },
    { label: '文件访问基址', keys: ['upload_base_url'], hint: '图片 / 视频的公开访问地址，须为 https 完整地址' },
    { label: '微信 AppID / AppSecret', keys: ['wx_appid', 'wx_appsecret'], hint: '小程序码与微信登录依赖，未配则相关功能不可用' },
    { label: '小程序接口基址', keys: ['miniprogram_base_url'], hint: '小程序端冷启动引导地址，须为 https 完整地址' },
  ]
  return defs.map((c) => {
    const first = find(c.keys[0])
    let ok: boolean
    if (c.keys[0] === 'cookie_secure') {
      ok = ['true', '1'].includes((first?.value || '').trim())
    } else {
      ok = c.keys.every((k) => !!find(k)?.configured)
    }
    return { ...c, ok, readOnly: !!first?.readOnly, envVar: first?.envVar }
  })
})
const readyCount = computed(() => checks.value.filter((c) => c.ok).length)
// 分两类呈现：只读项顶着「待配置」却在这一页改不了，会让人以为能点开解决
// （旧版「查看」跳到一个改不了的灰行，等于死路）。拆开后「本页可配置」可点，
// 「部署环境变量」明确要改环境变量并重启。
const uiChecks = computed(() => checks.value.filter((c) => !c.readOnly))
const envChecks = computed(() => checks.value.filter((c) => c.readOnly))
const uiReadyCount = computed(() => uiChecks.value.filter((c) => c.ok).length)
const envReadyCount = computed(() => envChecks.value.filter((c) => c.ok).length)
const uiPending = computed(() => uiChecks.value.length - uiReadyCount.value)
const envPending = computed(() => envChecks.value.length - envReadyCount.value)

// 只读项在界面里改不了，就把「到底要设什么」直接给出来（复制走即可）
function copyEnv(c: ReadyCheck) {
  const text = c.envSample || ''
  if (!text) return
  const ok = () => ElMessage.success('已复制：' + text)
  try {
    // 非 HTTPS / 无剪贴板权限时 navigator.clipboard 不存在：把内容显示出来供手动复制
    if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
      navigator.clipboard.writeText(text).then(ok).catch(() => ElMessage.info(text))
    } else {
      ElMessage.info(text)
    }
  } catch {
    ElMessage.info(text)
  }
}

// 定位到对应配置行（行上锚点 id="set-<key>"）。
// 防御 scrollIntoView 缺失：jsdom 等环境没有该方法（真实浏览器均有），缺省时静默跳过。
function jumpTo(key: string) {
  const el = document.getElementById('set-' + key)
  if (el && typeof el.scrollIntoView === 'function') {
    el.scrollIntoView({ behavior: 'smooth', block: 'center' })
  }
}

// ---- 模块导航 + 滚动联动（让长列表滚动时随时知道「我在哪个模块、一键跳过去」）----
// 顶部工具条吸顶（搜索 + 模块锚点），分组标题也吸顶，二者用 --toolbar-h 衔接。
// 滚动时根据各分组顶部位置算出「当前模块」并高亮对应锚点（scroll-spy）。
const toolbarRef = ref<HTMLElement | null>(null)
// 工具条实时高度：搜索行 + 模块锚点行，窄屏锚点换行会变高，故用 ResizeObserver 动态量
const toolbarH = ref(112)
const activeKey = ref('')
let scrollEl: HTMLElement | null = null
let ro: ResizeObserver | null = null
let scrollHandler: (() => void) | null = null

function measureToolbar() {
  if (toolbarRef.value) toolbarH.value = Math.ceil(toolbarRef.value.getBoundingClientRect().height)
}

// 工具条挂载后才能量高 + 监听尺寸变化（加载时 v-else 未渲染，onMounted 时 ref 还是空）
function attachToolbarObserver() {
  if (typeof ResizeObserver === 'undefined' || !toolbarRef.value || ro) return
  ro = new ResizeObserver(() => {
    measureToolbar()
    computeActive()
  })
  ro.observe(toolbarRef.value)
}

// 当前可视模块 = 顶部越过「工具条底边 + 余量」的最后一个分组
function computeActive() {
  if (!scrollEl) return
  const base = scrollEl.getBoundingClientRect().top
  const line = base + toolbarH.value + 6
  let cur = filteredGroups.value[0]?.key || ''
  for (const g of filteredGroups.value) {
    const el = document.getElementById('grp-' + g.key)
    if (el && el.getBoundingClientRect().top <= line) cur = g.key
  }
  activeKey.value = cur
}

// 点击模块锚点平滑滚动到对应分组；scroll-margin-top 已为吸顶工具条留出空间
function jumpToGroup(key: string) {
  const el = document.getElementById('grp-' + key)
  if (el && typeof el.scrollIntoView === 'function') {
    el.scrollIntoView({ behavior: 'smooth', block: 'start' })
    activeKey.value = key
  }
}

onMounted(() => {
  measureToolbar()
  attachToolbarObserver()
  // 真正的滚动容器是 .admin-main；测试里没有它，缺省跳过滚动监听
  scrollEl = document.querySelector('.admin-main') as HTMLElement | null
  if (scrollEl) {
    scrollHandler = () => computeActive()
    scrollEl.addEventListener('scroll', scrollHandler, { passive: true })
  }
  nextTick(computeActive)
})

onUnmounted(() => {
  ro?.disconnect()
  if (scrollEl && scrollHandler) scrollEl.removeEventListener('scroll', scrollHandler)
})

// 数据加载完、工具条真正挂载后再补一次量高 + 监听（初始 loading=true 时 ref 为空）
watch(loading, (v) => {
  if (!v) nextTick(() => {
    measureToolbar()
    attachToolbarObserver()
    computeActive()
  })
})

const changedKeys = computed(() =>
  items.value
    .filter((it) => !it.readOnly)
    .filter((it) => cleared[it.key] || draft[it.key] !== original[it.key])
    .map((it) => it.key),
)
const dirty = computed(() => changedKeys.value.length > 0)
const changedCount = computed(() => changedKeys.value.length)

// ---- 搜索过滤 ----
// 命中范围：中文名 / key / 描述 / 环境变量名 / 分组中文名。
// 分组中文名参与匹配是特性不是漏洞：搜「存储」的人往往就是想找那一组。
const keyword = ref('')
const filteredGroups = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  if (!kw) return groups.value
  const hit = (it: SettingItem) =>
    [it.name, it.key, it.description, it.envVar, GROUP_LABELS[it.group] || it.group]
      .filter(Boolean)
      .some((s) => s!.toLowerCase().includes(kw))
  return groups.value
    .map((g) => ({ ...g, items: g.items.filter(hit) }))
    .filter((g) => g.items.length > 0)
})
const matchCount = computed(() => filteredGroups.value.reduce((n, g) => n + g.items.length, 0))

// 搜索过滤后可见分组变化，重新判定当前模块（默认落到第一个）
watch(filteredGroups, () => nextTick(computeActive))

// ---- 破坏性更改二次确认 ----
// 这些改动会让「已经存下来的文件」失联、或把「已配好的凭据」清掉回退到
// 可能并不存在的环境变量上。分组导语里的警告只配看，这里的确认逼一次清醒决策：
// 误确认只是一次多余点击，漏确认可能就是一批文件打不开 + 工单。
const DESTRUCTIVE: Record<string, string> = {
  storage_driver: '存储方式变更后，按原方式上传的历史文件可能无法访问（请先确认历史文件仍可访问）',
  upload_dir: '本地上传目录变更后，旧目录中的已有文件将不再可见',
  storage_public: '存储空间公开访问变更会影响所有已生成文件链接的可访问范围',
}
function destructiveWarnings(): string[] {
  const warns: string[] = []
  changedKeys.value.forEach((k) => {
    const it = items.value.find((i) => i.key === k)
    if (!it) return
    const label = it.name || k
    if (it.secret && cleared[k]) {
      warns.push(`「${label}」将被清空并回退环境变量；若部署环境没有对应变量，相关功能会直接失效`)
    } else if (DESTRUCTIVE[k]) {
      warns.push(`「${label}」：${DESTRUCTIVE[k]}`)
    }
  })
  return warns
}

// ---- 依赖联动 ----
// 以下拉 / 开关的当前草稿值为准：用户刚切换就能看到哪些项「转亮 / 置灰」，不必先保存再刷新。
const driverVal = computed(() => (draft.storage_driver || '').trim() || 'local')
const cdnOn = computed(() => (draft.cdn_enabled || '') === 'true')
const wxAppidFilled = computed(
  () => !!(draft.wx_appid || '').trim() || !!items.value.find((i) => i.key === 'wx_appid')?.configured,
)

// local 只关心「文件落盘目录」；minio / oss 只关心端点 / 桶 / 凭据。
// 两类项互斥出现，避免对着一堆当前选项下用不上的输入框。
const CLOUD_ONLY = [
  'storage_endpoint',
  'storage_bucket',
  'storage_access_key',
  'storage_secret_key',
  'storage_use_ssl',
]
const CDN_ONLY = ['cdn_access_key_id', 'cdn_access_key_secret', 'cdn_endpoint']

interface DepState {
  focus: boolean
  dim: boolean
}
function depState(key: string): DepState {
  if (key === 'upload_dir') {
    return { focus: driverVal.value === 'local', dim: driverVal.value !== 'local' }
  }
  if (CLOUD_ONLY.includes(key)) {
    return { focus: driverVal.value !== 'local', dim: driverVal.value === 'local' }
  }
  if (CDN_ONLY.includes(key)) {
    return { focus: cdnOn.value, dim: !cdnOn.value }
  }
  if (key === 'wx_appsecret') {
    return { focus: wxAppidFilled.value, dim: !wxAppidFilled.value }
  }
  return { focus: false, dim: false }
}

// ---- 首次配置向导 ----
// 35 个输入框平铺给第一次部署的人，最大的问题不是「找不到」，而是「不知道从哪开始、按什么顺序」。
// 向导按依赖顺序分 6 步，每步只显示当前相关的项（复用 depState 过滤），
// 默认落在「第一个有未配置项的步骤」——已配好的步骤直接跳过，不让人重复确认。
interface WizardStep {
  key: string
  title: string
  blurb: string
  /** 候选 key；条件显隐的按 depState 过滤，后端缺省的（旧版本）自动跳过 */
  keys: string[]
}
const WIZARD_STEPS: WizardStep[] = [
  {
    key: 'brand',
    title: '品牌与身份',
    blurb: '先定品牌名：后台侧边栏、登录页与小程序都会展示它。随时可改、即时生效。',
    keys: ['brand_name'],
  },
  {
    key: 'storage',
    title: '存储与上传',
    blurb: '图片 / 视频 / 小程序码存哪、怎么访问。演示选 local 即可；生产建议 minio / oss，并配上公开访问基址。本组改动需重启服务。',
    keys: [
      'storage_driver',
      'upload_dir',
      'storage_endpoint',
      'storage_bucket',
      'storage_access_key',
      'storage_secret_key',
      'storage_use_ssl',
      'upload_base_url',
    ],
  },
  {
    key: 'limits',
    title: '上传限制',
    blurb: '限制单文件大小与允许的类型，防止撑爆存储。默认值已可用，按需调整。',
    keys: ['upload_max_mb', 'upload_allow_ext', 'upload_video_max_mb'],
  },
  {
    key: 'wechat',
    title: '微信凭据',
    blurb: '小程序码与微信登录依赖微信 AppID / AppSecret。没有微信业务可先跳过，相关功能不可用而已。',
    keys: ['wx_appid', 'wx_appsecret'],
  },
  {
    key: 'miniprogram',
    title: '小程序接入',
    blurb: '小程序端冷启动第一个请求打到哪。必须是小程序能访问到的公网 https 地址，填错小程序直接连不上。',
    keys: ['miniprogram_base_url', 'pinned_company_id'],
  },
  {
    key: 'final',
    title: '安全确认',
    blurb: '最后核对一遍：哪些改了要重启、哪些只能改环境变量。确认无误再保存。',
    keys: [],
  },
]

// 只读项不进向导：它们本来就只能改环境变量，放进来只会给人「这里能配」的错觉
function stepVisible(s: WizardStep): SettingItem[] {
  return s.keys
    .map((k) => items.value.find((i) => i.key === k))
    .filter((it): it is SettingItem => !!it && !it.readOnly && !depState(it.key).dim)
}
function stepDone(s: WizardStep): boolean {
  return s.key !== 'final' && stepVisible(s).every((it) => it.configured)
}
const firstUnfinishedStep = computed(() => {
  const idx = WIZARD_STEPS.findIndex((s) => !stepDone(s))
  return idx < 0 ? WIZARD_STEPS.length - 1 : idx
})

const WIZARD_SEEN_KEY = 'settings_wizard_seen'
const wizardOpen = ref(false)
const wizardStep = ref(0)
// 打开向导时快照草稿：「稍后再说」要能原样恢复，
// 不能把用户手滑改的半截状态留在主列表里（那会变成一次意外的未保存改动）
let wizardSnapshot: Record<string, string> = {}
let wizardClearedSnapshot: Record<string, boolean> = {}

const currentStep = computed(() => WIZARD_STEPS[wizardStep.value])
const currentVisible = computed(() => stepVisible(currentStep.value))
const isLastStep = computed(() => wizardStep.value === WIZARD_STEPS.length - 1)
const wizardRestartKeys = computed(() =>
  changedKeys.value.filter((k) => items.value.find((i) => i.key === k)?.restart),
)
function stepStatus(i: number): 'success' | 'process' | 'wait' {
  if (i === wizardStep.value) return 'process'
  return stepDone(WIZARD_STEPS[i]) ? 'success' : 'wait'
}
function wizardSeen(): boolean {
  try {
    return !!localStorage.getItem(WIZARD_SEEN_KEY)
  } catch {
    // 隐私模式等读不了的场景：不当「没看过」，避免每次刷新都自动弹
    return true
  }
}
function markWizardSeen() {
  try {
    localStorage.setItem(WIZARD_SEEN_KEY, '1')
  } catch {
    // 写不进只影响「不再自动弹」，向导仍可手动打开，静默跳过即可
  }
}
function openWizard() {
  wizardSnapshot = { ...draft }
  wizardClearedSnapshot = { ...cleared }
  wizardStep.value = firstUnfinishedStep.value
  wizardOpen.value = true
}
function prevStep() {
  if (wizardStep.value > 0) wizardStep.value--
}
function nextStep() {
  if (wizardStep.value < WIZARD_STEPS.length - 1) wizardStep.value++
}
function wizardCancel() {
  Object.assign(draft, wizardSnapshot)
  Object.assign(cleared, wizardClearedSnapshot)
  markWizardSeen()
  wizardOpen.value = false
}
async function wizardFinish() {
  markWizardSeen()
  // 一步没改就直接关：不让「完成」按钮制造一次空保存的等待
  if (!dirty.value) {
    wizardOpen.value = false
    return
  }
  if (await save()) wizardOpen.value = false
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
      // 全新部署（一项都没就绪、也没主动关过向导）自动拉起向导：
      // 否则用户第一次打开设置页，看到的只是一片空输入框，不知道从哪开始。
      if (readyCount.value === 0 && !wizardSeen()) {
        openWizard()
      }
    }
  } catch {
    // 拦截器已提示（如非平台超管越权）
  } finally {
    loading.value = false
  }
}

// 返回是否保存成功：向导的「保存并完成」据此决定是否关闭对话框
async function save(): Promise<boolean> {
  if (!dirty.value) return true
  // 破坏性更改先过确认闸门：取消则整个保存作废（草稿保留，可改回去或再次确认）
  const warns = destructiveWarnings()
  if (warns.length > 0) {
    try {
      await ElMessageBox.confirm(
        `以下更改可能影响已有数据：${warns.join('；')}。确认保存吗？`,
        '破坏性更改确认',
        { type: 'warning', confirmButtonText: '确认保存', cancelButtonText: '取消' },
      )
    } catch {
      return false
    }
  }
  saving.value = true
  const payload: Record<string, string> = {}
  changedKeys.value.forEach((k) => {
    payload[k] = cleared[k] ? '' : draft[k]
  })
  // 先记下本次保存了几项：下面的乐观推进会把草稿变成「无变更」，
  // 事后再读 changedCount 只会得到 0（提示语会说「已保存 0 项」）。
  const savedCount = Object.keys(payload).length
  try {
    // 后端逐条校验类型，任一非法即返回 code:1（拦截器已弹错误提示并 reject）
    const r = await updateSettings(payload)
    // 成功后把「原值」推进到新值，按钮回到禁用态
    Object.keys(payload).forEach((k) => {
      const wasCleared = cleared[k]
      cleared[k] = false
      const it = items.value.find((x) => x.key === k)
      // 乐观推进 configured：保存成功即反映到「待配置」标记与就绪统计上，
      // 不用整页刷新（刷新会丢掉其它未保存改动）
      if (it) it.configured = !wasCleared
      // 敏感项保存后输入框回到空（下发的仍是掩码），原值也保持空，避免显示成「已修改」
      original[k] = it?.secret ? '' : payload[k]
      draft[k] = original[k]
    })
    // 品牌名变了要立刻同步侧边栏展示，避免用户以为没生效
    if (payload.brand_name !== undefined) {
      settings.setBrandName(payload.brand_name || '')
    }
    const restart = r?.data?.restartKeys || []
    if (restart.length > 0) {
      ElMessage.warning(`已保存 ${savedCount} 项；${restart.length} 项需重启服务后生效：${restart.join('、')}`)
    } else {
      ElMessage.success(`已保存 ${savedCount} 项设置`)
    }
    return true
  } catch {
    // 失败保留草稿，方便用户修正后重试
    return false
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
/* 外壳与分组卡片放开 overflow：默认 el-card / el-card__body 的 overflow 会让 sticky
   被卡片自身「夹住」而失效，必须让吸顶相对真正滚动的 .admin-main 生效 */
.settings-shell {
  overflow: visible;
}
.settings-shell :deep(.el-card__body) {
  overflow: visible;
}
.group-card {
  overflow: visible;
  /* 模块锚点跳转时为吸顶工具条留出空间，避免标题被工具条盖住 */
  scroll-margin-top: calc(var(--toolbar-h, 112px) + 4px);
}
/* 分组标题吸顶：滚到本组内容时标题停在工具条下方，始终知道当前在哪个模块 */
.group-card :deep(.el-card__header) {
  position: sticky;
  top: calc(var(--toolbar-h, 112px) + 2px);
  z-index: 4;
  background: #fff;
  border-bottom: 1px solid #ebeef5;
}
.group-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.group-title-wrap {
  display: flex;
  align-items: center;
  gap: 10px;
}
.group-accent {
  flex: none;
  width: 4px;
  height: 16px;
  border-radius: 2px;
  background: #409eff;
}
.group-title {
  font-size: 16px;
  font-weight: 700;
  color: #303133;
}
.group-count {
  flex: none;
  font-size: 12px;
  color: #409eff;
  background: #ecf5ff;
  border: 1px solid #d9ecff;
  border-radius: 999px;
  padding: 2px 10px;
}
.group-blurb {
  margin-top: 6px;
  font-size: 12px;
  color: #909399;
  line-height: 1.6;
}
/* 顶部工具条：搜索 + 模块锚点导航，吸顶常驻 */
.toolbar {
  position: sticky;
  top: 0;
  z-index: 6;
  background: #fff;
  border-bottom: 1px solid #ebeef5;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.05);
  padding: 12px 0 4px;
  margin-bottom: 14px;
}
/* 模块锚点导航：横向排开所有模块，超出可横滑；当前模块高亮 */
.module-nav {
  display: flex;
  gap: 8px;
  margin-top: 12px;
  overflow-x: auto;
  padding-bottom: 6px;
  scrollbar-width: thin;
}
.module-nav::-webkit-scrollbar {
  height: 6px;
}
.module-nav::-webkit-scrollbar-thumb {
  background: #dcdfe6;
  border-radius: 3px;
}
.module-pill {
  flex: none;
  border: 1px solid #dcdfe6;
  background: #fff;
  color: #606266;
  font-size: 13px;
  line-height: 1;
  padding: 7px 12px;
  border-radius: 999px;
  cursor: pointer;
  white-space: nowrap;
  transition: all 0.15s;
}
.module-pill:hover {
  color: #409eff;
  border-color: #c6e2ff;
  background: #ecf5ff;
}
.module-pill.active {
  color: #fff;
  background: #409eff;
  border-color: #409eff;
}
/* 上线就绪面板 */
.ready-card {
  margin-bottom: 14px;
  border-radius: 8px;
}
.ready-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 0;
  border-bottom: 1px solid #f2f6fc;
}
.ready-row:last-child {
  border-bottom: none;
}
.ready-badge {
  flex: none;
  width: 20px;
  height: 20px;
  border-radius: 50%;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  font-weight: 700;
  color: #fff;
}
.ready-badge.ok {
  background: #67c23a;
}
.ready-badge.warn {
  background: #e6a23c;
}
.ready-main {
  flex: 1;
  min-width: 0;
}
.ready-label {
  font-size: 13px;
  font-weight: 600;
  color: #303133;
}
/* 就绪检查按「本页可配置 / 部署环境变量」分节 */
.ready-section-title {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 10px 0 2px;
  font-size: 12px;
  font-weight: 600;
  color: #606266;
}
.ready-section-count {
  font-weight: 400;
  color: #909399;
}
/* 只读项：给出要设的环境变量（可复制），不再跳到一个改不了的灰行 */
.ready-env {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 6px;
  flex-wrap: wrap;
}
.ready-env-code {
  font-size: 12px;
  color: #303133;
  background: #f5f7fa;
  border: 1px solid #ebeef5;
  border-radius: 4px;
  padding: 3px 8px;
  word-break: break-all;
}
.ready-env-tip {
  font-size: 12px;
  color: #909399;
}
.ready-desc {
  margin-top: 2px;
  font-size: 12px;
  color: #909399;
  line-height: 1.5;
}
.ready-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 10px;
  padding-top: 10px;
  border-top: 1px dashed #ebeef5;
  font-size: 12px;
  color: #909399;
  line-height: 1.6;
}
/* 搜索过滤 */
.filter-bar {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 14px;
}
.filter-input {
  max-width: 460px;
}
.filter-count {
  font-size: 12px;
  color: #909399;
  white-space: nowrap;
}
.filter-empty {
  padding: 24px 0;
  text-align: center;
  font-size: 13px;
  color: #909399;
}
/* 配置向导 */
.wizard-steps {
  margin-bottom: 4px;
}
.wizard-progress {
  text-align: center;
  font-size: 12px;
  color: #909399;
  margin: 8px 0 4px;
}
.wizard-blurb {
  font-size: 12px;
  color: #909399;
  line-height: 1.6;
  margin-bottom: 8px;
}
.wizard-body {
  max-height: 46vh;
  overflow-y: auto;
}
.wizard-final {
  margin-top: 10px;
  padding-top: 10px;
  border-top: 1px dashed #ebeef5;
}
.wizard-final-line {
  margin: 8px 0 0;
  font-size: 12px;
  color: #606266;
  line-height: 1.7;
}
.wizard-foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.wizard-foot-right {
  display: flex;
  gap: 8px;
}
</style>
