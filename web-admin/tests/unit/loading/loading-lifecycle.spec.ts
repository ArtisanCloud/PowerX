import { beforeEach, describe, expect, it, vi } from "vitest";
import { ref } from "vue";

const state = vi.hoisted(() => ({
  pending: { value: 0 }, auto: { value: false }, nav: { value: false },
  progress: vi.fn(), message: vi.fn(),
}));
vi.mock("~/composables/useGlobalLoading", () => ({
  useGL_ReqPending: () => state.pending,
  useGL_AutoVisible: () => state.auto,
  useGL_NavPending: () => state.nav,
  useGlobalLoading: () => ({ setProgress: state.progress, setMessage: state.message }),
}));
import { setApiConfig, useApiClient } from "~/composables/api";

const deferred = () => {
  let resolve!: (value: any) => void;
  const promise = new Promise((yes) => { resolve = yes; });
  return { promise, resolve };
};

let network: ReturnType<typeof vi.fn>;
beforeEach(async () => {
  (process as any).client = true;
  localStorage.clear();
  state.pending.value = 0; state.auto.value = false; state.nav.value = false;
  vi.clearAllMocks();
  const states = new Map<string, any>();
  vi.stubGlobal("useState", (key: string, init: () => any) => {
    if (!states.has(key)) states.set(key, ref(init()));
    return states.get(key);
  });
  vi.stubGlobal("defineNuxtPlugin", (fn: any) => fn);
  vi.stubGlobal("useGL_ReqPending", () => state.pending);
  vi.stubGlobal("useGL_AutoVisible", () => state.auto);
  vi.stubGlobal("useGL_NavPending", () => state.nav);
  network = vi.fn();
  const fetch = Object.assign(vi.fn(), {
    create: (hooks: any) => async (url: string, options: any = {}) => {
      const context = { options };
      hooks.onRequest(context);
      const response = await network(url);
      hooks.onResponse(context);
      if (response?.fail) {
        hooks.onResponseError(context);
        throw { status: 403, message: "forbidden", response: { status: 403 } };
      }
      return response;
    },
  });
  vi.stubGlobal("$fetch", fetch);
  const plugin = await import("~/plugins/global-loading");
  (plugin.default as any)({ hook: vi.fn() });
  setApiConfig({ baseURL: "/api/v1" });
});

describe("request loading ownership", () => {
  it("counts API requests once and keeps silent requests from ending other loading", async () => {
    const busy = deferred(), silent = deferred();
    network.mockImplementation((url) => url.endsWith("/busy") ? busy.promise : silent.promise);
    const one = useApiClient().get("/busy", { skipAuth: true });
    const two = useApiClient().get("/silent", { skipAuth: true, useGlobalLoading: false });
    await vi.waitFor(() => expect(network).toHaveBeenCalledTimes(2));
    expect(state.pending.value).toBe(1);
    silent.resolve({ ok: true }); await two;
    expect(state.pending.value).toBe(1); expect(state.auto.value).toBe(true);
    busy.resolve({ ok: true }); await one;
    expect(state.pending.value).toBe(0); expect(state.auto.value).toBe(false);
  });

  it("does not let a failed silent request decrement a pending API request", async () => {
    const busy = deferred();
    network.mockImplementation((url) => url.endsWith("/busy") ? busy.promise : { fail: true });
    const one = useApiClient().get("/busy", { skipAuth: true });
    await expect(useApiClient().get("/silent", { skipAuth: true, useGlobalLoading: false })).rejects.toThrow();
    expect(state.pending.value).toBe(1);
    busy.resolve({ ok: true }); await one;
    expect(state.pending.value).toBe(0);
  });

  it("ends direct fetch HTTP errors once even when both ofetch response hooks run", async () => {
    const busy = deferred();
    network.mockImplementation((url) => url === "/busy" ? busy.promise : { fail: true });
    const one = (globalThis as any).$fetch("/busy");
    await expect((globalThis as any).$fetch("/error")).rejects.toMatchObject({ status: 403 });
    expect(state.pending.value).toBe(1); expect(state.auto.value).toBe(true);
    busy.resolve({ ok: true }); await one;
    expect(state.pending.value).toBe(0);
  });

  it("keeps navigation loading active after the last API response", async () => {
    state.nav.value = true;
    network.mockResolvedValue({ ok: true });
    await useApiClient().get("/busy", { skipAuth: true });
    expect(state.pending.value).toBe(0); expect(state.auto.value).toBe(true);
  });

  it("loads user menus without opening request loading", async () => {
    vi.stubGlobal("useNuxtApp", () => ({ $i18n: { locale: ref("zh") } }));
    network.mockResolvedValue({ data: { categories: [{ id: "main", order: 1, children: [{ id: "agent", path: "/agent/sessions", title: "智能会话", order: 1 }] }] } });
    // 菜单请求使用真实 API 客户端和测试会话，不改变 loading 路径。
    const { commitAuthSession } = await import("~/utils/auth-session");
    commitAuthSession({ access_token: `h.${btoa(JSON.stringify({ tid: "tenant", exp: Math.floor(Date.now() / 1000) + 3600 }))}.s`, refresh_token: "test-refresh", expires_in: 3600 });
    const { useMenuService } = await import("~/composables/api/services/menuService");
    const menus = await useMenuService().getUserMenus();
    expect(menus.categories[0]?.children[0]?.path).toBe("/agent/sessions");
    expect(state.pending.value).toBe(0); expect(state.auto.value).toBe(false);
  });
});
