import { flushPromises, mount } from '@vue/test-utils';
import { reactive } from 'vue';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { TextDecoder, TextEncoder } from 'node:util';

const api = vi.hoisted(() => ({ list: vi.fn(), create: vi.fn(), previewImport: vi.fn(), getImport: vi.fn(), commitImport: vi.fn() }));
const access = vi.hoisted(() => ({ hasPermission: vi.fn() }));
vi.mock('~/composables/api/services/customerService', () => ({ useCustomerService: () => api }));
vi.mock('~/composables/api/services/meService', () => ({ useUserContext: () => access }));
const user = reactive({ currentTenantUuid: 'tenant-a' });
vi.mock('~/stores/user', () => ({ useUserStore: () => user }));
import Page from '~/pages/customers/index.vue';
import CreateModal from '~/components/customers/CustomerCreateModal.vue';
import ImportModal from '~/components/customers/CustomerImportModal.vue';

const stubs = {
  UModal: { props: ['open'], emits: ['update:open'], template: '<section v-if="open"><slot name="body"/><slot name="footer"/></section>' },
  UForm: { emits: ['submit'], template: '<form @submit.prevent="$emit(\'submit\')"><slot/></form>' },
  UFormField: { props: ['label'], template: '<label>{{label}}<slot/></label>' },
  UInput: { props: ['modelValue', 'disabled', 'type'], emits: ['update:modelValue'], template: '<input :type="type || \'text\'" :value="modelValue" :disabled="disabled" @input="$emit(\'update:modelValue\',$event.target.value)"/>' },
  USelect: { props: ['items', 'modelValue'], emits: ['update:modelValue'], template: '<select :value="modelValue" @change="$emit(\'update:modelValue\',$event.target.value)"><option v-for="item in items" :value="item.value">{{item.label}}</option></select>' },
  UCheckbox: { props: ['modelValue', 'label', 'disabled'], emits: ['update:modelValue'], template: '<label><input type="checkbox" :checked="modelValue" :disabled="disabled" @change="$emit(\'update:modelValue\',$event.target.checked)"/>{{label}}</label>' },
  UButton: { props: ['disabled', 'loading', 'type'], template: '<button :type="type || \'button\'" :disabled="disabled || loading"><slot/></button>' },
  UBadge: { template: '<span><slot/></span>' }, UAlert: { props: ['title', 'description'], template: '<p>{{title}} {{description}}</p>' },
  NuxtLink: { props: ['to'], template: '<a :href="to"><slot/></a>' },
};
const wrappers: ReturnType<typeof mount>[] = [];
const render = (component: any, props: any = {}) => { const wrapper = mount(component, { props, global: { stubs } }); wrappers.push(wrapper); return wrapper; };
const input = { type: 'person', display_name: 'Ada', primary_email: 'ada@example.test', primary_phone: '', primary_contact_name: '', primary_contact_email: '', primary_contact_phone: '' };
const preview = { uuid: '11111111-1111-4111-8111-111111111111', file_name: 'customers.csv', checksum: 'a'.repeat(64), version: 1, status: 'preview', expires_at: '2099-01-01T00:00:00Z', created_at: '', total: 1, ready: 1, created: 0, skipped: 0, failed: 0, rows: [{ row: 2, input, status: 'ready' }] };
const replace = vi.fn();
const route = { query: {} as Record<string, any> };
beforeEach(() => {
  vi.clearAllMocks(); user.currentTenantUuid = 'tenant-a'; route.query = {};
  vi.stubGlobal('useI18n', () => ({ t: (key: string) => key })); vi.stubGlobal('definePageMeta', vi.fn());
  vi.stubGlobal('useRoute', () => route); vi.stubGlobal('useRouter', () => ({ replace }));
  vi.stubGlobal('TextDecoder', TextDecoder); vi.stubGlobal('TextEncoder', TextEncoder);
  api.list.mockResolvedValue({ items: [], pagination: { total: 41, page: 1, page_size: 20 } });
  api.create.mockResolvedValue({ uuid: 'customer-new', primary_contact_uuid: 'contact-new' });
  api.previewImport.mockResolvedValue(preview); api.getImport.mockResolvedValue(preview);
  api.commitImport.mockResolvedValue({ ...preview, status: 'completed', ready: 0, created: 1 });
  access.hasPermission.mockResolvedValue(true);
});
afterEach(() => { wrappers.splice(0).forEach(w => w.unmount()); });

describe('Customer admin page', () => {
  it('uses actual pagination and resets it when searching', async () => {
    const wrapper = render(Page); await flushPromises();
    expect(api.list).toHaveBeenLastCalledWith(1, 20, '');
    await wrapper.get('[data-testid="customer-next"]').trigger('click'); await flushPromises();
    expect(api.list).toHaveBeenLastCalledWith(2, 20, '');
    await wrapper.get('input').setValue('Ada'); await wrapper.get('input').trigger('keyup.enter'); await flushPromises();
    expect(api.list).toHaveBeenLastCalledWith(1, 20, 'Ada');
  });
  it('keeps create and import disabled when only the menu is granted', async () => {
    access.hasPermission.mockResolvedValue(false);
    const wrapper = render(Page); await flushPromises();
    expect(wrapper.get('[data-testid="customer-new"]').attributes('disabled')).toBeDefined();
    expect(wrapper.get('[data-testid="customer-import"]').attributes('disabled')).toBeDefined();
    expect(api.create).not.toHaveBeenCalled(); expect(api.previewImport).not.toHaveBeenCalled();
  });
  it('requires all import operations rather than only preview permission', async () => {
    access.hasPermission.mockImplementation(async code => code !== 'customer.admin_customers_imports_import_uuid_commit:create');
    const wrapper = render(Page); await flushPromises();
    expect(wrapper.get('[data-testid="customer-new"]').attributes('disabled')).toBeUndefined();
    expect(wrapper.get('[data-testid="customer-import"]').attributes('disabled')).toBeDefined();
  });
  it('clears the old tenant dialog and import link when switching tenants', async () => {
    const wrapper = render(Page); await flushPromises();
    await wrapper.get('[data-testid="customer-new"]').trigger('click'); await flushPromises();
    expect(wrapper.find('[data-testid="customer-create-form"]').exists()).toBe(true);
    route.query.import_uuid = preview.uuid; user.currentTenantUuid = 'tenant-b'; await flushPromises();
    expect(wrapper.find('[data-testid="customer-create-form"]').exists()).toBe(false);
    expect(replace).toHaveBeenLastCalledWith({ query: {} });
  });
});

describe('Customer creation', () => {
  it('submits a person without creating login credentials', async () => {
    const wrapper = render(CreateModal, { open: false, allowed: true }); await wrapper.setProps({ open: true });
    await wrapper.get('input').setValue('Ada'); await wrapper.get('input[type="email"]').setValue('ada@example.test');
    await wrapper.get('form').trigger('submit'); await flushPromises();
    expect(api.create).toHaveBeenCalledWith({ type: 'person', display_name: 'Ada', primary_email: 'ada@example.test', primary_phone: '' });
    expect(wrapper.emitted('created')?.[0]).toEqual(['customer-new']);
  });
  it('requires a named natural-person contact for companies', async () => {
    const wrapper = render(CreateModal, { open: false, allowed: true }); await wrapper.setProps({ open: true });
    await wrapper.get('select').setValue('company'); await wrapper.get('input').setValue('Acme');
    expect(wrapper.get('[data-testid="customer-create-submit"]').attributes('disabled')).toBeDefined();
    await wrapper.findAll('fieldset input')[0]!.setValue('Jane');
    await wrapper.get('form').trigger('submit'); await flushPromises();
    expect(api.create).toHaveBeenCalledWith(expect.objectContaining({ type: 'company', primary_contact: { display_name: 'Jane', email: '', phone: '' } }));
  });
  it('retains errors and the request ID without closing the form', async () => {
    api.create.mockRejectedValue({ data: { message: 'Creation rejected', error_code: 'CUSTOMER_ACCOUNT_INVALID', request_id: 'request-create' } });
    const wrapper = render(CreateModal, { open: false, allowed: true }); await wrapper.setProps({ open: true }); await wrapper.get('input').setValue('Ada');
    await wrapper.get('form').trigger('submit'); await flushPromises();
    expect(wrapper.text()).toContain('CUSTOMER_ACCOUNT_INVALID'); expect(wrapper.text()).toContain('request-create');
    expect(wrapper.emitted('created')).toBeUndefined();
  });
});

async function selectCSV(wrapper: ReturnType<typeof mount>, fileName = 'customers.csv') {
  const element = wrapper.get('[data-testid="customer-import-file"]').element;
  Object.defineProperty(element, 'files', { configurable: true, value: [{ name: fileName, size: 60, arrayBuffer: async () => new TextEncoder().encode('display_name,primary_email\nAda,ada@example.test\n').buffer }] });
  await wrapper.get('[data-testid="customer-import-file"]').trigger('change');
}

describe('Customer CSV import', () => {
  it('creates a preview and requires confirmation of the saved checksum', async () => {
    const wrapper = render(ImportModal, { open: false, allowed: true }); await wrapper.setProps({ open: true });
    await selectCSV(wrapper); await wrapper.get('[data-testid="customer-import-preview"]').trigger('click'); await flushPromises();
    expect(api.previewImport).toHaveBeenCalledWith('customers.csv', 'display_name,primary_email\nAda,ada@example.test\n');
    expect(api.create).not.toHaveBeenCalled(); expect(api.commitImport).not.toHaveBeenCalled();
    expect(wrapper.get('[data-testid="customer-import-commit"]').attributes('disabled')).toBeDefined();
    await wrapper.get('[data-testid="customer-import-confirm"] input').setValue(true);
    await wrapper.get('[data-testid="customer-import-commit"]').trigger('click'); await flushPromises();
    expect(api.commitImport).toHaveBeenCalledWith(preview.uuid, preview.checksum);
    expect(wrapper.emitted('completed')).toHaveLength(1);
  });
  it('reuses the same preview when retrying an uncertain commit', async () => {
    api.commitImport.mockRejectedValueOnce(new Error('network interrupted'));
    const wrapper = render(ImportModal, { open: false, allowed: true, initialId: preview.uuid }); await wrapper.setProps({ open: true }); await flushPromises();
    expect(api.getImport).toHaveBeenCalledWith(preview.uuid);
    await wrapper.get('[data-testid="customer-import-confirm"] input').setValue(true);
    await wrapper.get('[data-testid="customer-import-commit"]').trigger('click'); await flushPromises();
    await wrapper.get('[data-testid="customer-import-commit"]').trigger('click'); await flushPromises();
    expect(api.commitImport.mock.calls).toEqual([[preview.uuid, preview.checksum], [preview.uuid, preview.checksum]]);
  });
  it('rejects unsupported files before submitting', async () => {
    const wrapper = render(ImportModal, { open: false, allowed: true }); await wrapper.setProps({ open: true });
    await selectCSV(wrapper, 'customers.xlsx'); await wrapper.get('[data-testid="customer-import-preview"]').trigger('click'); await flushPromises();
    expect(wrapper.text()).toContain('customerAdmin.fileInvalid'); expect(api.previewImport).not.toHaveBeenCalled();
  });
  it('discards a late response after the dialog closes', async () => {
    let resolve!: (value: unknown) => void; api.previewImport.mockReturnValue(new Promise(done => { resolve = done; }));
    const wrapper = render(ImportModal, { open: false, allowed: true }); await wrapper.setProps({ open: true }); await selectCSV(wrapper);
    await wrapper.get('[data-testid="customer-import-preview"]').trigger('click'); await flushPromises();
    await wrapper.setProps({ open: false }); resolve(preview); await flushPromises();
    expect(wrapper.emitted('previewed')).toBeUndefined();
    expect(wrapper.find('[data-testid="customer-import-dialog"]').exists()).toBe(false);
  });
});
