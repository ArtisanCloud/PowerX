import { flushPromises, mount } from '@vue/test-utils';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue';
import InstallDialog from '~/components/plugins/InstallDialog.vue';

const calls = vi.hoisted(() => ({ create: vi.fn(), list: vi.fn(), get: vi.fn(), stream: vi.fn(), disconnect: vi.fn(), discard: vi.fn(), inspect: vi.fn(), upload: vi.fn(), enable: vi.fn(), push: vi.fn(), toast: vi.fn() }));
vi.mock('~/composables/api/services/adminPluginsService', () => ({ useAdminPluginsService: () => ({ inspectInstallation: calls.inspect, uploadInstallation: calls.upload, enable: calls.enable, createInstallation: calls.create, listInstallationTasks: calls.list, getInstallationTask: calls.get, installationEvents: calls.stream, discardInstallationSource: calls.discard }) }));
vi.mock('pinia', () => ({ storeToRefs: (store: any) => ({ isRoot: computed(() => store.isRoot), isCurrentTenantAdmin: computed(() => store.isCurrentTenantAdmin) }) }));

let receive: (event: any) => void = () => {};
vi.mock('~/utils/backup-sse', () => ({ connectBackupEvents: (_open: any, callback: any) => { receive = callback; return calls.disconnect; } }));
vi.mock('~/utils/auth-session', () => ({ readAuthSession: () => ({ sessionId: 'session-a' }), AUTH_SESSION_EVENT: 'session-changed' }));
const id = 'com.powerx.plugins.fixture';
let user: any;
const pending = () => ({ plugin_id: id, installation: { version: '1.0.0', revision: 'new', state: 'enabled', ready: false, tenant_ready: false, runtime_ready: true } });
const absent = () => ({ plugin_id: id, installation: { ready: false, reason: 'PLUGIN_NOT_INSTALLED' } });

function render(plugin?: any) {
  return mount(InstallDialog, { props: { modelValue: true, plugin }, global: { stubs: {
    UModal: { template: '<div><slot name="body" /><slot name="footer" /></div>' },
    UForm: { template: '<form><slot /></form>' },
    UAlert: { props: ['title', 'description'], template: '<div role="status">{{title}} {{description}}</div>' },
    UButton: { props: ['disabled', 'loading'], emits: ['click'], template: '<button :disabled="disabled || loading" @click="$emit(\'click\')"><slot /></button>' },
    USelect: { props: ['modelValue', 'items'], emits: ['update:modelValue'], template: '<select :value="modelValue" @change="$emit(\'update:modelValue\', $event.target.value)"><option v-for="item in items" :value="item.value">{{item.label}}</option></select>' },
    UInput: { template: '<input />' }, UTextarea: { props: ['modelValue'], emits: ['update:modelValue'], template: '<textarea :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />' }, USwitch: { template: '<span />' }, UCheckbox: { template: '<span />' }, UIcon: { template: '<i />' },
  } } });
}
function button(wrapper: any, key: string) { return wrapper.findAll('button').find((b: any) => b.text() === key); }
async function submit(wrapper: any, target = id) {
  await wrapper.find('select').setValue('local');
  const file = new File([`id: ${target}\nversion: 1.0.0\n`], 'plugin.yaml');
  Object.defineProperty(file, 'webkitRelativePath', { value: 'linux-amd64/plugin.yaml' });
  Object.defineProperty(file, 'text', { value: async () => `id: ${target}\nversion: 1.0.0\n` });
  const input = wrapper.find('input[type="file"]'); Object.defineProperty(input.element, 'files', { value: [file] });
  await input.trigger('change'); await button(wrapper, 'plugins.installDialog.actions.install').trigger('click'); await flushPromises(); await flushPromises();
}

beforeEach(() => {
  vi.clearAllMocks(); calls.inspect.mockReset(); calls.upload.mockReset(); calls.enable.mockReset(); calls.create.mockReset(); calls.list.mockReset(); calls.get.mockReset(); calls.disconnect.mockReset(); calls.list.mockResolvedValue([]);
  calls.create.mockImplementation(async (input) => task({ request_id: input.request_id, plugin_id: input.plugin_id, status: 'awaiting_upload', phase: 'upload' }));
  calls.upload.mockImplementation(async () => task()); calls.get.mockImplementation(async () => task());
  user = reactive({ isRoot: true, isCurrentTenantAdmin: true, user: { id: 1 }, currentTenantUuid: 'tenant-a', context: {} });
  for (const [name, value] of Object.entries({ ref, computed, reactive, watch, onBeforeUnmount, onMounted, useUserStore: () => user, useI18n: () => ({ t: (k: string) => k }), useRouter: () => ({ push: calls.push }), useToast: () => ({ add: calls.toast }), useState: (_k: string, init: () => unknown) => ref(init()) })) vi.stubGlobal(name, value);
});


const task = (change: Record<string, any> = {}) => ({ task_id: 'task-a', request_id: 'request-a', plugin_id: id, version: '1.0.0', status: 'queued', phase: 'queued', revision: 2, source_kind: 'local', created_at: '', updated_at: '', ...change });
describe('持久安装任务与推送恢复', () => {
  it('先持久创建任务再上传，事件更新完成，不周期读状态或调用旧同步安装', async () => {
    const wrapper=render(); await submit(wrapper);
    expect(calls.create).toHaveBeenCalledOnce(); expect(calls.upload).toHaveBeenCalledOnce();
    expect(calls.create.mock.invocationCallOrder[0]).toBeLessThan(calls.upload.mock.invocationCallOrder[0]);
    expect(wrapper.find('[data-testid="install-task"]').exists()).toBe(true);
    const count=calls.get.mock.calls.length; await new Promise(resolve=>setTimeout(resolve,50)); expect(calls.get).toHaveBeenCalledTimes(count);
    receive({name:'status',data:task({revision:3,status:'succeeded',phase:'completed'})});await nextTick();
    expect(wrapper.text()).toContain('plugins.installTask.status.succeeded');expect(calls.disconnect).toHaveBeenCalled();
    await button(wrapper,'plugins.installRecovery.done').trigger('click');expect(wrapper.emitted('installed')).toHaveLength(1);wrapper.unmount();
  });
  it('上传断开仍保留同一任务和原文件，重传仅使用该任务，不重复创建', async () => {
    calls.upload.mockRejectedValueOnce(new Error('connection interrupted'));
    const wrapper=render();await submit(wrapper);expect(wrapper.text()).toContain('connection interrupted');
    await button(wrapper,'plugins.installTask.upload').trigger('click'); await button(wrapper,'plugins.installDialog.actions.install').trigger('click');await flushPromises();
    expect(calls.create).toHaveBeenCalledOnce();expect(calls.upload).toHaveBeenCalledTimes(2);expect(calls.upload.mock.calls[1][0]).toBe('task-a');wrapper.unmount();
  });
  it('关闭释放连接，重新打开核对同一任务，不重复上传', async()=>{
    const wrapper=render();await submit(wrapper);await wrapper.setProps({modelValue:false});expect(calls.disconnect).toHaveBeenCalled();
    await wrapper.setProps({modelValue:true});await flushPromises();expect(calls.get).toHaveBeenCalledWith('task-a');expect(calls.upload).toHaveBeenCalledOnce();wrapper.unmount();
  });
  it('A 到 B 到 A 保留任务和表单，迟到 A 事件不改写 B',async()=>{
    const a={id,name:'A',version:'1.0.0'},b={id:'com.powerx.plugins.other',name:'B',version:'1.0.0'};
    const wrapper=render(a);await submit(wrapper);const oldReceive=receive;await wrapper.setProps({plugin:b});await flushPromises();
    oldReceive({name:'status',data:task({revision:3,status:'succeeded',phase:'completed'})});await nextTick();expect(wrapper.find('[data-testid="install-task"]').exists()).toBe(false);
    await wrapper.setProps({plugin:a});await flushPromises();expect(wrapper.text()).toContain(id);expect(calls.upload).toHaveBeenCalledOnce();wrapper.unmount();
  });
  it('新页面从正式列表恢复运行任务，不自动提交新的安装',async()=>{
    calls.list.mockResolvedValue([task()]);const wrapper=render();await wrapper.setProps({modelValue:false});await wrapper.setProps({modelValue:true});await flushPromises();
    expect(wrapper.find('[data-testid="install-task"]').exists()).toBe(true);expect(calls.create).not.toHaveBeenCalled();expect(calls.upload).not.toHaveBeenCalled();wrapper.unmount();
  });
  it('旧租户不能接纳迟到事件或手动恢复',async()=>{
    const wrapper=render();await submit(wrapper);user.currentTenantUuid='tenant-b';await nextTick();const reads=calls.get.mock.calls.length;
    receive({name:'status',data:task({status:'succeeded',revision:100})});await nextTick();expect(wrapper.text()).not.toContain('plugins.installTask.status.succeeded');expect(calls.get).toHaveBeenCalledTimes(reads);expect(calls.enable).not.toHaveBeenCalled();wrapper.unmount();
  });
  it('创建结果未知时锁定原意图，再次继续沿用同一请求编号',async()=>{
    calls.create.mockRejectedValueOnce(new Error('network interrupted'));
    const wrapper=render();await submit(wrapper);expect(wrapper.text()).toContain('plugins.installTask.creationUnknown');expect(wrapper.find('form').isVisible()).toBe(false);
    const original=calls.create.mock.calls[0][0];await button(wrapper,'plugins.installTask.continueRequest').trigger('click');await flushPromises();
    expect(calls.create).toHaveBeenCalledTimes(2);expect(calls.create.mock.calls[1][0]).toEqual(original);expect(calls.upload).toHaveBeenCalledOnce();wrapper.unmount();
  });
  it('异常提交回执只核对并显式启用，不重复上传或迁移',async()=>{
    calls.upload.mockResolvedValue(task({status:'needs_attention',phase:'activating',revision:3,installed_revision:'new'}));calls.inspect.mockResolvedValue(pending());calls.enable.mockResolvedValue({});
    const wrapper=render();await submit(wrapper);await button(wrapper,'plugins.installRecovery.check').trigger('click');await flushPromises();
    expect(wrapper.find('[data-testid="install-recovery"]').exists()).toBe(true);calls.inspect.mockResolvedValue({...pending(),installation:{...pending().installation,ready:true,tenant_ready:true}});
    await button(wrapper,'plugins.installRecovery.resume').trigger('click');await flushPromises();expect(calls.enable).toHaveBeenCalledWith(id,{expected_version:'1.0.0',expected_revision:'new'});expect(calls.upload).toHaveBeenCalledOnce();expect(wrapper.text()).toContain('plugins.installRecovery.readyTitle');wrapper.unmount();
  });

});
