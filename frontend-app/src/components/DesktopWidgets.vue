<template>
  <div class="widgets-container" v-if="visible">
    <!-- Toggle button -->
    <div class="widget-toggle" @click="visible = !visible">
      <q-icon name="sym_r_widgets" size="18px" />
    </div>

    <!-- Clock Widget -->
    <div
      v-if="enabledWidgets.clock"
      class="widget widget-clock"
      :style="widgetStyle('clock')"
      @mousedown="startDrag($event, 'clock')"
    >
      <div class="clock-time">{{ clockTime }}</div>
      <div class="clock-date">{{ weekDay }}, {{ dateStr }}</div>
    </div>

    <!-- System Widget -->
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
        <span>{{ m.load[0]?.toFixed(1) }} load</span>
      </div>
    </div>

    <!-- Temperature Widget -->
    <div
      v-if="enabledWidgets.temps && (m.tempCPU > 0 || m.tempGPU > 0)"
      class="widget widget-temps"
      :style="widgetStyle('temps')"
      @mousedown="startDrag($event, 'temps')"
    >
      <div class="widget-header">
        <q-icon name="sym_r_thermostat" size="14px" />
        <span>Temperature</span>
      </div>
      <div class="temp-grid">
        <div class="temp-item" v-if="m.tempCPU > 0">
          <span class="temp-val" :style="{ color: tempColor(m.tempCPU) }">{{ Math.round(m.tempCPU) }}</span>
          <span class="temp-unit">CPU</span>
        </div>
        <div class="temp-item" v-if="m.tempGPU > 0">
          <span class="temp-val" :style="{ color: tempColor(m.tempGPU) }">{{ Math.round(m.tempGPU) }}</span>
          <span class="temp-unit">GPU</span>
        </div>
        <div class="temp-item" v-if="m.tempNVMe > 0">
          <span class="temp-val" :style="{ color: tempColor(m.tempNVMe) }">{{ Math.round(m.tempNVMe) }}</span>
          <span class="temp-unit">NVMe</span>
        </div>
      </div>
    </div>

    <!-- Power Widget -->
    <div
      v-if="enabledWidgets.power && m.powerTotal > 0"
      class="widget widget-power"
      :style="widgetStyle('power')"
      @mousedown="startDrag($event, 'power')"
    >
      <div class="widget-header">
        <q-icon name="sym_r_bolt" size="14px" />
        <span>Power</span>
      </div>
      <div class="power-total">{{ m.powerTotal.toFixed(0) }}<span class="power-unit">W</span></div>
      <div class="power-breakdown">
        <span v-if="m.powerCPU > 0">CPU {{ m.powerCPU.toFixed(0) }}W</span>
        <span v-if="m.powerGPU > 0">GPU {{ m.powerGPU.toFixed(0) }}W</span>
      </div>
    </div>

    <!-- Network Widget -->
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
  </div>

  <!-- Toggle button when hidden -->
  <div v-if="!visible" class="widget-toggle widget-toggle-hidden" @click="visible = true">
    <q-icon name="sym_r_widgets" size="18px" />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue';
import { useMonitorStore } from 'stores/monitor';

const m = useMonitorStore();
const visible = ref(true);

// Widget positions (saved to localStorage)
const defaultPositions: Record<string, { x: number; y: number }> = {
  clock:   { x: -220, y: 30 },
  system:  { x: -220, y: 120 },
  temps:   { x: -220, y: 280 },
  power:   { x: -220, y: 400 },
  network: { x: -220, y: 490 },
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

function loadEnabled(): Record<string, boolean> {
  try {
    const saved = localStorage.getItem('packalares_widget_enabled');
    if (saved) return JSON.parse(saved);
  } catch {}
  return { clock: true, system: true, temps: true, power: true, network: true };
}

function widgetStyle(id: string) {
  const p = positions.value[id] || defaultPositions[id];
  // Negative x = offset from right
  if (p.x < 0) {
    return { position: 'absolute', right: Math.abs(p.x) + 'px', top: p.y + 'px' };
  }
  return { position: 'absolute', left: p.x + 'px', top: p.y + 'px' };
}

// Drag handling
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
  const x = e.clientX - dragOffset.x;
  const y = e.clientY - dragOffset.y;
  positions.value[dragWidget] = { x, y };
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

// Computed metrics
const cpuPct = computed(() => Math.round(m.cpuUsage || 0));
const memPct = computed(() => m.memTotal ? Math.round((m.memUsed / m.memTotal) * 100) : 0);
const diskPct = computed(() => m.diskTotal ? Math.round((m.diskUsed / m.diskTotal) * 100) : 0);

// Network sparkline history
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
  const m = Math.floor((seconds % 3600) / 60);
  return `${h}h ${m}m`;
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
  z-index: 50;
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
  min-width: 160px;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.3), inset 0 0.5px 0 rgba(255, 255, 255, 0.1);
  transition: box-shadow 0.2s;

  &:hover {
    box-shadow: 0 12px 40px rgba(0, 0, 0, 0.4), inset 0 0.5px 0 rgba(255, 255, 255, 0.15);
  }
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
.widget-clock {
  padding: 18px 22px;
  text-align: center;
}
.clock-time {
  font-size: 48px;
  font-weight: 200;
  letter-spacing: -2px;
  line-height: 1;
  text-shadow: 0 2px 10px rgba(0, 0, 0, 0.3);
}
.clock-date {
  font-size: 13px;
  color: rgba(255, 255, 255, 0.6);
  margin-top: 4px;
}

// ─── System Gauges ───
.gauge-row {
  display: flex;
  gap: 14px;
  justify-content: center;
}
.gauge-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  width: 48px;
}
.gauge-svg {
  width: 44px;
  height: 44px;
  transform: rotate(-90deg);
}
.gauge-bg {
  fill: none;
  stroke: rgba(255, 255, 255, 0.08);
  stroke-width: 3;
}
.gauge-fill {
  fill: none;
  stroke-width: 3;
  stroke-linecap: round;
  transition: stroke-dasharray 0.6s ease;
}
.gauge-label {
  text-align: center;
  margin-top: -30px;
  position: relative;
}
.gauge-val {
  font-size: 13px;
  font-weight: 600;
  display: block;
  line-height: 1;
}
.gauge-unit {
  font-size: 8px;
  text-transform: uppercase;
  color: rgba(255, 255, 255, 0.4);
  letter-spacing: 0.05em;
}

// ─── Temperature ───
.temp-grid {
  display: flex;
  gap: 16px;
  justify-content: center;
}
.temp-item {
  text-align: center;
}
.temp-val {
  font-size: 28px;
  font-weight: 300;
  line-height: 1;
  &::after { content: '\00B0'; font-size: 16px; }
}
.temp-unit {
  display: block;
  font-size: 9px;
  text-transform: uppercase;
  color: rgba(255, 255, 255, 0.4);
  margin-top: 2px;
}

// ─── Power ───
.power-total {
  font-size: 36px;
  font-weight: 300;
  text-align: center;
  line-height: 1;
}
.power-unit {
  font-size: 16px;
  color: rgba(255, 255, 255, 0.5);
}
.power-breakdown {
  display: flex;
  justify-content: center;
  gap: 12px;
  font-size: 10px;
  color: rgba(255, 255, 255, 0.4);
  margin-top: 4px;
}

// ─── Network ───
.net-row {
  display: flex;
  justify-content: space-between;
  margin-bottom: 6px;
}
.net-item {
  display: flex;
  align-items: center;
  gap: 4px;
}
.net-icon-down { color: #22c55e; }
.net-icon-up { color: #3b82f6; }
.net-val { font-size: 13px; font-weight: 500; }
.net-sparkline {
  height: 30px;
  opacity: 0.7;
}
.sparkline-svg { width: 100%; height: 100%; }
.spark-rx { fill: none; stroke: #22c55e; stroke-width: 1.5; }
.spark-tx { fill: none; stroke: #3b82f6; stroke-width: 1.5; }

// ─── Toggle ───
.widget-toggle, .widget-toggle-hidden {
  pointer-events: auto;
  position: absolute;
  top: 12px;
  right: 12px;
  z-index: 51;
  width: 32px;
  height: 32px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  color: rgba(255, 255, 255, 0.6);
  background: rgba(0, 0, 0, 0.3);
  backdrop-filter: blur(12px);
  border: 1px solid rgba(255, 255, 255, 0.08);
  transition: all 0.2s;

  &:hover {
    background: rgba(0, 0, 0, 0.5);
    color: #fff;
  }
}
</style>
