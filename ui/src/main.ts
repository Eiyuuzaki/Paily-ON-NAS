import { createApp, h } from 'vue'
import { createPinia } from 'pinia'
import { createVuetify } from 'vuetify'
import 'vuetify/styles'
import './style.css'
import App from './App.vue'
import router from './router'
import { useAuthStore } from './stores/auth'
import { mdiIcons } from './plugins/mdi-svg'

// ── 自定义 MDI SVG 图标集 ────────────────────────────────────────────────
// 用按需导入的 SVG path 代替 3.5MB 的 @mdi/font 字体，构建产物小得多。
const mdiSvgSet = {
  component: (props: { icon: string }) => {
    const path = mdiIcons[props.icon]
    if (!path) return null
    return h(
      'svg',
      { viewBox: '0 0 24 24', role: 'img', 'aria-hidden': 'true' },
      [h('path', { d: path })],
    )
  },
}

// ── Material Design 3 主题 ────────────────────────────────────────────────
const vuetify = createVuetify({
  icons: {
    defaultSet: 'mdi-svg',
    sets: {
      'mdi-svg': mdiSvgSet,
    },
  },
  theme: {
    defaultTheme: localStorage.getItem('paily_theme') || 'light',
    themes: {
      light: {
        dark: false,
        colors: {
          primary: '#6656d8',
          'primary-darken-1': '#4f3fc4',
          secondary: '#5c6bc0',
          accent: '#7c4dff',
          error: '#b00020',
          info: '#0288d1',
          success: '#2e7d32',
          warning: '#ed6c02',
          background: '#f5f5f7',
          surface: '#ffffff',
        },
      },
      dark: {
        dark: true,
        colors: {
          primary: '#b39dff',
          'primary-darken-1': '#8f7ae8',
          secondary: '#8c9eff',
          accent: '#b388ff',
          error: '#ff5252',
          info: '#40c4ff',
          success: '#69f0ae',
          warning: '#ffab40',
          background: '#121218',
          surface: '#1e1e26',
        },
      },
    },
  },
  defaults: {
    VCard: { rounded: 'lg' },
    VBtn: { rounded: 'lg' },
    VTextField: { variant: 'outlined', density: 'comfortable' },
    VSelect: { variant: 'outlined', density: 'comfortable' },
    VAutocomplete: { variant: 'outlined', density: 'comfortable' },
    VTextarea: { variant: 'outlined' },
    VDataTable: { density: 'comfortable' },
    VChip: { rounded: 'lg' },
  },
})

const app = createApp(App)
const pinia = createPinia()

app.use(pinia)
app.use(router)
app.use(vuetify)

// Restore auth session from localStorage before first navigation
useAuthStore().restoreSession()

app.mount('#app')
