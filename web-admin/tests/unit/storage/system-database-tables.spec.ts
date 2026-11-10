import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { mount, flushPromises } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import zh from '../../../i18n/locales/zh.json';
import Panel from '~/components/monitor/storage/SystemDatabaseTablesPanel.vue';
import { decodeDatabaseTableInventory } from '~/composables/api/services/storageDatabaseTablesService';
import { tableInventory } from './database-tables-fixture';

const api = vi.hoisted(() => ({ inventory: vi.fn(), retention: vi.fn() }));
vi.mock('~/composables/api/services/storageDatabaseTablesService', async importOriginal => ({ ...await importOriginal<object>(), useStorageDatabaseTablesService: () => api }));
const stubs = {
  UIcon: true,
  UAlert: { props: ['title', 'description'], template: '<section>{{title}} {{description}}</section>' },
  UCard: { template: '<div><slot/></div>' },
  UButton: { props: ['disabled', 'loading'], template: '<button :disabled="disabled || loading"><slot/></button>' },
  UModal: { props: ['open'], template: '<section v-if="open"><slot name="body"/><slot name="footer"/></section>' },
  UInput: { props: ['modelValue', 'size'], emits: ['update:modelValue'], template: '<input :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)"/>' },
};
let wrapper: ReturnType<typeof mount>;
beforeEach(() => {
  api.inventory.mockReset().mockResolvedValue(tableInventory());
  api.retention.mockReset();
  wrapper = mount(Panel, { global: { plugins: [createI18n({ legacy: false, locale: 'zh', messages: { zh } })], stubs } });
});
afterEach(() => { wrapper.unmount(); vi.useRealTimers(); });

it('shows real bytes and date boundaries while preserving absent and protected tables', async () => {
  await flushPromises();
  expect(wrapper.text()).toContain('能力调用追踪');
  expect(wrapper.text()).toContain('2026');
  expect(wrapper.text()).toContain('created_at');
  expect(wrapper.text()).toContain('当前数据库未建表');
  expect(wrapper.text()).toContain('受保护');
  expect(wrapper.find('[data-table-id="skills_execution_traces"]').text()).toContain('—');
  await wrapper.find('[data-table-id="capability_registry_invocation_traces"] [data-table-details]').trigger('click');
  expect(wrapper.find('[data-testid="system-table-detail"]').text()).toContain('TOAST');
  expect(wrapper.find('[data-testid="system-table-detail"]').text()).toContain('4 KiB');
  expect(wrapper.find('[data-testid="system-table-detail"]').text()).toContain('12,345');
});

it('preserves filters and last data during push updates and errors, never polls and aborts on leave', async () => {
  await flushPromises();
  await wrapper.find('select').setValue('protected');
  await wrapper.find('input').setValue('iam_authorization');
  await wrapper.setProps({ revision: 1 }); await flushPromises();
  expect((wrapper.find('input').element as HTMLInputElement).value).toBe('iam_authorization');
  expect(wrapper.findAll('tbody tr')).toHaveLength(1);
  api.inventory.mockRejectedValueOnce(new Error('connection lost'));
  await wrapper.findAll('button').find(b => b.text() === '刷新观测')!.trigger('click'); await flushPromises();
  expect(wrapper.text()).toContain('connection lost');
  expect(wrapper.text()).toContain('iam_authorization_segments');
  vi.useFakeTimers(); const calls = api.inventory.mock.calls.length;
  await vi.advanceTimersByTimeAsync(3600000); expect(api.inventory).toHaveBeenCalledTimes(calls);
  const signal = api.inventory.mock.calls.at(-1)![0]; wrapper.unmount(); expect(signal.aborted).toBe(true);
});

it('rejects malformed time ranges, missing sizes and inconsistent allocation rather than showing zero', () => {
  expect(decodeDatabaseTableInventory({ data: tableInventory() }).items).toHaveLength(3);
  for (const change of [{ total_bytes: undefined }, { total_bytes: 1 }, { oldest_at: '2027-01-01T00:00:00Z' }, { newest_at: null }, { state: 'not_present' }]) {
    const data = tableInventory(); Object.assign(data.items[0]!, change);
    expect(() => decodeDatabaseTableInventory(data)).toThrow('STORAGE_TABLE_INVENTORY_RESPONSE_INVALID');
  }
});

it('keeps pagination across data updates and returns to the first page after filtering', async () => {
  const data = tableInventory(), row = data.items[0]!;
  data.items = Object.keys(zh.storageGov.tables.purposes).map(id => ({ ...row, id }));
  api.inventory.mockResolvedValue(data);
  await wrapper.setProps({ revision: 1 }); await flushPromises();
  expect(wrapper.findAll('tbody tr')).toHaveLength(20);
  await wrapper.findAll('button').find(b => b.text() === '下一页')!.trigger('click');
  const firstID = wrapper.find('tbody tr').attributes('data-table-id');
  await wrapper.setProps({ revision: 2 }); await flushPromises();
  expect(wrapper.find('tbody tr').attributes('data-table-id')).toBe(firstID);
  await wrapper.find('input').setValue('iam_authorization_segments');
  expect(wrapper.findAll('tbody tr')).toHaveLength(1);
  expect(wrapper.find('tbody tr').attributes('data-table-id')).toBe('iam_authorization_segments');
});

it('filters cleanup states and combines them with search/category without writing a policy', async () => {
  await flushPromises();
  const data = tableInventory(), source = data.items[0]!;
  const probe = { ...source, id: 'capability_registry_health_probe_results', cleanup_mechanism: 'typed_policy' as const };
  const cache = { ...probe, id: 'capability_registry_discovery_cache' };
  data.items.push(probe, cache);
  api.inventory.mockResolvedValue(data);
  api.retention.mockImplementation(async (_signal, id) => {
    if (id === cache.id) throw new Error('policy unavailable');
    return { id, revision: 0, policy: { enabled: false, retention_days: 21, batch_limit: 100 }, ready: true, worker_enabled: true, enforcement_allowed: true, state_configured: true, runs: [] };
  });
  await wrapper.setProps({ revision: 1 }); await flushPromises();
  const row = wrapper.find(`[data-table-id="${probe.id}"]`);
  expect(row.find('[data-cleanup-state]').text()).toBe('未开启');
  expect(row.find('[data-retention-window]').text()).toContain('21 天');
  expect(row.find('[data-retention-window]').text()).toContain('尚未生效');
  expect(wrapper.find(`[data-table-id="${cache.id}"] [data-cleanup-state]`).text()).toContain('状态未知');
  await wrapper.find('[data-testid="system-tables-cleanup-filter"]').setValue('disabled');
  await wrapper.find('[data-testid="system-tables-category"]').setValue('observability');
  await wrapper.find('[data-testid="system-tables-search"]').setValue('health_probe');
  expect(wrapper.findAll('tbody tr')).toHaveLength(1);
  await wrapper.setProps({ revision: 2 }); await flushPromises();
  expect(wrapper.findAll('tbody tr')).toHaveLength(1);
  expect((wrapper.find('[data-testid="system-tables-cleanup-filter"]').element as HTMLSelectElement).value).toBe('disabled');
  await wrapper.find('[data-testid="system-tables-reset"]').trigger('click');
  expect(wrapper.findAll('tbody tr')).toHaveLength(5);
});

it('sorts actual bytes and timestamps, leaves missing values last, and retains sorting on push', async () => {
  await flushPromises();
  const data = tableInventory(), source = data.items[0]!;
  data.items.push({ ...source, id: 'event_task_histories', total_bytes: 4096, oldest_at: '2026-09-01T00:00:00Z' });
  api.inventory.mockResolvedValue(data);
  await wrapper.setProps({ revision: 1 }); await flushPromises();
  const ids = () => wrapper.findAll('tbody tr').map(r => r.attributes('data-table-id'));
  await wrapper.find('[data-testid="system-tables-direction"]').trigger('click');
  expect(ids()).toEqual(['event_task_histories', 'iam_authorization_segments', 'capability_registry_invocation_traces', 'skills_execution_traces']);
  await wrapper.find('[data-testid="system-tables-sort"]').setValue('oldest');
  expect(ids().slice(0, 2)).toEqual(['event_task_histories', 'capability_registry_invocation_traces']);
  await wrapper.find('[data-testid="system-tables-direction"]').trigger('click');
  expect(ids().slice(0, 2)).toEqual(['capability_registry_invocation_traces', 'event_task_histories']);
  await wrapper.setProps({ revision: 2 }); await flushPromises();
  expect((wrapper.find('[data-testid="system-tables-sort"]').element as HTMLSelectElement).value).toBe('oldest');
  expect(wrapper.find('[data-testid="system-tables-direction"]').text()).toContain('降序');
  expect(ids()[0]).toBe('capability_registry_invocation_traces');
  expect(ids().at(-1)).toBe('skills_execution_traces');
});

it('compares configured retention days without treating revision counts as days', async () => {
  await flushPromises();
  const data = tableInventory(), source = data.items[0]!;
  data.items = ['ops_storage_policy_revisions', 'ops_storage_samples', 'ops_storage_cleanup_runs'].map(id => ({ ...source, id, cleanup_mechanism: 'metadata_window' as const }));
  api.inventory.mockResolvedValue(data);
  await wrapper.setProps({ revision: 1 }); await flushPromises();
  await wrapper.find('[data-testid="system-tables-sort"]').setValue('retention');
  const ids = () => wrapper.findAll('tbody tr').map(r => r.attributes('data-table-id'));
  expect(ids()).toEqual(['ops_storage_samples', 'ops_storage_cleanup_runs', 'ops_storage_policy_revisions']);
  await wrapper.find('[data-testid="system-tables-direction"]').trigger('click');
  expect(ids()).toEqual(['ops_storage_cleanup_runs', 'ops_storage_samples', 'ops_storage_policy_revisions']);
  expect(wrapper.find('[data-table-id="ops_storage_policy_revisions"] [data-retention-window]').text()).toBe('最近 100 版');
});

it('opens the existing configuration destination without enabling cleanup', async () => {
  await flushPromises();
  await wrapper.find('[data-table-id="capability_registry_invocation_traces"] [data-configure-cleanup]').trigger('click');
  expect(wrapper.emitted('open-retention')).toHaveLength(1);
  expect(wrapper.find('[data-testid="system-table-detail"]').exists()).toBe(false);
});

it('can read both fixed policy states when their shared host document rejects concurrent readers', async () => {
  await flushPromises();
  const data = tableInventory(), source = data.items[0]!;
  data.items = ['capability_registry_health_probe_results', 'capability_registry_discovery_cache'].map(id => ({ ...source, id, cleanup_mechanism: 'typed_policy' as const }));
  let reading = false;
  api.retention.mockImplementation(async (_signal, id) => {
    if (reading) throw new Error('shared host lock busy');
    reading = true; await Promise.resolve(); reading = false;
    return { id, revision: 0, policy: { enabled: false, retention_days: 30, batch_limit: 100 }, ready: true, worker_enabled: true, enforcement_allowed: true, state_configured: true, runs: [] };
  });
  api.inventory.mockResolvedValue(data);
  await wrapper.setProps({ revision: 1 }); await flushPromises();
  expect(wrapper.findAll('tbody [data-cleanup-state]').map(s => s.text())).toEqual(['未开启', '未开启']);
  expect(api.retention).toHaveBeenCalledTimes(2);
});
