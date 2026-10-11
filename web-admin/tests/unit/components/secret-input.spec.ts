import { mount } from "@vue/test-utils";
import { computed, defineComponent, reactive, ref, watch } from "vue";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import SecretInput from "~/components/SecretInput.vue";
import ProviderModelForm from "~/components/settings/ai/ProviderModelForm.vue";

const stubs = {
  UInput: {
    inheritAttrs: false,
    props: ["modelValue", "type", "disabled"],
    emits: ["update:modelValue"],
    template: `<div><input v-bind="$attrs" :type="type" :value="modelValue" :disabled="disabled" @input="$emit('update:modelValue', $event.target.value)" /><slot name="trailing" /></div>`,
  },
  UTextarea: {
    props: ["modelValue", "disabled"],
    emits: ["update:modelValue"],
    template: `<div><textarea :value="modelValue" :disabled="disabled" @input="$emit('update:modelValue', $event.target.value)" /><slot name="trailing" /></div>`,
  },
  UButton: {
    props: ["disabled", "icon"],
    template: '<button :disabled="disabled"><slot /></button>',
  },
};
const global = { stubs };

beforeEach(() => vi.stubGlobal("useI18n", () => ({ t: (key: string) => key })));
afterEach(() => vi.unstubAllGlobals());

describe("secret visibility", () => {
  it("preserves the exact value and form attributes across repeated toggles without submitting", async () => {
    const submit = vi.fn();
    const model = ref(" initial secret ");
    const Form = defineComponent({
      components: { SecretInput },
      setup: () => ({ model, submit }),
      template: `<form @submit.prevent="submit"><SecretInput v-model="model" id="password" name="password" autocomplete="current-password" required /></form>`,
    });
    const wrapper = mount(Form, { global, attachTo: document.body });
    const input = wrapper.get("input");
    const button = wrapper.get("button");
    expect(input.attributes()).toMatchObject({ type: "password", id: "password", name: "password", autocomplete: "current-password", required: "" });
    expect(button.attributes()).toMatchObject({ type: "button", "aria-label": "common.showSecret", "aria-pressed": "false" });
    await input.setValue("  correct secret 🔑  ");
    for (let i = 0; i < 3; i++) {
      await button.trigger("click");
      expect(input.attributes("type")).toBe("text");
      expect(button.attributes("aria-label")).toBe("common.hideSecret");
      expect(button.attributes("aria-pressed")).toBe("true");
      await button.trigger("click");
      expect(input.attributes("type")).toBe("password");
    }
    expect(model.value).toBe("  correct secret 🔑  ");
    expect(submit).not.toHaveBeenCalled();
    wrapper.unmount();
  });

  it("keeps visibility independent for password confirmation", async () => {
    const Form = defineComponent({
      components: { SecretInput },
      template: '<div><SecretInput model-value="password" /><SecretInput model-value="confirmation" /></div>',
    });
    const wrapper = mount(Form, { global });
    await wrapper.findAll("button")[0]!.trigger("click");
    expect(wrapper.findAll("input").map(input => input.attributes("type"))).toEqual(["text", "password"]);
  });

  it("allows revealing a read-only credential", async () => {
    const wrapper = mount(SecretInput, { props: { modelValue: "copy-password" }, attrs: { readonly: true }, global });
    await wrapper.get("button").trigger("click");
    expect(wrapper.get("input").attributes("readonly")).toBeDefined();
    expect(wrapper.get("input").attributes("type")).toBe("text");
    expect(wrapper.emitted("update:modelValue")).toBeUndefined();
  });

  it("hides cleared values and blocks toggling while disabled or loading", async () => {
    const wrapper = mount(SecretInput, { props: { modelValue: "secret" }, global });
    await wrapper.get("button").trigger("click");
    await wrapper.setProps({ modelValue: "" });
    expect(wrapper.get("input").attributes("type")).toBe("password");
    await wrapper.setProps({ modelValue: "another-secret" });
    await wrapper.get("button").trigger("click");
    await wrapper.setProps({ disabled: true });
    expect(wrapper.get("input").attributes("type")).toBe("password");
    expect(wrapper.get("button").attributes("disabled")).toBeDefined();
    await wrapper.setProps({ disabled: false, loading: true });
    expect(wrapper.get("button").attributes("disabled")).toBeDefined();
  });

  it("preserves multiline private keys when hiding and showing", async () => {
    const key = "-----BEGIN PRIVATE KEY-----\nline-one\nline-two\n-----END PRIVATE KEY-----";
    const wrapper = mount(SecretInput, { props: { modelValue: key, multiline: true }, global });
    const textarea = wrapper.get("textarea");
    expect(wrapper.getComponent(stubs.UTextarea).classes()).toContain("secret-input-masked");
    await wrapper.get("button").trigger("click");
    expect(wrapper.getComponent(stubs.UTextarea).classes()).not.toContain("secret-input-masked");
    await textarea.setValue(`${key}\n`);
    expect(wrapper.emitted("update:modelValue")?.[0]).toEqual([`${key}\n`]);
    await wrapper.get("button").trigger("click");
    expect(wrapper.getComponent(stubs.UTextarea).classes()).toContain("secret-input-masked");
  });

  it("uses independent secret controls for provider credentials and hides them when changing provider", async () => {
    vi.stubGlobal("computed", computed);
    vi.stubGlobal("watch", watch);
    const state = reactive({ provider: "first", model: "test", apiKey: "api-secret", secretId: "public-id", secretKey: "private-key", baseURL: "", region: "", organization: "" });
    const wrapper = mount(ProviderModelForm, {
      props: { state, activeProvider: { id: "first", name: "First", auth: { scheme: "api_key", fields: ["api_key", "secret_id", "secret_key"] } } },
      global: {
        components: { SecretInput },
        mocks: { $t: (key: string) => key },
        stubs: { ...stubs, UForm: { template: '<form><slot /></form>' }, USelect: true, USelectMenu: true },
      },
    });
    expect(wrapper.findAllComponents(SecretInput)).toHaveLength(2);
    expect(wrapper.findAll('input[type="password"]')).toHaveLength(2);
    expect(wrapper.findAll('input[type="text"]')).toHaveLength(1);
    await wrapper.findAllComponents(SecretInput)[0]!.get("button").trigger("click");
    await wrapper.findAllComponents(SecretInput)[0]!.get("input").setValue("updated-api-key");
    expect(state.apiKey).toBe("updated-api-key");
    expect(state.secretKey).toBe("private-key");
    state.provider = "second";
    await wrapper.setProps({ activeProvider: { id: "second", name: "Second", auth: { scheme: "api_key", fields: ["api_key", "secret_id", "secret_key"] } } });
    expect(wrapper.findAll('input[type="password"]')).toHaveLength(2);
  });
});
