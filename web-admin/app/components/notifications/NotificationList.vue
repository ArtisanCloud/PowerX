<script setup lang="ts">
import type { Notification } from "~/types/notification";
import {
  NOTIFICATION_CATEGORIES,
  NOTIFICATION_TYPES,
} from "~/types/notification";

const props = defineProps<{
  compact?: boolean;
}>();

const emit = defineEmits<{
  select: [notification: Notification];
}>();

const { t } = useI18n();
const {
  notifications,
  loading,
  error,
  currentPage,
  pageSize,
  totalCount,
  filter,
  fetchNotifications,
  markAsRead,
  markAllAsRead,
  deleteNotification,
  getStats,
  setFilter,
  clearFilter,
  executeAction,
} = useNotifications();

// 统计信息
const stats = computed(() => getStats());

// 过滤选项
const categoryOptions = [
  { label: "全部分类", value: null },
  { label: "系统通知", value: NOTIFICATION_CATEGORIES.SYSTEM },
  { label: "Agent 通知", value: NOTIFICATION_CATEGORIES.AGENT },
  { label: "工作流通知", value: NOTIFICATION_CATEGORIES.WORKFLOW },
  { label: "用户通知", value: NOTIFICATION_CATEGORIES.USER },
  { label: "订单通知", value: NOTIFICATION_CATEGORIES.ORDER },
  { label: "插件通知", value: NOTIFICATION_CATEGORIES.PLUGIN },
];

const typeOptions = [
  { label: "全部类型", value: null },
  { label: "信息", value: NOTIFICATION_TYPES.INFO },
  { label: "成功", value: NOTIFICATION_TYPES.SUCCESS },
  { label: "警告", value: NOTIFICATION_TYPES.WARNING },
  { label: "错误", value: NOTIFICATION_TYPES.ERROR },
  { label: "系统", value: NOTIFICATION_TYPES.SYSTEM },
];

const readOptions = [
  { label: "全部状态", value: null },
  { label: "未读", value: "false" },
  { label: "已读", value: "true" },
];

// 当前过滤器值
const selectedCategory = ref(null);
const selectedType = ref(null);
const selectedRead = ref(null);
const showImportantOnly = ref(false);

// 监听过滤器变化
watch([selectedCategory, selectedType, selectedRead, showImportantOnly], () => {
  const newFilter: any = {};

  if (selectedCategory.value) newFilter.category = selectedCategory.value;
  if (selectedType.value) newFilter.type = selectedType.value;
  if (selectedRead.value !== "")
    newFilter.isRead = selectedRead.value === "true";
  if (showImportantOnly.value) newFilter.isImportant = true;

  setFilter(newFilter);
});

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
  const option = categoryOptions.find((opt) => opt.value === category);
  return option?.label || category;
};

// 格式化时间
const formatTime = (date: Date) => {
  const now = new Date();
  const diff = now.getTime() - date.getTime();
  const minutes = Math.floor(diff / 60000);
  const hours = Math.floor(diff / 3600000);
  const days = Math.floor(diff / 86400000);

  if (minutes < 1) return "刚刚";
  if (minutes < 60) return `${minutes}分钟前`;
  if (hours < 24) return `${hours}小时前`;
  if (days < 7) return `${days}天前`;
  return date.toLocaleDateString();
};

// 处理通知点击
const handleNotificationClick = async (notification: Notification) => {
  if (!notification.isRead) {
    await markAsRead(notification.id);
  }
  emit("select", notification);
};

// 处理操作点击
const handleActionClick = async (
  notification: Notification,
  actionId: string,
  event: Event
) => {
  event.stopPropagation();
  await executeAction(notification.id, actionId);
};

// 处理删除
const handleDelete = async (notification: Notification, event: Event) => {
  event.stopPropagation();
  await deleteNotification(notification.id);
};

// 清空所有过滤器
const handleClearFilters = () => {
  selectedCategory.value = "";
  selectedType.value = "";
  selectedRead.value = "";
  showImportantOnly.value = false;
  clearFilter();
};

// 初始化
onMounted(() => {
  fetchNotifications();
});
</script>

<template>
  <div class="flex flex-col h-full">
    <!-- 头部统计和操作 -->
    <div v-if="!compact" class="p-4 border-b border-gray-200">
      <div class="flex items-center justify-between mb-4">
        <h2 class="text-lg font-semibold text-gray-900">消息通知</h2>
        <div class="flex items-center space-x-2">
          <UButton
            variant="ghost"
            size="sm"
            icon="i-heroicons-check"
            @click="markAllAsRead"
            :disabled="stats.unread === 0"
          >
            全部已读
          </UButton>
        </div>
      </div>

      <!-- 统计信息 -->
      <div class="grid grid-cols-3 gap-4 mb-4">
        <div class="text-center">
          <div class="text-2xl font-bold text-gray-900">{{ stats.total }}</div>
          <div class="text-sm text-gray-500">总消息</div>
        </div>
        <div class="text-center">
          <div class="text-2xl font-bold text-blue-600">{{ stats.unread }}</div>
          <div class="text-sm text-gray-500">未读</div>
        </div>
        <div class="text-center">
          <div class="text-2xl font-bold text-red-600">
            {{ stats.important }}
          </div>
          <div class="text-sm text-gray-500">重要</div>
        </div>
      </div>

      <!-- 过滤器 -->
      <div class="grid grid-cols-2 md:grid-cols-4 gap-2">
        <USelect
          v-model="selectedCategory"
          :items="categoryOptions"
          placeholder="选择分类"
          size="sm"
        />
        <USelect
          v-model="selectedType"
          :items="typeOptions"
          placeholder="选择类型"
          size="sm"
        />
        <USelect
          v-model="selectedRead"
          :items="readOptions"
          placeholder="读取状态"
          size="sm"
        />
        <div class="flex items-center space-x-2">
          <UCheckbox v-model="showImportantOnly" label="仅重要" size="sm" />
          <UButton
            variant="ghost"
            size="sm"
            icon="i-heroicons-x-mark"
            @click="handleClearFilters"
            v-if="
              selectedCategory ||
              selectedType ||
              selectedRead ||
              showImportantOnly
            "
          >
            清空
          </UButton>
        </div>
      </div>
    </div>

    <!-- 消息列表 -->
    <div class="flex-1 overflow-y-auto">
      <div v-if="loading" class="p-4 text-center">
        <UIcon
          name="i-heroicons-arrow-path"
          class="w-6 h-6 animate-spin mx-auto mb-2"
        />
        <p class="text-gray-500">加载中...</p>
      </div>

      <div v-else-if="error" class="p-4 text-center">
        <UIcon
          name="i-heroicons-exclamation-triangle"
          class="w-6 h-6 text-red-500 mx-auto mb-2"
        />
        <p class="text-red-500">{{ error }}</p>
        <UButton
          variant="ghost"
          size="sm"
          @click="fetchNotifications"
          class="mt-2"
        >
          重试
        </UButton>
      </div>

      <div v-else-if="notifications.length === 0" class="p-8 text-center">
        <UIcon
          name="i-heroicons-bell-slash"
          class="w-12 h-12 text-gray-400 mx-auto mb-4"
        />
        <p class="text-gray-500">暂无消息通知</p>
      </div>

      <div v-else class="divide-y divide-gray-200">
        <div
          v-for="notification in notifications"
          :key="notification.id"
          class="p-4 hover:bg-gray-50 cursor-pointer transition-colors"
          :class="{
            'bg-blue-50': !notification.isRead,
            'border-l-4 border-l-red-500': notification.isImportant,
          }"
          @click="handleNotificationClick(notification)"
        >
          <div class="flex items-start space-x-3">
            <!-- 图标 -->
            <div class="flex-shrink-0 mt-1">
              <UIcon
                :name="getNotificationIcon(notification)"
                :class="['w-5 h-5', getNotificationColor(notification)]"
              />
            </div>

            <!-- 内容 -->
            <div class="flex-1 min-w-0">
              <div class="flex items-center justify-between mb-1">
                <h3 class="text-sm font-medium text-gray-900 truncate">
                  {{ notification.title }}
                  <UBadge
                    v-if="notification.isImportant"
                    color="error"
                    variant="soft"
                    size="xs"
                    class="ml-2"
                  >
                    重要
                  </UBadge>
                </h3>
                <div class="flex items-center space-x-2">
                  <UBadge
                    :color="
                      notification.type === 'error'
                        ? 'red'
                        : notification.type === 'warning'
                          ? 'yellow'
                          : notification.type === 'success'
                            ? 'green'
                            : 'blue'
                    "
                    variant="soft"
                    size="xs"
                  >
                    {{ getCategoryLabel(notification.category) }}
                  </UBadge>
                  <span class="text-xs text-gray-500">
                    {{ formatTime(notification.createdAt) }}
                  </span>
                </div>
              </div>

              <p class="text-sm text-gray-600 mb-2 line-clamp-2">
                {{ notification.content }}
              </p>

              <!-- 操作按钮 -->
              <div
                v-if="notification.actions?.length"
                class="flex items-center space-x-2"
              >
                <UButton
                  v-for="action in notification.actions"
                  :key="action.id"
                  :variant="action.type === 'primary' ? 'solid' : 'ghost'"
                  :color="action.type === 'danger' ? 'error' : 'primary'"
                  size="xs"
                  @click="handleActionClick(notification, action.id, $event)"
                >
                  {{ action.label }}
                </UButton>
              </div>
            </div>

            <!-- 删除按钮 -->
            <div class="flex-shrink-0">
              <UButton
                variant="ghost"
                color="neutral"
                size="xs"
                icon="i-heroicons-x-mark"
                @click="handleDelete(notification, $event)"
              />
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 分页 -->
    <div
      v-if="!compact && totalCount > pageSize"
      class="p-4 border-t border-gray-200"
    >
      <UPagination
        v-model:page="currentPage"
        :items-per-page="pageSize"
        :total="totalCount"
        :sibling-count="1"
        show-edges
        @update:page="fetchNotifications"
      />
    </div>
  </div>
</template>

<style scoped>
.line-clamp-2 {
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
</style>
