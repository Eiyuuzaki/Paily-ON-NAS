<script setup lang="ts">
import { ref, reactive, onMounted, computed } from 'vue'
import { getCFStatus, activateCF, deactivateCF, getCFAccounts, createCFAccount, updateCFAccount, deleteCFAccount, checkCFAccount, getCFSummary, setCFToken, getCFTokenStatus, getAccountUsage, type CFAccount, type CFSummary } from '../api/cf'
import { getExtAccess } from '../api/extaccess'

// ── activation ───────────────────────────────────────────────────────
const activated = ref<boolean | null>(null) // null = loading
const repoUrl = ref("")
const repoInput = ref("")
const activating = ref(false)
const snack = ref<string | null>(null)
const snackColor = ref("success")

function notify(text: string, color = "success") {
  snack.value = text
  snackColor.value = color
  setTimeout(() => (snack.value = null), 2500)
}

async function loadStatus() {
  const res = await getCFStatus()
  activated.value = res.data.activated
  repoUrl.value = res.data.repo_url || ""
}

async function activate() {
  if (!repoInput.value.trim()) return notify("请输入 GitHub 项目地址", "warning")
  activating.value = true
  try {
    const res = await activateCF(repoInput.value.trim())
    activated.value = true
    repoUrl.value = res.data.repo_url
    notify("CF 模块已激活")
    await loadAccounts()
  } catch (e: unknown) {
    notify("激活失败：" + errMsg(e), "error")
  } finally { activating.value = false }
}

async function deactivate() {
  await deactivateCF()
  activated.value = false
  notify("已停用 CF 模块")
}

function errMsg(e: unknown): string {
  // The API always reports failures as {"error": "..."} (see response.BadRequest
  // & friends). Reading "message" here yielded undefined for every error, so the
  // panel showed "AxiosError: Request failed with status code 400" and threw
  // away the reason the server actually gave.
  const anyE = e as { response?: { data?: { error?: string; message?: string } } }
  return anyE?.response?.data?.error || anyE?.response?.data?.message || String(e)
}

// ── accounts ─────────────────────────────────────────────────────────
const accounts = ref<CFAccount[]>([])
const loading = ref(false)
const dialogVisible = ref(false)
const isEditing = ref(false)
const editingId = ref("")
const form = reactive({ name: "", role: "backup", worker_url: "", worker_name: "", subdomain: "", uuid: "", note: "" })

const healthColor = (h: string) => (h === "ok" ? "success" : h === "degraded" ? "warning" : h === "down" ? "error" : "grey")
const healthText = (h: string) => ({ ok: "正常", degraded: "降级", down: "宕机", unknown: "未知" }[h] || h)
const roleText = (r: string) => ({ main: "主力", backup: "备用", asset: "资产" }[r] || r)

async function loadAccounts() {
  loading.value = true
  try {
    const res = await getCFAccounts()
    accounts.value = res.data.data
  } finally { loading.value = false }
}

// 账号的完整字段（uuid / worker_name / subdomain / note）只在 /cf/accounts 里，
// /cf/summary 的 accounts 是带统计的精简投影。表格必须用完整记录，
// 统计字段从 summary 按 id 合并进来，避免编辑对话框读到 undefined 后把原值清空。
async function reloadAll() {
  await Promise.all([loadAccounts(), loadSummary()])
}

function openCreate() {
  isEditing.value = false
  Object.assign(form, { name: "", role: "backup", worker_url: "", worker_name: "", subdomain: "", uuid: "", note: "" })
  dialogVisible.value = true
}

function openEdit(a: CFAccount) {
  isEditing.value = true
  editingId.value = a.id
  Object.assign(form, { name: a.name, role: a.role, worker_url: a.worker_url, worker_name: a.worker_name, subdomain: a.subdomain, uuid: a.uuid, note: a.note })
  dialogVisible.value = true
}

async function save() {
  if (!form.name.trim()) return notify("名称必填", "warning")
  try {
    if (isEditing.value) await updateCFAccount(editingId.value, { ...form })
    else await createCFAccount({ ...form })
    dialogVisible.value = false
    notify(isEditing.value ? "已更新" : "已添加")
    await reloadAll()
  } catch (e: unknown) { notify("保存失败：" + errMsg(e), "error") }
}

async function remove(a: CFAccount) {
  if (!confirm(`删除账号 ${a.name}？`)) return
  await deleteCFAccount(a.id)
  notify("已删除")
  await reloadAll()
}

const checking = ref<string | null>(null)
async function check(a: CFAccount) {
  checking.value = a.id
  try {
    const res = await checkCFAccount(a.id)
    a.health = res.data.health
    a.last_check_at = res.data.checked_at
    notify(`${a.name}: ${healthText(a.health)}`, a.health === "ok" ? "success" : "warning")
  } catch (e: unknown) { notify("探活失败：" + errMsg(e), "error") }
  finally { checking.value = null }
}

// ── summary ──────────────────────────────────────────────────────────
const summary = ref<CFSummary | null>(null)

async function loadSummary() {
  try {
    const res = await getCFSummary()
    summary.value = res.data
  } catch { /* keep old */ }
}

// 表格用完整账号记录，统计字段（node_alive / node_total）按 id 从 summary 合并。
// summary.accounts 是精简投影，不能直接当 CFAccount 用。
const mergedAccounts = computed(() => {
  const byId = new Map((summary.value?.accounts ?? []).map((a) => [a.id, a]))
  return accounts.value.map((a) => ({ ...a, ...(byId.get(a.id) ?? {}) }))
})

const quotaPct = computed(() => {
  if (!summary.value) return 0
  return Math.min(100, Math.round((summary.value.probes_today / summary.value.quota_daily) * 100))
})
const quotaColor = computed(() => (quotaPct.value >= 90 ? 'error' : quotaPct.value >= 70 ? 'warning' : 'success'))

// 真实用量：优先取主力号，否则第一个有数据的号
const realUsage = computed(() => {
  const main = accounts.value.find(a => a.role === 'main')
  if (main && usageMap.value[main.id] >= 0) return { id: main.id, name: main.name, total: usageMap.value[main.id] }
  for (const a of accounts.value) {
    if (usageMap.value[a.id] >= 0) return { id: a.id, name: a.name, total: usageMap.value[a.id] }
  }
  return null
})
const realUsagePct = computed(() => realUsage.value ? Math.min(100, Math.round(realUsage.value.total / 100000 * 1000) / 10) : 0)
const resetText = computed(() => {
  if (!summary.value) return ''
  const h = summary.value.quota_reset_hours
  return h >= 1 ? `${Math.floor(h)} 小时 ${Math.round((h % 1) * 60)} 分` : `${Math.round(h * 60)} 分钟`
})


// ── 订阅二维码 ──
const qrDialog = ref(false)
const qrDataUrl = ref('')
// The subscription endpoints are token-gated whenever one is configured, so a
// URL without it just returns 401. The other three places that print a
// subscription link (Dashboard, FormatConfigs, ExtAccessCard) append it; this
// one did not, which made the QR code a dead link.
const extToken = ref('')
const subUrl = computed(() => {
  const host = window.location.hostname
  const proto = window.location.protocol
  const q = extToken.value ? `?token=${encodeURIComponent(extToken.value)}` : ''
  return `${proto}//${host}:21380/clash${q}`
})

async function loadExtToken() {
  try {
    const res: any = await getExtAccess()
    extToken.value = (res.data?.data ?? res.data)?.token ?? ''
  } catch (e) {
    console.warn('[CFPanel] extaccess token', e)
  }
}

async function openQr() {
  qrDialog.value = true
  try {
    const QR = (await import('qrcode')).default
    qrDataUrl.value = await QR.toDataURL(subUrl.value, { width: 260, margin: 2 })
  } catch { qrDataUrl.value = '' }
}

// ── 每号用量 ──
const usageMap = ref<Record<string, number>>({})

async function loadUsages() {
  for (const a of accounts.value) {
    try {
      const res = await getAccountUsage(a.id)
      usageMap.value[a.id] = (res.data as any)?.total_today ?? 0
    } catch { usageMap.value[a.id] = -1 }
  }
}

// ── Analytics Token（按账号）──
const tokenDialog = ref(false)
const tokenAccount = ref<CFAccount | null>(null)
const tokenAccountName = computed(() => tokenAccount.value?.name ?? '')
const tokenInput = ref('')
const tokenSaving = ref(false)
const showToken = ref(false)

function openTokenDialog(acc: CFAccount) {
  tokenAccount.value = acc
  tokenInput.value = ''
  tokenDialog.value = true
}

async function saveToken() {
  if (!tokenAccount.value || !tokenInput.value.trim()) return
  tokenSaving.value = true
  try {
    await setCFToken(tokenAccount.value.id, tokenInput.value.trim())
    tokenInput.value = ''
    tokenDialog.value = false
    notify(tokenAccount.value.name + ' 号 Analytics Token 已保存', 'success')
    await loadAccounts()
  } catch (e: any) {
    notify(e?.message || 'Token 保存失败', 'error')
  } finally {
    tokenSaving.value = false
  }
}
const remainingQuota = computed(() => {
  if (!summary.value) return null
  return Math.max(0, summary.value.quota_daily - summary.value.probes_today)
})

const probePctOfQuota = computed(() => {
  if (!summary.value) return '0'
  return ((summary.value.probes_today / summary.value.quota_daily) * 100).toFixed(1)
})

async function checkAll() {
  for (const a of accounts.value) {
    await check(a)
  }
  await loadSummary()
}

onMounted(async () => {
  await loadExtToken()
  await loadStatus()
  if (activated.value) {
    await loadAccounts()
    await loadSummary()
    loadUsages()
  }
})
</script>



<template>
  <div>
    <!-- 页头：标题 | 状态标签 + 辅助链接 + 次要操作 -->
    <div class="page-head">
      <div>
        <h1 class="page-title">CF 模块</h1>
        <p class="page-sub">
          Cloudflare 号池与 Worker 运行观测
          <template v-if="activated === true">
            · <a :href="repoUrl" target="_blank" class="cf-link">{{ repoUrl }}</a>
          </template>
        </p>
      </div>
      <div class="d-flex ga-2 align-center flex-wrap">
        <v-tooltip text="订阅二维码" location="bottom">
          <template #activator="{ props }">
            <v-btn v-bind="props" size="small" variant="text" icon="mdi-qrcode" aria-label="订阅二维码" @click="openQr"></v-btn>
          </template>
        </v-tooltip>
        <v-chip v-if="activated === true" size="small" color="success" variant="tonal" prepend-icon="mdi-check-circle-outline">已启用</v-chip>
        <v-chip v-if="activated === true" size="small" :color="summary?.ech_enabled ? 'success' : 'warning'" variant="tonal">
          ECH {{ summary?.ech_enabled ? "已开启" : "已关闭" }}
        </v-chip>
        <v-chip v-else-if="activated === false" size="small" color="grey" variant="tonal">未激活</v-chip>
        <v-btn
          v-if="activated === true"
          variant="text"
          size="small"
          color="on-surface-variant"
          @click="deactivate"
        >停用模块</v-btn>
      </div>
    </div>

    <!-- 激活向导（未激活时唯一内容）-->
    <v-card v-if="activated === false" color="surface-container" elevation="0" class="pa-6">
      <div class="text-title-medium font-weight-medium mb-2">激活 CF 模块</div>
      <p class="text-body-2 text-medium-emphasis mb-4" style="max-width: 560px">
        输入托管 Worker 源码的 GitHub 项目地址以激活本模块。
        激活后可在此管理 CF 号池：账号、Worker 健康度与配额观测。凭据只保存在本机 NAS。
      </p>
      <v-text-field
        v-model="repoInput"
        label="GitHub 项目地址"
        placeholder="https://github.com/user/repo"
        variant="outlined"
        density="comfortable"
        hide-details="auto"
        style="max-width: 480px"
        class="mb-4"
      />
      <v-btn color="primary" :loading="activating" @click="activate">激活</v-btn>
    </v-card>

    <template v-if="activated === true">
      <!-- 统计指标行：3 个等高指标 + ECH 降为辅助 chip -->
      <v-card color="surface-container" rounded="lg" elevation="0" class="pa-5 mb-4">
        <div class="stats-row">
          <div class="stat-cell">
            <div class="text-caption text-medium-emphasis">存活</div>
            <div class="text-subtitle-1 font-weight-bold">{{ summary?.cf_alive ?? "—" }} <span class="text-body-2 text-medium-emphasis">/ {{ summary?.cf_total ?? "—" }}</span></div>
          </div>
          <v-divider vertical inset />
          <div class="stat-cell">
            <div class="text-caption text-medium-emphasis">探测</div>
            <div class="text-subtitle-1 font-weight-bold">{{ summary?.probes_today?.toLocaleString() ?? "—" }}</div>
          </div>
          <v-divider vertical inset />
          <div class="stat-cell">
            <div class="text-caption text-medium-emphasis">剩余</div>
            <div class="text-subtitle-1 font-weight-bold">{{ remainingQuota?.toLocaleString() ?? "—" }}</div>
            <div class="text-caption text-medium-emphasis">{{ resetText }}</div>
          </div>
        </div>
        <div v-if="(summary?.egress_regions ?? []).length" class="mt-4">
          <div class="text-caption text-medium-emphasis mb-1">出口分布</div>
          <div class="d-flex ga-3 flex-wrap">
            <span v-for="r in summary?.egress_regions" :key="r.region" class="text-body-2">
              <b>{{ r.region }}</b> × {{ r.cnt }}
            </span>
          </div>
        </div>
      </v-card><!-- 配额水位（inline，非卡片）--><!-- 配额水位（inline，非卡片）-->
      <div class="mb-6">
        <div class="d-flex align-center mb-2 quota-title-row">
          <span class="text-subtitle-2 font-weight-medium">配额</span>
          <v-spacer />
          <span class="text-body-2 text-medium-emphasis">
            <template v-if="realUsage">
              <b>{{ realUsage.name }}</b> {{ realUsage.total.toLocaleString() }} / 100k（{{ realUsagePct }}%）· {{ resetText }}后重置
            </template>
            <template v-else>
              {{ summary?.probes_today?.toLocaleString() ?? 0 }} / 100k（{{ probePctOfQuota }}%）
            </template>
          </span>
        </div>
        <v-progress-linear :model-value="realUsage ? realUsagePct : quotaPct" :color="realUsage ? (realUsagePct >= 90 ? 'error' : realUsagePct >= 70 ? 'warning' : 'success') : quotaColor" height="8" rounded />
      </div>

      <!-- 号池（核心操作区）-->
      <div class="mb-4">
        <div class="d-flex align-center ga-2 flex-wrap mb-2">
          <span class="text-subtitle-1 font-weight-medium">号池</span>
          <span class="text-caption text-medium-emphasis">2 个账号</span>
          <v-spacer />
          <v-tooltip text="依次检测所有账号的 Worker 可用性" location="bottom">
            <template #activator="{ props }">
              <v-btn v-bind="props" size="small" variant="tonal" prepend-icon="mdi-pulse" :loading="checking !== null" @click="checkAll">全部探活</v-btn>
            </template>
          </v-tooltip>
          <v-tooltip text="重新加载账号列表与统计" location="bottom">
            <template #activator="{ props }">
              <v-btn v-bind="props" size="small" variant="text" icon="mdi-refresh" :loading="loading" @click="loadAccounts(); loadSummary()" />
            </template>
          </v-tooltip>
          <v-btn color="primary" prepend-icon="mdi-plus" @click="openCreate">添加账号</v-btn>
        </div>

        <v-card elevation="0" variant="outlined" rounded="lg">
          <v-data-table
            :headers="[
              { title: '名称', key: 'name', minWidth: 80 },
              { title: '角色', key: 'role', minWidth: 80 },
              { title: 'Worker', key: 'worker_url', minWidth: 170 },
              { title: '节点（活/总）', key: 'nodes', minWidth: 95 },
              { title: '健康', key: 'health', minWidth: 90 },
              { title: '最近探活', key: 'last_check_at', minWidth: 130 },
              { title: '今日用量', key: 'usage', minWidth: 90 },
              { title: '操作', key: 'actions', sortable: false, minWidth: 130 },
            ]"
            :items="mergedAccounts"
            :loading="loading"
            hover
            class="cf-table"
            no-data-text="暂无账号 — 点击「添加账号」录入第一个 CF 号"
          >
            <template #item.role="{ item }">
              <v-chip size="small" :color="item.role === 'main' ? 'primary' : 'default'" variant="tonal">{{ roleText(item.role) }}</v-chip>
            </template>
            <template #item.worker_url="{ item }">
              <a v-if="item.worker_url" :href="item.worker_url" target="_blank" class="cf-link">{{ item.worker_url }}</a>
              <span v-else class="text-medium-emphasis">—</span>
            </template>
            <template #item.nodes="{ item }">
              <span class="text-body-2">{{ item.node_alive ?? 0 }} / {{ item.node_total ?? 0 }}</span>
            </template>
            <template #item.health="{ item }">
              <v-chip size="small" variant="tonal" :color="healthColor(item.health)" :prepend-icon="item.health === 'ok' ? 'mdi-check-circle-outline' : item.health === 'down' ? 'mdi-alert-circle-outline' : undefined">{{ healthText(item.health) }}</v-chip>
            </template>
            <template #item.usage="{ item }">
              <span class="text-body-2">{{ usageMap[item.id] !== undefined ? (usageMap[item.id] >= 0 ? usageMap[item.id].toLocaleString() : "—") : "…" }}</span>
            </template>
            <template #item.last_check_at="{ item }">
              <span class="text-body-2">{{ item.last_check_at ? new Date(item.last_check_at).toLocaleString() : "—" }}</span>
            </template>
            <template #item.actions="{ item }">
              <div class="d-flex ga-1">
                <v-tooltip text="探活：立即检测该 Worker 可用性" location="top">
                  <template #activator="{ props }">
                    <v-btn v-bind="props" icon="mdi-pulse" size="small" variant="text" color="primary" :loading="checking === item.id" @click="check(item)" />
                  </template>
                </v-tooltip>
                <v-tooltip text="编辑账号信息" location="top">
                  <template #activator="{ props }">
                    <v-btn v-bind="props" icon="mdi-pencil" size="small" variant="text" @click="openEdit(item)" />
                  </template>
                </v-tooltip>
                <v-tooltip text="配置 Analytics Token（每号独立）" location="top">
                  <template #activator="{ props }">
                    <v-btn v-bind="props" icon="mdi-key-outline" size="small" variant="text" color="primary" @click="openTokenDialog(item)" />
                  </template>
                </v-tooltip>
                <v-tooltip text="删除账号（节点保留，仅移除登记）" location="top">
                  <template #activator="{ props }">
                    <v-btn v-bind="props" icon="mdi-delete-outline" size="small" variant="text" color="error" @click="remove(item)" />
                  </template>
                </v-tooltip>
              </div>
            </template>
          </v-data-table>
        </v-card>
      </div>

      <!-- 运维参考：v-expansion-panel 原生折叠（键盘/触屏/悬停全支持）-->
                  <v-expansion-panels variant="accordion" rounded="lg" class="ops-panels">
        <v-expansion-panel title="运维参考" text="429+1027 = 配额满（UTC 0 恢复）· 忽通忽断 = 线路抖动 · 换号 = 改 NS（见 CF-域名故障切换SOP.md）" />
      </v-expansion-panels>
    </template>

    <!-- 编辑对话框 -->
    <v-dialog v-model="dialogVisible" max-width="560">
      <v-card>
        <v-card-title>{{ isEditing ? "编辑账号" : "添加账号" }}</v-card-title>
        <v-card-text>
          <v-text-field v-model="form.name" label="名称（如 C / D）" variant="outlined" class="mb-2" />
          <v-select v-model="form.role" :items="[{title: '主力', value: 'main'}, {title: '备用', value: 'backup'}, {title: '资产', value: 'asset'}]" label="角色" variant="outlined" class="mb-2" />
          <v-text-field v-model="form.worker_url" label="Worker URL" placeholder="https://xxx.workers.dev" variant="outlined" class="mb-2" />
          <v-text-field v-model="form.worker_name" label="Worker 名称" variant="outlined" class="mb-2" />
          <v-text-field v-model="form.subdomain" label="workers.dev 子域" variant="outlined" class="mb-2" />
          <v-text-field v-model="form.uuid" label="VLESS UUID" variant="outlined" class="mb-2" />
          <v-text-field v-model="form.note" label="备注" variant="outlined" />
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="dialogVisible = false">取消</v-btn>
          <v-btn color="primary" @click="save">保存</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
    <!-- Analytics Token 输入对话框 -->
    <v-dialog v-model="tokenDialog" max-width="480">
      <v-card>
        <v-card-title>为 {{ tokenAccountName }} 配置 Analytics Token</v-card-title>
        <v-card-text>
          <p class="text-body-2 text-medium-emphasis mb-3">
            请登录 {{ tokenAccountName }} 号对应的 Cloudflare 账户，在
            My Profile → API Tokens 创建；权限只需
            Account → Workers Observability → Read，并限定到该账户。
            Token 只保存在本机 NAS（chmod 600），绝不外传。
          </p>
          <v-text-field
            v-model="tokenInput"
            label="Analytics API Token"
            variant="outlined"
            :type="showToken ? 'text' : 'password'"
            :append-inner-icon="showToken ? 'mdi-eye-off' : 'mdi-eye'"
            @click:append-inner="showToken = !showToken"
            hide-details="auto"
          />
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="tokenDialog = false">取消</v-btn>
          <v-btn color="primary" :loading="tokenSaving" @click="saveToken">保存</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

      <!-- 订阅二维码 -->
      <v-dialog v-model="qrDialog" max-width="340">
        <v-card class="pa-5 text-center">
          <div class="text-subtitle-1 font-weight-medium mb-3">Clash 订阅</div>
          <img v-if="qrDataUrl" :src="qrDataUrl" alt="QR" style="width: 260px; height: 260px" />
          <div class="text-caption text-medium-emphasis mt-3" style="word-break: break-all">{{ subUrl }}</div>
        </v-card>
      </v-dialog>

    <v-snackbar :model-value="snack !== null" :color="snackColor" timeout="2500" @update:model-value="(v: boolean) => { if (!v) snack = null }">{{ snack }}</v-snackbar>
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
  margin: 4px 0 0;
  color: rgb(var(--v-theme-on-surface-variant));
}
.cf-link {
  color: rgb(var(--v-theme-primary));
  text-decoration: none;
}
.cf-link:hover {
  text-decoration: underline;
}
.stats-row {
  display: flex;
  align-items: stretch;
  gap: 0;
  flex-wrap: wrap;
}
@media (max-width: 750px) {
  .stats-row { flex-direction: column; gap: 16px; }
  .stats-row .v-divider { display: none; }
  .stat-cell { padding: 0 !important; min-width: 0 !important; }
  .quota-title-row { flex-direction: column; align-items: flex-start !important; gap: 4px; }
}
.stat-cell {
  padding: 0 24px;
  min-width: 160px;
}
.stat-cell:first-child { padding-left: 0; }
.cf-table :deep(th) {
  font-weight: 500;
  color: rgb(var(--v-theme-on-surface-variant));
}
.cf-table :deep(td) {
  border-bottom: 1px solid rgb(var(--v-theme-outline-variant)) !important;
}
.cf-table :deep(tr:hover td) {
  background: rgb(var(--v-theme-surface-container-low));
}
</style>