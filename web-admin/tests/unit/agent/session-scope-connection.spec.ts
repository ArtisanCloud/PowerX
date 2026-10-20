import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { createApp, defineComponent, ref, type App } from "vue";
import {
  useDualChannelConnection,
  type DualChannelConnection,
} from "~/composables/agent/useDualChannelConnection";
import { savePendingDurableRun } from "~/utils/agent/pendingDurableRun";
import {
  sessionWorkspaceKey,
  type ChatSessionScope,
} from "~/utils/agent/sessionScope";
import { AUTH_SESSION_KEY } from "~/utils/auth-session";

vi.mock("~/stores/envStore", () => ({
  useEnvStore: () => ({ currentEnv: "dev" }),
}));
vi.mock("~/stores/user", () => ({
  useUserStore: () => ({ user: { id: 1 }, currentTenantUuid: "tenant" }),
}));
vi.mock("~/stores/message", () => ({
  useMessageStore: () => ({
    setMessages: vi.fn(),
    getMessagesBySession: () => [],
  }),
}));

let app: App | undefined;
beforeEach(() => {
  vi.stubGlobal("process", { ...process, client: true });
  localStorage.clear();
  localStorage.setItem(
    AUTH_SESSION_KEY,
    JSON.stringify({
      version: 1,
      status: "active",
      sessionId: "fixture",
      revision: 1,
      accessToken: "fixture",
      refreshToken: "fixture",
      tokenType: "Bearer",
      expiresAt: Date.now() + 60000,
      scope: "access",
      tenantUuid: "tenant",
    }),
  );
  vi.stubGlobal("useRuntimeConfig", () => ({
    public: { apiBase: "http://core/api/v1" },
  }));
  vi.stubGlobal("useI18n", () => ({ t: (key: string) => key }));
  vi.stubGlobal(
    "WebSocket",
    class {
      constructor() {
        throw new Error("disabled fixture");
      }
    },
  );
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response(": connected\n\n")),
  );
});
afterEach(() => {
  app?.unmount();
  vi.unstubAllGlobals();
});

it("only discovers pending Runs in the selected type and team, and resumes with that scope", async () => {
  const scope = ref<ChatSessionScope>({ sessionType: "team", teamId: "3" });
  const key = ref(sessionWorkspaceKey("1", "tenant", "dev", scope.value));
  const runId = "2a07c49a-02fb-45ef-b9ee-af6aa23ba132";
  savePendingDurableRun(localStorage, `1:tenant:dev:${key.value}`, {
    runId,
    agentId: "leader",
    sessionId: "42",
    updatedAt: Date.now(),
  });
  let chat!: DualChannelConnection;
  app = createApp(
    defineComponent({
      setup() {
        chat = useDualChannelConnection(ref("leader"), ref("42"), {
          scope,
          key,
        });
        return () => null;
      },
    }),
  );
  app.mount(document.createElement("div"));
  expect(chat.getPendingRun()?.runId).toBe(runId);
  scope.value = { sessionType: "agent" };
  key.value = sessionWorkspaceKey("1", "tenant", "dev", scope.value);
  expect(chat.getPendingRun()).toBeNull();
  scope.value = { sessionType: "team", teamId: "4" };
  key.value = sessionWorkspaceKey("1", "tenant", "dev", scope.value);
  expect(chat.getPendingRun()).toBeNull();
  scope.value = { sessionType: "team", teamId: "3" };
  key.value = sessionWorkspaceKey("1", "tenant", "dev", scope.value);
  await chat.resumePendingRun();
  const requests = vi
    .mocked(fetch)
    .mock.calls.map(([url]) => new URL(String(url)))
    .filter((url) => url.searchParams.has("run_id"));
  expect(requests).toHaveLength(1);
  expect(requests[0]!.searchParams.get("session_type")).toBe("team");
  expect(requests[0]!.searchParams.get("team_id")).toBe("3");
  expect(requests[0]!.searchParams.has("q")).toBe(false);
});
