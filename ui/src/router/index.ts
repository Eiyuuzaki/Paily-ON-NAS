import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'
import { useAuthStore } from '../stores/auth'

declare module 'vue-router' {
  interface RouteMeta {
    requiresAuth?: boolean
  }
}

const routes: RouteRecordRaw[] = [
  {
    path: '/login',
    name: 'Login',
    component: () => import('../pages/Login.vue'),
    meta: { requiresAuth: false },
  },
  {
    path: '/',
    redirect: '/dashboard',
  },
  {
    path: '/',
    component: () => import('../layouts/AdminLayout.vue'),
    meta: { requiresAuth: true },
    children: [
      {
        path: 'dashboard',
        name: 'Dashboard',
        component: () => import('../pages/Dashboard.vue'),
      },
      {
        path: 'nodes',
        name: 'Nodes',
        component: () => import('../pages/Nodes.vue'),
      },
      {
        path: 'sources',
        name: 'Sources',
        component: () => import('../pages/Sources.vue'),
      },
      {
        path: 'sponsors',
        name: 'Sponsors',
        component: () => import('../pages/Sponsors.vue'),
      },
      {
        path: 'filter',
        name: 'Filter',
        component: () => import('../pages/Filter.vue'),
      },
      {
        path: 'config',
        name: 'Config',
        component: () => import('../pages/Config.vue'),
      },
      {
        path: 'format-configs',
        name: 'FormatConfigs',
        component: () => import('../pages/FormatConfigs.vue'),
      },
      {
        path: 'logs',
        name: 'Logs',
        component: () => import('../pages/Logs.vue'),
      },
    ],
  },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

router.beforeEach((to) => {
  const authStore = useAuthStore()
  if (to.meta.requiresAuth && !authStore.isLoggedIn) {
    return '/login'
  }
})

export default router
