<script setup lang="ts">
import { ref, computed } from 'vue'
import ExprEditor from '../components/ExprEditor.vue'
import StatusTag from '../components/StatusTag.vue'
import { previewFilter } from '../api/filter'
import type { NodeSummary, SourceSummary, FilterTarget } from '../types/api'

const activeTab = ref<FilterTarget>('node')
const snack = ref<string | null>(null)
const snackColor = ref('success')

function notify(text: string, color = 'success') {
  snack.value = text
  snackColor.value = color
  setTimeout(() => (snack.value = null), 2000)
}

// node tab
const nodeExpr = ref('')
const nodeResults = ref<NodeSummary[]>([])
const nodeLoading = ref(false)

// source tab
const sourceExpr = ref('')
const sourceResults = ref<SourceSummary[]>([])
const sourceLoading = ref(false)

async function previewNodes() {
  if (!nodeExpr.value.trim()) {
    notify('请输入表达式', 'warning')
    return
  }
  nodeLoading.value = true
  try {
    const res = await previewFilter({ expr: nodeExpr.value.trim(), target: 'node' })
    nodeResults.value = res.data.nodes ?? []
    notify(`匹配 ${nodeResults.value.length} 个节点`)
  } catch {
    nodeResults.value = []
  } finally {
    nodeLoading.value = false
  }
}

async function previewSources() {
  if (!sourceExpr.value.trim()) {
    notify('请输入表达式', 'warning')
    return
  }
  sourceLoading.value = true
  try {
    const res = await previewFilter({ expr: sourceExpr.value.trim(), target: 'source' })
    sourceResults.value = res.data.sources ?? []
    notify(`匹配 ${sourceResults.value.length} 个来源`)
  } catch {
    sourceResults.value = []
  } finally {
    sourceLoading.value = false
  }
}

const nodeHeaders = [
  { title: '协议', key: 'protocol', width: 90 },
  { title: '服务器', key: 'server', minWidth: 160 },
  { title: '区域', key: 'region', width: 70 },
  { title: '分数', key: 'score', width: 130, sortable: false },
  { title: '存活', key: 'alive', width: 80 },
]
const sourceHeaders = [
  { title: '类型', key: 'type', width: 90 },
  { title: '标识符', key: 'identifier', minWidth: 140 },
  { title: '描述', key: 'info', minWidth: 120 },
  { title: '状态', key: 'status', width: 90 },
]
</script>

<template>
  <div>
    <div class="page-head">
      <div>
        <h1 class="page-title">过滤测试台</h1>
        <p class="page-sub">用 expr-lang 实时预览过滤结果（最多返回 100 条）</p>
      </div>
    </div>

    <v-card flat rounded="xl" class="bg-surface-container-low">
      <v-tabs v-model="activeTab" color="primary" density="comfortable">
        <v-tab value="node" prepend-icon="mdi-server-network">节点</v-tab>
        <v-tab value="source" prepend-icon="mdi-link-variant">来源</v-tab>
      </v-tabs>

      <v-window v-model="activeTab">
        <!-- 节点 -->
        <v-window-item value="node">
          <v-card-text>
            <v-row>
              <v-col cols="12" md="5">
                <div class="text-subtitle-2 mb-2">expr-lang 表达式</div>
                <ExprEditor v-model="nodeExpr" placeholder="Alive == true && Score > 0.5" />
                <v-btn
                  color="primary"
                  class="mt-3"
                  prepend-icon="mdi-play"
                  :loading="nodeLoading"
                  @click="previewNodes"
                >
                  预览匹配节点
                </v-btn>
              </v-col>
              <v-col cols="12" md="7">
                <div class="d-flex align-center mb-2 ga-2">
                  <span class="text-subtitle-2">匹配结果</span>
                  <v-chip size="small" variant="tonal" color="primary">{{ nodeResults.length }}</v-chip>
                </div>
                <v-card variant="outlined" rounded="xl">
                  <v-data-table
                    :headers="nodeHeaders"
                    :items="nodeResults"
                    :loading="nodeLoading"
                    density="compact"
                    hide-default-footer
                    :items-per-page="-1"
                    max-height="500"
                  >
                    <template #item.score="{ item }">
                      <v-progress-linear :model-value="Math.round(item.score * 100)" height="8" rounded color="primary" />
                    </template>
                    <template #item.alive="{ item }">
                      <StatusTag :status="item.alive ? 'alive' : 'dead'" />
                    </template>
                    <template #no-data>
                      <div class="py-6 text-center text-disabled text-caption">暂无结果，输入表达式后点击预览</div>
                    </template>
                  </v-data-table>
                </v-card>
              </v-col>
            </v-row>
          </v-card-text>
        </v-window-item>

        <!-- 来源 -->
        <v-window-item value="source">
          <v-card-text>
            <v-row>
              <v-col cols="12" md="5">
                <div class="text-subtitle-2 mb-2">expr-lang 表达式</div>
                <ExprEditor v-model="sourceExpr" placeholder="Status == 'active'" target="source" />
                <v-btn
                  color="primary"
                  class="mt-3"
                  prepend-icon="mdi-play"
                  :loading="sourceLoading"
                  @click="previewSources"
                >
                  预览匹配来源
                </v-btn>
              </v-col>
              <v-col cols="12" md="7">
                <div class="d-flex align-center mb-2 ga-2">
                  <span class="text-subtitle-2">匹配结果</span>
                  <v-chip size="small" variant="tonal" color="primary">{{ sourceResults.length }}</v-chip>
                </div>
                <v-card variant="outlined" rounded="xl">
                  <v-data-table
                    :headers="sourceHeaders"
                    :items="sourceResults"
                    :loading="sourceLoading"
                    density="compact"
                    hide-default-footer
                    :items-per-page="-1"
                    max-height="500"
                  >
                    <template #item.status="{ item }">
                      <StatusTag :status="item.status" />
                    </template>
                    <template #no-data>
                      <div class="py-6 text-center text-disabled text-caption">暂无结果，输入表达式后点击预览</div>
                    </template>
                  </v-data-table>
                </v-card>
              </v-col>
            </v-row>
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
  margin-bottom: 20px;
}
.page-title {
  font-size: 24px;
  line-height: 32px;
  font-weight: 400;
  margin: 0;
  color: rgb(var(--v-theme-on-surface));
}
.page-sub {
  font-size: 14px;
  line-height: 20px;
  margin: 4px 0 0;
  color: rgb(var(--v-theme-on-surface-variant));
}
</style>
