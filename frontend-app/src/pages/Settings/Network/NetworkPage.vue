<template>
  <div class="settings-page">
    <div class="page-title">Network</div>
    <div class="page-scroll">
      <!-- Network Info -->
      <div class="section-title">Network Configuration</div>
      <div class="settings-card">
        <div class="info-row">
          <span class="info-label">Server IP</span>
          <span class="info-value">{{ netInfo.ip }}</span>
        </div>
        <q-separator class="card-separator" />
        <div class="info-row">
          <span class="info-label">Domain</span>
          <span class="info-value">{{ netInfo.domain }}</span>
        </div>
        <q-separator class="card-separator" />
        <div class="info-row">
          <span class="info-label">User Zone</span>
          <span class="info-value">{{ netInfo.zone }}</span>
        </div>
      </div>

      <!-- Tailscale -->
      <div class="section-title">VPN / Tailscale</div>
      <div class="settings-card">
        <div class="info-row">
          <span class="info-label">Status</span>
          <q-badge
            :color="tsStatus === 'connected' ? 'green-8' : tsStatus === 'not configured' ? 'grey-7' : 'orange-7'"
            :label="tsStatus"
          />
        </div>
        <q-separator class="card-separator" />
        <div class="input-row">
          <span class="info-label">Auth Key</span>
          <q-input
            v-model="tsAuthKey"
            dense dark outlined
            placeholder="tskey-auth-..."
            class="setting-input"
            type="password"
          />
        </div>
        <q-separator class="card-separator" />
        <div class="input-row">
          <span class="info-label">Control URL</span>
          <q-input
            v-model="tsControlURL"
            dense dark outlined
            placeholder="https://controlplane.tailscale.com (default)"
            class="setting-input"
          />
        </div>
        <q-separator class="card-separator" />
        <div class="input-row">
          <span class="info-label">Hostname</span>
          <q-input
            v-model="tsHostname"
            dense dark outlined
            placeholder="packalares"
            class="setting-input"
          />
        </div>
        <div class="action-row">
          <q-btn
            flat dense
            label="Save & Connect"
            color="primary"
            class="save-btn"
            :loading="saving"
            @click="saveTailscale"
          />
          <span v-if="saveMsg" class="save-msg" :class="saveMsg.startsWith('Error') ? 'text-red-5' : 'text-green-5'">{{ saveMsg }}</span>
        </div>
      </div>

      <!-- Exposed Ports -->
      <div class="section-title">Exposed Ports</div>
      <div class="settings-card">
        <div class="info-row">
          <span class="info-label">HTTPS</span>
          <span class="info-value">443</span>
        </div>
        <q-separator class="card-separator" />
        <div class="info-row">
          <span class="info-label">HTTP</span>
          <span class="info-value">80</span>
        </div>
        <q-separator class="card-separator" />
        <div class="info-row">
          <span class="info-label">SMB</span>
          <span class="info-value">30445</span>
        </div>
      </div>
    </div>
  </div>
</template>

<script lang="ts" setup>
import { ref, onMounted } from 'vue';
import { api } from 'boot/axios';

const netInfo = ref({ ip: '--', domain: '--', zone: '--' });
const tsAuthKey = ref('');
const tsControlURL = ref('');
const tsHostname = ref('packalares');
const tsStatus = ref('not configured');
const saving = ref(false);
const saveMsg = ref('');

onMounted(async () => {
  try {
    const r: any = await api.get('/api/user/info');
    if (r) {
      netInfo.value.zone = r.zone || r.terminusName || '--';
      const parts = (r.zone || '').split('.');
      netInfo.value.domain = parts.length >= 2 ? parts.slice(1).join('.') : '--';
    }
  } catch {}

  // Load saved tailscale config
  tsAuthKey.value = localStorage.getItem('ts_auth_key') || '';
  tsControlURL.value = localStorage.getItem('ts_control_url') || '';
  tsHostname.value = localStorage.getItem('ts_hostname') || 'packalares';

  // Detect IP from window
  const host = window.location.hostname;
  if (/^\d+\.\d+\.\d+\.\d+$/.test(host)) {
    netInfo.value.ip = host;
  }
});

async function saveTailscale() {
  saving.value = true;
  saveMsg.value = '';
  try {
    // Save locally
    localStorage.setItem('ts_auth_key', tsAuthKey.value);
    localStorage.setItem('ts_control_url', tsControlURL.value);
    localStorage.setItem('ts_hostname', tsHostname.value);

    // TODO: POST to /api/settings/tailscale to update K8s Secret
    // For now, just save locally
    saveMsg.value = 'Saved locally (backend API not yet implemented)';
    tsStatus.value = tsAuthKey.value ? 'pending restart' : 'not configured';
  } catch (e: any) {
    saveMsg.value = 'Error: ' + (e.message || 'unknown');
  }
  saving.value = false;
}
</script>

<style lang="scss" scoped>
.settings-page { height: 100%; display: flex; flex-direction: column; }
.page-title { font-size: 18px; font-weight: 600; color: var(--ink-1); padding: 16px 24px; height: 56px; display: flex; align-items: center; flex-shrink: 0; }
.page-scroll { flex: 1; overflow-y: auto; padding: 0 24px 24px; }
.section-title { font-size: 13px; font-weight: 500; color: var(--ink-2); margin-top: 20px; margin-bottom: 8px; text-transform: uppercase; letter-spacing: 0.5px; }
.settings-card { background: var(--bg-2); border-radius: 12px; border: 1px solid var(--separator); overflow: hidden; }
.info-row { display: flex; justify-content: space-between; align-items: center; padding: 14px 20px; }
.info-label { font-size: 14px; color: var(--ink-1); font-weight: 500; white-space: nowrap; min-width: 120px; }
.info-value { font-size: 13px; color: var(--ink-2); font-family: 'JetBrains Mono', monospace; }
.card-separator { background: var(--separator); margin: 0 20px; }
.input-row { display: flex; align-items: center; padding: 10px 20px; gap: 12px; }
.setting-input { flex: 1; }
.action-row { display: flex; align-items: center; gap: 12px; padding: 12px 20px; }
.save-btn { background: rgba(76, 159, 231, 0.15); }
.save-msg { font-size: 12px; }
</style>
