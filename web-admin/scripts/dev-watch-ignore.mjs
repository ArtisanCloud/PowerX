import { relative, resolve, sep } from 'node:path';

// Nuxt 源码扫描不应把历史构建、验收输出或测试报告当成应用源码。
export const generatedSourceIgnores = [
  '**/.nuxt/**', '**/.nuxt-*/**', '**/.nuxt_*/**', '**/.nuxt.*/**',
  '**/.output/**', '**/.output-*/**', '**/.output_*/**', '**/.output.*/**',
  '.vitest-cache/**', 'test-results/**', 'playwright-report/**', 'coverage/**',
];

const generatedDirectory = /^\.(?:nuxt|output)(?:$|[-._])/;
const rootReports = new Set(['.vitest-cache', 'test-results', 'playwright-report', 'coverage']);

// Vite／Nitro 仍须监听本次构建目录里的模板；只排除其他生成目录。
// 同时匹配目录本身，确保 chokidar 在遍历之前剪枝，而非打开所有文件后再忽略事件。
export function createGeneratedWatchIgnore(rootDir, buildDir = '.nuxt') {
  const root = resolve(rootDir);
  const active = resolve(root, buildDir);
  const inside = (path, parent) => path === parent || path.startsWith(parent + sep);
  return (filename) => {
    const path = resolve(root, filename);
    if (!inside(path, root)) return false;
    if (inside(path, active) || inside(active, path)) return false;
    const parts = relative(root, path).split(sep);
    return rootReports.has(parts[0]) || parts.some(part => generatedDirectory.test(part));
  };
}
