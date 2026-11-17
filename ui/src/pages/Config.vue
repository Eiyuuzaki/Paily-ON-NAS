<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import ExprEditor from '../components/ExprEditor.vue'
import { getConfig, updateConfig } from '../api/config'
import type { ConfigMap } from '../types/api'

// Keys with friendly labels; all others rendered generically
const KEY_LABELS: Record<string, string> = {
  max_distribute: '最大分发节点数',
  dead_latency_ms: '初筛 dead 延迟阈值 (ms)',
  fetch_dead_window_days: '来源拉取 dead 判断窗口（天）',
  node_dead_window_days: '节点全 dead 判断窗口（天）',
  fetch_history_window_days: '抓取历史保留天数',
  initial_check_history_days: '初筛历史保留天数（已弃用）',
  deep_check_history_days: '复筛历史保留天数（已弃用）',
  score_decay_alpha: 'EWMA 衰减因子',
  score_weight_latency: '评分权重·延迟',
  score_weight_stability: '评分权重·稳定性',
  score_weight_speed: '评分权重·速度',
  grade_s_min: '（遗留）S 级最低分',
  grade_a_min: '（遗留）A 级最低分',
  grade_b_min: '（遗留）B 级最低分',
  grade_c_min: '（遗留）C 级最低分',
  grade_jitter_weight: '抖动权重（×抖动计入等效延迟）',
  grade_fail_cost_ms: '失败惩罚成本 (ms)',
  grade_s_ms: 'S 级上限·等效延迟 (ms)',
  grade_a_ms: 'A 级上限·等效延迟 (ms)',
  grade_b_ms: 'B 级上限·等效延迟 (ms)',
  grade_c_ms: 'C 级上限·等效延迟 (ms)',
  min_fetch_samples: 'fetch dead 触发最少记录数',
  min_check_samples: 'node dead 触发最少初筛数',
  cleanup_interval_minutes: '历史清理周期（分钟）',
  default_sponsor_text: '默认 Sponsor 文字',
  node_name_template: '节点命名模板（expr-lang）',
  fetch_interval_minutes: '抓取间隔（分钟）',
  filename_clash: 'Clash 配置文件名',
  filename_singbox: 'Sing-box 配置文件名',
  filename_base64: 'Base64 配置文件名',
  score_latency_curve_k: '延迟评分对数衰减曲线斜率 (k)',
  score_jitter_linear_threshold_ms: '抖动线性阈值 (ms)',
  node_distribute_filter: '分发过滤器（expr-lang，返回 true 则不分发）',
  node_alive_check_window_minutes: '存活联合评价窗口（分钟，0=单次提交即决定）',
  check_run_history_days: 'Checker 提交记录最大历史天数',
  priority_ratio: '机场节点占比（0~1，越高机场节点越多）',
  check_rep_per_server: '每入口每轮探测代表数（0=全量）',
  check_rep_min_group: '小源全测阈值（活节点数≤此值不抽代表）',
  ingress_max_new_per_source: '免费源每轮新节点上限（0=不限）',
  node_dead_retention_days: '死透归档保留天数（0=不归档）',
  cf_per_worker_max: '每地区·每个 CF Worker 最多入口数',
  region_other_max: '每地区·非 CF 节点上限',
  dist_allow_paid_selfsigned: '允许付费/最高来源使用自签证书',
  dist_allow_plaintext: '允许明文 HTTP/SOCKS 代理（危险）',
  dist_selfsigned_fallback: '自签兜底策略',
  cf_module: 'Cloudflare 号池模块（1=开 / 0=关）',
  ech_enabled: '启用 ECH 注入（1=开 / 0=关）',
  check_dead_grace_minutes: '判死后宽限期（分钟，期间保持全量探测）',
  ingress_max_per_endpoint: '入池·同一端点最大新节点数',
  ingress_zero_yield_quota: '入池·零产出协议配额',
  node_never_alive_retention_days: '从未存活节点保留天数（0=不归档）',
  profile_update_interval: '订阅 profile 更新间隔（小时）',
  distribute_per_egress_max: '同一出口最多分发数（旧参数，已由每 Worker 配额取代）',
}

const loading = ref(false)
const saving = ref(false)
const snack = ref<string | null>(null)
const snackColor = ref('success')
const serverKeys = ref<string[]>([])
const localConfig = reactive<ConfigMap>({})
const search = ref('')

const EXPR_KEYS = ['node_name_template', 'node_distribute_filter']
const DIST_KEYS = ['cf_per_worker_max', 'region_other_max', 'dist_allow_paid_selfsigned', 'dist_allow_plaintext', 'dist_selfsigned_fallback']
// Internal/technical keys that should not appear as editable fields.
const HIDE_KEYS = ['distribute_per_egress_max', 'cf_module', 'grade_s_min', 'grade_a_min', 'grade_b_min', 'grade_c_min']
const HIDE_PREFIX = ['cf_usage_']
const FALLBACK_OPTS = [
  { title: '不足即补（推荐）', value: 'short' },
  { title: '仅空白地区补', value: 'empty' },
  { title: '从不补（最严格）', value: 'none' },
]
const showPlainWarn = ref(false)
function setDistBool(k: string, v: boolean) {
  if (k === 'dist_allow_plaintext' && v) { showPlainWarn.value = true; return }
  localConfig[k] = v ? '1' : '0'
}
function confirmPlaintext() { localConfig['dist_allow_plaintext'] = '1'; showPlainWarn.value = false }

const DEFAULTS: Record<string, string> = {
  max_distribute: '200',
  priority_ratio: '0.6',
  dead_latency_ms: '3000',
  fetch_dead_window_days: '7',
  node_dead_window_days: '4',
  fetch_history_window_days: '7',
  initial_check_history_days: '3',
  deep_check_history_days: '7',
  score_decay_alpha: '0.7',
  score_weight_latency: '0.7',
  score_weight_stability: '0.3',
  score_weight_speed: '0.55',
  score_latency_curve_k: '0.3',
  score_jitter_linear_threshold_ms: '200',
  grade_s_min: '0.85',
  grade_a_min: '0.78',
  grade_b_min: '0.60',
  grade_c_min: '0.35',
  grade_jitter_weight: '2.0',
  grade_fail_cost_ms: '3000',
  grade_s_ms: '200',
  grade_a_ms: '400',
  grade_b_ms: '600',
  grade_c_ms: '1000',
  cleanup_interval_minutes: '30',
  default_sponsor_text: '',
  node_distribute_filter: '',
  node_name_template: `"[" + node.protocol + "] " + node.grade + " - " + node.region_flag + " " + node.region + " " + node.seq`,
  filename_clash: 'Paily Connect.yaml',
  filename_singbox: 'Paily Connect.json',
  filename_base64: 'Paily Connect.txt',
  node_alive_check_window_minutes: '240',
  check_run_history_days: '7',
  cf_per_worker_max: '4',
  region_other_max: '16',
  dist_allow_paid_selfsigned: '1',
  dist_allow_plaintext: '0',
  dist_selfsigned_fallback: 'short',
}

function showSnack(text: string, color = 'success') {
  snack.value = text
  snackColor.value = color
  setTimeout(() => (snack.value = null), 2000)
}

async function loadConfig() {
  loading.value = true
  try {
    const res = await getConfig()
    const data = res.data
    serverKeys.value = Object.keys(data)
    Object.assign(localConfig, data)
    for (const [k, v] of Object.entries(DEFAULTS)) {
      if (!serverKeys.value.includes(k)) {
        serverKeys.value.push(k)
        localConfig[k] = v
      }
    }
  } finally {
    loading.value = false
  }
}

function validate(): boolean {
  const fh = Number(localConfig['fetch_history_window_days'])
  const fd = Number(localConfig['fetch_dead_window_days'])
  const ih = Number(localConfig['check_run_history_days'])
  const nd = Number(localConfig['node_dead_window_days'])

  if (!isNaN(fh) && !isNaN(fd) && fh < fd) {
    showSnack('抓取历史保留天数必须 ≥ 来源死亡判定天数', 'error')
    return false
  }
  if (!isNaN(ih) && !isNaN(nd) && ih < nd) {
    showSnack('Checker 提交历史保留天数必须 ≥ 节点死亡判定天数', 'error')
    return false
  }
  return true
}

async function saveConfig() {
  if (!validate()) return
  saving.value = true
  try {
    // 后端 PUT /config 绑定 map[string]string：滑杆是 v-model.number，
    // 直接发送会得到 number，请求体在绑定阶段就会 400。统一转字符串。
    const payload: ConfigMap = {}
    for (const k of serverKeys.value) payload[k] = String(localConfig[k] ?? '')
    await updateConfig(payload)
    showSnack('配置已保存')
  } finally {
    saving.value = false
  }
}

const normalKeys = computed(() =>
  serverKeys.value.filter((k) => !EXPR_KEYS.includes(k) && !DIST_KEYS.includes(k) && !HIDE_KEYS.includes(k) && !HIDE_PREFIX.some((p) => k.startsWith(p)) && (!search.value || k.includes(search.value) ||
    (KEY_LABELS[k] ?? '').includes(search.value))),
)

// 把 priority_ratio 这类 0~1 的键渲染成滑杆
const RATIO_KEYS = ['priority_ratio', 'score_decay_alpha', 'score_weight_latency', 'score_weight_stability', 'score_weight_speed']

onMounted(loadConfig)
</script>

<template>
  <div>
    <div class="page-head">
      <div>
        <h1 class="page-title">运行时配置</h1>
        <p class="page-sub">修改后立即写入数据库，部分项需重启 core 生效</p>
      </div>
      <div class="d-flex ga-2">
        <v-btn variant="tonal" prepend-icon="mdi-refresh" :loading="loading" @click="loadConfig">重新加载</v-btn>
        <v-btn color="primary" prepend-icon="mdi-content-save" :loading="saving" @click="saveConfig">保存全部</v-btn>
      </div>
    </div>

    <v-card flat rounded="xl" class="mb-4">
      <v-card-text class="pb-0">
        <v-text-field
          v-model="search"
          prepend-inner-icon="mdi-magnify"
          label="搜索配置项"
          density="compact"
          hide-details
          clearable
          class="mb-3"
        />
      </v-card-text>
    </v-card>

    <v-sheet color="surface-container" rounded="lg" class="mb-4 pa-4">
      <div class="text-subtitle-1 mb-2">分发规则</div>
      <v-row dense>
        <v-col cols="12" sm="6">
          <v-text-field v-model="localConfig['cf_per_worker_max']" type="number" variant="filled" hide-details :label="KEY_LABELS['cf_per_worker_max']" />
        </v-col>
        <v-col cols="12" sm="6">
          <v-text-field v-model="localConfig['region_other_max']" type="number" variant="filled" hide-details :label="KEY_LABELS['region_other_max']" />
        </v-col>
      </v-row>
      <v-divider class="my-3" />
      <div class="d-flex align-center justify-space-between">
        <div class="text-body-2">{{ KEY_LABELS['dist_allow_paid_selfsigned'] }}</div>
        <v-switch :model-value="localConfig['dist_allow_paid_selfsigned']==='1'" @update:model-value="(v:any)=>setDistBool('dist_allow_paid_selfsigned', v)" color="primary" hide-details density="compact" class="flex-grow-0 ml-4" />
      </div>
      <v-divider class="my-3" />
      <div class="d-flex align-center justify-space-between">
        <div class="text-body-2">{{ KEY_LABELS['dist_selfsigned_fallback'] }}</div>
        <v-select :items="FALLBACK_OPTS" v-model="localConfig['dist_selfsigned_fallback']" variant="filled" density="compact" hide-details class="flex-grow-0" style="max-width: 200px" />
      </div>
      <v-divider class="my-3" />
      <div class="d-flex align-center justify-space-between">
        <div>
          <div class="text-body-2">{{ KEY_LABELS['dist_allow_plaintext'] }}</div>
          <div class="text-caption text-medium-emphasis">不加密，可被窃听/篡改，默认关闭</div>
        </div>
        <v-switch :model-value="localConfig['dist_allow_plaintext']==='1'" @update:model-value="(v:any)=>setDistBool('dist_allow_plaintext', v)" color="error" hide-details density="compact" class="flex-grow-0 ml-4" />
      </div>
    </v-sheet>

    <v-card flat rounded="xl" :loading="loading">
      <v-card-text>
        <v-row dense>
          <v-col v-for="key in normalKeys" :key="key" cols="12" md="6" lg="4">
            <v-text-field
              v-if="!RATIO_KEYS.includes(key)"
              v-model="localConfig[key]"
              :label="KEY_LABELS[key] ?? key"
              :hint="key"
              persistent-hint
              density="comfortable"
              class="mb-2"
            />
            <div v-else class="mb-2">
              <div class="text-caption mb-1">{{ KEY_LABELS[key] ?? key }}</div>
              <div class="d-flex align-center ga-3">
                <v-slider
                  v-model.number="localConfig[key]"
                  :min="0"
                  :max="1"
                  :step="0.05"
                  hide-details
                  color="primary"
                  class="flex-grow-1"
                />
                <v-text-field
                  v-model="localConfig[key]"
                  density="compact"
                  hide-details
                  style="max-width: 88px"
                />
              </div>
            </div>
          </v-col>
        </v-row>

        <v-divider class="my-4" />

        <div v-if="serverKeys.includes('node_name_template')" class="mb-4">
          <div class="text-subtitle-2 mb-2">节点命名模板（expr-lang）</div>
          <ExprEditor
            v-model="localConfig['node_name_template']"
            target="node-name"
            placeholder="例：'[' + node.protocol + '] ' + node.region_flag + node.region + ' ' + node.seq"
          />
        </div>

        <div v-if="serverKeys.includes('node_distribute_filter')">
          <div class="text-subtitle-2 mb-2">分发过滤器（expr-lang，返回 true 则不分发）</div>
          <ExprEditor
            v-model="localConfig['node_distribute_filter']"
            placeholder="例：Latency > 3000　返回 true 的节点将被排除"
          />
        </div>

        <div v-if="serverKeys.length === 0 && !loading" class="py-8 text-center text-disabled">
          暂无配置项
        </div>
      </v-card-text>
    </v-card>

    <v-dialog v-model="showPlainWarn" max-width="480">
      <v-card>
        <v-card-title>启用明文代理？</v-card-title>
        <v-card-text>
          明文 HTTP / SOCKS 代理不加密：链路上的任何人都能窃听、篡改你的流量，也能冒充节点做中间人。开启后这些节点会进入订阅。除非你完全清楚风险，否则请保持关闭。
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="showPlainWarn = false">取消</v-btn>
          <v-btn color="error" variant="flat" @click="confirmPlaintext">我已知晓，仍要启用</v-btn>
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
