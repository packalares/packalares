<template>
  <div class="settings-root">
    <div class="settings-sidebar">
      <!-- User card -->
      <div
        class="sidebar-user-card"
        :class="{ 'sidebar-item-active': currentPath === '/settings/account' }"
        @click="navigateTo('/settings/account')"
      >
        <q-avatar size="36px" color="grey-8" text-color="white" icon="sym_r_person" class="user-avatar" />
        <div class="sidebar-user-info">
          <div class="sidebar-user-name">{{ userName }}</div>
          <div class="sidebar-user-role">{{ userRole }}</div>
        </div>
      </div>

      <div class="sidebar-divider" />

      <!-- Navigation items -->
      <div class="sidebar-nav">
        <div
          v-for="item in navItems"
          :key="item.path"
          class="nav-item"
          :class="{ 'nav-item-active': currentPath === item.path }"
          @click="navigateTo(item.path)"
        >
          <q-icon :name="item.icon" size="18px" class="nav-icon" />
          <span class="nav-text">{{ item.label }}</span>
        </div>
      </div>
    </div>

    <div class="settings-content">
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

interface NavItem {
  icon: string;
  label: string;
  path: string;
}

const navItems: NavItem[] = [
  { icon: 'sym_r_computer', label: 'System', path: '/settings/system' },
  { icon: 'sym_r_language', label: 'Network', path: '/settings/network' },
  { icon: 'sym_r_storage', label: 'Storage', path: '/settings/storage' },
  { icon: 'sym_r_memory', label: 'GPU', path: '/settings/gpu' },
  { icon: 'sym_r_palette', label: 'Appearance', path: '/settings/appearance' },
  { icon: 'sym_r_info', label: 'About', path: '/settings/about' },
];

const navigateTo = (path: string) => {
  router.push(path);
};
</script>

<style lang="scss" scoped>
.settings-root {
  display: flex;
  width: 100%;
  height: 100vh;
  background-color: var(--bg-1);
}

.settings-sidebar {
  width: 220px;
  min-width: 220px;
  height: 100%;
  background-color: var(--bg-1);
  border-right: 1px solid var(--separator);
  display: flex;
  flex-direction: column;
  padding: 16px 10px;
  overflow-y: auto;
}

.sidebar-user-card {
  display: flex;
  align-items: center;
  padding: 8px 10px;
  border-radius: 10px;
  cursor: pointer;
  transition: all 0.12s ease;
  gap: 10px;

  &:hover {
    background-color: var(--glass);
  }
}

.user-avatar {
  flex-shrink: 0;
}

.sidebar-user-info {
  overflow: hidden;
}

.sidebar-user-name {
  font-size: 13px;
  font-weight: 600;
  color: var(--ink-1);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.sidebar-user-role {
  font-size: 11px;
  color: var(--ink-3);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.sidebar-divider {
  height: 1px;
  background: var(--separator);
  margin: 8px 10px;
}

.sidebar-nav {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.nav-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 10px;
  border-radius: 8px;
  cursor: pointer;
  transition: all 0.12s ease;

  .nav-icon {
    color: var(--ink-3);
    flex-shrink: 0;
  }

  .nav-text {
    font-size: 13px;
    font-weight: 500;
    color: var(--ink-2);
  }

  &:hover {
    background-color: var(--glass);

    .nav-icon { color: var(--ink-2); }
    .nav-text { color: var(--ink-1); }
  }
}

.nav-item-active {
  background-color: var(--accent-soft) !important;

  .nav-icon { color: var(--accent) !important; }
  .nav-text { color: var(--accent) !important; font-weight: 600; }
}

.sidebar-item-active {
  background-color: var(--accent-soft) !important;

  .sidebar-user-name { color: var(--accent) !important; }
  .sidebar-user-role { color: var(--accent) !important; opacity: 0.7; }
}

.settings-content {
  flex: 1;
  height: 100%;
  overflow-y: auto;
  background-color: var(--bg-1);
}
</style>
