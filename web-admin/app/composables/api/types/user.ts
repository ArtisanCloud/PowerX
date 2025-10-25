// 用户状态枚举
export enum UserStatus {
  Inactive = 0,
  Active = 1,
  Suspended = 2,
}

// 成员状态枚举
export enum MemberStatus {
  Inactive = 0,
  Active = 1,
  Suspended = 2,
}

// 状态映射工具函数 - 返回i18n键值
export const getUserStatusText = (status: UserStatus | number): string => {
  switch (status) {
    case UserStatus.Active:
      return "organization.user.status.active";
    case UserStatus.Inactive:
      return "organization.user.status.inactive";
    case UserStatus.Suspended:
      return "organization.user.status.suspended";
    default:
      return "organization.user.status.inactive";
  }
};

export const getMemberStatusText = (status: MemberStatus | number): string => {
  switch (status) {
    case MemberStatus.Active:
      return "organization.user.status.active";
    case MemberStatus.Inactive:
      return "organization.user.status.inactive";
    case MemberStatus.Suspended:
      return "organization.user.status.suspended";
    default:
      return "organization.user.status.inactive";
  }
};

// 状态显示名称映射 - 返回用于显示的键值
export const getStatusDisplayKey = (
  status: UserStatus | MemberStatus | number
): string => {
  switch (status) {
    case UserStatus.Active:
    case MemberStatus.Active:
      return "active";
    case UserStatus.Inactive:
    case MemberStatus.Inactive:
      return "inactive";
    case UserStatus.Suspended:
    case MemberStatus.Suspended:
      return "suspended";
    default:
      return "inactive";
  }
};

// 状态颜色映射
export const getStatusColor = (
  status: UserStatus | MemberStatus | number
): string => {
  const statusKey = getStatusDisplayKey(status);
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
