<template>
  <el-card>
    <div class="page-bar">
      <h3>公司简介</h3>
      <el-button :icon="Refresh" :loading="loading" @click="load">重新加载</el-button>
    </div>

    <el-form :model="form" label-width="110px" style="max-width: 680px">
      <el-form-item label="简介文字">
        <el-input v-model="form.introText" type="textarea" :rows="5" placeholder="首页简介区文字" />
      </el-form-item>
      <el-form-item label="视频号 ID">
        <el-input v-model="form.videoChannelId" placeholder="形如 gh_xxx；为空则展示封面占位" />
      </el-form-item>
      <el-form-item label="降级 MP4">
        <el-input v-model="form.backupMp4Url" placeholder="视频号异常时播放的 MP4 地址" />
      </el-form-item>
      <el-form-item label="封面 / 占位图">
        <UploadField v-model="form.cover" :multiple="false" accept="image/*" />
      </el-form-item>
      <el-form-item>
        <el-button type="primary" :loading="saving" @click="save">保存</el-button>
      </el-form-item>
    </el-form>
  </el-card>
</template>

<script setup lang="ts">
import { ElMessage } from 'element-plus'
import { reactive, ref, onMounted } from 'vue'
import { Refresh } from '@element-plus/icons-vue'
import { getCompanyProfile, saveCompanyProfile } from '@/api/content'
import UploadField from '@/components/UploadField.vue'

const loading = ref(false)
const saving = ref(false)
const form = reactive({ id: 1, introText: '', videoChannelId: '', backupMp4Url: '', cover: '' })

async function load() {
  loading.value = true
  try {
    const d = await getCompanyProfile()
    Object.assign(form, d)
  } catch {
    /* 拦截器已提示 */
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  try {
    await saveCompanyProfile({
      introText: form.introText,
      videoChannelId: form.videoChannelId,
      backupMp4Url: form.backupMp4Url,
      cover: form.cover,
    })
    ElMessage.success('已保存')
  } catch {
    /* 拦截器已提示 */
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
</style>
