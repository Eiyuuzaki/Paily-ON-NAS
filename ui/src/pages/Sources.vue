<script setup lang="ts">
import { ref, reactive, onMounted, computed } from 'vue'
import StatusTag from '../components/StatusTag.vue'
import ExprEditor from '../components/ExprEditor.vue'
import SourceDetailDrawer from '../components/SourceDetailDrawer.vue'
import NodeDetailDrawer from '../components/NodeDetailDrawer.vue'
import { getSources, createSource, updateSource, deleteSource, searchSources, ignoreDeadSource } from '../api/sources'
import type { SourceSummary, SourceType, SourceStatus } from '../types/api'
import { formatBytes, formatUnixDate } from '../utils/format'
import { copyText } from '../utils/clipboard'

// ── list state ──────────────────────────────────────────────────────────
const tableData = ref<SourceSummary[]>([])
const loading = ref(false)
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const filtersOpen = ref(true)
const snake = ref<string | null>(null)
const pageOptions = [20, 50, 100]

const filters = reactive({
  type: '' as '' | SourceType,
  status: '' as '' | SourceStatus,
  identifier: '',
  info: '',
  sort_by: '' as '' | 'node_count',
  sort_dir: 'desc' as 'asc' | 'desc',
})

const advancedMode = ref(false)
const exprValue = ref('')

// ── drawers ─────────────────────────────────────────────────────────────
const drawerVisible = ref(false)
const selectedSourceId = ref<string | null>(null)
const nodeDrawerVisible = ref(false)
const selectedNodeId = ref<string | null>(null)

function openNodeDrawer(id: string) {
  selectedNodeId.value = id
  nodeDrawerVisible.value = true
}

function snack(text: string) {
  snake.value = text
  setTimeout(() => (snake.value = null), 1800)
}

// ── create dialog ───────────────────────────────────────────────────────
const createDialogVisible = ref(false)
const createLoading = ref(false)
const createForm = reactive({
  type: 'subscribe' as SourceType,
  content: '',
  identifier: '',
  info: '',
  priority: 0,
})
const nodeForm = reactive({
  formatType: 'mixed' as 'mixed' | 'clash' | 'singbox',
  nodeContent: '',
})

// ── edit dialog ─────────────────────────────────────────────────────────
const editDialogVisible = ref(false)
const editLoading = ref(false)
const editingId = ref('')
const editForm = reactive({
  identifier: '',
  info: '',
  content: '',
  type: '' as SourceType | '',
  priority: 0,
})

// ── fetch ───────────────────────────────────────────────────────────────
async function fetchSources() {
  loading.value = true
  try {
    if (advancedMode.value && exprValue.value.trim()) {
      const res = await searchSources({ expr: exprValue.value.trim(), page: page.value, limit: pageSize.value })
      tableData.value = res.data.data
      total.value = res.data.pagination.total
    } else {
      const params: Record<string, unknown> = { page: page.value, limit: pageSize.value }
      if (filters.type) params.type = filters.type
      if (filters.status) params.status = filters.status
      if (filters.identifier) params.identifier = filters.identifier
      if (filters.info) params.info = filters.info
      if (filters.sort_by) params.sort_by = filters.sort_by
      if (filters.sort_dir) params.sort_dir = filters.sort_dir
      const res = await getSources(params)
      tableData.value = res.data.data
      total.value = res.data.pagination.total
    }
  } finally {
    loading.value = false
  }
}

function handleFilter() {
  page.value = 1
  fetchSources()
}

function resetFilters() {
  Object.assign(filters, { type: '', status: '', identifier: '', info: '', sort_by: '', sort_dir: 'desc' })
  exprValue.value = ''
  page.value = 1
  fetchSources()
}

// ── 优先级快捷编辑 ──────────────────────────────────────────────────────
const priorityItems = [
  { title: '公益（0）', value: 0 },
  { title: '机场 / 付费（1）', value: 1 },
  { title: '最高（2）', value: 2 },
]

async function changePriority(row: SourceSummary, val: number) {
  const prev = row.priority ?? 0
  row.priority = val
  try {
    await updateSource(row.id, { priority: val })
    snack(`优先级已改为 ${val}`)
  } catch {
    row.priority = prev
    snack('修改失败')
  }
}

async function handleIgnoreDead(row: SourceSummary, val: boolean) {
  try {
    await ignoreDeadSource(row.id, val)
    row.ignore_dead = val
  } catch {
    row.ignore_dead = !val
  }
}

// ── delete ───────────────────────────────────────────────────────────────
async function handleDelete(row: SourceSummary) {
  if (!confirm(`确定删除来源 "${row.identifier || row.id}"？`)) return
  await deleteSource(row.id)
  snack('已删除')
  fetchSources()
}

async function copyId(id: string) {
  const ok = await copyText(id)
  snack(ok ? '已复制 ID' : '复制失败')
}

function formatContent(content: string | undefined): string {
  if (!content) return '—'
  const lines = content.split('\n').filter((l) => l.trim())
  if (lines.length <= 1) return lines[0] || '—'
  return lines[0] + ` 等${lines.length - 1}行`
}

function openDetail(row: SourceSummary) {
  selectedSourceId.value = row.id
  drawerVisible.value = true
}

// ── create ────────────────────────────────────────────────────────────────
function openCreate() {
  Object.assign(createForm, { type: 'subscribe', content: '', identifier: '', info: '', priority: 0 })
  Object.assign(nodeForm, { formatType: 'mixed', nodeContent: '' })
  createDialogVisible.value = true
}

async function submitCreate() {
  let content: string
  if (createForm.type === 'node') {
    if (!nodeForm.nodeContent.trim()) {
      snack('节点内容不能为空')
      return
    }
    content = JSON.stringify({ type: nodeForm.formatType, content: nodeForm.nodeContent })
  } else {
    if (!createForm.content.trim()) {
      snack('内容不能为空')
      return
    }
    content = createForm.content
  }
  createLoading.value = true
  try {
    await createSource({
      type: createForm.type,
      content,
      ...(createForm.identifier ? { identifier: createForm.identifier } : {}),
      ...(createForm.info ? { info: createForm.info } : {}),
    })
    snack('来源已创建')
    createDialogVisible.value = false
    fetchSources()
  } finally {
    createLoading.value = false
  }
}

// ── edit ──────────────────────────────────────────────────────────────────
function openEdit(row: SourceSummary) {
  editingId.value = row.id
  editForm.identifier = row.identifier
  editForm.info = row.info
  editForm.type = row.type
  editForm.content = row.content || ''
  editForm.priority = row.priority ?? 0
  editDialogVisible.value = true
}

async function submitEdit() {
  editLoading.value = true
  try {
    const payload: { identifier?: string; info?: string; content?: string; priority?: number } = {
      identifier: editForm.identifier,
      info: editForm.info,
      priority: editForm.priority,
    }
    if (editForm.type === 'subscribe') payload.content = editForm.content
    await updateSource(editingId.value, payload)
    snack('已保存')
    editDialogVisible.value = false
    fetchSources()
  } finally {
    editLoading.value = false
  }
}

// ── 表格列 ────────────────────────────────────────────────────────────────
const headers = [
  { title: 'ID', key: 'id', minWidth: 96, sortable: false },
  { title: '类型', key: 'type', minWidth: 76, sortable: false },
  { title: '标识符', key: 'identifier', minWidth: 160 },
  { title: '优先级', key: 'priority', minWidth: 116, sortable: false },
  { title: '额度', key: 'quota', minWidth: 190, sortable: false },
  { title: '内容', key: 'content', minWidth: 160, sortable: false },
  { title: '存活 / 节点数', key: 'node_count', minWidth: 110, sortable: false },
  { title: '状态', key: 'status', minWidth: 88, sortable: false },
  { title: '忽略死亡', key: 'ignore_dead', minWidth: 92, sortable: false },
  { title: '操作', key: 'actions', minWidth: 120, sortable: false, align: 'end' as const },
]

const priorityColor = (p: number) => (p >= 2 ? 'deep-purple' : p === 1 ? 'primary' : 'default')
const priorityLabel = (p: number) => (p >= 2 ? '最高' : p === 1 ? '机场' : '公益')
const priorityIcon = (p: number) => (p >= 2 ? 'mdi-star' : p === 1 ? 'mdi-airplane' : 'mdi-account-group')

/** 点击循环切换优先级 0 → 1 → 2 → 0 */
async function cyclePriority(row: SourceSummary) {
  const cur = row.priority ?? 0
  const next = cur >= 2 ? 0 : cur + 1
  await changePriority(row, next)
}

/** 源额度使用率（0-100），无额度时返回 null */
function quotaPct(row: SourceSummary): number | null {
  const t = row.total || 0
  if (t <= 0) return null
  const used = (row.upload || 0) + (row.download || 0)
  return Math.min(100, Math.round((used / t) * 100))
}

const hasAnyQuota = computed(() => tableData.value.some((s) => (s.total || 0) > 0))

onMounted(fetchSources)
</script>

<template>
  <div>
    <div class="page-head">
      <div>
        <h1 class="page-title">来源管理</h1>
        <p class="page-sub">共 {{ total }} 个来源 · 优先级决定订阅中机场节点占比</p>
      </div>
      <div class="d-flex ga-2">
        <v-btn
          :prepend-icon="filtersOpen ? 'mdi-filter-remove-outline' : 'mdi-filter-outline'"
          variant="tonal"
          @click="filtersOpen = !filtersOpen"
        >
          筛选
        </v-btn>
        <v-btn color="primary" prepend-icon="mdi-plus" @click="openCreate">新建来源</v-btn>
      </div>
    </div>

    <!-- 过滤器 -->
    <v-expand-transition>
      <v-card flat v-show="filtersOpen" rounded="xl" class="mb-4">
        <v-card-text>
          <v-row dense>
            <v-col cols="6" sm="4" md="2">
              <v-select
                v-model="filters.type"
                :items="[{ title: '全部', value: '' }, { title: '订阅', value: 'subscribe' }, { title: '节点', value: 'node' }]"
                label="类型"
                density="compact"
                hide-details
              />
            </v-col>
            <v-col cols="6" sm="4" md="2">
              <v-select
                v-model="filters.status"
                :items="[{ title: '全部', value: '' }, { title: '活跃', value: 'active' }, { title: '死亡', value: 'dead' }]"
                label="状态"
                density="compact"
                hide-details
              />
            </v-col>
            <v-col cols="6" sm="4" md="2">
              <v-text-field v-model="filters.identifier" label="标识符" density="compact" hide-details clearable />
            </v-col>
            <v-col cols="6" sm="4" md="2">
              <v-text-field v-model="filters.info" label="描述" density="compact" hide-details clearable />
            </v-col>
            <v-col cols="6" sm="4" md="2">
              <v-select
                v-model="filters.sort_by"
                :items="[{ title: '默认', value: '' }, { title: '节点数', value: 'node_count' }]"
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
            <v-col cols="12" md="4" class="d-flex align-center ga-2">
              <v-btn color="primary" prepend-icon="mdi-magnify" @click="handleFilter">查询</v-btn>
              <v-btn variant="text" @click="resetFilters">重置</v-btn>
              <v-btn
                variant="text"
                size="small"
                :prepend-icon="advancedMode ? 'mdi-chevron-up' : 'mdi-code-braces'"
                @click="advancedMode = !advancedMode"
              >
                expr
              </v-btn>
            </v-col>
          </v-row>
          <div v-if="advancedMode" class="mt-2">
            <ExprEditor v-model="exprValue" placeholder="输入 expr-lang 表达式筛选来源…" target="source" />
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
        hide-default-footer
        class="sources-table"
      >
        <template #item.id="{ item }">
          <v-tooltip :text="item.id" location="top">
            <template #activator="{ props }">
              <v-btn v-bind="props" variant="text" size="small" class="mono px-1" color="primary" @click="copyId(item.id)">
                {{ item.id.slice(0, 8) }}…
              </v-btn>
            </template>
          </v-tooltip>
        </template>

        <template #item.type="{ item }">
          <v-chip size="x-small" variant="tonal" :color="item.type === 'subscribe' ? 'info' : 'secondary'">
            {{ item.type === 'subscribe' ? '订阅' : '节点' }}
          </v-chip>
        </template>

        <template #item.priority="{ item }">
          <v-tooltip location="top">
            <template #activator="{ props }">
              <v-chip
                v-bind="props"
                :color="priorityColor(item.priority ?? 0)"
                :prepend-icon="priorityIcon(item.priority ?? 0)"
                size="small"
                variant="flat"
                class="priority-chip"
                @click="cyclePriority(item)"
              >
                {{ priorityLabel(item.priority ?? 0) }}
              </v-chip>
            </template>
            <div class="tooltip-body">
              <div>点击切换：公益 → 机场 → 最高</div>
              <div class="tooltip-note">仅支持带额度头的机场自动识别</div>
              <div class="tooltip-note">不带额度头的机场需导入后手动切换</div>
            </div>
          </v-tooltip>
        </template>

        <template #item.quota="{ item }">
          <div v-if="(item.total || 0) > 0" class="quota-cell">
            <div class="text-caption">
              <span class="font-weight-medium">{{ formatBytes((item.upload || 0) + (item.download || 0)) }}</span>
              <span class="text-disabled"> / {{ formatBytes(item.total) }}</span>
            </div>
            <v-progress-linear
              :model-value="quotaPct(item) ?? 0"
              :color="(quotaPct(item) ?? 0) >= 90 ? 'error' : (quotaPct(item) ?? 0) >= 70 ? 'warning' : 'success'"
              height="6"
              rounded
              class="my-1"
            />
            <div class="text-caption text-disabled">到期 {{ formatUnixDate(item.expire) }}</div>
          </div>
          <span v-else class="text-disabled text-caption">—</span>
        </template>

        <template #item.content="{ item }">
          <span v-if="item.type === 'node'" class="text-caption text-disabled">结构化节点</span>
          <span v-else class="text-caption mono content-cell">{{ formatContent(item.content) }}</span>
        </template>

        <template #item.node_count="{ item }">
          <!--
            「存活 / 总数」：total 是最近一次成功抓取到的节点数，
            alive 是其中当前仍存活的。两者都不可用时显示 —。
          -->
          <div v-if="item.type === 'subscribe' && item.latest_fetch_node_count != null" class="node-count">
            <span class="alive-num" :class="{ 'alive-num--zero': (item.alive_node_count ?? 0) === 0 }">
              {{ item.alive_node_count ?? 0 }}
            </span>
            <span class="text-disabled"> / {{ item.latest_fetch_node_count }}</span>
          </div>
          <span v-else class="text-disabled">—</span>
        </template>

        <template #item.status="{ item }">
          <StatusTag :status="item.status" />
        </template>

        <template #item.ignore_dead="{ item }">
          <v-switch
            :model-value="item.ignore_dead ?? false"
            density="compact"
            hide-details
            color="primary"
            @update:model-value="(v: any) => handleIgnoreDead(item, !!v)"
          />
        </template>

        <template #item.actions="{ item }">
          <v-btn icon="mdi-eye-outline" size="x-small" variant="text" color="primary" title="详情" @click="openDetail(item)" />
          <v-btn icon="mdi-pencil-outline" size="x-small" variant="text" color="info" title="编辑" @click="openEdit(item)" />
          <v-btn icon="mdi-delete-outline" size="x-small" variant="text" color="error" title="删除" @click="handleDelete(item)" />
        </template>

        <template #no-data>
          <div class="py-8 text-center text-disabled">暂无来源</div>
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
          @update:model-value="page = 1; fetchSources()"
        />
        <v-pagination
          v-model="page"
          :length="Math.max(1, Math.ceil(total / pageSize))"
          :total-visible="5"
          density="compact"
          @update:model-value="fetchSources"
        />
      </div>
    </v-card>

    <!-- 新建对话框 -->
    <v-dialog v-model="createDialogVisible" max-width="620" scrollable>
      <v-card flat rounded="xl" class="bg-surface-container-low">
        <v-card-title>新建来源</v-card-title>
        <v-card-text>
          <v-select
            v-model="createForm.type"
            :items="[{ title: '订阅（subscribe）', value: 'subscribe' }, { title: '节点（node）', value: 'node' }]"
            label="类型"
            class="mb-3"
          />

          <template v-if="createForm.type === 'subscribe'">
            <v-textarea
              v-model="createForm.content"
              label="内容（每行一个条目）"
              rows="6"
              class="mono"
              placeholder="每行一个订阅条目，支持 https:// / expr: / exprdate: / extract: 四种格式"
            />
            <v-alert type="info" variant="tonal" density="compact" class="text-caption mt-2">
              <div><b>纯链接</b>：https://… 订阅链接或节点 URI</div>
              <div><b>expr:</b> expr-lang 表达式，可用 date.year/month/day</div>
              <div><b>exprdate:</b> 用 {y}{m}{d} 作日期占位符</div>
              <div><b>extract:</b> 拉取页面并自动提取其中的链接 / 节点 URI</div>
            </v-alert>
          </template>

          <template v-else>
            <v-select
              v-model="nodeForm.formatType"
              :items="[
                { title: 'mixed（混合格式）', value: 'mixed' },
                { title: 'clash（Clash YAML）', value: 'clash' },
                { title: 'singbox（Sing-box JSON）', value: 'singbox' },
              ]"
              label="格式"
              class="mb-3"
            />
            <v-textarea v-model="nodeForm.nodeContent" label="节点内容" rows="6" class="mono" placeholder="粘贴节点配置内容…" />
          </template>

          <v-row dense class="mt-1">
            <v-col cols="12" sm="6">
              <v-text-field v-model="createForm.identifier" label="标识符（可选）" />
            </v-col>
            <v-col cols="12" sm="6">
              <v-text-field v-model="createForm.info" label="描述（可选）" />
            </v-col>
          </v-row>
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="createDialogVisible = false">取消</v-btn>
          <v-btn color="primary" :loading="createLoading" @click="submitCreate">创建</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- 编辑对话框 -->
    <v-dialog v-model="editDialogVisible" max-width="620" scrollable>
      <v-card flat rounded="xl" class="bg-surface-container-low">
        <v-card-title>编辑来源</v-card-title>
        <v-card-text>
          <v-row dense>
            <v-col cols="12" sm="6">
              <v-text-field v-model="editForm.identifier" label="标识符" />
            </v-col>
            <v-col cols="12" sm="6">
              <v-text-field v-model="editForm.info" label="描述" />
            </v-col>
            <v-col cols="12">
              <v-select
                v-model="editForm.priority"
                :items="priorityItems"
                label="优先级"
                hint="仅支持带额度头的机场自动识别；不带额度头的机场需手动切换"
                persistent-hint
              />
            </v-col>
          </v-row>

          <template v-if="editForm.type === 'subscribe'">
            <v-textarea v-model="editForm.content" label="内容（每行一个条目）" rows="6" class="mono mt-2" />
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

    <SourceDetailDrawer
      v-if="selectedSourceId"
      v-model="drawerVisible"
      :source-id="selectedSourceId"
      @open-node="openNodeDrawer"
    />
    <NodeDetailDrawer
      v-if="selectedNodeId"
      v-model="nodeDrawerVisible"
      :node-id="selectedNodeId"
      @open-source="(id: string) => { selectedSourceId = id; drawerVisible = true }"
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


.priority-chip {
  cursor: pointer;
  transition: filter 0.15s ease;
}
.priority-chip:hover {
  filter: brightness(1.08);
}
/* 存活数：加粗突出，0 存活用 error 色提示 */
.node-count {
  white-space: nowrap;
}
.alive-num {
  font-weight: 700;
  font-variant-numeric: tabular-nums;
  color: rgb(var(--v-theme-success));
}
.alive-num--zero {
  color: rgb(var(--v-theme-error));
}

/* 优先级 chip 的 tooltip 内容 */
.tooltip-body {
  font-size: 12px;
  line-height: 1.6;
}
.tooltip-note {
  opacity: 0.75;
}
.quota-cell {
  min-width: 150px;
}
.content-cell {
  font-size: 11px;
  opacity: 0.8;
}
.sources-table :deep(td) {
  font-size: 13px;
}
/* 表头不换行，避免中文竖排 */
.sources-table :deep(th .v-data-table-header__content) {
  white-space: nowrap;
  word-break: keep-all;
}
.sources-table :deep(th) {
  white-space: nowrap;
}
</style>
