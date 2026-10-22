<script setup lang="ts">
import { ref, reactive } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const router = useRouter()
const authStore = useAuthStore()

const loading = ref(false)
const errorMsg = ref('')
const showPassword = ref(false)
const form = reactive({ password: '' })
const formRef = ref<any>(null)

const rules = {
  password: [(v: string) => !!v || '请输入密码'],
}

async function handleLogin() {
  errorMsg.value = ''
  const { valid } = await formRef.value.validate()
  if (!valid) return

  loading.value = true
  try {
    await authStore.login(form.password)
    router.push('/dashboard')
  } catch (e: any) {
    errorMsg.value = e?.response?.data?.error || '密码错误，请重试'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="login-shell">
    <div class="login-glow" />
    <v-card class="login-card" elevation="12" rounded="xl">
      <div class="login-head">
        <v-avatar color="primary" size="56" rounded="lg" class="mb-4">
          <v-icon icon="mdi-hexagon-multiple" size="30" color="white" />
        </v-avatar>
        <div class="login-title">Paily <span class="muted">ON NAS</span></div>
        <div class="login-sub">节点聚合管理控制台</div>
      </div>

      <v-card-text class="pb-2">
        <v-form ref="formRef" :model="form" :rules="rules" @submit.prevent="handleLogin">
          <v-text-field
            v-model="form.password"
            :type="showPassword ? 'text' : 'password'"
            :append-inner-icon="showPassword ? 'mdi-eye-off' : 'mdi-eye'"
            label="管理密码"
            prepend-inner-icon="mdi-lock-outline"
            autofocus
            @click:append-inner="showPassword = !showPassword"
            @keyup.enter="handleLogin"
          />

          <v-alert
            v-if="errorMsg"
            type="error"
            variant="tonal"
            density="compact"
            class="mb-3"
            :text="errorMsg"
          />

          <v-btn
            block
            size="large"
            color="primary"
            :loading="loading"
            class="mt-2"
            @click="handleLogin"
          >
            登录
          </v-btn>
        </v-form>
      </v-card-text>

      <v-card-subtitle class="text-center text-caption pt-0 pb-4">
        自用部署 · 请勿公网暴露
      </v-card-subtitle>
    </v-card>
  </div>
</template>

<style scoped>
.login-shell {
  position: relative;
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgb(var(--v-theme-background));
  overflow: hidden;
}

.login-glow {
  position: absolute;
  width: 620px;
  height: 620px;
  border-radius: 50%;
  background: radial-gradient(circle, rgba(var(--v-theme-primary), 0.28) 0%, transparent 68%);
  filter: blur(10px);
  pointer-events: none;
}

.login-card {
  position: relative;
  width: min(92vw, 400px);
  backdrop-filter: blur(6px);
}

.login-head {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 32px 24px 8px;
}

.login-title {
  font-size: 22px;
  font-weight: 700;
  letter-spacing: 0.3px;
}

.login-title .muted {
  font-weight: 400;
  opacity: 0.65;
}

.login-sub {
  font-size: 13px;
  opacity: 0.6;
  margin-top: 2px;
}
</style>
