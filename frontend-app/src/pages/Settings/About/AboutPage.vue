<template>
  <div class="settings-page">
    <div class="page-title">About</div>
    <div class="page-scroll">
      <div class="about-hero">
        <div class="about-logo">
          <q-icon name="sym_r_deployed_code" size="40px" color="white" />
        </div>
        <div class="about-name">Packalares</div>
        <div class="about-tagline">Self-hosted OS for your personal cloud</div>
        <span class="version-badge">v1.0.0</span>
      </div>

      <div class="section-title">System</div>
      <div class="settings-card">
        <div class="info-row">
          <span class="info-label">Hostname</span>
          <span class="info-value">{{ info.hostname }}</span>
        </div>
        <q-separator class="card-separator" />
        <div class="info-row">
          <span class="info-label">Operating System</span>
          <span class="info-value">{{ info.os_version }}</span>
        </div>
        <q-separator class="card-separator" />
        <div class="info-row">
          <span class="info-label">Kernel</span>
          <span class="info-value">{{ info.kernel }}</span>
        </div>
        <q-separator class="card-separator" />
        <div class="info-row">
          <span class="info-label">Architecture</span>
          <span class="info-value">{{ info.arch }}</span>
        </div>
        <q-separator class="card-separator" />
        <div class="info-row">
          <span class="info-label">CPU</span>
          <span class="info-value">{{ info.cpu_model }}</span>
        </div>
      </div>

      <div class="section-title">Resources</div>
      <div class="settings-card">
        <div class="link-row" @click="openLink('https://github.com/packalares')">
          <div class="link-info">
            <div class="link-icon-wrap">
              <q-icon name="sym_r_code" size="16px" />
            </div>
            <span class="info-label">Source Code</span>
          </div>
          <q-icon name="sym_r_open_in_new" size="14px" color="grey-6" />
        </div>
        <q-separator class="card-separator" />
        <div class="link-row" @click="openLink('https://github.com/packalares/packalares/issues')">
          <div class="link-info">
            <div class="link-icon-wrap">
              <q-icon name="sym_r_bug_report" size="16px" />
            </div>
            <span class="info-label">Report an Issue</span>
          </div>
          <q-icon name="sym_r_open_in_new" size="14px" color="grey-6" />
        </div>
      </div>

      <div class="about-footer">Built for self-hosted independence.</div>
    </div>
  </div>
</template>

<script lang="ts" setup>
import { ref, onMounted } from 'vue';
import { api } from 'boot/axios';

const info = ref({ hostname: '--', os_version: '--', kernel: '--', arch: '--', cpu_model: '--' });

onMounted(async () => {
  try {
    const r: any = await api.get('/api/monitor/metrics');
    if (r) {
      info.value = {
        hostname: r.hostname || '--',
        os_version: r.os_version || '--',
        kernel: r.kernel || '--',
        arch: r.arch || '--',
        cpu_model: r.cpu_model || '--',
      };
    }
  } catch {}
});

const openLink = (url: string) => window.open(url, '_blank');
</script>

<style lang="scss" scoped>
.about-hero {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 40px 20px 28px;
}

.about-logo {
  width: 72px;
  height: 72px;
  border-radius: 20px;
  background: linear-gradient(135deg, #0a84ff 0%, #5e5ce6 100%);
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: 0 8px 24px rgba(10, 132, 255, 0.25);
}

.about-name {
  font-size: 22px;
  font-weight: 700;
  color: var(--ink-1);
  margin-top: 16px;
  letter-spacing: -0.3px;
}

.about-tagline {
  font-size: 13px;
  color: var(--ink-3);
  margin-top: 4px;
}

.version-badge {
  margin-top: 8px;
  font-size: 11px;
  font-weight: 600;
  color: var(--accent);
  background: var(--accent-soft);
  padding: 3px 10px;
  border-radius: 6px;
}

.link-icon-wrap {
  width: 28px;
  height: 28px;
  border-radius: 7px;
  background: var(--glass);
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--ink-3);
}

.about-footer {
  text-align: center;
  font-size: 11px;
  color: var(--ink-3);
  margin-top: 32px;
  padding-bottom: 16px;
  letter-spacing: 0.3px;
}
</style>
