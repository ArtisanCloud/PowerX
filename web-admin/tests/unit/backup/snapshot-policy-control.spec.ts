import { afterEach, beforeEach, describe, it, expect, vi } from "vitest";
import { mount, flushPromises } from "@vue/test-utils";
import { ref } from "vue";
import { createI18n } from "vue-i18n";
import zh from "../../../i18n/locales/zh.json";
import Panel from "~/components/ops/backup/SystemSnapshotPolicies.vue";
import {
  decodeSnapshotPolicyConfiguration,
  decodeSnapshotPolicyMutation,
} from "~/composables/api/services/snapshotPolicyService";
const api = vi.hoisted(() => ({
  configuration: vi.fn(),
  change: vi.fn(),
  adopt: vi.fn(),
  request: vi.fn(),
}));
const push = vi.hoisted(() => ({
  callback: undefined as undefined | (() => void),
}));
vi.mock(
  "~/composables/api/services/snapshotPolicyService",
  async (original) => ({
    ...(await original<any>()),
    useSnapshotPolicyService: () => api,
  }),
);
vi.mock("~/composables/useBackupEvents", () => ({
  useBackupEvents: (_resource: string, callback: () => void) => {
    push.callback = callback;
    return { status: ref("connected") };
  },
}));
const stubs = {
  UAlert: {
    props: ["title", "description"],
    template: "<div>{{title}} {{description}}</div>",
  },
  UBadge: { template: "<span><slot/></span>" },
  UButton: {
    props: ["disabled", "loading"],
    template: '<button :disabled="disabled || loading"><slot/></button>',
  },
  UModal: {
    props: ["open"],
    emits: ["update:open"],
    template:
      '<section v-if="open"><button data-close @click="$emit(\'update:open\',false)">Close</button><slot name="body"/><slot name="footer"/></section>',
  },
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
      '<select :value="modelValue" @change="$emit(\'update:modelValue\',$event.target.value)"><option v-for="item in items" :value="item.value">{{item.label}}</option></select>',
  },
  UCheckbox: {
    props: ["modelValue", "label"],
    emits: ["update:modelValue"],
    template:
      '<label><input type="checkbox" :checked="modelValue" @change="$emit(\'update:modelValue\',$event.target.checked)"/>{{label}}</label>',
  },
  UFormField: { template: "<label><slot/></label>" },
  UForm: {
    emits: ["submit"],
    template: "<form @submit.prevent=\"$emit('submit')\"><slot/></form>",
  },
};
const id = "0d2d2cc3-53e0-498d-97bc-0a3a2ea9955c";
const fixture = () => ({
  authority: "database",
  state: "applied",
  ready: true,
  desired_revision: 2,
  applied_revision: 2,
  snapshot_revision: 2,
  legacy_digest: "a".repeat(64),
  execution_ready: { [id]: false },
  policies: [
    {
      id: id,
      revision: 1,
      fingerprint: "b".repeat(64),
      enabled: false,
      current: true,
      settings: {
        name: "每日备份",
        interval_minutes: 60,
        retention_count: 3,
        retention_days: 7,
      },
    },
  ],
});
const clone = (v: any) => JSON.parse(JSON.stringify(v));
let current: any;
const views: ReturnType<typeof mount>[] = [];
async function view() {
  const w = mount(Panel, {
    global: {
      stubs,
      plugins: [createI18n({ legacy: false, locale: "zh", messages: { zh } })],
    },
  });
  views.push(w);
  await flushPromises();
  return w;
}
async function edit(w: any) {
  await w.get("[data-testid=snapshot-policy-edit]").trigger("click");
}
async function save(w: any) {
  await w.get("form").trigger("submit");
  await flushPromises();
}
beforeEach(() => {
  vi.resetAllMocks();
  current = fixture();
  api.configuration.mockImplementation(async () => clone(current));
});
afterEach(() => {
  views.splice(0).forEach((w) => w.unmount());
  vi.useRealTimers();
});
describe("system snapshot policy authority", () => {
  it("preserves row and catalog draft revisions across SSE, closure and conflict", async () => {
    const w = await view();
    await edit(w);
    await w.get("[data-testid=snapshot-policy-name]").setValue("我的草稿");
    await w.get("[data-close]").trigger("click");
    current.desired_revision = 3;
    current.policies[0].revision = 2;
    current.policies[0].settings.name = "other";
    push.callback?.();
    await flushPromises();
    await edit(w);
    expect(w.get("[data-testid=snapshot-policy-name]").element.value).toBe(
      "我的草稿",
    );
    api.change.mockRejectedValue({ status: 409, message: "conflict" });
    await save(w);
    expect(api.change.mock.calls[0][0]).toMatchObject({
      expected_revision: 2,
      expected_policy_revision: 1,
      settings: { name: "我的草稿" },
    });
    expect(w.find("[data-testid=snapshot-policy-unknown]").exists()).toBe(
      false,
    );
    await w.get("[data-testid=snapshot-policy-latest]").trigger("click");
    await flushPromises();
    expect(w.get("[data-testid=snapshot-policy-name]").element.value).toBe(
      "other",
    );
  });
  it("keeps lost writes query-only through a missing receipt and reconciles original UUID", async () => {
    const w = await view();
    await edit(w);
    api.change.mockRejectedValue({ status: 502, message: "lost" });
    await save(w);
    const sent = api.change.mock.calls[0][0];
    expect(w.find("[data-testid=snapshot-policy-unknown]").exists()).toBe(true);
    api.request.mockRejectedValue({ status: 404 });
    await w
      .get("[data-testid=snapshot-policy-reconcile-editor]")
      .trigger("click");
    await flushPromises();
    expect(w.find("[data-testid=snapshot-policy-unknown]").exists()).toBe(true);
    api.request.mockResolvedValue({
      request_id: sent.request_id,
      operation_revision: 3,
      policy_id: id,
      configuration: { ...current, state: "pending", desired_revision: 3 },
    });
    await w
      .get("[data-testid=snapshot-policy-reconcile-editor]")
      .trigger("click");
    await flushPromises();
    expect(api.change).toHaveBeenCalledTimes(1);
    expect(api.request.mock.calls[0][0]).toBe(sent.request_id);
    expect(w.find("[data-testid=snapshot-policy-unknown]").exists()).toBe(
      false,
    );
  });
  it("requires adoption and keeps new definitions paused until separately enabled", async () => {
    current = {
      ...fixture(),
      authority: "legacy",
      state: "legacy",
      desired_revision: 0,
      applied_revision: 0,
      snapshot_revision: null,
    };
    const w = await view();
    expect(
      w.get("[data-testid=snapshot-policy-adopt]").attributes("disabled"),
    ).toBeDefined();
    api.adopt.mockImplementation(async (i: any) => ({
      request_id: i.request_id,
      operation_revision: 1,
      policy_id: "",
      configuration: {
        ...fixture(),
        desired_revision: 1,
        applied_revision: 0,
        snapshot_revision: null,
        state: "pending",
      },
    }));
    await w
      .get("[data-testid=snapshot-policy-adopt-confirm] input")
      .setValue(true);
    await w.get("[data-testid=snapshot-policy-adopt]").trigger("click");
    await flushPromises();
    expect(api.adopt).toHaveBeenCalledTimes(1);
    await edit(w);
    api.change.mockImplementation(async (i: any) => ({
      request_id: i.request_id,
      operation_revision: 2,
      policy_id: id,
      configuration: { ...fixture(), state: "pending" },
    }));
    await save(w);
    expect(api.change.mock.calls[0][0]).toMatchObject({
      action: "save",
      confirm: true,
      settings: { interval_minutes: 60 },
    });
    expect(api.change.mock.calls[0][0].enabled).toBeUndefined();
  });
  it("captures exact status revisions and requires separate confirmation", async () => {
    const w = await view();
    await w.get("[data-testid=snapshot-policy-toggle]").trigger("click");
    await w.get("[data-testid=snapshot-policy-status-save]").trigger("click");
    expect(api.change).not.toHaveBeenCalled();
    current.desired_revision = 9;
    push.callback?.();
    await flushPromises();
    await w
      .get("[data-testid=snapshot-policy-status-confirm] input")
      .setValue(true);
    api.change.mockRejectedValue({ status: 409, message: "conflict" });
    await w.get("[data-testid=snapshot-policy-status-save]").trigger("click");
    await flushPromises();
    expect(api.change.mock.calls[0][0]).toMatchObject({
      action: "status",
      enabled: true,
      expected_revision: 2,
      expected_policy_revision: 1,
    });
  });
  it("does not poll while idle and aborts pending reads on unmount", async () => {
    const w = await view();
    vi.useFakeTimers();
    await vi.advanceTimersByTimeAsync(60000);
    expect(api.configuration).toHaveBeenCalledTimes(1);
    const signal = api.configuration.mock.calls[0][0];
    w.unmount();
    expect(signal.aborted).toBe(true);
  });
  it("blocks old backends and invalid typed responses", async () => {
    api.configuration.mockRejectedValue({ status: 404 });
    const w = await view();
    expect(
      w.get("[data-testid=snapshot-policy-new]").attributes("disabled"),
    ).toBeDefined();
    expect(() =>
      decodeSnapshotPolicyConfiguration({ ...fixture(), execution_ready: [] }),
    ).toThrow();
    expect(() =>
      decodeSnapshotPolicyMutation({
        request_id: "bad",
        operation_revision: 1,
        policy_id: id,
        configuration: fixture(),
      }),
    ).toThrow();
  });
});
