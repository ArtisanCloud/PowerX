import { flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { createPinia, defineStore, setActivePinia } from "pinia";
import { computed, h, ref } from "vue";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const calls = vi.hoisted(() => ({ save: vi.fn(), refresh: vi.fn(), invalidate: vi.fn() }));
vi.mock("~/stores/role", () => ({ useRoleStore: () => roleStore() }));
vi.mock("~/stores/permission", () => ({ usePermissionStore: () => permissionStore() }));
vi.mock("~/stores/user", () => ({ useUserStore: () => userStore() }));
vi.mock("~/composables/api/services/tenantService", () => ({ useTenantService: () => ({}) }));
vi.mock("~/composables/api/services/menuService", () => ({ invalidateUserMenusCache: calls.invalidate }));
vi.mock("~/composables/useOneShotAlert", () => ({ useOneShotAlert: () => ({ notifyOnce: vi.fn(), visible: ref(false), hide: vi.fn() }) }));
vi.mock("#imports", () => ({ useI18n: () => ({ t: (key: string) => key, te: () => false, locale: ref("zh") }) }));

import PermissionShell from "~/components/settings/users/PermissionShell.vue";
import PermissionManager from "~/components/settings/users/PermissionManager.vue";
import RoleManager from "~/components/settings/users/RoleManager.vue";

const roles = [
  { id: 1, name: "Super Admin", code: "role_admin", scope: "tenant", builtin: true },
  { id: 4, name: "Tenant User", code: "role_user", scope: "tenant", builtin: true },
];
const roleStore = defineStore("editor-test-roles", {
  state: () => ({ roles: [...roles], loading: false, error: null, pagination: { total: 2, pages: 1 } }),
  actions: { async ensureInitialized() {}, async fetchRoles() {} },
});
const permissionStore = defineStore("editor-test-permissions", {
  state: () => ({
    normalizedList: [{ id: 42, module: "menu", resource: "customer", action: "view", code: "menu.customer:view", type: "menu", name: "客户管理" }],
    pluginCatalog: { plugins: [] },
    roleSelection: {} as Record<number, number[]>,
    roleInitialSelection: {} as Record<number, number[]>,
  }),
  actions: {
    async fetchAllActive() {},
    async fetchPluginCatalog() {},
    async fetchRolePermissionIDs(id: number) {
      this.roleSelection[id] = [...(this.roleInitialSelection[id] || [])];
    },
    async setRolePermissionIDs(id: number, ids: number[]) {
      await calls.save(id, [...ids]);
      this.roleInitialSelection[id] = [...ids];
    },
  },
});
const userStore = defineStore("editor-test-user", {
  state: () => ({ context: { user: { id: 1 } } as any, isRoot: true, currentTenantUuid: "tenant-a", isLoading: false, error: null as string | null }),
  actions: {
    async fetchUserContext(options?: { force?: boolean }) {
      if (!options?.force) return;
      this.isLoading = true;
      try { await calls.refresh(); } finally { this.isLoading = false; }
    },
  },
});
const stubs = {
  PermissionRoot: true, PermissionTenant: true, SelectTree: true, UCard: true,
  UIcon: true, USelect: true, UCheckbox: true, UFormField: true, UTextarea: true, UBadge: { template: "<span><slot/></span>" },
  UButton: { props: ["disabled", "loading"], template: '<button :disabled="disabled || loading"><slot/></button>' },
  UInput: { props: ["modelValue", "placeholder"], emits: ["update:modelValue"], template: '<input :placeholder="placeholder" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)"/>' },
  UTabs: { props: ["modelValue", "items"], emits: ["update:modelValue"], template: '<nav><button v-for="i in items" @click="$emit(\'update:modelValue\', i.value)">{{i.label}}</button></nav>' },
  UModal: { template: "<div/>" },
  MenuPermissionTree: { props: ["selectedIds"], emits: ["toggle"], template: '<button data-testid="customer-menu" @click="$emit(\'toggle\',[42],!selectedIds.includes(42))">客户管理 {{selectedIds.includes(42)}}</button>' },
};
const wrappers: VueWrapper[] = [];
const render = (component = PermissionShell) => {
  const wrapper = mount(component, { global: { stubs, mocks: { $t: (key: string) => key } } });
  wrappers.push(wrapper);
  return wrapper;
};
const button = (wrapper: VueWrapper, label: string) => wrapper.findAll("button").find((b) => b.text() === label)!;
const selectedName = (wrapper: VueWrapper) => wrapper.findAll("h3").find((h) => ["Super Admin", "Tenant User"].includes(h.text()))?.text();
const stateKey = "powerx.permission.manager.view.v1";

beforeEach(() => {
  setActivePinia(createPinia());
  localStorage.clear();
  vi.clearAllMocks();
  calls.save.mockResolvedValue(undefined);
  calls.refresh.mockResolvedValue(undefined);
  (process as any).client = true;
  const state = ref(0);
  vi.stubGlobal("useState", () => state);
  vi.stubGlobal("ref", ref);
  vi.stubGlobal("computed", computed);
});
afterEach(() => {
  wrappers.splice(0).forEach((w) => w.unmount());
  vi.unstubAllGlobals();
});

describe("role permission editor", () => {
  it("renders system root as Core managed while keeping the tenant admin edit action", async () => {
    roleStore().roles = [{ id: 9, name: "System Root", code: "root", scope: "system", builtin: false }, ...roles];
    const wrapper = mount(RoleManager, {global: {stubs: {...stubs, UTable: {
      props: ["data", "columns"],
      setup: (props: any) => () => h("div", props.data.map((role: any) => h("section", {"data-role": role.code}, [role.name, props.columns.find((column: any) => column.id === "actions").cell({row: {original: role}})]))),
    }}, mocks: {$t: (key: string) => key}}});
    wrappers.push(wrapper);
    await flushPromises();
    const root = wrapper.get('[data-role="root"]');
    expect(root.text()).toContain("common.coreManaged");
    expect(root.findAll("button")).toHaveLength(0);
    expect(wrapper.get('[data-role="role_admin"]').text()).toContain("编辑");
  });

  it("excludes managed root even when restored from local storage, keeping tenant admin editable", async () => {
    roleStore().roles = [{ id: 9, name: "System Root", code: "root", scope: "system", builtin: true }, ...roles];
    localStorage.setItem(stateKey, JSON.stringify({ roleId: 9 }));
    const wrapper = render(PermissionManager);
    await flushPromises();
    expect(wrapper.text()).not.toContain("System Root");
    expect(selectedName(wrapper)).toBe("Super Admin");
    expect(JSON.parse(localStorage.getItem(stateKey)!).roleId).toBe(1);
    expect(calls.save).not.toHaveBeenCalled();
  });

  it("keeps the editor, selected role and filters mounted during consecutive saves", async () => {
    const wrapper = render();
    await flushPromises();
    await button(wrapper, "角色配置").trigger("click");
    await flushPromises();
    await wrapper.findAll("h4").find((h) => h.text() === "Tenant User")!.trigger("click");
    await flushPromises();
    const editor = (wrapper.findComponent(PermissionManager).vm as any).$.uid;
    await wrapper.get('input[placeholder="organization.permission.permissionSearch.placeholder"]').setValue("customer");
    await wrapper.get('[data-testid="customer-menu"]').trigger("click");
    let finish!: () => void;
    calls.refresh.mockReturnValueOnce(new Promise<void>((resolve) => { finish = resolve; }));
    await button(wrapper, "保存").trigger("click");
    await flushPromises();
    expect(userStore().isLoading).toBe(true);
    expect((wrapper.findComponent(PermissionManager).vm as any).$.uid).toBe(editor);
    expect(selectedName(wrapper)).toBe("Tenant User");
    finish();
    await flushPromises();
    expect((wrapper.get('input[placeholder="organization.permission.permissionSearch.placeholder"]').element as HTMLInputElement).value).toBe("customer");
    await wrapper.get('[data-testid="customer-menu"]').trigger("click");
    await button(wrapper, "保存").trigger("click");
    await flushPromises();
    expect(calls.save.mock.calls).toEqual([[4, [42]], [4, []]]);
    expect(selectedName(wrapper)).toBe("Tenant User");
    expect((wrapper.findComponent(PermissionManager).vm as any).$.uid).toBe(editor);
  });

  it.each([false, true])("restores a non-first role before persisting defaults (delayed roles: %s)", async (delayed) => {
    localStorage.setItem(stateKey, JSON.stringify({ roleId: 4, permissionTab: "capabilities", permissionSearchQuery: "customer", searchQuery: "Tenant" }));
    if (delayed) roleStore().roles = [];
    const wrapper = render(PermissionManager);
    await flushPromises();
    if (delayed) {
      expect(JSON.parse(localStorage.getItem(stateKey)!).roleId).toBe(4);
      roleStore().roles = [...roles];
      await flushPromises();
    }
    expect(selectedName(wrapper)).toBe("Tenant User");
    const saved = JSON.parse(localStorage.getItem(stateKey)!);
    expect(saved).toMatchObject({ roleId: 4, permissionTab: "capabilities", permissionSearchQuery: "customer", searchQuery: "Tenant" });
  });

  it("keeps edits on a background refresh failure, then removes root content on an actual privilege change", async () => {
    const wrapper = render();
    await flushPromises();
    await button(wrapper, "角色配置").trigger("click");
    await flushPromises();
    const editor = (wrapper.findComponent(PermissionManager).vm as any).$.uid;
    userStore().error = "Context refresh failed";
    await flushPromises();
    expect(wrapper.get('[role="alert"]').text()).toContain("Context refresh failed");
    expect((wrapper.findComponent(PermissionManager).vm as any).$.uid).toBe(editor);
    userStore().isRoot = false;
    await flushPromises();
    expect(wrapper.findComponent(PermissionManager).exists()).toBe(false);
    expect(wrapper.findComponent({ name: "PermissionTenant" }).exists()).toBe(true);
  });

  it("shows initial loading without mounting a role editor before context exists", async () => {
    userStore().context = null;
    userStore().isLoading = true;
    const wrapper = render();
    await flushPromises();
    expect(wrapper.text()).toContain("common.loading");
    expect(wrapper.findComponent(PermissionManager).exists()).toBe(false);
  });
});
