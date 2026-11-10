import { mount, flushPromises } from '@vue/test-utils';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { createI18n } from 'vue-i18n';
import zh from '../../../i18n/locales/zh.json';
import { emitBackupEvent } from '../../support/backup-events-stub';
vi.mock('~/composables/useBackupEvents', () => import('../../support/backup-events-stub'));
const api = vi.hoisted(() => ({ overview: vi.fn(), catalog: vi.fn(), savePolicy: vi.fn(), saveTarget: vi.fn(), testTarget: vi.fn(), job: vi.fn(), preview: vi.fn(), protect: vi.fn(), download: vi.fn(), run: vi.fn(), runRequest: vi.fn(), migrate: vi.fn() }));
vi.mock('~/composables/api/services/archiveOpsService', () => ({ useArchiveOpsService: () => api }));
const auth = vi.hoisted(() => ({ sessionId: 'archive-session', tenantUuid: 'tenant-owned', status: 'active' }));
vi.mock('~/utils/auth-session', () => ({ AUTH_SESSION_EVENT: 'powerx:auth-session-changed', readAuthSession: () => auth }));
import Panel from '~/components/ops/backup/ArchiveGovernanceBody.vue';
const stubs = { UIcon: true,
 UButton: { props: ['disabled', 'loading'], template: '<button :disabled="disabled || loading"><slot/></button>' }, UBadge: { template: '<span><slot/></span>' },
 UAlert: { props: ['title', 'description'], template: '<div>{{title}} {{description}}<slot name="actions"/></div>' },
 UModal: { props: ['open', 'title'], template: '<section v-if="open"><h3>{{title}}</h3><slot name="body"/><slot name="footer"/></section>' },
 UInput: { props: ['modelValue'], emits: ['update:modelValue'], template: '<input :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)"/>' },
 USelect: { props: ['modelValue', 'items'], emits: ['update:modelValue'], template: '<select :value="modelValue" @change="$emit(\'update:modelValue\', $event.target.value)"><option v-for="item in items" :value="item.value">{{item.label}}</option></select>' },
 UFormField: { props: ['label'], template: '<label>{{label}}<slot/></label>' }, UCheckbox: true,
 UForm: { emits: ['submit'], template: '<form @submit.prevent="$emit(\'submit\')"><slot/></form>' },
};
let w: ReturnType<typeof mount>;
let policy: any, policyRevision:number, runProtocol:number;
const boot = () => mount(Panel, { global: { plugins: [createI18n({ legacy: false, locale: 'zh', messages: { zh } })], stubs } });
const runReceipt = (requestId: string) => ({ request_id: requestId, policy_id: policy.id, job_id: '22222222-2222-4222-8222-222222222222', policy_revision: 2, source_id: 'logs', status: 'running', phase: 'capturing', remote_state: 'pending', created_at: new Date().toISOString(), updated_at: new Date().toISOString() });
const button = (label: string) => w.findAll('button').find(b => b.text() === label)!;
const tab = async (value: string) => { await w.get(`[data-section-select]`).setValue(value); await flushPromises(); };
beforeEach(() => {
 vi.resetAllMocks(); sessionStorage.clear(); auth.sessionId='archive-session'; auth.status='active'; runProtocol=1; policyRevision=1; policy = { id: '11111111-1111-4111-8111-111111111111', name: '日志策略', revision: 2, source_id: 'logs', target_id: 'target', interval_minutes: 5, enabled: false, remote_cleanup_enabled: false, remote_max_bytes: 1024 ** 3, remote_retention_days: 180 };
 api.overview.mockImplementation(async () => ({ run_request_version: runProtocol, execution_version: 1, target_configuration: {authority:'database',ready:true,state:'applied',desired_revision:1,applied_revision:1,snapshot_revision:1,targets:[]}, policy_configuration:{authority:'database',ready:true,state:'applied',desired_revision:policyRevision,applied_revision:policyRevision,snapshot_revision:policyRevision,policies:[]},instance: 'test', sources: [{ id: 'logs', name: '日志', kind: 'application_logs' }], policies: [policy], targets: [{ id: 'target', name: '独立目标', driver: 's3', endpoint: 'https://archive.invalid', bucket: 'private', prefix: 'host/test', verification_valid: false }], credentials: [], jobs: [], restores: [], pending_transfers: 0 }));
 api.catalog.mockResolvedValue({ items: [{ id: 'historical', policy_id: 'p', kind: 'application_logs', source_id: 'gone', source_file_name: 'server.log.2.gz', status: 'pruned', remote_state: 'success', protected: true, size_bytes: 10, file_count: 1, created_at: new Date().toISOString() }], pagination: { page: 1, pages: 1, total: 1 } });
 api.savePolicy.mockImplementation(async p => { policy = p.action==='save'?{...policy,...p.settings,enabled:false,remote_cleanup_enabled:false,revision:p.expected_policy_revision+1}:{...policy,enabled:p.enabled,remote_cleanup_enabled:p.remote_cleanup_enabled,revision:p.expected_policy_revision+1,execution_ready:!!p.enabled||!!p.remote_cleanup_enabled};policyRevision++;return {request_id:p.request_id,operation_revision:policyRevision,policy_id:policy.id,configuration:{authority:'database',ready:true,state:'applied',desired_revision:policyRevision,applied_revision:policyRevision,snapshot_revision:policyRevision,policies:[policy]}}; });
 api.run.mockImplementation(async (_policy, key) => { const {job_id, ...out} = runReceipt(key); return {...out,id:job_id}; });
 w = boot();
});
afterEach(() => { w.unmount(); vi.useRealTimers(); });
it('never starts transmission on load, save, or failed target verification; enabling requires its own confirmation', async () => {
 await flushPromises(); expect(api.savePolicy).not.toHaveBeenCalled();
 await tab('policies'); await button('配置策略').trigger('click'); await flushPromises();
 await w.get('#log-archive-policy').trigger('submit'); await flushPromises();
 expect(api.savePolicy.mock.calls[0]![0]).toMatchObject({action:'save',settings:expect.objectContaining({name:'日志策略'})});
 await button('开启归档').trigger('click'); expect(api.savePolicy).toHaveBeenCalledTimes(1);
 await button('确认执行').trigger('click'); await flushPromises(); expect(api.savePolicy.mock.calls[1]![0].enabled).toBe(true);
 await tab('targets'); api.testTarget.mockRejectedValueOnce(new Error('target inaccessible'));
 await button('测试写入与取回').trigger('click'); await flushPromises(); expect(w.text()).toContain('target inaccessible'); expect(api.savePolicy).toHaveBeenCalledTimes(2);
});
it('keeps filters and forms through tab and push changes, retrieves source-gone history, and makes no periodic reads', async () => {
 await flushPromises(); vi.useFakeTimers(); await w.get('[data-testid="archive-search"]').setValue('server.log');
 await tab('targets'); await tab('catalog'); expect((w.get('[data-testid="archive-search"]').element as HTMLInputElement).value).toBe('server.log');
 expect(w.get('[data-testid="archive-catalog-item"]').text()).toContain('server.log.2.gz');
 await button('下载并校验').trigger('click'); await flushPromises(); expect(api.download.mock.calls[0]![0]).toBe('historical');
 const count = api.catalog.mock.calls.length; await vi.advanceTimersByTimeAsync(3600000); expect(api.catalog).toHaveBeenCalledTimes(count);
 emitBackupEvent('archives'); await flushPromises(); expect(api.catalog).toHaveBeenCalledTimes(count + 1);
 const signal = api.catalog.mock.calls.at(-1)![1]; w.unmount(); expect(signal.aborted).toBe(true);
});
it('starts a manual archive and migration from their buttons with separate UUID request keys', async () => {
 policy.enabled = true;policy.execution_ready=true; await flushPromises(); await tab('policies');
 await button('归档一批').trigger('click'); await flushPromises();
 expect(api.run).toHaveBeenCalledWith(policy.id, expect.stringMatching(/^[0-9a-f-]{36}$/), expect.any(AbortSignal), 2);
 await tab('catalog'); await button('迁移副本').trigger('click'); await flushPromises();
 const modal = w.findAll('section').filter(s => s.find('h3').exists() && s.find('h3').text() === '迁移副本').at(-1)!;
 await modal.get('select').setValue(policy.id);
 await button('确认执行').trigger('click'); await flushPromises();
 expect(api.migrate).toHaveBeenCalledWith('historical', policy.id, expect.stringMatching(/^[0-9a-f-]{36}$/), expect.any(AbortSignal));
 expect(api.migrate.mock.calls[0]![2]).not.toBe(api.run.mock.calls[0]![1]);
});
it('separates local capture from remote confirmation and retains the original job identity', async () => {
 const row = { id: 'original-job', policy_id: 'p', kind: 'application_logs', source_id: 'logs', status: 'success', remote_state: 'failed', remote_error: '远端写入未确认', file_count: 1, size_bytes: 10, created_at: new Date().toISOString() };
 api.catalog.mockResolvedValue({ items: [row], pagination: { page: 1, pages: 1, total: 1 } });
 await flushPromises(); emitBackupEvent('archives'); await flushPromises();
 expect(w.get('[data-archive-job-id="original-job"] [data-testid="archive-local-state"]').text()).toBe('本地采集已完成');
 expect(w.get('[data-testid="archive-remote-state"]').text()).toBe('远端传输未完成');
 expect(w.get('[data-testid="archive-job-id"]').text()).toContain('original-job');
 expect(w.get('[data-testid="archive-transfer-warning"]').text()).toContain('远端可能已收到部分内容');
 expect(button('下载并校验').attributes('disabled')).toBeUndefined();
 expect(w.text()).toContain('远端写入未确认');
 row.remote_state = 'future-unknown-state'; emitBackupEvent('archives'); await flushPromises();
 expect(w.get('[data-testid="archive-remote-state"]').text()).toBe('远端状态未知');
 expect(api.run).not.toHaveBeenCalled(); expect(api.migrate).not.toHaveBeenCalled();
});
it('keeps the failed original job and search draft during metadata failure; push recovery does not replay a mutation', async () => {
 const row = { id: 'original-job', policy_id: 'p', kind: 'application_logs', source_id: 'logs', status: 'success', remote_state: 'failed', remote_error: '远端写入未确认', file_count: 1, size_bytes: 10, created_at: new Date().toISOString() };
 api.catalog.mockResolvedValue({ items: [row], pagination: { page: 1, pages: 1, total: 1 } });
 await flushPromises(); emitBackupEvent('archives'); await flushPromises();
 await w.get('[data-testid="archive-search"]').setValue('canary');
 api.catalog.mockRejectedValueOnce(new Error('metadata unavailable')); emitBackupEvent('archives'); await flushPromises();
 expect(w.text()).toContain('metadata unavailable'); expect(w.text()).toContain('original-job'); expect(w.text()).toContain('远端写入未确认');
 row.remote_state = 'success'; row.remote_error = ''; emitBackupEvent('archives'); await flushPromises();
 expect(w.get('[data-testid="archive-remote-state"]').text()).toBe('远端已校验'); expect(w.find('[data-testid="archive-transfer-warning"]').exists()).toBe(false);
 await tab('targets'); await tab('catalog'); expect((w.get('[data-testid="archive-search"]').element as HTMLInputElement).value).toBe('canary');
 vi.useFakeTimers(); const count = api.catalog.mock.calls.length; await vi.advanceTimersByTimeAsync(3600000); expect(api.catalog).toHaveBeenCalledTimes(count);
 expect(api.run).not.toHaveBeenCalled(); expect(api.migrate).not.toHaveBeenCalled();
});
it('retains the UUID after lost acceptance, blocks repeat clicks and 404, and reconciles only the original request', async () => {
 policy.enabled = true; policy.execution_ready = true; api.run.mockRejectedValueOnce(new Error('accepted response lost'));
 await flushPromises(); await tab('policies'); await w.get('[data-testid="archive-run-start"]').trigger('click'); await flushPromises();
 const id = api.run.mock.calls[0]![1]; expect(w.get('[data-testid="archive-run-request-id"]').text()).toBe(id);
 expect(w.get('[data-testid="archive-run-start"]').attributes('disabled')).toBeDefined();
 await w.get('[data-testid="archive-run-start"]').trigger('click');
 await tab('targets'); await tab('catalog'); emitBackupEvent('archives'); await flushPromises(); expect(api.run).toHaveBeenCalledTimes(1); expect(api.runRequest).not.toHaveBeenCalled();
 api.runRequest.mockRejectedValueOnce(Object.assign(new Error('not found'), {status:404})); await w.get('[data-testid="archive-run-reconcile"]').trigger('click'); await flushPromises();
 expect(w.get('[data-testid="archive-run-request-id"]').text()).toBe(id);
 api.runRequest.mockResolvedValueOnce({...runReceipt(id),source_id:'different'}); await w.get('[data-testid="archive-run-reconcile"]').trigger('click'); await flushPromises(); expect(w.find('[data-testid="archive-run-pending"]').exists()).toBe(true);
 api.runRequest.mockResolvedValueOnce(runReceipt(id)); await w.get('[data-testid="archive-run-reconcile"]').trigger('click'); await flushPromises();
 expect(api.runRequest).toHaveBeenLastCalledWith(policy.id,id,expect.any(AbortSignal)); expect(w.find('[data-testid="archive-run-pending"]').exists()).toBe(false);
 expect(w.get('[data-testid="archive-run-recorded"]').text()).toContain(runReceipt(id).job_id); expect(api.run).toHaveBeenCalledTimes(1);
 await w.get('[data-testid="archive-run-view-job"]').trigger('click'); await flushPromises(); expect((w.get('[data-testid="archive-search"]').element as HTMLInputElement).value).toBe(runReceipt(id).job_id);
});
it('restores pending request after remount without replay or timed request reads', async () => {
 policy.enabled=true; policy.execution_ready=true; api.run.mockRejectedValueOnce(new Error('lost reply')); await flushPromises(); await tab('policies'); await w.get('[data-testid="archive-run-start"]').trigger('click'); await flushPromises();
 const id=api.run.mock.calls[0]![1]; w.unmount(); w=boot(); await flushPromises(); expect(w.get('[data-testid="archive-run-request-id"]').text()).toBe(id); await tab('policies'); expect(w.get('[data-testid="archive-run-start"]').attributes('disabled')).toBeDefined();
 vi.useFakeTimers(); await vi.advanceTimersByTimeAsync(3600000); expect(api.run).toHaveBeenCalledTimes(1); expect(api.runRequest).not.toHaveBeenCalled();
});
it('isolates login owners and ignores a late response belonging to the previous login', async () => {
 let finish!: (value: any) => void; api.run.mockImplementationOnce(() => new Promise(resolve => { finish=resolve; })); policy.enabled=true; policy.execution_ready=true;
 await flushPromises(); await tab('policies'); await w.get('[data-testid="archive-run-start"]').trigger('click'); await flushPromises(); const id=api.run.mock.calls[0]![1];
 auth.sessionId='second-login'; window.dispatchEvent(new Event('powerx:auth-session-changed')); await flushPromises(); expect(w.find('[data-testid="archive-run-pending"]').exists()).toBe(false);
 const {job_id,...receipt}=runReceipt(id); finish({...receipt,id:job_id}); await flushPromises(); expect(w.find('[data-testid="archive-run-recorded"]').exists()).toBe(false);
 auth.sessionId='archive-session'; window.dispatchEvent(new Event('powerx:auth-session-changed')); await flushPromises(); expect(w.get('[data-testid="archive-run-request-id"]').text()).toBe(id); expect(api.run).toHaveBeenCalledTimes(1);
});

it('blocks new submission on older Core or failed context persistence', async () => {
 policy.enabled=true; policy.execution_ready=true; runProtocol=0; await flushPromises(); emitBackupEvent('archives'); await flushPromises(); await tab('policies');
 expect(w.find('[data-testid="archive-run-upgrade"]').exists()).toBe(true); expect(w.get('[data-testid="archive-run-start"]').attributes('disabled')).toBeDefined();
 await w.get('[data-testid="archive-run-start"]').trigger('click'); expect(api.run).not.toHaveBeenCalled();
 runProtocol=1; emitBackupEvent('archives'); await flushPromises(); const fail=vi.spyOn(Storage.prototype,'setItem').mockImplementationOnce(()=>{throw new Error('quota exhausted')});
 await w.get('[data-testid="archive-run-start"]').trigger('click'); await flushPromises(); expect(api.run).not.toHaveBeenCalled(); fail.mockRestore();
});
it('does not present old running as active and preserves interruption across tabs', async () => {
 const row:any={id:'original-interrupted',policy_id:policy.id,source_id:'logs',kind:'application_logs',status:'running',phase:'capturing',remote_state:'pending',file_count:20,size_bytes:0,created_at:new Date().toISOString()};
 api.catalog.mockResolvedValue({items:[row],pagination:{page:1,pages:1,total:1}});await flushPromises();emitBackupEvent('archives');await flushPromises();
 expect(w.get('[data-testid="archive-execution-state"]').text()).toBe('未确认有执行器正在处理');expect(w.get('[data-testid="archive-local-state"]').text()).not.toContain('采集中');
 row.status='interrupted';row.execution_state='interrupted';row.interruption_pending=true;policy.enabled=true;policy.execution_ready=true;emitBackupEvent('archives');await flushPromises();
 expect(w.get('[data-testid="archive-local-state"]').text()).toBe('本地采集已中断，内容保留');expect(button('下载并校验').attributes('disabled')).toBeDefined();expect(w.find('[data-testid="archive-retry-original"]').exists()).toBe(false);
 await tab('policies');expect(w.find('[data-testid="archive-execution-attention"]').exists()).toBe(true);expect(w.get('[data-testid="archive-run-start"]').attributes('disabled')).toBeDefined();
 await tab('targets');expect(w.find('[data-testid="archive-execution-attention"]').exists()).toBe(true);expect(w.emitted('attention')!.at(-1)).toEqual([true]);expect(api.run).not.toHaveBeenCalled();
});
it('confirms retransfer of the original complete interrupted job without new capture', async () => {
 const row:any={id:'original-complete',policy_id:policy.id,source_id:'logs',kind:'application_logs',status:'success',phase:'interrupted',execution_state:'interrupted',interruption_pending:true,remote_state:'unconfirmed',target:{id:'target'},file_count:1,size_bytes:10,created_at:new Date().toISOString()};
 api.retry=vi.fn();api.catalog.mockResolvedValue({items:[row],pagination:{page:1,pages:1,total:1}});await flushPromises();emitBackupEvent('archives');await flushPromises();
 await w.get('[data-testid="archive-retry-original"]').trigger('click');await flushPromises();expect(api.retry).not.toHaveBeenCalled();expect(w.text()).toContain(row.id);
 await w.get('[data-testid="archive-retry-confirm"]').trigger('click');await flushPromises();expect(api.retry).toHaveBeenCalledExactlyOnceWith(row.id,expect.any(AbortSignal));expect(api.run).not.toHaveBeenCalled();
});

it('shows unconfirmed external command protection across task tabs without starting work',async()=>{
 const current=api.overview.getMockImplementation()!;
 api.overview.mockImplementation(async()=>({...await current(),command_execution:{protocol:1,active:0,unconfirmed:1,jobs:[]}}));
 emitBackupEvent('archives');await flushPromises();
 expect(w.get('[data-testid="archive-command-unconfirmed"]').text()).toContain('本机外部命令尚未确认退出');
 await tab('policies');expect(w.get('[data-testid="archive-command-unconfirmed"]').exists()).toBe(true);
 expect(api.run).not.toHaveBeenCalled();expect(api.retry).not.toHaveBeenCalled();
});
