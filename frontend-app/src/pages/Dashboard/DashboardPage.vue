<template>
  <div class="dash-root">
    <!-- Left Sidebar -->
    <div class="dash-sidebar">
      <div class="sidebar-header">
        <q-icon name="sym_r_monitoring" size="26px" class="header-icon" />
        <span class="sidebar-title">System</span>
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
          <q-item-section avatar style="min-width: 32px">
            <q-icon :name="item.icon" size="18px" />
          </q-item-section>
          <q-item-section>
            <q-item-label class="nav-label">{{ item.label }}</q-item-label>
          </q-item-section>
          <q-item-section side v-if="item.badge">
            <span class="nav-badge">{{ item.badge }}</span>
          </q-item-section>
        </q-item>
      </q-list>
      <div class="sidebar-footer">
        <div class="sys-uptime">
          <span class="uptime-dot"></span>
          <span class="uptime-text">{{ formatUptime(metrics.uptime) }}</span>
        </div>
      </div>
    </div>

    <!-- Main Content -->
    <div class="dash-content">

      <!-- ═══════ OVERVIEW ═══════ -->
      <template v-if="activeNav === 'overview'">
        <!-- Top Resource Strip -->
        <div class="resource-strip">
          <div class="strip-card" v-for="r in resourceCards" :key="r.label">
            <div class="strip-label">{{ r.label }}</div>
            <div class="strip-value" :style="{ color: r.color }">{{ r.value }}</div>
            <div class="strip-bar-track">
              <div class="strip-bar-fill" :style="{ width: r.percent + '%', background: r.color }"></div>
            </div>
          </div>
        </div>

        <!-- Charts Grid -->
        <div class="charts-grid">
          <div class="chart-panel">
            <div class="chart-head">
              <span class="chart-title">CPU</span>
              <span class="chart-live" :style="{ color: '#4c9fe7' }">{{ metrics.cpu_usage?.toFixed(1) ?? '0' }}%</span>
            </div>
            <canvas ref="cpuCanvas" class="chart-canvas"></canvas>
          </div>
          <div class="chart-panel">
            <div class="chart-head">
              <span class="chart-title">Memory</span>
              <span class="chart-live" :style="{ color: '#29cc5f' }">{{ memPercent.toFixed(1) }}%</span>
            </div>
            <canvas ref="memCanvas" class="chart-canvas"></canvas>
          </div>
          <div class="chart-panel">
            <div class="chart-head">
              <span class="chart-title">Network</span>
              <span class="chart-live" :style="{ color: '#e0a040' }">
                <span style="font-size:10px">RX</span> {{ formatRate(metrics.network?.rx_bytes_per_sec) }}
                <span style="font-size:10px;margin-left:6px">TX</span> {{ formatRate(metrics.network?.tx_bytes_per_sec) }}
              </span>
            </div>
            <canvas ref="netCanvas" class="chart-canvas"></canvas>
          </div>
          <div class="chart-panel">
            <div class="chart-head">
              <span class="chart-title">Disk I/O</span>
              <span class="chart-live" :style="{ color: '#c07ae0' }">
                <span style="font-size:10px">R</span> {{ formatRate(metrics.disk_io?.read_bytes_per_sec) }}
                <span style="font-size:10px;margin-left:6px">W</span> {{ formatRate(metrics.disk_io?.write_bytes_per_sec) }}
              </span>
            </div>
            <canvas ref="diskIOCanvas" class="chart-canvas"></canvas>
          </div>
        </div>

        <!-- Per-Core CPU + System Info -->
        <div class="bottom-row">
          <div class="cores-panel">
            <div class="panel-head">CPU Cores</div>
            <div class="cores-list">
              <div class="core-row" v-for="(val, i) in (metrics.cpu_cores || [])" :key="i">
                <span class="core-id">{{ i }}</span>
                <div class="core-bar-track">
                  <div class="core-bar-fill" :style="{ width: val.toFixed(0) + '%' }"></div>
                </div>
                <span class="core-val">{{ val.toFixed(0) }}%</span>
              </div>
            </div>
          </div>
          <div class="info-panel">
            <div class="panel-head">System</div>
            <div class="info-grid">
              <div class="info-item">
                <span class="info-label">Load</span>
                <span class="info-val" v-if="metrics.load">{{ fmtLoad(metrics.load) }}</span>
              </div>
              <div class="info-item">
                <span class="info-label">Pods</span>
                <span class="info-val">{{ pods.length }}</span>
              </div>
              <div class="info-item">
                <span class="info-label">Power</span>
                <span class="info-val">{{ metrics.power?.total_watts?.toFixed(1) ?? '--' }} W</span>
              </div>
              <div class="info-item">
                <span class="info-label">Net RX</span>
                <span class="info-val">{{ formatRate(metrics.network?.rx_bytes_per_sec) }}</span>
              </div>
              <div class="info-item">
                <span class="info-label">Net TX</span>
                <span class="info-val">{{ formatRate(metrics.network?.tx_bytes_per_sec) }}</span>
              </div>
              <div class="info-item">
                <span class="info-label">Disk R</span>
                <span class="info-val">{{ formatRate(metrics.disk_io?.read_bytes_per_sec) }}</span>
              </div>
              <div class="info-item">
                <span class="info-label">Disk W</span>
                <span class="info-val">{{ formatRate(metrics.disk_io?.write_bytes_per_sec) }}</span>
              </div>
              <div class="info-item" v-if="gpuData.driver_installed">
                <span class="info-label">GPU</span>
                <span class="info-val">{{ gpuData.gpus[0]?.utilization ?? 0 }}%</span>
              </div>
            </div>
          </div>
        </div>
      </template>

      <!-- ═══════ NETWORK ═══════ -->
      <template v-if="activeNav === 'network'">
        <div class="page-title">Network</div>
        <div class="stat-row">
          <div class="stat-box"><span class="stat-label">Download</span><span class="stat-val rx">{{ formatRate(metrics.network?.rx_bytes_per_sec) }}</span></div>
          <div class="stat-box"><span class="stat-label">Upload</span><span class="stat-val tx">{{ formatRate(metrics.network?.tx_bytes_per_sec) }}</span></div>
        </div>
        <div class="full-chart-panel">
          <canvas ref="netFullCanvas" class="chart-canvas-full"></canvas>
        </div>
      </template>

      <!-- ═══════ STORAGE ═══════ -->
      <template v-if="activeNav === 'storage'">
        <div class="page-title">Storage</div>
        <div class="stat-row">
          <div class="stat-box"><span class="stat-label">Used</span><span class="stat-val">{{ formatBytes(metrics.disk?.used) }}</span></div>
          <div class="stat-box"><span class="stat-label">Total</span><span class="stat-val">{{ formatBytes(metrics.disk?.total) }}</span></div>
          <div class="stat-box"><span class="stat-label">Usage</span><span class="stat-val">{{ diskPercent.toFixed(1) }}%</span></div>
        </div>
        <div class="disk-usage-bar">
          <div class="disk-used" :style="{ width: diskPercent + '%' }"></div>
        </div>
        <div class="page-subtitle">Disk I/O</div>
        <div class="stat-row">
          <div class="stat-box"><span class="stat-label">Read</span><span class="stat-val rx">{{ formatRate(metrics.disk_io?.read_bytes_per_sec) }}</span></div>
          <div class="stat-box"><span class="stat-label">Write</span><span class="stat-val tx">{{ formatRate(metrics.disk_io?.write_bytes_per_sec) }}</span></div>
        </div>
        <div class="full-chart-panel">
          <canvas ref="diskIOFullCanvas" class="chart-canvas-full"></canvas>
        </div>
      </template>

      <!-- ═══════ POWER ═══════ -->
      <template v-if="activeNav === 'power'">
        <div class="page-title">Power Consumption</div>
        <div class="stat-row">
          <div class="stat-box"><span class="stat-label">CPU</span><span class="stat-val">{{ metrics.power?.cpu_watts?.toFixed(1) ?? '--' }} W</span></div>
          <div class="stat-box"><span class="stat-label">GPU</span><span class="stat-val">{{ metrics.power?.gpu_watts?.toFixed(1) ?? '--' }} W</span></div>
          <div class="stat-box"><span class="stat-label">Total</span><span class="stat-val power-total">{{ metrics.power?.total_watts?.toFixed(1) ?? '--' }} W</span></div>
        </div>
        <div class="full-chart-panel">
          <canvas ref="powerCanvas" class="chart-canvas-full"></canvas>
        </div>
      </template>

      <!-- ═══════ LOGS ═══════ -->
      <template v-if="activeNav === 'logs'">
        <div class="page-title">Logs</div>
        <div class="logs-controls">
          <q-input
            v-model="logQuery"
            dense dark outlined
            placeholder='{namespace="os-system"}'
            class="log-query-input"
            @keyup.enter="fetchLogs"
          >
            <template #prepend>
              <q-icon name="sym_r_search" size="18px" />
            </template>
          </q-input>
          <q-select
            v-model="logNamespace"
            dense dark outlined
            :options="logNamespaceOptions"
            label="Namespace"
            class="log-ns-select"
            @update:model-value="onNamespaceChange"
          />
          <q-btn flat dense label="Fetch" class="log-fetch-btn" @click="fetchLogs" />
        </div>
        <div class="logs-output" ref="logsContainer">
          <div v-if="logEntries.length === 0" class="logs-empty">No logs. Enter a LogQL query and click Fetch.</div>
          <div v-for="(entry, i) in logEntries" :key="i" class="log-line">
            <span class="log-ts">{{ entry.ts }}</span>
            <span class="log-ns">{{ entry.ns }}</span>
            <span class="log-msg">{{ entry.msg }}</span>
          </div>
        </div>
      </template>

      <!-- ═══════ PODS ═══════ -->
      <template v-if="activeNav === 'pods'">
        <div class="page-title">Pods ({{ pods.length }})</div>
        <div class="pod-table-wrap">
          <q-table
            flat dense
            :rows="pods"
            :columns="podColumns"
            row-key="name"
            :rows-per-page-options="[25, 50, 0]"
            class="pod-table"
            dark
            :pagination="{ rowsPerPage: 25 }"
          >
            <template v-slot:body-cell-status="props">
              <q-td :props="props">
                <span class="pod-dot" :class="'dot-' + (props.row.status || '').toLowerCase()"></span>
                {{ props.row.status }}
              </q-td>
            </template>
          </q-table>
        </div>
      </template>
    </div>
  </div>
</template>

<script lang="ts" setup>
import { ref, computed, onMounted, onUnmounted, nextTick, watch } from 'vue';
import { api, getWsUrl } from 'boot/axios';

// ─── Types ───
interface Metrics {
  cpu_usage?: number;
  cpu_cores?: number[];
  memory?: { used: number; total: number };
  disk?: { used: number; total: number };
  disk_io?: { read_bytes_per_sec: number; write_bytes_per_sec: number };
  network?: { rx_bytes_per_sec: number; tx_bytes_per_sec: number };
  power?: { cpu_watts: number; gpu_watts: number; total_watts: number };
  uptime?: number;
  load?: number[];
}

interface Pod {
  name: string;
  namespace: string;
  status: string;
  ready: string;
  restarts: number;
  age: string;
  ip?: string;
  node?: string;
}

interface GpuData {
  gpus: { name?: string; utilization?: number; temperature?: number; vram_used_mb?: number; vram_total_mb?: number }[];
  gpu_count: number;
  driver_installed: boolean;
}

interface LogEntry { ts: string; ns: string; msg: string }

// ─── State ───
const activeNav = ref('overview');
const metrics = ref<Metrics>({});
const pods = ref<Pod[]>([]);
const gpuData = ref<GpuData>({ gpus: [], gpu_count: 0, driver_installed: false });

const navItems = [
  { key: 'overview', label: 'Overview', icon: 'sym_r_dashboard' },
  { key: 'network', label: 'Network', icon: 'sym_r_lan' },
  { key: 'storage', label: 'Storage', icon: 'sym_r_hard_drive' },
  { key: 'power', label: 'Power', icon: 'sym_r_bolt' },
  { key: 'logs', label: 'Logs', icon: 'sym_r_terminal' },
  { key: 'pods', label: 'Pods', icon: 'sym_r_deployed_code', badge: '' },
];

// Canvas refs
const cpuCanvas = ref<HTMLCanvasElement | null>(null);
const memCanvas = ref<HTMLCanvasElement | null>(null);
const netCanvas = ref<HTMLCanvasElement | null>(null);
const diskIOCanvas = ref<HTMLCanvasElement | null>(null);
const netFullCanvas = ref<HTMLCanvasElement | null>(null);
const diskIOFullCanvas = ref<HTMLCanvasElement | null>(null);
const powerCanvas = ref<HTMLCanvasElement | null>(null);

// History arrays
const MAX_POINTS = 60;
const cpuHistory = ref<number[]>([]);
const memHistory = ref<number[]>([]);
const netRxHistory = ref<number[]>([]);
const netTxHistory = ref<number[]>([]);
const diskRHistory = ref<number[]>([]);
const diskWHistory = ref<number[]>([]);
const powerHistory = ref<number[]>([]);

let ws: WebSocket | null = null;
let pollTimer: ReturnType<typeof setInterval> | null = null;
let animFrameId: number | null = null;

// Logs
const logQuery = ref('{namespace=~".+"}');
const logNamespace = ref('All');
const logNamespaceOptions = ['All', 'os-system', 'os-framework', 'monitoring', 'user-space-admin', 'kube-system'];
const logEntries = ref<LogEntry[]>([]);
const logsContainer = ref<HTMLElement | null>(null);

// ─── Computed ───
const memPercent = computed(() => {
  const m = metrics.value.memory;
  return m && m.total ? (m.used / m.total) * 100 : 0;
});

const diskPercent = computed(() => {
  const d = metrics.value.disk;
  return d && d.total ? (d.used / d.total) * 100 : 0;
});

const resourceCards = computed(() => [
  { label: 'CPU', value: (metrics.value.cpu_usage?.toFixed(1) ?? '0') + '%', percent: metrics.value.cpu_usage ?? 0, color: '#4c9fe7' },
  { label: 'MEM', value: formatBytes(metrics.value.memory?.used), percent: memPercent.value, color: '#29cc5f' },
  { label: 'DISK', value: diskPercent.value.toFixed(0) + '%', percent: diskPercent.value, color: '#e0a040' },
  { label: 'NET', value: formatRate(metrics.value.network?.rx_bytes_per_sec), percent: Math.min((metrics.value.network?.rx_bytes_per_sec ?? 0) / 1_000_000 * 100, 100), color: '#e07a7a' },
  { label: 'POWER', value: (metrics.value.power?.total_watts?.toFixed(0) ?? '0') + 'W', percent: Math.min((metrics.value.power?.total_watts ?? 0) / 200 * 100, 100), color: '#c07ae0' },
  { label: 'PODS', value: String(pods.value.length), percent: pods.value.length > 0 ? 100 : 0, color: '#70b0e0' },
]);

const podColumns = [
  { name: 'name', label: 'Name', field: 'name', align: 'left' as const, sortable: true },
  { name: 'namespace', label: 'Namespace', field: 'namespace', align: 'left' as const, sortable: true },
  { name: 'status', label: 'Status', field: 'status', align: 'left' as const, sortable: true },
  { name: 'ready', label: 'Ready', field: 'ready', align: 'center' as const },
  { name: 'restarts', label: 'Restarts', field: 'restarts', align: 'center' as const, sortable: true },
  { name: 'node', label: 'Node', field: 'node', align: 'left' as const },
  { name: 'age', label: 'Age', field: 'age', align: 'left' as const },
];

// ─── Helpers ───
function formatBytes(b?: number): string {
  if (b == null) return '--';
  if (b === 0) return '0 B';
  const u = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(b) / Math.log(1024));
  return (b / Math.pow(1024, i)).toFixed(1) + ' ' + u[i];
}

function formatRate(bps?: number): string {
  if (bps == null || bps === 0) return '0 B/s';
  if (bps < 1024) return bps.toFixed(0) + ' B/s';
  if (bps < 1024 * 1024) return (bps / 1024).toFixed(1) + ' KB/s';
  return (bps / 1024 / 1024).toFixed(1) + ' MB/s';
}

function formatUptime(s?: number): string {
  if (s == null) return '--';
  const d = Math.floor(s / 86400);
  const h = Math.floor((s % 86400) / 3600);
  const m = Math.floor((s % 3600) / 60);
  if (d > 0) return `${d}d ${h}h`;
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}

function fmtLoad(l: number[]): string {
  return l.map(v => v.toFixed(2)).join('  ');
}

function pushHistory(arr: number[], val: number) {
  arr.push(val);
  if (arr.length > MAX_POINTS) arr.shift();
}

// ─── WebSocket ───
function startWS() {
  ws = new WebSocket(getWsUrl());
  ws.onmessage = (e) => {
    try {
      const msg = JSON.parse(e.data);
      if (msg.type === 'metrics' && msg.data) {
        const d = typeof msg.data === 'string' ? JSON.parse(msg.data) : msg.data;
        metrics.value = d;
        pushHistory(cpuHistory.value, d.cpu_usage ?? 0);
        pushHistory(memHistory.value, d.memory && d.memory.total ? (d.memory.used / d.memory.total) * 100 : 0);
        pushHistory(netRxHistory.value, d.network?.rx_bytes_per_sec ?? 0);
        pushHistory(netTxHistory.value, d.network?.tx_bytes_per_sec ?? 0);
        pushHistory(diskRHistory.value, d.disk_io?.read_bytes_per_sec ?? 0);
        pushHistory(diskWHistory.value, d.disk_io?.write_bytes_per_sec ?? 0);
        pushHistory(powerHistory.value, d.power?.total_watts ?? 0);
        // Update pod badge
        const podsNav = navItems.find(n => n.key === 'pods');
        if (podsNav) podsNav.badge = String(pods.value.length);
      }
    } catch { /* */ }
  };
  ws.onclose = () => { setTimeout(() => { if (ws) startWS(); }, 5000); };
}

// ─── API ───
async function fetchPods() {
  try {
    const res: any = await api.get('/api/monitor/status');
    pods.value = res?.pods || res?.data?.pods || [];
  } catch { pods.value = []; }
}

async function fetchGpu() {
  try {
    const res: any = await api.get('/api/monitor/gpu/list');
    gpuData.value = res || { gpus: [], gpu_count: 0, driver_installed: false };
  } catch { gpuData.value = { gpus: [], gpu_count: 0, driver_installed: false }; }
}

async function fetchLogs() {
  try {
    const q = logNamespace.value !== 'All'
      ? `{namespace="${logNamespace.value}"}`
      : logQuery.value;
    const res: any = await api.get('/api/logs', { params: { query: q, limit: 200 } });
    const streams = res?.data?.result || [];
    const entries: LogEntry[] = [];
    for (const s of streams) {
      const ns = s.stream?.namespace || s.stream?.pod || '';
      for (const v of (s.values || [])) {
        const ts = new Date(Number(v[0]) / 1_000_000).toLocaleTimeString();
        entries.push({ ts, ns, msg: v[1] });
      }
    }
    logEntries.value = entries.reverse();
    nextTick(() => { if (logsContainer.value) logsContainer.value.scrollTop = logsContainer.value.scrollHeight; });
  } catch { logEntries.value = []; }
}

function onNamespaceChange() { fetchLogs(); }

// ─── Chart Drawing ───
function drawLine(canvas: HTMLCanvasElement | null, data: number[], color: string, gStart: string, gEnd: string, maxVal = 100, unit = '%') {
  if (!canvas) return;
  const ctx = canvas.getContext('2d');
  if (!ctx) return;
  const dpr = window.devicePixelRatio || 1;
  const rect = canvas.getBoundingClientRect();
  canvas.width = rect.width * dpr;
  canvas.height = rect.height * dpr;
  ctx.scale(dpr, dpr);
  const w = rect.width, h = rect.height;
  const pad = { top: 8, right: 8, bottom: 20, left: 40 };
  const cw = w - pad.left - pad.right, ch = h - pad.top - pad.bottom;
  ctx.clearRect(0, 0, w, h);

  // Grid
  ctx.font = '10px monospace';
  ctx.textAlign = 'right';
  ctx.textBaseline = 'middle';
  for (let i = 0; i <= 4; i++) {
    const val = (maxVal * i) / 4;
    const y = pad.top + ch - (ch * i) / 4;
    ctx.fillStyle = '#555';
    ctx.fillText(unit === '%' ? val.toFixed(0) + '%' : formatRate(val), pad.left - 4, y);
    ctx.strokeStyle = 'rgba(255,255,255,0.03)';
    ctx.beginPath(); ctx.moveTo(pad.left, y); ctx.lineTo(w - pad.right, y); ctx.stroke();
  }

  if (data.length < 2) return;
  const step = cw / (MAX_POINTS - 1);
  const off = MAX_POINTS - data.length;
  const pts = data.map((v, i) => ({
    x: pad.left + (off + i) * step,
    y: pad.top + ch - (ch * Math.min(v, maxVal)) / maxVal,
  }));

  // Fill
  const grad = ctx.createLinearGradient(0, pad.top, 0, pad.top + ch);
  grad.addColorStop(0, gStart); grad.addColorStop(1, gEnd);
  ctx.beginPath();
  ctx.moveTo(pts[0].x, pad.top + ch);
  ctx.lineTo(pts[0].x, pts[0].y);
  for (let i = 1; i < pts.length; i++) {
    const cpx = (pts[i - 1].x + pts[i].x) / 2;
    ctx.bezierCurveTo(cpx, pts[i - 1].y, cpx, pts[i].y, pts[i].x, pts[i].y);
  }
  ctx.lineTo(pts[pts.length - 1].x, pad.top + ch);
  ctx.closePath(); ctx.fillStyle = grad; ctx.fill();

  // Line
  ctx.beginPath();
  ctx.moveTo(pts[0].x, pts[0].y);
  for (let i = 1; i < pts.length; i++) {
    const cpx = (pts[i - 1].x + pts[i].x) / 2;
    ctx.bezierCurveTo(cpx, pts[i - 1].y, cpx, pts[i].y, pts[i].x, pts[i].y);
  }
  ctx.strokeStyle = color; ctx.lineWidth = 1.5; ctx.stroke();
}

function drawDualLine(canvas: HTMLCanvasElement | null, d1: number[], d2: number[], c1: string, c2: string, g1s: string, g1e: string) {
  if (!canvas) return;
  const max = Math.max(...d1, ...d2, 1024) * 1.2;
  drawLine(canvas, d1, c1, g1s, g1e, max, 'rate');
  // Draw second line without fill
  const ctx = canvas.getContext('2d');
  if (!ctx || d2.length < 2) return;
  const dpr = window.devicePixelRatio || 1;
  const rect = canvas.getBoundingClientRect();
  const w = rect.width, h = rect.height;
  const pad = { top: 8, right: 8, bottom: 20, left: 40 };
  const cw = w - pad.left - pad.right, ch = h - pad.top - pad.bottom;
  const step = cw / (MAX_POINTS - 1);
  const off = MAX_POINTS - d2.length;
  const pts = d2.map((v, i) => ({
    x: pad.left + (off + i) * step,
    y: pad.top + ch - (ch * Math.min(v, max)) / max,
  }));
  ctx.beginPath();
  ctx.moveTo(pts[0].x, pts[0].y);
  for (let i = 1; i < pts.length; i++) {
    const cpx = (pts[i - 1].x + pts[i].x) / 2;
    ctx.bezierCurveTo(cpx, pts[i - 1].y, cpx, pts[i].y, pts[i].x, pts[i].y);
  }
  ctx.strokeStyle = c2; ctx.lineWidth = 1.5; ctx.setLineDash([4, 3]); ctx.stroke();
  ctx.setLineDash([]);
}

function renderCharts() {
  drawLine(cpuCanvas.value, cpuHistory.value, '#4c9fe7', 'rgba(76,159,231,0.2)', 'rgba(76,159,231,0)');
  drawLine(memCanvas.value, memHistory.value, '#29cc5f', 'rgba(41,204,95,0.2)', 'rgba(41,204,95,0)');
  drawDualLine(netCanvas.value, netRxHistory.value, netTxHistory.value, '#e0a040', '#e07a50', 'rgba(224,160,64,0.15)', 'rgba(224,160,64,0)');
  drawDualLine(diskIOCanvas.value, diskRHistory.value, diskWHistory.value, '#c07ae0', '#7ae0c0', 'rgba(192,122,224,0.15)', 'rgba(192,122,224,0)');
  // Full page charts
  drawDualLine(netFullCanvas.value, netRxHistory.value, netTxHistory.value, '#e0a040', '#e07a50', 'rgba(224,160,64,0.2)', 'rgba(224,160,64,0)');
  drawDualLine(diskIOFullCanvas.value, diskRHistory.value, diskWHistory.value, '#c07ae0', '#7ae0c0', 'rgba(192,122,224,0.2)', 'rgba(192,122,224,0)');
  drawLine(powerCanvas.value, powerHistory.value, '#e07ae0', 'rgba(224,122,224,0.2)', 'rgba(224,122,224,0)', Math.max(...powerHistory.value, 50) * 1.2, 'rate');
  animFrameId = requestAnimationFrame(renderCharts);
}

// ─── Lifecycle ───
watch(activeNav, () => {
  nextTick(() => {
    if (!animFrameId) animFrameId = requestAnimationFrame(renderCharts);
  });
});

onMounted(async () => {
  await Promise.all([fetchPods(), fetchGpu()]);
  startWS();
  pollTimer = setInterval(fetchPods, 30000);
  nextTick(() => { animFrameId = requestAnimationFrame(renderCharts); });
});

onUnmounted(() => {
  if (pollTimer) clearInterval(pollTimer);
  if (animFrameId) cancelAnimationFrame(animFrameId);
  if (ws) { ws.onclose = null; ws.close(); ws = null; }
});
</script>

<style lang="scss" scoped>
$bg-deep: #0d0f12;
$bg-card: #13161b;
$bg-hover: #1a1d24;
$border: rgba(255,255,255,0.06);
$ink-1: #e8eaed;
$ink-2: #9aa0a6;
$ink-3: #5f6368;
$accent: #4c9fe7;

.dash-root {
  display: flex;
  width: 100%;
  height: 100vh;
  background: $bg-deep;
  overflow: hidden;
  font-family: 'Inter', -apple-system, sans-serif;
  color: $ink-1;
}

// ── Sidebar ──
.dash-sidebar {
  width: 200px;
  min-width: 200px;
  background: $bg-deep;
  border-right: 1px solid $border;
  display: flex;
  flex-direction: column;
  padding: 8px;
}
.sidebar-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 14px 10px 18px;
}
.header-icon { color: $accent; }
.sidebar-title { font-size: 15px; font-weight: 600; letter-spacing: 0.5px; }
.sidebar-nav-item {
  border-radius: 6px;
  min-height: 34px;
  margin-bottom: 1px;
  .q-item__section--avatar { padding-right: 0; }
}
.sidebar-item-active { background: rgba($accent, 0.12) !important; }
.sidebar-item-active .nav-label { color: $accent !important; }
.sidebar-item-active .q-icon { color: $accent !important; }
.nav-label { font-size: 13px; color: $ink-2; }
.nav-badge { font-size: 10px; color: $ink-3; background: rgba(255,255,255,0.06); padding: 1px 6px; border-radius: 8px; font-family: monospace; }
.sidebar-footer { margin-top: auto; padding: 12px 10px; }
.sys-uptime { display: flex; align-items: center; gap: 6px; }
.uptime-dot { width: 6px; height: 6px; border-radius: 50%; background: #29cc5f; box-shadow: 0 0 6px #29cc5f; }
.uptime-text { font-size: 11px; color: $ink-3; font-family: monospace; }

// ── Content ──
.dash-content {
  flex: 1;
  overflow-y: auto;
  padding: 20px 24px 40px;
  scrollbar-width: thin;
  scrollbar-color: #2a2d33 transparent;
}
.page-title { font-size: 18px; font-weight: 600; margin-bottom: 20px; }
.page-subtitle { font-size: 15px; font-weight: 500; margin: 24px 0 12px; color: $ink-2; }

// ── Resource Strip ──
.resource-strip {
  display: flex;
  gap: 10px;
  margin-bottom: 16px;
}
.strip-card {
  flex: 1;
  background: $bg-card;
  border: 1px solid $border;
  border-radius: 8px;
  padding: 10px 12px;
}
.strip-label { font-size: 10px; font-weight: 600; color: $ink-3; letter-spacing: 1px; text-transform: uppercase; }
.strip-value { font-size: 18px; font-weight: 700; font-family: 'JetBrains Mono', monospace; margin: 2px 0 6px; }
.strip-bar-track { height: 3px; background: rgba(255,255,255,0.05); border-radius: 2px; }
.strip-bar-fill { height: 100%; border-radius: 2px; transition: width 0.4s ease; }

// ── Charts ──
.charts-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
  margin-bottom: 16px;
}
.chart-panel {
  background: $bg-card;
  border: 1px solid $border;
  border-radius: 8px;
  padding: 10px 12px;
}
.chart-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 4px;
}
.chart-title { font-size: 11px; font-weight: 600; color: $ink-3; text-transform: uppercase; letter-spacing: 0.5px; }
.chart-live { font-size: 12px; font-weight: 600; font-family: 'JetBrains Mono', monospace; }
.chart-canvas { width: 100%; height: 140px; display: block; }

.full-chart-panel {
  background: $bg-card;
  border: 1px solid $border;
  border-radius: 8px;
  padding: 12px;
}
.chart-canvas-full { width: 100%; height: 280px; display: block; }

// ── Bottom Row ──
.bottom-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
}
.cores-panel, .info-panel {
  background: $bg-card;
  border: 1px solid $border;
  border-radius: 8px;
  padding: 12px;
}
.panel-head { font-size: 11px; font-weight: 600; color: $ink-3; text-transform: uppercase; letter-spacing: 0.5px; margin-bottom: 10px; }
.cores-list { max-height: 200px; overflow-y: auto; scrollbar-width: thin; }
.core-row { display: flex; align-items: center; gap: 6px; margin-bottom: 3px; }
.core-id { font-size: 10px; color: $ink-3; width: 16px; text-align: right; font-family: monospace; }
.core-bar-track { flex: 1; height: 10px; background: rgba(255,255,255,0.04); border-radius: 2px; }
.core-bar-fill { height: 100%; background: $accent; border-radius: 2px; transition: width 0.3s; }
.core-val { font-size: 10px; color: $ink-2; width: 28px; text-align: right; font-family: monospace; }

.info-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 6px; }
.info-item { display: flex; justify-content: space-between; padding: 4px 0; border-bottom: 1px solid $border; }
.info-label { font-size: 12px; color: $ink-3; }
.info-val { font-size: 12px; color: $ink-1; font-family: monospace; font-weight: 500; }

// ── Stats ──
.stat-row { display: flex; gap: 10px; margin-bottom: 16px; }
.stat-box {
  flex: 1;
  background: $bg-card;
  border: 1px solid $border;
  border-radius: 8px;
  padding: 14px;
  display: flex;
  flex-direction: column;
}
.stat-label { font-size: 11px; color: $ink-3; text-transform: uppercase; letter-spacing: 0.5px; margin-bottom: 4px; }
.stat-val { font-size: 22px; font-weight: 700; font-family: 'JetBrains Mono', monospace; }
.stat-val.rx { color: #e0a040; }
.stat-val.tx { color: #e07a50; }
.stat-val.power-total { color: #c07ae0; }

// ── Disk Bar ──
.disk-usage-bar {
  height: 20px;
  background: rgba(255,255,255,0.04);
  border-radius: 4px;
  margin-bottom: 24px;
  overflow: hidden;
}
.disk-used {
  height: 100%;
  background: linear-gradient(90deg, #e0a040, #e07a50);
  border-radius: 4px;
  transition: width 0.4s;
}

// ── Logs ──
.logs-controls {
  display: flex;
  gap: 8px;
  margin-bottom: 12px;
  align-items: center;
}
.log-query-input { flex: 1; }
.log-ns-select { width: 180px; }
.log-fetch-btn { background: rgba($accent, 0.15); color: $accent; }
.logs-output {
  background: $bg-card;
  border: 1px solid $border;
  border-radius: 8px;
  height: calc(100vh - 200px);
  overflow-y: auto;
  padding: 8px;
  font-family: 'JetBrains Mono', 'Fira Code', monospace;
  font-size: 11px;
  line-height: 1.6;
  scrollbar-width: thin;
}
.logs-empty { color: $ink-3; padding: 40px; text-align: center; }
.log-line { display: flex; gap: 8px; padding: 1px 4px; border-bottom: 1px solid rgba(255,255,255,0.02); }
.log-line:hover { background: rgba(255,255,255,0.02); }
.log-ts { color: $ink-3; white-space: nowrap; min-width: 70px; }
.log-ns { color: $accent; white-space: nowrap; min-width: 100px; max-width: 140px; overflow: hidden; text-overflow: ellipsis; }
.log-msg { color: $ink-2; word-break: break-all; }

// ── Pod Table ──
.pod-table-wrap { overflow: hidden; border-radius: 8px; border: 1px solid $border; }
.pod-table {
  background: $bg-card !important;
  :deep(thead th) { font-size: 11px; font-weight: 600; color: $ink-3 !important; background: $bg-deep !important; border-bottom: 1px solid $border; text-transform: uppercase; letter-spacing: 0.5px; }
  :deep(tbody td) { font-size: 12px; color: $ink-1 !important; border-bottom: 1px solid $border; font-family: monospace; }
  :deep(.q-table__bottom) { color: $ink-3 !important; border-top: 1px solid $border; }
  :deep(.q-field__native), :deep(.q-field__control) { color: $ink-1 !important; }
}
.pod-dot { display: inline-block; width: 6px; height: 6px; border-radius: 50%; margin-right: 6px; }
.dot-running { background: #29cc5f; box-shadow: 0 0 4px #29cc5f; }
.dot-pending { background: #e0a040; }
.dot-failed, .dot-crashloopbackoff, .dot-error { background: #e05050; }
.dot-succeeded, .dot-completed { background: #29cc5f; }
</style>
