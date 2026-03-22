const { configure } = require('quasar/wrappers');

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
    devServer: { open: false, port: 9000 },
  };
});
// This file is intentionally empty - quasar reads it
