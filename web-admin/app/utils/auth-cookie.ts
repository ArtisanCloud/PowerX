import { useCookie } from "#app";

const authCookieOptions = {
  sameSite: "lax" as const,
  path: "/",
};

// 先同步浏览器 Cookie，避免 iframe 在 Nuxt Cookie watcher 执行前读取旧 token。
function writeBrowserAuthCookie(name: string, value: string | null) {
  document.cookie = value
    ? `${name}=${encodeURIComponent(value)}; path=/; SameSite=Lax`
    : `${name}=; Max-Age=0; path=/; SameSite=Lax`;
}

export function syncAuthCookies(accessToken?: string | null, refreshToken?: string | null) {
  if (!process.client) return;
  const normalizedAccessToken = String(accessToken || "").trim();
  writeBrowserAuthCookie("access_token", normalizedAccessToken || null);
  const accessTokenCookie = useCookie<string | null>("access_token", authCookieOptions);
  accessTokenCookie.value = normalizedAccessToken || null;

  if (typeof refreshToken !== "undefined") {
    const normalizedRefreshToken = String(refreshToken || "").trim();
    writeBrowserAuthCookie("refresh_token", normalizedRefreshToken || null);
    const refreshTokenCookie = useCookie<string | null>("refresh_token", authCookieOptions);
    refreshTokenCookie.value = normalizedRefreshToken || null;
  }
}

export function clearAuthCookies() {
  if (!process.client) return;
  syncAuthCookies(null, null);
}
