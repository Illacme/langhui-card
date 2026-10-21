<template>
  <!-- 首次部署引导：后端进入「初始配置模式」时前端也显示同一份指引。
       只读——不提供任何写接口、不调用任何修改接口，只展示并复制配置文本。 -->
  <div class="sg-wrap">
    <div class="sg-card">
      <h1 class="sg-title">初始配置引导</h1>
      <p class="sg-lead">
        后端已启动，但检测到尚未完成初始配置（<code>DB_DSN</code> 未提供），
        因此暂不加载业务功能。按下面四步补上配置即可。
      </p>

      <h2 class="sg-h2">还差这些配置</h2>
      <ul v-if="status.missing.length" class="sg-list">
        <li v-for="m in status.missing" :key="m.env">
          <code>{{ m.env }}</code> —— {{ m.why }}
        </li>
      </ul>
      <p v-else class="sg-lead">未检测到缺失项。</p>

      <h2 class="sg-h2">可以直接复制这份配置</h2>
      <p class="sg-lead">
        写进 <code>.env</code>（与 <code>docker-compose.yml</code> 同级），然后重启服务。
      </p>
      <pre class="sg-pre"><code>{{ status.envText }}</code></pre>
      <el-button type="primary" size="small" @click="copyEnv">复制配置</el-button>

      <h2 class="sg-h2">接下来</h2>
      <ol class="sg-list">
        <li>复制上面内容，写入 <code>.env</code></li>
        <li>按实际环境改掉数据库主机与口令</li>
        <li>重启服务（<code>docker compose up -d</code> 或重启进程）</li>
        <li>刷新本页：配置完成后会自动进入登录页</li>
      </ol>

      <p class="sg-note">
        本页<b>不写入任何文件、不提供任何修改接口</b>，也不会创建管理员，因此不会被用来重置系统。
        配置源仍然是 <code>.env</code> / 环境变量——这是刻意的安全取舍：数据库地址、签名密钥、Cookie
        策略一旦能在界面里改错，就会把「改回来的入口」一起弄丢。
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ElMessage } from 'element-plus'
import type { SetupStatus } from '@/api/setup'

const props = defineProps<{ status: SetupStatus }>()

async function copyEnv() {
  const text = props.status.envText || ''
  if (!text) return
  try {
    // 非 HTTPS / 无剪贴板权限时 navigator.clipboard 不存在：把内容显示出来供手动复制
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
</script>

<style scoped>
.sg-wrap {
  min-height: 100vh;
  background: #f5f7fa;
  padding: 40px 16px;
  box-sizing: border-box;
}
.sg-card {
  max-width: 760px;
  margin: 0 auto;
  background: #fff;
  border: 1px solid #ebeef5;
  border-radius: 8px;
  padding: 28px 32px;
  color: #303133;
}
.sg-title {
  margin: 0 0 8px;
  font-size: 20px;
}
.sg-h2 {
  margin: 24px 0 8px;
  font-size: 15px;
}
.sg-lead {
  margin: 0;
  color: #606266;
  font-size: 14px;
  line-height: 1.7;
}
.sg-list {
  margin: 8px 0;
  padding-left: 22px;
  font-size: 14px;
  line-height: 1.9;
}
code {
  background: #f5f7fa;
  border: 1px solid #ebeef5;
  border-radius: 4px;
  padding: 1px 6px;
  font-size: 13px;
}
.sg-pre {
  background: #f5f7fa;
  border: 1px solid #ebeef5;
  border-radius: 6px;
  padding: 14px 16px;
  overflow-x: auto;
}
.sg-pre code {
  background: none;
  border: none;
  padding: 0;
  white-space: pre;
}
.sg-note {
  margin-top: 20px;
  padding-top: 14px;
  border-top: 1px dashed #ebeef5;
  font-size: 12px;
  color: #909399;
  line-height: 1.7;
}
.sg-hint {
  margin-top: 8px;
  font-size: 12px;
  color: #909399;
}
@media (max-width: 767px) {
  .sg-card {
    padding: 20px 18px;
  }
}
</style>
