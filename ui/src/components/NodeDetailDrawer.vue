<script setup lang="ts">
import { ref, watch } from 'vue'
import { getNode, getInitialChecks, getDeepChecks } from '../api/nodes'
import { getCheckers } from '../api/checkers'
import StatusTag from './StatusTag.vue'
import type { NodeDetail, InitialCheckRecord, DeepCheckRecord } from '../types/api'
import { formatDate } from '../utils/format'

const props = defineProps<{ nodeId: string; modelValue: boolean }>()
const emit = defineEmits<{
  (e: 'update:modelValue', v: boolean): void
  (e: 'open-source', id: string): void
}>()

const detail = ref<NodeDetail | null>(null)
const loadingDetail = ref(false)
const activeTab = ref('info')

const initialChecks = ref<InitialCheckRecord[]>([])
const initialTotal = ref(0)
const initialPage = ref(1)
const loadingInitial = ref(false)
const initialTagFilter = ref('')

const deepChecks = ref<DeepCheckRecord[]>([])
const deepTotal = ref(0)
const deepPage = ref(1)
const loadingDeep = ref(false)
const deepTagFilter = ref('')

const checkerTags = ref<string[]>([])

async function loadDetail() {
  loadingDetail.value = true
  try {
    const res = await getNode(props.nodeId)
    detail.value = res.data
  } finally {
    loadingDetail.value = false
  }
}

async function loadInitialChecks(p = 1) {
  loadingInitial.value = true
  try {
    const params: Record<string, unknown> = { page: p, limit: 20 }
    if (initialTagFilter.value) params.tag = initialTagFilter.value
    const res = await getInitialChecks(props.nodeId, params)
    initialChecks.value = res.data.data
    initialTotal.value = res.data.pagination.total
    initialPage.value = p
  } finally {
    loadingInitial.value = false
  }
}

async function loadDeepChecks(p = 1) {
  loadingDeep.value = true
  try {
    const params: Record<string, unknown> = { page: p, limit: 20 }
    if (deepTagFilter.value) params.tag = deepTagFilter.value
    const res = await getDeepChecks(props.nodeId, params)
    deepChecks.value = res.data.data
    deepTotal.value = res.data.pagination.total
    deepPage.value = p
  } finally {
    loadingDeep.value = false
  }
}

watch(
  () => [props.modelValue, props.nodeId] as const,
  ([open]) => {
    if (open) {
      activeTab.value = 'info'
      detail.value = null
      initialChecks.value = []
      deepChecks.value = []
      initialTagFilter.value = ''
      deepTagFilter.value = ''
      loadDetail()
      getCheckers().then((r) => { checkerTags.value = r.data.checkers }).catch(() => {})
    }
  },
  { immediate: true },
)

function onTabChange(tab: unknown) {
  if (tab === 'initial' && initialChecks.value.length === 0) loadInitialChecks()
  if (tab === 'deep' && deepChecks.value.length === 0) loadDeepChecks()
}

function parseStreaming(raw: unknown): Record<string, boolean> {
  if (!raw) return {}
  if (typeof raw === 'string') {
    try {
      return JSON.parse(raw)
    } catch {
      return {}
    }
  }
  return raw as Record<string, boolean>
}

function close() {
  emit('update:modelValue', false)
}

const initialHeaders = [
  { title: 'ID', key: 'id', minWidth: 64 },
  { title: 'Checker', key: 'checker_tag', minWidth: 120 },
  { title: '延迟 (ms)', key: 'latency_ms', minWidth: 110 },
  { title: '检测时间', key: 'checked_at', minWidth: 160 },
]
const deepHeaders = [
  { title: 'ID', key: 'id', minWidth: 64 },
  { title: 'Checker', key: 'checker_tag', minWidth: 110 },
  { title: '平均延迟', key: 'avg_latency_ms', minWidth: 95 },
  { title: '抖动', key: 'jitter_ms', minWidth: 80 },
  { title: '速度', key: 'speed_kbps', minWidth: 100 },
  { title: '流媒体', key: 'streaming', minWidth: 150, sortable: false },
  { title: '检测时间', key: 'checked_at', minWidth: 160 },
]
</script>

<template>
  <v-navigation-drawer
    :model-value="modelValue"
    location="right"
    temporary
    width="700"
    @update:model-value="(v: boolean) => emit('update:modelValue', v)"
  >
    <div class="drawer-head">
      <div>
        <div class="text-subtitle-1 font-weight-bold">节点详情</div>
        <div v-if="detail" class="text-caption text-disabled mono">{{ detail.server }}</div>
      </div>
      <v-spacer />
      <v-btn icon="mdi-close" variant="text" size="small" @click="close" />
    </div>
    <v-divider />

    <v-tabs v-model="activeTab" color="primary" density="comfortable" @update:model-value="onTabChange">
      <v-tab value="info">基本信息</v-tab>
      <v-tab value="initial">初步检测</v-tab>
      <v-tab value="deep">深度检测</v-tab>
    </v-tabs>

    <v-window v-model="activeTab">
      <!-- 基本信息 -->
      <v-window-item value="info">
        <v-card-text :loading="loadingDetail" style="min-height: 200px">
          <template v-if="detail">
            <v-row dense>
              <v-col cols="6">
                <div class="text-caption text-disabled">协议</div>
                <v-chip size="small" variant="tonal" color="primary">{{ detail.protocol }}</v-chip>
              </v-col>
              <v-col cols="6">
                <div class="text-caption text-disabled">区域</div>
                <div>{{ detail.region || '—' }}</div>
              </v-col>
              <v-col cols="12" class="mt-2">
                <div class="text-caption text-disabled">服务器</div>
                <div class="mono">{{ detail.server }}</div>
              </v-col>
              <v-col cols="12" class="mt-2">
                <div class="text-caption text-disabled">ID</div>
                <div class="mono text-caption">{{ detail.id }}</div>
              </v-col>
              <v-col cols="12" class="mt-2">
                <div class="text-caption text-disabled">Hash</div>
                <div class="mono text-caption">{{ detail.hash }}</div>
              </v-col>
              <v-col cols="6" class="mt-2">
                <div class="text-caption text-disabled">存活</div>
                <StatusTag :status="detail.alive ? 'alive' : 'dead'" />
              </v-col>
              <v-col cols="12" class="mt-2">
                <div class="text-caption text-disabled mb-1">分数 {{ detail.score.toFixed(3) }}</div>
                <v-progress-linear
                  :model-value="Math.round(detail.score * 100)"
                  :color="detail.score >= 0.6 ? 'success' : detail.score >= 0.3 ? 'warning' : 'error'"
                  height="10"
                  rounded
                />
              </v-col>
              <v-col cols="6" class="mt-2">
                <div class="text-caption text-disabled">创建时间</div>
                <div class="text-caption">{{ formatDate(detail.created_at) }}</div>
              </v-col>
              <v-col cols="6" class="mt-2">
                <div class="text-caption text-disabled">更新时间</div>
                <div class="text-caption">{{ formatDate(detail.updated_at) }}</div>
              </v-col>
            </v-row>

            <!-- 复筛结果 -->
            <div class="mt-4">
              <div class="text-subtitle-2 mb-2">复筛检测结果（按 Checker）</div>
              <template v-if="detail.latest_deep_check_by_tag && Object.keys(detail.latest_deep_check_by_tag).length">
                <v-expansion-panels variant="accordion">
                  <v-expansion-panel v-for="(dc, tag) in detail.latest_deep_check_by_tag" :key="tag">
                    <v-expansion-panel-title>
                      <span class="font-weight-medium">{{ tag }}</span>
                      <span class="text-caption text-disabled ml-4">
                        {{ dc.avg_latency_ms }} ms · 抖动 {{ dc.jitter_ms }} ms · {{ dc.speed_kbps }} kbps
                      </span>
                    </v-expansion-panel-title>
                    <v-expansion-panel-text>
                      <div class="d-flex flex-wrap ga-1 mb-2">
                        <v-chip
                          v-for="(val, key) in parseStreaming(dc.streaming_result)"
                          :key="key"
                          size="x-small"
                          :color="val ? 'success' : 'default'"
                          variant="tonal"
                        >
                          {{ key }}: {{ val ? '✓' : '✗' }}
                        </v-chip>
                        <span v-if="!Object.keys(parseStreaming(dc.streaming_result)).length" class="text-caption text-disabled">
                          无流媒体数据
                        </span>
                      </div>
                      <div class="text-caption text-disabled">检测时间: {{ formatDate(dc.checked_at) }}</div>
                    </v-expansion-panel-text>
                  </v-expansion-panel>
                </v-expansion-panels>
              </template>
              <template v-else-if="detail.latest_deep_check">
                <div class="d-flex flex-wrap ga-1">
                  <v-chip
                    v-for="(val, key) in parseStreaming(detail.latest_deep_check.streaming_result)"
                    :key="key"
                    size="x-small"
                    :color="val ? 'success' : 'default'"
                    variant="tonal"
                  >
                    {{ key }}: {{ val ? '✓' : '✗' }}
                  </v-chip>
                </div>
              </template>
              <span v-else class="text-caption text-disabled">暂无复筛检测数据</span>
            </div>

            <!-- 关联来源 -->
            <div v-if="detail.sources && detail.sources.length" class="mt-4">
              <div class="text-subtitle-2 mb-2">关联来源（{{ detail.sources.length }}）</div>
              <div class="d-flex flex-wrap ga-1">
                <v-chip
                  v-for="src in detail.sources"
                  :key="src.id"
                  size="small"
                  variant="tonal"
                  color="info"
                  @click="emit('open-source', src.id)"
                >
                  {{ src.info || src.identifier || src.id }}
                </v-chip>
              </div>
            </div>

            <!-- raw -->
            <div v-if="detail.raw" class="mt-4">
              <div class="text-subtitle-2 mb-2">节点配置</div>
              <v-sheet rounded="lg" class="pa-3 code-box mono">
                <pre>{{ JSON.stringify(detail.raw, null, 2) }}</pre>
              </v-sheet>
            </div>
          </template>
        </v-card-text>
      </v-window-item>

      <!-- 初步检测 -->
      <v-window-item value="initial">
        <v-card-text>
          <div class="d-flex align-center ga-2 mb-3">
            <span class="text-caption text-disabled">Checker</span>
            <v-select
              v-model="initialTagFilter"
              :items="checkerTags"
              density="compact"
              hide-details
              clearable
              placeholder="全部"
              style="max-width: 160px"
              @update:model-value="loadInitialChecks(1)"
            />
          </div>
          <v-data-table :headers="initialHeaders" :items="initialChecks" :loading="loadingInitial" density="compact" hide-default-footer :items-per-page="-1">
            <template #item.checked_at="{ item }">{{ formatDate(item.checked_at) }}</template>
            <template #no-data><div class="py-6 text-center text-disabled text-caption">暂无记录</div></template>
          </v-data-table>
          <div class="d-flex justify-end mt-2">
            <v-pagination v-model="initialPage" :length="Math.max(1, Math.ceil(initialTotal / 20))" density="compact" :total-visible="5" @update:model-value="loadInitialChecks" />
          </div>
        </v-card-text>
      </v-window-item>

      <!-- 深度检测 -->
      <v-window-item value="deep">
        <v-card-text>
          <div class="d-flex align-center ga-2 mb-3">
            <span class="text-caption text-disabled">Checker</span>
            <v-select
              v-model="deepTagFilter"
              :items="checkerTags"
              density="compact"
              hide-details
              clearable
              placeholder="全部"
              style="max-width: 160px"
              @update:model-value="loadDeepChecks(1)"
            />
          </div>
          <v-data-table :headers="deepHeaders" :items="deepChecks" :loading="loadingDeep" density="compact" hide-default-footer :items-per-page="-1">
            <template #item.streaming="{ item }">
              <span v-for="(val, key) in parseStreaming(item.streaming_result)" :key="key" class="text-caption mr-2">
                {{ key }}:{{ val ? '✓' : '✗' }}
              </span>
            </template>
            <template #item.checked_at="{ item }">{{ formatDate(item.checked_at) }}</template>
            <template #no-data><div class="py-6 text-center text-disabled text-caption">暂无记录</div></template>
          </v-data-table>
          <div class="d-flex justify-end mt-2">
            <v-pagination v-model="deepPage" :length="Math.max(1, Math.ceil(deepTotal / 20))" density="compact" :total-visible="5" @update:model-value="loadDeepChecks" />
          </div>
        </v-card-text>
      </v-window-item>
    </v-window>
  </v-navigation-drawer>
</template>

<style scoped>
.drawer-head {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 12px 16px;
}
.code-box {
  max-height: 320px;
  overflow: auto;
  background: rgba(var(--v-border-color), 0.08);
  font-size: 11px;
}
.code-box pre {
  margin: 0;
  white-space: pre;
}
</style>
