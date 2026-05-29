import { createPinia, setActivePinia } from "pinia";
import { beforeEach, describe, expect, it } from "vitest";
import { useUserStore } from "~/stores/user";
import type { UserContextData } from "~/composables/api/services/meService";

function makeContext(overrides: Partial<UserContextData>): UserContextData {
  return {
    is_root: false,
    current_tenant_uuid: "6b5d0240-9920-46da-b707-88200e0f51ea",
    current_member_id: 101,
    user: {
      id: 1,
      email: "tenant-admin@example.com",
      phone: "",
      display_name: "Tenant Admin",
      avatar_url: "",
      status: 1,
      is_root: false,
    },
    members: [
      {
        tenant_uuid: "6b5d0240-9920-46da-b707-88200e0f51ea",
        tenant_name: "Tenant A",
        member_id: 101,
        is_admin: true,
      },
    ],
    ...overrides,
  };
}

describe("root 平台身份菜单分流", () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    localStorage.clear();
  });

  it("root 默认平台身份不得被推导为当前租户管理员", () => {
    const store = useUserStore();
    store.context = makeContext({
      is_root: true,
      current_tenant_uuid: "",
      current_member_id: null,
      user: {
        id: 1,
        email: "root@example.com",
        phone: "",
        display_name: "Root",
        avatar_url: "",
        status: 1,
        is_root: true,
      },
      members: [],
    });

    expect(store.isRoot).toBe(true);
    expect(store.currentTenant).toBeNull();
    expect(store.isCurrentTenantAdmin).toBe(false);
  });

  it("root 拥有系统租户成员关系时仍不能自动等同业务租户 admin", () => {
    const store = useUserStore();
    store.context = makeContext({
      is_root: true,
      current_tenant_uuid: "00000000-0000-4000-8000-000000000000",
      current_member_id: 1,
      user: {
        id: 1,
        email: "root@example.com",
        phone: "",
        display_name: "Root",
        avatar_url: "",
        status: 1,
        is_root: true,
      },
      members: [
        {
          tenant_uuid: "00000000-0000-4000-8000-000000000000",
          tenant_name: "System",
          member_id: 1,
          is_admin: true,
        },
      ],
    });

    expect(store.isRoot).toBe(true);
    expect(store.currentTenant?.tenant_name).toBe("System");
    expect(store.isCurrentTenantAdmin).toBe(false);
  });

  it("普通租户用户只能由当前成员角色决定 tenant admin", () => {
    const store = useUserStore();
    store.context = makeContext({});

    expect(store.isRoot).toBe(false);
    expect(store.currentTenant?.tenant_name).toBe("Tenant A");
    expect(store.isCurrentTenantAdmin).toBe(true);
  });
});
