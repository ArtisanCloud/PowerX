import { useCookie } from "#app";

const authCookieOptions = {
  sameSite: "lax" as const,
  path: "/",
};

export function syncAuthCookies(accessToken?: string | null, refreshToken?: string | null) {
  if (!process.client) return;
  const normalizedAccessToken = String(accessToken || "").trim();
  const accessTokenCookie = useCookie<string | null>("access_token", authCookieOptions);
  accessTokenCookie.value = normalizedAccessToken || null;

  if (typeof refreshToken !== "undefined") {
    const normalizedRefreshToken = String(refreshToken || "").trim();
    const refreshTokenCookie = useCookie<string | null>("refresh_token", authCookieOptions);
    refreshTokenCookie.value = normalizedRefreshToken || null;
  }
}

export function clearAuthCookies() {
  if (!process.client) return;
  syncAuthCookies(null, null);
}
