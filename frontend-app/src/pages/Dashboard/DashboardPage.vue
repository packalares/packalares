<template>
  <div class="dash-root">
    <!-- Left Sidebar -->
    <div class="dash-sidebar">
      <div class="sidebar-header">
        <q-icon name="sym_r_dashboard" size="28px" color="white" />
        <span class="sidebar-title">Dashboard</span>
      </div>

      <q-list dense class="sidebar-nav">
        <q-item
          v-for="item in navItems"
          :key="item.key"
          clickable
          :active="activeNav === item.key"
          active-class="sidebar-item-active"
          class="sidebar-nav-item"
          @click="activeNav = item.key"
        >
          <q-item-section avatar style="min-width: 36px">
            <q-icon
              :name="item.icon"
              size="20px"
              :class="{ 'text-accent-active': activeNav === item.key }"
            />
          </q-item-section>
          <q-item-section>
            <q-item-label
              :class="activeNav === item.key ? 'text-accent-active' : 'text-ink-1'"
            >
              {{ item.label }}
            </q-item-label>
          </q-item-section>
        </q-item>
      </q-list>
    </div>

    <!-- Main Content -->
    <div class="dash-content">
      <!-- Overview -->
      <template v-if="activeNav === 'overview'">
        <div class="dash-page-title">Overview</div>

        <!-- Resource Cards -->
        <div class="resource-cards">
          <div class="resource-card card">
            <div class="resource-card-header">
              <q-icon name="sym_r_memory" size="22px" class="resource-icon resource-icon-cpu" />
              <span class="resource-label">CPU</span>
            </div>
            <div class="resource-value">{{ metrics.cpu_usage?.toFixed(1) ?? '--' }}%</div>
            <q-linear-progress
              :value="(metrics.cpu_usage ?? 0) / 100"
              class="resource-bar"
              :color="progressColor(metrics.cpu_usage ?? 0)"
              track-color="grey-9"
              rounded
              size="6px"
            />
          </div>

          <div class="resource-card card">
            <div class="resource-card-header">
              <q-icon name="sym_r_speed" size="22px" class="resource-icon resource-icon-mem" />
              <span class="resource-label">Memory</span>
            </div>
            <div class="resource-value">
              {{ formatBytes(metrics.memory?.used) }} / {{ formatBytes(metrics.memory?.total) }}
            </div>
            <q-linear-progress
              :value="memPercent / 100"
              class="resource-bar"
              :color="progressColor(memPercent)"
              track-color="grey-9"
              rounded
              size="6px"
            />
          </div>

          <div class="resource-card card">
            <div class="resource-card-header">
              <q-icon name="sym_r_hard_drive" size="22px" class="resource-icon resource-icon-disk" />
              <span class="resource-label">Disk</span>
            </div>
            <div class="resource-value">
              {{ formatBytes(metrics.disk?.used) }} / {{ formatBytes(metrics.disk?.total) }}
            </div>
            <q-linear-progress
              :value="diskPercent / 100"
              class="resource-bar"
              :color="progressColor(diskPercent)"
              track-color="grey-9"
              rounded
              size="6px"
            />
          </div>

          <div class="resource-card card">
            <div class="resource-card-header">
              <q-icon name="sym_r_deployed_code" size="22px" class="resource-icon resource-icon-pods" />
              <span class="resource-label">Pods</span>
            </div>
            <div class="resource-value">{{ pods.length }} running</div>
            <q-linear-progress
              :value="pods.length > 0 ? 1 : 0"
              class="resource-bar"
              color="positive"
              track-color="grey-9"
              rounded
              size="6px"
            />
          </div>
        </div>

        <!-- Uptime & Load -->
        <div class="info-row">
          <div class="info-chip card">
            <q-icon name="sym_r_schedule" size="16px" style="color: var(--ink-3)" />
            <span class="info-chip-label">Uptime</span>
            <span class="info-chip-value">{{ formatUptime(metrics.uptime) }}</span>
          </div>
          <div class="info-chip card" v-if="metrics.load">
            <q-icon name="sym_r_trending_up" size="16px" style="color: var(--ink-3)" />
            <span class="info-chip-label">Load</span>
            <span class="info-chip-value">{{ metrics.load }}</span>
          </div>
        </div>

        <!-- Charts Row -->
        <div class="charts-row">
          <div class="chart-card card">
            <div class="chart-title">CPU Usage %</div>
            <canvas ref="cpuCanvas" class="chart-canvas"></canvas>
          </div>
          <div class="chart-card card">
            <div class="chart-title">Memory Usage %</div>
            <canvas ref="memCanvas" class="chart-canvas"></canvas>
          </div>
        </div>

        <!-- GPU Section -->
        <div v-if="gpuData.driver_installed && gpuData.gpus.length > 0" class="gpu-section">
          <div class="section-title">GPU</div>
          <div class="gpu-cards">
            <div v-for="(gpu, i) in gpuData.gpus" :key="i" class="gpu-card card">
              <div class="gpu-name">{{ gpu.name || `GPU ${i}` }}</div>
              <div class="gpu-details">
                <span v-if="gpu.memory_total">
                  {{ formatBytes(gpu.memory_used) }} / {{ formatBytes(gpu.memory_total) }}
                </span>
                <span v-if="gpu.utilization !== undefined">
                  {{ gpu.utilization }}% util
                </span>
                <span v-if="gpu.temperature !== undefined">
                  {{ gpu.temperature }}C
                </span>
              </div>
            </div>
          </div>
        </div>

        <!-- Pod Table -->
        <div class="section-title">Pods</div>
        <div class="pod-table-wrap card">
          <q-table
            flat
            dense
            :rows="pods"
            :columns="podColumns"
            row-key="name"
            :rows-per-page-options="[20, 50, 0]"
            class="pod-table"
            dark
            :pagination="{ rowsPerPage: 20 }"
          >
            <template v-slot:body-cell-phase="props">
              <q-td :props="props">
                <q-badge
                  :label="props.row.phase"
                  :class="'pod-status-badge pod-status-' + (props.row.phase || '').toLowerCase()"
                />
              </q-td>
            </template>
          </q-table>
        </div>
      </template>

      <!-- Applications view -->
      <template v-if="activeNav === 'applications'">
        <div class="dash-page-title">Applications</div>
        <div class="pod-table-wrap card">
          <q-table
            flat
            dense
            :rows="appPods"
            :columns="podColumns"
            row-key="name"
            :rows-per-page-options="[20, 50, 0]"
            class="pod-table"
            dark
            :pagination="{ rowsPerPage: 20 }"
          >
            <template v-slot:body-cell-phase="props">
              <q-td :props="props">
                <q-badge
                  :label="props.row.phase"
                  :class="'pod-status-badge pod-status-' + (props.row.phase || '').toLowerCase()"
                />
              </q-td>
            </template>
          </q-table>
        </div>
      </template>

      <!-- Nodes view -->
      <template v-if="activeNav === 'nodes'">
        <div class="dash-page-title">Nodes</div>
        <div class="resource-cards">
          <div class="resource-card card">
            <div class="resource-card-header">
              <q-icon name="sym_r_dns" size="22px" class="resource-icon resource-icon-cpu" />
              <span class="resource-label">Node</span>
            </div>
            <div class="resource-value">1 node</div>
            <div class="node-info">
              <div class="node-info-row">
                <span class="node-info-label">CPU</span>
                <span class="node-info-value">{{ metrics.cpu_usage?.toFixed(1) ?? '--' }}%</span>
              </div>
              <div class="node-info-row">
                <span class="node-info-label">Memory</span>
                <span class="node-info-value">{{ memPercent.toFixed(1) }}%</span>
              </div>
              <div class="node-info-row">
                <span class="node-info-label">Disk</span>
                <span class="node-info-value">{{ diskPercent.toFixed(1) }}%</span>
              </div>
              <div class="node-info-row">
                <span class="node-info-label">Pods</span>
                <span class="node-info-value">{{ pods.length }}</span>
              </div>
            </div>
          </div>
        </div>
      </template>
    </div>
  </div>
</template>

<script lang="ts" setup>
import { ref, computed, onMounted, onUnmounted, nextTick, watch } from 'vue';
import { api } from 'boot/axios';

interface Metrics {
  cpu_usage?: number;
  memory?: { used: number; total: number };
  disk?: { used: number; total: number };
  uptime?: number;
  load?: string;
}

interface Pod {
  name: string;
  namespace: string;
  phase: string;
  ready: string;
  restarts: number;
  age: string;
}

interface GpuInfo {
  name?: string;
  memory_used?: number;
  memory_total?: number;
  utilization?: number;
  temperature?: number;
}

interface GpuData {
  gpus: GpuInfo[];
  gpu_count: number;
  driver_installed: boolean;
}

const activeNav = ref('overview');

const navItems = [
  { key: 'overview', label: 'Overview', icon: 'sym_r_dashboard' },
  { key: 'applications', label: 'Applications', icon: 'sym_r_wysiwyg' },
  { key: 'nodes', label: 'Nodes', icon: 'sym_r_dns' },
];

const metrics = ref<Metrics>({});
const pods = ref<Pod[]>([]);
const gpuData = ref<GpuData>({ gpus: [], gpu_count: 0, driver_installed: false });

const cpuCanvas = ref<HTMLCanvasElement | null>(null);
const memCanvas = ref<HTMLCanvasElement | null>(null);

const cpuHistory = ref<number[]>([]);
const memHistory = ref<number[]>([]);
const MAX_POINTS = 60;

let pollTimer: ReturnType<typeof setInterval> | null = null;
let animFrameId: number | null = null;

const memPercent = computed(() => {
  const m = metrics.value.memory;
  if (!m || !m.total) return 0;
  return (m.used / m.total) * 100;
});

const diskPercent = computed(() => {
  const d = metrics.value.disk;
  if (!d || !d.total) return 0;
  return (d.used / d.total) * 100;
});

const appPods = computed(() =>
  pods.value.filter(
    (p) => !p.namespace.startsWith('kube-') && p.namespace !== 'default'
  )
);

const podColumns = [
  { name: 'name', label: 'Name', field: 'name', align: 'left' as const, sortable: true },
  { name: 'namespace', label: 'Namespace', field: 'namespace', align: 'left' as const, sortable: true },
  { name: 'phase', label: 'Status', field: 'phase', align: 'left' as const, sortable: true },
  { name: 'ready', label: 'Ready', field: 'ready', align: 'center' as const },
  { name: 'restarts', label: 'Restarts', field: 'restarts', align: 'center' as const, sortable: true },
  { name: 'age', label: 'Age', field: 'age', align: 'left' as const },
];

function progressColor(val: number): string {
  if (val > 90) return 'negative';
  if (val > 70) return 'warning';
  return 'positive';
}

function formatBytes(bytes?: number): string {
  if (bytes == null) return '--';
  if (bytes === 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  const val = bytes / Math.pow(1024, i);
  return val.toFixed(1) + ' ' + units[i];
}

function formatUptime(seconds?: number): string {
  if (seconds == null) return '--';
  const d = Math.floor(seconds / 86400);
  const h = Math.floor((seconds % 86400) / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  if (d > 0) return `${d}d ${h}h ${m}m`;
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}

async function fetchMetrics() {
  try {
    const res: any = await api.get('/api/metrics');
    metrics.value = res || {};

    const cpu = res.cpu_usage ?? 0;
    cpuHistory.value.push(cpu);
    if (cpuHistory.value.length > MAX_POINTS) cpuHistory.value.shift();

    const mem =
      res.memory && res.memory.total
        ? (res.memory.used / res.memory.total) * 100
        : 0;
    memHistory.value.push(mem);
    if (memHistory.value.length > MAX_POINTS) memHistory.value.shift();
  } catch {
    /* silent */
  }
}

async function fetchPods() {
  try {
    const res: any = await api.get('/api/status');
    pods.value = res?.data?.pods || res?.pods || [];
  } catch {
    pods.value = [];
  }
}

async function fetchGpu() {
  try {
    const res: any = await api.get('/api/gpu/list');
    gpuData.value = res || { gpus: [], gpu_count: 0, driver_installed: false };
  } catch {
    gpuData.value = { gpus: [], gpu_count: 0, driver_installed: false };
  }
}

function drawChart(
  canvas: HTMLCanvasElement | null,
  data: number[],
  color: string,
  gradientStart: string,
  gradientEnd: string
) {
  if (!canvas) return;
  const ctx = canvas.getContext('2d');
  if (!ctx) return;

  const dpr = window.devicePixelRatio || 1;
  const rect = canvas.getBoundingClientRect();
  canvas.width = rect.width * dpr;
  canvas.height = rect.height * dpr;
  ctx.scale(dpr, dpr);

  const w = rect.width;
  const h = rect.height;
  const pad = { top: 10, right: 10, bottom: 24, left: 36 };
  const chartW = w - pad.left - pad.right;
  const chartH = h - pad.top - pad.bottom;

  ctx.clearRect(0, 0, w, h);

  // Y axis labels & grid
  ctx.font = '10px Inter, sans-serif';
  ctx.textAlign = 'right';
  ctx.textBaseline = 'middle';
  for (let i = 0; i <= 4; i++) {
    const val = i * 25;
    const y = pad.top + chartH - (chartH * val) / 100;
    ctx.fillStyle = '#707070';
    ctx.fillText(val + '%', pad.left - 6, y);

    ctx.strokeStyle = 'rgba(255,255,255,0.04)';
    ctx.lineWidth = 1;
    ctx.beginPath();
    ctx.moveTo(pad.left, y);
    ctx.lineTo(w - pad.right, y);
    ctx.stroke();
  }

  // X axis labels
  ctx.textAlign = 'center';
  ctx.textBaseline = 'top';
  ctx.fillStyle = '#707070';
  const points = data.length || 1;
  const step = chartW / (MAX_POINTS - 1);

  // Label every ~15 points
  for (let i = 0; i < MAX_POINTS; i += 15) {
    const x = pad.left + i * step;
    const secondsAgo = (MAX_POINTS - 1 - i) * 5;
    ctx.fillText(secondsAgo === 0 ? 'now' : `-${secondsAgo}s`, x, h - pad.bottom + 6);
  }

  if (data.length < 2) return;

  // Build points
  const pts: { x: number; y: number }[] = [];
  const offset = MAX_POINTS - data.length;
  for (let i = 0; i < data.length; i++) {
    const x = pad.left + (offset + i) * step;
    const y = pad.top + chartH - (chartH * Math.min(data[i], 100)) / 100;
    pts.push({ x, y });
  }

  // Gradient fill
  const grad = ctx.createLinearGradient(0, pad.top, 0, pad.top + chartH);
  grad.addColorStop(0, gradientStart);
  grad.addColorStop(1, gradientEnd);

  ctx.beginPath();
  ctx.moveTo(pts[0].x, pad.top + chartH);
  ctx.lineTo(pts[0].x, pts[0].y);

  // Bezier curve through points
  for (let i = 1; i < pts.length; i++) {
    const prev = pts[i - 1];
    const cur = pts[i];
    const cpx = (prev.x + cur.x) / 2;
    ctx.bezierCurveTo(cpx, prev.y, cpx, cur.y, cur.x, cur.y);
  }

  ctx.lineTo(pts[pts.length - 1].x, pad.top + chartH);
  ctx.closePath();
  ctx.fillStyle = grad;
  ctx.fill();

  // Line
  ctx.beginPath();
  ctx.moveTo(pts[0].x, pts[0].y);
  for (let i = 1; i < pts.length; i++) {
    const prev = pts[i - 1];
    const cur = pts[i];
    const cpx = (prev.x + cur.x) / 2;
    ctx.bezierCurveTo(cpx, prev.y, cpx, cur.y, cur.x, cur.y);
  }
  ctx.strokeStyle = color;
  ctx.lineWidth = 2;
  ctx.stroke();

  // Latest value dot
  const last = pts[pts.length - 1];
  ctx.beginPath();
  ctx.arc(last.x, last.y, 3, 0, Math.PI * 2);
  ctx.fillStyle = color;
  ctx.fill();
}

function renderCharts() {
  drawChart(
    cpuCanvas.value,
    cpuHistory.value,
    '#4c9fe7',
    'rgba(76, 159, 231, 0.25)',
    'rgba(76, 159, 231, 0.0)'
  );
  drawChart(
    memCanvas.value,
    memHistory.value,
    '#29cc5f',
    'rgba(41, 204, 95, 0.25)',
    'rgba(41, 204, 95, 0.0)'
  );
  animFrameId = requestAnimationFrame(renderCharts);
}

async function poll() {
  await Promise.all([fetchMetrics(), fetchPods()]);
}

watch(activeNav, () => {
  if (activeNav.value === 'overview') {
    nextTick(() => {
      if (!animFrameId) {
        animFrameId = requestAnimationFrame(renderCharts);
      }
    });
  } else {
    if (animFrameId) {
      cancelAnimationFrame(animFrameId);
      animFrameId = null;
    }
  }
});

onMounted(async () => {
  await Promise.all([poll(), fetchGpu()]);
  pollTimer = setInterval(poll, 5000);
  nextTick(() => {
    animFrameId = requestAnimationFrame(renderCharts);
  });
});

onUnmounted(() => {
  if (pollTimer) clearInterval(pollTimer);
  if (animFrameId) cancelAnimationFrame(animFrameId);
});
</script>

<style lang="scss" scoped>
.dash-root {
  display: flex;
  width: 100%;
  height: 100vh;
  background-color: var(--bg-1);
  overflow: hidden;
}

/* ── Sidebar ── */
.dash-sidebar {
  width: 240px;
  min-width: 240px;
  height: 100%;
  background-color: var(--bg-1);
  border-right: 1px solid var(--separator);
  display: flex;
  flex-direction: column;
  padding: 12px 8px;
}

.sidebar-header {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 16px 12px 20px;
}

.sidebar-title {
  font-size: 18px;
  font-weight: 600;
  color: var(--ink-1);
}

.sidebar-nav-item {
  border-radius: 8px;
  min-height: 40px;
  margin-bottom: 2px;
  color: var(--ink-1);

  .q-item__section--avatar {
    padding-right: 0;
  }
}

.sidebar-item-active {
  background-color: var(--accent-soft) !important;
}

.text-accent-active {
  color: var(--accent) !important;
}

.text-ink-1 {
  color: var(--ink-1);
}

/* ── Main Content ── */
.dash-content {
  flex: 1;
  height: 100%;
  overflow-y: auto;
  padding: 24px 32px 40px;
}

.dash-page-title {
  font-size: 22px;
  font-weight: 600;
  color: var(--ink-1);
  margin-bottom: 24px;
}

/* ── Resource Cards ── */
.resource-cards {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 16px;
  margin-bottom: 16px;
}

.resource-card {
  padding: 16px;
}

.resource-card-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}

.resource-icon {
  border-radius: 6px;
  padding: 2px;
}

.resource-icon-cpu {
  color: var(--accent);
}

.resource-icon-mem {
  color: var(--positive);
}

.resource-icon-disk {
  color: var(--warning);
}

.resource-icon-pods {
  color: #b07fe0;
}

.resource-label {
  font-size: 13px;
  font-weight: 500;
  color: var(--ink-2);
}

.resource-value {
  font-size: 18px;
  font-weight: 600;
  color: var(--ink-1);
  margin-bottom: 10px;
}

.resource-bar {
  border-radius: 3px;
}

/* ── Info Row ── */
.info-row {
  display: flex;
  gap: 12px;
  margin-bottom: 24px;
}

.info-chip {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 14px;
  font-size: 13px;
}

.info-chip-label {
  color: var(--ink-3);
}

.info-chip-value {
  color: var(--ink-1);
  font-weight: 500;
}

/* ── Charts ── */
.charts-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
  margin-bottom: 24px;
}

.chart-card {
  padding: 16px;
}

.chart-title {
  font-size: 13px;
  font-weight: 500;
  color: var(--ink-2);
  margin-bottom: 8px;
}

.chart-canvas {
  width: 100%;
  height: 180px;
  display: block;
}

/* ── Section Title ── */
.section-title {
  font-size: 16px;
  font-weight: 600;
  color: var(--ink-1);
  margin-bottom: 12px;
}

/* ── GPU ── */
.gpu-section {
  margin-bottom: 24px;
}

.gpu-cards {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 12px;
}

.gpu-card {
  padding: 14px;
}

.gpu-name {
  font-size: 14px;
  font-weight: 500;
  color: var(--ink-1);
  margin-bottom: 6px;
}

.gpu-details {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  font-size: 12px;
  color: var(--ink-3);
}

/* ── Pod Table ── */
.pod-table-wrap {
  margin-bottom: 24px;
  overflow: hidden;
}

.pod-table {
  background: transparent !important;

  :deep(thead th) {
    font-size: 12px;
    font-weight: 600;
    color: var(--ink-3) !important;
    background: var(--bg-3) !important;
    border-bottom: 1px solid var(--separator);
  }

  :deep(tbody td) {
    font-size: 13px;
    color: var(--ink-1) !important;
    border-bottom: 1px solid var(--separator);
  }

  :deep(.q-table__bottom) {
    color: var(--ink-3) !important;
    border-top: 1px solid var(--separator);
  }

  :deep(.q-field__native),
  :deep(.q-field__control) {
    color: var(--ink-1) !important;
  }
}

.pod-status-badge {
  font-size: 11px;
  padding: 2px 8px;
  border-radius: 4px;
  text-transform: capitalize;
}

.pod-status-running {
  background: rgba(41, 204, 95, 0.15) !important;
  color: var(--positive) !important;
}

.pod-status-succeeded {
  background: rgba(41, 204, 95, 0.15) !important;
  color: var(--positive) !important;
}

.pod-status-pending {
  background: rgba(254, 190, 1, 0.15) !important;
  color: var(--warning) !important;
}

.pod-status-failed {
  background: rgba(255, 77, 77, 0.15) !important;
  color: var(--negative) !important;
}

.pod-status-unknown {
  background: rgba(255, 255, 255, 0.06) !important;
  color: var(--ink-3) !important;
}

/* ── Node Info ── */
.node-info {
  margin-top: 12px;
}

.node-info-row {
  display: flex;
  justify-content: space-between;
  padding: 4px 0;
  font-size: 13px;
}

.node-info-label {
  color: var(--ink-3);
}

.node-info-value {
  color: var(--ink-1);
  font-weight: 500;
}

/* ── Scrollbar ── */
.dash-content::-webkit-scrollbar {
  width: 6px;
}

.dash-content::-webkit-scrollbar-track {
  background: transparent;
}

.dash-content::-webkit-scrollbar-thumb {
  background: var(--bg-3);
  border-radius: 3px;
}
</style>
