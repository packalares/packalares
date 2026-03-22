<template>
  <div class="settings-page">
    <div class="page-title">Storage</div>
    <div class="page-scroll">
      <!-- Disk Overview -->
      <div class="section-title">Disk Usage</div>
      <div class="settings-card">
        <div class="metric-row">
          <div class="metric-header">
            <span class="info-label">Total Disk</span>
            <span class="metric-value" :class="usageColor(diskPercent)">
              {{ formatBytes(diskUsed) }} / {{ formatBytes(diskTotal) }}
              ({{ diskPercent.toFixed(1) }}%)
            </span>
          </div>
          <q-linear-progress
            :value="diskPercent / 100"
            :color="usageQColor(diskPercent)"
            track-color="grey-9"
            rounded
            size="10px"
            class="q-mt-sm"
          />
        </div>
      </div>

      <!-- Mount Points -->
      <div class="section-title">Volumes</div>
      <div class="settings-card">
        <template v-if="volumes.length > 0">
          <template v-for="(vol, idx) in volumes" :key="vol.mount">
            <div class="volume-row">
              <div class="volume-header">
                <div class="volume-info">
                  <q-icon name="sym_r_folder" size="18px" color="grey-5" class="q-mr-sm" />
                  <span class="volume-mount">{{ vol.mount }}</span>
                </div>
                <span class="volume-size">
                  {{ formatBytes(vol.used) }} / {{ formatBytes(vol.total) }}
                </span>
              </div>
              <q-linear-progress
                :value="vol.total > 0 ? vol.used / vol.total : 0"
                :color="usageQColor(vol.total > 0 ? (vol.used / vol.total) * 100 : 0)"
                track-color="grey-9"
                rounded
                size="6px"
                class="q-mt-xs"
              />
              <div class="volume-detail">
                <span>{{ vol.filesystem }}</span>
                <span>{{ vol.type }}</span>
              </div>
            </div>
            <q-separator v-if="idx < volumes.length - 1" class="card-separator" />
          </template>
        </template>
        <div v-else class="empty-state">
          <q-icon name="sym_r_storage" size="48px" color="grey-7" />
          <div class="empty-text">Loading storage information...</div>
        </div>
      </div>
    </div>
  </div>
</template>

<script lang="ts" setup>
import { ref, computed, onMounted } from 'vue';
import { api } from 'boot/axios';

interface VolumeInfo {
  mount: string;
  filesystem: string;
  type: string;
  total: number;
  used: number;
}

const diskTotal = ref(0);
const diskUsed = ref(0);
const volumes = ref<VolumeInfo[]>([]);

const diskPercent = computed(() => {
  if (!diskTotal.value) return 0;
  return (diskUsed.value / diskTotal.value) * 100;
});

const usageColor = (pct: number) => {
  if (pct >= 80) return 'text-usage-red';
  if (pct >= 50) return 'text-usage-yellow';
  return 'text-usage-green';
};

const usageQColor = (pct: number) => {
  if (pct >= 80) return 'red-6';
  if (pct >= 50) return 'amber-7';
  return 'green-6';
};

const formatBytes = (bytes: number) => {
  if (!bytes) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let i = 0;
  let val = bytes;
  while (val >= 1024 && i < units.length - 1) {
    val /= 1024;
    i++;
  }
  return val.toFixed(1) + ' ' + units[i];
};

onMounted(async () => {
  try {
    const res: any = await api.get('/api/metrics');
    if (res) {
      diskTotal.value = res.disk_total || 0;
      diskUsed.value = res.disk_used || 0;

      if (res.volumes && Array.isArray(res.volumes)) {
        volumes.value = res.volumes.map((v: any) => ({
          mount: v.mount || v.mountpoint || '--',
          filesystem: v.filesystem || v.device || '--',
          type: v.type || v.fstype || '--',
          total: v.total || 0,
          used: v.used || 0,
        }));
      } else {
        // Fallback: create a single entry from aggregate data
        volumes.value = [
          {
            mount: '/',
            filesystem: 'root',
            type: 'ext4',
            total: res.disk_total || 0,
            used: res.disk_used || 0,
          },
        ];
      }
    }
  } catch {
    // keep defaults
  }
});
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

.info-label {
  font-size: 14px;
  color: var(--ink-1);
  font-weight: 500;
}

.metric-row {
  padding: 16px 20px;
}

.metric-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.metric-value {
  font-size: 13px;
  font-weight: 600;
  font-family: 'JetBrains Mono', 'SF Mono', monospace;
}

.volume-row {
  padding: 14px 20px;
}

.volume-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.volume-info {
  display: flex;
  align-items: center;
}

.volume-mount {
  font-size: 14px;
  color: var(--ink-1);
  font-weight: 500;
  font-family: 'JetBrains Mono', 'SF Mono', monospace;
}

.volume-size {
  font-size: 13px;
  color: var(--ink-2);
  font-family: 'JetBrains Mono', 'SF Mono', monospace;
}

.volume-detail {
  display: flex;
  justify-content: space-between;
  margin-top: 6px;
  font-size: 12px;
  color: var(--ink-3);
}

.card-separator {
  background: var(--separator);
  margin: 0 20px;
}

.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 40px 20px;
}

.empty-text {
  font-size: 13px;
  color: var(--ink-3);
  margin-top: 12px;
}

.text-usage-green {
  color: #29cc5f;
}

.text-usage-yellow {
  color: #febe01;
}

.text-usage-red {
  color: #ff4d4d;
}
</style>
