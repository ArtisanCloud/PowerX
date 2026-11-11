import { describe, expect, it } from 'vitest';
import { canResumeInstall, installPackageIdentity, readLocalInstallIdentity, reconcileInstall, type InstallRecovery, type InstallInspection } from '~/utils/plugin-install-recovery';

const target = { id: 'com.powerx.plugins.fixture', version: '1.0.0' };
const state = (): InstallRecovery => ({ target, enable: true, force: true, baselineRevision: 'old', outcome: 'unknown' });
const inspection = (change: Partial<InstallInspection['installation']> = {}): InstallInspection => ({ plugin_id: target.id, installation: { version: target.version, revision: 'new', ready: true, tenant_ready: true, runtime_ready: true, ...change } });

describe('安装写入结果核对', () => {
  it('不能把 registry enabled 或已有同版本当作本次安装成功', () => {
    expect(reconcileInstall(state(), inspection({ tenant_ready: false })).outcome).toBe('pending');
    expect(reconcileInstall(state(), inspection({ runtime_ready: false })).outcome).toBe('pending');
    expect(reconcileInstall(state(), inspection({ revision: 'old' })).outcome).toBe('unknown');
    expect(reconcileInstall(state(), inspection()).outcome).toBe('ready');
  });
  it('版本、回执修订及插件身份不匹配时不得恢复或报成功', () => {
    expect(reconcileInstall(state(), inspection({ version: '2.0.0' })).outcome).toBe('changed');
    expect(reconcileInstall({ ...state(), receiptRevision: 'expected' }, inspection()).outcome).toBe('changed');
    expect(reconcileInstall(state(), { ...inspection(), plugin_id: 'other' }).outcome).toBe('unknown');
  });
  it('缺少记录、旧版不完整回包都不能直接恢复', () => {
    const missing = reconcileInstall(state(), { plugin_id: target.id, installation: { ready: false, reason: 'PLUGIN_NOT_INSTALLED' } });
    expect(missing.outcome).toBe('not_installed'); expect(canResumeInstall(missing)).toBe(false);
    expect(reconcileInstall(state(), inspection({ revision: undefined })).outcome).toBe('unknown');
  });
  it('只有已核对目标版本与修订的待完成安装才允许显式恢复', () => {
    const pending = reconcileInstall(state(), inspection({ ready: false, tenant_ready: false }));
    expect(canResumeInstall(pending)).toBe(true);
    expect(canResumeInstall(state())).toBe(false);
    expect(canResumeInstall({ ...pending, enable: false })).toBe(false);
    expect(canResumeInstall({ ...pending, outcome: 'changed' })).toBe(false);
  });
  it('安装但不启用时只确认安装身份，不伪报运行就绪', () => {
    expect(reconcileInstall({ ...state(), enable: false }, inspection({ ready: false, tenant_ready: false, runtime_ready: false })).outcome).toBe('ready');
  });
});

describe('本地安装身份只读识别', () => {
  const file = (path: string, text: string) => ({ name: 'plugin.yaml', webkitRelativePath: path, size: text.length, text: async () => text } as File);
  it('读取根清单，不误选内嵌文档示例', async () => {
    expect(await readLocalInstallIdentity([file('package/docs/demo/plugin.yaml', 'id: other\nversion: 2'), file('package/plugin.yaml', `id: '${target.id}' # comment\nversion: "1.0.0"\n`)])).toEqual(target);
  });
  it('拒绝歧义清单、路径标识以及非字面量版本', async () => {
    expect(await readLocalInstallIdentity([file('a/plugin.yaml', 'id: a\nversion: 1'), file('b/plugin.yaml', 'id: b\nversion: 1')])).toBeUndefined();
    expect(installPackageIdentity('../unsafe', '1')).toBeUndefined();
    expect(await readLocalInstallIdentity([file('package/plugin.yaml', 'id: fixture\nversion: *shared')])).toBeUndefined();
  });
});
