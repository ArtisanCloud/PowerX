import type { LoginResponse } from "./api/services/authService";
import { useAuthService } from "./api/services/authService";
import { AUTH_SESSION_EVENT, AUTH_SESSION_KEY, captureAuthSession, clearAuthSession, commitAuthSession, isAuthSessionCurrent, readAuthSession, refreshAuthSession, syncAuthSessionCookies } from "~/utils/auth-session";
/**
 * 认证状态管理
 */
export const useAuth = () => {
  // 认证状态 - 使用useState确保SSR兼容性
  const isAuthenticated = useState("auth.isAuthenticated", () => false);
  const user = useState("auth.user", () => null);
  const token = useState<string | null>("auth.token", () => null);

  /**
   * 保存认证信息
   */
  const setAuth = (authData: LoginResponse) => {
    const accessToken = String(authData?.access_token || "").trim();
    const expiresIn = Number(authData?.expires_in || 0);

    if (!accessToken || !Number.isFinite(expiresIn) || expiresIn <= 0) {
      throw new Error("invalid auth payload");
    }

    if (process.client) commitAuthSession(authData);

    // 更新状态
    token.value = accessToken;
    isAuthenticated.value = true;
  };

  /**
   * 清除认证信息
   */
  const clearAuth = () => {
    if (process.client) {
      clearAuthSession();
    }

    token.value = null;
    user.value = null;
    isAuthenticated.value = false;
  };

  /**
   * 检查token是否过期
   */
  const isTokenExpired = (): boolean => {
    if (!process.client) return true;
    const session = readAuthSession();
    return !session || session.status !== 'active' || session.expiresAt <= Date.now();
  };

  /**
   * 获取当前token
   */
  const getToken = (): string | null => {
    if (!process.client) return null;

    if (!readAuthSession()) { clearAuth(); return null; }
    if (isTokenExpired()) return null;
    return readAuthSession()?.accessToken || null;
  };

  /**
   * 初始化认证状态
   */
  const initAuth = () => {
    if (!process.client) return;

    const session = readAuthSession();
    if (!session) { clearAuth(); return; }
    syncAuthSessionCookies();
    if (session.status === 'active') {
      token.value = session.accessToken;
      isAuthenticated.value = true;
    } else {
      token.value = null;
      user.value = null;
      isAuthenticated.value = false;
    }
  };

  const refreshAuth = () => {
    const cfg = useRuntimeConfig();
    const baseURL = String(cfg.public.apiBase || '/api/v1').replace(/\/+$/, '');
    return refreshAuthSession(async refreshToken => {
      const response: any = await $fetch(`${baseURL}/admin/user/auth/refresh`, {
        method: 'POST', body: { refresh_token: refreshToken }, timeout: 30000,
      });
      return response?.data ?? response;
    });
  };

  if (process.client && !(window as any).__powerxAuthSessionListener) {
    const synchronize = () => initAuth();
    window.addEventListener(AUTH_SESSION_EVENT, synchronize);
    window.addEventListener('storage', event => {
      if (event.key === AUTH_SESSION_KEY) synchronize();
    });
    (window as any).__powerxAuthSessionListener = true;
  }

  /**
   * 登出
   */
  const logout = async () => {
    const logoutSession = readAuthSession();
    const refreshToken = logoutSession?.refreshToken;
    // Invalidate immediately so an in-flight refresh cannot resurrect this login.
    clearAuth();
    const signedOut = captureAuthSession();
    try {
      // 可以调用后端登出API
      const { logout: apiLogout } = useAuthService();
      if (refreshToken) {
        await apiLogout({ refresh_token: refreshToken }, logoutSession!.accessToken);
      }
    } catch (error) {
      console.error("登出API调用失败:", error);
    } finally {
      // 清理认证信息
      if (!isAuthSessionCurrent(signedOut)) return;

      // 清理用户store状态
      const { useUserStore } = await import("~/stores/user");
      const userStore = useUserStore();
      if (!isAuthSessionCurrent(signedOut)) return;
      if (userStore.clearUserState) {
        userStore.clearUserState();
      }

      // 清理可能的cookie和其他存储
    if (process.client) {
      // 清理特定的认证相关cookie
      const authCookies = [
        "px_token",
        "auth_token",
        "auth-token",
        "px_current_tenant_uuid",
      ];
      authCookies.forEach((cookieName) => {
        document.cookie = `${cookieName}=; expires=Thu, 01 Jan 1970 00:00:00 UTC; path=/;`;
        document.cookie = `${cookieName}=; expires=Thu, 01 Jan 1970 00:00:00 UTC; path=/; domain=${window.location.hostname};`;
      });

        // 清理sessionStorage
        ['powerx-auth-error', 'powerx-must-change-password'].forEach(key => sessionStorage.removeItem(key));

        // 清理可能的其他localStorage项
        ['__px_access_token', 'token', 'powerx-local-logged-out'].forEach(key => localStorage.removeItem(key));
      }

      await navigateTo("/users/login");
    }
  };

  return {
    isAuthenticated: readonly(isAuthenticated),
    user: readonly(user),
    token: readonly(token),
    setAuth,
    clearAuth,
    getToken,
    isTokenExpired,
    logout,
    initAuth,
    refreshAuth,
  };
};
