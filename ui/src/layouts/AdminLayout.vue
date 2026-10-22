<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useTheme } from 'vuetify'
import { useAuthStore } from '../stores/auth'
import { useSettingsStore } from '../stores/settings'
import QuickOpenDrawer from '../components/QuickOpenDrawer.vue'

const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const settings = useSettingsStore()
const theme = useTheme()

const drawer = ref(true)
const rail = ref(false)
const isDark = computed(() => settings.theme === 'dark')

// 全局 API 错误提示（由 request.ts 派发）
const apiError = ref<string | null>(null)
function onApiError(e: Event) {
  apiError.value = (e as CustomEvent).detail || '请求失败'
}
onMounted(() => window.addEventListener('paily-api-error', onApiError))
onUnmounted(() => window.removeEventListener('paily-api-error', onApiError))

// 主题跟随 store，切换时同步给 Vuetify
watch(
  () => settings.theme,
  (t) => {
    theme.change(t)
  },
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
    :rail="rail"
    :permanent="$vuetify.display.mdAndUp"
    :temporary="$vuetify.display.smAndDown"
    width="220"
    rail-width="72"
    class="app-drawer"
  >
    <!-- 品牌区 -->
    <div class="brand" :class="{ 'brand--rail': rail }">
      <v-icon icon="mdi-hexagon-multiple" color="primary" size="28" />
      <span v-if="!rail" class="brand-text">Paily <span class="brand-accent">ON NAS</span></span>
    </div>

    <v-divider />

    <v-list density="compact" nav class="mt-2">
      <v-list-item
        v-for="item in menuItems"
        :key="item.path"
        :to="item.path"
        :prepend-icon="item.icon"
        :title="rail ? undefined : item.title"
        rounded="lg"
        color="primary"
      >
        <template v-if="rail" #append><span /></template>
      </v-list-item>
    </v-list>

    <template #append>
      <v-divider />
      <v-list density="compact" nav>
        <v-list-item
          :prepend-icon="rail ? 'mdi-chevron-right' : 'mdi-chevron-left'"
          :title="rail ? undefined : '收起侧栏'"
          rounded="lg"
          @click="rail = !rail"
        />
      </v-list>
    </template>
  </v-navigation-drawer>

  <v-app-bar flat border class="app-bar">
    <v-app-bar-nav-icon
      v-if="$vuetify.display.smAndDown"
      @click="drawer = !drawer"
    />
    <v-app-bar-title class="text-subtitle-1 font-weight-medium">
      {{ menuItems.find((m) => m.path === route.path)?.title || 'Paily' }}
    </v-app-bar-title>

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

  <v-snackbar v-model="apiError" :timeout="3000" color="error" location="bottom">
    {{ apiError }}
    <template #actions>
      <v-btn variant="text" @click="apiError = null">关闭</v-btn>
    </template>
  </v-snackbar>
</template>

<style scoped>
.app-drawer :deep(.v-navigation-drawer__content) {
  display: flex;
  flex-direction: column;
}

.brand {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 18px 20px;
  min-height: 64px;
}
.brand--rail {
  justify-content: center;
  padding-inline: 0;
}
.brand-text {
  font-size: 17px;
  font-weight: 700;
  letter-spacing: 0.2px;
  white-space: nowrap;
}
.brand-accent {
  font-weight: 400;
  opacity: 0.7;
}

.app-main {
  background: rgb(var(--v-theme-background));
}
</style>
