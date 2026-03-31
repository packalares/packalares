import { defineStore } from 'pinia';

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
});
