<template>
  <div class="settings-page">
    <div class="page-header">
      <div class="page-title">VPN</div>
      <div class="page-description">Configure VPN access. Only one VPN can be active at a time.</div>
    </div>
    <div class="page-scroll">

    <!-- VPN Status -->
    <div class="settings-card">
      <div class="card-header">
        <div class="card-header-icon card-header-icon--vpn">
          <q-icon name="sym_r_vpn_lock" size="18px" />
        </div>
        <div class="card-header-text">
          <div class="card-header-title">VPN Status</div>
          <div class="card-header-subtitle">Current VPN connection</div>
        </div>
        <div class="card-header-actions">
          <span class="status-badge" :class="statusClass">{{ statusLabel }}</span>
        </div>
      </div>
      <template v-if="vpnStatus.type !== 'none'">
        <div class="info-row">
          <span class="info-label">Type</span>
          <span class="info-value">{{ vpnStatus.type === 'tailscale' ? 'Tailscale' : 'WireGuard' }}</span>
        </div>
        <div class="info-row">
          <span class="info-label">IP Address</span>
          <span class="info-value">{{ vpnStatus.ip || '--' }}</span>
        </div>
      </template>
      <div v-else class="card-body">
        <div style="font-size:11px;color:var(--ink-3);line-height:1.5">
          <q-icon name="sym_r_info" size="13px" style="vertical-align:middle;margin-right:4px" />
          No VPN active. Enable Tailscale or WireGuard below.
        </div>
      </div>
    </div>

    <!-- Tailscale -->
    <div class="settings-card q-mt-lg">
      <div class="card-header">
        <div class="card-header-icon card-header-icon--network">
          <q-icon name="sym_r_cloud" size="18px" />
        </div>
        <div class="card-header-text">
          <div class="card-header-title">Tailscale</div>
          <div class="card-header-subtitle">Connect via Tailscale / Headscale network</div>
        </div>
        <div v-if="vpnStatus.type === 'tailscale'" class="card-header-actions">
          <span class="status-badge status-connected">active</span>
        </div>
      </div>

      <!-- Tailscale live status -->
      <template v-if="vpnStatus.type === 'tailscale' && vpnStatus.tailscale">
        <div class="info-row">
          <span class="info-label">Status</span>
          <span class="info-value" style="display:flex;align-items:center;gap:6px">
            <span class="status-dot dot-green"></span> Connected
          </span>
        </div>
        <div class="info-row">
          <span class="info-label">IP</span>
          <span class="info-value">{{ vpnStatus.ip }}</span>
        </div>
        <div class="info-row">
          <span class="info-label">Hostname</span>
          <span class="info-value">{{ vpnStatus.tailscale.hostname || '--' }}</span>
        </div>
        <template v-if="vpnStatus.tailscale.peers?.length">
          <q-separator class="card-separator" />
          <div style="padding:0 16px 8px">
            <div style="font-size:11px;color:var(--ink-3);margin-bottom:6px;font-weight:600">Peers</div>
            <table class="data-table" style="margin:0">
              <thead><tr><th>Name</th><th>IP</th><th style="text-align:right">Status</th></tr></thead>
              <tbody>
                <tr v-for="p in vpnStatus.tailscale.peers" :key="p.name">
                  <td class="td-label">{{ p.name }}</td>
                  <td class="td-mono">{{ p.ip }}</td>
                  <td style="text-align:right">
                    <span class="status-badge" :class="p.online ? 'status-connected' : 'status-disconnected'" style="font-size:10px">{{ p.online ? 'online' : 'offline' }}</span>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </template>
        <q-separator class="card-separator" />
        <div class="card-footer">
          <q-btn
            unelevated dense
            label="Disconnect"
            class="btn-danger"
            :loading="tsLoading"
            @click="disableTailscale"
          />
        </div>
      </template>

      <!-- Tailscale config form (when not connected) -->
      <template v-else>
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
          <span v-if="tsMsg" class="footer-msg" :class="tsMsg.startsWith('Error') ? 'text-red-5' : 'text-green-5'">{{ tsMsg }}</span>
          <q-btn
            unelevated dense
            label="Connect"
            class="btn-primary"
            :loading="tsLoading"
            @click="enableTailscale"
          />
        </div>
      </template>
    </div>

    <!-- WireGuard -->
    <div class="settings-card q-mt-lg">
      <div class="card-header">
        <div class="card-header-icon card-header-icon--security">
          <q-icon name="sym_r_shield" size="18px" />
        </div>
        <div class="card-header-text">
          <div class="card-header-title">WireGuard</div>
          <div class="card-header-subtitle">Connect via WireGuard VPN tunnel</div>
        </div>
        <div v-if="vpnStatus.type === 'wireguard'" class="card-header-actions">
          <span class="status-badge status-connected">active</span>
        </div>
      </div>

      <!-- WireGuard live status (when connected) -->
      <template v-if="vpnStatus.wireguard?.active">
        <div class="info-row">
          <span class="info-label">Status</span>
          <span class="info-value" style="display:flex;align-items:center;gap:6px">
            <span class="status-dot dot-green"></span> Connected
          </span>
        </div>
        <div class="info-row">
          <span class="info-label">IP</span>
          <span class="info-value">{{ vpnStatus.wireguard.ip }}</span>
        </div>
        <div class="info-row">
          <span class="info-label">Endpoint</span>
          <span class="info-value">{{ vpnStatus.wireguard.endpoint || '--' }}</span>
        </div>
        <div class="info-row">
          <span class="info-label">Last Handshake</span>
          <span class="info-value">{{ vpnStatus.wireguard.latestHandshake || '--' }}</span>
        </div>
        <div class="info-row">
          <span class="info-label">Transfer</span>
          <span class="info-value">{{ vpnStatus.wireguard.transfer || '--' }}</span>
        </div>
        <div class="info-row">
          <span class="info-label">Kill Switch</span>
          <span class="info-value">{{ vpnStatus.wireguard.killSwitch ? 'Active' : 'Off' }}</span>
        </div>
        <div class="card-footer">
          <q-btn
            unelevated dense
            label="Disable"
            class="btn-danger"
            :loading="wgLoading"
            @click="disableWireGuard"
          />
        </div>
      </template>

      <!-- WireGuard config form (when not connected) -->
      <template v-else>
        <div class="form-grid cols-1">
          <div class="form-group">
            <label class="form-label">Configuration</label>
            <q-input
              v-model="wgConfig"
              type="textarea"
              dense dark outlined
              :rows="8"
              input-class="mono-textarea"
              placeholder="[Interface]
PrivateKey = ...
Address = 10.8.0.2/32
DNS = 10.8.0.1

[Peer]
PublicKey = ...
Endpoint = vpn.example.com:51820
AllowedIPs = 0.0.0.0/0"
            />
          </div>
        </div>
        <q-separator class="card-separator" />
        <div class="info-row">
          <span class="info-label">Kill Switch</span>
          <q-toggle v-model="wgKillSwitch" dense color="primary" />
        </div>
        <div class="card-body" style="padding-top:0">
          <div style="font-size:11px;color:var(--ink-3);line-height:1.5">
            <q-icon name="sym_r_info" size="13px" style="vertical-align:middle;margin-right:4px" />
            Block all internet traffic if VPN disconnects. LAN and cluster access preserved.
          </div>
        </div>
        <div class="card-footer">
          <span v-if="wgMsg" class="footer-msg" :class="wgMsg.startsWith('Error') ? 'text-red-5' : 'text-green-5'">{{ wgMsg }}</span>
          <q-btn
            unelevated dense
            label="Enable"
            class="btn-primary"
            :loading="wgLoading"
            :disable="!wgConfig.trim()"
            @click="enableWireGuard"
          />
        </div>
      </template>
    </div>

    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue';
import { useQuasar } from 'quasar';
import { api } from 'boot/axios';

const $q = useQuasar();

const vpnStatus = ref<any>({ type: 'none', ip: '', connected: false });

// Tailscale
const tsAuthKey = ref('');
const tsControlURL = ref('');
const tsHostname = ref('packalares');
const tsLoading = ref(false);
const tsMsg = ref('');

// WireGuard
const wgConfig = ref('');
const wgKillSwitch = ref(true);
const wgLoading = ref(false);
const wgMsg = ref('');

let pollTimer: ReturnType<typeof setInterval>;

const statusClass = computed(() => {
  if (vpnStatus.value.connected) return 'status-connected';
  if (vpnStatus.value.type !== 'none') return 'status-connecting';
  return 'status-disconnected';
});

const statusLabel = computed(() => {
  if (vpnStatus.value.connected) return 'connected';
  if (vpnStatus.value.type !== 'none') return 'connecting';
  return 'disconnected';
});

async function loadStatus() {
  try {
    const res: any = await api.get('/api/settings/vpn/status');
    vpnStatus.value = res?.data ?? res;
  } catch {}
}

async function loadTailscaleConfig() {
  try {
    const res: any = await api.get('/api/settings/tailscale');
    const d = res?.data ?? res;
    tsAuthKey.value = d.auth_key || '';
    tsControlURL.value = d.control_url || '';
    tsHostname.value = d.hostname || 'packalares';
  } catch {}
}

async function enableTailscale() {
  if (vpnStatus.value.type === 'wireguard') {
    const ok = await confirmSwitch('WireGuard');
    if (!ok) return;
  }
  tsLoading.value = true;
  tsMsg.value = '';
  try {
    await api.post('/api/settings/tailscale', {
      auth_key: tsAuthKey.value,
      hostname: tsHostname.value,
      control_url: tsControlURL.value,
    });
    tsMsg.value = 'Connecting...';
    setTimeout(loadStatus, 3000);
  } catch (e: any) {
    tsMsg.value = 'Error: ' + (e?.message || 'unknown');
  } finally {
    tsLoading.value = false;
  }
}

async function disableTailscale() {
  tsLoading.value = true;
  try {
    await api.post('/api/settings/vpn/tailscale/disable');
    vpnStatus.value = { type: 'none', ip: '', connected: false };
  } catch (e: any) {
    $q.notify({ type: 'negative', message: e?.message || 'Failed' });
  } finally {
    tsLoading.value = false;
  }
}

async function enableWireGuard() {
  if (vpnStatus.value.type === 'tailscale') {
    const ok = await confirmSwitch('Tailscale');
    if (!ok) return;
  }
  wgLoading.value = true;
  wgMsg.value = '';
  try {
    await api.post('/api/settings/vpn/wireguard/enable', {
      config: wgConfig.value,
      killSwitch: wgKillSwitch.value,
    });
    wgMsg.value = 'Enabling...';
    setTimeout(loadStatus, 3000);
  } catch (e: any) {
    wgMsg.value = 'Error: ' + (e?.response?.data?.error || e?.message || 'unknown');
  } finally {
    wgLoading.value = false;
  }
}

async function disableWireGuard() {
  wgLoading.value = true;
  try {
    await api.post('/api/settings/vpn/wireguard/disable');
    vpnStatus.value = { type: 'none', ip: '', connected: false };
  } catch (e: any) {
    $q.notify({ type: 'negative', message: e?.message || 'Failed' });
  } finally {
    wgLoading.value = false;
  }
}

function confirmSwitch(currentVPN: string): Promise<boolean> {
  return new Promise((resolve) => {
    $q.dialog({
      title: 'Switch VPN',
      message: `${currentVPN} is currently active. Enabling a different VPN will disconnect it. Continue?`,
      cancel: true,
      persistent: true,
    }).onOk(() => resolve(true)).onCancel(() => resolve(false));
  });
}

onMounted(async () => {
  await Promise.all([loadStatus(), loadTailscaleConfig()]);
  pollTimer = setInterval(loadStatus, 10000);
});

onUnmounted(() => {
  clearInterval(pollTimer);
});
</script>

<style scoped lang="scss">
.mono-textarea {
  font-family: 'JetBrains Mono', 'Fira Code', monospace !important;
  font-size: 12px !important;
  line-height: 1.5 !important;
}
</style>
