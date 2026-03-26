<template>
  <div class="settings-page">
    <div class="page-title">Storage</div>
    <div class="page-scroll">
      <!-- Disk Usage -->
      <div class="section-title">Disk Usage</div>
      <div class="settings-card">
        <div class="metric-row">
          <div class="metric-header">
            <span class="info-label">Used</span>
            <span class="metric-value" :class="diskPct > 80 ? 'text-red-5' : diskPct > 50 ? 'text-amber-7' : 'text-green-5'">
              {{ formatBytes(diskUsed) }} / {{ formatBytes(diskTotal) }}
            </span>
          </div>
          <q-linear-progress :value="diskPct / 100" :color="diskPct > 80 ? 'red-6' : diskPct > 50 ? 'amber-7' : 'green-6'" track-color="grey-9" rounded size="6px" class="q-mt-sm" />
          <div class="metric-sub">{{ diskPct.toFixed(1) }}% used</div>
        </div>
      </div>

      <!-- Network Mounts -->
      <div class="section-title">Network Mounts</div>
      <div class="settings-card">
        <div v-if="mounts.length === 0" class="empty-state">
          <q-icon name="sym_r_folder_off" size="32px" color="grey-7" class="q-mb-sm" />
          <div>No mounts configured</div>
        </div>
        <template v-for="(m, i) in mounts" :key="m.name">
          <div class="mount-row">
            <div class="mount-info">
              <div class="mount-icon-wrap">
                <q-icon :name="m.type === 'smb' ? 'sym_r_folder_shared' : m.type === 'nfs' ? 'sym_r_dns' : 'sym_r_cloud'" size="16px" />
              </div>
              <div>
                <div class="mount-name">{{ m.name }}</div>
                <div class="mount-path">{{ m.type }}://{{ m.remote }} &rarr; {{ m.local_path }}</div>
              </div>
            </div>
            <q-btn flat dense round icon="sym_r_delete" size="xs" color="negative" @click="removeMount(m.name)" />
          </div>
          <q-separator v-if="i < mounts.length - 1" class="card-separator" />
        </template>
      </div>

      <!-- Add Mount -->
      <div class="section-title">Add Mount</div>
      <div class="settings-card">
        <div class="input-row">
          <span class="input-label">Type</span>
          <q-select v-model="newMount.type" :options="['smb', 'nfs', 'rclone']" dense dark outlined class="setting-input" />
        </div>
        <div class="input-row">
          <span class="input-label">Name</span>
          <q-input v-model="newMount.name" dense dark outlined placeholder="my-nas" class="setting-input" />
        </div>
        <div class="input-row">
          <span class="input-label">Remote</span>
          <q-input v-model="newMount.remote" dense dark outlined :placeholder="newMount.type === 'smb' ? '//192.168.1.100/share' : '192.168.1.100:/export'" class="setting-input" />
        </div>
        <div class="input-row" v-if="newMount.type === 'smb'">
          <span class="input-label">Username</span>
          <q-input v-model="newMount.username" dense dark outlined placeholder="guest" class="setting-input" />
        </div>
        <div class="input-row" v-if="newMount.type === 'smb'">
          <span class="input-label">Password</span>
          <q-input v-model="newMount.password" dense dark outlined type="password" class="setting-input" />
        </div>
        <div class="action-row">
          <q-btn unelevated dense label="Add Mount" class="btn-primary" :loading="adding" @click="addMount" />
          <span v-if="mountMsg" class="save-msg" :class="mountMsg.startsWith('Error') ? 'text-red-5' : 'text-green-5'">{{ mountMsg }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<script lang="ts" setup>
import { ref, computed, onMounted } from 'vue';
import { api } from 'boot/axios';
import { formatBytes } from 'src/utils/helpers';

interface Mount { name: string; type: string; remote: string; local_path: string; }

const diskUsed = ref(0);
const diskTotal = ref(0);
const diskPct = computed(() => diskTotal.value ? (diskUsed.value / diskTotal.value) * 100 : 0);
const mounts = ref<Mount[]>([]);
const adding = ref(false);
const mountMsg = ref('');
const newMount = ref({ type: 'smb', name: '', remote: '', username: '', password: '' });

async function loadDisk() {
  try { const r: any = await api.get('/api/monitor/metrics'); diskUsed.value = r?.disk?.used || 0; diskTotal.value = r?.disk?.total || 0; } catch {}
}

async function loadMounts() {
  try { const r: any = await api.get('/api/mounts'); mounts.value = r?.mounts || r || []; } catch { mounts.value = []; }
}

async function addMount() {
  if (!newMount.value.name || !newMount.value.remote) { mountMsg.value = 'Error: name and remote required'; return; }
  adding.value = true; mountMsg.value = '';
  try {
    await api.post('/api/mounts', newMount.value);
    mountMsg.value = 'Mount added';
    newMount.value = { type: 'smb', name: '', remote: '', username: '', password: '' };
    await loadMounts();
  } catch (e: any) { mountMsg.value = 'Error: ' + (e?.response?.data || e.message || 'failed'); }
  adding.value = false;
}

async function removeMount(name: string) { try { await api.delete('/api/mounts/' + name); await loadMounts(); } catch {} }

onMounted(() => { loadDisk(); loadMounts(); });
</script>

<style lang="scss" scoped>
.mount-row { display: flex; justify-content: space-between; align-items: center; padding: 10px 20px; }
.mount-info { display: flex; align-items: center; gap: 10px; }
.mount-icon-wrap {
  width: 30px; height: 30px; border-radius: 8px;
  background: rgba(255,255,255,0.04); display: flex; align-items: center; justify-content: center; color: var(--ink-3);
}
.mount-name { font-size: 13px; font-weight: 500; color: var(--ink-1); }
.mount-path { font-size: 11px; color: var(--ink-3); font-family: 'JetBrains Mono', monospace; margin-top: 1px; }
</style>
