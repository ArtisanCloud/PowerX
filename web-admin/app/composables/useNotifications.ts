import type {
  Notification,
  NotificationFilter,
  NotificationStats,
} from "~/types/notification";
import {
  useNotificationService,
  type NotificationRecord,
} from "~/composables/api/services/notificationService";

export const useNotifications = () => {
  const notifications = ref<Notification[]>([]);
  const loading = ref(false);
  const error = ref<string | null>(null);
  const currentPage = ref(1);
  const pageSize = ref(20);
  const totalCount = ref(0);
  const filter = ref<NotificationFilter>({});

  const api = useNotificationService();

  const normalizeDate = (value?: Date | string | null) => {
    if (!value) return new Date();
    if (value instanceof Date) return value;
    const parsed = new Date(value);
    return Number.isNaN(parsed.getTime()) ? new Date() : parsed;
  };

  const nextId = () => {
    if (process.client && typeof crypto?.randomUUID === "function") {
      return crypto.randomUUID();
    }
    return `notif-${Date.now()}-${Math.random().toString(16).slice(2, 8)}`;
  };

  const normalizeRecord = (record: NotificationRecord): Notification => {
    const createdAt = normalizeDate(record.createdAt);
    return {
      id: record.id,
      title: record.title,
      content: record.content,
      type: record.type as Notification["type"],
      category: record.category as Notification["category"],
      isRead: record.isRead,
      isImportant: record.isImportant,
      createdAt,
      updatedAt: normalizeDate(record.updatedAt || createdAt),
      userId: record.userId,
      relatedId: record.relatedId,
      relatedType: record.relatedType,
      actions: record.actions as any,
      metadata: record.metadata,
    };
  };

  const matchesFilter = (notification: Notification) => {
    if (filter.value.category && notification.category !== filter.value.category) return false;
    if (filter.value.type && notification.type !== filter.value.type) return false;
    if (typeof filter.value.isRead === "boolean" && notification.isRead !== filter.value.isRead) return false;
    if (typeof filter.value.isImportant === "boolean" && notification.isImportant !== filter.value.isImportant) return false;
    return true;
  };

  // 获取通知列表
  const fetchNotifications = async () => {
    loading.value = true;
    error.value = null;

    try {
      const { data } = await api.list({
        page: currentPage.value,
        pageSize: pageSize.value,
        category: filter.value.category,
        type: filter.value.type,
        isRead: filter.value.isRead,
        isImportant: filter.value.isImportant,
      });
      const items = data?.items || [];
      notifications.value = items.map(normalizeRecord);
      totalCount.value = data?.pagination?.total ?? items.length;
    } catch (err) {
      error.value = "获取通知失败";
      console.error("获取通知失败:", err);
    } finally {
      loading.value = false;
    }
  };

  const addNotification = async (input: Partial<Notification>) => {
    const createdAt = normalizeDate(input.createdAt || input.updatedAt || null);
    const payload: Notification = {
      id: input.id || nextId(),
      title: input.title || "新通知",
      content: input.content || "",
      type: (input.type || "info") as Notification["type"],
      category: (input.category || "system") as Notification["category"],
      isRead: input.isRead ?? false,
      isImportant: input.isImportant ?? false,
      createdAt,
      updatedAt: normalizeDate(input.updatedAt || createdAt),
      userId: input.userId,
      relatedId: input.relatedId,
      relatedType: input.relatedType,
      actions: input.actions,
      metadata: input.metadata,
    };
    if (matchesFilter(payload)) {
      if (currentPage.value === 1) {
        notifications.value = [payload, ...notifications.value];
        if (notifications.value.length > pageSize.value) {
          notifications.value = notifications.value.slice(0, pageSize.value);
        }
      }
      totalCount.value += 1;
    }
  };

  // 获取单个通知详情
  const getNotification = async (id: string): Promise<Notification | null> => {
    const { data } = await api.get(id);
    if (!data) return null;
    return normalizeRecord(data);
  };

  // 标记为已读
  const markAsRead = async (id: string) => {
    await api.markRead(id);
    notifications.value = notifications.value.map((n) =>
      n.id === id ? { ...n, isRead: true, updatedAt: new Date() } : n
    );
  };

  // 批量标记为已读
  const markAllAsRead = async () => {
    const unread = notifications.value.filter((n) => !n.isRead);
    await Promise.all(unread.map((n) => api.markRead(n.id)));
    notifications.value = notifications.value.map((n) => ({
      ...n,
      isRead: true,
      updatedAt: new Date(),
    }));
  };

  // 删除通知
  const deleteNotification = async (id: string) => {
    await api.delete(id);
    notifications.value = notifications.value.filter((n) => n.id !== id);
    if (totalCount.value > 0) totalCount.value -= 1;
  };

  // 获取统计信息
  const getStats = (): NotificationStats => {
    const total = totalCount.value;
    const unread = notifications.value.filter((n) => !n.isRead).length;
    const important = notifications.value.filter((n) => n.isImportant).length;

    const byCategory: Record<string, number> = {};
    const byType: Record<string, number> = {};

    notifications.value.forEach((n) => {
      byCategory[n.category] = (byCategory[n.category] || 0) + 1;
      byType[n.type] = (byType[n.type] || 0) + 1;
    });

    return {
      total,
      unread,
      important,
      byCategory,
      byType,
    };
  };

  // 设置过滤器
  const setFilter = (newFilter: NotificationFilter) => {
    filter.value = { ...newFilter };
    currentPage.value = 1;
    fetchNotifications();
  };

  // 清空过滤器
  const clearFilter = () => {
    filter.value = {};
    currentPage.value = 1;
    fetchNotifications();
  };

  // 执行通知操作
  const executeAction = async (notificationId: string, actionId: string) => {
    const notification = mockNotifications.find((n) => n.id === notificationId);
    const action = notification?.actions?.find((a) => a.id === actionId);

    if (!action) return;

    switch (action.action) {
      case "navigate":
        if (action.params?.path) {
          await navigateTo(action.params.path);
        }
        break;
      case "retry":
        console.log("重试操作:", action.params);
        break;
      case "update":
        console.log("更新操作:", action.params);
        break;
      default:
        console.log("执行操作:", action.action, action.params);
    }

    // 标记为已读
    await markAsRead(notificationId);
  };

  watch([currentPage, pageSize], () => {
    fetchNotifications();
  });

  return {
    notifications: readonly(notifications),
    loading: readonly(loading),
    error: readonly(error),
    currentPage,
    pageSize,
    totalCount: readonly(totalCount),
    filter: readonly(filter),
    fetchNotifications,
    getNotification,
    markAsRead,
    markAllAsRead,
    deleteNotification,
    getStats,
    setFilter,
    clearFilter,
    executeAction,
    addNotification,
  };
};
