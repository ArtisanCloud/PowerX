import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { flushPromises, mount } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import { defineComponent } from 'vue';
import zh from '../../../i18n/locales/zh.json';
import Panel from '~/components/monitor/storage/StorageRuntimePanel.vue';
import { decodeStorageRuntime, decodeRuntimePreview } from '~/composables/api/services/storageRuntimeService';
const api = vi.hoisted(() => ({ status: vi.fn(), savePolicy: vi.fn(), saveBudget: vi.fn(), preview: vi.fn(), run: vi.fn(), acknowledge: vi.fn() }));
vi.mock('~/composables/api/services/storageRuntimeService', async original => ({ ...await original<any>(), useStorageRuntimeService: () => api }));
const GiB = 1024 ** 3;
export const runtimeFixture = () => ({ configured: true, worker_enabled: true, admission_enabled: true, notifications_configured: false, database_cleanup_enabled: false,
  budget_policy: { revision: 1, min_free_bytes: 20 * GiB, critical_free_bytes: 10 * GiB, max_used_bytes: 90 * GiB, max_concurrent: 1 },
  budget: { available_bytes: 30 * GiB, used_bytes: 70 * GiB, reserved_bytes: 0, min_free_bytes: 20 * GiB, critical_free_bytes: 10 * GiB, active: [] },
  lifecycle: { configured: true, allow_enforce: true, revision: 1, database_cleanup_enabled: false, sources: [{ id: 'logs', name: '正式文件日志', kind: 'application_logs', scope: 'prod', instance_id: 'installed', format: 'numeric', supported: true, require_archive: true }], policies: [] as any[], runs: [] as any[] },
});
const stubs = {
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
function view() { const w = mount(Panel, { global: { stubs, plugins: [createI18n({ legacy: false, locale: 'zh', messages: { zh } })] } }); wrappers.push(w); return w; }
const button = (w: any, text: string) => w.findAll('button').find((b: any) => b.text() === text)!;
beforeEach(() => {
  vi.resetAllMocks(); persisted = runtimeFixture(); api.status.mockImplementation(async () => clone(persisted));
  api.savePolicy.mockImplementation(async (source, input) => { const p = { source_id: source, revision: input.expected_revision + 1, enabled: input.enabled, cleanup_mode: input.cleanup_mode, retention_days: input.retention_days, max_bytes: input.max_bytes, require_archive: input.require_archive, owner_instance: 'installed', operator_id: 1, updated_at: new Date().toISOString() }; persisted.lifecycle.policies = [p]; return p; });
  api.preview.mockResolvedValue({ source_id: 'logs', policy_revision: 0, candidate_names: ['app.log.2.gz'], candidate_bytes: 4096, protected: { latest_rotation: 1 }, observed_bytes: 8192, over_budget: false, complete: true });
});
afterEach(() => { wrappers.splice(0).forEach(w => w.unmount()); vi.useRealTimers(); });
async function edit(w: any) { await flushPromises(); await button(w, '配置策略').trigger('click'); }
async function enable(w: any, mode = 'archive_then_delete') { await w.get('[data-testid="runtime-cleanup-mode"]').setValue(mode); await w.get('[data-testid="runtime-enabled"] input').setValue(true); await w.get('[data-testid="runtime-confirm"] input').setValue(true); await w.get('form').trigger('submit'); await flushPromises(); }
describe('storage runtime safety and state', () => {
  it('previews without a write, requires confirmation, and revalidates saved archive policy', async () => {
    const w = view(); await edit(w); expect(w.get('[data-testid="runtime-save"]').attributes('disabled')).toBeDefined();
    await button(w, '预览日志清理').trigger('click'); await flushPromises(); expect(api.preview).toHaveBeenCalledWith('logs', 7, GiB, expect.any(AbortSignal)); expect(api.savePolicy).not.toHaveBeenCalled(); expect(api.run).not.toHaveBeenCalled();
    await enable(w); expect(api.savePolicy).toHaveBeenCalledTimes(1); expect(api.savePolicy.mock.calls[0]![1]).toMatchObject({ enabled: true, retention_days: 7, require_archive: true, cleanup_mode: 'archive_then_delete', confirm: true, expected_revision: 0 }); expect(w.text()).toContain('已从运行账本读回并确认保存'); expect(w.get('[data-testid="runtime-run"]').attributes('disabled')).toBeDefined();
  });
  it('allows explicit safe deletion without archiving, but never enables a disabled mode', async () => {
    const w = view(); await edit(w); expect(w.get('[data-testid="runtime-enabled"] input').attributes('disabled')).toBeDefined();
    await enable(w, 'expire_delete'); expect(api.savePolicy.mock.calls[0]![1]).toMatchObject({ cleanup_mode: 'expire_delete', require_archive: false, enabled: true });
  });
  it('recovers a lost save response only through independent readback', async () => {
    const implementation = api.savePolicy.getMockImplementation()!; api.savePolicy.mockImplementation(async (...args) => { await implementation(...args); throw new Error('response lost'); });
    const w = view(); await edit(w); await enable(w); expect(w.text()).toContain('已从运行账本读回并确认保存'); expect(api.savePolicy).toHaveBeenCalledTimes(1); expect(api.status).toHaveBeenCalledTimes(2);
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
  it('accepts independent database status but rejects unknown run states and untrusted preview paths', () => {
    expect(decodeStorageRuntime(runtimeFixture()).database_cleanup_enabled).toBe(false);
    expect(decodeStorageRuntime({ ...runtimeFixture(), database_cleanup_enabled: true }).database_cleanup_enabled).toBe(true);
    const unknown = runtimeFixture(); unknown.lifecycle.runs = [{ state: 'success', deleted_files: 0 }]; expect(() => decodeStorageRuntime(unknown)).toThrow();
    expect(() => decodeRuntimePreview({ source_id: 'logs', policy_revision: 1, candidate_names: ['/var/data'], candidate_bytes: 1, observed_bytes: 2, protected: {}, complete: true, over_budget: false })).toThrow();
  });
});
