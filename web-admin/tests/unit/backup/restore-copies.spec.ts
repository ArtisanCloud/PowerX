import { mount, flushPromises } from '@vue/test-utils';
import { ref } from 'vue';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
const service = vi.hoisted(() => ({ list: vi.fn(), importFile: vi.fn(), plan: vi.fn(), create: vi.fn(), connection: vi.fn(), remove: vi.fn() }));
vi.mock('~/composables/api/services/restoreCopyService', () => ({ useRestoreCopyService: () => service }));
import Panel from '~/components/ops/backup/RestoreCopiesPanel.vue';
import SecretInput from '~/components/SecretInput.vue';

const stubs = {
  SecretInput,
  UCard: { template: '<div><slot name="header"/><slot/></div>' },
  UButton: { props: ['disabled', 'loading'], template: '<button :disabled="disabled || loading"><slot/></button>' },
  UBadge: { template: '<span><slot/></span>' },
  UAlert: { props: ['title', 'description'], template: '<p>{{title}} {{description}}</p>' },
  UModal: { props: ['open'], emits: ['update:open'], template: '<div v-if="open"><slot name="body"/><slot name="footer"/></div>' },
  UFormField: { template: '<label><slot/></label>' },
  UInput: { props: ['modelValue', 'type'], emits: ['update:modelValue'], template: '<input :type="type" :value="modelValue" @input="$emit(\'update:modelValue\',$event.target.value)"/>' },
  UTextarea: true,
  USelect: { props: ['items', 'modelValue'], emits: ['update:modelValue'], template: '<select :value="modelValue" @change="$emit(\'update:modelValue\',$event.target.value)"><option v-for="item in items" :value="item.value">{{item.label}}</option></select>' },
  UCheckbox: { props: ['modelValue', 'label'], emits: ['update:modelValue'], template: '<label><input type="checkbox" :checked="modelValue" @change="$emit(\'update:modelValue\',$event.target.checked)"/>{{label}}</label>' },
};
const job = { id: 7, status: 'success', storage_uri: 'file:///backup/a.dump', policy_id: 1, trigger_type: 'manual', operator: 'root' };
const copy = { id: 'a3aaa535-afec-4a9c-88bc-c3ab0eac2cc9', database: 'powerx_restore_copy_a3aaa535afec4a9c88bcc3ab0eac2cc9', current_database: 'original', host: 'localhost', port: 5432, source_name: 'a.dump', source_sha256: 'sha', status: 'success', phase: 'completed', table_count: 428, size_bytes: 100, created_at: '2026-10-11T00:00:00Z', updated_at: '', warnings: [], business_counts: { orders: 19 } };
const plan = { host: 'localhost', port: 5432, current_database: 'original', target_pattern: 'powerx_restore_copy_<UUID>', archive_name: 'a.dump', archive_sha256: 'sha', archive_size: 123, archive_version: '16.13', server_version: '14.15', extensions: [], requires_compatibility_consent: true, ready: true, problems: [] };
const wrappers: ReturnType<typeof mount>[] = [];
const panel = () => { const wrapper = mount(Panel, { props: { jobs: [job] }, global: { stubs } }); wrappers.push(wrapper); return wrapper; };
const button = (wrapper: ReturnType<typeof mount>, label: string) => wrapper.findAll('button').find(b => b.text() === `backupCenter.copies.${label}`)!;
beforeEach(() => {
  vi.clearAllMocks(); vi.stubGlobal('useI18n', () => ({ t: (key: string) => key, locale: ref('zh') }));
  service.list.mockResolvedValue([]); service.plan.mockResolvedValue(plan); service.create.mockResolvedValue(copy); service.remove.mockResolvedValue(undefined);
});
afterEach(() => { wrappers.splice(0).forEach(wrapper => wrapper.unmount()); vi.useRealTimers(); });

describe('restore to a retained separate database', () => {
  it('requires both target and older-server consent before creating a copy', async () => {
    const wrapper = panel(); await flushPromises(); wrapper.vm.open(job); await flushPromises();
    await button(wrapper, 'check').trigger('click'); await flushPromises();
    expect(service.plan).toHaveBeenCalledWith({ source_job_id: '7' });
    expect(wrapper.get('[data-testid="restore-copy-wizard"]').text()).toContain('original');
    expect(button(wrapper, 'start').attributes('disabled')).toBeDefined();
    await wrapper.get('[data-testid="restore-target-confirm"] input').setValue(true);
    expect(button(wrapper, 'start').attributes('disabled')).toBeDefined();
    await wrapper.get('[data-testid="restore-older-consent"] input').setValue(true);
    await button(wrapper, 'start').trigger('click'); await flushPromises();
    expect(service.create).toHaveBeenCalledWith({ source_job_id: '7' }, true, expect.stringMatching(/^[a-f0-9-]{36}$/));
  });

  it('keeps the same idempotency key when retrying an uncertain creation result', async () => {
    service.plan.mockResolvedValue({ ...plan, requires_compatibility_consent: false });
    service.create.mockRejectedValueOnce(new Error('network interrupted'));
    const wrapper = panel(); await flushPromises(); wrapper.vm.open(job); await flushPromises();
    await button(wrapper, 'check').trigger('click'); await flushPromises();
    await wrapper.get('[data-testid="restore-target-confirm"] input').setValue(true);
    await button(wrapper, 'start').trigger('click'); await flushPromises();
    await button(wrapper, 'start').trigger('click'); await flushPromises();
    expect(service.create.mock.calls[0]![2]).toEqual(service.create.mock.calls[1]![2]);
  });

  it('does not create a database when preflight reports missing dependencies', async () => {
    service.plan.mockResolvedValue({ ...plan, ready: false, problems: ['vector unavailable'] });
    const wrapper = panel(); await flushPromises(); wrapper.vm.open(job); await flushPromises();
    await button(wrapper, 'check').trigger('click'); await flushPromises();
    expect(wrapper.text()).toContain('vector unavailable');
    await wrapper.get('[data-testid="restore-target-confirm"] input').setValue(true);
    await wrapper.get('[data-testid="restore-older-consent"] input').setValue(true);
    expect(button(wrapper, 'start').attributes('disabled')).toBeDefined();
    expect(service.create).not.toHaveBeenCalled();
  });

  it('clears connection credentials when the connection dialog closes', async () => {
    service.list.mockResolvedValue([copy]); service.connection.mockResolvedValue({ access_mode: 'direct', host: 'localhost', port: 5432, database: copy.database, username: 'copy-reader', password: 'private-test-password', ssl_mode: 'disable', read_only: true, expires_at: '2026-10-12T00:00:00Z' });
    const wrapper = panel(); await flushPromises(); await button(wrapper, 'connection').trigger('click'); await flushPromises();
    expect(wrapper.get('input[type="password"]').element.value).toBe('private-test-password');
	const writeText = vi.fn().mockResolvedValue(undefined);
	Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } });
	await button(wrapper, 'copyPassword').trigger('click'); await flushPromises();
	expect(writeText).toHaveBeenCalledWith('private-test-password');
    const modal = wrapper.findAllComponents(stubs.UModal).find(m => m.props('open'))!;
    modal.vm.$emit('update:open', false); await flushPromises();
    expect(wrapper.find('input[type="password"]').exists()).toBe(false);
    expect(wrapper.text()).not.toContain('private-test-password');
  });

  it('shows endpoint configuration errors and Trace in the dialog without offering credentials', async () => {
    service.list.mockResolvedValue([copy]);
    service.connection.mockRejectedValue({ data: { message: 'Client endpoint is not configured', error_code: 'RESTORE_CONNECTION_NOT_CONFIGURED', request_id: 'trace-endpoint' } });
    const wrapper = panel(); await flushPromises();
    await button(wrapper, 'connection').trigger('click'); await flushPromises();
    const dialog = wrapper.get('[data-testid="restore-copy-connection"]');
    expect(dialog.text()).toContain('Client endpoint is not configured');
    expect(dialog.text()).toContain('RESTORE_CONNECTION_NOT_CONFIGURED');
    expect(dialog.text()).toContain('trace-endpoint');
    expect(dialog.text()).toContain('backupCenter.copies.accessConfigHint');
    expect(dialog.find('input').exists()).toBe(false);
    expect(button(wrapper, 'copyDSN')).toBeUndefined();
  });

  it('copies the laptop tunnel endpoint while displaying the different server target', async () => {
    service.list.mockResolvedValue([copy]);
    service.connection.mockResolvedValue({ access_mode: 'ssh_tunnel', host: '127.0.0.1', port: 15433, tunnel: { host: '127.0.0.1', port: 25432 }, database: copy.database, username: 'copy-reader', password: 'p@ss/word', ssl_mode: 'disable', read_only: true, expires_at: '2026-10-12T00:00:00Z' });
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } });
    const wrapper = panel(); await flushPromises();
    await button(wrapper, 'connection').trigger('click'); await flushPromises();
    expect(wrapper.get('[data-testid="restore-copy-tunnel"]').text()).toContain('backupCenter.copies.sshTunnelHint');
    await button(wrapper, 'copyDSN').trigger('click'); await flushPromises();
    expect(writeText).toHaveBeenCalledWith(`postgresql://copy-reader:p%40ss%2Fword@127.0.0.1:15433/${copy.database}?sslmode=disable`);
    expect(writeText.mock.calls[0]![0]).not.toContain(':25432/');
  });

  it('does not copy an incomplete endpoint response', async () => {
    service.list.mockResolvedValue([copy]);
    service.connection.mockResolvedValue({ access_mode: 'ssh_tunnel', host: '127.0.0.1', port: 15432, database: copy.database, username: 'copy-reader', password: 'private-test-password', read_only: true });
    const wrapper = panel(); await flushPromises();
    await button(wrapper, 'connection').trigger('click'); await flushPromises();
    expect(button(wrapper, 'copyDSN').attributes('disabled')).toBeDefined();
    expect(wrapper.text()).toContain('backupCenter.copies.invalidConnection');
  });

  it('does not reopen a closed dialog or retain credentials when the response arrives late', async () => {
    service.list.mockResolvedValue([copy]);
    let resolve!: (value: unknown) => void;
    service.connection.mockReturnValueOnce(new Promise(done => { resolve = done; }));
    const wrapper = panel(); await flushPromises();
    await button(wrapper, 'connection').trigger('click'); await flushPromises();
    const modal = wrapper.findAllComponents(stubs.UModal).find(m => m.props('open'))!;
    modal.vm.$emit('update:open', false); await flushPromises();
    resolve({ access_mode: 'direct', host: 'localhost', port: 5432, database: copy.database, username: 'copy-reader', password: 'private-late-password', read_only: true });
    await flushPromises();
    expect(wrapper.find('[data-testid="restore-copy-connection"]').exists()).toBe(false);
    expect(wrapper.find('input[type="password"]').exists()).toBe(false);
  });

  it('requires the exact new database name and never submits the original database for deletion', async () => {
    service.list.mockResolvedValue([copy]); const wrapper = panel(); await flushPromises();
    await button(wrapper, 'delete').trigger('click'); await flushPromises();
    await wrapper.get('input').setValue(copy.current_database); expect(wrapper.findAll('button').at(-1)!.attributes('disabled')).toBeDefined();
    await wrapper.get('input').setValue(copy.database); await wrapper.findAll('button').at(-1)!.trigger('click'); await flushPromises();
    expect(service.remove).toHaveBeenCalledWith(copy.id, copy.database);
  });

  it('polls active jobs and releases polling on unmount', async () => {
    vi.useFakeTimers(); service.list.mockResolvedValue([{ ...copy, status: 'running', phase: 'restoring' }]);
    const wrapper = panel(); await flushPromises(); expect(service.list).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(2000); expect(service.list).toHaveBeenCalledTimes(2);
    wrapper.unmount(); await vi.advanceTimersByTimeAsync(10000); expect(service.list).toHaveBeenCalledTimes(2);
  });

  it('keeps a newly created task when an older list response arrives late', async () => {
    let resolveOld!: (items: unknown[]) => void;
    service.list.mockReturnValueOnce(new Promise(resolve => { resolveOld = resolve; }));
    service.plan.mockResolvedValue({ ...plan, requires_compatibility_consent: false });
    const wrapper = panel(); wrapper.vm.open(job); await flushPromises();
    await button(wrapper, 'check').trigger('click'); await flushPromises();
    await wrapper.get('[data-testid="restore-target-confirm"] input').setValue(true);
    await button(wrapper, 'start').trigger('click'); await flushPromises();
    resolveOld([]); await flushPromises();
    expect(wrapper.get('[data-testid="restore-copy"]').text()).toContain(copy.database);
  });
});
