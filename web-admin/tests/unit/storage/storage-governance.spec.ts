import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { flushPromises, mount } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import { defineComponent } from 'vue';
import zh from '../../../i18n/locales/zh.json';
import Workspace from '~/components/monitor/storage/StorageGovernanceWorkspace.vue';
import { policy } from './fixtures';
import { beginArchiveRunIntent } from '~/utils/archive-run-request';
const archiveSession = vi.hoisted(() => ({ sessionId: 'storage-root', tenantUuid: 'storage-tenant', status: 'active' }));
vi.mock('~/utils/auth-session', () => ({ AUTH_SESSION_EVENT: 'powerx:auth-session-changed', readAuthSession: () => archiveSession }));

const api = vi.hoisted(() => ({ overview: vi.fn(), estimate: vi.fn(), policy: vi.fn() }));
vi.mock('~/composables/api/services/storageGovernanceService', () => ({ useStorageGovernanceService: () => api }));
const snapshot = () => ({
  mode: 'observe', cleanup_enabled: false, identity: { environment: 'dev' }, observed_at: '2026-10-28T00:00:00Z', expires_at: '2026-10-28T00:05:00Z',
  filesystem: { view: 'process_filesystem', state: 'observed', total_bytes: 120 * 1024 ** 3, used_bytes: 90 * 1024 ** 3, available_bytes: 25 * 1024 ** 3, reserved_bytes: 5 * 1024 ** 3 },
  resources: [{ id: 'registered', name: 'Registered backup files', scope: 'dev', state: 'observed', physical_bytes: 1024, attributed_bytes: 1024 }, { id: 'missing', name: 'Missing logs', scope: 'shared', state: 'unavailable', physical_bytes: null, attributed_bytes: null }],
  coverage: 'partial', unattributed_bytes: null, limitations: [],
});
const stubs = {
  StorageResourceConfiguration: true,
  NuxtLink: { template: '<a><slot /></a>' },
  UCard: { template: '<div><slot /></div>' }, UAlert: { props: ['title', 'description'], template: '<div role="status">{{title}} {{description}}</div>' },
  UIcon: true,
  UModal: { props: ['open'], emits: ['update:open'], template: '<section v-if="open"><slot name="body" /></section>' },
  UCheckbox: true,
  UButton: { props: ['loading', 'to'], template: '<button :disabled="loading"><slot /></button>' },
  UForm: { emits: ['submit'], template: '<form @submit.prevent="$emit(\'submit\')"><slot /></form>' },
  UFormField: { props: ['label'], template: '<label>{{label}}<slot /></label>' },
  UInput: defineComponent({ props: ['modelValue'], emits: ['update:modelValue'], template: '<input :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />' }),
};
const wrappers: ReturnType<typeof mount>[] = [];
function view() {
  const wrapper = mount(Workspace, { props: { allowed: true, identityKey: 'root-A' }, global: { plugins: [createI18n({ legacy: false, locale: 'zh', messages: { zh } })], stubs } });
  wrappers.push(wrapper); return wrapper;
}
beforeEach(() => { vi.clearAllMocks(); sessionStorage.clear(); api.overview.mockResolvedValue(snapshot()); api.policy.mockResolvedValue(policy()); });
afterEach(() => { wrappers.splice(0).forEach(w => w.unmount()); vi.useRealTimers(); });

describe('read-only storage workspace', () => {
  it('shows a persisted unknown archive warning immediately on the default overview without mounting archive actions', async () => {
    beginArchiveRunIntent(sessionStorage, { version: 1, owner: archiveSession, request_id: crypto.randomUUID(), policy_id: crypto.randomUUID(), policy_revision: 3, source_id: 'logs', created_at: new Date().toISOString() });
    const w=view();await flushPromises();expect(w.find('[data-testid="storage-archive-attention"]').exists()).toBe(true);expect(w.find('[data-testid="archive-governance"]').exists()).toBe(false);
  });
  it('restores persisted attention when Root permissions become ready after mount', async () => {
    beginArchiveRunIntent(sessionStorage, { version: 1, owner: archiveSession, request_id: crypto.randomUUID(), policy_id: crypto.randomUUID(), policy_revision: 3, source_id: 'logs', created_at: new Date().toISOString() });
    const w=mount(Workspace,{props:{allowed:false,identityKey:'loading'},global:{plugins:[createI18n({legacy:false,locale:'zh',messages:{zh}})],stubs}});wrappers.push(w);await flushPromises();expect(w.find('[data-testid="storage-archive-attention"]').exists()).toBe(false);await w.setProps({allowed:true,identityKey:'root-ready'});await flushPromises();expect(w.find('[data-testid="storage-archive-attention"]').exists()).toBe(true);
  });
  it('shows scanned partial bytes separately and explains missing sources', async () => {
    const data = snapshot(); data.resources = [
      { id: 'snapshot', name: 'Large snapshot', scope: 'dev', state: 'partial', physical_bytes: null, attributed_bytes: null, partial_bytes: 8192, problem: 'scan_limit_reached' },
      { id: 'logs', name: 'Absent logs', scope: 'dev', state: 'unavailable', physical_bytes: null, attributed_bytes: null, problem: 'source_missing' },
    ] as any;
    api.overview.mockResolvedValue(data);
    const w = view(); await flushPromises(); const text = w.get('[data-testid="storage-resources"]').text();
    expect(text).toContain('已扫描 8 KiB'); expect(text).toContain('未完成，未去重'); expect(text).toContain('目录或文件尚不存在'); expect(text).not.toContain('0 B');
  });
  it('shows real observations and unknown values with no periodic requests', async () => {
    vi.useFakeTimers(); const w = view(); await flushPromises();
    expect(w.text()).toContain('自动清理未启用');
    expect(w.get('[data-testid="storage-powerx-summary"]').text()).toContain('PowerX 总占用：尚未完整统计');
    expect(w.find('[data-testid="storage-disk-metrics"]').exists()).toBe(false);
    expect(w.get('[data-testid="storage-resources"]').text()).not.toContain('Missing logs');
    await w.get('[data-testid="storage-overview-disk"]').trigger('click');
    expect(w.get('[data-testid="storage-resources"]').text()).toContain('Missing logs');
    expect(w.get('[data-testid="storage-resources"]').text()).toContain('未知');
    expect(w.get('[data-testid="storage-disk-metrics"]').text()).toContain('整盘已用（所有软件与文件）');
    expect(w.get('[data-testid="storage-disk-metrics"]').text()).toContain('90 GiB');
    expect(w.find('[data-testid="storage-powerx-summary"]').exists()).toBe(false);
    await vi.advanceTimersByTimeAsync(10 * 60 * 1000);
    expect(api.overview).toHaveBeenCalledTimes(1);
    const signal = api.overview.mock.calls[0]![0]; w.unmount(); expect(signal.aborted).toBe(true);
  });
  it('preserves drafts across sections and clears stale estimates after editing', async () => {
    const w = view(); await flushPromises();
    await w.findAll('[role="tab"]').find(tab => tab.text() === '资源保留策略')!.trigger('click');
    await flushPromises();
    await w.get('[data-testid="storage-advanced-draft"]').trigger('click');
    await w.get('[data-testid="storage-policy-count"]').setValue('30');
    await w.findAll('[role="tab"]')[0]!.trigger('click');
    await w.get('[data-section-select]').setValue('policies');
    expect((w.get('[data-testid="storage-policy-count"]').element as HTMLInputElement).value).toBe('30');
    expect(api.policy).toHaveBeenCalledTimes(1);
  });
  it('retains labelled prior observations on failure and cancels work on identity changes', async () => {
    const w = view(); await flushPromises();
    api.overview.mockRejectedValueOnce(new Error('observer unavailable'));
    await w.find('button').trigger('click'); await flushPromises();
    expect(w.get('[data-testid="storage-error"]').text()).toContain('observer unavailable');
    expect(w.text()).toContain('Registered backup files');
    await w.setProps({ allowed: false, identityKey: 'tenant-B' });
    expect(w.text()).toContain('仅平台 Root');
    expect(w.text()).not.toContain('Registered backup files');
    expect(api.overview).toHaveBeenCalledTimes(2);
  });
  it('does not publish an old identity response after access is revoked', async () => {
    let resolve!: (value: ReturnType<typeof snapshot>) => void;
    api.overview.mockImplementationOnce(() => new Promise(r => { resolve = r; }));
    const w = view(); await w.setProps({ allowed: false, identityKey: 'signed-out' });
    resolve(snapshot()); await flushPromises();
    expect(w.text()).not.toContain('Registered backup files');
  });
});
