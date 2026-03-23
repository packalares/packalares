const { configure } = require('quasar/wrappers');

const apiTarget = process.env.API_PROXY_TARGET || 'https://localhost';

module.exports = configure(function () {
  return {
    boot: ['axios'],
    css: ['app.scss'],
    extras: ['material-symbols-rounded', 'roboto-font'],
    supportTS: {
      tsCheckerConfig: { eslint: undefined }
    },
    build: {
      vueRouterMode: 'history',
      publicPath: '/',
      extendWebpack(cfg) {
        cfg.resolve.extensions.push('.ts', '.tsx');
      },
    },
    framework: {
      plugins: ['Notify', 'Dialog', 'Loading'],
      config: {
        dark: true,
        notify: { position: 'top-right', timeout: 3000 }
      }
    },
    devServer: {
      open: false,
      port: process.env.DEV_PORT ? parseInt(process.env.DEV_PORT) : 9000,
      proxy: {
        '/api': {
          target: apiTarget,
          changeOrigin: true,
          secure: false,
        },
        '/ws': {
          target: apiTarget.replace('https://', 'wss://').replace('http://', 'ws://'),
          ws: true,
          secure: false,
        },
      },
    },
  };
});
