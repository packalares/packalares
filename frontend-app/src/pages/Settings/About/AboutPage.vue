<template>
  <div class="settings-page">
    <div class="page-title">About</div>
    <div class="page-scroll">
      <div class="about-hero">
        <div class="about-logo">
          <q-icon name="sym_r_deployed_code" size="56px" color="blue-5" />
        </div>
        <div class="about-name">Packalares</div>
        <div class="about-tagline">Self-hosted OS for your personal cloud</div>
        <q-badge color="blue-8" label="v1.0.0" class="q-mt-sm" />
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
            <q-icon name="sym_r_code" size="18px" color="grey-5" class="q-mr-sm" />
            <span class="info-label">Source Code</span>
          </div>
          <q-icon name="sym_r_open_in_new" size="16px" color="grey-6" />
        </div>
        <q-separator class="card-separator" />
        <div class="link-row" @click="openLink('https://github.com/packalares/packalares/issues')">
          <div class="link-info">
            <q-icon name="sym_r_bug_report" size="18px" color="grey-5" class="q-mr-sm" />
            <span class="info-label">Report an Issue</span>
          </div>
          <q-icon name="sym_r_open_in_new" size="16px" color="grey-6" />
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
.settings-page { height: 100%; display: flex; flex-direction: column; }
.page-title { font-size: 18px; font-weight: 600; color: var(--ink-1); padding: 16px 24px; height: 56px; display: flex; align-items: center; flex-shrink: 0; }
.page-scroll { flex: 1; overflow-y: auto; padding: 0 24px 24px; }
.about-hero { display: flex; flex-direction: column; align-items: center; padding: 32px 20px 24px; }
.about-logo { width: 80px; height: 80px; border-radius: 20px; background: var(--bg-2); border: 1px solid var(--separator); display: flex; align-items: center; justify-content: center; }
.about-name { font-size: 24px; font-weight: 700; color: var(--ink-1); margin-top: 12px; }
.about-tagline { font-size: 13px; color: var(--ink-3); margin-top: 4px; }
.section-title { font-size: 13px; font-weight: 500; color: var(--ink-2); margin-top: 20px; margin-bottom: 8px; text-transform: uppercase; letter-spacing: 0.5px; }
.settings-card { background: var(--bg-2); border-radius: 12px; border: 1px solid var(--separator); overflow: hidden; }
.info-row { display: flex; justify-content: space-between; align-items: center; padding: 14px 20px; }
.info-label { font-size: 14px; color: var(--ink-1); font-weight: 500; }
.info-value { font-size: 13px; color: var(--ink-2); font-family: 'JetBrains Mono', monospace; }
.card-separator { background: var(--separator); margin: 0 20px; }
.link-row { display: flex; justify-content: space-between; align-items: center; padding: 14px 20px; cursor: pointer; transition: background-color 0.15s; &:hover { background: var(--glass); } }
.link-info { display: flex; align-items: center; }
.about-footer { text-align: center; font-size: 12px; color: var(--ink-3); margin-top: 32px; padding-bottom: 16px; }
</style>
