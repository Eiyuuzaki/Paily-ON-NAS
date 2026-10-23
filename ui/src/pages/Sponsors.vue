<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import ExprEditor from '../components/ExprEditor.vue'
import StatusTag from '../components/StatusTag.vue'
import { getSponsors, createSponsor, updateSponsor, deleteSponsor } from '../api/sponsors'
import { previewFilter } from '../api/filter'
import type { SponsorSummary, NodeSummary } from '../types/api'

// ── list ─────────────────────────────────────────────────────────────────
const tableData = ref<SponsorSummary[]>([])
const loading = ref(false)
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const pageOptions = [20, 50, 100]
const snack = ref<string | null>(null)
const snackColor = ref('success')

function notify(text: string, color = 'success') {
  snack.value = text
  snackColor.value = color
  setTimeout(() => (snack.value = null), 2000)
}

// ── form shared ───────────────────────────────────────────────────────────
const dialogVisible = ref(false)
const isEditing = ref(false)
const editingId = ref('')
const formLoading = ref(false)
const form = reactive({ name: '', text: '', filter_expr: '', priority: 0 })

// ── preview ───────────────────────────────────────────────────────────────
const previewVisible = ref(false)
const previewLoading = ref(false)
const previewNodes = ref<NodeSummary[]>([])

async function fetchSponsors() {
  loading.value = true
  try {
    const res = await getSponsors({ page: page.value, limit: pageSize.value })
    tableData.value = res.data.data
    total.value = res.data.pagination.total
  } finally {
    loading.value = false
  }
}

async function handleDelete(row: SponsorSummary) {
  if (!confirm(`确定删除赞助商 "${row.name}"？`)) return
  await deleteSponsor(row.id)
  notify('已删除')
  fetchSponsors()
}

function openCreate() {
  isEditing.value = false
  Object.assign(form, { name: '', text: '', filter_expr: '', priority: 0 })
  dialogVisible.value = true
}

function openEdit(row: SponsorSummary) {
  isEditing.value = true
  editingId.value = row.id
  Object.assign(form, { name: row.name, text: row.text, filter_expr: row.filter_expr, priority: row.priority })
  dialogVisible.value = true
}

async function submitForm() {
  if (!form.name.trim()) {
    notify('名称不能为空', 'warning')
    return
  }
  formLoading.value = true
  try {
    const payload = { name: form.name, text: form.text, filter_expr: form.filter_expr, priority: form.priority }
    if (isEditing.value) {
      await updateSponsor(editingId.value, payload)
      notify('已保存')
    } else {
      await createSponsor(payload)
      notify('已创建')
    }
    dialogVisible.value = false
    fetchSponsors()
  } finally {
    formLoading.value = false
  }
}

async function handlePreview() {
  if (!form.filter_expr.trim()) {
    notify('请先填写过滤表达式', 'warning')
    return
  }
  previewLoading.value = true
  previewVisible.value = true
  try {
    const res = await previewFilter({ expr: form.filter_expr, target: 'node' })
    previewNodes.value = res.data.nodes ?? []
  } catch {
    previewNodes.value = []
  } finally {
    previewLoading.value = false
  }
}

const headers = [
  { title: '名称', key: 'name', minWidth: 130 },
  { title: '文本', key: 'text', minWidth: 120 },
  { title: '过滤表达式', key: 'filter_expr', minWidth: 220, sortable: false },
  { title: '优先级', key: 'priority', minWidth: 100 },
  { title: '操作', key: 'actions', minWidth: 120, sortable: false, align: 'end' as const },
]

const previewHeaders = [
  { title: '协议', key: 'protocol', minWidth: 90 },
  { title: '服务器', key: 'server', minWidth: 160 },
  { title: '区域', key: 'region', minWidth: 70 },
  { title: '存活', key: 'alive', minWidth: 80 },
  { title: '分数', key: 'score', minWidth: 120, sortable: false },
]

onMounted(fetchSponsors)
</script>

<template>
  <div>
    <div class="page-head">
      <div>
        <h1 class="page-title">赞助商管理</h1>
        <p class="page-sub">按过滤表达式把赞助商文本附加到匹配的节点名称上</p>
      </div>
      <v-btn color="primary" prepend-icon="mdi-plus" @click="openCreate">新建赞助商</v-btn>
    </div>

    <v-card flat rounded="xl" class="bg-surface-container-low">
      <v-data-table :headers="headers" :items="tableData" :loading="loading" item-value="id" hide-default-footer>
        <template #item.filter_expr="{ item }">
          <code class="mono text-caption">{{ item.filter_expr.length > 60 ? item.filter_expr.slice(0, 60) + '…' : item.filter_expr }}</code>
        </template>
        <template #item.priority="{ item }">
          <v-chip size="small" variant="tonal" color="primary">{{ item.priority }}</v-chip>
        </template>
        <template #item.actions="{ item }">
          <v-btn icon="mdi-pencil-outline" size="x-small" variant="text" color="info" title="编辑" @click="openEdit(item)" />
          <v-btn icon="mdi-delete-outline" size="x-small" variant="text" color="error" title="删除" @click="handleDelete(item)" />
        </template>
        <template #no-data>
          <div class="py-8 text-center text-disabled">暂无赞助商</div>
        </template>
      </v-data-table>

      <v-divider />
      <div class="d-flex align-center justify-end pa-3 ga-3 flex-wrap">
        <span class="text-caption text-disabled">共 {{ total }} 条</span>
        <v-select v-model="pageSize" :items="pageOptions" density="compact" hide-details style="max-width: 110px"
          @update:model-value="page = 1; fetchSponsors()" />
        <v-pagination v-model="page" :length="Math.max(1, Math.ceil(total / pageSize))" :total-visible="5" density="compact"
          @update:model-value="fetchSponsors" />
      </div>
    </v-card>

    <!-- 新建 / 编辑 -->
    <v-dialog v-model="dialogVisible" max-width="640" scrollable>
      <v-card flat rounded="xl" class="bg-surface-container-low">
        <v-card-title>{{ isEditing ? '编辑赞助商' : '新建赞助商' }}</v-card-title>
        <v-card-text>
          <v-text-field v-model="form.name" label="名称" class="mb-2" />
          <v-text-field v-model="form.text" label="文本（附加到节点名）" class="mb-2" />
          <v-text-field v-model.number="form.priority" type="number" label="优先级" class="mb-2" />
          <div class="text-subtitle-2 mb-2">过滤表达式</div>
          <ExprEditor v-model="form.filter_expr" placeholder="输入 expr-lang 过滤表达式…" />
        </v-card-text>
        <v-card-actions>
          <v-btn variant="text" @click="handlePreview" prepend-icon="mdi-eye-outline">预览匹配节点</v-btn>
          <v-spacer />
          <v-btn variant="text" @click="dialogVisible = false">取消</v-btn>
          <v-btn color="primary" :loading="formLoading" @click="submitForm">
            {{ isEditing ? '保存' : '创建' }}
          </v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- 预览 -->
    <v-dialog v-model="previewVisible" max-width="720" scrollable>
      <v-card flat rounded="xl" class="bg-surface-container-low">
        <v-card-title class="d-flex align-center">
          预览匹配节点
          <v-chip size="small" variant="tonal" class="ml-2">{{ previewNodes.length }}</v-chip>
        </v-card-title>
        <v-card-text>
          <v-data-table :headers="previewHeaders" :items="previewNodes" :loading="previewLoading" density="compact"
            hide-default-footer :items-per-page="-1" max-height="420">
            <template #item.alive="{ item }">
              <StatusTag :status="item.alive ? 'alive' : 'dead'" />
            </template>
            <template #item.score="{ item }">
              <v-progress-linear :model-value="Math.round(item.score * 100)" height="8" rounded color="primary" />
            </template>
            <template #no-data>
              <div class="py-6 text-center text-disabled">无匹配节点</div>
            </template>
          </v-data-table>
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn color="primary" @click="previewVisible = false">关闭</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

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


</style>
