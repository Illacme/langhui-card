import { createApp } from 'vue'
import { createPinia } from 'pinia'

import './styles/admin.css'
import './styles/responsive.css'

// Element Plus 按需引入后，组件 CSS 由 Components 插件按模板用量注入；
// 但 ElMessage / ElMessageBox 这类「命令式 API」不会出现在模板里，
// 必须在此手动引入其样式，否则弹窗无样式。每个组件的 style/css 已包含 base 变量。
import 'element-plus/es/components/message/style/css'
import 'element-plus/es/components/message-box/style/css'

import App from './App.vue'
import router from './router'

const app = createApp(App)

// 全局错误边界：捕获渲染/生命周期中的异常并上报但不重抛，
// 避免 dev 模式重抛单个子组件异常从而冻结整棵响应式树
// （#53 崩溃正是经此路径放大为关闭/下拉/开关失灵）。
app.config.errorHandler = (err, instance, info) => {
  console.error('[Vue 运行时异常]', info, err)
}

// 说明：Element Plus 已改为「按需引入」（见 vite.config.ts 的 Components 插件），
// 组件与样式在编译期按模板实际用量注入，不再全量 app.use(ElementPlus) 或引入整包 CSS。
// 命令式 API（ElMessage / ElMessageBox）改为在各 .vue 与 src/api/request.ts 中显式 import，
// 其样式已在本文件顶部手动引入。图标此前通过全局 barrel 注册；现改为各组件显式 import，
// 因此此处不再注册全局图标，避免打入未使用的图标。
app.use(createPinia())
app.use(router)
app.mount('#app')
