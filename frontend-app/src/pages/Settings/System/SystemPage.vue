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
          <span class="stat-card-value" :class="usageColor(monitorStore.cpuUsage)">{{ monitorStore.cpuUsage.toFixed(1) }}%</span>
          <span class="stat-card-sub">{{ sysInfo.cpu_count }} cores</span>
          <div class="stat-card-bar">
            <div class="stat-card-bar-fill" :style="{ width: monitorStore.cpuUsage + '%', background: usageBarColor(monitorStore.cpuUsage) }"></div>
          </div>
        </div>
        <div class="stat-card">
          <span class="stat-card-label">Memory</span>
          <span class="stat-card-value" :class="usageColor(memPct)">{{ memPct.toFixed(1) }}%</span>
          <span class="stat-card-sub">{{ formatBytes(monitorStore.memUsed) }} / {{ formatBytes(monitorStore.memTotal) }}</span>
          <div class="stat-card-bar">
            <div class="stat-card-bar-fill" :style="{ width: memPct + '%', background: usageBarColor(memPct) }"></div>
          </div>
        </div>
        <div class="stat-card">
          <span class="stat-card-label">Storage</span>
          <span class="stat-card-value" :class="usageColor(diskPct)">{{ diskPct.toFixed(1) }}%</span>
          <span class="stat-card-sub">{{ formatBytes(monitorStore.diskUsed) }} / {{ formatBytes(monitorStore.diskTotal) }}</span>
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
              {{ formatUptime(monitorStore.uptime) }}
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
        <div class="info-grid-2col">
          <div class="info-row">
            <span class="info-label">CPU</span>
            <span class="info-value">{{ sysInfo.cpu_model }} ({{ sysInfo.cpu_count }} cores)</span>
          </div>
          <div class="info-row">
            <span class="info-label">Memory</span>
            <span class="info-value">{{ formatBytes(sysInfo.memory_total) }}</span>
          </div>
        </div>
        <q-separator class="card-separator" />
        <div class="info-row">
          <span class="info-label">Load Average</span>
          <span class="info-value">{{ fmtLoad(monitorStore.load) }}</span>
        </div>
      </div>

      <!-- Devices -->
      <div v-if="sysInfo.devices.length" class="settings-card">
        <div class="card-header">
          <div class="card-header-icon card-header-icon--network">
            <q-icon name="sym_r_devices" size="18px" />
          </div>
          <div class="card-header-text">
            <div class="card-header-title">Hardware Devices</div>
            <div class="card-header-subtitle">{{ sysInfo.devices.length }} devices detected</div>
          </div>
        </div>
        <template v-for="(dev, i) in sysInfo.devices" :key="i">
          <div class="device-row">
            <div class="device-info">
              <div class="device-icon-wrap">
                <q-icon :name="deviceIcon(dev.type)" size="16px" />
              </div>
              <div>
                <div class="device-name">{{ dev.name }}</div>
                <div class="device-meta">
                  <span class="device-type">{{ dev.type }}</span>
                  <span v-if="dev.driver" class="device-driver">{{ dev.driver }}</span>
                </div>
              </div>
            </div>
          </div>
          <q-separator v-if="i < sysInfo.devices.length - 1" class="card-separator" />
        </template>
      </div>
    </div>
  </div>
</template>

<script lang="ts" setup>
import { ref, computed, onMounted } from 'vue';
import { api } from 'boot/axios';
import { useMonitorStore } from 'stores/monitor';
import { formatBytes, formatUptime, usageColor } from 'src/utils/helpers';

const monitorStore = useMonitorStore();

function usageBarColor(pct: number): string {
  if (pct > 80) return '#f87171';
  if (pct > 50) return '#fbbf24';
  return '#34d399';
}

interface Device { type: string; name: string; driver: string; }

const sysInfo = ref({
  hostname: '--', os_version: '--', kernel: '--', arch: '--',
  cpu_model: '--', cpu_count: 0, memory_total: 0, disk_total: 0,
  devices: [] as Device[],
});

const memPct = computed(() => monitorStore.memTotal ? (monitorStore.memUsed / monitorStore.memTotal) * 100 : 0);
const diskPct = computed(() => monitorStore.diskTotal ? (monitorStore.diskUsed / monitorStore.diskTotal) * 100 : 0);

function fmtLoad(l: number[]) { return l.map(v => v.toFixed(2)).join('  /  '); }

function deviceIcon(type: string): string {
  switch (type) {
    case 'gpu': return 'sym_r_memory';
    case 'igpu': return 'sym_r_memory';
    case 'npu': return 'sym_r_smart_toy';
    case 'wifi': return 'sym_r_wifi';
    case 'ethernet': return 'sym_r_lan';
    case 'nvme': return 'sym_r_storage';
    case 'thunderbolt': return 'sym_r_bolt';
    case 'audio': return 'sym_r_volume_up';
    case 'bluetooth': return 'sym_r_bluetooth';
    default: return 'sym_r_devices';
  }
}

onMounted(async () => {
  try {
    const r: any = await api.get('/api/monitor/system/info');
    if (r) {
      sysInfo.value = {
        hostname: r.hostname || '--',
        os_version: r.os_version || '--',
        kernel: r.kernel || '--',
        arch: r.arch || '--',
        cpu_model: r.cpu_model || '--',
        cpu_count: r.cpu_count || 0,
        memory_total: r.memory_total || 0,
        disk_total: r.disk_total || 0,
        devices: r.devices || [],
      };
    }
  } catch {}
});
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

.device-row { display: flex; justify-content: space-between; align-items: center; padding: 10px 20px; }
.device-info { display: flex; align-items: center; gap: 10px; }
.device-icon-wrap {
  width: 30px; height: 30px; border-radius: 8px;
  background: rgba(255,255,255,0.04); display: flex; align-items: center; justify-content: center; color: var(--ink-3);
}
.device-name { font-size: 13px; font-weight: 500; color: var(--ink-1); }
.device-meta { display: flex; gap: 8px; margin-top: 2px; }
.device-type {
  font-size: 10px; font-weight: 600; text-transform: uppercase;
  color: var(--accent); background: var(--accent-soft);
  padding: 1px 6px; border-radius: 3px;
}
.device-driver {
  font-size: 10px; color: var(--ink-3);
  font-family: 'Inter', sans-serif;
}
</style>
