<template>
  <div class="iframe-root">
    <!-- Left Sidebar -->
    <div class="iframe-sidebar" style="width:220px;min-width:220px">
      <div class="sidebar-brand">
        <div class="brand-icon">
          <q-icon name="sym_r_storefront" size="18px" color="white" />
        </div>
        <div class="brand-info">
          <div class="brand-title">Market</div>
          <div class="brand-sub">App Store</div>
        </div>
      </div>
      <div class="sidebar-divider"></div>

      <div class="sidebar-nav">
        <div class="nav-item" :class="{ active: activeTab === 'discover' }" @click="activeTab = 'discover'; activeCategory = 'all'">
          <q-icon name="sym_r_explore" size="17px" class="nav-icon" />
          <span class="nav-text">Discover</span>
        </div>
        <div class="nav-item" :class="{ active: activeTab === 'installed' }" @click="activeTab = 'installed'">
          <q-icon name="sym_r_download_done" size="17px" class="nav-icon" />
          <span class="nav-text">Installed</span>
          <span v-if="installedApps.length > 0" class="nav-badge">{{ installedApps.length }}</span>
        </div>
        <div class="market-section-label">Categories</div>
        <div
          v-for="cat in categories"
          :key="cat.name"
          class="nav-item"
          :class="{ active: activeTab === 'discover' && activeCategory === cat.name }"
          @click="selectCategory(cat.name)"
        >
          <q-icon :name="categoryIcon(cat.name)" size="17px" class="nav-icon" />
          <span class="nav-text">{{ cat.name }}</span>
          <span class="nav-badge">{{ cat.count }}</span>
        </div>
      </div>
    </div>

    <!-- Main Content -->
    <div class="iframe-content market-content">
      <!-- Toolbar -->
      <div class="market-toolbar">
        <q-input
          v-model="searchQuery"
          outlined
          dense
          placeholder="Search apps..."
          class="market-search"
          clearable
          @clear="searchQuery = ''"
        >
          <template v-slot:prepend>
            <q-icon name="sym_r_search" size="20px" />
          </template>
        </q-input>
        <q-space />
        <div class="sync-area">
          <div v-if="syncStatus.state === 'running'" class="sync-progress">
            <q-spinner-dots size="16px" color="white" />
            <span class="sync-text">Syncing {{ syncStatus.currentApp || '...' }} ({{ syncStatus.syncedApps }}/{{ syncStatus.totalApps }})</span>
          </div>
          <div v-else-if="syncStatus.lastSync" class="sync-last">
            <q-icon name="sym_r_check_circle" size="14px" color="positive" />
            <span class="sync-text">{{ syncStatus.totalApps }} apps synced</span>
          </div>
          <q-btn
            flat dense no-caps
            :label="syncStatus.state === 'running' ? 'Syncing...' : 'Sync'"
            icon="sym_r_sync"
            class="sync-btn"
            :loading="syncStatus.state === 'running'"
            :disable="syncStatus.state === 'running'"
            @click="triggerSync"
          />
        </div>
      </div>

      <!-- Discover View -->
      <div v-if="activeTab === 'discover'" class="market-grid-area">
        <div class="market-section-title" v-if="activeCategory === 'all'">
          All Apps
        </div>
        <div class="market-section-title" v-else>
          {{ activeCategory }}
        </div>

        <div v-if="loading" class="market-grid">
          <div v-for="n in 8" :key="n" class="app-card-skeleton card">
            <q-skeleton type="rect" width="48px" height="48px" class="skeleton-icon" />
            <q-skeleton type="text" width="70%" class="q-mt-sm" />
            <q-skeleton type="text" width="50%" />
            <q-skeleton type="text" width="90%" class="q-mt-xs" />
            <q-skeleton type="QBtn" width="72px" height="28px" class="q-mt-sm" />
          </div>
        </div>

        <div v-else-if="filteredApps.length === 0" class="market-empty">
          <q-icon name="sym_r_search_off" size="64px" color="grey-7" />
          <div class="empty-text">No apps found</div>
        </div>

        <div v-else class="market-grid">
          <div
            v-for="app in filteredApps"
            :key="app.name"
            class="app-card card"
            @click="openDetail(app)"
          >
            <div class="app-card-header">
              <img
                :src="app.icon || FALLBACK_ICON"
                :alt="app.title"
                class="app-icon"
                @error="onIconError"
              />
              <div class="app-card-meta">
                <div class="app-card-title">{{ app.title }}</div>
                <div class="app-card-developer">{{ app.developer || 'Unknown' }}</div>
              </div>
            </div>
            <div class="app-card-desc">{{ app.description }}</div>
            <div class="app-card-footer">
              <div class="app-card-tags">
                <q-badge
                  v-for="cat in (app.categories || []).slice(0, 2)"
                  :key="cat"
                  :label="cat"
                  class="app-tag"
                />
              </div>
              <!-- Unified app state display -->
              <q-btn v-if="getAppDisplayState(app.name, app.hasChart) === 'running'" flat dense no-caps label="Open" class="app-btn-open" @click.stop="openApp(app.name)" />
              <div v-else-if="getAppDisplayState(app.name, app.hasChart) === 'starting'" class="app-install-progress">
                <q-spinner-dots size="16px" color="indigo-4" />
                <span class="app-progress-text">Starting...</span>
              </div>
              <span v-else-if="getAppDisplayState(app.name, app.hasChart) === 'pending'" class="app-state-failed" style="color:var(--warning)">Pending</span>
              <span v-else-if="getAppDisplayState(app.name, app.hasChart) === 'failed'" class="app-state-failed">Failed</span>
              <span v-else-if="getAppDisplayState(app.name, app.hasChart) === 'stopped'" class="app-state-failed">Stopped</span>
              <div v-else-if="getAppDisplayState(app.name, app.hasChart) === 'uninstalling'" class="app-install-progress">
                <q-linear-progress :value="installProgress[app.name] ? installProgress[app.name].step / installProgress[app.name].totalSteps : 0.3" color="negative" track-color="grey-9" rounded size="4px" class="app-progress-bar" :indeterminate="!installProgress[app.name]" />
                <span class="app-progress-text">{{ installProgress[app.name]?.detail || 'Removing...' }}</span>
              </div>
              <div v-else-if="getAppDisplayState(app.name, app.hasChart) === 'downloading' || getAppDisplayState(app.name, app.hasChart) === 'installing'" class="app-install-progress">
                <q-linear-progress :value="installProgress[app.name] ? installProgress[app.name].step / installProgress[app.name].totalSteps : 0.2" color="indigo-4" track-color="grey-9" rounded size="4px" class="app-progress-bar" :indeterminate="!installProgress[app.name]" />
                <span class="app-progress-text">{{ installProgress[app.name]?.detail || (getAppDisplayState(app.name, app.hasChart) === 'downloading' ? 'Downloading...' : 'Installing...') }}</span>
              </div>
              <q-btn v-else-if="getAppDisplayState(app.name, app.hasChart) === 'not_installed'" flat dense no-caps :label="app.requiredDisk ? 'Install \u00b7 ' + app.requiredDisk : 'Install'" class="app-btn-install" @click.stop="installApp(app)" />
              <span v-else-if="getAppDisplayState(app.name, app.hasChart) === 'no_chart'" class="app-no-chart">Not synced</span>
            </div>
          </div>
        </div>
      </div>

      <!-- Installed View -->
      <div v-else class="market-grid-area">
        <div class="market-section-title">Installed Apps</div>

        <div v-if="installedApps.length === 0" class="market-empty">
          <q-icon name="sym_r_apps" size="64px" color="grey-7" />
          <div class="empty-text">No apps installed yet</div>
        </div>

        <div v-else class="market-grid">
          <div
            v-for="app in installedAppsDetail"
            :key="app.name"
            class="app-card card"
            @click="openDetail(app)"
          >
            <div class="app-card-header">
              <img
                :src="app.icon || FALLBACK_ICON"
                :alt="app.title"
                class="app-icon"
                @error="onIconError"
              />
              <div class="app-card-meta">
                <div class="app-card-title">{{ app.title }}</div>
                <div class="app-card-developer">{{ app.developer || 'Unknown' }}</div>
              </div>
            </div>
            <div class="app-card-desc">{{ app.description }}</div>
            <div class="app-card-footer">
              <q-badge :label="getAppDisplayState(app.name)" :class="'status-badge status-' + getAppDisplayState(app.name)" />
              <div class="app-card-footer-actions">
                <template v-if="getAppDisplayState(app.name) === 'running'">
                  <q-btn flat dense no-caps label="Open" class="app-btn-open" @click.stop="openApp(app.name)" />
                  <q-btn flat dense no-caps label="Uninstall" class="app-btn-uninstall" @click.stop="confirmUninstall(app)" />
                </template>
                <div v-else-if="getAppDisplayState(app.name) === 'starting'" class="app-install-progress">
                  <q-spinner-dots size="16px" color="indigo-4" /><span class="app-progress-text">Starting...</span>
                </div>
                <div v-else-if="getAppDisplayState(app.name) === 'uninstalling'" class="app-install-progress">
                  <q-linear-progress :value="installProgress[app.name] ? installProgress[app.name].step / installProgress[app.name].totalSteps : 0.3" color="negative" track-color="grey-9" rounded size="4px" :indeterminate="!installProgress[app.name]" />
                  <span class="app-progress-text">{{ installProgress[app.name]?.detail || 'Removing...' }}</span>
                </div>
                <span v-else-if="getAppDisplayState(app.name) === 'pending'" class="app-state-failed" style="color:var(--warning)">Pending</span>
                <span v-else-if="getAppDisplayState(app.name) === 'failed'" class="app-state-failed">Failed</span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- App Detail Page (replaces content area) -->
    <div v-if="detailApp" class="detail-page">
      <div class="detail-page-scroll">
        <!-- Back -->
        <div class="detail-back" @click="detailApp = null">
          <q-icon name="sym_r_arrow_back" size="16px" />
          <span>Back</span>
        </div>

        <!-- Hero banner -->
        <div class="detail-hero-card">
          <img :src="detailApp.icon || FALLBACK_ICON" :alt="detailApp.title" class="detail-icon" @error="onIconError" />
          <div class="detail-hero-info">
            <div class="detail-title">{{ detailData?.title || detailApp.title }}</div>
            <div class="detail-developer">{{ detailData?.developer || detailApp.developer || 'Unknown developer' }}</div>
          </div>
          <div class="detail-hero-actions">
            <template v-if="appStates[detailApp.name] === 'uninstalling'">
              <div class="detail-progress-wrap">
                <q-linear-progress :value="installProgress[detailApp.name] ? installProgress[detailApp.name].step / installProgress[detailApp.name].totalSteps : 0.3" color="negative" track-color="grey-9" rounded size="5px" :indeterminate="!installProgress[detailApp.name]" style="width:160px" />
                <span class="detail-progress-text">{{ installProgress[detailApp.name]?.detail || 'Removing...' }}</span>
              </div>
            </template>
            <template v-else-if="isInstalled(detailApp.name) && appStates[detailApp.name] !== 'installing' && appStates[detailApp.name] !== 'downloading'">
              <q-btn unelevated no-caps label="Open" class="btn-primary" icon="sym_r_open_in_new" @click="openApp(detailApp.name)" style="padding:6px 24px" />
              <q-btn flat no-caps label="Uninstall" class="btn-danger" @click="confirmUninstall(detailApp)" />
            </template>
            <template v-else-if="appStates[detailApp.name] === 'downloading' || appStates[detailApp.name] === 'installing' || installingSet.has(detailApp.name)">
              <div class="detail-progress-wrap">
                <q-linear-progress :value="installProgress[detailApp.name] ? installProgress[detailApp.name].step / installProgress[detailApp.name].totalSteps : 0.2" color="primary" track-color="grey-9" rounded size="5px" :indeterminate="!installProgress[detailApp.name]" style="width:160px" />
                <span class="detail-progress-text">{{ installProgress[detailApp.name]?.detail || (appStates[detailApp.name] === 'downloading' ? 'Downloading...' : 'Installing...') }}</span>
              </div>
            </template>
            <template v-else-if="detailApp.hasChart">
              <q-btn unelevated no-caps label="Install" class="btn-primary" icon="sym_r_download" @click="installApp(detailApp)" style="padding:6px 24px" />
            </template>
            <template v-else>
              <span class="detail-no-chart">Chart not synced</span>
            </template>
          </div>
        </div>

        <!-- Stats strip with icons -->
        <div class="detail-stats-strip">
          <div class="stat-item" v-if="detailData?.version || detailApp.version">
            <q-icon name="sym_r_new_releases" size="18px" class="stat-icon" />
            <span class="stat-val">v{{ detailData?.version || detailApp.version }}</span>
            <span class="stat-lbl">Version</span>
          </div>
          <div class="stat-item" v-if="detailData?.installCount">
            <q-icon name="sym_r_download" size="18px" class="stat-icon" />
            <span class="stat-val">{{ detailData.installCount.toLocaleString() }}</span>
            <span class="stat-lbl">Downloads</span>
          </div>
          <div class="stat-item" v-if="detailData?.language?.length">
            <q-icon name="sym_r_translate" size="18px" class="stat-icon" />
            <span class="stat-val">{{ detailData.language.join(', ').substring(0, 12) }}</span>
            <span class="stat-lbl">Language</span>
          </div>
          <div class="stat-item" v-if="detailData?.requiredMemory">
            <q-icon name="sym_r_memory" size="18px" class="stat-icon" />
            <span class="stat-val">{{ detailData.requiredMemory }}</span>
            <span class="stat-lbl">Memory</span>
          </div>
          <div class="stat-item" v-if="detailData?.requiredDisk">
            <q-icon name="sym_r_storage" size="18px" class="stat-icon" />
            <span class="stat-val">{{ detailData.requiredDisk }}</span>
            <span class="stat-lbl">Disk</span>
          </div>
          <div class="stat-item" v-if="detailData?.requiredCpu">
            <q-icon name="sym_r_developer_board" size="18px" class="stat-icon" />
            <span class="stat-val">{{ detailData.requiredCpu }}</span>
            <span class="stat-lbl">CPU</span>
          </div>
          <div class="stat-item" v-if="detailData?.requiredGpu">
            <q-icon name="sym_r_memory_alt" size="18px" class="stat-icon" />
            <span class="stat-val">{{ detailData.requiredGpu }}</span>
            <span class="stat-lbl">GPU</span>
          </div>
        </div>

        <!-- Screenshots -->
        <div class="detail-screenshots-wrap" v-if="detailData?.promoteImage?.length">
          <div class="detail-screenshots">
            <img v-for="(img, idx) in detailData.promoteImage" :key="idx" :src="img" class="detail-screenshot" @click="previewImg = img" @error="(e: Event) => ((e.target as HTMLImageElement).style.display = 'none')" />
          </div>
        </div>

        <!-- Two-column: description + info sidebar -->
        <div class="detail-body">
          <div class="detail-main">
            <div class="detail-section-title">About this App</div>
            <div class="detail-content-card">
              <div class="detail-description" v-if="detailLoading">
                <q-skeleton v-for="n in 6" :key="n" type="text" :width="(100 - n * 5) + '%'" class="q-mb-xs" />
              </div>
              <div class="detail-description" v-else v-html="renderMarkdown(detailData?.fullDescription || detailData?.description || detailApp.description || 'No description available.')" />
            </div>

            <!-- Permissions -->
            <template v-if="detailData?.permission">
              <div class="detail-section-title" style="margin-top:20px">Required Permissions</div>
              <div class="detail-content-card">
                <div class="detail-permissions">
                  <div class="perm-item" v-if="detailData.permission.appData">
                    <q-icon name="sym_r_folder" size="18px" class="perm-icon" />
                    <div class="perm-info">
                      <div class="perm-name">Access to Files</div>
                      <div class="perm-desc">This app can access your file storage</div>
                    </div>
                  </div>
                  <div class="perm-item" v-if="detailData.permission.sysData?.length">
                    <q-icon name="sym_r_settings" size="18px" class="perm-icon" />
                    <div class="perm-info">
                      <div class="perm-name">Shared App</div>
                      <div class="perm-desc">Uses system services: {{ detailData.permission.sysData.map((s: any) => s.group || s.svc).join(', ') }}</div>
                    </div>
                  </div>
                </div>
              </div>
            </template>
          </div>

          <div class="detail-sidebar">
            <div class="detail-section-title">Information</div>
            <div class="detail-info-card">
              <div class="di-row" v-if="detailData?.website">
                <span class="di-label">Documents</span>
                <a :href="detailData.website" target="_blank" class="di-link">Website</a>
              </div>
              <div class="di-row" v-if="detailData?.doc">
                <span class="di-label">Documentation</span>
                <a :href="detailData.doc" target="_blank" class="di-link">Docs</a>
              </div>
              <div class="di-row" v-if="detailData?.sourceCode">
                <span class="di-label">Source code</span>
                <a :href="detailData.sourceCode" target="_blank" class="di-link">GitHub</a>
              </div>
              <div class="di-row" v-if="detailData?.version || detailApp.version">
                <span class="di-label">App version</span>
                <span class="di-value">{{ detailData?.version || detailApp.version }}</span>
              </div>
              <div class="di-row" v-if="detailData?.developer || detailApp.developer">
                <span class="di-label">Developer</span>
                <span class="di-value">{{ detailData?.developer || detailApp.developer }}</span>
              </div>
              <div class="di-row" v-if="(detailData?.categories || detailApp.categories || []).length">
                <span class="di-label">Category</span>
                <span class="di-value">{{ (detailData?.categories || detailApp.categories || []).join(', ') }}</span>
              </div>
              <div class="di-row" v-if="detailData?.language?.length">
                <span class="di-label">Language</span>
                <span class="di-value">{{ detailData.language.join(', ') }}</span>
              </div>
              <div class="di-row" v-if="detailData?.supportArch?.length">
                <span class="di-label">Architecture</span>
                <span class="di-value">{{ detailData.supportArch.join(', ') }}</span>
              </div>
              <div class="di-row" v-if="detailData?.license?.length">
                <span class="di-label">License</span>
                <span class="di-value">{{ detailData.license.map((l: any) => l.text).join(', ') }}</span>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Image preview overlay -->
      <div v-if="previewImg" class="preview-overlay" @click="previewImg = ''">
        <img :src="previewImg" class="preview-img" />
      </div>
    </div>
  </div>
</template>

<script lang="ts" setup>
import { ref, computed, onMounted, onUnmounted, reactive, watch } from 'vue';
import { api } from 'boot/axios';
import { useQuasar } from 'quasar';
const $q = useQuasar();

interface MarketApp {
  name: string;
  title: string;
  chartName?: string;
  description: string;
  fullDescription?: string;
  icon: string;
  version: string;
  categories: string[];
  developer: string;
  promoteImage?: string[];
  requiredMemory?: string;
  requiredCpu?: string;
  requiredDisk?: string;
  requiredGpu?: string;
  hasChart?: boolean;
}

interface Category {
  name: string;
  count: number;
}

interface InstalledApp {
  name: string;
  state: string;
  status?: string;
}

const loading = ref(true);
const searchQuery = ref('');
const userZone = ref('');
const activeTab = ref<'discover' | 'installed'>('discover');
const activeCategory = ref('all');
const apps = ref<MarketApp[]>([]);
const categories = ref<Category[]>([]);
const installedApps = ref<InstalledApp[]>([]);
const detailApp = ref<MarketApp | null>(null);
const detailData = ref<MarketApp | null>(null);
const detailLoading = ref(false);
const previewImg = ref('');
const installingSet = reactive(new Set<string>());
const appStates = reactive<Record<string, string>>({});
const installProgress = reactive<Record<string, { step: number; totalSteps: number; detail: string }>>({});

const syncStatus = reactive({
  state: '' as string,
  totalApps: 0,
  syncedApps: 0,
  currentApp: '',
  lastSync: '',
  errors: [] as string[],
});

let ws: WebSocket | null = null;
let syncPollTimer: ReturnType<typeof setInterval> | null = null;

const installedStatusMap = computed(() => {
  const map: Record<string, string> = {};
  installedApps.value.forEach((a) => {
    map[a.name] = a.state || a.status || 'unknown';
  });
  return map;
});

// Single source of truth for app display state
// Returns: 'downloading' | 'installing' | 'running' | 'starting' | 'failed' | 'uninstalling' | 'stopped' | 'not_installed' | 'no_chart'
function getAppDisplayState(name: string, hasChart?: boolean): string {
  // WebSocket/realtime state takes priority
  const ws = appStates[name];
  if (ws === 'downloading' || ws === 'installing' || ws === 'uninstalling' || ws === 'failed') return ws;
  if (installingSet.has(name)) return 'installing';

  // Check installed status
  const installed = installedStatusMap.value[name];
  if (installed) {
    if (ws === 'running' || installed === 'running') return 'running';
    if (installed === 'failed' || installed === 'install_failed' || installed === 'installFailed') return 'failed';
    if (installed === 'pending') return 'pending';
    if (installed === 'stopped') return 'stopped';
    if (installed === 'uninstalling') return 'uninstalling';
    return 'starting'; // installed but not running yet
  }

  // Not installed
  if (hasChart === false) return 'no_chart';
  return 'not_installed';
}

const installedAppsDetail = computed(() => {
  const names = new Set(installedApps.value.map((a) => a.name));
  return apps.value.filter((a) => names.has(a.name));
});

const filteredApps = computed(() => {
  let list = apps.value;
  if (activeCategory.value !== 'all') {
    list = list.filter(
      (a) =>
        a.categories &&
        a.categories.some(
          (c) => c.toLowerCase() === activeCategory.value.toLowerCase()
        )
    );
  }
  if (searchQuery.value) {
    const q = searchQuery.value.toLowerCase();
    list = list.filter(
      (a) =>
        a.title.toLowerCase().includes(q) ||
        a.name.toLowerCase().includes(q) ||
        (a.description && a.description.toLowerCase().includes(q))
    );
  }
  return list;
});

function isInstalled(name: string): boolean {
  return installedApps.value.some((a) => a.name === name);
}

function selectCategory(name: string) {
  activeTab.value = 'discover';
  activeCategory.value = name;
}

function appUrl(name: string): string {
  const host = window.location.hostname;
  const isIP = /^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}$/.test(host);
  // On subdomain (e.g. market.admin.olares.local), replace first part with app name
  if (!isIP && host.split('.').length >= 3) {
    return 'https://' + name + '.' + host.split('.').slice(1).join('.');
  }
  // On IP or short hostname, use userZone
  if (userZone.value) {
    return 'https://' + name + '.' + userZone.value;
  }
  return 'https://' + name + '.' + host;
}

function openApp(name: string) {
  window.open(appUrl(name), '_blank');
}

const categoryIcons: Record<string, string> = {
  Productivity: 'sym_r_work',
  Utilities: 'sym_r_build',
  Entertainment: 'sym_r_sports_esports',
  'Social Network': 'sym_r_forum',
  Blockchain: 'sym_r_token',
  AI: 'sym_r_smart_toy',
  Media: 'sym_r_movie',
  Development: 'sym_r_code',
  'Developer Tools': 'sym_r_code',
  Communication: 'sym_r_chat',
  Security: 'sym_r_shield',
  Creativity: 'sym_r_palette',
  Fun: 'sym_r_celebration',
  Lifestyle: 'sym_r_favorite',
};

function categoryIcon(name: string): string {
  return categoryIcons[name] || 'sym_r_category';
}

const FALLBACK_ICON = 'data:image/svg+xml;base64,' + btoa('<svg xmlns="http://www.w3.org/2000/svg" width="128" height="128"><rect width="128" height="128" rx="24" fill="#2f3040"/><text x="64" y="76" text-anchor="middle" font-size="48" font-family="sans-serif" fill="#636366">?</text></svg>');

function onIconError(e: Event) {
  const img = e.target as HTMLImageElement;
  if (img.src !== FALLBACK_ICON) {
    img.src = FALLBACK_ICON;
  }
}

// renderMarkdown converts simple markdown to HTML for display.
// SECURITY: HTML entities are escaped FIRST (line 1), so injected <script> etc. are safe.
// Only safe tags are generated (h2-h4, strong, em, code, li, ul, p, br).
// Do NOT add link/image rendering without XSS sanitization.
function renderMarkdown(text: string): string {
  if (!text) return '';
  return text
    .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
    .replace(/^### (.+)$/gm, '<h4>$1</h4>')
    .replace(/^## (.+)$/gm, '<h3>$1</h3>')
    .replace(/^# (.+)$/gm, '<h2>$1</h2>')
    .replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
    .replace(/\*(.+?)\*/g, '<em>$1</em>')
    .replace(/`(.+?)`/g, '<code>$1</code>')
    .replace(/^- (.+)$/gm, '<li>$1</li>')
    .replace(/(<li>.*<\/li>)/s, '<ul>$1</ul>')
    .replace(/\n\n/g, '</p><p>')
    .replace(/\n/g, '<br>')
    .replace(/^/, '<p>').replace(/$/, '</p>');
}

async function openDetail(app: MarketApp) {
  detailApp.value = app;
  detailData.value = null;
  detailLoading.value = true;
  try {
    const res: any = await api.get('/api/market/app/' + app.name);
    detailData.value = res.data || null;
  } catch {
    detailData.value = app;
  } finally {
    detailLoading.value = false;
  }
}

async function fetchApps() {
  try {
    const res: any = await api.get('/api/market/apps');
    apps.value = res.data || [];
  } catch {
    apps.value = [];
  }
}

async function fetchCategories() {
  try {
    const res: any = await api.get('/api/market/categories');
    categories.value = res.data || [];
  } catch {
    categories.value = [];
  }
}

async function fetchInstalled() {
  try {
    const res: any = await api.get('/api/apps/apps');
    installedApps.value = res.data || [];
  } catch {
    installedApps.value = [];
  }
}

async function installApp(app: MarketApp) {
  installingSet.add(app.name);
  appStates[app.name] = 'downloading';
  try {
    await api.post('/api/apps/install', { name: app.name, chart: app.chartName || app.name });
    // WebSocket will update the state; fall back to polling
    let attempts = 0;
    const poll = setInterval(async () => {
      attempts++;
      await fetchInstalled();
      if (isInstalled(app.name) || attempts >= 60) {
        clearInterval(poll);
        installingSet.delete(app.name);
        delete appStates[app.name];
      }
    }, 3000);
  } catch (e: any) {
    console.error('Install failed:', e);
    installingSet.delete(app.name);
    delete appStates[app.name];
    $q.notify({ type: 'negative', message: `Install failed: ${e.message || 'unknown error'}` });
  }
}

function confirmUninstall(app: MarketApp) {
  $q.dialog({
    title: 'Uninstall ' + app.title,
    message: 'Are you sure you want to uninstall ' + app.title + '? This will remove all app data.',
    cancel: true,
    persistent: true,
    dark: true,
    color: 'negative',
    ok: {
      label: 'Uninstall',
      flat: true,
      color: 'negative',
    },
  }).onOk(() => {
    uninstallApp(app);
  });
}

async function uninstallApp(app: MarketApp) {
  try {
    appStates[app.name] = 'uninstalling';
    await api.post('/api/apps/uninstall', { name: app.name });
    // WebSocket will update state to uninstalled; close detail panel
    if (detailApp.value?.name === app.name) {
      detailApp.value = null;
    }
  } catch (e: any) {
    console.error('Uninstall failed:', e);
    delete appStates[app.name];
    $q.notify({ type: 'negative', message: `Uninstall failed: ${e.message || 'unknown error'}` });
  }
}

function connectWebSocket() {
  const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  // Connect to main domain or IP for WS (not the market subdomain)
  let wsHost = window.location.host;
  const hostname = window.location.hostname;
  const isIP = /^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}$/.test(hostname);
  if (!isIP && hostname.split('.').length >= 3) {
    // On subdomain: strip first part to get main domain
    wsHost = hostname.split('.').slice(1).join('.');
  }
  const wsUrl = proto + '//' + wsHost + '/ws';
  try {
    ws = new WebSocket(wsUrl);

    ws.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data);
        if (msg.type === 'app_state' && msg.data) {
          const { name, state } = msg.data as { name: string; state: string };
          if (state === 'running') {
            installingSet.delete(name);
            delete appStates[name];
            delete installProgress[name];
            fetchInstalled();
          } else if (state === 'failed') {
            installingSet.delete(name);
            appStates[name] = 'failed';
            // Keep the error detail from install_progress if available
            const errDetail = installProgress[name]?.detail || '';
            $q.notify({ type: 'negative', message: `${name} failed: ${errDetail || 'installation error'}`, timeout: 10000 });
          } else if (state === 'uninstalling') {
            appStates[name] = state;
          } else if (state === 'uninstalled') {
            delete appStates[name];
            delete installProgress[name];
            installingSet.delete(name);
            installedApps.value = installedApps.value.filter((a) => a.name !== name);
            fetchInstalled();
            $q.notify({ type: 'positive', message: `${name} uninstalled` });
          } else {
            appStates[name] = state;
          }
        }
        if (msg.type === 'install_progress' && msg.data) {
          const d = msg.data as { name: string; step: number; totalSteps: number; detail: string; state: string };
          if (d.state === 'running') {
            delete installProgress[d.name];
          } else {
            installProgress[d.name] = { step: d.step, totalSteps: d.totalSteps, detail: d.detail };
          }
        }
      } catch {
        // ignore non-JSON messages
      }
    };

    ws.onclose = () => {
      // Reconnect after 5s
      setTimeout(() => {
        if (!ws || ws.readyState === WebSocket.CLOSED) {
          connectWebSocket();
        }
      }, 5000);
    };

    ws.onerror = () => {
      ws?.close();
    };
  } catch {
    // WebSocket not available
  }
}

async function triggerSync() {
  try {
    await api.post('/api/market/sync', { source: 'olares' });
    syncStatus.state = 'running';
    startSyncPoll();
  } catch (e: any) {
    $q.notify({ type: 'negative', message: 'Sync failed: ' + (e?.message || 'unknown') });
  }
}

async function fetchSyncStatus() {
  try {
    const r: any = await api.get('/api/market/sync/status');
    const d = r?.data || r || {};
    syncStatus.state = d.state || '';
    syncStatus.totalApps = d.total_apps || 0;
    syncStatus.syncedApps = d.synced_apps || 0;
    syncStatus.currentApp = d.current_app || '';
    syncStatus.lastSync = d.last_sync || '';
    syncStatus.errors = d.errors || [];

    if (d.state === 'done' || d.state === 'error' || d.state === '') {
      stopSyncPoll();
      if (d.state === 'done') {
        // Reload catalog after sync completes
        await Promise.all([fetchApps(), fetchCategories()]);
        $q.notify({ type: 'positive', message: `Synced ${d.total_apps} apps` });
      }
    }
  } catch {}
}

function startSyncPoll() {
  if (syncPollTimer) return;
  syncPollTimer = setInterval(fetchSyncStatus, 2000);
}

function stopSyncPoll() {
  if (syncPollTimer) {
    clearInterval(syncPollTimer);
    syncPollTimer = null;
  }
}

// Watch for detail panel close to clean up
watch(detailApp, (val) => {
  if (!val) {
    detailData.value = null;
    detailLoading.value = false;
  }
});

onMounted(async () => {
  loading.value = true;
  // Fetch user zone for app URLs
  try {
    const r: any = await api.get('/api/user/info');
    userZone.value = r?.zone || r?.terminusName || '';
  } catch {}
  await Promise.all([fetchApps(), fetchCategories(), fetchInstalled(), fetchSyncStatus()]);
  loading.value = false;
  connectWebSocket();
  // If sync is running, start polling
  if (syncStatus.state === 'running') {
    startSyncPoll();
  }
});

onUnmounted(() => {
  if (ws) { ws.close(); ws = null; }
  stopSyncPoll();
});
</script>

<style lang="scss">
/* All styles in components.scss */
</style>
