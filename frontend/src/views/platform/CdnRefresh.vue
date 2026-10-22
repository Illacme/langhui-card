<template>
  <el-card>
    <div class="page-bar">
      <h3>CDN 刷新</h3>
    </div>

    <el-alert
      type="info"
      :closable="false"
      title="覆盖同名文件后，用它让 CDN 边缘的旧缓存立刻失效。前提是在「系统设置 → 文件存储与 CDN」里开启 CDN 刷新并填好阿里云凭据（也可在部署环境用环境变量 CDN_ENABLED 配置）。"
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
import { ref, computed } from 'vue'
import { cdnRefresh } from '@/api/storage'

const urls = ref('')
const loading = ref(false)

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
