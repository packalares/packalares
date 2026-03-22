import { boot } from 'quasar/wrappers';
import axios, { AxiosInstance } from 'axios';

const api: AxiosInstance = axios.create({ baseURL: '/' });

api.interceptors.response.use(
  (response) => response.data,
  (error) => {
    if (error.response?.status === 401) {
      const rd = encodeURIComponent(window.location.href);
      // On subdomains, redirect to the main domain's login page
      const host = window.location.hostname;
      const parts = host.split('.');
      let loginUrl = '/login?rd=' + rd;
      if (parts.length >= 3) {
        // subdomain: market.laurs.olares.local → laurs.olares.local/login
        const mainDomain = parts.slice(1).join('.');
        loginUrl = 'https://' + mainDomain + '/login?rd=' + rd;
      }
      window.location.href = loginUrl;
    }
    return Promise.reject(error);
  }
);

export default boot(({ app }) => {
  app.config.globalProperties.$api = api;
});

export { api };
