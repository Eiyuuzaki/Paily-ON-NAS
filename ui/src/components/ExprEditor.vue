<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  modelValue: string
  placeholder?: string
  target?: 'node' | 'node-name' | 'source'
}>()
const emit = defineEmits<{ (e: 'update:modelValue', v: string): void }>()

const nodeFields = [
  { name: 'Server', type: 'string', desc: '服务器地址' },
  { name: 'Protocol', type: 'string', desc: '协议类型（vmess/trojan/ss…）' },
  { name: 'Password', type: 'string', desc: '密码／密钥' },
  { name: 'Hash', type: 'string', desc: '节点唯一哈希' },
  { name: 'Region', type: 'string', desc: '区域代码（US/HK/JP…）' },
  { name: 'Alive', type: 'bool', desc: '是否存活' },
  { name: 'Score', type: 'float', desc: '评分 0~1' },
  { name: 'Latency', type: 'int', desc: '最新初筛延迟（ms，-1 不可达）' },
  { name: 'SourcePriority', type: 'int', desc: '来源优先级（0 公益 / 1 机场 / 2 最高）' },
  { name: 'SourceIDs', type: '[]string', desc: '来源 UUID 列表' },
  { name: 'Sources', type: '[]SourceInfo', desc: '来源详情（.ID/.Type/.Identifier/.Info/.Status/.Content/.Priority）' },
  { name: 'Streaming', type: 'map[string]bool', desc: '解锁标志，如 Streaming["netflix"]' },
  { name: 'RawConfig', type: 'map[string]any', desc: '解析后的 clash proxy 配置原始字段' },
]

const nameFields = [
  { name: 'node.region', type: 'string', desc: '区域代码（US/HK/JP…）' },
  { name: 'node.region_flag', type: 'string', desc: '区域旗帜 emoji' },
  { name: 'node.grade', type: 'string', desc: '等级（A/B/C/D）' },
  { name: 'node.score', type: 'float', desc: '评分 0~1' },
  { name: 'node.seq', type: 'string', desc: '序号字符串' },
  { name: 'node.server', type: 'string', desc: '服务器地址' },
  { name: 'node.protocol', type: 'string', desc: '协议类型' },
  { name: 'node.alive', type: 'bool', desc: '是否存活' },
  { name: 'node.source_priority', type: 'int', desc: '来源优先级（0 公益 / 1 机场 / 2 最高）' },
  { name: 'node.streaming', type: 'map[string]bool', desc: '解锁标志，如 node.streaming["netflix"]' },
  { name: 'node.sponsor', type: 'string', desc: '赞助商文本' },
  { name: 'unlock_flags', type: 'string', desc: '解锁标志汇总字符串' },
  { name: 'filter_mode', type: 'string', desc: '过滤模式（select / mark）' },
]

const sourceFields = [
  { name: 'ID', type: 'string', desc: '来源 UUID' },
  { name: 'Identifier', type: 'string', desc: '标识符' },
  { name: 'Info', type: 'string', desc: '描述信息' },
  { name: 'Status', type: 'string', desc: '状态（active / dead）' },
  { name: 'Type', type: 'string', desc: '来源类型（subscribe / node）' },
]

const currentFields = computed(() =>
  props.target === 'source' ? sourceFields : props.target === 'node-name' ? nameFields : nodeFields,
)

const headers = [
  { title: '字段', key: 'name', width: 200 },
  { title: '类型', key: 'type', width: 130 },
  { title: '说明', key: 'desc' },
]
</script>

<template>
  <div>
    <v-textarea
      :model-value="modelValue"
      :placeholder="placeholder ?? '输入 expr 表达式…'"
      rows="3"
      class="mono"
      variant="outlined"
      hide-details
      @update:model-value="(v: string) => emit('update:modelValue', v)"
    />

    <v-expansion-panels variant="accordion" class="mt-2">
      <v-expansion-panel title="expr-lang 可用字段参考" elevation="0">
        <v-expansion-panel-text>
          <v-data-table
            :headers="headers"
            :items="currentFields"
            density="compact"
            hide-default-footer
            :items-per-page="-1"
          >
            <template #item.name="{ item }">
              <code class="mono">{{ item.name }}</code>
            </template>
            <template #item.type="{ item }">
              <span class="text-caption text-disabled">{{ item.type }}</span>
            </template>
          </v-data-table>
          <div class="mt-2 text-caption text-disabled">
            语言教程：
            <a href="https://expr-lang.org/docs/language-definition" target="_blank" rel="noopener">
              expr-lang.org/docs/language-definition
            </a>
          </div>
        </v-expansion-panel-text>
      </v-expansion-panel>
    </v-expansion-panels>
  </div>
</template>

<style scoped>
.mono :deep(textarea) {
  font-family: ui-monospace, Consolas, monospace;
  font-size: 12px;
}
</style>
