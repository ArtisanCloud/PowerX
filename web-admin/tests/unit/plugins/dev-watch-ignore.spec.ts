import { mkdtemp, mkdir, writeFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { once } from 'node:events';
import { describe, expect, it } from 'vitest';
import chokidar from 'chokidar';
import { createGeneratedWatchIgnore } from '../../../scripts/dev-watch-ignore.mjs';

describe('开发监听排除历史生成目录', () => {
  it('排除目录入口及后代，保留源码、配置和当前生成模板', () => {
    const root = join(tmpdir(), 'powerx-watch-root');
    const ignored = createGeneratedWatchIgnore(root);
    for (const path of ['.output', '.output-proof/public/asset.js', '.nuxt-old', '.nuxt-build/dist/client.js', '.vitest-cache/hash.js', 'test-results/run/trace.zip', 'playwright-report/index.html', 'coverage/report.json']) {
      expect(ignored(join(root, path)), path).toBe(true);
    }
    for (const path of ['app/pages/index.vue', 'app/pages/coverage/index.vue', 'app/components/test-results/index.vue', 'server/api/test.get.ts', 'i18n/locales/zh.json', 'nuxt.config.ts', '.nuxtignore', '.nuxt', '.nuxt/routes.mjs']) {
      expect(ignored(join(root, path)), path).toBe(false);
    }
    expect(ignored(join(root, '..', 'external-package', 'index.ts'))).toBe(false);
  });

  it('自定义构建目录仍可热更新，普通开发目录则被忽略', () => {
    const root = join(tmpdir(), 'powerx-watch-root');
    const ignored = createGeneratedWatchIgnore(root, '.nuxt-proof');
    expect(ignored(join(root, '.nuxt-proof'))).toBe(false);
    expect(ignored(join(root, '.nuxt-proof', 'routes.mjs'))).toBe(false);
    expect(ignored(join(root, '.nuxt', 'routes.mjs'))).toBe(true);
    expect(ignored(join(root, '.nuxt-other', 'routes.mjs'))).toBe(true);
  });

  it('真实 watcher 跳过产物目录，仍收到源码和当前模板变更', async () => {
    const root = await mkdtemp(join(tmpdir(), 'powerx-watch-fixture-'));
    const paths = ['app/component.vue', '.nuxt-proof/routes.mjs', '.nuxt-old/routes.mjs', '.output-proof/public/asset.js', 'test-results/run/trace.zip'];
    try {
      // 先创建 fixture，再新建 watcher，避免用异步目录发现顺序冒充监听结果。
      for (const path of paths) {
        await mkdir(join(root, path, '..'), { recursive: true });
        await writeFile(join(root, path), 'before');
      }
      const active = chokidar.watch(root, { ignored: createGeneratedWatchIgnore(root, '.nuxt-proof'), ignoreInitial: true });
      try {
        await once(active, 'ready');
        const watched = Object.keys(active.getWatched());
        expect(watched.some(path => path.includes('.output-proof') || path.includes('.nuxt-old') || path.includes('test-results'))).toBe(false);
        expect(watched).toContain(join(root, 'app'));
        expect(watched).toContain(join(root, '.nuxt-proof'));
        for (const file of paths.slice(0, 2)) {
          const event = once(active, 'change');
          await writeFile(join(root, file), 'after');
          expect((await event)[0]).toBe(join(root, file));
        }
      } finally { await active.close(); }
    } finally { await rm(root, { recursive: true, force: true }); }
  });
});
