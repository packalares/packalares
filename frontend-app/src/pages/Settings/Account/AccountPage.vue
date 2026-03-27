<template>
  <div class="settings-page">
    <div class="page-header">
      <div class="page-title">Account</div>
      <div class="page-description">Manage your profile, password, two-factor authentication, and sessions.</div>
    </div>
    <div class="page-scroll">

      <!-- Profile Card -->
      <div class="settings-card">
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

      <!-- Change Password -->
      <div class="settings-card q-mt-lg">
        <div class="card-header">
          <div class="card-header-icon card-header-icon--security">
            <q-icon name="sym_r_lock" size="18px" />
          </div>
          <div class="card-header-text">
            <div class="card-header-title">Change Password</div>
            <div class="card-header-subtitle">Update your account password</div>
          </div>
        </div>
        <div class="form-grid cols-1">
          <div class="form-group">
            <label class="form-label">Current Password</label>
            <q-input v-model="currentPassword" dense dark outlined type="password" />
          </div>
        </div>
        <div class="form-grid cols-2">
          <div class="form-group">
            <label class="form-label">New Password</label>
            <q-input v-model="newPassword" dense dark outlined type="password" />
          </div>
          <div class="form-group">
            <label class="form-label">Confirm</label>
            <q-input v-model="confirmPassword" dense dark outlined type="password" />
          </div>
        </div>
        <div class="card-footer">
          <span v-if="pwMsg" class="footer-msg" :class="pwMsg.startsWith('Error') ? 'text-red-5' : 'text-green-5'">{{ pwMsg }}</span>
          <q-btn unelevated dense label="Change Password" class="btn-primary" :loading="changingPw" @click="changePassword" />
        </div>
      </div>

      <!-- TOTP 2FA -->
      <div class="settings-card q-mt-lg">
        <div class="card-header">
          <div class="card-header-icon card-header-icon--account">
            <q-icon name="sym_r_security" size="18px" />
          </div>
          <div class="card-header-text">
            <div class="card-header-title">Two-Factor Authentication</div>
            <div class="card-header-subtitle">Add an extra layer of security with a TOTP authenticator</div>
          </div>
          <div class="card-header-actions">
            <span
              class="status-badge"
              :class="totpEnabled ? 'status-connected' : 'status-disconnected'"
            >{{ totpEnabled ? 'Enabled' : 'Disabled' }}</span>
          </div>
        </div>
        <template v-if="!totpEnabled">
          <div class="totp-setup" v-if="totpURI">
            <div class="totp-layout">
              <div class="totp-qr">
                <img :src="'https://api.qrserver.com/v1/create-qr-code/?size=160x160&data=' + encodeURIComponent(totpURI)" alt="TOTP QR" />
              </div>
              <div class="totp-right">
                <div class="totp-instructions">Scan this QR code with your authenticator app, then enter the 6-digit code below.</div>
                <div class="totp-secret-display">
                  <span class="totp-secret-label">Secret</span>
                  <code class="totp-secret-code">{{ totpSecret }}</code>
                </div>
                <div class="form-group">
                  <label class="form-label">Verification Code</label>
                  <q-input v-model="totpCode" dense dark outlined placeholder="000000" maxlength="6" style="max-width: 160px" @keyup.enter="verifyTOTP" />
                </div>
              </div>
            </div>
            <div class="card-footer">
              <q-btn flat dense label="Cancel" class="btn-ghost" @click="totpURI = ''" />
              <q-btn unelevated dense label="Verify & Enable" class="btn-primary" @click="verifyTOTP" />
            </div>
          </div>
          <div v-else class="card-footer">
            <span v-if="totpMsg" class="footer-msg" :class="totpMsg.startsWith('Error') ? 'text-red-5' : 'text-green-5'">{{ totpMsg }}</span>
            <q-btn unelevated dense label="Setup TOTP" class="btn-primary" @click="setupTOTP" />
          </div>
        </template>
        <template v-else>
          <div class="card-footer">
            <span v-if="totpMsg" class="footer-msg" :class="totpMsg.startsWith('Error') ? 'text-red-5' : 'text-green-5'">{{ totpMsg }}</span>
            <q-btn flat dense label="Disable TOTP" class="btn-danger" @click="disableTOTP" />
          </div>
        </template>
      </div>

      <!-- Active Sessions -->
      <div class="settings-card q-mt-lg">
        <div class="card-header">
          <div class="card-header-icon card-header-icon--system">
            <q-icon name="sym_r_devices" size="18px" />
          </div>
          <div class="card-header-text">
            <div class="card-header-title">Active Sessions</div>
            <div class="card-header-subtitle">Manage devices logged into your account</div>
          </div>
        </div>
        <div v-if="sessions.length === 0" class="empty-state">
          <div class="empty-state-icon">
            <q-icon name="sym_r_devices" size="24px" color="grey-6" />
          </div>
          No active sessions
        </div>
        <div v-else class="sessions-grid">
          <div v-for="s in sessions" :key="s.id" class="session-card">
            <div class="session-info">
              <div class="session-icon-wrap">
                <q-icon name="sym_r_devices" size="16px" />
              </div>
              <div>
                <div class="session-id">{{ s.id }}</div>
                <div class="session-time">{{ new Date(s.last_activity).toLocaleString() }}</div>
              </div>
            </div>
            <q-btn flat dense round icon="sym_r_close" size="xs" color="negative" @click="revokeSession(s.id)" />
          </div>
        </div>
        <div v-if="sessions.length > 1" class="card-footer">
          <q-btn flat dense label="Logout All Other Devices" class="btn-danger" @click="confirmLogoutAll" />
        </div>
      </div>

      <!-- Logout -->
      <div class="settings-card q-mt-lg">
        <div class="card-header">
          <div class="card-header-icon card-header-icon--danger">
            <q-icon name="sym_r_logout" size="18px" />
          </div>
          <div class="card-header-text">
            <div class="card-header-title">Session</div>
            <div class="card-header-subtitle">End your current session</div>
          </div>
        </div>
        <div class="card-footer">
          <q-btn flat dense label="Logout" class="btn-danger" icon="sym_r_logout" @click="confirmLogout" />
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
    await api.put('/api/auth/totp/setup', { token: totpCode.value });
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

function confirmLogout() {
  $q.dialog({
    title: 'Logout',
    message: 'Are you sure you want to logout?',
    cancel: true,
    persistent: true,
  }).onOk(async () => {
    try { await api.post('/api/auth/logout'); } catch {}
    window.location.href = '/login';
  });
}

function confirmLogoutAll() {
  $q.dialog({
    title: 'Logout All Devices',
    message: 'This will revoke all sessions except the current one. Continue?',
    cancel: true,
    persistent: true,
  }).onOk(async () => {
    for (const s of sessions.value) {
      try { await api.delete('/api/auth/sessions', { data: { session_id: s.id } }); } catch {}
    }
    try {
      const r: any = await api.get('/api/auth/sessions');
      sessions.value = r?.sessions || [];
    } catch {}
  });
}
</script>

<style lang="scss" scoped>
.profile-header {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 18px 20px;
}

.profile-avatar {
  width: 44px;
  height: 44px;
  border-radius: 12px;
  background: linear-gradient(135deg, var(--accent-bold) 0%, #a78bfa 100%);
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  box-shadow: 0 2px 8px rgba(99,102,241,0.3);
}

.profile-info { flex: 1; min-width: 0; }
.profile-name { font-size: 15px; font-weight: 600; color: var(--ink-1); }
.profile-email { font-size: 12px; color: var(--ink-3); margin-top: 1px; }

.role-badge {
  font-size: 11px;
  font-weight: 600;
  color: var(--accent);
  background: var(--accent-soft);
  padding: 4px 10px;
  border-radius: var(--radius-xs);
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.totp-setup { padding: 0; }
.totp-layout {
  display: flex;
  gap: 20px;
  padding: 16px;
  align-items: flex-start;
}
.totp-qr {
  flex-shrink: 0;
  img { border-radius: var(--radius); background: #fff; padding: 6px; width: 140px; height: 140px; }
}
.totp-right {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.totp-instructions { font-size: 12px; color: var(--ink-2); line-height: 1.5; }
.totp-secret-display { display: flex; align-items: center; gap: 8px; }
.totp-secret-label { font-size: 11px; color: var(--ink-3); text-transform: uppercase; letter-spacing: 0.05em; }
.totp-secret-code {
  font-size: 12px;
  color: var(--ink-1);
  background: var(--bg-3);
  padding: 4px 10px;
  border-radius: var(--radius-xs);
  font-family: 'JetBrains Mono', monospace;
  letter-spacing: 0.06em;
}

.sessions-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 1px;
  padding: 8px 16px;
}
.session-card {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 8px 12px;
  border-radius: 6px;
  &:hover { background: rgba(255,255,255,0.02); }
}
.session-info { display: flex; align-items: center; gap: 8px; }
.session-icon-wrap {
  width: 28px; height: 28px; border-radius: 6px;
  background: var(--glass); display: flex; align-items: center; justify-content: center; color: var(--ink-3);
}
.session-id { font-size: 11px; font-weight: 500; color: var(--ink-1); font-family: 'JetBrains Mono', monospace; }
.session-time { font-size: 10px; color: var(--ink-3); margin-top: 1px; }
</style>
