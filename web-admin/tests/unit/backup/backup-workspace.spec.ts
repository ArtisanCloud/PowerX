import { flushPromises, mount } from '@vue/test-utils';
import { ref } from 'vue';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const service = vi.hoisted(() => ({
  getOverview: vi.fn(), listPolicies: vi.fn(), listJobs: vi.fn(),
  listRestoreDrills: vi.fn(), listAlerts: vi.fn(), enablePolicy: vi.fn(),
  disablePolicy: vi.fn(), triggerJob: vi.fn(), triggerCleanup: vi.fn(),
}));
vi.mock('~/composables/api/services/backupOpsService', () => ({ useBackupOpsService: () => service }));
const access = { canExecute: ref(true), permissionHint: ref(''), loadUserContext: vi.fn() };
vi.mock('~/composables/useOpsAccess', () => ({ useOpsAccess: () => access }));
import BackupPage from '~/pages/ops/backup.vue';

const stubs = {
  UCard: { template: '<div><slot name="header"/><slot/><slot name="footer"/></div>' },
  UButton: { props: ['disabled'], template: '<button :disabled="disabled"><slot/></button>' },
  UBadge: { template: '<span><slot/></span>' },
  UAlert: { props: ['title'], template: '<p>{{ title }}</p>' },
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
  access.canExecute.value = true;
  const policy = { id: 7, name: '开发库备份', enabled: false, interval_hours: 1, schedule: '1h', retention_count: 168, retention_days: 7, retention_mode: 'age_and_count', drill_enabled: false };
  service.getOverview.mockResolvedValue({ policies_enabled: 0, runtime: { source_database: 'powerx_dev', artifact_directory: '/opt/powerx-dev/storage/backups', ready: true, problems: [] } });
  service.listPolicies.mockResolvedValue({ items: [policy] });
  service.listJobs.mockResolvedValue({ items: [], total: 0 });
  service.listRestoreDrills.mockResolvedValue({ items: [] });
  service.listAlerts.mockResolvedValue({ items: [] });
  service.enablePolicy.mockImplementation(async () => { policy.enabled = true; });
});

describe('backup workspace navigation', () => {
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
