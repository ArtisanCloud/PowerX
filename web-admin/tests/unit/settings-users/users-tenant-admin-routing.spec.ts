import { beforeEach, describe, expect, it, vi } from "vitest";
import { mount } from "@vue/test-utils";
import { ref } from "vue";
import UsersShell from "~/components/settings/users/UsersShell.vue";

const fetchUserContextMock = vi.fn();

const isRootRef = ref(false);
const isCurrentTenantAdminRef = ref(true);
const currentTenantUuidRef = ref("6b5d0240-9920-46da-b707-88200e0f51ea");
const isLoadingRef = ref(false);
const errorRef = ref("");
const displayNameRef = ref("tenant-admin");
const avatarUrlRef = ref("");

vi.mock("~/stores/user", () => ({
  useUserStore: () => ({
    fetchUserContext: fetchUserContextMock,
  }),
}));

vi.mock("pinia", async (importOriginal) => {
  const actual = await importOriginal<typeof import("pinia")>();
  return {
    ...actual,
    storeToRefs: () => ({
      isRoot: isRootRef,
      isCurrentTenantAdmin: isCurrentTenantAdminRef,
      currentTenantUuid: currentTenantUuidRef,
      isLoading: isLoadingRef,
      error: errorRef,
      displayName: displayNameRef,
      avatarUrl: avatarUrlRef,
    }),
  };
});

vi.mock("#imports", () => ({
  useI18n: () => ({ t: (k: string) => k }),
  useState: (_key: string, init: () => any) => ref(init()),
}));

describe("Tenant Admin 视图路由", () => {
  beforeEach(() => {
    (globalThis as any).definePageMeta = vi.fn();
    (globalThis as any).useI18n = () => ({ t: (k: string) => k });
    (globalThis as any).useState = (_key: string, init: () => any) =>
      ref(init());
    (globalThis as any).ref = ref;
    fetchUserContextMock.mockReset();
    fetchUserContextMock.mockResolvedValue(undefined);
    isRootRef.value = false;
    isCurrentTenantAdminRef.value = true;
    isLoadingRef.value = false;
    errorRef.value = "";
  });

  it("UsersShell 在 tenant admin 上下文应渲染 admin 视图", async () => {
    const wrapper = mount(UsersShell, {
      global: {
        mocks: { $t: (k: string) => k },
        stubs: {
          UsersRoot: { template: "<div data-test='root' />" },
          UsersTenantAdmin: { template: "<div data-test='tenant-admin' />" },
          UsersTenantMember: { template: "<div data-test='member' />" },
          UButton: { template: "<button><slot /></button>" },
          UIcon: { template: "<i />" },
        },
      },
    });

    await Promise.resolve();
    expect(wrapper.find("[data-test='tenant-admin']").exists()).toBe(true);
    expect(wrapper.find("[data-test='member']").exists()).toBe(false);
  });

  it("UsersShell 在 root 上下文默认应渲染当前租户管理视图（非租户目录）", async () => {
    isRootRef.value = true;
    isCurrentTenantAdminRef.value = true;

    const wrapper = mount(UsersShell, {
      global: {
        mocks: { $t: (k: string) => k },
        stubs: {
          UsersRoot: { template: "<div data-test='root-directory' />" },
          UsersTenantAdmin: { template: "<div data-test='tenant-admin' />" },
          UsersTenantMember: { template: "<div data-test='member' />" },
          UButton: {
            emits: ["click"],
            template: "<button @click=\"$emit('click',$event)\"><slot /></button>",
          },
          UIcon: { template: "<i />" },
        },
      },
    });

    await Promise.resolve();
    expect(wrapper.find("[data-test='tenant-admin']").exists()).toBe(true);
    expect(wrapper.find("[data-test='root-directory']").exists()).toBe(false);
  });
});
