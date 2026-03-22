<template>
  <div class="settings-page">
    <div class="page-title">Network</div>
    <div class="page-scroll">
      <!-- Network Interfaces -->
      <div class="section-title">Network Configuration</div>
      <div class="settings-card">
        <div class="info-row">
          <span class="info-label">IP Address</span>
          <span class="info-value">{{ networkInfo.ip_address }}</span>
        </div>
        <q-separator class="card-separator" />
        <div class="info-row">
          <span class="info-label">Gateway</span>
          <span class="info-value">{{ networkInfo.gateway }}</span>
        </div>
        <q-separator class="card-separator" />
        <div class="info-row">
          <span class="info-label">DNS Servers</span>
          <span class="info-value">{{ networkInfo.dns }}</span>
        </div>
        <q-separator class="card-separator" />
        <div class="info-row">
          <span class="info-label">MAC Address</span>
          <span class="info-value">{{ networkInfo.mac_address }}</span>
        </div>
      </div>

      <!-- Tailscale Status -->
      <div class="section-title">VPN / Tailscale</div>
      <div class="settings-card">
        <div class="info-row">
          <span class="info-label">Tailscale IP</span>
          <span class="info-value">{{ networkInfo.tailscale_ip }}</span>
        </div>
        <q-separator class="card-separator" />
        <div class="info-row">
          <span class="info-label">Status</span>
          <q-badge
            :color="networkInfo.tailscale_status === 'connected' ? 'green-8' : 'grey-7'"
            :label="networkInfo.tailscale_status"
          />
        </div>
      </div>

      <!-- Ports -->
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
          <span class="info-label">SSH</span>
          <span class="info-value">22</span>
        </div>
      </div>
    </div>
  </div>
</template>

<script lang="ts" setup>
import { ref, onMounted } from 'vue';
import { api } from 'boot/axios';

interface NetworkInfo {
  ip_address: string;
  gateway: string;
  dns: string;
  mac_address: string;
  tailscale_ip: string;
  tailscale_status: string;
}

const networkInfo = ref<NetworkInfo>({
  ip_address: '--',
  gateway: '--',
  dns: '--',
  mac_address: '--',
  tailscale_ip: '--',
  tailscale_status: 'unknown',
});

onMounted(async () => {
  try {
    const res: any = await api.get('/api/metrics');
    if (res) {
      networkInfo.value = {
        ip_address: res.ip_address || res.network?.ip_address || '--',
        gateway: res.gateway || res.network?.gateway || '--',
        dns: res.dns || res.network?.dns || '--',
        mac_address: res.mac_address || res.network?.mac_address || '--',
        tailscale_ip: res.tailscale_ip || res.network?.tailscale_ip || '--',
        tailscale_status: res.tailscale_status || res.network?.tailscale_status || 'unknown',
      };
    }
  } catch {
    // keep defaults
  }
});
</script>

<style lang="scss" scoped>
.settings-page {
  height: 100%;
  display: flex;
  flex-direction: column;
}

.page-title {
  font-size: 18px;
  font-weight: 600;
  color: var(--ink-1);
  padding: 16px 24px;
  height: 56px;
  display: flex;
  align-items: center;
  flex-shrink: 0;
}

.page-scroll {
  flex: 1;
  overflow-y: auto;
  padding: 0 24px 24px;
}

.section-title {
  font-size: 13px;
  font-weight: 500;
  color: var(--ink-2);
  margin-top: 20px;
  margin-bottom: 8px;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.settings-card {
  background: var(--bg-2);
  border-radius: 12px;
  border: 1px solid var(--separator);
  overflow: hidden;
}

.info-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 14px 20px;
}

.info-label {
  font-size: 14px;
  color: var(--ink-1);
  font-weight: 500;
}

.info-value {
  font-size: 13px;
  color: var(--ink-2);
  font-family: 'JetBrains Mono', 'SF Mono', monospace;
}

.card-separator {
  background: var(--separator);
  margin: 0 20px;
}
</style>
