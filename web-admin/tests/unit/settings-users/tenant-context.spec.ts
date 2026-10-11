import { commitAuthSession } from "~/utils/auth-session";
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
    expect(getStoredTenantUUID()).toBeNull();
  });

  it("resolveTenantUUIDForRequest 使用宿主快照而不是旧插件镜像", () => {
    const payload = {
      tid: "6b5d0240-9920-46da-b707-88200e0f51ea",
      sub: "u1", exp: Math.floor(Date.now()/1000) + 3600,
    };
    const encoded = btoa(JSON.stringify(payload))
      .replace(/\+/g, "-")
      .replace(/\//g, "_")
      .replace(/=+$/, "");
    commitAuthSession({access_token:`h.${encoded}.s`, refresh_token:'refresh', expires_in:3600});
    localStorage.setItem("access_token", 'old-plugin-token');
    expect(getStoredTenantUUID()).toBe(payload.tid);

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
