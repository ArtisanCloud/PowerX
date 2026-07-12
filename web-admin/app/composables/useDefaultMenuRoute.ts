import type { MenuCategory, MenuItem } from "~/composables/api/services/menuService";
import { useMenuService } from "~/composables/api/services/menuService";

type MenuAccess = {
  defaultPath: string | null;
  allowedPaths: Set<string>;
};

const firstVisiblePath = (items?: MenuItem[]): string | null => {
  if (!items?.length) return null;
  for (const item of items) {
    if (!item || item.visible === false) continue;
    const childPath = firstVisiblePath(item.children);
    if (childPath) return childPath;
    const path = routePathForItem(item);
    if (path) return path;
  }
  return null;
};

const normalizePath = (input?: string): string => {
  if (!input) return "";
  let value = input.trim();
  if (!value) return "";
  if (/^[a-zA-Z][a-zA-Z0-9+.-]*:/.test(value)) return "";
  if (!value.startsWith("/")) value = `/${value}`;
  value = value.replace(/\/{2,}/g, "/");
  if (value.length > 1 && value.endsWith("/")) value = value.slice(0, -1);
  return value;
};

const stripQueryAndHash = (input?: string): string => {
  const value = String(input || "").trim();
  if (!value) return "";
  const q = value.indexOf("?");
  const h = value.indexOf("#");
  const cut = [q, h].filter((idx) => idx >= 0).sort((a, b) => a - b)[0];
  return cut >= 0 ? value.slice(0, cut) : value;
};

const resolvePluginID = (item: MenuItem): string => {
  const explicit = String((item as any).pluginId || "").trim();
  if (explicit) return explicit;
  const id = String(item.id || "").trim();
  const colonMatch = id.match(/^plugin:([^:]+):/);
  if (colonMatch?.[1]) return colonMatch[1];
  const dottedMatch = id.match(/^plugins\.((?:[^.]+\.){3}[^.]+)/);
  if (dottedMatch?.[1]) return dottedMatch[1];
  return "";
};

const routePathForItem = (item: MenuItem): string => {
  const rawPath = stripQueryAndHash(String(item.path || "").trim());
  if (!rawPath) return "";
  if (String(item.origin || "") !== "plugin" || rawPath.startsWith("/_p/")) {
    return rawPath;
  }
  const pluginID = resolvePluginID(item);
  if (!pluginID) return rawPath;
  const normalizedPath = rawPath.startsWith("/") ? rawPath : `/${rawPath}`;
  return `/_p/${pluginID}/admin${normalizedPath}`;
};

const stripLocalePrefix = (path: string): string => {
  const normalized = normalizePath(path);
  return normalized.replace(/^\/[a-z]{2}(?:-[A-Z]{2})?(?=\/)/, "") || "/";
};

const normalizeMenuRoute = (item: MenuItem): string => normalizePath(routePathForItem(item));

const collectVisiblePaths = (items?: MenuItem[], out = new Set<string>()) => {
  if (!items?.length) return out;
  for (const item of items) {
    if (!item || item.visible === false) continue;
    const visibleChildren = (item.children || []).filter(
      (child) => child && child.visible !== false
    );
    if (visibleChildren.length > 0) {
      collectVisiblePaths(visibleChildren, out);
      continue;
    }
    const path = normalizeMenuRoute(item);
    if (path) out.add(path);
  }
  return out;
};

const pathMatchesMenu = (targetPath: string, menuPath: string): boolean => {
  const target = stripLocalePrefix(targetPath);
  const menu = normalizePath(menuPath);
  if (!target || !menu) return false;
  if (target === menu) return true;
  if (
    /^\/plugins\/[^/]+$/.test(target) &&
    (menu === "/plugins/market" || menu === "/plugins/installed")
  ) {
    return true;
  }
  return target.startsWith(`${menu}/`);
};

export const useDefaultMenuRoute = () => {
  const menuService = useMenuService();

  const loadMenuAccess = async (): Promise<MenuAccess> => {
    const response = await menuService.getUserMenus();
    const categories = (Array.isArray(response.categories)
      ? response.categories
      : []) as MenuCategory[];
    const data = (Array.isArray(response.data) ? response.data : []) as MenuItem[];
    const roots = categories.length > 0
      ? [...categories]
          .sort((a, b) => a.order - b.order)
          .flatMap((category) => category.children || [])
      : data;
    const defaultPath = firstVisiblePath(roots);
    const allowedPaths = collectVisiblePaths(roots);
    return { defaultPath: defaultPath ? normalizePath(defaultPath) : null, allowedPaths };
  };

  const resolveDefaultRoute = async (): Promise<string> => {
    const { defaultPath } = await loadMenuAccess();
    if (defaultPath) return defaultPath;
    throw new Error("no accessible menu route");
  };

  const resolveAccessibleRoute = async (targetPath?: string): Promise<string> => {
    const { defaultPath, allowedPaths } = await loadMenuAccess();
    const target = String(targetPath || "").trim();
    if (target) {
      for (const menuPath of allowedPaths) {
        if (pathMatchesMenu(target, menuPath)) return target;
      }
    }
    if (defaultPath) return defaultPath;
    throw new Error("no accessible menu route");
  };

  const isMenuRouteAllowed = async (targetPath: string): Promise<boolean> => {
    const { allowedPaths } = await loadMenuAccess();
    for (const menuPath of allowedPaths) {
      if (pathMatchesMenu(targetPath, menuPath)) return true;
    }
    return false;
  };

  return { isMenuRouteAllowed, resolveAccessibleRoute, resolveDefaultRoute };
};
