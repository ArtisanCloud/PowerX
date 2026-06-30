import { useApiClient } from "../index";
import type { ApiResponse } from "../types/types";

/** ===== Types ===== */
export interface MenuTitleI18n {
  namespace?: string;
  key?: string;
  default?: string;
}

export interface MenuCategory {
  id: string;
  title: string;
  order: number;
  origin: string;
  children: MenuItem[];
  titleI18n?: MenuTitleI18n;
}

export interface MenuItem {
  id: string;
  title: string;
  icon?: string;
  path?: string;
  children?: MenuItem[];
  badge?: string | number;
  order: number;
  visible: boolean;
  origin: string;
  permissions?: string[];
  parentId?: string;
  slot?: string;
  titleI18n?: MenuTitleI18n;
  pluginVersion?: string;
}

type MenusResponse = {
  /** 后端将来若提供“已排好序的扁平顶层菜单” */
  menus?: unknown[];
  categories?: unknown[];
  i18n?: unknown[];
};

export interface MenuI18nPayload {
  pluginId?: string;
  format?: string;
  defaultNamespace?: string;
  namespaces?: string[];
  locales?: Record<string, Record<string, any>>;
}

export type UserMenusResult = ApiResponse<MenuItem[]> & {
  categories: MenuCategory[];
};

const MENU_CACHE_TTL_MS = 10_000;

type UserMenusCacheEntry = {
  key: string;
  fetchedAt: number;
  data: UserMenusResult;
};

const normalizeTenantKey = () => {
  if (!process.client) return "server";
  return String(localStorage.getItem("px_current_tenant_uuid") || "no-tenant");
};

const normalizeTokenKey = () => {
  if (!process.client) return "server";
  const token = String(localStorage.getItem("access_token") || "");
  if (!token) return "anonymous";
  return `${token.length}:${token.slice(0, 12)}:${token.slice(-8)}`;
};

const resolveMenuCacheKey = (locale: string) =>
  `${normalizeTenantKey()}:${normalizeTokenKey()}:${locale}`;

export const invalidateUserMenusCache = () => {
  const cache = useState<UserMenusCacheEntry | null>(
    "px-user-menus-cache",
    () => null
  );
  const inflight = useState<Promise<UserMenusResult> | null>(
    "px-user-menus-inflight",
    () => null
  );
  cache.value = null;
  inflight.value = null;
};

export interface MenuCreateParams {
  title: string;
  icon: string;
  path?: string;
  parentId?: string;
  order: number;
  visible: boolean;
  permissions?: string[];
  badge?: string | number;
}

export interface MenuUpdateParams {
  title?: string;
  icon?: string;
  path?: string;
  parentId?: string;
  order?: number;
  visible?: boolean;
  permissions?: string[];
  badge?: string | number;
}

/** ===== Utils ===== */

const SLOT_ROOT = "group.root" as const;
const KEY_PLUGINS = "plugins" as const;
const KEY_SYSTEM = "system" as const;
const ORIGIN_PLUGIN = "plugin" as const;

const localeParamMap: Record<string, string> = {
  zh: "zh-CN",
  en: "en-US",
  ja: "ja-JP",
  ko: "ko-KR",
};

const backendLocaleToFrontend: Record<string, string> = Object.fromEntries(
  Object.entries(localeParamMap).map(([frontend, backend]) => [backend, frontend])
);

/** DEV 下冻结，帮助发现谁在改顺序 */
function deepFreezeDev<T>(obj: T): T {
  if (process.dev && obj && typeof obj === "object") {
    Object.freeze(obj as any);
    for (const k of Object.keys(obj as any)) {
      const v = (obj as any)[k];
      if (v && typeof v === "object" && !Object.isFrozen(v)) deepFreezeDev(v);
    }
  }
  return obj;
}

function normalizeTitleI18n(raw: unknown): MenuTitleI18n | undefined {
  if (!raw || typeof raw !== "object") return undefined;
  const obj = raw as Record<string, unknown>;
  const key = typeof obj.key === "string" ? obj.key : undefined;
  const namespace = typeof obj.namespace === "string" ? obj.namespace : undefined;
  const def = typeof obj.default === "string" ? obj.default : undefined;
  if (!key && !namespace && !def) return undefined;
  return { key, namespace, default: def };
}

/** 将 unknown 规范化为 MenuItem（递归处理 children；不排序） */
function normalizeMenuItem(raw: unknown): MenuItem {
  const n = (raw ?? {}) as Record<string, unknown>;
  const childrenRaw = Array.isArray(n.children) ? n.children : undefined;

  return {
    id: String(n.id ?? ""),
    title: String(n.title ?? ""),
    icon: typeof n.icon === "string" ? n.icon : undefined,
    path: typeof n.path === "string" ? n.path : undefined,
    // 默认 Infinity，避免其他地方“按 order 排”时把无序项顶到前面
    order: Number.isFinite(n.order as number)
      ? Number(n.order)
      : Number.POSITIVE_INFINITY,
    visible: typeof n.visible === "boolean" ? (n.visible as boolean) : true,
    origin: String(n.origin ?? ""),
    permissions: Array.isArray(n.permissions)
      ? (n.permissions as string[])
      : undefined,
    parentId:
      typeof n.parentId === "string" ? (n.parentId as string) : undefined,
    slot: typeof n.slot === "string" ? (n.slot as string) : undefined,
    children: childrenRaw?.map(normalizeMenuItem),
    // badge 保留到页面层处理（翻译等），这里不处理
    badge: ((): MenuItem["badge"] => {
      const b = n.badge;
      if (typeof b === "string" || typeof b === "number") return b;
      return undefined;
    })(),
    titleI18n: normalizeTitleI18n((n as any).titleI18n),
    pluginVersion:
      typeof n.pluginVersion === "string"
        ? (n.pluginVersion as string)
        : undefined,
  };
}

function normalizeMenuCategory(raw: unknown): MenuCategory {
  const n = (raw ?? {}) as Record<string, unknown>;
  const childrenRaw = Array.isArray(n.children) ? n.children : [];

  return {
    id: String(n.id ?? ""),
    title: typeof n.title === "string" ? (n.title as string) : "",
    order: Number.isFinite(n.order as number)
      ? Number(n.order)
      : Number.POSITIVE_INFINITY,
    origin: String(n.origin ?? ""),
    children: childrenRaw.map(normalizeMenuItem),
    titleI18n: normalizeTitleI18n((n as any).titleI18n),
  };
}

/** 后端顶层排序规则的稳定比较器（仅用于顶层） */
function compareTopLevel(
  a: MenuItem & { _i: number },
  b: MenuItem & { _i: number }
): number {
  const pa = a.slot === SLOT_ROOT ? 0 : 1;
  const pb = b.slot === SLOT_ROOT ? 0 : 1;
  if (pa !== pb) return pa - pb;

  if (a.order !== b.order) return a.order - b.order;

  // title、id 都可能为空字符串；localeCompare 保持一致性
  const ta = a.title ?? "";
  const tb = b.title ?? "";
  if (ta !== tb) return ta.localeCompare(tb);

  const ia = a.id ?? "";
  const ib = b.id ?? "";
  if (ia !== ib) return ia.localeCompare(ib);

  return a._i - b._i; // 兜底：保持稳定
}

/** 优先使用 data.menus（若存在），否则从 categories 恢复顶层 */
function parseMenusFromResponse(
  resp: unknown
): {
  flatMenus: MenuItem[];
  categories: MenuCategory[];
  i18nPayloads: MenuI18nPayload[];
} {
  const data = (resp as any)?.data ?? resp ?? {};
  const menusRaw = Array.isArray((data as any).menus)
    ? ((data as any).menus as unknown[])
    : null;
  if (menusRaw) {
    // 后端已拍好序：仅 normalize，不再排序
    return {
      flatMenus: menusRaw.map(normalizeMenuItem),
      categories: [],
      i18nPayloads: Array.isArray((data as any).i18n)
        ? ((data as any).i18n as MenuI18nPayload[])
        : [],
    };
  }

  const catsRaw = Array.isArray((data as any).categories)
    ? ((data as any).categories as unknown[])
    : [];

  return {
    flatMenus: toTopLevelMenusFromCategories(catsRaw),
    categories: catsRaw.map(normalizeMenuCategory),
    i18nPayloads: Array.isArray((data as any).i18n)
      ? ((data as any).i18n as MenuI18nPayload[])
      : [],
  };
}

/** 从 categories 恢复顶层菜单并按“置顶→插件→系统”稳定排序 */
function toTopLevelMenusFromCategories(categories: unknown[]): MenuItem[] {
  if (!Array.isArray(categories)) return [];

  let seq = 0;
  const top: (MenuItem & { _i: number })[] = [];
  const plugin: (MenuItem & { _i: number })[] = [];
  const system: (MenuItem & { _i: number })[] = [];

  const liftChildren = (
    parentRaw: unknown,
    bucket: (MenuItem & { _i: number })[]
  ) => {
    const p = (parentRaw ?? {}) as Record<string, unknown>;
    const kids = Array.isArray(p.children) ? (p.children as unknown[]) : [];
    for (const k of kids) {
      const child = normalizeMenuItem(k) as MenuItem & { _i: number };
      (child as any)._i = seq++;
      bucket.push(child);
    }
  };

  for (let ci = 0; ci < categories.length; ci++) {
    const cat = (categories[ci] ?? {}) as Record<string, unknown>;
    const catId = typeof cat.id === "string" ? (cat.id as string) : "";
    const kids = Array.isArray(cat.children) ? (cat.children as unknown[]) : [];

    for (let j = 0; j < kids.length; j++) {
      const raw = kids[j];
      const item = normalizeMenuItem(raw) as MenuItem & { _i: number };
      (item as any)._i = seq++;

      // ① 置顶：只要 slot===group.root，不管 origin
      if (item.slot === SLOT_ROOT) {
        top.push(item);
        continue;
      }

      // ② 插件：分类=plugins 或 origin=plugin
      if (catId === KEY_PLUGINS || item.origin === ORIGIN_PLUGIN) {
        plugin.push(item);
        continue;
      }

      // 兼容 system 分类下的 plugins 容器：抬升 children
      if (
        catId === KEY_SYSTEM &&
        item.id === KEY_PLUGINS &&
        Array.isArray(item.children) &&
        item.children.length > 0
      ) {
        liftChildren(raw, plugin);
        continue; // 容器本身不进
      }

      // ③ 其它：系统
      system.push(item);
    }
  }

  // 桶内稳定排序
  top.sort(compareTopLevel);
  plugin.sort(compareTopLevel);
  system.sort(compareTopLevel);

  // 顺序：置顶 → 插件 → 系统
  return [...top, ...plugin, ...system].map(({ _i, ...r }) => r);
}

/** ===== Service ===== */

export const useMenuService = () => {
  const apiClient = useApiClient();
  const nuxtApp = useNuxtApp();
  const i18n = nuxtApp.$i18n as any;
  const locale = i18n?.global?.locale ?? i18n?.locale;
  const mergeLocaleMessage =
    i18n?.global?.mergeLocaleMessage?.bind(i18n.global) ??
    i18n?.mergeLocaleMessage?.bind(i18n);
  const baseUrl = "/admin/menus";

  return {
    /** 获取用户菜单（根据权限过滤）——只返回顶层扁平 MenuItem[]，顺序符合后端规则 */
    getUserMenus: async (options: { force?: boolean } = {}) => {
      const currentLocale = String(locale.value ?? "").trim();
      const resolvedLocale =
        (localeParamMap[currentLocale] ?? currentLocale) || "zh-CN";
      const cacheKey = resolveMenuCacheKey(resolvedLocale);
      const cache = useState<UserMenusCacheEntry | null>(
        "px-user-menus-cache",
        () => null
      );
      const inflight = useState<Promise<UserMenusResult> | null>(
        "px-user-menus-inflight",
        () => null
      );
      const cached = cache.value;
      if (
        !options.force &&
        cached?.key === cacheKey &&
        Date.now() - cached.fetchedAt < MENU_CACHE_TTL_MS
      ) {
        return cached.data;
      }
      if (!options.force && inflight.value) {
        return inflight.value;
      }

      const run = async () => {
        const res = await apiClient.get<ApiResponse<MenusResponse>>(baseUrl, {
          params: { locale: resolvedLocale },
        });
        const serverResp = (res?.data ?? res) as ApiResponse<MenusResponse>;
        const { flatMenus, categories, i18nPayloads } =
          parseMenusFromResponse(serverResp);

        registerMenuLocales(i18nPayloads, mergeLocaleMessage);

        deepFreezeDev(flatMenus);
        deepFreezeDev(categories);

        const normalized: UserMenusResult = {
          code: serverResp.code ?? 200,
          message: serverResp.message ?? "success",
          data: flatMenus,
          timestamp: serverResp.timestamp,
          categories,
        };
        cache.value = {
          key: cacheKey,
          fetchedAt: Date.now(),
          data: normalized,
        };
        return normalized;
      };

      const promise = run();
      inflight.value = promise;
      try {
        return await promise;
      } finally {
        if (inflight.value === promise) {
          inflight.value = null;
        }
      }
    },

    /** 其余 CRUD 直通 */
    getMenu: (id: string) =>
      apiClient.get<ApiResponse<MenuItem>>(`${baseUrl}/${id}`),
    createMenu: (data: MenuCreateParams) =>
      apiClient.post<ApiResponse<MenuItem>>(baseUrl, data),
    updateMenu: (id: string, data: MenuUpdateParams) =>
      apiClient.put<ApiResponse<MenuItem>>(`${baseUrl}/${id}`, data),
    deleteMenu: (id: string) =>
      apiClient.delete<ApiResponse<null>>(`${baseUrl}/${id}`),
    updateMenuOrder: (menuOrders: { id: string; order: number }[]) =>
      apiClient.post<ApiResponse<null>>(`${baseUrl}/order`, { menuOrders }),
  };
};

function registerMenuLocales(
  payloads: MenuI18nPayload[] | undefined,
  mergeLocaleMessage?: (locale: string, message: Record<string, any>) => void
) {
  if (!payloads?.length || typeof mergeLocaleMessage !== "function") return;

  for (const payload of payloads) {
    const locales = payload?.locales;
    if (!locales) continue;

    for (const [localeCode, namespaces] of Object.entries(locales)) {
      if (!namespaces || typeof namespaces !== "object") continue;

      const messageToMerge: Record<string, any> = {};
      for (const [namespace, value] of Object.entries(namespaces)) {
        if (!value || typeof value !== "object") continue;

        if (payload?.defaultNamespace && namespace === payload.defaultNamespace) {
          Object.assign(messageToMerge, value);
        } else {
          messageToMerge[namespace] = value;
        }
      }

      if (Object.keys(messageToMerge).length > 0) {
        const targetLocale =
          backendLocaleToFrontend[localeCode] ?? localeCode;
        mergeLocaleMessage(targetLocale, messageToMerge);
      }
    }
  }
}
