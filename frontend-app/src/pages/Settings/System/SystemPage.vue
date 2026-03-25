<template>
  <div class="settings-page">
    <div class="page-title">System</div>
    <div class="page-scroll">
      <!-- System Information -->
      <div class="section-title">System Information</div>
      <div class="settings-card">
        <div class="info-row">
          <span class="info-label">Hostname</span>
          <span class="info-value">{{ sysInfo.hostname }}</span>
        </div>
        <q-separator class="card-separator" />
        <div class="info-row">
          <span class="info-label">Operating System</span>
          <span class="info-value">{{ sysInfo.os_version }}</span>
        </div>
        <q-separator class="card-separator" />
        <div class="info-row">
          <span class="info-label">Kernel</span>
          <span class="info-value">{{ sysInfo.kernel }}</span>
        </div>
        <q-separator class="card-separator" />
        <div class="info-row">
          <span class="info-label">Architecture</span>
          <span class="info-value">{{ sysInfo.arch }}</span>
        </div>
        <q-separator class="card-separator" />
        <div class="info-row">
          <span class="info-label">CPU</span>
          <span class="info-value">{{ sysInfo.cpu_model }} ({{ sysInfo.cpu_count }} cores)</span>
        </div>
        <q-separator class="card-separator" />
        <div class="info-row">
          <span class="info-label">Uptime</span>
          <span class="info-value">{{ formatUptime(sysInfo.uptime) }}</span>
        </div>
      </div>

      <!-- CPU -->
      <div class="section-title">CPU</div>
      <div class="settings-card">
        <div class="metric-row">
          <div class="metric-header">
            <span class="info-label">Usage</span>
            <span class="metric-value" :class="usageColor(sysInfo.cpu_usage)">{{ sysInfo.cpu_usage.toFixed(1) }}%</span>
          </div>
          <q-linear-progress :value="sysInfo.cpu_usage / 100" :color="usageQColor(sysInfo.cpu_usage)" track-color="grey-9" rounded size="8px" class="q-mt-sm" />
        </div>
        <q-separator class="card-separator" />
        <div class="info-row">
          <span class="info-label">Load Averages</span>
          <span class="info-value">{{ fmtLoad(sysInfo.load) }}</span>
        </div>
      </div>

      <!-- Memory -->
      <div class="section-title">Memory</div>
      <div class="settings-card">
        <div class="metric-row">
          <div class="metric-header">
            <span class="info-label">RAM</span>
            <span class="metric-value" :class="usageColor(memPct)">
              {{ formatBytes(sysInfo.mem_used) }} / {{ formatBytes(sysInfo.mem_total) }} ({{ memPct.toFixed(1) }}%)
            </span>
          </div>
          <q-linear-progress :value="memPct / 100" :color="usageQColor(memPct)" track-color="grey-9" rounded size="8px" class="q-mt-sm" />
        </div>
      </div>

      <!-- Disk -->
      <div class="section-title">Disk</div>
      <div class="settings-card">
        <div class="metric-row">
          <div class="metric-header">
            <span class="info-label">Storage</span>
            <span class="metric-value" :class="usageColor(diskPct)">
              {{ formatBytes(sysInfo.disk_used) }} / {{ formatBytes(sysInfo.disk_total) }} ({{ diskPct.toFixed(1) }}%)
            </span>
          </div>
          <q-linear-progress :value="diskPct / 100" :color="usageQColor(diskPct)" track-color="grey-9" rounded size="8px" class="q-mt-sm" />
        </div>
      </div>
    </div>
  </div>
</template>

<script lang="ts" setup>
import { ref, computed, onMounted, onUnmounted } from 'vue';
import { api } from 'boot/axios';

const sysInfo = ref({
  hostname: '--', os_version: '--', kernel: '--', arch: '--',
  cpu_model: '--', cpu_count: 0, cpu_usage: 0, uptime: 0,
  load: [0, 0, 0] as number[],
  mem_used: 0, mem_total: 0, disk_used: 0, disk_total: 0,
});

const memPct = computed(() => sysInfo.value.mem_total ? (sysInfo.value.mem_used / sysInfo.value.mem_total) * 100 : 0);
const diskPct = computed(() => sysInfo.value.disk_total ? (sysInfo.value.disk_used / sysInfo.value.disk_total) * 100 : 0);

function usageColor(pct: number) { return pct >= 80 ? 'text-red-5' : pct >= 50 ? 'text-amber-7' : 'text-green-5'; }
function usageQColor(pct: number) { return pct >= 80 ? 'red-6' : pct >= 50 ? 'amber-7' : 'green-6'; }

function formatBytes(b: number) {
  if (!b) return '0 B';
  const u = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(b) / Math.log(1024));
  return (b / Math.pow(1024, i)).toFixed(1) + ' ' + u[i];
}

function formatUptime(s: number) {
  if (!s) return '--';
  const d = Math.floor(s / 86400), h = Math.floor((s % 86400) / 3600), m = Math.floor((s % 3600) / 60);
  return d > 0 ? `${d}d ${h}h ${m}m` : h > 0 ? `${h}h ${m}m` : `${m}m`;
}

function fmtLoad(l: number[]) { return l.map(v => v.toFixed(2)).join(' / '); }

let timer: ReturnType<typeof setInterval> | null = null;

async function fetch() {
  try {
    const r: any = await api.get('/api/monitor/metrics');
    if (r) {
      sysInfo.value = {
        hostname: r.hostname || '--',
        os_version: r.os_version || '--',
        kernel: r.kernel || '--',
        arch: r.arch || '--',
        cpu_model: r.cpu_model || '--',
        cpu_count: r.cpu_count || r.cpu_cores?.length || 0,
        cpu_usage: r.cpu_usage || 0,
        uptime: r.uptime || 0,
        load: r.load || [0, 0, 0],
        mem_used: r.memory?.used || 0,
        mem_total: r.memory?.total || 0,
        disk_used: r.disk?.used || 0,
        disk_total: r.disk?.total || 0,
      };
    }
  } catch {}
}

onMounted(() => { fetch(); timer = setInterval(fetch, 10000); });
onUnmounted(() => { if (timer) clearInterval(timer); });
</script>

<style lang="scss" scoped>
.settings-page { height: 100%; display: flex; flex-direction: column; }
.page-title { font-size: 18px; font-weight: 600; color: var(--ink-1); padding: 16px 24px; height: 56px; display: flex; align-items: center; flex-shrink: 0; }
.page-scroll { flex: 1; overflow-y: auto; padding: 0 24px 24px; }
.section-title { font-size: 13px; font-weight: 500; color: var(--ink-2); margin-top: 20px; margin-bottom: 8px; text-transform: uppercase; letter-spacing: 0.5px; }
.settings-card { background: var(--bg-2); border-radius: 12px; border: 1px solid var(--separator); overflow: hidden; }
.info-row { display: flex; justify-content: space-between; align-items: center; padding: 14px 20px; }
.info-label { font-size: 14px; color: var(--ink-1); font-weight: 500; }
.info-value { font-size: 13px; color: var(--ink-2); font-family: 'JetBrains Mono', monospace; }
.metric-row { padding: 16px 20px; }
.metric-header { display: flex; justify-content: space-between; align-items: center; }
.metric-value { font-size: 13px; font-weight: 600; font-family: 'JetBrains Mono', monospace; }
.card-separator { background: var(--separator); margin: 0 20px; }
</style>
