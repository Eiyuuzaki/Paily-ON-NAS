<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { getFetchLogs, getCheckLogs } from '../api/logs'
import StatusTag from '../components/StatusTag.vue'
import SourceDetailDrawer from '../components/SourceDetailDrawer.vue'
import NodeDetailDrawer from '../components/NodeDetailDrawer.vue'
import type { FetchLogEntry, CheckLogEntry } from '../types/api'
import { formatDate } from '../utils/format'

const activeTab = ref('fetch')
const pageOptions = [20, 50, 100]

// ── drawers ───────────────────────────────────────────────────────────
const sourceDrawerVisible = ref(false)
const selectedSourceId = ref<string | null>(null)
const nodeDrawerVisible = ref(false)
const selectedNodeId = ref<string | null>(null)

function openSource(id: string) {
  selectedSourceId.value = id
  sourceDrawerVisible.value = true
}
function openNode(id: string) {
  selectedNodeId.value = id
  nodeDrawerVisible.value = true
}

// ── Fetch logs ────────────────────────────────────────────────────────
const fetchLogs = ref<FetchLogEntry[]>([])
const fetchTotal = ref(0)
const fetchPage = ref(1)
const fetchPageSize = ref(20)
const fetchLoading = ref(false)
const fetchFilter = reactive({ source_id: '' })

async function loadFetchLogs(p = fetchPage.value) {
  fetchLoading.value = true
  fetchPage.value = p
  try {
    const params: Record<string, unknown> = { page: p, limit: fetchPageSize.value }
    if (fetchFilter.source_id) params.source_id = fetchFilter.source_id
    const res = await getFetchLogs(params)
    fetchLogs.value = res.data.data
    fetchTotal.value = res.data.pagination.total
  } finally {
    fetchLoading.value = false
  }
}

// ── Check logs ────────────────────────────────────────────────────────
const checkLogs = ref<CheckLogEntry[]>([])
const checkTotal = ref(0)
const checkPage = ref(1)
const checkPageSize = ref(20)
const checkLoading = ref(false)
const checkFilter = reactive({ node_id: '' })

async function loadCheckLogs(p = checkPage.value) {
  checkLoading.value = true
  checkPage.value = p
  try {
    const params: Record<string, unknown> = { page: p, limit: checkPageSize.value }
    if (checkFilter.node_id) params.node_id = checkFilter.node_id
    const res = await getCheckLogs(params)
    checkLogs.value = res.data.data
    checkTotal.value = res.data.pagination.total
  } finally {
    checkLoading.value = false
  }
}

function onTabChange(tab: unknown) {
  if (tab === 'check' && checkLogs.value.length === 0) loadCheckLogs(1)
}

const fetchHeaders = [
  { title: 'ID', key: 'id', width: 70 },
  { title: '来源 ID', key: 'source_id', minWidth: 280 },
  { title: '结果', key: 'success', width: 90 },
  { title: '节点数', key: 'node_count', width: 90 },
  { title: '抓取时间', key: 'fetched_at', minWidth: 170 },
]
const checkHeaders = [
  { title: 'ID', key: 'id', width: 70 },
  { title: '节点 ID', key: 'node_id', minWidth: 280 },
  { title: '延迟 (ms)', key: 'latency_ms', width: 110 },
  { title: '检测时间', key: 'checked_at', minWidth: 170 },
]

onMounted(() => loadFetchLogs(1))
</script>

<template>
  <div>
    <div class="page-head">
      <h1 class="page-title">日志</h1>
    </div>

    <v-card rounded="lg">
      <v-tabs v-model="activeTab" color="primary" density="comfortable" @update:model-value="onTabChange">
        <v-tab value="fetch" prepend-icon="mdi-download-network">抓取日志</v-tab>
        <v-tab value="check" prepend-icon="mdi-heart-pulse">检测日志</v-tab>
      </v-tabs>

      <v-window v-model="activeTab">
        <!-- 抓取日志 -->
        <v-window-item value="fetch">
          <v-card-text>
            <div class="d-flex align-center ga-2 mb-3 flex-wrap">
              <v-text-field
                v-model="fetchFilter.source_id"
                label="来源 ID（UUID）"
                density="compact"
                hide-details
                clearable
                class="mono filter-input"
                @keyup.enter="loadFetchLogs(1)"
              />
              <v-btn color="primary" prepend-icon="mdi-magnify" @click="loadFetchLogs(1)">查询</v-btn>
              <v-btn variant="text" @click="fetchFilter.source_id = ''; loadFetchLogs(1)">重置</v-btn>
            </div>

            <v-data-table
              :headers="fetchHeaders"
              :items="fetchLogs"
              :loading="fetchLoading"
              density="compact"
              hide-default-footer
            >
              <template #item.source_id="{ item }">
                <v-btn variant="text" size="small" class="mono px-1" color="primary" @click="openSource(item.source_id)">
                  {{ item.source_id }}
                </v-btn>
              </template>
              <template #item.success="{ item }">
                <StatusTag :status="item.success ? '成功' : '失败'" />
              </template>
              <template #item.fetched_at="{ item }">
                {{ formatDate(item.fetched_at) }}
              </template>
              <template #no-data>
                <div class="py-8 text-center text-disabled">暂无日志</div>
              </template>
            </v-data-table>

            <v-divider />
            <div class="d-flex align-center justify-end pa-3 ga-3 flex-wrap">
              <span class="text-caption text-disabled">共 {{ fetchTotal }} 条</span>
              <v-select
                v-model="fetchPageSize"
                :items="pageOptions"
                density="compact"
                hide-details
                style="max-width: 110px"
                @update:model-value="fetchPage = 1; loadFetchLogs(1)"
              />
              <v-pagination
                v-model="fetchPage"
                :length="Math.max(1, Math.ceil(fetchTotal / fetchPageSize))"
                :total-visible="5"
                density="compact"
                @update:model-value="loadFetchLogs"
              />
            </div>
          </v-card-text>
        </v-window-item>

        <!-- 检测日志 -->
        <v-window-item value="check">
          <v-card-text>
            <div class="d-flex align-center ga-2 mb-3 flex-wrap">
              <v-text-field
                v-model="checkFilter.node_id"
                label="节点 ID（UUID）"
                density="compact"
                hide-details
                clearable
                class="mono filter-input"
                @keyup.enter="loadCheckLogs(1)"
              />
              <v-btn color="primary" prepend-icon="mdi-magnify" @click="loadCheckLogs(1)">查询</v-btn>
              <v-btn variant="text" @click="checkFilter.node_id = ''; loadCheckLogs(1)">重置</v-btn>
            </div>

            <v-data-table
              :headers="checkHeaders"
              :items="checkLogs"
              :loading="checkLoading"
              density="compact"
              hide-default-footer
            >
              <template #item.node_id="{ item }">
                <v-btn variant="text" size="small" class="mono px-1" color="primary" @click="openNode(item.node_id)">
                  {{ item.node_id }}
                </v-btn>
              </template>
              <template #item.checked_at="{ item }">
                {{ formatDate(item.checked_at) }}
              </template>
              <template #no-data>
                <div class="py-8 text-center text-disabled">暂无日志</div>
              </template>
            </v-data-table>

            <v-divider />
            <div class="d-flex align-center justify-end pa-3 ga-3 flex-wrap">
              <span class="text-caption text-disabled">共 {{ checkTotal }} 条</span>
              <v-select
                v-model="checkPageSize"
                :items="pageOptions"
                density="compact"
                hide-details
                style="max-width: 110px"
                @update:model-value="checkPage = 1; loadCheckLogs(1)"
              />
              <v-pagination
                v-model="checkPage"
                :length="Math.max(1, Math.ceil(checkTotal / checkPageSize))"
                :total-visible="5"
                density="compact"
                @update:model-value="loadCheckLogs"
              />
            </div>
          </v-card-text>
        </v-window-item>
      </v-window>
    </v-card>

    <SourceDetailDrawer
      v-if="selectedSourceId"
      v-model="sourceDrawerVisible"
      :source-id="selectedSourceId"
      @open-node="openNode"
    />
    <NodeDetailDrawer
      v-if="selectedNodeId"
      v-model="nodeDrawerVisible"
      :node-id="selectedNodeId"
      @open-source="openSource"
    />
  </div>
</template>

<style scoped>
.page-head {
  margin-bottom: 20px;
}
.page-title {
  font-size: 24px;
  font-weight: 700;
  margin: 0;
}
.filter-input {
  max-width: 320px;
}
</style>
