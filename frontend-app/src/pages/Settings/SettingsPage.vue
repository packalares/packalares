<template>
  <div class="iframe-root">
    <div class="iframe-sidebar">
      <div
        class="sidebar-brand sidebar-brand--clickable"
        :class="{ active: currentPath === '/settings/account' }"
        @click="navigateTo('/settings/account')"
      >
        <div class="brand-icon">
          <q-icon name="sym_r_person" size="18px" color="white" />
        </div>
        <div class="brand-info">
          <div class="brand-title">{{ userName }}</div>
          <div class="brand-sub">{{ userRole }}</div>
        </div>
      </div>

      <div class="sidebar-divider" />

      <div class="sidebar-nav">
        <div
          v-for="item in navItems"
          :key="item.path"
          class="nav-item"
          :class="{ active: currentPath === item.path }"
          @click="navigateTo(item.path)"
        >
          <q-icon :name="item.icon" size="17px" class="nav-icon" />
          <span class="nav-text">{{ item.label }}</span>
        </div>
      </div>
    </div>

    <div class="iframe-content">
      <router-view />
    </div>
  </div>
</template>

<script lang="ts" setup>
import { computed } from 'vue';
import { useRoute, useRouter } from 'vue-router';

const route = useRoute();
const router = useRouter();
const currentPath = computed(() => route.path);
const userName = 'admin';
const userRole = 'Administrator';

interface NavItem { icon: string; label: string; path: string; }

const navItems: NavItem[] = [
  { icon: 'sym_r_computer', label: 'System', path: '/settings/system' },
  { icon: 'sym_r_language', label: 'Network', path: '/settings/network' },
  { icon: 'sym_r_storage', label: 'Storage', path: '/settings/storage' },
  { icon: 'sym_r_memory', label: 'GPU', path: '/settings/gpu' },
  { icon: 'sym_r_palette', label: 'Appearance', path: '/settings/appearance' },
  { icon: 'sym_r_system_update', label: 'Update', path: '/settings/update' },
  { icon: 'sym_r_info', label: 'About', path: '/settings/about' },
];

const navigateTo = (path: string) => router.push(path);
</script>

<!-- All sidebar styles in components.scss (.iframe-root, .iframe-sidebar, etc.) -->
