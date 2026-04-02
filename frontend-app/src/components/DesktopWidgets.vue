<template>
  <div class="widgets-container" v-if="visible">
    <div class="widget-toggle" @click="toggleWidgets(false)">
      <q-icon name="sym_r_widgets" size="18px" />
    </div>

    <!-- Clock -->
    <div
      v-if="enabledWidgets.clock"
      class="widget widget-clock"
      :style="widgetStyle('clock')"
      @mousedown="startDrag($event, 'clock')"
    >
      <div class="clock-time">{{ clockTime }}</div>
      <div class="clock-date">{{ weekDay }}, {{ dateStr }}</div>
    </div>

    <!-- Weather -->
    <div
      v-if="enabledWidgets.weather && m.weather.city"
      class="widget widget-weather"
      :style="widgetStyle('weather')"
      @mousedown="startDrag($event, 'weather')"
    >
      <div class="weather-top">
        <div class="weather-left">
          <div class="weather-city">{{ m.weather.city }}</div>
          <div class="weather-temp-big">{{ Math.round(m.weather.temperature) }}&deg;</div>
        </div>
        <div class="weather-right">
          <q-icon :name="'sym_r_' + (m.weather.icon || 'partly_cloudy_day')" size="36px" class="weather-cond-icon" />
          <div class="weather-cond-text">{{ m.weather.description }}</div>
        </div>
      </div>
      <div class="weather-bottom">
        <span>Wind {{ m.weather.windSpeed?.toFixed(0) }} km/h</span>
        <span>{{ m.weather.country }}</span>
      </div>
    </div>

    <!-- System -->
    <div
      v-if="enabledWidgets.system"
      class="widget widget-system"
      :style="widgetStyle('system')"
      @mousedown="startDrag($event, 'system')"
    >
      <div class="widget-header">
        <q-icon name="sym_r_monitor_heart" size="14px" />
        <span>System</span>
      </div>
      <div class="gauge-row">
        <div class="gauge-item">
          <svg viewBox="0 0 36 36" class="gauge-svg">
            <path class="gauge-bg" d="M18 2.0845 a 15.9155 15.9155 0 0 1 0 31.831 a 15.9155 15.9155 0 0 1 0 -31.831" />
            <path class="gauge-fill" :stroke="gaugeColor(cpuPct)" :stroke-dasharray="cpuPct + ', 100'" d="M18 2.0845 a 15.9155 15.9155 0 0 1 0 31.831 a 15.9155 15.9155 0 0 1 0 -31.831" />
          </svg>
          <div class="gauge-label">
            <span class="gauge-val">{{ cpuPct }}</span>
            <span class="gauge-unit">CPU</span>
          </div>
        </div>
        <div class="gauge-item">
          <svg viewBox="0 0 36 36" class="gauge-svg">
            <path class="gauge-bg" d="M18 2.0845 a 15.9155 15.9155 0 0 1 0 31.831 a 15.9155 15.9155 0 0 1 0 -31.831" />
            <path class="gauge-fill" :stroke="gaugeColor(memPct)" :stroke-dasharray="memPct + ', 100'" d="M18 2.0845 a 15.9155 15.9155 0 0 1 0 31.831 a 15.9155 15.9155 0 0 1 0 -31.831" />
          </svg>
          <div class="gauge-label">
            <span class="gauge-val">{{ memPct }}</span>
            <span class="gauge-unit">MEM</span>
          </div>
        </div>
        <div class="gauge-item">
          <svg viewBox="0 0 36 36" class="gauge-svg">
            <path class="gauge-bg" d="M18 2.0845 a 15.9155 15.9155 0 0 1 0 31.831 a 15.9155 15.9155 0 0 1 0 -31.831" />
            <path class="gauge-fill" :stroke="gaugeColor(diskPct)" :stroke-dasharray="diskPct + ', 100'" d="M18 2.0845 a 15.9155 15.9155 0 0 1 0 31.831 a 15.9155 15.9155 0 0 1 0 -31.831" />
          </svg>
          <div class="gauge-label">
            <span class="gauge-val">{{ diskPct }}</span>
            <span class="gauge-unit">DSK</span>
          </div>
        </div>
      </div>
      <div class="widget-footer">
        <span>{{ formatUptime(m.uptime) }}</span>
        <span v-if="m.cpuFreqMHz > 0">{{ (m.cpuFreqMHz / 1000).toFixed(1) }} GHz</span>
        <span>{{ m.load[0]?.toFixed(1) }} load</span>
      </div>
      <div v-if="m.fans.length" class="widget-footer" style="margin-top:2px">
        <span v-for="f in m.fans" :key="f.name">{{ f.name.replace(/_/g,' ') }}: {{ f.rpm }} RPM</span>
      </div>
    </div>

    <!-- Temperature -->
    <div
      v-if="enabledWidgets.temps"
      class="widget widget-temps"
      :style="widgetStyle('temps')"
      @mousedown="startDrag($event, 'temps')"
    >
      <div class="widget-header">
        <q-icon name="sym_r_thermostat" size="14px" />
        <span>Temperature</span>
      </div>
      <div class="temp-grid">
        <div class="temp-item">
          <span class="temp-val" :style="{ color: m.tempCPU > 0 ? tempColor(m.tempCPU) : 'rgba(255,255,255,0.3)' }">{{ m.tempCPU > 0 ? Math.round(m.tempCPU) : '--' }}</span>
          <span class="temp-unit">CPU</span>
        </div>
        <div class="temp-item" v-if="m.gpuTemp > 0 || m.tempGPU > 0">
          <span class="temp-val" :style="{ color: tempColor(m.gpuTemp || m.tempGPU) }">{{ Math.round(m.gpuTemp || m.tempGPU) }}</span>
          <span class="temp-unit">GPU</span>
        </div>
        <div class="temp-item">
          <span class="temp-val" :style="{ color: m.tempNVMe > 0 ? tempColor(m.tempNVMe) : 'rgba(255,255,255,0.3)' }">{{ m.tempNVMe > 0 ? Math.round(m.tempNVMe) : '--' }}</span>
          <span class="temp-unit">NVMe</span>
        </div>
      </div>
    </div>

    <!-- Power -->
    <div
      v-if="enabledWidgets.power"
      class="widget widget-power"
      :style="widgetStyle('power')"
      @mousedown="startDrag($event, 'power')"
    >
      <div class="widget-header">
        <q-icon name="sym_r_bolt" size="14px" />
        <span>Power</span>
      </div>
      <div class="power-total">{{ m.powerTotal > 0 ? m.powerTotal.toFixed(0) : '--' }}<span class="power-unit">W</span></div>
      <div class="power-breakdown">
        <span>CPU {{ m.powerCPU > 0 ? m.powerCPU.toFixed(0) + 'W' : '--' }}</span>
        <span>GPU {{ m.powerGPU > 0 ? m.powerGPU.toFixed(0) + 'W' : '--' }}</span>
      </div>
    </div>

    <!-- Network -->
    <div
      v-if="enabledWidgets.network"
      class="widget widget-network"
      :style="widgetStyle('network')"
      @mousedown="startDrag($event, 'network')"
    >
      <div class="widget-header">
        <q-icon name="sym_r_speed" size="14px" />
        <span>Network</span>
      </div>
      <div class="net-row">
        <div class="net-item">
          <q-icon name="sym_r_arrow_downward" size="12px" class="net-icon-down" />
          <span class="net-val">{{ formatRate(m.netRx) }}</span>
        </div>
        <div class="net-item">
          <q-icon name="sym_r_arrow_upward" size="12px" class="net-icon-up" />
          <span class="net-val">{{ formatRate(m.netTx) }}</span>
        </div>
      </div>
      <div class="net-sparkline">
        <svg viewBox="0 0 120 30" preserveAspectRatio="none" class="sparkline-svg">
          <polyline :points="rxSparkline" class="spark-rx" />
          <polyline :points="txSparkline" class="spark-tx" />
        </svg>
      </div>
    </div>

    <!-- GPU (only if detected) -->
    <div
      v-if="enabledWidgets.gpu && m.gpuName"
      class="widget widget-gpu"
      :style="widgetStyle('gpu')"
      @mousedown="startDrag($event, 'gpu')"
    >
      <div class="widget-header">
        <q-icon name="sym_r_memory" size="14px" />
        <span>GPU</span>
        <span class="gpu-header-info">{{ shortGPUName }} {{ gpuMemGB }}GB</span>
      </div>
      <div class="gauge-row">
        <div class="gauge-item">
          <svg viewBox="0 0 36 36" class="gauge-svg">
            <path class="gauge-bg" d="M18 2.0845 a 15.9155 15.9155 0 0 1 0 31.831 a 15.9155 15.9155 0 0 1 0 -31.831" />
            <path class="gauge-fill" :stroke="gaugeColor(m.gpuUtil)" :stroke-dasharray="m.gpuUtil + ', 100'" d="M18 2.0845 a 15.9155 15.9155 0 0 1 0 31.831 a 15.9155 15.9155 0 0 1 0 -31.831" />
          </svg>
          <div class="gauge-label">
            <span class="gauge-val">{{ m.gpuUtil }}</span>
            <span class="gauge-unit">UTIL</span>
          </div>
        </div>
        <div class="gauge-item">
          <svg viewBox="0 0 36 36" class="gauge-svg">
            <path class="gauge-bg" d="M18 2.0845 a 15.9155 15.9155 0 0 1 0 31.831 a 15.9155 15.9155 0 0 1 0 -31.831" />
            <path class="gauge-fill" :stroke="gaugeColor(gpuMemPct)" :stroke-dasharray="gpuMemPct + ', 100'" d="M18 2.0845 a 15.9155 15.9155 0 0 1 0 31.831 a 15.9155 15.9155 0 0 1 0 -31.831" />
          </svg>
          <div class="gauge-label">
            <span class="gauge-val">{{ gpuMemPct }}</span>
            <span class="gauge-unit">VRAM</span>
          </div>
        </div>
      </div>
    </div>
  </div>

  <div v-if="!visible" class="widget-toggle widget-toggle-hidden" @click="toggleWidgets(true)">
    <q-icon name="sym_r_widgets" size="18px" />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue';
import { useMonitorStore } from 'stores/monitor';

const m = useMonitorStore();
const visible = ref(localStorage.getItem('packalares_widgets_visible') !== 'false');

function toggleWidgets(show: boolean) {
  visible.value = show;
  localStorage.setItem('packalares_widgets_visible', String(show));
}

// Layout constants
const WIDGET_WIDTH = 240;
const HALF_WIDTH = 114;
const RIGHT_MARGIN = 20;
const BOTTOM_MARGIN = 60; // above dock

const defaultPositions: Record<string, { x: number; y: number }> = {
  // Right column
  clock:   { x: -(WIDGET_WIDTH + RIGHT_MARGIN), y: 20 },
  system:  { x: -(WIDGET_WIDTH + RIGHT_MARGIN), y: 110 },
  temps:   { x: -(WIDGET_WIDTH + RIGHT_MARGIN), y: 270 },
  power:   { x: -(WIDGET_WIDTH + RIGHT_MARGIN), y: 370 },
  network: { x: -(WIDGET_WIDTH + RIGHT_MARGIN), y: 470 },
  gpu:     { x: -(WIDGET_WIDTH + RIGHT_MARGIN), y: 570 },
  // Bottom-left
  weather: { x: 20, y: -(BOTTOM_MARGIN + 120) },
};

const positions = ref<Record<string, { x: number; y: number }>>(loadPositions());
const enabledWidgets = ref<Record<string, boolean>>(loadEnabled());

function loadPositions(): Record<string, { x: number; y: number }> {
  try {
    const saved = localStorage.getItem('packalares_widget_positions');
    if (saved) return JSON.parse(saved);
  } catch {}
  return { ...defaultPositions };
}

function savePositions() {
  localStorage.setItem('packalares_widget_positions', JSON.stringify(positions.value));
}

function resetPositions() {
  positions.value = { ...defaultPositions };
  localStorage.removeItem('packalares_widget_positions');
}

defineExpose({ resetPositions });

function loadEnabled(): Record<string, boolean> {
  try {
    const saved = localStorage.getItem('packalares_widget_enabled');
    if (saved) return JSON.parse(saved);
  } catch {}
  return { clock: true, weather: true, system: true, temps: true, power: true, network: true, gpu: true };
}

function widgetStyle(id: string) {
  const p = positions.value[id] || defaultPositions[id] || { x: 0, y: 0 };
  const s: Record<string, string> = { position: 'absolute' };
  if (p.x < 0) { s.right = Math.abs(p.x) + 'px'; } else { s.left = p.x + 'px'; }
  if (p.y < 0) { s.bottom = Math.abs(p.y) + 'px'; } else { s.top = p.y + 'px'; }
  return s;
}

// Drag
let dragWidget = '';
let dragOffset = { x: 0, y: 0 };

function startDrag(e: MouseEvent, id: string) {
  if (e.button !== 0) return;
  dragWidget = id;
  const el = (e.target as HTMLElement).closest('.widget') as HTMLElement;
  if (!el) return;
  const rect = el.getBoundingClientRect();
  dragOffset = { x: e.clientX - rect.left, y: e.clientY - rect.top };
  document.addEventListener('mousemove', onDrag);
  document.addEventListener('mouseup', stopDrag);
  e.preventDefault();
}

function onDrag(e: MouseEvent) {
  if (!dragWidget) return;
  positions.value[dragWidget] = { x: e.clientX - dragOffset.x, y: e.clientY - dragOffset.y };
}

function stopDrag() {
  dragWidget = '';
  savePositions();
  document.removeEventListener('mousemove', onDrag);
  document.removeEventListener('mouseup', stopDrag);
}

// Clock
const clockTime = ref('');
const weekDay = ref('');
const dateStr = ref('');
let clockTimer: ReturnType<typeof setInterval>;

function updateClock() {
  const now = new Date();
  clockTime.value = now.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  weekDay.value = now.toLocaleDateString([], { weekday: 'long' });
  dateStr.value = now.toLocaleDateString([], { month: 'long', day: 'numeric', year: 'numeric' });
}

// Computed
const cpuPct = computed(() => Math.round(m.cpuUsage || 0));
const memPct = computed(() => m.memTotal ? Math.round((m.memUsed / m.memTotal) * 100) : 0);
const diskPct = computed(() => m.diskTotal ? Math.round((m.diskUsed / m.diskTotal) * 100) : 0);
const gpuMemPct = computed(() => m.gpuMemTotal ? Math.round((m.gpuMemUsed / m.gpuMemTotal) * 100) : 0);

const shortGPUName = computed(() => {
  const n = m.gpuName;
  if (!n) return '';
  return n.replace(/NVIDIA /, '').replace(/GeForce /, '').replace(/ Laptop GPU/, '');
});
const gpuMemGB = computed(() => m.gpuMemTotal ? Math.round(m.gpuMemTotal / 1024) : 0);

// Sparkline
const rxHistory = ref<number[]>(Array(30).fill(0));
const txHistory = ref<number[]>(Array(30).fill(0));
let sparkTimer: ReturnType<typeof setInterval>;

function updateSparkline() {
  rxHistory.value.push(m.netRx);
  txHistory.value.push(m.netTx);
  if (rxHistory.value.length > 30) rxHistory.value.shift();
  if (txHistory.value.length > 30) txHistory.value.shift();
}

const rxSparkline = computed(() => buildSparkline(rxHistory.value));
const txSparkline = computed(() => buildSparkline(txHistory.value));

function buildSparkline(data: number[]): string {
  const max = Math.max(...data, 1);
  return data.map((v, i) => `${(i / (data.length - 1)) * 120},${30 - (v / max) * 28}`).join(' ');
}

// Helpers
function gaugeColor(pct: number): string {
  if (pct > 80) return '#ef4444';
  if (pct > 50) return '#f59e0b';
  return '#22c55e';
}

function tempColor(t: number): string {
  if (t > 80) return '#ef4444';
  if (t > 60) return '#f59e0b';
  return '#22c55e';
}

function formatRate(bytesPerSec: number): string {
  if (bytesPerSec > 1048576) return (bytesPerSec / 1048576).toFixed(1) + ' MB/s';
  if (bytesPerSec > 1024) return (bytesPerSec / 1024).toFixed(0) + ' KB/s';
  return bytesPerSec.toFixed(0) + ' B/s';
}

function formatUptime(seconds: number): string {
  const d = Math.floor(seconds / 86400);
  const h = Math.floor((seconds % 86400) / 3600);
  if (d > 0) return `${d}d ${h}h`;
  const mn = Math.floor((seconds % 3600) / 60);
  return `${h}h ${mn}m`;
}

onMounted(() => {
  updateClock();
  clockTimer = setInterval(updateClock, 1000);
  sparkTimer = setInterval(updateSparkline, 5000);
});

onUnmounted(() => {
  clearInterval(clockTimer);
  clearInterval(sparkTimer);
});
</script>

<style scoped lang="scss">
.widgets-container {
  position: absolute;
  inset: 0;
  pointer-events: none;
  z-index: 5;
}

.widget {
  pointer-events: auto;
  background: rgba(0, 0, 0, 0.45);
  backdrop-filter: blur(24px) saturate(1.4);
  -webkit-backdrop-filter: blur(24px) saturate(1.4);
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 16px;
  padding: 14px 16px;
  color: #fff;
  cursor: grab;
  user-select: none;
  width: 240px;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.3), inset 0 0.5px 0 rgba(255, 255, 255, 0.1);
  transition: box-shadow 0.2s;
  &:hover { box-shadow: 0 12px 40px rgba(0, 0, 0, 0.4), inset 0 0.5px 0 rgba(255, 255, 255, 0.15); }
  &:active { cursor: grabbing; }
}

.widget-header {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: rgba(255, 255, 255, 0.5);
  margin-bottom: 10px;
}

.widget-footer {
  display: flex;
  justify-content: space-between;
  font-size: 10px;
  color: rgba(255, 255, 255, 0.4);
  margin-top: 8px;
}

// ─── Clock ───
.widget-clock { padding: 18px 20px; }
.clock-time {
  font-size: 44px;
  font-weight: 200;
  letter-spacing: -2px;
  line-height: 1;
  text-shadow: 0 2px 10px rgba(0, 0, 0, 0.3);
}
.clock-date {
  font-size: 12px;
  color: rgba(255, 255, 255, 0.6);
  margin-top: 6px;
}

// ─── Weather ───
.widget-weather { padding: 14px 16px; }
.weather-top {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
}
.weather-left { flex: 1; }
.weather-city {
  font-size: 12px;
  font-weight: 500;
  color: rgba(255, 255, 255, 0.7);
  line-height: 1;
}
.weather-temp-big {
  font-size: 42px;
  font-weight: 200;
  letter-spacing: -2px;
  line-height: 1;
  margin-top: 2px;
}
.weather-right {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
  padding-top: 2px;
}
.weather-cond-icon { color: #fbbf24; }
.weather-cond-text {
  font-size: 10px;
  color: rgba(255, 255, 255, 0.5);
  text-align: center;
  max-width: 80px;
}
.weather-bottom {
  display: flex;
  justify-content: space-between;
  font-size: 10px;
  color: rgba(255, 255, 255, 0.35);
  margin-top: 6px;
}

// ─── Gauges ───
.gauge-row { display: flex; gap: 14px; justify-content: center; }
.gauge-item { display: flex; flex-direction: column; align-items: center; width: 52px; }
.gauge-svg { width: 48px; height: 48px; transform: rotate(-90deg); }
.gauge-bg { fill: none; stroke: rgba(255, 255, 255, 0.08); stroke-width: 3; }
.gauge-fill { fill: none; stroke-width: 3; stroke-linecap: round; transition: stroke-dasharray 0.6s ease; }
.gauge-label { text-align: center; margin-top: -32px; position: relative; }
.gauge-val { font-size: 14px; font-weight: 600; display: block; line-height: 1; }
.gauge-unit { font-size: 8px; text-transform: uppercase; color: rgba(255, 255, 255, 0.4); letter-spacing: 0.05em; }

// ─── Temperature ───
.temp-grid { display: flex; gap: 20px; justify-content: center; }
.temp-item { text-align: center; }
.temp-val {
  font-size: 28px; font-weight: 300; line-height: 1;
  &::after { content: '\00B0'; font-size: 16px; }
}
.temp-unit { display: block; font-size: 9px; text-transform: uppercase; color: rgba(255, 255, 255, 0.4); margin-top: 2px; }

// ─── Power ───
.power-total { font-size: 36px; font-weight: 300; text-align: center; line-height: 1; }
.power-unit { font-size: 16px; color: rgba(255, 255, 255, 0.5); }
.power-breakdown { display: flex; justify-content: center; gap: 12px; font-size: 10px; color: rgba(255, 255, 255, 0.4); margin-top: 4px; }

// ─── Network ───
.net-row { display: flex; justify-content: space-between; margin-bottom: 6px; }
.net-item { display: flex; align-items: center; gap: 4px; }
.net-icon-down { color: #22c55e; }
.net-icon-up { color: #3b82f6; }
.net-val { font-size: 13px; font-weight: 500; }
.net-sparkline { height: 30px; opacity: 0.7; }
.sparkline-svg { width: 100%; height: 100%; }
.spark-rx { fill: none; stroke: #22c55e; stroke-width: 1.5; }
.spark-tx { fill: none; stroke: #3b82f6; stroke-width: 1.5; }

// ─── GPU ───
.gpu-header-info { margin-left: auto; font-weight: 400; font-size: 10px; text-transform: none; letter-spacing: 0; }

// ─── Toggle ───
.widget-toggle, .widget-toggle-hidden {
  pointer-events: auto;
  position: absolute;
  top: 12px;
  right: 12px;
  z-index: 6;
  width: 32px; height: 32px;
  border-radius: 10px;
  display: flex; align-items: center; justify-content: center;
  cursor: pointer;
  color: rgba(255, 255, 255, 0.6);
  background: rgba(0, 0, 0, 0.3);
  backdrop-filter: blur(12px);
  border: 1px solid rgba(255, 255, 255, 0.08);
  transition: all 0.2s;
  &:hover { background: rgba(0, 0, 0, 0.5); color: #fff; }
}
</style>
