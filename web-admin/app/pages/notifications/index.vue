<script setup lang="ts">
import type { Notification } from "~/types/notification";
import NotificationList from "~/components/notifications/NotificationList.vue";
import NotificationDetail from "~/components/notifications/NotificationDetail.vue";

const { t } = useI18n();

definePageMeta({
  layout: "default",
});

useHead({
  title: "消息通知中心",
  meta: [{ name: "description", content: "查看和管理系统消息通知" }],
});

// 当前选中的通知
const selectedNotification = ref<Notification | null>(null);

// 是否显示详情面板（移动端）
const showDetail = ref(false);

// 处理通知选择
const handleNotificationSelect = (notification: Notification) => {
  selectedNotification.value = notification;
  showDetail.value = true;
};

// 处理返回列表
const handleBackToList = () => {
  showDetail.value = false;
  selectedNotification.value = null;
};

// 处理关闭详情
const handleCloseDetail = () => {
  selectedNotification.value = null;
  showDetail.value = false;
};

// 响应式布局
const { width } = useWindowSize();
const isMobile = computed(() => width.value < 768);
</script>

<template>
  <div class="h-full flex flex-col">
    <!-- 移动端视图 -->
    <div v-if="isMobile" class="h-full">
      <!-- 列表视图 -->
      <div v-if="!showDetail" class="h-full">
        <NotificationList @select="handleNotificationSelect" />
      </div>

      <!-- 详情视图 -->
      <div v-else class="h-full">
        <NotificationDetail
          :notification="selectedNotification"
          @back="handleBackToList"
          @close="handleCloseDetail"
        />
      </div>
    </div>

    <!-- 桌面端视图 -->
    <div v-else class="h-full flex">
      <!-- 左侧列表 -->
      <div class="w-1/2 border-r border-gray-200">
        <NotificationList @select="handleNotificationSelect" />
      </div>

      <!-- 右侧详情 -->
      <div class="w-1/2">
        <NotificationDetail
          :notification="selectedNotification"
          @close="handleCloseDetail"
        />
      </div>
    </div>
  </div>
</template>
