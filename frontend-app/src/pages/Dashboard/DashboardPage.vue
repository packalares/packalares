<template>
  <div class="iframe-root">
    <!-- Left Sidebar -->
    <div class="iframe-sidebar">
      <div class="sidebar-brand">
        <div class="brand-icon">
          <q-icon name="sym_r_monitoring" size="18px" color="white" />
        </div>
        <div class="brand-info">
          <div class="brand-title">System</div>
          <div class="brand-sub">Dashboard</div>
        </div>
      </div>
      <div class="sidebar-divider"></div>
      <div class="sidebar-nav">
        <div
          v-for="item in navItems"
          :key="item.key"
          class="nav-item"
          :class="{ active: activeNav === item.key }"
          @click="activeNav = item.key"
        >
          <q-icon :name="item.icon" size="17px" class="nav-icon" />
          <span class="nav-text">{{ item.label }}</span>
          <span v-if="item.badge" class="nav-badge">{{ item.badge }}</span>
        </div>
      </div>
      <div class="sidebar-footer">
        <div class="sys-uptime">
          <span class="uptime-dot"></span>
          <span class="uptime-text">{{ formatUptime(metrics.uptime) }}</span>
        </div>
      </div>
    </div>

    <!-- Main Content -->
    <div class="iframe-content">

      <!-- ═══════ OVERVIEW ═══════ -->
      <template v-if="activeNav === 'overview'">
        <div class="page-header"><div class="page-title">Overview</div></div>
        <div class="page-scroll">
        <!-- Top Resource Strip -->
        <div class="resource-strip">
          <div class="strip-card" v-for="r in resourceCards" :key="r.label">
            <div class="strip-top">
              <div class="strip-label">{{ r.label }}</div>
              <div class="strip-value" :style="{ color: r.color }">{{ r.value }}</div>
            </div>
            <div class="strip-bar-track">
              <div class="strip-bar-fill" :style="{ width: r.percent + '%', background: r.color }"></div>
            </div>
          </div>
        </div>

        <!-- Charts Grid -->
        <div class="charts-grid">
          <div class="chart-panel">
            <div class="chart-head">
              <span class="chart-title">
                <span class="chart-dot" style="background:#4c9fe7"></span>
                CPU
              </span>
              <span class="chart-live" :style="{ color: '#4c9fe7' }">{{ metrics.cpu_usage?.toFixed(1) ?? '0' }}%</span>
            </div>
            <canvas ref="cpuCanvas" class="chart-canvas"></canvas>
          </div>
          <div class="chart-panel">
            <div class="chart-head">
              <span class="chart-title">
                <span class="chart-dot" style="background:#29cc5f"></span>
                Memory
              </span>
              <span class="chart-live" :style="{ color: '#29cc5f' }">{{ memPercent.toFixed(1) }}%</span>
            </div>
            <canvas ref="memCanvas" class="chart-canvas"></canvas>
          </div>
          <div class="chart-panel">
            <div class="chart-head">
              <span class="chart-title">
                <span class="chart-dot" style="background:#e0a040"></span>
                Network
              </span>
              <span class="chart-live" :style="{ color: '#e0a040' }">
                <span class="chart-dir">RX</span> {{ formatRate(metrics.network?.rx_bytes_per_sec) }}
                <span class="chart-dir" style="margin-left:6px">TX</span> {{ formatRate(metrics.network?.tx_bytes_per_sec) }}
              </span>
            </div>
            <canvas ref="netCanvas" class="chart-canvas"></canvas>
          </div>
          <div class="chart-panel">
            <div class="chart-head">
              <span class="chart-title">
                <span class="chart-dot" style="background:#c07ae0"></span>
                Disk I/O
              </span>
              <span class="chart-live" :style="{ color: '#c07ae0' }">
                <span class="chart-dir">R</span> {{ formatRate(metrics.disk_io?.read_bytes_per_sec) }}
                <span class="chart-dir" style="margin-left:6px">W</span> {{ formatRate(metrics.disk_io?.write_bytes_per_sec) }}
              </span>
            </div>
            <canvas ref="diskIOCanvas" class="chart-canvas"></canvas>
          </div>
        </div>

        <!-- Per-Core CPU + System Info -->
        <div class="bottom-row">
          <div class="cores-panel">
            <div class="panel-head">
              <span class="chart-dot" style="background:#4c9fe7"></span>
              CPU Cores
            </div>
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
        </div>
      </template>

      <!-- ═══════ NETWORK ═══════ -->
      <template v-if="activeNav === 'network'">
        <div class="page-header"><div class="page-title">Network</div></div>
        <div class="page-scroll">
        <div class="stat-row">
          <div class="stat-box"><span class="stat-label">Download</span><span class="stat-val rx">{{ formatRate(metrics.network?.rx_bytes_per_sec) }}</span></div>
          <div class="stat-box"><span class="stat-label">Upload</span><span class="stat-val tx">{{ formatRate(metrics.network?.tx_bytes_per_sec) }}</span></div>
        </div>
        <div class="full-chart-panel">
          <canvas ref="netFullCanvas" class="chart-canvas-full"></canvas>
        </div>
        </div>
      </template>

      <!-- ═══════ STORAGE ═══════ -->
      <template v-if="activeNav === 'storage'">
        <div class="page-header"><div class="page-title">Storage</div></div>
        <div class="page-scroll">
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
        </div>
      </template>

      <!-- ═══════ POWER ═══════ -->
      <template v-if="activeNav === 'power'">
        <div class="page-header"><div class="page-title">Power Consumption</div></div>
        <div class="page-scroll">
        <div class="stat-row">
          <div class="stat-box"><span class="stat-label">CPU</span><span class="stat-val">{{ metrics.power?.cpu_watts?.toFixed(1) ?? '--' }} W</span></div>
          <div class="stat-box"><span class="stat-label">GPU</span><span class="stat-val">{{ metrics.power?.gpu_watts?.toFixed(1) ?? '--' }} W</span></div>
          <div class="stat-box"><span class="stat-label">Total</span><span class="stat-val power-total">{{ metrics.power?.total_watts?.toFixed(1) ?? '--' }} W</span></div>
        </div>
        <div class="full-chart-panel">
          <canvas ref="powerCanvas" class="chart-canvas-full"></canvas>
        </div>
        </div>
      </template>

      <!-- ═══════ LOGS ═══════ -->
      <template v-if="activeNav === 'logs'">
        <div class="page-header"><div class="page-title">Logs</div></div>
        <div class="page-scroll">
        <div class="logs-toolbar">
          <q-select v-model="logNamespace" dense dark outlined :options="logNamespaceOptions" style="width:140px" @update:model-value="fetchLogs" />
          <q-select v-model="logTimeRange" dense dark outlined :options="logTimeOptions" option-label="label" option-value="value" emit-value map-options style="width:120px" @update:model-value="fetchLogs" />
          <q-input v-model="logSearch" dense dark outlined placeholder="Search..." style="flex:1;min-width:100px" @keyup.enter="fetchLogs">
            <template #prepend><q-icon name="sym_r_search" size="14px" color="grey-6" /></template>
          </q-input>
          <q-btn flat dense round icon="sym_r_refresh" size="sm" @click="fetchLogs" :loading="logLoading" />
          <q-toggle v-model="logAutoRefresh" dense color="primary" />
          <span style="font-size:11px;color:var(--ink-3)">Auto</span>
        </div>
        <div class="log-table-wrap">
          <q-table
            flat dark
            :rows="logEntries"
            :columns="logColumns"
            row-key="__idx"
            :rows-per-page-options="[50, 100, 200, 0]"
            v-model:pagination="logPagination"
            :loading="logLoading"
            class="log-table"
            :no-data-label="logLoading ? 'Loading...' : 'No log entries found.'"
            rows-per-page-label="Per page"
          >
            <template v-slot:body="props">
              <q-tr :props="props" :class="{ 'log-row-zebra': props.rowIndex % 2 === 0 }">
                <q-td key="ts" :props="props" class="log-cell-ts">
                  <q-icon name="sym_r_schedule" size="12px" class="log-cell-icon" />
                  {{ props.row.ts }}
                </q-td>
                <q-td key="level" :props="props" class="log-cell-level">
                  <span class="log-level-badge" :class="'badge-' + props.row.level">{{ props.row.level }}</span>
                </q-td>
                <q-td key="ns" :props="props" class="log-cell-ns">
                  <q-icon name="sym_r_folder" size="12px" class="log-cell-icon" />
                  {{ props.row.ns }}
                </q-td>
                <q-td key="pod" :props="props" class="log-cell-pod">{{ props.row.pod }}</q-td>
                <q-td key="msg" :props="props" class="log-cell-msg" :class="'log-msg-' + props.row.level">{{ props.row.msg }}</q-td>
              </q-tr>
            </template>
            <template v-slot:bottom="scope">
              <div class="log-bottom">
                <span class="log-bottom-count">{{ logEntries.length }} entries</span>
                <q-space />
                <span class="log-bottom-label">Per page</span>
                <q-select
                  v-model="logPagination.rowsPerPage"
                  dense dark outlined
                  :options="[50, 100, 200]"
                  style="width:70px"
                  @update:model-value="saveLogPrefs"
                />
                <q-btn flat dense icon="sym_r_chevron_left" size="sm" :disable="scope.isFirstPage" @click="scope.prevPage" />
                <span class="log-bottom-page">{{ scope.pagination.page }} / {{ scope.pagesNumber }}</span>
                <q-btn flat dense icon="sym_r_chevron_right" size="sm" :disable="scope.isLastPage" @click="scope.nextPage" />
              </div>
            </template>
          </q-table>
        </div>
        </div>
      </template>

      <!-- ═══════ PODS ═══════ -->
      <template v-if="activeNav === 'pods'">
        <div class="page-header"><div class="page-title">Pods ({{ pods.length }})</div></div>
        <div class="page-scroll">
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
        </div>
      </template>
    </div>
  </div>
</template>

<script lang="ts" setup>
import { ref, computed, onMounted, onUnmounted, nextTick, watch } from 'vue';
import { api, getWsUrl } from 'boot/axios';
import { formatBytes, formatRate, formatUptime, fmtLoad } from 'src/utils/helpers';

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

interface LogEntry { ts: string; ns: string; pod: string; msg: string; level: string }

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

// Logs — persist preferences
function loadLogPrefs() {
  try {
    const s = localStorage.getItem('packalares_log_prefs');
    if (s) return JSON.parse(s);
  } catch {}
  return {};
}
function saveLogPrefs() {
  localStorage.setItem('packalares_log_prefs', JSON.stringify({
    namespace: logNamespace.value,
    timeRange: logTimeRange.value,
    rowsPerPage: logPagination.value.rowsPerPage,
  }));
}
const _logPrefs = loadLogPrefs();
const logNamespace = ref(_logPrefs.namespace || 'All');
const logNamespaceOptions = ['All', 'os-system', 'os-framework', 'monitoring', 'user-space-admin', 'kube-system'];
const logSearch = ref('');
const logTimeRange = ref(_logPrefs.timeRange || '15m');
const logTimeOptions = [
  { label: 'All time', value: 'all' },
  { label: 'Last 5m', value: '5m' },
  { label: 'Last 15m', value: '15m' },
  { label: 'Last 1h', value: '1h' },
  { label: 'Last 6h', value: '6h' },
  { label: 'Last 24h', value: '24h' },
];
const logEntries = ref<(LogEntry & { __idx: number })[]>([]);
const logLoading = ref(false);
const logAutoRefresh = ref(false);
const logPagination = ref({ rowsPerPage: _logPrefs.rowsPerPage || 100, page: 1, sortBy: null, descending: false });
let logAutoTimer: ReturnType<typeof setInterval> | null = null;

const logColumns = [
  { name: 'ts', label: 'Time', field: 'ts', align: 'left' as const, style: 'width:80px;white-space:nowrap;color:var(--ink-3);font-size:11px' },
  { name: 'level', label: 'Level', field: 'level', align: 'center' as const, style: 'width:50px' },
  { name: 'ns', label: 'Namespace', field: 'ns', align: 'left' as const, style: 'width:110px;white-space:nowrap;color:var(--accent);font-size:11px' },
  { name: 'pod', label: 'Pod', field: 'pod', align: 'left' as const, style: 'width:140px;white-space:nowrap;color:var(--ink-3);font-size:11px;overflow:hidden;text-overflow:ellipsis;max-width:140px' },
  { name: 'msg', label: 'Message', field: 'msg', align: 'left' as const, style: 'font-size:11px;word-break:break-all' },
];

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

function parseLogLevel(msg: string): string {
  const m = msg.toLowerCase();
  if (m.includes('level=error') || m.includes('"level":"error"') || m.includes(' error ') || m.includes('[error]')) return 'error';
  if (m.includes('level=warn') || m.includes('"level":"warn"') || m.includes(' warn') || m.includes('[warn')) return 'warn';
  if (m.includes('level=debug') || m.includes('"level":"debug"') || m.includes('[debug]')) return 'debug';
  return 'info';
}

function timeRangeToNano(range: string): string {
  const units: Record<string, number> = { m: 60, h: 3600, d: 86400 };
  const match = range.match(/^(\d+)([mhd])$/);
  if (!match) return '';
  const secs = parseInt(match[1]) * (units[match[2]] || 60);
  return String((Date.now() - secs * 1000) * 1_000_000);
}

async function fetchLogs() {
  logLoading.value = true;
  saveLogPrefs();
  try {
    let q = logNamespace.value !== 'All'
      ? `{namespace="${logNamespace.value}"}`
      : `{namespace=~".+"}`;
    if (logSearch.value) q += ` |~ "${logSearch.value}"`;

    const params: Record<string, string> = { query: q, limit: '500', direction: 'backward' };
    if (logTimeRange.value !== 'all') {
      const start = timeRangeToNano(logTimeRange.value);
      if (start) params.start = start;
    }

    const res: any = await api.get('/api/logs', { params });
    const streams = res?.data?.result || [];
    const entries: (LogEntry & { __idx: number })[] = [];
    let idx = 0;
    for (const s of streams) {
      const ns = s.stream?.namespace || '';
      const pod = s.stream?.pod || '';
      for (const v of (s.values || [])) {
        const ts = new Date(Number(v[0]) / 1_000_000).toLocaleTimeString();
        const msg = v[1];
        entries.push({ ts, ns, pod, msg, level: parseLogLevel(msg), __idx: idx++ });
      }
    }
    logEntries.value = entries;
  } catch { logEntries.value = []; }
  logLoading.value = false;
}

watch(logAutoRefresh, (on) => {
  if (logAutoTimer) { clearInterval(logAutoTimer); logAutoTimer = null; }
  if (on) logAutoTimer = setInterval(fetchLogs, 5000);
});

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
  ctx.font = '10px Inter, sans-serif';
  ctx.textAlign = 'right';
  ctx.textBaseline = 'middle';
  for (let i = 0; i <= 4; i++) {
    const val = (maxVal * i) / 4;
    const y = pad.top + ch - (ch * i) / 4;
    ctx.fillStyle = '#9aa0a6';
    ctx.fillText(unit === '%' ? val.toFixed(0) + '%' : formatRate(val), pad.left - 4, y);
    ctx.strokeStyle = 'rgba(255,255,255,0.05)';
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
watch(activeNav, (nav) => {
  nextTick(() => {
    if (!animFrameId) animFrameId = requestAnimationFrame(renderCharts);
  });
  if (nav === 'logs' && logEntries.value.length === 0) fetchLogs();
  // Stop log auto-refresh when leaving logs tab
  if (nav !== 'logs' && logAutoTimer) { clearInterval(logAutoTimer); logAutoTimer = null; logAutoRefresh.value = false; }
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
  if (logAutoTimer) clearInterval(logAutoTimer);
});
</script>

<style lang="scss" scoped>
// Use CSS custom properties for theme-awareness
$bg-deep: var(--bg-0);
$bg-card: var(--bg-2);
$bg-hover: var(--bg-3);
$border: var(--border);
$ink-1: var(--ink-1);
$ink-2: var(--ink-2);
$ink-3: var(--ink-3);
$accent: #4c9fe7;

// ── Dashboard-specific ──
.sidebar-footer { margin-top: auto; padding: 12px 10px; }
.sys-uptime { display: flex; align-items: center; gap: 6px; }
.uptime-dot { width: 6px; height: 6px; border-radius: 50%; background: #29cc5f; box-shadow: 0 0 6px #29cc5f; }
.uptime-text { font-size: 11px; color: var(--ink-3); font-family: 'Inter', sans-serif; }
.page-subtitle { font-size: 13px; font-weight: 500; margin: 20px 0 10px; color: $ink-2; }

// ── Resource Strip ──
.resource-strip {
  display: grid;
  grid-template-columns: 1fr 1fr 1fr;
  gap: 10px;
  margin-bottom: 16px;
}
.strip-card {
  flex: 1;
  background: $bg-card;
  border: 1px solid $border;
  border-radius: 10px;
  padding: 12px 14px 10px;
}
.strip-top { display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 8px; }
.strip-label { font-size: 10px; font-weight: 600; color: $ink-3; letter-spacing: 0.08em; text-transform: uppercase; }
.strip-value { font-size: 20px; font-weight: 700; font-family: 'Inter', sans-serif; line-height: 1; }
.strip-bar-track { height: 4px; background: var(--track-bg); border-radius: 2px; }
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
  border-radius: 10px;
  padding: 12px 14px;
}
.chart-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 6px;
}
.chart-title { font-size: 11px; font-weight: 600; color: $ink-2; text-transform: uppercase; letter-spacing: 0.5px; display: flex; align-items: center; gap: 6px; }
.chart-dot { width: 7px; height: 7px; border-radius: 50%; flex-shrink: 0; }
.chart-dir { font-size: 9px; font-weight: 600; letter-spacing: 0.04em; opacity: 0.7; }
.chart-live { font-size: 12px; font-weight: 600; font-family: 'Inter', sans-serif; }
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
  border-radius: 10px;
  padding: 14px;
}
.panel-head { font-size: 11px; font-weight: 600; color: $ink-2; text-transform: uppercase; letter-spacing: 0.5px; margin-bottom: 10px; display: flex; align-items: center; gap: 6px; }
.cores-list { max-height: 200px; overflow-y: auto; scrollbar-width: thin; }
.core-row { display: flex; align-items: center; gap: 6px; margin-bottom: 3px; }
.core-id { font-size: 10px; color: $ink-3; width: 16px; text-align: right; font-family: 'Inter', sans-serif; }
.core-bar-track { flex: 1; height: 10px; background: var(--hover-bg); border-radius: 2px; }
.core-bar-fill { height: 100%; background: $accent; border-radius: 2px; transition: width 0.3s; }
.core-val { font-size: 10px; color: $ink-2; width: 28px; text-align: right; font-family: 'Inter', sans-serif; }

.info-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 6px; }
.info-item { display: flex; justify-content: space-between; padding: 4px 0; border-bottom: 1px solid $border; }
.info-label { font-size: 12px; color: $ink-3; }
.info-val { font-size: 12px; color: $ink-1; font-family: 'Inter', sans-serif; font-weight: 500; }

// ── Stats ──
.stat-row { display: flex; gap: 10px; margin-bottom: 16px; }
.stat-box {
  flex: 1;
  background: $bg-card;
  border: 1px solid $border;
  border-radius: 10px;
  padding: 16px;
  display: flex;
  flex-direction: column;
}
.stat-label { font-size: 10px; color: $ink-3; text-transform: uppercase; letter-spacing: 0.06em; font-weight: 600; margin-bottom: 6px; }
.stat-val { font-size: 24px; font-weight: 700; font-family: 'Inter', sans-serif; letter-spacing: -0.02em; }
.stat-val.rx { color: #e0a040; }
.stat-val.tx { color: #e07a50; }
.stat-val.power-total { color: #c07ae0; }

// ── Disk Bar ──
.disk-usage-bar {
  height: 20px;
  background: var(--hover-bg);
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
.logs-toolbar { display: flex; gap: 6px; margin-bottom: 10px; align-items: center; }
.log-table-wrap { border-radius: 8px; border: 1px solid $border; overflow: visible; }
.log-table {
  background: var(--bg-2) !important;

  :deep(thead th) {
    font-size: 11px !important;
    font-weight: 600 !important;
    color: var(--ink-3) !important;
    text-transform: uppercase !important;
    letter-spacing: 0.03em !important;
    padding: 8px 10px !important;
    background: var(--subtle-bg) !important;
    border-bottom: 1px solid var(--separator) !important;
  }

  :deep(tbody td) {
    font-size: 11px !important;
    font-family: 'Inter', sans-serif !important;
    padding: 6px 10px !important;
    border-bottom: 1px solid var(--separator) !important;
    vertical-align: top !important;
    line-height: 1.5 !important;
  }

  :deep(.q-table__bottom) {
    font-size: 12px !important;
    border-top: 1px solid var(--separator) !important;
    min-height: 40px !important;
    color: var(--ink-2) !important;
    padding: 4px 10px !important;
  }

  :deep(.q-table__bottom .q-table__control) {
    font-size: 12px !important;
  }

  :deep(.q-table__separator) {
    min-width: 8px !important;
  }
}

.log-row-zebra td { background: var(--subtle-bg) !important; }

.log-cell-icon { color: var(--ink-3); margin-right: 4px; vertical-align: middle; }
.log-cell-ts { color: var(--ink-3) !important; white-space: nowrap; width: 90px; }
.log-cell-level { width: 54px; text-align: center !important; }
.log-cell-ns { color: var(--accent) !important; white-space: nowrap; width: 120px; }
.log-cell-pod { color: var(--ink-3) !important; white-space: nowrap; width: 150px; max-width: 150px; overflow: hidden; text-overflow: ellipsis; }
.log-cell-msg {
  color: var(--ink-2) !important;
  word-break: break-word !important;
  white-space: pre-wrap !important;
}
.log-msg-error { color: #f87171 !important; }
.log-msg-warn { color: #fbbf24 !important; }
.log-bottom {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  padding: 4px 0;
  font-size: 12px;
}
.log-bottom-count { color: var(--ink-3); font-size: 11px; }
.log-bottom-label { color: var(--ink-3); font-size: 11px; }
.log-bottom-page { color: var(--ink-2); font-size: 11px; min-width: 40px; text-align: center; }

// ── Pod Table ──
.pod-table-wrap { overflow: hidden; border-radius: 8px; border: 1px solid $border; }
.pod-table {
  background: $bg-card !important;
  :deep(thead th) {
    font-size: 11px;
    font-weight: 600;
    color: $ink-2 !important;
    background: $bg-deep !important;
    border-bottom: 1px solid $border;
    text-transform: uppercase;
    letter-spacing: 0.5px;
    padding: 10px 14px !important;
  }
  :deep(tbody td) {
    font-size: 12px;
    color: $ink-1 !important;
    border-bottom: 1px solid $border;
    font-family: 'Inter', sans-serif;
    padding: 8px 14px !important;
    line-height: 1.5;
  }
  :deep(tbody tr:hover td) { background: rgba(255,255,255,0.02) !important; }
  :deep(.q-table__bottom) { color: $ink-3 !important; border-top: 1px solid $border; padding: 8px 14px !important; }
  :deep(.q-field__native), :deep(.q-field__control) { color: $ink-1 !important; }
}
.pod-dot { display: inline-block; width: 7px; height: 7px; border-radius: 50%; margin-right: 8px; }
.dot-running { background: #29cc5f; box-shadow: 0 0 4px #29cc5f; }
.dot-pending { background: #e0a040; }
.dot-failed, .dot-crashloopbackoff, .dot-error { background: #e05050; }
.dot-succeeded, .dot-completed { background: #29cc5f; }
</style>
