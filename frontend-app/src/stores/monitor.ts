import { defineStore } from 'pinia';
import { api } from 'boot/axios';

export const useMonitorStore = defineStore('monitor', {
  state: () => ({
    cpu: { ratio: 0, total: 0, usage: 0 },
    memory: { ratio: 0, total: 0, usage: 0 },
    disk: { ratio: 0, total: 0, usage: 0 },
    gpu: { ratio: 0, total: 0, usage: 0 },
    cpuUsage: 0,
    memUsed: 0,
    memTotal: 0,
    diskUsed: 0,
    diskTotal: 0,
    uptime: 0,
    load: [0, 0, 0],
  }),
  actions: {
    async loadCluster() {
      try {
        const d: any = await api.get('/api/monitor/cluster');
        this.cpu = d.cpu || this.cpu;
        this.memory = d.memory || this.memory;
        this.disk = d.disk || this.disk;
        this.gpu = d.gpu || this.gpu;
      } catch {}
    },
    async loadMetrics() {
      try {
        const d: any = await api.get('/api/metrics');
        this.cpuUsage = d.cpu_usage || 0;
        this.memUsed = d.memory?.used || 0;
        this.memTotal = d.memory?.total || 0;
        this.diskUsed = d.disk?.used || 0;
        this.diskTotal = d.disk?.total || 0;
        this.uptime = d.uptime || 0;
        this.load = d.load || [0, 0, 0];
      } catch {}
    },
  },
});
