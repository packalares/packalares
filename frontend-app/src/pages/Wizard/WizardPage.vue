<template>
  <div class="wizard-root">
    <!-- Step indicator -->
    <div class="step-indicator">
      <div
        v-for="(s, i) in steps"
        :key="i"
        class="step-dot-group"
        :class="{ active: i === step, done: i < step }"
      >
        <div class="step-dot">
          <svg v-if="i < step" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round">
            <polyline points="20 6 9 17 4 12" />
          </svg>
          <span v-else class="step-num">{{ i + 1 }}</span>
        </div>
        <span class="step-label">{{ s }}</span>
      </div>
    </div>

    <!-- Card container with transitions -->
    <div class="wizard-stage">
      <transition name="slide" mode="out-in">

        <!-- Step 0: Welcome -->
        <div v-if="step === 0" key="welcome" class="wizard-card welcome-card">
          <div class="logo-mark">
            <svg width="56" height="56" viewBox="0 0 56 56" fill="none">
              <rect width="56" height="56" rx="14" fill="var(--accent-bold)" />
              <path d="M16 28L24 20L32 28L24 36Z" fill="rgba(255,255,255,0.9)" />
              <path d="M24 28L32 20L40 28L32 36Z" fill="rgba(255,255,255,0.55)" />
            </svg>
          </div>
          <h1 class="card-title">Welcome to Packalares</h1>
          <p class="card-desc">
            Let's set up your personal cloud. This wizard will guide you through
            creating your admin account, reviewing your network configuration,
            and activating the system.
          </p>
          <button class="btn-action" @click="step = 1">Get Started</button>
        </div>

        <!-- Step 1: Create Account -->
        <div v-else-if="step === 1" key="account" class="wizard-card">
          <h1 class="card-title">Create Account</h1>
          <p class="card-desc">Set your administrator username and password.</p>

          <div class="field-group">
            <label class="field-label">Username</label>
            <input
              v-model="username"
              type="text"
              class="field-input"
              autocomplete="username"
              spellcheck="false"
            />
          </div>

          <div class="field-group">
            <label class="field-label">Password</label>
            <input
              v-model="password"
              type="password"
              class="field-input"
              autocomplete="new-password"
              placeholder="Minimum 8 characters"
            />
          </div>

          <div class="field-group">
            <label class="field-label">Confirm Password</label>
            <input
              v-model="passwordConfirm"
              type="password"
              class="field-input"
              autocomplete="new-password"
              placeholder="Re-enter password"
              @keydown.enter="goToNetwork"
            />
          </div>

          <p v-if="accountError" class="field-error">{{ accountError }}</p>

          <div class="card-actions">
            <button class="btn-ghost" @click="step = 0">Back</button>
            <button class="btn-action" @click="goToNetwork">Continue</button>
          </div>
        </div>

        <!-- Step 2: Domain & Network -->
        <div v-else-if="step === 2" key="network" class="wizard-card">
          <h1 class="card-title">Domain &amp; Network</h1>
          <p class="card-desc">Review your network configuration. Tailscale settings are optional.</p>

          <div class="info-grid">
            <div class="info-cell">
              <span class="info-cell-label">Hostname</span>
              <span class="info-cell-value">{{ sysHostname || '--' }}</span>
            </div>
            <div class="info-cell">
              <span class="info-cell-label">Domain</span>
              <span class="info-cell-value">{{ sysDomain || '--' }}</span>
            </div>
            <div class="info-cell">
              <span class="info-cell-label">Server IP</span>
              <span class="info-cell-value">{{ serverIP || '--' }}</span>
            </div>
            <div class="info-cell">
              <span class="info-cell-label">User Zone</span>
              <span class="info-cell-value">{{ sysZone || '--' }}</span>
            </div>
          </div>

          <p class="card-hint">You can configure Tailscale VPN and SSH access in Settings → Network after setup.</p>

          <div class="card-actions">
            <button class="btn-ghost" @click="step = 1">Back</button>
            <button class="btn-action" @click="goToCerts">Continue</button>
          </div>
        </div>

        <!-- Step 3: Certificate Setup (only for domain access) -->
        <div v-else-if="step === 3" key="certs" class="wizard-card">
          <h1 class="card-title">Certificate Setup</h1>
          <p class="card-desc">
            Your browser needs to trust the self-signed certificate for each subdomain.
            Click each link below and accept the certificate warning.
          </p>

          <div class="cert-list">
            <div v-for="sub in certSubdomains" :key="sub.name" class="cert-row">
              <div class="cert-status" :class="sub.ok ? 'cert-ok' : (sub.pending ? 'cert-pending' : 'cert-fail')">
                <svg v-if="sub.ok" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12" /></svg>
                <span v-else-if="sub.pending" class="cert-spinner"></span>
                <svg v-else width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10" /><line x1="15" y1="9" x2="9" y2="15" /><line x1="9" y1="9" x2="15" y2="15" /></svg>
              </div>
              <span class="cert-name">{{ sub.name }}</span>
              <button v-if="!sub.ok && !sub.pending" class="cert-accept-btn" @click="openCertPopup(sub)">Accept</button>
              <span v-else-if="sub.pending" class="cert-waiting">Waiting...</span>
              <span v-else class="cert-accepted">Trusted</span>
            </div>
          </div>

          <div class="cert-download">
            <p class="cert-download-text">Or download and install the CA certificate to trust all subdomains at once:</p>
            <a :href="caDownloadUrl" download="packalares-ca.crt" class="cert-download-link">
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>
              Download CA Certificate
            </a>
          </div>

          <div class="card-actions">
            <button class="btn-ghost" @click="step = 2">Back</button>
            <button class="btn-action" :disabled="!allCertsOk" @click="step = 4">
              {{ allCertsOk ? 'Continue' : 'Accept all certificates to continue' }}
            </button>
          </div>
        </div>

        <!-- Step 4: Complete -->
        <div v-else-if="step === 4" key="complete" class="wizard-card">
          <h1 class="card-title">Ready to Activate</h1>
          <p class="card-desc">Review your settings and activate Packalares.</p>

          <div class="summary-grid">
            <div class="summary-row">
              <span class="summary-label">Username</span>
              <span class="summary-value">{{ username }}</span>
            </div>
            <div class="summary-row">
              <span class="summary-label">Domain</span>
              <span class="summary-value">{{ sysDomain || '--' }}</span>
            </div>
            <div class="summary-row">
              <span class="summary-label">Server IP</span>
              <span class="summary-value">{{ serverIP || '--' }}</span>
            </div>
            <div class="summary-row">
              <span class="summary-label">User Zone</span>
              <span class="summary-value">{{ sysZone || '--' }}</span>
            </div>
          </div>

          <p v-if="activateError" class="field-error">{{ activateError }}</p>

          <div class="card-actions">
            <button class="btn-ghost" :disabled="activating" @click="step = needsCerts ? 3 : 2">Back</button>
            <button class="btn-action" :disabled="activating" @click="activate">
              <span v-if="activating" class="spinner-inline" />
              {{ activating ? 'Activating...' : 'Launch Desktop' }}
            </button>
          </div>
        </div>

      </transition>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue';
import axios from 'axios';

// Steps labels (dynamic — certs step only for domain access)
const needsCerts = ref(false);
const steps = computed(() => {
  if (needsCerts.value) return ['Welcome', 'Account', 'Network', 'Certificates', 'Complete'];
  return ['Welcome', 'Account', 'Network', 'Complete'];
});
const step = ref(0);

// Account
const username = ref('admin');
const password = ref('');
const passwordConfirm = ref('');
const accountError = ref('');

// Network info
const sysHostname = ref('');
const sysDomain = ref('');
const sysZone = ref('');
const serverIP = ref('');

// Certificates
interface CertCheck { name: string; ok: boolean; pending: boolean }
const certSubdomains = ref<CertCheck[]>([]);
const allCertsOk = computed(() => certSubdomains.value.length > 0 && certSubdomains.value.every(c => c.ok));
const caDownloadUrl = computed(() => {
  const host = window.location.hostname;
  if (/^\d/.test(host)) return `https://${host}/ca.crt`;
  return '/ca.crt';
});
let certCheckTimer: ReturnType<typeof setInterval> | null = null;

// Activation
const activating = ref(false);
const activateError = ref('');

// ---------- Init ----------
onMounted(async () => {
  // Detect server IP from browser
  const host = window.location.hostname;
  if (/^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}$/.test(host)) {
    serverIP.value = host;
  }

  // Load user info
  try {
    const res = await axios.get('/api/user/info');
    const d = res?.data?.data ?? res?.data;
    if (d) {
      if (d.name) username.value = d.name;
      if (d.server_ip) serverIP.value = d.server_ip;
      if (d.zone) {
        sysZone.value = d.zone;
        const parts = d.zone.split('.');
        sysHostname.value = parts[0] || '';
        sysDomain.value = parts.length >= 2 ? parts.slice(1).join('.') : '';
      }
      if (d.terminusName) {
        const atIdx = (d.terminusName as string).indexOf('@');
        if (atIdx >= 0) {
          sysDomain.value = (d.terminusName as string).slice(atIdx + 1);
        }
      }
    }
  } catch {
    // Try olares-info endpoint
    try {
      const res2 = await axios.get('/bfl/info/v1/olares-info');
      const d2 = res2?.data?.data ?? res2?.data;
      if (d2?.olaresId) {
        const atIdx = (d2.olaresId as string).indexOf('@');
        if (atIdx >= 0) {
          username.value = (d2.olaresId as string).slice(0, atIdx);
          sysDomain.value = (d2.olaresId as string).slice(atIdx + 1);
          sysZone.value = username.value + '.' + sysDomain.value;
          sysHostname.value = username.value;
        }
      }
    } catch {
      // defaults remain
    }
  }

  // Load Tailscale config
  // Tailscale configured post-wizard in Settings → Network
});

// ---------- Cert checking ----------
function initCertSubdomains() {
  if (!sysZone.value) return;
  const prefixes = ['api', 'desktop', 'auth', 'settings', 'market', 'dashboard', 'files'];
  certSubdomains.value = prefixes.map(p => ({ name: `${p}.${sysZone.value}`, ok: false, pending: false }));
}

async function checkOneCert(sub: CertCheck): Promise<boolean> {
  try {
    await fetch(`https://${sub.name}/api/auth/health`, { cache: 'no-store' });
    return true;
  } catch {
    return false;
  }
}

async function checkAllCerts() {
  for (const sub of certSubdomains.value) {
    if (!sub.ok) {
      sub.ok = await checkOneCert(sub);
      if (sub.ok) sub.pending = false;
    }
  }
}

function openCertPopup(sub: CertCheck) {
  sub.pending = true;
  const popup = window.open(
    `https://${sub.name}`,
    'cert_accept',
    'width=600,height=450,scrollbars=yes,resizable=yes'
  );

  // Poll until the popup loads successfully (cert accepted) or is closed
  const pollId = setInterval(async () => {
    // Check if popup was closed by user
    if (!popup || popup.closed) {
      clearInterval(pollId);
      sub.ok = await checkOneCert(sub);
      sub.pending = false;
      return;
    }

    // Try to detect if the cert was accepted
    const ok = await checkOneCert(sub);
    if (ok) {
      clearInterval(pollId);
      sub.ok = true;
      sub.pending = false;
      try { popup.close(); } catch {}
    }
  }, 1500);
}

function goToCerts() {
  const host = window.location.hostname;
  needsCerts.value = !/^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}$/.test(host);
  if (needsCerts.value) {
    initCertSubdomains();
    checkAllCerts();
    certCheckTimer = setInterval(checkAllCerts, 3000);
    step.value = 3;
  } else {
    step.value = 4; // skip certs for IP access
  }
}

// ---------- Validation ----------
function goToNetwork() {
  accountError.value = '';
  if (!username.value.trim()) {
    accountError.value = 'Username is required.';
    return;
  }
  if (password.value.length < 8) {
    accountError.value = 'Password must be at least 8 characters.';
    return;
  }
  if (password.value !== passwordConfirm.value) {
    accountError.value = 'Passwords do not match.';
    return;
  }
  step.value = 2;
}

// ---------- Activate ----------
async function activate() {
  activating.value = true;
  activateError.value = '';

  try {
    // Set password and mark wizard complete
    await axios.put(`/bfl/iam/v1alpha1/users/${username.value}/password`, {
      current_password: '',
      password: password.value,
    }, { headers: { 'X-Requested-With': 'packalares' } });

    // Done — redirect to login
    window.location.href = '/login';
  } catch (err: any) {
    const msg =
      err?.response?.data?.message ||
      err?.response?.data?.data?.message ||
      err?.message ||
      'Activation failed. Please try again.';
    activateError.value = msg;
  } finally {
    activating.value = false;
  }
}

onUnmounted(() => {
  if (certCheckTimer) clearInterval(certCheckTimer);
});
</script>

<style scoped>
/* ===== Root layout ===== */
.wizard-root {
  width: 100vw;
  height: 100vh;
  background: var(--bg-0, #131316);
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  font-family: 'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
  -webkit-font-smoothing: antialiased;
  overflow: hidden;
  position: relative;
}

/* Subtle background texture */
.wizard-root::before {
  content: '';
  position: absolute;
  inset: 0;
  background:
    radial-gradient(ellipse 80% 60% at 50% -10%, rgba(99, 102, 241, 0.08) 0%, transparent 60%),
    radial-gradient(ellipse 60% 50% at 80% 100%, rgba(129, 140, 248, 0.04) 0%, transparent 50%);
  pointer-events: none;
}

/* ===== Step indicator ===== */
.step-indicator {
  display: flex;
  gap: 32px;
  margin-bottom: 40px;
  z-index: 1;
}

.step-dot-group {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
}

.step-dot {
  width: 32px;
  height: 32px;
  border-radius: 50%;
  background: var(--bg-3, #262730);
  border: 2px solid var(--border, rgba(255,255,255,0.07));
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--ink-3, rgba(226,228,234,0.32));
  font-size: 13px;
  font-weight: 600;
  transition: all 0.3s ease;
}

.step-dot-group.active .step-dot {
  background: var(--accent-bold, #6366f1);
  border-color: var(--accent-bold, #6366f1);
  color: #fff;
  box-shadow: 0 0 0 4px rgba(99, 102, 241, 0.2), 0 2px 8px rgba(99, 102, 241, 0.3);
}

.step-dot-group.done .step-dot {
  background: var(--positive, #34d399);
  border-color: var(--positive, #34d399);
  color: #fff;
}

.step-num {
  font-variant-numeric: tabular-nums;
}

.step-label {
  font-size: 11px;
  font-weight: 600;
  color: var(--ink-3, rgba(226,228,234,0.32));
  text-transform: uppercase;
  letter-spacing: 0.06em;
  transition: color 0.3s ease;
}

.step-dot-group.active .step-label {
  color: var(--ink-1, #e2e4ea);
}

.step-dot-group.done .step-label {
  color: var(--positive, #34d399);
}

/* ===== Card container ===== */
.wizard-stage {
  z-index: 1;
  width: 100%;
  max-width: 520px;
  padding: 0 24px;
}

.wizard-card {
  background: var(--bg-2, #1e1f25);
  border: 1px solid var(--border, rgba(255,255,255,0.07));
  border-radius: 16px;
  padding: 40px;
  box-shadow:
    0 1px 3px rgba(0,0,0,0.24),
    0 8px 32px rgba(0,0,0,0.2);
}

.welcome-card {
  text-align: center;
}

/* ===== Logo ===== */
.logo-mark {
  margin-bottom: 24px;
  display: inline-block;
}

/* ===== Typography ===== */
.card-title {
  font-size: 22px;
  font-weight: 700;
  color: var(--ink-1, #e2e4ea);
  margin: 0 0 8px;
  letter-spacing: -0.02em;
}

.card-desc {
  font-size: 14px;
  color: var(--ink-2, rgba(226,228,234,0.55));
  margin: 0 0 28px;
  line-height: 1.6;
}

/* ===== Fields ===== */
.field-group {
  margin-bottom: 16px;
}

.field-label {
  display: block;
  font-size: 12px;
  font-weight: 600;
  color: var(--ink-2, rgba(226,228,234,0.55));
  margin-bottom: 6px;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.field-input {
  width: 100%;
  height: 42px;
  padding: 0 14px;
  background: var(--input-bg, rgba(255,255,255,0.04));
  border: 1px solid var(--input-border, rgba(255,255,255,0.08));
  border-radius: 8px;
  color: var(--ink-1, #e2e4ea);
  font-family: 'Inter', sans-serif;
  font-size: 14px;
  font-weight: 500;
  outline: none;
  transition: border-color 0.15s, box-shadow 0.15s;
  box-sizing: border-box;
}

.field-input::placeholder {
  color: var(--ink-3, rgba(226,228,234,0.32));
}

.field-input:focus {
  border-color: var(--accent, #818cf8);
  box-shadow: 0 0 0 3px var(--input-focus, rgba(129,140,248,0.25));
}

.field-error {
  color: var(--negative, #f87171);
  font-size: 13px;
  font-weight: 500;
  margin: 4px 0 0;
}

/* ===== Info grid (network step) ===== */
.info-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
  margin-bottom: 20px;
}

.info-cell {
  background: var(--bg-3, #262730);
  border-radius: 10px;
  padding: 14px 16px;
  border: 1px solid var(--border, rgba(255,255,255,0.07));
}

.info-cell-label {
  display: block;
  font-size: 11px;
  font-weight: 600;
  color: var(--ink-3, rgba(226,228,234,0.32));
  text-transform: uppercase;
  letter-spacing: 0.06em;
  margin-bottom: 4px;
}

.info-cell-value {
  font-size: 13px;
  font-weight: 600;
  color: var(--ink-1, #e2e4ea);
  font-family: 'Inter', sans-serif;
  word-break: break-all;
}

/* ===== Tailscale collapsible ===== */
.ts-section {
  margin-bottom: 20px;
  border: 1px solid var(--border, rgba(255,255,255,0.07));
  border-radius: 10px;
  overflow: hidden;
}

.ts-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 16px;
  cursor: pointer;
  transition: background 0.1s;
}

.ts-header:hover {
  background: rgba(255,255,255,0.02);
}

.ts-header-label {
  font-size: 13px;
  font-weight: 600;
  color: var(--ink-2, rgba(226,228,234,0.55));
}

.chevron {
  color: var(--ink-3, rgba(226,228,234,0.32));
  transition: transform 0.25s ease;
}

.chevron.rotated {
  transform: rotate(180deg);
}

.ts-fields {
  padding: 0 16px 16px;
}

/* ===== Summary grid (complete step) ===== */
.summary-grid {
  margin-bottom: 24px;
  border: 1px solid var(--border, rgba(255,255,255,0.07));
  border-radius: 10px;
  overflow: hidden;
}

.summary-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 16px;
}

.summary-row + .summary-row {
  border-top: 1px solid var(--separator, rgba(255,255,255,0.05));
}

.summary-label {
  font-size: 13px;
  color: var(--ink-2, rgba(226,228,234,0.55));
  font-weight: 500;
}

.summary-value {
  font-size: 13px;
  color: var(--ink-1, #e2e4ea);
  font-family: 'Inter', sans-serif;
  font-weight: 500;
}

/* ===== Buttons ===== */
.card-actions {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  margin-top: 8px;
}

.btn-action {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  height: 40px;
  padding: 0 24px;
  background: var(--accent-bold, #6366f1);
  color: #fff;
  border: none;
  border-radius: 8px;
  font-family: 'Inter', sans-serif;
  font-size: 14px;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.15s ease;
  box-shadow: 0 1px 3px rgba(99,102,241,0.35), inset 0 1px 0 rgba(255,255,255,0.12);
}

.btn-action:hover:not(:disabled) {
  background: #7c83f7;
  box-shadow: 0 2px 10px rgba(99,102,241,0.45), inset 0 1px 0 rgba(255,255,255,0.15);
  transform: translateY(-0.5px);
}

.btn-action:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.btn-ghost {
  height: 40px;
  padding: 0 20px;
  background: rgba(255,255,255,0.05);
  color: var(--ink-2, rgba(226,228,234,0.55));
  border: none;
  border-radius: 8px;
  font-family: 'Inter', sans-serif;
  font-size: 14px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.12s ease;
}

.btn-ghost:hover:not(:disabled) {
  background: rgba(255,255,255,0.08);
  color: var(--ink-1, #e2e4ea);
}

.btn-ghost:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

/* ===== Inline spinner ===== */
.spinner-inline {
  width: 16px;
  height: 16px;
  border: 2px solid rgba(255,255,255,0.3);
  border-top-color: #fff;
  border-radius: 50%;
  animation: spin 0.7s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

/* ===== Slide transition ===== */
.slide-enter-active,
.slide-leave-active {
  transition: opacity 0.25s ease, transform 0.25s ease;
}

.slide-enter-from {
  opacity: 0;
  transform: translateX(24px);
}

.slide-leave-to {
  opacity: 0;
  transform: translateX(-24px);
}

/* ===== Expand transition ===== */
.expand-enter-active,
.expand-leave-active {
  transition: all 0.25s ease;
  overflow: hidden;
  max-height: 300px;
}

.expand-enter-from,
.expand-leave-to {
  max-height: 0;
  opacity: 0;
  padding-top: 0;
  padding-bottom: 0;
}

/* ===== Responsive ===== */
@media (max-width: 560px) {
  .wizard-card {
    padding: 28px 20px;
  }
  .step-indicator {
    gap: 16px;
  }
  .step-label {
    display: none;
  }
  .info-grid {
    grid-template-columns: 1fr;
  }
}

/* ===== Certificate step ===== */
.cert-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin: 16px 0;
}

.cert-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 12px;
  background: rgba(255, 255, 255, 0.03);
  border-radius: 8px;
  border: 1px solid rgba(255, 255, 255, 0.05);
}

.cert-status {
  width: 22px;
  height: 22px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.cert-ok {
  background: rgba(52, 211, 153, 0.15);
  color: #34d399;
}

.cert-fail {
  background: rgba(248, 113, 113, 0.15);
  color: #f87171;
}

.cert-name {
  flex: 1;
  font-size: 13px;
  color: rgba(255, 255, 255, 0.7);
  font-family: 'Inter', monospace;
}

.cert-pending {
  background: rgba(251, 191, 36, 0.15);
  color: #fbbf24;
}

.cert-spinner {
  width: 10px;
  height: 10px;
  border: 2px solid rgba(251, 191, 36, 0.3);
  border-top-color: #fbbf24;
  border-radius: 50%;
  animation: cert-spin 0.8s linear infinite;
}

@keyframes cert-spin {
  to { transform: rotate(360deg); }
}

.cert-accept-btn {
  font-size: 12px;
  color: #818cf8;
  font-weight: 600;
  padding: 4px 12px;
  border-radius: 6px;
  background: rgba(129, 140, 248, 0.1);
  border: none;
  cursor: pointer;
  transition: background 0.15s;
}

.cert-accept-btn:hover {
  background: rgba(129, 140, 248, 0.2);
}

.cert-waiting {
  font-size: 12px;
  color: #fbbf24;
  font-weight: 500;
}

.cert-accepted {
  font-size: 12px;
  color: #34d399;
  font-weight: 500;
}

.cert-download {
  margin: 20px 0 8px;
  padding: 14px 16px;
  background: rgba(255, 255, 255, 0.02);
  border: 1px solid rgba(255, 255, 255, 0.06);
  border-radius: 10px;
}

.cert-download-text {
  font-size: 12px;
  color: rgba(255, 255, 255, 0.45);
  margin: 0 0 10px;
  line-height: 1.5;
}

.cert-download-link {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: #818cf8;
  text-decoration: none;
  font-weight: 600;
  padding: 6px 14px;
  border-radius: 8px;
  background: rgba(129, 140, 248, 0.1);
  transition: background 0.15s;
}

.cert-download-link:hover {
  background: rgba(129, 140, 248, 0.2);
}
</style>
