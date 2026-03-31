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
        <q-btn
          v-if="updatesAvailable > 0"
          unelevated dense
          label="Update All"
          class="btn-primary q-ml-sm"
          :loading="updatingAll"
          @click="updateAll"
        />
      </div>
      <div class="page-description">
        Manage container image versions and apply updates to your system.
        <span v-if="lastChecked" style="margin-left: 8px; opacity: 0.7">Last checked: {{ lastChecked }}</span>
      </div>
    </div>
    <div class="page-scroll">

      <div class="settings-card">
        <div class="card-header">
          <div class="card-header-icon card-header-icon--update">
            <q-icon name="sym_r_system_update_alt" size="18px" />
          </div>
          <div class="card-header-text">
            <div class="card-header-title">Container Images</div>
            <div class="card-header-subtitle">
              {{ images.length ? images.length + ' images tracked' : 'Packalares framework images' }}
              <template v-if="updatesAvailable > 0">
                &mdash; <span class="updates-count">{{ updatesAvailable }} update{{ updatesAvailable > 1 ? 's' : '' }} available</span>
              </template>
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
          <div>No Packalares images found.</div>
        </div>

        <!-- Image list as table -->
        <table v-else class="data-table">
          <thead>
            <tr>
              <th>Deployment</th>
              <th>Image</th>
              <th>Digest</th>
              <th>Pod</th>
              <th style="text-align:right">Status</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="img in images" :key="img.name + img.namespace">
              <td>
                <span class="update-name">{{ img.name }}</span>
                <span v-if="img.namespace !== frameworkNs" class="update-ns">{{ img.namespace }}</span>
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
              <td>
                <span :class="['pod-status', podStatusClass(img)]">
                  {{ updating[img.name] ? updating[img.name] : img.podStatus }}
                </span>
              </td>
              <td style="text-align:right">
                <span
                  v-if="updating[img.name]"
                  class="status-badge status-connecting"
                >{{ updating[img.name] }}</span>
                <span
                  v-else-if="!img.updateAvailable"
                  class="status-badge status-connected"
                >up to date</span>
                <q-btn
                  v-else
                  unelevated dense
                  label="Update"
                  class="btn-primary btn-sm"
                  @click="restartDeployment(img)"
                />
              </td>
            </tr>
          </tbody>
        </table>

        <!-- Error log -->
        <div v-if="errors.length" class="error-log">
          <div v-for="(err, i) in errors" :key="i" class="error-line">{{ err }}</div>
        </div>
      </div>
    </div>
  </div>
</template>

<script lang="ts" setup>
import { ref, reactive, computed, onMounted, onUnmounted } from 'vue';
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
  podStatus: string;
}

const images = ref<ImageInfo[]>([]);
const loading = ref(false);
const checking = ref(false);
const lastChecked = ref('');
const updating = reactive<Record<string, string>>({}); // name -> status text
const updatingAll = ref(false);
const errors = ref<string[]>([]);
const frameworkNs = 'os-framework';
let pollTimer: ReturnType<typeof setInterval> | null = null;

const updatesAvailable = computed(() => images.value.filter(i => i.updateAvailable).length);

function shortImage(fullImage: string): string {
  return fullImage.replace(/^ghcr\.io\/packalares\//, '');
}

function podStatusClass(img: ImageInfo): string {
  const status = updating[img.name] || img.podStatus;
  if (!status) return '';
  const s = status.toLowerCase();
  if (s === 'running') return 'pod-running';
  if (s === 'pending' || s === 'containercreating') return 'pod-pending';
  if (s === 'terminating' || s.includes('restarting') || s.includes('updating') || s.includes('pulling')) return 'pod-updating';
  if (s.includes('error') || s.includes('crash') || s.includes('fail') || s === 'imagepullbackoff') return 'pod-error';
  return 'pod-pending';
}

async function checkUpdates() {
  checking.value = true;
  loading.value = true;
  try {
    const resp: any = await api.get('/api/settings/updates');
    const data = resp?.data ?? resp;
    if (Array.isArray(data)) {
      images.value = data;
      // Clear updating status for pods that are now Running
      for (const img of data) {
        if (updating[img.name] && img.podStatus === 'Running' && !img.updateAvailable) {
          delete updating[img.name];
          $q.notify({ type: 'positive', message: `${img.name} updated` });
        }
      }
    }
    lastChecked.value = new Date().toLocaleTimeString();
  } catch (e: any) {
    errors.value.push(`Failed to check updates: ${e.message || e}`);
  }
  checking.value = false;
  loading.value = false;
}

async function restartDeployment(img: ImageInfo) {
  updating[img.name] = 'Restarting...';
  errors.value = [];
  try {
    const path = img.namespace === frameworkNs
      ? `/api/settings/updates/${img.name}`
      : `/api/settings/updates/${img.namespace}/${img.name}`;
    const resp: any = await api.post(path);
    const data = resp?.data ?? resp;
    if (data?.error || data?.message?.includes('failed') || data?.message?.includes('not found')) {
      errors.value.push(`${img.name}: ${data.message || data.error}`);
      delete updating[img.name];
      return;
    }
    updating[img.name] = 'Pulling image...';
    // Start polling for this deployment
    startPollIfNeeded();
  } catch (e: any) {
    errors.value.push(`${img.name}: ${e.message || e}`);
    delete updating[img.name];
  }
}

async function updateAll() {
  updatingAll.value = true;
  const toUpdate = images.value.filter(i => i.updateAvailable);
  for (const img of toUpdate) {
    await restartDeployment(img);
    // Small delay between restarts to avoid overwhelming the API
    await new Promise(r => setTimeout(r, 1000));
  }
  updatingAll.value = false;
}

function startPollIfNeeded() {
  if (pollTimer) return;
  pollTimer = setInterval(async () => {
    const anyUpdating = Object.keys(updating).length > 0;
    if (!anyUpdating) {
      if (pollTimer) {
        clearInterval(pollTimer);
        pollTimer = null;
      }
      return;
    }
    await checkUpdates();
  }, 3000);
}

onMounted(() => {
  checkUpdates();
  // If there are leftover updating states (page refresh), start polling
  if (Object.keys(updating).length > 0) {
    startPollIfNeeded();
  }
});

onUnmounted(() => {
  if (pollTimer) {
    clearInterval(pollTimer);
    pollTimer = null;
  }
});
</script>

<style lang="scss" scoped>
.updates-count {
  color: var(--positive);
  font-weight: 600;
}

.update-name {
  font-size: 13px;
  font-weight: 600;
  color: var(--ink-1);
}

.update-ns {
  font-size: 10px;
  color: var(--ink-3);
  margin-left: 6px;
  padding: 1px 5px;
  background: var(--bg-3);
  border-radius: 3px;
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

.pod-status {
  font-size: 11px;
  font-weight: 500;
}

.pod-running {
  color: var(--positive);
}

.pod-pending, .pod-updating {
  color: var(--warning);
}

.pod-error {
  color: var(--negative);
}

.error-log {
  padding: 12px 16px;
  border-top: 1px solid var(--bg-3);
}

.error-line {
  font-size: 12px;
  color: var(--negative);
  padding: 2px 0;
  font-family: 'Inter', monospace;
}
</style>
