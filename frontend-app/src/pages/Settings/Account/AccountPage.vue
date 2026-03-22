<template>
  <div class="settings-page">
    <div class="page-title">Account</div>
    <div class="page-scroll">
      <!-- Profile Card -->
      <div class="section-title">Profile</div>
      <div class="settings-card">
        <div class="profile-header">
          <q-avatar size="72px" color="grey-8" text-color="white" icon="person" />
          <div class="profile-info">
            <div class="profile-name">{{ userInfo.name || 'admin' }}</div>
            <div class="profile-email">{{ userInfo.email || 'admin@packalares.local' }}</div>
            <q-badge
              :color="userInfo.role === 'admin' ? 'blue-8' : 'grey-7'"
              :label="userInfo.role || 'admin'"
              class="q-mt-xs"
            />
          </div>
        </div>
      </div>

      <!-- Password Change -->
      <div class="section-title">Security</div>
      <div class="settings-card">
        <div class="form-row">
          <label class="form-label">Current Password</label>
          <q-input
            v-model="currentPassword"
            type="password"
            dense
            outlined
            dark
            class="form-input"
            placeholder="Enter current password"
          />
        </div>
        <q-separator class="card-separator" />
        <div class="form-row">
          <label class="form-label">New Password</label>
          <q-input
            v-model="newPassword"
            type="password"
            dense
            outlined
            dark
            class="form-input"
            placeholder="Enter new password"
          />
        </div>
        <q-separator class="card-separator" />
        <div class="form-row">
          <label class="form-label">Confirm Password</label>
          <q-input
            v-model="confirmPassword"
            type="password"
            dense
            outlined
            dark
            class="form-input"
            placeholder="Confirm new password"
          />
        </div>
        <div class="form-actions">
          <q-btn
            label="Change Password"
            no-caps
            class="action-btn"
            :loading="changingPassword"
            @click="changePassword"
          />
        </div>
      </div>

      <!-- Two-Factor Authentication -->
      <div class="section-title">Two-Factor Authentication</div>
      <div class="settings-card">
        <div class="form-row row items-center justify-between">
          <div>
            <div class="form-label" style="margin-bottom: 0">TOTP Authenticator</div>
            <div class="form-hint">Use an authenticator app for two-factor verification</div>
          </div>
          <q-toggle
            v-model="totpEnabled"
            color="blue-8"
            @update:model-value="toggleTotp"
          />
        </div>
      </div>

      <!-- Logout -->
      <div class="settings-card q-mt-lg">
        <div class="form-row row items-center justify-between">
          <div>
            <div class="form-label" style="margin-bottom: 0; color: var(--negative)">
              Sign Out
            </div>
            <div class="form-hint">End your current session</div>
          </div>
          <q-btn
            flat
            no-caps
            label="Logout"
            class="logout-btn"
            @click="confirmLogout"
          />
        </div>
      </div>
    </div>

    <!-- Logout Confirm Dialog -->
    <q-dialog v-model="showLogoutDialog">
      <q-card class="dialog-card">
        <q-card-section>
          <div class="dialog-title">Sign Out</div>
          <div class="dialog-message">Are you sure you want to sign out?</div>
        </q-card-section>
        <q-card-actions align="right" class="dialog-actions">
          <q-btn flat no-caps label="Cancel" class="dialog-cancel" v-close-popup />
          <q-btn
            flat
            no-caps
            label="Sign Out"
            class="dialog-confirm-danger"
            :loading="loggingOut"
            @click="logout"
          />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </div>
</template>

<script lang="ts" setup>
import { ref, onMounted, reactive } from 'vue';
import { useQuasar } from 'quasar';
import { api } from 'boot/axios';

const $q = useQuasar();

const userInfo = reactive({
  name: '',
  email: '',
  role: 'admin',
});

const currentPassword = ref('');
const newPassword = ref('');
const confirmPassword = ref('');
const changingPassword = ref(false);
const totpEnabled = ref(false);
const showLogoutDialog = ref(false);
const loggingOut = ref(false);

onMounted(async () => {
  try {
    const res: any = await api.get('/bfl/backend/v1/user-info');
    if (res) {
      userInfo.name = res.name || res.username || 'admin';
      userInfo.email = res.email || '';
      userInfo.role = res.role || 'admin';
    }
  } catch {
    userInfo.name = 'admin';
    userInfo.role = 'admin';
  }
});

const changePassword = async () => {
  if (!currentPassword.value || !newPassword.value) {
    $q.notify({ type: 'warning', message: 'Please fill in all password fields' });
    return;
  }
  if (newPassword.value !== confirmPassword.value) {
    $q.notify({ type: 'negative', message: 'New passwords do not match' });
    return;
  }
  if (newPassword.value.length < 6) {
    $q.notify({ type: 'warning', message: 'Password must be at least 6 characters' });
    return;
  }

  changingPassword.value = true;
  try {
    await api.post('/api/auth/password', {
      current_password: currentPassword.value,
      new_password: newPassword.value,
    });
    $q.notify({ type: 'positive', message: 'Password changed successfully' });
    currentPassword.value = '';
    newPassword.value = '';
    confirmPassword.value = '';
  } catch (err: any) {
    $q.notify({
      type: 'negative',
      message: err?.response?.data?.message || 'Failed to change password',
    });
  } finally {
    changingPassword.value = false;
  }
};

const toggleTotp = async (val: boolean) => {
  $q.notify({
    type: 'info',
    message: val ? 'TOTP setup is not yet implemented' : 'TOTP has been disabled',
  });
};

const confirmLogout = () => {
  showLogoutDialog.value = true;
};

const logout = async () => {
  loggingOut.value = true;
  try {
    await api.post('/api/logout');
  } catch {
    // proceed even on error
  }
  window.location.href = '/login';
};
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

.profile-header {
  display: flex;
  align-items: center;
  padding: 20px;
  gap: 16px;
}

.profile-info {
  flex: 1;
}

.profile-name {
  font-size: 18px;
  font-weight: 600;
  color: var(--ink-1);
}

.profile-email {
  font-size: 13px;
  color: var(--ink-2);
  margin-top: 2px;
}

.form-row {
  padding: 14px 20px;
}

.form-label {
  font-size: 13px;
  font-weight: 500;
  color: var(--ink-1);
  margin-bottom: 6px;
  display: block;
}

.form-hint {
  font-size: 12px;
  color: var(--ink-3);
  margin-top: 2px;
}

.form-input {
  :deep(.q-field__control) {
    background: var(--bg-1);
    border-color: var(--separator);
  }
  :deep(.q-field__native) {
    color: var(--ink-1);
  }
}

.card-separator {
  background: var(--separator);
  margin: 0 20px;
}

.form-actions {
  padding: 12px 20px 16px;
  display: flex;
  justify-content: flex-end;
}

.action-btn {
  background: var(--accent) !important;
  color: #fff !important;
  border-radius: 8px;
  font-size: 13px;
  padding: 6px 16px;
}

.logout-btn {
  color: var(--negative) !important;
  border: 1px solid var(--negative);
  border-radius: 8px;
  font-size: 13px;
}

.dialog-card {
  background: var(--bg-2);
  border-radius: 12px;
  min-width: 360px;
  border: 1px solid var(--separator);
}

.dialog-title {
  font-size: 16px;
  font-weight: 600;
  color: var(--ink-1);
}

.dialog-message {
  font-size: 13px;
  color: var(--ink-2);
  margin-top: 8px;
}

.dialog-actions {
  padding: 8px 16px 16px;
}

.dialog-cancel {
  color: var(--ink-2) !important;
  border: 1px solid var(--separator);
  border-radius: 8px;
  font-size: 13px;
}

.dialog-confirm-danger {
  color: #fff !important;
  background: var(--negative) !important;
  border-radius: 8px;
  font-size: 13px;
}
</style>
