// 消息通知类型定义
export interface Notification {
  id: string;
  title: string;
  content: string;
  type: "info" | "success" | "warning" | "error" | "system";
  category: "system" | "agent" | "workflow" | "user" | "order" | "plugin";
  isRead: boolean;
  isImportant: boolean;
  createdAt: Date;
  updatedAt: Date;
  userId?: string;
  relatedId?: string; // 关联的资源ID
  relatedType?: string; // 关联的资源类型
  actions?: NotificationAction[];
  metadata?: Record<string, any>;
}

export interface NotificationAction {
  id: string;
  label: string;
  type: "primary" | "secondary" | "danger";
  action: string;
  params?: Record<string, any>;
}

// 消息通知常量
export const NOTIFICATION_TYPES = {
  INFO: "info",
  SUCCESS: "success",
  WARNING: "warning",
  ERROR: "error",
  SYSTEM: "system",
} as const;

export const NOTIFICATION_CATEGORIES = {
  SYSTEM: "system",
  AGENT: "agent",
  WORKFLOW: "workflow",
  USER: "user",
  ORDER: "order",
  PLUGIN: "plugin",
} as const;

// 消息通知过滤器
export interface NotificationFilter {
  category?: string;
  type?: string;
  isRead?: boolean;
  isImportant?: boolean;
  dateRange?: {
    start: Date;
    end: Date;
  };
}

// 消息通知统计
export interface NotificationStats {
  total: number;
  unread: number;
  important: number;
  byCategory: Record<string, number>;
  byType: Record<string, number>;
}
