<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { getFetchLogs } from '../api/logs'
import { getSource, getSourceNodes, updateSource, deleteSource, ignoreDeadSource } from '../api/sources'
import StatusTag from './StatusTag.vue'
import type { FetchLogEntry, NodeSummary, SourceDetail, SourceUpdateRequest } from '../types/api'
import { formatBytes, formatUnixDate, formatDate } from '../utils/format'

const props = defineProps<{ sourceId: string; modelValue: boolean }>()
const emit = defineEmits<{
  (e: 'update:modelValue', v: boolean): void
  (e: 'open-node', id: string): void
  (e: 'deleted', id: string): void
}>()

const activeTab = ref('info')
const snack = ref<string | null>(null)
const snackColor = ref('success')

function notify(text: string, color = 'success') {
  snack.value = text
  snackColor.value = color
  setTimeout(() => (snack.value = null), 2000)
}

const detail = ref<SourceDetail | null>(null)
const loadingDetail = ref(false)
const fetchLogs = ref<FetchLogEntry[]>([])
const loadingLogs = ref(false)
const sourceNodes = ref<NodeSummary[]>([])
const nodesTotal = ref(0)
const loadingNodes = ref(false)

// edit
const editDialogVisible = ref(false)
const editLoading = ref(false)
const editForm = ref({ identifier: '', info: '', content: '', priority: 0 })

const priorityItems = [
  { title: '公益（0）', value: 0 },
  { title: '机场 / 付费（1）', value: 1 },
  { title: '最高（2）', value: 2 },
]

const priorityLabel = computed(() => {
  const p = detail.value?.priority ?? 0
  return p >= 2 ? '最高' : p === 1 ? '机场' : '公益'
})
const priorityColor = computed(() => {
  const p = detail.value?.priority ?? 0
  return p >= 2 ? 'deep-purple' : p === 1 ? 'primary' : 'default'
})

const quotaUsed = computed(() => (detail.value?.upload || 0) + (detail.value?.download || 0))
const quotaPct = computed(() => {
  const t = detail.value?.total || 0
  return t > 0 ? Math.min(100, Math.round((quotaUsed.value / t) * 100)) : 0
})
const hasQuota = computed(() => (detail.value?.total || 0) > 0)

async function loadDetail() {
  loadingDetail.value = true
  try {
    const res = await getSource(props.sourceId)
    detail.value = res.data
  } finally {
    loadingDetail.value = false
  }
}

async function loadFetchLogs() {
  loadingLogs.value = true
  try {
    const res = await getFetchLogs({ source_id: props.sourceId, limit: 20 })
    fetchLogs.value = res.data.data
  } finally {
    loadingLogs.value = false
  }
}

async function loadSourceNodes() {
  loadingNodes.value = true
  try {
    const res = await getSourceNodes(props.sourceId, { limit: 50 })
    sourceNodes.value = res.data.data
    nodesTotal.value = res.data.pagination.total
  } finally {
    loadingNodes.value = false
  }
}

watch(
  () => [props.modelValue, props.sourceId] as const,
  ([open]) => {
    if (open) {
      activeTab.value = 'info'
      detail.value = null
      fetchLogs.value = []
      sourceNodes.value = []
      nodesTotal.value = 0
      loadDetail()
    }
  },
  { immediate: true },
)

function onTabChange(tab: unknown) {
  if (tab === 'logs' && fetchLogs.value.length === 0) loadFetchLogs()
  if (tab === 'nodes' && sourceNodes.value.length === 0) loadSourceNodes()
}

function openEdit() {
  editForm.value = {
    identifier: detail.value?.identifier ?? '',
    info: detail.value?.info ?? '',
    content: detail.value?.content ?? '',
    priority: detail.value?.priority ?? 0,
  }
  editDialogVisible.value = true
}

async function submitEdit() {
  editLoading.value = true
  try {
    const payload: SourceUpdateRequest = {
      identifier: editForm.value.identifier,
      info: editForm.value.info,
      priority: editForm.value.priority,
    }
    if (detail.value?.type === 'subscribe') payload.content = editForm.value.content
    await updateSource(props.sourceId, payload)
    notify('已保存')
    editDialogVisible.value = false
    await loadDetail()
  } finally {
    editLoading.value = false
  }
}

async function handleDelete() {
  if (!confirm('确定删除该来源？此操作不可恢复。')) return
  await deleteSource(props.sourceId)
  notify('已删除')
  emit('update:modelValue', false)
  emit('deleted', props.sourceId)
}

async function handleIgnoreDead(val: boolean) {
  if (!detail.value) return
  await ignoreDeadSource(props.sourceId, val)
  detail.value.ignore_dead = val
}

async function changePriority(val: number) {
  if (!detail.value) return
  const prev = detail.value.priority ?? 0
  detail.value.priority = val
  try {
    await updateSource(props.sourceId, { priority: val })
    notify(`优先级改为 ${val}`)
  } catch {
    detail.value.priority = prev
  }
}

function close() {
  emit('update:modelValue', false)
}

const nodeHeaders = [
  { title: '协议', key: 'protocol', minWidth: 84 },
  { title: '服务器', key: 'server', minWidth: 150 },
  { title: '区域', key: 'region', minWidth: 70 },
  { title: '存活', key: 'alive', minWidth: 80 },
  { title: '分数', key: 'score', minWidth: 110, sortable: false },
  { title: '', key: 'actions', minWidth: 64, sortable: false, align: 'end' as const },
]
const logHeaders = [
  { title: 'ID', key: 'id', minWidth: 64 },
  { title: '结果', key: 'success', minWidth: 90 },
  { title: '节点数', key: 'node_count', minWidth: 90 },
  { title: '抓取时间', key: 'fetched_at', minWidth: 160 },
]
</script>

<template>
  <v-navigation-drawer
    :model-value="modelValue"
    location="right"
    temporary
    width="680"
    @update:model-value="(v: boolean) => emit('update:modelValue', v)"
  >
    <div class="drawer-head">
      <div>
        <div class="text-subtitle-1 font-weight-bold">来源详情</div>
        <div v-if="detail" class="text-caption text-disabled mono">{{ detail.id.slice(0, 8) }}…</div>
      </div>
      <v-spacer />
      <v-btn icon="mdi-pencil-outline" variant="text" size="small" title="编辑" @click="openEdit" />
      <v-btn icon="mdi-delete-outline" variant="text" size="small" color="error" title="删除" @click="handleDelete" />
      <v-btn icon="mdi-close" variant="text" size="small" @click="close" />
    </div>

    <v-divider />

    <v-tabs v-model="activeTab" color="primary" density="comfortable" @update:model-value="onTabChange">
      <v-tab value="info">基本信息</v-tab>
      <v-tab value="quota">额度</v-tab>
      <v-tab value="logs">抓取日志</v-tab>
      <v-tab value="nodes">节点列表</v-tab>
    </v-tabs>

    <v-window v-model="activeTab">
      <!-- 基本信息 -->
      <v-window-item value="info">
        <v-card-text :loading="loadingDetail" style="min-height: 200px">
          <template v-if="detail">
            <v-list density="compact" lines="two">
              <v-list-item title="类型" :subtitle="detail.type === 'subscribe' ? '订阅（subscribe）' : '节点（node）'" />
              <v-list-item title="状态">
                <template #subtitle><StatusTag :status="detail.status" /></template>
              </v-list-item>
              <v-list-item title="标识符" :subtitle="detail.identifier || '—'" />
              <v-list-item title="描述" :subtitle="detail.info || '—'" />
              <v-list-item title="优先级">
                <template #subtitle>
                  <v-chip :color="priorityColor" size="small" variant="flat" class="mt-1">{{ priorityLabel }}</v-chip>
                </template>
              </v-list-item>
              <v-list-item title="忽略死亡">
                <template #subtitle>
                  <v-switch
                    :model-value="detail.ignore_dead ?? false"
                    density="compact"
                    hide-details
                    color="primary"
                    class="mt-1"
                    @update:model-value="(v: any) => handleIgnoreDead(!!v)"
                  />
                </template>
              </v-list-item>
              <v-list-item title="创建时间" :subtitle="formatDate(detail.created_at)" />
              <v-list-item title="更新时间" :subtitle="formatDate(detail.updated_at)" />
            </v-list>

            <!-- 节点存活概览 -->
            <v-row v-if="detail.latest_fetch_node_count != null" dense class="mt-1">
              <v-col cols="6">
                <div class="text-caption text-disabled">最近抓取节点</div>
                <div class="text-h6">{{ detail.latest_fetch_node_count }}</div>
              </v-col>
              <v-col cols="6">
                <div class="text-caption text-disabled">当前存活</div>
                <div class="text-h6" :class="(detail.alive_node_count ?? 0) > 0 ? 'text-success' : 'text-error'">
                  {{ detail.alive_node_count ?? 0 }}
                </div>
              </v-col>
              <v-col cols="12">
                <v-progress-linear
                  :model-value="detail.latest_fetch_node_count > 0
                    ? Math.round(((detail.alive_node_count ?? 0) / detail.latest_fetch_node_count) * 100)
                    : 0"
                  :color="(detail.alive_node_count ?? 0) > 0 ? 'success' : 'error'"
                  height="8"
                  rounded
                />
              </v-col>
            </v-row>

            <div v-if="detail.content" class="mt-2">
              <div class="text-subtitle-2 mb-2">内容</div>
              <v-sheet rounded="lg" class="pa-3 mono content-box">
                {{ detail.content }}
              </v-sheet>
            </div>
          </template>
        </v-card-text>
      </v-window-item>

      <!-- 额度 -->
      <v-window-item value="quota">
        <v-card-text>
          <div v-if="hasQuota">
            <div class="d-flex align-center justify-space-between mb-2">
              <span class="text-subtitle-2">流量额度</span>
              <v-chip size="small" variant="tonal" :color="quotaPct >= 90 ? 'error' : quotaPct >= 70 ? 'warning' : 'success'">
                已用 {{ quotaPct }}%
              </v-chip>
            </div>
            <v-progress-linear
              :model-value="quotaPct"
              :color="quotaPct >= 90 ? 'error' : quotaPct >= 70 ? 'warning' : 'success'"
              height="14"
              rounded
              class="mb-3"
            />
            <v-row dense>
              <v-col cols="6">
                <div class="text-caption text-disabled">已用</div>
                <div class="text-h6">{{ formatBytes(quotaUsed) }}</div>
              </v-col>
              <v-col cols="6">
                <div class="text-caption text-disabled">总额</div>
                <div class="text-h6">{{ formatBytes(detail?.total) }}</div>
              </v-col>
              <v-col cols="6" class="mt-3">
                <div class="text-caption text-disabled">上传</div>
                <div>{{ formatBytes(detail?.upload) }}</div>
              </v-col>
              <v-col cols="6" class="mt-3">
                <div class="text-caption text-disabled">下载</div>
                <div>{{ formatBytes(detail?.download) }}</div>
              </v-col>
              <v-col cols="12" class="mt-3">
                <div class="text-caption text-disabled">到期</div>
                <div>{{ formatUnixDate(detail?.expire) }}</div>
              </v-col>
            </v-row>
            <v-alert type="info" variant="tonal" density="compact" class="text-caption mt-4">
              额度来自订阅响应的 subscription-userinfo 头，随每次抓取更新。
            </v-alert>
          </div>

          <div v-else>
            <v-alert type="warning" variant="tonal" density="compact" class="text-caption">
              该来源没有额度信息。只有机场订阅的响应头才会携带 subscription-userinfo。
            </v-alert>
            <div class="mt-4">
              <div class="text-subtitle-2 mb-2">手动设置优先级</div>
              <v-btn-toggle
                :model-value="detail?.priority ?? 0"
                mandatory
                color="primary"
                rounded="lg"
                @update:model-value="(v: any) => changePriority(Number(v))"
              >
                <v-btn v-for="p in priorityItems" :key="p.value" :value="p.value" size="small">{{ p.title }}</v-btn>
              </v-btn-toggle>
            </div>
          </div>
        </v-card-text>
      </v-window-item>

      <!-- 抓取日志 -->
      <v-window-item value="logs">
        <v-card-text>
          <v-data-table :headers="logHeaders" :items="fetchLogs" :loading="loadingLogs" density="compact" hide-default-footer :items-per-page="-1">
            <template #item.success="{ item }">
              <StatusTag :status="item.success ? '成功' : '失败'" />
            </template>
            <template #item.fetched_at="{ item }">{{ formatDate(item.fetched_at) }}</template>
            <template #no-data><div class="py-6 text-center text-disabled text-caption">暂无日志</div></template>
          </v-data-table>
        </v-card-text>
      </v-window-item>

      <!-- 节点列表 -->
      <v-window-item value="nodes">
        <v-card-text>
          <div class="text-caption text-disabled mb-2">
            共 {{ nodesTotal }} 个节点（最多展示 50 条），其中存活 {{ detail?.alive_node_count ?? 0 }} 个
          </div>
          <v-data-table :headers="nodeHeaders" :items="sourceNodes" :loading="loadingNodes" density="compact" hide-default-footer :items-per-page="-1">
            <template #item.alive="{ item }">
              <StatusTag :status="item.alive ? 'alive' : 'dead'" />
            </template>
            <template #item.score="{ item }">
              <v-progress-linear :model-value="Math.round(item.score * 100)" height="8" rounded color="primary" />
            </template>
            <template #item.actions="{ item }">
              <v-btn icon="mdi-arrow-top-right" size="x-small" variant="text" color="primary" @click="emit('open-node', item.id)" />
            </template>
            <template #no-data><div class="py-6 text-center text-disabled text-caption">暂无节点</div></template>
          </v-data-table>
        </v-card-text>
      </v-window-item>
    </v-window>

    <!-- 编辑对话框 -->
    <v-dialog v-model="editDialogVisible" max-width="600" scrollable>
      <v-card rounded="lg">
        <v-card-title>编辑来源</v-card-title>
        <v-card-text>
          <v-text-field v-model="editForm.identifier" label="标识符" class="mb-2" />
          <v-text-field v-model="editForm.info" label="描述" class="mb-2" />
          <v-select v-model="editForm.priority" :items="priorityItems" label="优先级" class="mb-2" />
          <template v-if="detail?.type === 'subscribe'">
            <v-textarea v-model="editForm.content" label="内容（每行一个条目）" rows="6" class="mono" />
            <v-alert type="info" variant="tonal" density="compact" class="text-caption mt-2">
              支持 https:// / expr: / exprdate: / extract: 四种格式
            </v-alert>
          </template>
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="editDialogVisible = false">取消</v-btn>
          <v-btn color="primary" :loading="editLoading" @click="submitEdit">保存</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <v-snackbar :model-value="!!snack" :color="snackColor" :timeout="2000" location="bottom">
      {{ snack }}
    </v-snackbar>
  </v-navigation-drawer>
</template>

<style scoped>
.drawer-head {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 12px 16px;
}
.content-box {
  max-height: 240px;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-all;
  font-size: 11px;
  background: rgba(var(--v-border-color), 0.08);
}
</style>
