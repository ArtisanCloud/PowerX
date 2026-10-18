import { mount, flushPromises } from '@vue/test-utils';
import { emitBackupEvent } from '../../support/backup-events-stub';
vi.mock('~/composables/useBackupEvents', () => import('../../support/backup-events-stub'));
import { ref } from 'vue';
import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest';
const service = vi.hoisted(() => ({ overview: vi.fn(), createOneTime: vi.fn(), savePolicy: vi.fn(), saveTarget: vi.fn(), run: vi.fn(), retry: vi.fn(), protect: vi.fn(), restore: vi.fn(), cleanup: vi.fn(), download: vi.fn(), removeRestore: vi.fn(), testTarget: vi.fn() }));
vi.mock('~/composables/api/services/archiveOpsService', () => ({ useArchiveOpsService: () => service }));
import Panel from '~/components/ops/backup/ResourceArchivesPanel.vue';
const stubs = { UIcon: true,
 UCard: { template: '<div><slot name="header"/><slot/></div>' },
 UButton: { props: ['disabled', 'loading'], template: '<button :disabled="disabled || loading"><slot/></button>' },
 UBadge: { template: '<span><slot/></span>' }, UAlert: { props: ['title'], template: '<p>{{title}}</p>' },
 UModal: { props: ['open'], emits: ['update:open'], template: '<div v-if="open"><slot name="body"/><slot name="footer"/></div>' },
 UInput: { props: ['modelValue'], emits: ['update:modelValue'], template: '<input :value="modelValue" @input="$emit(\'update:modelValue\',$event.target.value)"/>' },
 USelect: true, UFormField: { template: '<label><slot/></label>' }, UCheckbox: true,
 UForm: { emits: ['submit'], template: '<form @submit.prevent="$emit(\'submit\')"><slot/></form>' },
};
const policy = { id: 'policy', name: '媒体备份', source_id: 'media', interval_minutes: 60, retention_days: 7, retention_count: 168, remote_retention_days: 90, remote_retention_count: 2160, include_database: true, enabled: false, target_id: '' };
const job = { id: 'job', policy_id: 'policy', source_id: 'media', kind: 'media', status: 'success', phase: 'completed', file_count: 2, size_bytes: 10, new_bytes: 0, created_at: '2026-10-11T00:00:00Z', remote_state: 'failed', remote_error: 'connection unavailable', protected: false, warnings: [] };
const overview = () => ({ instance: 'test', directory: '/stable/archives', sources: [{ id: 'media', name: '媒体来源', kind: 'media', directory: '/media', ready: true }], credentials: [], policies: [policy], targets: [], jobs: [job], restores: [], pending_transfers: 1 });
const wrappers: ReturnType<typeof mount>[] = [];
const panel = () => { const w = mount(Panel, { attachTo: document.body, global: { stubs } }); wrappers.push(w); return w; };
const button = (w: ReturnType<typeof mount>, key: string) => w.findAll('button').find(b => b.text() === `backupCenter.${key}`)!;
beforeEach(() => { vi.clearAllMocks(); vi.stubGlobal('useI18n', () => ({ t: (key: string) => key, locale: ref('zh') })); service.overview.mockResolvedValue(overview()); service.run.mockResolvedValue(job); });
afterEach(() => { wrappers.splice(0).forEach(w => w.unmount()); vi.useRealTimers(); });
describe('media and remote archive operations', () => {
 it('creates a media-only export with no saved policy and preserves its retry key', async () => {
   service.overview.mockResolvedValue({...overview(),policies:[],jobs:[],pending_transfers:0});
   service.createOneTime.mockRejectedValueOnce(new Error('response lost')).mockResolvedValue({...job,one_time:true,policy_id:'',remote_state:'not_requested'});
   const w=panel();await flushPromises();
   await button(w,'onDemand.createMedia').trigger('click');
   const form=w.get('#one-time-media-form');
   await form.trigger('submit');await flushPromises();
   expect(w.text()).toContain('response lost');
   await form.trigger('submit');await flushPromises();
   expect(service.createOneTime.mock.calls[0]).toEqual(service.createOneTime.mock.calls[1]);
   expect(service.createOneTime.mock.calls[0]![0]).toBe('media');
   expect(service.savePolicy).not.toHaveBeenCalled();expect(service.run).not.toHaveBeenCalled();
   expect(w.get('[data-section="jobs"]').isVisible()).toBe(true);
 });
 it('shows source paths, independent retention and remote failure explicitly', async () => { const w = panel(); await flushPromises(); expect(w.text()).toContain('/stable/archives'); expect(w.text()).toContain('/media'); expect(w.text()).toContain('connection unavailable'); expect(w.text()).toContain('backupCenter.archives.credentialsMissing'); });
 it('keeps jobs, policies and sources in separate vertical panels', async () => {
   const w = panel(); await flushPromises();
   expect(w.get('[data-section="export"]').isVisible()).toBe(true);
   expect(w.get('[data-section="jobs"]').isVisible()).toBe(false);
   expect(w.get('[data-section="policies"]').isVisible()).toBe(false);
   expect(w.get('[data-section="sources"]').isVisible()).toBe(false);
   await w.findAll('[role="tab"]').find(b => b.text() === 'backupCenter.onDemand.advanced')!.trigger('click');
   expect(w.get('[data-section="jobs"]').isVisible()).toBe(false);
   expect(w.get('[data-section="policies"]').isVisible()).toBe(true);
   expect(service.run).not.toHaveBeenCalled();
 });
 it('enables a specific saved policy without silently starting a backup', async () => { const w = panel(); await flushPromises(); await button(w, 'workspace.enableSchedule').trigger('click'); await flushPromises(); expect(service.savePolicy).toHaveBeenCalledWith({ ...policy, enabled: true }); expect(service.run).not.toHaveBeenCalled(); });
 it('requires run confirmation and preserves the idempotency key across an uncertain retry', async () => { service.run.mockRejectedValueOnce(new Error('network lost')); const w = panel(); await flushPromises(); await button(w, 'archives.runNow').trigger('click'); expect(service.run).not.toHaveBeenCalled(); await button(w, 'confirm').trigger('click'); await flushPromises(); await button(w, 'confirm').trigger('click'); await flushPromises(); expect(service.run.mock.calls[0]![1]).toEqual(service.run.mock.calls[1]![1]); });
 it('retries the failed remote job without creating another snapshot', async () => { const w = panel(); await flushPromises(); await button(w, 'archives.retry').trigger('click'); await flushPromises(); expect(service.retry).toHaveBeenCalledWith('job'); expect(service.run).not.toHaveBeenCalled(); });
 it('does not offer restoration for SQL logs', async () => { service.overview.mockResolvedValue({ ...overview(), jobs: [{ ...job, kind: 'postgresql_logs' }] }); const w = panel(); await flushPromises(); expect(button(w, 'archives.restore')).toBeUndefined(); });
 it('refreshes active jobs from push without timed requests and unsubscribes on unmount', async () => { vi.useFakeTimers(); service.overview.mockResolvedValue({ ...overview(), jobs: [{ ...job, status: 'running' }] }); const w = panel(); await flushPromises(); await w.findAll('[role="tab"]').find(b=>b.text()==='backupCenter.archives.sources')!.trigger('click'); await vi.advanceTimersByTimeAsync(30000); expect(service.overview).toHaveBeenCalledTimes(1); emitBackupEvent('archives'); await flushPromises(); expect(service.overview).toHaveBeenCalledTimes(2); w.unmount(); emitBackupEvent('archives'); await vi.advanceTimersByTimeAsync(10000); expect(service.overview).toHaveBeenCalledTimes(2); });
});
