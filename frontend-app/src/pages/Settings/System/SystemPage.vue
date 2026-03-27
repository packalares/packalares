<template>
  <div class="settings-page">
    <div class="page-header">
      <div class="page-title">System</div>
      <div class="page-description">Hardware metrics, resource utilization, and system information.</div>
    </div>
    <div class="page-scroll">

      <!-- Resource Overview Strip -->
      <div class="stat-grid cols-3">
        <div class="stat-card">
          <span class="stat-card-label">CPU</span>
          <span class="stat-card-value" :class="usageColor(sysInfo.cpu_usage)">{{ sysInfo.cpu_usage.toFixed(1) }}%</span>
          <span class="stat-card-sub">{{ sysInfo.cpu_count }} cores</span>
          <div class="stat-card-bar">
            <div class="stat-card-bar-fill" :style="{ width: sysInfo.cpu_usage + '%', background: usageBarColor(sysInfo.cpu_usage) }"></div>
          </div>
        </div>
        <div class="stat-card">
          <span class="stat-card-label">Memory</span>
          <span class="stat-card-value" :class="usageColor(memPct)">{{ memPct.toFixed(1) }}%</span>
          <span class="stat-card-sub">{{ formatBytes(sysInfo.mem_used) }} / {{ formatBytes(sysInfo.mem_total) }}</span>
          <div class="stat-card-bar">
            <div class="stat-card-bar-fill" :style="{ width: memPct + '%', background: usageBarColor(memPct) }"></div>
          </div>
        </div>
        <div class="stat-card">
          <span class="stat-card-label">Storage</span>
          <span class="stat-card-value" :class="usageColor(diskPct)">{{ diskPct.toFixed(1) }}%</span>
          <span class="stat-card-sub">{{ formatBytes(sysInfo.disk_used) }} / {{ formatBytes(sysInfo.disk_total) }}</span>
          <div class="stat-card-bar">
            <div class="stat-card-bar-fill" :style="{ width: diskPct + '%', background: usageBarColor(diskPct) }"></div>
          </div>
        </div>
      </div>

      <!-- System Information -->
      <div class="settings-card">
        <div class="card-header">
          <div class="card-header-icon card-header-icon--system">
            <q-icon name="sym_r_computer" size="18px" />
          </div>
          <div class="card-header-text">
            <div class="card-header-title">System Information</div>
            <div class="card-header-subtitle">Hardware and operating system details</div>
          </div>
          <div class="card-header-actions">
            <span class="uptime-badge">
              <span class="uptime-dot-sm"></span>
              {{ formatUptime(sysInfo.uptime) }}
            </span>
          </div>
        </div>
        <div class="info-grid-2col">
          <div class="info-row">
            <span class="info-label">Hostname</span>
            <span class="info-value">{{ sysInfo.hostname }}</span>
          </div>
          <div class="info-row">
            <span class="info-label">Architecture</span>
            <span class="info-value">{{ sysInfo.arch }}</span>
          </div>
        </div>
        <q-separator class="card-separator" />
        <div class="info-grid-2col">
          <div class="info-row">
            <span class="info-label">Operating System</span>
            <span class="info-value">{{ sysInfo.os_version }}</span>
          </div>
          <div class="info-row">
            <span class="info-label">Kernel</span>
            <span class="info-value">{{ sysInfo.kernel }}</span>
          </div>
        </div>
        <q-separator class="card-separator" />
        <div class="info-row">
          <span class="info-label">CPU</span>
          <span class="info-value">{{ sysInfo.cpu_model }} ({{ sysInfo.cpu_count }} cores)</span>
        </div>
        <q-separator class="card-separator" />
        <div class="info-row">
          <span class="info-label">Load Average</span>
          <span class="info-value">{{ fmtLoad(sysInfo.load) }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<script lang="ts" setup>
import { ref, computed, onMounted, onUnmounted } from 'vue';
import { api } from 'boot/axios';
import { formatBytes, formatUptime, usageColor, usageQColor } from 'src/utils/helpers';

function usageBarColor(pct: number): string {
  if (pct > 80) return '#f87171';
  if (pct > 50) return '#fbbf24';
  return '#34d399';
}

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
.uptime-badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  background: var(--positive-soft);
  color: var(--positive);
  padding: 4px 12px;
  border-radius: var(--radius-sm);
  font-weight: 600;
  font-size: 12px;
  font-family: 'Inter', sans-serif;
}

.uptime-dot-sm {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--positive);
  box-shadow: 0 0 6px var(--positive);
}
</style>
