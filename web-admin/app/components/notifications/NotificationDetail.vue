<script setup lang="ts">
import type { Notification } from "~/types/notification";

const props = defineProps<{
  notification: Notification | null;
}>();

const emit = defineEmits<{
  close: [];
  back: [];
}>();

const { executeAction, markAsRead } = useNotifications();

// 获取通知图标
const getNotificationIcon = (notification: Notification) => {
  switch (notification.type) {
    case "success":
      return "i-heroicons-check-circle";
    case "warning":
      return "i-heroicons-exclamation-triangle";
    case "error":
      return "i-heroicons-x-circle";
    case "info":
      return "i-heroicons-information-circle";
    default:
      return "i-heroicons-bell";
  }
};

// 获取通知颜色
const getNotificationColor = (notification: Notification) => {
  switch (notification.type) {
    case "success":
      return "text-green-500";
    case "warning":
      return "text-yellow-500";
    case "error":
      return "text-red-500";
    case "info":
      return "text-blue-500";
    default:
      return "text-gray-500";
  }
};

// 获取分类标签
const getCategoryLabel = (category: string) => {
  const categoryMap: Record<string, string> = {
    system: "系统通知",
    agent: "Agent 通知",
    workflow: "工作流通知",
    user: "用户通知",
    order: "订单通知",
    plugin: "插件通知",
  };
  return categoryMap[category] || category;
};

// 格式化完整时间
const formatFullTime = (date: Date) => {
  return date.toLocaleString("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
};

// 处理操作点击
const handleActionClick = async (actionId: string) => {
  if (!props.notification) return;
  await executeAction(props.notification.id, actionId);
};

// 标记为已读
const handleMarkAsRead = async () => {
  if (!props.notification || props.notification.isRead) return;
  await markAsRead(props.notification.id);
};

// 监听通知变化，自动标记为已读
watch(
  () => props.notification,
  (newNotification) => {
    if (newNotification && !newNotification.isRead) {
      handleMarkAsRead();
    }
  },
  { immediate: true }
);
</script>

<template>
  <div v-if="notification" class="flex flex-col h-full">
    <!-- 头部 -->
    <div class="flex items-center justify-between p-4 border-b border-gray-200">
      <div class="flex items-center space-x-3">
        <UButton
          variant="ghost"
          color="neutral"
          size="sm"
          icon="i-heroicons-arrow-left"
          @click="emit('back')"
        >
          返回
        </UButton>
        <h2 class="text-lg font-semibold text-gray-900">消息详情</h2>
      </div>
      <UButton
        variant="ghost"
        color="neutral"
        size="sm"
        icon="i-heroicons-x-mark"
        @click="emit('close')"
      />
    </div>

    <!-- 内容 -->
    <div class="flex-1 overflow-y-auto p-6">
      <!-- 通知头部信息 -->
      <div class="flex items-start space-x-4 mb-6">
        <div class="flex-shrink-0 mt-1">
          <div
            class="w-12 h-12 rounded-full flex items-center justify-center"
            :class="{
              'bg-green-100': notification.type === 'success',
              'bg-yellow-100': notification.type === 'warning',
              'bg-red-100': notification.type === 'error',
              'bg-blue-100': notification.type === 'info',
              'bg-gray-100': notification.type === 'system',
            }"
          >
            <UIcon
              :name="getNotificationIcon(notification)"
              :class="['w-6 h-6', getNotificationColor(notification)]"
            />
          </div>
        </div>
        <div class="flex-1">
          <div class="flex items-center space-x-2 mb-2">
            <h1 class="text-xl font-semibold text-gray-900">
              {{ notification.title }}
            </h1>
            <UBadge
              v-if="notification.isImportant"
              color="error"
              variant="soft"
              size="sm"
            >
              重要
            </UBadge>
            <UBadge
              v-if="!notification.isRead"
              color="info"
              variant="soft"
              size="sm"
            >
              未读
            </UBadge>
          </div>
          <div class="flex items-center space-x-4 text-sm text-gray-500">
            <span>{{ getCategoryLabel(notification.category) }}</span>
            <span>{{ formatFullTime(notification.createdAt) }}</span>
          </div>
        </div>
      </div>

      <!-- 通知内容 -->
      <div class="mb-6">
        <div class="prose prose-sm max-w-none">
          <p class="text-gray-700 leading-relaxed whitespace-pre-wrap">
            {{ notification.content }}
          </p>
        </div>
      </div>

      <!-- 关联信息 -->
      <div
        v-if="notification.relatedId || notification.metadata"
        class="mb-6 p-4 bg-gray-50 rounded-lg"
      >
        <h3 class="text-sm font-medium text-gray-900 mb-3">相关信息</h3>
        <div class="space-y-2 text-sm">
          <div v-if="notification.relatedId" class="flex justify-between">
            <span class="text-gray-500">关联资源ID:</span>
            <span class="text-gray-900 font-mono">{{
              notification.relatedId
            }}</span>
          </div>
          <div v-if="notification.relatedType" class="flex justify-between">
            <span class="text-gray-500">资源类型:</span>
            <span class="text-gray-900">{{ notification.relatedType }}</span>
          </div>
          <div v-if="notification.userId" class="flex justify-between">
            <span class="text-gray-500">用户ID:</span>
            <span class="text-gray-900 font-mono">{{
              notification.userId
            }}</span>
          </div>
          <div
            v-if="notification.metadata"
            v-for="(value, key) in notification.metadata"
            :key="key"
            class="flex justify-between"
          >
            <span class="text-gray-500">{{ key }}:</span>
            <span class="text-gray-900">{{ value }}</span>
          </div>
        </div>
      </div>

      <!-- 操作按钮 -->
      <div v-if="notification.actions?.length" class="mb-6">
        <h3 class="text-sm font-medium text-gray-900 mb-3">可用操作</h3>
        <div class="flex flex-wrap gap-3">
          <UButton
            v-for="action in notification.actions"
            :key="action.id"
            :variant="action.type === 'primary' ? 'solid' : 'outline'"
            :color="action.type === 'danger' ? 'error' : 'primary'"
            @click="handleActionClick(action.id)"
          >
            {{ action.label }}
          </UButton>
        </div>
      </div>

      <!-- 时间信息 -->
      <div class="border-t border-gray-200 pt-4">
        <div
          class="grid grid-cols-1 md:grid-cols-2 gap-4 text-sm text-gray-500"
        >
          <div>
            <span class="font-medium">创建时间:</span>
            <span class="ml-2">{{
              formatFullTime(notification.createdAt)
            }}</span>
          </div>
          <div>
            <span class="font-medium">更新时间:</span>
            <span class="ml-2">{{
              formatFullTime(notification.updatedAt)
            }}</span>
          </div>
        </div>
      </div>
    </div>
  </div>

  <!-- 空状态 -->
  <div v-else class="flex items-center justify-center h-full">
    <div class="text-center">
      <UIcon
        name="i-heroicons-bell-slash"
        class="w-12 h-12 text-gray-400 mx-auto mb-4"
      />
      <p class="text-gray-500">请选择一个通知查看详情</p>
    </div>
  </div>
</template>
