import { boot } from 'quasar/wrappers';
import axios, { AxiosInstance } from 'axios';

// Returns the API base URL depending on how the user accesses the system:
//   IP access (188.241.210.104): returns '' → calls /api/* on same origin
//   Subdomain (market.laurs.olares.local): returns 'https://api.laurs.olares.local'
export function getApiBase(): string {
  const host = window.location.hostname;
  if (/^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}$/.test(host)) {
    return ''; // same origin on IP
  }
  const parts = host.split('.');
  if (parts.length >= 3) {
    return 'https://api.' + parts.slice(1).join('.');
  }
  return '';
}

// Returns the main domain for redirects:
//   IP: '' (same origin)
//   Subdomain: 'https://laurs.olares.local'
export function getMainDomain(): string {
  const host = window.location.hostname;
  if (/^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}$/.test(host)) {
    return '';
  }
  const parts = host.split('.');
  if (parts.length >= 3) {
    return 'https://' + parts.slice(1).join('.');
  }
  return '';
}

// Returns the WebSocket URL:
//   IP: wss://188.241.210.104/ws
//   Subdomain: wss://api.laurs.olares.local/ws
export function getWsUrl(): string {
  const base = getApiBase();
  if (base) {
    return base.replace('https://', 'wss://') + '/ws';
  }
  const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  return proto + '//' + window.location.host + '/ws';
}

const api: AxiosInstance = axios.create({ baseURL: getApiBase() });

api.interceptors.response.use(
  (response) => response.data,
  (error) => {
    if (error.response?.status === 401) {
      const rd = encodeURIComponent(window.location.href);
      const main = getMainDomain();
      window.location.href = main + '/login?rd=' + rd;
    }
    return Promise.reject(error);
  }
);

export default boot(({ app }) => {
  app.config.globalProperties.$api = api;
});

export { api };
