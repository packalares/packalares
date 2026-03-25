import { boot } from 'quasar/wrappers';
import axios, { AxiosInstance } from 'axios';

function isIP(host: string): boolean {
  return /^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}$/.test(host);
}

// Returns the user zone (e.g. admin.olares.local) from the hostname.
// Works for both zone access and subdomain access.
function getZone(): string {
  const host = window.location.hostname;
  if (isIP(host)) return '';
  const parts = host.split('.');
  if (parts.length >= 4) {
    // subdomain.admin.olares.local → admin.olares.local
    return parts.slice(1).join('.');
  }
  if (parts.length === 3) {
    // admin.olares.local → admin.olares.local
    return host;
  }
  return '';
}

// Returns the API base URL:
//   IP access: '' → /api/* on same origin
//   Zone (admin.olares.local): '' → /api/* on same origin
//   Subdomain (desktop.admin.olares.local): 'https://api.admin.olares.local'
export function getApiBase(): string {
  const host = window.location.hostname;
  if (isIP(host)) return '';
  const parts = host.split('.');
  if (parts.length >= 4) {
    // On a subdomain — use api.zone for cross-origin API calls
    return 'https://api.' + parts.slice(1).join('.');
  }
  // On the zone itself or IP — same origin
  return '';
}

// Returns the auth URL for login redirects:
//   IP: '/login'
//   Domain: 'https://auth.zone/login'
export function getAuthUrl(rd?: string): string {
  const zone = getZone();
  const rdParam = rd ? '?rd=' + encodeURIComponent(rd) : '';
  if (!zone) return '/login' + rdParam;
  return 'https://auth.' + zone + '/login' + rdParam;
}

// Returns the desktop URL:
//   IP: '/desktop'
//   Domain: 'https://desktop.zone'
export function getDesktopUrl(): string {
  const zone = getZone();
  if (!zone) return '/desktop';
  return 'https://desktop.' + zone;
}

// Returns the WebSocket URL:
//   IP: wss://host/ws
//   Subdomain: wss://api.zone/ws
export function getWsUrl(): string {
  const base = getApiBase();
  if (base) {
    return base.replace('https://', 'wss://') + '/ws';
  }
  const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  return proto + '//' + window.location.host + '/ws';
}

const api: AxiosInstance = axios.create({
  baseURL: getApiBase(),
  withCredentials: true,
});

api.interceptors.response.use(
  (response) => response.data,
  (error) => {
    if (error.response?.status === 401) {
      // Don't redirect on auth/totp or login page requests
      const url = error.config?.url || '';
      const onLoginPage = window.location.pathname.startsWith('/login');
      if (!url.includes('/auth/totp') && !url.includes('/auth/login') && !onLoginPage) {
        window.location.href = getAuthUrl(window.location.href);
      }
    }
    return Promise.reject(error);
  }
);

export default boot(({ app }) => {
  app.config.globalProperties.$api = api;
});

export { api };
