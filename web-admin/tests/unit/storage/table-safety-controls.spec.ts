import { afterEach, expect, it, vi } from 'vitest';
import { mount, flushPromises } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import zh from '../../../i18n/locales/zh.json';
import Monitor from '~/components/monitor/storage/TableMonitoringControl.vue';
import Retention from '~/components/monitor/storage/SystemTableRetentionControl.vue';
import { decodeTableMonitoring, decodeTableRetention } from '~/composables/api/services/storageDatabaseTablesService';
import { tableMonitoring } from './database-tables-fixture';
const api = vi.hoisted(() => ({ saveMonitoring: vi.fn(), adoptMonitoring:vi.fn(), monitoringRequest:vi.fn(), monitoring:vi.fn(), retention: vi.fn(), saveRetention: vi.fn(), previewRetention: vi.fn(), runRetention: vi.fn(), changeRetention:vi.fn(),adoptRetention:vi.fn(),retentionRequest:vi.fn(),retentionRunRequest:vi.fn() }));
vi.mock('~/composables/api/services/storageDatabaseTablesService', async original => ({ ...await original<object>(), useStorageDatabaseTablesService: () => api }));
const stubs = {
 UForm:{props:['state','schema'],emits:['submit'],template:'<form @submit.prevent="$emit(\'submit\')"><slot/></form>'},
 UFormField:{template:'<label><slot/></label>'},
 UInput:{props:['modelValue'],emits:['update:modelValue'],template:'<input :value="modelValue" @input="$emit(\'update:modelValue\',Number($event.target.value))">'},
 UCheckbox:{props:['modelValue','label'],emits:['update:modelValue'],template:'<label><input type="checkbox" :checked="modelValue" @change="$emit(\'update:modelValue\',$event.target.checked)">{{label}}</label>'},
 UAlert: { props: ['title', 'description'], template: '<aside>{{title}} {{description}}</aside>' }, UIcon: true, UModal: { props: ['open'], template: '<section v-if="open"><slot name="body"/></section>' }, UButton: { props: ['disabled', 'loading'], template: '<button :disabled="disabled || loading"><slot/></button>' } };
const global = () => ({ plugins: [createI18n({ legacy: false, locale: 'zh', messages: { zh } })], stubs });
let wrapper: ReturnType<typeof mount> | undefined;
afterEach(() => { wrapper?.unmount(); wrapper = undefined; vi.clearAllMocks(); vi.useRealTimers(); });
it('keeps an edited monitoring policy through push updates and saves its original CAS revision', async () => {
  const status = tableMonitoring(); api.saveMonitoring.mockImplementation(async (_signal,p,revision,key) => ({request_id:key,operation_revision:revision+1,operation_policy:p,configuration:{...status.configuration,state:'pending',desired_revision:revision+1,desired_policy:p}}));
  wrapper = mount(Monitor, { props: { status }, global: global() });
  await wrapper.find('button').trigger('click');
  await wrapper.findAll('[role="tab"]').find(t => t.text() === '告警阈值')!.trigger('click');
  await wrapper.find('[data-testid="table-growth-mib"]').setValue(128);
  await wrapper.find('[data-testid="table-monitoring-enable"] input').setValue(true);
  await wrapper.setProps({ status: { ...status, policy: { ...status.policy, revision: 4, growth_bytes: 512 * 2 ** 20 } } });
  expect((wrapper.find('[data-testid="table-growth-mib"]').element as HTMLInputElement).value).toBe('128');
  await wrapper.find('form').trigger('submit'); await flushPromises();
  expect(api.saveMonitoring.mock.calls[0]![1]).toMatchObject({ enabled: true, growth_bytes: 128 * 2 ** 20 });
  expect(api.saveMonitoring.mock.calls[0]![2]).toBe(1);
  expect(wrapper.emitted('saved')).toHaveLength(1);
});
it('shows collection and worker alerts outside the settings modal and blocks unavailable enabling', async () => {
  const status = tableMonitoring(); status.worker_enabled = false; status.state = 'stale'; status.alerts = [{ code: 'table_sampling_stalled', level: 'warning' }];
  wrapper = mount(Monitor, { props: { status }, global: global() });
  expect(wrapper.text()).toContain('超过允许时间');
  await wrapper.find('button').trigger('click'); await wrapper.find('[data-testid="table-monitoring-enable"] input').setValue(true);
  expect(wrapper.find('form button').attributes('disabled')).toBeDefined();
});
it('shows unknown cleanup health when monitoring has not collected it, rather than claiming the worker never started', async () => {
  wrapper = mount(Monitor, { props: { status: tableMonitoring() }, global: global() });
  await wrapper.find('button').trigger('click');
  const panel = wrapper.find('[data-testid="table-monitoring-status"]');
  expect(panel.text()).toContain('未知／尚未检查');
  expect(panel.text()).not.toContain('尚未启动');
});
it('distinguishes a disabled audit-backlog probe from an unknown or empty result', async () => {
  const status = tableMonitoring();
  const backlog = { state: 'disabled' as const, observed_at: '2026-11-04T10:12:22Z', eligible_present: false, quarantine_present: false, policy_revision: 0 };
  wrapper = mount(Monitor, { props: { status: { ...status, backlog } }, global: global() });
  await wrapper.find('button').trigger('click');
  const value = () => wrapper!.findAll('dt').find(dt => dt.text() === '符合清理条件的过期调用审计')!.element.nextElementSibling!.textContent;
  expect(value()).toBe('未启用');
  await wrapper.setProps({ status: { ...status, backlog: null } });
  expect(value()).toBe('未知／尚未检查');
  await wrapper.setProps({ status: { ...status, backlog: { ...backlog, state: 'observed' } } });
  expect(value()).toBe('有界探测未发现');
});
const id = 'capability_registry_health_probe_results';
const retentionStatus = () => ({ configuration:{authority:'database',state:'applied',ready:true,execution_ready:true,desired_revision:1,applied_revision:1,snapshot_revision:1,legacy_digest:'a'.repeat(64)},id, revision: 1, policy: { enabled: false, retention_days: 30, batch_limit: 200 }, state_configured: true, enforcement_allowed: true, worker_enabled: true, ready: true, runs: [] });
it('never auto-enables or polls, keeps dirty cleanup policy on push, and uses the original revision', async () => {
  api.retention.mockResolvedValue(retentionStatus()); api.changeRetention.mockRejectedValue({status:409,message:'revision conflict'});
  wrapper = mount(Retention, { props: { id }, global: global() }); await flushPromises();
  expect(api.saveRetention).not.toHaveBeenCalled(); expect(api.runRetention).not.toHaveBeenCalled();
  await wrapper.find('input[type="number"]').setValue(14);
  api.retention.mockResolvedValue({ ...retentionStatus(), revision: 3 }); await wrapper.setProps({ revision: 1 }); await flushPromises();
  expect((wrapper.find('input[type="number"]').element as HTMLInputElement).value).toBe('14');
  await wrapper.find('form').trigger('submit'); await flushPromises();
  expect(api.changeRetention.mock.calls[0]![2].expected_revision).toBe(1); expect(wrapper.text()).toContain('revision conflict');
  vi.useFakeTimers(); const calls = api.retention.mock.calls.length; await vi.advanceTimersByTimeAsync(3600000); expect(api.retention).toHaveBeenCalledTimes(calls);
});
it('requires a preview and separate confirmation, then protects an unknown deletion result from retries', async () => {
  const status = { ...retentionStatus(), policy: { enabled: true, retention_days: 30, batch_limit: 200 } };
  api.retention.mockResolvedValue(status); api.previewRetention.mockResolvedValue({ id: 'preview-fixture', table_id: id, revision: 1, rows: 1, state: 'preview', at: '2026-11-04T00:00:00Z', cutoff: '2026-10-05T00:00:00Z', policy_snapshot: status.policy }); api.runRetention.mockRejectedValue(new Error('response lost'));
  wrapper = mount(Retention, { props: { id }, global: global() }); await flushPromises();
  await wrapper.findAll('[role=tab]').find(b=>b.text()==='预览执行')!.trigger('click');
  await wrapper.findAll('button').find(b => b.text() === '预览可清理记录')!.trigger('click'); await flushPromises();
  let execute = wrapper.findAll('button').find(b => b.text() === '确认删除本批')!; expect(execute.attributes('disabled')).toBeDefined();
  await wrapper.get('[data-testid=system-retention-execute-confirm] input').setValue(true); await execute.trigger('click'); await flushPromises();
  expect(api.runRetention).toHaveBeenCalledTimes(1); expect(wrapper.text()).toContain('请求结果尚未确认');
  execute = wrapper.findAll('button').find(b => b.text() === '确认删除本批')!; expect(execute.attributes('disabled')).toBeDefined();
  await wrapper.setProps({ revision: 4 }); await flushPromises(); expect(api.runRetention).toHaveBeenCalledTimes(1);
});
it('rejects malformed or falsely zero monitoring and retention replies', () => {
  for (const change of [{ policy: { ...tableMonitoring().policy, growth_bytes: null } }, { cleanup: { state: 'success', consecutive_failures: 0 } }, { tables: [{ id, state: 'unavailable', bytes: 0 }] }]) expect(() => decodeTableMonitoring({ ...tableMonitoring(), ...change })).toThrow();
  expect(() => decodeTableRetention({ ...retentionStatus(), policy: { enabled: true, retention_days: 0, batch_limit: 0 } }, id)).toThrow();
});

it('requires explicit adoption, preserves the old source and never enables alerts during adoption',async()=>{
 const status=tableMonitoring();status.policy.enabled=true;
 status.configuration={...status.configuration!,authority:'legacy',state:'legacy',desired_revision:0,applied_revision:0,snapshot_revision:null,desired_policy:null,legacy_revision:7,legacy_enabled:true};
 api.adoptMonitoring.mockImplementation(async(_signal,c,key)=>({request_id:key,operation_revision:1,operation_policy:{...status.policy,enabled:false},configuration:{...c,authority:'database',state:'pending',desired_revision:1,desired_policy:{...status.policy,enabled:false}}}));
 wrapper=mount(Monitor,{props:{status},global:global()});await wrapper.find('button').trigger('click');
 expect(api.adoptMonitoring).not.toHaveBeenCalled();expect(api.saveMonitoring).not.toHaveBeenCalled();
 const adopt=wrapper.get('[data-testid="monitor-adopt"]');expect(adopt.attributes('disabled')).toBeDefined();
 await wrapper.get('[data-testid="monitor-adoption-confirm"] input').setValue(true);await adopt.trigger('click');await flushPromises();
 expect(api.adoptMonitoring).toHaveBeenCalledTimes(1);expect(api.saveMonitoring).not.toHaveBeenCalled();expect(wrapper.text()).toContain('等待发布确认');
 expect((wrapper.get('[data-testid="table-monitoring-enable"] input').element as HTMLInputElement).checked).toBe(false);
});
it('protects an unknown write from replay and reconciles only its original request',async()=>{
 const status=tableMonitoring();api.saveMonitoring.mockRejectedValue(new Error('lost response'));
 wrapper=mount(Monitor,{props:{status},global:global()});await wrapper.find('button').trigger('click');await wrapper.find('form').trigger('submit');await flushPromises();
 expect(wrapper.find('[data-testid="monitor-unknown"]').exists()).toBe(true);
 const [_,policy,revision,key]=api.saveMonitoring.mock.calls[0]!;
 await wrapper.find('form').trigger('submit');await flushPromises();expect(api.saveMonitoring).toHaveBeenCalledTimes(1);
 api.monitoringRequest.mockResolvedValue({request_id:key,operation_revision:revision+1,operation_policy:policy,configuration:{...status.configuration,state:'pending',desired_revision:revision+1,desired_policy:policy}});
 await wrapper.get('[data-testid="monitor-reconcile"]').trigger('click');await flushPromises();
 expect(api.monitoringRequest.mock.calls[0]![1]).toBe(key);expect(wrapper.find('[data-testid="monitor-unknown"]').exists()).toBe(false);expect(api.saveMonitoring).toHaveBeenCalledTimes(1);
});
it('keeps the unknown request when readback is missing or belongs to a different submission',async()=>{
 const status=tableMonitoring();api.saveMonitoring.mockRejectedValue(new Error('lost response'));
 wrapper=mount(Monitor,{props:{status},global:global()});await wrapper.find('button').trigger('click');await wrapper.find('form').trigger('submit');await flushPromises();
 api.monitoringRequest.mockRejectedValueOnce({statusCode:404});await wrapper.get('[data-testid="monitor-reconcile"]').trigger('click');await flushPromises();
 expect(wrapper.find('[data-testid="monitor-unknown"]').exists()).toBe(true);
 const [_,policy,,key]=api.saveMonitoring.mock.calls[0]!;
 api.monitoringRequest.mockResolvedValueOnce({request_id:key,operation_revision:2,operation_policy:{...policy,enabled:!policy.enabled},configuration:{...status.configuration}});
 await wrapper.get('[data-testid="monitor-reconcile"]').trigger('click');await flushPromises();expect(wrapper.find('[data-testid="monitor-unknown"]').exists()).toBe(true);expect(api.saveMonitoring).toHaveBeenCalledTimes(1);
});
it('keeps a conflicting draft and requires an explicit latest-version read to replace it',async()=>{
 const status=tableMonitoring();api.saveMonitoring.mockRejectedValue({statusCode:409,message:'revision conflict'});
 wrapper=mount(Monitor,{props:{status},global:global()});await wrapper.find('button').trigger('click');await wrapper.get('[data-testid="table-growth-mib"]').setValue(128);
 await wrapper.find('form').trigger('submit');await flushPromises();expect((wrapper.get('[data-testid="table-growth-mib"]').element as HTMLInputElement).value).toBe('128');
 expect(wrapper.find('[data-testid="monitor-unknown"]').exists()).toBe(false);
 api.monitoring.mockResolvedValue({...status,configuration:{...status.configuration,desired_revision:4,applied_revision:4,snapshot_revision:4,desired_policy:{...status.policy,growth_bytes:512*2**20}}});
 await wrapper.findAll('button').find(b=>b.text()==='放弃草稿并载入最新版本')!.trigger('click');await flushPromises();
 expect((wrapper.get('[data-testid="table-growth-mib"]').element as HTMLInputElement).value).toBe('512');expect(api.monitoring).toHaveBeenCalledTimes(1);
});
it('treats an older backend as read-only and makes no periodic configuration requests',async()=>{
 const status=tableMonitoring();delete status.configuration;
 wrapper=mount(Monitor,{props:{status},global:global()});await wrapper.find('button').trigger('click');expect(wrapper.find('form').exists()).toBe(false);
 vi.useFakeTimers();await vi.advanceTimersByTimeAsync(3600000);expect(api.monitoring).not.toHaveBeenCalled();expect(api.saveMonitoring).not.toHaveBeenCalled();expect(api.adoptMonitoring).not.toHaveBeenCalled();
});
it('reports the confirmed saved policy so the table list can update without polling', async () => {
  const original = retentionStatus(), saved = { ...original, revision: 2, policy: { ...original.policy, retention_days: 14 } };
  api.retention.mockResolvedValue(original); api.changeRetention.mockImplementation(async(_signal,id,input)=>{api.retention.mockResolvedValue(saved);return {table_id:id,request_id:input.request_id,operation_revision:2,configuration:{...original.configuration,desired_revision:2}}});
  wrapper = mount(Retention, { props: { id }, global: global() }); await flushPromises();
  await wrapper.find('input[type="number"]').setValue(14);
  await wrapper.find('form').trigger('submit'); await flushPromises();
  expect(wrapper.emitted('saved')?.at(-1)).toEqual([saved]);
});
