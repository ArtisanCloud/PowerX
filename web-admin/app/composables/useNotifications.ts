import type {
  Notification,
  NotificationFilter,
  NotificationStats,
} from "~/types/notification";

export const useNotifications = () => {
  const notifications = ref<Notification[]>([]);
  const loading = ref(false);
  const error = ref<string | null>(null);
  const currentPage = ref(1);
  const pageSize = ref(20);
  const totalCount = ref(0);
  const filter = ref<NotificationFilter>({});

  // 模拟数据
  const mockNotifications: Notification[] = [
    {
      id: "1",
      title: "Agent 部署成功",
      content: '您的智能客服 Agent "PowerX Assistant" 已成功部署并开始运行。',
      type: "success",
      category: "agent",
      isRead: false,
      isImportant: true,
      createdAt: new Date(Date.now() - 300000),
      updatedAt: new Date(Date.now() - 300000),
      relatedId: "agent-001",
      relatedType: "agent",
      actions: [
        {
          id: "view-agent",
          label: "查看 Agent",
          type: "primary",
          action: "navigate",
          params: { path: "/agent/agent-001" },
        },
      ],
    },
    {
      id: "2",
      title: "工作流执行异常",
      content: '工作流 "订单处理流程" 在执行过程中遇到错误，请检查配置。',
      type: "error",
      category: "workflow",
      isRead: false,
      isImportant: true,
      createdAt: new Date(Date.now() - 600000),
      updatedAt: new Date(Date.now() - 600000),
      relatedId: "workflow-002",
      relatedType: "workflow",
      actions: [
        {
          id: "view-workflow",
          label: "查看工作流",
          type: "primary",
          action: "navigate",
          params: { path: "/workflow/workflow-002" },
        },
        {
          id: "retry-workflow",
          label: "重试执行",
          type: "secondary",
          action: "retry",
          params: { workflowId: "workflow-002" },
        },
      ],
    },
    {
      id: "3",
      title: "新用户注册",
      content: '用户 "张三" 已成功注册并完成邮箱验证。',
      type: "info",
      category: "user",
      isRead: true,
      isImportant: false,
      createdAt: new Date(Date.now() - 900000),
      updatedAt: new Date(Date.now() - 900000),
      relatedId: "user-003",
      relatedType: "user",
    },
    {
      id: "4",
      title: "系统维护通知",
      content: "系统将于今晚 23:00-01:00 进行维护升级，期间服务可能暂时中断。",
      type: "warning",
      category: "system",
      isRead: false,
      isImportant: true,
      createdAt: new Date(Date.now() - 1200000),
      updatedAt: new Date(Date.now() - 1200000),
    },
    {
      id: "5",
      title: "插件更新可用",
      content: '插件 "数据分析助手" 有新版本可用，建议及时更新。',
      type: "info",
      category: "plugin",
      isRead: true,
      isImportant: false,
      createdAt: new Date(Date.now() - 1800000),
      updatedAt: new Date(Date.now() - 1800000),
      relatedId: "plugin-005",
      relatedType: "plugin",
      actions: [
        {
          id: "update-plugin",
          label: "立即更新",
          type: "primary",
          action: "update",
          params: { pluginId: "plugin-005" },
        },
      ],
    },
    {
      id: "6",
      title: "Token 余额不足",
      content: "您的 Token 余额已不足 1000，请及时充值以确保服务正常运行。",
      type: "warning",
      category: "system",
      isRead: false,
      isImportant: true,
      createdAt: new Date(Date.now() - 2400000),
      updatedAt: new Date(Date.now() - 2400000),
      actions: [
        {
          id: "recharge",
          label: "立即充值",
          type: "primary",
          action: "navigate",
          params: { path: "/billing/recharge" },
        },
      ],
    },
    {
      id: "7",
      title: "新订单提醒",
      content: "您有一个新的订单 #12345 需要处理。",
      type: "info",
      category: "order",
      isRead: true,
      isImportant: false,
      createdAt: new Date(Date.now() - 3600000),
      updatedAt: new Date(Date.now() - 3600000),
      relatedId: "order-12345",
      relatedType: "order",
      actions: [
        {
          id: "view-order",
          label: "查看订单",
          type: "primary",
          action: "navigate",
          params: { path: "/orders/order-12345" },
        },
      ],
    },
  ];

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

  // 获取通知列表
  const fetchNotifications = async () => {
    loading.value = true;
    error.value = null;

    try {
      // 模拟 API 调用
      await new Promise((resolve) => setTimeout(resolve, 500));

      let filteredNotifications = [...mockNotifications];

      // 应用过滤器
      if (filter.value.category) {
        filteredNotifications = filteredNotifications.filter(
          (n) => n.category === filter.value.category
        );
      }
      if (filter.value.type) {
        filteredNotifications = filteredNotifications.filter(
          (n) => n.type === filter.value.type
        );
      }
      if (filter.value.isRead !== undefined) {
        filteredNotifications = filteredNotifications.filter(
          (n) => n.isRead === filter.value.isRead
        );
      }
      if (filter.value.isImportant !== undefined) {
        filteredNotifications = filteredNotifications.filter(
          (n) => n.isImportant === filter.value.isImportant
        );
      }

      // 分页
      const start = (currentPage.value - 1) * pageSize.value;
      const end = start + pageSize.value;

      totalCount.value = filteredNotifications.length;
      notifications.value = filteredNotifications.slice(start, end);
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
      type: input.type || "info",
      category: input.category || "system",
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
    mockNotifications.unshift(payload);
    await fetchNotifications();
  };

  // 获取单个通知详情
  const getNotification = async (id: string): Promise<Notification | null> => {
    const notification = mockNotifications.find((n) => n.id === id);
    return notification ? { ...notification } : null;
  };

  // 标记为已读
  const markAsRead = async (id: string) => {
    const notification = mockNotifications.find((n) => n.id === id);
    if (notification) {
      notification.isRead = true;
      notification.updatedAt = new Date();
    }
    await fetchNotifications();
  };

  // 批量标记为已读
  const markAllAsRead = async () => {
    mockNotifications.forEach((n) => {
      n.isRead = true;
      n.updatedAt = new Date();
    });
    await fetchNotifications();
  };

  // 删除通知
  const deleteNotification = async (id: string) => {
    const index = mockNotifications.findIndex((n) => n.id === id);
    if (index > -1) {
      mockNotifications.splice(index, 1);
    }
    await fetchNotifications();
  };

  // 获取统计信息
  const getStats = (): NotificationStats => {
    const total = mockNotifications.length;
    const unread = mockNotifications.filter((n) => !n.isRead).length;
    const important = mockNotifications.filter((n) => n.isImportant).length;

    const byCategory: Record<string, number> = {};
    const byType: Record<string, number> = {};

    mockNotifications.forEach((n) => {
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
