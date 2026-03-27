<template>
  <div class="settings-page">
    <div class="page-header">
      <div class="page-title">Network</div>
      <div class="page-description">Manage network interfaces, VPN connections, SSH access, and exposed ports.</div>
    </div>
    <div class="page-scroll">

      <!-- Network Configuration -->
      <div class="settings-card">
        <div class="card-header">
          <div class="card-header-icon card-header-icon--network">
            <q-icon name="sym_r_lan" size="18px" />
          </div>
          <div class="card-header-text">
            <div class="card-header-title">Network Configuration</div>
            <div class="card-header-subtitle">Server identity and routing information</div>
          </div>
        </div>
        <div class="info-grid-2col">
          <div class="info-row">
            <span class="info-label">Server IP</span>
            <span class="info-value">{{ netInfo.ip }}</span>
          </div>
          <div class="info-row">
            <span class="info-label">Domain</span>
            <span class="info-value">{{ netInfo.domain }}</span>
          </div>
        </div>
        <q-separator class="card-separator" />
        <div class="info-row">
          <span class="info-label">User Zone</span>
          <span class="info-value">{{ netInfo.zone }}</span>
        </div>
      </div>

      <!-- SSH Access -->
      <div class="settings-card q-mt-lg">
        <div class="card-header">
          <div class="card-header-icon card-header-icon--security">
            <q-icon name="sym_r_terminal" size="18px" />
          </div>
          <div class="card-header-text">
            <div class="card-header-title">SSH Access</div>
            <div class="card-header-subtitle">Secure shell login configuration</div>
          </div>
          <div class="card-header-actions">
            <span
              class="status-badge"
              :class="sshStatus.enabled ? 'status-connected' : 'status-disconnected'"
            >{{ sshStatus.enabled ? 'active' : 'inactive' }}</span>
          </div>
        </div>
        <div class="info-row">
          <span class="info-label">Enable SSH</span>
          <q-toggle
            v-model="sshEnabled"
            dense
            color="primary"
          />
        </div>
        <q-separator class="card-separator" />
        <div class="form-grid cols-1">
          <div class="form-group">
            <label class="form-label">Port</label>
            <q-input
              v-model.number="sshPort"
              dense dark outlined
              type="number"
              :rules="[portRule]"
              style="max-width: 140px"
            />
          </div>
        </div>
        <div class="card-footer">
          <span v-if="sshMsg" class="footer-msg" :class="sshMsg.startsWith('Error') ? 'text-red-5' : 'text-green-5'">{{ sshMsg }}</span>
          <q-btn
            unelevated dense
            label="Apply"
            class="btn-primary"
            :loading="sshSaving"
            @click="applySSH"
          />
        </div>
      </div>

      <!-- Tailscale VPN -->
      <div class="settings-card q-mt-lg">
        <div class="card-header">
          <div class="card-header-icon card-header-icon--vpn">
            <q-icon name="sym_r_vpn_lock" size="18px" />
          </div>
          <div class="card-header-text">
            <div class="card-header-title">VPN / Tailscale</div>
            <div class="card-header-subtitle">Connect to your Tailscale network for secure remote access</div>
          </div>
          <div class="card-header-actions">
            <span
              class="status-badge"
              :class="tsStatus === 'connected' ? 'status-connected' : tsStatus === 'connecting' ? 'status-connecting' : 'status-disconnected'"
            >{{ tsStatus }}</span>
          </div>
        </div>
        <div class="form-grid cols-1">
          <div class="form-group">
            <label class="form-label">Auth Key</label>
            <q-input
              v-model="tsAuthKey"
              dense dark outlined
              placeholder="tskey-auth-..."
              type="password"
            />
          </div>
          <div class="form-group">
            <label class="form-label">Control URL</label>
            <q-input
              v-model="tsControlURL"
              dense dark outlined
              placeholder="https://controlplane.tailscale.com"
            />
          </div>
          <div class="form-group">
            <label class="form-label">Hostname</label>
            <q-input
              v-model="tsHostname"
              dense dark outlined
              placeholder="packalares"
            />
          </div>
        </div>
        <div class="card-footer">
          <span v-if="saveMsg" class="footer-msg" :class="saveMsg.startsWith('Error') ? 'text-red-5' : 'text-green-5'">{{ saveMsg }}</span>
          <q-btn
            unelevated dense
            label="Save & Connect"
            class="btn-primary"
            :loading="saving"
            @click="saveTailscale"
          />
        </div>
      </div>

      <!-- Exposed Ports -->
      <div class="settings-card q-mt-lg">
        <div class="card-header">
          <div class="card-header-icon card-header-icon--ports">
            <q-icon name="sym_r_swap_horiz" size="18px" />
          </div>
          <div class="card-header-text">
            <div class="card-header-title">Exposed Ports</div>
            <div class="card-header-subtitle">Services accessible from the network</div>
          </div>
        </div>
        <table class="data-table">
          <thead>
            <tr>
              <th>Service</th>
              <th>Protocol</th>
              <th style="text-align:right">Port</th>
            </tr>
          </thead>
          <tbody>
            <tr>
              <td class="td-label">Web UI</td>
              <td class="td-mono">HTTPS</td>
              <td style="text-align:right"><span class="port-badge">443</span></td>
            </tr>
            <tr>
              <td class="td-label">HTTP Redirect</td>
              <td class="td-mono">HTTP</td>
              <td style="text-align:right"><span class="port-badge">80</span></td>
            </tr>
            <tr>
              <td class="td-label">SSH</td>
              <td class="td-mono">TCP</td>
              <td style="text-align:right"><span class="port-badge">{{ sshStatus.port }}</span></td>
            </tr>
            <tr>
              <td class="td-label">Kubernetes API</td>
              <td class="td-mono">HTTPS</td>
              <td style="text-align:right"><span class="port-badge">6443</span></td>
            </tr>
            <tr>
              <td class="td-label">Tailscale P2P</td>
              <td class="td-mono">UDP</td>
              <td style="text-align:right"><span class="port-badge">41641</span></td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

<script lang="ts" setup>
import { ref, reactive, onMounted } from 'vue';
import { api } from 'boot/axios';

const netInfo = ref({ ip: '--', domain: '--', zone: '--' });
const tsAuthKey = ref('');
const tsControlURL = ref('');
const tsHostname = ref('packalares');
const tsStatus = ref('not configured');
const saving = ref(false);
const saveMsg = ref('');

const sshStatus = reactive({
  enabled: false,
  port: 22,
});
const sshEnabled = ref(false);
const sshPort = ref(22);
const sshSaving = ref(false);
const sshMsg = ref('');

function portRule(val: number): boolean | string {
  if (val === 22) return true;
  if (val >= 1024 && val <= 65535) return true;
  return 'Port must be 22 or 1024-65535';
}

onMounted(async () => {
  try {
    const r: any = await api.get('/api/user/info');
    const d = r?.data ?? r;
    if (d) {
      netInfo.value.zone = d.zone || d.terminusName || '--';
      const parts = (d.zone || '').split('.');
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

  // Load SSH status
  try {
    const ssh: any = await api.get('/api/settings/ssh');
    const sshData = ssh?.data ?? ssh;
    if (sshData) {
      sshStatus.enabled = sshData.enabled ?? false;
      sshStatus.port = sshData.port ?? 22;
      sshEnabled.value = sshStatus.enabled;
      sshPort.value = sshStatus.port;
    }
  } catch {
    // SSH endpoint may not be available; keep defaults
  }

  // Detect server IP from parent window or current host
  const host = window.location.hostname;
  const parentHost = window.parent?.location?.hostname || '';
  if (/^\d+\.\d+\.\d+\.\d+$/.test(parentHost)) {
    netInfo.value.ip = parentHost;
  } else if (/^\d+\.\d+\.\d+\.\d+$/.test(host)) {
    netInfo.value.ip = host;
  }
});

async function applySSH() {
  sshSaving.value = true;
  sshMsg.value = '';
  try {
    const resp: any = await api.post('/api/settings/ssh', {
      enabled: sshEnabled.value,
      port: sshPort.value,
    });
    const d = resp?.data ?? resp;
    if (d && d.enabled !== undefined) {
      sshStatus.enabled = d.enabled;
      sshStatus.port = d.port;
      sshEnabled.value = d.enabled;
      sshPort.value = d.port;
      sshMsg.value = 'Applied successfully';
    } else if (resp?.code === 1) {
      sshMsg.value = 'Error: ' + (resp?.message || 'unknown');
    } else {
      sshMsg.value = 'Applied successfully';
    }
  } catch (e: any) {
    sshMsg.value = 'Error: ' + (e.message || 'unknown');
  }
  sshSaving.value = false;
}

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
</style>
