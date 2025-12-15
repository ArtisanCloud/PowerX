// middleware/app.global.ts
export default defineNuxtRouteMiddleware((to) => {
  if (process.env.NUXT_PUBLIC_E2E_SKIP_AUTH === "true") {
    return
  }

  // 1) 根路径 -> /home（仅服务端执行一次也OK）
  if (to.path === "/") {
    return navigateTo("/home");
  }

  // 2) 只在客户端做 localStorage 鉴权
  if (process.server) return;

  // —— 若你有 i18n，to.path 可能是 /zh/home、/en/intro 等
  //    下面做一个“可选语言前缀”的正则匹配
  const withLocale = (p: string | RegExp) =>
    new RegExp(
      `^/(?:[a-z]{2}(?:-[A-Z]{2})?)?${
        typeof p === "string" ? p.replace("/", "\\/") : (p as RegExp).source
      }(?:/|$)`
    );

  const PUBLIC_RULES: RegExp[] = [
    withLocale("/home"),
    withLocale("/intro"),
    withLocale("/users/login"),
    withLocale("/users/register"),
  ];

  const publicHit = PUBLIC_RULES.some((re) => re.test(to.path));
  console.log("🚦 publicHit:", publicHit);
  if (publicHit) return;

  // 3) 其余路由需要登录
  const isTokenExpired = (): boolean => {
    const expiresAt = localStorage.getItem("expires_at");
    if (!expiresAt) return true;
    return Date.now() > Number(expiresAt);
  };

  const token = localStorage.getItem("access_token");

  if (!token || isTokenExpired()) {
    [
      "access_token",
      "refresh_token",
      "token_type",
      "expires_in",
      "expires_at",
    ].forEach((k) => localStorage.removeItem(k));

    const currentPath = to.fullPath;
    if (!withLocale("/users/login").test(to.path)) {
      return navigateTo(
        `/users/login?redirect=${encodeURIComponent(currentPath)}`
      );
    }
  }
});
