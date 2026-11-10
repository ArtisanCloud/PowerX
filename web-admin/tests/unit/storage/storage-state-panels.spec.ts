import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { flushPromises, mount } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import { defineComponent } from 'vue';
import zh from '../../../i18n/locales/zh.json';
import PolicyPanel from '~/components/monitor/storage/StoragePolicyDraftPanel.vue';
import HistoryPanel from '~/components/monitor/storage/StorageHistoryPanel.vue';
import LogPanel from '~/components/monitor/storage/StorageLogCleanupPanel.vue';
import CleanupRecords from '~/components/monitor/storage/StorageCleanupRecords.vue';
import { policy, resources, GiB } from './fixtures';
const api = vi.hoisted(() => ({ policy: vi.fn(), savePolicy: vi.fn(), history: vi.fn(), estimate: vi.fn(), previewCleanup: vi.fn(), executeCleanup: vi.fn(), cleanupRun: vi.fn(), cleanupRuns: vi.fn() }));
vi.mock('~/composables/api/services/storageGovernanceService', () => ({ useStorageGovernanceService: () => api }));
const stubs = {
  StorageResourceConfiguration: true,
  NuxtLink: { template: '<a><slot /></a>' },
  UAlert: { props: ['title', 'description'], template: '<div>{{title}} {{description}}</div>' }, UCard: { template: '<div><slot /></div>' },
  UButton: { props: ['loading', 'disabled', 'to'], template: '<button :disabled="loading || disabled"><slot /></button>' },
  UModal: { props: ['open'], emits: ['update:open'], template: '<section v-if="open"><button data-modal-close @click="$emit(\'update:open\', false)">Close</button><slot name="body" /></section>' },
  UForm: { emits: ['submit'], template: '<form @submit.prevent="$emit(\'submit\')"><slot /></form>' },
  UFormField: { props: ['label'], template: '<label>{{label}}<slot /></label>' },
  UInput: defineComponent({ props: ['modelValue'], emits: ['update:modelValue'], template: '<input :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />' }), UCheckbox: true,
};
const wrappers: ReturnType<typeof mount>[] = [];
function view(component: any, props = {}) { const w = mount(component, { props, global: { plugins: [createI18n({ legacy: false, locale: 'zh', messages: { zh } })], stubs } }); wrappers.push(w); return w; }
let persisted = policy();
const wireClone = (data: unknown) => JSON.parse(JSON.stringify(data));
beforeEach(() => { vi.resetAllMocks(); persisted = policy(); api.policy.mockImplementation(async () => wireClone(persisted)); api.savePolicy.mockImplementation(async (input: any) => { persisted = { ...persisted, revision: persisted.revision + 1, budget: input.budget, resources: input.resources.map((r: any) => ({ retention_mode: r.retention_mode, ...r })) }; return wireClone(persisted); }); });
afterEach(() => { wrappers.splice(0).forEach(w => w.unmount()); vi.useRealTimers(); });
const openDraft = async (w: any) => { if (!w.find('[data-testid="storage-resource-form"]').exists()) await w.get('[data-testid="storage-advanced-draft"]').trigger('click'); };
const env = async (w: any, value: string) => { const opened = w.find('[data-testid="storage-resource-form"]').exists(); await w.get('[data-testid="storage-policy-environment"]').setValue(value); if (opened) await openDraft(w); };
const source = async (w: any, value: string) => { const opened = w.find('[data-testid="storage-resource-form"]').exists(); await w.get('[data-testid="storage-policy-source"]').setValue(value); if (opened) await openDraft(w); };
const value = (w: any, id: string) => (w.get(`[data-testid="${id}"]`).element as HTMLInputElement).value;

describe('environment-scoped resource policies', () => {
  it('uses distinct dev, prod and shared defaults; copy retention has no day input', async () => {
    const w = view(PolicyPanel, { resources: resources() }); await flushPromises(); await openDraft(w);
    expect(value(w, 'storage-policy-count')).toBe('24'); expect(value(w, 'storage-policy-budget')).toBe('1'); expect(w.find('[data-testid="storage-policy-days"]').exists()).toBe(false);
    await env(w, 'prod:prod-install'); expect(value(w, 'storage-policy-count')).toBe('2'); expect(value(w, 'storage-policy-budget')).toBe('6');
    await env(w, 'shared:'); expect(value(w, 'storage-policy-days')).toBe('2'); expect(value(w, 'storage-policy-budget')).toBe('3');
    expect(w.text()).toContain('共享：主机统一归因'); expect(api.savePolicy).not.toHaveBeenCalled();
  });
  it('preserves cross-environment edits but saves only the current resource and uses authoritative readback', async () => {
    const w = view(PolicyPanel, { resources: resources() }); await flushPromises(); await openDraft(w);
    await w.get('[data-testid="storage-policy-count"]').setValue('30'); await env(w, 'prod:prod-install'); await w.get('[data-testid="storage-policy-count"]').setValue('4');
    await source(w, 'prod-logs'); await w.get('[data-testid="storage-policy-days"]').setValue('14'); await env(w, 'dev:dev-install');
    expect(value(w, 'storage-policy-count')).toBe('30'); await w.get('[data-testid="storage-resource-form"]').trigger('submit'); await flushPromises(); await openDraft(w);
    expect(api.policy).toHaveBeenCalledTimes(2); expect(api.savePolicy.mock.calls[0]![0].resources).toHaveLength(1);
    expect(api.savePolicy.mock.calls[0]![0].resources[0]).toMatchObject({ resource_id: 'dev-backup', retention_mode: 'count', target_days: 0, retention_count: 30 });
    expect(w.text()).toContain('其他未保存资源未提交'); await env(w, 'prod:prod-install'); expect(value(w, 'storage-policy-days')).toBe('14'); await source(w, 'prod-backup'); expect(value(w, 'storage-policy-count')).toBe('4'); await source(w, 'prod-logs'); expect(value(w, 'storage-policy-days')).toBe('14');
  });
  it('separates host budget edits from resource submission and proposes from actual disk capacity', async () => {
    const p = policy(); p.revision = 0; p.budget = { max_managed_bytes: 0, min_free_bytes: 0, critical_free_bytes: 0 }; persisted = p;
    const w = view(PolicyPanel, { resources: resources(), filesystem: { total_bytes: 120 * GiB, available_bytes: 30 * GiB, reserved_bytes: 5 * GiB } }); await flushPromises(); await openDraft(w);
    expect(w.get('[data-testid="storage-policy-save"]').attributes('disabled')).toBeDefined();
    await w.get('[data-testid="storage-host-open"]').trigger('click'); expect(value(w, 'storage-host-budget')).toBe('');
    await w.get('[data-testid="storage-host-suggest"]').trigger('click'); expect(value(w, 'storage-host-budget')).toBe('95');
    await w.get('[data-testid="storage-host-form"]').trigger('submit'); await flushPromises(); await openDraft(w); expect(api.savePolicy.mock.calls[0]![0].resources).toEqual([]); expect(w.text()).toContain('未提交资源的未保存修改');
  });
  it('keeps legacy fields until explicitly adopting an environment preset', async () => {
    persisted.resources = [{ resource_id: 'dev-backup', target_days: 9, max_bytes: 2 * GiB, retention_count: 12, min_verified_points: 0, protect_latest: true, require_verified_archive: false }];
    const w = view(PolicyPanel, { resources: resources() }); await flushPromises(); await openDraft(w); expect(value(w, 'storage-policy-days')).toBe('9'); expect(value(w, 'storage-policy-count')).toBe('12');
    expect(value(w, 'storage-policy-mode')).toBe('legacy'); expect(api.savePolicy).not.toHaveBeenCalled();
    await w.findAll('button').find(b => b.text() === '采用此环境建议')!.trigger('click'); expect(value(w, 'storage-policy-count')).toBe('24'); expect(w.find('[data-testid="storage-policy-days"]').exists()).toBe(false);
  });
  it('retains the draft on a revision conflict or failed readback', async () => {
    const w = view(PolicyPanel, { resources: resources() }); await flushPromises(); await openDraft(w); await w.get('[data-testid="storage-policy-count"]').setValue('31');
    api.savePolicy.mockRejectedValueOnce(new Error('STORAGE_POLICY_REVISION_CONFLICT')); await w.get('[data-testid="storage-resource-form"]').trigger('submit'); await flushPromises(); await openDraft(w); expect(w.text()).toContain('STORAGE_POLICY_REVISION_CONFLICT');
    api.policy.mockRejectedValueOnce(new Error('readback unavailable')); await w.get('[data-testid="storage-resource-form"]').trigger('submit'); await flushPromises(); await openDraft(w); expect(w.text()).toContain('readback unavailable'); expect(value(w, 'storage-policy-count')).toBe('31');
  });
  it('binds manual estimates to the selected environment resource and cancels late results on selection change', async () => {
    const w = view(PolicyPanel, { resources: resources() }); await flushPromises(); await openDraft(w); await env(w, 'prod:prod-install'); await source(w, 'prod-logs');
    await w.get('[data-testid="storage-policy-days"]').setValue('14'); await w.get('[data-testid="storage-policy-budget"]').setValue('3'); await w.get('[data-testid="storage-calculate"]').trigger('click'); await w.get('[data-testid="storage-daily"]').setValue('0.3');
    let resolve!: (v: any) => void; api.estimate.mockImplementationOnce(() => new Promise(r => { resolve = r; })); await w.get('[data-testid="storage-estimate-form"]').trigger('submit'); await flushPromises(); await openDraft(w);
    expect(api.estimate.mock.calls[0]![0]).toMatchObject({ resource_id: 'prod-logs', retention_mode: 'days', target_days: 14, max_bytes: 3 * GiB });
    const signal = api.estimate.mock.calls[0]![1]; await env(w, 'dev:dev-install'); resolve({ state: 'budget_insufficient' }); await flushPromises(); await openDraft(w); expect(signal.aborted).toBe(true); expect(w.find('[data-testid="storage-estimate"]').exists()).toBe(false);
  });
  it('aborts requests on unmount and disables writes without deployment identity', async () => {
    persisted.identity = { instance_id: 'dev-install' }; const w = view(PolicyPanel, { resources: resources() }); await flushPromises(); await openDraft(w); expect(w.get('[data-testid="storage-policy-save"]').attributes('disabled')).toBeDefined();
    const signal = api.policy.mock.calls[0]![0]; w.unmount(); expect(signal.aborted).toBe(true);
  });
});

describe('resource task navigation and draft preservation', () => {
  it('shows only the selected resource, embeds its owner configuration and supports keyboard navigation without writes', async () => {
    const w = view(PolicyPanel, { resources: resources() }); await flushPromises();
    expect(w.find('[data-testid="storage-resource-form"]').exists()).toBe(false);
    expect(w.getComponent({ name: 'StorageResourceConfiguration' }).props('source').resource.id).toBe('dev-backup');
    await env(w, 'prod:prod-install'); await w.get('[data-resource-tab="prod-backup"]').trigger('keydown', { key: 'End' });
    expect(w.get('[data-testid="storage-resource-content"] h3').text()).toBe('正式应用日志');
    expect(w.get('[data-resource-tab="prod-logs"]').attributes('aria-selected')).toBe('true');
    expect(w.getComponent({ name: 'StorageResourceConfiguration' }).props('source').resource.id).toBe('prod-logs');
    await w.get('[data-testid="storage-live-budget"]').trigger('click'); expect(w.emitted('open-runtime')?.[0]).toEqual([{ action: 'budget' }]);
    expect(api.savePolicy).not.toHaveBeenCalled(); expect(api.executeCleanup).not.toHaveBeenCalled();
  });
  it('keeps pending save attention bound to its source across resource switches and blocks navigation during its write', async () => {
    const w = view(PolicyPanel, { resources: resources() }); await flushPromises(); const control = w.getComponent({ name: 'StorageResourceConfiguration' });
    control.vm.$emit('busy', { sourceId: 'dev-backup', busy: true }); await flushPromises(); await env(w, 'prod:prod-install'); expect(w.get('[data-testid="storage-resource-content"] h3').text()).toBe('开发备份');
    control.vm.$emit('busy', { sourceId: 'dev-backup', busy: false }); control.vm.$emit('attention', { sourceId: 'dev-backup', pending: true }); await flushPromises(); await env(w, 'prod:prod-install'); expect(w.text()).toContain('保存结果尚未确认');
    control.vm.$emit('attention', { sourceId: 'dev-backup', pending: false }); await flushPromises(); expect(w.text()).not.toContain('保存结果尚未确认'); expect(api.savePolicy).not.toHaveBeenCalled();
  });
  it('preserves dirty drafts across a refresh but keeps the original edit revision for external conflicts', async () => {
    const w = view(PolicyPanel, { resources: resources() }); await flushPromises(); await openDraft(w);
    await w.get('[data-testid="storage-policy-count"]').setValue('33');
    persisted.revision = 8; persisted.resources = [{ resource_id: 'dev-backup', retention_mode: 'count', target_days: 0, retention_count: 3, max_bytes: GiB, min_verified_points: 0, protect_latest: true, require_verified_archive: false }];
    await w.findAll('button').find(b => b.text() === zh.storageGov.drafts.reload)!.trigger('click'); await flushPromises(); await openDraft(w);
    expect(value(w, 'storage-policy-count')).toBe('33');
    api.savePolicy.mockRejectedValueOnce(new Error('STORAGE_POLICY_REVISION_CONFLICT'));
    await w.get('[data-testid="storage-resource-form"]').trigger('submit'); await flushPromises();
    expect(api.savePolicy.mock.calls[0]![0].expected_revision).toBe(1); expect(value(w, 'storage-policy-count')).toBe('33');
    await w.get('[data-testid="storage-draft-discard"]').trigger('click'); expect(value(w, 'storage-policy-count')).toBe('3');
    await w.get('[data-testid="storage-resource-form"]').trigger('submit'); await flushPromises(); expect(api.savePolicy.mock.calls[1]![0].expected_revision).toBe(8);
  });
  it('blocks resource switches while a manual preview is in flight', async () => {
    const logs = { ...policy().catalog[2]!, resource: { ...policy().catalog[2]!.resource, id: 'logs', scope: 'dev', instance_id: 'dev-install' }, cleanup: { supported: true } };
    persisted.catalog.push(logs); const w = view(PolicyPanel, { resources: resources() }); await flushPromises(); await source(w, 'logs');
    await w.get('[data-testid="storage-log-open"]').trigger('click'); let resolve!: (value: unknown) => void;
    api.previewCleanup.mockImplementationOnce(() => new Promise(r => { resolve = r; })); await w.get('[data-testid="storage-log-form"]').trigger('submit'); await flushPromises();
    expect(w.get('[data-resource-tab="dev-backup"]').attributes('disabled')).toBeDefined(); await source(w, 'dev-backup');
    expect(w.get('[data-testid="storage-resource-content"]').text()).toContain('正式应用日志');
    resolve({ id: 'preview', resource: logs.resource, state: 'preview', can_execute: false, items: [], skipped: {} }); await flushPromises();
    await w.get('[data-modal-close]').trigger('click'); await source(w, 'dev-backup'); expect(w.get('[data-testid="storage-resource-content"] h3').text()).toBe('开发备份'); expect(api.executeCleanup).not.toHaveBeenCalled();
  });
  it('keeps manual cutoff and unconfirmed batches across resources; recovery only reads the original batch', async () => {
    const logs = { ...policy().catalog[2]!, resource: { ...policy().catalog[2]!.resource, id: 'logs', scope: 'dev', instance_id: 'dev-install' }, cleanup: { supported: true, retention_days: 7, retention_count: 5 } };
    persisted.catalog.push(logs); const w = view(PolicyPanel, { resources: resources() }); await flushPromises(); await source(w, 'logs');
    await w.get('[data-testid="storage-log-open"]').trigger('click'); await w.get('[data-testid="storage-log-days"]').setValue('12'); await w.get('[data-modal-close]').trigger('click');
    await source(w, 'dev-backup'); await source(w, 'logs'); await w.get('[data-testid="storage-log-open"]').trigger('click'); expect(value(w, 'storage-log-days')).toBe('12');
    const record = { id: 'original', resource: logs.resource, state: 'preview', can_execute: true, items: [], skipped: {}, deleted_files: 0, removed_file_bytes: 0 };
    api.previewCleanup.mockResolvedValue(record); await w.get('[data-testid="storage-log-form"]').trigger('submit'); await flushPromises();
    api.executeCleanup.mockRejectedValueOnce(new Error('response lost')); api.cleanupRun.mockRejectedValueOnce(new Error('unavailable'));
    await w.get('[data-testid="storage-log-execute"]').trigger('click'); await flushPromises(); await w.get('[data-modal-close]').trigger('click');
    await source(w, 'dev-backup'); await source(w, 'logs'); expect(w.text()).toContain('核对原批次结果');
    await w.get('[data-testid="storage-log-open"]').trigger('click'); expect(w.find('[data-testid="storage-log-form"]').exists()).toBe(false);
    expect(w.find('[data-testid="storage-log-execute"]').exists()).toBe(false);
    api.cleanupRun.mockResolvedValueOnce({ ...record, state: 'completed' }); await w.get('[data-testid="storage-log-review"]').trigger('click'); await flushPromises();
    expect(api.cleanupRun.mock.calls.at(-1)?.[0]).toBe('original'); expect(api.executeCleanup).toHaveBeenCalledTimes(1); expect(api.previewCleanup).toHaveBeenCalledTimes(1);
  });
});

describe('storage observation history', () => {
  it('selects registered resource history and keeps absent samples unknown', async () => {
    api.history.mockResolvedValue({ sample_count: 2, expected_samples: 4032, missing_samples: 4030, metric_changes: 0, unavailable_samples: 0, sources: [{ id: 'logs', name: '应用日志', kind: 'application_logs', scope: 'dev', instance_id: 'dev-install' }], points: [{ observed_at: '2026-10-29T00:00:00Z', database_bytes: 1024, resources: [] }, { observed_at: '2026-10-29T00:05:00Z', database_bytes: 1024, resources: [{ id: 'logs', bytes: 2048, state: 'observed', net_change_bytes: 1024 }] }] });
    const w = view(HistoryPanel); await flushPromises(); await w.get('[data-testid="storage-history-source"]').setValue('resource:logs');
    const table = w.get('[data-testid="storage-history-table"]').text(); expect(table).toContain('2 KiB'); expect(table).toContain('+1 KiB'); expect(table).toContain('该时刻未取得记录'); expect(api.history).toHaveBeenCalledTimes(1);
    await w.get('[data-testid="storage-history-disk"]').trigger('click'); await w.get('[data-testid="storage-history-powerx"]').trigger('click'); expect(value(w, 'storage-history-source')).toBe('resource:logs');
  });
  it('shows gaps and unavailable data without polling', async () => {
    vi.useFakeTimers(); api.history.mockResolvedValue({ sample_count: 2, expected_samples: 4032, missing_samples: 4030, metric_changes: 0, unavailable_samples: 1, sampling: { configured: true, worker_enabled: true, interval_seconds: 300, metadata_retention_days: 15 }, points: [{ observed_at: '2026-10-29T00:00:00Z', used_bytes: 900 * GiB, available_bytes: 30 * GiB, database_bytes: 335 * 1024 ** 2 }, { observed_at: '2026-10-29T00:05:00Z', used_bytes: null, available_bytes: null, database_bytes: null }] });
    const w = view(HistoryPanel); await flushPromises(); expect(w.text()).toContain('4030'); expect(w.text()).toContain('—'); expect(w.text()).toContain('用量变化不能证明日志保留了多少天'); expect(w.get('[data-testid="storage-sampling-status"]').text()).toContain('已配置每 5 分钟');
    expect(w.get('[data-testid="storage-history-table"]').text()).toContain('335 MiB');
    expect(w.get('[data-testid="storage-history-table"]').text()).not.toContain('900 GiB');
    expect(w.get('[data-testid="storage-history-table"]').text()).not.toContain('整盘已用');
    await w.get('[data-testid="storage-history-disk"]').trigger('click');
    expect(w.get('[data-testid="storage-history-table"]').text()).toContain('900 GiB');
    expect(w.get('[data-testid="storage-history-table"]').text()).not.toContain('335 MiB');
    expect(w.get('[data-testid="storage-history-scope"]').text()).toContain('不代表 PowerX 占用');
    await vi.advanceTimersByTimeAsync(3600000); expect(api.history).toHaveBeenCalledTimes(1);
  });
});

describe('bounded application-log cleanup', () => {
  const sourceOption = () => ({ ...policy().catalog[2]!, resource: { id: 'logs', name: '应用日志', kind: 'application_logs', scope: 'dev', instance_id: 'dev-install' }, cleanup: { supported: true, retention_days: 7, retention_count: 10 } });
  const preview = () => ({ id: '18b36224-0b65-43d7-8870-eb844bf41a27', resource: sourceOption().resource, state: 'preview', created_at: '2026-10-29T00:00:00Z', expires_at: '2026-10-29T00:10:00Z', target_days: 7, actor_id: 1, can_execute: true, skipped: { active_file: 2 }, items: [{ name: 'info-2020-01-01T00-00-00.000.log', bytes: 1024, outcome: 'candidate' }], deleted_files: 0, removed_file_bytes: 0, reclaimed_bytes: null });
  it('uses discovered logs without requiring a host-budget draft', async () => {
    persisted.catalog = [sourceOption()]; persisted.budget = { max_managed_bytes: 0, min_free_bytes: 0, critical_free_bytes: 0 };
    const w = view(PolicyPanel, { resources: resources() }); await flushPromises();
    await w.get('[data-testid="storage-log-open"]').trigger('click'); expect(w.get('[data-testid="storage-log-days"]').element.value).toBe('7'); expect(w.find('[data-testid="storage-host-open"]').exists()).toBe(false); expect(w.find('[data-testid="storage-resource-form"]').exists()).toBe(false);
  });
  it('previews without deleting, requires confirmation, then verifies canonical readback', async () => {
    api.previewCleanup.mockResolvedValue(preview()); const result = { ...preview(), state: 'completed', can_execute: false, deleted_files: 1, removed_file_bytes: 1024, items: [{ ...preview().items[0], outcome: 'deleted' }] };
    api.executeCleanup.mockResolvedValue(result); api.cleanupRun.mockResolvedValue(result);
    const w = view(LogPanel, { source: sourceOption() }); await w.get('[data-testid="storage-log-form"]').trigger('submit'); await flushPromises();
    expect(api.executeCleanup).not.toHaveBeenCalled(); await w.get('[data-testid="storage-log-execute"]').trigger('click'); await flushPromises();
    expect(api.executeCleanup).toHaveBeenCalledTimes(1); expect(api.cleanupRun).toHaveBeenCalledTimes(1); expect(w.get('[data-testid="storage-log-result"]').text()).toContain('已回读确认删除 1 个文件'); expect(w.find('[data-testid="storage-log-execute"]').exists()).toBe(false);
  });
  it('never resubmits deletion when the response/readback cannot confirm the result', async () => {
    api.previewCleanup.mockResolvedValue(preview()); api.executeCleanup.mockRejectedValue(new Error('response lost')); api.cleanupRun.mockResolvedValue({ ...preview(), state: 'unconfirmed', can_execute: false });
    const w = view(LogPanel, { source: sourceOption() }); await w.get('[data-testid="storage-log-form"]').trigger('submit'); await flushPromises(); await w.get('[data-testid="storage-log-execute"]').trigger('click'); await flushPromises();
    expect(w.text()).toContain('结果未确认'); expect(w.find('[data-testid="storage-log-result"]').exists()).toBe(false); expect(w.find('[data-testid="storage-log-execute"]').exists()).toBe(false); expect(api.executeCleanup).toHaveBeenCalledTimes(1);
  });
  it('accepts authoritative completion readback after a lost execution response', async () => {
    api.previewCleanup.mockResolvedValue(preview()); api.executeCleanup.mockRejectedValue(new Error('response lost')); api.cleanupRun.mockResolvedValue({ ...preview(), state: 'completed', can_execute: false, deleted_files: 1, removed_file_bytes: 1024 });
    const w = view(LogPanel, { source: sourceOption() }); await w.get('[data-testid="storage-log-form"]').trigger('submit'); await flushPromises(); await w.get('[data-testid="storage-log-execute"]').trigger('click'); await flushPromises();
    expect(w.get('[data-testid="storage-log-result"]').text()).toContain('已回读确认删除 1 个文件'); expect(api.executeCleanup).toHaveBeenCalledTimes(1);
  });
  it('keeps blocked previews read-only and fetches records only on entry/manual events', async () => {
    vi.useFakeTimers(); api.previewCleanup.mockResolvedValue({ ...preview(), can_execute: false, blocked_reason: 'archive_verification_required' }); api.cleanupRuns.mockResolvedValue([]);
    const w = view(LogPanel, { source: sourceOption() }); await w.get('[data-testid="storage-log-form"]').trigger('submit'); await flushPromises(); expect(w.find('[data-testid="storage-log-execute"]').exists()).toBe(false); expect(w.text()).toContain('需要已验证归档');
    view(CleanupRecords); await flushPromises(); await vi.advanceTimersByTimeAsync(3600000); expect(api.cleanupRuns).toHaveBeenCalledTimes(1);
  });
});
