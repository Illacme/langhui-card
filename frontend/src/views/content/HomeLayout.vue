<template>
  <el-card>
    <div class="page-bar">
      <h3>首页布局</h3>
      <el-button :icon="Refresh" :loading="loading" @click="load">重新加载</el-button>
    </div>

    <p class="desc">控制小程序首页「经典业绩」与「业务服务」两个板块的先后顺序。</p>

    <el-form label-width="120px" style="max-width: 480px">
      <el-form-item label="板块顺序">
        <el-radio-group v-model="firstBlock">
          <el-radio value="achievement">业绩在前，服务在后</el-radio>
          <el-radio value="service">服务在前，业绩在后</el-radio>
        </el-radio-group>
      </el-form-item>
      <el-form-item label="当前顺序">
        <el-tag v-for="b in order" :key="b" class="order-tag">{{ blockLabel(b) }}</el-tag>
      </el-form-item>
      <el-form-item>
        <el-button type="primary" :loading="saving" @click="save">保存</el-button>
      </el-form-item>
    </el-form>
  </el-card>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { Refresh } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { getHomeLayout, saveHomeLayout } from '@/api/content'

const BLOCKS = ['achievement', 'service']
const loading = ref(false)
const saving = ref(false)
const firstBlock = ref<string>('achievement')

const order = computed(() => {
  const rest = BLOCKS.filter((b) => b !== firstBlock.value)
  return [firstBlock.value, ...rest]
})

function blockLabel(b: string) {
  return b === 'achievement' ? '经典业绩' : '业务服务'
}

async function load() {
  loading.value = true
  try {
    const o = await getHomeLayout()
    if (o && o.length) firstBlock.value = o[0]
  } catch {
    /* 拦截器已提示 */
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  try {
    await saveHomeLayout(order.value)
    ElMessage.success('已保存首页布局')
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
.desc {
  color: #606266;
  font-size: 13px;
}
.order-tag {
  margin-right: 8px;
}
</style>
