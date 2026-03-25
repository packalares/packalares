<template>
  <div class="settings-page">
    <div class="page-title">Account</div>
    <div class="page-scroll">
      <!-- Profile -->
      <div class="settings-card">
        <div class="profile-header">
          <q-avatar size="72px" color="grey-8" text-color="white" icon="sym_r_person" />
          <div class="profile-info">
            <div class="profile-name">{{ userInfo.name || 'admin' }}</div>
            <div class="profile-email">{{ userInfo.email || '--' }}</div>
            <q-badge color="blue-8" :label="userInfo.role || 'admin'" />
          </div>
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
          <q-btn flat dense label="Change Password" color="primary" :loading="changingPw" @click="changePassword" />
          <span v-if="pwMsg" class="save-msg" :class="pwMsg.startsWith('Error') ? 'text-red-5' : 'text-green-5'">{{ pwMsg }}</span>
        </div>
      </div>

      <!-- TOTP 2FA -->
      <div class="section-title">Two-Factor Authentication</div>
      <div class="settings-card">
        <div class="info-row">
          <span class="info-label">TOTP Authenticator</span>
          <q-badge :color="totpEnabled ? 'green-8' : 'grey-7'" :label="totpEnabled ? 'Enabled' : 'Disabled'" />
        </div>
        <template v-if="!totpEnabled">
          <q-separator class="card-separator" />
          <div class="totp-setup" v-if="totpURI">
            <div class="totp-instructions">Scan this QR code with your authenticator app, then enter the code below:</div>
            <div class="totp-qr">
              <img :src="'https://api.qrserver.com/v1/create-qr-code/?size=200x200&data=' + encodeURIComponent(totpURI)" alt="TOTP QR" />
            </div>
            <div class="totp-secret-display">Secret: <code>{{ totpSecret }}</code></div>
            <div class="input-row">
              <span class="input-label">Code</span>
              <q-input v-model="totpCode" dense dark outlined placeholder="000000" maxlength="6" class="setting-input" @keyup.enter="verifyTOTP" />
            </div>
            <div class="action-row">
              <q-btn flat dense label="Verify & Enable" color="positive" @click="verifyTOTP" />
              <q-btn flat dense label="Cancel" color="grey" @click="totpURI = ''" />
            </div>
          </div>
          <div v-else class="action-row">
            <q-btn flat dense label="Setup TOTP" color="primary" @click="setupTOTP" />
          </div>
        </template>
        <template v-else>
          <div class="action-row">
            <q-btn flat dense label="Disable TOTP" color="negative" @click="disableTOTP" />
          </div>
        </template>
        <span v-if="totpMsg" class="save-msg q-ml-md" :class="totpMsg.startsWith('Error') ? 'text-red-5' : 'text-green-5'">{{ totpMsg }}</span>
      </div>

      <!-- Logout -->
      <div class="section-title">Session</div>
      <div class="settings-card">
        <div class="action-row">
          <q-btn flat dense label="Logout" color="negative" icon="sym_r_logout" @click="logout" />
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

  // Check TOTP status
  try {
    const s: any = await api.get('/api/auth/state');
    totpEnabled.value = s?.totp_enabled || false;
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
    const r: any = await api.get('/api/auth/totp/setup');
    totpURI.value = r?.uri || r?.data?.uri || '';
    totpSecret.value = r?.secret || r?.data?.secret || '';
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

async function logout() {
  try { await api.post('/api/auth/logout'); } catch {}
  window.location.href = '/login';
}
</script>

<style lang="scss" scoped>
.settings-page { height: 100%; display: flex; flex-direction: column; }
.page-title { font-size: 18px; font-weight: 600; color: var(--ink-1); padding: 16px 24px; height: 56px; display: flex; align-items: center; flex-shrink: 0; }
.page-scroll { flex: 1; overflow-y: auto; padding: 0 24px 24px; }
.section-title { font-size: 13px; font-weight: 500; color: var(--ink-2); margin-top: 20px; margin-bottom: 8px; text-transform: uppercase; letter-spacing: 0.5px; }
.settings-card { background: var(--bg-2); border-radius: 12px; border: 1px solid var(--separator); overflow: hidden; }
.profile-header { display: flex; align-items: center; gap: 16px; padding: 24px 20px; }
.profile-info { display: flex; flex-direction: column; gap: 4px; }
.profile-name { font-size: 18px; font-weight: 600; color: var(--ink-1); }
.profile-email { font-size: 13px; color: var(--ink-3); }
.info-row { display: flex; justify-content: space-between; align-items: center; padding: 14px 20px; }
.info-label { font-size: 14px; color: var(--ink-1); font-weight: 500; }
.card-separator { background: var(--separator); margin: 0 20px; }
.input-row { display: flex; align-items: center; padding: 8px 20px; gap: 12px; }
.input-label { font-size: 13px; color: var(--ink-1); font-weight: 500; min-width: 80px; }
.setting-input { flex: 1; }
.action-row { display: flex; align-items: center; gap: 12px; padding: 12px 20px; }
.save-msg { font-size: 12px; }
.totp-setup { padding: 0 20px 12px; }
.totp-instructions { font-size: 13px; color: var(--ink-2); margin-bottom: 12px; }
.totp-qr { display: flex; justify-content: center; margin-bottom: 12px; }
.totp-qr img { border-radius: 8px; }
.totp-secret-display { font-size: 12px; color: var(--ink-3); text-align: center; margin-bottom: 12px; }
.totp-secret-display code { color: var(--ink-1); background: var(--bg-3); padding: 2px 8px; border-radius: 4px; font-family: monospace; }
</style>
