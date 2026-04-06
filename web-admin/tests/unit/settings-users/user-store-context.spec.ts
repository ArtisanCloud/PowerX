import { beforeEach, describe, expect, it, vi } from "vitest";
import { mount } from "@vue/test-utils";
import { ref } from "vue";
import UsersShell from "~/components/settings/users/UsersShell.vue";

const fetchUserContextMock = vi.fn();

const isRootRef = ref(false);
const isCurrentTenantAdminRef = ref(false);
const currentTenantUuidRef = ref("6b5d0240-9920-46da-b707-88200e0f51ea");
const isLoadingRef = ref(false);
const errorRef = ref("");
const displayNameRef = ref("tester");
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

describe("UsersShell 上下文刷新", () => {
  beforeEach(() => {
    fetchUserContextMock.mockReset();
    fetchUserContextMock.mockResolvedValue(undefined);
    isRootRef.value = false;
    isCurrentTenantAdminRef.value = false;
    isLoadingRef.value = false;
    errorRef.value = "";
  });

  it("挂载时应强制刷新用户上下文", async () => {
    mount(UsersShell, {
      global: {
        mocks: { $t: (k: string) => k },
        stubs: {
          UsersRoot: { template: "<div data-test='root' />" },
          UsersTenantAdmin: { template: "<div data-test='admin' />" },
          UsersTenantMember: { template: "<div data-test='member' />" },
          UButton: { template: "<button><slot /></button>" },
          UIcon: { template: "<i />" },
        },
      },
    });

    await Promise.resolve();
    expect(fetchUserContextMock).toHaveBeenCalledWith({ force: true });
  });

  it("错误态点击重试应强制刷新上下文", async () => {
    errorRef.value = "mock error";

    const wrapper = mount(UsersShell, {
      global: {
        mocks: { $t: (k: string) => k },
        stubs: {
          UsersRoot: { template: "<div data-test='root' />" },
          UsersTenantAdmin: { template: "<div data-test='admin' />" },
          UsersTenantMember: { template: "<div data-test='member' />" },
          UIcon: { template: "<i />" },
          UButton: {
            emits: ["click"],
            template: "<button @click=\"$emit('click')\"><slot /></button>",
          },
        },
      },
    });

    await Promise.resolve();
    const retryBtn = wrapper.find("button");
    expect(retryBtn.exists()).toBe(true);
    await retryBtn.trigger("click");
    expect(fetchUserContextMock).toHaveBeenCalledWith({ force: true });
  });
});
