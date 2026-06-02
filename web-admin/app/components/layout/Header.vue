<script setup lang="ts">
import { useDebounceFn } from "@vueuse/core";
import { useUserStore } from "~/stores/user";
import { useAuth } from "~/composables/useAuth";
import { useWSBus } from "~/composables/useWSBus";
import type { MenuCategory, MenuItem, UserMenusResult } from "~/composables/api/services/menuService";

const { t, te } = useI18n();
const route = useRoute();
const localePath = useLocalePath() as (path: string) => string;
const userStore = useUserStore();
const userContactLabel = computed(() => {
  const user = userStore.user;
  return user?.email || user?.phone || "";
});
const canAccessSettings = computed(
  () => Boolean(userStore.isCurrentTenantAdmin)
);

// 使用通知系统
const { getStats, notifications, fetchNotifications, addNotification, clearAllNotifications } =
  useNotifications();
const wsBus = useWSBus();

// 获取通知统计信息
const notificationStats = computed(() => getStats());
const unreadCount = computed(() => notificationStats.value.unread);
const { data: menuResponse } = useNuxtData<UserMenusResult>("user-menus");

const pageTitle = computed(() => {
  const raw = route.meta.title;
  const title = Array.isArray(raw) ? raw[0] : raw;
  if (typeof title === "string" && title.trim()) {
    return title.startsWith("menu.") ? t(title) : title;
  }
  return t("dashboard.title");
});

const isPluginPath = (path?: string) => !!path && /^\/+_p\//.test(path);

const normalizeForCompare = (input?: string): string => {
  if (!input) return "";
  let value = input.trim();
  if (!value) return "";
  if (/^[a-zA-Z][a-zA-Z0-9+.-]*:/.test(value)) return "";
  if (!value.startsWith("/")) value = `/${value}`;
  value = value.replace(/\/{2,}/g, "/");
  if (value.length > 1 && value.endsWith("/")) value = value.slice(0, -1);
  return value;
};

const normalizeMenuPath = (path?: string): string => {
  if (!path) return "";
  const raw = isPluginPath(path) ? path : localePath(path);
  return normalizeForCompare(raw);
};

const linkFor = (path?: string) => {
  if (!path) return "";
  return isPluginPath(path) ? path : localePath(path);
};

const toSegments = (path: string): string[] =>
  path && path !== "/" ? path.split("/").filter(Boolean) : [];

const translateMenuTitle = (item: MenuItem): string => {
  const key = item.titleI18n?.key?.trim();
  if (key) {
    const fallback = item.titleI18n?.default ?? item.title ?? key;
    return te(key) ? t(key) : fallback;
  }
  if (item.title?.startsWith?.("menu.")) {
    return te(item.title) ? t(item.title) : item.title;
  }
  return item.title || item.titleI18n?.default || t("menu.untitled", "未命名菜单");
};

type BreadcrumbEntry = {
  title: string;
  path?: string;
  normalized: string;
  segments: string[];
};

const collectMenuBreadcrumbs = (
  items: MenuItem[] | undefined,
  parents: BreadcrumbEntry[] = []
): BreadcrumbEntry[][] => {
  if (!items?.length) return [];
  const chains: BreadcrumbEntry[][] = [];
  for (const item of items) {
    if (!item || item.visible === false) continue;
    const normalized = normalizeMenuPath(item.path);
    const entry: BreadcrumbEntry = {
      title: translateMenuTitle(item),
      path: item.path,
      normalized,
      segments: toSegments(normalized),
    };
    const chain = [...parents, entry];
    if (normalized) chains.push(chain);
    chains.push(...collectMenuBreadcrumbs(item.children, chain));
  }
  return chains;
};

const menuBreadcrumbs = computed(() => {
  const payload = menuResponse.value;
  const categories = (payload?.categories || []) as MenuCategory[];
  const sourceItems =
    categories.length > 0
      ? categories.flatMap((category) => category.children || [])
      : payload?.data || [];
  const chains = collectMenuBreadcrumbs(sourceItems);
  const current = normalizeForCompare(route.path);
  const currentSegments = toSegments(current);
  let best: BreadcrumbEntry[] = [];
  let bestDepth = -1;

  for (const chain of chains) {
    const leaf = chain[chain.length - 1];
    if (!leaf?.normalized) continue;
    if (currentSegments.length < leaf.segments.length) continue;
    let matched = true;
    for (let i = 0; i < leaf.segments.length; i += 1) {
      const menuSegment = leaf.segments[i];
      const routeSegment = currentSegments[i];
      if (menuSegment === "*") continue;
      if (menuSegment.startsWith(":")) {
        if (!routeSegment) matched = false;
        continue;
      }
      if (menuSegment !== routeSegment) {
        matched = false;
        break;
      }
    }
    if (
      matched &&
      (leaf.segments.length > bestDepth ||
        (leaf.segments.length === bestDepth && chain.length > best.length))
    ) {
      bestDepth = leaf.segments.length;
      best = chain;
    }
  }

  if (best.length === 0) {
    return [{ title: pageTitle.value }];
  }
  return best.map((entry, index) => ({
    title: entry.title,
    path: index < best.length - 1 ? entry.path : undefined,
  }));
});

let unsubscribeNotifications: (() => void) | null = null;
const UUID_RE =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

const ensureWSNotificationSubscription = (tenantUUID: string) => {
  if (!UUID_RE.test(tenantUUID)) return;
  wsBus.connect(tenantUUID);
  if (unsubscribeNotifications) return;
  unsubscribeNotifications = wsBus.subscribe("_topic.system.notification", (payload) => {
    if (!payload) return;
    addNotification(payload);
  });
};

// 初始化通知数据和用户数据
onMounted(async () => {
  try {
    await userStore.fetchUserContext();
  } catch (error) {
    console.error("初始化用户数据失败:", error);
  }

  try {
    fetchNotifications();
  } catch (error) {
    console.error("fetchNotifications error:", error);
  }

  // WS 只要求有效 tenant_uuid；token 由 useWSBus 内部从 localStorage/cookie 解析。
  ensureWSNotificationSubscription(String(userStore.currentTenantUuid || "").trim());
});

watch(
  () => String(userStore.currentTenantUuid || "").trim(),
  (tenantUUID) => {
    ensureWSNotificationSubscription(tenantUUID);
  },
  { immediate: true }
);

onBeforeUnmount(() => {
  if (unsubscribeNotifications) {
    unsubscribeNotifications();
    unsubscribeNotifications = null;
  }
});

// 用户菜单项
const userMenuItems = computed(() => {
  const firstGroup: any[] = [
    {
      label: t("header.profile"),
      icon: "i-heroicons-user",
      to: "/profile",
    },
  ];
  if (canAccessSettings.value) {
    firstGroup.push({
      label: t("header.settings"),
      icon: "i-heroicons-cog-6-tooth",
      to: "/settings",
    });
  }
  return [
    firstGroup,
    [
      {
        label: t("header.logout"),
        icon: "i-heroicons-arrow-right-on-rectangle",
        onSelect: handleLogout,
      },
    ],
  ];
});

// 通知菜单项
const notificationItems = computed(() => {
  const recentNotifications = notifications.value.slice(0, 5); // 只显示最近5条

  const notificationMenuItems = recentNotifications.map((notification) => ({
    label: notification.title,
    description:
      notification.content.length > 50
        ? notification.content.substring(0, 50) + "..."
        : notification.content,
    icon: getNotificationIcon(notification.type),
    badge: !notification.isRead ? "new" : undefined,
    to: `/notifications?id=${notification.id}`,
  }));

  // 添加分隔符和查看全部按钮
  const menuItems = [notificationMenuItems];

  menuItems.push([
    {
      label: "清空消息",
      icon: "i-heroicons-trash",
      onSelect: handleClearNotifications,
      description: "",
    },
  ]);

  if (recentNotifications.length > 0) {
    menuItems.push([
      {
        label: "查看所有通知",
        icon: "i-heroicons-eye",
        to: "/notifications",
        description: "",
        badge: unreadCount.value.toString(),
      },
    ]);
  } else {
    menuItems.push([
      {
        label: "暂无通知",
        icon: "i-heroicons-bell-slash",
        description: "",
        badge: unreadCount.value > 0 ? unreadCount.value.toString() : undefined,
      },
    ]);
  }

  return menuItems;
});

const handleClearNotifications = async () => {
  try {
    await clearAllNotifications();
  } catch (error) {
    console.error("clearAllNotifications error:", error);
  }
};

// 获取通知图标
const getNotificationIcon = (type: string) => {
  switch (type) {
    case "success":
      return "i-heroicons-check-circle";
    case "warning":
      return "i-heroicons-exclamation-triangle";
    case "error":
      return "i-heroicons-x-circle";
    case "info":
      return "i-heroicons-information-circle";
    case "system":
      return "i-heroicons-cog-6-tooth";
    default:
      return "i-heroicons-bell";
  }
};

// 退出登录
const handleLogout = async () => {
  try {
    // 使用统一的认证退出方法
    const { logout } = useAuth();
    await logout();
  } catch (error) {
    console.error("退出登录失败:", error);
  }
};

// 搜索功能
const searchQuery = ref("");
const isSearchFocused = ref(false);
const searchSuggestions = ref<any[]>([]);
const showSearchSuggestions = ref(false);

// 使用搜索功能
const { quickSearch } = useSearch();

const handleSearch = async () => {
  try {
    const q = searchQuery.value?.trim();
    if (!q) return;
    await navigateTo({ path: "/search", query: { q } });
  } catch (err) {
    console.error("handleSearch error:", err);
  } finally {
    showSearchSuggestions.value = false;
  }
};

// 搜索建议 - 使用防抖和错误处理
const handleSearchInput = useDebounceFn(async (value: string) => {
  try {
    if (!value?.trim()) {
      showSearchSuggestions.value = false;
      searchSuggestions.value = [];
      return;
    }

    showSearchSuggestions.value = true;
    const results = await quickSearch(value);

    // 转换为建议格式
    searchSuggestions.value = results.slice(0, 5).map((r) => ({
      id: r.id,
      title: r.title,
      type: r.type,
      category: r.category,
      url: r.url,
    }));
  } catch (e) {
    console.error("handleSearchInput error:", e);
    // 不要把异常往外抛，避免"Unhandled error …"
    searchSuggestions.value = [];
  }
}, 250);

// 处理输入框失焦
const handleBlur = () => {
  // SSR 安全：只在客户端用 window
  if (process.client) {
    window.setTimeout(() => {
      showSearchSuggestions.value = false;
      isSearchFocused.value = false;
    }, 200);
  } else {
    showSearchSuggestions.value = false;
    isSearchFocused.value = false;
  }
};

// 选择搜索建议
const selectSearchSuggestion = async (result: any) => {
  try {
    if (!result?.url) return;
    await navigateTo(result.url);
  } catch (err) {
    console.error("selectSearchSuggestion error:", err);
  } finally {
    showSearchSuggestions.value = false;
    searchQuery.value = "";
  }
};

// 获取搜索结果类型图标
const getSearchResultTypeIcon = (type: string) => {
  switch (type) {
    case "user":
      return "i-heroicons-user";
    case "content":
      return "i-heroicons-document-text";
    case "product":
      return "i-heroicons-cube";
    case "order":
      return "i-heroicons-shopping-cart";
    case "plugin":
      return "i-heroicons-puzzle-piece";
    case "agent":
      return "i-heroicons-cpu-chip";
    case "workflow":
      return "i-heroicons-arrow-path";
    case "setting":
      return "i-heroicons-cog-6-tooth";
    case "notification":
      return "i-heroicons-bell";
    default:
      return "i-heroicons-magnifying-glass";
  }
};
</script>

<template>
  <header
    class="bg-white/95 dark:bg-gray-900/95 backdrop-blur-sm border-b border-gray-200/60 dark:border-gray-700/60 shadow-sm h-16 flex items-center justify-between px-6 sticky top-0 z-40"
  >
    <!-- 左侧：面包屑导航 -->
    <div class="flex items-center space-x-4">
      <nav class="flex" aria-label="Breadcrumb">
        <ol class="flex items-center space-x-2">
          <li>
            <NuxtLink
              :to="$localePath('/dashboard')"
              class="text-gray-500 hover:text-gray-700"
            >
              <UIcon class="w-4 h-4 inline-block" name="i-heroicons-home" />
            </NuxtLink>
          </li>
          <li
            v-for="(crumb, index) in menuBreadcrumbs"
            :key="`${index}:${crumb.title}`"
            class="flex items-center"
          >
            <UIcon
              class="w-4 h-4 text-gray-400 mx-2 inline-block"
              name="i-heroicons-chevron-right"
            />
            <NuxtLink
              v-if="crumb.path"
              :to="linkFor(crumb.path)"
              class="text-sm font-medium text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200"
            >
              {{ crumb.title }}
            </NuxtLink>
            <span
              v-else
              class="text-sm font-medium text-gray-900 dark:text-gray-100"
            >
              {{ crumb.title }}
            </span>
          </li>
        </ol>
      </nav>
    </div>

    <!-- 中间：搜索框 -->
    <div class="flex-1 max-w-lg mx-8">
      <div class="relative">
        <UInput
          v-model="searchQuery"
          :placeholder="t('header.searchPlaceholder')"
          icon="i-heroicons-magnifying-glass"
          size="md"
          class="w-full"
          @keyup.enter="handleSearch"
          @update:model-value="handleSearchInput"
          @focus="isSearchFocused = true"
          @blur="handleBlur"
        />

        <!-- 搜索建议下拉框 -->
        <div
          v-if="showSearchSuggestions && searchSuggestions.length > 0"
          class="absolute top-full left-0 right-0 mt-1 bg-white/95 dark:bg-gray-900/95 backdrop-blur-sm border border-gray-200/60 dark:border-gray-700/60 rounded-lg shadow-xl z-50"
        >
          <div class="p-2">
            <div class="text-xs text-gray-500 mb-2">
              {{ t("header.searchSuggestions") }}
            </div>
            <div class="space-y-1">
              <button
                v-for="suggestion in searchSuggestions"
                :key="suggestion.id"
                class="w-full text-left px-3 py-2 text-sm hover:bg-gray-50 dark:hover:bg-gray-700 rounded flex items-center space-x-2"
                @mousedown.prevent="selectSearchSuggestion(suggestion)"
              >
                <UIcon
                  :name="getSearchResultTypeIcon(suggestion.type)"
                  class="w-4 h-4 text-gray-400"
                />
                <div class="flex-1">
                  <div class="font-medium">{{ suggestion.title }}</div>
                  <div class="text-xs text-gray-500">
                    {{ suggestion.category }}
                  </div>
                </div>
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 右侧：操作按钮 -->
    <div class="flex items-center space-x-4">
      <!-- 通知 -->
      <UDropdownMenu :items="notificationItems">
        <UButton variant="ghost" size="sm" class="relative">
          <span class="w-5 h-5 inline-block">
            <UIcon class="w-5 h-5 inline-block" name="i-heroicons-bell" />
          </span>
          <UBadge
            v-if="unreadCount > 0"
            :label="unreadCount.toString()"
            size="xs"
            color="error"
            class="absolute -top-1 -right-1"
          />
        </UButton>
      </UDropdownMenu>

      <!-- 用户菜单 -->
      <UDropdownMenu :items="userMenuItems">
        <UButton variant="ghost" class="flex items-center space-x-2">
          <!-- 用户头像 -->
          <div
            v-if="userStore.avatarUrl"
            class="w-8 h-8 rounded-full overflow-hidden bg-gray-300"
          >
            <img
              :src="userStore.avatarUrl"
              :alt="userStore.displayName"
              class="w-full h-full object-cover"
            />
          </div>
          <div
            v-else
            class="w-8 h-8 bg-gray-300 rounded-full flex items-center justify-center"
          >
            <span class="w-5 h-5 text-gray-600 inline-block">
              <UIcon
                class="w-5 h-5 text-gray-600 inline-block"
                name="i-heroicons-user"
              />
            </span>
          </div>

          <!-- 用户信息 -->
          <div class="hidden md:block text-left">
            <div class="text-sm font-medium text-gray-900 dark:text-gray-100">
              {{ userStore.displayName || "管理员" }}
            </div>
            <div class="text-xs text-gray-500 dark:text-gray-400">
              {{ userContactLabel || "-" }}
            </div>
          </div>

          <span class="w-4 h-4 text-gray-400 inline-block">
            <UIcon
              class="w-4 h-4 text-gray-400 inline-block"
              name="i-heroicons-chevron-down"
            />
          </span>
        </UButton>
      </UDropdownMenu>
    </div>
  </header>
</template>
