import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { mount, flushPromises } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import Panel from '~/components/ops/backup/BackupCleanupReviews.vue';
import { decodeCleanupReviews, decodeCleanupInspection } from '~/composables/api/services/backupCleanupReviewService';
import zh from '../../../i18n/locales/zh.json';
const api = vi.hoisted(() => ({ list: vi.fn(), inspect: vi.fn(), verify: vi.fn() }));
const state = vi.hoisted(() => ({ callback: undefined as undefined | (() => void), session: { sessionId:'one', tenantUuid:'tenant', status:'active' } as any }));
vi.mock('~/composables/api/services/backupCleanupReviewService', async original => ({ ...(await original<any>()), useBackupCleanupReviewService: () => api }));
vi.mock('~/composables/useBackupEvents', () => ({ useBackupEvents: (resource: string, callback: () => void) => { expect(resource).toBe('backup_cleanup'); state.callback = callback; } }));
vi.mock('~/utils/auth-session', () => ({ AUTH_SESSION_EVENT:'fixture:identity', readAuthSession: () => state.session }));
const row = () => ({ artifact_id:'1', job_id:'2', policy_id:'3', state:'unconfirmed', token:'7a3bf1aa-2735-458a-bd5b-b9a4e102c0bd', size_bytes:123, updated_at:'2026-11-09T01:00:00Z', protected:false, inspectable:true });
const list = () => ({ items:[row()], total:1, page:1, page_size:20, observed_at:'2026-11-09T01:00:00Z' });
const detail = () => ({ item:row(), state:'present_unverified', files:[{role:'dump',state:'present',size_bytes:123}], read_only:true,content_verified:false,observed_at:'2026-11-09T01:00:00Z' });
const stubs = { UIcon:true,
 UAlert:{props:['title','description'],template:'<div>{{title}} {{description}}</div>'},UBadge:{template:'<span><slot/></span>'},
 UButton:{props:['disabled','loading'],template:'<button :disabled="disabled || loading"><slot/></button>'},
 UModal:{props:['open'],emits:['update:open'],template:'<section v-if="open"><slot name="body"/><slot name="footer"/></section>'},
};
let wrapper:ReturnType<typeof mount>|undefined;
const mountPanel=async()=>{wrapper=mount(Panel,{global:{plugins:[createI18n({legacy:false,locale:'zh',messages:{zh}})],stubs}});await flushPromises();return wrapper};
beforeEach(()=>{vi.clearAllMocks();api.list.mockResolvedValue(list());api.inspect.mockResolvedValue(detail());state.session={sessionId:'one',tenantUuid:'tenant',status:'active'}});
afterEach(()=>{wrapper?.unmount();wrapper=undefined;vi.useRealTimers()});
describe('backup cleanup review',()=>{
 it('strictly decodes read-only unverified results',()=>{
  expect(decodeCleanupReviews({data:list()}).items[0].artifact_id).toBe('1');expect(decodeCleanupInspection(detail()).read_only).toBe(true);
  for(const change of [{read_only:false},{content_verified:true},{files:[{role:'dump',state:'present'},{role:'dump',state:'missing'}]},{state:'deleted'}])expect(()=>decodeCleanupInspection({...detail(),...change})).toThrow();
  for(const change of [{artifact_id:1},{token:'bad'},{updated_at:'bad'}])expect(()=>decodeCleanupReviews({...list(),items:[{...row(),...change}]})).toThrow();
  expect(decodeCleanupReviews({...list(),items:[{...row(),size_bytes:-1}]}).items[0].size_bytes).toBe(-1);
 });
 it('renders metadata and navigates to the original job',async()=>{
  const w=await mountPanel();await w.get('[data-review-id="1"]').trigger('click');await flushPromises();expect(api.inspect).toHaveBeenCalledWith('1',row().token,expect.any(AbortSignal));expect(w.text()).toContain(zh.backupReview.noProof);await w.get('[data-testid="review-open-job"]').trigger('click');expect(w.emitted('open-job')).toEqual([['2']]);
 });
 it('keeps frozen intent on push and never polls',async()=>{
  const w=await mountPanel();await w.get('[data-review-id="1"]').trigger('click');await flushPromises();api.list.mockResolvedValue({...list(),items:[{...row(),token:'ac5369d5-616f-4532-a089-11fcf34d9ab4'}]});state.callback?.();await flushPromises();expect(w.get('[data-testid="review-frozen-token"]').text()).toBe(row().token);expect(w.text()).toContain(zh.backupReview.stale);expect(api.inspect).toHaveBeenCalledTimes(1);vi.useFakeTimers();await vi.advanceTimersByTimeAsync(60000);expect(api.list).toHaveBeenCalledTimes(2);
 });
 it('preserves rows and exposes unknown status on failure',async()=>{
  const w=await mountPanel();api.list.mockRejectedValue(new Error('raw SQL password'));await w.get('[data-testid="review-refresh"]').trigger('click');await flushPromises();expect(w.text()).toContain(zh.backupReview.loadFailed);expect(w.text()).not.toContain('password');expect(w.find('[data-review-id="1"]').exists()).toBe(true);expect(w.emitted('attention')?.at(-1)).toEqual([{count:null,problem:true}]);
 });
 it('aborts and clears identity-owned detail',async()=>{
  const w=await mountPanel();let complete:(value:any)=>void=()=>{};api.inspect.mockImplementation(()=>new Promise(done=>complete=done));await w.get('[data-review-id="1"]').trigger('click');const signal=api.inspect.mock.calls[0][2];state.session={sessionId:'two',tenantUuid:'other',status:'active'};window.dispatchEvent(new Event('fixture:identity'));await flushPromises();expect(signal.aborted).toBe(true);complete(detail());await flushPromises();expect(w.find('[data-testid="review-frozen-token"]').exists()).toBe(false);
 });
 it('requires explicit reconciliation when the row leaves the page',async()=>{
  const w=await mountPanel();await w.get('[data-review-id="1"]').trigger('click');await flushPromises();api.list.mockResolvedValue({...list(),items:[],total:0});await w.get('[data-testid="review-reload-intent"]').trigger('click');await flushPromises();expect(w.text()).toContain(zh.backupReview.noLongerListed);expect(api.inspect).toHaveBeenCalledTimes(1);
 });
 it('starts a check only on demand and stops it explicitly',async()=>{
  const w=await mountPanel();await w.get('[data-review-id="1"]').trigger('click');await flushPromises();expect(api.verify).not.toHaveBeenCalled();
  let done:(v:any)=>void=()=>{};api.verify.mockImplementation(()=>new Promise(resolve=>done=resolve));await w.get('[data-testid="review-open-verification"]').trigger('click');await w.get('[data-testid="review-verify"]').trigger('click');const signal=api.verify.mock.calls[0][2];expect(w.find('[data-testid="review-stop-verify"]').exists()).toBe(true);
  await w.get('[data-testid="review-stop-verify"]').trigger('click');expect(signal.aborted).toBe(true);expect(w.text()).toContain(zh.backupReview.verify.incomplete);done({state:'verified'});await flushPromises();expect(w.find('[data-testid="verification-result"]').exists()).toBe(false);
 });
 it('accepts a final result but does not clear the cleanup state or submit another call',async()=>{
  const w=await mountPanel();await w.get('[data-review-id="1"]').trigger('click');await flushPromises();api.verify.mockResolvedValue({state:'verified',content_verified:true});await w.get('[data-testid="review-open-verification"]').trigger('click');await w.get('[data-testid="review-verify"]').trigger('click');await flushPromises();expect(w.text()).toContain(zh.backupReview.verify.states.verified);expect(w.text()).toContain(zh.backupReview.states.unconfirmed);expect(api.verify).toHaveBeenCalledTimes(1);
 });

 it('aborts the integrity stream when the modal closes or the owner changes',async()=>{
  for(const change of ['close','identity']){
   const w=await mountPanel();await w.get('[data-review-id="1"]').trigger('click');await flushPromises();api.verify.mockImplementation(()=>new Promise(()=>{}));await w.get('[data-testid="review-open-verification"]').trigger('click');await w.get('[data-testid="review-verify"]').trigger('click');const signal=api.verify.mock.calls.at(-1)?.[2];
   if(change==='close'){await w.findAll('button').find(b=>b.text()===zh.backupReview.close)!.trigger('click')}else{state.session={sessionId:'changed',tenantUuid:'other',status:'active'};window.dispatchEvent(new Event('fixture:identity'))}
   await flushPromises();expect(signal.aborted).toBe(true);expect(w.find('[data-testid="verification-result"]').exists()).toBe(false);w.unmount();wrapper=undefined;
  }
 });

 it('preserves a running integrity read across unchanged push or reconnect synchronization',async()=>{
  const w=await mountPanel();await w.get('[data-review-id="1"]').trigger('click');await flushPromises();let done:(v:any)=>void=()=>{};api.verify.mockImplementation(()=>new Promise(resolve=>done=resolve));await w.get('[data-testid="review-open-verification"]').trigger('click');await w.get('[data-testid="review-verify"]').trigger('click');const signal=api.verify.mock.calls[0][2];
  state.callback?.();await flushPromises();expect(signal.aborted).toBe(false);expect(w.text()).not.toContain(zh.backupReview.stale);
  await w.findAll('[role="tab"]').find(b=>b.text()===zh.backupReview.metadataTab)!.trigger('click');expect(signal.aborted).toBe(false);await w.findAll('[role="tab"]').find(b=>b.text()===zh.backupReview.verify.title)!.trigger('click');expect(w.find('[data-testid="review-stop-verify"]').exists()).toBe(true);
  done({state:'verified',content_verified:true});await flushPromises();expect(w.get('[data-testid="verification-result"]').text()).toContain(zh.backupReview.verify.states.verified);
 });

});
