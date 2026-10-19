import { useNotificationReadsStore } from '~/stores/notificationReads';
import { notificationNavigationPath, primaryNotificationNavigation } from '~/utils/notification-navigation';
import type {
  Notification,
  NotificationFilter,
  NotificationStats,
} from "~/types/notification";
import {
  useNotificationService,
  type NotificationRecord,
} from "~/composables/api/services/notificationService";
import { computed, onScopeDispose, ref, readonly, watch } from "vue";
import { useUserStore } from "~/stores/user";
import { AUTH_SESSION_EVENT, AUTH_SESSION_KEY, captureAuthSession, readAuthSession } from "~/utils/auth-session";

export const useNotifications = (initialFilter: NotificationFilter = {}) => {
  const notifications = ref<Notification[]>([]);
  const loading = ref(false);
  const error = ref<string | null>(null);
  const currentPage = ref(1);
  const pageSize = ref(20);
  const totalCount = ref(0);
  const filter = ref<NotificationFilter>({ ...initialFilter });

  const api = useNotificationService();
  const userStore = useUserStore();
  const identity = computed(() => [userStore.user?.id || "", userStore.currentTenantUuid || "", userStore.currentMemberUuid || ""].join(":"));
  const sessionId = ref(captureAuthSession()?.sessionId || "");
  const identityScope = computed(() => `${identity.value}:${sessionId.value}`);
  const identityReady = computed(() => {
    void identityScope.value;
    const session = readAuthSession();
    return session?.status === 'active'
      && session.sessionId === userStore.contextSessionStamp?.sessionId
      && session.tenantUuid === userStore.currentTenantUuid
      && Boolean(userStore.currentMemberUuid);
  });
  const synchronizeSession = () => { sessionId.value = captureAuthSession()?.sessionId || ""; };
  const synchronizeStorage = (event: StorageEvent) => {
    if (event.key === AUTH_SESSION_KEY || event.key === null) synchronizeSession();
  };
  let disposed = false;
  if (process.client) {
    window.addEventListener(AUTH_SESSION_EVENT, synchronizeSession);
    window.addEventListener("storage", synchronizeStorage);
  }
  onScopeDispose(() => {
    disposed = true;
    if (!process.client) return;
    window.removeEventListener(AUTH_SESSION_EVENT, synchronizeSession);
    window.removeEventListener("storage", synchronizeStorage);
  });
  const readStore = useNotificationReadsStore();
  const readScope = () => `${identity.value}:${captureAuthSession()?.sessionId || ""}`;
  readStore.bind(readScope());
  let hasRequestedList = false;
  let refreshQueued = false;
  const refreshFilteredList = () => {
    if (!hasRequestedList || typeof filter.value.isRead !== "boolean" || refreshQueued) return;
    refreshQueued = true;
    const scope = readScope();
    // 合并同一批回执造成的刷新；只有变更事件触发，不周期查询。
    void Promise.resolve().then(() => {
      refreshQueued = false;
      if (!disposed && scope === readScope()) void fetchNotifications();
    });
  };
  watch(() => readStore.revision, () => {
    if (readStore.scope !== readScope()) return;
    const before = notifications.value.length;
    notifications.value = notifications.value.map(n => readStore.reads[n.id] ? { ...n, isRead: true } : n).filter(matchesFilter);
    if (filter.value.isRead === false) totalCount.value = Math.max(0, totalCount.value - before + notifications.value.length);
    refreshFilteredList();
  }, { flush: "sync" });
  let generation = 0;
  let listRequest = 0;
  const stamp = () => `${generation}:${identity.value}:${captureAuthSession()?.sessionId || ""}`;
  const accepts = (input: { userId?: string }) => input.userId === undefined || input.userId === "" || (typeof input.userId === "string" && input.userId.toLowerCase() === String(userStore.currentMemberUuid || "").toLowerCase());
  watch(identityScope, () => {
    generation += 1;
    listRequest += 1;
    hasRequestedList = false;
    notifications.value = [];
    totalCount.value = 0;
    currentPage.value = 1;
    loading.value = false;
    error.value = null;
    readStore.bind(readScope());
  }, { flush: "sync" });

  const normalizeDate = (value?: Date | string | null) => {
    if (!value) return new Date();
    if (value instanceof Date) return value;
    const parsed = new Date(value);
    return Number.isNaN(parsed.getTime()) ? new Date() : parsed;
  };

  const normalizeRecord = (record: NotificationRecord): Notification => {
    const createdAt = normalizeDate(record.createdAt);
    return {
      id: record.id,
      title: record.title,
      content: record.content,
      type: record.type as Notification["type"],
      category: record.category as Notification["category"],
      isRead: record.isRead || readStore.reads[record.id] === true,
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
    if (disposed) return;
    hasRequestedList = true;
    const request = ++listRequest;
    const requestIdentity = stamp();
    const isCurrentRequest = () => !disposed && request === listRequest && requestIdentity === stamp();
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
      if (!isCurrentRequest()) return;
      const items = (data?.items || []).filter(accepts);
      notifications.value = items.map(normalizeRecord).filter(matchesFilter);
      const confirmedReads = filter.value.isRead === false
        ? items.filter(item => !item.isRead && readStore.reads[item.id]).length
        : 0;
      totalCount.value = Math.max(0, (data?.pagination?.total ?? items.length) - confirmedReads);
    } catch (err) {
      if (!isCurrentRequest()) return;
      error.value = "获取通知失败";
      console.error("获取通知失败:", err);
    } finally {
      if (isCurrentRequest()) loading.value = false;
    }
  };

  const addNotification = async (input: Partial<Notification>) => {
    // Only persisted notifications belong in the bell. Task/progress events
    // without a server identity must never become unresolvable local records.
    if (typeof input.id !== 'string' || !input.id.trim() || !userStore.currentTenantUuid || !accepts(input)) return;
    if (notifications.value.some(n => n.id === input.id)) return;
    const createdAt = normalizeDate(input.createdAt || input.updatedAt || null);
    const payload: Notification = {
      id: input.id,
      title: input.title || "新通知",
      content: input.content || "",
      type: (input.type || "info") as Notification["type"],
      category: (input.category || "system") as Notification["category"],
      isRead: input.isRead === true || readStore.reads[input.id || ""] === true,
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
    const requestIdentity = stamp();
    const { data } = await api.get(id);
    if (requestIdentity !== stamp() || !data || !accepts(data)) return null;
    return normalizeRecord(data);
  };

  // 标记为已读
  const markAsRead = async (id: string) => {
    const requestIdentity = stamp();
    await api.markRead(id);
    if (requestIdentity !== stamp()) return;
    readStore.record(readScope(), [id]);
  };

  // 批量标记为已读
  const markAllAsRead = async () => {
    const requestIdentity = stamp();
    const unread = notifications.value.filter((n) => !n.isRead);
    await Promise.all(unread.map((n) => api.markRead(n.id)));
    if (requestIdentity !== stamp()) return;
    readStore.record(readScope(), unread.map(n => n.id));
  };

  // 删除通知
  const deleteNotification = async (id: string) => {
    const requestIdentity = stamp();
    await api.delete(id);
    if (requestIdentity !== stamp()) return;
    notifications.value = notifications.value.filter((n) => n.id !== id);
    if (totalCount.value > 0) totalCount.value -= 1;
  };

  // 清空通知
  const clearAllNotifications = async () => {
    const requestIdentity = stamp();
    await api.clearAll();
    if (requestIdentity !== stamp()) return;
    notifications.value = [];
    totalCount.value = 0;
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

  const isNotificationRead = (notification: Pick<Notification, "id" | "isRead">) => notification.isRead || readStore.reads[notification.id] === true;

  const applyReadReceipt = (input: any) => {
    if (input?.eventType !== 'notification_read' || input?.payload_version !== 1 || input?.contract !== 'notification_read_receipt.v1' || input?.tenant_uuid !== userStore.currentTenantUuid || input?.userId !== userStore.currentMemberUuid || !Array.isArray(input?.notification_uuids) || input.notification_uuids.length < 1 || input.notification_uuids.length > 100 || input.notification_uuids.some((id: unknown) => typeof id !== 'string' || !/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(id))) return;
    readStore.record(readScope(), input.notification_uuids);
  };

  const openNotification = async (notificationId: string) => {
    const notification = notifications.value.find(n => n.id === notificationId);
    if (!notification) return;
    const action = primaryNotificationNavigation(notification);
    if (action) { await executeAction(notificationId, action.id); return; }
    await navigateTo(`/notifications?id=${encodeURIComponent(notificationId)}`);
  };

  // 执行通知操作
  const executeAction = async (notificationId: string, actionId: string, displayed?: Notification) => {
    const notification = notifications.value.find((n) => n.id === notificationId) || (displayed?.id === notificationId && accepts(displayed) ? displayed : undefined);
    const action = notification?.actions?.find((a) => a.id === actionId);

    if (!action) return;
    const requestStamp = stamp();

    switch (action.action) {
      case "navigate":
        await navigateTo(notificationNavigationPath(action.params?.path));
        break;
      case "retry":
        console.info("重试操作:", action.params);
        break;
      case "update":
        console.info("更新操作:", action.params);
        break;
      default:
        console.info("执行操作:", action.action, action.params);
    }

    // 标记为已读
    if (requestStamp === stamp()) await markAsRead(notificationId);
  };

  watch([currentPage, pageSize], () => {
    fetchNotifications();
  });

  return {
    identityScope,
    identityReady,
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
    clearAllNotifications,
    getStats,
    setFilter,
    clearFilter,
    executeAction,
    openNotification,
    applyReadReceipt,
    isNotificationRead,
    addNotification,
  };
};
