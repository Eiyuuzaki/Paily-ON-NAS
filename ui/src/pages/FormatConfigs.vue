<script setup lang="ts">
import { ref, reactive, onMounted, computed } from 'vue'
import { getFormatConfig, updateFormatConfig } from '../api/formatConfigs'
import type { FormatName } from '../api/formatConfigs'

const BASE = (import.meta.env.VITE_API_BASE as string) || ''

const formats: FormatName[] = ['clash', 'singbox', 'base64']
const subUrls: Record<FormatName, string> = {
  clash: `${BASE}/clash`,
  singbox: `${BASE}/singbox`,
  base64: `${BASE}/base64`,
}
const formatLabels: Record<FormatName, string> = {
  clash: 'Clash',
  singbox: 'Sing-box',
  base64: 'Base64',
}
const formatIcons: Record<FormatName, string> = {
  clash: 'mdi-shield-half-full',
  singbox: 'mdi-cube-outline',
  base64: 'mdi-code-braces',
}

interface TabState {
  config: Record<string, unknown>
  loading: boolean
  saving: boolean
  loaded: boolean
}

const activeTab = ref<FormatName>('clash')
const copied = ref<string | null>(null)
const snack = ref<string | null>(null)
const snackColor = ref('success')
const tabs = reactive<Record<FormatName, TabState>>({
  clash: { config: {}, loading: false, saving: false, loaded: false },
  singbox: { config: {}, loading: false, saving: false, loaded: false },
  base64: { config: {}, loading: false, saving: false, loaded: false },
})

function notify(text: string, color = 'success') {
  snack.value = text
  snackColor.value = color
  setTimeout(() => (snack.value = null), 2000)
}

async function loadTab(fmt: FormatName) {
  const t = tabs[fmt]
  if (t.loaded) return
  t.loading = true
  try {
    const res = await getFormatConfig(fmt)
    for (const k of Object.keys(t.config)) delete (t.config as Record<string, unknown>)[k]
    Object.assign(t.config, res.data.config)
    if ((fmt === 'clash' || fmt === 'singbox') && !('template' in t.config)) t.config['template'] = ''
    t.loaded = true
  } finally {
    t.loading = false
  }
}

async function reloadTab(fmt: FormatName) {
  tabs[fmt].loaded = false
  await loadTab(fmt)
}

async function saveTab(fmt: FormatName) {
  const t = tabs[fmt]
  t.saving = true
  try {
    await updateFormatConfig(fmt, { config: t.config })
    notify(`${formatLabels[fmt]} 配置已保存`)
  } finally {
    t.saving = false
  }
}

async function copyUrl(fmt: FormatName, url: string) {
  try {
    await navigator.clipboard.writeText(url)
    copied.value = fmt
    setTimeout(() => (copied.value = null), 1600)
  } catch {
    notify('复制失败', 'error')
  }
}

function onTabChange(fmt: unknown) {
  loadTab(fmt as FormatName)
}

function fieldType(fmt: FormatName, key: string): 'template' | 'base64default' | 'generic' {
  if ((fmt === 'clash' || fmt === 'singbox') && key === 'template') return 'template'
  if (fmt === 'base64' && key === 'default') return 'base64default'
  return 'generic'
}

function strVal(v: unknown): string {
  return v == null ? '' : String(v)
}

const currentConfigKeys = computed(() => Object.keys(tabs[activeTab.value].config))

onMounted(() => loadTab('clash'))
</script>

<template>
  <div>
    <div class="page-head">
      <div>
        <h1 class="page-title">格式配置</h1>
        <p class="page-sub">各订阅格式的模板与交付参数</p>
      </div>
    </div>

    <v-card flat rounded="xl" class="bg-surface-container-low">
      <v-tabs v-model="activeTab" color="primary" density="comfortable" @update:model-value="onTabChange">
        <v-tab v-for="fmt in formats" :key="fmt" :value="fmt" :prepend-icon="formatIcons[fmt]">
          {{ formatLabels[fmt] }}
        </v-tab>
      </v-tabs>

      <v-window v-model="activeTab">
        <v-window-item v-for="fmt in formats" :key="fmt" :value="fmt">
          <v-card-text :loading="tabs[fmt].loading" style="min-height: 240px">
            <v-row dense>
              <v-col
                v-for="key in Object.keys(tabs[fmt].config)"
                :key="key"
                :cols="fieldType(fmt, key) === 'template' ? 12 : 12"
                :md="fieldType(fmt, key) === 'template' ? 12 : 6"
              >
                <!-- template 大文本框 -->
                <template v-if="fieldType(fmt, key) === 'template'">
                  <div class="text-subtitle-2 mb-2">{{ key }}</div>
                  <v-textarea
                    :model-value="strVal(tabs[fmt].config[key])"
                    :placeholder="key"
                    rows="18"
                    class="mono"
                    @update:model-value="(v: string) => (tabs[fmt].config[key] = v)"
                  />
                </template>

                <!-- base64 default 枚举 -->
                <v-select
                  v-else-if="fieldType(fmt, key) === 'base64default'"
                  :model-value="strVal(tabs[fmt].config[key])"
                  :items="[{ title: 'v2rayn', value: 'v2rayn' }, { title: 'subconverter', value: 'subconverter' }]"
                  label="default"
                  @update:model-value="(v: string) => (tabs[fmt].config[key] = v)"
                />

                <!-- 通用 -->
                <v-text-field
                  v-else
                  :model-value="strVal(tabs[fmt].config[key])"
                  :label="key"
                  :placeholder="key"
                  @update:model-value="(v: string) => (tabs[fmt].config[key] = v)"
                />
              </v-col>
            </v-row>

            <div v-if="Object.keys(tabs[fmt].config).length === 0 && !tabs[fmt].loading" class="text-disabled py-6 text-center">
              暂无字段
            </div>

            <v-divider class="my-4" />

            <div class="d-flex align-center ga-3 flex-wrap">
              <v-icon icon="mdi-link-variant" color="primary" />
              <span class="text-caption text-disabled">订阅链接</span>
              <span class="mono text-caption">{{ subUrls[fmt] }}</span>
              <v-btn
                size="small"
                variant="tonal"
                :prepend-icon="copied === fmt ? 'mdi-check' : 'mdi-content-copy'"
                @click="copyUrl(fmt, subUrls[fmt])"
              >
                {{ copied === fmt ? '已复制' : '复制' }}
              </v-btn>
              <v-spacer />
              <v-btn variant="text" prepend-icon="mdi-refresh" @click="reloadTab(fmt)">重新加载</v-btn>
              <v-btn color="primary" prepend-icon="mdi-content-save" :loading="tabs[fmt].saving" @click="saveTab(fmt)">
                保存
              </v-btn>
            </div>
          </v-card-text>
        </v-window-item>
      </v-window>
    </v-card>

    <v-snackbar :model-value="!!snack" :color="snackColor" :timeout="2000" location="bottom">
      {{ snack }}
    </v-snackbar>
  </div>
</template>

<style scoped>
.page-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  flex-wrap: wrap;
  margin-bottom: 24px;
}
.page-title {
  font-size: 24px;
  line-height: 32px;
  font-weight: 400;
  margin: 0;
  letter-spacing: 0;
  color: rgb(var(--v-theme-on-surface));
}
.page-sub {
  font-size: 14px;
  line-height: 20px;
  margin: 4px 0 0;
  color: rgb(var(--v-theme-on-surface-variant));
}


.mono :deep(textarea) {
  font-family: ui-monospace, Consolas, monospace;
  font-size: 12px;
}
</style>
