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
            <span class="metric-value">{{ formatBytes(diskUsed) }} / {{ formatBytes(diskTotal) }} ({{ diskPct.toFixed(1) }}%)</span>
          </div>
          <q-linear-progress :value="diskPct / 100" :color="diskPct > 80 ? 'red-6' : diskPct > 50 ? 'amber-7' : 'green-6'" track-color="grey-9" rounded size="10px" class="q-mt-sm" />
        </div>
      </div>

      <!-- Network Mounts -->
      <div class="section-title">Network Mounts</div>
      <div class="settings-card">
        <div v-if="mounts.length === 0" class="empty-state">No mounts configured.</div>
        <template v-for="(m, i) in mounts" :key="m.name">
          <div class="mount-row">
            <div class="mount-info">
              <q-icon :name="m.type === 'smb' ? 'sym_r_folder_shared' : 'sym_r_cloud'" size="20px" class="q-mr-sm" style="color:var(--ink-3)" />
              <div>
                <div class="mount-name">{{ m.name }}</div>
                <div class="mount-path">{{ m.type }}://{{ m.remote }} → {{ m.local_path }}</div>
              </div>
            </div>
            <q-btn flat dense icon="sym_r_delete" color="negative" size="sm" @click="removeMount(m.name)" />
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
          <q-btn flat dense label="Add Mount" color="primary" :loading="adding" @click="addMount" />
          <span v-if="mountMsg" class="save-msg" :class="mountMsg.startsWith('Error') ? 'text-red-5' : 'text-green-5'">{{ mountMsg }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<script lang="ts" setup>
import { ref, computed, onMounted } from 'vue';
import { api } from 'boot/axios';

interface Mount { name: string; type: string; remote: string; local_path: string; }

const diskUsed = ref(0);
const diskTotal = ref(0);
const diskPct = computed(() => diskTotal.value ? (diskUsed.value / diskTotal.value) * 100 : 0);
const mounts = ref<Mount[]>([]);
const adding = ref(false);
const mountMsg = ref('');
const newMount = ref({ type: 'smb', name: '', remote: '', username: '', password: '' });

function formatBytes(b: number) {
  if (!b) return '0 B';
  const u = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(b) / Math.log(1024));
  return (b / Math.pow(1024, i)).toFixed(1) + ' ' + u[i];
}

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
.settings-page { height: 100%; display: flex; flex-direction: column; }
.page-title { font-size: 18px; font-weight: 600; color: var(--ink-1); padding: 16px 24px; height: 56px; display: flex; align-items: center; flex-shrink: 0; }
.page-scroll { flex: 1; overflow-y: auto; padding: 0 24px 24px; }
.section-title { font-size: 13px; font-weight: 500; color: var(--ink-2); margin-top: 20px; margin-bottom: 8px; text-transform: uppercase; letter-spacing: 0.5px; }
.settings-card { background: var(--bg-2); border-radius: 12px; border: 1px solid var(--separator); overflow: hidden; }
.info-label { font-size: 14px; color: var(--ink-1); font-weight: 500; }
.metric-row { padding: 16px 20px; }
.metric-header { display: flex; justify-content: space-between; align-items: center; }
.metric-value { font-size: 13px; font-weight: 600; font-family: 'JetBrains Mono', monospace; color: var(--ink-2); }
.card-separator { background: var(--separator); margin: 0 20px; }
.empty-state { padding: 20px; color: var(--ink-3); font-size: 13px; text-align: center; }
.mount-row { display: flex; justify-content: space-between; align-items: center; padding: 12px 20px; }
.mount-info { display: flex; align-items: center; }
.mount-name { font-size: 14px; font-weight: 500; color: var(--ink-1); }
.mount-path { font-size: 12px; color: var(--ink-3); font-family: monospace; }
.input-row { display: flex; align-items: center; padding: 8px 20px; gap: 12px; }
.input-label { font-size: 13px; color: var(--ink-1); font-weight: 500; min-width: 80px; }
.setting-input { flex: 1; }
.action-row { display: flex; align-items: center; gap: 12px; padding: 12px 20px; }
.save-msg { font-size: 12px; }
</style>
