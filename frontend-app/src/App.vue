<template>
  <router-view />
</template>

<script setup lang="ts">
import { onMounted } from 'vue';
import { applyTheme } from 'src/composables/useTheme';

onMounted(() => {
  const theme = localStorage.getItem('packalares_theme') || 'dark';
  applyTheme(theme);

  // Listen for theme changes from other iframes/tabs
  const channel = new BroadcastChannel('packalares_settings');
  channel.onmessage = (e) => {
    if (e.data?.type === 'theme') {
      applyTheme(e.data.value);
    }
  };
});
</script>
