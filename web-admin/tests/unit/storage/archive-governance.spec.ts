import { mount, flushPromises } from '@vue/test-utils';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { createI18n } from 'vue-i18n';
import zh from '../../../i18n/locales/zh.json';
import { emitBackupEvent } from '../../support/backup-events-stub';
vi.mock('~/composables/useBackupEvents', () => import('../../support/backup-events-stub'));
const api = vi.hoisted(() => ({ overview: vi.fn(), catalog: vi.fn(), savePolicy: vi.fn(), saveTarget: vi.fn(), testTarget: vi.fn(), job: vi.fn(), preview: vi.fn(), protect: vi.fn(), download: vi.fn(), run: vi.fn(), migrate: vi.fn() }));
vi.mock('~/composables/api/services/archiveOpsService', () => ({ useArchiveOpsService: () => api }));
import Panel from '~/components/ops/backup/ArchiveGovernanceBody.vue';
const stubs = { UIcon: true,
 UButton: { props: ['disabled', 'loading'], template: '<button :disabled="disabled || loading"><slot/></button>' }, UBadge: { template: '<span><slot/></span>' },
 UAlert: { props: ['title', 'description'], template: '<p>{{title}} {{description}}</p>' },
 UModal: { props: ['open', 'title'], template: '<section v-if="open"><h3>{{title}}</h3><slot name="body"/><slot name="footer"/></section>' },
 UInput: { props: ['modelValue'], emits: ['update:modelValue'], template: '<input :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)"/>' },
 USelect: { props: ['modelValue', 'items'], emits: ['update:modelValue'], template: '<select :value="modelValue" @change="$emit(\'update:modelValue\', $event.target.value)"><option v-for="item in items" :value="item.value">{{item.label}}</option></select>' },
 UFormField: { props: ['label'], template: '<label>{{label}}<slot/></label>' }, UCheckbox: true,
 UForm: { emits: ['submit'], template: '<form @submit.prevent="$emit(\'submit\')"><slot/></form>' },
};
let w: ReturnType<typeof mount>;
let policy: any;
const button = (label: string) => w.findAll('button').find(b => b.text() === label)!;
const tab = async (value: string) => { await w.get(`[data-section-select]`).setValue(value); await flushPromises(); };
beforeEach(() => {
 vi.clearAllMocks(); policy = { id: 'p', name: '日志策略', revision: 2, source_id: 'logs', target_id: 'target', interval_minutes: 5, enabled: false, remote_cleanup_enabled: false, remote_max_bytes: 1024 ** 3, remote_retention_days: 180 };
 api.overview.mockImplementation(async () => ({ instance: 'test', sources: [{ id: 'logs', name: '日志', kind: 'application_logs' }], policies: [policy], targets: [{ id: 'target', name: '独立目标', driver: 's3', endpoint: 'https://archive.invalid', bucket: 'private', prefix: 'host/test', verification_valid: false }], credentials: [], jobs: [], restores: [], pending_transfers: 0 }));
 api.catalog.mockResolvedValue({ items: [{ id: 'historical', policy_id: 'p', kind: 'application_logs', source_id: 'gone', source_file_name: 'server.log.2.gz', status: 'pruned', remote_state: 'success', protected: true, size_bytes: 10, file_count: 1, created_at: new Date().toISOString() }], pagination: { page: 1, pages: 1, total: 1 } });
 api.savePolicy.mockImplementation(async p => { policy = { ...p, revision: p.revision + 1 }; return policy; });
 w = mount(Panel, { global: { plugins: [createI18n({ legacy: false, locale: 'zh', messages: { zh } })], stubs } });
});
afterEach(() => { w.unmount(); vi.useRealTimers(); });
it('never starts transmission on load, save, or failed target verification; enabling requires its own confirmation', async () => {
 await flushPromises(); expect(api.savePolicy).not.toHaveBeenCalled();
 await tab('policies'); await button('配置策略').trigger('click'); await flushPromises();
 await w.get('#log-archive-policy').trigger('submit'); await flushPromises();
 expect(api.savePolicy.mock.calls[0]![0]).toMatchObject({ enabled: false, remote_cleanup_enabled: false });
 await button('开启归档').trigger('click'); expect(api.savePolicy).toHaveBeenCalledTimes(1);
 await button('确认执行').trigger('click'); await flushPromises(); expect(api.savePolicy.mock.calls[1]![0].enabled).toBe(true);
 await tab('targets'); api.testTarget.mockRejectedValueOnce(new Error('target inaccessible'));
 await button('测试写入与取回').trigger('click'); await flushPromises(); expect(w.text()).toContain('target inaccessible'); expect(api.savePolicy).toHaveBeenCalledTimes(2);
});
it('keeps filters and forms through tab and push changes, retrieves source-gone history, and makes no periodic reads', async () => {
 await flushPromises(); vi.useFakeTimers(); await w.get('[data-testid="archive-search"]').setValue('server.log');
 await tab('targets'); await tab('catalog'); expect((w.get('[data-testid="archive-search"]').element as HTMLInputElement).value).toBe('server.log');
 expect(w.get('[data-testid="archive-catalog-item"]').text()).toContain('server.log.2.gz');
 await button('下载并校验').trigger('click'); await flushPromises(); expect(api.download.mock.calls[0]![0]).toBe('historical');
 const count = api.catalog.mock.calls.length; await vi.advanceTimersByTimeAsync(3600000); expect(api.catalog).toHaveBeenCalledTimes(count);
 emitBackupEvent('archives'); await flushPromises(); expect(api.catalog).toHaveBeenCalledTimes(count + 1);
 const signal = api.catalog.mock.calls.at(-1)![1]; w.unmount(); expect(signal.aborted).toBe(true);
});
it('starts a manual archive and migration from their buttons with separate UUID request keys', async () => {
 policy.enabled = true; await flushPromises(); await tab('policies');
 await button('归档一批').trigger('click'); await flushPromises();
 expect(api.run).toHaveBeenCalledWith('p', expect.stringMatching(/^[0-9a-f-]{36}$/), expect.any(AbortSignal));
 await tab('catalog'); await button('迁移副本').trigger('click'); await flushPromises();
 const modal = w.findAll('section').filter(s => s.find('h3').exists() && s.find('h3').text() === '迁移副本').at(-1)!;
 await modal.get('select').setValue('p');
 await button('确认执行').trigger('click'); await flushPromises();
 expect(api.migrate).toHaveBeenCalledWith('historical', 'p', expect.stringMatching(/^[0-9a-f-]{36}$/), expect.any(AbortSignal));
 expect(api.migrate.mock.calls[0]![2]).not.toBe(api.run.mock.calls[0]![1]);
});
