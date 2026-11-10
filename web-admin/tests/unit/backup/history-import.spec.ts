import { beforeEach, afterEach, describe, it, expect, vi } from "vitest";
import { mount, flushPromises } from "@vue/test-utils";
import { createI18n } from "vue-i18n";
import Panel from "~/components/ops/backup/BackupRetentionHistory.vue";
import {
  decodeHistory,
  decodeHistoryReceipt,
  consumeHistoryStream,
} from "~/composables/api/services/backupHistoryService";
import zh from "../../../i18n/locales/zh.json";
const api = vi.hoisted(() => ({
  preview: vi.fn(),
  recent: vi.fn(),
  request: vi.fn(),
  import: vi.fn(),
}));
const state = vi.hoisted(() => ({
  identity: "one",
  push: undefined as undefined | (() => void),
}));
vi.mock(
  "~/composables/api/services/backupHistoryService",
  async (original) => ({
    ...(await original<any>()),
    useBackupHistoryService: () => api,
  }),
);
vi.mock("~/composables/useBackupEvents", () => ({
  useBackupEvents: (_resource: string, fn: () => void) => {
    state.push = fn;
    return { status: { value: "connected" } };
  },
}));
vi.mock("~/utils/auth-session", () => ({
  AUTH_SESSION_EVENT: "owned_identity_change",
  readAuthSession: () => ({
    sessionId: state.identity,
    tenantUuid: "owned",
    status: "active",
  }),
}));
const key = "9df237fb-7068-43a3-a81b-82497adeb2f6",
  snapshot = "a".repeat(64),
  source = "b".repeat(64);
const preview = () => ({
  snapshot,
  source_digest: source,
  observed_at: "2026-11-09T01:00:00Z",
  total: 3,
  foreign: 0,
  covered: 0,
  missing: 3,
  conflicts: 0,
  invalid: 0,
  unknown: 1,
  complete: false,
  restore_ready: false,
  can_import: true,
  pause_ready: true,
  remaining: 4096,
  page: 1,
  page_size: 20,
  filtered: 3,
  items: [
    {
      row_id: "1",
      request_id: key,
      artifact_id: "1",
      actor_id: "9",
      state: "checking",
      coverage: "missing",
      created_at: "2026-11-09T01:00:00Z",
    },
  ],
});
const receipt = (stage = "completed") => ({
  request_id: key,
  actor_id: "1",
  expected_snapshot: snapshot,
  reason: "copy old identity history",
  state: stage,
  total: stage === "pending" ? 0 : 3,
  covered: stage === "pending" ? 0 : 3,
  imported: stage === "pending" ? 0 : 3,
  unknown: stage === "pending" ? 0 : 1,
  complete: stage === "completed",
  created_at: "2026-11-09T01:00:00Z",
  completed_at: stage === "pending" ? null : "2026-11-09T01:01:00Z",
  can_resume: stage === "pending",
  ...(stage === "completed" ? { source_digest: source } : {}),
});
const stubs = {
  UIcon: true,
  UAlert: {
    props: ["title", "description"],
    template: '<div>{{title}} {{description}}<slot name="actions"/></div>',
  },
  UBadge: { template: "<span><slot/></span>" },
  UButton: {
    props: ["disabled", "loading"],
    template: '<button :disabled="disabled||loading"><slot/></button>',
  },
  UFormField: { template: "<label><slot/></label>" },
  UTextarea: {
    props: ["modelValue"],
    emits: ["update:modelValue"],
    template:
      '<textarea :value="modelValue" @input="$emit(\'update:modelValue\',$event.target.value)"/>',
  },
  UCheckbox: {
    props: ["modelValue", "label"],
    emits: ["update:modelValue"],
    template:
      '<label><input type="checkbox" :checked="modelValue" @change="$emit(\'update:modelValue\',$event.target.checked)"/>{{label}}</label>',
  },
};
let w: ReturnType<typeof mount> | undefined;
const panel = async () => {
  w = mount(Panel, {
    global: {
      stubs,
      plugins: [createI18n({ legacy: false, locale: "zh", messages: { zh } })],
    },
  });
  await flushPromises();
  return w;
};
const begin = async (p: any) => {
  await p.get("[data-testid=history-open-import]").trigger("click");
  await p.get("textarea").setValue("copy old identity history");
  await p.get("input[type=checkbox]").setValue(true);
  await p.get("[data-testid=history-submit]").trigger("click");
  await flushPromises();
};
beforeEach(() => {
  vi.clearAllMocks();
  state.identity = "one";
  api.preview.mockResolvedValue(preview());
  api.recent.mockResolvedValue([]);
  api.import.mockRejectedValue(new Error("lost response"));
  api.request.mockResolvedValue(receipt());
  vi.stubGlobal("crypto", { randomUUID: () => key });
});
afterEach(() => {
  w?.unmount();
  w = undefined;
  vi.unstubAllGlobals();
});
describe("legacy receipt coverage import", () => {
  it("shows completed proof before offering an explicit new import", async () => {
    api.import.mockResolvedValue(receipt());
    const p = await panel();
    await begin(p);
    expect(p.find("textarea").exists()).toBe(false);
    expect(p.get("[data-testid=history-receipt]").text()).toContain(key);
    await p.get("[data-testid=history-new-import]").trigger("click");
    await flushPromises();
    expect(p.get("textarea").element.value).toBe("");
    expect(p.find("[data-testid=history-receipt]").exists()).toBe(false);
    expect(api.import).toHaveBeenCalledTimes(1);
  });
  it("never treats incomplete or malformed proof as complete", () => {
    expect(decodeHistory(preview()).complete).toBe(false);
    expect(decodeHistoryReceipt(receipt()).complete).toBe(true);
    for (const patch of [
      { covered: 2 },
      { complete: false },
      { can_resume: true },
      { source_digest: "bad" },
    ])
      expect(() => decodeHistoryReceipt({ ...receipt(), ...patch })).toThrow();
    expect(() => decodeHistory({ ...preview(), complete: true })).toThrow();
  });
  it("keeps original UUID/input after a lost response and reconciles through GET only", async () => {
    const p = await panel();
    await begin(p);
    expect(p.get("[data-testid=history-frozen-id]").text()).toBe(key);
    expect(p.text()).toContain(zh.backupHistory.unknown);
    await p.get("[data-testid=history-reconcile]").trigger("click");
    await flushPromises();
    expect(api.request).toHaveBeenCalledWith(key, expect.any(AbortSignal));
    expect(api.import).toHaveBeenCalledTimes(1);
    expect(p.get("[data-testid=history-receipt]").text()).toContain(
      zh.backupHistory.states.completed,
    );
    expect(p.find("[data-testid=history-frozen-id]").exists()).toBe(false);
  });
  it("explicitly retries the identical frozen snapshot and reason", async () => {
    const p = await panel();
    await begin(p);
    const original = { ...api.import.mock.calls[0][0] };
    await p.get("[data-testid=history-resume]").trigger("click");
    await flushPromises();
    expect(api.import.mock.calls[1][0]).toEqual(original);
  });
  it("preserves draft and unknown request across vertical sections and pushes", async () => {
    const p = await panel();
    await begin(p);
    await p
      .get(
        '[role=tab][aria-label="' + zh.backupHistory.sections.inventory + '"]',
      )
      .trigger("click");
    state.push?.();
    await flushPromises();
    await p
      .get('[role=tab][aria-label="' + zh.backupHistory.sections.import + '"]')
      .trigger("click");
    expect(p.get("[data-testid=history-frozen-id]").text()).toBe(key);
    expect(api.import).toHaveBeenCalledTimes(1);
  });
  it("does not poll and filters only on explicit selection", async () => {
    const p = await panel();
    const count = api.preview.mock.calls.length;
    await p.get("[data-testid=history-filter]").setValue("conflict");
    await flushPromises();
    expect(api.preview).toHaveBeenLastCalledWith(
      1,
      "conflict",
      expect.any(AbortSignal),
    );
    expect(api.preview).toHaveBeenCalledTimes(count + 1);
    expect(api.import).not.toHaveBeenCalled();
  });
  it("blocks stale preview submission while preserving the draft", async () => {
    const p = await panel();
    await p.get("[data-testid=history-open-import]").trigger("click");
    await p.get("textarea").setValue("keep my draft");
    await p.get("input").setValue(true);
    api.preview.mockRejectedValue(new Error("offline"));
    state.push?.();
    await flushPromises();
    expect(
      p.get("[data-testid=history-submit]").attributes("disabled"),
    ).toBeDefined();
    expect(p.get("textarea").element.value).toBe("keep my draft");
    expect(p.text()).toContain(zh.backupHistory.loadFailed);
  });
  it("finds an admitted import from independent history without a POST", async () => {
    api.recent.mockResolvedValue([receipt("pending")]);
    const p = await panel();
    expect(p.get("[data-testid=history-frozen-id]").text()).toBe(key);
    expect(api.import).not.toHaveBeenCalled();
  });
  it("aborts old stream and clears pending input on identity change", async () => {
    api.import.mockImplementation(() => new Promise(() => {}));
    const p = await panel();
    await begin(p);
    const signal = api.import.mock.calls[0][1];
    state.identity = "two";
    window.dispatchEvent(new Event("owned_identity_change"));
    await flushPromises();
    expect(signal.aborted).toBe(true);
    expect(p.find("[data-testid=history-frozen-id]").exists()).toBe(false);
    expect(p.get("textarea").element.value).toBe("");
  });
});
const frame = (event: string, v: any) =>
    `event: ${event}\ndata: ${JSON.stringify(v)}\n\n`,
  stream = (s: string) =>
    new ReadableStream<Uint8Array>({
      start(c) {
        c.enqueue(new TextEncoder().encode(s));
        c.close();
      },
    });
it("requires a bound terminal SSE receipt; EOF remains unknown", async () => {
  const input = {
      request_id: key,
      expected_snapshot: snapshot,
      reason: "copy old identity history",
      confirm: true as const,
    },
    signal = new AbortController().signal;
  expect(
    (
      await consumeHistoryStream(
        stream(
          frame("accepted", receipt("pending")) + frame("complete", receipt()),
        ),
        input,
        signal,
        () => {},
      )
    ).complete,
  ).toBe(true);
  for (const s of [
    frame("accepted", receipt("pending")),
    frame("complete", { ...receipt(), expected_snapshot: "c".repeat(64) }),
    frame("complete", receipt("pending")),
  ])
    await expect(
      consumeHistoryStream(stream(s), input, signal, () => {}),
    ).rejects.toThrow();
});
