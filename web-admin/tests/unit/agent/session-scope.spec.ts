import { beforeEach, describe, expect, it, vi } from "vitest";
import { createPinia, setActivePinia } from "pinia";
import { ref } from "vue";
import { useAgentSessionStore } from "~/stores/agentSession";
import { useChatSessions } from "~/composables/agent/useChatSessions";
import {
  sessionWorkspaceKey,
  type ChatSessionScope,
} from "~/utils/agent/sessionScope";

const api = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  patch: vi.fn(),
  delete: vi.fn(),
}));
vi.mock("~/composables/api", () => ({ useApiClient: () => api }));
vi.mock("vue-i18n", () => ({ useI18n: () => ({ t: (key: string) => key }) }));
vi.mock("~/stores/envStore", () => ({
  useEnvStore: () => ({ currentEnv: "dev" }),
}));
vi.mock("~/stores/user", () => ({
  useUserStore: () => ({ user: { id: 1 }, currentTenantUuid: "tenant-a" }),
}));

beforeEach(() => {
  setActivePinia(createPinia());
  localStorage.clear();
  vi.clearAllMocks();
  const state = new Map<string, unknown>();
  vi.stubGlobal("useState", (key: string, init: () => unknown) => {
    if (!state.has(key)) state.set(key, ref(init()));
    return state.get(key);
  });
});

describe("session workspace isolation", () => {
  it("keeps distinct lists and selections for single chat and two teams sharing one leader", () => {
    const store = useAgentSessionStore();
    const scopes: ChatSessionScope[] = [
      { sessionType: "agent" },
      { sessionType: "team", teamId: "3" },
      { sessionType: "team", teamId: "4" },
    ];
    const keys = scopes.map((scope) =>
      sessionWorkspaceKey("1", "tenant-a", "dev", scope),
    );
    keys.forEach((key, index) => {
      store.activateScope(key);
      expect(store.currentSessionId).toBeNull();
      store.addSession("leader", { id: index + 1, title: `session-${index}` });
    });
    keys.forEach((key, index) => {
      store.activateScope(key);
      expect(store.currentSessionId).toBe(index + 1);
      expect(store.getSessionsByAgent("leader").map((s) => s.id)).toEqual([
        index + 1,
      ]);
    });
    store.activateScope(
      sessionWorkspaceKey("1", "tenant-b", "dev", scopes[0]!),
    );
    expect(store.sessionsByAgent).toEqual({});
  });

  it("sends scope on create, list, history and rename; keeps the last Agent selection separate", async () => {
    const scope = ref<ChatSessionScope>({ sessionType: "team", teamId: "3" });
    const chat = useChatSessions({ scope });
    api.get.mockResolvedValue({ code: 200, data: { items: [] } });
    api.post.mockResolvedValue({
      data: { id: 8, sessionType: "team", teamId: 3 },
    });
    await chat.createSession("leader");
    expect(api.post).toHaveBeenCalledWith(
      "/agents/sessions",
      expect.objectContaining({
        sessionType: "team",
        teamId: 3,
        agentUuid: "leader",
      }),
    );
    await chat.listSessions("leader", true);
    await chat.loadSessionMessages(8, true);
    await chat.renameSession("leader", 8, "renamed");
    for (const [, options] of api.get.mock.calls)
      expect(options.params).toMatchObject({
        session_type: "team",
        team_id: "3",
      });
    expect(api.patch.mock.calls[0]![2].params).toMatchObject({
      session_type: "team",
      team_id: "3",
    });
    expect(chat.getLastSelectedAgentId()).toBe("leader");
    scope.value = { sessionType: "agent" };
    expect(chat.currentSessionId.value).toBeNull();
    expect(chat.getLastSelectedAgentId()).toBeNull();
  });

  it("an old list response cannot replace the newly selected workspace", async () => {
    let finish!: (value: unknown) => void;
    api.get.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        }),
    );
    const scope = ref<ChatSessionScope>({ sessionType: "team", teamId: "3" });
    const chat = useChatSessions({ scope });
    const oldKey = chat.scopeKey.value;
    const pending = chat.listSessions("leader");
    scope.value = { sessionType: "team", teamId: "4" };
    api.get.mockResolvedValueOnce({
      code: 200,
      data: { items: [{ id: 4, sessionType: "team", teamId: 4 }] },
    });
    await chat.listSessions("leader");
    finish({
      code: 200,
      data: { items: [{ id: 3, sessionType: "team", teamId: 3 }] },
    });
    await pending;
    expect(chat.sessionsByAgent.value.leader!.map((s) => s.id)).toEqual([4]);
    expect(
      useAgentSessionStore()
        .workspace(oldKey)
        .sessionsByAgent.leader!.map((s) => s.id),
    ).toEqual([3]);
  });

  it("keeps unresolved history in its own scope and rejects an unscoped creation response", async () => {
    const scope = ref<ChatSessionScope>({ sessionType: "legacy" });
    const chat = useChatSessions({ scope });
    api.get.mockResolvedValue({
      code: 200,
      data: {
        items: [
          { id: 1, sessionType: "legacy" },
          { id: 2, sessionType: "agent" },
        ],
      },
    });
    await chat.listSessions("leader");
    expect(chat.sessionsByAgent.value.leader!.map((s) => s.id)).toEqual([1]);
    scope.value = { sessionType: "agent" };
    api.post.mockResolvedValue({ data: { id: 8 } });
    await expect(chat.createSession("leader")).rejects.toThrow(
      "agent.chat.sessionScopeMismatch",
    );
  });
});
