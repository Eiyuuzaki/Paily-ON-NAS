<script setup lang="ts">
/**
 * 流量统计接入指引。
 *
 * 抽成独立组件是为了让两处渲染共用同一份文案：
 *   - 宽屏：TrafficGuideDialog 里展示
 *   - 窄屏：卡片内的 v-expansion-panel 里展示
 *
 * `configured` 区分两种空状态：后端没配 MIHOMO_API_URL（要改 .env 重启）
 * 和配了但没有流量经过（要把设备代理指过来）。两者的处理方式完全不同。
 */
defineProps<{
  configured: boolean
}>()
</script>

<template>
  <div class="guide">
    <template v-if="configured">
      <p class="guide-lead">
        traffic reporter 已在运行，但目前没有流量经过 NAS mihomo。
      </p>

      <h4 class="guide-h">方式一：设备把 NAS 当代理（推荐）</h4>
      <p>在 mihomo 配置里开启以下三项：</p>
      <pre class="guide-code">mixed-port: 7890
allow-lan: true
external-controller: 0.0.0.0:9090</pre>
      <p>
        然后手机的 Wi-Fi 高级设置 → 手动代理 → 填 <code>NAS-IP</code> + <code>7890</code>。
      </p>

      <h4 class="guide-h">方式二：旁路由模式</h4>
      <p>把 NAS mihomo 设为网关和 DNS，所有流量强制过网。</p>

      <div class="guide-note">
        订阅模式（设备直连节点）不经过 NAS，这部分流量无法统计。
      </div>
    </template>

    <template v-else>
      <p class="guide-lead">
        未配置 mihomo 接口，traffic reporter 没有启动。
      </p>

      <h4 class="guide-h">1. 编辑 NAS 上的 <code>/vol1/1000/paily/.env</code></h4>
      <pre class="guide-code">MIHOMO_API_URL=http://172.17.0.1:9090
MIHOMO_API_SECRET=你的mihomo密钥</pre>

      <h4 class="guide-h">2. 重启 core 让配置生效</h4>
      <pre class="guide-code">cd /vol1/1000/paily
docker compose up -d --no-deps core</pre>

      <h4 class="guide-h">3. 确认 mihomo 对得上</h4>
      <p>
        <code>external-controller</code> 监听在 <code>0.0.0.0:9090</code>，
        且 <code>secret</code> 与上面一致。
      </p>

      <div class="guide-note">
        <code>172.17.0.1</code> 是 Docker 网桥到宿主机的地址，适用于 mihomo 跑在
        NAS 本身的情况。secret 不一致会导致 reporter 一直 401，仪表盘永远为空。
      </div>
    </template>
  </div>
</template>

<style scoped>
.guide {
  font-size: 13px;
  line-height: 1.7;
  color: rgb(var(--v-theme-on-surface));
}
.guide-lead {
  margin: 0 0 16px;
  color: rgb(var(--v-theme-on-surface-variant));
}
.guide-h {
  margin: 18px 0 6px;
  font-size: 13px;
  font-weight: 600;
  color: rgb(var(--v-theme-on-surface));
}
.guide p {
  margin: 0 0 8px;
}
.guide code {
  padding: 1px 5px;
  border-radius: 4px;
  background: rgb(var(--v-theme-surface-container-high));
  font-family: ui-monospace, Consolas, monospace;
  font-size: 0.9em;
}
.guide-code {
  margin: 6px 0 10px;
  padding: 12px 14px;
  border-radius: 10px;
  background: rgb(var(--v-theme-surface-container-high));
  font-family: ui-monospace, Consolas, monospace;
  font-size: 12px;
  line-height: 1.65;
  white-space: pre;
  overflow-x: auto;
  text-align: left;
}
.guide-note {
  margin-top: 18px;
  padding-top: 14px;
  border-top: 1px solid rgba(var(--v-theme-outline-variant), 0.6);
  font-size: 12px;
  color: rgb(var(--v-theme-on-surface-variant));
}
</style>
