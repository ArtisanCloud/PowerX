import { describe, expect, it, vi } from "vitest";
import { mount } from "@vue/test-utils";
import UsersRoot from "~/components/settings/users/UsersRoot.vue";

const switchTenantMock = vi.fn();

vi.mock("~/stores/user", () => ({
  useUserStore: () => ({
    switchTenant: switchTenantMock,
  }),
}));

vi.mock("~/composables/api/services/tenantService", () => ({
  TenantStatus: {
    Active: "active",
    Inactive: "inactive",
    Suspended: "suspended",
  },
  getTenantStatusColor: () => "success",
  getTenantStatusDisplayKey: () => "active",
  useTenantService: () => ({
    getTenants: vi.fn().mockResolvedValue({
      code: 200,
      data: {
        items: [
          {
            id: 1,
            uuid: "6b5d0240-9920-46da-b707-88200e0f51ea",
            name: "System",
            domain: "system.local",
            status: "active",
            user_count: 1,
            createdAt: "2026-01-01T00:00:00Z",
            plan: "free",
          },
        ],
        pagination: { total: 1, pages: 1 },
      },
    }),
  }),
}));

vi.mock("#imports", () => ({
  useI18n: () => ({ t: (k: string) => k }),
  useToast: () => ({ add: vi.fn() }),
}));

describe("Users 动作语义拆分", () => {
  it("点击租户行不触发切租户；点击“切换并管理”才触发", async () => {
    switchTenantMock.mockReset();
    const wrapper = mount(UsersRoot, {
      global: {
        mocks: { $t: (k: string) => k },
        stubs: {
          UTooltip: { template: "<div><slot /></div>" },
          UIcon: { template: "<i />" },
          UFormField: { template: "<div><slot /></div>" },
          UModal: { template: "<div><slot /><slot name='content' /></div>" },
          UCard: {
            template:
              "<div><slot name='header' /><slot /><slot name='footer' /></div>",
          },
          UInput: { template: "<input />" },
          USelect: { template: "<select />" },
          UBadge: { template: "<span><slot /></span>" },
          UsersTenantAdmin: { template: "<div />" },
          UButton: {
            emits: ["click"],
            template: "<button @click=\"$emit('click',$event)\"><slot /></button>",
          },
        },
      },
    });

    await Promise.resolve();
    await Promise.resolve();

    await wrapper.find(".divide-y > div").trigger("click");
    expect(switchTenantMock).toHaveBeenCalledTimes(0);

    const switchBtn = wrapper
      .findAll("button")
      .find((b) => b.text().includes("切换并管理"));
    expect(switchBtn).toBeTruthy();
    await switchBtn!.trigger("click");
    expect(switchTenantMock).toHaveBeenCalledTimes(1);
  });
});
