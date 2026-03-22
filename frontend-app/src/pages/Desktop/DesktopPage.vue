<template>
  <div class="desktop-root" @click="onDesktopClick" @contextmenu.prevent>
    <!-- Wallpaper -->
    <div class="desktop-bg-container">
      <img class="desktop-bg" :src="wallpaper" />
    </div>

    <!-- Daily Info: bottom-right time/date/stats (matching Olares DailyDescription) -->
    <div class="daily-info">
      <div class="daily-weather">
        <div class="daily-time">{{ clockTime }}</div>
        <div class="daily-date-block">
          <p class="daily-week">{{ weekDay }}</p>
          <p class="daily-date">{{ dateStr }}</p>
        </div>
      </div>
      <div class="daily-stats">
        <div class="stat-item" v-for="s in stats" :key="s.name">
          <q-knob
            readonly
            :model-value="s.value"
            size="24px"
            font-size="80px"
            :thickness="0.5"
            :color="s.color"
            track-color="grey-8"
          />
          <div class="stat-text">
            <p class="text-uppercase">{{ s.name }}</p>
            <p>{{ s.value }}%</p>
          </div>
        </div>
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
          <span v-else class="material-symbols-rounded dock-avatar-fallback">person</span>
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
            <span class="material-symbols-rounded dock-icon-glyph">{{ app.icon }}</span>
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
          <span class="material-symbols-rounded">grid_view</span>
        </div>
        <div class="dock-bottom-btn" @click.stop="toggleSearch">
          <span class="material-symbols-rounded">search</span>
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
              <span class="material-symbols-rounded titlebar-icon">{{ getAppById(win.appId)?.icon || 'web' }}</span>
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
    <transition
      enter-active-class="animated fadeIn"
      leave-active-class="animated fadeOut"
    >
      <div
        v-if="showLaunchPad"
        class="launchpad-overlay"
        @click.self="showLaunchPad = false"
      >
        <div class="launchpad-mask"></div>
        <div class="launchpad-content">
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
                <q-icon name="search" size="18px" color="grey-5" />
              </template>
              <template #append>
                <q-icon
                  v-if="launchSearch"
                  name="close"
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
                <span class="material-symbols-rounded">{{ app.icon }}</span>
              </div>
              <div class="launchpad-app-name">{{ app.title }}</div>
            </div>
          </div>
          <div v-if="filteredLaunchApps.length === 0" class="launchpad-empty">
            No apps found
          </div>
        </div>
      </div>
    </transition>

    <!-- Search overlay -->
    <transition
      enter-active-class="animated fadeIn"
      leave-active-class="animated fadeOut"
    >
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
              <q-icon name="search" size="20px" />
            </template>
          </q-input>
          <div class="search-results" v-if="searchResults.length">
            <div
              v-for="app in searchResults"
              :key="app.id"
              class="search-result-item"
              @click.stop="onSearchResultClick(app)"
            >
              <span class="material-symbols-rounded q-mr-sm">{{ app.icon }}</span>
              {{ app.title }}
            </div>
          </div>
        </div>
      </div>
    </transition>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onUnmounted, nextTick } from 'vue';
import { api } from 'boot/axios';
import { useUserStore } from 'stores/user';
import { useMonitorStore } from 'stores/monitor';

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

// ─── Stores ──────────────────────────────────────────────────

const userStore = useUserStore();
const monitorStore = useMonitorStore();

// ─── State ───────────────────────────────────────────────────

const wallpaper = ref('/bg/0.jpg');
const clockTime = ref('');
const weekDay = ref('');
const dateStr = ref('');

const systemApps: AppInfo[] = [
  { id: 'files', name: 'files', title: 'Files', icon: 'folder', url: '/files/', status: 'running' },
  { id: 'settings', name: 'settings', title: 'Settings', icon: 'settings', url: '/settings/', status: 'running' },
  { id: 'market', name: 'market', title: 'Market', icon: 'storefront', url: '/market/', status: 'running' },
  { id: 'dashboard', name: 'dashboard', title: 'Dashboard', icon: 'monitoring', url: '/dashboard/', status: 'running' },
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

// Context menu
const contextMenu = reactive({
  show: false,
  x: 0,
  y: 0,
  app: null as AppInfo | null,
});

// Drag/resize tracking
const dragging = ref<string | null>(null);
const resizing = ref<string | null>(null);
const windowsLayerRef = ref<HTMLElement | null>(null);
const dockAppsRef = ref<HTMLElement | null>(null);

// ─── Clock ───────────────────────────────────────────────────

let clockInterval: ReturnType<typeof setInterval> | null = null;
let monitorInterval: ReturnType<typeof setInterval> | null = null;

function updateClock() {
  const now = new Date();
  const h = now.getHours().toString().padStart(2, '0');
  const m = now.getMinutes().toString().padStart(2, '0');
  clockTime.value = `${h}:${m}`;

  const days = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'];
  weekDay.value = days[now.getDay()];

  const y = now.getFullYear();
  const mo = (now.getMonth() + 1).toString().padStart(2, '0');
  const d = now.getDate().toString().padStart(2, '0');
  dateStr.value = `${y}/${mo}/${d}`;
}

// ─── Stats ───────────────────────────────────────────────────

const stats = computed(() => [
  { name: 'CPU', value: Math.round(monitorStore.cpu.ratio * 100) / 100 || 0, color: 'light-blue-5' },
  { name: 'MEM', value: Math.round(monitorStore.memory.ratio * 100) / 100 || 0, color: 'green-5' },
  { name: 'DISK', value: Math.round(monitorStore.disk.ratio * 100) / 100 || 0, color: 'orange-5' },
]);

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
}

function onWindowClose(winId: string | undefined) {
  if (!winId) return;
  windows.value = windows.value.filter((w) => w.id !== winId);
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
  // Could open profile; for now do nothing
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
    const data: any = await api.get('/server/init');
    if (data?.terminus) {
      userStore.terminusName = data.terminus.terminusName || '';
      userStore.avatar = data.terminus.avatar || '';
    }
    if (data?.config) {
      if (data.config.bgIndex !== undefined) {
        wallpaper.value = `/bg/${data.config.bgIndex}.jpg`;
      }
    }
  } catch {
    // Init endpoint may not be available; use defaults
  }
}

async function loadApps() {
  try {
    const data: any = await api.post('/server/myApps');
    if (data?.code === 200 && Array.isArray(data.data)) {
      const remote: AppInfo[] = data.data.map((a: any) => ({
        id: a.name || a.id,
        name: a.name,
        title: a.title || a.name,
        icon: a.icon || 'web',
        url: a.url ? '//' + a.url : '#',
        status: a.status || 'running',
      }));
      // Merge: keep system apps, add remote apps that are not duplicates
      const sysIds = new Set(systemApps.map((s) => s.id));
      const extraApps = remote.filter((a) => !sysIds.has(a.id));
      allApps.value = [...systemApps, ...extraApps];
      // Dock: system apps + any configured dock apps from remote
      dockApps.value = [...systemApps];
    }
  } catch {
    // Keep defaults
  }
}

// ─── Lifecycle ───────────────────────────────────────────────

onMounted(async () => {
  updateClock();
  clockInterval = setInterval(updateClock, 1000);

  window.addEventListener('keydown', onKeydown);

  await loadInit();
  await loadApps();
  await monitorStore.loadCluster();

  monitorInterval = setInterval(() => {
    monitorStore.loadCluster();
  }, 30000);
});

onUnmounted(() => {
  if (clockInterval) clearInterval(clockInterval);
  if (monitorInterval) clearInterval(monitorInterval);
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
   DAILY INFO (bottom-right, matching Olares DailyDescription)
   ═══════════════════════════════════════════════════════════ */
.daily-info {
  position: absolute;
  bottom: 122px;
  right: 165px;
  z-index: 1;
}
.daily-weather {
  display: flex;
  height: 72px;
}
.daily-time {
  font-size: 70px;
  font-family: Roboto, sans-serif;
  font-weight: 700;
  color: #fff;
  line-height: 72px;
  text-shadow: 0 2px 6px rgba(0, 0, 0, 0.16);
}
.daily-date-block {
  display: flex;
  flex-direction: column;
  justify-content: flex-start;
  padding-top: 14px;
  margin-left: 14px;
  p { margin: 0; }
}
.daily-week {
  font-size: 20px;
  font-family: Roboto, sans-serif;
  font-weight: 700;
  color: #fff;
  text-shadow: 0 2px 6px rgba(0, 0, 0, 0.16);
}
.daily-date {
  font-size: 12px;
  font-family: Roboto, sans-serif;
  font-weight: 400;
  color: #fff;
  text-shadow: 0 2px 6px rgba(0, 0, 0, 0.16);
}
.daily-stats {
  display: flex;
  margin-top: 15px;
  gap: 16px;
}
.stat-item {
  display: flex;
  align-items: center;
  opacity: 0.8;
  p { margin: 0; }
}
.stat-text {
  font-size: 12px;
  font-family: Roboto, sans-serif;
  font-weight: 400;
  color: #fff;
  line-height: 14px;
  text-shadow: 0 2px 6px rgba(0, 0, 0, 0.16);
  margin-left: 8px;
}

/* ═══════════════════════════════════════════════════════════
   DOCK BAR (left side, glassmorphism — matching Olares)
   ═══════════════════════════════════════════════════════════ */
.dock-bar {
  position: absolute;
  left: 24px;
  top: 50%;
  transform: translateY(-50%);
  z-index: 999;
  width: 60px;
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 20px 0;
  border-radius: 16px;
  max-height: 94vh;

  /* Glassmorphism matching Olares dock */
  background: rgba(246, 246, 246, 0.3);
  border: 1px solid rgba(255, 255, 255, 0.2);
  filter: drop-shadow(0px 0px 40px rgba(0, 0, 0, 0.2))
    drop-shadow(0px 0px 2px rgba(0, 0, 0, 0.4));
  backdrop-filter: blur(120px);
}

/* Avatar */
.dock-avatar-section {
  display: flex;
  flex-direction: column;
  align-items: center;
  width: 100%;
}
.dock-avatar {
  width: 36px;
  height: 36px;
  border-radius: 50%;
  overflow: hidden;
  box-shadow: 0 0 11px 0 rgba(0, 0, 0, 0.2);
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(255, 255, 255, 0.15);
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
  width: 36px;
  height: 0;
  border-bottom: 1px solid rgba(31, 24, 20, 0.3);
  margin: 12px 0;
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
  width: 44px;
  height: 44px;
  border-radius: 22%;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(255, 255, 255, 0.12);
  box-shadow: 0 2px 10px 0 rgba(0, 0, 0, 0.2);
  transition: transform 0.15s ease;
}
.dock-app-icon:hover {
  transform: scale(1.15);
}
.dock-icon-glyph {
  font-size: 26px;
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
  width: 32px;
  height: 32px;
  border-radius: 18px;
  background: rgba(255, 255, 255, 0.2);
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  margin-top: 10px;
  transition: background 0.15s;
  .material-symbols-rounded {
    font-size: 16px;
    color: #fff;
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
  height: 100%;
  margin-left: 108px;
  padding-top: 36px;
  display: flex;
  flex-direction: column;
  align-items: center;
  overflow: hidden;
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
  .material-symbols-rounded {
    font-size: 34px;
    color: #fff;
  }
}
.launchpad-app-name {
  margin-top: 10px;
  font-size: 13px;
  font-family: Roboto, sans-serif;
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
</style>
