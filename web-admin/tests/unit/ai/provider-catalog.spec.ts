import { beforeEach, describe, expect, it, vi } from "vitest";
import { createPinia, setActivePinia } from "pinia";
import { AISettingService } from "~/composables/api/services/aiSettingService";
import { useAISettingsStore } from "~/stores/aiSettings";

const { get } = vi.hoisted(() => ({ get: vi.fn() }));
vi.mock("~/composables/api", () => ({ useApiClient: () => ({ get }) }));

const provider = {
  id: "qwen-cn", name: "Qwen（官方国内）", configured: true,
  apps: [{ id: "vision", name: "Vision" }],
  auth: { scheme: "bearer", fields: ["api_key", "base_url"], defaults: { base_url: "https://example.invalid/v1" } },
};
const response = (providers: unknown) => ({ data: { env: "dev", modality: "image", providers } });

beforeEach(() => { get.mockReset(); setActivePinia(createPinia()); });

describe("Admin 供应商目录合同", () => {
  it("实例地址传到模型接口，云目录继续按原合同读取", async () => {
    get.mockResolvedValue({ data: { models: ["custom:14b"] } });
    await AISettingService.getModels("ollama", "llm", "dev", "", "http://private.invalid:11434/v1");
    const query = new URL(get.mock.calls[0][0], "http://localhost").searchParams;
    expect(query.get("base_url")).toBe("http://private.invalid:11434/v1");
    expect(get.mock.calls[0][1]).toEqual({ useGlobalLoading: false });
    await AISettingService.getModels("qwen-cn", "llm", "dev", "", "https://cloud.invalid/v1");
    expect(get.mock.calls[1][0]).not.toContain("base_url");
  });
  it.each([false, true])("切换实例后拒绝慢速旧请求覆盖新目录（旧请求失败：%s）", async (failure) => {
    const store = useAISettingsStore();
    let finish!: (result: any) => void;
    let fail!: (error: Error) => void;
    get.mockReturnValueOnce(new Promise((resolve, reject) => { finish = resolve; fail = reject; }));
    const old = store.fetchModels("ollama", "llm", "dev", "", "http://old.invalid");
    get.mockResolvedValueOnce({ data: { models: ["new:14b"] } });
    await store.fetchModels("ollama", "llm", "dev", "", "http://new.invalid");
    if (failure) fail(new Error("old instance offline"));
    else finish({ data: { models: ["old:latest"] } });
    expect(await old).toBe(false);
    expect(store.models).toEqual(["new:14b"]);
    expect(store.modelsLoaded).toBe(true);
    expect(store.modelsLoading).toBe(false);
    expect(store.modelsError).toBe(false);
  });
  it("读取正式 id/name，保留认证字段、App 和配置状态", async () => {
    get.mockResolvedValue(response([provider]));
    expect(await AISettingService.getProviders("image", "dev")).toEqual([provider]);
    expect(get.mock.calls[0][0]).toContain("modality=image&env=dev");
  });

  it("空目录保持为空", async () => {
    get.mockResolvedValue(response([]));
    expect(await AISettingService.getProviders("image", "dev")).toEqual([]);
  });

  it.each([
    { name: "旧大写字段", providers: [{ ID: "qwen-cn", Name: "Qwen" }] },
    { name: "空显示名", providers: [{ id: "qwen-cn", name: "" }] },
    { name: "缺少标识", providers: [{ id: undefined, name: "Qwen" }] },
    { name: "非数组", providers: null },
  ])("无效目录显式失败，不把损坏数据渲染为空白选项：$name", async ({ providers }) => {
    get.mockResolvedValue(response(providers));
    await expect(AISettingService.getProviders("image", "dev")).rejects.toThrow("AI_PROVIDER_CATALOG_INVALID");
  });

  it("加载失败后结束 loading，保留已填配置，重试成功后清除错误", async () => {
    const store = useAISettingsStore();
    store.providers = [provider];
    store.credentials = [{ provider: "qwen-cn", data: { api_key: "existing-secret" } }] as any;
    const credentials = JSON.stringify(store.credentials);
    let reject!: (error: Error) => void;
    get.mockReturnValueOnce(new Promise((_, fail) => { reject = fail; }));
    const pending = store.fetchProviders("image", "dev");
    const assertion = expect(pending).rejects.toThrow("offline");
    expect(store.providersLoading).toBe(true);
    reject(new Error("offline"));
    await assertion;
    expect(store.providersLoading).toBe(false);
    expect(store.providersError).toBe(true);
    expect(store.providers).toEqual([]);
    expect(JSON.stringify(store.credentials)).toBe(credentials);
    get.mockResolvedValueOnce(response([provider]));
    await store.fetchProviders("image", "dev");
    expect(store.providersError).toBe(false);
    expect(store.providers[0].id).toBe("qwen-cn");
  });

  it("型号空目录结束 loading；请求失败可识别并重试", async () => {
    const store = useAISettingsStore();
    get.mockResolvedValueOnce({ data: [] });
    await store.fetchModels("qwen-cn", "image", "dev");
    expect(store.models).toEqual([]);
    expect(store.modelsLoading).toBe(false);
    expect(store.modelsError).toBe(false);
    get.mockRejectedValueOnce(new Error("offline"));
    await expect(store.fetchModels("qwen-cn", "image", "dev")).rejects.toThrow("offline");
    expect(store.modelsLoading).toBe(false);
    expect(store.modelsError).toBe(true);
    get.mockResolvedValueOnce({ data: ["qwen-image-2.0-pro"] });
    await store.fetchModels("qwen-cn", "image", "dev");
    expect(store.modelsError).toBe(false);
    expect(store.models).toEqual(["qwen-image-2.0-pro"]);
  });
});
