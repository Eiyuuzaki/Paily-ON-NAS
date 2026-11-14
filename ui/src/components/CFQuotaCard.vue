<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { getCFSummary, getCFAccounts, getAccountUsage } from '../api/cf'

const accounts = ref<{ id: string; name: string }[]>([])
const usageMap = ref<Record<string, number>>({})
const alive = ref<number | null>(null)
const total = ref<number | null>(null)
const loaded = ref(false)

// 段/圆点配色映射到左卡同款类（airport=primary / free=tertiary）
const SEGS = ['split-airport', 'split-free']
const DOTS = ['dot-airport', 'dot-free']

const rows = computed(() => {
  const out: { name: string; used: number; pct: number; pctOfAccount: number; segClass: string; dotClass: string; unknown: boolean }[] = []
  let i = 0
  for (const a of accounts.value) {
    const u = usageMap.value[a.id]
    const unknown = u === undefined || u < 0
    const used = unknown ? 0 : u
    out.push({
      name: a.name,
      used,
      pct: 0,
      pctOfAccount: Math.min(100, Math.round(used / 100) / 10),
      segClass: SEGS[i % SEGS.length],
      dotClass: DOTS[i % DOTS.length],
      unknown,
    })
    i++
  }
  const tq = out.length * 100000
  for (const r of out) r.pct = Math.min(100, Math.round(r.used / tq * 1000) / 10)
  return out.sort((x, y) => y.used - x.used)
})

// 总配额 = 每号 100k × 号数；条的总宽代表总配额池，每段宽 = 该号已用/总配额
const totalQuota = computed(() => accounts.value.length * 100000)

const resetText = computed(() => {
  const now = new Date()
  const reset = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate() + 1))
  const mins = Math.round((reset.getTime() - now.getTime()) / 60000)
  return mins >= 60 ? `${Math.floor(mins / 60)} 小时 ${mins % 60} 分` : `${mins} 分钟`
})

async function fetchData() {
  const [sum, acc] = await Promise.all([getCFSummary(), getCFAccounts()])
  alive.value = sum.data.cf_alive ?? 0
  total.value = sum.data.cf_total ?? 0
  accounts.value = acc.data.data
  loaded.value = true
  for (const a of accounts.value) {
    let u: Awaited<ReturnType<typeof getAccountUsage>> | null = null
    for (let k = 0; k < 2; k++) {
      try { u = await getAccountUsage(a.id); break }
      catch (e) { console.warn('[CFQuotaCard] usage', a.name, 'attempt', k, e instanceof Error ? e.message : e) }
    }
    usageMap.value[a.id] = u ? ((u.data as Record<string, unknown>).total_today as number ?? -1) : -1
  }
}

onMounted(async () => {
  const delays = [1500, 4000, 8000]
  for (let attempt = 0; attempt < delays.length + 1; attempt++) {
    if (attempt > 0) await new Promise(r2 => setTimeout(r2, delays[attempt - 1]))
    try {
      await fetchData()
      break
    } catch (e) { console.warn('[CFQuotaCard] attempt', attempt, 'failed:', e instanceof Error ? e.message : e) }
  }
})
</script>

<template>
  <v-card rounded="xl" flat class="bg-surface-container-low">
    <div class="card-head">
      <v-icon icon="mdi-cloud-outline" size="20" class="mr-2" style="color: rgb(var(--v-theme-on-surface-variant))" />
      <span class="md3-title-medium">CF 号池配额</span>
      <v-spacer />
      <span class="md3-label-small text-disabled mr-2">{{ resetText }}后重置</span>
    </div>
    <div class="card-body">
      <div class="split-bar">
        <div v-for="r in rows" :key="r.name" class="split-seg" :class="r.segClass"
             :style="{ width: r.pct + '%' }" />
      </div>
      <!-- 配额轴标：0 → 单号完整配额 100k -->
      <div class="bar-axis md3-label-small">
        <span>0</span>
        <span>{{ totalQuota.toLocaleString() }}</span>
      </div>
      <div class="split-legend" v-if="rows.length">
        <div class="legend-item" v-for="r in rows" :key="r.name">
          <span class="dot" :class="r.dotClass" />
          <div>
            <div class="md3-label-medium legend-name">{{ r.name }} 号</div>
            <div class="md3-title-large legend-val">{{ r.unknown ? '—' : r.used.toLocaleString() }}</div>
            <div class="md3-body-small legend-pct">{{ r.pctOfAccount }}% · 剩余 {{ r.unknown ? '—' : (100000 - r.used).toLocaleString() }}</div>
          </div>
        </div>
      </div>
      <div v-else class="md3-body-small" style="color: rgb(var(--v-theme-on-surface-variant))">
        {{ loaded ? '暂无用量数据' : '加载中…' }}
      </div>
      <v-divider class="my-3" />
      <div class="md3-body-medium" style="color: rgb(var(--v-theme-on-surface))">
        存活节点 {{ alive ?? '—' }} / {{ total ?? '—' }} · 探测与实际流量共用配额
      </div>
    </div>
  </v-card>
</template>

<style scoped>
.card-head {
  display: flex;
  align-items: center;
  padding: 20px 20px 0;
  color: rgb(var(--v-theme-on-surface));
}
.card-body {
  padding: 16px 20px 20px;
}
.split-bar {
  display: flex;
  height: 12px;
  border-radius: 6px;
  overflow: hidden;
  background: rgb(var(--v-theme-surface-container-high));
}
.split-seg {
  transition: width 0.4s ease;
}
.split-airport {
  background: rgb(var(--v-theme-primary));
}
.split-free {
  background: rgb(var(--v-theme-tertiary));
}
.bar-axis {
  display: flex;
  justify-content: space-between;
  margin-top: 4px;
  color: rgb(var(--v-theme-on-surface-variant));
}
.split-legend {
  display: flex;
  gap: 32px;
  margin-top: 20px;
}
.legend-item {
  display: flex;
  gap: 10px;
  align-items: flex-start;
}
.dot {
  width: 12px;
  height: 12px;
  border-radius: 50%;
  margin-top: 4px;
  flex-shrink: 0;
}
.dot-airport {
  background: rgb(var(--v-theme-primary));
}
.dot-free {
  background: rgb(var(--v-theme-tertiary));
}
.legend-name {
  color: rgb(var(--v-theme-on-surface-variant));
}
.legend-val {
  font-weight: 500;
  color: rgb(var(--v-theme-on-surface));
  line-height: 1.2;
}
.legend-pct {
  color: rgb(var(--v-theme-on-surface-variant));
}
.total-row {
  display: flex;
  justify-content: space-between;
  color: rgb(var(--v-theme-on-surface));
}
.total-label {
  color: rgb(var(--v-theme-on-surface-variant));
}
.total-val {
  font-weight: 500;
}

</style>
