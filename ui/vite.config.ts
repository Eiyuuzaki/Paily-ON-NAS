import { defineConfig, loadEnv } from 'vite'
import vue from '@vitejs/plugin-vue'
import vuetify from 'vite-plugin-vuetify'

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd())
  const apiBase = env.VITE_API_BASE || 'http://localhost:21380'

  return {
    plugins: [
      vue(),
      // 按需自动引入 Vuetify 组件与样式，控制包体
      vuetify({ autoImport: true }),
    ],
    server: {
      proxy: {
        '/api': apiBase,
        '/clash': apiBase,
        '/singbox': apiBase,
        '/base64': apiBase,
      },
    },
  }
})
