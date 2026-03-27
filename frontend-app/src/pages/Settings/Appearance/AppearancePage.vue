<template>
  <div class="settings-page">
    <div class="page-header">
      <div class="page-title">Appearance</div>
      <div class="page-description">Customize your desktop wallpaper and color theme.</div>
    </div>
    <div class="page-scroll">

      <!-- Wallpaper -->
      <div class="settings-card">
        <div class="card-header">
          <div class="card-header-icon card-header-icon--appearance">
            <q-icon name="sym_r_wallpaper" size="18px" />
          </div>
          <div class="card-header-text">
            <div class="card-header-title">Wallpaper</div>
            <div class="card-header-subtitle">Choose a desktop background</div>
          </div>
        </div>
        <div class="wallpaper-grid">
          <div
            v-for="wp in wallpapers"
            :key="wp"
            class="wallpaper-item"
            :class="{ 'wallpaper-selected': selectedWallpaper === wp }"
            @click="selectWallpaper(wp)"
          >
            <img :src="wp" class="wallpaper-thumb" loading="lazy" />
            <div v-if="selectedWallpaper === wp" class="wallpaper-check">
              <q-icon name="sym_r_check_circle" color="white" size="18px" />
            </div>
          </div>
        </div>
      </div>

      <!-- Theme -->
      <div class="settings-card q-mt-lg">
        <div class="card-header">
          <div class="card-header-icon card-header-icon--appearance">
            <q-icon name="sym_r_palette" size="18px" />
          </div>
          <div class="card-header-text">
            <div class="card-header-title">Theme</div>
            <div class="card-header-subtitle">Switch between dark and light mode</div>
          </div>
        </div>
        <div class="theme-selector">
          <div
            class="theme-option"
            :class="{ 'theme-active': selectedTheme === 'dark' }"
            @click="selectTheme('dark')"
          >
            <div class="theme-preview theme-dark">
              <div class="tp-sidebar"></div>
              <div class="tp-content">
                <div class="tp-bar"></div>
                <div class="tp-card"></div>
                <div class="tp-card short"></div>
              </div>
            </div>
            <div class="theme-label">
              <div class="theme-radio" :class="{ checked: selectedTheme === 'dark' }"></div>
              <span>Dark</span>
            </div>
          </div>
          <div
            class="theme-option"
            :class="{ 'theme-active': selectedTheme === 'light' }"
            @click="selectTheme('light')"
          >
            <div class="theme-preview theme-light">
              <div class="tp-sidebar"></div>
              <div class="tp-content">
                <div class="tp-bar"></div>
                <div class="tp-card"></div>
                <div class="tp-card short"></div>
              </div>
            </div>
            <div class="theme-label">
              <div class="theme-radio" :class="{ checked: selectedTheme === 'light' }"></div>
              <span>Light</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script lang="ts" setup>
import { ref } from 'vue';
import { applyTheme } from 'src/composables/useTheme';

const wallpapers = [
  '/bg/macos1.jpg', '/bg/macos2.jpg', '/bg/macos3.jpg', '/bg/macos4.jpg', '/bg/macos5.jpg',
  '/bg/0.jpg', '/bg/1.jpg', '/bg/2.jpg', '/bg/3.jpg', '/bg/4.jpg',
  '/bg/5.jpg', '/bg/6.jpg', '/bg/7.jpg', '/bg/8.jpg', '/bg/9.jpg',
  '/bg/10.jpg', '/bg/11.jpg', '/bg/12.jpg',
];

const selectedWallpaper = ref(localStorage.getItem('packalares_wallpaper') || '/bg/macos4.jpg');
const selectedTheme = ref(localStorage.getItem('packalares_theme') || 'dark');

const wpChannel = new BroadcastChannel('packalares_settings');

function broadcast(msg: object) {
  wpChannel.postMessage(msg);
  // Also postMessage to parent for cross-origin iframe communication
  try { window.parent.postMessage(msg, '*'); } catch {}
}

function selectWallpaper(wp: string) {
  selectedWallpaper.value = wp;
  localStorage.setItem('packalares_wallpaper', wp);
  broadcast({ type: 'wallpaper', value: wp });
}

function selectTheme(theme: string) {
  selectedTheme.value = theme;
  localStorage.setItem('packalares_theme', theme);
  applyTheme(theme);
  broadcast({ type: 'theme', value: theme });
}

// Apply theme on load
applyTheme(selectedTheme.value);
</script>

<style lang="scss" scoped>
.wallpaper-grid {
  display: grid;
  grid-template-columns: repeat(5, 1fr);
  gap: 6px;
  padding: 12px;
}

.wallpaper-item {
  position: relative;
  cursor: pointer;
  border-radius: var(--radius);
  overflow: hidden;
  border: 2px solid transparent;
  transition: all 0.15s ease;

  &:hover {
    border-color: rgba(255, 255, 255, 0.15);
    transform: scale(1.02);
  }
}

.wallpaper-selected {
  border-color: var(--accent) !important;
  box-shadow: 0 0 0 1px var(--accent);
}

.wallpaper-thumb {
  width: 100%;
  aspect-ratio: 16 / 10;
  object-fit: cover;
  display: block;
}

.wallpaper-check {
  position: absolute;
  bottom: 6px;
  right: 6px;
  width: 24px;
  height: 24px;
  border-radius: 50%;
  background: var(--accent);
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: 0 2px 6px rgba(0, 0, 0, 0.3);
}

.theme-selector {
  display: flex;
  gap: 14px;
  padding: 16px;
}

.theme-option {
  cursor: pointer;
  border-radius: 12px;
  overflow: hidden;
  border: 2px solid transparent;
  transition: all 0.15s ease;
  flex: 1;
  max-width: 180px;

  &:hover {
    border-color: rgba(255, 255, 255, 0.12);
  }
}

.theme-active {
  border-color: var(--accent) !important;
  box-shadow: 0 0 0 1px var(--accent);
}

.theme-preview {
  height: 90px;
  display: flex;
  overflow: hidden;
}

.theme-dark {
  .tp-sidebar { width: 36px; background: #17171c; border-right: 1px solid rgba(255,255,255,0.05); }
  .tp-content { flex: 1; background: #1e1f25; padding: 8px; }
  .tp-bar { height: 6px; background: #262730; border-radius: 3px; margin-bottom: 6px; }
  .tp-card { height: 16px; background: #2f3040; border-radius: 4px; margin-bottom: 4px; &.short { width: 55%; } }
}

.theme-light {
  .tp-sidebar { width: 36px; background: #f0f1f3; border-right: 1px solid rgba(0,0,0,0.05); }
  .tp-content { flex: 1; background: #fafbfc; padding: 8px; }
  .tp-bar { height: 6px; background: #e4e6ea; border-radius: 3px; margin-bottom: 6px; }
  .tp-card { height: 16px; background: #edeef1; border-radius: 4px; margin-bottom: 4px; &.short { width: 55%; } }
}

.theme-label {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 12px;
  font-size: 13px;
  font-weight: 500;
  color: var(--ink-1);
}

.theme-radio {
  width: 16px;
  height: 16px;
  border-radius: 50%;
  border: 2px solid var(--ink-3);
  position: relative;
  transition: all 0.15s ease;

  &.checked {
    border-color: var(--accent-bold);
    background: var(--accent-bold);

    &::after {
      content: '';
      position: absolute;
      top: 3px;
      left: 3px;
      width: 6px;
      height: 6px;
      border-radius: 50%;
      background: #fff;
    }
  }
}
</style>
