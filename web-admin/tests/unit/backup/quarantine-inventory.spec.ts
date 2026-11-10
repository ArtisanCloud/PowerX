import { afterEach, beforeEach, describe, it, expect, vi } from "vitest";
import { mount, flushPromises } from "@vue/test-utils";
import { createI18n } from "vue-i18n";
import Panel from "~/components/ops/backup/BackupQuarantineInventory.vue";
import {
  decodeQuarantineInventory,
  consumeQuarantineInspection,
} from "~/composables/api/services/quarantineInventoryService";
import zh from "../../../i18n/locales/zh.json";
const api = vi.hoisted(() => ({
  list: vi.fn(),
  request: vi.fn(),
  inspect: vi.fn(),
}));
const state = vi.hoisted(() => ({
  identity: "one",
  push: undefined as undefined | (() => Promise<void>),
}));
vi.mock(
  "~/composables/api/services/quarantineInventoryService",
  async (original) => ({
    ...(await original<any>()),
    useQuarantineInventoryService: () => api,
  }),
);
vi.mock("~/composables/useBackupEvents", () => ({
  useBackupEvents: (_r: string, fn: () => Promise<void>) => {
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
  snapshot = "a".repeat(64),
  date = "2026-11-09T01:00:00Z";
const row = () => ({
  request_id: key,
  artifact_id: "1",
  job_id: "2",
  policy_id: "3",
  token,
  result: "quarantined",
  diagnosis: "missing",
  reason: "owned historical quarantine reason",
  actor_id: "1",
  created_at: date,
  completed_at: date,
  history_status: "covered",
  current: "quarantined",
  cleanup_state: "quarantined",
  size_bytes: 17,
  can_inspect: true,
  snapshot,
  valid_backup_proof: false as const,
});
const page = () => ({
  items: [row()],
  page: 1,
  page_size: 20,
  total: 1,
  unattributed_quarantined_artifacts: 0,
  observed_at: date,
});
const verification = (state = "missing") => ({
  artifact_id: "1",
  token,
  state,
  bytes_read: state === "verified" ? 17 : 0,
  total_bytes: 17,
  dump_matched: state === "verified",
  content_verified: state === "verified",
  read_only: true,
  cleanup_unchanged: true,
  observed_at: date,
});
const result = () => ({
  request_id: key,
  snapshot,
  verification: verification("verified"),
  currently_quarantined: true,
  read_only: true,
  quarantine_unchanged: true,
  valid_backup_proof: false,
});
const stubs = {
  UIcon: true,
  UAlert: {
    props: ["title", "description"],
    template: "<div>{{title}} {{description}}</div>",
  },
  UBadge: { template: "<span><slot/></span>" },
  UButton: {
    props: ["disabled", "loading", "type"],
    template:
      '<button :type="type||\'button\'" :disabled="disabled||loading"><slot/></button>',
  },
  UFormField: { template: "<label><slot/></label>" },
  UInput: {
    props: ["modelValue"],
    emits: ["update:modelValue"],
    template:
      '<input :value="modelValue" @input="$emit(\'update:modelValue\',$event.target.value)"/>',
  },
  USelect: {
    props: ["modelValue", "items"],
    emits: ["update:modelValue"],
    template:
      '<select :value="modelValue" @change="$emit(\'update:modelValue\',$event.target.value)"><option v-for="i in items" :value="i.value">{{i.label}}</option></select>',
  },
  UModal: {
    props: ["open"],
    emits: ["update:open"],
    template: '<div v-if="open"><slot name="body"/><slot name="footer"/></div>',
  },
};
let wrapper: ReturnType<typeof mount> | undefined;
const panel = async () => {
  wrapper = mount(Panel, {
    global: {
      stubs,
      plugins: [createI18n({ legacy: false, locale: "zh", messages: { zh } })],
    },
  });
  await flushPromises();
  return wrapper;
};
const detail = async (p: any) => {
  await p.get("[data-inventory-id]").trigger("click");
  await flushPromises();
};
const inspection = async (p: any) => {
  await detail(p);
  await p
    .findAll("[role=tab]")
    .find((b: any) => b.text() === zh.quarantineInventory.inspectionTitle)!
    .trigger("click");
  await p.get("[data-testid=inventory-inspect]").trigger("click");
  await flushPromises();
};
beforeEach(() => {
  vi.clearAllMocks();
  state.identity = "one";
  api.list.mockResolvedValue(page());
  api.request.mockResolvedValue(row());
  api.inspect.mockResolvedValue(result());
});
afterEach(() => {
  wrapper?.unmount();
  wrapper = undefined;
  vi.useRealTimers();
});
describe("readonly quarantine inventory", () => {
  it("strictly separates historical and current state without restore authority", () => {
    expect(decodeQuarantineInventory(page()).items[0].valid_backup_proof).toBe(
      false,
    );
    for (const patch of [
      { valid_backup_proof: true },
      { can_inspect: true, snapshot: "" },
      { current: "missing", can_inspect: true },
      { history_status: "conflict", can_inspect: true },
      { completed_at: null },
      { token: "bad" },
    ])
      expect(() =>
        decodeQuarantineInventory({
          ...page(),
          items: [{ ...row(), ...patch }],
        }),
      ).toThrow();
    expect(() =>
      decodeQuarantineInventory({ ...page(), total: 4097 }),
    ).toThrow();
  });
  it("never scans on mount, navigation, push or elapsed time", async () => {
    const p = await panel();
    await detail(p);
    await state.push?.();
    expect(api.inspect).not.toHaveBeenCalled();
    vi.useFakeTimers();
    await vi.advanceTimersByTimeAsync(60000);
    expect(api.list).toHaveBeenCalledTimes(2);
    expect(api.inspect).not.toHaveBeenCalled();
  });
  it("applies filters to the server and preserves an unapplied draft across push", async () => {
    const p = await panel();
    await p.get("[data-testid=inventory-query]").setValue(key);
    await state.push?.();
    expect(api.list.mock.calls.at(-1)?.[0].q).toBe("");
    expect(p.get("[data-testid=inventory-query]").element.value).toBe(key);
    await p.get("form").trigger("submit");
    await flushPromises();
    expect(api.list.mock.calls.at(-1)?.[0]).toMatchObject({ q: key, page: 1 });
  });
  it("submits current-state selection and sort direction while keeping unapplied changes separate", async () => {
    const p = await panel();
    await p
      .get("[data-testid=inventory-current-filter]")
      .setValue("not_quarantined");
    await p
      .findAll("button")
      .find((b) => b.text() === zh.quarantineInventory.desc)!
      .trigger("click");
    expect(api.list).toHaveBeenCalledTimes(1);
    await p.get("form").trigger("submit");
    await flushPromises();
    expect(api.list.mock.calls.at(-1)?.[0]).toMatchObject({
      current: "not_quarantined",
      direction: "asc",
    });
    await (p.vm as any).refresh();
    expect(api.inspect).not.toHaveBeenCalled();
  });

  it("preserves prior results and accepted page on failed filters", async () => {
    const p = await panel();
    await p.get("[data-testid=inventory-query]").setValue("new query");
    api.list.mockRejectedValue(new Error("private SQL detail"));
    await p.get("form").trigger("submit");
    await flushPromises();
    expect(p.find("[data-inventory-id]").exists()).toBe(true);
    expect(p.text()).toContain(zh.quarantineInventory.loadFailed);
    expect(p.text()).not.toContain("private SQL");
    expect(p.get("[data-testid=inventory-query]").element.value).toBe(
      "new query",
    );
  });
  it("shows historical success and a restored current marker separately", async () => {
    api.list.mockResolvedValue({
      ...page(),
      items: [
        {
          ...row(),
          current: "not_quarantined",
          cleanup_state: "unconfirmed",
          history_status: "sql_replayed",
        },
      ],
    });
    api.request.mockResolvedValue({
      ...row(),
      current: "not_quarantined",
      cleanup_state: "unconfirmed",
      history_status: "sql_replayed",
    });
    const p = await panel();
    await detail(p);
    expect(p.text()).toContain(zh.quarantineInventory.results.quarantined);
    expect(p.text()).toContain(zh.quarantineInventory.current.not_quarantined);
    expect(p.text()).toContain(zh.quarantineInventory.history.sql_replayed);
    expect(api.inspect).not.toHaveBeenCalled();
  });
  it("renders explicit orphan-marker coverage warning without claiming full coverage", async () => {
    api.list.mockResolvedValue({
      ...page(),
      unattributed_quarantined_artifacts: 2,
    });
    const p = await panel();
    expect(p.text()).toContain("台账覆盖不完整");
  });
  it("completes only an explicit check and retains the quarantine and no-restore explanation", async () => {
    const p = await panel();
    await inspection(p);
    expect(api.inspect).toHaveBeenCalledTimes(1);
    expect(p.get("[data-testid=inventory-inspection-result]").text()).toContain(
      zh.quarantineInventory.noRestoreProof,
    );
    expect(p.text()).toContain(zh.quarantineInventory.current.quarantined);
    await state.push?.();
    expect(api.inspect).toHaveBeenCalledTimes(1);
  });
  it("preserves running inspection through unchanged push and tab switches", async () => {
    api.inspect.mockImplementation(() => new Promise(() => {}));
    const p = await panel();
    await inspection(p);
    const signal = api.inspect.mock.calls[0][1];
    await state.push?.();
    await p
      .findAll("[role=tab]")
      .find((b: any) => b.text() === zh.quarantineInventory.historyTab)!
      .trigger("click");
    expect(signal.aborted).toBe(false);
    expect(api.inspect).toHaveBeenCalledTimes(1);
  });
  it("invalidates a running check when current metadata changes through push", async () => {
    api.inspect.mockImplementation(() => new Promise(() => {}));
    const p = await panel();
    await inspection(p);
    const signal = api.inspect.mock.calls[0][1];
    api.request.mockResolvedValue({ ...row(), snapshot: "b".repeat(64) });
    await state.push?.();
    expect(signal.aborted).toBe(true);
    expect(p.text()).toContain(zh.quarantineInventory.incomplete);
    expect(api.inspect).toHaveBeenCalledTimes(1);
  });
  it("blocks diagnosis after detail read fails, retaining historical context", async () => {
    api.request.mockRejectedValue(new Error("offline"));
    const p = await panel();
    await detail(p);
    expect(p.get("[data-testid=inventory-request-id]").text()).toBe(key);
    await p
      .findAll("[role=tab]")
      .find((b: any) => b.text() === zh.quarantineInventory.inspectionTitle)!
      .trigger("click");
    expect(
      p.get("[data-testid=inventory-inspect]").attributes("disabled"),
    ).toBeDefined();
  });
  it("aborts owned requests and clears old identity details", async () => {
    api.inspect.mockImplementation(() => new Promise(() => {}));
    const p = await panel();
    await inspection(p);
    const signal = api.inspect.mock.calls[0][1];
    state.identity = "two";
    window.dispatchEvent(new Event("owned_identity"));
    await flushPromises();
    expect(signal.aborted).toBe(true);
    expect(p.find("[data-testid=inventory-detail]").exists()).toBe(false);
  });
  it("aborts an inspection on close and unmount", async () => {
    api.inspect.mockImplementation(() => new Promise(() => {}));
    const p = await panel();
    await inspection(p);
    const signal = api.inspect.mock.calls[0][1];
    await p
      .findAll("button")
      .find((b: any) => b.text() === zh.backupReview.close)!
      .trigger("click");
    await flushPromises();
    expect(signal.aborted).toBe(true);
    await inspection(p);
    const signal2 = api.inspect.mock.calls.at(-1)?.[1];
    p.unmount();
    wrapper = undefined;
    expect(signal2.aborted).toBe(true);
  });
});
const frame = (event: string, data: any) =>
    `event: ${event}\ndata: ${JSON.stringify(data)}\n\n`,
  stream = (text: string) =>
    new ReadableStream<Uint8Array>({
      start(c) {
        c.enqueue(new TextEncoder().encode(text));
        c.close();
      },
    });
it("binds SSE completion to request, token, snapshot and immutable quarantine; EOF never succeeds", async () => {
  const signal = new AbortController().signal;
  expect(
    (
      await consumeQuarantineInspection(
        stream(frame("complete", result())),
        row(),
        signal,
        () => {},
      )
    ).valid_backup_proof,
  ).toBe(false);
  for (const text of [
    frame("progress", verification("running")),
    frame("incomplete", {}),
    frame("complete", { ...result(), valid_backup_proof: true }),
    frame("complete", { ...result(), snapshot: "b".repeat(64) }),
    frame("complete", { ...result(), currently_quarantined: false }),
    frame("complete", {
      ...result(),
      verification: { ...verification(), token: key },
    }),
  ])
    await expect(
      consumeQuarantineInspection(stream(text), row(), signal, () => {}),
    ).rejects.toThrow();
});
