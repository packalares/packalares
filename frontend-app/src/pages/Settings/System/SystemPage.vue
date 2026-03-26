<template>
  <div class="settings-page">
    <div class="page-title">System</div>
    <div class="page-scroll">
      <!-- System Information -->
      <div class="section-title">Information</div>
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
          <span class="info-value uptime-value">{{ formatUptime(sysInfo.uptime) }}</span>
        </div>
      </div>

      <!-- CPU -->
      <div class="section-title">Processor</div>
      <div class="settings-card">
        <div class="metric-row">
          <div class="metric-header">
            <span class="info-label">CPU Usage</span>
            <span class="metric-value" :class="usageColor(sysInfo.cpu_usage)">{{ sysInfo.cpu_usage.toFixed(1) }}%</span>
          </div>
          <q-linear-progress :value="sysInfo.cpu_usage / 100" :color="usageQColor(sysInfo.cpu_usage)" track-color="grey-9" rounded size="6px" class="q-mt-sm" />
        </div>
        <q-separator class="card-separator" />
        <div class="info-row">
          <span class="info-label">Load Average</span>
          <span class="info-value">{{ fmtLoad(sysInfo.load) }}</span>
        </div>
      </div>

      <!-- Memory -->
      <div class="section-title">Memory</div>
      <div class="settings-card">
        <div class="metric-row">
          <div class="metric-header">
            <span class="info-label">RAM Usage</span>
            <span class="metric-value" :class="usageColor(memPct)">
              {{ formatBytes(sysInfo.mem_used) }} / {{ formatBytes(sysInfo.mem_total) }}
            </span>
          </div>
          <q-linear-progress :value="memPct / 100" :color="usageQColor(memPct)" track-color="grey-9" rounded size="6px" class="q-mt-sm" />
          <div class="metric-sub">{{ memPct.toFixed(1) }}% used</div>
        </div>
      </div>

      <!-- Disk -->
      <div class="section-title">Storage</div>
      <div class="settings-card">
        <div class="metric-row">
          <div class="metric-header">
            <span class="info-label">Disk Usage</span>
            <span class="metric-value" :class="usageColor(diskPct)">
              {{ formatBytes(sysInfo.disk_used) }} / {{ formatBytes(sysInfo.disk_total) }}
            </span>
          </div>
          <q-linear-progress :value="diskPct / 100" :color="usageQColor(diskPct)" track-color="grey-9" rounded size="6px" class="q-mt-sm" />
          <div class="metric-sub">{{ diskPct.toFixed(1) }}% used</div>
        </div>
      </div>
    </div>
  </div>
</template>

<script lang="ts" setup>
import { ref, computed, onMounted, onUnmounted } from 'vue';
import { api } from 'boot/axios';
import { formatBytes, formatUptime, usageColor, usageQColor } from 'src/utils/helpers';

const sysInfo = ref({
  hostname: '--', os_version: '--', kernel: '--', arch: '--',
  cpu_model: '--', cpu_count: 0, cpu_usage: 0, uptime: 0,
  load: [0, 0, 0] as number[],
  mem_used: 0, mem_total: 0, disk_used: 0, disk_total: 0,
});

const memPct = computed(() => sysInfo.value.mem_total ? (sysInfo.value.mem_used / sysInfo.value.mem_total) * 100 : 0);
const diskPct = computed(() => sysInfo.value.disk_total ? (sysInfo.value.disk_used / sysInfo.value.disk_total) * 100 : 0);


function fmtLoad(l: number[]) { return l.map(v => v.toFixed(2)).join('  /  '); }

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
.uptime-value {
  background: var(--positive-soft);
  color: var(--positive);
  padding: 3px 10px;
  border-radius: var(--radius-xs);
  font-weight: 600;
  font-size: 12px;
}
</style>
