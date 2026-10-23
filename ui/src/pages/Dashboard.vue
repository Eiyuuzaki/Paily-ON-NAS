<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { getStats } from '../api/stats'
import { getTrafficStats, type TrafficStatsResponse } from '../api/traffic'
import { formatBytes } from '../utils/format'

interface Stats {
  totalNodes: number | null
  aliveNodes: number | null
  totalSources: number | null
  deadSources: number | null
}

const stats = ref<Stats>({
  totalNodes: null,
  aliveNodes: null,
  totalSources: null,
  deadSources: null,
})
const traffic = ref<TrafficStatsResponse | null>(null)
const loading = ref(true)
const trafficLoading = ref(true)
const copiedKey = ref<string | null>(null)

const BASE = (import.meta.env.VITE_API_BASE as string) || ''
const subscriptions = [
  { key: 'clash', label: 'Clash', url: `${BASE}/clash`, desc: 'Clash / Mihomo YAML', icon: 'mdi-shield-half-full' },
  { key: 'singbox', label: 'Sing-box', url: `${BASE}/singbox`, desc: 'Sing-box JSON', icon: 'mdi-cube-outline' },
  { key: 'base64', label: 'Base64', url: `${BASE}/base64`, desc: '通用 Base64 订阅', icon: 'mdi-code-braces' },
]

// MD3：使用不同容器色阶区分，而不是四种高饱和糖果色
const overview = computed(() => [
  {
    title: '节点总数',
    value: stats.value.totalNodes,
    icon: 'mdi-server-network',
    container: 'primary-container',
    onContainer: 'on-primary-container',
  },
  {
    title: '存活节点',
    value: stats.value.aliveNodes,
    icon: 'mdi-heart-pulse',
    container: 'success-container',
    onContainer: 'success',
  },
  {
    title: '来源总数',
    value: stats.value.totalSources,
    icon: 'mdi-link-variant',
    container: 'tertiary-container',
    onContainer: 'on-tertiary-container',
  },
  {
    title: '死亡来源',
    value: stats.value.deadSources,
    icon: 'mdi-link-variant-off',
    container: 'surface-container-high',
    onContainer: 'on-surface-variant',
  },
])

const aliveRatio = computed(() => {
  const t = stats.value.totalNodes || 0
  const a = stats.value.aliveNodes || 0
  return t > 0 ? Math.round((a / t) * 100) : 0
})

const ratioColor = computed(() =>
  aliveRatio.value >= 50 ? 'success' : aliveRatio.value >= 20 ? 'warning' : 'error',
)

const trafficSplit = computed(() => {
  const list = traffic.value?.sources || []
  let airport = 0
  let free = 0
  for (const s of list) {
    const total = s.upload + s.download
    if (s.priority >= 1) airport += total
    else free += total
  }
  const sum = airport + free
  return {
    airport,
    free,
    airportPct: sum > 0 ? Math.round((airport / sum) * 100) : 0,
    freePct: sum > 0 ? Math.round((free / sum) * 100) : 0,
    hasData: sum > 0,
  }
})

async function copyUrl(key: string, url: string) {
  try {
    await navigator.clipboard.writeText(url)
    copiedKey.value = key
    setTimeout(() => (copiedKey.value = null), 1600)
  } catch {
    /* ignore */
  }
}

async function fetchStats() {
  loading.value = true
  try {
    const res = await getStats()
    stats.value.totalNodes = res.data.total_nodes
    stats.value.aliveNodes = res.data.alive_nodes
    stats.value.totalSources = res.data.total_sources
    stats.value.deadSources = res.data.dead_sources
  } finally {
    loading.value = false
  }
}

async function fetchTraffic() {
  trafficLoading.value = true
  try {
    const res = await getTrafficStats()
    traffic.value = res.data
  } catch {
    traffic.value = null
  } finally {
    trafficLoading.value = false
  }
}

onMounted(() => {
  fetchStats()
  fetchTraffic()
})
</script>

<template>
  <div>
    <!-- 页头 -->
    <div class="page-head">
      <div>
        <h1 class="md3-headline-small page-title">仪表盘</h1>
        <p class="md3-body-medium page-sub">节点池健康度与订阅分发概览</p>
      </div>
      <v-btn
        icon="mdi-refresh"
        variant="flat"
        class="bg-surface-container-high"
        :loading="loading"
        @click="fetchStats(); fetchTraffic()"
      />
    </div>

    <!-- 概览：MD3 容器色块 -->
    <div class="overview-grid">
      <v-card
        v-for="c in overview"
        :key="c.title"
        class="overview-card"
        :class="`bg-${c.container}`"
        flat
        rounded="xl"
      >
        <div class="overview-inner">
          <!-- 用内联 CSS 变量着色，避免 v-icon 的 color prop 走不到 on-* token -->
          <v-icon
            :icon="c.icon"
            size="28"
            class="overview-icon"
            :style="{ color: `rgb(var(--v-theme-${c.onContainer}))` }"
          />
          <div>
            <div
              class="overview-label md3-label-medium"
              :style="{ color: `rgb(var(--v-theme-${c.onContainer}))` }"
            >
              {{ c.title }}
            </div>
            <div
              class="overview-value md3-headline-large"
              :style="{ color: `rgb(var(--v-theme-${c.onContainer}))` }"
            >
              {{ loading ? '—' : (c.value ?? '—') }}
            </div>
          </div>
        </div>
      </v-card>
    </div>

    <!-- 健康度 + 流量 -->
    <div class="detail-grid">
      <v-card rounded="xl" flat class="bg-surface-container-low">
        <div class="card-head">
          <v-icon icon="mdi-chart-donut" size="20" class="mr-2" color="on-surface-variant" />
          <span class="md3-title-medium">节点存活率</span>
        </div>
        <div class="ring-wrap">
          <v-progress-circular
            :model-value="aliveRatio"
            :size="140"
            :width="16"
            :color="ratioColor"
            bg-color="surface-container-high"
          >
            <div class="ring-num md3-headline-medium">{{ aliveRatio }}%</div>
          </v-progress-circular>
          <div class="ring-caption md3-body-small">
            {{ stats.aliveNodes ?? 0 }} / {{ stats.totalNodes ?? 0 }} 个节点存活
          </div>
        </div>
      </v-card>

      <v-card rounded="xl" flat class="bg-surface-container-low">
        <div class="card-head">
          <v-icon icon="mdi-swap-vertical-bold" size="20" class="mr-2" color="on-surface-variant" />
          <span class="md3-title-medium">实际流量统计</span>
          <v-spacer />
          <v-chip size="small" variant="tonal" color="on-surface-variant" class="md3-label-small">
            来自 NAS mihomo
          </v-chip>
        </div>

        <div class="card-body">
          <template v-if="trafficLoading">
            <v-skeleton-loader type="list-item-two-line" />
          </template>

          <template v-else-if="trafficSplit.hasData">
            <!-- MD3 分段条 -->
            <div class="split-bar">
              <div class="split-seg split-airport" :style="{ width: trafficSplit.airportPct + '%' }" />
              <div class="split-seg split-free" :style="{ width: trafficSplit.freePct + '%' }" />
            </div>

            <div class="split-legend">
              <div class="legend-item">
                <span class="dot dot-airport" />
                <div>
                  <div class="md3-label-medium legend-name">机场节点</div>
                  <div class="md3-title-large legend-val">{{ formatBytes(trafficSplit.airport) }}</div>
                  <div class="md3-body-small legend-pct">{{ trafficSplit.airportPct }}%</div>
                </div>
              </div>
              <div class="legend-item">
                <span class="dot dot-free" />
                <div>
                  <div class="md3-label-medium legend-name">公益节点</div>
                  <div class="md3-title-large legend-val">{{ formatBytes(trafficSplit.free) }}</div>
                  <div class="md3-body-small legend-pct">{{ trafficSplit.freePct }}%</div>
                </div>
              </div>
            </div>

            <v-divider class="my-3" />
            <div class="total-row md3-body-medium">
              <span class="total-label">合计</span>
              <span class="total-val">
                上传 {{ formatBytes(traffic.total_upload) }} · 下载 {{ formatBytes(traffic.total_download) }}
              </span>
            </div>

            <div class="hint md3-body-small">
              仅统计经过 NAS mihomo 的流量。若手机等设备直连节点（订阅模式），这部分流量不计入。
            </div>
          </template>

          <div v-else-if="!trafficLoading" class="empty-state">
            <v-icon icon="mdi-chart-timeline-variant-shimmer" size="40" class="mb-2" color="on-surface-variant" />
            <div class="md3-body-medium empty-text">暂无流量数据</div>
            <div class="md3-body-small empty-sub">
              需要设备把 NAS mihomo（端口 7890）作为代理，流量才会被统计
            </div>
          </div>
        </div>
      </v-card>
    </div>

    <!-- 订阅链接 -->
    <v-card rounded="xl" flat class="bg-surface-container-low mt-4">
      <div class="card-head">
        <v-icon icon="mdi-link-variant" size="20" class="mr-2" color="on-surface-variant" />
        <span class="md3-title-medium">订阅链接</span>
      </div>
      <div class="card-body">
        <div class="sub-grid">
          <v-card
            v-for="sub in subscriptions"
            :key="sub.key"
            variant="flat"
            rounded="lg"
            class="sub-card bg-surface-container-lowest"
          >
            <div class="sub-inner">
              <div class="sub-top">
                <v-icon :icon="sub.icon" size="20" color="primary" />
                <span class="md3-title-small sub-name">{{ sub.label }}</span>
              </div>
              <div class="md3-body-small sub-desc">{{ sub.desc }}</div>
              <div class="mono sub-url">{{ sub.url }}</div>
              <v-btn
                block
                variant="flat"
                color="primary"
                size="small"
                :prepend-icon="copiedKey === sub.key ? 'mdi-check' : 'mdi-content-copy'"
                class="mt-3"
                @click="copyUrl(sub.key, sub.url)"
              >
                {{ copiedKey === sub.key ? '已复制' : '复制链接' }}
              </v-btn>
            </div>
          </v-card>
        </div>
      </div>
    </v-card>
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


/* ── 概览卡片 ── */
.overview-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 16px;
}
.overview-card {
  transition: transform 0.15s ease;
}
.overview-card:hover {
  transform: translateY(-2px);
}
.overview-inner {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 20px;
}
.overview-icon {
  opacity: 0.9;
  flex-shrink: 0;
}
.overview-label {
  margin-bottom: 2px;
}
.overview-value {
  font-weight: 500;
  line-height: 1.1;
}

/* ── 详情两列 ── */
.detail-grid {
  display: grid;
  grid-template-columns: 1fr 1.4fr;
  gap: 16px;
  margin-top: 16px;
}
@media (max-width: 960px) {
  .detail-grid {
    grid-template-columns: 1fr;
  }
}
.card-head {
  display: flex;
  align-items: center;
  padding: 20px 20px 0;
  color: rgb(var(--v-theme-on-surface));
}
.card-body {
  padding: 16px 20px 20px;
}

/* ── 存活率环 ── */
.ring-wrap {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 20px 20px 28px;
}
.ring-num {
  font-weight: 500;
  color: rgb(var(--v-theme-on-surface));
}
.ring-caption {
  margin-top: 14px;
  color: rgb(var(--v-theme-on-surface-variant));
}

/* ── 流量分段条 ── */
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
.hint {
  margin-top: 12px;
  padding: 10px 12px;
  border-radius: 8px;
  background: rgb(var(--v-theme-surface-container));
  color: rgb(var(--v-theme-on-surface-variant));
  line-height: 1.5;
}

/* ── 空状态 ── */
.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 28px 12px;
  text-align: center;
}
.empty-text {
  color: rgb(var(--v-theme-on-surface));
  font-weight: 500;
}
.empty-sub {
  margin-top: 4px;
  color: rgb(var(--v-theme-on-surface-variant));
}

/* ── 订阅卡 ── */
.sub-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
  gap: 12px;
}
.sub-card {
  border: 1px solid rgb(var(--v-theme-outline-variant));
}
.sub-inner {
  padding: 16px;
}
.sub-top {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 4px;
}
.sub-name {
  color: rgb(var(--v-theme-on-surface));
}
.sub-desc {
  color: rgb(var(--v-theme-on-surface-variant));
  margin-bottom: 8px;
}
.sub-url {
  color: rgb(var(--v-theme-primary));
  word-break: break-all;
  line-height: 1.5;
  font-size: 12px;
}
</style>
