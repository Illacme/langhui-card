<template>
  <!-- 安装向导：首次部署时用表单完成「连库 → 建超管 → 写配置 → 自重启 → 进登录页」。
       与后端 /setup 内置页是同一套接口（/api/setup/test、/api/setup/install），
       区别只在外壳：后端页是纯静态自包含 HTML，前端页走 Element Plus 风格，
       生产由 nginx 托管时运维看到的是这一页。 -->
  <div class="si-wrap">
    <div class="si-card">
      <div class="si-brand">朗汇机电 · 电子名片</div>
      <h1 class="si-title">安装向导</h1>
      <p class="si-lead">
        首次部署需要两步：连接数据库、创建管理员账号。保存后服务会自动重启，随后即可登录使用。
      </p>

      <ol class="si-steps">
        <li :class="{ active: step >= 1 }"><b>1</b><span>数据库</span></li>
        <li :class="{ active: step >= 2 }"><b>2</b><span>管理员账号</span></li>
        <li :class="{ active: step >= 3 }"><b>3</b><span>完成</span></li>
      </ol>

      <!-- ---------- 表单 ---------- -->
      <div v-show="stage === 'form'">
        <section class="si-block">
          <h2 class="si-h2">选择数据库</h2>
          <div class="si-engines">
            <div
              class="si-engine"
              :class="{ active: engine === 'mysql' }"
              @click="engine = 'mysql'"
            >
              <b>MySQL</b>
              <p>适合生产与多人使用。需要一台可访问的 MySQL 服务，docker compose 部署已内置。</p>
            </div>
            <div
              class="si-engine"
              :class="{ active: engine === 'sqlite' }"
              @click="engine = 'sqlite'"
            >
              <b>SQLite</b>
              <p>零依赖单机版，数据即一个文件。适合试用与小规模内部使用；不支持多副本并发写，请勿放在网络盘。</p>
            </div>
          </div>

          <div v-show="engine === 'mysql'" class="si-grid">
            <label>主机<el-input v-model="form.dbHost" autocomplete="off" /></label>
            <label>端口<el-input v-model="form.dbPort" autocomplete="off" /></label>
            <label>库名<el-input v-model="form.dbName" autocomplete="off" /></label>
            <label>用户名<el-input v-model="form.dbUser" autocomplete="off" /></label>
            <label>密码<el-input v-model="form.dbPassword" type="password" show-password autocomplete="new-password" /></label>
            <div></div>
            <el-checkbox v-model="form.createDatabase" class="si-chk">
              数据库不存在时自动创建
            </el-checkbox>
            <p v-if="mysqlNote" class="si-tip si-tip--hint">{{ mysqlNote }}</p>
          </div>

          <div v-show="engine === 'sqlite'" class="si-grid">
            <label class="wide">数据文件路径<el-input v-model="form.sqlitePath" autocomplete="off" /></label>
            <p class="si-tip">相对路径相对于服务进程的工作目录；目录不存在时会自动创建。</p>
          </div>

          <p class="si-dsn">连接串预览：<code>{{ dsnPreview }}</code></p>
        </section>

        <section class="si-block">
          <h2 class="si-h2">管理员账号</h2>
          <div class="si-grid">
            <label>用户名<el-input v-model="form.adminUser" autocomplete="off" /></label>
            <label>密码<el-input v-model="form.adminPass" type="password" show-password autocomplete="new-password" /></label>
            <label>确认密码<el-input v-model="form.adminPass2" type="password" show-password autocomplete="new-password" /></label>
          </div>
          <p class="si-tip">平台超管账号。登录后可在后台右上角「修改密码」随时更换。</p>
        </section>

        <details class="si-block si-details">
          <summary>高级选项</summary>
          <div class="si-grid">
            <label class="wide">JWT 密钥（留空由服务端自动生成）<el-input v-model="form.jwtKey" autocomplete="off" /></label>
            <el-checkbox v-model="form.cookieSecure" class="si-chk">
              Cookie 强制 HTTPS（站点启用 https 后再勾选）
            </el-checkbox>
          </div>
        </details>

        <el-alert
          v-if="alertMsg"
          :type="alertType"
          :title="alertMsg"
          :closable="false"
          show-icon
          class="si-alert"
        />

        <div class="si-actions">
          <el-button :loading="testing" @click="onTest">测试连接</el-button>
          <el-button type="primary" :loading="installing" @click="onInstall">开始安装</el-button>
        </div>

        <div v-if="confirming" class="si-confirm">
          <p>确认按以下配置安装？目标数据库将被初始化，管理员账号按上面填写的内容创建。此操作只需一次。</p>
          <p class="si-dsn"><code>{{ dsnPreview }}</code></p>
          <div class="si-actions">
            <el-button @click="confirming = false">取消</el-button>
            <el-button type="danger" :loading="installing" @click="doInstall">确认安装</el-button>
          </div>
        </div>
      </div>

      <!-- ---------- 进度 ---------- -->
      <section v-show="stage === 'progress'" class="si-block">
        <h2 class="si-h2">安装进度</h2>
        <ul class="si-psteps">
          <li :class="pClass(1)"><i></i><span>写入配置并初始化数据库</span></li>
          <li :class="pClass(2)"><i></i><span>重启服务并加载配置</span></li>
          <li :class="pClass(3)"><i></i><span>完成，进入登录页</span></li>
        </ul>
        <p class="si-tip">{{ progressTip }}</p>
        <div class="si-actions">
          <el-button v-if="showRecheck" @click="recheck">重新检测</el-button>
        </div>
      </section>

      <!-- ---------- 手动配置兜底 ---------- -->
      <details class="si-block si-details">
        <summary>手动配置（不使用安装向导）</summary>
        <p class="si-tip">
          把下面内容写进 <code>.env</code>（与 <code>docker-compose.yml</code> 同级）后重启。
          优先级：环境变量 &gt; <code>{{ configFile }}</code> &gt; 内置缺省值。
        </p>
        <pre class="si-pre"><code>{{ props.status.envText }}</code></pre>
        <el-button size="small" @click="copyEnv">复制配置</el-button>
      </details>

      <p class="si-note">
        安全说明：安装向导仅在首次配置前可用，安装一次后即关闭；不会覆盖已有数据的数据库；
        配置以 0600 权限写入 <code>{{ configFile }}</code>；响应与日志中不出现明文口令。
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import {
  fetchSetupStatus,
  install as runInstall,
  testConnection,
  type InstallPayload,
  type SetupStatus,
} from '@/api/setup'

const props = defineProps<{ status: SetupStatus }>()

const engine = ref<'mysql' | 'sqlite'>('mysql')
const stage = ref<'form' | 'progress'>('form')
const testing = ref(false)
const installing = ref(false)
const confirming = ref(false)
const alertMsg = ref('')
const alertType = ref<'success' | 'error'>('error')
const progressTip = ref('')
const showRecheck = ref(false)

// 安装进度：0=未开始，1..3 已完成到哪一步；'fail' 记失败
const done = ref(0)
const running = ref(0)
const failed = ref(0)

const form = reactive({
  dbHost: '127.0.0.1',
  dbPort: '3306',
  dbName: 'langhui',
  dbUser: 'root',
  dbPassword: '',
  createDatabase: true,
  sqlitePath: '',
  adminUser: 'admin',
  adminPass: '',
  adminPass2: '',
  jwtKey: '',
  cookieSecure: false,
})

const configFile = computed(() => props.status.configFile || './data/runtime.env')
const mysqlNote = computed(() => props.status.mysqlHint?.note || '')
const step = computed(() => (stage.value === 'progress' ? 3 : 1))

// 预览与服务端掩码口径一致：只用于给运维确认「填进去的是什么」，密码一律打星
const dsnPreview = computed(() => {
  if (engine.value === 'mysql') {
    const pw = form.dbPassword ? '******' : ''
    return `${form.dbUser}:${pw}@tcp(${form.dbHost}:${form.dbPort || '3306'})/${form.dbName}?charset=utf8mb4&parseTime=true`
  }
  return `sqlite://${form.sqlitePath}`
})

function payload(): InstallPayload {
  const base = {
    adminUser: form.adminUser.trim(),
    adminPass: form.adminPass,
    adminPass2: form.adminPass2,
    jwtKey: form.jwtKey.trim(),
    cookieSecure: form.cookieSecure,
  }
  if (engine.value === 'mysql') {
    return {
      ...base,
      engine: 'mysql',
      dbHost: form.dbHost.trim(),
      dbPort: form.dbPort.trim(),
      dbUser: form.dbUser.trim(),
      dbPassword: form.dbPassword,
      dbName: form.dbName.trim(),
      createDatabase: form.createDatabase,
    }
  }
  return { ...base, engine: 'sqlite', sqlitePath: form.sqlitePath.trim() }
}

function setAlert(type: 'success' | 'error', msg: string) {
  alertType.value = type
  alertMsg.value = msg
}
function clearAlert() {
  alertMsg.value = ''
}

// 客户端预检与服务端同规则，只为少一次往返；真正的判据在服务端
function precheck(): string {
  if (engine.value === 'mysql') {
    if (!form.dbHost.trim() || !form.dbUser.trim() || !form.dbName.trim()) {
      return '请填写完整的 MySQL 连接信息（主机、用户名、库名）'
    }
  } else if (!form.sqlitePath.trim()) {
    return '请填写 SQLite 数据文件路径'
  }
  if (form.adminPass.length < 8) return '管理员密码至少 8 位'
  if (form.adminPass !== form.adminPass2) return '两次输入的管理员密码不一致'
  if (!form.adminUser.trim()) return '请填写管理员用户名'
  return ''
}

async function onTest() {
  clearAlert()
  testing.value = true
  try {
    const r = await testConnection(payload())
    setAlert('success', `连接正常（${r.driver}）${r.note ? '：' + r.note : ''}`)
  } catch (e: any) {
    setAlert('error', e?.message || '连接失败')
  } finally {
    testing.value = false
  }
}

function onInstall() {
  clearAlert()
  const err = precheck()
  if (err) {
    setAlert('error', err)
    return
  }
  confirming.value = true
}

async function doInstall() {
  clearAlert()
  installing.value = true
  try {
    const r = await runInstall(payload())
    confirming.value = false
    startProgress(r.restarted)
  } catch (e: any) {
    setAlert('error', e?.message || '安装失败')
  } finally {
    installing.value = false
  }
}

function pClass(n: number): string {
  if (failed.value === n) return 'fail'
  if (done.value >= n) return 'done'
  if (running.value === n) return 'run'
  return ''
}

function startProgress(restarted: boolean) {
  stage.value = 'progress'
  done.value = 1
  if (restarted) {
    running.value = 2
    progressTip.value = '服务正在重启，请稍候…'
    poll()
  } else {
    failed.value = 2
    progressTip.value = '服务未能自动重启（Windows 或容器托管环境），请手动重启服务后点击下方按钮重新检测。'
    showRecheck.value = true
  }
}

// 轮询直到后端说 configured=true。请求失败（重启瞬间连接被拒）属预期，继续等：
// 这里必须用会抛错的 fetchSetupStatus，用折叠成 null 的 getSetupStatus 会把
// 「重启中」误判成「装好了」。
function poll() {
  let tries = 0
  const timer = window.setInterval(async () => {
    tries++
    try {
      const s = await fetchSetupStatus()
      if (s.configured) {
        window.clearInterval(timer)
        done.value = 2
        running.value = 3
        countdown()
      }
    } catch {
      /* 重启瞬间连不上是正常的，继续轮询 */
    }
    if (tries >= 50) {
      window.clearInterval(timer)
      failed.value = 2
      progressTip.value = '等待服务重启超时，请手动刷新页面或重启服务。'
      showRecheck.value = true
    }
  }, 1200)
}

function countdown() {
  let n = 3
  progressTip.value = `安装完成，${n} 秒后进入登录页…`
  const t = window.setInterval(() => {
    n--
    if (n <= 0) {
      window.clearInterval(t)
      // 整页重载：main.ts 会重新查状态，这次 configured=true → 挂载正常应用
      window.location.href = '/'
      return
    }
    progressTip.value = `安装完成，${n} 秒后进入登录页…`
  }, 1000)
}

function recheck() {
  window.location.reload()
}

async function copyEnv() {
  const text = props.status.envText || ''
  if (!text) return
  try {
    if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text)
      ElMessage.success('已复制，粘贴进 .env 即可')
    } else {
      ElMessage.info(text)
    }
  } catch {
    ElMessage.info(text)
  }
}

onMounted(() => {
  const s = props.status
  if (s.mysqlHint) {
    form.dbHost = s.mysqlHint.host || form.dbHost
    form.dbPort = s.mysqlHint.port || form.dbPort
    form.dbUser = s.mysqlHint.user || form.dbUser
    form.dbName = s.mysqlHint.database || form.dbName
  }
  if (s.sqlitePath) form.sqlitePath = s.sqlitePath
  if (s.adminUser) form.adminUser = s.adminUser
  // 编排环境预填了 MySQL 连接信息，说明 compose 里就带 MySQL，默认选中它
  if (s.mysqlHint) engine.value = 'mysql'
})
</script>

<style scoped>
.si-wrap {
  min-height: 100vh;
  background: #f5f7fa;
  padding: 40px 16px;
  box-sizing: border-box;
}
.si-card {
  max-width: 760px;
  margin: 0 auto;
  background: #fff;
  border: 1px solid #ebeef5;
  border-radius: 8px;
  padding: 28px 32px;
  color: #303133;
}
.si-brand {
  font-size: 13px;
  color: #909399;
  letter-spacing: 0.5px;
}
.si-title {
  margin: 6px 0 8px;
  font-size: 22px;
}
.si-lead {
  margin: 0;
  color: #606266;
  font-size: 14px;
  line-height: 1.7;
}
.si-steps {
  list-style: none;
  display: flex;
  gap: 10px;
  margin: 18px 0;
  padding: 0;
}
.si-steps li {
  flex: 1;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 12px;
  border: 1px solid #ebeef5;
  border-radius: 8px;
  background: #fff;
  color: #909399;
  font-size: 13px;
}
.si-steps li b {
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: #f0f2f5;
  color: #909399;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  flex: none;
}
.si-steps li.active {
  border-color: #409eff;
  color: #303133;
}
.si-steps li.active b {
  background: #409eff;
  color: #fff;
}
.si-block {
  border: 1px solid #ebeef5;
  border-radius: 8px;
  padding: 20px;
  margin-bottom: 16px;
}
.si-block.si-details {
  padding: 0;
}
.si-block.si-details summary {
  cursor: pointer;
  padding: 14px 20px;
  font-size: 14px;
  font-weight: 500;
  list-style: none;
}
.si-block.si-details summary::before {
  content: '▸';
  margin-right: 8px;
  color: #909399;
}
.si-block.si-details[open] summary::before {
  content: '▾';
}
.si-block.si-details > *:not(summary) {
  margin-left: 20px;
  margin-right: 20px;
  margin-bottom: 14px;
}
.si-block.si-details > summary {
  margin-bottom: 0;
}
.si-h2 {
  margin: 0 0 14px;
  font-size: 15px;
}
.si-engines {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
  margin-bottom: 16px;
}
.si-engine {
  border: 1px solid #ebeef5;
  border-radius: 8px;
  padding: 14px 16px;
  cursor: pointer;
  background: #fff;
}
.si-engine:hover {
  border-color: #c0d9f5;
}
.si-engine b {
  display: block;
  font-size: 15px;
  margin-bottom: 6px;
}
.si-engine p {
  margin: 0;
  font-size: 12px;
  color: #909399;
  line-height: 1.6;
}
.si-engine.active {
  border-color: #409eff;
  box-shadow: 0 0 0 1px #409eff inset;
}
.si-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px 16px;
  align-items: start;
}
.si-grid label {
  display: flex;
  flex-direction: column;
  gap: 6px;
  font-size: 13px;
  color: #606266;
}
.si-grid .wide {
  grid-column: 1 / -1;
}
.si-chk {
  grid-column: 1 / -1;
  height: auto;
  font-size: 13px;
  color: #606266;
}
.si-tip {
  margin: 0;
  font-size: 12px;
  color: #909399;
  line-height: 1.7;
  grid-column: 1 / -1;
}
.si-tip--hint {
  color: #e6a23c;
}
.si-dsn {
  margin: 14px 0 0;
  font-size: 12px;
  color: #606266;
  word-break: break-all;
}
code {
  background: #f5f7fa;
  border: 1px solid #ebeef5;
  border-radius: 4px;
  padding: 2px 8px;
  font-size: 12px;
}
.si-pre {
  background: #f5f7fa;
  border: 1px solid #ebeef5;
  border-radius: 6px;
  padding: 14px 16px;
  overflow-x: auto;
}
.si-pre code {
  background: none;
  border: none;
  padding: 0;
  white-space: pre;
}
.si-alert {
  margin-top: 4px;
}
.si-actions {
  display: flex;
  gap: 12px;
  justify-content: flex-end;
  margin-top: 18px;
}
.si-confirm {
  margin-top: 16px;
  padding: 14px 16px;
  border: 1px solid #fde2e2;
  background: #fefafa;
  border-radius: 8px;
}
.si-confirm p {
  margin: 0 0 8px;
  font-size: 13px;
  color: #606266;
  line-height: 1.7;
}
.si-psteps {
  list-style: none;
  margin: 0;
  padding: 0;
}
.si-psteps li {
  display: flex;
  align-items: center;
  padding: 10px 0;
  font-size: 14px;
  color: #909399;
  border-bottom: 1px dashed #f0f2f5;
}
.si-psteps li:last-child {
  border-bottom: none;
}
.si-psteps li.run,
.si-psteps li.done {
  color: #303133;
}
.si-psteps i {
  display: inline-block;
  width: 16px;
  height: 16px;
  border-radius: 50%;
  border: 2px solid #dcdfe6;
  margin-right: 10px;
  flex: none;
  position: relative;
}
.si-psteps li.done i {
  background: #67c23a;
  border-color: #67c23a;
}
.si-psteps li.done i::after {
  content: '';
  position: absolute;
  left: 4px;
  top: 1px;
  width: 4px;
  height: 8px;
  border: solid #fff;
  border-width: 0 2px 2px 0;
  transform: rotate(45deg);
}
.si-psteps li.run i {
  border-color: #409eff;
  border-top-color: transparent;
  animation: si-spin 0.8s linear infinite;
}
.si-psteps li.fail i {
  background: #f56c6c;
  border-color: #f56c6c;
}
@keyframes si-spin {
  to {
    transform: rotate(360deg);
  }
}
.si-note {
  margin: 20px 0 0;
  font-size: 12px;
  color: #909399;
  line-height: 1.8;
}
@media (max-width: 767px) {
  .si-wrap {
    padding: 20px 12px;
  }
  .si-card {
    padding: 20px 18px;
  }
  .si-steps {
    flex-direction: column;
  }
  .si-engines,
  .si-grid {
    grid-template-columns: 1fr;
  }
  .si-actions {
    flex-direction: column;
  }
}
</style>
