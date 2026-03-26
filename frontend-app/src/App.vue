<template>
  <router-view />
</template>

<script setup lang="ts">
import { onMounted } from 'vue';

function applyTheme(theme: string) {
  const root = document.documentElement;
  if (theme === 'light') {
    root.style.setProperty('--bg-0', '#e8eaee');
    root.style.setProperty('--bg-1', '#f0f1f4');
    root.style.setProperty('--bg-2', '#ffffff');
    root.style.setProperty('--bg-3', '#f4f5f7');
    root.style.setProperty('--bg-4', '#e9eaed');
    root.style.setProperty('--ink-1', '#1a1c22');
    root.style.setProperty('--ink-2', 'rgba(26,28,34,0.55)');
    root.style.setProperty('--ink-3', 'rgba(26,28,34,0.32)');
    root.style.setProperty('--separator', 'rgba(0,0,0,0.06)');
    root.style.setProperty('--border', 'rgba(0,0,0,0.08)');
    root.style.setProperty('--glass', 'rgba(0,0,0,0.03)');
    root.style.setProperty('--glass-border', 'rgba(0,0,0,0.08)');
    root.style.setProperty('--dock-bg', 'rgba(240,241,244,0.85)');
    root.style.setProperty('--shadow-card', '0 1px 3px rgba(0,0,0,0.06), 0 4px 14px rgba(0,0,0,0.04)');
    root.style.setProperty('--shadow-elevated', '0 2px 6px rgba(0,0,0,0.08), 0 12px 32px rgba(0,0,0,0.06)');
    root.style.setProperty('--shadow-sm', '0 1px 2px rgba(0,0,0,0.04)');
    root.style.setProperty('--input-bg', 'rgba(0,0,0,0.03)');
    root.style.setProperty('--input-border', 'rgba(0,0,0,0.10)');
    root.style.setProperty('--input-focus', 'rgba(99,102,241,0.18)');
  }
  // Dark theme is the CSS default — no need to set properties
}

onMounted(() => {
  const theme = localStorage.getItem('packalares_theme') || 'dark';
  if (theme !== 'dark') {
    applyTheme(theme);
  }

  // Listen for theme changes from other iframes/tabs
  const channel = new BroadcastChannel('packalares_settings');
  channel.onmessage = (e) => {
    if (e.data?.type === 'theme') {
      applyTheme(e.data.value);
    }
  };
});
</script>
