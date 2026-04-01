import { defineStore } from 'pinia';
import { getWsUrl } from 'boot/axios';
import { useMonitorStore } from './monitor';

export const useWebSocketStore = defineStore('websocket', {
  state: () => ({
    connected: false,
    reconnectAttempts: 0,
    ws: null as WebSocket | null,
  }),
  actions: {
    start() {
      const url = getWsUrl();

      try {
        this.ws = new WebSocket(url);
      } catch (e) {
        console.warn('WebSocket connection failed:', e);
        this.scheduleReconnect();
        return;
      }

      this.ws.onopen = () => {
        console.log('WebSocket connected');
        this.connected = true;
        this.reconnectAttempts = 0;
      };

      this.ws.onmessage = (event) => {
        try {
          const msg = JSON.parse(event.data);
          this.handleMessage(msg);
        } catch (e) {
          // ignore non-JSON messages
        }
      };

      this.ws.onclose = () => {
        console.log('WebSocket disconnected');
        this.connected = false;
        this.ws = null;
        this.scheduleReconnect();
      };

      this.ws.onerror = () => {
        this.connected = false;
      };
    },

    handleMessage(msg: { type: string; data: unknown }) {
      switch (msg.type) {
        case 'metrics': {
          // Update monitor store with pushed metrics
          const monitorStore = useMonitorStore();
          const d = msg.data as Record<string, unknown>;
          if (d) {
            monitorStore.cpuUsage = (d.cpu_usage as number) || 0;
            const mem = d.memory as Record<string, number>;
            if (mem) {
              monitorStore.memUsed = mem.used || 0;
              monitorStore.memTotal = mem.total || 0;
            }
            const disk = d.disk as Record<string, number>;
            if (disk) {
              monitorStore.diskUsed = disk.used || 0;
              monitorStore.diskTotal = disk.total || 0;
            }
            monitorStore.uptime = (d.uptime as number) || 0;
            monitorStore.load = (d.load as [number, number, number]) || [0, 0, 0];
            monitorStore.cpuFreqMHz = (d.cpu_freq_mhz as number) || 0;
            const swap = d.swap as Record<string, number>;
            if (swap) {
              monitorStore.swapUsed = swap.used || 0;
              monitorStore.swapTotal = swap.total || 0;
            }
            const temps = d.temps as Record<string, number>;
            if (temps) {
              monitorStore.tempCPU = temps.cpu || 0;
              monitorStore.tempGPU = temps.gpu || 0;
              monitorStore.tempNVMe = temps.nvme || 0;
            }
            const power = d.power as Record<string, number>;
            if (power) {
              monitorStore.powerCPU = power.cpu_watts || 0;
              monitorStore.powerGPU = power.gpu_watts || 0;
              monitorStore.powerTotal = power.total_watts || 0;
            }
            const fans = d.fans as { fans?: Array<{ name: string; rpm: number }> };
            if (fans?.fans) {
              monitorStore.fans = fans.fans;
            }
            const diskIO = d.disk_io as Record<string, number>;
            if (diskIO) {
              monitorStore.diskRead = diskIO.read_bytes_per_sec || 0;
              monitorStore.diskWrite = diskIO.write_bytes_per_sec || 0;
            }
            const net = d.network as Record<string, number>;
            if (net) {
              monitorStore.netRx = net.rx_bytes_per_sec || 0;
              monitorStore.netTx = net.tx_bytes_per_sec || 0;
            }
          }
          break;
        }
        case 'app_state': {
          // App state change notification
          console.log('App state:', msg.data);
          break;
        }
        case 'alert': {
          // System alert
          console.log('Alert:', msg.data);
          break;
        }
        case 'connected':
        case 'ping':
        case 'install_progress':
          // Heartbeat and app-specific messages handled by individual pages
          break;
        default:
          console.log('Unknown WS message:', msg.type);
      }
    },

    scheduleReconnect() {
      if (this.reconnectAttempts >= 10) return; // Give up after 10 attempts
      this.reconnectAttempts++;
      const delay = Math.min(1000 * Math.pow(2, this.reconnectAttempts), 30000);
      setTimeout(() => this.start(), delay);
    },

    stop() {
      if (this.ws) {
        this.ws.close();
        this.ws = null;
      }
      this.connected = false;
    },
  },
});
