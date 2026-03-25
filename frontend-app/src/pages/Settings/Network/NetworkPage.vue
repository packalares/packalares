<template>
  <div class="settings-page">
    <div class="page-title">Network</div>
    <div class="page-scroll">
      <!-- Network Info -->
      <div class="section-title">Configuration</div>
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
          <span
            class="status-badge"
            :class="tsStatus === 'connected' ? 'status-connected' : tsStatus === 'connecting' ? 'status-connecting' : 'status-disconnected'"
          >{{ tsStatus }}</span>
        </div>
        <q-separator class="card-separator" />
        <div class="input-row">
          <span class="input-label">Auth Key</span>
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
          <span class="input-label">Control URL</span>
          <q-input
            v-model="tsControlURL"
            dense dark outlined
            placeholder="https://controlplane.tailscale.com"
            class="setting-input"
          />
        </div>
        <q-separator class="card-separator" />
        <div class="input-row">
          <span class="input-label">Hostname</span>
          <q-input
            v-model="tsHostname"
            dense dark outlined
            placeholder="packalares"
            class="setting-input"
          />
        </div>
        <div class="action-row">
          <q-btn
            unelevated dense
            label="Save & Connect"
            class="btn-primary"
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
          <span class="info-value port-value">443</span>
        </div>
        <q-separator class="card-separator" />
        <div class="info-row">
          <span class="info-label">HTTP</span>
          <span class="info-value port-value">80</span>
        </div>
        <q-separator class="card-separator" />
        <div class="info-row">
          <span class="info-label">SMB</span>
          <span class="info-value port-value">30445</span>
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

  try {
    const ts: any = await api.get('/api/settings/tailscale');
    tsAuthKey.value = ts?.auth_key || ts?.data?.auth_key || '';
    tsControlURL.value = ts?.control_url || ts?.data?.control_url || '';
    tsHostname.value = ts?.hostname || ts?.data?.hostname || 'packalares';
    tsStatus.value = tsAuthKey.value ? 'configured' : 'not configured';
  } catch {
    tsAuthKey.value = localStorage.getItem('ts_auth_key') || '';
    tsControlURL.value = localStorage.getItem('ts_control_url') || '';
    tsHostname.value = localStorage.getItem('ts_hostname') || 'packalares';
  }

  const host = window.location.hostname;
  if (/^\d+\.\d+\.\d+\.\d+$/.test(host)) {
    netInfo.value.ip = host;
  }
});

async function saveTailscale() {
  saving.value = true;
  saveMsg.value = '';
  try {
    await api.post('/api/settings/tailscale', {
      auth_key: tsAuthKey.value,
      hostname: tsHostname.value,
      control_url: tsControlURL.value,
    });
    saveMsg.value = 'Saved. Tailscale restarting...';
    tsStatus.value = tsAuthKey.value ? 'connecting' : 'not configured';
    try {
      const ts: any = await api.get('/api/settings/tailscale');
      tsAuthKey.value = ts?.auth_key || ts?.data?.auth_key || '';
      tsControlURL.value = ts?.control_url || ts?.data?.control_url || '';
      tsHostname.value = ts?.hostname || ts?.data?.hostname || 'packalares';
      tsStatus.value = tsAuthKey.value ? 'connecting' : 'not configured';
    } catch {}
  } catch (e: any) {
    saveMsg.value = 'Error: ' + (e.message || 'unknown');
  }
  saving.value = false;
}
</script>

<style lang="scss" scoped>
.port-value {
  background: var(--bg-3);
  padding: 3px 10px;
  border-radius: var(--radius-xs);
  font-size: 12px;
  font-weight: 500;
}
</style>
