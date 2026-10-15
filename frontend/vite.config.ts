import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { fileURLToPath, URL } from 'node:url'

// PC 管理后台 Vite 配置
// 开发期通过 /api 代理把请求转发到本地 Go 后端（默认 8080），避免跨域。
export default defineConfig({
  plugins: [vue()],
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
