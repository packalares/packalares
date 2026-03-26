<template>
  <div class="settings-page">
    <div class="page-title">
      Update
      <q-btn
        unelevated dense
        :label="checking ? 'Checking...' : 'Check for Updates'"
        class="btn-primary q-ml-md"
        :loading="checking"
        @click="checkUpdates"
      />
    </div>
    <div class="page-scroll">
      <div class="section-title">Container Images</div>

      <!-- Loading state -->
      <div v-if="loading && !images.length" class="settings-card">
        <div class="info-row">
          <span class="info-label">Loading deployment images...</span>
          <q-spinner size="18px" color="grey-5" />
        </div>
      </div>

      <!-- Empty state -->
      <div v-else-if="!images.length" class="settings-card">
        <div class="info-row">
          <span class="info-label">No Packalares images found in the framework namespace.</span>
        </div>
      </div>

      <!-- Image list -->
      <div v-else class="settings-card">
        <template v-for="(img, idx) in images" :key="img.name + img.currentImage">
          <q-separator v-if="idx > 0" class="card-separator" />
          <div class="update-row">
            <div class="update-info">
              <div class="update-name">{{ img.name }}</div>
              <div class="update-image-line">
                <span class="update-image-name">{{ shortImage(img.currentImage) }}</span>
                <span class="update-tag current-tag">:{{ img.currentTag }}</span>
                <span class="update-digest">{{ img.currentDigest || '' }}</span>
                <template v-if="img.updateAvailable">
                  <q-icon name="sym_r_arrow_forward" size="14px" class="update-arrow" />
                  <span class="update-digest latest-digest">{{ img.remoteDigest || '' }}</span>
                </template>
              </div>
            </div>
            <div class="update-actions">
              <span
                v-if="!img.updateAvailable && !restartingSet.has(img.name)"
                class="status-badge status-connected"
              >up to date</span>
              <span
                v-else-if="restartingSet.has(img.name)"
                class="status-badge status-connecting"
              >restarting...</span>
              <q-btn
                v-else
                unelevated dense
                label="Update"
                class="btn-primary btn-sm"
                :loading="restartingSet.has(img.name)"
                @click="restartDeployment(img)"
              />
            </div>
          </div>
        </template>
      </div>

      <!-- Last checked -->
      <div v-if="lastChecked" class="last-checked">
        Last checked: {{ lastChecked }}
      </div>
    </div>
  </div>
</template>

<script lang="ts" setup>
import { ref, reactive, onMounted } from 'vue';
import { useQuasar } from 'quasar';
import { api } from 'boot/axios';

const $q = useQuasar();

interface ImageInfo {
  name: string;
  namespace: string;
  currentImage: string;
  currentTag: string;
  currentDigest: string;
  remoteDigest: string;
  updateAvailable: boolean;
}

const images = ref<ImageInfo[]>([]);
const loading = ref(false);
const checking = ref(false);
const lastChecked = ref('');
const restartingSet = reactive(new Set<string>());

function shortImage(fullImage: string): string {
  // Shorten ghcr.io/packalares/foo to foo
  return fullImage.replace(/^ghcr\.io\/packalares\//, '');
}

async function checkUpdates() {
  checking.value = true;
  loading.value = true;
  try {
    const resp: any = await api.get('/api/settings/updates');
    const data = resp?.data ?? resp;
    if (Array.isArray(data)) {
      images.value = data;
    }
    const now = new Date();
    lastChecked.value = now.toLocaleTimeString();
  } catch (e: any) {
    console.error('Failed to check updates:', e);
  }
  checking.value = false;
  loading.value = false;
}

async function restartDeployment(img: ImageInfo) {
  restartingSet.add(img.name);
  const oldDigest = img.currentDigest;
  try {
    await api.post(`/api/settings/updates/${img.name}`);
    // Wait for pod to restart then re-check
    // First wait 5s for old pod to terminate, then poll for new pod with valid digest
    await new Promise(r => setTimeout(r, 5000));
    let attempts = 0;
    const poll = setInterval(async () => {
      attempts++;
      try {
        const resp: any = await api.get('/api/settings/updates');
        const data = resp?.data ?? resp;
        if (Array.isArray(data)) {
          const updated = data.find((d: ImageInfo) => d.name === img.name);
          if (updated && updated.currentDigest !== 'unknown') {
            // Pod has a valid digest — restart completed
            clearInterval(poll);
            restartingSet.delete(img.name);
            images.value = data;
            lastChecked.value = new Date().toLocaleTimeString();
            if (updated.currentDigest === updated.remoteDigest) {
              $q.notify({ type: 'positive', message: `${img.name} updated successfully` });
            } else if (updated.currentDigest === oldDigest) {
              $q.notify({ type: 'info', message: `${img.name} restarted (already latest)` });
            }
          }
        }
      } catch {}
      if (attempts >= 15) {
        clearInterval(poll);
        restartingSet.delete(img.name);
        await checkUpdates();
      }
    }, 4000);
  } catch (e: any) {
    console.error('Failed to restart deployment:', e);
    restartingSet.delete(img.name);
  }
}

onMounted(() => {
  checkUpdates();
});
</script>

<style lang="scss" scoped>
.update-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 14px 20px;
  gap: 16px;
}

.update-info {
  flex: 1;
  min-width: 0;
}

.update-name {
  font-size: 13px;
  font-weight: 600;
  color: var(--ink-1);
  margin-bottom: 4px;
}

.update-image-line {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}

.update-image-name {
  font-size: 12px;
  color: var(--ink-3);
  font-family: 'JetBrains Mono', monospace;
}

.update-tag {
  display: inline-flex;
  align-items: center;
  padding: 1px 6px;
  border-radius: 4px;
  font-size: 11px;
  font-family: 'JetBrains Mono', monospace;
  font-weight: 500;
}

.current-tag {
  background: var(--bg-3);
  color: var(--ink-2);
}

.latest-tag {
  background: var(--positive-soft, rgba(52, 211, 153, 0.1));
  color: var(--positive, #34d399);
}

.update-digest {
  font-size: 10px;
  color: var(--ink-3);
  font-family: 'JetBrains Mono', monospace;
  margin-left: 4px;
}

.latest-digest {
  color: var(--positive);
}

.update-arrow {
  color: var(--ink-3);
}

.update-actions {
  flex-shrink: 0;
}

.btn-sm {
  font-size: 12px !important;
  padding: 4px 14px !important;
}

.last-checked {
  margin-top: 16px;
  font-size: 11px;
  color: var(--ink-3);
  text-align: right;
}
</style>
