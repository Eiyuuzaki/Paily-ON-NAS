<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useTheme, useDisplay } from 'vuetify'
import { useAuthStore } from '../stores/auth'
import { useSettingsStore } from '../stores/settings'
import QuickOpenDrawer from '../components/QuickOpenDrawer.vue'

const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const settings = useSettingsStore()
const theme = useTheme()
const { mdAndUp } = useDisplay()

// 桌面端默认展开常驻；移动端默认收起，点汉堡图标才弹出（MD3 规范）
const drawer = ref(mdAndUp.value)
const rail = ref(false)
const isDesktop = computed(() => mdAndUp.value)
const isDark = computed(() => settings.theme === 'dark')

// 屏幕尺寸变化时同步：切到桌面自动展开，切到移动自动收起
watch(mdAndUp, (desktop) => {
  drawer.value = desktop
  if (!desktop) rail.value = false
})

// 移动端点菜单跳转后自动收起抽屉
watch(
  () => route.path,
  () => {
    if (!isDesktop.value) drawer.value = false
  },
)

// 全局 API 错误提示（由 request.ts 派发）
const apiError = ref<string | null>(null)
function onApiError(e: Event) {
  apiError.value = (e as CustomEvent).detail || '请求失败'
}
onMounted(() => window.addEventListener('paily-api-error', onApiError))
onUnmounted(() => window.removeEventListener('paily-api-error', onApiError))

watch(
  () => settings.theme,
  (t) => theme.change(t),
  { immediate: true },
)

function logout() {
  authStore.logout()
  router.push('/login')
}

const menuItems = [
  { path: '/dashboard', title: '仪表盘', icon: 'mdi-view-dashboard-outline' },
  { path: '/nodes', title: '节点管理', icon: 'mdi-server-network' },
  { path: '/sources', title: '来源管理', icon: 'mdi-link-variant' },
  { path: '/sponsors', title: '赞助商管理', icon: 'mdi-star-outline' },
  { path: '/filter', title: '过滤测试台', icon: 'mdi-filter-outline' },
  { path: '/config', title: '运行时配置', icon: 'mdi-cog-outline' },
  { path: '/format-configs', title: '格式配置', icon: 'mdi-file-code-outline' },
  { path: '/logs', title: '日志', icon: 'mdi-text-box-outline' },
]
</script>

<template>
  <v-navigation-drawer
    v-model="drawer"
    :rail="rail && isDesktop"
    :permanent="isDesktop"
    :temporary="!isDesktop"
    width="228"
    rail-width="80"
    class="app-drawer"
    :class="{ 'app-drawer--rail': rail }"
  >
    <!-- 品牌区 -->
    <div class="brand" :class="{ 'brand--rail': rail }">
      <div class="brand-logo">
        <v-icon icon="mdi-hexagon-multiple" size="22" color="on-primary" />
      </div>
      <div v-if="!rail" class="brand-copy">
        <div class="brand-title">Paily</div>
        <div class="brand-sub">ON NAS</div>
      </div>
    </div>

    <!--
      Navigation rail（收起态）
      MD3 规范：active indicator 固定 56x32px、圆角 16px，
      图标 24px 在容器内绝对居中，未选中项仅显示图标。
      这里不用 v-list-item —— 它的 prepend/title 布局在 rail 下会把图标挤偏。
    -->
    <div v-if="rail" class="rail">
      <router-link
        v-for="item in menuItems"
        :key="item.path"
        :to="item.path"
        class="rail-item"
        :class="{ 'rail-item--active': route.path === item.path }"
        :title="item.title"
      >
        <span class="rail-indicator">
          <v-icon :icon="item.icon" size="24" />
        </span>
      </router-link>
    </div>

    <v-list v-else nav class="nav-list">
      <v-list-item
        v-for="item in menuItems"
        :key="item.path"
        :to="item.path"
        :prepend-icon="item.icon"
        :title="item.title"
        rounded="pill"
        color="secondary-container"
        class="nav-item"
      />
    </v-list>

    <template #append>
      <v-divider v-if="!rail" class="mx-3" />
      <div v-if="rail" class="rail">
        <div class="rail-item" title="展开侧栏" @click="rail = false">
          <span class="rail-indicator">
            <v-icon icon="mdi-chevron-right" size="24" />
          </span>
        </div>
      </div>
      <v-list v-else nav class="py-2">
        <v-list-item
          prepend-icon="mdi-chevron-left"
          title="收起侧栏"
          rounded="pill"
          class="nav-item"
          @click="rail = true"
        />
      </v-list>
    </template>
  </v-navigation-drawer>

  <v-app-bar flat class="app-bar">
    <v-app-bar-nav-icon v-if="!isDesktop" icon="mdi-menu" @click="drawer = !drawer" />

    <v-spacer />

    <QuickOpenDrawer />

    <v-btn
      :icon="isDark ? 'mdi-weather-sunny' : 'mdi-weather-night'"
      variant="text"
      :title="isDark ? '切换到亮色' : '切换到暗色'"
      @click="settings.toggleTheme()"
    />

    <v-btn
      prepend-icon="mdi-logout"
      variant="text"
      color="error"
      class="ml-1"
      @click="logout"
    >
      <span class="d-none d-sm-inline">退出</span>
    </v-btn>
  </v-app-bar>

  <v-main class="app-main">
    <v-container fluid class="pa-4 pa-md-6">
      <router-view v-slot="{ Component, route: r }">
        <transition name="page-slide" mode="out-in">
          <component :is="Component" :key="r.path" />
        </transition>
      </router-view>
    </v-container>
  </v-main>

  <v-snackbar v-model="apiError" :timeout="3000" color="error" location="bottom" rounded="lg">
    {{ apiError }}
    <template #actions>
      <v-btn variant="text" @click="apiError = null">关闭</v-btn>
    </template>
  </v-snackbar>
</template>

<style scoped>
/*
  MD3 背景层级：
  展开态用 surface-container-low（与右侧 surface 轻微分层）；
  rail 收起态改用 surface-container —— 在 80px 的窄条上需要更明确的
  色阶，否则会和右侧主内容区糊成一片。
*/
.app-drawer {
  background: rgb(var(--v-theme-surface-container-low)) !important;
  border: none !important;
}
.app-drawer--rail {
  background: rgb(var(--v-theme-surface-container)) !important;
}
.app-drawer :deep(.v-navigation-drawer__content) {
  display: flex;
  flex-direction: column;
}

/* 品牌区 */
.brand {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 20px 20px 16px;
}
.brand--rail {
  justify-content: center;
  padding-inline: 0;
  padding-bottom: 12px;
}
.brand-logo {
  width: 40px;
  height: 40px;
  border-radius: 12px;
  background: rgb(var(--v-theme-primary));
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}
.brand-copy {
  line-height: 1.2;
}
.brand-title {
  font-size: 17px;
  font-weight: 700;
  color: rgb(var(--v-theme-on-surface));
}
.brand-sub {
  font-size: 11px;
  font-weight: 500;
  letter-spacing: 1.2px;
  color: rgb(var(--v-theme-on-surface-variant));
}

/* ── Navigation Rail（收起态）──────────────────────────────────────────── */
.rail {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  padding: 4px 0;
}
.rail-item {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  height: 56px;
  text-decoration: none;
  color: rgb(var(--v-theme-on-surface-variant));
  transition: color 0.15s ease;
  -webkit-tap-highlight-color: transparent;
}
.rail-item:hover {
  color: rgb(var(--v-theme-on-surface));
}
/*
  MD3 active indicator：固定 56 x 32px，圆角 16px。
  不要用 9999px 的胶囊 —— 在 80px 宽的 rail 里会变成横向拉伸的椭圆。
  固定尺寸，不随容器拉伸，图标因此在容器内绝对居中。
*/
.rail-indicator {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 56px;
  height: 32px;
  border-radius: 16px;
  transition: background-color 0.18s ease;
}
.rail-item--active {
  color: rgb(var(--v-theme-on-secondary-container));
}
.rail-item--active .rail-indicator {
  background: rgb(var(--v-theme-secondary-container));
}

/* 导航项：MD3 药丸高亮，紧凑高度 */
.nav-list {
  padding-top: 4px;
}
.nav-item {
  /* MD3 navigation drawer 的单项高度规范是 56dp，但中文界面 44px 更紧凑耐看 */
  min-height: 44px !important;
  margin-bottom: 2px;
  /* 图标与文字间距：MD3 建议 12dp */
  --v-list-prepend-gap: 12px;
}
.nav-item :deep(.v-list-item__prepend) {
  /* 收紧左侧内边距，让图标不贴边也不过远 */
  padding-inline-end: 12px;
}
.nav-item :deep(.v-list-item-title) {
  font-size: 14px;
  font-weight: 500;
  letter-spacing: 0.1px;
}
/* 选中态：药丸背景 + 加粗（MD3 navigation drawer 规范） */
.nav-item.v-list-item--active {
  background: rgb(var(--v-theme-secondary-container));
}
.nav-item.v-list-item--active :deep(.v-list-item-title) {
  font-weight: 600;
  color: rgb(var(--v-theme-on-secondary-container));
}
.nav-item.v-list-item--active :deep(.v-icon) {
  color: rgb(var(--v-theme-on-secondary-container));
  opacity: 1;
}

/* 顶栏：surface 底色，无阴影，只留一层极淡分隔 */
.app-bar {
  background: rgb(var(--v-theme-surface)) !important;
  border-bottom: 1px solid rgba(var(--v-theme-outline-variant), 0.5);
}

.app-main {
  background: rgb(var(--v-theme-surface));
}
</style>
