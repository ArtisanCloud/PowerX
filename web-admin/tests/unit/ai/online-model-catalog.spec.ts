import { mount, flushPromises } from '@vue/test-utils';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import Modal from '~/components/settings/ai/OnlineModelCatalogModal.vue';
import zh from '../../../i18n/locales/zh.json';

const { sync } = vi.hoisted(() => ({ sync: vi.fn() }));
vi.mock('~/composables/api/services/aiSettingService', () => ({ AISettingService: { syncOnlineModels: sync } }));
const t = (key: string, params: Record<string, unknown> = {}) => Object.entries(params).reduce((text, [name, value]) => text.replaceAll(`{${name}}`, String(value)), key.split('.').reduce((o: any, part) => o?.[part], zh) || key);
const response = () => ({ provider: 'openai', env: 'dev', app: '', protocol: 'openai', source: 'https://provider.invalid/v1/models', updated_at: '2026-10-30T00:00:00Z', revision: 'revision-1', models: [
  { id: 'gpt-4o', label: 'GPT 4o', modalities: ['llm'], reported_modalities: ['llm'], manual_modalities: [], match: 'local' },
  { id: 'brand-new', label: 'Brand new', modalities: [], manual_modalities: [], match: 'unmatched' },
] });
function render(props: Record<string, unknown> = {}) {
  return mount(Modal, { props: { provider: 'openai', providerName: 'OpenAI', env: 'dev', app: '', modality: 'llm', connectionDirty: false, ...props }, global: { mocks: { $t: t }, stubs: {
    UModal: { template: '<div><slot name="body" /><slot name="footer" /></div>' },
    UButton: { props: ['disabled', 'loading'], template: '<button type="button" :disabled="disabled"><slot /></button>' },
    UInput: { props: ['modelValue'], emits: ['update:modelValue'], template: '<input :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />' },
    UCheckbox: { props: ['modelValue', 'disabled', 'label'], emits: ['update:modelValue'], template: '<label><input type="checkbox" :checked="modelValue" :disabled="disabled" @change="$emit(\'update:modelValue\', $event.target.checked)" />{{ label }}</label>' },
    UIcon: true,
  } } });
}
beforeEach(() => { sync.mockReset(); vi.stubGlobal('useI18n', () => ({ t })); });
afterEach(() => vi.unstubAllGlobals());

describe('在线目录获取和匹配保存', () => {
  it('完整展示在线型号，未知用途不需要再写入本地候选', async () => {
    sync.mockResolvedValueOnce(response());
    const wrapper = render(); await flushPromises();
    expect(sync).toHaveBeenCalledExactlyOnceWith({ provider: 'openai', env: 'dev', app: '', modality: 'llm', action: 'refresh' });
    expect(wrapper.get('[data-testid="online-model-summary"]').text()).toContain('在线获取 2 个型号');
    expect(wrapper.get('[data-testid="online-model-list"]').text()).toContain('brand-new');
    expect(wrapper.text()).toContain('https://provider.invalid/v1/models');
    await wrapper.get('#online-tab-undeclared').trigger('click');
    expect(wrapper.get('[data-testid="online-model-list"]').text()).toContain('brand-new');
    expect(wrapper.get('[data-testid="online-model-list"]').text()).not.toContain('gpt-4o');
    expect(wrapper.find('[data-testid="online-model-assign"]').exists()).toBe(false);
    expect(wrapper.emitted('saved')).toHaveLength(1); wrapper.unmount();
  });
  it('失败保留已有结果，显示结构化错误码和重试，不产生假保存事件', async () => {
    sync.mockResolvedValueOnce(response()); const wrapper = render(); await flushPromises();
    sync.mockRejectedValueOnce({ data: { error_code: 'AI_MODEL_SYNC_AUTH_FAILED', details: { trace_id: 'fixture-trace' } } });
    const refresh = wrapper.findAll('button').find(button => button.text() === '更新在线模型')!;
    await refresh.trigger('click'); await flushPromises();
    expect(wrapper.get('[role="alert"]').text()).toContain('供应商拒绝了目录查询');
    expect(wrapper.get('[role="alert"]').text()).toContain('fixture-trace');
    expect(wrapper.find('[data-testid="online-model-summary"]').exists()).toBe(true);
    expect(wrapper.emitted('saved')).toHaveLength(1);
    wrapper.unmount();
  });
  it('连接未保存时保留提示，媒体用途也完整显示在线型号', async () => {
    const dirty = render({ connectionDirty: true }); await flushPromises();
    expect(sync).not.toHaveBeenCalled(); expect(dirty.text()).toContain('请先关闭此窗口并保存连接配置'); dirty.unmount();
    sync.mockResolvedValueOnce(response()); const media = render({ modality: 'vlm' }); await flushPromises();
    expect(media.text()).toContain('brand-new'); expect(media.text()).toContain('按当前用途保存');
    expect(media.find('[data-testid="online-model-assign"]').exists()).toBe(false); media.unmount();
  });
  it('忙碌时阻止重复同步，关闭后旧结果不回写', async () => {
    let finish!: (result: unknown) => void; sync.mockImplementationOnce(() => new Promise(resolve => { finish = resolve; }));
    const wrapper = render(); await flushPromises();
    const refresh = wrapper.findAll('button').find(button => button.text() === '更新在线模型')!;
    expect(refresh.attributes('disabled')).toBeDefined();
    await refresh.trigger('click'); expect(sync).toHaveBeenCalledTimes(1);
    wrapper.unmount(); finish(response()); await flushPromises(); expect(wrapper.emitted('saved')).toBeUndefined();
  });
  it('已停用的旧版免费 Key 显示明确恢复步骤、实际目录地址和 403', async () => {
    sync.mockRejectedValueOnce({ data: { error_code: 'AI_MODEL_SYNC_AUTH_FAILED', request_id: 'request-403', details: { upstream_reason: 'legacy_free_key_retired', source: 'https://provider.invalid/v1/models', upstream_status: 403 } } });
    const wrapper = render(); await flushPromises();
    const alert = wrapper.get('[role="alert"]').text();
    expect(alert).toContain('供应商已停用旧版免费 API Key');
    expect(alert).toContain('绑定 GitHub');
    expect(alert).toContain('https://provider.invalid/v1/models');
    expect(alert).toContain('HTTP 403');
    expect(alert).toContain('request-403');
    expect(wrapper.emitted('saved')).toBeUndefined();
    wrapper.unmount();
  });
  it('供应商用途与本地分类分开，并保留搜索及键盘切换', async () => {
    const value: any = response();
    value.models.push({ id: 'fresh-vision', label: 'Fresh vision', modalities: [], reported_modalities: ['vlm'], input_modalities: ['image', 'text'], output_modalities: ['text'], match: 'provider' });
    sync.mockResolvedValueOnce(value); const wrapper = render({ modality: 'vlm' }); await flushPromises();
    expect(wrapper.get('[data-testid="online-model-summary"]').text()).toContain('供应商已声明用途 2 个，未声明 1 个');
    expect(wrapper.get('[data-testid="online-model-list"]').text()).toContain('输入：image, text');
    await wrapper.get('input').setValue('fresh');
    await wrapper.get('#online-tab-all').trigger('keydown', { key: 'End' });
    expect(wrapper.get('#online-tab-undeclared').attributes('aria-selected')).toBe('true');
    expect((wrapper.get('input').element as HTMLInputElement).value).toBe('fresh');
    await wrapper.get('#online-tab-undeclared').trigger('keydown', { key: 'Home' });
    expect(wrapper.get('[data-testid="online-model-list"]').text()).toContain('fresh-vision'); wrapper.unmount();
  });
  it('仅同步目录，不执行归类写入或推理，刷新失败保留原结果', async () => {
    sync.mockResolvedValueOnce(response()); const wrapper = render({ modality: 'vlm' }); await flushPromises();
    expect(wrapper.findAll('input[type="checkbox"]')).toHaveLength(0);
    sync.mockRejectedValueOnce({ data: { error_code: 'AI_MODEL_DISCOVERY_TIMEOUT' } });
    await wrapper.findAll('button').find(button => button.text() === '更新在线模型')!.trigger('click'); await flushPromises();
    expect(wrapper.get('[data-testid="online-model-list"]').text()).toContain('brand-new');
    expect(sync.mock.calls.every(([body]) => body.action === 'refresh')).toBe(true); wrapper.unmount();
  });
});
