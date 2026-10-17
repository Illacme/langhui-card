import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import Components from 'unplugin-vue-components/vite'
import { ElementPlusResolver } from 'unplugin-vue-components/resolvers'
import { fileURLToPath, URL } from 'node:url'

// PC 管理后台 Vite 配置
// 开发期通过 /api 代理把请求转发到本地 Go 后端（默认 8080），避免跨域。
export default defineConfig({
  plugins: [
    vue(),
    // Element Plus 按需引入：自动扫描模板中的 el-* 组件并按需注册、注入对应 CSS，
    // 取代全量 app.use(ElementPlus) 与 import 'element-plus/dist/index.css'（约 361KB），
    // 首屏 CSS 体积显著下降。ElMessage / ElMessageBox 等命令式 API 改为「显式 import」
    // （见各 .vue 与 src/api/request.ts），其样式在 src/main.ts 中手动引入，避免依赖
    // AutoImport 的运行时注入（实测该注入在构建产物中未生效，会导致线上 ReferenceError）。
    Components({
      resolvers: [ElementPlusResolver({ importStyle: 'css' })],
      dts: 'src/components.d.ts',
    }),
  ],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  server: {
    host: '0.0.0.0',
    port: 5173,
    proxy: {
      '/api': {
        target: process.env.VITE_BACKEND_URL || 'http://127.0.0.1:8080',
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: 'dist',
    chunkSizeWarningLimit: 1500,
    rollupOptions: {
      output: {
        // 拆分体积较大的第三方依赖，提升缓存命中率与首屏加载速度：
        //   - wangeditor 富文本编辑器（约 800KB）单独成块，仅在编辑页按需加载
        //   - element-plus / vue 生态各自成块
        manualChunks(id) {
          if (id.includes('@wangeditor')) return 'wangeditor'
          if (id.includes('element-plus')) return 'element-plus'
          if (id.includes('node_modules')) return 'vendor'
        },
      },
    },
  },
})
