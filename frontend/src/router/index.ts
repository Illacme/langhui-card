import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '@/store/auth'

const routes = [
  {
    path: '/login',
    name: 'login',
    component: () => import('@/views/Login.vue'),
    meta: { public: true },
  },
  {
    path: '/',
    component: () => import('@/layout/AdminLayout.vue'),
    redirect: '/apply/open',
    children: [
      {
        path: 'apply/open',
        name: 'apply-open',
        component: () => import('@/views/ApplyOpen.vue'),
        meta: { title: '开通审核', group: '审核管理' },
      },
      {
        path: 'apply/modify',
        name: 'apply-modify',
        component: () => import('@/views/ApplyModify.vue'),
        meta: { title: '名片修改审核', group: '审核管理' },
      },
      {
        path: 'employees',
        name: 'employees',
        component: () => import('@/views/Employees.vue'),
        meta: { title: '员工管理', group: '审核管理' },
      },
      {
        path: 'stats',
        name: 'stats',
        component: () => import('@/views/Stats.vue'),
        meta: { title: '名片统计', group: '数据统计' },
      },
      {
        path: 'content/company-info',
        name: 'company-info',
        component: () => import('@/views/content/CompanyInfo.vue'),
        meta: { title: '公司信息', group: '企业内容' },
      },
      {
        path: 'content/company-profile',
        name: 'company-profile',
        component: () => import('@/views/content/CompanyProfile.vue'),
        meta: { title: '公司简介', group: '企业内容' },
      },
      {
        path: 'content/categories',
        name: 'categories',
        component: () => import('@/views/content/Categories.vue'),
        meta: { title: '业绩分类', group: '企业内容' },
      },
      {
        path: 'content/projects',
        name: 'projects',
        component: () => import('@/views/content/Projects.vue'),
        meta: { title: '项目案例', group: '企业内容' },
      },
      {
        path: 'content/services',
        name: 'services',
        component: () => import('@/views/content/Services.vue'),
        meta: { title: '业务服务', group: '企业内容' },
      },
      {
        path: 'content/home-layout',
        name: 'home-layout',
        component: () => import('@/views/content/HomeLayout.vue'),
        meta: { title: '首页布局', group: '企业内容' },
      },
      {
        path: 'content/cdn-refresh',
        name: 'cdn-refresh',
        component: () => import('@/views/content/CdnRefresh.vue'),
        meta: { title: 'CDN 刷新', group: '企业内容' },
      },
    ],
  },
  { path: '/:pathMatch(.*)*', redirect: '/apply/open' },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

// 路由守卫：未登录访问非公开页 → /login；已登录访问 /login → 后台首页
// （令牌在 httpOnly Cookie 中，JS 以非敏感登录标记 loggedIn 判断）
router.beforeEach((to) => {
  const auth = useAuthStore()
  if (!to.meta.public && !auth.loggedIn) {
    return { path: '/login' }
  }
  if (to.path === '/login' && auth.loggedIn) {
    return { path: '/' }
  }
})

export default router
