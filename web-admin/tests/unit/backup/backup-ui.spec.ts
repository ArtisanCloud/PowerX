import { mount } from '@vue/test-utils';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import BackupJobTable from '~/components/ops/backup/BackupJobTable.vue';
import BackupRuntimePanel from '~/components/ops/backup/BackupRuntimePanel.vue';
const t = (key: string) => key;
beforeEach(() => vi.stubGlobal('useI18n', () => ({ t })));
const global = { stubs: { UCard: { template: '<div><slot name="header"/><slot/></div>' }, UButton: { props: ['disabled'], template: '<button :disabled="disabled"><slot/></button>' }, UBadge: { template: '<span><slot/></span>' }, UAlert: { props: ['title', 'description'], template: '<div>{{title}} {{description}}</div>' } } };
describe('backup operations', () => {
  const job = { id: 1, policy_id: 2, status: 'success', trigger_type: 'manual', operator: 'root', storage_uri: 'file:///opt/powerx/storage/backups/test.dump', checksum: 'abc', protected: true };
  it('disables write actions for read-only users', () => {
    const wrapper = mount(BackupJobTable, { props: { items: [job], canExecute: false }, global });
    expect(wrapper.findAll('button').every(button => button.attributes('disabled') !== undefined)).toBe(true);
    expect(wrapper.text()).toContain('backupCenter.protected');
  });
  it('shows an expired artifact without offering restore', () => {
    const wrapper = mount(BackupJobTable, { props: { items: [{ ...job, storage_uri: '' }], canExecute: true }, global });
    expect(wrapper.text()).toContain('backupCenter.pruned');
    expect(wrapper.findAll('button')).toHaveLength(0);
  });
  it('emits the actual backup for restore and protection actions', async () => {
    const wrapper = mount(BackupJobTable, { props: { items: [job], canExecute: true }, global });
    await wrapper.findAll('button')[0]!.trigger('click');
    await wrapper.findAll('button')[1]!.trigger('click');
    expect(wrapper.emitted('restore-verify')?.[0]).toEqual([job]);
    expect(wrapper.emitted('protection')?.[0]).toEqual([job]);
  });
  it('shows the server source, directory and dependency errors without inventing defaults', () => {
    const wrapper = mount(BackupRuntimePanel, { props: { enabledCount: 0, runtime: { source_host: 'localhost', source_port: 5432, source_database: 'powerx_pro', artifact_directory: '/opt/powerx/storage/backups', path_template: 'job_<id>_<UTC>.dump', format: 'custom .dump SHA256', scope: 'all_database_schemas', restore_mode: 'new_isolated_database', ready: false, problems: ['pg_dump is unavailable'] } }, global });
    expect(wrapper.text()).toContain('powerx_pro');
    expect(wrapper.text()).toContain('/opt/powerx/storage/backups');
    expect(wrapper.text()).toContain('pg_dump is unavailable');
    const missing = mount(BackupRuntimePanel, { props: { enabledCount: 0 }, global });
    expect(missing.text()).toContain('backupCenter.runtimeMissing');
    expect(missing.text()).not.toContain('powerx_bak');
  });
});
