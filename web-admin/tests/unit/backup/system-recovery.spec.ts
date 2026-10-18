import { mount, flushPromises } from '@vue/test-utils';
vi.mock('~/composables/useBackupEvents', () => import('../../support/backup-events-stub'));
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
const auth=vi.hoisted(()=>({session:null as any}));
vi.mock('~/utils/auth-session',()=>({AUTH_SESSION_EVENT:'powerx:auth-session-changed',readAuthSession:()=>auth.session}));
const service=vi.hoisted(()=>({overview:vi.fn(),restore:vi.fn(),prepare:vi.fn(),cutover:vi.fn(),progress:vi.fn(),watchProgress:vi.fn(),environmentAction:vi.fn()}));
vi.mock('~/composables/api/services/systemRecoveryService',()=>({useSystemRecoveryService:()=>service}));
import Panel from '~/components/ops/backup/SystemRecoveryPanel.vue';
const stubs={
 UCard:{template:'<div><slot name="header"/><slot/></div>'},
 UButton:{props:['disabled','loading'],template:'<button :disabled="disabled || loading"><slot/></button>'},
 UBadge:{template:'<span><slot/></span>'},UAlert:{props:['title'],template:'<p>{{title}}</p>'},
 UModal:{props:['open'],template:'<section v-if="open"><slot name="body"/><slot name="footer"/></section>'},
 UFormField:{template:'<label><slot/></label>'},
 USelect:{props:['modelValue','items'],emits:['update:modelValue'],template:'<select :value="modelValue" @change="$emit(\'update:modelValue\',$event.target.value)"><option v-for="item in items" :key="item.value" :value="item.value">{{item.label}}</option></select>'},
 UCheckbox:{props:['modelValue','label'],emits:['update:modelValue'],template:'<label><input type="checkbox" :checked="modelValue" @change="$emit(\'update:modelValue\',$event.target.checked)"/>{{label}}</label>'}
};
const verified={id:'candidate',kind:'prepare',snapshot_id:'snapshot',target_id:'formal',status:'verified',phase:'awaiting_review',business_verified:true,writes_may_have_resumed:false,checks:[{name:'database_rows',passed:true,detail:'All business rows restored'}],created_at:'2026-10-15T00:00:00Z'};
const overview=()=>({configured:true,online:true,generation:3,targets:[{id:'local',label:'Development',mode:'isolated',environment:'dev'},{id:'formal',label:'Production candidate',mode:'formal',environment:'prod',allow_overwrite:true}],jobs:[],progress_tokens:{}});
const wrappers:ReturnType<typeof mount>[]=[];
const panel=()=>{const w=mount(Panel,{props:{snapshots:[{id:'snapshot',status:'ready',recovery_supported:true,version:'v1',created_at:'2026-10-15T00:00:00Z'} as any]},global:{stubs}});wrappers.push(w);return w};
const button=(w:ReturnType<typeof mount>,key:string)=>w.findAll('button').find(item=>item.text()===key)!;
beforeEach(()=>{auth.session=null;vi.clearAllMocks();service.watchProgress.mockReturnValue(vi.fn());vi.stubGlobal('useI18n',()=>({t:(key:string)=>key,te:()=>true}));service.overview.mockResolvedValue(overview());service.restore.mockResolvedValue({recovery:{...verified,status:'queued',business_verified:false},progress_token:'readonly-ticket'})});
afterEach(()=>{wrappers.splice(0).forEach(w=>w.unmount());vi.useRealTimers()});
describe('restore current instance',()=>{
 it('continues a confirmed restore at the same-origin maintenance page',async()=>{
  const navigate=vi.fn();vi.stubGlobal('navigateTo',navigate);
  service.overview.mockResolvedValue({...overview(),recovery_entry:'/__powerx_recovery/'});
  const w=panel();await flushPromises();await button(w,'backupCenter.recovery.prepare').trigger('click');await w.find('input[type="checkbox"]').setValue(true);await button(w,'backupCenter.confirm').trigger('click');await flushPromises();
  expect(navigate).toHaveBeenCalledWith('/__powerx_recovery/',{external:true});
 });
 it('links only to the recognized same-origin independent recovery console',async()=>{
  service.overview.mockResolvedValue({...overview(),recovery_entry:'/__powerx_recovery/'});const w=panel();await flushPromises();
  expect(button(w,'backupCenter.recovery.console').attributes('href')).toBe('/__powerx_recovery/');
  service.overview.mockResolvedValue({...overview(),recovery_entry:'https://another.invalid/'});const other=panel();await flushPromises();
  expect(button(other,'backupCenter.recovery.console')).toBeUndefined();
 });
 it('reports offline availability to the snapshot list and does not open a confirmation',async()=>{
  service.overview.mockResolvedValue({...overview(),online:false});const w=panel();await flushPromises();
  expect(w.emitted('availability')?.at(-1)?.[0]).toMatchObject({configured:true,online:false,status:'offline'});
  (w.vm as any).openForSnapshot('snapshot');await flushPromises();
  expect(w.find('input[type="checkbox"]').exists()).toBe(false);expect(service.restore).not.toHaveBeenCalled();
 });
 it('uses one confirmation and only the observed generation, without environment selection',async()=>{const w=panel();await flushPromises();await button(w,'backupCenter.recovery.prepare').trigger('click');expect(w.findAll('select')).toHaveLength(1);expect(button(w,'backupCenter.confirm').attributes('disabled')).toBeDefined();await w.find('input[type="checkbox"]').setValue(true);await button(w,'backupCenter.confirm').trigger('click');await flushPromises();expect(service.restore).toHaveBeenCalledWith('snapshot',3,expect.any(String));expect(w.text()).toContain('backupCenter.recovery.states.queued');expect(button(w,'backupCenter.recovery.cutover')).toBeUndefined()});
 it('preserves the idempotency key after an uncertain submission',async()=>{service.restore.mockRejectedValueOnce(new Error('network lost'));const w=panel();await flushPromises();await button(w,'backupCenter.recovery.prepare').trigger('click');await w.find('input[type="checkbox"]').setValue(true);await button(w,'backupCenter.confirm').trigger('click');await flushPromises();await button(w,'backupCenter.confirm').trigger('click');await flushPromises();expect(service.restore.mock.calls[0]?.[2]).toEqual(service.restore.mock.calls[1]?.[2])});
 it('does not expose legacy switch, return, preview or cleanup actions',async()=>{service.overview.mockResolvedValue({...overview(),last_switch_id:'legacy',jobs:[verified]});const w=panel();await flushPromises();for(const key of ['cutover','rollback','cleanup','openPreview'])expect(button(w,`backupCenter.recovery.${key}`)).toBeUndefined()});
 it('uses independent push during maintenance, makes no periodic requests, and releases the stream',async()=>{
 vi.useFakeTimers();const stop=vi.fn();service.watchProgress.mockReturnValue(stop);const running={...verified,id:'restore',kind:'restore',status:'running',phase:'maintenance'};service.overview.mockResolvedValue({...overview(),maintenance:true,jobs:[running],progress_tokens:{restore:'ticket'}});const w=panel();await flushPromises();expect(service.watchProgress).toHaveBeenCalledWith('restore','ticket',expect.any(Function),expect.any(Function));await vi.advanceTimersByTimeAsync(30000);expect(service.overview).toHaveBeenCalledTimes(1);expect(service.progress).not.toHaveBeenCalled();service.watchProgress.mock.calls[0][2]({...running,phase:'instance_replaced'});await flushPromises();expect(w.text()).toContain('instance_replaced');w.unmount();expect(stop).toHaveBeenCalledTimes(1);
 });
 it('blocks another restore during a pending task or unresolved maintenance',async()=>{for(const status of ['queued','running','attention_required']){service.overview.mockResolvedValue({...overview(),jobs:[{...verified,kind:'restore',status}]});const w=panel();await flushPromises();expect(button(w,'backupCenter.recovery.prepare').attributes('disabled')).toBeDefined()}});
 it('restores a protection snapshot through the same operation',async()=>{service.overview.mockResolvedValue({...overview(),jobs:[{...verified,kind:'restore',status:'complete',rollback_snapshot_id:'snapshot'}]});const w=panel();await flushPromises();await button(w,'backupCenter.recovery.restoreProtection').trigger('click');await w.find('input[type="checkbox"]').setValue(true);await button(w,'backupCenter.confirm').trigger('click');await flushPromises();expect(service.restore).toHaveBeenCalledWith('snapshot',3,expect.any(String))});
 it('requires a registered installation before enabling destructive restore',async()=>{service.overview.mockResolvedValue({...overview(),targets:[]});const w=panel();await flushPromises();expect(button(w,'backupCenter.recovery.prepare').attributes('disabled')).toBeDefined();expect(w.text()).toContain('backupCenter.recovery.installationRequired')});
 it('releases old progress tickets when the signed-in identity changes',async()=>{
  auth.session={sessionId:'A',tenantUuid:'tenant',status:'active'};const stop=vi.fn();service.watchProgress.mockReturnValue(stop);
  service.overview.mockResolvedValueOnce({...overview(),jobs:[{...verified,id:'restore',kind:'restore',status:'running'}],progress_tokens:{restore:'ticket-A'}}).mockResolvedValue({...overview(),jobs:[]});
  const w=panel();await flushPromises();expect(service.watchProgress).toHaveBeenCalledTimes(1);
  auth.session={sessionId:'B',tenantUuid:'tenant',status:'active'};window.dispatchEvent(new CustomEvent('powerx:auth-session-changed'));await flushPromises();
  expect(stop).toHaveBeenCalledTimes(1);expect(service.watchProgress).toHaveBeenCalledTimes(1);expect(w.text()).not.toContain('ticket-A');expect(service.overview).toHaveBeenCalledTimes(2);
  service.watchProgress.mock.calls[0][2]({...verified,id:'old-identity-event',status:'running'});await flushPromises();expect(w.text()).toContain('backupCenter.recovery.empty');
 });

});
