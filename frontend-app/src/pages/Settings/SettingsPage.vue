<template>
  <div class="settings-root">
    <div class="settings-sidebar">
      <div
        class="sidebar-user-card"
        :class="{ active: currentPath === '/settings/account' }"
        @click="navigateTo('/settings/account')"
      >
        <div class="user-avatar-wrap">
          <q-icon name="sym_r_person" size="18px" color="white" />
        </div>
        <div class="sidebar-user-info">
          <div class="sidebar-user-name">{{ userName }}</div>
          <div class="sidebar-user-role">{{ userRole }}</div>
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

<style lang="scss" scoped>
.settings-root {
  display: flex;
  width: 100%;
  height: 100vh;
  background: var(--bg-1);
}

.settings-sidebar {
  width: 216px;
  min-width: 216px;
  height: 100%;
  background: var(--bg-1);
  border-right: 1px solid var(--separator);
  display: flex;
  flex-direction: column;
  padding: 20px 10px;
}

.sidebar-user-card {
  display: flex;
  align-items: center;
  padding: 8px 10px;
  border-radius: var(--radius-sm);
  cursor: pointer;
  gap: 10px;
  transition: background 0.1s;

  &:hover { background: rgba(255,255,255,0.03); }
  &.active {
    background: var(--accent-soft);
    .sidebar-user-name { color: var(--accent); }
    .sidebar-user-role { color: var(--accent); opacity: 0.6; }
    .user-avatar-wrap { background: var(--accent-bold); }
  }
}

.user-avatar-wrap {
  width: 32px;
  height: 32px;
  border-radius: 9px;
  background: var(--bg-3);
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  transition: background 0.15s;
}

.sidebar-user-info { overflow: hidden; }
.sidebar-user-name {
  font-size: 13px;
  font-weight: 600;
  color: var(--ink-1);
  white-space: nowrap;
  text-overflow: ellipsis;
  overflow: hidden;
}
.sidebar-user-role {
  font-size: 11px;
  color: var(--ink-3);
  white-space: nowrap;
}

.sidebar-divider {
  height: 1px;
  background: var(--separator);
  margin: 10px 10px;
}

.sidebar-nav {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 1px;
}

.nav-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 7px 10px;
  border-radius: 7px;
  cursor: pointer;
  transition: all 0.1s;

  .nav-icon { color: var(--ink-3); flex-shrink: 0; transition: color 0.1s; }
  .nav-text { font-size: 13px; font-weight: 500; color: var(--ink-2); transition: color 0.1s; }

  &:hover {
    background: rgba(255,255,255,0.03);
    .nav-icon { color: var(--ink-2); }
    .nav-text { color: var(--ink-1); }
  }

  &.active {
    background: var(--accent-soft);
    .nav-icon { color: var(--accent) !important; }
    .nav-text { color: var(--accent) !important; font-weight: 600; }
  }
}

.settings-content {
  flex: 1;
  height: 100%;
  overflow-y: auto;
  background: var(--bg-1);
}
</style>
