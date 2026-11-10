import { mount } from "@vue/test-utils";
import { createPinia, setActivePinia } from "pinia";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { AISettingService } from "~/composables/api/services/aiSettingService";
import { useAISettingsStore } from "~/stores/aiSettings";
import TestPanel from "~/components/settings/ai/TestPanel.vue";
import zh from "../../../i18n/locales/zh.json";

const { post, get } = vi.hoisted(() => ({ post: vi.fn(), get: vi.fn() }));
vi.mock("~/composables/api", () => ({ useApiClient: () => ({ post, get }) }));
const payload = { env: "dev", modality: "image", image: { provider: "agnes", model: "agnes-image-2.5-flash" } } as any;
const verified = { ok: true, verification: { endpoint_reachable: true, api_key_verified: true, authentication: "api_key", generation_submitted: false } };
const t = (key: string) => key.split(".").reduce((obj: any, part) => obj?.[part], zh) ?? key;

beforeEach(() => { post.mockReset(); get.mockReset(); setActivePinia(createPinia()); });

describe("只读连接检测的局部反馈", () => {
  it.each(["connection", "quick"])("%s 的旧响应在上下文失效后不能回写结果", async action => {
    let finish!: (response: unknown) => void;
    post.mockReturnValueOnce(new Promise(resolve => { finish = resolve; }));
    const store = useAISettingsStore();
    const pending = action === "connection" ? store.testConnection("agnes", payload) : store.testQuickCall("agnes", "model", payload);
    store.clearTestMessage();
    finish({ data: { ...verified, result: "old result" } });
    await pending;
    expect(store.lastTestMessage).toBe("");
    expect(store.lastTestDetail).toBe("");
    expect(store.testing).toBe(false);
  });

  it("失效的失败响应也不会污染新 Provider 的结果区", async () => {
    let fail!: (error: unknown) => void;
    post.mockReturnValueOnce(new Promise((_resolve, reject) => { fail = reject; }));
    const store = useAISettingsStore();
    const pending = store.testConnection("agnes", payload);
    store.clearTestMessage();
    fail(new Error("AI_CONNECTION_AUTH_FAILED"));
    await expect(pending).rejects.toThrow("AI_CONNECTION_AUTH_FAILED");
    expect(store.lastTestMessage).toBe("");
    expect(store.lastTestDetail).toBe("");
    expect(store.testing).toBe(false);
  });

  it("检测和后续凭据刷新都不触发全屏 Loading", async () => {
    post.mockResolvedValue({ data: verified });
    await AISettingService.testConnection(payload);
    expect(post).toHaveBeenCalledWith(expect.stringContaining("test/connection"), payload, { useGlobalLoading: false });
    get.mockResolvedValue({ data: { env: "dev", credentials: [] } });
    const store = useAISettingsStore();
    await store.fetchCredentials("dev", { useGlobalLoading: false });
    expect(get).toHaveBeenCalledWith(expect.stringContaining("credentials?env=dev"), { useGlobalLoading: false });
    expect(store.credentials).toEqual([]);
  });

  it("检测进行中拒绝重复请求，保留表单与凭据", async () => {
    let finish!: (response: unknown) => void;
    post.mockReturnValueOnce(new Promise(resolve => { finish = resolve; }));
    const store = useAISettingsStore();
    store.credentials = [{ provider: "agnes", data: { api_key: "existing-secret" } }] as any;
    const before = JSON.stringify(store.credentials);
    const pending = store.testConnection("agnes", payload);
    expect(store.testingAction).toBe("connection");
    expect(store.testing).toBe(true);
    await expect(store.testConnection("agnes", payload)).rejects.toThrow("AI_TEST_IN_PROGRESS");
    expect(post).toHaveBeenCalledTimes(1);
    finish({ data: verified });
    await pending;
    expect(store.testing).toBe(false);
    expect(store.testingAction).toBeNull();
    expect(JSON.stringify(store.credentials)).toBe(before);
  });

  it.each([
    { ok: false, message: "AI_CONNECTION_AUTH_FAILED" },
    { ok: true },
    { ok: true, verification: { ...verified.verification, api_key_verified: false } },
    { ok: true, verification: { ...verified.verification, generation_submitted: true } },
  ])("本地校验或不可信响应不能显示连接成功：%j", async (result) => {
    post.mockResolvedValue({ data: result });
    const store = useAISettingsStore();
    await expect(store.testConnection("agnes", payload)).rejects.toThrow();
    expect(store.testing).toBe(false);
    expect(store.testingAction).toBeNull();
    expect(store.lastTestMessage).not.toContain("连接测试成功");
  });

  it("供应商失败后恢复按钮，原错误码保留以便本地化", async () => {
    const error = Object.assign(new Error("AI_CONNECTION_AUTH_FAILED"), { errorCode: "AI_CONNECTION_AUTH_FAILED", status: 422 });
    post.mockRejectedValue(error);
    const store = useAISettingsStore();
    await expect(store.testConnection("agnes", payload)).rejects.toBe(error);
    expect(store.testingAction).toBeNull();
    expect(store.lastTestDetail).toContain("AI_CONNECTION_AUTH_FAILED");
  });

  it("参数检查失败不能报成功，也不显示全屏 Loading", async () => {
    post.mockResolvedValue({ data: { ok: false, message: "invalid parameter" } });
    const store = useAISettingsStore();
    await expect(store.testQuickCall("agnes", "agnes-image-2.5-flash", payload)).rejects.toThrow("invalid parameter");
    expect(post.mock.calls[0][2]).toEqual({ useGlobalLoading: false });
    expect(store.testing).toBe(false);
  });

  it("试跑失败直接显示参数原因，并保留完整技术详情", async () => {
    const message = "Unsupported parameter: 'max_tokens'. Use 'max_completion_tokens' instead.";
    const raw = `openai invoke url=https://api.openai.com/v1/chat/completions status=400 body=${JSON.stringify({ error: { message } })}`;
    post.mockResolvedValue({ data: { ok: false, message: raw } });
    const store = useAISettingsStore();
    await expect(store.testQuickCall("openai", "gpt-6-luna", payload)).rejects.toThrow(raw);
    expect(store.lastTestMessage).toBe(`快速调用测试失败: ${message}`);
    expect(store.lastTestDetail).toBe(raw);
    expect(store.testing).toBe(false);
  });
});

describe("检测面板", () => {
  const render = (props: Record<string, unknown> = {}) => mount(TestPanel, {
    props: { currentTitle: "视频生成", currentState: { provider: "agnes", model: "agnes-video-2.5", apiKey: "existing-secret" }, quickCallValidates: true, ...props },
    global: {
      mocks: { $t: t },
      stubs: { UButton: { props: ["disabled", "loading"], template: '<button type="button" :disabled="disabled" :data-loading="loading"><slot /></button>' } },
    },
  });

  it("未选型号仍可检查供应商连接，试跑和参数检查需先选择模型", async () => {
    const onTestConnection = vi.fn(), onTestQuickCall = vi.fn();
    const wrapper = render({ currentState: { provider: "ollama", model: "", baseURL: "" }, onTestConnection, onTestQuickCall });
    const buttons = wrapper.findAll("button");
    expect(buttons[0].attributes("disabled")).toBeUndefined();
    expect(buttons[1].attributes("disabled")).toBeDefined();
    expect(wrapper.text()).toContain(t("settings.ai.connectionTest.selectModelBeforeRun"));
    await buttons[0].trigger("click");
    await buttons[1].trigger("click");
    expect(onTestConnection).toHaveBeenCalledOnce();
    expect(onTestQuickCall).not.toHaveBeenCalled();
    await wrapper.setProps({ currentState: { provider: "ollama", model: "qwen3:14b" } });
    expect(buttons[1].attributes("disabled")).toBeUndefined();
    expect(wrapper.text()).not.toContain(t("settings.ai.connectionTest.selectModelBeforeRun"));
    wrapper.unmount();
  });

  it("只让当前按钮转圈并禁用重复操作，保留结果区和配置", async () => {
    const onTestConnection = vi.fn();
    const wrapper = render({ connectionTesting: true, lastTestMessage: "旧成功消息", onTestConnection });
    const buttons = wrapper.findAll("button");
    expect(buttons.every(button => button.attributes("disabled") !== undefined)).toBe(true);
    expect(buttons[0].attributes("data-loading")).toBe("true");
    expect(wrapper.get('[role="status"]').attributes("aria-busy")).toBe("true");
    expect(wrapper.text()).toContain(t("settings.ai.connectionTest.connecting"));
    expect(wrapper.text()).not.toContain("旧成功消息");
    await buttons[0].trigger("click");
    expect(onTestConnection).not.toHaveBeenCalled();
    await wrapper.setProps({ connectionTesting: false, lastTestMessage: "API Key 验证通过" });
    expect(wrapper.get('[role="status"]').text()).toContain("API Key 验证通过");
    expect(wrapper.props("currentState").apiKey).toBe("existing-secret");
    wrapper.unmount();
  });

  it("媒体参数检查与文本付费调用使用准确按钮名称", async () => {
    const wrapper = render();
    expect(wrapper.findAll("button")[1].text()).toBe("检查参数");
    expect(wrapper.text()).toContain("不生成内容");
    await wrapper.setProps({ quickCallValidates: false });
    expect(wrapper.findAll("button")[1].text()).toBe("试跑文本（调用模型）");
    wrapper.unmount();
  });
});
