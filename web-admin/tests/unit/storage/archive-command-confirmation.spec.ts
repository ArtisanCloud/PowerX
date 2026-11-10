import { mount, flushPromises } from "@vue/test-utils";
import { beforeEach, afterEach, expect, it, vi } from "vitest";
import { createI18n } from "vue-i18n";
import { webcrypto, createHash } from "node:crypto";
import zh from "../../../i18n/locales/zh.json";
import {
  readCommandConfirmationIntent,
  saveCommandConfirmationIntent,
  settleCommandConfirmation,
  commandConfirmationDigest,
  decodeCommandReview,
  decodeCommandReceipt,
} from "~/utils/archive-command-confirmation";
const api = vi.hoisted(() => ({
  review: vi.fn(),
  confirm: vi.fn(),
  request: vi.fn(),
}));
const auth = vi.hoisted(() => ({
  sessionId: "command-owner",
  tenantUuid: "tenant",
  status: "active",
}));
vi.mock("~/composables/api/services/archiveCommandConfirmationService", () => ({
  useArchiveCommandConfirmationService: () => api,
}));
vi.mock("~/utils/auth-session", () => ({
  AUTH_SESSION_EVENT: "owned-auth",
  readAuthSession: () => auth,
}));
import Dialog from "~/components/ops/backup/ArchiveCommandConfirmationDialog.vue";
const record = "11111111-1111-4111-8111-111111111111",
  execution = "22222222-2222-4222-8222-222222222222",
  command = "33333333-3333-4333-8333-333333333333",
  version = "a".repeat(64);
const review = () => ({
  record_id: record,
  job_id: record,
  execution_id: execution,
  command_id: command,
  kind: "pg_dump",
  state: "running",
  review_version: version,
  eligible: true,
  boot_matches: true,
  leader: "absent",
  group: "absent",
  read_only: true,
  observed_at: new Date().toISOString(),
});
const stubs = {
  UModal: {
    props: ["open"],
    template:
      '<section v-if="open"><slot name="body"/><slot name="footer"/></section>',
  },
  UAlert: {
    props: ["title", "description"],
    template: '<div>{{title}} {{description}}<slot name="actions"/></div>',
  },
  UButton: {
    props: ["disabled", "loading"],
    template: '<button :disabled="disabled||loading"><slot/></button>',
  },
  UFormField: { template: "<label><slot/></label>" },
  UTextarea: {
    props: ["modelValue", "disabled"],
    emits: ["update:modelValue"],
    template:
      '<textarea :disabled="disabled" :value="modelValue" @input="$emit(\'update:modelValue\',$event.target.value)"/>',
  },
  UCheckbox: {
    props: ["modelValue", "disabled"],
    emits: ["update:modelValue"],
    template:
      '<input type="checkbox" :disabled="disabled" :checked="modelValue" @change="$emit(\'update:modelValue\',$event.target.checked)"/>',
  },
  UIcon: true,
};
let w: any;
const boot = (open = true) =>
  mount(Dialog, {
    props: { open, recordId: record },
    global: {
      plugins: [createI18n({ legacy: false, locale: "zh", messages: { zh } })],
      stubs,
    },
  });
const tab = async (value: string) => {
  await w.get("[data-section-select]").setValue(value);
  await flushPromises();
};
const owner = () => ({
  sessionId: auth.sessionId,
  tenantUuid: auth.tenantUuid,
});
beforeEach(() => {
  vi.resetAllMocks();
  sessionStorage.clear();
  vi.stubGlobal("crypto", webcrypto);
  auth.sessionId = "command-owner";
  api.review.mockResolvedValue(review());
  w = boot();
});
afterEach(() => {
  w.unmount();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});
const prepare = async () => {
  await flushPromises();
  await tab("review");
  await w.get('[data-testid="command-confirmation-review"]').trigger("click");
  await flushPromises();
  await tab("confirm");
  await w.get("textarea").setValue("人工核对本机 <完整启动身份>");
  await w.get("input[type=checkbox]").setValue(true);
};
const receipt = (i: any) => ({
  request_id: i.request_id,
  record_id: i.record_id,
  job_id: i.job_id,
  execution_id: i.execution_id,
  command_id: i.command_id,
  state: "confirmed",
  input_digest: i.input_digest,
  expected_version: i.expected_version,
  action: "confirm_local_command_drain",
  reason: "核对后记录本机排空证明",
  operator: 1,
  at: new Date().toISOString(),
  current_record_matches: true,
});
it("separates read review from confirmation and preserves draft across task sections", async () => {
  await prepare();
  await tab("overview");
  await tab("confirm");
  expect(w.get("textarea").element.value).toContain("完整启动身份");
  expect(w.get("input").element.checked).toBe(true);
  expect(api.confirm).not.toHaveBeenCalled();
  expect(api.review).toHaveBeenCalledTimes(1);
});
it("holds original request after reply loss, 404, mismatched input and historical receipt, then settles only current exact proof", async () => {
  await prepare();
  api.confirm.mockRejectedValueOnce(new Error("lost successful response"));
  await w.get('[data-testid="command-confirmation-submit"]').trigger("click");
  await vi.waitFor(() => expect(api.confirm).toHaveBeenCalledTimes(1));
  await flushPromises();
  const i = readCommandConfirmationIntent(sessionStorage, owner())!;
  expect(i).toBeDefined();
  expect(JSON.stringify(i)).not.toContain("完整启动身份");
  expect(
    w.get('[data-testid="command-confirmation-submit"]').attributes("disabled"),
  ).toBeDefined();
  for (const v of [
    new Error("404"),
    { ...receipt(i), input_digest: "b".repeat(64) },
    { ...receipt(i), current_record_matches: false },
  ]) {
    if (v instanceof Error) api.request.mockRejectedValueOnce(v);
    else api.request.mockResolvedValueOnce(v);
    await w
      .get('[data-testid="command-confirmation-reconcile"]')
      .trigger("click");
    await flushPromises();
    expect(
      readCommandConfirmationIntent(sessionStorage, owner()),
    ).toBeDefined();
  }
  api.request.mockResolvedValueOnce(receipt(i));
  await w
    .get('[data-testid="command-confirmation-reconcile"]')
    .trigger("click");
  await flushPromises();
  expect(
    readCommandConfirmationIntent(sessionStorage, owner()),
  ).toBeUndefined();
  expect(w.get('[data-testid="command-confirmation-audit"]').text()).toContain(
    "本机排空证明",
  );
  expect(api.confirm).toHaveBeenCalledTimes(1);
});
it("restores original pending request after remount without timers or automatic posts", async () => {
  await prepare();
  api.confirm.mockRejectedValueOnce(new Error("lost"));
  await w.get('[data-testid="command-confirmation-submit"]').trigger("click");
  await vi.waitFor(() => expect(api.confirm).toHaveBeenCalledTimes(1));
  await flushPromises();
  w.unmount();
  w = boot(false);
  await flushPromises();
  expect(w.find('[data-testid="command-confirmation-pending"]').exists()).toBe(
    true,
  );
  vi.useFakeTimers();
  await vi.advanceTimersByTimeAsync(3600000);
  expect(api.request).not.toHaveBeenCalled();
  expect(api.confirm).toHaveBeenCalledTimes(1);
});
it("invalidates old review on push changes and retains reason and current task", async () => {
  await prepare();
  await w.setProps({ revision: 4, connectionStatus: "disconnected" });
  await flushPromises();
  expect(
    w.get('[data-testid="command-confirmation-submit"]').attributes("disabled"),
  ).toBeDefined();
  expect(w.get("textarea").element.value).toContain("完整启动身份");
  expect(
    w.find('[data-testid="command-confirmation-disconnected"]').exists(),
  ).toBe(true);
});
it("never confirms an ineligible legacy or active identity", async () => {
  api.review.mockResolvedValueOnce({
    ...review(),
    eligible: false,
    problem: "identity_missing",
    boot_matches: false,
    leader: "identity_missing",
    group: "unknown",
  });
  await prepare();
  expect(
    w.get('[data-testid="command-confirmation-submit"]').attributes("disabled"),
  ).toBeDefined();
  await w.get('[data-testid="command-confirmation-submit"]').trigger("click");
  expect(api.confirm).not.toHaveBeenCalled();
});
it("ignores an old identity response and retains its original request", async () => {
  await prepare();
  let finish!: (v: any) => void;
  api.confirm.mockImplementationOnce(() => new Promise((r) => (finish = r)));
  await w.get('[data-testid="command-confirmation-submit"]').trigger("click");
  await vi.waitFor(() => expect(api.confirm).toHaveBeenCalledTimes(1));
  await flushPromises();
  const old = owner(),
    i = readCommandConfirmationIntent(sessionStorage, old)!;
  auth.sessionId = "different";
  window.dispatchEvent(new Event("owned-auth"));
  finish(receipt(i));
  await flushPromises();
  expect(readCommandConfirmationIntent(sessionStorage, old)).toBeDefined();
  expect(
    readCommandConfirmationIntent(sessionStorage, owner()),
  ).toBeUndefined();
});
it("keeps corrupt session context visible and prevents a new mutation", async () => {
  w.unmount();
  sessionStorage.setItem(
    "powerx.archive-command-confirmation.v1:" +
      JSON.stringify([auth.sessionId, auth.tenantUuid]),
    "broken",
  );
  w = boot(false);
  await flushPromises();
  expect(w.find('[data-testid="command-confirmation-context"]').exists()).toBe(
    true,
  );
  await tab("confirm");
  expect(
    w.get('[data-testid="command-confirmation-submit"]').attributes("disabled"),
  ).toBeDefined();
  expect(api.confirm).not.toHaveBeenCalled();
});
it("rejects cached replacement and receipt execution mismatch", () => {
  const i: any = {
    version: 1,
    owner: owner(),
    request_id: crypto.randomUUID(),
    record_id: record,
    job_id: record,
    execution_id: execution,
    command_id: command,
    expected_version: version,
    input_digest: "b".repeat(64),
    created_at: new Date().toISOString(),
  };
  saveCommandConfirmationIntent(sessionStorage, i);
  expect(() => saveCommandConfirmationIntent(sessionStorage, i)).toThrow();
  expect(() =>
    settleCommandConfirmation(sessionStorage, i, {
      ...receipt(i),
      execution_id: record,
    } as any),
  ).toThrow();
});
it("matches Go JSON HTML and unicode escaping for immutable input digest", async () => {
  const input: any = {
    request_id: record,
    expected_version: version,
    execution_id: execution,
    command_id: command,
    action: "confirm_local_command_drain",
    reason: "核对 <>&\u2028\u2029",
    confirm: true,
  };
  const json = JSON.stringify([record, input]).replace(
    /[<>&\u2028\u2029]/g,
    (c) => "\\u" + c.charCodeAt(0).toString(16).padStart(4, "0"),
  );
  expect(await commandConfirmationDigest(record, input)).toBe(
    createHash("sha256").update(json).digest("hex"),
  );
});
it("rejects impossible eligible response and incomplete receipt", () => {
  expect(() =>
    decodeCommandReview({ ...review(), leader: "matching" }),
  ).toThrow();
  expect(() => decodeCommandReceipt({ state: "confirmed" })).toThrow();
});

it("settles a matching pre-write rejection without granting drain or replaying", async () => {
  await prepare();
  api.confirm.mockRejectedValueOnce(new Error("control unavailable"));
  await w.get('[data-testid="command-confirmation-submit"]').trigger("click");
  await vi.waitFor(() => expect(api.confirm).toHaveBeenCalledTimes(1));
  await flushPromises();
  const i = readCommandConfirmationIntent(sessionStorage, owner())!;
  api.request.mockResolvedValueOnce({
    ...receipt(i),
    state: "rejected",
    current_record_matches: false,
  });
  await w
    .get('[data-testid="command-confirmation-reconcile"]')
    .trigger("click");
  await flushPromises();
  expect(
    readCommandConfirmationIntent(sessionStorage, owner()),
  ).toBeUndefined();
  expect(w.get('[data-testid="command-confirmation-result"]').text()).toContain(
    "未接纳",
  );
  expect(api.confirm).toHaveBeenCalledTimes(1);
});

const target = "44444444-4444-4444-8444-444444444444";
const probeReview = () => ({
  ...review(),
  subject: "probe",
  job_id: "",
  target_id: target,
  kind: "ssh",
  probe: {
    target_id: target,
    name: "原探测目标",
    original_revision: 2,
    current_revision: 3,
    target_state: "changed",
    digest: version,
    policies: [{ id: record, name: "相关暂停策略", revision: 1, paused: true }],
  },
});
it("binds probe target in input and minimal intent, restores after lost reply and refuses a different target receipt", async () => {
  api.review.mockResolvedValueOnce(probeReview());
  await prepare();
  await tab("review");
  expect(w.get('[data-testid="command-probe-scope"]').text()).toContain(
    "原探测目标",
  );
  expect(w.text()).toContain("不验证当前目标");
  await tab("confirm");
  api.confirm.mockRejectedValueOnce(new Error("reply lost"));
  await w.get('[data-testid="command-confirmation-submit"]').trigger("click");
  await vi.waitFor(() => expect(api.confirm).toHaveBeenCalledTimes(1));
  await flushPromises();
  const i = readCommandConfirmationIntent(sessionStorage, owner())!;
  expect(i.target_id).toBe(target);
  expect(i.job_id).toBe("");
  expect(api.confirm.mock.calls[0][1].target_id).toBe(target);
  w.unmount();
  w = boot(false);
  await flushPromises();
  api.request.mockResolvedValueOnce({ ...receipt(i), target_id: record });
  await w
    .get('[data-testid="command-confirmation-reconcile"]')
    .trigger("click");
  await flushPromises();
  expect(readCommandConfirmationIntent(sessionStorage, owner())).toBeDefined();
  api.request.mockResolvedValueOnce({ ...receipt(i), target_id: target });
  await w
    .get('[data-testid="command-confirmation-reconcile"]')
    .trigger("click");
  await flushPromises();
  expect(
    readCommandConfirmationIntent(sessionStorage, owner()),
  ).toBeUndefined();
  expect(api.confirm).toHaveBeenCalledTimes(1);
});
it("decodes legacy probe as ineligible, rejects scope swapping and an active referenced policy", () => {
  expect(
    decodeCommandReview({
      ...probeReview(),
      probe: undefined,
      target_id: undefined,
      eligible: false,
      problem: "probe_scope_missing",
    }).eligible,
  ).toBe(false);
  expect(() =>
    decodeCommandReview({
      ...probeReview(),
      probe: { ...probeReview().probe, target_id: record },
    }),
  ).toThrow();
  expect(() =>
    decodeCommandReview({
      ...probeReview(),
      probe: {
        ...probeReview().probe,
        policies: [{ id: record, name: "active", revision: 1, paused: false }],
      },
    }),
  ).toThrow();
  expect(() =>
    decodeCommandReview({ ...probeReview(), target_id: undefined }),
  ).toThrow();
});
it("preserves original job digest and appends target scope in Go field order", async () => {
  const input: any = {
    request_id: record,
    expected_version: version,
    execution_id: execution,
    command_id: command,
    action: "confirm_local_command_drain",
    reason: "原目标范围核对",
    confirm: true,
    target_id: target,
  };
  expect(await commandConfirmationDigest(record, input)).toBe(
    createHash("sha256")
      .update(JSON.stringify([record, input]))
      .digest("hex"),
  );
});
