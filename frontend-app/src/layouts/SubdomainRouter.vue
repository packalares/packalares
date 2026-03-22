<template>
  <component :is="currentPage" />
</template>

<script setup lang="ts">
import { computed, defineAsyncComponent } from 'vue';

const subdomainPages: Record<string, ReturnType<typeof defineAsyncComponent>> = {
  desktop: defineAsyncComponent(() => import('pages/Desktop/DesktopPage.vue')),
  settings: defineAsyncComponent(() => import('pages/Settings/SettingsPage.vue')),
  market: defineAsyncComponent(() => import('pages/Market/MarketPage.vue')),
  files: defineAsyncComponent(() => import('pages/Files/FilesPage.vue')),
  dashboard: defineAsyncComponent(() => import('pages/Dashboard/DashboardPage.vue')),
  auth: defineAsyncComponent(() => import('pages/Login/LoginPage.vue')),
};

const defaultPage = defineAsyncComponent(() => import('pages/Desktop/DesktopPage.vue'));

const currentPage = computed(() => {
  const host = window.location.hostname;
  const parts = host.split('.');
  if (parts.length >= 3) {
    const sub = parts[0];
    if (sub in subdomainPages) return subdomainPages[sub];
  }
  // IP access or unknown subdomain — show desktop
  return defaultPage;
});
</script>
