import { defineStore } from "pinia";
import type {
  UserContextData,
  ContextUser,
  ContextMember,
} from "~/composables/api/services/meService";
import { useMe } from "~/composables/useMe";

export const useUserStore = defineStore("user", {
  state: () => ({
    // 用户上下文数据
    context: null as UserContextData | null,

    // 加载状态
    isLoading: false,
    error: null as string | null,

    // 缓存时间戳
    lastFetchedAt: null as number | null,
  }),

  getters: {
    // 基本用户信息
    user: (state): ContextUser | null => state.context?.user || null,

    // 是否为Root用户
    isRoot: (state): boolean => state.context?.is_root || false,

    // 当前租户ID
    currentTenantId: (state): number | null =>
      state.context?.current_tenant_id || null,

    // 当前成员ID
    currentMemberId: (state): number | null =>
      state.context?.current_member_id || null,

    // 用户所属的租户列表
    memberTenants: (state): ContextMember[] => state.context?.members || [],

    // 当前租户信息
    currentTenant: (state): ContextMember | null => {
      if (!state.context?.current_tenant_id || !state.context?.members)
        return null;
      return (
        state.context.members.find(
          (m: ContextMember) => m.tenant_id === state.context!.current_tenant_id
        ) || null
      );
    },

    // 是否为当前租户的管理员
    isCurrentTenantAdmin: (state): boolean => {
      if (state.context?.is_root) return true;
      const currentTenant = state.context?.members?.find(
        (m: ContextMember) => m.tenant_id === state.context!.current_tenant_id
      );
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

    // 获取用户在指定租户中的角色
    getTenantRole:
      (state) =>
      (tenantId: number): ContextMember | null => {
        return (
          state.context?.members?.find(
            (m: ContextMember) => m.tenant_id === tenantId
          ) || null
        );
      },
  },

  actions: {
    // 加载用户上下文
    async fetchUserContext({ force = false }: { force?: boolean } = {}) {
      // 如果正在加载，直接返回
      if (this.isLoading) return;

      // 如果不强制刷新且缓存未过期（5分钟），直接返回
      if (
        !force &&
        this.lastFetchedAt &&
        Date.now() - this.lastFetchedAt < 5 * 60 * 1000
      ) {
        return;
      }

      this.isLoading = true;
      this.error = null;

      try {
        const { getUserContext } = useMe();
        const response = await getUserContext();

        this.context = response;
        this.lastFetchedAt = Date.now();
      } catch (error: any) {
        this.error = error?.message || "网络请求失败";
        console.error("获取用户上下文失败:", error);
        throw error;
      } finally {
        this.isLoading = false;
      }
    },

    // 切换当前租户
    async switchTenant(tenantId: number) {
      // 检查用户是否有权限访问该租户
      const targetTenant = this.memberTenants.find(
        (m: ContextMember) => m.tenant_id === tenantId
      );
      if (!targetTenant && !this.isRoot) {
        throw new Error("您没有权限访问该租户");
      }

      try {
        const { switchTenant } = useMe();
        const response = await switchTenant(tenantId);

        // 直接更新上下文，无需再次请求
        this.context = response;
        this.lastFetchedAt = Date.now();
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
    },

    // 刷新用户上下文
    async refreshUserContext() {
      await this.fetchUserContext({ force: true });
    },
  },
});

// 导出类型供其他地方使用
export type UserStoreState = ReturnType<typeof useUserStore>;
