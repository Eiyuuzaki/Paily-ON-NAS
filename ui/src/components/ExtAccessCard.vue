<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import {
  getExtAccess,
  setExtToken,
  rotateExtToken,
  setExtHost,
  testExtAccess,
  type ExtAccessInfo,
  type ExtTestResult,
} from '../api/extaccess'
import { copyText } from '../utils/clipboard'

const info = ref<ExtAccessInfo | null>(null)
const showToken = ref(false)
const tokenDraft = ref('')
const hostDraft = ref('')
const saving = ref(false)
const testing = ref(false)
const testResult = ref<ExtTestResult | null>(null)
const copiedKey = ref('')
const mode = ref<'public' | 'lan'>('public')

// Do not hardcode the NAS address here: this file is committed, and a LAN IP in a
// public source tree is an internal detail that does not belong in it. In a
// browser window.location.hostname is always set, so the fallback only covers
// non-browser contexts.
const lanBase = computed(() => `http://${window.location.hostname || 'localhost'}:${info.value?.lan_port ?? 21380}`)

const linkRows = computed(() => {
  const tok = info.value?.token ?? ''
  const base = mode.value === 'lan' ? lanBase.value : `https://${info.value?.public_host ?? 'sub.lencf.dpdns.org'}`
  return [
    { key: 'clash', label: 'Clash', url: `${base}/clash?token=${tok}` },
    { key: 'singbox', label: 'Sing-box', url: `${base}/singbox?token=${tok}` },
    { key: 'base64', label: 'Base64', url: `${base}/base64?token=${tok}` },
  ]
})

async function load() {
  try {
    const res: any = await getExtAccess()
    info.value = res.data?.data ?? res.data
    tokenDraft.value = info.value?.token ?? ''
    hostDraft.value = info.value?.public_host ?? ''
  } catch (e) {
    console.warn('[ExtAccessCard] load failed', e)
  }
}

async function saveToken() {
  const tok = tokenDraft.value.trim()
  if (tok.length < 16) return
  saving.value = true
  try {
    await setExtToken(tok)
    await load()
  } catch (e) {
    console.warn('[ExtAccessCard] saveToken', e)
  } finally {
    saving.value = false
  }
}

async function doRotate() {
  saving.value = true
  try {
    await rotateExtToken()
    await load()
    tokenDraft.value = info.value?.token ?? ''
  } catch (e) {
    console.warn('[ExtAccessCard] rotate', e)
  } finally {
    saving.value = false
  }
}

async function saveHost() {
  saving.value = true
  try {
    await setExtHost(hostDraft.value.trim())
    await load()
  } catch (e) {
    console.warn('[ExtAccessCard] saveHost', e)
  } finally {
    saving.value = false
  }
}

async function runTest() {
  testing.value = true
  try {
    const res: any = await testExtAccess()
    testResult.value = res.data?.data ?? res.data
  } catch (e) {
    console.warn('[ExtAccessCard] test', e)
  } finally {
    testing.value = false
  }
}

function copy(url: string, key: string) {
  copyText(url)
  copiedKey.value = key
  setTimeout(() => (copiedKey.value = ''), 1500)
}

onMounted(load)
</script>

<template>
  <v-card flat rounded="xl" class="bg-surface-container-low">
    <v-card-title class="d-flex align-center">
      <span>外网访问</span>
      <v-spacer />
      <v-chip
        v-if="testResult"
        size="small"
        :color="testResult.ok ? 'success' : 'error'"
        variant="tonal"
        class="mr-2"
      >
        {{ testResult.ok ? `公网可达 · ${testResult.latency_ms}ms` : `不可达 · ${testResult.http_code}` }}
      </v-chip>
      <v-btn variant="text" size="small" :loading="testing" @click="runTest">测试</v-btn>
    </v-card-title>
    <v-card-text>
      <v-row dense>
        <v-col cols="12" md="6">
          <v-text-field
            v-model="tokenDraft"
            label="订阅 Token"
            :type="showToken ? 'text' : 'password'"
            density="comfortable"
            hide-details
          >
            <template #append-inner>
              <v-icon
                :icon="showToken ? 'mdi-eye-off' : 'mdi-eye'"
                size="small"
                class="mr-2"
                @click="showToken = !showToken"
              />
              <v-icon icon="mdi-refresh" size="small" @click="doRotate" />
            </template>
          </v-text-field>
        </v-col>
        <v-col cols="12" md="6">
          <v-text-field
            v-model="hostDraft"
            label="公网域名"
            density="comfortable"
            hide-details
          />
        </v-col>
      </v-row>
      <div class="d-flex justify-end mt-2">
        <v-btn variant="text" size="small" :loading="saving" @click="saveHost">保存域名</v-btn>
        <v-btn variant="text" size="small" :loading="saving" @click="saveToken">保存令牌</v-btn>
      </div>

      <v-divider class="my-3" />

      <div class="d-flex align-center mb-2">
        <v-btn-toggle v-model="mode" mandatory density="compact" variant="outlined" color="primary">
          <v-btn value="public" size="small">公网</v-btn>
          <v-btn value="lan" size="small">局域网</v-btn>
        </v-btn-toggle>
      </div>
      <div class="text-caption text-disabled mb-1">订阅链接</div>
      <div
        v-for="row in linkRows"
        :key="row.key"
        class="d-flex align-center ga-3 py-1 url-row"
      >
        <v-icon icon="mdi-link-variant" size="small" color="primary" />
        <span class="text-caption">{{ row.label }}</span>
        <span class="mono text-caption url-text url-hidden text-medium-emphasis">{{ row.url }}</span>
        <v-spacer />
        <v-btn
          size="x-small"
          variant="tonal"
          :prepend-icon="copiedKey === row.key ? 'mdi-check' : 'mdi-content-copy'"
          @click="copy(row.url, row.key)"
        >
          {{ copiedKey === row.key ? '已复制' : '复制' }}
        </v-btn>
      </div>
    </v-card-text>
  </v-card>
</template>

<style scoped>
.url-text.url-hidden {
  filter: blur(5px);
  transition: filter 0.15s ease;
}
.url-row:hover .url-text.url-hidden {
  filter: none;
}
.url-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
  flex: 1;
}
.mono {
  font-family: monospace;
}
</style>
