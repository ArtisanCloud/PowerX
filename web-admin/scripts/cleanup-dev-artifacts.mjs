import { readdir, lstat, realpath, readFile, rm, mkdir, open, unlink } from 'node:fs/promises';
import { resolve, relative, dirname, basename, sep } from 'node:path';
import { fileURLToPath } from 'node:url';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';

const exec = promisify(execFile);
const DAY = 86_400_000;
const within = (path, parent) => path === parent || path.startsWith(parent + sep);
const generatedName = /^\.(nuxt|output)-(.+)$/;
const managedSuffix = /(?:^|-)validation(?:$|-)|(?:^|-)\d{8}(?:$|-)/;

async function canonicalPath(path) {
  const suffix = [];
  let parent = resolve(path);
  while (true) {
    try { return resolve(await realpath(parent), ...suffix.reverse()); }
    catch (error) {
      if (error.code !== 'ENOENT' || dirname(parent) === parent) throw error;
      suffix.push(basename(parent)); parent = dirname(parent);
    }
  }
}

export function validatePolicy(value) {
  if (!value || !Number.isInteger(value.keepGroups) || value.keepGroups < 1 || !Number.isInteger(value.minAgeDays) || value.minAgeDays < 1) {
    throw new Error('清理规则无效：keepGroups、minAgeDays 必须为正整数。');
  }
  return { keepGroups: value.keepGroups, minAgeDays: value.minAgeDays };
}

async function regularFile(path) {
  try { return (await lstat(path)).isFile(); } catch { return false; }
}

// 不跟随符号链接；任何读取失败或跨文件系统挂载都使整组停止自动清理。
async function scanTree(path, device) {
  const stat = await lstat(path);
  if (stat.dev !== device) throw new Error('CROSS_DEVICE_TREE');
  let bytes = stat.isFile() || stat.isSymbolicLink() ? stat.size : 0;
  let newest = stat.mtimeMs;
  if (stat.isDirectory() && !stat.isSymbolicLink()) {
    for (const entry of await readdir(path)) {
      const child = await scanTree(resolve(path, entry), device);
      bytes += child.bytes; newest = Math.max(newest, child.newest);
    }
  }
  return { bytes, newest };
}

export async function inventoryArtifacts(rootDir) {
  const root = await realpath(rootDir);
  const groups = new Map();
  for (const entry of await readdir(root, { withFileTypes: true })) {
    const match = generatedName.exec(entry.name);
    if (!match) continue;
    const [, kind, id] = match;
    const group = groups.get(id) || { id, directories: [], newest: 0, logicalBytes: 0, protected: [] };
    groups.set(id, group);
    const path = resolve(root, entry.name), stat = await lstat(path);
    const directory = { name: entry.name, path, dev: stat.dev, ino: stat.ino };
    group.directories.push(directory);
    if (/recovery/i.test(id) || id === 'build') group.protected.push('RESERVED_BUILD_OR_RECOVERY');
    if (!managedSuffix.test(id)) group.protected.push('NOT_MANAGED_VALIDATION_OUTPUT');
    if (!entry.isDirectory() || entry.isSymbolicLink()) { group.protected.push('NOT_A_REAL_DIRECTORY'); continue; }
    if (group.protected.length) continue;
    const markers = kind === 'nuxt' ? ['nuxt.d.ts', 'tsconfig.json'] : ['nitro.json'];
    if (!(await Promise.all(markers.map(file => regularFile(resolve(path, file))))).every(Boolean)) {
      group.protected.push('GENERATED_MARKERS_MISSING'); continue;
    }
    try {
      const tree = await scanTree(path, stat.dev);
      group.newest = Math.max(group.newest, tree.newest);
      group.logicalBytes += tree.bytes;
    } catch { group.protected.push('TREE_INSPECTION_FAILED'); }
  }
  return [...groups.values()];
}

// 原始进程参数及全局打开文件清单只在内存内检查，不输出或保存。
export async function inspectUsage(rootDir, groups, environment = process.env) {
  const root = await realpath(rootDir);
  const result = { available: false, busyProject: false, openDirectories: new Set(), trackedDirectories: new Set(), configuredPaths: [] };
  try {
    for (const key of ['POWERX_NUXT_BUILD_DIR', 'POWERX_NITRO_OUTPUT_DIR', 'POWERX_WEB_OUTPUT']) {
      if (environment[key]) {
        const path = resolve(root, environment[key]);
        result.configuredPaths.push(path, await canonicalPath(path));
      }
    }
    const [files, processes, tracked] = await Promise.all([
      exec('lsof', ['-nP', '-a', '-u', String(process.getuid()), '-Fpfn'], { timeout: 10000, maxBuffer: 64 << 20 }),
      exec('ps', ['-axo', 'pid=,ppid=,args='], { timeout: 5000, maxBuffer: 16 << 20 }),
      exec('git', ['ls-files', '-z', '--', '.'], { cwd: root, timeout: 5000, maxBuffer: 16 << 20 }),
    ]);
    const rows = new Map();
    for (const line of processes.stdout.split('\n')) {
      const match = /^\s*(\d+)\s+(\d+)\s+(.*)$/.exec(line);
      if (match) rows.set(Number(match[1]), { parent: Number(match[2]), args: match[3] });
    }
    const ownAncestors = new Set([process.pid]);
    for (let pid = process.ppid; pid && !ownAncestors.has(pid); pid = rows.get(pid)?.parent) ownAncestors.add(pid);
    const projectPIDs = new Set();
    let pid = 0, fd = '';
    for (const line of files.stdout.split('\n')) {
      if (line[0] === 'p') pid = Number(line.slice(1));
      else if (line[0] === 'f') fd = line.slice(1);
      else if (line[0] === 'n') {
        const path = line.slice(1);
        if (fd === 'cwd' && within(path, root)) projectPIDs.add(pid);
        for (const group of groups) for (const dir of group.directories) {
          if (within(path, dir.path)) result.openDirectories.add(dir.name);
        }
      }
    }
    const names = groups.flatMap(group => group.directories.map(dir => dir.name));
    const scripts = [];
    for (const entry of await readdir(resolve(root, 'scripts'), { withFileTypes: true })) {
      if (!entry.isFile() || !/\.(?:mjs|js|ts)$/.test(entry.name)) continue;
      const source = await readFile(resolve(root, 'scripts', entry.name), 'utf8');
      scripts.push({ name: entry.name, references: names.filter(name => source.includes(name)) });
    }
    for (const [id, row] of rows) {
      if (ownAncestors.has(id)) continue;
      for (const name of names) if (row.args.includes(name)) result.openDirectories.add(name);
      const executable = basename(row.args.split(/\s+/, 1)[0] || '');
      if (!/^(?:node|nodejs|npm|npx|pnpm|yarn|bun)$/.test(executable)) continue;
      // 不能仅凭参数猜测任意包装脚本是否已加载产物；其他本项目 Node 任务运行时整体保护。
      if (projectPIDs.has(id)) result.busyProject = true;
      for (const script of scripts) if (row.args.includes(script.name)) for (const name of script.references) result.openDirectories.add(name);
    }
    for (const file of tracked.stdout.split('\0')) {
      const first = file.split('/')[0];
      if (names.includes(first)) result.trackedDirectories.add(first);
    }
    result.available = true;
  } catch { /* 无法完整核验占用或 Git 状态时，保持所有目录。 */ }
  return result;
}

export function planCleanup(groups, policy, usage, now = Date.now()) {
  validatePolicy(policy);
  const managed = groups.filter(group => group.protected.length === 0).sort((a, b) => b.newest - a.newest || a.id.localeCompare(b.id));
  const recent = new Set(managed.slice(0, policy.keepGroups).map(group => group.id));
  return groups.map(group => {
    const reasons = [...group.protected];
    if (!usage.available) reasons.push('USAGE_INSPECTION_UNAVAILABLE');
    if (usage.busyProject) reasons.push('PROJECT_DEV_OR_BUILD_RUNNING');
    if (recent.has(group.id)) reasons.push('KEEP_RECENT_GROUP');
    if (now - group.newest < policy.minAgeDays * DAY) reasons.push('WITHIN_RETENTION_DAYS');
    for (const dir of group.directories) {
      if (usage.openDirectories.has(dir.name)) reasons.push('IN_USE');
      if (usage.trackedDirectories.has(dir.name)) reasons.push('CONTAINS_TRACKED_FILES');
      if (usage.configuredPaths.some(path => within(path, dir.path) || within(dir.path, path))) reasons.push('CONFIGURED_BUILD_OR_OUTPUT');
    }
    return { ...group, reasons: [...new Set(reasons)], action: reasons.length ? 'keep' : 'remove' };
  });
}

// 执行前重新检查目录身份、最近修改和进程占用；只允许删除计划内的直接子目录。
export async function applyCleanup(rootDir, plan, policy, inspect = inspectUsage, now = Date.now()) {
  const root = await realpath(rootDir), removed = [], skipped = [];
  for (const group of plan.filter(item => item.action === 'remove')) {
    const freshGroups = await inventoryArtifacts(root);
    const current = planCleanup(freshGroups, policy, await inspect(root, freshGroups), now).find(item => item.id === group.id);
    if (!current || current.action !== 'remove') { skipped.push(group.id); continue; }
    let safe = current.directories.length === group.directories.length;
    for (const dir of group.directories) {
      const currentDir = current.directories.find(value => value.name === dir.name);
      safe &&= Boolean(currentDir && dirname(dir.path) === root && currentDir.dev === dir.dev && currentDir.ino === dir.ino);
    }
    if (!safe) { skipped.push(group.id); continue; }
    for (const dir of current.directories) {
      const stat = await lstat(dir.path);
      if (!stat.isDirectory() || stat.isSymbolicLink() || stat.dev !== dir.dev || stat.ino !== dir.ino) throw new Error('目录身份已变化，停止清理。');
      await rm(dir.path, { recursive: true, force: false });
    }
    removed.push({ id: group.id, directories: group.directories.map(dir => dir.name), logicalBytes: current.logicalBytes });
  }
  return { removed, skipped };
}

async function main() {
  const args = process.argv.slice(2);
  if (args.includes('--help')) {
    console.log('预览：npm run artifacts:preview\n执行：npm run artifacts:clean\n规则：dev-artifacts.config.json；前端 npm run dev 启动前自动执行。仅管理验证构建产物，保护正式／恢复目录、Git 文件和运行中的任务。');
    return;
  }
  if (args.some(arg => !['--apply', '--json'].includes(arg))) throw new Error('不支持的参数。请使用 --help。');
  const root = await realpath(fileURLToPath(new URL('..', import.meta.url)));
  const policy = validatePolicy(JSON.parse(await readFile(resolve(root, 'dev-artifacts.config.json'), 'utf8')));
  const lockDir = resolve(root, 'node_modules/.cache/powerx-dev-artifacts');
  await mkdir(lockDir, { recursive: true });
  const lockPath = resolve(lockDir, 'cleanup.lock');
  let lock;
  try { lock = await open(lockPath, 'wx'); }
  catch (error) {
    if (error.code !== 'EEXIST') throw error;
    const stat = await lstat(lockPath);
    if (!stat.isFile() || stat.isSymbolicLink()) throw new Error('清理锁不是普通文件，请检查。');
    const owner = Number((await readFile(lockPath, 'utf8')).trim());
    let alive = true;
    if (Number.isInteger(owner) && owner > 0) {
      try { process.kill(owner, 0); } catch (failure) { if (failure.code === 'ESRCH') alive = false; }
    } else if (Date.now() - stat.mtimeMs > 60000) alive = false;
    if (alive) { console.log('[dev-artifacts] 另一次清理尚未结束，本轮不删除任何目录。'); return; }
    const current = await lstat(lockPath);
    if (current.dev !== stat.dev || current.ino !== stat.ino) throw new Error('清理锁已变化，请重试。');
    await unlink(lockPath);
    lock = await open(lockPath, 'wx');
  }
  try {
    await lock.writeFile(String(process.pid));
    const groups = await inventoryArtifacts(root);
    const usage = await inspectUsage(root, groups);
    const plan = planCleanup(groups, policy, usage);
    const result = args.includes('--apply') ? await applyCleanup(root, plan, policy) : { removed: [], skipped: [] };
    const report = { mode: args.includes('--apply') ? 'apply' : 'preview', policy, inspectionAvailable: usage.available, busyProject: usage.busyProject, groups: plan, ...result };
    if (args.includes('--json')) console.log(JSON.stringify(report, null, 2));
    else {
      const eligible = plan.filter(group => group.action === 'remove');
      console.log(`[dev-artifacts] 保留最近 ${policy.keepGroups} 组、至少 ${policy.minAgeDays} 天；可清理 ${eligible.length} 组，已清理 ${result.removed.length} 组。`);
      if (!usage.available) console.log('[dev-artifacts] 无法完整核验进程／Git 状态，已保护全部目录。');
      else if (usage.busyProject) console.log('[dev-artifacts] 检测到本项目开发、构建或验证任务仍在运行，已保护全部目录。');
      const removed = new Set(result.removed.map(group => group.id));
      const skipped = new Set(result.skipped);
      for (const group of plan) {
        if (args.includes('--apply') && !removed.has(group.id) && !skipped.has(group.id)) continue;
        const label = removed.has(group.id) ? '已清理' : skipped.has(group.id) ? '已跳过（执行前状态变化）' : group.action === 'remove' ? '可清理' : '保留';
        console.log(`  ${label} ${group.id}${group.reasons.length ? ` (${group.reasons.join(', ')})` : ''}`);
      }
    }
  } finally { await lock.close(); await unlink(lockPath); }
}

if (process.argv[1] && await realpath(process.argv[1]).catch(() => null) === await realpath(fileURLToPath(import.meta.url))) {
  main().catch(error => { console.error(`[dev-artifacts] ${error.message}`); process.exitCode = 1; });
}
