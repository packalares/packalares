<template>
  <div class="settings-page">
    <div class="page-title">GPU</div>
    <div class="page-scroll">
      <!-- Loading state -->
      <div v-if="loading" class="empty-state">
        <q-spinner-dots size="40px" color="grey-5" />
        <div class="empty-text">Detecting GPUs...</div>
      </div>

      <!-- No GPU detected -->
      <div v-else-if="gpus.length === 0" class="empty-state-full">
        <q-icon name="sym_r_memory" size="64px" color="grey-7" />
        <div class="empty-title">No GPU Detected</div>
        <div class="empty-text">
          No compatible GPU was found on this system. GPU acceleration is unavailable.
        </div>
      </div>

      <!-- GPU Cards -->
      <template v-else>
        <div v-for="(gpu, idx) in gpus" :key="idx">
          <div class="section-title">GPU {{ idx }}</div>
          <div class="settings-card">
            <!-- Name -->
            <div class="info-row">
              <span class="info-label">Name</span>
              <span class="info-value">{{ gpu.name }}</span>
            </div>
            <q-separator class="card-separator" />

            <!-- Driver -->
            <div class="info-row">
              <span class="info-label">Driver</span>
              <span class="info-value">{{ gpu.driver }}</span>
            </div>
            <q-separator class="card-separator" />

            <!-- VRAM -->
            <div class="metric-row">
              <div class="metric-header">
                <span class="info-label">VRAM</span>
                <span class="metric-value" :class="usageColor(vramPercent(gpu))">
                  {{ gpu.vram_used_mb }} MB / {{ gpu.vram_total_mb }} MB
                  ({{ vramPercent(gpu).toFixed(1) }}%)
                </span>
              </div>
              <q-linear-progress
                :value="vramPercent(gpu) / 100"
                :color="usageQColor(vramPercent(gpu))"
                track-color="grey-9"
                rounded
                size="8px"
                class="q-mt-sm"
              />
            </div>
            <q-separator class="card-separator" />

            <!-- GPU Utilization -->
            <div class="metric-row">
              <div class="metric-header">
                <span class="info-label">Utilization</span>
                <span class="metric-value" :class="usageColor(gpu.utilization)">
                  {{ gpu.utilization }}%
                </span>
              </div>
              <q-linear-progress
                :value="gpu.utilization / 100"
                :color="usageQColor(gpu.utilization)"
                track-color="grey-9"
                rounded
                size="8px"
                class="q-mt-sm"
              />
            </div>
            <q-separator class="card-separator" />

            <!-- Temperature -->
            <div class="info-row">
              <span class="info-label">Temperature</span>
              <span
                class="info-value"
                :class="{
                  'text-usage-red': gpu.temperature >= 80,
                  'text-usage-yellow': gpu.temperature >= 60 && gpu.temperature < 80,
                  'text-usage-green': gpu.temperature < 60,
                }"
              >
                {{ gpu.temperature }}&deg;C
              </span>
            </div>
            <q-separator class="card-separator" />

            <!-- Power -->
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

interface GpuInfo {
  name: string;
  driver: string;
  vram_total_mb: number;
  vram_used_mb: number;
  utilization: number;
  temperature: number;
  power_draw: number;
  power_limit: number;
}

const gpus = ref<GpuInfo[]>([]);
const loading = ref(true);

const vramPercent = (gpu: GpuInfo) => {
  if (!gpu.vram_total_mb) return 0;
  return (gpu.vram_used_mb / gpu.vram_total_mb) * 100;
};

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

onMounted(async () => {
  try {
    const res: any = await api.get('/api/monitor/gpu/list');
    if (res && Array.isArray(res)) {
      gpus.value = res.map((g: any) => ({
        name: g.name || g.product_name || 'Unknown GPU',
        driver: g.driver || g.driver_version || '--',
        vram_total_mb: g.vram_total_mb || g.memory_total || 0,
        vram_used_mb: g.vram_used_mb || g.memory_used || 0,
        utilization: g.utilization || g.gpu_utilization || 0,
        temperature: g.temperature || g.gpu_temp || 0,
        power_draw: g.power_draw || 0,
        power_limit: g.power_limit || 0,
      }));
    } else if (res && res.gpus) {
      gpus.value = res.gpus.map((g: any) => ({
        name: g.name || g.product_name || 'Unknown GPU',
        driver: g.driver || g.driver_version || '--',
        vram_total_mb: g.vram_total_mb || g.memory_total || 0,
        vram_used_mb: g.vram_used_mb || g.memory_used || 0,
        utilization: g.utilization || g.gpu_utilization || 0,
        temperature: g.temperature || g.gpu_temp || 0,
        power_draw: g.power_draw || 0,
        power_limit: g.power_limit || 0,
      }));
    }
  } catch {
    gpus.value = [];
  } finally {
    loading.value = false;
  }
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

.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 60px 20px;
}

.empty-state-full {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 80px 20px;
  margin-top: 40px;
}

.empty-title {
  font-size: 18px;
  font-weight: 600;
  color: var(--ink-1);
  margin-top: 16px;
}

.empty-text {
  font-size: 13px;
  color: var(--ink-3);
  margin-top: 8px;
  text-align: center;
  max-width: 300px;
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
