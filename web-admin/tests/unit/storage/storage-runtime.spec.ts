import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { flushPromises, mount } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import { defineComponent } from 'vue';
import zh from '../../../i18n/locales/zh.json';
import Panel from '~/components/monitor/storage/StorageRuntimePanel.vue';
import { decodeStorageRuntime, decodeRuntimePreview, decodeLifecycleConfiguration, decodeLifecycleMutation } from '~/composables/api/services/storageRuntimeService';
const api = vi.hoisted(() => ({ status: vi.fn(), savePolicy: vi.fn(), adoptPolicy: vi.fn(), policyRequest: vi.fn(), policyConfiguration: vi.fn(), saveBudget: vi.fn(), preview: vi.fn(), run: vi.fn(), acknowledge: vi.fn() }));
vi.mock('~/composables/api/services/storageRuntimeService', async original => ({ ...await original<any>(), useStorageRuntimeService: () => api }));
const GiB = 1024 ** 3;
export const runtimeFixture = () => ({ configured: true, worker_enabled: true, admission_enabled: true, notifications_configured: false, database_cleanup_enabled: false,
  budget_policy: { revision: 1, min_free_bytes: 20 * GiB, critical_free_bytes: 10 * GiB, max_used_bytes: 90 * GiB, max_concurrent: 1 },
  budget: { available_bytes: 30 * GiB, used_bytes: 70 * GiB, reserved_bytes: 0, min_free_bytes: 20 * GiB, critical_free_bytes: 10 * GiB, active: [] },
  lifecycle_configurations: { logs: { source_id:'logs', authority:'database', ready:true, can_enable:true, state:'applied', desired_revision:1, applied_revision:1, snapshot_revision:1, desired_policy:{enabled:false, cleanup_mode:'disabled',retention_days:7,max_bytes:GiB}, legacy_revision:0,legacy_digest:'a'.repeat(64),legacy_enabled:false } } as Record<string,any>,
  lifecycle: { configured: true, allow_enforce: true, revision: 1, database_cleanup_enabled: false, sources: [{ id: 'logs', name: '正式文件日志', kind: 'application_logs', scope: 'prod', instance_id: 'installed', format: 'numeric', supported: true, require_archive: true }], policies: [{source_id:'logs',revision:1,enabled:false,cleanup_mode:'disabled',retention_days:7,max_bytes:GiB,require_archive:false}] as any[], runs: [] as any[] },
});
const stubs = {
  UIcon: true,
  UAlert: { props: ['title', 'description'], template: '<div>{{title}}{{description}}<slot name="actions" /></div>' },
  UButton: { props: ['disabled', 'loading'], template: '<button :disabled="disabled || loading"><slot /></button>' },
  UModal: { props: ['open'], template: '<section v-if="open"><slot name="body" /></section>' },
  UForm: { emits: ['submit'], template: '<form @submit.prevent="$emit(\'submit\')"><slot /></form>' },
  UFormField: { props: ['label'], template: '<label>{{label}}<slot /></label>' },
  UInput: defineComponent({ props: ['modelValue', 'disabled'], emits: ['update:modelValue'], template: '<input :value="modelValue" :disabled="disabled" @input="$emit(\'update:modelValue\', $event.target.value)" />' }),
  USelect: defineComponent({ props: ['modelValue', 'items', 'disabled'], emits: ['update:modelValue'], template: '<select :value="modelValue" :disabled="disabled" @change="$emit(\'update:modelValue\', $event.target.value)"><option v-for="item in items" :key="item.value" :value="item.value">{{item.label}}</option></select>' }),
  UCheckbox: defineComponent({ props: ['modelValue', 'label', 'disabled'], emits: ['update:modelValue'], template: '<label><input type="checkbox" :checked="modelValue" :disabled="disabled" @change="$emit(\'update:modelValue\', $event.target.checked)" />{{label}}</label>' }),
};
const wrappers: ReturnType<typeof mount>[] = [];
const clone = (d: any) => JSON.parse(JSON.stringify(d));
let persisted: ReturnType<typeof runtimeFixture>;
function view(props = {}) { const w = mount(Panel, { props, global: { stubs, plugins: [createI18n({ legacy: false, locale: 'zh', messages: { zh } })] } }); wrappers.push(w); return w; }
const button = (w: any, text: string) => w.findAll('button').find((b: any) => b.text() === text)!;
beforeEach(() => {
  vi.resetAllMocks(); persisted = runtimeFixture(); api.status.mockImplementation(async () => clone(persisted));
  api.savePolicy.mockImplementation(async (source, input) => { const p = { source_id: source, revision: input.expected_revision + 1, enabled: input.enabled, cleanup_mode: input.cleanup_mode, retention_days: input.retention_days, max_bytes: input.max_bytes, require_archive: input.require_archive, owner_instance: 'installed', operator_id: 1, updated_at: new Date().toISOString() }; persisted.lifecycle.policies = [p]; const configuration={...persisted.lifecycle_configurations[source],desired_revision:p.revision,applied_revision:p.revision,snapshot_revision:p.revision,desired_policy:{enabled:p.enabled,cleanup_mode:p.cleanup_mode,retention_days:p.retention_days,max_bytes:p.max_bytes}};persisted.lifecycle_configurations[source]=configuration;return {source_id:source,request_id:input.request_id,operation_revision:p.revision,operation_policy:configuration.desired_policy,configuration}; });
  api.policyRequest.mockImplementation(async (source,key) => {const c=persisted.lifecycle_configurations[source];return {source_id:source,request_id:key,operation_revision:c.desired_revision,operation_policy:c.desired_policy,configuration:c}});
  api.preview.mockResolvedValue({ source_id: 'logs', policy_revision: 0, candidate_names: ['app.log.2.gz'], candidate_bytes: 4096, protected: { latest_rotation: 1 }, observed_bytes: 8192, over_budget: false, complete: true });
});
afterEach(() => { wrappers.splice(0).forEach(w => w.unmount()); vi.useRealTimers(); });
async function edit(w: any) { await flushPromises(); await button(w, '配置策略').trigger('click'); }
async function enable(w: any, mode = 'archive_then_delete') { await w.get('[data-testid="runtime-cleanup-mode"]').setValue(mode); await w.get('[data-testid="runtime-enabled"] input').setValue(true); await w.get('[data-testid="runtime-confirm"] input').setValue(true); await w.get('form').trigger('submit'); await flushPromises(); }
describe('storage runtime safety and state', () => {
  it('previews without a write, requires confirmation, and revalidates saved archive policy', async () => {
    const w = view(); await edit(w); expect(w.get('[data-testid="runtime-save"]').attributes('disabled')).toBeDefined();
    await button(w, '预览日志清理').trigger('click'); await flushPromises(); expect(api.preview).toHaveBeenCalledWith('logs', 7, GiB, expect.any(AbortSignal)); expect(api.savePolicy).not.toHaveBeenCalled(); expect(api.run).not.toHaveBeenCalled();
    await enable(w); expect(api.savePolicy).toHaveBeenCalledTimes(1); expect(api.savePolicy.mock.calls[0]![1]).toMatchObject({ enabled: true, retention_days: 7, require_archive: true, cleanup_mode: 'archive_then_delete', confirm: true, expected_revision: 1 }); expect(w.text()).toContain('已发布并确认生效'); expect(w.get('[data-testid="runtime-run"]').attributes('disabled')).toBeDefined();
  });
  it('allows explicit safe deletion without archiving, but never enables a disabled mode', async () => {
    const w = view(); await edit(w); expect(w.get('[data-testid="runtime-enabled"] input').attributes('disabled')).toBeDefined();
    await enable(w, 'expire_delete'); expect(api.savePolicy.mock.calls[0]![1]).toMatchObject({ cleanup_mode: 'expire_delete', require_archive: false, enabled: true });
  });
  it('recovers a lost save response only through independent readback', async () => {
    const implementation = api.savePolicy.getMockImplementation()!; api.savePolicy.mockImplementation(async (...args) => { await implementation(...args); throw new Error('response lost'); });
    const w = view(); await edit(w); await enable(w); expect(w.text()).toContain('已发布并确认生效'); expect(api.savePolicy).toHaveBeenCalledTimes(1); expect(api.status).toHaveBeenCalledTimes(2);
  });
  it('never repeats a destructive batch when its response is lost', async () => {
    const w = view(); await edit(w); await enable(w); await button(w, '预览日志清理').trigger('click'); await flushPromises();
    api.run.mockImplementation(async (source_id, expected_revision, request_key) => { persisted.lifecycle.runs = [{ id: 'run-1', source_id, policy_revision: expected_revision, request_key, instance_id: 'installed', state: 'unconfirmed', started_at: new Date().toISOString(), finished_at: null, deleted_files: 0, removed_bytes: 0, removed_allocated_bytes: 0, available_before: 100, available_after: null }]; throw new Error('response lost'); });
    await w.get('[data-testid="runtime-run"]').trigger('click'); await flushPromises(); expect(api.run).toHaveBeenCalledTimes(1); expect(w.get('[data-testid="runtime-run"]').attributes('disabled')).toBeDefined(); expect(w.text()).toContain('结果未确认'); expect(w.text()).toContain('—');
  });
  it('preserves unsaved edits on push refresh, does not poll, and aborts when leaving', async () => {
    vi.useFakeTimers(); const w = view(); await edit(w); await w.get('[data-testid="runtime-days"]').setValue('14'); await w.setProps({ revision: 2 }); await flushPromises(); expect((w.get('[data-testid="runtime-days"]').element as HTMLInputElement).value).toBe('14'); expect(api.status).toHaveBeenCalledTimes(2);
    await vi.advanceTimersByTimeAsync(3600000); expect(api.status).toHaveBeenCalledTimes(2); const signal = api.status.mock.calls[1]![0]; w.unmount(); expect(signal.aborted).toBe(true);
  });
  it('embeds the selected source’s real settings, saves in place and updates the read-back summary', async () => {
    persisted.lifecycle.policies = [{ source_id: 'logs', revision: 1, enabled: false, cleanup_mode: 'expire_delete', retention_days: 9, max_bytes: GiB, require_archive: false }];
    const w = view({ sourceId: 'logs' }); await flushPromises(); expect(w.get('[data-testid="resource-log-settings"]').text()).toContain('9 天');
    expect(w.text()).not.toContain('日志交付'); await w.get('[data-testid="resource-log-edit"]').trigger('click'); await w.get('[data-testid="runtime-days"]').setValue('14'); await w.get('[data-testid="runtime-confirm"] input').setValue(true); await w.get('form').trigger('submit'); await flushPromises();
    expect(api.savePolicy).toHaveBeenCalledWith('logs', expect.objectContaining({ expected_revision: 1, retention_days: 14, enabled: false }), expect.any(AbortSignal)); expect(w.get('[data-testid="resource-log-settings"]').text()).toContain('14 天'); expect(api.run).not.toHaveBeenCalled();
  });
  it('keeps source-local edits and their original revision when a newer overview arrives', async () => {
    persisted.lifecycle.policies = [{ source_id: 'logs', revision: 1, enabled: false, cleanup_mode: 'expire_delete', retention_days: 9, max_bytes: GiB, require_archive: false }];
    const w = view({ sourceId: 'logs' }); await flushPromises(); await w.get('[data-testid="resource-log-edit"]').trigger('click'); await w.get('[data-testid="runtime-days"]').setValue('18'); const newer = clone(persisted); newer.lifecycle.policies[0].revision = 5; newer.lifecycle.policies[0].retention_days = 21; await w.setProps({ initial: newer }); await flushPromises();
    expect((w.get('[data-testid="runtime-days"]').element as HTMLInputElement).value).toBe('18'); api.savePolicy.mockRejectedValueOnce(Object.assign(new Error('REVISION_CONFLICT'),{status:409})); await w.get('[data-testid="runtime-confirm"] input').setValue(true); await w.get('form').trigger('submit'); await flushPromises(); expect(api.savePolicy.mock.calls[0]?.[1].expected_revision).toBe(1);
  });
  it('opens the requested source after loading, retains source edits and does not replay navigation on refresh', async () => {
    const w = view({ navigation: { request: 1, action: 'policy', sourceId: 'logs' } }); await flushPromises();
    expect(w.find('[data-testid="runtime-days"]').exists()).toBe(true); await w.get('[data-testid="runtime-days"]').setValue('18');
    await w.setProps({ revision: 1 }); await flushPromises(); expect((w.get('[data-testid="runtime-days"]').element as HTMLInputElement).value).toBe('18');
    expect(api.preview).not.toHaveBeenCalled(); expect(api.savePolicy).not.toHaveBeenCalled(); expect(api.run).not.toHaveBeenCalled();
    expect(w.emitted('observed')).toHaveLength(2);
  });
  it('opens the live host budget and fails closed for an unavailable source', async () => {
    const w = view({ navigation: { request: 1, action: 'budget' } }); await flushPromises(); expect(w.text()).toContain('GiB');
    await w.setProps({ navigation: { request: 2, action: 'policy', sourceId: 'unregistered' } }); await flushPromises();
    expect(w.text()).toContain('该资源当前没有可用的自动清理适配器'); expect(api.saveBudget).not.toHaveBeenCalled(); expect(api.savePolicy).not.toHaveBeenCalled();
  });
  it('adopts a legacy source explicitly, pauses it, and keeps publication pending distinct from applied', async () => {
    const legacy = persisted.lifecycle_configurations.logs!; Object.assign(legacy, {authority:'legacy',state:'legacy',desired_revision:0,applied_revision:0,snapshot_revision:null,desired_policy:null,legacy_revision:7,legacy_enabled:true});
    persisted.lifecycle.policies[0] = { ...persisted.lifecycle.policies[0],revision:7,enabled:true,cleanup_mode:'expire_delete'};
    api.adoptPolicy.mockImplementation(async (source,input) => {
      const p = {...persisted.lifecycle.policies[0],enabled:false};const configuration={...legacy,authority:'database',state:'pending',desired_revision:1,desired_policy:{enabled:false,cleanup_mode:p.cleanup_mode,retention_days:p.retention_days,max_bytes:p.max_bytes}};
      persisted.lifecycle_configurations[source]=configuration;persisted.lifecycle.policies[0]={...p,enabled:false};
      return {source_id:source,request_id:input.request_id,operation_revision:1,operation_policy:configuration.desired_policy,configuration};
    });
    const w=view();await edit(w);expect(w.find('[data-testid="runtime-save"]').exists()).toBe(false);
    expect(w.get('[data-testid="runtime-adopt"]').attributes('disabled')).toBeDefined();await w.get('[data-testid="runtime-adoption-confirm"] input').setValue(true);await w.get('[data-testid="runtime-adopt"]').trigger('click');await flushPromises();
    expect(api.adoptPolicy).toHaveBeenCalledWith('logs',expect.objectContaining({legacy_revision:7,confirm:true}),expect.any(AbortSignal));expect(api.savePolicy).not.toHaveBeenCalled();expect(api.run).not.toHaveBeenCalled();expect(w.get('[data-testid="runtime-config-versions"]').text()).toContain('期望 v1');expect(w.get('[data-testid="runtime-save-result"]').text()).toContain('保存不代表已生效');expect(w.find('[data-testid="runtime-run"]').exists()).toBe(false);
  });
  it('keeps an uncertain request query-only even after not-found, and verifies the exact original receipt', async () => {
    api.savePolicy.mockRejectedValue(new Error('timeout'));api.policyRequest.mockRejectedValue(Object.assign(new Error('not found'),{status:404}));
    const w=view();await edit(w);await enable(w,'expire_delete');expect(w.get('[data-testid="runtime-config-unknown"]').exists()).toBe(true);expect(w.get('[data-testid="runtime-save"]').attributes('disabled')).toBeDefined();
    const sent=api.savePolicy.mock.calls[0]![1];api.policyRequest.mockResolvedValue({source_id:'logs',request_id:sent.request_id,operation_revision:2,operation_policy:{enabled:true,cleanup_mode:'expire_delete',retention_days:7,max_bytes:GiB},configuration:{...persisted.lifecycle_configurations.logs,desired_revision:2,desired_policy:{enabled:true,cleanup_mode:'expire_delete',retention_days:7,max_bytes:GiB},state:'pending'}});
    await w.get('[data-testid="runtime-reconcile"]').trigger('click');await flushPromises();expect(api.savePolicy).toHaveBeenCalledTimes(1);expect(api.policyRequest).toHaveBeenCalledTimes(2);expect(w.find('[data-testid="runtime-config-unknown"]').exists()).toBe(false);
  });
  it('rejects a mismatched receipt and blocks writes on an old backend', async () => {
    api.savePolicy.mockResolvedValue({source_id:'other',request_id:'wrong',operation_revision:99,operation_policy:{enabled:true},configuration:{authority:'database'}});api.policyRequest.mockRejectedValue(new Error('unavailable'));
    const w=view();await edit(w);await enable(w);expect(w.get('[data-testid="runtime-config-unknown"]').exists()).toBe(true);
    const previous=clone(persisted);delete previous.lifecycle_configurations;api.status.mockResolvedValue(previous);const old=view();await edit(old);expect(old.find('[data-testid="runtime-save"]').exists()).toBe(false);expect(old.text()).toContain('配置服务尚未就绪');
  });
  it('preserves the form across sections and requires explicit loading after a revision conflict', async () => {
    const w=view();await edit(w);await w.get('[data-testid="runtime-days"]').setValue('18');
    await button(w,'预览与清理').trigger('click');await button(w,'保留策略').trigger('click');expect((w.get('[data-testid="runtime-days"]').element as HTMLInputElement).value).toBe('18');
    persisted.lifecycle_configurations.logs!.desired_revision=4;persisted.lifecycle_configurations.logs!.desired_policy!.retention_days=21;
    api.savePolicy.mockRejectedValue(Object.assign(new Error('REVISION_CONFLICT'),{status:409}));await w.get('[data-testid="runtime-confirm"] input').setValue(true);await w.get('form').trigger('submit');await flushPromises();expect(api.policyRequest).not.toHaveBeenCalled();expect(w.find('[data-testid="runtime-config-unknown"]').exists()).toBe(false);
    await button(w,zh.storageGov.tableMonitor.control.loadLatest).trigger('click');await flushPromises();expect((w.get('[data-testid="runtime-days"]').element as HTMLInputElement).value).toBe('21');
  });
  it('allows pausing when the publisher is unavailable, and updates a running result from push without polling', async () => {
    const w=view();await edit(w);await enable(w,'expire_delete');await button(w,'预览日志清理').trigger('click');await flushPromises();
    api.run.mockImplementation(async (source_id,policy_revision,request_key)=>{persisted.lifecycle.runs=[{id:'run-push',source_id,policy_revision,request_key,instance_id:'installed',state:'executing',started_at:new Date().toISOString(),finished_at:null,deleted_files:0,removed_bytes:0,removed_allocated_bytes:0,available_before:100,available_after:null}]});
    await w.get('[data-testid="runtime-run"]').trigger('click');await flushPromises();expect(w.text()).toContain('执行中');
    const updated=clone(persisted);updated.lifecycle.runs[0].state='completed';updated.lifecycle.runs[0].deleted_files=2;updated.lifecycle_configurations.logs.can_enable=false;await w.setProps({initial:updated});await flushPromises();expect(w.text()).toContain('批次完成');
    expect(w.get('[data-testid="runtime-enabled"] input').attributes('disabled')).toBeUndefined();await w.get('[data-testid="runtime-enabled"] input').setValue(false);await w.get('[data-testid="runtime-confirm"] input').setValue(true);await w.get('form').trigger('submit');await flushPromises();expect(api.savePolicy.mock.calls.at(-1)?.[1].enabled).toBe(false);
  });
  it('validates database authority and immutable receipt envelopes', () => {
    const c=runtimeFixture().lifecycle_configurations.logs!;expect(decodeLifecycleConfiguration(c).authority).toBe('database');expect(decodeStorageRuntime({...runtimeFixture(),lifecycle_configurations:null}).lifecycle_configurations).toBeNull();
    expect(()=>decodeLifecycleConfiguration({...c,desired_policy:{...c.desired_policy,max_bytes:-1}})).toThrow();expect(()=>decodeStorageRuntime({...runtimeFixture(),lifecycle_configurations:{other:c}})).toThrow();
    expect(()=>decodeLifecycleMutation({source_id:'other',request_id:crypto.randomUUID(),operation_revision:1,operation_policy:c.desired_policy,configuration:c})).toThrow();
  });
  it('accepts independent database status but rejects unknown run states and untrusted preview paths', () => {
    expect(decodeStorageRuntime(runtimeFixture()).database_cleanup_enabled).toBe(false);
    expect(decodeStorageRuntime({ ...runtimeFixture(), database_cleanup_enabled: true }).database_cleanup_enabled).toBe(true);
    const unknown = runtimeFixture(); unknown.lifecycle.runs = [{ state: 'success', deleted_files: 0 }]; expect(() => decodeStorageRuntime(unknown)).toThrow();
    expect(() => decodeRuntimePreview({ source_id: 'logs', policy_revision: 1, candidate_names: ['/var/data'], candidate_bytes: 1, observed_bytes: 2, protected: {}, complete: true, over_budget: false })).toThrow();
  });
});
