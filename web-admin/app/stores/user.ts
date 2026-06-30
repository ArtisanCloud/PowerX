import { defineStore } from "pinia";
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
      if (!this.lastFetchedAt) return false;
      return Date.now() - this.lastFetchedAt < 5 * 60 * 1000;
    },

    invalidateContextCache() {
      this.lastFetchedAt = null;
    },

    // 加载用户上下文
    async fetchUserContext({ force = false }: { force?: boolean } = {}) {
      if (this.contextRefreshInFlight) {
        return this.contextRefreshInFlight;
      }

      if (force && Date.now() - this.lastForcedContextRefreshAt < 1000) {
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

        this.context = response;
        this.lastFetchedAt = Date.now();
        if (force) {
          this.lastForcedContextRefreshAt = this.lastFetchedAt;
        }
        this.persistCurrentTenantUUID();
      };

      const inflight = run();
      this.contextRefreshInFlight = inflight;
      try {
        await inflight;
      } catch (error: any) {
        this.error = error?.message || "网络请求失败";
        console.error("获取用户上下文失败:", error);
        throw error;
      } finally {
        this.isLoading = false;
        if (this.contextRefreshInFlight === inflight) {
          this.contextRefreshInFlight = null;
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

      try {
        const { switchTenant } = useMe();
        const response = await switchTenant(tenantUuid);
        const { setAuth } = useAuth();
        setAuth({
          token_type: response.token_type || "Bearer",
          access_token: response.access_token,
          refresh_token: response.refresh_token,
          expires_in: response.expires_in || 3600,
          scope: response.scope || "access",
        });

        this.context = response.context;
        this.lastFetchedAt = Date.now();
        this.persistCurrentTenantUUID();
      } catch (error: any) {
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

      window.addEventListener("storage", async (event: StorageEvent) => {
        const key = event.key || "";
        if (!key) return;
        if (
          key !== "px_current_tenant_uuid" &&
          key !== "access_token" &&
          key !== "refresh_token" &&
          key !== "expires_at"
        ) {
          return;
        }
        if (event.oldValue === event.newValue) {
          return;
        }

        if (!shouldSync()) {
          this.clearUserState();
          return;
        }

        this.invalidateContextCache();
        try {
          await this.fetchUserContext({ force: true });
        } catch (error: any) {
          const statusCode = extractStatusCode(error);
          if (statusCode === 401 || statusCode === 403) {
            options?.onUnauthorized?.();
          }
        }
      });
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
