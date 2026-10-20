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
const { identityScope, identityReady, getNotification } = useNotifications();
const route = useRoute();
const detailLoading = ref(false);
const detailError = ref('');
const retry = ref(0);
const requestedID = computed(() => typeof route.query?.id === 'string' ? route.query.id.trim() : '');
watch([requestedID, identityScope, identityReady, retry], async ([id, , ready], _previous, onCleanup) => {
  let current = true;
  onCleanup(() => { current = false; });
  selectedNotification.value = null;
  detailError.value = '';
  showDetail.value = Boolean(id);
  detailLoading.value = Boolean(id);
  if (!id || !ready) return;
  try {
    const notification = await getNotification(id);
    if (!current) return;
    selectedNotification.value = notification;
    if (!notification) detailError.value = 'unavailable';
  } catch (error: any) {
    if (!current) return;
    const status = error?.statusCode || error?.status || error?.response?.status;
    detailError.value = status === 404 || status === 403 ? 'unavailable' : 'loadError';
  } finally {
    if (current) detailLoading.value = false;
  }
}, { immediate: true, flush: 'sync' });

// 处理通知选择
const handleNotificationSelect = (notification: Notification) => {
  selectedNotification.value = notification;
  detailError.value = '';
  detailLoading.value = false;
  showDetail.value = true;
  void navigateTo({ path: route.path, query: { ...route.query, id: notification.id } }, { replace: true });
};

// 处理返回列表
const handleBackToList = () => {
  showDetail.value = false;
  selectedNotification.value = null;
  const { id: _id, ...query } = route.query || {};
  void navigateTo({ path: route.path, query }, { replace: true });
};

// 处理关闭详情
const handleCloseDetail = () => {
  handleBackToList();
};

// 响应式布局
const { width } = useWindowSize();
const isMobile = computed(() => width.value < 768);
</script>

<template>
  <div class="h-full flex flex-col">
    <!-- 保留同一列表实例；窄屏进入详情或改变屏幕宽度时保留过滤、分页上下文。 -->
    <div class="h-full flex">
      <div v-show="!isMobile || !showDetail" class="w-full md:w-1/2 min-w-0 md:border-r border-default">
        <NotificationList @select="handleNotificationSelect" />
      </div>

      <div v-show="!isMobile || showDetail" class="w-full md:w-1/2 min-w-0">
        <NotificationDetail
          :notification="selectedNotification"
          :loading="detailLoading"
          :error-key="detailError"
          @retry="retry++"
          @back="handleBackToList"
          @close="handleCloseDetail"
        />
      </div>
    </div>
  </div>
</template>
