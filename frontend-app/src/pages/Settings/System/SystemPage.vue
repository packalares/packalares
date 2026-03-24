<template>
  <div class="settings-page">
    <div class="page-title">System</div>
    <div class="page-scroll">
      <!-- System Information -->
      <div class="section-title">System Information</div>
      <div class="settings-card">
        <div class="info-row">
          <span class="info-label">Hostname</span>
          <span class="info-value">{{ systemInfo.hostname }}</span>
        </div>
        <q-separator class="card-separator" />
        <div class="info-row">
          <span class="info-label">Operating System</span>
          <span class="info-value">{{ systemInfo.os_version }}</span>
        </div>
        <q-separator class="card-separator" />
        <div class="info-row">
          <span class="info-label">Kernel</span>
          <span class="info-value">{{ systemInfo.kernel }}</span>
        </div>
        <q-separator class="card-separator" />
        <div class="info-row">
          <span class="info-label">Uptime</span>
          <span class="info-value">{{ systemInfo.uptime }}</span>
        </div>
      </div>

      <!-- CPU Usage -->
      <div class="section-title">CPU</div>
      <div class="settings-card">
        <div class="metric-row">
          <div class="metric-header">
            <span class="info-label">CPU Usage</span>
            <span class="metric-value" :class="usageColor(cpuPercent)">
              {{ cpuPercent.toFixed(1) }}%
            </span>
          </div>
          <q-linear-progress
            :value="cpuPercent / 100"
            :color="usageQColor(cpuPercent)"
            track-color="grey-9"
            rounded
            size="8px"
            class="q-mt-sm"
          />
        </div>
        <q-separator class="card-separator" />
        <div class="info-row">
          <span class="info-label">Load Averages</span>
          <span class="info-value">
            {{ systemInfo.load_1m }} / {{ systemInfo.load_5m }} / {{ systemInfo.load_15m }}
          </span>
        </div>
      </div>

      <!-- Memory Usage -->
      <div class="section-title">Memory</div>
      <div class="settings-card">
        <div class="metric-row">
          <div class="metric-header">
            <span class="info-label">RAM Usage</span>
            <span class="metric-value" :class="usageColor(memPercent)">
              {{ formatBytes(systemInfo.memory_used) }} / {{ formatBytes(systemInfo.memory_total) }}
              ({{ memPercent.toFixed(1) }}%)
            </span>
          </div>
          <q-linear-progress
            :value="memPercent / 100"
            :color="usageQColor(memPercent)"
            track-color="grey-9"
            rounded
            size="8px"
            class="q-mt-sm"
          />
        </div>
      </div>
    </div>
  </div>
</template>

<script lang="ts" setup>
import { ref, computed, onMounted, onUnmounted } from 'vue';
import { api } from 'boot/axios';

interface SystemMetrics {
  hostname: string;
  os_version: string;
  kernel: string;
  uptime: string;
  cpu_percent: number;
  memory_total: number;
  memory_used: number;
  load_1m: string;
  load_5m: string;
  load_15m: string;
}

const systemInfo = ref<SystemMetrics>({
  hostname: '--',
  os_version: '--',
  kernel: '--',
  uptime: '--',
  cpu_percent: 0,
  memory_total: 0,
  memory_used: 0,
  load_1m: '--',
  load_5m: '--',
  load_15m: '--',
});

const cpuPercent = computed(() => systemInfo.value.cpu_percent || 0);
const memPercent = computed(() => {
  if (!systemInfo.value.memory_total) return 0;
  return (systemInfo.value.memory_used / systemInfo.value.memory_total) * 100;
});

const usageColor = (pct: number) => {
  if (pct >= 80) return 'text-usage-red';
  if (pct >= 50) return 'text-usage-yellow';
  return 'text-usage-green';
};

const usageQColor = (pct: number) => {
  if (pct >= 80) return 'red-6';
  if (pct >= 50) return 'amber-7';
  return 'green-6';
};

const formatBytes = (bytes: number) => {
  if (!bytes) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let i = 0;
  let val = bytes;
  while (val >= 1024 && i < units.length - 1) {
    val /= 1024;
    i++;
  }
  return val.toFixed(1) + ' ' + units[i];
};

let pollTimer: ReturnType<typeof setInterval> | null = null;

const fetchMetrics = async () => {
  try {
    const res: any = await api.get('/api/monitor/metrics');
    if (res) {
      systemInfo.value = {
        hostname: res.hostname || '--',
        os_version: res.os_version || '--',
        kernel: res.kernel || '--',
        uptime: res.uptime || '--',
        cpu_percent: res.cpu_percent || 0,
        memory_total: res.memory_total || 0,
        memory_used: res.memory_used || 0,
        load_1m: res.load_1m ?? '--',
        load_5m: res.load_5m ?? '--',
        load_15m: res.load_15m ?? '--',
      };
    }
  } catch {
    // keep last known values
  }
};

onMounted(() => {
  fetchMetrics();
  pollTimer = setInterval(fetchMetrics, 10000);
});

onUnmounted(() => {
  if (pollTimer) clearInterval(pollTimer);
});
</script>

<style lang="scss" scoped>
.settings-page {
  height: 100%;
  display: flex;
  flex-direction: column;
}

.page-title {
  font-size: 18px;
  font-weight: 600;
  color: var(--ink-1);
  padding: 16px 24px;
  height: 56px;
  display: flex;
  align-items: center;
  flex-shrink: 0;
}

.page-scroll {
  flex: 1;
  overflow-y: auto;
  padding: 0 24px 24px;
}

.section-title {
  font-size: 13px;
  font-weight: 500;
  color: var(--ink-2);
  margin-top: 20px;
  margin-bottom: 8px;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.settings-card {
  background: var(--bg-2);
  border-radius: 12px;
  border: 1px solid var(--separator);
  overflow: hidden;
}

.info-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 14px 20px;
}

.info-label {
  font-size: 14px;
  color: var(--ink-1);
  font-weight: 500;
}

.info-value {
  font-size: 13px;
  color: var(--ink-2);
  font-family: 'JetBrains Mono', 'SF Mono', monospace;
}

.metric-row {
  padding: 16px 20px;
}

.metric-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.metric-value {
  font-size: 13px;
  font-weight: 600;
  font-family: 'JetBrains Mono', 'SF Mono', monospace;
}

.card-separator {
  background: var(--separator);
  margin: 0 20px;
}

.text-usage-green {
  color: #29cc5f;
}

.text-usage-yellow {
  color: #febe01;
}

.text-usage-red {
  color: #ff4d4d;
}
</style>
