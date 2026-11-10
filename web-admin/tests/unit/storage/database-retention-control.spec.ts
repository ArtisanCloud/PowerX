import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { mount, flushPromises } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import zh from '../../../i18n/locales/zh.json';
import Panel from '~/components/monitor/storage/DatabaseRetentionPanel.vue';

const api = vi.hoisted(() => ({ databaseStatus: vi.fn(), saveDatabasePolicy: vi.fn(), previewDatabase: vi.fn(), runDatabase: vi.fn(),adoptDatabasePolicy:vi.fn(),changeDatabasePolicy:vi.fn(),databasePolicyRequest:vi.fn(),databaseRunRequest:vi.fn() }));
vi.mock('~/composables/api/services/storageRuntimeService', () => ({ useStorageRuntimeService: () => api }));
const stubs = {
  UAlert: { props: ['title', 'description'], template: '<section><p>{{title}} {{description}}</p><slot name="actions"/></section>' },
  UCard: { template: '<div><slot/></div>' },
  UButton: { props: ['disabled', 'loading'], template: '<button :disabled="disabled || loading"><slot/></button>' },
  UModal: { props: ['open'], template: '<section v-if="open"><slot name="body"/><slot name="footer"/></section>' },
  UForm: { emits: ['submit'], template: '<form @submit.prevent="$emit(\'submit\')"><slot/></form>' },
  UFormField: { props: ['label'], template: '<label>{{label}}<slot/></label>' },
  UInput: { props: ['modelValue', 'disabled'], emits: ['update:modelValue'], template: '<input :disabled="disabled" :value="modelValue" @input="$emit(\'update:modelValue\', Number($event.target.value))"/>' },
  UCheckbox: { props: ['modelValue', 'label', 'disabled'], emits: ['update:modelValue'], template: '<label>{{label}}<input type="checkbox" :disabled="disabled" :checked="modelValue" @change="$emit(\'update:modelValue\', $event.target.checked)"/></label>' },
};
let status: any;
let wrapper: ReturnType<typeof mount>;
const button = (label: string) => wrapper.findAll('button').find(b => b.text() === label)!;
beforeEach(() => {
  vi.resetAllMocks();
  status = { configuration:{authority:'database',state:'applied',ready:true,execution_ready:true,desired_revision:1,applied_revision:1,snapshot_revision:1,legacy_digest:'a'.repeat(64),effective_capacity:null},state_configured: true, enforcement_allowed: true, migration_ready: true, worker_enabled: true, ready: true, problem: 'database_retention_runtime_not_configured', migration_problem: 'database_retention_migration_required', relation_bytes: 1024, runs: [], policy: { revision: 1, enabled: false, reject_new_when_full: false, adopt_legacy: false, success_days: 30, failure_days: 90, pending_hours: 24, quarantine_days: 30, batch_limit: 200, max_bytes: 8 * 1024 ** 3 } };
  api.databaseStatus.mockImplementation(async () => structuredClone(status));
  wrapper = mount(Panel, { global: { plugins: [createI18n({ legacy: false, locale: 'zh', messages: { zh } })], stubs } });
});
afterEach(() => { wrapper.unmount(); vi.useRealTimers(); });


const byId=(id:string)=>wrapper.get(`[data-testid="${id}"]`);
const mutation=(key:string,revision:number)=>({request_id:key,operation_revision:revision,configuration:structuredClone(status.configuration)});
async function editAndConfirm(){await flushPromises();await byId('db-retention-edit').trigger('click');await wrapper.find('form input').setValue('14');await wrapper.findAll('form input[type="checkbox"]').at(-1)!.setValue(true);}
it('adopts explicitly and activates separately with frozen revision',async()=>{
 status.configuration.authority='legacy';status.configuration.desired_revision=0;status.configuration.applied_revision=0;status.configuration.snapshot_revision=null;status.policy.revision=0;await wrapper.setProps({revision:1});await flushPromises();
 api.adoptDatabasePolicy.mockImplementation(async (in_:any)=>{status.configuration={...status.configuration,authority:'database',desired_revision:1,applied_revision:1,snapshot_revision:1};status.policy.revision=1;return mutation(in_.request_id,1)});
 await byId('db-retention-adopt').trigger('click');expect(byId('db-retention-action-save').attributes('disabled')).toBeDefined();await byId('db-retention-action-confirm').get('input').setValue(true);await byId('db-retention-action-save').trigger('click');await flushPromises();expect(api.adoptDatabasePolicy.mock.calls[0]![0]).toMatchObject({legacy_digest:'a'.repeat(64),confirm:true});expect(status.policy.enabled).toBe(false);
 api.changeDatabasePolicy.mockImplementation(async(in_:any)=>{status.policy.enabled=in_.enabled;status.policy.revision=2;return mutation(in_.request_id,2)});await byId('db-retention-toggle').trigger('click');await byId('db-retention-action-confirm').get('input').setValue(true);await byId('db-retention-action-save').trigger('click');await flushPromises();expect(api.changeDatabasePolicy.mock.calls[0]![0]).toMatchObject({action:'status',expected_revision:1,enabled:true});expect(api.changeDatabasePolicy.mock.calls[0]![0]).not.toHaveProperty('settings');
});
it('preserves draft and original CAS through push and conflict; latest is explicit',async()=>{
 await editAndConfirm();status.policy.revision=3;status.policy.success_days=21;status.configuration.desired_revision=3;await wrapper.setProps({revision:4});await flushPromises();expect((wrapper.find('form input').element as HTMLInputElement).value).toBe('14');api.changeDatabasePolicy.mockRejectedValue(Object.assign(new Error('STORAGE_CONTROL_REVISION_CONFLICT'),{status:409}));await wrapper.find('form').trigger('submit');await flushPromises();expect(api.changeDatabasePolicy.mock.calls[0]![0]).toMatchObject({expected_revision:1,action:'save',settings:{success_days:14}});expect(wrapper.text()).toContain('STORAGE_CONTROL_REVISION_CONFLICT');expect((wrapper.find('form input').element as HTMLInputElement).value).toBe('14');await byId('db-retention-latest').trigger('click');await flushPromises();expect((wrapper.find('form input').element as HTMLInputElement).value).toBe('21');
});
it('keeps unknown save locked and resolves original receipt using GET only',async()=>{
 await editAndConfirm();api.changeDatabasePolicy.mockRejectedValue(Object.assign(new Error('lost response'),{status:502}));await wrapper.find('form').trigger('submit');await flushPromises();const key=api.changeDatabasePolicy.mock.calls[0]![0].request_id;expect(wrapper.text()).toContain(key);expect(byId('db-retention-save').attributes('disabled')).toBeDefined();api.databasePolicyRequest.mockRejectedValueOnce(Object.assign(new Error('not found'),{status:404}));await byId('db-retention-reconcile-editor').trigger('click');await flushPromises();expect(wrapper.find('[data-testid="db-retention-unknown"]').exists()).toBe(true);api.databasePolicyRequest.mockResolvedValue(mutation(key,2));await byId('db-retention-reconcile-editor').trigger('click');await flushPromises();expect(api.databasePolicyRequest).toHaveBeenLastCalledWith(key,expect.any(AbortSignal));expect(api.changeDatabasePolicy).toHaveBeenCalledTimes(1);expect(wrapper.find('[data-testid="db-retention-unknown"]').exists()).toBe(false);
});
it('binds execution to confirmed preview and reconciles a lost result without replay',async()=>{
 status.policy.enabled=true;await wrapper.setProps({revision:1});await flushPromises();const preview={id:crypto.randomUUID(),revision:1,state:'preview',at:new Date().toISOString(),expires_at:new Date(Date.now()+300000).toISOString(),counts:{traces:1,events:0,quarantined:0,classified:0},policy_snapshot:structuredClone(status.policy)};api.previewDatabase.mockResolvedValue(preview);await byId('db-retention-preview').trigger('click');await flushPromises();api.runDatabase.mockRejectedValue(Object.assign(new Error('lost execution response'),{status:502}));await byId('db-retention-run').trigger('click');await flushPromises();const call=api.runDatabase.mock.calls[0]!;expect(call[0]).toBe(1);expect(call[2]).toBe(preview.id);api.databaseRunRequest.mockResolvedValue({...preview,id:call[1],state:'completed'});await byId('db-retention-reconcile-run').trigger('click');await flushPromises();expect(api.databaseRunRequest).toHaveBeenCalledWith(call[1],expect.any(AbortSignal));expect(api.runDatabase).toHaveBeenCalledTimes(1);
});
it('does not poll and aborts read on unmount',async()=>{await flushPromises();vi.useFakeTimers();const count=api.databaseStatus.mock.calls.length;await vi.advanceTimersByTimeAsync(3600000);expect(api.databaseStatus).toHaveBeenCalledTimes(count);const signal=api.databaseStatus.mock.calls.at(-1)![0];wrapper.unmount();expect(signal.aborted).toBe(true)});

it('shows effective capacity and keeps alerts during pending relaxation',async()=>{status.configuration.applied_revision=1;status.configuration.desired_revision=2;status.configuration.state='pending';status.configuration.effective_capacity={...status.policy,reject_new_when_full:true,max_bytes:64*2**20};status.policy.max_bytes=8*2**30;status.policy.reject_new_when_full=false;status.relation_bytes=100*2**20;await wrapper.setProps({revision:2});await flushPromises();expect(wrapper.text()).toContain('64 MiB');expect(wrapper.text()).toContain('当前占用已达到预算');expect(wrapper.text()).toContain('待发布的放宽设置尚未生效');});
