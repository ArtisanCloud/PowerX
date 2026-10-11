import { useCookie } from "#app";
import { readAuthSession } from "~/utils/auth-session";
import { decodeJwtPayload } from "~/utils/auth-token";

export const TENANT_UUID_STORAGE_KEY = "px_current_tenant_uuid";

const normalize = (value?: string | null) =>
  value ? value.trim().toLowerCase() : "";

export const getStoredTenantUUID = (): string | null => {
  if (process.client) {
    const session = readAuthSession();
    return session?.status === 'active' && session.tenantUuid ? normalize(session.tenantUuid) : null;
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

    const claims = decodeJwtPayload(readAuthSession()?.accessToken);
    const tokenTenantUUID =
      (typeof claims?.tid === "string" && claims.tid) ||
      (typeof claims?.tenant_uuid === "string" && claims.tenant_uuid) ||
      "";
    const tokenNormalized = tokenTenantUUID ? normalize(tokenTenantUUID) : "";
    if (tokenNormalized) return tokenNormalized;
    if (stored) return normalize(stored);
    return tokenNormalized || undefined;
  })();

export const extractTenantUUIDFromJWT = (token?: string | null) => {
  const claims = decodeJwtPayload(token);
  const raw =
    (typeof claims?.tid === "string" && claims.tid) ||
    (typeof claims?.tenant_uuid === "string" && claims.tenant_uuid) ||
    "";
  const normalized = raw ? normalize(raw) : "";
  return normalized || undefined;
};
