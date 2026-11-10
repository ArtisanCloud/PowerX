import { mount, flushPromises } from "@vue/test-utils";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { createI18n } from "vue-i18n";
import { webcrypto } from "node:crypto";
import zh from "../../../i18n/locales/zh.json";
import {
  saveDispositionIntent,
  readDispositionIntent,
  settleDisposition,
  dispositionDigest,
  consumeInterruptionReview,
} from "~/utils/archive-disposition";
const api = vi.hoisted(() => ({
  review: vi.fn(),
  retain: vi.fn(),
  request: vi.fn(),
  requestReview: vi.fn(),
  closeRequest: vi.fn(),
}));
const archives = vi.hoisted(() => ({ job: vi.fn() }));
const auth = vi.hoisted(() => ({
  sessionId: "disposition-owned",
  tenantUuid: "tenant",
  status: "active",
}));
vi.mock("~/composables/api/services/archiveDispositionService", () => ({
  useArchiveDispositionService: () => api,
}));
vi.mock("~/composables/api/services/archiveOpsService", () => ({
  useArchiveOpsService: () => archives,
}));
vi.mock("~/utils/auth-session", () => ({
  AUTH_SESSION_EVENT: "owned-auth",
  readAuthSession: () => auth,
}));
import Dialog from "~/components/ops/backup/ArchiveInterruptionDialog.vue";
const job = "11111111-1111-4111-8111-111111111111",
  policy = "22222222-2222-4222-8222-222222222222",
  version = "a".repeat(64);
const review = () => ({
  job_id: job,
  policy_id: policy,
  policy_revision: 2,
  review_version: version,
  object_count: 1,
  bytes: 10,
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
      props: { open, jobId: job, closureAvailable: true },
      global: {
        plugins: [
          createI18n({ legacy: false, locale: "zh", messages: { zh } }),
        ],
        stubs,
      },
    }),
  tab = async (v: string) => {
    await w.get("[data-section-select]").setValue(v);
    await flushPromises();
  };
beforeEach(() => {
  vi.resetAllMocks();
  sessionStorage.clear();
  vi.stubGlobal("crypto", webcrypto);
  auth.sessionId = "disposition-owned";
  archives.job.mockResolvedValue({
    id: job,
    source_id: "owned",
    status: "interrupted",
    interruption_pending: true,
  });
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
  await tab("verification");
  await w.get('[data-testid="archive-disposition-verify"]').trigger("click");
  await flushPromises();
  await tab("confirm");
  await w.get("textarea").setValue("保留中断内容 <原任务>");
  await w.get("input[type=checkbox]").setValue(true);
};
const receipt = (i: any) => ({
  request_id: i.request_id,
  job_id: i.job_id,
  state: "retained",
  input_digest: i.input_digest,
  expected_version: i.expected_version,
  action: "retain_and_close",
  reason: "reviewed",
  operator: 1,
  created_at: new Date().toISOString(),
  current_record_matches: true,
});
it("verifies and confirms separately, preserving reason across tabs without mutations", async () => {
  await prepare();
  expect(api.retain).not.toHaveBeenCalled();
  await tab("overview");
  await tab("confirm");
  expect(w.get("textarea").element.value).toContain("原任务");
  expect(w.get("input").element.checked).toBe(true);
  expect(api.review).toHaveBeenCalledTimes(1);
});
it("retains original UUID on lost response, 404 or rolled-back history; reconciles without replay", async () => {
  await prepare();
  api.retain.mockRejectedValueOnce(new Error("reply lost"));
  await w.get('[data-testid="archive-disposition-submit"]').trigger("click");
  await vi.waitFor(() => expect(api.retain).toHaveBeenCalledTimes(1));
  await flushPromises();
  const owner = { sessionId: auth.sessionId, tenantUuid: auth.tenantUuid },
    i = readDispositionIntent(sessionStorage, owner)!;
  expect(i).toBeDefined();
  expect(JSON.stringify(i)).not.toContain("保留中断内容");
  api.request.mockRejectedValueOnce(new Error("404"));
  await w.get('[data-testid="archive-disposition-reconcile"]').trigger("click");
  await flushPromises();
  expect(readDispositionIntent(sessionStorage, owner)?.request_id).toBe(
    i.request_id,
  );
  api.request.mockResolvedValueOnce({
    ...receipt(i),
    current_record_matches: false,
  });
  await w.get('[data-testid="archive-disposition-reconcile"]').trigger("click");
  await flushPromises();
  expect(readDispositionIntent(sessionStorage, owner)).toBeDefined();
  api.request.mockResolvedValueOnce(receipt(i));
  await w.get('[data-testid="archive-disposition-reconcile"]').trigger("click");
  await flushPromises();
  expect(readDispositionIntent(sessionStorage, owner)).toBeUndefined();
  expect(api.retain).toHaveBeenCalledTimes(1);
});
it("restores pending intent after remount without timed queries", async () => {
  await prepare();
  api.retain.mockRejectedValueOnce(new Error("lost"));
  await w.get('[data-testid="archive-disposition-submit"]').trigger("click");
  await vi.waitFor(() => expect(api.retain).toHaveBeenCalledTimes(1));
  await flushPromises();
  w.unmount();
  w = boot(false);
  await flushPromises();
  expect(w.find('[data-testid="archive-disposition-pending"]').exists()).toBe(
    true,
  );
  vi.useFakeTimers();
  await vi.advanceTimersByTimeAsync(3600000);
  expect(api.retain).toHaveBeenCalledTimes(1);
  expect(api.request).not.toHaveBeenCalled();
  expect(api.review).toHaveBeenCalledTimes(1);
});
it("ignores an old-login response and retains that login original intent", async () => {
  await prepare();
  let finish!: (v: any) => void;
  api.retain.mockImplementationOnce(
    () =>
      new Promise((r) => {
        finish = r;
      }),
  );
  await w.get('[data-testid="archive-disposition-submit"]').trigger("click");
  await vi.waitFor(() => expect(api.retain).toHaveBeenCalledTimes(1));
  await flushPromises();
  const o = { sessionId: auth.sessionId, tenantUuid: auth.tenantUuid },
    i = readDispositionIntent(sessionStorage, o)!;
  auth.sessionId = "other";
  window.dispatchEvent(new Event("owned-auth"));
  finish(receipt(i));
  await flushPromises();
  expect(readDispositionIntent(sessionStorage, o)).toBeDefined();
  expect(
    readDispositionIntent(sessionStorage, {
      sessionId: "other",
      tenantUuid: "tenant",
    }),
  ).toBeUndefined();
});
it("rejects replacement cached input and mismatched receipts", () => {
  const o = { sessionId: "owned", tenantUuid: "t" },
    i: any = {
      version: 1,
      owner: o,
      request_id: crypto.randomUUID(),
      job_id: job,
      expected_version: version,
      input_digest: "b".repeat(64),
      created_at: new Date().toISOString(),
    };
  saveDispositionIntent(sessionStorage, i);
  expect(() =>
    settleDisposition(sessionStorage, i, {
      ...receipt(i),
      job_id: policy,
    } as any),
  ).toThrow();
  expect(() => saveDispositionIntent(sessionStorage, i)).toThrow();
});
it("requires a complete matching verification stream", async () => {
  const s = (raw: string) =>
    new ReadableStream({
      start(c) {
        c.enqueue(new TextEncoder().encode(raw));
        c.close();
      },
    });
  await expect(
    consumeInterruptionReview(
      s('event: progress\ndata: {"objects":1,"bytes":10}\n\n'),
      job,
      new AbortController().signal,
      () => {},
    ),
  ).rejects.toThrow();
  const raw = "event: complete\ndata: " + JSON.stringify(review()) + "\n\n";
  expect(
    (
      await consumeInterruptionReview(
        s(raw),
        job,
        new AbortController().signal,
        () => {},
      )
    ).review_version,
  ).toBe(version);
  await expect(
    consumeInterruptionReview(
      s(raw),
      policy,
      new AbortController().signal,
      () => {},
    ),
  ).rejects.toThrow();
});
it("matches Go canonical JSON escaping for human reasons", async () => {
  const input: any = {
    request_id: "33333333-3333-4333-8333-333333333333",
    expected_version: version,
    action: "retain_and_close",
    reason: "保留 <>& \u2028 原任务",
    confirm: true,
  };
  const text = JSON.stringify([job, input]).replace(
    /[<>&\u2028\u2029]/g,
    (c) => "\\u" + c.charCodeAt(0).toString(16).padStart(4, "0"),
  );
  const h = await webcrypto.subtle.digest(
    "SHA-256",
    new TextEncoder().encode(text),
  );
  expect(await dispositionDigest(job, input)).toBe(
    Buffer.from(h).toString("hex"),
  );
});

it("push invalidation preserves reason but requires a new content proof", async () => {
  await prepare();
  await w.setProps({ revision: 1 });
  await flushPromises();
  expect(w.get("textarea").element.value).toContain("原任务");
  expect(
    w.get('[data-testid="archive-disposition-submit"]').attributes("disabled"),
  ).toBeDefined();
  expect(api.retain).not.toHaveBeenCalled();
});

it("shows a corrupt saved context and blocks new mutation instead of hiding the error", async () => {
  w.unmount();
  sessionStorage.setItem(
    "powerx.archive-interruption.v1:" +
      JSON.stringify([auth.sessionId, auth.tenantUuid]),
    "{",
  );
  w = boot(false);
  await flushPromises();
  expect(w.get('[data-testid="archive-disposition-context"]').text()).toContain(
    "原请求",
  );
  expect(api.retain).not.toHaveBeenCalled();
});
it("displays the persisted audit of a retained capture without another verification or write", async () => {
  w.unmount();
  archives.job.mockResolvedValue({
    id: job,
    status: "retained",
    protected: true,
    capture_retention: {
      request_id: "44444444-4444-4444-8444-444444444444",
      reason: "已核验并保留原内容",
      operator: 11,
      at: new Date().toISOString(),
      object_count: 1,
      bytes: 10,
    },
  });
  w = boot();
  await flushPromises();
  expect(w.get('[data-testid="archive-retention-audit"]').text()).toContain(
    "已核验并保留原内容",
  );
  expect(api.review).not.toHaveBeenCalled();
  expect(api.retain).not.toHaveBeenCalled();
});

const prepareUnknown = async () => {
  await prepare();
  api.retain.mockRejectedValueOnce(new Error("lost original response"));
  await w.get('[data-testid="archive-disposition-submit"]').trigger("click");
  await vi.waitFor(() => expect(api.retain).toHaveBeenCalledTimes(1));
  await flushPromises();
  const owner = { sessionId: auth.sessionId, tenantUuid: auth.tenantUuid };
  const intent = readDispositionIntent(sessionStorage, owner)!;
  api.requestReview.mockResolvedValue({
    request_id: intent.request_id,
    job_id: intent.job_id,
    input_digest: intent.input_digest,
    review_version: version,
    registered_epoch_matches: false,
    interruption_pending: true,
    original_locks_available: true,
    read_only: true,
    policy_revision: 2,
    observed_at: new Date().toISOString(),
  });
  await tab("requestReview");
  await w.get('[data-testid="archive-closure-review"]').trigger("click");
  await flushPromises();
  await tab("requestClose");
  await w
    .get('[data-testid="archive-closure-reason"]')
    .setValue("历史结果无法确定，保留保护并人工结案");
  await w.get('[data-testid="archive-closure-confirm"]').setValue(true);
  return { owner, intent };
};
it("manually reviews unknown history, preserves closure draft across tabs, and never repeats retention", async () => {
  await prepareUnknown();
  await tab("overview");
  await tab("requestClose");
  expect(
    w.get('[data-testid="archive-closure-reason"]').element.value,
  ).toContain("历史结果");
  expect(w.get('[data-testid="archive-closure-confirm"]').element.checked).toBe(
    true,
  );
  expect(api.closeRequest).not.toHaveBeenCalled();
  expect(api.retain).toHaveBeenCalledTimes(1);
});
it("keeps closure intent after reply loss and remount, only original GET with matching current proof settles", async () => {
  const { owner, intent } = await prepareUnknown();
  api.closeRequest.mockRejectedValueOnce(new Error("closure reply lost"));
  await w.get('[data-testid="archive-closure-submit"]').trigger("click");
  await vi.waitFor(() => expect(api.closeRequest).toHaveBeenCalledTimes(1));
  await flushPromises();
  const saved = readDispositionIntent(sessionStorage, owner)!;
  expect(saved.closure).toBeDefined();
  expect(JSON.stringify(saved)).not.toContain("历史结果");
  w.unmount();
  w = boot(false);
  await flushPromises();
  await tab("requestClose");
  expect(
    w.get('[data-testid="archive-closure-submit"]').attributes("disabled"),
  ).toBeDefined();
  const response = {
    ...receipt(intent),
    state: "closed_unknown",
    current_record_matches: false,
    current_protection_matches: false,
    closure: {
      request_id: saved.closure!.request_id,
      input_digest: saved.closure!.input_digest,
      review_version: saved.closure!.expected_version,
    },
  };
  api.request.mockResolvedValueOnce(response);
  await w.get('[data-testid="archive-disposition-reconcile"]').trigger("click");
  await flushPromises();
  expect(readDispositionIntent(sessionStorage, owner)).toBeDefined();
  api.request.mockResolvedValueOnce({
    ...response,
    current_protection_matches: true,
    closure: { ...response.closure, input_digest: "f".repeat(64) },
  });
  await w.get('[data-testid="archive-disposition-reconcile"]').trigger("click");
  await flushPromises();
  expect(readDispositionIntent(sessionStorage, owner)).toBeDefined();
  api.request.mockResolvedValueOnce({
    ...response,
    current_protection_matches: true,
    closure: {
      ...response.closure,
      reason: "人工结案保留保护",
      operator: 1,
      at: new Date().toISOString(),
    },
  });
  await w.get('[data-testid="archive-disposition-reconcile"]').trigger("click");
  await flushPromises();
  expect(readDispositionIntent(sessionStorage, owner)).toBeUndefined();
  expect(w.get('[data-testid="archive-closure-audit"]').text()).toContain(
    "人工结案保留保护",
  );
  expect(api.closeRequest).toHaveBeenCalledTimes(1);
  expect(api.retain).toHaveBeenCalledTimes(1);
});
it("invalidates closure review on SSE changes and keeps reason; old Core cannot submit", async () => {
  await prepareUnknown();
  await w.setProps({ revision: 3 });
  await flushPromises();
  expect(
    w.get('[data-testid="archive-closure-submit"]').attributes("disabled"),
  ).toBeDefined();
  expect(
    w.get('[data-testid="archive-closure-reason"]').element.value,
  ).toContain("历史结果");
  await w.setProps({ closureAvailable: false });
  await tab("requestReview");
  expect(
    w.get('[data-testid="archive-closure-review"]').attributes("disabled"),
  ).toBeDefined();
  expect(api.closeRequest).not.toHaveBeenCalled();
});

it('blocks content verification when the original local command proof is unknown',async()=>{
 archives.job.mockResolvedValue({id:job,status:'interrupted',interruption_pending:true,source_id:'owned',command_drain_state:'unconfirmed'});
 await w.setProps({revision:22});await flushPromises();
 expect(w.get('[data-testid="archive-command-protection"]').text()).toContain('尚未确认退出');
 await tab('verification');expect(w.get('[data-testid="archive-disposition-verify"]').attributes('disabled')).toBeDefined();expect(api.review).not.toHaveBeenCalled();expect(api.retain).not.toHaveBeenCalled();
});
