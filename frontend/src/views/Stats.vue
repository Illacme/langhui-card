<template>
  <div>
    <el-card>
      <div class="page-bar">
        <h3>名片统计</h3>
        <div class="bar-right">
          <el-input
            v-model="keyword"
            placeholder="搜索姓名/职位"
            clearable
            :prefix-icon="Search"
            style="width: 200px"
          />
          <el-button :icon="Refresh" :loading="loading" @click="load">刷新</el-button>
        </div>
      </div>

      <!-- 汇总三卡：访问 / 拨号 / 保存（口径见《首页组件与接口字段映射》§4：只记点击，不校验真实结果） -->
      <div class="stat-cards">
        <div class="stat-card">
          <div class="stat-label">总访问</div>
          <div class="stat-value">{{ summary.visit }}</div>
        </div>
        <div class="stat-card">
          <div class="stat-label">拨号点击</div>
          <div class="stat-value">{{ summary.call }}</div>
        </div>
        <div class="stat-card">
          <div class="stat-label">保存点击</div>
          <div class="stat-value">{{ summary.save }}</div>
        </div>
      </div>

      <el-table :data="paged" v-loading="loading" border stripe>
        <el-table-column prop="staffId" label="编号" width="80" align="center" />
        <el-table-column prop="name" label="姓名" width="120" />
        <el-table-column prop="title" label="职位" width="120" />
        <el-table-column prop="visit" label="访问" width="110" align="right" sortable />
        <el-table-column prop="call" label="拨号" width="110" align="right" sortable />
        <el-table-column prop="save" label="保存" width="110" align="right" sortable />
      </el-table>
      <p class="tip">
        口径：只记前端点击、不校验真实结果（需求 §6）。想看某员工的名片实际效果，在小程序首页输入其编号即可预览。
      </p>

      <el-pagination
        class="table-pager"
        v-model:current-page="currentPage"
        v-model:page-size="size"
        :total="total"
        :page-sizes="[10, 20, 50]"
        layout="total, sizes, prev, pager, next"
        background
      />
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { Refresh, Search } from '@element-plus/icons-vue'
import { adminStats } from '@/api/stats'
import { useTable } from '@/composables/useTable'

const rows = ref<any[]>([])
const { keyword, currentPage, size, total, paged } = useTable(
  rows,
  (r, kw) =>
    (r.name || '').toLowerCase().includes(kw) || (r.title || '').toLowerCase().includes(kw)
)
const loading = ref(false)
const summary = ref({ visit: 0, call: 0, save: 0 })

async function load() {
  loading.value = true
  try {
    const r = await adminStats()
    summary.value = r.summary
    rows.value = r.byStaff
  } catch {
    /* 拦截器已提示 */
  } finally {
    loading.value = false
  }
}

// 埋点口径见《首页组件与接口字段映射》§4：只记前端点击，不校验真实结果；
// 本页无写操作，故不需要「查看名片」之类的跳转（PC 端无法直接打开小程序，
// 运营可在小程序首页输入编号预览）。

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
.bar-right {
  display: flex;
  align-items: center;
  gap: 8px;
}
.stat-cards {
  display: flex;
  gap: 16px;
  margin-bottom: 16px;
}
.stat-card {
  flex: 1;
  padding: 16px 20px;
  border: 1px solid #ebeef5;
  border-radius: 6px;
  background: #fafcff;
}
.stat-label {
  color: #909399;
  font-size: 13px;
}
.stat-value {
  margin-top: 6px;
  font-size: 28px;
  font-weight: 600;
  color: #303133;
}
.table-pager {
  margin-top: 14px;
  justify-content: flex-end;
}
.tip {
  margin: 12px 0 0;
  color: #909399;
  font-size: 12px;
}
/* 手机端：三张汇总卡由横排改纵向堆叠，避免窄屏被压扁 */
@media (max-width: 767px) {
  .stat-cards {
    flex-direction: column;
  }
  .stat-card {
    padding: 14px 16px;
  }
  .stat-value {
    font-size: 24px;
  }
}
</style>
