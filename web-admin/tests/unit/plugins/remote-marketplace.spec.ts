import { mount, flushPromises } from '@vue/test-utils';
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import RemoteMarketplace from '~/components/plugins/RemoteMarketplace.vue';

const api = vi.hoisted(() => ({ catalog: vi.fn(), versions: vi.fn(), install: vi.fn(), list: vi.fn() }));
vi.mock('~/composables/api/services/marketplaceService', () => ({ useMarketplaceService: () => api }));
vi.mock('~/composables/api/services/adminPluginsService', () => ({ useAdminPluginsService: () => ({ list: api.list }) }));
const plugin = { id: 'com.test.market', name: 'Remote Plugin', summary: 'Published remotely', categories: [], owner_dev_id: 'developer', installation: { status: 'not_installed', origin: { type: 'unknown' } } };
const version = { id: 'version-1', plugin_id: plugin.id, semver: '1.2.0', channel: 'stable', min_core: '1.0.0', os: 'darwin', arch: 'arm64', sha256: 'a'.repeat(64), size: 1024 };
const catalog = { inventory_ready: true, connection: { configured: true, download_ready: true, os: 'darwin', arch: 'arm64', core_version: '1.0.0' }, items: [plugin], total: 1, page: 1, size: 12 };
const global = { stubs: {
  UButton: { props: ['disabled', 'loading', 'to'], template: '<button :disabled="disabled || loading"><slot/></button>' },
  UModal: { props: ['open'], template: '<section v-if="open" role="dialog"><slot name="body"/><slot name="footer"/></section>' },
  UAlert: { props: ['title', 'description'], template: '<aside>{{ title }} {{ description }}<slot name="actions"/></aside>' },
  USelect: { props: ['modelValue', 'items'], emits: ['update:modelValue'], template: '<select :value="modelValue" @change="$emit(\'update:modelValue\', $event.target.value)"><option v-for="item in items" :key="item.value" :value="item.value">{{item.label}}</option></select>' },
  UFormField: { template: '<div><slot/></div>' },
  UCheckbox: { props: ['modelValue', 'label', 'disabled'], emits: ['update:modelValue'], template: '<label><input type="checkbox" :checked="modelValue" :disabled="disabled" @change="$emit(\'update:modelValue\', $event.target.checked)"/>{{label}}</label>' },
  UIcon: true, USkeleton: true, UBadge: true, UPagination: true,
} };

beforeEach(() => {
  vi.clearAllMocks();
  for (const [name, value] of Object.entries({ ref, computed, onMounted, onBeforeUnmount })) vi.stubGlobal(name, value);
  vi.stubGlobal('useI18n', () => ({ t: (key: string) => key }));
  api.catalog.mockResolvedValue(structuredClone(catalog));
  api.list.mockResolvedValue([]);
  api.versions.mockResolvedValue({ items: [version], total: 1, page: 1, size: 20 });
  api.install.mockResolvedValue({ installed: { id: plugin.id, version: version.semver, state: 'installed' }, enabled: false });
});
afterEach(() => { vi.unstubAllGlobals(); vi.useRealTimers(); });
const button = (wrapper: ReturnType<typeof mount>, text: string) => wrapper.findAll('button').find(b => b.text() === `plugins.remoteMarket.${text}`)!;

describe('remote marketplace', () => {
  it('shows unconfirmed outcomes and clears the notice after a manual refresh without reinstalling', async () => {
    api.catalog.mockResolvedValueOnce({ ...catalog, connection: { ...catalog.connection, instance: { state: 'registered', pending_reports: 0, blocked_reports: 0, unconfirmed_installs: 1 } } }).mockResolvedValueOnce(structuredClone(catalog));
    const w = mount(RemoteMarketplace, { global }); await flushPromises();
    expect(w.get('[data-testid="marketplace-result-unconfirmed"]').text()).toContain('plugins.remoteMarket.resultUnconfirmed');
    expect(api.catalog).toHaveBeenCalledTimes(1); expect(api.install).not.toHaveBeenCalled();
    await w.findAll('button').find(b => b.text() === 'plugins.presentation.refresh')!.trigger('click'); await flushPromises();
    expect(w.find('[data-testid="marketplace-result-unconfirmed"]').exists()).toBe(false);
    expect(api.catalog).toHaveBeenCalledTimes(2); expect(api.install).not.toHaveBeenCalled(); w.unmount();
  });
  it.each([['credential_rejected','registrationRejected'],['unavailable','registrationUnavailable']])('explains %s without suggesting automatic reenrollment',async(state,label)=>{
    api.catalog.mockResolvedValue({...catalog,connection:{...catalog.connection,download_ready:false,instance:{state}}});
    const w=mount(RemoteMarketplace,{global});await flushPromises();
    expect(w.get('[data-testid="marketplace-registration-notice"]').text()).toContain(`plugins.remoteMarket.${label}`);
    expect(w.text()).not.toContain('plugins.remoteMarket.authHelp');expect(api.install).not.toHaveBeenCalled();w.unmount();
  });
  it('shows the remote catalog while keeping installed metadata separate', async () => {
    api.list.mockResolvedValue([{ id: 'com.local.only', name: 'Local Only', version: '9.0.0' }]);
    const w = mount(RemoteMarketplace, { global }); await flushPromises();
    expect(w.text()).toContain('Remote Plugin'); expect(w.text()).not.toContain('Local Only'); expect(api.list).not.toHaveBeenCalled();
    await button(w, 'viewVersions').trigger('click'); await flushPromises();
    expect(api.versions).toHaveBeenCalledWith(plugin.id, 1);
    await button(w, 'installVersion').trigger('click'); await flushPromises();
    expect(api.install).toHaveBeenCalledExactlyOnceWith(plugin.id, '1.2.0', { enable: true });
    expect(w.find('[role="dialog"]').exists()).toBe(false);
    expect(w.text()).toContain('plugins.remoteMarket.installed'); w.unmount();
  });
  it('shows version failures and supports retry without losing catalog context', async () => {
    api.versions.mockRejectedValueOnce(new Error('Marketplace HTTP 500'));
    const w = mount(RemoteMarketplace, { global }); await flushPromises();
    await button(w, 'viewVersions').trigger('click'); await flushPromises();
    expect(w.text()).toContain('Marketplace HTTP 500'); expect(button(w, 'installVersion').attributes('disabled')).toBeDefined();
    await button(w, 'retry').trigger('click'); await flushPromises();
    expect(w.find('select').element.value).toBe('1.2.0');
    api.install.mockRejectedValueOnce(new Error('SHA256 mismatch'));
    await button(w, 'installVersion').trigger('click'); await flushPromises();
    expect(w.text()).toContain('SHA256 mismatch'); expect(w.find('select').element.value).toBe('1.2.0'); w.unmount();
  });
  it('blocks installation without download authorization or installed-state readback', async () => {
    api.catalog.mockResolvedValue({ ...catalog, connection: { ...catalog.connection, download_ready: false } });
    const w = mount(RemoteMarketplace, { global }); await flushPromises();
    expect(w.text()).toContain('plugins.remoteMarket.authRequired');
    await button(w, 'viewVersions').trigger('click'); await flushPromises();
    expect(button(w, 'installVersion').attributes('disabled')).toBeDefined(); expect(api.install).not.toHaveBeenCalled(); w.unmount();
    api.catalog.mockResolvedValue({ ...catalog, inventory_ready: false, items: [{ ...plugin, installation: { status: 'unknown', origin: { type: 'unknown' } } }] });
    const w2 = mount(RemoteMarketplace, { global }); await flushPromises();
    await button(w2, 'viewVersions').trigger('click'); await flushPromises();
    expect(button(w2, 'installVersion').attributes('disabled')).toBeDefined(); w2.unmount();
  });
  it('blocks a same-ID installation with an unconfirmed publisher', async () => {
    api.catalog.mockResolvedValue({ ...catalog, items: [{ ...plugin, installation: { status: 'source_conflict', version: '9.0.0', origin: { type: 'local', source_id: 'local' } } }] });
    const w = mount(RemoteMarketplace, { global }); await flushPromises();
    expect(w.text()).toContain('plugins.installFlow.providerUnknownShort');
    expect(w.text()).not.toContain('plugins.remoteMarket.installedVersion');
    await button(w, 'viewVersions').trigger('click'); await flushPromises();
    expect(button(w, 'installVersion')).toBeUndefined();
    expect(button(w, 'manageInstalled')).toBeDefined();
    expect(api.list).not.toHaveBeenCalled(); expect(api.install).not.toHaveBeenCalled(); w.unmount();
  });
  it('queries installation filters independently and preserves the remote catalog context', async () => {
    const w = mount(RemoteMarketplace, { global }); await flushPromises();
    const filter = w.findAll('button').find(b => b.text() === 'plugins.installFlow.installedFilter')!;
    await filter.trigger('click'); await flushPromises();
    expect(api.catalog).toHaveBeenLastCalledWith(1, 'installed');
    expect(w.text()).toContain('Remote Plugin'); w.unmount();
  });

  it('replaces a local installation from the same publisher only after explicit confirmation', async () => {
    api.catalog.mockResolvedValue({ ...catalog, items: [{ ...plugin, installation: { status: 'installed', version: '1.0.0', origin: { type: 'local' }, provider: { id: 'developer', authority: 'http://market.test/api/v1' }, revision: 'old-revision', can_replace: true } }] });
    const w = mount(RemoteMarketplace, { global }); await flushPromises();
    await button(w, 'viewVersions').trigger('click'); await flushPromises();
    const replace = w.findAll('button').find(b => b.text() === 'plugins.installFlow.replaceInstall')!;
    expect(replace.attributes('disabled')).toBeDefined();
    await w.findAll('input[type="checkbox"]').at(-1)!.setValue(true);
    await replace.trigger('click'); await flushPromises();
    expect(api.install).toHaveBeenCalledExactlyOnceWith(plugin.id, '1.2.0', { enable: true, replace: true, expected_revision: 'old-revision' });
    w.unmount();
  });
  it('requires separate publisher binding and replacement confirmation for legacy installations', async () => {
    api.catalog.mockResolvedValue({ ...catalog, items: [{ ...plugin, installation: { status: 'source_conflict', version: '1.0.0', origin: { type: 'local' }, provider: { id: 'developer', authority: 'http://market.test/api/v1' }, revision: 'legacy-revision', can_replace: true, reason: 'provider_unconfirmed' } }] });
    const w = mount(RemoteMarketplace, { global }); await flushPromises();
    await button(w, 'viewVersions').trigger('click'); await flushPromises();
    const replace = w.findAll('button').find(b => b.text() === 'plugins.installFlow.replaceInstall')!;
    const checkboxes = w.findAll('input[type="checkbox"]');
    expect(checkboxes).toHaveLength(3);
    await checkboxes[2]!.setValue(true); expect(replace.attributes('disabled')).toBeDefined();
    await checkboxes[1]!.setValue(true); await replace.trigger('click'); await flushPromises();
    expect(api.install).toHaveBeenCalledExactlyOnceWith(plugin.id, '1.2.0', { enable: true, replace: true, expected_revision: 'legacy-revision', confirm_legacy_provider: true });
    w.unmount();
  });
  it('keeps replacement context and links to drain details when active tenant instances block installation', async () => {
    api.catalog.mockResolvedValue({ ...catalog, items: [{ ...plugin, installation: { status: 'installed', version: '1.0.0', origin: { type: 'local' }, revision: 'old-revision', can_replace: true } }] });
    api.install.mockRejectedValue(Object.assign(new Error('Installation blocked'), { details: { code: 'PLUGIN_DRAIN_REQUIRED' } }));
    const w = mount(RemoteMarketplace, { global }); await flushPromises();
    await button(w, 'viewVersions').trigger('click'); await flushPromises();
    await w.findAll('input[type="checkbox"]').at(-1)!.setValue(true);
    await w.findAll('button').find(b => b.text() === 'plugins.installFlow.replaceInstall')!.trigger('click'); await flushPromises();
    expect(w.text()).toContain('plugins.installDialog.drain.noticeRequired');
    expect(w.text()).toContain('plugins.installDialog.drain.viewDetail');
    expect(w.find('[role="dialog"]').exists()).toBe(true);
    expect(w.find('select').element.value).toBe('1.2.0'); w.unmount();
  });
  it('does not periodically query marketplace data', async () => {
    vi.useFakeTimers();
    const w = mount(RemoteMarketplace, { global }); await flushPromises();
    await vi.advanceTimersByTimeAsync(60000); await flushPromises();
    expect(api.catalog).toHaveBeenCalledTimes(1); expect(api.versions).not.toHaveBeenCalled(); w.unmount();
  });
});
