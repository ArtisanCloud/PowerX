import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { flushPromises, mount } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import { defineComponent } from 'vue';
import Panel from '~/components/monitor/storage/StorageExecutionControl.vue';
import { decodeExecutionConfiguration, decodeExecutionMutation } from '~/composables/api/services/storageExecutionService';
import zh from '../../../i18n/locales/zh.json';
const mocks = vi.hoisted(() => ({ configuration: vi.fn(), change: vi.fn(), request: vi.fn(), get: vi.fn(), post: vi.fn() }));
vi.mock('~/composables/api/services/storageExecutionService', async original => ({ ...await original<any>(), useStorageExecutionService: () => mocks }));
vi.mock('~/composables/api/index', () => ({ useApiClient: () => mocks }));
const stubs = {
  UIcon: true,
  UButton: { props: ['disabled', 'loading'], template: '<button :disabled="disabled || loading"><slot /></button>' },
  UAlert: { props: ['title', 'description'], template: '<div>{{title}}{{description}}<slot name="actions" /></div>' },
  UModal: { props: ['open', 'dismissible', 'close'], template: '<section v-if="open" :data-close="close" :data-dismissible="dismissible"><slot name="body" /></section>' },
  UForm: { emits: ['submit'], template: '<form @submit.prevent="$emit(\'submit\')"><slot /></form>' },
  UFormField: { props: ['label'], template: '<label>{{label}}<slot /></label>' },
  UTextarea: defineComponent({ props: ['modelValue', 'disabled'], emits: ['update:modelValue'], template: '<textarea :value="modelValue" :disabled="disabled" @input="$emit(\'update:modelValue\',$event.target.value)" />' }),
  UCheckbox: defineComponent({ props: ['modelValue', 'disabled', 'label'], emits: ['update:modelValue'], template: '<label><input type="checkbox" :checked="modelValue" :disabled="disabled" @change="$emit(\'update:modelValue\',$event.target.checked)" />{{label}}</label>' }),
};
const config = (revision = 1, mode = 'paused') => ({ mode, desired_revision: revision, applied_revision: revision, confirmed: true, can_pause: true, can_resume: mode === 'paused', worker_ready: true, policies: [] });
const copy = (x: any) => JSON.parse(JSON.stringify(x));
const views: ReturnType<typeof mount>[] = [];
let current: any;
async function view() { const w = mount(Panel, { props: { revision: 0 }, global: { stubs, plugins: [createI18n({ legacy: false, locale: 'zh', messages: { zh } })] } }); views.push(w); await flushPromises(); return w; }
async function prepare(w: any) { await w.get('[data-testid=execution-emergency]').trigger('click'); await w.get('[data-testid=execution-reason]').setValue('手动紧急暂停验收'); await w.get('[data-testid=execution-confirm] input').setValue(true); }
async function submit(w: any) { await w.get('form').trigger('submit'); await flushPromises(); }
beforeEach(() => { vi.resetAllMocks(); current = config(); mocks.configuration.mockImplementation(async () => copy(current)); });
afterEach(() => { views.splice(0).forEach(w => w.unmount()); vi.useRealTimers(); });
describe('managed execution protection', () => {
  it('preserves a frozen CAS reason across SSE and task navigation; refresh is explicit', async () => {
    const w = await view(); await prepare(w); current = config(3); await w.setProps({ revision: 1 }); await flushPromises();
    await w.get('[role=tab][aria-label="执行状态"]').trigger('click'); await w.get('[role=tab][aria-label="操作确认"]').trigger('click');
    expect((w.get('[data-testid=execution-reason]').element as HTMLTextAreaElement).value).toBe('手动紧急暂停验收'); expect(w.find('[data-testid=execution-stale]').exists()).toBe(true);
    mocks.change.mockRejectedValue({ status: 409, message: 'STORAGE_CONTROL_REVISION_CONFLICT' }); await submit(w);
    expect(mocks.change.mock.calls[0][0].expected_revision).toBe(1); expect(w.find('[data-testid=execution-unknown]').exists()).toBe(false);
    await w.get('[data-testid=execution-latest]').trigger('click'); await flushPromises(); expect(w.find('[data-testid=execution-stale]').exists()).toBe(false); expect((w.get('[data-testid=execution-reason]').element as HTMLTextAreaElement).value).toBe('手动紧急暂停验收');
  });
  it('locks unknown writes and modal closing until the exact original UUID receipt is found', async () => {
    const w = await view(); await prepare(w); mocks.change.mockRejectedValue({ status: 502 }); await submit(w);
    const sent = mocks.change.mock.calls[0][0]; expect(w.find('[data-testid=execution-unknown]').exists()).toBe(true); expect(w.get('[data-close]').attributes('data-close')).toBe('false');
    mocks.request.mockRejectedValue({ status: 404 }); await w.get('[data-testid=execution-reconcile]').trigger('click'); await flushPromises(); expect(w.find('[data-testid=execution-unknown]').exists()).toBe(true);
    mocks.request.mockResolvedValue({ request_id: sent.request_id, operation_revision: 2, operation: { ...sent, reason: '错误的回执内容' }, configuration: config(2) }); await w.get('[data-testid=execution-reconcile]').trigger('click'); await flushPromises(); expect(w.find('[data-testid=execution-unknown]').exists()).toBe(true);
    mocks.request.mockResolvedValue({ request_id: sent.request_id, operation_revision: 2, operation: sent, configuration: config(2) }); await w.get('[data-testid=execution-reconcile]').trigger('click'); await flushPromises(); expect(w.find('[data-testid=execution-unknown]').exists()).toBe(false); expect(mocks.change).toHaveBeenCalledTimes(1); expect(mocks.request.mock.calls.every(x => x[0] === sent.request_id)).toBe(true);
  });
  it('allows pause with an unavailable worker but blocks resume', async () => {
    current.worker_ready = false; current.can_resume = false; const w = await view(); expect(w.get('[data-testid=execution-emergency]').attributes('disabled')).toBeUndefined(); await w.get('[data-testid=execution-manage]').trigger('click'); expect(w.get('[data-testid=execution-resume]').attributes('disabled')).toBeDefined();
    await prepare(w); mocks.change.mockImplementation(async op => ({ request_id: op.request_id, operation_revision: 2, operation: op, configuration: { ...current, desired_revision: 2, confirmed: false } })); await submit(w); expect(mocks.change).toHaveBeenCalledTimes(1); expect(w.text()).toContain('已暂停受控执行');
  });
  it('requires policy pauses and both operator declarations for handoff completion', async () => {
    current = { ...config(1, 'handoff'), policies: [{ domain: 'database-retention', kind: 'invocations', paused: false, confirmed: true }] }; const w = await view(); await w.get('[data-testid=execution-manage]').trigger('click'); await w.get('[role=tab][aria-label="执行器交接"]').trigger('click'); expect(w.get('[data-testid=execution-complete]').attributes('disabled')).toBeDefined();
    current.policies[0].paused = true; await w.setProps({ revision: 1 }); await flushPromises(); await w.get('[data-testid=execution-complete]').trigger('click'); await w.get('[data-testid=execution-reason]').setValue('旧执行器交接核对完成'); await w.get('[data-testid=execution-confirm] input').setValue(true); expect(w.get('[data-testid=execution-submit]').attributes('disabled')).toBeDefined(); await w.get('[data-testid=execution-external] input').setValue(true); await w.get('[data-testid=execution-tasks] input').setValue(true); expect(w.get('[data-testid=execution-submit]').attributes('disabled')).toBeUndefined();
    mocks.change.mockImplementation(async op => ({ request_id: op.request_id, operation_revision: 2, operation: op, configuration: config(2) })); await submit(w); expect(mocks.change.mock.calls[0][0]).toMatchObject({ action: 'complete_handoff', external_stopped: true, admitted_tasks_checked: true }); expect(w.text()).toContain('已暂停受控执行');
  });
  it('has no idle polling and fails closed for old backends or revision rollback', async () => {
    const w = await view(); vi.useFakeTimers(); await vi.advanceTimersByTimeAsync(60000); expect(mocks.configuration).toHaveBeenCalledTimes(1); vi.useRealTimers(); current = { ...config(0, 'unmanaged'), confirmed: false }; await w.setProps({ revision: 1 }); await flushPromises(); expect(w.get('[data-testid=execution-emergency]').attributes('disabled')).toBeDefined(); expect(w.text()).toContain('执行状态待确认');
    mocks.configuration.mockRejectedValue({ status: 404 }); const old = await view(); expect(old.get('[data-testid=execution-emergency]').attributes('disabled')).toBeDefined();
  });
  it('rejects malformed status and impossible permission/publication combinations', () => {
    expect(decodeExecutionConfiguration(config()).mode).toBe('paused'); expect(() => decodeExecutionConfiguration({ ...config(), mode: 'handoff', can_resume: true })).toThrow(); expect(() => decodeExecutionConfiguration({ ...config(), applied_revision: 0 })).toThrow(); expect(() => decodeExecutionMutation({ request_id: crypto.randomUUID(), operation_revision: 1, operation: {} })).toThrow(); const recovery={state:'pending',epoch:crypto.randomUUID(),job_id:crypto.randomUUID(),snapshot_id:crypto.randomUUID(),created_at:new Date().toISOString()};expect(()=>decodeExecutionConfiguration({...config(2,'open'),recovery})).toThrow();const op={request_id:crypto.randomUUID(),expected_revision:0,action:'pause',reason:'typed reply',confirm:true,external_stopped:false,admitted_tasks_checked:false};expect(()=>decodeExecutionMutation({request_id:op.request_id,operation_revision:1,operation:{...op,environment_checked:0},configuration:config()})).toThrow();
  });
  it('does not show permission to execute when an open gate loses its publisher', async () => {
    current = { ...config(2, 'open'), worker_ready: false }; const w = await view(); expect(w.get('[data-testid=execution-status]').text()).toBe(zh.storageGov.execution.openPending);
  });
  it('uses the shared API client, relative routes, cancellation and no write retries', async () => {
    const original = await vi.importActual<typeof import('~/composables/api/services/storageExecutionService')>('~/composables/api/services/storageExecutionService');
    const api = original.useStorageExecutionService(), signal = new AbortController().signal;
    mocks.get.mockResolvedValue({ data: config() }); await api.configuration(signal);
    expect(mocks.get).toHaveBeenCalledWith('/admin/ops/storage/execution-control', expect.objectContaining({ signal, retry: 0, useGlobalLoading: false }));
    const op = { request_id: crypto.randomUUID(), expected_revision: 1, action: 'pause' as const, reason: '原因与原请求', confirm: true, external_stopped: false, admitted_tasks_checked: false };
    mocks.post.mockResolvedValue({ data: { request_id: op.request_id, operation_revision: 2, operation: op, configuration: config(2) } }); await api.change(op, signal);
    expect(mocks.post).toHaveBeenCalledWith('/admin/ops/storage/execution-control/changes', op, expect.objectContaining({ retry: 0, signal }));
  });
  it('requires all restore declarations and keeps acknowledgement separate from resume', async () => {
    const epoch=crypto.randomUUID();current={...config(2,'recovery'),confirmed:false,can_pause:false,can_resume:false,recovery:{state:'pending',prepared:true,resources:[],epoch,job_id:crypto.randomUUID(),snapshot_id:crypto.randomUUID(),created_at:new Date().toISOString()}};
    const w=await view();expect(w.get('[data-testid=execution-emergency]').attributes('disabled')).toBeDefined();await w.get('[data-testid=execution-recovery-open]').trigger('click');await w.get('[data-testid=execution-recovery-acknowledge]').trigger('click');await w.get('[data-testid=execution-reason]').setValue('本次恢复已经逐项核对');await w.get('[data-testid=execution-confirm] input').setValue(true);await w.get('[data-testid=execution-external] input').setValue(true);await w.get('[data-testid=execution-tasks] input').setValue(true);expect(w.get('[data-testid=execution-submit]').attributes('disabled')).toBeDefined();await w.get('[data-testid=execution-environment] input').setValue(true);
    mocks.change.mockImplementation(async op=>({request_id:op.request_id,operation_revision:3,operation:op,configuration:{...config(3),recovery:{...current.recovery,state:'acknowledged'}}}));await submit(w);expect(mocks.change.mock.calls[0][0]).toMatchObject({action:'acknowledge_restore',expected_recovery_epoch:epoch,environment_checked:true});expect(w.text()).toContain('已暂停受控执行');expect(mocks.change).toHaveBeenCalledTimes(1);
  });
  it('rebuilds before acknowledgement, accepts a version jump and waits for resource publication events', async () => {
    const epoch=crypto.randomUUID();current={...config(2,'recovery'),confirmed:false,can_pause:false,can_resume:false,recovery:{state:'pending',prepared:false,resources:[],epoch,job_id:crypto.randomUUID(),snapshot_id:crypto.randomUUID(),created_at:new Date().toISOString()}};
    const w=await view();await w.get('[data-testid=execution-recovery-open]').trigger('click');expect(w.get('[data-testid=execution-recovery-acknowledge]').attributes('disabled')).toBeDefined();
    await w.get('[data-testid=execution-recovery-prepare]').trigger('click');await w.get('[data-testid=execution-reason]').setValue('核对旧执行器并重建跨资源版本');for(const id of ['execution-confirm','execution-external','execution-tasks','execution-environment'])await w.get(`[data-testid=${id}] input`).setValue(true);
    mocks.change.mockImplementation(async op=>{current={...current,desired_revision:12,recovery:{...current.recovery,prepared:true,resources:[{domain:'host-budget',kind:'budget',revision:15,state:'pending',problem:'STORAGE_EXECUTION_PUBLICATION_PENDING'}]}};return {request_id:op.request_id,operation_revision:12,operation:op,configuration:copy(current)}});await submit(w);
    expect(mocks.change.mock.calls[0][0]).toMatchObject({action:'prepare_restore',expected_revision:2,expected_recovery_epoch:epoch});expect(w.find('[data-testid=execution-unknown]').exists()).toBe(false);expect(w.get('[data-testid=execution-recovery-acknowledge]').attributes('disabled')).toBeDefined();
    current.applied_revision=12;current.recovery.resources[0].state='ready';await w.setProps({revision:1});await flushPromises();expect(w.get('[data-testid=execution-recovery-acknowledge]').attributes('disabled')).toBeUndefined();expect(mocks.change).toHaveBeenCalledTimes(1);
    vi.useFakeTimers();const reads=mocks.configuration.mock.calls.length;await vi.advanceTimersByTimeAsync(120000);expect(mocks.configuration).toHaveBeenCalledTimes(reads);
  });
  it('freezes the restore epoch across SSE; explicit latest clears old declarations', async () => {
    const epoch=crypto.randomUUID();current={...config(0,'recovery'),confirmed:false,can_pause:false,can_resume:false,recovery:{state:'pending',prepared:true,resources:[],epoch,job_id:crypto.randomUUID(),snapshot_id:crypto.randomUUID(),created_at:new Date().toISOString()}};
    const w=await view();await w.get('[data-testid=execution-recovery-open]').trigger('click');await w.get('[data-testid=execution-recovery-acknowledge]').trigger('click');await w.get('[data-testid=execution-reason]').setValue('原来恢复任务的核对原因');for(const id of ['execution-confirm','execution-external','execution-tasks','execution-environment'])await w.get(`[data-testid=${id}] input`).setValue(true);
    current.recovery.epoch=crypto.randomUUID();await w.setProps({revision:1});await flushPromises();expect(w.find('[data-testid=execution-stale]').exists()).toBe(true);mocks.change.mockRejectedValue({status:409,message:'STORAGE_RECOVERY_EPOCH_CHANGED'});await submit(w);expect(mocks.change.mock.calls[0][0].expected_recovery_epoch).toBe(epoch);await w.get('[data-testid=execution-latest]').trigger('click');await flushPromises();for(const id of ['execution-confirm','execution-external','execution-tasks','execution-environment'])expect((w.get(`[data-testid=${id}] input`).element as HTMLInputElement).checked).toBe(false);expect((w.get('[data-testid=execution-reason]').element as HTMLTextAreaElement).value).toBe('原来恢复任务的核对原因');
  });
  it('keeps an unknown restore acknowledgement locked when a receipt belongs to another restore', async () => {
    const epoch=crypto.randomUUID();current={...config(0,'recovery'),confirmed:false,can_pause:false,can_resume:false,recovery:{state:'pending',prepared:true,resources:[],epoch,job_id:crypto.randomUUID(),snapshot_id:crypto.randomUUID(),created_at:new Date().toISOString()}};
    const w=await view();await w.get('[data-testid=execution-recovery-open]').trigger('click');await w.get('[data-testid=execution-recovery-acknowledge]').trigger('click');await w.get('[data-testid=execution-reason]').setValue('核对恢复后只查询原始请求');for(const id of ['execution-confirm','execution-external','execution-tasks','execution-environment'])await w.get(`[data-testid=${id}] input`).setValue(true);mocks.change.mockRejectedValue({status:502});await submit(w);const op=mocks.change.mock.calls[0][0];mocks.request.mockResolvedValue({request_id:op.request_id,operation_revision:1,operation:{...op,expected_recovery_epoch:crypto.randomUUID()},configuration:config(1)});await w.get('[data-testid=execution-reconcile]').trigger('click');await flushPromises();expect(w.find('[data-testid=execution-unknown]').exists()).toBe(true);expect(mocks.change).toHaveBeenCalledTimes(1);
  });
});

describe('core-only release availability', () => {
 it('keeps handoff unavailable while preserving emergency pause and diagnosis', async()=>{
  current={...config(),legacy_handoff_available:false} as any;
  const w=await view();expect(w.get('[data-testid=execution-emergency]').attributes('disabled')).toBeUndefined();
  await w.get('[data-testid=execution-manage]').trigger('click');
  await w.get('[role=tab][aria-label="执行器交接"]').trigger('click');
  expect(w.find('[data-testid=execution-handoff-release-unavailable]').exists()).toBe(true);
  expect(w.get('[data-testid=execution-begin]').attributes('disabled')).toBeDefined();
  expect(mocks.change).not.toHaveBeenCalled();
 });
 it('rejects an ambiguous availability flag instead of treating it as permission',()=>{
  expect(()=>decodeExecutionConfiguration({...config(),legacy_handoff_available:'false'})).toThrow();
  expect(decodeExecutionConfiguration({...config(),legacy_handoff_available:false}).legacy_handoff_available).toBe(false);
 });
});
