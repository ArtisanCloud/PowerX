import { flushPromises, mount } from '@vue/test-utils'
import { reactive, ref } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
const api = vi.hoisted(() => Object.fromEntries(['get','listAuthIdentities','listMemberships','listLoginEvents','listMiniAppEntries','listContacts','createContact','updateContact','listContactIdentities','resolveContactIdentity','bindContactIdentity','setPrimaryContact','previewContactRepair','getContactRepair','commitContactRepair'].map(k => [k,vi.fn()])) as Record<string, ReturnType<typeof vi.fn>>)
const access = vi.hoisted(() => ({ hasPermission: vi.fn() }))
const metadata = vi.hoisted(() => ({ listDictionaries: vi.fn(), listDictionaryItems: vi.fn() }))
vi.mock('~/composables/api/services/customerService', () => ({ useCustomerService: () => api }))
vi.mock('~/composables/api/services/meService', () => ({ useUserContext: () => access }))
vi.mock('~/composables/api/services/metadataGovernanceService', () => ({ useMetadataGovernanceService: () => metadata }))
const user = reactive({ currentTenantUuid: 'tenant-a' }), route = reactive({ params: { customer_uuid: 'customer-a' }, query: {} as Record<string,string> })
vi.mock('~/stores/user', () => ({ useUserStore: () => user }))
import Page from '~/pages/customers/[customer_uuid].vue'
import RepairModal from '~/components/customers/CustomerContactRepairModal.vue'
import { customerNameValid } from '~/utils/customer-admin'
const stubs = {
 UForm: { emits: ['submit'], template: '<form @submit.prevent="$emit(\'submit\')"><slot/></form>' },
 UModal: { props: ['open'], emits: ['update:open'], template: '<section v-if="open"><button data-testid="modal-close" @click="$emit(\'update:open\',false)">close</button><slot name="body"/></section>' },
 UButton: { props: ['disabled','loading'], template: '<button :disabled="disabled || loading"><slot/></button>' },
 UInput: { props: ['modelValue','disabled','type'], emits: ['update:modelValue'], template: '<input :type="type || \'text\'" :disabled="disabled" :value="modelValue" @input="$emit(\'update:modelValue\',$event.target.value)"/>' },
 USelect: { props: ['modelValue','items','disabled'], emits: ['update:modelValue'], template: '<select :value="modelValue" :disabled="disabled" @change="$emit(\'update:modelValue\',$event.target.value)"><option v-for="i in items" :value="i.value">{{i.label}}</option></select>' },
 UFormField: { template: '<label><slot/></label>' },
 UCheckbox: { props: ['modelValue','disabled'], emits: ['update:modelValue'], template: '<input type="checkbox" :checked="modelValue" :disabled="disabled" @change="$emit(\'update:modelValue\',$event.target.checked)"/>' },
 UBadge: { template: '<span><slot/></span>' }, UAlert: { props: ['title','description'], template: '<p>{{title}} {{description}}</p>' }
}
const account = { uuid:'customer-a',type:'person' as const,status:'active',display_name:'Customer A',primary_contact_uuid:'contact-one' }
const contacts = [{ uuid:'contact-one',customer_uuid:'customer-a',display_name:'First',status:'active' as const,roles:['primary'],tags:[] },{ uuid:'contact-two',customer_uuid:'customer-a',display_name:'Second',status:'active' as const,roles:[],tags:[] }]
const preview = { uuid:'repair-one',checksum:'a'.repeat(64),status:'preview' as const,expires_at:'2099-01-01T00:00:00Z',proposal:{customer_uuid:'customer-a',type:'person' as const,action:'reuse_contact' as const,contact_uuid:'contact-one',expected_primary_contact_uuid:''} }
const wrappers: ReturnType<typeof mount>[] = []
const render = (component:any,props:any={}) => { const w=mount(component,{props,global:{stubs:{...stubs,USelectMenu:stubs.USelect}}}); wrappers.push(w); return w }
function deferred<T>() { let resolve!: (value:T)=>void; const promise = new Promise<T>(r=>{resolve=r}); return {promise,resolve} }
beforeEach(() => {
 vi.clearAllMocks(); user.currentTenantUuid='tenant-a'; route.params.customer_uuid='customer-a'; route.query={}
 vi.stubGlobal('definePageMeta',vi.fn()); vi.stubGlobal('useI18n',()=>({t:(k:string)=>k,locale:ref('zh')})); vi.stubGlobal('useRoute',()=>route); vi.stubGlobal('useRouter',()=>({replace:vi.fn()}))
 access.hasPermission.mockResolvedValue(true); api.get.mockImplementation(async id=>({...account,uuid:id,display_name:id==='customer-a'?'Customer A':'Customer B'}))
 for (const name of ['listAuthIdentities','listMemberships','listLoginEvents','listMiniAppEntries','listContactIdentities']) api[name]!.mockResolvedValue({items:[]})
 api.listContacts.mockResolvedValue({items:contacts}); api.createContact.mockResolvedValue(contacts[1]); api.updateContact.mockResolvedValue(contacts[1]); api.setPrimaryContact.mockResolvedValue({primary_contact_uuid:'contact-two'})
 metadata.listDictionaries.mockResolvedValue({items:[{uuid:'dict',namespace:'corex.customer.contact_identity_channel'}]}); metadata.listDictionaryItems.mockResolvedValue({items:[{uuid:'channel',display_name:'Current channel'}]})
 api.previewContactRepair.mockResolvedValue(preview); api.getContactRepair.mockResolvedValue(preview); api.commitContactRepair.mockResolvedValue({...preview,status:'completed',primary_contact_uuid:'contact-one'})
})
afterEach(()=>wrappers.splice(0).forEach(w=>w.unmount()))
async function contactTab(w:ReturnType<typeof mount>) { await w.findAll('button').find(b=>b.text()==='customerMaster.contacts')!.trigger('click') }
describe('Customer detail tenant and operation isolation',()=>{
 it('keeps a late dictionary response out of the new tenant',async()=>{
  const late=deferred<any>(); metadata.listDictionaryItems.mockReturnValueOnce(late.promise)
  const w=render(Page); await flushPromises(); user.currentTenantUuid='tenant-b'; await flushPromises()
  late.resolve({items:[{uuid:'old-channel',display_name:'Old tenant channel'}]}); await flushPromises()
  await contactTab(w); await w.get('[data-testid="contact-identities"]').trigger('click'); await flushPromises()
  expect(w.text()).toContain('Current channel'); expect(w.text()).not.toContain('Old tenant channel')
 })
 it('uses the actual permission codes for hyphenated account routes',async()=>{
  access.hasPermission.mockImplementation(async code=>code==='customer.admin_customers_accounts_customer_uuid_auth_identities:read')
  const w=render(Page); await flushPromises(); expect(w.text()).toContain('customerMaster.authIdentities'); expect(api.listAuthIdentities).toHaveBeenCalledWith('customer-a'); expect(api.listMemberships).not.toHaveBeenCalled()
 })
 it('discards a late customer load after changing customers',async()=>{
  const late=deferred<any>(); api.get.mockReturnValueOnce(late.promise)
  const w=render(Page); await flushPromises(); route.params.customer_uuid='customer-b'; await flushPromises()
  late.resolve(account); await flushPromises(); expect(w.text()).toContain('Customer B'); expect(w.text()).not.toContain('Customer A')
 })
 it('clears an open dialog and ignores a late write result after tenant changes',async()=>{
  const w=render(Page); await flushPromises(); await contactTab(w); await w.get('[data-testid="contact-edit"]').trigger('click')
  const late=deferred<any>(); api.updateContact.mockReturnValueOnce(late.promise); await w.get('[data-testid="contact-save"]').trigger('click')
  user.currentTenantUuid='tenant-b'; await flushPromises(); expect(w.find('[data-testid="contact-save"]').exists()).toBe(false)
  const calls=api.listContacts.mock.calls.length; late.resolve(contacts[0]); await flushPromises(); expect(api.listContacts.mock.calls.length).toBe(calls)
 })
 it('checks each contact mutation permission independently',async()=>{
  access.hasPermission.mockImplementation(async code=>![ 'customer.admin_customers_customer_uuid_contacts:create','customer.admin_customers_customer_uuid_contacts_contact_uuid:update','customer.admin_customers_customer_uuid_contacts_contact_uuid_primary:create'].includes(code))
  const w=render(Page); await flushPromises(); await contactTab(w)
  expect(w.get('[data-testid="contact-create"]').attributes('disabled')).toBeDefined(); expect(w.find('[data-testid="contact-edit"]').exists()).toBe(false); expect(w.find('[data-testid="contact-set-primary"]').exists()).toBe(false)
  await w.get('[data-testid="contact-identities"]').trigger('click'); await flushPromises(); expect(w.find('[data-testid="contact-bind"]').exists()).toBe(true)
 })
 it('prevents deactivating the referenced primary and sends the previewed reference on explicit set',async()=>{
  const w=render(Page); await flushPromises(); await contactTab(w)
  expect(w.findAll('[data-testid="contact-disable"]')[0]!.attributes('disabled')).toBeDefined()
  await w.get('[data-testid="contact-set-primary"]').trigger('click'); await w.get('[data-testid="primary-confirm"]').trigger('click'); await flushPromises()
  expect(api.setPrimaryContact).toHaveBeenCalledWith('customer-a','contact-two','contact-one')
 })
 it('rejects late identity reads after the dialog closes',async()=>{
  const w=render(Page); await flushPromises(); await contactTab(w); const late=deferred<any>(); api.listContactIdentities.mockReturnValueOnce(late.promise)
  await w.get('[data-testid="contact-identities"]').trigger('click'); await w.get('[data-testid="modal-close"]').trigger('click')
  late.resolve({items:[{uuid:'old',external_subject:'old-secret',channel_dictionary_item_uuid:'channel'}]}); await flushPromises(); expect(w.text()).not.toContain('old-secret')
 })
 it('keeps code and request ID on failed primary confirmation',async()=>{
  api.setPrimaryContact.mockRejectedValue({data:{message:'Changed',error_code:'CONTACT_PRIMARY_CHANGED',request_id:'request-primary'}})
  const w=render(Page); await flushPromises(); await contactTab(w); await w.get('[data-testid="contact-set-primary"]').trigger('click'); await w.get('[data-testid="primary-confirm"]').trigger('click'); await flushPromises()
  expect(w.text()).toContain('CONTACT_PRIMARY_CHANGED'); expect(w.text()).toContain('request-primary')
 })
})
describe('Saved contact repair preview',()=>{
 it('does not emit a saved preview into a new tenant before component disposal',async()=>{
  const late=deferred<any>(); api.previewContactRepair.mockReturnValueOnce(late.promise)
  const w=render(RepairModal,{open:true,allowed:true,account:{...account,primary_contact_uuid:''},contacts:[contacts[0]]})
  await w.get('[data-testid="repair-preview"]').trigger('click'); user.currentTenantUuid='tenant-b'; late.resolve(preview); await flushPromises()
  expect(w.emitted('previewed')).toBeUndefined(); expect(w.find('[data-testid="repair-commit"]').exists()).toBe(false)
 })
 it('requires preview and explicit confirmation before committing the exact checksum',async()=>{
  const w=render(RepairModal,{open:true,allowed:true,account:{...account,primary_contact_uuid:''},contacts:[contacts[0]]})
  await w.get('[data-testid="repair-preview"]').trigger('click'); await flushPromises(); expect(api.commitContactRepair).not.toHaveBeenCalled()
  expect(w.get('[data-testid="repair-commit"]').attributes('disabled')).toBeDefined(); await w.get('[data-testid="repair-confirm"]').setValue(true)
  await w.get('[data-testid="repair-commit"]').trigger('click'); await flushPromises(); expect(api.commitContactRepair).toHaveBeenCalledWith('repair-one',preview.checksum); expect(w.emitted('completed')).toHaveLength(1)
 })
 it('rejects a recovery record for another customer and cannot commit it',async()=>{
  api.getContactRepair.mockResolvedValue({...preview,proposal:{...preview.proposal,customer_uuid:'another'}})
  const w=render(RepairModal,{open:true,allowed:true,account,contacts,initialId:'repair-one'}); await flushPromises(); expect(w.text()).toContain('customerMaintenance.repairScopeMismatch'); expect(w.find('[data-testid="repair-commit"]').exists()).toBe(false)
 })
})

describe('Shared customer field rule',()=>{
 it('counts Unicode characters consistently with Core',()=>{
  expect(customerNameValid('中'.repeat(43),true)).toBe(true); expect(customerNameValid('😀'.repeat(128),true)).toBe(true); expect(customerNameValid('😀'.repeat(129),true)).toBe(false)
  expect(customerNameValid('A\u0000B',true)).toBe(false); expect(customerNameValid('\ud800',true)).toBe(false)
 })
})
