import { defineStore } from 'pinia';
import { api } from 'boot/axios';

export const useUserStore = defineStore('user', {
  state: () => ({
    username: '',
    terminusName: '',
    avatar: '',
    zone: '',
    role: '',
    authenticated: false,
  }),
  actions: {
    async loadUserInfo() {
      try {
        const data: any = await api.get('/api/user/info');
        const u = data.data || data;
        this.username = u.name || '';
        this.terminusName = u.terminusName || '';
        this.avatar = u.avatar || '';
        this.zone = u.zone || '';
        this.role = u.owner_role || '';
        this.authenticated = true;
      } catch (e) {
        console.warn('Failed to load user info:', e);
      }
    },
    async logout() {
      try { await api.post('/api/auth/logout'); } catch {}
      const host = window.location.hostname;
      const parts = host.split('.');
      const authUrl = parts.length >= 3
        ? 'https://auth.' + parts.slice(1).join('.') + '/login'
        : '/login';
      const w = window.top || window;
      w.location.href = authUrl;
    },
  },
});
