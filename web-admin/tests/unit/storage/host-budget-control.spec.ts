import {beforeEach,afterEach,describe,it,expect,vi} from 'vitest';
import {mount,flushPromises} from '@vue/test-utils';
import {defineComponent} from 'vue';
import {createI18n} from 'vue-i18n';
import zh from '../../../i18n/locales/zh.json';
import Panel from '~/components/monitor/storage/HostBudgetControl.vue';
import {decodeHostBudgetConfiguration,decodeHostBudgetMutation} from '~/composables/api/services/storageRuntimeService';
const api=vi.hoisted(()=>({budgetConfiguration:vi.fn(),saveBudget:vi.fn(),adoptBudget:vi.fn(),budgetRequest:vi.fn()}));
vi.mock('~/composables/api/services/storageRuntimeService',async original=>({...await original<any>(),useStorageRuntimeService:()=>api}));
const stubs = {
  UIcon: true,
  UAlert: { props: ['title', 'description'], template: '<div>{{title}}{{description}}<slot name="actions" /></div>' },
  UButton: { props: ['disabled', 'loading'], template: '<button :disabled="disabled || loading"><slot /></button>' },
  UModal: { props: ['open'], template: '<section v-if="open"><slot name="body" /></section>' },
  UForm: { emits: ['submit'], template: '<form @submit.prevent="$emit(\'submit\')"><slot /></form>' },
  UFormField: { props: ['label'], template: '<label>{{label}}<slot /></label>' },
  UInput: defineComponent({ props: ['modelValue', 'disabled'], emits: ['update:modelValue'], template: '<input :value="modelValue" :disabled="disabled" @input="$emit(\'update:modelValue\', $event.target.value)" />' }),
  USelect: defineComponent({ props: ['modelValue', 'items', 'disabled'], emits: ['update:modelValue'], template: '<select :value="modelValue" :disabled="disabled" @change="$emit(\'update:modelValue\', $event.target.value)"><option v-for="item in items" :key="item.value" :value="item.value">{{item.label}}</option></select>' }),
  UCheckbox: defineComponent({ props: ['modelValue', 'label', 'disabled'], emits: ['update:modelValue'], template: '<label><input type="checkbox" :checked="modelValue" :disabled="disabled" @change="$emit(\'update:modelValue\', $event.target.checked)" />{{label}}</label>' }),
};

const GiB=2**30,policy=()=>({min_free_bytes:20*GiB,critical_free_bytes:10*GiB,max_used_bytes:0,max_concurrent:1});
const fixture=()=>({authority:'database',state:'applied',ready:true,can_manage:true,owner_instance:'owned',desired_revision:1,applied_revision:1,snapshot_revision:1,desired_policy:policy(),effective_policy:policy(),pending_protection:false,legacy_revision:1,legacy_digest:'a'.repeat(64)});
const clone=(x:any)=>x?JSON.parse(JSON.stringify(x)):x;let current:any;const views:ReturnType<typeof mount>[]=[];
async function view(){const w=mount(Panel,{props:{open:false,runtime:{budget_configuration:clone(current)} as any},global:{stubs,plugins:[createI18n({legacy:false,locale:'zh',messages:{zh}})]}});views.push(w);await w.setProps({open:true});await flushPromises();return w;}
async function edit(w:any){await w.get('[role="tab"][aria-label="预算设置"]').trigger('click');}
async function save(w:any){await w.get('[data-testid=budget-confirm] input').setValue(true);await w.get('form').trigger('submit');await flushPromises();}
beforeEach(()=>{vi.resetAllMocks();current=fixture();api.budgetConfiguration.mockImplementation(async()=>clone(current));});
afterEach(()=>{views.splice(0).forEach(w=>w.unmount());vi.useRealTimers();});
describe('host budget database authority',()=>{
 it('preserves the CAS draft across push and conflict until explicit reload',async()=>{
  const w=await view();await edit(w);await w.get('[data-testid=budget-minimum]').setValue('18');current.desired_revision=4;current.desired_policy.min_free_bytes=21*GiB;await w.setProps({runtime:{budget_configuration:clone(current)} as any});
  await w.get('[role="tab"][aria-label="保护概览"]').trigger('click');await edit(w);expect((w.get('[data-testid=budget-minimum]').element as HTMLInputElement).value).toBe('18');api.saveBudget.mockRejectedValue({status:409,message:'conflict'});await save(w);expect(api.saveBudget.mock.calls[0][0].expected_revision).toBe(1);expect(w.find('[data-testid=budget-unknown]').exists()).toBe(false);expect((w.get('[data-testid=budget-minimum]').element as HTMLInputElement).value).toBe('18');await w.get('[data-testid=budget-load-latest]').trigger('click');await flushPromises();expect((w.get('[data-testid=budget-minimum]').element as HTMLInputElement).value).toBe('21');
 });
 it('keeps response loss query-only through not-found and reconciles exact UUID receipt',async()=>{
  const w=await view();await edit(w);api.saveBudget.mockRejectedValue({status:503,message:'lost'});await save(w);expect(w.find('[data-testid=budget-unknown]').exists()).toBe(true);expect(w.get('[data-testid=budget-save]').attributes('disabled')).toBeDefined();const sent=api.saveBudget.mock.calls[0][0];api.budgetRequest.mockRejectedValue({status:404});await w.get('[data-testid=budget-reconcile]').trigger('click');await flushPromises();expect(w.find('[data-testid=budget-unknown]').exists()).toBe(true);api.budgetRequest.mockResolvedValue({request_id:sent.request_id,operation_revision:2,operation_policy:sent.settings,configuration:{...current,state:'pending',desired_revision:2,pending_protection:true}});await w.get('[data-testid=budget-reconcile]').trigger('click');await flushPromises();expect(api.saveBudget).toHaveBeenCalledTimes(1);expect(w.find('[data-testid=budget-unknown]').exists()).toBe(false);expect(w.text()).toContain('等待发布确认');
 });
 it('requires explicit adoption and preserves protection',async()=>{
  current={...fixture(),authority:'legacy',state:'legacy',desired_revision:0,applied_revision:0,snapshot_revision:null,desired_policy:null};const w=await view();await edit(w);expect(w.get('[data-testid=budget-adopt]').attributes('disabled')).toBeDefined();api.adoptBudget.mockImplementation(async(input:any)=>({request_id:input.request_id,operation_revision:1,operation_policy:policy(),configuration:{...fixture(),state:'pending',applied_revision:0,snapshot_revision:null}}));await w.get('[data-testid=budget-confirm] input').setValue(true);await w.get('[data-testid=budget-adopt]').trigger('click');await flushPromises();expect(api.adoptBudget).toHaveBeenCalledWith(expect.objectContaining({legacy_revision:1,legacy_digest:'a'.repeat(64),confirm:true}),expect.any(AbortSignal));expect(api.saveBudget).not.toHaveBeenCalled();
 });
 it('blocks foreign ownership and old backends without idle polling',async()=>{
  current.can_manage=false;const w=await view();await edit(w);expect(w.get('[data-testid=budget-save]').attributes('disabled')).toBeDefined();expect(w.text()).toContain('归属实例');vi.useFakeTimers();const count=api.budgetConfiguration.mock.calls.length;await vi.advanceTimersByTimeAsync(60000);expect(api.budgetConfiguration).toHaveBeenCalledTimes(count);vi.useRealTimers();current=undefined;api.budgetConfiguration.mockRejectedValue({status:404,message:'upgrade'});const old=await view();await edit(old);expect(old.find('[data-testid=budget-save]').exists()).toBe(false);
 });
 it('does not label a competing owner publication as this operation being applied',async()=>{
  const w=await view();await edit(w);api.saveBudget.mockImplementation(async(input:any)=>({request_id:input.request_id,operation_revision:2,operation_policy:input.settings,configuration:{...fixture(),desired_revision:2,applied_revision:2,snapshot_revision:2,owner_instance:'other',can_manage:false}}));await save(w);expect(w.text()).toContain('预算归属已由其他实例取得');
 });
 it('validates typed settings and authority responses',()=>{expect(decodeHostBudgetConfiguration(fixture()).authority).toBe('database');expect(()=>decodeHostBudgetConfiguration({...fixture(),effective_policy:{...policy(),critical_free_bytes:21*GiB}})).toThrow();expect(()=>decodeHostBudgetMutation({request_id:crypto.randomUUID(),operation_revision:1,operation_policy:{...policy(),max_concurrent:17},configuration:fixture()})).toThrow();});
});
