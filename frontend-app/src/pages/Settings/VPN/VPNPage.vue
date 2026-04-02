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
      <div v-if="vpnStatus.type !== 'none'" class="info-grid-2col">
        <div class="info-row">
          <span class="info-label">Type</span>
          <span class="info-value">{{ vpnStatus.type === 'tailscale' ? 'Tailscale' : 'WireGuard' }}</span>
        </div>
        <div class="info-row">
          <span class="info-label">IP Address</span>
          <span class="info-value">{{ vpnStatus.ip || '--' }}</span>
        </div>
      </div>
      <div v-else class="card-body">
        <span class="text-muted">No VPN active</span>
      </div>
    </div>

    <!-- Tailscale -->
    <div class="settings-card q-mt-sm">
      <div class="card-header">
        <div class="card-header-icon card-header-icon--network">
          <q-icon name="sym_r_cloud" size="18px" />
        </div>
        <div class="card-header-text">
          <div class="card-header-title">Tailscale</div>
          <div class="card-header-subtitle">Connect via Tailscale / Headscale network</div>
        </div>
        <div v-if="vpnStatus.type === 'tailscale'" class="card-header-actions">
          <span class="status-badge status-connected">Active</span>
        </div>
      </div>

      <!-- Tailscale live status -->
      <template v-if="vpnStatus.type === 'tailscale' && vpnStatus.tailscale">
        <div class="info-grid-2col">
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
        </div>
        <!-- Peers -->
        <template v-if="vpnStatus.tailscale.peers?.length">
          <div class="card-body">
            <table class="data-table">
              <thead><tr><th>Name</th><th>IP</th><th>Status</th></tr></thead>
              <tbody>
                <tr v-for="p in vpnStatus.tailscale.peers" :key="p.name">
                  <td>{{ p.name }}</td>
                  <td>{{ p.ip }}</td>
                  <td><span class="badge" :class="p.online ? 'badge-green' : 'badge-gray'">{{ p.online ? 'online' : 'offline' }}</span></td>
                </tr>
              </tbody>
            </table>
          </div>
        </template>
      </template>

      <!-- Tailscale config form -->
      <div class="form-grid cols-1">
        <div class="form-group">
          <span class="form-label">Auth Key</span>
          <q-input v-model="tsAuthKey" type="password" dense outlined placeholder="tskey-auth-..." />
        </div>
        <div class="form-group">
          <span class="form-label">Control URL</span>
          <q-input v-model="tsControlURL" dense outlined placeholder="https://controlplane.tailscale.com" />
        </div>
        <div class="form-group">
          <span class="form-label">Hostname</span>
          <q-input v-model="tsHostname" dense outlined placeholder="packalares" />
        </div>
      </div>
      <div class="card-footer">
        <q-btn
          v-if="vpnStatus.type !== 'tailscale'"
          label="Connect"
          color="primary"
          no-caps dense
          :loading="tsLoading"
          @click="enableTailscale"
        />
        <q-btn
          v-else
          label="Disconnect"
          color="negative"
          flat no-caps dense
          :loading="tsLoading"
          @click="disableTailscale"
        />
      </div>
    </div>

    <!-- WireGuard -->
    <div class="settings-card q-mt-sm">
      <div class="card-header">
        <div class="card-header-icon card-header-icon--security">
          <q-icon name="sym_r_shield" size="18px" />
        </div>
        <div class="card-header-text">
          <div class="card-header-title">WireGuard</div>
          <div class="card-header-subtitle">Connect via WireGuard VPN tunnel</div>
        </div>
        <div v-if="vpnStatus.type === 'wireguard'" class="card-header-actions">
          <span class="status-badge status-connected">Active</span>
        </div>
      </div>

      <!-- WireGuard live status -->
      <template v-if="vpnStatus.wireguard?.active">
        <div class="info-grid-2col">
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
        </div>
      </template>

      <!-- WireGuard config form -->
      <div class="form-grid cols-1">
        <div class="form-group">
          <span class="form-label">Configuration</span>
          <q-input
            v-model="wgConfig"
            type="textarea"
            dense outlined
            placeholder="[Interface]
PrivateKey = ...
Address = 10.8.0.2/32
DNS = 10.8.0.1

[Peer]
PublicKey = ...
Endpoint = vpn.example.com:51820
AllowedIPs = 0.0.0.0/0"
            :rows="8"
            input-class="mono-textarea"
          />
        </div>
        <div class="form-group">
          <q-toggle v-model="wgKillSwitch" label="Kill Switch" />
          <span class="form-hint">Block all internet if VPN disconnects. LAN access preserved.</span>
        </div>
      </div>
      <div class="card-footer">
        <q-btn
          v-if="vpnStatus.type !== 'wireguard'"
          label="Enable"
          color="primary"
          no-caps dense
          :loading="wgLoading"
          :disable="!wgConfig.trim()"
          @click="enableWireGuard"
        />
        <q-btn
          v-else
          label="Disable"
          color="negative"
          flat no-caps dense
          :loading="wgLoading"
          @click="disableWireGuard"
        />
      </div>
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

// Tailscale form
const tsAuthKey = ref('');
const tsControlURL = ref('');
const tsHostname = ref('packalares');
const tsLoading = ref(false);

// WireGuard form
const wgConfig = ref('');
const wgKillSwitch = ref(false);
const wgLoading = ref(false);

let pollTimer: ReturnType<typeof setInterval>;

const statusClass = computed(() => {
  if (vpnStatus.value.connected) return 'status-connected';
  if (vpnStatus.value.type !== 'none') return 'status-connecting';
  return 'status-disconnected';
});

const statusLabel = computed(() => {
  if (vpnStatus.value.connected) return 'Connected';
  if (vpnStatus.value.type !== 'none') return 'Connecting...';
  return 'Disconnected';
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
  try {
    await api.post('/api/settings/tailscale', {
      auth_key: tsAuthKey.value,
      hostname: tsHostname.value,
      control_url: tsControlURL.value,
    });
    $q.notify({ type: 'positive', message: 'Tailscale connecting...' });
    setTimeout(loadStatus, 3000);
  } catch (e: any) {
    $q.notify({ type: 'negative', message: e?.message || 'Failed' });
  } finally {
    tsLoading.value = false;
  }
}

async function disableTailscale() {
  tsLoading.value = true;
  try {
    await api.post('/api/settings/vpn/tailscale/disable');
    $q.notify({ type: 'info', message: 'Tailscale disconnected' });
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
  try {
    await api.post('/api/settings/vpn/wireguard/enable', {
      config: wgConfig.value,
      killSwitch: wgKillSwitch.value,
    });
    $q.notify({ type: 'positive', message: 'WireGuard enabled' });
    setTimeout(loadStatus, 2000);
  } catch (e: any) {
    $q.notify({ type: 'negative', message: e?.response?.data?.error || e?.message || 'Failed' });
  } finally {
    wgLoading.value = false;
  }
}

async function disableWireGuard() {
  wgLoading.value = true;
  try {
    await api.post('/api/settings/vpn/wireguard/disable');
    $q.notify({ type: 'info', message: 'WireGuard disabled' });
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

.text-muted {
  font-size: 13px;
  color: var(--ink-3);
}

.card-footer {
  padding: 10px 16px;
  display: flex;
  justify-content: flex-end;
  border-top: 1px solid var(--separator);
}
</style>
