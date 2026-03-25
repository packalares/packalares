<template>
  <div class="settings-page">
    <div class="page-title">GPU</div>
    <div class="page-scroll">
      <!-- Loading -->
      <div v-if="loading" class="empty-state-full">
        <q-spinner-dots size="40px" color="grey-5" />
        <div class="empty-text">Detecting GPUs...</div>
      </div>

      <!-- No GPU -->
      <div v-else-if="gpus.length === 0" class="empty-state-full">
        <div class="empty-icon-wrap">
          <q-icon name="sym_r_memory" size="36px" color="grey-6" />
        </div>
        <div class="empty-title">No GPU Detected</div>
        <div class="empty-text">No compatible GPU was found. GPU acceleration is unavailable.</div>
      </div>

      <!-- GPU Cards -->
      <template v-else>
        <div v-for="(gpu, idx) in gpus" :key="idx">
          <div class="section-title">GPU {{ idx }}</div>
          <div class="settings-card">
            <div class="info-row">
              <span class="info-label">Name</span>
              <span class="info-value">{{ gpu.name }}</span>
            </div>
            <q-separator class="card-separator" />
            <div class="info-row">
              <span class="info-label">Driver</span>
              <span class="info-value">{{ gpu.driver }}</span>
            </div>
            <q-separator class="card-separator" />
            <div class="metric-row">
              <div class="metric-header">
                <span class="info-label">VRAM</span>
                <span class="metric-value" :class="usageColor(vramPercent(gpu))">
                  {{ gpu.vram_used_mb }} / {{ gpu.vram_total_mb }} MB
                </span>
              </div>
              <q-linear-progress :value="vramPercent(gpu) / 100" :color="usageQColor(vramPercent(gpu))" track-color="grey-9" rounded size="6px" class="q-mt-sm" />
              <div class="metric-sub">{{ vramPercent(gpu).toFixed(1) }}% used</div>
            </div>
            <q-separator class="card-separator" />
            <div class="metric-row">
              <div class="metric-header">
                <span class="info-label">Utilization</span>
                <span class="metric-value" :class="usageColor(gpu.utilization)">{{ gpu.utilization }}%</span>
              </div>
              <q-linear-progress :value="gpu.utilization / 100" :color="usageQColor(gpu.utilization)" track-color="grey-9" rounded size="6px" class="q-mt-sm" />
            </div>
            <q-separator class="card-separator" />
            <div class="info-row">
              <span class="info-label">Temperature</span>
              <span class="info-value temp-value" :class="gpu.temperature >= 80 ? 'temp-hot' : gpu.temperature >= 60 ? 'temp-warm' : 'temp-cool'">
                {{ gpu.temperature }}&deg;C
              </span>
            </div>
            <q-separator class="card-separator" />
            <div class="info-row">
              <span class="info-label">Power</span>
              <span class="info-value">{{ gpu.power_draw }}W / {{ gpu.power_limit }}W</span>
            </div>
          </div>
        </div>
      </template>
    </div>
  </div>
</template>

<script lang="ts" setup>
import { ref, onMounted } from 'vue';
import { api } from 'boot/axios';

interface GpuInfo {
  name: string; driver: string;
  vram_total_mb: number; vram_used_mb: number;
  utilization: number; temperature: number;
  power_draw: number; power_limit: number;
}

const gpus = ref<GpuInfo[]>([]);
const loading = ref(true);

const vramPercent = (g: GpuInfo) => g.vram_total_mb ? (g.vram_used_mb / g.vram_total_mb) * 100 : 0;
const usageColor = (p: number) => p >= 80 ? 'text-red-5' : p >= 50 ? 'text-amber-7' : 'text-green-5';
const usageQColor = (p: number) => p >= 80 ? 'red-6' : p >= 50 ? 'amber-7' : 'green-6';

onMounted(async () => {
  try {
    const res: any = await api.get('/api/monitor/gpu/list');
    const list = res?.gpus || (Array.isArray(res) ? res : []);
    gpus.value = list.map((g: any) => ({
      name: g.name || g.product_name || 'Unknown GPU',
      driver: g.driver || g.driver_version || '--',
      vram_total_mb: g.vram_total_mb || g.memory_total || 0,
      vram_used_mb: g.vram_used_mb || g.memory_used || 0,
      utilization: g.utilization || g.gpu_utilization || 0,
      temperature: g.temperature || g.gpu_temp || 0,
      power_draw: g.power_draw || 0,
      power_limit: g.power_limit || 0,
    }));
  } catch { gpus.value = []; }
  loading.value = false;
});
</script>

<style lang="scss" scoped>
.empty-state-full {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 80px 20px;
}

.empty-icon-wrap {
  width: 72px;
  height: 72px;
  border-radius: 20px;
  background: var(--bg-2);
  border: 1px solid var(--separator);
  display: flex;
  align-items: center;
  justify-content: center;
}

.empty-title {
  font-size: 16px;
  font-weight: 600;
  color: var(--ink-1);
  margin-top: 16px;
}

.empty-text {
  font-size: 13px;
  color: var(--ink-3);
  margin-top: 6px;
  text-align: center;
  max-width: 280px;
}

.metric-sub {
  font-size: 11px;
  color: var(--ink-3);
  margin-top: 6px;
  text-align: right;
}

.temp-value {
  padding: 2px 10px;
  border-radius: 6px;
  font-weight: 600;
  font-size: 12px;
}

.temp-cool { background: var(--positive-soft); color: var(--positive); }
.temp-warm { background: var(--warning-soft); color: var(--warning); }
.temp-hot { background: var(--negative-soft); color: var(--negative); }
</style>
