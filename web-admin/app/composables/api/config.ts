/**
 * API配置 - 统一管理所有服务的基础URL
 */

export const API_CONFIG = {
  // 管理员相关的Agent API
  ADMIN_AGENTS: "/admin/agents",

  // 用户交互相关的Agent API
  AGENTS: "/agents",

  // 其他服务的基础URL可以在这里添加
  // USERS: '/users',
  // PRODUCTS: '/products',
  // etc...
} as const;

/**
 * API端点构建器
 */
export class ApiEndpoints {
  /**
   * 管理员Agent相关端点
   */
  static readonly ADMIN_AGENTS = {
    PROVIDERS: `${API_CONFIG.ADMIN_AGENTS}/providers`,
    MODELS: `${API_CONFIG.ADMIN_AGENTS}/models`,
    SETTINGS_SAVE: `${API_CONFIG.ADMIN_AGENTS}/settings/save`,
    SETTINGS_ACTIVE: `${API_CONFIG.ADMIN_AGENTS}/settings/active`,
    TEST_CONNECTION: `${API_CONFIG.ADMIN_AGENTS}/test/connection`,
    TEST_CALL: `${API_CONFIG.ADMIN_AGENTS}/test/call`,
    PROFILES: `${API_CONFIG.ADMIN_AGENTS}/settings/profiles`,
    CREDENTIALS: `${API_CONFIG.ADMIN_AGENTS}/settings/credentials`,
    SKILLS_SOURCE_POLICY: `${API_CONFIG.ADMIN_AGENTS}/settings/skills/source-policy`,
    COST_USAGE_REPORT: `/internal/provider-usage/report`,
    COST_QUOTAS: `/internal/provider-quotas`,
    COST_ENFORCE: `/internal/provider-quotas/enforce`,
  } as const;

  /**
   * 用户Agent交互相关端点
   */
  static readonly AGENTS = {
    STATUS: `${API_CONFIG.AGENTS}/status`,
    INTENT: `${API_CONFIG.AGENTS}/intent`,
    PLAN_PREVIEW: `${API_CONFIG.AGENTS}/plan/preview`,
    CHAT_STREAM: `${API_CONFIG.AGENTS}/chat/stream`,
    CHAT: `${API_CONFIG.AGENTS}/chat`,
  } as const;
}
