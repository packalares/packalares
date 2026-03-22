import { RouteRecordRaw } from 'vue-router';

const routes: RouteRecordRaw[] = [
  { path: '/login', component: () => import('pages/Login/LoginPage.vue'), meta: { public: true } },
  {
    path: '/desktop',
    component: () => import('layouts/DesktopLayout.vue'),
    children: [
      { path: '', component: () => import('pages/Desktop/DesktopPage.vue') },
    ],
  },
  {
    path: '/settings',
    component: () => import('layouts/AppLayout.vue'),
    children: [
      {
        path: '',
        component: () => import('pages/Settings/SettingsPage.vue'),
        children: [
          { path: '', redirect: '/settings/account' },
          { path: 'account', component: () => import('pages/Settings/Account/AccountPage.vue') },
          { path: 'system', component: () => import('pages/Settings/System/SystemPage.vue') },
          { path: 'network', component: () => import('pages/Settings/Network/NetworkPage.vue') },
          { path: 'storage', component: () => import('pages/Settings/Storage/StoragePage.vue') },
          { path: 'gpu', component: () => import('pages/Settings/GPU/GpuPage.vue') },
          { path: 'appearance', component: () => import('pages/Settings/Appearance/AppearancePage.vue') },
          { path: 'about', component: () => import('pages/Settings/About/AboutPage.vue') },
        ],
      },
    ],
  },
  {
    path: '/market',
    component: () => import('layouts/AppLayout.vue'),
    children: [
      { path: '', component: () => import('pages/Market/MarketPage.vue') },
    ],
  },
  {
    path: '/dashboard',
    component: () => import('layouts/AppLayout.vue'),
    children: [
      { path: '', component: () => import('pages/Dashboard/DashboardPage.vue') },
    ],
  },
  {
    path: '/files',
    component: () => import('layouts/AppLayout.vue'),
    children: [
      { path: '', component: () => import('pages/Files/FilesPage.vue') },
    ],
  },
  // Root path — renders correct page based on subdomain or defaults to desktop
  {
    path: '/',
    component: () => import('layouts/SubdomainRouter.vue'),
  },
  { path: '/:catchAll(.*)*', component: () => import('pages/Desktop/DesktopPage.vue') },
];

export default routes;
