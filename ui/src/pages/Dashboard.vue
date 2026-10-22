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
  { key: 'clash', label: 'Clash', url: `${BASE}/clash`, desc: 'Clash / Mihomo YAML', icon: 'mdi-shield-half-full', color: 'primary' },
  { key: 'singbox', label: 'Sing-box', url: `${BASE}/singbox`, desc: 'Sing-box JSON', icon: 'mdi-cube-outline', color: 'info' },
  { key: 'base64', label: 'Base64', url: `${BASE}/base64`, desc: '通用 Base64 订阅', icon: 'mdi-code-braces', color: 'secondary' },
]

const overviewCards = computed(() => [
  { title: '节点总数', value: stats.value.totalNodes, icon: 'mdi-server-network', color: 'primary' },
  { title: '存活节点', value: stats.value.aliveNodes, icon: 'mdi-heart-pulse', color: 'success' },
  { title: '来源总数', value: stats.value.totalSources, icon: 'mdi-link-variant', color: 'info' },
  { title: '死亡来源', value: stats.value.deadSources, icon: 'mdi-link-variant-off', color: 'error' },
])

const aliveRatio = computed(() => {
  const t = stats.value.totalNodes || 0
  const a = stats.value.aliveNodes || 0
  return t > 0 ? Math.round((a / t) * 100) : 0
})

// 流量占比：机场（priority>=1）vs 公益（priority==0）
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
    /* 剪贴板不可用（非 HTTPS）时静默 */
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
    <div class="page-head">
      <div>
        <h1 class="page-title">仪表盘</h1>
        <p class="page-sub">节点池健康度与订阅分发概览</p>
      </div>
      <v-btn
        icon="mdi-refresh"
        variant="tonal"
        color="primary"
        :loading="loading"
        @click="fetchStats(); fetchTraffic()"
      />
    </div>

    <!-- 概览卡片 -->
    <v-row dense>
      <v-col v-for="c in overviewCards" :key="c.title" cols="12" sm="6" lg="3">
        <v-card class="stat-card" :color="c.color" variant="tonal" rounded="lg">
          <v-card-text class="d-flex align-center">
            <v-icon :icon="c.icon" size="40" class="mr-4 opacity-70" />
            <div>
              <div class="stat-title">{{ c.title }}</div>
              <div class="stat-value">
                {{ loading ? '—' : (c.value ?? '—') }}
              </div>
            </div>
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>

    <!-- 健康度 + 流量统计 -->
    <v-row dense class="mt-1">
      <v-col cols="12" md="5">
        <v-card rounded="lg" class="h-100">
          <v-card-title class="text-subtitle-1">
            <v-icon icon="mdi-chart-donut" class="mr-2" size="20" />
            节点存活率
          </v-card-title>
          <v-card-text>
            <div class="d-flex align-center justify-center py-3">
              <v-progress-circular
                :model-value="aliveRatio"
                :size="130"
                :width="14"
                :color="aliveRatio >= 50 ? 'success' : aliveRatio >= 20 ? 'warning' : 'error'"
              >
                <span class="text-h5 font-weight-bold">{{ aliveRatio }}%</span>
              </v-progress-circular>
            </div>
            <div class="text-center text-caption opacity-70">
              {{ stats.aliveNodes ?? 0 }} / {{ stats.totalNodes ?? 0 }} 个节点存活
            </div>
          </v-card-text>
        </v-card>
      </v-col>

      <v-col cols="12" md="7">
        <v-card rounded="lg" class="h-100">
          <v-card-title class="text-subtitle-1 d-flex align-center">
            <v-icon icon="mdi-swap-vertical-bold" class="mr-2" size="20" />
            实际流量统计
            <v-spacer />
            <v-chip size="small" variant="tonal" color="default">
              来自 NAS mihomo
            </v-chip>
          </v-card-title>

          <v-card-text>
            <v-skeleton-loader v-if="trafficLoading" type="list-item-two-line" />

            <template v-else-if="trafficSplit.hasData">
              <!-- 比例条 -->
              <div class="split-bar mb-3">
                <div
                  class="split-bar__seg split-bar__airport"
                  :style="{ width: trafficSplit.airportPct + '%' }"
                />
                <div
                  class="split-bar__seg split-bar__free"
                  :style="{ width: trafficSplit.freePct + '%' }"
                />
              </div>

              <v-row dense class="mb-2">
                <v-col cols="6">
                  <div class="legend">
                    <span class="legend-dot legend-dot--airport" />
                    机场（高优先级）
                    <div class="legend-val">{{ formatBytes(trafficSplit.airport) }}</div>
                    <div class="text-caption opacity-70">{{ trafficSplit.airportPct }}%</div>
                  </div>
                </v-col>
                <v-col cols="6">
                  <div class="legend">
                    <span class="legend-dot legend-dot--free" />
                    公益节点
                    <div class="legend-val">{{ formatBytes(trafficSplit.free) }}</div>
                    <div class="text-caption opacity-70">{{ trafficSplit.freePct }}%</div>
                  </div>
                </v-col>
              </v-row>

              <v-divider class="my-3" />
              <div class="d-flex justify-space-between text-body-2">
                <span class="opacity-70">合计</span>
                <span class="font-weight-medium">
                  上传 {{ formatBytes(traffic.total_upload) }} ·
                  下载 {{ formatBytes(traffic.total_download) }}
                </span>
              </div>

              <v-alert
                type="info"
                variant="tonal"
                density="compact"
                class="mt-3 text-caption"
              >
                仅统计经过 NAS mihomo 的流量。若手机等设备直连节点（订阅模式），这部分流量不计入。
              </v-alert>
            </template>

            <v-alert
              v-else
              type="warning"
              variant="tonal"
              density="compact"
              class="text-caption"
            >
              暂无流量数据。需要设备把 NAS mihomo（端口 7890）作为代理，流量才会被统计。
            </v-alert>
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>

    <!-- 订阅链接 -->
    <v-card rounded="lg" class="mt-4">
      <v-card-title class="text-subtitle-1">
        <v-icon icon="mdi-link-variant" class="mr-2" size="20" />
        订阅链接
      </v-card-title>
      <v-card-text>
        <v-row dense>
          <v-col v-for="sub in subscriptions" :key="sub.key" cols="12" md="4">
            <v-card variant="outlined" rounded="lg" class="sub-card h-100">
              <v-card-text>
                <div class="d-flex align-center mb-2">
                  <v-icon :icon="sub.icon" :color="sub.color" class="mr-2" />
                  <span class="font-weight-bold">{{ sub.label }}</span>
                  <v-chip size="x-small" variant="tonal" class="ml-2">{{ sub.desc }}</v-chip>
                </div>
                <div class="sub-url mono mb-3">{{ sub.url }}</div>
                <v-btn
                  block
                  variant="tonal"
                  :color="sub.color"
                  size="small"
                  :prepend-icon="copiedKey === sub.key ? 'mdi-check' : 'mdi-content-copy'"
                  @click="copyUrl(sub.key, sub.url)"
                >
                  {{ copiedKey === sub.key ? '已复制' : '复制链接' }}
                </v-btn>
              </v-card-text>
            </v-card>
          </v-col>
        </v-row>
      </v-card-text>
    </v-card>
  </div>
</template>

<style scoped>
.page-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  margin-bottom: 20px;
}
.page-title {
  font-size: 24px;
  font-weight: 700;
  margin: 0;
}
.page-sub {
  font-size: 13px;
  opacity: 0.6;
  margin: 2px 0 0;
}

.stat-card {
  transition: transform 0.15s ease;
}
.stat-card:hover {
  transform: translateY(-2px);
}
.stat-title {
  font-size: 13px;
  opacity: 0.75;
}
.stat-value {
  font-size: 30px;
  font-weight: 700;
  line-height: 1.1;
}

.split-bar {
  display: flex;
  height: 18px;
  border-radius: 9px;
  overflow: hidden;
  background: rgba(var(--v-border-color), 0.12);
}
.split-bar__seg {
  transition: width 0.4s ease;
}
.split-bar__airport {
  background: rgb(var(--v-theme-primary));
}
.split-bar__free {
  background: rgb(var(--v-theme-success));
}

.legend {
  font-size: 13px;
}
.legend-dot {
  display: inline-block;
  width: 10px;
  height: 10px;
  border-radius: 50%;
  margin-right: 6px;
}
.legend-dot--airport {
  background: rgb(var(--v-theme-primary));
}
.legend-dot--free {
  background: rgb(var(--v-theme-success));
}
.legend-val {
  font-size: 20px;
  font-weight: 700;
  margin-top: 4px;
}

.sub-url {
  font-size: 12px;
  opacity: 0.75;
  word-break: break-all;
  line-height: 1.5;
}
</style>
