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

  router.beforeEach((to) => {
    const sub = getSubdomain();

    // Wizard only accessible on auth subdomain or IP
    if (to.path === '/wizard') {
      const host = window.location.hostname;
      const isIP = /^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}$/.test(host);
      const isAuth = host.startsWith('auth.');
      if (!isIP && !isAuth) return '/';
    }

    // On a known subdomain: enforce path belongs to this subdomain
    if (sub && sub !== 'auth') {
      const expectedPrefix = '/' + sub;
      if (to.path !== '/' && !to.path.startsWith(expectedPrefix)) {
        return '/';
      }
    }

    // Unknown paths → redirect to correct root for this subdomain
    if (to.matched.length === 0 || to.matched[0]?.path === '/:catchAll(.*)*') {
      if (sub && subdomainRoutes[sub]) {
        return subdomainRoutes[sub];
      }
      return '/desktop';
    }
  });

  return router;
});
