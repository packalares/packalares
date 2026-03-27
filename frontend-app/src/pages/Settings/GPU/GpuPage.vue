<template>
  <div class="settings-page">
    <div class="page-header">
      <div class="page-title">GPU</div>
      <div class="page-description">GPU hardware details, VRAM utilization, temperature, and power draw.</div>
    </div>
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
          <!-- GPU Stat Strip -->
          <div class="stat-grid cols-4 q-mb-sm" v-if="idx === 0">
            <div class="stat-card">
              <span class="stat-card-label">Utilization</span>
              <span class="stat-card-value" :class="usageColor(gpu.utilization)">{{ gpu.utilization }}%</span>
              <div class="stat-card-bar">
                <div class="stat-card-bar-fill" :style="{ width: gpu.utilization + '%', background: '#c07ae0' }"></div>
              </div>
            </div>
            <div class="stat-card">
              <span class="stat-card-label">VRAM</span>
              <span class="stat-card-value" :class="usageColor(vramPercent(gpu))">{{ vramPercent(gpu).toFixed(0) }}%</span>
              <span class="stat-card-sub">{{ gpu.vram_used_mb }} / {{ gpu.vram_total_mb }} MB</span>
              <div class="stat-card-bar">
                <div class="stat-card-bar-fill" :style="{ width: vramPercent(gpu) + '%', background: '#818cf8' }"></div>
              </div>
            </div>
            <div class="stat-card">
              <span class="stat-card-label">Temperature</span>
              <span class="stat-card-value" :class="gpu.temperature >= 80 ? 'text-red-5' : gpu.temperature >= 60 ? 'text-amber-7' : 'text-green-5'">{{ gpu.temperature }}&deg;C</span>
            </div>
            <div class="stat-card">
              <span class="stat-card-label">Power</span>
              <span class="stat-card-value">{{ gpu.power_draw }}W</span>
              <span class="stat-card-sub">Limit: {{ gpu.power_limit }}W</span>
            </div>
          </div>

          <!-- GPU Details Card -->
          <div class="settings-card">
            <div class="card-header">
              <div class="card-header-icon card-header-icon--gpu">
                <q-icon name="sym_r_memory" size="18px" />
              </div>
              <div class="card-header-text">
                <div class="card-header-title">GPU {{ idx }} -- {{ gpu.name }}</div>
                <div class="card-header-subtitle">Driver {{ gpu.driver }}</div>
              </div>
              <div class="card-header-actions">
                <span class="temp-value" :class="gpu.temperature >= 80 ? 'temp-hot' : gpu.temperature >= 60 ? 'temp-warm' : 'temp-cool'">
                  {{ gpu.temperature }}&deg;C
                </span>
              </div>
            </div>
            <div class="info-grid-2col">
              <div class="info-row">
                <span class="info-label">Name</span>
                <span class="info-value">{{ gpu.name }}</span>
              </div>
              <div class="info-row">
                <span class="info-label">Driver</span>
                <span class="info-value">{{ gpu.driver }}</span>
              </div>
            </div>
            <q-separator class="card-separator" />
            <div class="metric-row">
              <div class="metric-header">
                <span class="info-label">VRAM Usage</span>
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
                <span class="info-label">GPU Utilization</span>
                <span class="metric-value" :class="usageColor(gpu.utilization)">{{ gpu.utilization }}%</span>
              </div>
              <q-linear-progress :value="gpu.utilization / 100" :color="usageQColor(gpu.utilization)" track-color="grey-9" rounded size="6px" class="q-mt-sm" />
            </div>
            <q-separator class="card-separator" />
            <div class="info-row">
              <span class="info-label">Power Draw</span>
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
import { usageColor, usageQColor } from 'src/utils/helpers';

interface GpuInfo {
  name: string; driver: string;
  vram_total_mb: number; vram_used_mb: number;
  utilization: number; temperature: number;
  power_draw: number; power_limit: number;
}

const gpus = ref<GpuInfo[]>([]);
const loading = ref(true);

const vramPercent = (g: GpuInfo) => g.vram_total_mb ? (g.vram_used_mb / g.vram_total_mb) * 100 : 0;

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
  display: flex; flex-direction: column; align-items: center;
  justify-content: center; padding: 80px 20px;
}
.empty-icon-wrap {
  width: 64px; height: 64px; border-radius: 16px;
  background: var(--bg-2); border: 1px solid var(--border);
  display: flex; align-items: center; justify-content: center;
  box-shadow: var(--shadow-card);
}
.empty-title { font-size: 15px; font-weight: 600; color: var(--ink-1); margin-top: 16px; }
.empty-text { font-size: 13px; color: var(--ink-3); margin-top: 4px; text-align: center; max-width: 260px; line-height: 1.5; }
.temp-value { padding: 3px 10px; border-radius: var(--radius-xs); font-weight: 600; font-size: 12px; }
.temp-cool { background: var(--positive-soft); color: var(--positive); }
.temp-warm { background: var(--warning-soft); color: var(--warning); }
.temp-hot { background: var(--negative-soft); color: var(--negative); }
</style>
