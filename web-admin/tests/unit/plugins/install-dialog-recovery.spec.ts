import { flushPromises, mount } from '@vue/test-utils';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { computed, nextTick, onBeforeUnmount, reactive, ref, watch } from 'vue';
import InstallDialog from '~/components/plugins/InstallDialog.vue';

const calls = vi.hoisted(() => ({ inspect: vi.fn(), upload: vi.fn(), enable: vi.fn(), push: vi.fn(), toast: vi.fn() }));
vi.mock('~/composables/api/services/adminPluginsService', () => ({ useAdminPluginsService: () => ({ inspectInstallation: calls.inspect, installFromLocal: calls.upload, enable: calls.enable }) }));
vi.mock('pinia', () => ({ storeToRefs: (store: any) => ({ isRoot: computed(() => store.isRoot), isCurrentTenantAdmin: computed(() => store.isCurrentTenantAdmin) }) }));

const id = 'com.powerx.plugins.fixture';
let user: any;
const pending = () => ({ plugin_id: id, installation: { version: '1.0.0', revision: 'new', state: 'enabled', ready: false, tenant_ready: false, runtime_ready: true } });
const absent = () => ({ plugin_id: id, installation: { ready: false, reason: 'PLUGIN_NOT_INSTALLED' } });

function render() {
  return mount(InstallDialog, { props: { modelValue: true }, global: { stubs: {
    UModal: { template: '<div><slot name="body" /><slot name="footer" /></div>' },
    UForm: { template: '<form><slot /></form>' },
    UAlert: { props: ['title', 'description'], template: '<div role="status">{{title}} {{description}}</div>' },
    UButton: { props: ['disabled', 'loading'], emits: ['click'], template: '<button :disabled="disabled || loading" @click="$emit(\'click\')"><slot /></button>' },
    USelect: { props: ['modelValue', 'items'], emits: ['update:modelValue'], template: '<select :value="modelValue" @change="$emit(\'update:modelValue\', $event.target.value)"><option v-for="item in items" :value="item.value">{{item.label}}</option></select>' },
    UInput: { template: '<input />' }, UTextarea: { template: '<textarea />' }, USwitch: { template: '<span />' }, UCheckbox: { template: '<span />' }, UIcon: { template: '<i />' },
  } } });
}
function button(wrapper: any, key: string) { return wrapper.findAll('button').find((b: any) => b.text() === key); }
async function submit(wrapper: any) {
  await wrapper.find('select').setValue('local');
  const file = new File([`id: ${id}\nversion: 1.0.0\n`], 'plugin.yaml');
  Object.defineProperty(file, 'webkitRelativePath', { value: 'linux-amd64/plugin.yaml' });
  Object.defineProperty(file, 'text', { value: async () => `id: ${id}\nversion: 1.0.0\n` });
  const input = wrapper.find('input[type="file"]'); Object.defineProperty(input.element, 'files', { value: [file] });
  await input.trigger('change'); await button(wrapper, 'plugins.installDialog.actions.install').trigger('click'); await flushPromises(); await flushPromises();
}

beforeEach(() => {
  vi.clearAllMocks(); calls.inspect.mockReset(); calls.upload.mockReset(); calls.enable.mockReset();
  user = reactive({ isRoot: true, isCurrentTenantAdmin: true, user: { id: 1 }, currentTenantUuid: 'tenant-a', context: {} });
  for (const [name, value] of Object.entries({ ref, computed, reactive, watch, onBeforeUnmount, useUserStore: () => user, useI18n: () => ({ t: (k: string) => k }), useRouter: () => ({ push: calls.push }), useToast: () => ({ add: calls.toast }), useState: (_k: string, init: () => unknown) => ref(init()) })) vi.stubGlobal(name, value);
});

describe('安装弹窗超时恢复', () => {
  it('超时保留文件和表单，阻止再次上传；显式恢复只调用启用接口', async () => {
    calls.inspect.mockResolvedValueOnce(absent()).mockResolvedValueOnce(pending()).mockResolvedValueOnce({ ...pending(), installation: { ...pending().installation, ready: true, tenant_ready: true } });
    calls.upload.mockRejectedValue(new Error('请求超时，请稍后重试')); calls.enable.mockResolvedValue({ ok: true });
    const wrapper = render(); await submit(wrapper);
    expect(calls.upload).toHaveBeenCalledTimes(1); expect(calls.enable).not.toHaveBeenCalled();
    expect(button(wrapper, 'plugins.installDialog.actions.install')).toBeUndefined();
    expect(wrapper.find('form').isVisible()).toBe(false);
    expect(wrapper.find('input[type="file"]').element.files?.length).toBe(1);
    await button(wrapper, 'plugins.installRecovery.resume').trigger('click'); await flushPromises(); await flushPromises();
    expect(calls.enable).toHaveBeenCalledWith(id, { expected_version: '1.0.0', expected_revision: 'new' });
    expect(calls.upload).toHaveBeenCalledTimes(1);
    await button(wrapper, 'plugins.installRecovery.done').trigger('click'); expect(wrapper.emitted('installed')).toHaveLength(1); wrapper.unmount();
  });
  it('超时后暂未查到安装时只允许继续核对，不自动重试', async () => {
    calls.inspect.mockResolvedValue(absent()); calls.upload.mockRejectedValue(new Error('请求超时'));
    const wrapper = render(); await submit(wrapper);
    expect(wrapper.text()).toContain('plugins.installRecovery.not_installedTitle');
    expect(button(wrapper, 'plugins.installRecovery.resume')).toBeUndefined();
    expect(button(wrapper, 'plugins.installDialog.actions.install')).toBeUndefined();
    expect(calls.upload).toHaveBeenCalledTimes(1); expect(wrapper.emitted('installed')).toBeUndefined(); wrapper.unmount();
  });
  it('登录租户变化后禁止恢复旧租户的操作', async () => {
    calls.inspect.mockResolvedValueOnce(absent()).mockResolvedValueOnce(pending()); calls.upload.mockRejectedValue(new Error('请求超时'));
    const wrapper = render(); await submit(wrapper); user.currentTenantUuid = 'tenant-b'; await nextTick();
    expect(wrapper.text()).toContain('plugins.installRecovery.sessionChanged'); expect(button(wrapper, 'plugins.installRecovery.resume')).toBeUndefined();
    await button(wrapper, 'plugins.installRecovery.check').trigger('click'); await flushPromises(); expect(calls.inspect).toHaveBeenCalledTimes(2); expect(calls.enable).not.toHaveBeenCalled(); wrapper.unmount();
  });
  it('服务端明确待完成时使用提交回执；恢复版本冲突时保留现场', async () => {
    calls.inspect.mockResolvedValueOnce({ ...pending(), installation: { ...pending().installation, revision: 'old' } }).mockResolvedValueOnce(pending());
    calls.upload.mockRejectedValue(Object.assign(new Error('PLUGIN_INSTALL_FINALIZATION_REQUIRED'), { status: 500, data: { details: { code: 'PLUGIN_INSTALL_FINALIZATION_REQUIRED', plugin_id: id, installed: { version: '1.0.0', revision: 'new' } } } }));
    calls.enable.mockRejectedValue(Object.assign(new Error('安装已变化，请重新核对'), { status: 409 }));
    const wrapper = render(); await submit(wrapper);
    await button(wrapper, 'plugins.installRecovery.resume').trigger('click'); await flushPromises();
    expect(wrapper.text()).toContain('安装已变化，请重新核对'); expect(calls.upload).toHaveBeenCalledTimes(1); expect(wrapper.emitted('installed')).toBeUndefined();
    expect(wrapper.find('input[type="file"]').element.files?.length).toBe(1); wrapper.unmount();
  });
});
