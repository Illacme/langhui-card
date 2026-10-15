<template>
  <el-container class="admin-layout">
    <el-aside width="220px" class="admin-aside">
      <div class="brand">
        <span class="brand-logo">朗</span>
        <span class="brand-name">朗汇机电 · 管理后台</span>
      </div>
      <el-menu
        :default-active="activeMenu"
        router
        class="admin-menu"
        background-color="#1f2d3d"
        text-color="#c0c4cc"
        active-text-color="#ffffff"
      >
        <template v-for="g in menuGroups" :key="g.group">
          <el-sub-menu :index="g.group">
            <template #title>
              <el-icon><component :is="g.icon" /></el-icon>
              <span>{{ g.group }}</span>
            </template>
            <el-menu-item v-for="m in g.items" :key="m.path" :index="m.path">
              {{ m.title }}
            </el-menu-item>
          </el-sub-menu>
        </template>
      </el-menu>
    </el-aside>

    <el-container>
      <el-header class="admin-header">
        <div class="header-title">{{ currentTitle }}</div>
        <div class="header-right">
          <span class="header-user">
            <el-icon><UserFilled /></el-icon>
            {{ auth.username || '超管' }}
          </span>
          <el-button text @click="showPwd = true">
            <el-icon><Key /></el-icon> 修改密码
          </el-button>
          <el-button text type="danger" @click="onLogout">
            <el-icon><SwitchButton /></el-icon> 退出
          </el-button>
        </div>
      </el-header>

      <el-main class="admin-main">
        <router-view v-slot="{ Component }">
          <component :is="Component" />
        </router-view>
      </el-main>
    </el-container>

    <ChangePasswordDialog v-model="showPwd" />
  </el-container>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { UserFilled, Key, SwitchButton, Document, Van } from '@element-plus/icons-vue'
import { useAuthStore } from '@/store/auth'
import ChangePasswordDialog from '@/components/ChangePasswordDialog.vue'

const auth = useAuthStore()
const route = useRoute()
const router = useRouter()
const showPwd = ref(false)

const menuGroups = [
  {
    group: '审核管理',
    icon: 'Van',
    items: [
      { path: '/apply/open', title: '开通审核' },
      { path: '/apply/modify', title: '名片修改审核' },
      { path: '/employees', title: '员工管理' },
    ],
  },
  {
    group: '企业内容',
    icon: 'Document',
    items: [
      { path: '/content/company-info', title: '公司信息' },
      { path: '/content/company-profile', title: '公司简介' },
      { path: '/content/categories', title: '业绩分类' },
      { path: '/content/projects', title: '项目案例' },
      { path: '/content/services', title: '业务服务' },
      { path: '/content/home-layout', title: '首页布局' },
      { path: '/content/cdn-refresh', title: 'CDN 刷新' },
    ],
  },
]

const activeMenu = computed(() => route.path)
const currentTitle = computed(() => (route.meta?.title as string) || '朗汇机电管理后台')

function onLogout() {
  auth.logout()
  router.replace('/login')
}
</script>

<style scoped>
.admin-layout {
  height: 100vh;
}
.admin-aside {
  background: #1f2d3d;
  overflow: hidden;
}
.brand {
  height: 60px;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 0 16px;
  color: #fff;
  font-weight: 600;
  background: #18222e;
}
.brand-logo {
  width: 28px;
  height: 28px;
  border-radius: 6px;
  background: #409eff;
  color: #fff;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 16px;
}
.brand-name {
  font-size: 15px;
}
.admin-menu {
  border-right: none;
}
.admin-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  background: #fff;
  border-bottom: 1px solid #ebeef5;
}
.header-title {
  font-size: 16px;
  font-weight: 600;
  color: #303133;
}
.header-right {
  display: flex;
  align-items: center;
  gap: 8px;
}
.header-user {
  display: flex;
  align-items: center;
  gap: 4px;
  color: #606266;
  margin-right: 8px;
}
.admin-main {
  background: #f5f7fa;
  padding: 16px;
}
</style>
