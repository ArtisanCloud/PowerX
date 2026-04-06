import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  TENANT_UUID_STORAGE_KEY,
  extractTenantUUIDFromJWT,
  getStoredTenantUUID,
  persistTenantUUID,
  resolveTenantUUIDForRequest,
} from "~/utils/tenant-context";

describe("tenant context utils", () => {
  beforeEach(() => {
    localStorage.clear();
    vi.restoreAllMocks();
    (process as any).client = true;
  });

  it("persistTenantUUID 会标准化并写入 localStorage", () => {
    persistTenantUUID(" 6B5D0240-9920-46DA-B707-88200E0F51EA ");
    expect(localStorage.getItem(TENANT_UUID_STORAGE_KEY)).toBe(
      "6b5d0240-9920-46da-b707-88200e0f51ea"
    );
    expect(getStoredTenantUUID()).toBe("6b5d0240-9920-46da-b707-88200e0f51ea");
  });

  it("resolveTenantUUIDForRequest 在无 stored tenant 时回落到 access_token.tid", () => {
    const payload = {
      tid: "6b5d0240-9920-46da-b707-88200e0f51ea",
      sub: "u1",
    };
    const encoded = btoa(JSON.stringify(payload))
      .replace(/\+/g, "-")
      .replace(/\//g, "_")
      .replace(/=+$/, "");
    localStorage.setItem("access_token", `h.${encoded}.s`);

    expect(resolveTenantUUIDForRequest()).toBe(
      "6b5d0240-9920-46da-b707-88200e0f51ea"
    );
  });

  it("extractTenantUUIDFromJWT 读取 tenant_uuid 并标准化", () => {
    const payload = {
      tenant_uuid: "6B5D0240-9920-46DA-B707-88200E0F51EA",
    };
    const encoded = btoa(JSON.stringify(payload))
      .replace(/\+/g, "-")
      .replace(/\//g, "_")
      .replace(/=+$/, "");
    expect(extractTenantUUIDFromJWT(`h.${encoded}.s`)).toBe(
      "6b5d0240-9920-46da-b707-88200e0f51ea"
    );
  });
});
