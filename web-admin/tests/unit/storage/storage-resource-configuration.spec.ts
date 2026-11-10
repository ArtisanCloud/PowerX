import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { flushPromises, mount } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import zh from '../../../i18n/locales/zh.json';
import Backup from '~/components/monitor/storage/StorageBackupConfiguration.vue';
import Archive from '~/components/ops/backup/ArchiveGovernanceBody.vue';
import Owner from '~/components/monitor/storage/StorageResourceConfiguration.vue';
import { catalog } from './fixtures';
vi.mock('~/composables/useBackupEvents', () => import('../../support/backup-events-stub'));
const api = vi.hoisted(() => ({ backup: { listPolicies: vi.fn(), createPolicy: vi.fn(), updatePolicy: vi.fn(), enablePolicy: vi.fn(), disablePolicy: vi.fn(), setCurrentPolicy: vi.fn() }, snapshot: { overview: vi.fn(), savePolicy: vi.fn(), setPolicyEnabled: vi.fn(), setDefaultPolicy: vi.fn() }, archive: { overview: vi.fn(), catalog: vi.fn(), saveTarget: vi.fn(), targetRequest:vi.fn(), savePolicy: vi.fn() } }));
vi.mock('~/composables/api/services/backupOpsService', () => ({ useBackupOpsService: () => api.backup }));
vi.mock('~/composables/api/services/systemSnapshotService', () => ({ useSystemSnapshotService: () => api.snapshot }));
vi.mock('~/composables/api/services/archiveOpsService', () => ({ useArchiveOpsService: () => api.archive }));
const stubs = {
 DatabaseBackupPolicies: true,SystemSnapshotPolicies:true,
  UAlert: { props: ['title', 'description'], template: '<div>{{title}} {{description}}<slot name="actions" /></div>' }, UBadge: { template: '<span><slot/></span>' },
  UButton: { props: ['disabled', 'loading'], template: '<button :disabled="disabled || loading"><slot/></button>' },
  UModal: { props: ['open', 'title'], emits: ['update:open'], template: '<section v-if="open"><h3>{{title}}</h3><button data-close @click="$emit(\'update:open\', false)">Close</button><slot name="body"/><slot name="footer"/></section>' },
  UInput: { props: ['modelValue', 'disabled'], emits: ['update:modelValue'], template: '<input :disabled="disabled" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />' },
  USelect: { props: ['modelValue', 'items', 'disabled'], emits: ['update:modelValue'], template: '<select :value="modelValue" :disabled="disabled" @change="$emit(\'update:modelValue\', $event.target.value)"><option v-for="item in items" :value="typeof item === \'string\' ? item : item.value">{{typeof item === \'string\' ? item : item.label}}</option></select>' },
  UCheckbox: { props: ['modelValue', 'label', 'disabled'], emits: ['update:modelValue'], template: '<label><input type="checkbox" :disabled="disabled" :checked="modelValue" @change="$emit(\'update:modelValue\', $event.target.checked)"/>{{label}}</label>' },
  UFormField: { props: ['label'], template: '<label>{{label}}<slot/></label>' },
  UForm: { emits: ['submit'], template: '<form @submit.prevent="$emit(\'submit\')"><slot/></form>' },
};
const wrappers: ReturnType<typeof mount>[] = [];
const clone = (value: any) => JSON.parse(JSON.stringify(value));
function view(component: any, props: any) { const w = mount(component, { props, global: { stubs, plugins: [createI18n({ legacy: false, locale: 'zh', messages: { zh } })] } }); wrappers.push(w); return w; }
const button = (w: any, name: string) => w.findAll('button').find((b: any) => b.text() === name)!;
let backups: any[], snapshots: any[], archive: any;
beforeEach(() => {
  vi.resetAllMocks();
  backups = [{ id: '1', name: '数据库备份', schedule: '24h', interval_hours: 24, retention_count: 3, retention_days: 0, retention_mode: 'count', enabled: false, is_current: true, timezone: 'Asia/Shanghai' }];
  snapshots = [{ id: 's1', name: '系统快照', interval_minutes: 60, retention_count: 5, retention_days: 0, enabled: false, is_default: true }];
  api.backup.listPolicies.mockImplementation(async () => ({ items: clone(backups), total: backups.length, page: 1, pageSize: 100 }));
  api.backup.updatePolicy.mockImplementation(async (id, input) => { const row = backups.find(r => r.id === id); Object.assign(row, input, { schedule: input.interval_value + ({ minute: 'm', hour: 'h', day: 'd' } as Record<string, string>)[input.interval_unit] }); return clone(row); });
  api.snapshot.overview.mockImplementation(async () => ({ policies: clone(snapshots), problems: [], ready: true }));
  api.snapshot.savePolicy.mockImplementation(async (input, id) => { if (id) { const row = snapshots.find(r => r.id === id); Object.assign(row, input); return clone(row); } const row = { ...input, id: 'created', is_default: false }; snapshots.push(row); return clone(row); });
  archive = { policy_configuration:{authority:'database',ready:true,state:'applied',desired_revision:1,applied_revision:1,snapshot_revision:1,policies:[]},target_configuration:{authority:'database',ready:true,state:'applied',desired_revision:1,applied_revision:1,snapshot_revision:1,targets:[]}, instance: 'local', sources: [{ id: 'logs', name: '应用日志', kind: 'application_logs' }], targets: [{ id: 'target', name: '私有归档', revision: 1, driver: 's3', endpoint: 'https://archive.invalid', bucket: 'private', prefix: 'host/local', region: 'test-region', credential_ref: 'private-role', force_path_style: true, verification_valid: false }], policies: [{ id: 'policy', name: '应用归档', revision: 2, source_id: 'logs', target_id: 'target', interval_minutes: 5, retention_days: 7, retention_count: 168, remote_retention_days: 180, remote_retention_count: 10000, remote_max_bytes: 1024 ** 3, enabled: false, remote_cleanup_enabled: false, include_database: false }], jobs: [], restores: [], credentials: [], s3_credentials: ['private-role'], pending_transfers: 0 };
  api.archive.overview.mockImplementation(async () => clone(archive));
  api.archive.saveTarget.mockImplementation(async input => { const result = { ...input.settings, id:input.target_id||input.request_id, revision: input.expected_target_revision + 1, verification_valid: false }; archive.targets = [result]; archive.target_configuration.desired_revision++;return {request_id:input.request_id,operation_revision:input.expected_revision+1,target_id:result.id,configuration:clone(archive.target_configuration)}; });
  api.archive.savePolicy.mockImplementation(async input => { const result={...input.settings,id:input.policy_id||input.request_id,revision:input.expected_policy_revision+1,enabled:false,remote_cleanup_enabled:false};archive.policies=[result];archive.policy_configuration.desired_revision++;return {request_id:input.request_id,operation_revision:archive.policy_configuration.desired_revision,policy_id:result.id,configuration:clone(archive.policy_configuration)}; });
});
afterEach(() => { wrappers.splice(0).forEach(w => w.unmount()); vi.useRealTimers(); });
async function editBackup(w: any, count: string) { await flushPromises(); await w.get('[data-testid="resource-backup-edit"]').trigger('click'); await w.get('[data-testid="resource-backup-count"]').setValue(count); await w.get('[data-testid="resource-backup-confirm"] input').setValue(true); }
async function saveBackup(w: any) { await w.get('[data-testid="resource-backup-form"]').trigger('submit'); await flushPromises(); }
describe('resource-local backup and snapshot settings', () => {
  it('routes both resources to their shared authority editors without legacy calls', async () => {
    const backup=view(Backup,{kind:'backup'}),snapshot=view(Backup,{kind:'snapshot'});await flushPromises();expect(backup.findComponent({name:'DatabaseBackupPolicies'}).exists()).toBe(true);expect(snapshot.findComponent({name:'SystemSnapshotPolicies'}).exists()).toBe(true);expect(api.snapshot.savePolicy).not.toHaveBeenCalled();expect(api.backup.updatePolicy).not.toHaveBeenCalled();
  });
});
describe('resource-local archive configuration', () => {
  it('shows saved target and policy without listing artifacts or sending data, edits locally and verifies the saved target', async () => {
    const w = view(Archive, { configurationOnly: true }); await flushPromises(); expect(w.get('[data-testid="resource-archive-settings"]').text()).toContain('https://archive.invalid'); expect(api.archive.catalog).not.toHaveBeenCalled(); expect(api.archive.savePolicy).not.toHaveBeenCalled();
    await w.get('[data-testid="resource-archive-target-edit"]').trigger('click'); await w.get('#s3-archive-target').findAll('input')[3]!.setValue('host/changed'); await w.get('#s3-archive-target').trigger('submit'); await flushPromises();
    expect(api.archive.saveTarget).toHaveBeenCalledWith(expect.objectContaining({ target_id: 'target', expected_revision:1, expected_target_revision: 1, settings:expect.objectContaining({prefix: 'host/changed'}) }), expect.any(AbortSignal)); expect(w.text()).toContain('配置已存入数据库'); expect(w.text()).toContain('host/changed'); expect(api.archive.savePolicy).not.toHaveBeenCalled();
  });
  it('keeps archive drafts across closing and reopening, and saving a draft does not enable transmission or remote cleanup', async () => {
    const w = view(Archive, { configurationOnly: true }); await flushPromises(); await w.get('[data-testid="resource-archive-policy-edit"]').trigger('click'); await w.get('#log-archive-policy input').setValue('修改后的名称'); await w.get('[data-close]').trigger('click'); await w.get('[data-testid="resource-archive-policy-edit"]').trigger('click'); expect((w.get('#log-archive-policy input').element as HTMLInputElement).value).toBe('修改后的名称');
    await w.get('#log-archive-policy').trigger('submit'); await flushPromises(); expect(api.archive.savePolicy.mock.calls[0]?.[0]).toMatchObject({expected_policy_revision:2,action:'save'});expect(api.archive.savePolicy.mock.calls[0]?.[0].enabled).toBeUndefined();expect(w.text()).toContain('策略已存入数据库');
  });
  it('blocks replay of an uncertain target creation and checks its original result using only reads', async () => {
    archive.targets = []; archive.policies = []; const w = view(Archive, { configurationOnly: true }); await flushPromises(); await w.get('[data-testid="resource-archive-target-edit"]').trigger('click');
    const fields = w.get('#s3-archive-target').findAll('input'); for (const [index, text] of ['新增目标', 'https://archive.invalid', 'private', 'host/new', 'test-region'].entries()) await fields[index]!.setValue(text);
    await w.get('#s3-archive-target select').setValue('private-role');
    api.archive.saveTarget.mockImplementationOnce(async input => { const row = { ...input.settings, id: input.request_id, revision: 1 }; archive.targets = [row];api.archive.targetRequest.mockResolvedValue({request_id:input.request_id,operation_revision:2,target_id:input.request_id,configuration:clone(archive.target_configuration)}); throw new Error('response lost'); });
    await w.get('#s3-archive-target').trigger('submit'); await flushPromises(); expect(w.text()).toContain('保存结果尚未确认'); await w.get('#s3-archive-target').trigger('submit'); await flushPromises(); expect(api.archive.saveTarget).toHaveBeenCalledTimes(1);
    api.archive.overview.mockImplementation(async () => clone(archive)); await w.get('[data-testid="target-reconcile"]').trigger('click'); await flushPromises(); expect(api.archive.saveTarget).toHaveBeenCalledTimes(1); expect(api.archive.targetRequest).toHaveBeenCalledTimes(1);expect(w.text()).toContain('配置已存入数据库');
  });
});
describe('installation ownership', () => {
  it('does not bind another installation’s source to the local configuration API', async () => {
    const w = view(Owner, { source: catalog()[1], instanceId: 'dev-install' }); await flushPromises(); expect(w.text()).toContain('此资源属于其他安装实例'); expect(api.backup.listPolicies).not.toHaveBeenCalled(); expect(api.snapshot.overview).not.toHaveBeenCalled(); expect(api.archive.overview).not.toHaveBeenCalled();
  });
});
