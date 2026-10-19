import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils';
import { reactive, ref } from 'vue';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

enableAutoUnmount(afterEach);

const service = vi.hoisted(() => ({
  getOverview: vi.fn(), listPolicies: vi.fn(), listJobs: vi.fn(),
  listRestoreDrills: vi.fn(), listAlerts: vi.fn(), enablePolicy: vi.fn(),
  createOneTime: vi.fn(), downloadJob: vi.fn(), disablePolicy: vi.fn(), triggerJob: vi.fn(), triggerCleanup: vi.fn(),
  getJob: vi.fn(), getRestoreDrill: vi.fn(),
}));
const route = reactive({ path: '/ops/backup', query: {} as Record<string, string> });
vi.mock('~/composables/api/services/backupOpsService', () => ({ useBackupOpsService: () => service }));
const access = { canExecute: ref(true), isRoot: ref(false), permissionHint: ref(''), loadUserContext: vi.fn() };
vi.mock('~/composables/useOpsAccess', () => ({ useOpsAccess: () => access }));
import BackupPage from '~/pages/ops/backup.vue';

const stubs = { UIcon: true, SystemSnapshotsPanel: true, RestoreCopiesPanel: true,
  UCard: { template: '<div><slot name="header"/><slot/><slot name="footer"/></div>' },
  UButton: { props: ['disabled'], template: '<button :disabled="disabled"><slot/></button>' },
  UBadge: { template: '<span><slot/></span>' },
  UAlert: { props: ['title'], template: '<div>{{ title }}<slot name="actions"/></div>' },
  UInput: true, UFormField: true, USelect: true, UCheckbox: true, UForm: true,
  UTabs: {
    props: ['items', 'modelValue'], emits: ['update:modelValue'],
    template: '<div><button v-for="item in items" :key="item.value" @click="$emit(\'update:modelValue\',item.value)">{{item.label}}</button><slot :name="items.find(item => item.value===modelValue)?.slot"/></div>',
  },
  UModal: { props: ['open'], template: '<div v-if="open"><slot name="body"/><slot name="footer"/></div>' },
};
const page = () => mount(BackupPage, { global: { stubs } });

beforeEach(() => {
  vi.clearAllMocks();
  vi.stubGlobal('useI18n', () => ({ t: (key: string) => key, locale: ref('zh') }));
  route.query = {};
  vi.stubGlobal('useRoute', () => route);
  vi.stubGlobal('navigateTo', vi.fn(async (to) => { route.query = to.query; }));
  access.canExecute.value = true; access.isRoot.value = false;
  const policy = { id: 7, name: '开发库备份', enabled: false, interval_hours: 1, schedule: '1h', retention_count: 168, retention_days: 7, retention_mode: 'age_and_count', drill_enabled: false };
  service.getOverview.mockResolvedValue({ policies_enabled: 0, runtime: { source_database: 'powerx_dev', artifact_directory: '/opt/powerx-dev/storage/backups', ready: true, problems: [] } });
  service.listPolicies.mockResolvedValue({ items: [policy] });
  service.listJobs.mockResolvedValue({ items: [], total: 0 });
  service.listRestoreDrills.mockResolvedValue({ items: [] });
  service.listAlerts.mockResolvedValue({ items: [] });
  service.enablePolicy.mockImplementation(async () => { policy.enabled = true; });
});

describe('backup workspace navigation', () => {
  it('opens a linked historical task directly even when it is absent from the first list page', async () => {
    access.isRoot.value = true;
    route.query = { job_id: '798' };
    service.getJob.mockResolvedValue({ id: 798, status: 'failed', error_message: 'historical backup failed' });
    const wrapper = page(); await flushPromises();
    expect(service.getJob).toHaveBeenCalledWith('798');
    expect(wrapper.get('[data-section="files"]').attributes('style') || '').not.toContain('display: none');
    expect(wrapper.text()).toContain('historical backup failed');
    expect(wrapper.text()).not.toContain('backupCenter.workspace.availableHint');
    expect(wrapper.find('[data-testid="backup-job-views"]').exists()).toBe(false);
    await wrapper.get('[data-testid="backup-linked-task"] button').trigger('click');
    await flushPromises();
    expect(wrapper.find('[data-testid="backup-linked-task"]').exists()).toBe(false);
    wrapper.unmount();
  });
  it('opens the linked restore report and replaces it when the route changes', async () => {
    route.query = { drill_id: '99' };
    service.getRestoreDrill.mockResolvedValue({ id: 99, source_job_id: 798, status: 'failed', report_uri: 'restore failed report' });
    const wrapper = page(); await flushPromises();
    expect(service.getRestoreDrill).toHaveBeenCalledWith('99');
    expect(wrapper.get('[data-section="verification"]').attributes('style') || '').not.toContain('display: none');
    const report = wrapper.get('[data-section="verification"] details');
    expect(report.attributes('open')).toBeDefined();
    expect(report.text()).toContain('restore failed report');
    service.getJob.mockRejectedValue({statusCode:404});
    route.query = { job_id: '800' }; await flushPromises();
    expect(wrapper.get('[data-testid="backup-linked-task"]').text()).toContain('backupCenter.taskLink.unavailable');
    expect(wrapper.text()).not.toContain('restore failed report');
    wrapper.unmount();
  });
  it('rejects ambiguous links without requesting a task', async () => {
    route.query = { job_id: '798', drill_id: '99' };
    const wrapper = page(); await flushPromises();
    expect(wrapper.get('[data-testid="backup-linked-task"]').text()).toContain('backupCenter.taskLink.unavailable');
    expect(service.getJob).not.toHaveBeenCalled();
    expect(service.getRestoreDrill).not.toHaveBeenCalled();
    wrapper.unmount();
  });
  it('disables policy-free full-database export for non-root operators', async () => {
    const wrapper=page();await flushPromises();
    const create=wrapper.findAll('button').find(b=>b.text()==='backupCenter.onDemand.createDatabase')!;
    expect(create.attributes('disabled')).toBeDefined();
    await create.trigger('click');
    expect(service.createOneTime).not.toHaveBeenCalled();
    wrapper.unmount();
  });
  it('creates a one-time database backup with no policy and opens its file list', async () => {
    access.isRoot.value = true;
    service.listPolicies.mockResolvedValue({items:[]});
    service.createOneTime.mockResolvedValue({id:901,policy_id:0,status:'success'});
    const wrapper = page(); await flushPromises();
    wrapper.findComponent(stubs.UTabs).vm.$emit('update:modelValue','operations'); await flushPromises();
    const once = wrapper.findAll('button').find(b=>b.text()==='backupCenter.onDemand.createDatabase')!;
    expect(once.attributes('disabled')).toBeUndefined();
    await once.trigger('click'); expect(service.createOneTime).not.toHaveBeenCalled();
    await wrapper.findAll('button').find(b=>b.text()==='backupCenter.confirm')!.trigger('click');
    await flushPromises();
    expect(service.createOneTime).toHaveBeenCalledTimes(1);
    expect(service.triggerJob).not.toHaveBeenCalled();
    expect(service.enablePolicy).not.toHaveBeenCalled();
    expect(wrapper.get('[data-section="files"]').attributes('style')).not.toContain('display: none');
    expect(wrapper.text()).toContain('backupCenter.onDemand.databaseReady');
    wrapper.unmount();
  });
  it('starts with operations and separates runtime settings and instructions', async () => {
    const wrapper = page(); await flushPromises();
    expect(wrapper.find('[data-testid="backup-operations"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="backup-guide"]').exists()).toBe(false);
    expect(wrapper.text()).not.toContain('backupCenter.policySteps');
    const tabs = wrapper.findComponent(stubs.UTabs);
    tabs.vm.$emit('update:modelValue', 'configuration'); await flushPromises();
    expect(wrapper.find('[data-testid="backup-configuration"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="backup-operations"]').exists()).toBe(false);
    expect(wrapper.text()).toContain('/opt/powerx-dev/storage/backups');
    tabs.vm.$emit('update:modelValue', 'guide'); await flushPromises();
    expect(wrapper.find('[data-testid="backup-guide"]').exists()).toBe(true);
    expect(wrapper.text()).not.toContain('backupCenter.workspace.enableSchedule');
    expect(service.triggerJob).not.toHaveBeenCalled();
  });

  it('uses a vertical task view and does not stack policies under backup files', async () => {
    const wrapper = mount(BackupPage, {attachTo:document.body, global:{stubs}}); await flushPromises();
    expect(wrapper.get('[role="tablist"]').attributes('aria-orientation')).toBe('vertical');
    expect(wrapper.get('[data-section="export"]').isVisible()).toBe(true);
    expect(wrapper.get('[data-section="policies"]').isVisible()).toBe(false);
    await wrapper.findAll('[role="tab"]').find(b => b.text() === 'backupCenter.onDemand.advanced')!.trigger('click');
    expect(wrapper.get('[data-section="policies"]').isVisible()).toBe(true);
    expect(wrapper.get('[data-section="files"]').isVisible()).toBe(false);
    expect(service.triggerJob).not.toHaveBeenCalled();
    expect(service.triggerCleanup).not.toHaveBeenCalled();
    wrapper.unmount();
  });

  it('defaults to registered backups and resets pagination when opening execution history', async () => {
	service.listJobs.mockResolvedValue({ items: [], total: 42 });
	const wrapper = page(); await flushPromises();
	expect(service.listJobs).toHaveBeenLastCalledWith({ view: 'available', page: 1, pageSize: 20 });
	await wrapper.findAll('button').find(b => b.text() === 'backupCenter.next')!.trigger('click');
	await flushPromises();
	expect(service.listJobs).toHaveBeenLastCalledWith({ view: 'available', page: 2, pageSize: 20 });
	const views = wrapper.get('[data-testid="backup-job-views"]');
	await views.findAll('button').find(b => b.text() === 'backupCenter.workspace.executionHistory')!.trigger('click');
	await flushPromises();
	expect(service.listJobs).toHaveBeenLastCalledWith({ view: 'history', page: 1, pageSize: 20 });
	expect(wrapper.text()).toContain('backupCenter.workspace.historyHint');
	await views.findAll('button').find(b => b.text() === 'backupCenter.workspace.availableBackups')!.trigger('click');
	await flushPromises();
	expect(service.listJobs).toHaveBeenLastCalledWith({ view: 'available', page: 1, pageSize: 20 });
	expect(service.triggerCleanup).not.toHaveBeenCalled();
  });

  it('clears the previous list when switching history fails', async () => {
    service.listJobs.mockResolvedValueOnce({items:[{id:99,status:'success',storage_uri:'file:///backup/a.dump'}],total:1});
    const wrapper = page(); await flushPromises();
    service.listJobs.mockRejectedValueOnce(new Error('history unavailable'));
    await wrapper.get('[data-testid="backup-job-views"]').findAll('button').find(b => b.text() === 'backupCenter.workspace.executionHistory')!.trigger('click');
    await flushPromises();
    expect(wrapper.text()).toContain('history unavailable');
    expect(wrapper.text()).not.toContain('a.dump');
  });

  it('enables the actual selected policy and updates its visible schedule state', async () => {
    const wrapper = page(); await flushPromises();
    const policy = wrapper.get('[data-testid="backup-policy"]');
    await policy.findAll('button').find(button => button.text() === 'backupCenter.workspace.enableSchedule')!.trigger('click');
    await flushPromises();
    expect(service.enablePolicy).toHaveBeenCalledWith(7);
    expect(wrapper.text()).toContain('backupCenter.workspace.scheduleOn');
    expect(service.triggerJob).not.toHaveBeenCalled();
  });

  it('preserves execution restrictions for read-only operators', async () => {
    access.canExecute.value = false;
    const wrapper = page(); await flushPromises();
    expect(wrapper.get('[data-testid="backup-policy"]').findAll('button').every(button => button.attributes('disabled') !== undefined)).toBe(true);
    expect(service.enablePolicy).not.toHaveBeenCalled();
    expect(service.triggerJob).not.toHaveBeenCalled();
  });
});
