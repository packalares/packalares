import { boot } from 'quasar/wrappers';
import axios, { AxiosInstance } from 'axios';

const api: AxiosInstance = axios.create({ baseURL: '/' });

api.interceptors.response.use(
  (response) => response.data,
  (error) => {
    if (error.response?.status === 401) {
      const rd = encodeURIComponent(window.location.pathname);
      window.location.href = '/login/?rd=' + rd;
    }
    return Promise.reject(error);
  }
);

export default boot(({ app }) => {
  app.config.globalProperties.$api = api;
});

export { api };
