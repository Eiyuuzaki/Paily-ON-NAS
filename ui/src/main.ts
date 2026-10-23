import { createApp, h } from 'vue'
import { createPinia } from 'pinia'
import { createVuetify } from 'vuetify'
import 'vuetify/styles'
import './style.css'
import App from './App.vue'
import router from './router'
import { useAuthStore } from './stores/auth'
import { mdiIcons } from './plugins/mdi-svg'
import { md3Light, md3Dark } from './plugins/md3-theme'

// ── 自定义 MDI SVG 图标集 ────────────────────────────────────────────────
// 用按需导入的 SVG path 代替 3.5MB 的 @mdi/font 字体，构建产物小得多。
//
// 注意：path 必须显式设置 fill="currentColor"，否则浏览器默认 fill 是黑色，
// 在暗色主题下黑图标几乎不可见（浅色主题下也能看见，所以容易漏掉）。
const mdiSvgSet = {
  component: (props: { icon: string }) => {
    const path = mdiIcons[props.icon]
    if (!path) return null
    return h(
      'svg',
      { viewBox: '0 0 24 24', role: 'img', 'aria-hidden': 'true' },
      [h('path', { d: path, fill: 'currentColor' })],
    )
  },
}

// ── Material Design 3 主题 ────────────────────────────────────────────────
// 启用 MD3 组件默认值（药丸按钮、分割式进度条、outlined 输入框等），
// 再叠加自定义的 tonal 色阶色板。
const vuetify = createVuetify({
  icons: {
    defaultSet: 'mdi-svg',
    // Vuetify 内部组件（如 v-app-bar-nav-icon 的 $menu、下拉箭头 $subgroup、
    // 复选框 $checkboxOn 等）通过别名引用图标，必须显式映射到本图标集，
    // 否则这些内置图标会渲染为空白。
    aliases: {
      menu: 'mdi-menu',
      subgroup: 'mdi-menu-down',
      dropdown: 'mdi-menu-down',
      expand: 'mdi-chevron-down',
      collapse: 'mdi-chevron-up',
      complete: 'mdi-check',
      close: 'mdi-close',
      clear: 'mdi-close',
      cancel: 'mdi-close',
      delete: 'mdi-delete-outline',
      prev: 'mdi-chevron-left',
      next: 'mdi-chevron-right',
      first: 'mdi-chevron-double-left',
      last: 'mdi-chevron-double-right',
      checkboxOn: 'mdi-checkbox-marked',
      checkboxOff: 'mdi-checkbox-blank-outline',
      checkboxIndeterminate: 'mdi-minus-box',
      radioOn: 'mdi-radiobox-marked',
      radioOff: 'mdi-radiobox-blank',
      sortAsc: 'mdi-arrow-up',
      sortDesc: 'mdi-arrow-down',
      loading: 'mdi-loading',
      search: 'mdi-magnify',
      plus: 'mdi-plus',
      minus: 'mdi-minus',
      info: 'mdi-information-outline',
      warning: 'mdi-alert-outline',
      error: 'mdi-alert-circle-outline',
      success: 'mdi-check-circle-outline',
    },
    sets: {
      'mdi-svg': mdiSvgSet,
    },
  },
  theme: {
    defaultTheme: localStorage.getItem('paily_theme') || 'light',
    themes: {
      light: md3Light,
      dark: md3Dark,
    },
  },
  defaults: {
    // MD3 组件规范
    global: {
      ripple: true,
    },
    VAppBar: { flat: true },
    VCard: { rounded: 'xl' },
    VBtn: { rounded: 'xl', style: 'text-transform: none; letter-spacing: 0.1px;' },
    VChip: { rounded: 'sm' },
    VTextField: { variant: 'outlined', density: 'comfortable', rounded: 'sm' },
    VSelect: { variant: 'outlined', density: 'comfortable', rounded: 'sm' },
    VAutocomplete: { variant: 'outlined', density: 'comfortable', rounded: 'sm' },
    VTextarea: { variant: 'outlined', rounded: 'sm' },
    VNumberInput: { variant: 'outlined', rounded: 'sm' },
    VDataTable: { density: 'comfortable' },
    VList: { prependGap: 16 },
    VListItem: { rounded: 'xl' },
    VProgressCircular: { rounded: true },
    VProgressLinear: { rounded: true, roundedBar: true },
    VTab: { rounded: 0 },
    VTabs: { color: 'primary', sliderVariant: 'primary' },
    VSwitch: { indentDetails: true },
    VSlider: { color: 'primary', indentDetails: true },
    VDialog: { scrim: 'rgba(0,0,0,0.4)' },
    VSnackbar: { rounded: 'lg' },
    VNavigationDrawer: {},
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
