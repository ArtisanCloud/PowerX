// 租户状态枚举
export enum TenantStatus {
  Inactive = 0,
  Active = 1,
  Suspended = 2,
}

// 租户套餐枚举
export enum TenantPlan {
  Free = "free",
  Pro = "pro",
  Enterprise = "enterprise",
}

// 租户状态映射工具函数 - 返回i18n键值
export const getTenantStatusText = (status: TenantStatus | number): string => {
  switch (status) {
    case TenantStatus.Active:
      return "organization.user.status.active";
    case TenantStatus.Inactive:
      return "organization.user.status.inactive";
    case TenantStatus.Suspended:
      return "organization.user.status.suspended";
    default:
      return "organization.user.status.inactive";
  }
};

// 租户套餐映射工具函数 - 返回i18n键值
export const getTenantPlanText = (plan: TenantPlan | string): string => {
  switch (plan) {
    case TenantPlan.Free:
      return "organization.user.plan.free";
    case TenantPlan.Pro:
      return "organization.user.plan.pro";
    case TenantPlan.Enterprise:
      return "organization.user.plan.enterprise";
    default:
      return `organization.user.plan.${plan}`;
  }
};

// 租户状态显示名称映射 - 返回用于显示的键值
export const getTenantStatusDisplayKey = (
  status: TenantStatus | number
): string => {
  switch (status) {
    case TenantStatus.Active:
      return "active";
    case TenantStatus.Inactive:
      return "inactive";
    case TenantStatus.Suspended:
      return "suspended";
    default:
      return "inactive";
  }
};

// 租户状态颜色映射
export const getTenantStatusColor = (status: TenantStatus | number): string => {
  const statusKey = getTenantStatusDisplayKey(status);
  switch (statusKey) {
    case "active":
      return "success";
    case "inactive":
      return "neutral";
    case "suspended":
      return "warning";
    default:
      return "neutral";
  }
};
