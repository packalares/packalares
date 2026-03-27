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

          <div class="ts-section">
            <div class="ts-header" @click="showTailscale = !showTailscale">
              <span class="ts-header-label">Tailscale (optional)</span>
              <svg
                :class="{ rotated: showTailscale }"
                class="chevron"
                width="16" height="16" viewBox="0 0 24 24"
                fill="none" stroke="currentColor" stroke-width="2"
                stroke-linecap="round" stroke-linejoin="round"
              >
                <polyline points="6 9 12 15 18 9" />
              </svg>
            </div>
            <transition name="expand">
              <div v-if="showTailscale" class="ts-fields">
                <div class="field-group">
                  <label class="field-label">Auth Key</label>
                  <input
                    v-model="tsAuthKey"
                    type="password"
                    class="field-input"
                    placeholder="tskey-auth-..."
                  />
                </div>
                <div class="field-group">
                  <label class="field-label">Control URL</label>
                  <input
                    v-model="tsControlURL"
                    type="text"
                    class="field-input"
                    placeholder="https://controlplane.tailscale.com"
                  />
                </div>
                <div class="field-group">
                  <label class="field-label">Hostname</label>
                  <input
                    v-model="tsHostname"
                    type="text"
                    class="field-input"
                    placeholder="packalares"
                  />
                </div>
              </div>
            </transition>
          </div>

          <div class="card-actions">
            <button class="btn-ghost" @click="step = 1">Back</button>
            <button class="btn-action" @click="step = 3">Continue</button>
          </div>
        </div>

        <!-- Step 3: Complete -->
        <div v-else-if="step === 3" key="complete" class="wizard-card">
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
            <div v-if="tsAuthKey" class="summary-row">
              <span class="summary-label">Tailscale</span>
              <span class="summary-value">Configured</span>
            </div>
          </div>

          <p v-if="activateError" class="field-error">{{ activateError }}</p>

          <div class="card-actions">
            <button class="btn-ghost" :disabled="activating" @click="step = 2">Back</button>
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
import { ref, onMounted } from 'vue';
import axios from 'axios';

// Steps labels
const steps = ['Welcome', 'Account', 'Network', 'Complete'];
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

// Tailscale
const showTailscale = ref(false);
const tsAuthKey = ref('');
const tsControlURL = ref('');
const tsHostname = ref('packalares');

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
    const res = await axios.get('/bfl/backend/v1/user-info');
    const d = res?.data?.data ?? res?.data;
    if (d) {
      if (d.name) username.value = d.name;
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
  try {
    const tsRes = await axios.get('/api/settings/tailscale');
    const ts = tsRes?.data?.data ?? tsRes?.data;
    if (ts) {
      tsAuthKey.value = ts.auth_key || '';
      tsControlURL.value = ts.control_url || '';
      tsHostname.value = ts.hostname || 'packalares';
      if (tsAuthKey.value) showTailscale.value = true;
    }
  } catch {
    // not configured yet
  }
});

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
    // Step 1: Bind zone (sets terminus name, advances wizard status)
    await axios.post('/bfl/settings/v1alpha1/binding-zone', {});

    // Step 2: Activate system (generates TLS cert, sets wizard to wait_reset_password)
    await axios.post('/bfl/settings/v1alpha1/activate', {
      language: navigator.language || 'en-US',
      location: Intl.DateTimeFormat().resolvedOptions().timeZone || '',
      theme: 'dark',
    });

    // Step 3: Set password (advances wizard to completed)
    await axios.put(`/bfl/iam/v1alpha1/users/${username.value}/password`, {
      current_password: '',
      password: password.value,
    });

    // Step 4: Save Tailscale config if provided
    if (tsAuthKey.value) {
      try {
        await axios.post('/api/settings/tailscale', {
          auth_key: tsAuthKey.value,
          hostname: tsHostname.value,
          control_url: tsControlURL.value,
        });
      } catch {
        // non-fatal
      }
    }

    // Done -- redirect to login
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
</style>
