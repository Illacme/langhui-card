import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'
import { fileURLToPath, URL } from 'node:url'

// 单元测试配置（Vitest）。
// - jsdom 环境：前端逻辑（localStorage / DOM 占位）需要在类浏览器环境跑。
// - 复用 vite 的 '@' -> src 别名，保证测试里的 import 与生产代码一致。
export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    include: ['src/**/*.spec.ts'],
    // 测试不依赖真实后端，统一关掉网络与副作用
    clearMocks: true,
    restoreMocks: true,
  },
})
