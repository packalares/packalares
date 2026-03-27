<template>
  <div class="settings-page">
    <div class="page-header">
      <div class="page-title">Storage</div>
      <div class="page-description">Monitor disk usage and manage network mounts (SMB, NFS, rclone).</div>
    </div>
    <div class="page-scroll">

      <!-- Disk Usage Stat Card -->
      <div class="stat-grid cols-2">
        <div class="stat-card">
          <span class="stat-card-label">Disk Usage</span>
          <span class="stat-card-value" :class="diskPct > 80 ? 'text-red-5' : diskPct > 50 ? 'text-amber-7' : 'text-green-5'">{{ diskPct.toFixed(1) }}%</span>
          <span class="stat-card-sub">{{ formatBytes(diskUsed) }} / {{ formatBytes(diskTotal) }}</span>
          <div class="stat-card-bar">
            <div class="stat-card-bar-fill" :style="{ width: diskPct + '%', background: diskPct > 80 ? '#f87171' : diskPct > 50 ? '#fbbf24' : '#34d399' }"></div>
          </div>
        </div>
        <div class="stat-card">
          <span class="stat-card-label">Available</span>
          <span class="stat-card-value">{{ formatBytes(diskTotal - diskUsed) }}</span>
          <span class="stat-card-sub">{{ (100 - diskPct).toFixed(1) }}% free</span>
        </div>
      </div>

      <!-- Network Mounts -->
      <div class="settings-card">
        <div class="card-header">
          <div class="card-header-icon card-header-icon--mount">
            <q-icon name="sym_r_folder_shared" size="18px" />
          </div>
          <div class="card-header-text">
            <div class="card-header-title">Network Mounts</div>
            <div class="card-header-subtitle">{{ mounts.length }} mount{{ mounts.length !== 1 ? 's' : '' }} configured</div>
          </div>
        </div>
        <div v-if="mounts.length === 0" class="empty-state">
          <div class="empty-state-icon">
            <q-icon name="sym_r_folder_off" size="24px" color="grey-6" />
          </div>
          <div>No mounts configured</div>
          <div style="font-size: 11px; color: var(--ink-3)">Add an SMB, NFS, or rclone mount below</div>
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
      <div class="settings-card q-mt-lg">
        <div class="card-header">
          <div class="card-header-icon card-header-icon--storage">
            <q-icon name="sym_r_add_circle" size="18px" />
          </div>
          <div class="card-header-text">
            <div class="card-header-title">Add Mount</div>
            <div class="card-header-subtitle">Connect a new network share</div>
          </div>
        </div>
        <div class="form-grid cols-2">
          <div class="form-group">
            <label class="form-label">Type</label>
            <q-select v-model="newMount.type" :options="['smb', 'nfs', 'rclone']" dense dark outlined />
          </div>
          <div class="form-group">
            <label class="form-label">Name</label>
            <q-input v-model="newMount.name" dense dark outlined placeholder="my-nas" />
          </div>
        </div>
        <div class="form-grid cols-1" style="padding-top: 0">
          <div class="form-group">
            <label class="form-label">Remote</label>
            <q-input v-model="newMount.remote" dense dark outlined :placeholder="newMount.type === 'smb' ? '//192.168.1.100/share' : '192.168.1.100:/export'" />
          </div>
        </div>
        <div v-if="newMount.type === 'smb'" class="form-grid cols-2" style="padding-top: 0">
          <div class="form-group">
            <label class="form-label">Username</label>
            <q-input v-model="newMount.username" dense dark outlined placeholder="guest" />
          </div>
          <div class="form-group">
            <label class="form-label">Password</label>
            <q-input v-model="newMount.password" dense dark outlined type="password" />
          </div>
        </div>
        <div class="card-footer">
          <span v-if="mountMsg" class="footer-msg" :class="mountMsg.startsWith('Error') ? 'text-red-5' : 'text-green-5'">{{ mountMsg }}</span>
          <q-btn unelevated dense label="Add Mount" class="btn-primary" :loading="adding" @click="addMount" />
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
.mount-path { font-size: 11px; color: var(--ink-3); font-family: 'Inter', sans-serif; margin-top: 1px; }
</style>
