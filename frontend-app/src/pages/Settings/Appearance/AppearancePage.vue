<template>
  <div class="settings-page">
    <div class="page-title">Appearance</div>
    <div class="page-scroll">
      <!-- Theme -->
      <div class="section-title">Theme</div>
      <div class="settings-card">
        <div class="theme-selector">
          <div
            class="theme-option"
            :class="{ 'theme-option-active': selectedTheme === 'dark' }"
            @click="selectTheme('dark')"
          >
            <div class="theme-preview theme-preview-dark">
              <div class="preview-sidebar"></div>
              <div class="preview-content">
                <div class="preview-bar"></div>
                <div class="preview-card"></div>
                <div class="preview-card short"></div>
              </div>
            </div>
            <div class="theme-label">
              <q-radio
                v-model="selectedTheme"
                val="dark"
                color="blue-8"
                dark
                dense
              />
              <span>Dark</span>
            </div>
          </div>

          <div
            class="theme-option"
            :class="{ 'theme-option-active': selectedTheme === 'light' }"
            @click="selectTheme('light')"
          >
            <div class="theme-preview theme-preview-light">
              <div class="preview-sidebar"></div>
              <div class="preview-content">
                <div class="preview-bar"></div>
                <div class="preview-card"></div>
                <div class="preview-card short"></div>
              </div>
            </div>
            <div class="theme-label">
              <q-radio
                v-model="selectedTheme"
                val="light"
                color="blue-8"
                dark
                dense
              />
              <span>Light</span>
            </div>
          </div>
        </div>
      </div>

      <!-- Wallpaper -->
      <div class="section-title">Wallpaper</div>
      <div class="settings-card">
        <div class="wallpaper-grid">
          <div
            v-for="(wp, idx) in wallpapers"
            :key="idx"
            class="wallpaper-item"
            :class="{ 'wallpaper-selected': selectedWallpaper === idx }"
            @click="selectWallpaper(idx)"
          >
            <div
              class="wallpaper-thumb"
              :style="{ background: wp.gradient }"
            ></div>
            <q-icon
              v-if="selectedWallpaper === idx"
              name="check_circle"
              color="blue-5"
              size="20px"
              class="wallpaper-check"
            />
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script lang="ts" setup>
import { ref } from 'vue';

const selectedTheme = ref('dark');
const selectedWallpaper = ref(0);

interface WallpaperDef {
  gradient: string;
}

const wallpapers: WallpaperDef[] = [
  { gradient: 'linear-gradient(135deg, #0a0a0a 0%, #1a1a2e 50%, #16213e 100%)' },
  { gradient: 'linear-gradient(135deg, #141e30 0%, #243b55 100%)' },
  { gradient: 'linear-gradient(135deg, #1e3c72 0%, #2a5298 100%)' },
  { gradient: 'linear-gradient(135deg, #232526 0%, #414345 100%)' },
  { gradient: 'linear-gradient(135deg, #0f2027 0%, #203a43 50%, #2c5364 100%)' },
  { gradient: 'linear-gradient(135deg, #2c3e50 0%, #3498db 100%)' },
];

const selectTheme = (theme: string) => {
  selectedTheme.value = theme;
};

const selectWallpaper = (idx: number) => {
  selectedWallpaper.value = idx;
};
</script>

<style lang="scss" scoped>
.settings-page {
  height: 100%;
  display: flex;
  flex-direction: column;
}

.page-title {
  font-size: 18px;
  font-weight: 600;
  color: var(--ink-1);
  padding: 16px 24px;
  height: 56px;
  display: flex;
  align-items: center;
  flex-shrink: 0;
}

.page-scroll {
  flex: 1;
  overflow-y: auto;
  padding: 0 24px 24px;
}

.section-title {
  font-size: 13px;
  font-weight: 500;
  color: var(--ink-2);
  margin-top: 20px;
  margin-bottom: 8px;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.settings-card {
  background: var(--bg-2);
  border-radius: 12px;
  border: 1px solid var(--separator);
  overflow: hidden;
}

/* Theme selector */
.theme-selector {
  display: flex;
  gap: 16px;
  padding: 20px;
}

.theme-option {
  cursor: pointer;
  border-radius: 10px;
  overflow: hidden;
  border: 2px solid transparent;
  transition: border-color 0.15s;

  &:hover {
    border-color: var(--glass-border);
  }
}

.theme-option-active {
  border-color: var(--accent) !important;
}

.theme-preview {
  width: 166px;
  height: 100px;
  display: flex;
  border-radius: 8px 8px 0 0;
  overflow: hidden;
}

.theme-preview-dark {
  .preview-sidebar {
    width: 40px;
    background: #1a1a1a;
    border-right: 1px solid rgba(255, 255, 255, 0.06);
  }
  .preview-content {
    flex: 1;
    background: #1f1f1f;
    padding: 8px;
  }
  .preview-bar {
    height: 8px;
    background: #333;
    border-radius: 4px;
    margin-bottom: 6px;
  }
  .preview-card {
    height: 20px;
    background: #2a2a2a;
    border-radius: 4px;
    margin-bottom: 4px;
    &.short {
      width: 60%;
    }
  }
}

.theme-preview-light {
  .preview-sidebar {
    width: 40px;
    background: #f0f0f0;
    border-right: 1px solid rgba(0, 0, 0, 0.08);
  }
  .preview-content {
    flex: 1;
    background: #fafafa;
    padding: 8px;
  }
  .preview-bar {
    height: 8px;
    background: #e0e0e0;
    border-radius: 4px;
    margin-bottom: 6px;
  }
  .preview-card {
    height: 20px;
    background: #eee;
    border-radius: 4px;
    margin-bottom: 4px;
    &.short {
      width: 60%;
    }
  }
}

.theme-label {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 8px 12px;
  font-size: 13px;
  color: var(--ink-1);
}

/* Wallpaper grid */
.wallpaper-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 12px;
  padding: 20px;
}

.wallpaper-item {
  position: relative;
  cursor: pointer;
  border-radius: 8px;
  overflow: hidden;
  border: 2px solid transparent;
  transition: border-color 0.15s;

  &:hover {
    border-color: var(--glass-border);
  }
}

.wallpaper-selected {
  border-color: var(--accent) !important;
}

.wallpaper-thumb {
  width: 100%;
  aspect-ratio: 16 / 10;
  border-radius: 6px;
}

.wallpaper-check {
  position: absolute;
  bottom: 6px;
  right: 6px;
}
</style>
