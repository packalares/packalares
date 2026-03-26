<template>
  <div class="market-root">
    <!-- Left Sidebar -->
    <div class="market-sidebar">
      <div class="sidebar-header">
        <q-icon name="sym_r_storefront" size="28px" color="white" />
        <span class="sidebar-title">Market</span>
      </div>

      <q-list dense class="sidebar-nav">
        <q-item
          clickable
          :active="activeTab === 'discover'"
          active-class="sidebar-item-active"
          class="sidebar-nav-item"
          @click="activeTab = 'discover'; activeCategory = 'all'"
        >
          <q-item-section avatar style="min-width: 36px">
            <q-icon
              name="sym_r_explore"
              size="20px"
              :class="{ 'text-accent-active': activeTab === 'discover' }"
            />
          </q-item-section>
          <q-item-section>
            <q-item-label
              :class="activeTab === 'discover' ? 'text-accent-active' : 'text-ink-1'"
            >
              Discover
            </q-item-label>
          </q-item-section>
        </q-item>

        <q-item
          clickable
          :active="activeTab === 'installed'"
          active-class="sidebar-item-active"
          class="sidebar-nav-item"
          @click="activeTab = 'installed'"
        >
          <q-item-section avatar style="min-width: 36px">
            <q-icon
              name="sym_r_download_done"
              size="20px"
              :class="{ 'text-accent-active': activeTab === 'installed' }"
            />
          </q-item-section>
          <q-item-section>
            <q-item-label
              :class="activeTab === 'installed' ? 'text-accent-active' : 'text-ink-1'"
            >
              Installed
            </q-item-label>
          </q-item-section>
          <q-item-section side v-if="installedApps.length > 0">
            <q-badge
              :label="installedApps.length"
              class="badge-count"
            />
          </q-item-section>
        </q-item>
      </q-list>

      <q-separator class="sidebar-separator" />

      <div class="sidebar-section-label">Categories</div>
      <q-list dense class="sidebar-nav">
        <q-item
          v-for="cat in categories"
          :key="cat.name"
          clickable
          :active="activeTab === 'discover' && activeCategory === cat.name"
          active-class="sidebar-item-active"
          class="sidebar-nav-item"
          @click="selectCategory(cat.name)"
        >
          <q-item-section avatar style="min-width: 36px">
            <q-icon
              :name="categoryIcon(cat.name)"
              size="20px"
              :class="{
                'text-accent-active':
                  activeTab === 'discover' && activeCategory === cat.name,
              }"
            />
          </q-item-section>
          <q-item-section>
            <q-item-label
              :class="
                activeTab === 'discover' && activeCategory === cat.name
                  ? 'text-accent-active'
                  : 'text-ink-1'
              "
            >
              {{ cat.name }}
            </q-item-label>
          </q-item-section>
          <q-item-section side>
            <span class="category-count">{{ cat.count }}</span>
          </q-item-section>
        </q-item>
      </q-list>
    </div>

    <!-- Main Content -->
    <div class="market-content">
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
        <!-- Back button -->
        <div class="detail-back" @click="detailApp = null">
          <q-icon name="sym_r_arrow_back" size="18px" />
          <span>Back to Market</span>
        </div>

        <!-- Hero: icon + title + install -->
        <div class="detail-hero">
          <img
            :src="detailApp.icon || FALLBACK_ICON"
            :alt="detailApp.title"
            class="detail-icon"
            @error="onIconError"
          />
          <div class="detail-hero-info">
            <div class="detail-title">{{ detailData?.title || detailApp.title }}</div>
            <div class="detail-developer">{{ detailData?.developer || detailApp.developer || 'Unknown developer' }}</div>
            <div class="detail-meta-row">
              <span class="detail-version">v{{ detailData?.version || detailApp.version || '1.0' }}</span>
              <q-badge v-for="cat in (detailData?.categories || detailApp.categories || []).slice(0,3)" :key="cat" :label="cat" class="detail-cat-badge" />
            </div>
          </div>
          <div class="detail-hero-actions">
            <template v-if="appStates[detailApp.name] === 'uninstalling'">
              <div class="detail-install-progress">
                <q-linear-progress :value="installProgress[detailApp.name] ? installProgress[detailApp.name].step / installProgress[detailApp.name].totalSteps : 0.3" color="negative" track-color="grey-9" rounded size="6px" :indeterminate="!installProgress[detailApp.name]" style="width:200px" />
                <span class="detail-progress-text">{{ installProgress[detailApp.name]?.detail || 'Removing...' }}</span>
              </div>
            </template>
            <template v-else-if="isInstalled(detailApp.name) && appStates[detailApp.name] !== 'installing' && appStates[detailApp.name] !== 'downloading'">
              <q-btn unelevated no-caps label="Open" class="detail-btn-open" icon="sym_r_open_in_new" @click="openApp(detailApp.name)" />
              <q-btn flat no-caps label="Uninstall" class="detail-btn-uninstall" @click="confirmUninstall(detailApp)" />
            </template>
            <template v-else-if="appStates[detailApp.name] === 'downloading' || appStates[detailApp.name] === 'installing' || installingSet.has(detailApp.name)">
              <div class="detail-install-progress">
                <q-linear-progress :value="installProgress[detailApp.name] ? installProgress[detailApp.name].step / installProgress[detailApp.name].totalSteps : 0.2" color="indigo-4" track-color="grey-9" rounded size="6px" :indeterminate="!installProgress[detailApp.name]" style="width:200px" />
                <span class="detail-progress-text">{{ installProgress[detailApp.name]?.detail || (appStates[detailApp.name] === 'downloading' ? 'Downloading...' : 'Installing...') }}</span>
              </div>
            </template>
            <template v-else-if="detailApp.hasChart">
              <q-btn unelevated no-caps label="Install" class="detail-btn-install" icon="sym_r_download" @click="installApp(detailApp)" />
              <div class="detail-req-hint" v-if="detailData?.requiredMemory || detailData?.requiredDisk">
                <span v-if="detailData?.requiredMemory">Memory: {{ detailData.requiredMemory }}</span>
                <span v-if="detailData?.requiredDisk">Disk: {{ detailData.requiredDisk }}</span>
              </div>
            </template>
            <template v-else>
              <span class="detail-no-chart">Chart not synced — run Sync first</span>
            </template>
          </div>
        </div>

        <!-- Requirements strip -->
        <div class="detail-req-strip" v-if="detailData?.requiredMemory || detailData?.requiredCpu || detailData?.requiredDisk">
          <div class="req-item" v-if="detailData?.requiredMemory"><span class="req-label">Memory</span><span class="req-value">{{ detailData.requiredMemory }}</span></div>
          <div class="req-item" v-if="detailData?.requiredCpu"><span class="req-label">CPU</span><span class="req-value">{{ detailData.requiredCpu }}</span></div>
          <div class="req-item" v-if="detailData?.requiredDisk"><span class="req-label">Disk</span><span class="req-value">{{ detailData.requiredDisk }}</span></div>
          <div class="req-item" v-if="detailData?.requiredGpu"><span class="req-label">GPU</span><span class="req-value">{{ detailData.requiredGpu }}</span></div>
        </div>

        <!-- Screenshots carousel -->
        <div class="detail-screenshots-wrap" v-if="detailData?.promoteImage?.length">
          <div class="detail-screenshots">
            <img
              v-for="(img, idx) in detailData.promoteImage"
              :key="idx"
              :src="img"
              class="detail-screenshot"
              @click="previewImg = img"
              @error="(e: Event) => ((e.target as HTMLImageElement).style.display = 'none')"
            />
          </div>
        </div>

        <!-- Two-column layout: description + sidebar -->
        <div class="detail-body">
          <div class="detail-main">
            <div class="detail-section-title">About this app</div>
            <div class="detail-description" v-if="detailLoading">
              <q-skeleton v-for="n in 6" :key="n" type="text" :width="(100 - n * 5) + '%'" class="q-mb-xs" />
            </div>
            <div class="detail-description" v-else v-html="renderMarkdown(detailData?.fullDescription || detailData?.description || detailApp.description || 'No description available.')" />
          </div>

          <div class="detail-sidebar">
            <div class="detail-info-card">
              <div class="info-item" v-if="detailData?.developer || detailApp.developer">
                <span class="info-item-label">Developer</span>
                <span class="info-item-value">{{ detailData?.developer || detailApp.developer }}</span>
              </div>
              <div class="info-item" v-if="detailData?.version || detailApp.version">
                <span class="info-item-label">Version</span>
                <span class="info-item-value">{{ detailData?.version || detailApp.version }}</span>
              </div>
              <div class="info-item" v-if="detailData?.lastUpdated">
                <span class="info-item-label">Updated</span>
                <span class="info-item-value">{{ detailData.lastUpdated }}</span>
              </div>
              <div class="info-item" v-if="detailData?.installCount">
                <span class="info-item-label">Installs</span>
                <span class="info-item-value">{{ detailData.installCount.toLocaleString() }}</span>
              </div>
              <div class="info-item" v-if="detailData?.supportArch?.length">
                <span class="info-item-label">Architecture</span>
                <span class="info-item-value">{{ detailData.supportArch.join(', ') }}</span>
              </div>
            </div>

            <div class="detail-links" v-if="detailData?.website || detailData?.doc || detailData?.sourceCode">
              <a v-if="detailData?.website" :href="detailData.website" target="_blank" class="detail-link">
                <q-icon name="sym_r_language" size="14px" /><span>Website</span>
              </a>
              <a v-if="detailData?.doc" :href="detailData.doc" target="_blank" class="detail-link">
                <q-icon name="sym_r_description" size="14px" /><span>Documentation</span>
              </a>
              <a v-if="detailData?.sourceCode" :href="detailData.sourceCode" target="_blank" class="detail-link">
                <q-icon name="sym_r_code" size="14px" /><span>Source Code</span>
              </a>
            </div>

            <div class="detail-license" v-if="detailData?.license?.length">
              <span class="info-item-label">License</span>
              <span class="info-item-value" v-for="l in detailData.license" :key="l.text">{{ l.text }}</span>
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
    map[a.name] = a.state || a.status || 'running';
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
    if (installed === 'failed' || installed === 'install_failed') return 'failed';
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

<style lang="scss" scoped>
.market-root {
  display: flex;
  width: 100%;
  height: 100vh;
  background-color: var(--bg-1);
  position: relative;
  overflow: hidden;
}

/* -- Sidebar -- */
.market-sidebar {
  width: 240px;
  min-width: 240px;
  height: 100%;
  background-color: var(--bg-1);
  border-right: 1px solid var(--separator);
  display: flex;
  flex-direction: column;
  padding: 12px 8px;
  overflow-y: auto;
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

.sidebar-separator {
  margin: 8px 12px;
  background-color: var(--separator);
}

.sidebar-section-label {
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  color: var(--ink-3);
  padding: 8px 16px 4px;
}

.category-count {
  font-size: 12px;
  color: var(--ink-3);
}

.badge-count {
  background-color: var(--accent);
  color: #fff;
  font-size: 11px;
  padding: 2px 6px;
  border-radius: 10px;
}

/* -- Main Content -- */
.market-content {
  flex: 1;
  height: 100%;
  overflow-y: auto;
  background-color: var(--bg-1);
  display: flex;
  flex-direction: column;
}

.market-toolbar {
  padding: 20px 32px 0;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  gap: 12px;
}

.sync-area {
  display: flex;
  align-items: center;
  gap: 10px;
}

.sync-progress, .sync-last {
  display: flex;
  align-items: center;
  gap: 6px;
}

.sync-text {
  font-size: 12px;
  color: var(--ink-2);
  white-space: nowrap;
}

.sync-btn {
  background: var(--accent-soft) !important;
  color: var(--accent) !important;
  border-radius: var(--radius-sm) !important;
  font-size: 12px !important;
  font-weight: 600 !important;
}

.market-search {
  max-width: 400px;

  :deep(.q-field__control) {
    background: var(--bg-2);
    border-color: var(--separator);
    border-radius: 8px;
    color: var(--ink-1);
  }

  :deep(.q-field__native) {
    color: var(--ink-1);
  }

  :deep(.q-field__prepend),
  :deep(.q-field__append) {
    color: var(--ink-3);
  }

  :deep(.q-field--outlined .q-field__control:before) {
    border-color: var(--separator);
  }

  :deep(.q-field--outlined .q-field__control:hover:before) {
    border-color: var(--accent);
  }

  :deep(.q-field--outlined.q-field--focused .q-field__control:after) {
    border-color: var(--accent);
  }
}

.market-grid-area {
  flex: 1;
  padding: 20px 32px 32px;
}

.market-section-title {
  font-size: 20px;
  font-weight: 600;
  color: var(--ink-1);
  margin-bottom: 20px;
}

.market-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 16px;
}

.market-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 80px 20px;
  gap: 12px;

  .empty-text {
    font-size: 14px;
    color: var(--ink-3);
  }
}

/* -- App Card -- */
.app-card {
  padding: 16px;
  cursor: pointer;
  transition: border-color 0.15s, transform 0.15s;
  display: flex;
  flex-direction: column;

  &:hover {
    border-color: var(--accent);
    transform: translateY(-2px);
  }
}

.app-card-skeleton {
  padding: 16px;

  .skeleton-icon {
    border-radius: 12px;
  }
}

.app-card-header {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 10px;
}

.app-icon {
  width: 48px;
  height: 48px;
  border-radius: 12px;
  object-fit: cover;
  flex-shrink: 0;
  background: var(--bg-3);
}

.app-card-meta {
  flex: 1;
  min-width: 0;
}

.app-card-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--ink-1);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.app-card-developer {
  font-size: 12px;
  color: var(--ink-3);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.app-card-desc {
  font-size: 12px;
  line-height: 1.4;
  color: var(--ink-2);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  min-height: 17px;
  margin-bottom: 12px;
}

.app-card-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-top: auto;
}

.app-card-footer-actions {
  display: flex;
  gap: 4px;
}

.app-card-tags {
  display: flex;
  gap: 4px;
  flex: 1;
  min-width: 0;
  overflow: hidden;
}

.app-tag {
  background: var(--bg-3) !important;
  color: var(--ink-3) !important;
  border-radius: 4px;
  font-size: 10px;
  padding: 1px 6px;
  white-space: nowrap;
}

.app-install-progress {
  display: flex;
  flex-direction: column;
  gap: 3px;
  min-width: 100px;
  max-width: 140px;
}

.app-progress-bar {
  border-radius: 2px;
}

.app-progress-text {
  font-size: 10px;
  color: var(--ink-3);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.detail-install-progress {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.detail-progress-text {
  font-size: 13px;
  color: var(--ink-2);
}

.app-no-chart {
  font-size: 11px;
  color: var(--ink-3);
  padding: 2px 8px;
}

.app-state-failed {
  font-size: 11px;
  color: var(--negative);
  font-weight: 600;
  padding: 2px 8px;
}

.detail-req-hint {
  display: flex;
  gap: 12px;
  font-size: 11px;
  color: var(--ink-3);

  span {
    white-space: nowrap;
  }
}

.detail-no-chart {
  font-size: 13px;
  color: var(--ink-3);
  padding: 8px 0;
}

.app-btn-install {
  background: var(--accent) !important;
  color: #fff !important;
  border-radius: 6px;
  font-size: 12px;
  font-weight: 500;
  padding: 2px 14px;
  flex-shrink: 0;
}

.app-btn-installing {
  background: var(--bg-3) !important;
  color: var(--ink-3) !important;
  border-radius: 6px;
  font-size: 12px;
  padding: 2px 14px;
  flex-shrink: 0;
}

.app-btn-open {
  background: var(--positive) !important;
  color: #fff !important;
  border-radius: 6px;
  font-size: 12px;
  font-weight: 500;
  padding: 2px 14px;
  flex-shrink: 0;
}

.app-btn-uninstall {
  color: var(--negative) !important;
  font-size: 12px;
  padding: 2px 10px;
  flex-shrink: 0;
}

.status-badge {
  font-size: 11px;
  padding: 2px 8px;
  border-radius: 4px;
  text-transform: capitalize;
}

.status-running {
  background: rgba(41, 204, 95, 0.15) !important;
  color: var(--positive) !important;
}

.status-stopped {
  background: rgba(255, 77, 77, 0.15) !important;
  color: var(--negative) !important;
}

.status-pending {
  background: rgba(254, 190, 1, 0.15) !important;
  color: var(--warning) !important;
}

/* -- App Detail Page -- */
.detail-page {
  position: absolute;
  top: 0;
  left: 240px;
  right: 0;
  bottom: 0;
  background: var(--bg-1);
  z-index: 50;
  display: flex;
  flex-direction: column;
}

.detail-page-scroll {
  flex: 1;
  overflow-y: auto;
  padding: 24px 40px 48px;
  max-width: 960px;
}

.detail-back {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  font-weight: 500;
  color: var(--ink-3);
  cursor: pointer;
  padding: 4px 0;
  margin-bottom: 20px;
  transition: color 0.1s;

  &:hover { color: var(--ink-1); }
}

.detail-hero {
  display: flex;
  align-items: flex-start;
  gap: 20px;
  margin-bottom: 24px;
}

.detail-icon {
  width: 88px;
  height: 88px;
  border-radius: 22px;
  object-fit: cover;
  flex-shrink: 0;
  background: var(--bg-3);
  box-shadow: var(--shadow-card);
}

.detail-hero-info {
  flex: 1;
  min-width: 0;
  padding-top: 4px;
}

.detail-title {
  font-size: 24px;
  font-weight: 700;
  color: var(--ink-1);
  letter-spacing: -0.02em;
  line-height: 1.2;
}

.detail-developer {
  font-size: 14px;
  color: var(--ink-2);
  margin-top: 4px;
}

.detail-meta-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 8px;
}

.detail-version {
  font-size: 12px;
  font-weight: 600;
  color: var(--accent);
  background: var(--accent-soft);
  padding: 2px 8px;
  border-radius: 5px;
}

.detail-cat-badge {
  background: rgba(255,255,255,0.06) !important;
  color: var(--ink-3) !important;
  font-size: 11px !important;
  border-radius: 4px !important;
  padding: 2px 8px !important;
}

.detail-hero-actions {
  display: flex;
  flex-direction: column;
  gap: 8px;
  flex-shrink: 0;
  padding-top: 4px;
}

.detail-btn-install {
  background: var(--accent-bold) !important;
  color: #fff !important;
  border-radius: var(--radius-sm) !important;
  padding: 8px 28px !important;
  font-weight: 600 !important;
  font-size: 14px !important;
  box-shadow: 0 1px 3px rgba(99,102,241,0.35) !important;
}

.detail-btn-open {
  background: var(--positive) !important;
  color: #fff !important;
  border-radius: var(--radius-sm) !important;
  padding: 8px 28px !important;
  font-weight: 600 !important;
  font-size: 14px !important;
}

.detail-btn-uninstall {
  background: var(--negative-soft) !important;
  color: var(--negative) !important;
  border-radius: var(--radius-sm) !important;
  font-size: 12px !important;
}

/* Requirements strip */
.detail-req-strip {
  display: flex;
  gap: 20px;
  padding: 14px 20px;
  background: var(--bg-2);
  border-radius: var(--radius);
  border: 1px solid var(--border);
  margin-bottom: 24px;
}

.req-item {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.req-label {
  font-size: 11px;
  font-weight: 600;
  color: var(--ink-3);
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.req-value {
  font-size: 13px;
  font-weight: 600;
  color: var(--ink-1);
  font-family: 'JetBrains Mono', monospace;
}

/* Screenshots */
.detail-screenshots-wrap {
  margin-bottom: 28px;
}

.detail-screenshots {
  display: flex;
  gap: 10px;
  overflow-x: auto;
  padding-bottom: 8px;
  scroll-snap-type: x mandatory;

  &::-webkit-scrollbar { height: 4px; }
  &::-webkit-scrollbar-thumb { background: rgba(255,255,255,0.1); border-radius: 2px; }
}

.detail-screenshot {
  height: 240px;
  border-radius: var(--radius);
  object-fit: cover;
  flex-shrink: 0;
  cursor: pointer;
  scroll-snap-align: start;
  transition: transform 0.15s, box-shadow 0.15s;
  box-shadow: var(--shadow-card);

  &:hover {
    transform: scale(1.02);
    box-shadow: var(--shadow-elevated);
  }
}

/* Two-column body */
.detail-body {
  display: flex;
  gap: 32px;
}

.detail-main {
  flex: 1;
  min-width: 0;
}

.detail-section-title {
  font-size: 11px;
  font-weight: 700;
  color: var(--ink-3);
  text-transform: uppercase;
  letter-spacing: 0.06em;
  margin-bottom: 12px;
}

.detail-description {
  font-size: 14px;
  line-height: 1.7;
  color: var(--ink-1);

  :deep(h2) { font-size: 18px; font-weight: 700; margin: 20px 0 8px; color: var(--ink-1); }
  :deep(h3) { font-size: 15px; font-weight: 600; margin: 16px 0 6px; color: var(--ink-1); }
  :deep(h4) { font-size: 14px; font-weight: 600; margin: 12px 0 4px; color: var(--ink-2); }
  :deep(strong) { font-weight: 600; color: var(--ink-1); }
  :deep(code) { font-family: 'JetBrains Mono', monospace; font-size: 12px; background: var(--bg-3); padding: 1px 5px; border-radius: 3px; }
  :deep(ul) { padding-left: 20px; margin: 8px 0; }
  :deep(li) { margin-bottom: 4px; color: var(--ink-2); }
  :deep(p) { margin: 8px 0; }
}

/* Sidebar */
.detail-sidebar {
  width: 240px;
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.detail-info-card {
  background: var(--bg-2);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.info-item {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.info-item-label {
  font-size: 11px;
  font-weight: 600;
  color: var(--ink-3);
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.info-item-value {
  font-size: 13px;
  color: var(--ink-1);
  font-weight: 500;
}

.detail-links {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.detail-link {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: var(--accent);
  text-decoration: none;
  padding: 6px 12px;
  border-radius: var(--radius-sm);
  transition: background 0.1s;

  &:hover { background: var(--accent-soft); }
}

.detail-license {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 0 4px;
}

/* Image preview overlay */
.preview-overlay {
  position: fixed;
  inset: 0;
  z-index: 200;
  background: rgba(0,0,0,0.85);
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
}

.preview-img {
  max-width: 90%;
  max-height: 90%;
  border-radius: var(--radius);
  box-shadow: var(--shadow-elevated);
}

/* -- Scrollbar -- */
.market-content::-webkit-scrollbar,
.market-sidebar::-webkit-scrollbar,
.detail-panel::-webkit-scrollbar {
  width: 6px;
}

.market-content::-webkit-scrollbar-track,
.market-sidebar::-webkit-scrollbar-track,
.detail-panel::-webkit-scrollbar-track {
  background: transparent;
}

.market-content::-webkit-scrollbar-thumb,
.market-sidebar::-webkit-scrollbar-thumb,
.detail-panel::-webkit-scrollbar-thumb {
  background: var(--bg-3);
  border-radius: 3px;
}
</style>
