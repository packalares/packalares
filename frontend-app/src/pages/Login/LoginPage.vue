<template>
  <div class="login-root">
    <!-- Full-screen background -->
    <div class="bg-container">
      <img
        v-if="wallpaperUrl"
        class="desktop-bg"
        :src="wallpaperUrl"
        alt=""
      />
      <div v-else class="gradient-bg" />
    </div>

    <!-- First Factor: Password -->
    <transition
      enter-active-class="animated fadeIn"
      leave-active-class="animated fadeOut"
    >
      <div v-if="currentView === 'password'" class="login-box" key="password">
        <div class="login-card">
          <!-- Avatar -->
          <div class="avatar-ring">
            <div class="avatar-inner">
              <img
                :src="userAvatar || '/avatar-default.png'"
                class="avatar-img"
                alt=""
              />
            </div>
          </div>

          <!-- Username -->
          <p class="login-name">{{ userName }}</p>
          <p class="login-hint">Enter your password to log in</p>

          <!-- Password input -->
          <div class="password-row" :class="{ shake: passwordErr }">
            <div class="input-wrap">
              <input
                ref="passwordRef"
                v-model="password"
                type="password"
                class="password-input"
                :class="{ disable: loading }"
                :disabled="loading"
                @keydown.enter="onLogin"
              />
            </div>
            <label v-if="!password" class="placeholder-label">Password</label>
            <transition enter-active-class="animated fadeIn">
              <button
                v-if="password && !loading"
                class="submit-arrow"
                @click="onLogin"
                aria-label="Submit"
              >
                <svg
                  width="20"
                  height="20"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2"
                  stroke-linecap="round"
                  stroke-linejoin="round"
                >
                  <circle cx="12" cy="12" r="10" />
                  <polyline points="12 16 16 12 12 8" />
                  <line x1="8" y1="12" x2="16" y2="12" />
                </svg>
              </button>
            </transition>
          </div>

          <!-- Loading spinner -->
          <div v-if="loading" class="loading-spinner">
            <svg
              class="spinner-svg"
              width="24"
              height="24"
              viewBox="0 0 24 24"
            >
              <circle
                cx="12"
                cy="12"
                r="10"
                fill="none"
                stroke="rgba(255,255,255,0.3)"
                stroke-width="2"
              />
              <path
                d="M12 2 a10 10 0 0 1 10 10"
                fill="none"
                stroke="#fff"
                stroke-width="2"
                stroke-linecap="round"
              />
            </svg>
          </div>
        </div>
      </div>
    </transition>

    <!-- Second Factor: TOTP -->
    <transition
      enter-active-class="animated fadeIn"
      leave-active-class="animated fadeOut"
    >
      <div v-if="currentView === 'totp'" class="login-box" key="totp">
        <div class="login-card">
          <!-- Circular progress timer with lock icon -->
          <div class="totp-timer-ring">
            <svg
              :width="timerSize"
              :height="timerSize"
              :viewBox="`0 0 ${timerSize} ${timerSize}`"
            >
              <circle
                :cx="timerSize / 2"
                :cy="timerSize / 2"
                :r="timerRadius"
                fill="none"
                stroke="rgba(255,255,255,0.2)"
                stroke-width="3"
              />
              <circle
                :cx="timerSize / 2"
                :cy="timerSize / 2"
                :r="timerRadius"
                fill="none"
                stroke="#ffffff"
                stroke-width="3"
                stroke-linecap="round"
                :stroke-dasharray="timerCircumference"
                :stroke-dashoffset="timerOffset"
                class="progress-arc"
              />
            </svg>
            <!-- Lock icon in center -->
            <div class="lock-icon">
              <svg
                width="24"
                height="24"
                viewBox="0 0 24 24"
                fill="none"
                stroke="#ffffff"
                stroke-width="1.5"
                stroke-linecap="round"
                stroke-linejoin="round"
              >
                <rect x="3" y="11" width="18" height="11" rx="2" ry="2" />
                <path d="M7 11V7a5 5 0 0 1 10 0v4" />
              </svg>
            </div>
          </div>

          <p class="login-name">Two-Factor Authentication</p>
          <p class="login-hint">Enter the code from your authenticator app</p>

          <!-- 6 OTP digit inputs -->
          <div class="otp-row" :class="{ shake: totpErr }">
            <input
              v-for="(_, idx) in 6"
              :key="idx"
              ref="otpRefs"
              type="text"
              inputmode="numeric"
              maxlength="1"
              class="otp-input"
              :class="{ filled: otpDigits[idx] }"
              :value="otpDigits[idx]"
              :disabled="totpLoading"
              @input="onOtpInput($event, idx)"
              @keydown="onOtpKeydown($event, idx)"
              @paste="onOtpPaste($event, idx)"
              @focus="onOtpFocus($event)"
            />
          </div>

          <!-- TOTP loading spinner -->
          <div v-if="totpLoading" class="loading-spinner">
            <svg
              class="spinner-svg"
              width="24"
              height="24"
              viewBox="0 0 24 24"
            >
              <circle
                cx="12"
                cy="12"
                r="10"
                fill="none"
                stroke="rgba(255,255,255,0.3)"
                stroke-width="2"
              />
              <path
                d="M12 2 a10 10 0 0 1 10 10"
                fill="none"
                stroke="#fff"
                stroke-width="2"
                stroke-linecap="round"
              />
            </svg>
          </div>
        </div>
      </div>
    </transition>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onUnmounted, nextTick } from 'vue';
import { useQuasar } from 'quasar';
import { api } from 'boot/axios';

// ---------- Quasar ----------
const $q = useQuasar();

// ---------- State: shared ----------
type ViewType = 'password' | 'totp';
const currentView = ref<ViewType>('password');

// ---------- State: user info ----------
const userName = ref('');
const userAvatar = ref('');
const wallpaperUrl = ref('/bg/macos1.jpg');

const userInitial = computed(() => {
  const name = userName.value || '';
  return name.charAt(0).toUpperCase() || 'U';
});

// ---------- State: first factor ----------
const password = ref('');
const passwordRef = ref<HTMLInputElement | null>(null);
const loading = ref(false);
const passwordErr = ref(false);

// Auth token received from first factor (used if 2FA required)
let firstFactorToken = '';

// ---------- State: second factor ----------
const otpDigits = reactive<string[]>(['', '', '', '', '', '']);
const otpRefs = ref<HTMLInputElement[]>([]);
const totpLoading = ref(false);
const totpErr = ref(false);

// Timer
const timerSize = 100;
const timerRadius = (timerSize - 6) / 2; // account for stroke-width
const timerCircumference = 2 * Math.PI * timerRadius;
const totpPeriod = 30;
const timerProgress = ref(0);
let timerHandle: ReturnType<typeof setTimeout> | null = null;

const timerOffset = computed(() => {
  return timerCircumference * (1 - timerProgress.value / 100);
});

function updateTimerProgress() {
  timerProgress.value =
    (((Date.now() / 1000) % totpPeriod) / totpPeriod) * 100;
  timerHandle = setTimeout(updateTimerProgress, 1000);
}

// ---------- Redirect helpers ----------
function getRedirectUrl(responseRedirect?: string): string {
  const params = new URLSearchParams(window.location.search);
  const rd = params.get('rd');
  // ?rd= parameter takes priority (set by auth redirect from protected page)
  if (rd) return decodeURIComponent(rd);
  if (responseRedirect) return responseRedirect;
  return '/desktop/';
}

// ---------- Fetch user info ----------
async function fetchUserInfo() {
  try {
    // Check wizard status — redirect if not completed
    const infoRes: any = await api.get('/bfl/info/v1/olares-info');
    const infoData = infoRes?.data ?? infoRes;
    if (infoData?.wizardStatus && infoData.wizardStatus !== 'completed') {
      window.location.href = '/wizard';
      return;
    }
  } catch {
    // Fallback: check via user-info
    try {
      const r: any = await api.get('/api/user/info');
      const d = r?.data ?? r;
      if (d && d.wizard_complete === false) {
        window.location.href = '/wizard';
        return;
      }
    } catch {}
  }

  try {
    const res: any = await api.get('/api/user/info');
    const data = res?.data ?? res;
    if (data?.name) userName.value = data.name;
    if (data?.avatar) userAvatar.value = data.avatar;
    if (data?.loginBackground) wallpaperUrl.value = data.loginBackground;
  } catch {
    // Silently fall back to defaults
  }
}

// ---------- First factor ----------
function shakePassword() {
  passwordErr.value = true;
  setTimeout(() => {
    passwordErr.value = false;
  }, 800);
}

function clearPasswordAndFocus() {
  loading.value = false;
  setTimeout(() => {
    if (passwordRef.value) {
      passwordRef.value.focus();
      passwordRef.value.select();
    }
  }, 1200);
}

async function onLogin() {
  if (!password.value) {
    shakePassword();
    return;
  }

  loading.value = true;
  try {
    const res: any = await api.post('/api/auth/login', {
      username: userName.value,
      password: password.value,
      keep_me_logged_in: true,
    });

    const body = res?.data ?? res;
    const raw = res; // full response including requires_totp at root

    if (body?.status === 'OK' || body?.token || body?.redirect || raw?.status === 'OK') {
      // Check if 2FA is required
      if (body?.fa2 || body?.second_factor || raw?.requires_totp || body?.requires_totp) {
        firstFactorToken = body?.token || raw?.data?.token || '';
        currentView.value = 'totp';
        updateTimerProgress();
        await nextTick();
        if (otpRefs.value?.[0]) {
          otpRefs.value[0].focus();
        }
      } else {
        // Direct redirect
        const url = getRedirectUrl(body?.redirect);
        window.location.replace(url);
      }
    } else {
      // Error from server
      const msg = body?.message || 'Invalid credentials';
      $q.notify({
        type: 'negative',
        message: msg,
        position: 'top',
        timeout: 3000,
      });
      shakePassword();
      clearPasswordAndFocus();
    }
  } catch (err: any) {
    const msg =
      err?.response?.data?.message || err?.message || 'Authentication failed';
    $q.notify({
      type: 'negative',
      message: msg,
      position: 'top',
      timeout: 3000,
    });
    shakePassword();
    clearPasswordAndFocus();
  } finally {
    loading.value = false;
  }
}

// ---------- Second factor ----------
function onOtpInput(event: Event, idx: number) {
  const target = event.target as HTMLInputElement;
  const value = target.value.replace(/\D/g, '');
  otpDigits[idx] = value.charAt(0) || '';
  target.value = otpDigits[idx];

  if (otpDigits[idx] && idx < 5) {
    otpRefs.value[idx + 1]?.focus();
  }

  // Auto-submit when all 6 digits entered
  if (otpDigits.every((d) => d !== '')) {
    submitTotp();
  }
}

function onOtpKeydown(event: KeyboardEvent, idx: number) {
  if (event.key === 'Backspace') {
    if (!otpDigits[idx] && idx > 0) {
      event.preventDefault();
      otpDigits[idx - 1] = '';
      otpRefs.value[idx - 1]?.focus();
    } else {
      otpDigits[idx] = '';
    }
  } else if (event.key === 'ArrowLeft' && idx > 0) {
    otpRefs.value[idx - 1]?.focus();
  } else if (event.key === 'ArrowRight' && idx < 5) {
    otpRefs.value[idx + 1]?.focus();
  }
}

function onOtpPaste(event: ClipboardEvent, _idx: number) {
  event.preventDefault();
  const pasted = (event.clipboardData?.getData('text') || '').replace(
    /\D/g,
    ''
  );
  if (!pasted) return;

  for (let i = 0; i < 6; i++) {
    otpDigits[i] = pasted.charAt(i) || '';
  }

  // Focus last filled input or first empty
  const lastFilled = Math.min(pasted.length, 6) - 1;
  if (lastFilled >= 0 && lastFilled < 6) {
    otpRefs.value[lastFilled]?.focus();
  }

  // Auto-submit if we got all 6
  if (otpDigits.every((d) => d !== '')) {
    submitTotp();
  }
}

function onOtpFocus(event: Event) {
  (event.target as HTMLInputElement).select();
}

function shakeTotpAndClear() {
  totpErr.value = true;
  setTimeout(() => {
    totpErr.value = false;
  }, 800);
  nextTick(() => {
    for (let i = 0; i < 6; i++) {
      otpDigits[i] = '';
    }
    otpRefs.value[0]?.focus();
  });
}

async function submitTotp() {
  const token = otpDigits.join('');
  if (token.length < 6) return;

  totpLoading.value = true;
  try {
    const res: any = await api.post('/api/auth/totp', {
      token,
    });

    const body = res?.data ?? res;

    if (body?.status === 'OK' || body?.redirect) {
      const url = getRedirectUrl(body?.redirect);
      window.location.replace(url);
    } else {
      const msg = body?.message || 'Invalid verification code';
      $q.notify({
        type: 'negative',
        message: msg,
        position: 'top',
        timeout: 3000,
      });
      shakeTotpAndClear();
    }
  } catch (err: any) {
    const msg =
      err?.response?.data?.message || err?.message || 'Verification failed';
    $q.notify({
      type: 'negative',
      message: msg,
      position: 'top',
      timeout: 3000,
    });
    shakeTotpAndClear();
  } finally {
    totpLoading.value = false;
  }
}

// ---------- Lifecycle ----------
onMounted(async () => {
  await fetchUserInfo();
  await nextTick();
  passwordRef.value?.focus();
});

onUnmounted(() => {
  if (timerHandle) {
    clearTimeout(timerHandle);
    timerHandle = null;
  }
});
</script>

<style scoped>
/* ===== Full-screen background ===== */
.login-root {
  width: 100vw;
  height: 100vh;
  overflow: hidden;
  position: relative;
  font-family: 'Inter', -apple-system, BlinkMacSystemFont, sans-serif;
}

.bg-container {
  position: fixed;
  inset: 0;
  z-index: 0;
  display: flex;
  justify-content: center;
  align-items: center;
  overflow: hidden;
}

.desktop-bg {
  width: auto;
  min-width: 100%;
  height: 100%;
  object-fit: cover;
}

.gradient-bg {
  width: 100%;
  height: 100%;
  background: linear-gradient(135deg, #0f0c29 0%, #302b63 50%, #24243e 100%);
}

/* ===== Centered card layout ===== */
.login-box {
  position: fixed;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1;
}

.login-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  margin-bottom: 10vh;
}

/* ===== Avatar ===== */
.avatar-ring {
  width: 124px;
  height: 124px;
  border-radius: 62px;
  padding: 3px;
  background: linear-gradient(
    135deg,
    rgba(255, 255, 255, 0.6),
    rgba(255, 255, 255, 0.15)
  );
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
  box-shadow: 0 4px 30px rgba(0, 0, 0, 0.15);
}

.avatar-inner {
  width: 100%;
  height: 100%;
  border-radius: 50%;
  overflow: hidden;
  background: rgba(255, 255, 255, 0.15);
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  display: flex;
  align-items: center;
  justify-content: center;
}

.avatar-img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.avatar-initial {
  font-size: 48px;
  font-weight: 700;
  color: #ffffff;
  text-shadow: 0 2px 8px rgba(0, 0, 0, 0.2);
  user-select: none;
}

/* ===== Text ===== */
.login-name {
  font-size: 24px;
  font-weight: 700;
  color: #ffffff;
  margin: 16px 0 4px;
  text-align: center;
}

.login-hint {
  font-size: 14px;
  font-weight: 400;
  color: #ffffff;
  margin: 0 0 16px;
  text-align: center;
  opacity: 0.85;
}

/* ===== Password input row ===== */
.password-row {
  width: 220px;
  height: 32px;
  background: rgba(255, 255, 255, 0.4);
  border-radius: 8px;
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  padding-left: 12px;
  padding-right: 10px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  box-sizing: border-box;
  position: relative;
}

.input-wrap {
  width: 100%;
  height: 18px;
  overflow: hidden;
  display: flex;
  align-items: center;
  justify-content: flex-start;
}

.password-input {
  width: 180px;
  background-color: transparent;
  border: none;
  padding: 0;
  margin: 0;
  caret-color: #ffffff;
  letter-spacing: 1px;
  height: 28px;
  line-height: 28px;
  font-weight: 900;
  font-size: 22px;
  color: #ffffff;
}

.password-input.disable {
  pointer-events: none;
}

.password-input:focus {
  outline: none;
  box-shadow: none;
}

input[type='password']::-ms-reveal {
  display: none;
}

.placeholder-label {
  position: absolute;
  left: 14px;
  top: 50%;
  transform: translateY(-50%);
  color: rgba(255, 255, 255, 0.5);
  pointer-events: none;
  font-size: 13px;
}

/* ===== Arrow submit button ===== */
.submit-arrow {
  display: flex;
  align-items: center;
  justify-content: center;
  background: none;
  border: none;
  color: #ffffff;
  cursor: pointer;
  padding: 0;
  flex-shrink: 0;
  transition: opacity 0.2s ease;
}

.submit-arrow:hover {
  opacity: 0.8;
}

/* ===== Loading spinner ===== */
.loading-spinner {
  width: 32px;
  height: 32px;
  border-radius: 16px;
  background: rgba(31, 24, 20, 0.3);
  display: flex;
  align-items: center;
  justify-content: center;
  margin-top: 16px;
}

.spinner-svg {
  animation: spin 1s linear infinite;
}

@keyframes spin {
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
}

/* ===== Shake animation ===== */
.shake {
  animation: shake 800ms ease-in-out;
}

@keyframes shake {
  10%,
  90% {
    transform: translate3d(-1px, 0, 0);
  }
  20%,
  80% {
    transform: translate3d(2px, 0, 0);
  }
  30%,
  70% {
    transform: translate3d(-4px, 0, 0);
  }
  40%,
  60% {
    transform: translate3d(4px, 0, 0);
  }
  50% {
    transform: translate3d(-4px, 0, 0);
  }
}

/* ===== TOTP Timer Ring ===== */
.totp-timer-ring {
  position: relative;
  width: 100px;
  height: 100px;
  display: flex;
  align-items: center;
  justify-content: center;
  margin-bottom: 24px;
  border: 4px solid rgba(255, 255, 255, 0.3);
  border-radius: 50%;
  box-sizing: content-box;
}

.totp-timer-ring svg {
  position: absolute;
}

.progress-arc {
  transform: rotate(-90deg);
  transform-origin: center;
  transition: stroke-dashoffset 1s linear;
}

.lock-icon {
  position: relative;
  z-index: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  opacity: 0.7;
}

/* ===== OTP Inputs ===== */
.otp-row {
  display: flex;
  gap: 8px;
  justify-content: center;
}

.otp-input {
  width: 32px;
  height: 40px;
  text-align: center;
  font-size: 20px;
  font-weight: 600;
  line-height: 40px;
  border: none;
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.4);
  color: #ffffff;
  caret-color: #ffffff;
  backdrop-filter: blur(10px);
  -webkit-backdrop-filter: blur(10px);
}

.otp-input.filled {
  background: rgba(255, 255, 255, 0.3);
}

.otp-input:focus {
  outline: none;
  outline-color: rgba(255, 255, 255, 0.4);
}

.otp-input::-webkit-inner-spin-button,
.otp-input::-webkit-outer-spin-button {
  -webkit-appearance: none;
  margin: 0;
}

/* ===== Quasar animated classes (used by transitions) ===== */
</style>
