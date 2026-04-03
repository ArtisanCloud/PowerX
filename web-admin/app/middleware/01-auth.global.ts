// middleware/app.global.ts
export default defineNuxtRouteMiddleware(async (to) => {
  const skipAuth = process.env.NUXT_PUBLIC_E2E_SKIP_AUTH === "true";
  const runtimeConfig = useRuntimeConfig();
  const apiBase = String(runtimeConfig.public?.apiBase || "/api").replace(/\/+$/, "");
  const setupStatusPath = `${apiBase}/admin/setup/status`;

  const loadSetupStatus = async (): Promise<{ configured: boolean; requires_login: boolean; restart_required: boolean } | null> => {
    try {
      const resp: any = await $fetch(setupStatusPath, {
        method: "GET",
        timeout: 5000,
      });
      const payload = resp?.data ?? resp;
      return {
        configured: Boolean(payload?.configured),
        requires_login: Boolean(payload?.requires_login),
        restart_required: Boolean(payload?.restart_required),
      };
    } catch {
      return null;
    }
  };

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

  // 1) 首装判定在服务端/客户端都执行，避免首屏 SSR 进入业务页
  const setup = await loadSetupStatus();
  const shouldStayInSetup = Boolean(
    setup &&
    ((!setup.configured && !setup.requires_login) || setup.restart_required),
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
    !setup.restart_required &&
    withLocale("/setup").test(to.path)
  ) {
    return navigateTo("/home");
  }

  // 2) 只在客户端做 localStorage 鉴权
  if (process.server) return;

  const publicHit = PUBLIC_RULES.some((re) => re.test(to.path));
  console.log("🚦 publicHit:", publicHit);
  if (publicHit) return;

  if (skipAuth) {
    return;
  }

  // 3) 其余路由需要登录
  const isTokenExpired = (): boolean => {
    const expiresAt = localStorage.getItem("expires_at");
    if (!expiresAt) return true;
    return Date.now() > Number(expiresAt);
  };

  const clearAuthStorage = () => {
    [
      "access_token",
      "refresh_token",
      "token_type",
      "expires_in",
      "expires_at",
      "scope",
      "px_current_tenant_uuid",
    ].forEach((k) => localStorage.removeItem(k));
  };

  const redirectToLogin = () => {
    const currentPath = to.fullPath;
    if (!withLocale("/users/login").test(to.path)) {
      return navigateTo(
        `/users/login?redirect=${encodeURIComponent(currentPath)}`
      );
    }
  };

  const token = localStorage.getItem("access_token");
  const refreshToken = localStorage.getItem("refresh_token");
  const needRefresh = !token || isTokenExpired();

  if (needRefresh && refreshToken) {
    return $fetch("/api/v1/admin/user/auth/refresh", {
      method: "POST",
      body: { refresh_token: refreshToken },
      headers: {
        "Content-Type": "application/json",
        Accept: "application/json",
      },
      timeout: 30000,
    })
      .then((resp: any) => {
        const payload = resp?.data ?? resp;
        if (!payload?.access_token) {
          clearAuthStorage();
          return redirectToLogin();
        }

        const tokenType = String(payload.token_type || "Bearer");
        const expiresInSec = Math.max(0, Number(payload.expires_in || 0));
        const expiresAt = Date.now() + expiresInSec * 1000;

        localStorage.setItem("access_token", String(payload.access_token));
        localStorage.setItem("token_type", tokenType);
        localStorage.setItem("expires_in", String(expiresInSec));
        localStorage.setItem("expires_at", String(expiresAt));
        if (typeof payload.scope === "string") {
          localStorage.setItem("scope", payload.scope);
        }
        if (typeof payload.refresh_token === "string" && payload.refresh_token) {
          localStorage.setItem("refresh_token", payload.refresh_token);
        }

        return;
      })
      .catch(() => {
        clearAuthStorage();
        return redirectToLogin();
      });
  }

  if (needRefresh) {
    clearAuthStorage();
    return redirectToLogin();
  }
});
