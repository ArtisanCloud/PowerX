import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest';
import { defineComponent, reactive } from 'vue';
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils';
import { hostFixture } from './host-fixture';
const mocks = vi.hoisted(() => ({ getSnapshot: vi.fn(), user: null as any }));
vi.mock('~/composables/api/services/hostMonitorService', () => ({ useHostMonitorService: () => ({ getSnapshot: mocks.getSnapshot }) }));
vi.mock('~/stores/user', () => ({ useUserStore: () => mocks.user }));
vi.mock('vue-i18n', async () => { const { ref } = await import('vue'); return { useI18n: () => ({ locale: ref('en') }) }; });
import Workspace from '~/components/monitor/host/HostOverviewWorkspace.vue';
const Button = defineComponent({ props: ['loading', 'disabled', 'icon', 'color', 'variant'], template: '<button :disabled="disabled || loading"><slot /></button>' });
const Card = defineComponent({ template: '<div><slot /></div>' });
const Alert = defineComponent({ props: ['title', 'description', 'variant'], template: '<div role="alert">{{title}} {{description}}</div>' });
let wrapper: VueWrapper;
function create(props = {}) {
  wrapper = mount(Workspace, { props, global: { stubs: { UButton: Button, UCard: Card, UAlert: Alert, UIcon: true } } });
}
beforeEach(() => {
  mocks.getSnapshot.mockReset().mockResolvedValue(hostFixture());
  mocks.user = reactive({ isRoot: true, user: { id: 1 }, currentTenantUuid: 'tenant', contextSessionStamp: { sessionId: 'first' } });
});
afterEach(() => { wrapper?.unmount(); vi.useRealTimers(); });
describe('host snapshot lifecycle', () => {
  it('defaults omitted allowed to the Root session, loads once, navigates without requests or polling, refreshes manually', async () => {
    vi.useFakeTimers(); create(); await flushPromises();
    expect(mocks.getSnapshot).toHaveBeenCalledTimes(1);
    for (const tab of ['compute', 'disks', 'network', 'gpu', 'overview']) {
      await wrapper.get(`[data-testid="host-tab-${tab}"]`).trigger('click');
      expect(wrapper.get('[data-testid="host-panel"]').attributes('data-section')).toBe(tab);
      expect(wrapper.get(`[data-testid="host-tab-${tab}"]`).attributes('aria-selected')).toBe('true');
    }
    await vi.advanceTimersByTimeAsync(120000);
    expect(mocks.getSnapshot).toHaveBeenCalledTimes(1);
    await wrapper.get('[data-testid="host-tab-network"]').trigger('click');
    await wrapper.get('[data-testid="host-refresh"]').trigger('click'); await flushPromises();
    expect(mocks.getSnapshot).toHaveBeenCalledTimes(2);
    expect(wrapper.get('[data-testid="host-panel"]').attributes('data-section')).toBe('network');
    expect(wrapper.text()).toContain('eth0');
  });
  it('retains the sample and timestamp on failure, marks stale, and recovers', async () => {
    create(); await flushPromises();
    const sampled = wrapper.get('time').attributes('datetime');
    mocks.getSnapshot.mockRejectedValueOnce(new Error('503'));
    await wrapper.get('[data-testid="host-refresh"]').trigger('click'); await flushPromises();
    expect(wrapper.get('[data-testid="host-error"]').text()).toContain('Unable');
    expect(wrapper.get('[data-testid="host-stale"]').text()).toContain('Stale');
    expect(wrapper.text()).toContain('23.5%');
    expect(wrapper.get('time').attributes('datetime')).toBe(sampled);
    await wrapper.get('[data-testid="host-tab-network"]').trigger('click');
    expect(wrapper.text()).toContain('eth0');
    await wrapper.get('[data-testid="host-refresh"]').trigger('click'); await flushPromises();
    expect(wrapper.find('[data-testid="host-error"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="host-stale"]').exists()).toBe(false);
  });
  it('keeps disk pressure global and root ahead of full tmpfs, with df denominator excluding reserved space', async () => {
    create(); await flushPromises();
    const panel = wrapper.get('[data-testid="host-panel"]').text();
    expect(panel).toContain('Disks /'); expect(panel).toContain('95.0%');
    expect(wrapper.get('[data-testid="host-disk-warning"]').text()).toContain('/ 95.0%');
    expect(wrapper.get('[data-testid="host-disk-warning"]').text()).not.toContain('/run');
    await wrapper.get('[data-testid="host-tab-gpu"]').trigger('click');
    expect(wrapper.get('[data-testid="host-disk-warning"]').exists()).toBe(true);
    expect(wrapper.text()).toContain('does not indicate the absence');
  });
  it('makes no request and exposes no metrics without Root access', async () => {
    mocks.user.isRoot = false; create(); await flushPromises();
    expect(mocks.getSnapshot).not.toHaveBeenCalled();
    expect(wrapper.text()).toContain('Root users only');
    expect(wrapper.find('[data-testid="host-refresh"]').exists()).toBe(false);
  });
  it('aborts and ignores pending responses after access revocation', async () => {
    let resolve!: (value: ReturnType<typeof hostFixture>) => void;
    mocks.getSnapshot.mockReturnValueOnce(new Promise(r => { resolve = r; }));
    create(); await flushPromises();
    const signal = mocks.getSnapshot.mock.calls[0]![0] as AbortSignal;
    mocks.user.isRoot = false; await flushPromises(); expect(signal.aborted).toBe(true);
    resolve(hostFixture()); await flushPromises(); expect(wrapper.text()).not.toContain('fixture-host');
  });
  it('clears old data on new sessions even with an unchanged explicit identity prop', async () => {
    create({ identityKey: 'same-user-and-tenant' }); await flushPromises();
    mocks.getSnapshot.mockRejectedValueOnce(new Error('new session failed'));
    mocks.user.contextSessionStamp.sessionId = 'second'; await flushPromises();
    expect(mocks.getSnapshot).toHaveBeenCalledTimes(2);
    expect(wrapper.text()).not.toContain('fixture-host');
    expect(wrapper.find('[data-testid="host-stale"]').exists()).toBe(false);
  });
  it('aborts collection when leaving the workspace', async () => {
    mocks.getSnapshot.mockReturnValueOnce(new Promise(() => {})); create(); await flushPromises();
    const signal = mocks.getSnapshot.mock.calls[0]![0] as AbortSignal;
    wrapper.unmount(); expect(signal.aborted).toBe(true);
  });
});

it('renders soft alerts and localized scope and GPU diagnostics', async () => {
  create(); await flushPromises();
  for (const alert of wrapper.findAllComponents(Alert)) expect(alert.props('variant')).toBe('soft');
  expect(wrapper.text()).toContain('Resources visible to the operating system');
  expect(wrapper.text()).toContain('does not indicate the absence of a GPU');
  expect(wrapper.text()).not.toContain('nvidia-smi unavailable');
  expect(wrapper.text()).not.toContain('OS-visible resources');
});

it('does not warn on full simulator images and reveals them only in all mounts without refetching', async () => {
  const fixture = hostFixture();
  fixture.disks = [
    { mount_point: '/', filesystem: 'apfs', status: 'ok', total_bytes: 100, used_bytes: 80, available_bytes: 20 },
    { mount_point: '/Library/Developer/CoreSimulator/Volumes/iOS_21C62', filesystem: 'apfs', status: 'ok', total_bytes: 100, used_bytes: 99, available_bytes: 1 },
    { mount_point: '/Volumes/ReadOnlyImage', filesystem: 'apfs', read_only: true, status: 'ok', total_bytes: 100, used_bytes: 99, available_bytes: 1 },
  ];
  mocks.getSnapshot.mockResolvedValueOnce(fixture);
  create(); await flushPromises();
  expect(wrapper.find('[data-testid="host-disk-warning"]').exists()).toBe(false);
  await wrapper.get('[data-testid="host-tab-disks"]').trigger('click');
  expect(wrapper.text()).not.toContain('iOS_21C62');
  expect(wrapper.text()).not.toContain('/Volumes/ReadOnlyImage');
  await wrapper.get('[data-testid="host-all-mounts"]').trigger('click');
  expect(wrapper.text()).toContain('iOS_21C62');
  expect(wrapper.text()).toContain('/Volumes/ReadOnlyImage');
  expect(wrapper.text()).toContain('excluded from capacity alerts');
  await wrapper.get('[data-testid="host-tab-network"]').trigger('click');
  await wrapper.get('[data-testid="host-tab-disks"]').trigger('click');
  expect(wrapper.text()).toContain('iOS_21C62');
  expect(mocks.getSnapshot).toHaveBeenCalledTimes(1);
});
