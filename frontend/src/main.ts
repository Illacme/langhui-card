import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import * as ElementPlusIconsVue from '@element-plus/icons-vue'

import './styles/admin.css'

import App from './App.vue'
import router from './router'

const app = createApp(App)

// 全局错误边界：捕获渲染/生命周期中的异常并上报但不重抛，
// 避免 dev 模式重抛单个子组件异常从而冻结整棵响应式树
// （#53 崩溃正是经此路径放大为关闭/下拉/开关失灵）。
app.config.errorHandler = (err, instance, info) => {
  console.error('[Vue 运行时异常]', info, err)
}

// 注册全部 Element Plus 图标（便于菜单/按钮使用）
for (const [key, component] of Object.entries(ElementPlusIconsVue)) {
  app.component(key, component)
}

app.use(createPinia())
app.use(router)
app.use(ElementPlus)
app.mount('#app')
