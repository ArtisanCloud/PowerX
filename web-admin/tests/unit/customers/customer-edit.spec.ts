import { flushPromises, mount } from '@vue/test-utils';
import { reactive } from 'vue';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
const api = vi.hoisted(() => ({ updateProfile: vi.fn(), get: vi.fn(), list: vi.fn() }));
const access = vi.hoisted(() => ({ hasPermission: vi.fn() }));
vi.mock('~/composables/api/services/customerService', () => ({ useCustomerService: () => api }));
vi.mock('~/composables/api/services/meService', () => ({ useUserContext: () => access }));
const user = reactive({ currentTenantUuid: 'tenant-a' });
vi.mock('~/stores/user', () => ({ useUserStore: () => user }));
import Modal from '~/components/customers/CustomerEditModal.vue';
import Page from '~/pages/customers/index.vue';
const account = { uuid: 'customer-a', type: 'person' as const, status: 'active', display_name: 'Ada', nickname: 'A', primary_email: 'ada@example.test', primary_phone: '+86 13800000000', profile_version: 'a'.repeat(64) };
const stubs = {
  CustomerCreateModal: true, CustomerImportModal: true,
  UModal: { props: ['open'], emits: ['update:open'], template: '<section v-if="open"><button data-testid="close" @click="$emit(\'update:open\',false)">close</button><slot name="body"/></section>' },
  UForm: { emits: ['submit'], template: '<form @submit.prevent="$emit(\'submit\')"><slot/></form>' },
  UFormField: { template: '<label><slot/></label>' },
  UInput: { props: ['modelValue', 'type', 'disabled'], emits: ['update:modelValue'], template: '<input :value="modelValue" :type="type || \'text\'" :disabled="disabled" @input="$emit(\'update:modelValue\',$event.target.value)"/>' },
  UButton: { props: ['disabled', 'loading'], template: '<button :disabled="disabled || loading"><slot/></button>' },
  UAlert: { props: ['title'], template: '<p>{{title}}</p>' }, UBadge: { template: '<span><slot/></span>' },
};
const wrappers: ReturnType<typeof mount>[] = [];
function render(component: any = Modal, props: any = { open: true, allowed: true, account }) { const w = mount(component, { props, global: { stubs } }); wrappers.push(w); return w; }
function deferred<T>() { let resolve!: (value: T) => void; const promise = new Promise<T>(r => { resolve = r; }); return { promise, resolve }; }
beforeEach(() => {
  vi.clearAllMocks(); user.currentTenantUuid = 'tenant-a';
  vi.stubGlobal('useI18n', () => ({ t: (key: string) => key })); vi.stubGlobal('definePageMeta', vi.fn());
  vi.stubGlobal('useRoute', () => ({ query: {} })); vi.stubGlobal('useRouter', () => ({ replace: vi.fn() }));
  api.updateProfile.mockResolvedValue({ ...account, display_name: 'Updated', profile_version: 'b'.repeat(64) }); api.get.mockResolvedValue({ ...account, display_name: 'Newer', profile_version: 'c'.repeat(64) });
  api.list.mockResolvedValue({ items: [account], pagination: { total: 1 } }); access.hasPermission.mockResolvedValue(false);
});
afterEach(() => { wrappers.splice(0).forEach(w => w.unmount()); vi.unstubAllGlobals(); });
describe('Customer profile editing', () => {
  it('enables editing independently of create/import permissions and refreshes the same list after save', async () => {
    access.hasPermission.mockImplementation(async code => code === 'customer.admin_customers_accounts_customer_uuid:update');
    const w = render(Page, {}); await flushPromises(); expect(w.get('[data-testid="customer-new"]').attributes('disabled')).toBeDefined();
    await w.get('[data-testid="customer-edit"]').trigger('click'); await w.get('[data-testid="customer-edit-name"]').setValue('Updated');
    await w.get('form').trigger('submit'); await flushPromises(); expect(api.list).toHaveBeenCalledTimes(2); expect(w.find('[data-testid="customer-edit-form"]').exists()).toBe(false);
  });
  it('hides the list edit entry for a role with only read/create grants', async () => {
    access.hasPermission.mockImplementation(async code => code !== 'customer.admin_customers_accounts_customer_uuid:update');
    const w = render(Page, {}); await flushPromises(); expect(w.find('[data-testid="customer-edit"]').exists()).toBe(false);
  });
  it('prefills values and submits exactly four fields plus the captured version', async () => {
    const w = render(); expect((w.get('[data-testid="customer-edit-name"]').element as HTMLInputElement).value).toBe('Ada');
    expect(w.get('[data-testid="customer-edit-save"]').attributes('disabled')).toBeDefined();
    await w.get('[data-testid="customer-edit-name"]').setValue(' Updated '); await w.get('[data-testid="customer-edit-email"]').setValue('');
    await w.get('form').trigger('submit'); await flushPromises();
    expect(api.updateProfile).toHaveBeenCalledWith('customer-a', { display_name: 'Updated', nickname: 'A', primary_email: '', primary_phone: '+86 13800000000', expected_profile_version: 'a'.repeat(64) }, expect.any(AbortSignal));
    expect(w.emitted('updated')?.[0]?.[0]).toMatchObject({ display_name: 'Updated' });
  });
  it.each(['', '+-', 'not-a-number'])('rejects invalid required name or phone before requesting (%s)', async (value) => {
    const w = render(); await w.get('[data-testid="customer-edit-name"]').setValue(value === '' ? '' : 'Updated');
    if (value) await w.get('[data-testid="customer-edit-phone"]').setValue(value);
    await w.get('form').trigger('submit'); expect(api.updateProfile).not.toHaveBeenCalled();
  });
  it('keeps conflict errors and request ID, and reloads only on explicit action', async () => {
    api.updateProfile.mockRejectedValue({ data: { message: 'Changed', error_code: 'CUSTOMER_PROFILE_CHANGED', request_id: 'request-profile' } });
    const w = render(); await w.get('[data-testid="customer-edit-name"]').setValue('Updated'); await w.get('form').trigger('submit'); await flushPromises();
    expect(w.text()).toContain('request-profile'); expect(api.get).not.toHaveBeenCalled(); expect(w.emitted('updated')).toBeUndefined();
    await w.get('[data-testid="customer-edit-reload"]').trigger('click'); await flushPromises();
    expect((w.get('[data-testid="customer-edit-name"]').element as HTMLInputElement).value).toBe('Newer');
    api.updateProfile.mockResolvedValue(account); await w.get('[data-testid="customer-edit-name"]').setValue('Retry'); await w.get('form').trigger('submit');
    expect(api.updateProfile).toHaveBeenLastCalledWith('customer-a', expect.objectContaining({ expected_profile_version: 'c'.repeat(64) }), expect.any(AbortSignal));
  });
  it.each(['tenant', 'customer', 'close', 'revoke'])('cancels pending work and rejects a late result after %s changes', async (change) => {
    const late = deferred<any>(); api.updateProfile.mockReturnValue(late.promise); const w = render();
    await w.get('[data-testid="customer-edit-name"]').setValue('Updated'); await w.get('form').trigger('submit');
    const signal = api.updateProfile.mock.calls[0]![2] as AbortSignal;
    if (change === 'tenant') user.currentTenantUuid = 'tenant-b';
    if (change === 'customer') await w.setProps({ account: { ...account, uuid: 'customer-b' } });
    if (change === 'close') await w.setProps({ open: false });
    if (change === 'revoke') await w.setProps({ allowed: false });
    await flushPromises(); expect(signal.aborted).toBe(true);
    late.resolve({ ...account, display_name: 'Old tenant result' }); await flushPromises(); expect(w.emitted('updated')).toBeUndefined();
  });
});
