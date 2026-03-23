<template>
  <div class="settings-root">
    <div class="settings-sidebar">
      <!-- User card -->
      <div
        class="sidebar-user-card"
        :class="{ 'sidebar-item-active': currentPath === '/settings/account' }"
        @click="navigateTo('/settings/account')"
      >
        <q-avatar size="40px" color="grey-8" text-color="white" icon="sym_r_person" />
        <div class="sidebar-user-info">
          <div
            class="sidebar-user-name"
            :class="{ 'text-accent-active': currentPath === '/settings/account' }"
          >
            {{ userName }}
          </div>
          <div
            class="sidebar-user-role"
            :class="{ 'text-accent-active': currentPath === '/settings/account' }"
          >
            {{ userRole }}
          </div>
        </div>
      </div>

      <q-separator class="sidebar-separator" />

      <!-- Navigation items -->
      <q-list dense class="sidebar-nav">
        <q-item
          v-for="item in navItems"
          :key="item.path"
          clickable
          :active="currentPath === item.path"
          active-class="sidebar-item-active"
          class="sidebar-nav-item"
          @click="navigateTo(item.path)"
        >
          <q-item-section avatar style="min-width: 36px">
            <q-icon
              :name="item.icon"
              :color="currentPath === item.path ? undefined : undefined"
              :class="{ 'text-accent-active': currentPath === item.path }"
              size="20px"
            />
          </q-item-section>
          <q-item-section>
            <q-item-label
              :class="currentPath === item.path ? 'text-accent-active' : 'text-ink-1'"
            >
              {{ item.label }}
            </q-item-label>
          </q-item-section>
        </q-item>
      </q-list>
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
  width: 240px;
  min-width: 240px;
  height: 100%;
  background-color: var(--bg-1);
  border-right: 1px solid var(--separator);
  display: flex;
  flex-direction: column;
  padding: 12px 8px;
  overflow-y: auto;
}

.sidebar-user-card {
  display: flex;
  align-items: center;
  padding: 8px 12px;
  border-radius: 8px;
  cursor: pointer;
  transition: background-color 0.15s;

  &:hover {
    background-color: var(--glass);
  }
}

.sidebar-user-info {
  margin-left: 10px;
  overflow: hidden;
}

.sidebar-user-name {
  font-size: 14px;
  font-weight: 500;
  color: var(--ink-1);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.sidebar-user-role {
  font-size: 12px;
  color: var(--ink-3);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.sidebar-separator {
  margin: 8px 12px;
  background-color: var(--separator);
}

.sidebar-nav {
  flex: 1;
}

.sidebar-nav-item {
  border-radius: 8px;
  min-height: 40px;
  margin-bottom: 2px;
  color: var(--ink-1);

  .q-item__section--avatar {
    padding-right: 0;
  }
}

.sidebar-item-active {
  background-color: var(--accent-soft) !important;
}

.text-accent-active {
  color: var(--accent) !important;
}

.text-ink-1 {
  color: var(--ink-1);
}

.settings-content {
  flex: 1;
  height: 100%;
  overflow-y: auto;
  background-color: var(--bg-1);
}
</style>
