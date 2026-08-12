export const decodeJwtPayload = (token?: string | null): Record<string, any> | null => {
  const raw = String(token || "").trim();
  if (!raw) return null;
  const parts = raw.split(".");
  if (parts.length !== 3) return null;
  try {
    const padded = parts[1].padEnd(parts[1].length + ((4 - (parts[1].length % 4)) % 4), "=");
    const json = atob(padded.replace(/-/g, "+").replace(/_/g, "/"));
    return JSON.parse(json);
  } catch {
    return null;
  }
};

export const jwtExpiresAtMillis = (token?: string | null): number | null => {
  const claims = decodeJwtPayload(token);
  const exp = Number(claims?.exp || 0);
  if (!Number.isFinite(exp) || exp <= 0) return null;
  return exp * 1000;
};

export const syncExpiresAtFromJWT = (token?: string | null): number | null => {
  if (!process.client) return null;
  const expiresAt = jwtExpiresAtMillis(token);
  if (!expiresAt) return null;
  localStorage.setItem("expires_at", String(expiresAt));
  return expiresAt;
};

export const isAccessTokenExpired = (token?: string | null, skewMs = 0): boolean => {
  const raw = String(token || "").trim();
  if (!raw) return true;
  const jwtExpiresAt = jwtExpiresAtMillis(raw);
  if (jwtExpiresAt) return Date.now() + skewMs >= jwtExpiresAt;
  if (!process.client) return true;
  const stored = Number(localStorage.getItem("expires_at") || 0);
  return !Number.isFinite(stored) || stored <= 0 || Date.now() + skewMs >= stored;
};
