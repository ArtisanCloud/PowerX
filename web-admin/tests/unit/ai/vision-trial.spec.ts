import { mount, flushPromises } from '@vue/test-utils';
import { beforeEach, afterEach, describe, it, expect, vi } from 'vitest';
import Modal from '~/components/settings/ai/VisionTrialModal.vue';
import zh from '../../../i18n/locales/zh.json';

const { trial } = vi.hoisted(() => ({ trial: vi.fn() }));
vi.mock('~/composables/api/services/aiSettingService', () => ({ AISettingService: { trialVision: trial } }));
const t = (key: string, params: Record<string, unknown> = {}) => Object.entries(params).reduce((text, [k, v]) => text.replaceAll(`{${k}}`, String(v)), key.split('.').reduce((o: any, part) => o?.[part], zh) || key);
const context = { env: 'dev', provider: 'openai', app: '', model: 'gpt-6-luna', params: { maxTokens: 2048, reasoningEffort: 'none', jsonMode: false } };
const analysis = { type: 'analysis', provider: 'openai', model: 'gpt-6-luna', text: 'A blue picture.', trace_id: 'trial-trace', elapsed_ms: 1200, env: 'dev' };
function render(props: Record<string, unknown> = {}) {
  return mount(Modal, { props: { open: true, context, scope: 'dev/tenant-a', ...props }, global: { mocks: { $t: t }, stubs: {
    UModal: { props: ['open'], template: '<div v-if="open"><slot name="body" /><slot name="footer" /></div>' },
    UForm: { template: '<form @submit.prevent><slot /></form>' }, UFormField: { template: '<label><slot /></label>' },
    UButton: { props: ['disabled', 'loading'], template: '<button type="button" :disabled="disabled"><slot /></button>' },
    UInput: { props: ['modelValue'], emits: ['update:modelValue'], template: '<input :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />' },
    UTextarea: { props: ['modelValue'], emits: ['update:modelValue'], template: '<textarea :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />' },
    URadioGroup: { props: ['modelValue', 'items'], emits: ['update:modelValue'], template: '<div><button v-for="item in items" type="button" :data-mode="item.value" @click="$emit(\'update:modelValue\', item.value)">{{ item.label }}</button></div>' },
  } } });
}
async function useURL(wrapper: ReturnType<typeof render>) {
  await wrapper.get('[data-mode="url"]').trigger('click'); await wrapper.get('[data-testid="vision-trial-url"]').setValue('https://example.invalid/picture.png');
}
beforeEach(() => { trial.mockReset(); vi.stubGlobal('useI18n', () => ({ t })); });
afterEach(() => vi.unstubAllGlobals());

describe('显式图片识别试跑', () => {
  it('打开和预览不调用模型，只有明确执行才发送图片与要求并展示分析和 trace', async () => {
    trial.mockResolvedValueOnce(analysis); const wrapper = render();
    expect(trial).not.toHaveBeenCalled(); expect(wrapper.get('[data-testid="vision-trial-run"]').attributes('disabled')).toBeDefined();
    await useURL(wrapper); await wrapper.findAll('button').find(b => b.text() === '预览图片')!.trigger('click');
    expect(wrapper.get('img').attributes('src')).toBe('https://example.invalid/picture.png'); expect(trial).not.toHaveBeenCalled();
    await wrapper.get('textarea').setValue('Describe this picture'); await wrapper.get('[data-testid="vision-trial-run"]').trigger('click'); await flushPromises();
    expect(trial).toHaveBeenCalledExactlyOnceWith(context, { type: 'image_url', url: 'https://example.invalid/picture.png' }, 'Describe this picture', expect.any(AbortSignal));
    expect(wrapper.get('[data-testid="vision-trial-analysis"]').text()).toBe('A blue picture.'); expect(wrapper.text()).toContain('trial-trace'); wrapper.unmount();
  });
  it('缺少型号或无效 URL 时禁止执行，不自动保存或退回其他型号', async () => {
    const wrapper = render({ blockedReason: '请选择型号' }); await useURL(wrapper);
    expect(wrapper.get('[role="alert"]').text()).toContain('请选择型号'); expect(wrapper.get('[data-testid="vision-trial-run"]').attributes('disabled')).toBeDefined();
    await wrapper.setProps({ blockedReason: '' }); await wrapper.get('[data-testid="vision-trial-url"]').setValue('file:///etc/hosts');
    expect(wrapper.get('[data-testid="vision-trial-run"]').attributes('disabled')).toBeDefined(); expect(trial).not.toHaveBeenCalled(); wrapper.unmount();
  });
  it('上游失败保留输入并提供结构化技术详情，支持手动重试', async () => {
    trial.mockRejectedValueOnce({ data: { error_code: 'AI_PROVIDER_REJECTED', request_id: 'request-trace', details: { upstream_status: 429, provider_trace_id: 'provider-trace' } } });
    const wrapper = render(); await useURL(wrapper); await wrapper.get('[data-testid="vision-trial-run"]').trigger('click'); await flushPromises();
    expect(wrapper.get('[role="alert"]').text()).toContain('供应商拒绝'); expect(wrapper.text()).toContain('429'); expect(wrapper.text()).toContain('provider-trace');
    expect((wrapper.get('[data-testid="vision-trial-url"]').element as HTMLInputElement).value).toBe('https://example.invalid/picture.png');
    trial.mockResolvedValueOnce(analysis); await wrapper.get('[data-testid="vision-trial-run"]').trigger('click'); await flushPromises();
    expect(trial).toHaveBeenCalledTimes(2); expect(wrapper.find('[role="alert"]').exists()).toBe(false); wrapper.unmount();
  });
  it('分区切换保留输入，忙碌时禁止重复请求；关闭后旧结果不得回写', async () => {
    let finish!: (value: unknown) => void; trial.mockImplementationOnce(() => new Promise(resolve => { finish = resolve; }));
    const wrapper = render(); await useURL(wrapper);
    await wrapper.get('#vision-trial-tab-result').trigger('click'); await wrapper.get('#vision-trial-tab-input').trigger('click');
    expect((wrapper.get('[data-testid="vision-trial-url"]').element as HTMLInputElement).value).toContain('picture.png');
    await wrapper.get('[data-testid="vision-trial-run"]').trigger('click'); const signal = trial.mock.calls[0][3];
    expect(wrapper.get('[data-testid="vision-trial-run"]').attributes('disabled')).toBeDefined();
    await wrapper.findAll('button').find(b => b.text() === '完成')!.trigger('click'); expect(signal.aborted).toBe(true);
    finish(analysis); await flushPromises(); expect(wrapper.find('[data-testid="vision-trial-analysis"]').exists()).toBe(false); wrapper.unmount();
  });
  it('环境／租户切换清空图片草稿，不跨身份继承识别内容', async () => {
    const wrapper = render(); await useURL(wrapper); await wrapper.setProps({ scope: 'prod/tenant-b' });
    expect((wrapper.get('[data-testid="vision-trial-url"]').element as HTMLInputElement).value).toBe(''); expect(wrapper.emitted('update:open')?.at(-1)).toEqual([false]); wrapper.unmount();
  });
  it('上传文件在客户端编码后作为图片输入，文件类型错误不提交模型', async () => {
    const wrapper = render(); const input = wrapper.get('input[type="file"]');
    Object.defineProperty(input.element, 'files', { configurable: true, value: [new File(['text'], 'bad.txt', { type: 'text/plain' })] });
    await input.trigger('change'); expect(wrapper.get('[role="alert"]').text()).toContain('请选择 PNG'); expect(trial).not.toHaveBeenCalled();
    Object.defineProperty(input.element, 'files', { configurable: true, value: [new File(['fixture-image'], 'blue.png', { type: 'image/png' })] });
    await input.trigger('change'); await vi.waitFor(() => expect(wrapper.find('img').exists()).toBe(true));
    trial.mockResolvedValueOnce(analysis); await wrapper.get('[data-testid="vision-trial-run"]').trigger('click'); await flushPromises();
    expect(trial.mock.calls[0][1]).toMatchObject({ type: 'image_base64', content: expect.stringContaining('data:image/png;base64,') }); wrapper.unmount();
  });
});
