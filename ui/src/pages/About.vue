<script setup lang="ts">
import { computed } from 'vue'

const repoUrl = 'https://github.com/Eiyuuzaki/Paily-ON-NAS'
const upstreamUrl = 'https://github.com/openpaily'

const techStack = [
  { name: 'Go', version: '1.24+', role: '后端中枢 / 抓取 / 测活', icon: 'mdi-language-go' },
  { name: 'Vue', version: '3.5', role: '管理界面框架', icon: 'mdi-vuejs' },
  { name: 'Vuetify', version: '4.3', role: 'Material Design 3 组件', icon: 'mdi-material-design' },
  { name: 'PostgreSQL', version: '16', role: '数据存储', icon: 'mdi-database' },
  { name: 'mihomo', version: '—', role: '流量统计采集（可选）', icon: 'mdi-shield-half-full' },
]

const pipeline = [
  { step: '抓取', desc: '从公开订阅源拉取节点', icon: 'mdi-download-network' },
  { step: '去重', desc: '按协议指纹合并重复节点', icon: 'mdi-content-duplicate' },
  { step: '测活', desc: '并发初筛 + 复筛延迟与流媒体', icon: 'mdi-heart-pulse' },
  { step: '评分', desc: 'EWMA 综合延迟、稳定性、速度', icon: 'mdi-chart-line' },
  { step: '分发', desc: '输出 Clash / Sing-box / Base64', icon: 'mdi-share-variant' },
]

const features = [
  {
    icon: 'mdi-source-branch',
    title: '解除了上游的死依赖',
    desc: '上游依赖的若干服务已停止维护或归档，本项目内置了替代实现，可独立运行。',
  },
  {
    icon: 'mdi-lock-check',
    title: '机场额度与优先级',
    desc: '解析 subscription-userinfo 响应头读取机场额度；按优先级分配订阅名额，机场节点不被公益节点稀释。',
  },
  {
    icon: 'mdi-chart-donut',
    title: '存活可视与流量统计',
    desc: '每个来源展示「存活 / 抓取」数量；可选接入 mihomo，统计机场与公益节点的实际流量占比。',
  },
  {
    icon: 'mdi-code-braces',
    title: 'expr-lang 筛选引擎',
    desc: '用表达式实时预览过滤结果，支持按延迟、区域、评分、来源优先级等任意组合筛选。',
  },
]

const disclaimers = [
  {
    title: '不提供、不托管、不分发任何节点',
    body: '所有节点来自互联网上公开可访问的第三方订阅源。本项目仅提供聚合、去重、测活与格式转换的技术实现，不拥有、不控制、不背书任何节点。节点的合法性、安全性与可用性由原始提供方负责。',
  },
  {
    title: '严禁违反当地法律法规',
    body: '使用者应自行了解并遵守所在国家或地区关于网络访问、电信业务经营、计算机信息系统安全等方面的全部法律法规。因使用本项目产生的一切后果由使用者自行承担。',
  },
  {
    title: '严禁对外公开提供服务',
    body: '订阅接口默认无任何认证，任何能访问到端口的人都能获取全部订阅内容。请务必仅在本地内网或受控的私有网络中使用，不要做公网端口映射。',
  },
  {
    title: '按「现状」提供，不附带任何担保',
    body: '包括但不限于对适销性、特定用途适用性、无侵权、无缺陷、服务不中断的担保。使用本软件可能造成数据丢失、服务异常、设备负载升高、账号被封禁等风险。',
  },
  {
    title: '不是上游项目的官方分支',
    body: '本项目基于 openpaily 已归档的开源代码改造而来，与原作者无任何隶属关系，作者不对上游代码的质量问题负责。',
  },
]

const year = computed(() => new Date().getFullYear())
</script>

<template>
  <div>
    <div class="page-head">
      <div>
        <h1 class="page-title">关于</h1>
        <p class="page-sub">项目介绍、技术栈与免责声明</p>
      </div>
    </div>

    <!-- 项目简介 -->
    <v-card flat rounded="xl" class="bg-primary-container hero-card">
      <div class="hero">
        <div class="hero-logo">
          <v-icon icon="mdi-hexagon-multiple" size="30" style="color: rgb(var(--v-theme-on-primary))" />
        </div>
        <div class="hero-copy">
          <h2 class="hero-title">Paily ON NAS</h2>
          <p class="hero-slogan">把一台闲置的 NAS，变成你自己的节点聚合中枢。</p>
          <p class="hero-desc">
            在 NAS 上自建节点聚合与订阅分发平台：抓取公开订阅源 → 去重 → 测活 → 评分 →
            生成 Clash / Sing-box / Base64 订阅。面向 ARM NAS 优化，单机即可完整运行。
          </p>
          <div class="hero-meta">
            <v-chip size="small" variant="flat" class="meta-chip">
              <v-icon icon="mdi-scale-balance" size="14" class="mr-1" />
              AGPL-3.0
            </v-chip>
            <v-chip size="small" variant="flat" class="meta-chip">
              <v-icon icon="mdi-chip" size="14" class="mr-1" />
              linux / arm64
            </v-chip>
            <v-btn
              size="small"
              variant="flat"
              color="primary"
              prepend-icon="mdi-github"
              :href="repoUrl"
              target="_blank"
              rel="noopener"
            >
              查看源码
            </v-btn>
          </div>
        </div>
      </div>
    </v-card>

    <!-- 工作流程 -->
    <v-card flat rounded="xl" class="bg-surface-container-low mt-4">
      <div class="card-head">
        <v-icon icon="mdi-sitemap-outline" size="20" class="mr-2" style="color: rgb(var(--v-theme-on-surface-variant))" />
        <span class="md3-title-medium">工作流程</span>
      </div>
      <div class="card-body">
        <div class="pipeline">
          <template v-for="(p, i) in pipeline" :key="p.step">
            <div class="pipe-item">
              <div class="pipe-icon">
                <v-icon :icon="p.icon" size="20" />
              </div>
              <div class="pipe-step md3-label-large">{{ p.step }}</div>
              <div class="pipe-desc md3-body-small">{{ p.desc }}</div>
            </div>
            <v-icon
              v-if="i < pipeline.length - 1"
              icon="mdi-chevron-right"
              size="18"
              class="pipe-arrow"
            />
          </template>
        </div>
      </div>
    </v-card>

    <!-- 项目特色 -->
    <v-card flat rounded="xl" class="bg-surface-container-low mt-4">
      <div class="card-head">
        <v-icon icon="mdi-star-four-points-outline" size="20" class="mr-2" style="color: rgb(var(--v-theme-on-surface-variant))" />
        <span class="md3-title-medium">相对上游的改动</span>
      </div>
      <div class="card-body">
        <div class="feature-grid">
          <div v-for="f in features" :key="f.title" class="feature-item">
            <v-icon :icon="f.icon" size="22" color="primary" class="feature-icon" />
            <div>
              <div class="md3-title-small feature-title">{{ f.title }}</div>
              <div class="md3-body-small feature-desc">{{ f.desc }}</div>
            </div>
          </div>
        </div>
      </div>
    </v-card>

    <!-- 技术栈 -->
    <v-card flat rounded="xl" class="bg-surface-container-low mt-4">
      <div class="card-head">
        <v-icon icon="mdi-layers-outline" size="20" class="mr-2" style="color: rgb(var(--v-theme-on-surface-variant))" />
        <span class="md3-title-medium">技术栈</span>
      </div>
      <div class="card-body">
        <div class="stack-grid">
          <div v-for="t in techStack" :key="t.name" class="stack-item">
            <v-icon :icon="t.icon" size="20" class="stack-icon" />
            <div class="stack-copy">
              <div class="stack-name md3-label-large">
                {{ t.name }}
                <span class="stack-ver">{{ t.version }}</span>
              </div>
              <div class="md3-body-small stack-role">{{ t.role }}</div>
            </div>
          </div>
        </div>
      </div>
    </v-card>

    <!-- 免责声明 -->
    <v-card flat rounded="xl" class="bg-surface-container-low mt-4">
      <div class="card-head">
        <v-icon icon="mdi-alert-outline" size="20" class="mr-2" color="error" />
        <span class="md3-title-medium">免责声明</span>
      </div>
      <div class="card-body">
        <v-alert type="warning" variant="tonal" density="compact" class="mb-4 warn-banner">
          <div class="font-weight-medium">本项目仅供个人学习、技术研究与自用实验。</div>
          <div class="md3-body-small mt-1">严禁用于任何商业用途或对外提供服务。</div>
        </v-alert>

        <div class="disclaimer-list">
          <div v-for="(d, i) in disclaimers" :key="d.title" class="disclaimer-item">
            <div class="disclaimer-num">{{ i + 1 }}</div>
            <div>
              <div class="md3-title-small disclaimer-title">{{ d.title }}</div>
              <div class="md3-body-small disclaimer-body">{{ d.body }}</div>
            </div>
          </div>
        </div>

        <v-divider class="my-4" />

        <p class="md3-body-small closing">
          简言之：<b>这是技术学习项目，不是翻墙工具，更不是可以拿来运营的服务。</b>
          用它研究 Go 服务端架构、代理协议解析、并发测活调度，是可以的；拿它做生意或对外提供服务，不行。
        </p>
        <p class="md3-body-small closing muted">
          如果你不同意上述任何一条，请立即停止使用并删除本项目全部文件。
        </p>
      </div>
    </v-card>

    <!-- 页脚 -->
    <div class="about-footer md3-body-small">
      <span>Paily ON NAS · AGPL-3.0 · {{ year }}</span>
      <span class="dot-sep">·</span>
      <a :href="upstreamUrl" target="_blank" rel="noopener">基于 openpaily 改造</a>
    </div>
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

/* ── Hero ── */
.hero-card {
  overflow: hidden;
}
.hero {
  display: flex;
  gap: 22px;
  padding: 28px;
}
.hero-logo {
  width: 64px;
  height: 64px;
  border-radius: 18px;
  background: rgb(var(--v-theme-primary));
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}
.hero-copy {
  min-width: 0;
}
.hero-title {
  margin: 0;
  font-size: 26px;
  line-height: 34px;
  font-weight: 700;
  color: rgb(var(--v-theme-on-primary-container));
}
.hero-slogan {
  margin: 4px 0 0;
  font-size: 14px;
  font-weight: 500;
  color: rgb(var(--v-theme-on-primary-container));
  opacity: 0.9;
}
.hero-desc {
  margin: 10px 0 0;
  font-size: 13px;
  line-height: 1.7;
  color: rgb(var(--v-theme-on-primary-container));
  opacity: 0.8;
  max-width: 760px;
}
.hero-meta {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin-top: 16px;
}
.meta-chip {
  background: rgba(var(--v-theme-on-primary-container), 0.12) !important;
  color: rgb(var(--v-theme-on-primary-container)) !important;
  font-size: 12px;
}

/* ── 通用卡片头/体 ── */
.card-head {
  display: flex;
  align-items: center;
  padding: 20px 22px 0;
  color: rgb(var(--v-theme-on-surface));
}
.card-body {
  padding: 16px 22px 22px;
}

/* ── 流程 ── */
.pipeline {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}
.pipe-item {
  flex: 1 1 130px;
  min-width: 120px;
  padding: 14px 12px;
  border-radius: 14px;
  background: rgb(var(--v-theme-surface-container));
  text-align: center;
}
.pipe-icon {
  width: 36px;
  height: 36px;
  margin: 0 auto 8px;
  border-radius: 12px;
  background: rgb(var(--v-theme-primary-container));
  color: rgb(var(--v-theme-on-primary-container));
  display: flex;
  align-items: center;
  justify-content: center;
}
.pipe-step {
  color: rgb(var(--v-theme-on-surface));
  margin-bottom: 2px;
}
.pipe-desc {
  color: rgb(var(--v-theme-on-surface-variant));
  line-height: 1.5;
}
.pipe-arrow {
  color: rgb(var(--v-theme-outline));
  flex-shrink: 0;
}

/* ── 特色 ── */
.feature-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
  gap: 18px 24px;
}
.feature-item {
  display: flex;
  gap: 12px;
  align-items: flex-start;
}
.feature-icon {
  flex-shrink: 0;
  margin-top: 2px;
}
.feature-title {
  color: rgb(var(--v-theme-on-surface));
  margin-bottom: 3px;
}
.feature-desc {
  color: rgb(var(--v-theme-on-surface-variant));
  line-height: 1.6;
}

/* ── 技术栈 ── */
.stack-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(210px, 1fr));
  gap: 12px;
}
.stack-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px 14px;
  border-radius: 12px;
  background: rgb(var(--v-theme-surface-container));
}
.stack-icon {
  color: rgb(var(--v-theme-on-surface-variant));
  flex-shrink: 0;
}
.stack-copy {
  min-width: 0;
}
.stack-name {
  color: rgb(var(--v-theme-on-surface));
  line-height: 1.3;
}
.stack-ver {
  font-size: 11px;
  font-weight: 400;
  color: rgb(var(--v-theme-on-surface-variant));
  margin-left: 4px;
}
.stack-role {
  color: rgb(var(--v-theme-on-surface-variant));
}

/* ── 免责声明 ── */
.warn-banner {
  border-radius: 12px;
}
.disclaimer-list {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.disclaimer-item {
  display: flex;
  gap: 12px;
  align-items: flex-start;
}
.disclaimer-num {
  width: 22px;
  height: 22px;
  flex-shrink: 0;
  border-radius: 50%;
  background: rgb(var(--v-theme-surface-container-high));
  color: rgb(var(--v-theme-on-surface-variant));
  font-size: 12px;
  font-weight: 600;
  display: flex;
  align-items: center;
  justify-content: center;
  margin-top: 1px;
}
.disclaimer-title {
  color: rgb(var(--v-theme-on-surface));
  margin-bottom: 3px;
}
.disclaimer-body {
  color: rgb(var(--v-theme-on-surface-variant));
  line-height: 1.7;
}
.closing {
  margin: 0;
  color: rgb(var(--v-theme-on-surface));
  line-height: 1.8;
}
.closing.muted {
  margin-top: 8px;
  color: rgb(var(--v-theme-on-surface-variant));
}

/* ── 页脚 ── */
.about-footer {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 28px 0 8px;
  color: rgb(var(--v-theme-on-surface-variant));
  opacity: 0.7;
}
.about-footer a {
  color: inherit;
  text-decoration: none;
}
.about-footer a:hover {
  text-decoration: underline;
}
.dot-sep {
  opacity: 0.5;
}

/* 窄屏 */
@media (max-width: 700px) {
  .hero {
    flex-direction: column;
    gap: 16px;
    padding: 22px;
  }
  .pipe-arrow {
    display: none;
  }
}
</style>
