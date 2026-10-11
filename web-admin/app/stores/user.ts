import { defineStore } from "pinia";
import { AUTH_SESSION_KEY, AUTH_SESSION_EVENT, AuthSessionChangedError, captureAuthSession, isAuthSessionCurrent, type AuthSessionStamp } from "~/utils/auth-session";
import type {
  UserContextData,
  ContextUser,
  ContextMember,
} from "~/composables/api/services/meService";
import { useMe } from "~/composables/useMe";
import { useAuth } from "~/composables/useAuth";
import { persistTenantUUID } from "~/utils/tenant-context";

export const useUserStore = defineStore("user", {
  state: () => ({
    // 用户上下文数据
    context: null as UserContextData | null,

    // 加载状态
    isLoading: false,
    error: null as string | null,

    // 缓存时间戳
    lastFetchedAt: null as number | null,
    // 跨标签页 storage 监听是否已初始化
    storageSyncInited: false,
    contextRefreshInFlight: null as Promise<void> | null,
    lastForcedContextRefreshAt: 0,
    contextSessionStamp: null as AuthSessionStamp | null,
    contextRefreshStamp: null as AuthSessionStamp | null,
  }),

  getters: {
    // 基本用户信息
    user: (state): ContextUser | null => state.context?.user || null,

    // 是否为Root用户
    isRoot: (state): boolean => state.context?.is_root || false,

    // 当前租户 UUID
    currentTenantUuid: (state): string | null =>
      state.context?.current_tenant_uuid || null,

    // 当前成员ID
    currentMemberId: (state): number | null =>
      state.context?.current_member_id || null,

    // 当前成员 UUID
    currentMemberUuid: (state): string | null =>
      state.context?.current_member_uuid ||
      state.context?.members?.find(
        (m: ContextMember) =>
          m.tenant_uuid === state.context?.current_tenant_uuid
      )?.member_uuid ||
      null,

    // 用户所属的租户列表
    memberTenants: (state): ContextMember[] => state.context?.members || [],

    // 当前租户信息
    currentTenant: (state): ContextMember | null => {
      if (!state.context?.current_tenant_uuid || !state.context?.members)
        return null;
      return (
        state.context.members.find(
          (m: ContextMember) =>
            m.tenant_uuid === state.context!.current_tenant_uuid
        ) || null
      );
    },

    // 是否为当前租户所有者
    isCurrentTenantOwner: (state): boolean => {
      if (state.context?.is_root) return false;
      const currentTenant =
        state.context?.members?.find(
          (m: ContextMember) =>
            m.tenant_uuid === state.context!.current_tenant_uuid
        ) || null;
      return currentTenant?.is_owner || false;
    },

    // 是否为当前租户的管理员
    isCurrentTenantAdmin: (state): boolean => {
      if (state.context?.is_root) return false;
      const currentTenant =
        state.context?.members?.find(
          (m: ContextMember) =>
            m.tenant_uuid === state.context!.current_tenant_uuid
        ) || null;
      return currentTenant?.is_admin || false;
    },

    // 用户显示名称
    displayName: (state): string => {
      return (
        state.context?.user?.display_name ||
        state.context?.user?.email ||
        "未知用户"
      );
    },

    // 用户头像
    avatarUrl: (state): string => {
      return state.context?.user?.avatar_url || "";
    },

    // 用户状态是否正常
    isActive: (state): boolean => {
      return state.context?.user?.status === 1;
    },

    // 是否已登录
    isLoggedIn: (state): boolean => {
      return !!state.context?.user;
    },
  },

  actions: {
    shouldUseCachedContext(force: boolean) {
      if (force) return false;
      if (!this.lastFetchedAt || !isAuthSessionCurrent(this.contextSessionStamp)) return false;
      return Date.now() - this.lastFetchedAt < 5 * 60 * 1000;
    },

    invalidateContextCache() {
      this.lastFetchedAt = null;
    },

    // 加载用户上下文
    async fetchUserContext({ force = false }: { force?: boolean } = {}) {
      const stamp = captureAuthSession();
      if (this.contextRefreshInFlight && isAuthSessionCurrent(this.contextRefreshStamp)) {
        return this.contextRefreshInFlight;
      }

      if (force && isAuthSessionCurrent(this.contextSessionStamp) && Date.now() - this.lastForcedContextRefreshAt < 1000) {
        return;
      }

      // 如果不强制刷新且缓存未过期（5分钟），直接返回
      if (this.shouldUseCachedContext(force)) {
        return;
      }

      const run = async () => {
        this.isLoading = true;
        this.error = null;
        const { getUserContext } = useMe();
        const response = await getUserContext();
        if (!isAuthSessionCurrent(stamp)) throw new AuthSessionChangedError();
        this.contextSessionStamp = stamp;
        this.context = response;
        this.lastFetchedAt = Date.now();
        if (force) {
          this.lastForcedContextRefreshAt = this.lastFetchedAt;
          const menuRefreshToken = useState<number>("px-menu-refresh-token", () => 0);
          menuRefreshToken.value += 1;
        }
        this.persistCurrentTenantUUID();
      };

      const inflight = run();
      this.contextRefreshInFlight = inflight;
      this.contextRefreshStamp = stamp;
      try {
        await inflight;
      } catch (error: any) {
        if (!isAuthSessionCurrent(stamp)) throw new AuthSessionChangedError();
        this.error = error?.message || "网络请求失败";
        console.error("获取用户上下文失败:", error);
        throw error;
      } finally {
        if (this.contextRefreshInFlight === inflight) {
          this.isLoading = false;
          this.contextRefreshInFlight = null;
          this.contextRefreshStamp = null;
        }
      }
    },

    // 切换当前租户
    async switchTenant(tenantUuid: string) {
      // 检查用户是否有权限访问该租户
      const targetTenant = this.memberTenants.find(
        (m: ContextMember) => m.tenant_uuid === tenantUuid
      );
      if (!targetTenant && !this.isRoot) {
        throw new Error("您没有权限访问该租户");
      }

      const stamp = captureAuthSession();
      try {
        const { switchTenant } = useMe();
        const response = await switchTenant(tenantUuid);
        if (!isAuthSessionCurrent(stamp)) throw new AuthSessionChangedError();
        const { setAuth } = useAuth();
        setAuth({
          token_type: response.token_type || "Bearer",
          access_token: response.access_token,
          refresh_token: response.refresh_token,
          expires_in: response.expires_in || 3600,
          scope: response.scope || "access",
        });

        this.contextSessionStamp = captureAuthSession();
        this.context = response.context;
        this.lastFetchedAt = Date.now();
        const menuRefreshToken = useState<number>("px-menu-refresh-token", () => 0);
        menuRefreshToken.value += 1;
        this.persistCurrentTenantUUID();
      } catch (error: any) {
        if (error instanceof AuthSessionChangedError) throw error;
        console.error("切换租户失败:", error);
        throw new Error(error?.message || "切换租户失败");
      }
    },

    // 更新用户信息（本地更新，不调用API）
    updateUserInfo(userInfo: Partial<ContextUser>) {
      if (this.context?.user) {
        this.context.user = { ...this.context.user, ...userInfo };
      }
    },

    // 清除用户状态（退出登录时调用）
    clearUserState() {
      this.context = null;
      this.contextSessionStamp = null;
      this.error = null;
      this.lastFetchedAt = null;
      this.isLoading = false;
      this.persistCurrentTenantUUID();
    },

    initStorageSync(options?: {
      shouldSync?: () => boolean;
      onUnauthorized?: () => void;
    }) {
      if (!process.client || this.storageSyncInited) return;
      this.storageSyncInited = true;
      const shouldSync = options?.shouldSync ?? (() => true);

      const extractStatusCode = (error: any): number => {
        const direct = Number(
          error?.status || error?.statusCode || error?.response?.status || 0
        );
        if (direct > 0) return direct;
        const cause = error?.cause;
        return Number(
          cause?.status || cause?.statusCode || cause?.response?.status || 0
        );
      };

      const synchronize = async () => {
        const stamp = captureAuthSession();
        if (!shouldSync()) {
          this.clearUserState();
          return;
        }
        this.invalidateContextCache();
        try { await this.fetchUserContext({ force: true }); }
        catch (error: any) {
          if (error instanceof AuthSessionChangedError || !isAuthSessionCurrent(stamp)) return;
          if (extractStatusCode(error) === 401) options?.onUnauthorized?.();
        }
      };
      window.addEventListener("storage", event => {
        if (event.key === AUTH_SESSION_KEY && event.oldValue !== event.newValue) void synchronize();
      });
      window.addEventListener(AUTH_SESSION_EVENT, () => void synchronize());
    },

    // 刷新用户上下文
    async refreshUserContext() {
      await this.fetchUserContext({ force: true });
    },

    persistCurrentTenantUUID() {
      const uuid = this.context?.current_tenant_uuid?.trim();
      if (uuid) {
        persistTenantUUID(uuid);
      } else {
        persistTenantUUID(null);
      }
    },
  },
});

// 导出类型供其他地方使用
export type UserStoreState = ReturnType<typeof useUserStore>;
