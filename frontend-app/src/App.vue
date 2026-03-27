<template>
  <router-view />
</template>

<script setup lang="ts">
import { onMounted } from 'vue';
import { applyTheme } from 'src/composables/useTheme';

onMounted(() => {
  const theme = localStorage.getItem('packalares_theme') || 'dark';
  applyTheme(theme);

  // Listen for theme changes from other iframes/tabs (same-origin)
  const channel = new BroadcastChannel('packalares_settings');
  channel.onmessage = (e) => {
    if (e.data?.type === 'theme') {
      applyTheme(e.data.value);
      localStorage.setItem('packalares_theme', e.data.value);
    }
  };

  // Listen for theme changes from parent (cross-origin desktop → iframe)
  window.addEventListener('message', (e) => {
    if (e.data?.type === 'theme' && e.data.value) {
      applyTheme(e.data.value);
      localStorage.setItem('packalares_theme', e.data.value);
    }
    // Parent asking for theme query — respond with current theme
    if (e.data?.type === 'theme-query') {
      try { window.parent.postMessage({ type: 'theme-response', value: localStorage.getItem('packalares_theme') || 'dark' }, '*'); } catch {}
    }
  });

  // Ask parent for current theme (in case we loaded after theme was changed)
  try { window.parent.postMessage({ type: 'theme-query' }, '*'); } catch {}
});
</script>
