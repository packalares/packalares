<template>
  <div class="settings-page">
    <div class="page-header">
      <h2>VPN</h2>
      <p class="page-desc">Configure VPN access. Only one VPN can be active at a time.</p>
    </div>

    <!-- Status Card -->
    <div class="settings-card">
      <div class="card-header">
        <div class="card-header-icon card-header-icon--vpn">
          <q-icon name="sym_r_vpn_lock" size="20px" />
        </div>
        <div>
          <h3>VPN Status</h3>
          <p>Current VPN connection</p>
        </div>
        <span class="status-badge" :class="statusClass">
          <span class="status-dot" />
          {{ statusLabel }}
        </span>
      </div>
      <div v-if="vpnStatus.type !== 'none'" class="info-grid-2col">
        <div class="info-row"><span class="info-label">Type</span><span class="info-value">{{ vpnStatus.type === 'tailscale' ? 'Tailscale' : 'WireGuard' }}</span></div>
        <div class="info-row"><span class="info-label">IP Address</span><span class="info-value">{{ vpnStatus.ip || '--' }}</span></div>
      </div>
      <div v-else class="info-note">No VPN active</div>
    </div>

    <!-- Tailscale Card -->
    <div class="settings-card">
      <div class="card-header">
        <div class="card-header-icon">
          <q-icon name="sym_r_cloud" size="20px" />
        </div>
        <div>
          <h3>Tailscale</h3>
          <p>Connect via Tailscale / Headscale network</p>
        </div>
        <span v-if="vpnStatus.type === 'tailscale'" class="status-badge status-connected">
          <span class="status-dot" /> Active
        </span>
      </div>

      <div class="form-section">
        <div class="form-group">
          <label>Auth Key</label>
          <q-input v-model="tsAuthKey" type="password" dense outlined placeholder="tskey-auth-..." />
        </div>
        <div class="form-group">
          <label>Control URL</label>
          <q-input v-model="tsControlURL" dense outlined placeholder="https://controlplane.tailscale.com" />
        </div>
        <div class="form-group">
          <label>Hostname</label>
          <q-input v-model="tsHostname" dense outlined placeholder="packalares" />
        </div>
        <div class="form-actions">
          <q-btn
            v-if="vpnStatus.type !== 'tailscale'"
            label="Connect"
            color="primary"
            no-caps
            :loading="tsLoading"
            @click="enableTailscale"
          />
          <q-btn
            v-else
            label="Disconnect"
            color="negative"
            flat
            no-caps
            :loading="tsLoading"
            @click="disableTailscale"
          />
        </div>
      </div>

      <!-- Tailscale Peers -->
      <div v-if="vpnStatus.tailscale?.peers?.length" class="peers-section">
        <h4>Peers</h4>
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
    </div>

    <!-- WireGuard Card -->
    <div class="settings-card">
      <div class="card-header">
        <div class="card-header-icon">
          <q-icon name="sym_r_shield" size="20px" />
        </div>
        <div>
          <h3>WireGuard</h3>
          <p>Connect via WireGuard VPN tunnel</p>
        </div>
        <span v-if="vpnStatus.type === 'wireguard'" class="status-badge status-connected">
          <span class="status-dot" /> Active
        </span>
      </div>

      <div class="form-section">
        <div class="form-group">
          <label>Configuration</label>
          <q-input
            v-model="wgConfig"
            type="textarea"
            dense
            outlined
            placeholder="[Interface]
PrivateKey = ...
Address = 10.8.0.2/32
DNS = 10.8.0.1

[Peer]
PublicKey = ...
Endpoint = vpn.example.com:51820
AllowedIPs = 0.0.0.0/0"
            :rows="8"
            class="mono-input"
          />
        </div>
        <div class="form-group">
          <q-toggle v-model="wgKillSwitch" label="Kill Switch" />
          <p class="form-hint">Block all internet traffic if VPN disconnects. LAN access is preserved.</p>
        </div>
        <div class="form-actions">
          <q-btn
            v-if="vpnStatus.type !== 'wireguard'"
            label="Enable"
            color="primary"
            no-caps
            :loading="wgLoading"
            :disable="!wgConfig.trim()"
            @click="enableWireGuard"
          />
          <q-btn
            v-else
            label="Disable"
            color="negative"
            flat
            no-caps
            :loading="wgLoading"
            @click="disableWireGuard"
          />
        </div>
      </div>

      <!-- WireGuard Status Details -->
      <div v-if="vpnStatus.wireguard?.active" class="info-grid-2col" style="margin-top:12px">
        <div class="info-row"><span class="info-label">Public Key</span><span class="info-value mono">{{ vpnStatus.wireguard.publicKey }}</span></div>
        <div class="info-row"><span class="info-label">Endpoint</span><span class="info-value">{{ vpnStatus.wireguard.endpoint }}</span></div>
        <div class="info-row"><span class="info-label">Last Handshake</span><span class="info-value">{{ vpnStatus.wireguard.latestHandshake || '--' }}</span></div>
        <div class="info-row"><span class="info-label">Transfer</span><span class="info-value">{{ vpnStatus.wireguard.transfer || '--' }}</span></div>
        <div class="info-row"><span class="info-label">Kill Switch</span><span class="info-value">{{ vpnStatus.wireguard.killSwitch ? 'Active' : 'Off' }}</span></div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue';
import { useQuasar } from 'quasar';
import { api } from 'boot/axios';

const $q = useQuasar();

// VPN status
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

// Status polling
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
.mono-input :deep(textarea) {
  font-family: 'JetBrains Mono', 'Fira Code', monospace;
  font-size: 12px;
  line-height: 1.5;
}

.mono {
  font-family: 'JetBrains Mono', 'Fira Code', monospace;
  font-size: 11px;
  word-break: break-all;
}

.peers-section {
  margin-top: 16px;
  h4 {
    font-size: 12px;
    font-weight: 600;
    color: var(--text-secondary);
    margin: 0 0 8px;
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }
}

.form-hint {
  font-size: 11px;
  color: var(--text-tertiary);
  margin: 2px 0 0;
}

.info-note {
  font-size: 13px;
  color: var(--text-tertiary);
  padding: 8px 0;
}
</style>
