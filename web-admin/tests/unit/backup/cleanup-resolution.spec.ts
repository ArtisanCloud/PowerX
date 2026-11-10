import { afterEach, beforeEach, describe, it, expect, vi } from "vitest";
import { mount, flushPromises } from "@vue/test-utils";
import { createI18n } from "vue-i18n";
import Panel from "~/components/ops/backup/BackupCleanupResolution.vue";
import {
  decodeResolutionReceipt,
  consumeResolutionStream,
} from "~/composables/api/services/backupResolutionService";
import zh from "../../../i18n/locales/zh.json";
const api = vi.hoisted(() => ({
  preview: vi.fn(),
  list: vi.fn(),
  request: vi.fn(),
  resolve: vi.fn(),
}));
const state = vi.hoisted(() => ({
  identity: "one",
  push: undefined as undefined | (() => void),
}));
vi.mock(
  "~/composables/api/services/backupResolutionService",
  async (original) => ({
    ...(await original<any>()),
    useBackupResolutionService: () => api,
  }),
);
vi.mock("~/composables/useBackupEvents", () => ({
  useBackupEvents: (_r: string, fn: () => void) => {
    state.push = fn;
    return { status: { value: "connected" } };
  },
}));
vi.mock("~/utils/auth-session", () => ({
  AUTH_SESSION_EVENT: "owned_identity",
  readAuthSession: () => ({
    sessionId: state.identity,
    tenantUuid: "owned",
    status: "active",
  }),
}));
const key = "ad93fdb6-771c-4ddd-b328-cad697205b29",
  token = "7a3bf1aa-2735-458a-bd5b-b9a4e102c0bd",
  version = "a".repeat(64),
  history = "b".repeat(64);
const item = {
  artifact_id: "1",
  job_id: "2",
  policy_id: "3",
  state: "unconfirmed" as const,
  token,
  size_bytes: 17,
  updated_at: "2026-11-09T01:00:00Z",
  protected: false,
  inspectable: true,
};
const preview = () => ({
  item,
  review_version: version,
  history_digest: history,
  unknown_records: 1,
  can_resolve: true,
  observed_at: "2026-11-09T01:00:00Z",
});
const receipt = (state = "retained") => ({
  request_id: key,
  artifact_id: "1",
  token,
  review_version: version,
  history_digest: history,
  reason: "keep owned intent",
  state,
  actor_id: "1",
  created_at: "2026-11-09T01:00:00Z",
  completed_at: state === "checking" ? null : "2026-11-09T01:01:00Z",
  can_resume: state === "checking",
  independent_complete: state !== "checking",
  sql_committed: state === "retained",
  original_history_preserved: true,
  recovery_blocked: false,
});
const stubs = {
  UAlert: { props: ["title"], template: "<div>{{title}}</div>" },
  UBadge: { template: "<span><slot/></span>" },
  UButton: {
    props: ["disabled", "loading"],
    template: '<button :disabled="disabled||loading"><slot/></button>',
  },
  UFormField: { template: "<label><slot/></label>" },
  UTextarea: {
    props: ["modelValue", "disabled"],
    emits: ["update:modelValue"],
    template:
      '<textarea :value="modelValue" :disabled="disabled" @input="$emit(\'update:modelValue\',$event.target.value)"/>',
  },
  UCheckbox: {
    props: ["modelValue", "disabled", "label"],
    emits: ["update:modelValue"],
    template:
      '<label><input type="checkbox" :checked="modelValue" :disabled="disabled" @change="$emit(\'update:modelValue\',$event.target.checked)"/>{{label}}</label>',
  },
};
let w: ReturnType<typeof mount> | undefined;
const panel = async () => {
  w = mount(Panel, {
    props: { item },
    global: {
      stubs,
      plugins: [createI18n({ legacy: false, locale: "zh", messages: { zh } })],
    },
  });
  await flushPromises();
  return w;
};
const begin = async (p: any) => {
  await p.get("textarea").setValue("keep owned intent");
  await p.get("input[type=checkbox]").setValue(true);
  await p.get("[data-testid=resolution-submit]").trigger("click");
  await flushPromises();
};
beforeEach(() => {
  vi.clearAllMocks();
  state.identity = "one";
  api.preview.mockResolvedValue(preview());
  api.list.mockResolvedValue([]);
  api.resolve.mockRejectedValue(new Error("lost response"));
  api.request.mockResolvedValue(receipt());
  vi.stubGlobal("crypto", { randomUUID: () => key });
});
afterEach(() => {
  w?.unmount();
  w = undefined;
  vi.unstubAllGlobals();
});
describe("non-deleting cleanup resolution", () => {
  it("clears transient load failure when the original terminal receipt is confirmed", async () => {
    const p = await panel();
    await begin(p);
    api.list.mockRejectedValue(new Error("temporary receipt outage"));
    state.push?.();
    await flushPromises();
    expect(p.text()).toContain(zh.backupResolution.loadFailed);
    await p.get("[data-testid=resolution-reconcile]").trigger("click");
    await flushPromises();
    expect(p.text()).not.toContain(zh.backupResolution.loadFailed);
    expect(p.get("[data-testid=resolution-receipt]").text()).toContain(key);
  });
  it("validates independent completion, unchanged history and SQL facts", () => {
    expect(decodeResolutionReceipt(receipt()).state).toBe("retained");
    for (const patch of [
      { independent_complete: false },
      { sql_committed: false },
      { original_history_preserved: false },
      { history_digest: "bad" },
      { can_resume: true },
    ])
      expect(() =>
        decodeResolutionReceipt({ ...receipt(), ...patch }),
      ).toThrow();
  });
  it("freezes UUID and history after response loss and reconciles only with GET", async () => {
    const p = await panel();
    await begin(p);
    expect(p.get("[data-testid=resolution-frozen-id]").text()).toBe(key);
    await p.get("[data-testid=resolution-reconcile]").trigger("click");
    await flushPromises();
    expect(api.resolve).toHaveBeenCalledTimes(1);
    expect(p.get("[data-testid=resolution-receipt]").text()).toContain(
      zh.backupResolution.states.retained,
    );
    expect(p.find("textarea").exists()).toBe(false);
    expect(p.emitted("locked")?.at(-1)).toEqual([false]);
  });
  it("retains original UUID after a failed GET and retries only identical explicit input", async () => {
    api.request.mockRejectedValue(new Error("404"));
    const p = await panel();
    await begin(p);
    const input = { ...api.resolve.mock.calls[0][1] };
    await p.get("[data-testid=resolution-reconcile]").trigger("click");
    await flushPromises();
    expect(p.get("[data-testid=resolution-frozen-id]").text()).toBe(key);
    await p.get("[data-testid=resolution-resume]").trigger("click");
    await flushPromises();
    expect(api.resolve.mock.calls[1][1]).toEqual(input);
  });
  it("recovers a pending request without creating a new UUID or POST", async () => {
    api.list.mockResolvedValue([receipt("checking")]);
    const p = await panel();
    expect(p.get("[data-testid=resolution-frozen-id]").text()).toBe(key);
    expect(api.resolve).not.toHaveBeenCalled();
  });
  it("permits an explicit current review while retaining blocked historical outcome", async () => {
    api.list.mockResolvedValue([
      { ...receipt("checking"), can_resume: false, recovery_blocked: true },
    ]);
    const p = await panel();
    expect(p.text()).toContain(zh.backupResolution.states.blocked);
    expect(p.find("[data-testid=resolution-resume]").exists()).toBe(false);
    await p.get("[data-testid=resolution-new-review]").trigger("click");
    await flushPromises();
    expect(p.find("textarea").exists()).toBe(true);
    state.push?.();
    await flushPromises();
    expect(p.find("textarea").exists()).toBe(true);
    expect(api.resolve).not.toHaveBeenCalled();
  });
  it("disables submission when a pushed condition refresh fails but preserves the draft", async () => {
    const p = await panel();
    await p.get("textarea").setValue("my draft");
    await p.get("input").setValue(true);
    api.preview.mockRejectedValue(new Error("offline"));
    state.push?.();
    await flushPromises();
    expect(p.get("textarea").element.value).toBe("my draft");
    expect(
      p.get("[data-testid=resolution-submit]").attributes("disabled"),
    ).toBeDefined();
  });
  it("uses push reconciliation without replaying a mutation", async () => {
    const p = await panel();
    await begin(p);
    api.list.mockResolvedValue([receipt()]);
    state.push?.();
    await flushPromises();
    expect(p.get("[data-testid=resolution-receipt]").text()).toContain(key);
    expect(api.resolve).toHaveBeenCalledTimes(1);
  });
  it("aborts the previous stream and clears its input on identity change", async () => {
    api.resolve.mockImplementation(() => new Promise(() => {}));
    const p = await panel();
    await begin(p);
    const signal = api.resolve.mock.calls[0][2];
    state.identity = "two";
    window.dispatchEvent(new Event("owned_identity"));
    await flushPromises();
    expect(signal.aborted).toBe(true);
    expect(p.find("[data-testid=resolution-frozen-id]").exists()).toBe(false);
    expect(p.get("textarea").element.value).toBe("");
  });
  it("aborts owned streams when unmounted", async () => {
    api.resolve.mockImplementation(() => new Promise(() => {}));
    const p = await panel();
    await begin(p);
    const signal = api.resolve.mock.calls[0][2];
    p.unmount();
    w = undefined;
    expect(signal.aborted).toBe(true);
  });
});
const frame = (name: string, v: any) =>
    `event: ${name}\ndata: ${JSON.stringify(v)}\n\n`,
  stream = (s: string) =>
    new ReadableStream<Uint8Array>({
      start(c) {
        c.enqueue(new TextEncoder().encode(s));
        c.close();
      },
    });
it("requires an SSE result bound to original intent, UUID and history; EOF stays unknown", async () => {
  const input = {
      request_id: key,
      expected_token: token,
      review_version: version,
      history_digest: history,
      reason: "keep owned intent",
      confirm: true as const,
    },
    signal = new AbortController().signal;
  expect(
    (
      await consumeResolutionStream(
        stream(
          frame("accepted", receipt("checking")) + frame("complete", receipt()),
        ),
        "1",
        input,
        signal,
        () => {},
      )
    ).state,
  ).toBe("retained");
  for (const data of [
    frame("accepted", receipt("checking")),
    frame("complete", { ...receipt(), history_digest: "c".repeat(64) }),
    frame("complete", receipt("checking")),
    frame("uncertain", { request_id: key }),
  ])
    await expect(
      consumeResolutionStream(stream(data), "1", input, signal, () => {}),
    ).rejects.toThrow();
});
