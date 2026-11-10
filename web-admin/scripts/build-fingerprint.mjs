import { createHash } from 'node:crypto';
import { readdir, readFile, mkdir, writeFile, lstat } from 'node:fs/promises';
import { resolve, relative, join } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const canonical = value => value === null || typeof value !== 'object' ? JSON.stringify(value) : Array.isArray(value) ? `[${value.map(canonical).join(',')}]` : `{${Object.keys(value).sort().map(key => `${JSON.stringify(key)}:${canonical(value[key])}`).join(',')}}`;
// 使用固定类型载荷；JCS 属性为 UTF-16 排序，制品数组为 UTF-8 排序。
export function digest(manifest) {
  const { schema, product, component, release_id, source_commit, target, artifacts } = manifest;
  const sorted = [...artifacts].sort((a, b) => Buffer.compare(Buffer.from(a.kind), Buffer.from(b.kind)) || Buffer.compare(Buffer.from(a.path), Buffer.from(b.path)));
  return createHash('sha256').update(`powerx.build-fingerprint/v1\n${canonical({ schema, product, component, release_id, source_commit, target, artifacts: sorted })}`).digest('hex');
}

export async function generate(root, release, commit, builtAt, clientManifestPath) {
  if (!release || [...release].length > 128 || !commit || [...commit].length > 128 || !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{3})?Z$/.test(builtAt) || !Number.isFinite(Date.parse(builtAt))) throw new Error('构建版本、提交或 UTC 时间无效');
  const paths = [];
  async function walk(dir) {
    for (const name of await readdir(dir)) {
      const full = join(dir, name); const stat = await lstat(full);
      if (stat.isSymbolicLink()) throw new Error('构建资源不能使用符号链接');
      if (stat.isDirectory()) await walk(full);
      else if (/\.(js|css)$/.test(name)) paths.push(full);
    }
  }
  await walk(join(root, '_nuxt'));
  paths.sort((a, b) => Buffer.compare(Buffer.from(a), Buffer.from(b)));
  if (clientManifestPath) {
    const { default: clientManifest } = await import(pathToFileURL(resolve(clientManifestPath)).href);
    const priority = Object.values(clientManifest).filter(entry => entry.isEntry).flatMap(entry => [entry.file, ...(entry.css || [])]).map(file => join(root, '_nuxt', file));
    paths.sort((a, b) => {
      const ai = priority.indexOf(a), bi = priority.indexOf(b);
      return (ai < 0 ? 100 : ai) - (bi < 0 ? 100 : bi) || Buffer.compare(Buffer.from(a), Buffer.from(b));
    });
  }
  if (!paths.length) throw new Error('未找到最终 JS/CSS 产物');
  // 与识别器的资源预算一致；选择公开、可重复读取的最终资源，不包含 HTML 或清单。
  const artifacts = [];
  for (const file of paths.filter(file => !/[%?#:\\]/.test(relative(root, file))).slice(0, 4)) {
    const bytes = await readFile(file);
    if (bytes.length > 2 * 1024 * 1024) throw new Error('指纹资源超过市场单资源预算');
    artifacts.push({ kind: 'public_asset', path: relative(root, file).split('\\').join('/'), sha256: createHash('sha256').update(bytes).digest('hex'), size: bytes.length });
  }
  const manifest = { schema: 'powerx.build-fingerprint/v1', product: 'powerx', component: 'web-admin', release_id: release, source_commit: commit, target: { os: 'any', arch: 'any' }, artifacts, built_at: builtAt };
  manifest.build_id = digest(manifest);
  const out = join(root, '.well-known'); await mkdir(out, { recursive: true });
  await writeFile(join(out, 'powerx.json'), `${JSON.stringify(manifest, null, 2)}\n`);
  return manifest;
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  await generate(resolve(process.env.POWERX_FINGERPRINT_PUBLIC_DIR || '.output/public'), process.env.POWERX_VERSION || 'development', process.env.POWERX_SOURCE_SHA || 'unknown', process.env.POWERX_BUILD_TIME || new Date().toISOString(), resolve(process.env.POWERX_NUXT_BUILD_DIR || '.nuxt-build', 'dist/server/client.manifest.mjs'));
}
