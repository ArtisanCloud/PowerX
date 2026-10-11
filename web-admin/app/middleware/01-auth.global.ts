import { readAuthSession, captureAuthSession, isAuthSessionCurrent, AuthSessionChangedError } from "~/utils/auth-session";

// middleware/app.global.ts
export default defineNuxtRouteMiddleware(async (to) => {

  const skipAuth = process.env.NUXT_PUBLIC_E2E_SKIP_AUTH === "true";
  const setupStatus = useSetupStatus();

  // —— 若你有 i18n，to.path 可能是 /zh/home、/en/intro 等
  //    下面做一个“可选语言前缀”的正则匹配
  const withLocale = (p: string | RegExp) => {
    if (typeof p !== "string") {
      return new RegExp(`^/(?:[a-z]{2}(?:-[A-Z]{2})?/)?${p.source}(?:/|$)`);
    }
    const normalized = p.replace(/^\/+/, "");
    return new RegExp(`^/(?:[a-z]{2}(?:-[A-Z]{2})?/)?${normalized}(?:/|$)`);
  };

  const PUBLIC_RULES: RegExp[] = [
    withLocale("/home"),
    withLocale("/intro"),
    withLocale("/setup"),
    withLocale("/users/login"),
    withLocale("/users/register"),
  ];
  const AUTHENTICATED_NON_MENU_RULES: RegExp[] = [
    withLocale("/profile"),
    withLocale("/notifications"),
    withLocale("/search"),
  ];

  // 1) 首装判定在服务端/客户端都执行，避免首屏 SSR 进入业务页
  const setup = await setupStatus.load({ ttlMs: 5000 });
  const shouldStayInSetup = Boolean(
    setup &&
    !setup.configured &&
    !setup.requires_login,
  );
  if (to.path === "/") {
    if (shouldStayInSetup) {
      return navigateTo("/setup");
    }
    return navigateTo("/home");
  }

  // 首次安装：无用户且未完成初始化时，强制进入 /setup
  if (
    shouldStayInSetup &&
    !withLocale("/setup").test(to.path)
  ) {
    return navigateTo("/setup");
  }
  if (
    setup &&
    setup.configured &&
    withLocale("/setup").test(to.path)
  ) {
    return navigateTo("/home");
  }

  // 2) 只在客户端做 localStorage 鉴权
  if (process.server) return;

  const auth = useAuth();
  const publicHit = PUBLIC_RULES.some((re) => re.test(to.path));
  console.info("🚦 publicHit:", publicHit);

  if (skipAuth) return;
  const clearAuthStorage = () => auth.clearAuth();

  const redirectToLogin = () => {
    const currentPath = to.fullPath;
    if (!withLocale("/users/login").test(to.path)) {
      return navigateTo(
        `/users/login?redirect=${encodeURIComponent(currentPath)}`
      );
    }
  };

  const session = readAuthSession();
  let token = auth.getToken();
  let needRefresh = !token;
  if (publicHit) return;
  if (needRefresh && session?.status === "active") {
    const stamp = captureAuthSession();
    try {
      token = await auth.refreshAuth();
      needRefresh = !token;
    } catch (error) {
      if (error instanceof AuthSessionChangedError || (error as any)?.authSessionStamp && !isAuthSessionCurrent((error as any).authSessionStamp)) return;
      clearAuthStorage();
      return redirectToLogin();
    }
  }

  if (needRefresh) {
    clearAuthStorage();
    return redirectToLogin();
  }

  const authenticatedNonMenuHit = AUTHENTICATED_NON_MENU_RULES.some((re) =>
    re.test(to.path)
  );
  const shouldCheckMenuRoute = !publicHit && !authenticatedNonMenuHit;
  if (shouldCheckMenuRoute) {
    const { isMenuRouteAllowed, resolveDefaultRoute } = useDefaultMenuRoute();
    const stamp = captureAuthSession();
    try {
      const allowed = await isMenuRouteAllowed(to.path);
      if (!isAuthSessionCurrent(stamp)) return;
      if (!allowed) {
        const target = await resolveDefaultRoute();
        if (!isAuthSessionCurrent(stamp)) return;
        if (target !== to.path) {
          return navigateTo(target, { replace: true });
        }
      }
    } catch {
      if (!isAuthSessionCurrent(stamp)) return;
      return navigateTo("/home", { replace: true });
    }
  }

});
