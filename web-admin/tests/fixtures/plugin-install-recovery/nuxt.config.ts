import { fileURLToPath } from 'node:url';

// 仅专用浏览器验收通过 --extends 加载；正常产品构建不包含测试页面。
export default defineNuxtConfig({
  hooks: {
    'pages:extend'(pages) {
      pages.push({ name: 'plugin-install-recovery-fixture', path: '/__fixtures/plugin-install-recovery',
        file: fileURLToPath(new URL('./install.vue', import.meta.url)) });
    },
  },
});
