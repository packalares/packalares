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
      <!-- Search Bar -->
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
                :src="app.icon || '/icons/default-app.png'"
                :alt="app.title"
                class="app-icon"
                @error="(e: Event) => ((e.target as HTMLImageElement).src = '/icons/default-app.png')"
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
              <q-btn
                v-if="isInstalled(app.name) && appStates[app.name] !== 'installing' && appStates[app.name] !== 'downloading'"
                flat
                dense
                no-caps
                label="Open"
                class="app-btn-open"
                @click.stop="openApp(app.name)"
              />
              <q-btn
                v-else-if="appStates[app.name] === 'downloading'"
                flat
                dense
                no-caps
                label="Downloading..."
                class="app-btn-installing"
                loading
                disable
              />
              <q-btn
                v-else-if="appStates[app.name] === 'installing' || installingSet.has(app.name)"
                flat
                dense
                no-caps
                label="Installing..."
                class="app-btn-installing"
                loading
                disable
              />
              <q-btn
                v-else
                flat
                dense
                no-caps
                label="Install"
                class="app-btn-install"
                @click.stop="installApp(app)"
              />
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
                :src="app.icon || '/icons/default-app.png'"
                :alt="app.title"
                class="app-icon"
                @error="(e: Event) => ((e.target as HTMLImageElement).src = '/icons/default-app.png')"
              />
              <div class="app-card-meta">
                <div class="app-card-title">{{ app.title }}</div>
                <div class="app-card-developer">{{ app.developer || 'Unknown' }}</div>
              </div>
            </div>
            <div class="app-card-desc">{{ app.description }}</div>
            <div class="app-card-footer">
              <q-badge
                :label="installedStatusMap[app.name] || 'running'"
                :class="'status-badge status-' + (installedStatusMap[app.name] || 'running')"
              />
              <div class="app-card-footer-actions">
                <q-btn
                  flat
                  dense
                  no-caps
                  label="Open"
                  class="app-btn-open"
                  @click.stop="openApp(app.name)"
                />
                <q-btn
                  flat
                  dense
                  no-caps
                  label="Uninstall"
                  class="app-btn-uninstall"
                  @click.stop="confirmUninstall(app)"
                />
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Detail Panel -->
    <transition name="slide-panel">
      <div v-if="detailApp" class="detail-overlay" @click.self="detailApp = null">
        <div class="detail-panel card">
          <div class="detail-close">
            <q-btn flat round dense icon="close" @click="detailApp = null" />
          </div>

          <div class="detail-header">
            <img
              :src="detailApp.icon || '/icons/default-app.png'"
              :alt="detailApp.title"
              class="detail-icon"
              @error="(e: Event) => ((e.target as HTMLImageElement).src = '/icons/default-app.png')"
            />
            <div class="detail-info">
              <div class="detail-title">{{ detailApp.title }}</div>
              <div class="detail-developer">{{ detailApp.developer || 'Unknown' }}</div>
              <div class="detail-version">v{{ detailApp.version || '1.0.0' }}</div>
            </div>
          </div>

          <div class="detail-actions">
            <template v-if="isInstalled(detailApp.name)">
              <q-btn
                no-caps
                label="Open"
                class="detail-btn-open"
                @click="openApp(detailApp.name)"
              />
              <q-btn
                flat
                no-caps
                label="Uninstall"
                class="detail-btn-uninstall"
                @click="confirmUninstall(detailApp)"
              />
            </template>
            <template v-else-if="appStates[detailApp.name] === 'downloading'">
              <q-btn
                no-caps
                label="Downloading..."
                class="detail-btn-install"
                loading
                disable
              />
            </template>
            <template v-else-if="appStates[detailApp.name] === 'installing' || installingSet.has(detailApp.name)">
              <q-btn
                no-caps
                label="Installing..."
                class="detail-btn-install"
                loading
                disable
              />
            </template>
            <template v-else>
              <q-btn
                no-caps
                label="Install"
                class="detail-btn-install"
                @click="installApp(detailApp)"
              />
            </template>
          </div>

          <q-separator class="detail-sep" />

          <!-- Screenshots -->
          <div class="detail-section" v-if="detailData?.promoteImage?.length">
            <div class="detail-section-title">Screenshots</div>
            <div class="detail-screenshots">
              <img
                v-for="(img, idx) in detailData.promoteImage"
                :key="idx"
                :src="img"
                class="detail-screenshot"
                @error="(e: Event) => ((e.target as HTMLImageElement).style.display = 'none')"
              />
            </div>
          </div>

          <div class="detail-section">
            <div class="detail-section-title">Description</div>
            <div class="detail-description" v-if="detailLoading">
              <q-skeleton type="text" width="100%" />
              <q-skeleton type="text" width="90%" />
              <q-skeleton type="text" width="80%" />
            </div>
            <div class="detail-description" v-else>
              {{ detailData?.fullDescription || detailData?.description || detailApp.description || 'No description available.' }}
            </div>
          </div>

          <div class="detail-section" v-if="detailApp.categories?.length">
            <div class="detail-section-title">Categories</div>
            <div class="detail-tags">
              <q-badge
                v-for="cat in detailApp.categories"
                :key="cat"
                :label="cat"
                class="detail-tag"
              />
            </div>
          </div>

          <div class="detail-section" v-if="detailData?.requiredMemory || detailData?.requiredCpu">
            <div class="detail-section-title">Requirements</div>
            <div class="detail-requirements">
              <span v-if="detailData?.requiredMemory">Memory: {{ detailData.requiredMemory }}</span>
              <span v-if="detailData?.requiredCpu">CPU: {{ detailData.requiredCpu }}</span>
              <span v-if="detailData?.requiredDisk">Disk: {{ detailData.requiredDisk }}</span>
              <span v-if="detailData?.requiredGpu">GPU: {{ detailData.requiredGpu }}</span>
            </div>
          </div>
        </div>
      </div>
    </transition>
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
}

interface Category {
  name: string;
  count: number;
}

interface InstalledApp {
  name: string;
  status: string;
}

const loading = ref(true);
const searchQuery = ref('');
const activeTab = ref<'discover' | 'installed'>('discover');
const activeCategory = ref('all');
const apps = ref<MarketApp[]>([]);
const categories = ref<Category[]>([]);
const installedApps = ref<InstalledApp[]>([]);
const detailApp = ref<MarketApp | null>(null);
const detailData = ref<MarketApp | null>(null);
const detailLoading = ref(false);
const installingSet = reactive(new Set<string>());
const appStates = reactive<Record<string, string>>({});

let ws: WebSocket | null = null;

const installedStatusMap = computed(() => {
  const map: Record<string, string> = {};
  installedApps.value.forEach((a) => {
    map[a.name] = a.status;
  });
  return map;
});

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
  if (/^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}$/.test(host)) return '/' + name + '/';
  const parts = host.split('.');
  if (parts.length >= 3) {
    return 'https://' + name + '.' + parts.slice(1).join('.');
  }
  return '/' + name + '/';
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
    await api.post('/api/apps/uninstall', { name: app.name });
    detailApp.value = null;
    await fetchInstalled();
    $q.notify({ type: 'positive', message: app.title + ' uninstalled.' });
  } catch (e: any) {
    console.error('Uninstall failed:', e);
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
            // App is ready
            installingSet.delete(name);
            delete appStates[name];
            fetchInstalled();
          } else if (state === 'failed') {
            installingSet.delete(name);
            delete appStates[name];
            $q.notify({ type: 'negative', message: `${name} installation failed.` });
          } else {
            appStates[name] = state;
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

// Watch for detail panel close to clean up
watch(detailApp, (val) => {
  if (!val) {
    detailData.value = null;
    detailLoading.value = false;
  }
});

onMounted(async () => {
  loading.value = true;
  await Promise.all([fetchApps(), fetchCategories(), fetchInstalled()]);
  loading.value = false;
  connectWebSocket();
});

onUnmounted(() => {
  if (ws) {
    ws.close();
    ws = null;
  }
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

/* -- Detail Panel -- */
.detail-overlay {
  position: absolute;
  top: 0;
  right: 0;
  bottom: 0;
  left: 240px;
  z-index: 100;
  background: rgba(0, 0, 0, 0.4);
  display: flex;
  justify-content: flex-end;
}

.detail-panel {
  width: 500px;
  max-width: 100%;
  height: 100%;
  overflow-y: auto;
  padding: 24px;
  border-radius: 0;
  border-left: 1px solid var(--separator);
  background: var(--bg-2);
  position: relative;
}

.detail-close {
  position: absolute;
  top: 16px;
  right: 16px;
  z-index: 2;

  .q-btn {
    color: var(--ink-3);

    &:hover {
      color: var(--ink-1);
    }
  }
}

.detail-header {
  display: flex;
  align-items: flex-start;
  gap: 16px;
  margin-bottom: 20px;
  padding-right: 40px;
}

.detail-icon {
  width: 80px;
  height: 80px;
  border-radius: 20px;
  object-fit: cover;
  flex-shrink: 0;
  background: var(--bg-3);
}

.detail-info {
  flex: 1;
  min-width: 0;
}

.detail-title {
  font-size: 22px;
  font-weight: 600;
  color: var(--ink-1);
  margin-bottom: 4px;
}

.detail-developer {
  font-size: 14px;
  color: var(--ink-3);
  margin-bottom: 2px;
}

.detail-version {
  font-size: 13px;
  color: var(--accent);
  font-weight: 500;
}

.detail-actions {
  display: flex;
  gap: 12px;
  margin-bottom: 16px;
}

.detail-sep {
  background-color: var(--separator);
  margin: 16px 0;
}

.detail-section {
  margin-bottom: 16px;
}

.detail-section-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--ink-2);
  text-transform: uppercase;
  letter-spacing: 0.3px;
  margin-bottom: 8px;
}

.detail-description {
  font-size: 14px;
  line-height: 1.6;
  color: var(--ink-1);
  white-space: pre-wrap;
}

.detail-screenshots {
  display: flex;
  gap: 8px;
  overflow-x: auto;
  padding-bottom: 8px;

  &::-webkit-scrollbar {
    height: 4px;
  }
  &::-webkit-scrollbar-thumb {
    background: var(--bg-3);
    border-radius: 2px;
  }
}

.detail-screenshot {
  height: 200px;
  border-radius: 8px;
  object-fit: cover;
  flex-shrink: 0;
}

.detail-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.detail-tag {
  background: var(--bg-3) !important;
  color: var(--ink-2) !important;
  border-radius: 4px;
  font-size: 12px;
  padding: 2px 8px;
}

.detail-requirements {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  font-size: 13px;
  color: var(--ink-2);
}

.detail-btn-install {
  background: var(--accent) !important;
  color: #fff !important;
  border-radius: 8px;
  padding: 6px 32px;
  font-weight: 500;
}

.detail-btn-open {
  background: var(--positive) !important;
  color: #fff !important;
  border-radius: 8px;
  padding: 6px 32px;
  font-weight: 500;
}

.detail-btn-uninstall {
  background: rgba(255, 77, 77, 0.12) !important;
  color: var(--negative) !important;
  border-radius: 8px;
  padding: 6px 32px;
  font-weight: 500;
}

/* -- Transitions -- */
.slide-panel-enter-active,
.slide-panel-leave-active {
  transition: transform 0.25s ease;
}

.slide-panel-enter-from,
.slide-panel-leave-to {
  transform: translateX(100%);
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
