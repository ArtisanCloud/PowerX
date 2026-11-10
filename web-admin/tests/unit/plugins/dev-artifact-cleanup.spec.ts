import { mkdtemp, mkdir, writeFile, readFile, readdir, lstat, utimes, rm, symlink, rename, realpath } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { execFile, spawn } from 'node:child_process';
import { promisify } from 'node:util';
import { once } from 'node:events';
import { afterEach, describe, expect, it } from 'vitest';
import { applyCleanup, inventoryArtifacts, planCleanup, validatePolicy, inspectUsage } from '../../../scripts/cleanup-dev-artifacts.mjs';

const roots: string[] = [];
const DAY = 86_400_000;
const policy = { keepGroups: 3, minAgeDays: 7 };
const idle = () => ({ available: true, busyProject: false, openDirectories: new Set<string>(), trackedDirectories: new Set<string>(), configuredPaths: [] as string[] });
async function fixture() { const root = await realpath(await mkdtemp(join(tmpdir(), 'powerx-artifact-cleanup-'))); roots.push(root); return root; }
async function ageTree(path: string, ageDays: number) {
  const stat = await lstat(path);
  if (stat.isDirectory()) for (const file of await readdir(path)) await ageTree(join(path, file), ageDays);
  const at = new Date(Date.now() - ageDays * DAY);
  await utimes(path, at, at);
}
async function group(root: string, id: string, age: number) {
  for (const kind of ['nuxt', 'output']) {
    const path = join(root, `.${kind}-${id}`);
    await mkdir(path, { recursive: true });
    for (const marker of kind === 'nuxt' ? ['nuxt.d.ts', 'tsconfig.json'] : ['nitro.json']) await writeFile(join(path, marker), 'fixture');
    await ageTree(path, age);
  }
}
async function oldGroups(root: string) { for (let i = 0; i < 5; i++) await group(root, `case-${i}-validation`, 20 - i); }
afterEach(async () => { for (const root of roots.splice(0)) await rm(root, { recursive: true, force: true }); });

describe('开发验证产物自动清理', () => {
  it('真实空闲检查可放行，并识别已加载验证包的运行包装脚本', async () => {
    // 放在当前 Git 仓库的忽略目录内，不创建或切换任何 Git 分支。
    const cache = resolve('node_modules/.cache');
    await mkdir(cache, { recursive: true });
    const root = await realpath(await mkdtemp(join(cache, 'powerx-artifact-usage-'))); roots.push(root);
    await mkdir(join(root, 'scripts'));
    const id = `usage-${process.pid}-validation`;
    await group(root, id, 30);
    const groups = await inventoryArtifacts(root);
    const idleUsage = await inspectUsage(root, groups, {});
    expect(idleUsage.available).toBe(true);
    expect(idleUsage.busyProject).toBe(false);
    expect(idleUsage.openDirectories.size).toBe(0);
    const runner = join(root, 'scripts', 'retained-package-runner.mjs');
    await writeFile(runner, `const retainedOutput = ${JSON.stringify('.output-' + id)}; console.log('ready'); setTimeout(() => {}, 60000);`);
    const child = spawn(process.execPath, [runner], { cwd: root, stdio: ['ignore', 'pipe', 'ignore'] });
    const exited = once(child, 'exit');
    try {
      await once(child.stdout!, 'data');
      const busy = await inspectUsage(root, groups, {});
      expect(busy.available).toBe(true);
      expect(busy.busyProject).toBe(true);
      expect(busy.openDirectories.has(`.output-${id}`)).toBe(true);
    } finally { child.kill('SIGTERM'); await exited; }
  }, 15000);
  it('通过符号链接启动 CLI 也能运行，帮助命令不触碰项目产物', async () => {
    const root = await fixture();
    const link = join(root, 'cleanup-link.mjs');
    await symlink(resolve('scripts/cleanup-dev-artifacts.mjs'), link);
    const output = await promisify(execFile)(process.execPath, [link, '--help']);
    expect(output.stdout).toContain('artifacts:preview');
    expect(output.stdout).toContain('artifacts:clean');
  });
  it('按 nuxt/output 配对，实际删除最旧两组，保留最近三组及正式／恢复目录', async () => {
    const root = await fixture(); await oldGroups(root);
    for (const name of ['.nuxt', '.output', '.nuxt-build', '.nuxt-recovery', '.output-recovery']) {
      await mkdir(join(root, name)); await writeFile(join(root, name, 'must-stay'), 'protected');
    }
    const plan = planCleanup(await inventoryArtifacts(root), policy, idle());
    expect(plan.filter(group => group.action === 'remove').map(group => group.id).sort()).toEqual(['case-0-validation', 'case-1-validation']);
    const result = await applyCleanup(root, plan, policy, async () => idle());
    expect(result.removed).toHaveLength(2);
    expect(result.removed.every(group => group.directories.length === 2)).toBe(true);
    const remaining = await readdir(root);
    expect(remaining).not.toContain('.nuxt-case-0-validation');
    expect(remaining).toContain('.output-case-4-validation');
    for (const name of ['.nuxt', '.output', '.nuxt-build', '.nuxt-recovery', '.output-recovery']) expect(await readFile(join(root, name, 'must-stay'), 'utf8')).toBe('protected');
  });

  it('即使超过三组，七天内或子文件刚更新的产物仍然保留', async () => {
    const root = await fixture();
    for (let i = 0; i < 5; i++) await group(root, `new-${i}-validation`, 1 + i * 0.1);
    await group(root, 'old-validation', 30);
    await writeFile(join(root, '.nuxt-old-validation', 'fresh-file'), 'recent build');
    const plan = planCleanup(await inventoryArtifacts(root), policy, idle());
    expect(plan.every(group => group.action === 'keep')).toBe(true);
  });

  it('占用、跟踪文件、已配置路径和核验失败均阻止删除', async () => {
    const root = await fixture(); await oldGroups(root);
    const groups = await inventoryArtifacts(root);
    for (const change of [
      { available: false }, { busyProject: true },
      { openDirectories: new Set(['.output-case-0-validation']) },
      { trackedDirectories: new Set(['.nuxt-case-0-validation']) },
      { configuredPaths: [join(root, '.nuxt-case-0-validation')] },
    ]) {
      const plan = planCleanup(groups, policy, { ...idle(), ...change });
      expect(plan.find(group => group.id === 'case-0-validation')?.action).toBe('keep');
    }
    const plan = planCleanup(groups, policy, idle());
    const result = await applyCleanup(root, plan, policy, async () => ({ ...idle(), busyProject: true }));
    expect(result.removed).toHaveLength(0);
    expect(await readdir(root)).toHaveLength(10);
  });

  it('不删除伪装目录或符号链接指向的项目外文件', async () => {
    const root = await fixture(), outside = await fixture();
    await writeFile(join(outside, 'business-data'), 'must survive');
    await symlink(outside, join(root, '.nuxt-linked-validation'));
    await mkdir(join(root, '.nuxt-user-validation'));
    await writeFile(join(root, '.nuxt-user-validation', 'notes'), 'not generated');
    const plan = planCleanup(await inventoryArtifacts(root), policy, idle());
    expect(plan.every(group => group.action === 'keep')).toBe(true);
    await applyCleanup(root, plan, policy, async () => idle());
    expect(await readFile(join(outside, 'business-data'), 'utf8')).toBe('must survive');
    expect(await readFile(join(root, '.nuxt-user-validation', 'notes'), 'utf8')).toBe('not generated');
  });

  it('预览后被替换的目录不按旧计划删除', async () => {
    const root = await fixture(); await oldGroups(root);
    const plan = planCleanup(await inventoryArtifacts(root), policy, idle());
    const old = join(root, '.nuxt-case-0-validation');
    await rename(old, join(root, 'saved-outside-plan'));
    await group(root, 'case-0-validation', 30);
    const result = await applyCleanup(root, plan, policy, async () => idle());
    expect(result.skipped).toContain('case-0-validation');
    expect(await readdir(root)).toContain('.nuxt-case-0-validation');
  });

  it('拒绝零值、负数和字符串保留规则', () => {
    for (const value of [{ keepGroups: 0, minAgeDays: 7 }, { keepGroups: 3, minAgeDays: -1 }, { keepGroups: '3', minAgeDays: 7 }]) expect(() => validatePolicy(value)).toThrow();
  });
});
