import type { LoginResponse } from "./api/services/authService";
import { useAuthService } from "./api/services/authService";
import { clearAuthCookies, syncAuthCookies } from "~/utils/auth-cookie";
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
    if (process.client) {
      // 保存到localStorage
      localStorage.setItem("access_token", authData.access_token);
      localStorage.setItem("refresh_token", authData.refresh_token);
      localStorage.setItem("token_type", authData.token_type);
      localStorage.setItem("expires_in", authData.expires_in.toString());
      localStorage.setItem("scope", authData.scope);

      // 计算过期时间
      const expiresAt = Date.now() + authData.expires_in * 1000;
      localStorage.setItem("expires_at", expiresAt.toString());

      // 同步 cookie 供 iframe/插件代理层注入 Authorization。
      syncAuthCookies(authData.access_token, authData.refresh_token);
    }

    // 更新状态
    token.value = authData.access_token;
    isAuthenticated.value = true;
  };

  /**
   * 清除认证信息
   */
  const clearAuth = () => {
    if (process.client) {
      localStorage.removeItem("access_token");
      localStorage.removeItem("refresh_token");
      localStorage.removeItem("token_type");
      localStorage.removeItem("expires_in");
      localStorage.removeItem("expires_at");
      localStorage.removeItem("px_current_tenant_uuid");

      clearAuthCookies();
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

    const expiresAt = localStorage.getItem("expires_at");
    if (!expiresAt) return true;

    return Date.now() > parseInt(expiresAt);
  };

  /**
   * 获取当前token
   */
  const getToken = (): string | null => {
    if (!process.client) return null;

    if (isTokenExpired()) {
      clearAuth();
      return null;
    }

    return localStorage.getItem("access_token");
  };

  /**
   * 初始化认证状态
   */
  const initAuth = () => {
    if (!process.client) return;

    const accessToken = localStorage.getItem("access_token");
    if (accessToken && !isTokenExpired()) {
      token.value = accessToken;
      isAuthenticated.value = true;
    } else {
      clearAuth();
    }
  };

  /**
   * 登出
   */
  const logout = async () => {
    try {
      // 可以调用后端登出API
      const { logout: apiLogout } = useAuthService();
      const refreshToken = process.client ? localStorage.getItem("refresh_token") : null;
      if (refreshToken) {
        await apiLogout({ refresh_token: refreshToken });
      }
    } catch (error) {
      console.error("登出API调用失败:", error);
    } finally {
      // 清理认证信息
      clearAuth();

      // 清理用户store状态
      const { useUserStore } = await import("~/stores/user");
      const userStore = useUserStore();
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
        "i18n_redirected",
        "px_current_tenant_uuid",
      ];
      authCookies.forEach((cookieName) => {
        document.cookie = `${cookieName}=; expires=Thu, 01 Jan 1970 00:00:00 UTC; path=/;`;
        document.cookie = `${cookieName}=; expires=Thu, 01 Jan 1970 00:00:00 UTC; path=/; domain=${window.location.hostname};`;
      });

        // 清理sessionStorage
        sessionStorage.clear();

        // 清理可能的其他localStorage项
        const keysToRemove = Object.keys(localStorage).filter(
          (key) =>
            key.includes("auth") ||
            key.includes("token") ||
            key.includes("user") ||
            key.includes("px_")
        );
        keysToRemove.forEach((key) => localStorage.removeItem(key));
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
  };
};
