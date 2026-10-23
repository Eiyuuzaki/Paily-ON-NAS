<script setup lang="ts">
import { ref, reactive, onMounted, computed } from 'vue'
import StatusTag from '../components/StatusTag.vue'
import ExprEditor from '../components/ExprEditor.vue'
import NodeDetailDrawer from '../components/NodeDetailDrawer.vue'
import SourceDetailDrawer from '../components/SourceDetailDrawer.vue'
import { getNodes, searchNodes, deleteNode } from '../api/nodes'
import { getCheckers } from '../api/checkers'
import type { NodeSummary, NodeListResponse } from '../types/api'
import { formatLatency } from '../utils/format'

// ── state ──────────────────────────────────────────────────────────────
const tableData = ref<NodeSummary[]>([])
const loading = ref(false)
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const snake = ref<string | null>(null)

const filtersOpen = ref(true)

const filters = reactive({
  alive: '' as '' | 'true' | 'false',
  region: '',
  protocol: '',
  server: '',
  source_id: '',
  min_score: null as number | null,
  max_score: null as number | null,
  has_streaming: '',
  tag: '',
  sort_by: '' as '' | 'region' | 'score',
  sort_dir: 'desc' as 'asc' | 'desc',
})

const checkerTags = ref<string[]>([])
const advancedMode = ref(false)
const exprValue = ref('')

// ── detail drawers ─────────────────────────────────────────────────────
const drawerVisible = ref(false)
const selectedNodeId = ref<string | null>(null)
const sourceDrawerVisible = ref(false)
const selectedSourceId = ref<string | null>(null)

function openSourceDrawer(id: string) {
  selectedSourceId.value = id
  sourceDrawerVisible.value = true
}

function snack(text: string) {
  snake.value = text
  setTimeout(() => (snake.value = null), 1800)
}

// ── fetch ──────────────────────────────────────────────────────────────
async function fetchNodes() {
  loading.value = true
  try {
    let res: { data: NodeListResponse }
    if (advancedMode.value && exprValue.value.trim()) {
      res = await searchNodes({ expr: exprValue.value.trim(), page: page.value, limit: pageSize.value })
    } else {
      const params: Record<string, unknown> = { page: page.value, limit: pageSize.value }
      if (filters.alive !== '') params.alive = filters.alive
      if (filters.region) params.region = filters.region
      if (filters.protocol) params.protocol = filters.protocol
      if (filters.server) params.server = filters.server
      if (filters.source_id) params.source_id = filters.source_id
      if (filters.min_score !== null) params.min_score = filters.min_score
      if (filters.max_score !== null) params.max_score = filters.max_score
      if (filters.has_streaming) params.has_streaming = filters.has_streaming
      if (filters.tag) params.tag = filters.tag
      if (filters.sort_by) params.sort_by = filters.sort_by
      if (filters.sort_dir) params.sort_dir = filters.sort_dir
      res = await getNodes(params)
    }
    tableData.value = res.data.data
    total.value = res.data.pagination.total
  } finally {
    loading.value = false
  }
}

function handleFilter() {
  page.value = 1
  fetchNodes()
}

function resetFilters() {
  Object.assign(filters, {
    alive: '', region: '', protocol: '', server: '', source_id: '',
    min_score: null, max_score: null, has_streaming: '', tag: '',
    sort_by: '', sort_dir: 'desc',
  })
  exprValue.value = ''
  page.value = 1
  fetchNodes()
}

async function handleDelete(node: NodeSummary) {
  if (!confirm(`确定删除节点 ${node.server}？`)) return
  await deleteNode(node.id)
  snack('已删除')
  fetchNodes()
}

async function copyId(id: string) {
  try {
    await navigator.clipboard.writeText(id)
    snack('已复制 ID')
  } catch {
    snack('复制失败')
  }
}

function openDetail(node: NodeSummary) {
  selectedNodeId.value = node.id
  drawerVisible.value = true
}

// ── 表格列（Vuetify data-table 用 headers 定义）─────────────────────────
const headers = [
  { title: 'ID', key: 'id', width: 110, sortable: false },
  { title: '协议', key: 'protocol', width: 90 },
  { title: '服务器', key: 'server', minWidth: 200 },
  { title: '来源', key: 'sources', minWidth: 140, sortable: false },
  { title: '区域', key: 'region', width: 80 },
  { title: '延迟', key: 'latency', width: 140, sortable: false },
  { title: '分数', key: 'score', width: 170 },
  { title: '状态', key: 'alive', width: 90 },
  { title: '操作', key: 'actions', width: 130, sortable: false, align: 'end' as const },
]

const scoreColor = (s: number) => (s >= 0.6 ? 'success' : s >= 0.3 ? 'warning' : 'error')

const pageOptions = [20, 50, 100]

onMounted(async () => {
  fetchNodes()
  try {
    const res = await getCheckers()
    checkerTags.value = res.data.checkers
  } catch { /* non-critical */ }
})
</script>

<template>
  <div>
    <div class="page-head">
      <div>
        <h1 class="page-title">节点管理</h1>
        <p class="page-sub">共 {{ total }} 个节点</p>
      </div>
      <v-btn
        :prepend-icon="filtersOpen ? 'mdi-filter-remove-outline' : 'mdi-filter-outline'"
        variant="tonal"
        @click="filtersOpen = !filtersOpen"
      >
        {{ filtersOpen ? '收起筛选' : '展开筛选' }}
      </v-btn>
    </div>

    <!-- 过滤器 -->
    <v-expand-transition>
      <v-card flat v-show="filtersOpen" rounded="xl" class="mb-4">
        <v-card-text>
          <v-row dense>
            <v-col cols="6" sm="4" md="2">
              <v-select
                v-model="filters.alive"
                :items="[{ title: '全部', value: '' }, { title: '存活', value: 'true' }, { title: '死亡', value: 'false' }]"
                label="存活"
                density="compact"
                hide-details
              />
            </v-col>
            <v-col cols="6" sm="4" md="2">
              <v-text-field v-model="filters.region" label="区域" placeholder="HK/US" density="compact" hide-details clearable />
            </v-col>
            <v-col cols="6" sm="4" md="2">
              <v-text-field v-model="filters.protocol" label="协议" placeholder="vmess" density="compact" hide-details clearable />
            </v-col>
            <v-col cols="6" sm="4" md="2">
              <v-text-field v-model="filters.server" label="服务器" placeholder="domain/ip" density="compact" hide-details clearable />
            </v-col>
            <v-col cols="6" sm="4" md="2">
              <v-text-field v-model="filters.source_id" label="来源 ID" placeholder="uuid" density="compact" hide-details clearable />
            </v-col>
            <v-col cols="6" sm="4" md="2">
              <v-select
                v-model="filters.tag"
                :items="checkerTags"
                label="Checker"
                density="compact"
                hide-details
                clearable
              />
            </v-col>
            <v-col cols="6" sm="4" md="2">
              <v-text-field v-model.number="filters.min_score" type="number" label="最低分" step="0.1" min="0" max="1" density="compact" hide-details clearable />
            </v-col>
            <v-col cols="6" sm="4" md="2">
              <v-text-field v-model.number="filters.max_score" type="number" label="最高分" step="0.1" min="0" max="1" density="compact" hide-details clearable />
            </v-col>
            <v-col cols="6" sm="4" md="2">
              <v-text-field v-model="filters.has_streaming" label="流媒体" placeholder="netflix" density="compact" hide-details clearable />
            </v-col>
            <v-col cols="6" sm="4" md="2">
              <v-select
                v-model="filters.sort_by"
                :items="[{ title: '默认', value: '' }, { title: '区域', value: 'region' }, { title: '分数', value: 'score' }]"
                label="排序"
                density="compact"
                hide-details
              />
            </v-col>
            <v-col cols="6" sm="4" md="2">
              <v-select
                v-model="filters.sort_dir"
                :items="[{ title: '降序', value: 'desc' }, { title: '升序', value: 'asc' }]"
                label="方向"
                density="compact"
                hide-details
              />
            </v-col>
            <v-col cols="12" md="2" class="d-flex align-center ga-2">
              <v-btn color="primary" prepend-icon="mdi-magnify" @click="handleFilter">查询</v-btn>
              <v-btn variant="text" @click="resetFilters">重置</v-btn>
            </v-col>
          </v-row>

          <div class="mt-2">
            <v-btn
              variant="text"
              size="small"
              :prepend-icon="advancedMode ? 'mdi-chevron-up' : 'mdi-code-braces'"
              @click="advancedMode = !advancedMode"
            >
              {{ advancedMode ? '隐藏高级搜索' : '高级搜索（expr-lang）' }}
            </v-btn>
            <div v-if="advancedMode" class="mt-2">
              <ExprEditor v-model="exprValue" placeholder="输入 expr-lang 表达式，例如 Region == 'HK'" />
            </div>
          </div>
        </v-card-text>
      </v-card>
    </v-expand-transition>

    <!-- 表格 -->
    <v-card flat rounded="xl" class="bg-surface-container-low">
      <v-data-table
        :headers="headers"
        :items="tableData"
        :loading="loading"
        item-value="id"
        :items-per-page="pageSize"
        hide-default-footer
        class="nodes-table"
      >
        <template #item.id="{ item }">
          <v-tooltip :text="item.id" location="top">
            <template #activator="{ props }">
              <v-btn
                v-bind="props"
                variant="text"
                size="small"
                class="mono px-1"
                color="primary"
                @click="copyId(item.id)"
              >
                {{ item.id.slice(0, 8) }}…
              </v-btn>
            </template>
          </v-tooltip>
        </template>

        <template #item.sources="{ item }">
          <template v-if="item.sources && item.sources.length">
            <v-chip
              v-for="src in item.sources"
              :key="src.id"
              size="x-small"
              variant="tonal"
              class="ma-1"
              @click="openSourceDrawer(src.id)"
            >
              {{ src.info || src.identifier || src.id.slice(0, 8) }}
            </v-chip>
          </template>
          <span v-else class="text-disabled">—</span>
        </template>

        <template #item.latency="{ item }">
          <div class="text-caption lh-tight">
            <div v-if="item.latest_latency_ms != null">
              <span class="text-disabled">初筛</span> {{ formatLatency(item.latest_latency_ms) }}
            </div>
            <div v-if="item.latest_avg_latency_ms != null">
              <span class="text-disabled">平均</span> {{ formatLatency(item.latest_avg_latency_ms) }}
              <span v-if="item.latest_jitter_ms != null" class="text-disabled">±{{ item.latest_jitter_ms }}ms</span>
            </div>
            <span v-if="item.latest_latency_ms == null && item.latest_avg_latency_ms == null" class="text-disabled">—</span>
          </div>
        </template>

        <template #item.score="{ item }">
          <div class="d-flex align-center ga-2">
            <v-progress-linear
              :model-value="Math.round(item.score * 100)"
              :color="scoreColor(item.score)"
              height="8"
              rounded
              class="flex-grow-1"
            />
            <span class="text-caption mono">{{ item.score.toFixed(3) }}</span>
          </div>
        </template>

        <template #item.alive="{ item }">
          <StatusTag :status="item.alive ? 'alive' : 'dead'" />
        </template>

        <template #item.actions="{ item }">
          <v-btn icon="mdi-eye-outline" size="x-small" variant="text" color="primary" title="详情" @click="openDetail(item)" />
          <v-btn icon="mdi-delete-outline" size="x-small" variant="text" color="error" title="删除" @click="handleDelete(item)" />
        </template>

        <template #no-data>
          <div class="py-8 text-center text-disabled">暂无数据</div>
        </template>
      </v-data-table>

      <v-divider />
      <div class="d-flex align-center justify-end pa-3 ga-3 flex-wrap">
        <span class="text-caption text-disabled">共 {{ total }} 条</span>
        <v-select
          v-model="pageSize"
          :items="pageOptions"
          density="compact"
          hide-details
          style="max-width: 110px"
          @update:model-value="page = 1; fetchNodes()"
        />
        <v-pagination
          v-model="page"
          :length="Math.max(1, Math.ceil(total / pageSize))"
          :total-visible="5"
          density="compact"
          @update:model-value="fetchNodes"
        />
      </div>
    </v-card>

    <NodeDetailDrawer
      v-if="selectedNodeId"
      v-model="drawerVisible"
      :node-id="selectedNodeId"
      @open-source="openSourceDrawer"
    />
    <SourceDetailDrawer
      v-if="selectedSourceId"
      v-model="sourceDrawerVisible"
      :source-id="selectedSourceId"
      @open-node="(id: string) => { selectedNodeId = id; drawerVisible = true }"
    />

    <v-snackbar :model-value="!!snake" :timeout="1800" location="bottom">
      {{ snake }}
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


.lh-tight {
  line-height: 1.5;
}
.nodes-table :deep(td) {
  font-size: 13px;
}
.nodes-table :deep(th) {
  white-space: nowrap;
}
.nodes-table :deep(th .v-data-table-header__content) {
  white-space: nowrap;
  word-break: keep-all;
}
</style>
