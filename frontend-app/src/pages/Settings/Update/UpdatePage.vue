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
          v-if="packalaresUpdatesAvailable > 0"
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

      <!-- Loading state -->
      <div v-if="loading && !images.length" class="settings-card">
        <div class="empty-state">
          <q-spinner-dots size="32px" color="grey-5" />
          <div>Loading deployment images...</div>
        </div>
      </div>

      <!-- Empty state -->
      <div v-else-if="!images.length" class="settings-card">
        <div class="empty-state">
          <div class="empty-state-icon">
            <q-icon name="sym_r_inventory_2" size="24px" color="grey-6" />
          </div>
          <div>No Packalares images found.</div>
        </div>
      </div>

      <template v-else>
        <!-- Packalares Services -->
        <div class="settings-card">
          <div class="card-header">
            <div class="card-header-icon card-header-icon--update">
              <q-icon name="sym_r_system_update_alt" size="18px" />
            </div>
            <div class="card-header-text">
              <div class="card-header-title">Packalares Services</div>
              <div class="card-header-subtitle">
                {{ packalaresImages.length }} images
                <template v-if="packalaresUpdatesAvailable > 0">
                  &mdash; <span class="updates-count">{{ packalaresUpdatesAvailable }} update{{ packalaresUpdatesAvailable > 1 ? 's' : '' }} available</span>
                </template>
              </div>
            </div>
          </div>

          <table v-if="packalaresImages.length" class="data-table">
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
              <tr v-for="img in packalaresImages" :key="img.name + img.namespace">
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
                    {{ displayPodStatus(img) }}
                  </span>
                </td>
                <td style="text-align:right">
                  <span
                    v-if="img.updateStatus === 'restarting' || img.updateStatus === 'pulling'"
                    class="status-badge status-connecting"
                  >{{ img.updateStatus === 'pulling' ? 'Pulling image...' : 'Restarting...' }}</span>
                  <span
                    v-else-if="img.updateStatus === 'complete'"
                    class="status-badge status-connected"
                  >updated</span>
                  <span
                    v-else-if="img.updateStatus === 'failed'"
                    class="status-badge status-error"
                  >failed</span>
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
        </div>

        <!-- Infrastructure -->
        <div class="settings-card">
          <div
            class="card-header card-header--collapsible"
            @click="infraExpanded = !infraExpanded"
          >
            <div class="card-header-icon card-header-icon--infra">
              <q-icon name="sym_r_dns" size="18px" />
            </div>
            <div class="card-header-text">
              <div class="card-header-title">Infrastructure</div>
              <div class="card-header-subtitle">{{ infraImages.length }} mirrored third-party images (pinned versions)</div>
            </div>
            <q-icon
              :name="infraExpanded ? 'sym_r_expand_less' : 'sym_r_expand_more'"
              size="20px"
              class="collapse-icon"
            />
          </div>

          <table v-if="infraExpanded && infraImages.length" class="data-table">
            <thead>
              <tr>
                <th>Deployment</th>
                <th>Image</th>
                <th>Version</th>
                <th>Pod</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="img in infraImages" :key="img.name + img.namespace">
                <td>
                  <span class="update-name">{{ img.name }}</span>
                  <span v-if="img.namespace !== frameworkNs" class="update-ns">{{ img.namespace }}</span>
                </td>
                <td>
                  <span class="update-image-name">{{ shortImage(img.currentImage) }}</span>
                </td>
                <td>
                  <span class="update-tag current-tag">{{ img.currentTag }}</span>
                </td>
                <td>
                  <span :class="['pod-status', podStatusClass(img)]">
                    {{ img.podStatus }}
                  </span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </template>

      <!-- Error log -->
      <div v-if="errors.length" class="settings-card error-log">
        <div v-for="(err, i) in errors" :key="i" class="error-line">{{ err }}</div>
      </div>
    </div>
  </div>
</template>

<script lang="ts" setup>
import { ref, computed, onMounted, onUnmounted } from 'vue';
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
  type: string;        // "packalares" | "infrastructure"
  updateStatus: string; // "" | "restarting" | "pulling" | "complete" | "failed"
}

const images = ref<ImageInfo[]>([]);
const loading = ref(false);
const checking = ref(false);
const lastChecked = ref('');
const updatingAll = ref(false);
const errors = ref<string[]>([]);
const infraExpanded = ref(false);
const frameworkNs = 'os-framework';
let pollTimer: ReturnType<typeof setInterval> | null = null;

const packalaresImages = computed(() => images.value.filter(i => i.type === 'packalares'));
const infraImages = computed(() => images.value.filter(i => i.type === 'infrastructure'));
const packalaresUpdatesAvailable = computed(() => packalaresImages.value.filter(i => i.updateAvailable).length);
const anyUpdating = computed(() => images.value.some(i => i.updateStatus === 'restarting' || i.updateStatus === 'pulling'));

function shortImage(fullImage: string): string {
  return fullImage.replace(/^ghcr\.io\/packalares\//, '');
}

function displayPodStatus(img: ImageInfo): string {
  if (img.updateStatus === 'restarting' || img.updateStatus === 'pulling') {
    return img.podStatus || 'Updating';
  }
  return img.podStatus;
}

function podStatusClass(img: ImageInfo): string {
  const status = img.podStatus;
  if (!status) return '';
  const s = status.toLowerCase();
  if (s === 'running') return 'pod-running';
  if (s === 'pending' || s === 'containercreating') return 'pod-pending';
  if (s === 'terminating') return 'pod-updating';
  if (s.includes('error') || s.includes('crash') || s.includes('fail') || s === 'imagepullbackoff') return 'pod-error';
  if (img.updateStatus === 'restarting' || img.updateStatus === 'pulling') return 'pod-updating';
  return 'pod-pending';
}

async function checkUpdates() {
  checking.value = true;
  loading.value = true;
  try {
    const resp: any = await api.get('/api/settings/updates');
    const raw = resp?.data ?? resp;
    const data = Array.isArray(raw) ? raw : [];
    // Normalize: fill in missing type/updateStatus for older BFL versions
    for (const img of data) {
      if (!img.type) {
        img.type = img.currentTag === 'latest' ? 'packalares' : 'infrastructure';
      }
      if (!img.updateStatus) {
        img.updateStatus = '';
      }
    }
    if (data.length) {
      // Check if any previously-updating items just completed
      for (const img of data) {
        const prev = images.value.find(p => p.name === img.name && p.namespace === img.namespace);
        if (prev && (prev.updateStatus === 'restarting' || prev.updateStatus === 'pulling') && img.updateStatus === 'complete') {
          $q.notify({ type: 'positive', message: `${img.name} updated` });
        }
      }
      images.value = data;
    }
    lastChecked.value = new Date().toLocaleTimeString();
  } catch (e: any) {
    errors.value.push(`Failed to check updates: ${e.message || e}`);
  }
  checking.value = false;
  loading.value = false;

  // Manage polling based on whether any updates are in progress
  if (anyUpdating.value) {
    startPollIfNeeded();
  } else {
    stopPoll();
  }
}

async function restartDeployment(img: ImageInfo) {
  errors.value = [];
  try {
    const path = `/api/settings/updates/${img.namespace}/${img.name}`;
    const resp: any = await api.post(path);
    const data = resp?.data ?? resp;
    if (data?.error || data?.message?.includes('failed') || data?.message?.includes('not found')) {
      errors.value.push(`${img.name}: ${data.message || data.error}`);
      return;
    }
    // Immediately reflect the status change locally
    const found = images.value.find(i => i.name === img.name && i.namespace === img.namespace);
    if (found) {
      found.updateStatus = data.updateStatus || 'pulling';
    }
    startPollIfNeeded();
  } catch (e: any) {
    errors.value.push(`${img.name}: ${e.message || e}`);
  }
}

async function updateAll() {
  updatingAll.value = true;
  const toUpdate = packalaresImages.value.filter(i => i.updateAvailable && !i.updateStatus);
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
    await checkUpdates();
  }, 3000);
}

function stopPoll() {
  if (pollTimer) {
    clearInterval(pollTimer);
    pollTimer = null;
  }
}

onMounted(() => {
  checkUpdates();
});

onUnmounted(() => {
  stopPoll();
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

.card-header--collapsible {
  cursor: pointer;
  user-select: none;

  &:hover {
    opacity: 0.85;
  }
}

.collapse-icon {
  margin-left: auto;
  color: var(--ink-3);
}

.status-error {
  color: var(--negative);
  font-weight: 500;
  font-size: 12px;
}

.error-log {
  padding: 12px 16px;
}

.error-line {
  font-size: 12px;
  color: var(--negative);
  padding: 2px 0;
  font-family: 'Inter', monospace;
}
</style>
