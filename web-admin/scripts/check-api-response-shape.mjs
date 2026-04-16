#!/usr/bin/env node
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join, relative } from 'node:path';

const root = process.cwd();
const appRoot = join(root, 'app');

const exts = new Set(['.ts', '.tsx', '.js', '.jsx', '.vue']);
const unwrapAllowList = new Set([
  'app/composables/agent/useDualChannelConnection.ts',
]);
const pageApiAllowList = new Set([
  // 如需例外可在此登记
]);

function walk(dir, out = []) {
  for (const name of readdirSync(dir)) {
    if (name === '.nuxt' || name === 'node_modules' || name.startsWith('.')) continue;
    const full = join(dir, name);
    const st = statSync(full);
    if (st.isDirectory()) {
      walk(full, out);
      continue;
    }
    const lower = name.toLowerCase();
    const ok = Array.from(exts).some((ext) => lower.endsWith(ext));
    if (ok) out.push(full);
  }
  return out;
}

const files = walk(appRoot);
const badUnwrap = [];
const badPageApi = [];

const unwrapPattern = /\b(?:\w+)\?*\.data\?*\.data\b/g;
const pageImportPattern = /useApiClient\s*\}/;
const pageCallPattern = /useApiClient\s*\(/;

for (const file of files) {
  const rel = relative(root, file).replace(/\\/g, '/');
  const content = readFileSync(file, 'utf8');
  const lines = content.split(/\r?\n/);

  if (!unwrapAllowList.has(rel)) {
    for (let i = 0; i < lines.length; i++) {
      if (unwrapPattern.test(lines[i])) {
        badUnwrap.push(`${rel}:${i + 1}: ${lines[i].trim()}`);
      }
      unwrapPattern.lastIndex = 0;
    }
  }

  if (rel.startsWith('app/pages/settings/') && !pageApiAllowList.has(rel)) {
    for (let i = 0; i < lines.length; i++) {
      const line = lines[i];
      if (pageImportPattern.test(line) || pageCallPattern.test(line)) {
        badPageApi.push(`${rel}:${i + 1}: ${line.trim()}`);
      }
      pageImportPattern.lastIndex = 0;
      pageCallPattern.lastIndex = 0;
    }
  }
}

let failed = false;
if (badUnwrap.length > 0) {
  failed = true;
  console.error('❌ 检测到疑似错误的 API 响应解包写法（.data.data）：');
  for (const line of badUnwrap) console.error(`- ${line}`);
  console.error('\n请统一使用 useApiClient 返回的 ApiResponse 结构：res.data');
}

if (badPageApi.length > 0) {
  failed = true;
  console.error('\n❌ 检测到 pages 层直接使用 useApiClient：');
  for (const line of badPageApi) console.error(`- ${line}`);
  console.error('\n请在 services 层封装请求，pages 层只调用 service。');
}

if (failed) process.exit(1);
console.info('✅ API 响应解包与页面调用边界检查通过');
