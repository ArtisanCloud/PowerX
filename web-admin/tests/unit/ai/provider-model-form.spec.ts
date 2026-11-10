import { mount } from "@vue/test-utils";
import { computed, reactive, watch } from "vue";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import ProviderModelForm from "~/components/settings/ai/ProviderModelForm.vue";
import zh from "../../../i18n/locales/zh.json";

const t = (key: string, params: Record<string, string> = {}) => {
  const value = key.split(".").reduce((obj: any, part) => obj?.[part], zh) ?? key;
  return String(value).replace(/\{(\w+)\}/g, (match, name) => params[name] ?? match);
};
const options = [{ label: "Qwen（官方国内）", value: "qwen-cn" }, { label: "Agnes AI", value: "agnes" }];
const render = (props: Record<string, unknown> = {}) => {
  const state = reactive({ provider: "qwen-cn", model: "qwen-image-2.0-pro", apiKey: "existing-secret", baseURL: "https://example.invalid", region: "", organization: "" });
  return mount(ProviderModelForm, {
    props: { state, providerOptions: options, modelOptions: [{ label: state.model, value: state.model }], ...props },
    global: {
      mocks: { $t: t },
      stubs: {
        UForm: { template: "<form><slot /></form>" },
        USelectMenu: { inheritAttrs: false, props: ["items", "disabled", "modelValue"], emits: ["update:modelValue"], template: `<div><slot name="content-top" /><select v-bind="$attrs" :disabled="disabled" :value="modelValue" @change="$emit('update:modelValue', $event.target.value)"><option v-for="item in items" :disabled="item.disabled" :value="item.value">{{ item.label }}</option></select></div>` },
        UButton: { template: '<button type="button"><slot /></button>' },
        USelect: true,
        UInput: { props: ["modelValue"], emits: ["update:modelValue", "change"], template: '<input :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" @change="$emit(\'change\', $event)" />' },
        SecretInput: { props: ["modelValue"], template: '<input type="password" :value="modelValue" />' },
      },
    },
  });
};
beforeEach(() => { vi.stubGlobal("useI18n", () => ({ t })); vi.stubGlobal("computed", computed); vi.stubGlobal("watch", watch); });
afterEach(() => vi.unstubAllGlobals());

describe("Provider / Model 选择器状态", () => {
  it('已保存的 Provider 凭据跨模态复用，留空不再标必填，输入新 Key 明确提示共享更新', async () => {
    const state = reactive({ provider: 'openai', model: 'gpt-image-1', apiKey: '', baseURL: '', region: '', organization: '' });
    const wrapper = render({ state, hasSavedCredentials: true, activeProvider: { id: 'openai', name: 'OpenAI', auth: { scheme: 'bearer', fields: ['api_key', 'base_url'], defaults: { base_url: 'https://api.openai.com/v1' } } } });
    const key = wrapper.get('[data-testid="ai-api-key"]');
    expect(key.attributes('required')).toBeUndefined();
    expect(key.attributes('placeholder')).toBe('留空使用已保存的凭据');
    expect(key.element.value).toBe('');
    expect(wrapper.get('[data-testid="ai-shared-credential-hint"]').text()).toContain('已保存 OpenAI 公共凭据');
    state.apiKey = 'new-private-fixture';
    await wrapper.vm.$nextTick();
    expect(wrapper.get('[data-testid="ai-shared-credential-hint"]').text()).toContain('将更新当前租户、环境的 OpenAI 公共凭据');
    await wrapper.setProps({ hasSavedCredentials: false });
    expect(key.attributes('required')).toBeDefined();
    expect(key.attributes('placeholder')).not.toBe('留空使用已保存的凭据');
    expect(state.apiKey).toBe('new-private-fixture');
    wrapper.unmount();
  });
  it('视觉默认只列出已确认型号，其他分组保留未知和禁用原因，不修改配置草稿', async () => {
    const state = reactive({ provider: 'openai', model: 'mini', apiKey: '', baseURL: '', region: '', organization: '' });
    const wrapper = render({ state, visionMode: true, modelOptions: [
      { value: 'tts-1', label: 'tts-1', disabled: true, vision: { status: 'unsupported', source: 'official_docs', reason_code: 'AI_VISION_IMAGE_INPUT_UNSUPPORTED' } },
      { value: 'mini', label: 'mini', vision: { status: 'supported', source: 'official_docs', source_url: 'https://developers.openai.com/api/docs/models/gpt-5.4-mini', checked_at: '2026-11-01' } },
      { value: 'fresh', label: 'fresh', vision: { status: 'unknown', source: 'unknown' } },
    ] });
    const model = () => wrapper.findAll('select')[1];
    expect(model().findAll('option').map(item => item.text())).toEqual(['mini']);
    expect(wrapper.get('[data-testid="ai-vision-model-status"]').text()).toContain('支持图片识别');
    expect(wrapper.find('[data-testid="ai-model-details"]').exists()).toBe(false);
    await wrapper.get('[data-testid="ai-model-details-toggle"]').trigger('click');
    expect(wrapper.get('a').attributes('href')).toContain('/gpt-5.4-mini');
    await wrapper.get('[data-testid="ai-vision-models-other"]').trigger('click');
    expect(model().findAll('option').map(item => item.text())).toEqual(['mini · 当前配置', 'tts-1', 'fresh']);
    expect(model().findAll('option')[1].attributes('disabled')).toBeDefined();
    expect(state.model).toBe('mini');
    await model().setValue('fresh');
    expect(wrapper.get('[data-testid="ai-vision-model-status"]').text()).toContain('用途待确认');
    await wrapper.get('[data-testid="ai-vision-models-supported"]').trigger('click');
    expect(state.model).toBe('fresh');
    wrapper.unmount();
  });
  it('旧版错误视觉绑定仍可见并提示更换，不自动改成推荐列表第一项', () => {
    const state = reactive({ provider: 'openai', model: 'tts-1', apiKey: '', baseURL: '', region: '', organization: '' });
    const wrapper = render({ state, visionMode: true, modelOptions: [
      { value: 'tts-1', label: 'tts-1', disabled: true, vision: { status: 'unsupported', source: 'official_docs', reason_code: 'AI_VISION_IMAGE_INPUT_UNSUPPORTED' } },
      { value: 'mini', label: 'mini', vision: { status: 'supported', source: 'provider_api' } },
    ] });
    expect(wrapper.get('[data-testid="ai-vision-model-status"]').attributes('role')).toBe('alert');
    expect(wrapper.text()).toContain('不能用于视觉分析'); expect(state.model).toBe('tts-1'); wrapper.unmount();
  });
  it("清空自定义地址后明确展示默认入口，切换接入方式也同步提示", async () => {
    const state = reactive({ provider: 'hunyuan', model: 'hunyuan-lite', apiKey: '', baseURL: '', authMode: 'openai' });
    const wrapper = render({ state, activeProvider: { id: 'hunyuan', name: 'Hunyuan', auth: { scheme: 'bearer', fields: ['base_url'], modes: [
      { id: 'openai', scheme: 'bearer', fields: ['base_url'], defaults: { base_url: 'https://openai-mode.invalid/v1' } },
      { id: 'tc3', scheme: 'tc3', fields: ['base_url'], defaults: { base_url: 'https://tc3-mode.invalid' } },
    ] } } });
    expect(wrapper.get('[data-testid="ai-default-base-url-hint"]').text()).toContain('https://openai-mode.invalid/v1');
    state.authMode = 'tc3';
    await wrapper.vm.$nextTick();
    expect(wrapper.get('[data-testid="ai-default-base-url-hint"]').text()).toContain('https://tc3-mode.invalid');
    state.baseURL = 'https://custom.invalid';
    await wrapper.vm.$nextTick();
    expect(wrapper.find('[data-testid="ai-default-base-url-hint"]').exists()).toBe(false);
    wrapper.unmount();
  });
  it("提供更新在线模型入口，点击仅发出目录操作事件并保留表单", async () => {
    const wrapper = render();
    const before = { ...wrapper.props('state') };
    await wrapper.get('[data-testid="ai-online-models"]').trigger('click');
    expect(wrapper.emitted('updateOnlineModels')).toHaveLength(1);
    expect(wrapper.props('state')).toEqual(before);
    expect(wrapper.get('[data-testid="ai-online-models"]').attributes('type')).toBe('button');
    wrapper.unmount();
  });
  it("实例下拉只包含实际返回型号，不补回消失的旧型号或警告", async () => {
    const wrapper = render({ dynamicModels: true, modelOptions: [{ label: "custom:14b", value: "custom:14b" }] });
    expect(wrapper.findAll("select")[1].findAll("option").map(item => item.text())).toEqual(["custom:14b"]);
    expect(wrapper.text()).not.toContain("qwen-image-2.0-pro");
    expect(wrapper.findAll('[role="alert"]')).toHaveLength(0);
    const before = { ...wrapper.props("state") };
    await wrapper.get('[data-testid="ai-instance-models-refresh"]').trigger("click");
    expect(wrapper.emitted("retryModels")).toHaveLength(1);
    expect(wrapper.props("state")).toEqual(before);
    await wrapper.setProps({ modelsLoading: true });
    expect(wrapper.get('[data-testid="ai-instance-models-refresh"]').attributes("disabled")).toBeDefined();
    wrapper.unmount();
  });
  it("修改 Base URL 后按用户提交事件触发实例同步", async () => {
    const wrapper = render({ dynamicModels: true, activeProvider: { id: "ollama", name: "Ollama", auth: { scheme: "none", fields: ["base_url"] } } });
    await wrapper.get('[data-testid="ai-base-url"]').setValue("http://private.invalid:11434");
    expect(wrapper.props("state").baseURL).toBe("http://private.invalid:11434");
    expect(wrapper.emitted("baseUrlChanged")).toHaveLength(1);
    wrapper.unmount();
  });
  it("展示真实名称并透传稳定 provider id", async () => {
    const wrapper = render();
    expect(wrapper.findAll("select")[0].findAll("option").map(item => item.text())).toEqual(options.map(item => item.label));
    await wrapper.findAll("select")[0].setValue("agnes");
    expect(wrapper.emitted("providerChanged")?.[0]).toEqual(["agnes"]);
    wrapper.unmount();
  });
  it("空目录显示说明并停止转圈", () => {
    const wrapper = render({ providerOptions: [], modelOptions: [] });
    expect(wrapper.text()).toContain(t("settings.ai.catalog.noProviders"));
    expect(wrapper.text()).toContain(t("settings.ai.catalog.noModels"));
    for (const select of wrapper.findAll("select")) {
      expect(select.attributes("disabled")).toBeDefined();
      expect(select.attributes("aria-busy")).toBe("false");
    }
    expect(wrapper.findAll("option")).toHaveLength(0);
    wrapper.unmount();
  });
  it("按正式供应商身份展示认证字段，保留已有 Key 和 Base URL", () => {
    const wrapper = render({ activeProvider: {
      id: "qwen-cn", name: "Qwen（官方国内）",
      auth: { scheme: "bearer", fields: ["api_key", "base_url"], defaults: { base_url: "https://default.invalid/v1" } },
    } });
    expect((wrapper.get('input[type="password"]').element as HTMLInputElement).value).toBe("existing-secret");
    expect(wrapper.props("state").baseURL).toBe("https://example.invalid");
    wrapper.unmount();
  });
  it("失败提示可重试，目录读取不清空已填凭据", async () => {
    const wrapper = render({ providersError: true, modelsError: true });
    expect(wrapper.findAll('[role="alert"]')).toHaveLength(2);
    const before = { ...wrapper.props("state") };
    await wrapper.findAll('[role="alert"]')[0].get("button").trigger("click");
    await wrapper.findAll('[role="alert"]')[1].get("button").trigger("click");
    expect(wrapper.emitted("retryProviders")).toHaveLength(1);
    expect(wrapper.emitted("retryModels")).toHaveLength(1);
    expect(wrapper.props("state")).toEqual(before);
    wrapper.unmount();
  });
  it("实际加载时禁用选择器，加载结束恢复操作", async () => {
    const wrapper = render({ providersLoading: true, modelsLoading: true });
    expect(wrapper.findAll("select").every(item => item.attributes("disabled") !== undefined)).toBe(true);
    expect(wrapper.findAll("select").every(item => item.attributes("aria-busy") === "true")).toBe(true);
    expect(wrapper.text()).toContain(t("settings.ai.catalog.providersLoading"));
    expect(wrapper.text()).toContain(t("settings.ai.catalog.modelsLoading"));
    await wrapper.setProps({ providersLoading: false, modelsLoading: false });
    expect(wrapper.findAll("select").every(item => item.attributes("disabled") === undefined)).toBe(true);
    expect(wrapper.findAll("select").every(item => item.attributes("aria-busy") === "false")).toBe(true);
    expect(wrapper.text()).not.toContain(t("settings.ai.catalog.providersLoading"));
    expect(wrapper.text()).not.toContain(t("settings.ai.catalog.modelsLoading"));
    wrapper.unmount();
  });
});
