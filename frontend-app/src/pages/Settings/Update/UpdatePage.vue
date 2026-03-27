<template>
  <div class="settings-page">
    <div class="page-header">
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
      <div class="page-description">Manage container image versions and apply updates to your system.</div>
    </div>
    <div class="page-scroll">

      <!-- Container Images -->
      <div class="settings-card">
        <div class="card-header">
          <div class="card-header-icon card-header-icon--update">
            <q-icon name="sym_r_system_update_alt" size="18px" />
          </div>
          <div class="card-header-text">
            <div class="card-header-title">Container Images</div>
            <div class="card-header-subtitle">
              {{ images.length ? images.length + ' images tracked' : 'Packalares framework images' }}
              <span v-if="lastChecked" style="margin-left: 8px; opacity: 0.7">Last checked: {{ lastChecked }}</span>
            </div>
          </div>
        </div>

        <!-- Loading state -->
        <div v-if="loading && !images.length" class="empty-state">
          <q-spinner-dots size="32px" color="grey-5" />
          <div>Loading deployment images...</div>
        </div>

        <!-- Empty state -->
        <div v-else-if="!images.length" class="empty-state">
          <div class="empty-state-icon">
            <q-icon name="sym_r_inventory_2" size="24px" color="grey-6" />
          </div>
          <div>No Packalares images found in the framework namespace.</div>
        </div>

        <!-- Image list as table -->
        <table v-else class="data-table">
          <thead>
            <tr>
              <th>Deployment</th>
              <th>Image</th>
              <th>Digest</th>
              <th style="text-align:right">Status</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="img in images" :key="img.name + img.currentImage">
              <td>
                <span class="update-name">{{ img.name }}</span>
              </td>
              <td>
                <span class="update-image-name">{{ shortImage(img.currentImage) }}</span>
                <span class="update-tag current-tag">:{{ img.currentTag }}</span>
              </td>
              <td>
                <span class="update-digest">{{ img.currentDigest || '--' }}</span>
                <template v-if="img.updateAvailable">
                  <q-icon name="sym_r_arrow_forward" size="12px" class="update-arrow q-mx-xs" />
                  <span class="update-digest latest-digest">{{ img.remoteDigest || '' }}</span>
                </template>
              </td>
              <td style="text-align:right">
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
              </td>
            </tr>
          </tbody>
        </table>
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
.update-name {
  font-size: 13px;
  font-weight: 600;
  color: var(--ink-1);
}

.update-image-name {
  font-size: 12px;
  color: var(--ink-3);
  font-family: 'Inter', sans-serif;
}

.update-tag {
  display: inline-flex;
  align-items: center;
  padding: 1px 6px;
  border-radius: 4px;
  font-size: 11px;
  font-family: 'Inter', sans-serif;
  font-weight: 500;
}

.current-tag {
  background: var(--bg-3);
  color: var(--ink-2);
}

.update-digest {
  font-size: 10px;
  color: var(--ink-3);
  font-family: 'Inter', sans-serif;
}

.latest-digest {
  color: var(--positive);
}

.update-arrow {
  color: var(--ink-3);
}
</style>
