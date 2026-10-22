<script setup lang="ts">
import { ref } from 'vue'
import NodeDetailDrawer from './NodeDetailDrawer.vue'
import SourceDetailDrawer from './SourceDetailDrawer.vue'

const type = ref<'node' | 'source'>('node')
const uuid = ref('')
const menuOpen = ref(false)

const nodeDrawerVisible = ref(false)
const sourceDrawerVisible = ref(false)
const activeNodeId = ref<string | null>(null)
const activeSourceId = ref<string | null>(null)

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i
const errorMsg = ref('')

function open() {
  const id = uuid.value.trim()
  if (!UUID_RE.test(id)) {
    errorMsg.value = '请输入有效的 UUID'
    return
  }
  errorMsg.value = ''
  if (type.value === 'node') {
    activeNodeId.value = id
    nodeDrawerVisible.value = true
  } else {
    activeSourceId.value = id
    sourceDrawerVisible.value = true
  }
  uuid.value = ''
  menuOpen.value = false
}

function openSourceFromNode(id: string) {
  activeSourceId.value = id
  sourceDrawerVisible.value = true
}

function openNodeFromSource(id: string) {
  activeNodeId.value = id
  nodeDrawerVisible.value = true
}
</script>

<template>
  <v-menu v-model="menuOpen" :close-on-content-click="false" location="bottom end" offset="8">
    <template #activator="{ props }">
      <v-btn
        v-bind="props"
        icon="mdi-magnify-plus-outline"
        variant="text"
        title="按 UUID 快速打开详情"
      />
    </template>

    <v-card min-width="360" rounded="lg">
      <v-card-title class="text-subtitle-2">快速打开</v-card-title>
      <v-card-text class="pt-2">
        <v-btn-toggle v-model="type" mandatory density="comfortable" color="primary" class="mb-3" rounded="lg">
          <v-btn value="node" size="small" prepend-icon="mdi-server-network">节点</v-btn>
          <v-btn value="source" size="small" prepend-icon="mdi-link-variant">来源</v-btn>
        </v-btn-toggle>

        <v-text-field
          v-model="uuid"
          placeholder="粘贴 UUID…"
          density="compact"
          hide-details="auto"
          :error-messages="errorMsg"
          class="mono"
          clearable
          @keyup.enter="open"
        />

        <v-btn block color="primary" class="mt-3" prepend-icon="mdi-open-in-new" @click="open">
          打开详情
        </v-btn>
      </v-card-text>
    </v-card>
  </v-menu>

  <NodeDetailDrawer
    v-if="activeNodeId"
    v-model="nodeDrawerVisible"
    :node-id="activeNodeId"
    @open-source="openSourceFromNode"
  />
  <SourceDetailDrawer
    v-if="activeSourceId"
    v-model="sourceDrawerVisible"
    :source-id="activeSourceId"
    @open-node="openNodeFromSource"
  />
</template>
