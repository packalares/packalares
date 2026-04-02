<template>
  <div v-if="ready" class="desktop-root" @click="onDesktopClick" @contextmenu.prevent="onContextMenu">
    <!-- Wallpaper -->
    <div class="desktop-bg-container">
      <img class="desktop-bg" :src="wallpaper" />
    </div>

    <!-- Desktop Widgets -->
    <DesktopWidgets ref="widgetsRef" />

    <!-- Desktop Context Menu -->
    <div
      v-if="desktopCtx.show"
      class="desktop-ctx-menu glass"
      :style="{ left: desktopCtx.x + 'px', top: desktopCtx.y + 'px' }"
      @click.stop
    >
      <div class="ctx-item" @click="ctxRefresh">
        <q-icon name="sym_r_refresh" size="14px" />
        <span>Refresh</span>
      </div>
      <div class="ctx-item" @click="ctxResetWidgets">
        <q-icon name="sym_r_reset_wrench" size="14px" />
        <span>Reset Widgets</span>
      </div>
    </div>

    <!-- Dock Bar -->
    <div class="dock-bar glass">
      <!-- User avatar -->
      <div class="dock-avatar-section">
        <div class="dock-avatar" @click.stop="onAvatarClick">
          <img
            v-if="userStore.avatar"
            :src="userStore.avatar"
            class="dock-avatar-img"
          />
          <q-icon v-else name="sym_r_person" size="20px" color="white" class="dock-avatar-fallback" />
        </div>
        <div class="dock-separator"></div>
      </div>

      <!-- App icons -->
      <div class="dock-apps" ref="dockAppsRef">
        <div
          v-for="app in dockApps"
          :key="app.id"
          class="dock-app-slot"
          @click.stop="onDockAppClick(app)"
          @contextmenu.prevent.stop="onDockContextMenu($event, app)"
        >
          <div class="dock-app-icon" :class="{ 'dock-app-hover': true }">
            <img v-if="app.icon && (app.icon.startsWith('/') || app.icon.startsWith('http'))" :src="resolveIconUrl(app.icon)" style="width:22px;height:22px;border-radius:5px;object-fit:cover" />
            <q-icon v-else :name="'sym_r_' + (app.icon || 'web')" size="20px" color="white" />
          </div>
          <div
            v-if="isAppOpen(app.id)"
            class="dock-active-dot"
          ></div>
        </div>
      </div>

      <!-- Bottom section -->
      <div class="dock-bottom-section">
        <div class="dock-separator"></div>
        <div class="dock-bottom-btn" @click.stop="toggleLaunchPad">
          <q-icon name="sym_r_grid_view" size="18px" color="white" />
        </div>
        <div class="dock-bottom-btn" @click.stop="toggleSearch">
          <q-icon name="sym_r_search" size="18px" color="white" />
        </div>
      </div>
    </div>

    <!-- Context Menu -->
    <q-menu
      v-model="contextMenu.show"
      :target="false"
      no-parent-event
      :offset="[0, 0]"
      class="context-menu-popup"
      anchor="top left"
      self="top left"
      :style="`left:${contextMenu.x}px;top:${contextMenu.y}px;position:fixed`"
    >
      <q-list dense class="context-menu-list">
        <q-item
          v-if="!isAppOpen(contextMenu.app?.id)"
          clickable
          v-close-popup
          @click="onDockAppClick(contextMenu.app)"
        >
          <q-item-section>Open</q-item-section>
        </q-item>
        <q-item
          v-if="isAppOpen(contextMenu.app?.id)"
          clickable
          v-close-popup
          @click="onWindowClose(contextMenu.app?.id)"
        >
          <q-item-section class="text-negative">Quit</q-item-section>
        </q-item>
      </q-list>
    </q-menu>

    <!-- Windows Layer -->
    <div class="windows-layer" ref="windowsLayerRef">
      <template v-for="win in windows" :key="win.id">
        <div
          v-if="win.visible"
          class="app-window"
          :class="{ 'window-maximized': win.maximized, 'window-active': win.id === activeWindowId }"
          :style="windowStyle(win)"
          @mousedown.stop="bringToFront(win.id)"
        >
          <!-- Title bar -->
          <div
            class="window-titlebar"
            @mousedown.stop="startDrag($event, win)"
            @dblclick="toggleMaximize(win.id)"
          >
            <div class="titlebar-left">
              <q-icon :name="'sym_r_' + (getAppById(win.appId)?.icon || 'web')" size="16px" class="titlebar-icon" />
              <span class="titlebar-title">{{ win.title }}</span>
            </div>
            <div class="titlebar-buttons">
              <div
                class="traffic-btn traffic-close"
                @mousedown.stop
                @click.stop="onWindowClose(win.id)"
                title="Close"
              >
                <span class="traffic-symbol">&times;</span>
              </div>
              <div
                class="traffic-btn traffic-minimize"
                @mousedown.stop
                @click.stop="minimizeWindow(win.id)"
                title="Minimize"
              >
                <span class="traffic-symbol">&minus;</span>
              </div>
              <div
                class="traffic-btn traffic-maximize"
                @mousedown.stop
                @click.stop="toggleMaximize(win.id)"
                title="Maximize"
              >
                <span class="traffic-symbol">&#x2B1A;</span>
              </div>
            </div>
          </div>
          <!-- Iframe content -->
          <div class="window-content">
            <div v-if="dragging === win.id || resizing === win.id" class="iframe-overlay"></div>
            <iframe
              :src="win.url"
              :data-app-id="win.appId"
              class="window-iframe"
              allow="web-share; clipboard-read; clipboard-write"
            ></iframe>
          </div>
          <!-- Resize handle -->
          <div
            v-if="!win.maximized"
            class="resize-handle"
            @mousedown.stop="startResize($event, win)"
          ></div>
        </div>
      </template>
    </div>

    <!-- LaunchPad Overlay -->
      <div
        v-if="showLaunchPad"
        class="launchpad-overlay"
        @click="showLaunchPad = false"
      >
        <div class="launchpad-mask" @click="showLaunchPad = false"></div>
        <div class="launchpad-content" @click.stop>
          <div class="launchpad-search-row">
            <q-input
              v-model="launchSearch"
              dense
              dark
              class="launchpad-search-input"
              placeholder="Search apps..."
              @click.stop
            >
              <template #prepend>
                <q-icon name="sym_r_search" size="18px" color="grey-5" />
              </template>
              <template #append>
                <q-icon
                  v-if="launchSearch"
                  name="sym_r_close"
                  size="16px"
                  class="cursor-pointer"
                  @click.stop="launchSearch = ''"
                />
              </template>
            </q-input>
          </div>
          <div class="launchpad-grid">
            <div
              v-for="app in filteredLaunchApps"
              :key="app.id"
              class="launchpad-app"
              @click.stop="onLaunchAppClick(app)"
            >
              <div class="launchpad-app-icon">
                <img v-if="app.icon && (app.icon.startsWith('/') || app.icon.startsWith('http'))" :src="resolveIconUrl(app.icon)" style="width:36px;height:36px;border-radius:10px;object-fit:cover" />
                <q-icon v-else :name="'sym_r_' + (app.icon || 'web')" size="34px" color="white" />
              </div>
              <div class="launchpad-app-name">{{ app.title }}</div>
            </div>
          </div>
          <div v-if="filteredLaunchApps.length === 0" class="launchpad-empty">
            No apps found
          </div>
        </div>
      </div>

    <!-- Search overlay -->
      <div
        v-if="showSearchOverlay"
        class="search-overlay"
        @click.self="showSearchOverlay = false"
      >
        <div class="search-dialog-box">
          <q-input
            v-model="searchQuery"
            dense
            dark
            autofocus
            class="search-dialog-input"
            placeholder="Search..."
            @click.stop
          >
            <template #prepend>
              <q-icon name="sym_r_search" size="20px" />
            </template>
          </q-input>
          <div class="search-results" v-if="searchResults.length">
            <div
              v-for="app in searchResults"
              :key="app.id"
              class="search-result-item"
              @click.stop="onSearchResultClick(app)"
            >
              <img v-if="app.icon && (app.icon.startsWith('/') || app.icon.startsWith('http'))" :src="resolveIconUrl(app.icon)" style="width:20px;height:20px;border-radius:5px;object-fit:cover" class="q-mr-sm" />
              <q-icon v-else :name="'sym_r_' + (app.icon || 'web')" size="20px" class="q-mr-sm" />
              {{ app.title }}
            </div>
          </div>
        </div>
      </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onUnmounted, watch, nextTick } from 'vue';
import { api } from 'boot/axios';
import { useUserStore } from 'stores/user';
import DesktopWidgets from 'src/components/DesktopWidgets.vue';

// ─── Types ───────────────────────────────────────────────────

interface AppInfo {
  id: string;
  name: string;
  title: string;
  icon: string;
  url: string;
  status?: string;
}

interface WindowInfo {
  id: string;
  appId: string;
  title: string;
  url: string;
  x: number;
  y: number;
  width: number;
  height: number;
  z: number;
  visible: boolean;
  maximized: boolean;
  prevRect?: { x: number; y: number; width: number; height: number };
}

// Serializable subset of WindowInfo for localStorage persistence
interface SavedWindow {
  appId: string;
  url: string;
  x: number;
  y: number;
  width: number;
  height: number;
  visible: boolean;
}

// ─── Stores ──────────────────────────────────────────────────

const userStore = useUserStore();

// ─── State ───────────────────────────────────────────────────

const ready = ref(false);
const wallpaper = ref(localStorage.getItem('packalares_wallpaper') || '/bg/macos4.jpg');

// Watch for wallpaper changes from Settings/Appearance (works across tabs and iframes)
const wpChannel = new BroadcastChannel('packalares_settings');
const currentDesktopTheme = ref(localStorage.getItem('packalares_theme') || 'dark');

function broadcastToIframes(data: any) {
  document.querySelectorAll('iframe').forEach((iframe) => {
    try { (iframe as HTMLIFrameElement).contentWindow?.postMessage(data, '*'); } catch {}
  });
}

function handleSettingsMsg(data: any) {
  if (data?.type === 'wallpaper' && data.value) {
    wallpaper.value = data.value;
  }
  if (data?.type === 'theme' && data.value) {
    currentDesktopTheme.value = data.value;
    localStorage.setItem('packalares_theme', data.value);
    applyTheme(data.value);
    broadcastToIframes(data);
  }
  // Child iframe asking for current theme
  if (data?.type === 'theme-query') {
    broadcastToIframes({ type: 'theme', value: currentDesktopTheme.value });
  }
}
wpChannel.onmessage = (e) => handleSettingsMsg(e.data);
window.addEventListener('message', (e) => handleSettingsMsg(e.data));


// Resolve icon URL — on subdomain, /api/* paths need to go through the IP or API subdomain
function resolveIconUrl(icon: string): string {
  if (!icon) return '';
  if (icon.startsWith('http')) return icon;
  if (icon.startsWith('/api/')) {
    const host = window.location.hostname;
    if (isIP(host)) return icon; // relative works on IP
    // On subdomain, route through the main IP or api subdomain
    const parts = host.split('.');
    if (parts.length >= 3) {
      const zone = parts.slice(1).join('.');
      return `https://api.${zone}${icon}`;
    }
  }
  return icon;
}

// Build subdomain URLs dynamically from current hostname
function isIP(host: string): boolean {
  return /^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}$/.test(host);
}

function appUrl(name: string, path = '/'): string {
  const host = window.location.hostname;
  // IP access: use path-based routing
  if (isIP(host)) {
    return `/${name}${path}`;
  }
  // Subdomain access: use sibling subdomains with full path
  const parts = host.split('.');
  if (parts.length >= 3) {
    const zone = parts.slice(1).join('.');
    // System apps (settings, market, dashboard, files) need their route prefix
    // because the Vue router expects /settings/*, /market, /files, /dashboard
    const systemApps = ['settings', 'market', 'dashboard', 'files'];
    const prefix = systemApps.includes(name) ? `/${name}` : '';
    return `https://${name}.${zone}${prefix}${path}`;
  }
  return `/${name}${path}`;
}

const systemApps: AppInfo[] = [
  { id: 'files', name: 'files', title: 'Files', icon: 'folder', url: appUrl('files'), status: 'running' },
  { id: 'settings', name: 'settings', title: 'Settings', icon: 'settings', url: appUrl('settings', '/system'), status: 'running' },
  { id: 'market', name: 'market', title: 'Market', icon: 'storefront', url: appUrl('market'), status: 'running' },
  { id: 'dashboard', name: 'dashboard', title: 'Dashboard', icon: 'monitoring', url: appUrl('dashboard'), status: 'running' },
];

const allApps = ref<AppInfo[]>([...systemApps]);
const dockApps = ref<AppInfo[]>([...systemApps]);
const windows = ref<WindowInfo[]>([]);
const activeWindowId = ref<string | null>(null);
let nextZ = 1;

// LaunchPad
const showLaunchPad = ref(false);
const launchSearch = ref('');

// Search
const showSearchOverlay = ref(false);
const searchQuery = ref('');

// Context menu (dock apps)
const contextMenu = reactive({
  show: false,
  x: 0,
  y: 0,
  app: null as AppInfo | null,
});

// Desktop right-click context menu
const desktopCtx = reactive({ show: false, x: 0, y: 0 });
const widgetsRef = ref<InstanceType<typeof DesktopWidgets> | null>(null);

function onContextMenu(e: MouseEvent) {
  contextMenu.show = false;
  desktopCtx.x = e.clientX;
  desktopCtx.y = e.clientY;
  desktopCtx.show = true;
}

function ctxRefresh() {
  desktopCtx.show = false;
  window.location.reload();
}

function ctxResetWidgets() {
  desktopCtx.show = false;
  widgetsRef.value?.resetPositions();
}

// Drag/resize tracking
const dragging = ref<string | null>(null);
const resizing = ref<string | null>(null);
const windowsLayerRef = ref<HTMLElement | null>(null);
const dockAppsRef = ref<HTMLElement | null>(null);

// ─── Window Persistence ──────────────────────────────────────

const STORAGE_KEY = 'packalares_desktop_windows';
let saveTimer: ReturnType<typeof setTimeout> | null = null;

function saveWindowState() {
  // Debounce: clear any pending save
  if (saveTimer) clearTimeout(saveTimer);
  saveTimer = setTimeout(() => {
    const toSave: SavedWindow[] = windows.value.map((w) => ({
      appId: w.appId,
      url: w.url,
      x: w.x,
      y: w.y,
      width: w.width,
      height: w.height,
      visible: w.visible,
    }));
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(toSave));
    } catch {
      // localStorage full or unavailable — silently skip
    }
  }, 500);
}

function restoreWindowState() {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return;
    const saved: SavedWindow[] = JSON.parse(raw);
    if (!Array.isArray(saved) || saved.length === 0) return;

    // Build a set of known app IDs (system + loaded remote)
    const knownIds = new Set(allApps.value.map((a) => a.id));

    for (const sw of saved) {
      // Skip windows for apps that no longer exist (uninstalled)
      if (!knownIds.has(sw.appId)) continue;

      // Skip if a window for this app is already open
      if (windows.value.some((w) => w.appId === sw.appId)) continue;

      const app = allApps.value.find((a) => a.id === sw.appId);
      if (!app) continue;

      const win: WindowInfo = {
        id: sw.appId + '-' + Date.now() + '-' + Math.random().toString(36).slice(2, 6),
        appId: sw.appId,
        title: app.title,
        url: sw.url || app.url,
        x: sw.x,
        y: sw.y,
        width: sw.width,
        height: sw.height,
        z: ++nextZ,
        visible: sw.visible,
        maximized: false,
      };

      windows.value.push(win);
    }

    // Set the topmost visible window as active
    const visible = windows.value.filter((w) => w.visible);
    if (visible.length > 0) {
      activeWindowId.value = visible[visible.length - 1].id;
    }
  } catch {
    // Corrupted state — ignore
  }
}

// ─── Clock ───────────────────────────────────────────────────

// ─── Computed ────────────────────────────────────────────────

const filteredLaunchApps = computed(() => {
  const q = launchSearch.value.toLowerCase().trim();
  if (!q) return allApps.value;
  return allApps.value.filter(
    (a) => a.title.toLowerCase().includes(q) || a.name.toLowerCase().includes(q)
  );
});

const searchResults = computed(() => {
  const q = searchQuery.value.toLowerCase().trim();
  if (!q) return [];
  return allApps.value.filter(
    (a) => a.title.toLowerCase().includes(q) || a.name.toLowerCase().includes(q)
  );
});

// ─── Helpers ─────────────────────────────────────────────────

function isAppOpen(appId: string | undefined): boolean {
  if (!appId) return false;
  return windows.value.some((w) => w.appId === appId);
}

function getAppById(id: string): AppInfo | undefined {
  return allApps.value.find((a) => a.id === id);
}

function windowStyle(win: WindowInfo): Record<string, string> {
  if (win.maximized) {
    return {
      left: '84px',
      top: '0px',
      width: 'calc(100vw - 84px)',
      height: '100vh',
      zIndex: String(win.z),
    };
  }
  return {
    left: win.x + 'px',
    top: win.y + 'px',
    width: win.width + 'px',
    height: win.height + 'px',
    zIndex: String(win.z),
  };
}

// ─── Window Management ──────────────────────────────────────

function openWindow(app: AppInfo) {
  const existing = windows.value.find((w) => w.appId === app.id);
  if (existing) {
    existing.visible = true;
    bringToFront(existing.id);
    saveWindowState();
    return;
  }

  const vw = window.innerWidth - 84;
  const vh = window.innerHeight;
  const w = Math.min(Math.round(vw * 0.8), 1200);
  const h = Math.min(Math.round(vh * 0.8), 800);
  const x = 84 + Math.round((vw - w) / 2) + windows.value.length * 20;
  const y = Math.round((vh - h) / 2) + windows.value.length * 20;

  const win: WindowInfo = {
    id: app.id + '-' + Date.now(),
    appId: app.id,
    title: app.title,
    url: app.url,
    x,
    y,
    width: w,
    height: h,
    z: ++nextZ,
    visible: true,
    maximized: false,
  };

  windows.value.push(win);
  activeWindowId.value = win.id;
  saveWindowState();
}

function bringToFront(winId: string) {
  const win = windows.value.find((w) => w.id === winId);
  if (!win) return;
  nextZ++;
  win.z = nextZ;
  activeWindowId.value = winId;
}

function minimizeWindow(winId: string) {
  const win = windows.value.find((w) => w.id === winId);
  if (win) {
    win.visible = false;
    saveWindowState();
  }
}

function toggleMaximize(winId: string) {
  const win = windows.value.find((w) => w.id === winId);
  if (!win) return;
  if (win.maximized) {
    if (win.prevRect) {
      win.x = win.prevRect.x;
      win.y = win.prevRect.y;
      win.width = win.prevRect.width;
      win.height = win.prevRect.height;
    }
    win.maximized = false;
  } else {
    win.prevRect = { x: win.x, y: win.y, width: win.width, height: win.height };
    win.maximized = true;
  }
  bringToFront(winId);
  saveWindowState();
}

function onWindowClose(winId: string | undefined) {
  if (!winId) return;
  windows.value = windows.value.filter((w) => w.id !== winId);
  saveWindowState();
}

// ─── Drag Windows ───────────────────────────────────────────

let dragData = { startX: 0, startY: 0, origX: 0, origY: 0, winId: '' };

function startDrag(e: MouseEvent, win: WindowInfo) {
  if (win.maximized) return;
  bringToFront(win.id);
  dragging.value = win.id;
  dragData = {
    startX: e.clientX,
    startY: e.clientY,
    origX: win.x,
    origY: win.y,
    winId: win.id,
  };
  document.addEventListener('mousemove', onDragMove);
  document.addEventListener('mouseup', onDragEnd);
}

function onDragMove(e: MouseEvent) {
  const win = windows.value.find((w) => w.id === dragData.winId);
  if (!win) return;
  win.x = dragData.origX + (e.clientX - dragData.startX);
  win.y = dragData.origY + (e.clientY - dragData.startY);
  // Clamp
  if (win.y < 0) win.y = 0;
}

function onDragEnd() {
  dragging.value = null;
  document.removeEventListener('mousemove', onDragMove);
  document.removeEventListener('mouseup', onDragEnd);
  saveWindowState();
}

// ─── Resize Windows ─────────────────────────────────────────

let resizeData = { startX: 0, startY: 0, origW: 0, origH: 0, winId: '' };

function startResize(e: MouseEvent, win: WindowInfo) {
  bringToFront(win.id);
  resizing.value = win.id;
  resizeData = {
    startX: e.clientX,
    startY: e.clientY,
    origW: win.width,
    origH: win.height,
    winId: win.id,
  };
  document.addEventListener('mousemove', onResizeMove);
  document.addEventListener('mouseup', onResizeEnd);
}

function onResizeMove(e: MouseEvent) {
  const win = windows.value.find((w) => w.id === resizeData.winId);
  if (!win) return;
  const newW = resizeData.origW + (e.clientX - resizeData.startX);
  const newH = resizeData.origH + (e.clientY - resizeData.startY);
  win.width = Math.max(400, newW);
  win.height = Math.max(200, newH);
}

function onResizeEnd() {
  resizing.value = null;
  document.removeEventListener('mousemove', onResizeMove);
  document.removeEventListener('mouseup', onResizeEnd);
  saveWindowState();
}

// ─── Dock Interactions ──────────────────────────────────────

function onDockAppClick(app: AppInfo | null) {
  if (!app) return;
  openWindow(app);
}

function onDockContextMenu(e: MouseEvent, app: AppInfo) {
  contextMenu.x = e.clientX;
  contextMenu.y = e.clientY;
  contextMenu.app = app;
  contextMenu.show = true;
}

function onAvatarClick() {
  const existing = windows.value.find(w => w.appId === 'settings');
  if (existing) {
    existing.visible = true;
    bringToFront(existing.id);
    // Try to navigate the iframe to account page
    try {
      const iframe = document.querySelector(`iframe[data-app-id="settings"]`) as HTMLIFrameElement;
      if (iframe?.contentWindow) {
        iframe.contentWindow.location.hash = '';
        iframe.contentWindow.location.href = appUrl('settings', '/account');
      }
    } catch {}
    return;
  }
  onDockAppClick({ id: 'settings', name: 'settings', title: 'Settings', icon: 'settings', url: appUrl('settings', '/account'), status: 'running' });
}

function toggleLaunchPad() {
  showLaunchPad.value = !showLaunchPad.value;
  launchSearch.value = '';
}

function toggleSearch() {
  showSearchOverlay.value = !showSearchOverlay.value;
  searchQuery.value = '';
}

function onLaunchAppClick(app: AppInfo) {
  showLaunchPad.value = false;
  openWindow(app);
}

function onSearchResultClick(app: AppInfo) {
  showSearchOverlay.value = false;
  searchQuery.value = '';
  openWindow(app);
}

function onDesktopClick() {
  contextMenu.show = false;
  desktopCtx.show = false;
}

// ─── Keyboard shortcuts ─────────────────────────────────────

function onKeydown(e: KeyboardEvent) {
  if (e.shiftKey && e.code === 'Space') {
    e.preventDefault();
    toggleSearch();
  }
}

// ─── Data loading ────────────────────────────────────────────

async function loadInit() {
  try {
    const data: any = await api.get('/api/desktop/init');
    if (data?.terminus) {
      userStore.terminusName = data.terminus.terminusName || '';
      userStore.avatar = data.terminus.avatar || '';
    }
    // Load apps from init response
    if (data?.myApps?.code === 200 && Array.isArray(data.myApps.data)) {
      loadAppsFromData(data.myApps.data);
    }
  } catch {
    // Init endpoint may not be available; use defaults
  }
}

function loadAppsFromData(appData: any[]) {
  const remote: AppInfo[] = appData.map((a: any) => ({
    id: a.name || a.id,
    name: a.name,
    title: a.title || a.name,
    icon: a.icon || 'web',
    url: appUrl(a.name || a.id),
    status: a.status || 'running',
  }));
  // Merge: keep system apps, add remote apps that are not duplicates
  const sysIds = new Set(systemApps.map((s) => s.id));
  const extraApps = remote.filter((a) => !sysIds.has(a.id));
  allApps.value = [...systemApps, ...extraApps];
  // Dock: system apps + any configured dock apps from remote
  dockApps.value = [...systemApps];
}

// ─── Lifecycle ───────────────────────────────────────────────

// ─── Theme ────────────────────────────────────────────────────

import { applyTheme } from 'src/composables/useTheme';

onMounted(async () => {
  // Check wizard status — redirect if not completed
  try {
    const r: any = await api.get('/api/user/info');
    const d = r?.data ?? r;
    if (d && d.wizard_complete === false) {
      window.location.href = '/wizard';
      return;
    }
  } catch {}

  ready.value = true;

  // Apply saved theme
  const savedTheme = localStorage.getItem('packalares_theme') || 'dark';
  applyTheme(savedTheme);

  window.addEventListener('keydown', onKeydown);

  await loadInit();

  // Restore saved window state after apps are loaded
  restoreWindowState();
});

onUnmounted(() => {
  if (saveTimer) clearTimeout(saveTimer);
  window.removeEventListener('keydown', onKeydown);
  document.removeEventListener('mousemove', onDragMove);
  document.removeEventListener('mouseup', onDragEnd);
  document.removeEventListener('mousemove', onResizeMove);
  document.removeEventListener('mouseup', onResizeEnd);
});
</script>

<style lang="scss" scoped>
/* ═══════════════════════════════════════════════════════════
   DESKTOP ROOT
   ═══════════════════════════════════════════════════════════ */
.desktop-root {
  position: fixed;
  inset: 0;
  overflow: hidden;
  background: #000;
}

/* ─── Wallpaper ─────────────────────────────────────────── */
.desktop-bg-container {
  position: absolute;
  inset: 0;
  z-index: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
}
.desktop-bg {
  width: auto;
  min-width: 100%;
  height: 100%;
  object-fit: cover;
}

/* ═══════════════════════════════════════════════════════════
   DOCK BAR (left side, glassmorphism — matching Olares)
   ═══════════════════════════════════════════════════════════ */
.dock-bar {
  position: absolute;
  left: 12px;
  top: 50%;
  transform: translateY(-50%);
  z-index: 999;
  width: 56px;
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 10px 0;
  border-radius: 18px;
  max-height: 90vh;

  /* macOS-style frosted glass */
  background: rgba(255, 255, 255, 0.18);
  border: 0.5px solid rgba(255, 255, 255, 0.3);
  box-shadow:
    0 8px 32px rgba(0, 0, 0, 0.12),
    inset 0 0.5px 0 rgba(255, 255, 255, 0.35);
  backdrop-filter: blur(50px) saturate(1.8);
  -webkit-backdrop-filter: blur(50px) saturate(1.8);
}

/* Avatar */
.dock-avatar-section {
  display: flex;
  flex-direction: column;
  align-items: center;
  width: 100%;
}
.dock-avatar {
  width: 34px;
  height: 34px;
  border-radius: 50%;
  overflow: hidden;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.15);
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(255, 255, 255, 0.2);
  transition: transform 0.15s ease;
}
.dock-avatar:hover {
  transform: scale(1.1);
}
.dock-avatar-img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.dock-avatar-fallback {
  font-size: 22px;
  color: rgba(255, 255, 255, 0.7);
}
.dock-separator {
  width: 28px;
  height: 0;
  border-bottom: 0.5px solid rgba(255, 255, 255, 0.25);
  margin: 8px 0;
}

/* App icons */
.dock-apps {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  flex: 1;
  overflow: hidden;
  width: 100%;
}
.dock-app-slot {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 44px;
  height: 44px;
  cursor: pointer;
}
.dock-app-icon {
  width: 36px;
  height: 36px;
  border-radius: 9px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(255, 255, 255, 0.15);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.1);
  transition: transform 0.15s ease;
}
.dock-app-icon:hover {
  transform: scale(1.08);
}
.dock-icon-glyph {
  font-size: 20px;
  color: #fff;
}
.dock-active-dot {
  position: absolute;
  right: -7px;
  top: 50%;
  transform: translateY(-50%);
  width: 4px;
  height: 4px;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.8);
}

/* Bottom section */
.dock-bottom-section {
  display: flex;
  flex-direction: column;
  align-items: center;
  width: 100%;
}
.dock-bottom-btn {
  width: 30px;
  height: 30px;
  border-radius: 8px;
  background: transparent;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  margin-top: 6px;
  transition: background 0.2s ease;
  .material-symbols-rounded {
    font-size: 18px;
    color: rgba(255, 255, 255, 0.8);
  }
  &:hover {
    background: rgba(255, 255, 255, 0.35);
  }
}

/* ═══════════════════════════════════════════════════════════
   CONTEXT MENU
   ═══════════════════════════════════════════════════════════ */
.context-menu-list {
  min-width: 140px;
}

/* ═══════════════════════════════════════════════════════════
   WINDOWS
   ═══════════════════════════════════════════════════════════ */
.windows-layer {
  position: absolute;
  inset: 0;
  z-index: 10;
  pointer-events: none;
}
.app-window {
  position: absolute;
  border-radius: 10px;
  overflow: hidden;
  pointer-events: auto;
  box-shadow: 0 0 2px 0 rgba(0, 0, 0, 0.4), 0 0 40px 0 rgba(0, 0, 0, 0.2);
  display: flex;
  flex-direction: column;
  transition: box-shadow 0.15s;

  &.window-active {
    box-shadow: 0 0 2px 0 rgba(0, 0, 0, 0.5), 0 4px 60px 0 rgba(0, 0, 0, 0.35);
  }
  &.window-maximized {
    border-radius: 0;
    transition: left 0.25s ease, top 0.25s ease, width 0.25s ease, height 0.25s ease;
  }
}

/* Title bar — frosted glass */
.window-titlebar {
  height: 40px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 12px;
  background: rgba(38, 38, 38, 0.92);
  backdrop-filter: blur(30px) saturate(1.5);
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
  cursor: default;
  flex-shrink: 0;
  user-select: none;
}
.titlebar-left {
  display: flex;
  align-items: center;
  gap: 10px;
  overflow: hidden;
}
.titlebar-icon {
  font-size: 20px;
  color: rgba(255, 255, 255, 0.7);
  flex-shrink: 0;
}
.titlebar-title {
  font-family: 'Inter', sans-serif;
  font-weight: 600;
  font-size: 14px;
  color: rgba(255, 255, 255, 0.85);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* Traffic-light buttons */
.titlebar-buttons {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}
.traffic-btn {
  width: 14px;
  height: 14px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition: filter 0.12s;
  &:hover {
    filter: brightness(1.2);
  }
}
.traffic-symbol {
  font-size: 10px;
  line-height: 1;
  color: rgba(0, 0, 0, 0.5);
  opacity: 0;
  transition: opacity 0.12s;
}
.traffic-btn:hover .traffic-symbol {
  opacity: 1;
}
.traffic-close {
  background: #ff5f57;
}
.traffic-minimize {
  background: #ffbd2e;
}
.traffic-maximize {
  background: #28c840;
}

/* Window content */
.window-content {
  flex: 1;
  position: relative;
  background: #1f1f1f;
}
.window-iframe {
  width: 100%;
  height: 100%;
  border: none;
  background: #fff;
}
.iframe-overlay {
  position: absolute;
  inset: 0;
  z-index: 10;
  background: transparent;
}

/* Resize handle */
.resize-handle {
  position: absolute;
  bottom: 0;
  right: 0;
  width: 20px;
  height: 20px;
  cursor: nwse-resize;
  z-index: 20;
}

/* ═══════════════════════════════════════════════════════════
   LAUNCHPAD
   ═══════════════════════════════════════════════════════════ */
.launchpad-overlay {
  position: absolute;
  inset: 0;
  z-index: 2000;
  display: flex;
  align-items: center;
  justify-content: center;
}
.launchpad-mask {
  position: absolute;
  inset: 0;
  background: rgba(0, 0, 0, 0.5);
  backdrop-filter: blur(10px);
  z-index: -1;
}
.launchpad-content {
  width: calc(100% - 108px);
  max-height: calc(100% - 60px);
  margin-left: 108px;
  padding-top: 36px;
  display: flex;
  flex-direction: column;
  align-items: center;
  overflow: visible;
  pointer-events: none;
  & > * {
    pointer-events: auto;
  }
}
.launchpad-search-row {
  width: 100%;
  display: flex;
  justify-content: center;
  margin-bottom: 40px;
}
.launchpad-search-input {
  width: 240px;
  border-radius: 8px;
  padding-left: 8px;
  border: 1px solid rgba(255, 255, 255, 0.2);
  background: rgba(246, 246, 246, 0.1);
  box-shadow: 0 0 40px 0 rgba(0, 0, 0, 0.2), 0 0 2px 0 rgba(0, 0, 0, 0.4);
}
.launchpad-grid {
  display: flex;
  flex-wrap: wrap;
  gap: 40px;
  justify-content: center;
  max-width: 900px;
  padding: 0 40px;
  overflow-y: auto;
  flex: 1;
  scrollbar-width: none;
  &::-webkit-scrollbar { display: none; }
}
.launchpad-app {
  display: flex;
  flex-direction: column;
  align-items: center;
  cursor: pointer;
  width: 80px;
  transition: transform 0.12s;
  &:hover {
    transform: scale(1.08);
  }
}
.launchpad-app-icon {
  width: 64px;
  height: 64px;
  border-radius: 22%;
  background: rgba(255, 255, 255, 0.12);
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: 0 2px 10px rgba(0, 0, 0, 0.2);
  .q-icon {
    font-size: 34px;
  }
}
.launchpad-app-name {
  margin-top: 10px;
  font-size: 13px;
  font-family: 'Inter', sans-serif;
  font-weight: 500;
  color: #fff;
  text-align: center;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
  max-width: 80px;
}
.launchpad-empty {
  color: #e5e5e5;
  font-size: 18px;
  margin-top: 100px;
}

/* ═══════════════════════════════════════════════════════════
   SEARCH OVERLAY
   ═══════════════════════════════════════════════════════════ */
.search-overlay {
  position: absolute;
  inset: 0;
  z-index: 3000;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: flex-start;
  justify-content: center;
  padding-top: 15vh;
}
.search-dialog-box {
  width: 520px;
  border-radius: 12px;
  background: rgba(38, 38, 38, 0.95);
  backdrop-filter: blur(40px);
  border: 1px solid rgba(255, 255, 255, 0.1);
  padding: 8px;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.5);
}
.search-dialog-input {
  border: none;
}
.search-results {
  margin-top: 4px;
  max-height: 300px;
  overflow-y: auto;
}
.search-result-item {
  display: flex;
  align-items: center;
  padding: 10px 12px;
  border-radius: 8px;
  cursor: pointer;
  font-size: 14px;
  color: rgba(255, 255, 255, 0.85);
  .material-symbols-rounded {
    font-size: 20px;
    color: rgba(255, 255, 255, 0.5);
  }
  &:hover {
    background: rgba(255, 255, 255, 0.08);
  }
}

/* ═══ Desktop Context Menu ═══ */
.desktop-ctx-menu {
  position: fixed;
  z-index: 100;
  min-width: 160px;
  padding: 4px 0;
  border-radius: 10px;
  background: rgba(30, 30, 30, 0.85);
  backdrop-filter: blur(24px) saturate(1.4);
  -webkit-backdrop-filter: blur(24px) saturate(1.4);
  border: 1px solid rgba(255, 255, 255, 0.1);
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.4);
}
.ctx-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 14px;
  font-size: 12px;
  color: rgba(255, 255, 255, 0.85);
  cursor: pointer;
  &:hover { background: rgba(255, 255, 255, 0.1); }
}
</style>
