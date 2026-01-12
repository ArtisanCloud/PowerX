import { useCookie } from "#app";

export const TENANT_UUID_STORAGE_KEY = "px_current_tenant_uuid";

const normalize = (value?: string | null) =>
  value ? value.trim().toLowerCase() : "";

const decodeJwtPayload = (token?: string | null): Record<string, any> | null => {
  if (!token) return null;
  const parts = token.split(".");
  if (parts.length !== 3) return null;
  try {
    const padded =
      parts[1].padEnd(parts[1].length + ((4 - (parts[1].length % 4)) % 4), "=");
    const json = atob(padded.replace(/-/g, "+").replace(/_/g, "/"));
    return JSON.parse(json);
  } catch {
    return null;
  }
};

export const getStoredTenantUUID = (): string | null => {
  if (process.client) {
    const fromStorage = localStorage.getItem(TENANT_UUID_STORAGE_KEY);
    return fromStorage ? normalize(fromStorage) : null;
  }

  const cookie = useCookie<string | null>(TENANT_UUID_STORAGE_KEY);
  return cookie.value ? normalize(cookie.value) : null;
};

export const persistTenantUUID = (tenantUUID: string | null) => {
  const normalized = normalize(tenantUUID);

  if (process.client) {
    if (normalized) {
      localStorage.setItem(TENANT_UUID_STORAGE_KEY, normalized);
    } else {
      localStorage.removeItem(TENANT_UUID_STORAGE_KEY);
    }

    document.cookie = normalized
      ? `${TENANT_UUID_STORAGE_KEY}=${normalized}; path=/; SameSite=Lax`
      : `${TENANT_UUID_STORAGE_KEY}=; path=/; expires=${new Date(
          0
        ).toUTCString()}; SameSite=Lax`;
  }

  const cookie = useCookie<string | null>(TENANT_UUID_STORAGE_KEY);
  cookie.value = normalized || null;
};

export const resolveTenantUUIDForRequest = () =>
  (() => {
    const stored = getStoredTenantUUID();
    if (!process.client) return stored || undefined;

    const claims = decodeJwtPayload(localStorage.getItem("access_token"));
    const tokenTenantUUID =
      (typeof claims?.tid === "string" && claims.tid) ||
      (typeof claims?.tenant_uuid === "string" && claims.tenant_uuid) ||
      "";
    const tokenNormalized = tokenTenantUUID ? normalize(tokenTenantUUID) : "";

    if (tokenNormalized) {
      // 避免 db-refresh 后 localStorage 里残留旧租户导致请求打到错误租户
      if (!stored) return tokenNormalized;
      if (normalize(stored) !== tokenNormalized) return tokenNormalized;
      return normalize(stored);
    }

    return stored || undefined;
  })();
