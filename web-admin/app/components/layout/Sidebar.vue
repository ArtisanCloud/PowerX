<!-- SidebarMenu.vue（修正版，不依赖 Tooltip，v-if 链相邻无歧义） -->
<script setup lang="ts">
import {
  useMenuService,
  type MenuItem,
  type MenuCategory,
  type UserMenusResult,
} from "~/composables/api/services/menuService";
import { cloneWithFilteredChildren } from "~/composables/useCopy";
import { useUserStore } from "~/stores/user";
import SidebarMenuItem from "~/components/layout/SidebarMenuItem.vue";
import { LOGO_M_URL } from "~/utils/assets";

/* ---------- stores / utils ---------- */
const route = useRoute();
const menuService = useMenuService();
const userStore = useUserStore();
const { t, te, locale } = useI18n({ useScope: "global" });
const localePath = useLocalePath() as (p: string) => string;

/* ========== 折叠与密度 ========== */
const collapsed = useState<boolean>("sidebar-collapsed", () => false);
const density = useState<"comfortable" | "compact">(
  "menu-density",
  () => "comfortable"
);
const densityClass = computed(() =>
  density.value === "compact" ? "py-1.5 text-[13px]" : "py-2 text-sm"
);

/* ---------- helpers ---------- */
const isPluginPath = (p?: string) => !!p && p.startsWith("//_p/");
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
const toSegments = (path: string): string[] =>
  path && path !== "/" ? path.split("/").filter(Boolean) : [];
const normalizeMenuPath = (path?: string): string => {
  if (!path) return "";
  const raw = isPluginPath(path) ? path : localePath(path);
  return normalizeForCompare(raw);
};
const normalizedRoutePath = computed(() => normalizeForCompare(route.path));
const routeSegments = computed(() => toSegments(normalizedRoutePath.value));
const linkFor = (p?: string) => {
  if (!p) return "";
  return isPluginPath(p) ? p : localePath(p);
};

const translateMenuTitle = (item: MenuItem) => {
  // 插件菜单：后端给了 titleI18n
  const key = item.titleI18n?.key?.trim();

  if (key) {
    const fallback = item.titleI18n?.default ?? item.title ?? key;
    // 先看前端是否已加载到这个 key
    if (te(key)) return t(key);
    // 否则直接用预翻译/默认值，不要把 fallback 误传给 t()
    return fallback;
  }

  // 系统菜单：title 本身就是 i18n key（如 "menu.agent"）
  const rawTitle = item.title;
  if (rawTitle?.startsWith?.("menu.")) {
    // 有翻译就用翻译
    if (te(rawTitle)) return t(rawTitle);
    // 没有就用 item.titleI18n.default（如果给了），再不行就原文/“未命名菜单”
    return item.titleI18n?.default ?? rawTitle ?? t("menu.untitled", "未命名菜单");
  }

  // 兜底
  return rawTitle || item.titleI18n?.default || t("menu.untitled", "未命名菜单");
};

const translateCategoryTitle = (category: MenuCategory) => {
  const key = category.titleI18n?.key?.trim();
  if (key) {
    const fallback = category.titleI18n?.default ?? category.title ?? key;
    return t(key, fallback);
  }
  if (category.title?.startsWith?.("menu.")) {
    return t(category.title);
  }
  return category.title || t("menu.section.untitled", "未命名分类");
};

const resolveIcon = (name?: string) => {
  if (!name) return "i-heroicons-puzzle-piece";
  if (name.startsWith("i-")) return name;
  const iconMap: Record<string, string> = {
    Smile: "i-heroicons-face-smile",
    Settings: "i-heroicons-cog-6-tooth",
    User: "i-heroicons-user",
    Home: "i-heroicons-home",
    Plugin: "i-heroicons-puzzle-piece",
  };
  return iconMap[name] || "i-heroicons-puzzle-piece";
};

const isVisible = (it: MenuItem) => it.visible !== false;

const MARKET_CATEGORY_ID = "cat:market";
const isMarketSubCategory = (item: MenuItem) =>
  typeof item.id === "string" && item.id.startsWith(`${MARKET_CATEGORY_ID}:`);


/* ---------- 拉取菜单 ---------- */
const {
  data: menuResponse,
  pending: menuLoading,
  error: menuError,
  refresh: refreshMenus,
} = await useAsyncData("user-menus", () => menuService.getUserMenus(), {
  default: () => ({ data: [] as MenuItem[], categories: [] as MenuCategory[] }),
  transform: (response: any) => {
    if (response && Array.isArray(response.categories)) {
      // Case 1: response is { categories: [...] }
      return { data: [], categories: response.categories };
    }
    if (response && Array.isArray(response.data)) {
      // Case 2: response is { data: [...] }
      const categories = Array.isArray(response.categories)
        ? response.categories
        : [];
      return { data: response.data, categories: categories };
    }
    // Fallback
    return { data: [], categories: [] };
  },
  watch: [locale],
});

/* ---------- 子级排序（顶层不排序） ---------- */
const sortChildren = (a: MenuItem, b: MenuItem) => {
  const ao = Number.isFinite(a.order) ? a.order : Number.POSITIVE_INFINITY;
  const bo = Number.isFinite(b.order) ? b.order : Number.POSITIVE_INFINITY;
  if (ao !== bo) return ao - bo;
  const at = a.title ?? "";
  const bt = b.title ?? "";
  if (at !== bt) return at.localeCompare(bt);
  return (a.id ?? "").localeCompare(b.id ?? "");
};

/* ---------- 递归处理 ---------- */
const processMenuItems = (items: MenuItem[], level = 0): MenuItem[] => {
  const mapped = items.filter(isVisible).map((item) => ({
    ...item,
    title: translateMenuTitle(item),
    badge:
      typeof item.badge === "string" && item.badge.startsWith("menu.")
        ? t(item.badge)
        : item.badge,
    children: item.children?.length
      ? processMenuItems(item.children, level + 1)
      : undefined,
  }));
  if (level === 0) return mapped; // 顶层不排序
  return mapped.sort(sortChildren);
};

/* ---------- 分组 ---------- */
type MenuGroup = { id: string; title: string; items: MenuItem[] };

const viewGroups = computed<MenuGroup[]>(() => {
  const payload = menuResponse.value;
  const categories = payload?.categories || [];
  if (categories.length > 0) {
    const sorted = [...categories].sort((a, b) => a.order - b.order);
    return sorted
      .map((category) => ({
        id: category.id || `cat-${category.title}`,
        title: translateCategoryTitle(category),
        items: processMenuItems(category.children || []),
      }))
      .filter((group) => group.items.length > 0);
  }

  const flatMenus: MenuItem[] = payload?.data || [];

  const top: MenuItem[] = [];
  const plugin: MenuItem[] = [];
  const system: MenuItem[] = [];

  for (const item of flatMenus) {
    if (item.slot === "group.root") {
      top.push(item);
      continue;
    }

    if (
      item.origin === "system" &&
      item.id === "plugins" &&
      Array.isArray((item as any).children) &&
      (item as any).children.length > 0
    ) {
      const children = (item as any).children as MenuItem[];

      const pluginChildren = children.filter(
        (ch) => ch && ch.origin === "plugin"
      );
      if (pluginChildren.length > 0) {
        plugin.push(...pluginChildren);
      }

      const sysItem = cloneWithFilteredChildren(
        item,
        (ch) => !(ch && ch.origin === "plugin")
      );
      system.push(sysItem);
      continue;
    }

    if (item.origin === "plugin") {
      plugin.push(item);
      continue;
    }

    system.push(item);
  }

  return [
    { id: "top", title: "置顶", items: processMenuItems(top) },
    { id: "plugin", title: "应用", items: processMenuItems(plugin) },
    { id: "system", title: "系统", items: processMenuItems(system) },
  ].filter((group) => group.items.length > 0);
});

const OPEN_CAPABILITY_PATH = "/settings/open-capabilities";
const SETTINGS_ROOT_PATH = "/settings";

const attachToSettingsMenu = (groups: MenuGroup[], item: MenuItem): boolean => {
  const normalizedTarget = normalizeMenuPath(SETTINGS_ROOT_PATH);
  const normalizedExtra = normalizeMenuPath(item.path);
  if (!normalizedExtra) return false;

  const isSettingsItem = (menuItem: MenuItem) => {
    const normalized = normalizeMenuPath(menuItem.path);
    if (normalized && normalized === normalizedTarget) return true;
    if (
      typeof menuItem.id === "string" &&
      menuItem.id.trim().toLowerCase() === "settings"
    ) {
      return true;
    }
    return false;
  };

  const queue: MenuItem[] = [];
  for (const group of groups) {
    queue.push(...group.items);
  }

  while (queue.length > 0) {
    const current = queue.shift();
    if (!current) continue;
    if (isSettingsItem(current)) {
      const children = current.children ? [...current.children] : [];
      const exists = children.some(
        (child) => normalizeMenuPath(child.path) === normalizedExtra
      );
      if (!exists) {
        current.children = [...children, item];
      }
      return true;
    }
    if (current.children?.length) {
      queue.push(...current.children);
    }
  }

  return false;
};

const manualOpenCapabilityMenu = computed<MenuItem | null>(() => {
  if (!userStore.isRoot) return null;
  const label = t("menu.openCapabilities", "开放能力");
  return {
    id: "open-capabilities",
    title: label,
    icon: "i-heroicons-bolt",
    path: OPEN_CAPABILITY_PATH,
    order: 120,
    visible: true,
    origin: "system",
  };
});

const renderedGroups = computed<MenuGroup[]>(() => {
  const base = viewGroups.value.map((group) => ({
    ...group,
    items: [...group.items],
  }));
  const extra = manualOpenCapabilityMenu.value;
  if (!extra) return base;
  const normalized = normalizeMenuPath(extra.path);
  const exists = base.some((group) =>
    group.items.some((item) => normalizeMenuPath(item.path) === normalized)
  );
  if (exists) return base;

  const attached = attachToSettingsMenu(base, extra);
  if (attached) return base;

  const systemGroup = base.find((group) => group.id === "system");
  if (systemGroup) {
    systemGroup.items = [...systemGroup.items, extra].sort(sortChildren);
  } else {
    base.push({
      id: "system",
      title: t("menu.settings"),
      items: [extra],
    });
  }
  return base;
});

type MenuPathEntry = { normalized: string; segments: string[] };
const menuPathEntries = computed<MenuPathEntry[]>(() => {
  const entries: MenuPathEntry[] = [];
  const addItems = (items?: MenuItem[]) => {
    if (!items) return;
    for (const item of items) {
      if (!item) continue;
      if (item.path) {
        const normalized = normalizeMenuPath(item.path);
        if (normalized) {
          entries.push({ normalized, segments: toSegments(normalized) });
        }
      }
      if (item.children?.length) addItems(item.children);
    }
  };

  for (const group of renderedGroups.value) {
    addItems(group.items);
  }

  return entries;
});

const matchDepth = (entry: MenuPathEntry, segments: string[]): number => {
  if (!entry.normalized) return -1;
  if (entry.normalized === "/") return segments.length === 0 ? 0 : -1;
  if (segments.length < entry.segments.length) return -1;
  for (let i = 0; i < entry.segments.length; i += 1) {
    const menuSegment = entry.segments[i];
    const routeSegment = segments[i];
    if (menuSegment === "*") return entry.segments.length;
    if (menuSegment.startsWith(":")) {
      if (!routeSegment) return -1;
      continue;
    }
    if (menuSegment !== routeSegment) return -1;
  }
  return entry.segments.length;
};

const activeMenuPaths = computed<Set<string>>(() => {
  const matches = new Set<string>();
  const segments = routeSegments.value;
  const entries = menuPathEntries.value;
  let bestDepth = -1;

  for (const entry of entries) {
    const depth = matchDepth(entry, segments);
    if (depth < 0) continue;
    if (depth > bestDepth) {
      bestDepth = depth;
      matches.clear();
      matches.add(entry.normalized);
    } else if (depth === bestDepth) {
      matches.add(entry.normalized);
    }
  }

  return matches;
});

const isActive = (path?: string) => {
  if (!path) return false;
  const normalized = normalizeMenuPath(path);
  if (!normalized) return false;
  return activeMenuPaths.value.has(normalized);
};
const hasActiveChild = (children?: MenuItem[]): boolean => {
  if (!children?.length) return false;
  return children.some(
    (child) => isActive(child.path) || hasActiveChild(child.children)
  );
};

/* ---------- 展开状态 ---------- */
const expandedItems = ref<Set<string>>(new Set());
const toggleExpanded = (id: string) => {
  const s = expandedItems.value;
  s.has(id) ? s.delete(id) : s.add(id);
};
const expandByRoute = () => {
  const set = new Set<string>();
  const markExpanded = (items?: MenuItem[]) => {
    if (!items) return;
    for (const item of items) {
      if (!item) continue;
      if (item.children?.length) {
        if (hasActiveChild(item.children)) set.add(item.id);
        markExpanded(item.children);
      }
    }
  };

  for (const group of renderedGroups.value) {
    markExpanded(group.items);
  }

  expandedItems.value = set;
};

onMounted(async () => {
  expandByRoute();
  try {
    await userStore.fetchUserContext();
    // 调试菜单数据结构
    // console.log("菜单数据:", menuResponse.value);
  } catch (e) {
    console.error("初始化用户数据失败:", e);
  }
});
watch(
  () => route.path,
  () => expandByRoute()
);

watch(
  () => menuResponse.value,
  () => expandByRoute(),
  { deep: true }
);

watch(
  () => renderedGroups.value,
  () => expandByRoute(),
  { deep: true }
);

/* ---------- a11y：简单键盘支持 ---------- */
function onTreeKeydown(e: KeyboardEvent) {
  const target = e.target as HTMLElement;
  if (!target) return;
  if (
    e.key === "ArrowRight" &&
    target.getAttribute("aria-expanded") === "false"
  ) {
    target.dispatchEvent(new Event("click", { bubbles: true }));
    e.preventDefault();
  } else if (
    e.key === "ArrowLeft" &&
    target.getAttribute("aria-expanded") === "true"
  ) {
    target.dispatchEvent(new Event("click", { bubbles: true }));
    e.preventDefault();
  }
}
</script>

<template>
  <aside
    :class="collapsed ? 'w-16' : 'w-64'"
    class="bg-white/95 dark:bg-gray-900/95 backdrop-blur-sm border-r border-gray-200/60 dark:border-gray-700/60 shadow-lg flex flex-col h-screen relative z-30 transition-[width] duration-200"
  >
    <!-- 顶部：Logo + 折叠按钮 -->
    <div
      class="flex items-center justify-between h-16 border-b border-gray-200/60 dark:border-gray-700/60 bg-gradient-to-r from-blue-50 to-purple-50 dark:from-blue-900/30 dark:to-purple-900/30 px-2"
    >
      <NuxtLink :to="$localePath('/')" class="flex items-center space-x-2 px-2">
        <img :src="LOGO_M_URL" alt="Logo" class="w-8 h-8 rounded-lg" />
        <span
          v-if="!collapsed"
          class="text-xl font-bold bg-gradient-to-r from-blue-600 to-purple-600 bg-clip-text text-transparent"
        >
          PowerX
        </span>
      </NuxtLink>
      <button
        class="p-2 rounded-lg hover:bg-blue-50 dark:hover:bg-blue-900/20 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/40 transition-colors"
        @click="collapsed = !collapsed"
        :aria-label="
          collapsed
            ? t('common.expand', '展开侧栏')
            : t('common.collapse', '折叠侧栏')
        "
      >
        <UIcon
          :name="
            collapsed
              ? 'i-heroicons-chevron-double-right'
              : 'i-heroicons-chevron-double-left'
          "
          class="w-4 h-4 text-gray-600 dark:text-gray-300"
        />
      </button>
    </div>

    <!-- 菜单 -->
    <nav class="flex-1 overflow-y-auto py-4" @keydown="onTreeKeydown">
      <!-- 加载 -->
      <div v-if="menuLoading" class="px-3">
        <div class="space-y-2">
          <div v-for="i in 5" :key="i" class="animate-pulse">
            <div class="flex items-center space-x-3 px-3 py-2">
              <div class="w-5 h-5 bg-gray-200 rounded"></div>
              <div class="h-4 bg-gray-200 rounded w-2/3"></div>
            </div>
          </div>
        </div>
      </div>

      <!-- 错误 -->
      <div v-else-if="menuError" class="px-3">
        <div class="bg-red-50/70 border border-red-200 rounded-lg p-4">
          <div class="flex items-center space-x-2 text-red-700 mb-2">
            <UIcon class="w-5 h-5" name="i-heroicons-exclamation-triangle" />
            <span class="font-medium">{{ $t("menu.loadFailed") }}</span>
          </div>
          <p class="text-sm text-red-600 mb-3">
            {{ $t("menu.loadFailedDesc") }}
          </p>
          <UButton
            @click="() => refreshMenus()"
            size="xs"
            color="error"
            variant="soft"
          >
            {{ $t("common.reload") }}
          </UButton>
        </div>
      </div>

      <!-- 列表（分组渲染） -->
      <ul v-else class="space-y-1 px-3" role="tree" aria-label="主菜单">
        <li
          v-if="renderedGroups.length === 0"
          class="bg-gray-50 dark:bg-gray-800/50 border border-gray-200 dark:border-gray-700 p-4 rounded-lg text-center"
        >
          <div class="text-gray-500 dark:text-gray-400 text-sm">
            {{ $t("menu.noItemsFound") }}
          </div>
        </li>

        <template v-for="group in renderedGroups" :key="group.id">
          <!-- Sticky 分组 Header -->
          <li
            class="mt-4 first:mt-2 mb-1 px-2 sticky top-0 z-10 bg-white/95 dark:bg-gray-900/95 backdrop-blur-sm"
          >
            <div
              class="text-xs font-semibold tracking-wide text-gray-500 dark:text-gray-400 flex items-center justify-between"
            >
              <span class="uppercase truncate">
                <template v-if="collapsed">{{
                  group.title?.[0] || ""
                }}</template>
                <template v-else>{{ group.title }}</template>
              </span>
            </div>
            <div class="mt-2 h-px bg-gray-200/70 dark:bg-gray-700/70"></div>
          </li>

          <!-- 组内顶层项 -->
          <template v-if="group.id === MARKET_CATEGORY_ID">
            <li
              v-for="subCategory in group.items"
              :key="group.id + ':' + subCategory.id"
              class="mt-3"
            >
              <div
                :class="[
                  'flex items-center rounded-md text-slate-500 dark:text-slate-400 uppercase tracking-wide',
                  collapsed ? 'justify-center px-2 text-xs' : 'px-3 py-1 text-xs',
                ]"
              >
                <span class="inline-block w-4 h-4 mr-2" v-if="!collapsed">
                  <UIcon class="w-4 h-4" :name="resolveIcon(subCategory.icon)" />
                </span>
                <span class="truncate">
                  {{
                    collapsed
                      ? subCategory.title
                        ? subCategory.title[0]
                        : ''
                      : subCategory.title
                  }}
                </span>
              </div>
              <ul v-show="!collapsed" class="mt-1 space-y-1" role="group">
                <SidebarMenuItem
                  v-for="item in subCategory.children || []"
                  :key="subCategory.id + ':' + item.id"
                  :item="item"
                  :collapsed="collapsed"
                  :densityClass="densityClass"
                  :expandedItems="expandedItems"
                  :isActive="isActive"
                  :linkFor="linkFor"
                  :resolveIcon="resolveIcon"
                  :toggleExpanded="toggleExpanded"
                  :hasActiveChild="hasActiveChild"
                />
              </ul>
            </li>
          </template>
          <template v-else>
            <SidebarMenuItem
              v-for="item in group.items"
              :key="group.id + ':' + item.id"
              :item="item"
              :collapsed="collapsed"
              :densityClass="densityClass"
              :expandedItems="expandedItems"
              :isActive="isActive"
              :linkFor="linkFor"
              :resolveIcon="resolveIcon"
              :toggleExpanded="toggleExpanded"
              :hasActiveChild="hasActiveChild"
            />
          </template>
        </template>

      </ul>
    </nav>

    <!-- 底部用户信息 + 快捷开关 -->
    <div
      class="mt-auto border-t border-gray-200/60 dark:border-gray-700/60 bg-gradient-to-r from-gray-50 to-blue-50 dark:from-gray-800/50 dark:to-blue-900/30 px-2 py-3 h-[73px] flex items-center"
    >
      <div class="flex items-center gap-3 flex-1 min-w-0">
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
          <UIcon class="w-5 h-5 text-gray-600" name="i-heroicons-user" />
        </div>
        <div v-if="!collapsed" class="flex-1 min-w-0">
          <p
            class="text-sm font-medium text-gray-900 dark:text-gray-100 truncate"
          >
            {{ userStore.displayName || $t("user.admin") }}
          </p>
          <p class="text-xs text-gray-500 dark:text-gray-400 truncate">
            {{ userStore.user?.email || "admin@powerx.com" }}
          </p>
        </div>

        <div class="flex items-center gap-1">
          <button
            class="p-1.5 rounded-md hover:bg-slate-900/5 dark:hover:bg-white/5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-slate-400/40"
            @click="density = density === 'compact' ? 'comfortable' : 'compact'"
            :aria-label="t('common.toggleDensity', '切换密度')"
            title="切换密度"
          >
            <UIcon
              :name="
                density === 'compact'
                  ? 'i-heroicons-arrows-pointing-out'
                  : 'i-heroicons-arrows-pointing-in'
              "
              class="w-5 h-5"
            />
          </button>
          <button
            class="p-1.5 rounded-lg hover:bg-blue-50 dark:hover:bg-blue-900/20 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/40 transition-colors"
            @click="collapsed = !collapsed"
            :aria-label="
              collapsed
                ? t('common.expand', '展开侧栏')
                : t('common.collapse', '折叠侧栏')
            "
            :title="
              collapsed
                ? t('common.expand', '展开侧栏')
                : t('common.collapse', '折叠侧栏')
            "
          >
            <UIcon
              :name="
                collapsed
                  ? 'i-heroicons-chevron-double-right'
                  : 'i-heroicons-chevron-double-left'
              "
              class="w-4 h-4 text-gray-600 dark:text-gray-300"
            />
          </button>
        </div>
      </div>
    </div>
  </aside>
</template>

<style scoped>
@media (prefers-reduced-motion: reduce) {
  .transition-\[max-height,
  opacity\],
  .transition-transform,
  .transition-\[width\] {
    transition: none !important;
  }
}
</style>
