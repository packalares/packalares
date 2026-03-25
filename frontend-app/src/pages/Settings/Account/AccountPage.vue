<template>
  <div class="settings-page">
    <div class="page-title">Account</div>
    <div class="page-scroll">
      <!-- Profile -->
      <div class="settings-card profile-card">
        <div class="profile-header">
          <div class="profile-avatar">
            <q-icon name="sym_r_person" size="28px" color="white" />
          </div>
          <div class="profile-info">
            <div class="profile-name">{{ userInfo.name || 'admin' }}</div>
            <div class="profile-email">{{ userInfo.email || 'Local administrator' }}</div>
          </div>
          <span class="role-badge">{{ userInfo.role || 'admin' }}</span>
        </div>
      </div>

      <!-- Password -->
      <div class="section-title">Change Password</div>
      <div class="settings-card">
        <div class="input-row">
          <span class="input-label">Current</span>
          <q-input v-model="currentPassword" dense dark outlined type="password" class="setting-input" />
        </div>
        <div class="input-row">
          <span class="input-label">New</span>
          <q-input v-model="newPassword" dense dark outlined type="password" class="setting-input" />
        </div>
        <div class="input-row">
          <span class="input-label">Confirm</span>
          <q-input v-model="confirmPassword" dense dark outlined type="password" class="setting-input" />
        </div>
        <div class="action-row">
          <q-btn unelevated dense label="Change Password" class="btn-primary" :loading="changingPw" @click="changePassword" />
          <span v-if="pwMsg" class="save-msg" :class="pwMsg.startsWith('Error') ? 'text-red-5' : 'text-green-5'">{{ pwMsg }}</span>
        </div>
      </div>

      <!-- TOTP 2FA -->
      <div class="section-title">Two-Factor Authentication</div>
      <div class="settings-card">
        <div class="info-row">
          <span class="info-label">TOTP Authenticator</span>
          <span
            class="status-badge"
            :class="totpEnabled ? 'status-connected' : 'status-disconnected'"
          >{{ totpEnabled ? 'Enabled' : 'Disabled' }}</span>
        </div>
        <template v-if="!totpEnabled">
          <q-separator class="card-separator" />
          <div class="totp-setup" v-if="totpURI">
            <div class="totp-instructions">Scan this QR code with your authenticator app, then enter the 6-digit code below.</div>
            <div class="totp-qr">
              <img :src="'https://api.qrserver.com/v1/create-qr-code/?size=180x180&data=' + encodeURIComponent(totpURI)" alt="TOTP QR" />
            </div>
            <div class="totp-secret-display">
              <span class="totp-secret-label">Secret</span>
              <code class="totp-secret-code">{{ totpSecret }}</code>
            </div>
            <div class="input-row">
              <span class="input-label">Code</span>
              <q-input v-model="totpCode" dense dark outlined placeholder="000000" maxlength="6" class="setting-input" @keyup.enter="verifyTOTP" />
            </div>
            <div class="action-row">
              <q-btn unelevated dense label="Verify & Enable" class="btn-primary" @click="verifyTOTP" />
              <q-btn flat dense label="Cancel" class="btn-ghost" @click="totpURI = ''" />
            </div>
          </div>
          <div v-else class="action-row">
            <q-btn unelevated dense label="Setup TOTP" class="btn-primary" @click="setupTOTP" />
          </div>
        </template>
        <template v-else>
          <div class="action-row">
            <q-btn flat dense label="Disable TOTP" class="btn-danger" @click="disableTOTP" />
          </div>
        </template>
        <span v-if="totpMsg" class="save-msg q-ml-md q-mb-sm" :class="totpMsg.startsWith('Error') ? 'text-red-5' : 'text-green-5'">{{ totpMsg }}</span>
      </div>

      <!-- Active Sessions -->
      <div class="section-title">Active Sessions</div>
      <div class="settings-card">
        <div v-if="sessions.length === 0" class="empty-state">No active sessions</div>
        <template v-for="(s, i) in sessions" :key="s.id">
          <div class="session-row">
            <div class="session-info">
              <div class="session-icon-wrap">
                <q-icon name="sym_r_devices" size="16px" />
              </div>
              <div>
                <div class="session-id">{{ s.id }}</div>
                <div class="session-time">Last active: {{ new Date(s.last_activity).toLocaleString() }}</div>
              </div>
            </div>
            <q-btn flat dense round icon="sym_r_close" size="xs" color="negative" @click="revokeSession(s.id)" />
          </div>
          <q-separator v-if="i < sessions.length - 1" class="card-separator" />
        </template>
      </div>

      <!-- Logout -->
      <div class="section-title">Session</div>
      <div class="settings-card">
        <div class="action-row">
          <q-btn flat dense label="Logout" class="btn-danger" icon="sym_r_logout" @click="logout" />
        </div>
      </div>
    </div>
  </div>
</template>

<script lang="ts" setup>
import { ref, onMounted } from 'vue';
import { api } from 'boot/axios';
import { useQuasar } from 'quasar';

const $q = useQuasar();
const userInfo = ref({ name: '', email: '', role: 'admin' });
const currentPassword = ref('');
const newPassword = ref('');
const confirmPassword = ref('');
const changingPw = ref(false);
const pwMsg = ref('');
const sessions = ref<{id: string; last_activity: string; auth_level: number}[]>([]);
const totpEnabled = ref(false);
const totpURI = ref('');
const totpSecret = ref('');
const totpCode = ref('');
const totpMsg = ref('');

onMounted(async () => {
  try {
    const r: any = await api.get('/api/user/info');
    if (r) { userInfo.value = { name: r.name || r.username || 'admin', email: r.email || '', role: r.role || 'admin' }; }
  } catch {}

  try {
    const s: any = await api.get('/api/auth/state');
    totpEnabled.value = s?.totp_enabled || s?.data?.totp_enabled || false;
  } catch {}

  try {
    const s: any = await api.get('/api/auth/sessions');
    sessions.value = s?.sessions || [];
  } catch {}
});

async function changePassword() {
  if (!currentPassword.value || !newPassword.value) { pwMsg.value = 'Error: fill all fields'; return; }
  if (newPassword.value !== confirmPassword.value) { pwMsg.value = 'Error: passwords don\'t match'; return; }
  if (newPassword.value.length < 6) { pwMsg.value = 'Error: min 6 characters'; return; }
  changingPw.value = true; pwMsg.value = '';
  try {
    await api.post('/api/auth/password', { current_password: currentPassword.value, new_password: newPassword.value });
    pwMsg.value = 'Password changed';
    currentPassword.value = ''; newPassword.value = ''; confirmPassword.value = '';
  } catch (e: any) { pwMsg.value = 'Error: ' + (e?.response?.data?.message || 'failed'); }
  changingPw.value = false;
}

async function setupTOTP() {
  totpMsg.value = '';
  try {
    const r: any = await api.post('/api/auth/totp/setup', {});
    totpURI.value = r?.data?.otpauth || r?.otpauth || '';
    totpSecret.value = r?.data?.secret || r?.secret || '';
  } catch (e: any) { totpMsg.value = 'Error: ' + (e?.message || 'failed to generate'); }
}

async function verifyTOTP() {
  if (!totpCode.value || totpCode.value.length !== 6) { totpMsg.value = 'Error: enter 6-digit code'; return; }
  try {
    await api.post('/api/auth/totp', { token: totpCode.value });
    totpEnabled.value = true;
    totpURI.value = '';
    totpCode.value = '';
    totpMsg.value = 'TOTP enabled';
  } catch (e: any) { totpMsg.value = 'Error: invalid code'; }
}

async function disableTOTP() {
  try {
    await api.delete('/api/auth/totp/setup');
    totpEnabled.value = false;
    totpMsg.value = 'TOTP disabled';
  } catch (e: any) { totpMsg.value = 'Error: ' + (e?.message || 'failed'); }
}

async function revokeSession(id: string) {
  try {
    await api.delete('/api/auth/sessions', { data: { session_id: id } });
    sessions.value = sessions.value.filter(s => s.id !== id);
  } catch {}
}

async function logout() {
  try { await api.post('/api/auth/logout'); } catch {}
  window.location.href = '/login';
}
</script>

<style lang="scss" scoped>
.profile-card {
  margin-top: 4px;
}

.profile-header {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 20px;
}

.profile-avatar {
  width: 48px;
  height: 48px;
  border-radius: 14px;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.profile-info {
  flex: 1;
  min-width: 0;
}

.profile-name {
  font-size: 16px;
  font-weight: 600;
  color: var(--ink-1);
}

.profile-email {
  font-size: 12px;
  color: var(--ink-3);
  margin-top: 2px;
}

.role-badge {
  font-size: 11px;
  font-weight: 600;
  color: var(--accent);
  background: var(--accent-soft);
  padding: 4px 10px;
  border-radius: 6px;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.totp-setup {
  padding: 16px 20px;
}

.totp-instructions {
  font-size: 13px;
  color: var(--ink-2);
  margin-bottom: 16px;
  line-height: 1.5;
}

.totp-qr {
  display: flex;
  justify-content: center;
  margin-bottom: 16px;

  img {
    border-radius: 12px;
    background: white;
    padding: 8px;
  }
}

.totp-secret-display {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  margin-bottom: 16px;
}

.totp-secret-label {
  font-size: 11px;
  color: var(--ink-3);
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.totp-secret-code {
  font-size: 13px;
  color: var(--ink-1);
  background: var(--bg-3);
  padding: 4px 12px;
  border-radius: 6px;
  font-family: 'SF Mono', 'JetBrains Mono', monospace;
  letter-spacing: 1px;
}

.session-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 20px;
}

.session-info {
  display: flex;
  align-items: center;
  gap: 10px;
}

.session-icon-wrap {
  width: 32px;
  height: 32px;
  border-radius: 8px;
  background: var(--glass);
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--ink-3);
}

.session-id {
  font-size: 13px;
  font-weight: 500;
  color: var(--ink-1);
  font-family: 'SF Mono', 'JetBrains Mono', monospace;
}

.session-time {
  font-size: 11px;
  color: var(--ink-3);
  margin-top: 1px;
}
</style>
