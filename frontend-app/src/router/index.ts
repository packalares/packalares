import { route } from 'quasar/wrappers';
import { createRouter, createWebHistory } from 'vue-router';
import routes from './routes';

// Map subdomains to their Vue Router paths
const subdomainRoutes: Record<string, string> = {
  desktop: '/desktop',
  settings: '/settings/account',
  market: '/market',
  files: '/files',
  dashboard: '/dashboard',
  auth: '/login',
};

function getSubdomain(): string | null {
  const host = window.location.hostname;
  // Skip IP addresses (188.241.210.104 has 4 dots but is not a subdomain)
  if (/^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}$/.test(host)) return null;
  const parts = host.split('.');
  if (parts.length >= 3) {
    const sub = parts[0];
    if (sub in subdomainRoutes) return sub;
  }
  return null;
}

export default route(function () {
  const router = createRouter({
    scrollBehavior: () => ({ left: 0, top: 0 }),
    routes,
    history: createWebHistory(),
  });

  // No guard needed — SubdomainRouter handles / based on hostname

  return router;
});
