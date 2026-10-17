<template>
  <el-card>
    <div class="page-bar">
      <h3>公司信息</h3>
      <el-button :icon="Refresh" :loading="loading" @click="load">重新加载</el-button>
    </div>

    <el-form :model="form" label-width="90px" style="max-width: 640px">
      <el-form-item label="企业全称">
        <el-input v-model="form.fullName" placeholder="名片首页公司名" />
      </el-form-item>
      <el-form-item label="企业简称">
        <el-input v-model="form.shortName" placeholder="小程序顶栏居中展示，留空则显示企业全称" />
        <div class="field-tip">仅作为小程序中的显示值使用，和后台左边栏顶部标识没有关联</div>
      </el-form-item>
      <el-form-item label="地址">
        <el-input v-model="form.address" placeholder="名片首页地址" />
      </el-form-item>
      <el-form-item label="座机">
        <el-input v-model="form.tel" placeholder="名片首页座机" />
      </el-form-item>
      <el-form-item label="LOGO">
        <UploadField v-model="form.logo" :multiple="false" accept="image/*" />
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
import { getCompanyInfo, saveCompanyInfo } from '@/api/content'
import UploadField from '@/components/UploadField.vue'

const loading = ref(false)
const saving = ref(false)
const form = reactive({ id: 1, fullName: '', shortName: '', address: '', tel: '', logo: '' })

async function load() {
  loading.value = true
  try {
    const d = await getCompanyInfo()
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
    await saveCompanyInfo({
      fullName: form.fullName,
      shortName: form.shortName,
      address: form.address,
      tel: form.tel,
      logo: form.logo,
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
.field-tip {
  font-size: 12px;
  color: #909399;
  line-height: 1.6;
}
</style>
