<script setup lang="ts">
import {
  ref,
  reactive,
  computed,
  h,
  resolveComponent,
  watchEffect,
  onMounted,
  watch,
} from "vue";
import { watchDebounced } from "@vueuse/core";
import { useI18n } from "#imports";
import { storeToRefs } from "pinia";
import { useRoleStore } from "~/stores/role";
import { usePermissionStore } from "~/stores/permission"; // ✅ 权限 store
import SelectTree from "~/components/ui/SelectTree.vue";
import MenuPermissionTree from "~/components/settings/users/MenuPermissionTree.vue";
import { useOneShotAlert } from "~/composables/useOneShotAlert";
import { useTenantService } from "~/composables/api/services/tenantService";
import { normalizeApiError } from "~/composables/api/normalizeApiError";

const { t, te, locale } = useI18n();
const { notifyOnce, visible, title, description, color, variant, hide } =
  useOneShotAlert();

const tenantService = useTenantService();

/** ====== 类型 ====== */
type Role = {
  id: number;
  name: string;
  code: string;
  description?: string;
  userCount?: number;
  builtin?: boolean; // ✅ 与模板字段一致
};

type Permission = {
  id: number;
  plugin?: string;
  resource?: string;
  action?: string;
  name: string;
  code: string;
  module: string;
  description?: string;
  type: string;
  apiEndpoint?: string;
  httpMethod?: "GET" | "POST" | "PUT" | "DELETE" | "PATCH";
  dataScope?: "own" | "department" | "company" | "all";
  meta?: {
    label?: string;
    plugin_id?: string;
    plugin_name?: string;
    menu_id?: string;
    origin?: string;
  };
};

type MenuPermissionNode = {
  key: string;
  label: string;
  icon: string;
  hint?: string;
  hidden?: boolean;
  permission?: Permission;
  children: MenuPermissionNode[];
};

type MenuPermissionSection = {
  key: string;
  label: string;
  icon: string;
  nodes: MenuPermissionNode[];
};

type CapabilityTypeGroup = {
  key: "operation" | "api";
  label: string;
  types: Record<string, Permission[]>;
  count: number;
};

type CapabilityModuleGroup = {
  key: string;
  label: string;
  typeGroups: CapabilityTypeGroup[];
  count: number;
};

type CapabilitySourceGroup = {
  key: string;
  label: string;
  hint?: string;
  icon: string;
  modules: CapabilityModuleGroup[];
  count: number;
};

/** ====== Pinia Store ====== */
const roleStore = useRoleStore();
const { roles } = storeToRefs(roleStore);
roleStore.ensureInitialized?.();

const permissionStore = usePermissionStore();
const { normalizedList, roleSelection, pluginCatalog } =
  storeToRefs(permissionStore);

// 租户相关状态
interface TreeNode {
  label: string;
  value: string;
  children?: TreeNode[];
  disabled?: boolean;
  icon?: string;
}

const selectedTenant = ref<string | null>(null);
const tenants = ref<any[]>([]);
const loadingTenants = ref(false);
const isRootUser = ref(true); // 假设当前是 root 用户，实际应该从用户状态获取

// 租户树形选项计算属性
const tenantTreeItems = computed<TreeNode[]>(() => {
  return tenants.value.map((t) => ({
    label: t.name,
    value: t.uuid || String(t.id),
    icon: "i-heroicons-building-office",
    disabled: t.status === 0, // 假设 status 为 0 表示禁用
  }));
});

// 监听租户选择变化，同步到表单
watch(selectedTenant, (tenantUuid) => {
  roleForm.tenant_uuid = tenantUuid || undefined;
});

/** 页面展示用权限数组 */
const getPermissionDisplayName = (perm: Permission) => {
  const segs = [
    (perm as any).plugin,
    (perm as any).resource,
    (perm as any).action,
  ].filter(Boolean);
  if (segs.length === 3) return segs.join(".");

  // 很多项目会把 “plugin.resource.action” 放在 code 里
  if (perm.code && perm.code.includes(".")) return perm.code;

  if (import.meta.dev) {
    if (
      !(perm as any).plugin ||
      !(perm as any).resource ||
      !(perm as any).action
    ) {
      console.warn("权限缺少 plugin/resource/action：", perm);
    }
  }

  // 兜底：name
  return perm.name;
};

const permissions = computed<Permission[]>(() => normalizedList.value as any);
const menuPermissions = computed(() =>
  permissions.value
    .filter(
      (p) =>
        (p.module === "menu" || p.type === "menu") &&
        !isPluginMenuPermission(p),
    )
    .sort(
      (a, b) =>
        menuPermissionOrder(a.resource, a) - menuPermissionOrder(b.resource, b),
    ),
);
const isPluginSourcePermission = (perm: Permission) =>
  [
    String((perm as any).__raw?.source || ""),
    String((perm as any).__raw?.module || ""),
    String(perm.module || ""),
    String(perm.meta?.plugin_id || ""),
  ].some(
    (value) =>
      value.startsWith("plugin:") ||
      value.startsWith("com.powerx.plugin.") ||
      value.startsWith("com.powerx.plugins."),
  );
const isPluginMenuPermission = (perm: Permission) =>
  isPluginSourcePermission(perm) || Boolean(perm.resource?.startsWith("plugin."));
const nonMenuPermissions = computed(() =>
  permissions.value.filter(
    (p) =>
      !(p.module === "menu" || p.type === "menu") &&
      !isPluginSourcePermission(p),
  ),
);

/** ====== 当前选中角色 ====== */
const selectedRole = ref<Role | null>((roles.value as any[])[0] ?? null);
watchEffect(() => {
  if (!selectedRole.value && roles.value.length > 0) {
    selectedRole.value = roles.value[0] as unknown as Role;
  }
});

/** 选中角色变化时拉取其权限ID */
watch(selectedRole, async (r) => {
  if (r?.id) {
    await permissionStore.fetchRolePermissionIDs(r.id);
  }
});

/** ====== 搜索 & 表单状态 ====== */
const searchQuery = ref("");
const permissionSearchQuery = ref("");
const PERMISSION_FILTER_ALL = "__all__";
const PERMISSION_FILTER_CORE = "__core__";
const pluginPermissionPluginFilter = ref(PERMISSION_FILTER_ALL);
const pluginPermissionModuleFilter = ref(PERMISSION_FILTER_ALL);
const pluginPermissionTypeFilter = ref(PERMISSION_FILTER_ALL);
const pluginPermissionRegistrationFilter = ref(PERMISSION_FILTER_ALL);
const selectedCapabilitySourceKey = ref("");
const selectedCapabilityModuleKey = ref("");
const showRoleForm = ref(false);
const isEditing = ref(false);
const editingId = ref<number | null>(null);
const permissionTab = ref("menus");
const roleFormPermissionTab = ref("menus");
const permissionTabItems = [
  { label: "菜单权限", value: "menus", icon: "i-heroicons-bars-3" },
  {
    label: "能力/API权限",
    value: "capabilities",
    icon: "i-heroicons-command-line",
  },
];

type CapabilityFilterGroup = "operation" | "api";

const excludedCapabilityTypes = new Set(["menu", "api_key", "api_candidate"]);

const normalizePermissionSearchText = (value: unknown) =>
  String(value ?? "").trim().toLowerCase();

const normalizedPermissionSearchQuery = computed(() =>
  normalizePermissionSearchText(permissionSearchQuery.value),
);

const hasPermissionSearch = computed(
  () => normalizedPermissionSearchQuery.value.length > 0,
);

const permissionSearchMatches = (...values: unknown[]) => {
  const q = normalizedPermissionSearchQuery.value;
  if (!q) return true;
  return values
    .flatMap((value) => {
      if (Array.isArray(value)) return value;
      if (value && typeof value === "object") return Object.values(value);
      return [value];
    })
    .some((value) => normalizePermissionSearchText(value).includes(q));
};

const corePermissionRegistrationStatus = (perm: Permission) =>
  String((perm as any).__raw?.status || (perm as any).status || "active") ===
  "active"
    ? "registered"
    : "invalid";

const classifyCapabilityTypeGroup = (type: string): CapabilityFilterGroup =>
  String(type || "") === "api" ? "api" : "operation";

const capabilityTypeMatchesFilter = (type: string) =>
  pluginPermissionTypeFilter.value === PERMISSION_FILTER_ALL ||
  classifyCapabilityTypeGroup(type) === pluginPermissionTypeFilter.value;

const isVisibleCapabilityType = (type: unknown) => {
  const normalized = String(type || "").trim();
  return normalized !== "" && !excludedCapabilityTypes.has(normalized);
};

const permissionMatchesCapabilityFilters = (perm: Permission) => {
  if (String((perm as any).__raw?.status || (perm as any).status || "") !== "active") {
    return false;
  }
  if (isPluginSourcePermission(perm)) {
    return false;
  }
  if (String(perm.meta?.type || "") === "api_candidate") {
    return false;
  }
  if (!isVisibleCapabilityType(perm.type)) {
    return false;
  }
  if (["platform_capability_generated", "swagger"].includes(String((perm as any).__raw?.source || ""))) {
    return false;
  }
  if (
    perm.type === "api" &&
    !Object.values((perm.meta?.title_i18n || {}) as Record<string, string>).some(
      (value) => String(value || "").trim(),
    )
  ) {
    return false;
  }
  if (pluginPermissionPluginFilter.value !== PERMISSION_FILTER_ALL) {
    if (pluginPermissionPluginFilter.value !== PERMISSION_FILTER_CORE) {
      return false;
    }
  }
  if (
    pluginPermissionModuleFilter.value !== PERMISSION_FILTER_ALL &&
    perm.module !== pluginPermissionModuleFilter.value
  ) {
    return false;
  }
  if (
    pluginPermissionTypeFilter.value !== PERMISSION_FILTER_ALL &&
    !capabilityTypeMatchesFilter(perm.type)
  ) {
    return false;
  }
  if (
    pluginPermissionRegistrationFilter.value !== PERMISSION_FILTER_ALL &&
    corePermissionRegistrationStatus(perm) !==
      pluginPermissionRegistrationFilter.value
  ) {
    return false;
  }
  return true;
};

const filteredNonMenuPermissions = computed(() => {
  return nonMenuPermissions.value.filter((perm) => {
    if (!permissionMatchesCapabilityFilters(perm)) return false;
    return permissionSearchMatches(
      perm.name,
      perm.code,
      perm.description,
      perm.module,
      perm.plugin,
      perm.resource,
      perm.action,
      perm.type,
      perm.apiEndpoint,
      perm.httpMethod,
      perm.meta?.label,
      perm.meta?.plugin_id,
      perm.meta?.plugin_name,
      perm.meta?.menu_id,
      (perm as any).__raw?.source,
    );
  });
});

const roleForm = reactive({
  name: "",
  code: "",
  description: "",
  scope: "tenant" as "system" | "tenant",
  tenant_uuid: undefined as string | undefined,
  permissions: [] as number[],
});

/** ====== 首屏加载权限 & 当前角色权限 ====== */
onMounted(async () => {
  await Promise.all([
    permissionStore.fetchAllActive(),
    permissionStore.fetchPluginCatalog(),
  ]);
  if (selectedRole.value?.id) {
    await permissionStore.fetchRolePermissionIDs(selectedRole.value.id);
  }
});

const capabilityTypeGroupLabel = (group: CapabilityFilterGroup) =>
  group === "api"
    ? t("organization.permission.pluginCatalog.groups.api")
    : t("organization.permission.pluginCatalog.groups.operation");

const buildCoreCapabilitySourceGroup = (
  items: Permission[],
): CapabilitySourceGroup | null => {
  if (items.length === 0) return null;
  const moduleMap = new Map<
    string,
    Map<CapabilityFilterGroup, Record<string, Permission[]>>
  >();
  for (const perm of items) {
    const moduleKey = perm.module || "core";
    const groupKey = classifyCapabilityTypeGroup(perm.type);
    if (!moduleMap.has(moduleKey)) moduleMap.set(moduleKey, new Map());
    const groups = moduleMap.get(moduleKey)!;
    if (!groups.has(groupKey)) groups.set(groupKey, {});
    const typeMap = groups.get(groupKey)!;
    if (!typeMap[perm.type]) typeMap[perm.type] = [];
    typeMap[perm.type].push(perm);
  }
  const modules = Array.from(moduleMap.entries())
    .sort(([a], [b]) => a.localeCompare(b, "zh-CN"))
    .map(([module, groups]) => {
      const typeGroups = Array.from(groups.entries())
        .sort(([a], [b]) => (a === b ? 0 : a === "operation" ? -1 : 1))
        .map(([group, types]) => ({
          key: group,
          label: capabilityTypeGroupLabel(group),
          types,
          count: Object.values(types).reduce(
            (total, perms) => total + perms.length,
            0,
          ),
        }));
      return {
        key: module,
        label: module,
        typeGroups,
        count: typeGroups.reduce((total, group) => total + group.count, 0),
      };
    });
  return {
    key: PERMISSION_FILTER_CORE,
    label: t("organization.permission.pluginCatalog.sources.core"),
    icon: "i-heroicons-building-library",
    modules,
    count: modules.reduce((total, module) => total + module.count, 0),
  };
};

const coreCapabilitySourceGroup = computed(() =>
  buildCoreCapabilitySourceGroup(filteredNonMenuPermissions.value),
);

const menuLabels: Record<
  string,
  { label: string; group: string; icon: string }
> = {
  dashboard: {
    label: "仪表盘",
    group: "PINNED",
    icon: "i-heroicons-arrow-trending-up",
  },
  agent: { label: "智能体", group: "PINNED", icon: "i-heroicons-sparkles" },
  "agent.chat": {
    label: "智能会话",
    group: "PINNED",
    icon: "i-heroicons-chat-bubble-left-right",
  },
  "agent.plugin_chat": {
    label: "智能会话",
    group: "PINNED",
    icon: "i-heroicons-chat-bubble-left-right",
  },
  "agent.management": {
    label: "智能体管理",
    group: "PINNED",
    icon: "i-heroicons-squares-2x2",
  },
  "agent.team": {
    label: "团队管理",
    group: "PINNED",
    icon: "i-heroicons-user-group",
  },
  "agent.team_tasks": {
    label: "团队任务",
    group: "PINNED",
    icon: "i-heroicons-list-bullet",
  },
  "agent.traces": {
    label: "Agent 运行追踪",
    group: "PINNED",
    icon: "i-heroicons-document-magnifying-glass",
  },
  skills: {
    label: "技能库",
    group: "PINNED",
    icon: "i-heroicons-squares-plus",
  },
  knowledge: {
    label: "知识空间",
    group: "PINNED",
    icon: "i-heroicons-book-open",
  },
  workflow: {
    label: "流程",
    group: "PINNED",
    icon: "i-heroicons-arrow-path-rounded-square",
  },
  media: { label: "媒体", group: "PINNED", icon: "i-heroicons-photo" },
  monitor: { label: "监控中心", group: "SETTINGS", icon: "i-heroicons-eye" },
  plugins: {
    label: "插件市场",
    group: "SETTINGS",
    icon: "i-heroicons-puzzle-piece",
  },
  "plugins.market": {
    label: "插件管理",
    group: "SETTINGS",
    icon: "i-heroicons-building-storefront",
  },
  "plugins.subscriptions": {
    label: "插件订阅",
    group: "SETTINGS",
    icon: "i-heroicons-building-storefront",
  },
  "plugins.capabilities": {
    label: "插件能力",
    group: "SETTINGS",
    icon: "i-heroicons-table-cells",
  },
  "plugins.release": {
    label: "插件发布候选",
    group: "SETTINGS",
    icon: "i-heroicons-queue-list",
  },
  settings: {
    label: "设置",
    group: "SETTINGS",
    icon: "i-heroicons-cog-6-tooth",
  },
  "settings.users": {
    label: "用户管理",
    group: "SETTINGS",
    icon: "i-heroicons-users",
  },
  "settings.roles": {
    label: "角色管理",
    group: "SETTINGS",
    icon: "i-heroicons-key",
  },
  "settings.config": {
    label: "系统配置",
    group: "SETTINGS",
    icon: "i-heroicons-wrench-screwdriver",
  },
  "settings.metadata_governance": {
    label: "元数据治理",
    group: "SETTINGS",
    icon: "i-lucide-tags",
  },
  "settings.ai": {
    label: "AI 设置",
    group: "SETTINGS",
    icon: "i-heroicons-cpu-chip",
  },
  "settings.ai.model": {
    label: "模型设置",
    group: "SETTINGS",
    icon: "i-heroicons-cog-6-tooth",
  },
  "settings.ai.cost": {
    label: "成本设置",
    group: "SETTINGS",
    icon: "i-heroicons-banknotes",
  },
  "settings.ai.context_optimizer": {
    label: "上下文优化",
    group: "SETTINGS",
    icon: "i-heroicons-adjustments-horizontal",
  },
  "settings.ai.agent_access": {
    label: t("menu.aiSettingsAgentAccess"),
    group: "SETTINGS",
    icon: "i-heroicons-shield-check",
  },
  "settings.integration_api_keys": {
    label: "API Key 管理",
    group: "SETTINGS",
    icon: "i-heroicons-key",
  },
  "settings.open_capabilities": {
    label: t("menu.openCapabilities"),
    group: "SETTINGS",
    icon: "i-heroicons-bolt",
  },
  "settings.event_fabric": {
    label: t("menu.eventFabric"),
    group: "SETTINGS",
    icon: "i-heroicons-queue-list",
  },
};

const menuSectionOrder: Record<string, number> = {
  PINNED: -200,
  APPS: -50,
  SETTINGS: 0,
};

const menuOrder = [
  "agent",
  "agent.chat",
  "agent.plugin_chat",
  "agent.management",
  "agent.team",
  "agent.team_tasks",
  "agent.traces",
  "skills",
  "knowledge",
  "workflow",
  "media",
  "dashboard",
  "monitor",
  "plugins",
  "plugins.market",
  "plugins.subscriptions",
  "plugins.capabilities",
  "plugins.release",
  "settings",
  "settings.users",
  "settings.roles",
  "settings.config",
  "settings.metadata_governance",
  "settings.ai",
  "settings.ai.model",
  "settings.ai.cost",
  "settings.ai.context_optimizer",
  "settings.ai.agent_access",
  "settings.integration_api_keys",
  "settings.open_capabilities",
  "settings.event_fabric",
];

const menuPermissionOrder = (resource?: string, perm?: Permission) => {
  if ((perm && isPluginMenuPermission(perm)) || resource?.startsWith("plugin.")) {
    return 800;
  }
  const idx = menuOrder.indexOf(resource || "");
  return idx === -1 ? 999 : idx;
};

const getPluginMenuDisplayName = (perm: Permission) => {
  const meta = perm.meta || {};
  const resource = perm.resource || "";
  return (
    meta.plugin_name ||
    meta.plugin_id ||
    resource.replace(/^plugin\./, "").split(".").slice(0, -1).join(".") ||
    "未知 App"
  );
};

const getPluginMenuID = (perm: Permission) =>
  String(perm.meta?.plugin_id || permissionSourcePluginID(perm) || "").trim();

const getMenuPermissionMeta = (perm: Permission) => {
  if (isPluginMenuPermission(perm)) {
    return {
      label: perm.meta?.label || perm.meta?.menu_id || perm.resource,
      group: "APPS",
      icon: "i-heroicons-puzzle-piece",
    };
  }
  return (
    menuLabels[perm.resource || ""] || {
    label: perm.resource || perm.name,
    group: "其他菜单",
    icon: "i-heroicons-bars-3",
    }
  );
};

const splitMenuPath = (perm: Permission) => {
  const menuId = String(perm.meta?.menu_id || "").trim();
  const resource = perm.resource || "";
  if (isPluginMenuPermission(perm)) {
    const pluginId = getPluginMenuID(perm);
    const stripPluginPrefix = (value: string) => {
      let out = value.trim();
      if (out.startsWith("plugin.")) out = out.slice("plugin.".length);
      const prefixes = [
        pluginId ? `plugins.${pluginId}.` : "",
        pluginId ? `${pluginId}.` : "",
        pluginId ? `plugins.${pluginId}` : "",
        pluginId,
      ].filter(Boolean);
      let changed = true;
      while (changed) {
        changed = false;
        for (const prefix of prefixes) {
          if (out === prefix) {
            out = "";
            changed = true;
            break;
          }
          const dotted = `${prefix}.`;
          if (out.startsWith(dotted)) {
            out = out.slice(dotted.length);
            changed = true;
            break;
          }
        }
      }
      return out;
    };
    const raw = stripPluginPrefix(menuId || resource);
    const pluginNode = `__plugin:${pluginId || getPluginMenuDisplayName(perm)}`;
    if (!raw) return [pluginNode];
    const parts = raw.split(".").filter(Boolean);
    return [pluginNode, ...parts];
  }
  return (menuId || resource).split(".").filter(Boolean);
};

const fallbackMenuNodeLabel = (segment: string) =>
  segment
    .replace(/[_-]+/g, " ")
    .replace(/\b\w/g, (char) => char.toUpperCase());

const makeVirtualMenuNode = (
  key: string,
  segment: string,
  perm?: Permission,
): MenuPermissionNode => {
  if (segment.startsWith("__plugin:")) {
    return {
      key,
      label: perm ? getPluginMenuDisplayName(perm) : segment.slice("__plugin:".length),
      icon: "i-heroicons-puzzle-piece",
      children: [],
    };
  }
  const meta = menuLabels[key];
  return {
    key,
    label: meta?.label || fallbackMenuNodeLabel(segment),
    icon: meta?.icon || "i-heroicons-folder",
    children: [],
  };
};

const insertMenuPermissionNode = (
  roots: MenuPermissionNode[],
  pathParts: string[],
  perm: Permission,
) => {
  const meta = getMenuPermissionMeta(perm);
  const parts = pathParts.length ? pathParts : [`__root_${perm.id}`];
  let cursor = roots;
  let accumulated = "";
  parts.forEach((part, index) => {
    accumulated = accumulated ? `${accumulated}.${part}` : part;
    let node = cursor.find((item) => item.key === accumulated);
    if (!node) {
      node = makeVirtualMenuNode(accumulated, part, perm);
      cursor.push(node);
    }
    if (index === parts.length - 1) {
      node.label = meta.label;
      node.icon = meta.icon;
      node.hint = pathParts.length ? parts.join(" / ") : undefined;
      node.hidden = pathParts.length === 0;
      node.permission = perm;
    }
    cursor = node.children;
  });
};

const pruneHiddenRootMenuNodes = (nodes: MenuPermissionNode[]) => {
  for (let index = nodes.length - 1; index >= 0; index--) {
    const node = nodes[index];
    pruneHiddenRootMenuNodes(node.children);
    if (node.hidden) {
      nodes.splice(index, 1, ...node.children);
    }
  }
};

const sortMenuNodes = (nodes: MenuPermissionNode[]) => {
  nodes.sort((a, b) => {
    const ao = a.permission
      ? menuPermissionOrder(a.permission.resource, a.permission)
      : 999;
    const bo = b.permission
      ? menuPermissionOrder(b.permission.resource, b.permission)
      : 999;
    if (ao !== bo) return ao - bo;
    return a.label.localeCompare(b.label, "zh-CN");
  });
  nodes.forEach((node) => sortMenuNodes(node.children));
};

const allMenuPermissionSections = computed<MenuPermissionSection[]>(() => {
  const sections = new Map<string, MenuPermissionSection>();
  for (const perm of menuPermissions.value) {
    const meta = getMenuPermissionMeta(perm);
    const sectionKey = `system:${meta.group}`;
    let section = sections.get(sectionKey);
    if (!section) {
      section = {
        key: sectionKey,
        label: meta.group,
        icon: meta.group === "APPS"
          ? "i-heroicons-puzzle-piece"
          : "i-heroicons-bars-3",
        nodes: [],
      };
      sections.set(sectionKey, section);
    }
    insertMenuPermissionNode(section.nodes, splitMenuPath(perm), perm);
  }

  const pluginMenuNodes = pluginPermissionPlugins.value
    .map((plugin) => pluginMenuRootNode(plugin))
    .filter((node): node is MenuPermissionNode => Boolean(node))
    .sort((a, b) => a.label.localeCompare(b.label, "zh-CN"));
  if (pluginMenuNodes.length > 0) {
    const sectionKey = "system:APPS";
    let section = sections.get(sectionKey);
    if (!section) {
      section = {
        key: sectionKey,
        label: "APPS",
        icon: "i-heroicons-puzzle-piece",
        nodes: [],
      };
      sections.set(sectionKey, section);
    }
    section.nodes.push(...pluginMenuNodes);
  }

  const out = Array.from(sections.values());
  out.forEach((section) => {
    pruneHiddenRootMenuNodes(section.nodes);
    if (section.label !== "APPS") {
      sortMenuNodes(section.nodes);
    }
  });
  return out.sort((a, b) => {
    const orderDelta =
      (menuSectionOrder[a.label] ?? 999) - (menuSectionOrder[b.label] ?? 999);
    if (orderDelta !== 0) return orderDelta;
    return a.label.localeCompare(b.label, "zh-CN");
  });
});

const menuNodeMatchesSearch = (node: MenuPermissionNode) => {
  const perm = node.permission;
  return permissionSearchMatches(
    node.label,
    node.hint,
    node.key,
    perm?.name,
    perm?.code,
    perm?.description,
    perm?.module,
    perm?.resource,
    perm?.action,
    perm?.meta?.label,
    perm?.meta?.plugin_id,
    perm?.meta?.plugin_name,
    perm?.meta?.menu_id,
  );
};

const pluginPermissionCatalogItemByID = computed(() => {
  const out = new Map<
    number,
    {
      pluginID: string;
      module: string;
      type: string;
      registrationStatus: string;
    }
  >();
  pluginPermissionPlugins.value.forEach((plugin) => {
    collectPluginCatalogItems(plugin).forEach((item) => {
      out.set(Number(item.id), {
        pluginID: plugin.plugin_id,
        module: item.module,
        type: item.type,
        registrationStatus: item.registration_status,
      });
    });
  });
  return out;
});

const permissionSourcePluginID = (perm?: Permission) => {
  const source = String((perm as any)?.__raw?.source || "");
  if (source.startsWith("plugin:")) return source.slice("plugin:".length);
  return perm?.meta?.plugin_id || "";
};

const filterMenuNodes = (nodes: MenuPermissionNode[]): MenuPermissionNode[] => {
  if (!hasPermissionSearch.value) {
    return nodes;
  }
  return nodes
    .map((node) => {
      const children = filterMenuNodes(node.children);
      const nodeMatched = menuNodeMatchesSearch(node);
      if (nodeMatched || children.length > 0) {
        return { ...node, children };
      }
      return null;
    })
    .filter((node): node is MenuPermissionNode => Boolean(node));
};

const menuPermissionSections = computed<MenuPermissionSection[]>(() => {
  if (!hasPermissionSearch.value) {
    return allMenuPermissionSections.value;
  }
  return allMenuPermissionSections.value
    .map((section) => {
      const sectionMatched = permissionSearchMatches(
        section.label,
        section.key,
      );
      const nodes = sectionMatched ? section.nodes : filterMenuNodes(section.nodes);
      return { ...section, nodes };
    })
    .filter((section) => section.nodes.length > 0);
});

const collectMenuNodePermissionIds = (node: MenuPermissionNode): number[] => {
  const ids = new Set<number>();
  if (node.permission) ids.add(node.permission.id);
  for (const child of node.children) {
    for (const id of collectMenuNodePermissionIds(child)) ids.add(id);
  }
  return Array.from(ids);
};

const collectMenuNodeVisibleStateIds = (node: MenuPermissionNode): number[] => {
  if (node.children.length) {
    return node.children.flatMap((child) => collectMenuNodeVisibleStateIds(child));
  }
  return node.permission ? [node.permission.id] : [];
};

const collectMenuSectionPermissionIds = (section: MenuPermissionSection) =>
  Array.from(
    new Set(section.nodes.flatMap((node) => collectMenuNodePermissionIds(node))),
  );

const collectMenuSectionVisibleStateIds = (section: MenuPermissionSection) =>
  Array.from(
    new Set(
      section.nodes.flatMap((node) => collectMenuNodeVisibleStateIds(node)),
    ),
  );

const toggleMenuPermissionIds = (ids: number[], checked: boolean) => {
  const roleId = selectedRole.value?.id;
  if (!roleId) return;
  const set = new Set(roleSelection.value[roleId] || []);
  if (checked) ids.forEach((id) => set.add(id));
  else ids.forEach((id) => set.delete(id));
  roleSelection.value[roleId] = Array.from(set);
};

const isMenuPermissionIdsFullySelected = (ids: number[]) => {
  const roleId = selectedRole.value?.id;
  if (!roleId) return false;
  const cur = new Set(roleSelection.value[roleId] || []);
  return ids.length > 0 && ids.every((id) => cur.has(id));
};

const isMenuPermissionIdsPartiallySelected = (ids: number[]) => {
  const roleId = selectedRole.value?.id;
  if (!roleId) return false;
  const cur = new Set(roleSelection.value[roleId] || []);
  const picked = ids.filter((id) => cur.has(id)).length;
  return picked > 0 && picked < ids.length;
};

const toggleMenuSectionPermissions = (
  section: MenuPermissionSection,
  checked: boolean,
) => toggleMenuPermissionIds(collectMenuSectionPermissionIds(section), checked);

const toggleMenuNodePermissions = (node: MenuPermissionNode, checked: boolean) =>
  toggleMenuPermissionIds(collectMenuNodePermissionIds(node), checked);

const currentRoleMenuPermissionIds = computed(() => {
  const roleId = selectedRole.value?.id;
  return roleId ? roleSelection.value[roleId] || [] : [];
});

const isMenuSectionFullySelected = (section: MenuPermissionSection) =>
  isMenuPermissionIdsFullySelected(collectMenuSectionVisibleStateIds(section));

const isMenuSectionPartiallySelected = (section: MenuPermissionSection) =>
  isMenuPermissionIdsPartiallySelected(collectMenuSectionVisibleStateIds(section));

const menuSectionCheckboxValue = (section: MenuPermissionSection) => {
  if (isMenuSectionFullySelected(section)) return true;
  if (isMenuSectionPartiallySelected(section)) return "indeterminate";
  return false;
};

const formMenuSectionCheckboxValue = (section: MenuPermissionSection) => {
  const ids = collectMenuSectionVisibleStateIds(section);
  if (ids.length === 0) return false;
  const picked = ids.filter((id) => roleForm.permissions.includes(id)).length;
  if (picked === ids.length) return true;
  if (picked > 0) return "indeterminate";
  return false;
};

/** ====== 工具函数 ====== */
const resetRoleForm = () => {
  roleForm.name = "";
  roleForm.code = "";
  roleForm.description = "";
  roleForm.scope = "tenant";
  roleForm.tenant_uuid = undefined;
  roleForm.permissions = [];
  selectedTenant.value = null;
  isEditing.value = false;
  editingId.value = null;
  roleFormPermissionTab.value = "menus";
};

const openAddRoleForm = () => {
  resetRoleForm();
  loadTenantOptions(); // 加载租户选项
  showRoleForm.value = true;
};

// 加载租户选项
const loadTenantOptions = async () => {
  if (!isRootUser.value) return;
  loadingTenants.value = true;
  try {
    // 这里应该调用实际的租户 API
    const response = await tenantService.getTenants({
      page: 1,
      page_size: 100,
    });
    if (response?.code === 200 && response.data) {
      tenants.value = response.data.items;
    }
  } catch (err) {
    console.error("加载租户列表失败:", err);
  } finally {
    loadingTenants.value = false;
  }
};

const openEditRoleForm = (role: Role) => {
  roleForm.name = role.name;
  roleForm.code = role.code;
  roleForm.description = role.description || "";
  roleForm.permissions = [...(roleSelection.value[role.id] || [])];
  isEditing.value = true;
  editingId.value = role.id;
  showRoleForm.value = true;
};

const saveRole = async () => {
  if (!roleForm.name || !roleForm.code) {
    notifyOnce("请填写必填字段", "角色名称和代码为必填项", "warning" as const);
    return;
  }
  try {
    if (isEditing.value && editingId.value !== null) {
      // 更新角色
      await roleStore.updateRole(editingId.value, {
        name: roleForm.name,
        code: roleForm.code,
        description: roleForm.description,
      });
      // 更新权限
      roleSelection.value[editingId.value] = [...roleForm.permissions];
      await permissionStore.setRolePermissionIDs(
        editingId.value,
        roleForm.permissions,
      );
    } else {
      // 创建角色（直接带权限）
      const result = await roleStore.createRole({
        name: roleForm.name,
        code: roleForm.code,
        description: roleForm.description,
        scope: roleForm.scope,
        tenant_uuid: roleForm.tenant_uuid,
        perm_ids: roleForm.permissions, // 直接传递权限ID
      });

      // 更新本地权限选择状态
      if (result.role?.id) {
        const finalPermIds = result.perm?.now || roleForm.permissions;
        roleSelection.value[result.role.id] = [...finalPermIds];
        // 同时更新初始态
        permissionStore.roleInitialSelection[result.role.id] = [
          ...finalPermIds,
        ];
      }
    }
    showRoleForm.value = false;
    resetRoleForm();
  } catch (error) {
    console.error("保存角色失败:", error);
    const { title, description } = normalizeApiError(error, {
      meta: "metaText",
    });
    notifyOnce(
      title || "保存角色失败",
      description,
      "error" as const,
      "solid" as const,
    );
  }
};

const deleteRole = async (id: number) => {
  const role = roles.value.find((r: any) => r.id === id) as Role | undefined;
  if (role && role.builtin) {
    notifyOnce(
      "系统角色不能删除",
      "内置角色受系统保护，无法删除",
      "warning" as const,
    );
    return;
  }
  if (confirm("确定要删除此角色吗？")) {
    try {
      await roleStore.deleteRole(id);
      delete roleSelection.value[id];
      if (selectedRole.value?.id === id) {
        selectedRole.value = (roles.value[0] as any) ?? null;
      }
    } catch (error) {
      console.error("删除角色失败:", error);
      const { title, description } = normalizeApiError(error, {
        meta: "metaText",
      }); // ✨ 统一解析
      notifyOnce(
        title || "删除失败",
        description,
        "error" as const,
        "solid" as const,
      );
    }
  }
};

const filteredRoles = computed<Role[]>(() => {
  if (!searchQuery.value) return roles.value as any;
  const q = searchQuery.value.toLowerCase();
  return (roles.value as any).filter(
    (r: Role) =>
      r.name.toLowerCase().includes(q) ||
      r.code.toLowerCase().includes(q) ||
      (r.description || "").toLowerCase().includes(q),
  );
});

const selectRole = (role: Role) => {
  selectedRole.value = role;
};

/** —— 当前角色权限 —— */
const hasPermission = (permissionId: number) => {
  const roleId = selectedRole.value?.id;
  if (!roleId) return false;
  return (roleSelection.value[roleId] || []).includes(permissionId);
};

const pluginDisplayLabel = (plugin: any) =>
  String(plugin?.plugin_name || plugin?.name || plugin?.plugin_id || "").trim();

const comparePluginCatalogItems = (a: any, b: any) => {
  const labelDelta = pluginDisplayLabel(a).localeCompare(
    pluginDisplayLabel(b),
    "zh-CN",
  );
  if (labelDelta !== 0) return labelDelta;
  return String(a?.plugin_id || "").localeCompare(String(b?.plugin_id || ""), "en");
};

const pluginPermissionPlugins = computed(() =>
  [...(pluginCatalog.value.plugins || [])].sort(comparePluginCatalogItems),
);

const collectPluginMenuPermissions = (nodes: any[] = []): any[] =>
  nodes.flatMap((node) => [
    ...(node.permission ? [node.permission] : []),
    ...collectPluginMenuPermissions(node.children || []),
  ]);

const pluginCatalogPermissionToMenuPermission = (item: any): Permission => ({
  id: Number(item.id),
  name: pluginPermissionTitle(item),
  code: item.effective_permission_code || item.permission_code || "",
  module: item.module || "menu",
  plugin: item.module || "menu",
  resource: item.resource || item.permission_code || "",
  action: item.action || "read",
  description: pluginPermissionDescription(item),
  type: "menu",
  meta: {
    label: pluginPermissionTitle(item),
    plugin_id: item.plugin_id,
    menu_id: (item.menu_path || []).join("."),
  },
});

const pluginCatalogMenuNodeToMenuNode = (
  node: any,
  pluginID: string,
): MenuPermissionNode => {
  const permission = node.permission
    ? {
        ...pluginCatalogPermissionToMenuPermission({
          ...node.permission,
          plugin_id: pluginID,
        }),
      }
    : undefined;
  const children = (node.children || []).map((child: any) =>
    pluginCatalogMenuNodeToMenuNode(child, pluginID),
  );
  const label =
    localeText(node.label_i18n) ||
    (node.permission ? pluginPermissionTitle(node.permission) : "") ||
    fallbackMenuNodeLabel(String(node.key || ""));
  return {
    key: `${pluginID}:${node.key}`,
    label,
    icon: "i-heroicons-puzzle-piece",
    hint: node.permission?.menu_path?.join(" / ") || String(node.key || ""),
    permission,
    children,
  };
};

const pluginMenuRootNode = (plugin: any): MenuPermissionNode | null => {
  const children = (plugin.menu_tree || []).map((node: any) =>
    pluginCatalogMenuNodeToMenuNode(node, plugin.plugin_id),
  );
  if (children.length === 0) return null;
  return {
    key: `plugin:${plugin.plugin_id}`,
    label: plugin.plugin_name || plugin.name || plugin.plugin_id,
    icon: "i-heroicons-puzzle-piece",
    hint: plugin.plugin_id,
    children,
  };
};

const collectPluginCatalogItems = (plugin: any): any[] => [
  ...collectPluginMenuPermissions(plugin.menu_tree || []),
  ...(plugin.business_modules || []).flatMap((module: any) =>
    (module.resources || []).flatMap((resource: any) => [
      ...(resource.pages || []),
      ...(resource.actions || []),
    ]),
  ),
  ...(plugin.api_bindings || []).map((binding: any) => binding.permission),
  ...(plugin.runtime_contracts || []),
].filter(Boolean);

const isRuntimeContractCatalogItem = (item: any) =>
  String(item?.effective_permission_code || item?.permission_code || "").startsWith(
    "runtime.contract:",
  ) ||
  (String(item?.module || "") === "runtime" &&
    String(item?.resource || "") === "contract" &&
    String(item?.action || "") !== "");

const collectPluginCapabilityItems = (plugin: any): any[] =>
  collectPluginCatalogItems(plugin).filter((item) =>
    isVisibleCapabilityType(item.type) && !isRuntimeContractCatalogItem(item),
  );

const pluginPermissionPluginFilterItems = computed(() => [
  {
    label: t("organization.permission.pluginCatalog.filters.allSources"),
    value: PERMISSION_FILTER_ALL,
  },
  {
    label: t("organization.permission.pluginCatalog.sources.core"),
    value: PERMISSION_FILTER_CORE,
  },
  ...pluginPermissionPlugins.value.map((plugin) => ({
    label: plugin.plugin_name || plugin.name || plugin.plugin_id,
    value: plugin.plugin_id,
  })),
]);

const pluginPermissionModuleFilterItems = computed(() => {
  const modules = new Set<string>();
  if (
    pluginPermissionPluginFilter.value === PERMISSION_FILTER_ALL ||
    pluginPermissionPluginFilter.value === PERMISSION_FILTER_CORE
  ) {
    nonMenuPermissions.value.forEach((perm) => {
      if (
        pluginPermissionTypeFilter.value !== PERMISSION_FILTER_ALL &&
        !capabilityTypeMatchesFilter(perm.type)
      ) return;
      if (
        pluginPermissionRegistrationFilter.value !== PERMISSION_FILTER_ALL &&
        corePermissionRegistrationStatus(perm) !==
          pluginPermissionRegistrationFilter.value
      ) return;
      if (perm.module) modules.add(perm.module);
    });
  }
  pluginPermissionPlugins.value.forEach((plugin) => {
    if (
      pluginPermissionPluginFilter.value !== PERMISSION_FILTER_ALL &&
      plugin.plugin_id !== pluginPermissionPluginFilter.value
    ) {
      return;
    }
    collectPluginCapabilityItems(plugin).forEach((item) => {
      if (
        pluginPermissionTypeFilter.value !== PERMISSION_FILTER_ALL &&
        !capabilityTypeMatchesFilter(item.type)
      ) return;
      if (
        pluginPermissionRegistrationFilter.value !== PERMISSION_FILTER_ALL &&
        item.registration_status !== pluginPermissionRegistrationFilter.value
      ) return;
      if (item.module) modules.add(item.module);
    });
  });
  return [
    {
      label: t("organization.permission.pluginCatalog.filters.allModules"),
      value: PERMISSION_FILTER_ALL,
    },
    ...Array.from(modules)
      .sort()
      .map((module) => ({ label: module, value: module })),
  ];
});

const pluginPermissionTypeFilterItems = computed(() => [
  {
    label: t("organization.permission.pluginCatalog.filters.allTypes"),
    value: PERMISSION_FILTER_ALL,
  },
  {
    label: t("organization.permission.pluginCatalog.groups.operation"),
    value: "operation",
  },
  {
    label: t("organization.permission.pluginCatalog.groups.api"),
    value: "api",
  },
]);

const pluginPermissionRegistrationFilterItems = computed(() => [
  {
    label: t("organization.permission.pluginCatalog.filters.allRegistration"),
    value: PERMISSION_FILTER_ALL,
  },
  {
    label: t("organization.permission.pluginCatalog.filters.registered"),
    value: "registered",
  },
  {
    label: t("organization.permission.pluginCatalog.filters.invalid"),
    value: "invalid",
  },
]);

const hasPluginPermissionCatalogFilters = computed(
  () =>
    pluginPermissionPluginFilter.value !== PERMISSION_FILTER_ALL ||
    pluginPermissionModuleFilter.value !== PERMISSION_FILTER_ALL ||
    pluginPermissionTypeFilter.value !== PERMISSION_FILTER_ALL ||
    pluginPermissionRegistrationFilter.value !== PERMISSION_FILTER_ALL,
);

const resetPluginPermissionCatalogFilters = () => {
  pluginPermissionPluginFilter.value = PERMISSION_FILTER_ALL;
  pluginPermissionModuleFilter.value = PERMISSION_FILTER_ALL;
  pluginPermissionTypeFilter.value = PERMISSION_FILTER_ALL;
  pluginPermissionRegistrationFilter.value = PERMISSION_FILTER_ALL;
};

watch(pluginPermissionPluginFilter, () => {
  const moduleStillExists = pluginPermissionModuleFilterItems.value.some(
    (item) => item.value === pluginPermissionModuleFilter.value,
  );
  if (!moduleStillExists) {
    pluginPermissionModuleFilter.value = PERMISSION_FILTER_ALL;
  }
});

const pluginPermissionCatalogFilterMatches = (item: any, pluginID: string) => {
  if (isRuntimeContractCatalogItem(item)) {
    return false;
  }
  if (
    pluginPermissionPluginFilter.value !== PERMISSION_FILTER_ALL &&
    pluginID !== pluginPermissionPluginFilter.value
  ) {
    return false;
  }
  if (
    pluginPermissionModuleFilter.value !== PERMISSION_FILTER_ALL &&
    item.module !== pluginPermissionModuleFilter.value
  ) {
    return false;
  }
  if (
    pluginPermissionTypeFilter.value !== PERMISSION_FILTER_ALL &&
    !capabilityTypeMatchesFilter(item.type)
  ) {
    return false;
  }
  if (
    pluginPermissionRegistrationFilter.value !== PERMISSION_FILTER_ALL &&
    item.registration_status !== pluginPermissionRegistrationFilter.value
  ) {
    return false;
  }
  return true;
};

const pluginPermissionItemMatchesSearch = (
  item: any,
  pluginID: string,
) =>
  permissionSearchMatches(
    pluginID,
    item.module,
    item.resource,
    item.action,
    item.type,
    item.menu_path,
    item.page_permission_codes,
    item.permission_code,
    item.effective_permission_code,
    item.business_permission_code,
    item.risk_level,
    item.data_scope,
    item.status,
    item.registration_status,
    item.registration_errors,
    localeText(item.title_i18n),
    localeText(item.description_i18n),
    item.title_i18n,
    item.description_i18n,
    item.default_role_grants,
  );

const pluginPermissionItemVisible = (item: any, pluginID: string) => {
  if (!pluginPermissionCatalogFilterMatches(item, pluginID)) return false;
  if (!hasPermissionSearch.value) return true;
  return pluginPermissionItemMatchesSearch(item, pluginID);
};

const filterPluginMenuCatalogNodes = (nodes: any[] = [], pluginID: string): any[] =>
  nodes
    .map((node) => {
      const children = filterPluginMenuCatalogNodes(node.children || [], pluginID);
      const nodeMatches =
        node.permission && pluginPermissionItemVisible(node.permission, pluginID);
      if (nodeMatches || children.length > 0) {
        return { ...node, children };
      }
      return null;
    })
    .filter(Boolean);

const filterPluginBusinessModules = (modules: any[] = [], pluginID: string) =>
  modules
    .map((module) => {
      const resources = (module.resources || [])
        .map((resource: any) => ({
          ...resource,
          pages: (resource.pages || []).filter((item: any) =>
            pluginPermissionItemVisible(item, pluginID),
          ),
          actions: (resource.actions || []).filter((item: any) =>
            pluginPermissionItemVisible(item, pluginID),
          ),
        }))
        .filter(
          (resource: any) =>
            (resource.pages || []).length > 0 ||
            (resource.actions || []).length > 0,
        );
      return { ...module, resources };
    })
    .filter((module) => module.resources.length > 0);

const filterPluginAPIBindings = (bindings: any[] = [], pluginID: string) =>
  bindings.filter((binding) =>
    pluginPermissionItemVisible(binding.permission, pluginID),
  );

const filteredPluginPermissionPlugins = computed(() => {
  return pluginPermissionPlugins.value
    .map((plugin) => {
      const pluginID = plugin.plugin_id;
      return {
        ...plugin,
        menu_tree: filterPluginMenuCatalogNodes(plugin.menu_tree || [], pluginID),
        business_modules: filterPluginBusinessModules(
          plugin.business_modules || [],
          pluginID,
        ),
        api_bindings: filterPluginAPIBindings(plugin.api_bindings || [], pluginID),
      };
    })
    .filter(
      (plugin) =>
        (plugin.business_modules || []).length > 0 ||
        (plugin.api_bindings || []).length > 0,
    );
});

const filteredPluginPermissionCount = computed(() =>
  filteredPluginPermissionPlugins.value
    .reduce(
      (total, plugin) => total + collectPluginCapabilityItems(plugin).length,
      0,
    ),
);

const buildPluginCapabilitySourceGroup = (plugin: any) => {
  const moduleMap = new Map<
    string,
    {
      key: string;
      label: string;
      operationResources: any[];
      apiBindings: any[];
      count: number;
    }
  >();
  const ensureModule = (module: string) => {
    const key = module || "default";
    if (!moduleMap.has(key)) {
      moduleMap.set(key, {
        key,
        label: key,
        operationResources: [],
        apiBindings: [],
        count: 0,
      });
    }
    return moduleMap.get(key)!;
  };

  for (const module of plugin.business_modules || []) {
    const group = ensureModule(module.module);
    for (const resource of module.resources || []) {
      const pages = resource.pages || [];
      const actions = resource.actions || [];
      if (pages.length === 0 && actions.length === 0) continue;
      group.operationResources.push({ ...resource, pages, actions });
      group.count += pages.length + actions.length;
    }
  }
  for (const binding of plugin.api_bindings || []) {
    const module = binding?.permission?.module || "api";
    const group = ensureModule(module);
    group.apiBindings.push(binding);
    group.count += 1;
  }

  const modules = Array.from(moduleMap.values())
    .filter((module) => module.count > 0)
    .sort((a, b) => a.label.localeCompare(b.label, "zh-CN"));
  if (modules.length === 0) return null;
  return {
    key: plugin.plugin_id,
    label: plugin.plugin_name || plugin.name || plugin.plugin_id,
    hint: plugin.plugin_id,
    icon: "i-heroicons-puzzle-piece",
    plugin,
    modules,
    count: modules.reduce((total, module) => total + module.count, 0),
  };
};

const pluginCapabilitySourceGroups = computed(() =>
  filteredPluginPermissionPlugins.value
    .map((plugin) => buildPluginCapabilitySourceGroup(plugin))
    .filter(Boolean),
);

const capabilitySourceGroups = computed(() => [
  ...(coreCapabilitySourceGroup.value ? [coreCapabilitySourceGroup.value] : []),
  ...pluginCapabilitySourceGroups.value,
]);

const selectedCapabilitySource = computed(() =>
  capabilitySourceGroups.value.find(
    (source: any) => source.key === selectedCapabilitySourceKey.value,
  ),
);

const selectedCapabilityModule = computed(() =>
  (selectedCapabilitySource.value?.modules || []).find(
    (module: any) => module.key === selectedCapabilityModuleKey.value,
  ),
);

watchEffect(() => {
  const firstSource = capabilitySourceGroups.value[0] as any;
  if (!firstSource) {
    selectedCapabilitySourceKey.value = "";
    selectedCapabilityModuleKey.value = "";
    return;
  }
  const sourceExists = capabilitySourceGroups.value.some(
    (source: any) => source.key === selectedCapabilitySourceKey.value,
  );
  if (!sourceExists) {
    selectedCapabilitySourceKey.value = firstSource.key;
  }
  const source = capabilitySourceGroups.value.find(
    (item: any) => item.key === selectedCapabilitySourceKey.value,
  ) as any;
  const firstModule = source?.modules?.[0];
  if (!firstModule) {
    selectedCapabilityModuleKey.value = "";
    return;
  }
  const moduleExists = (source.modules || []).some(
    (module: any) => module.key === selectedCapabilityModuleKey.value,
  );
  if (!moduleExists) {
    selectedCapabilityModuleKey.value = firstModule.key;
  }
});

const filteredMenuPermissionCount = computed(() =>
  menuPermissionSections.value.reduce(
    (total, section) => total + collectMenuSectionPermissionIds(section).length,
    0,
  ),
);

const filteredPermissionGroupCount = computed(() =>
  coreCapabilitySourceGroup.value?.count || 0,
);

const visiblePermissionResultCount = computed(() =>
  permissionTab.value === "menus"
    ? filteredMenuPermissionCount.value
    : filteredPluginPermissionCount.value + filteredPermissionGroupCount.value,
);

const localeText = (value?: Record<string, string>) => {
  if (!value) return "";
  const current = String(locale.value || "").trim();
  return (
    value[current] ||
    value["zh-CN"] ||
    value.zh ||
    value.en ||
    value["en-US"] ||
    Object.values(value).find((item) => String(item || "").trim()) ||
    ""
  );
};

const pluginPermissionTitle = (item: any) =>
  localeText(item.title_i18n) || item.permission_code;

const pluginPermissionDescription = (item: any) =>
  localeText(item.description_i18n);

const isRawPermissionLabel = (value?: string) => {
  const text = String(value || "").trim();
  return (
    !text ||
    text.includes("/api/") ||
    text.startsWith("/") ||
    /^[a-z0-9_.-]+:[a-z0-9_.-]+$/i.test(text) ||
    /^[a-z0-9_.-]+\.[a-z0-9_.-]+$/i.test(text)
  );
};

const corePermissionTitle = (perm: Permission) => {
  const title = localeText(perm.meta?.title_i18n as any);
  if (title) return title;
  const label = String(perm.meta?.label || perm.name || "").trim();
  if (!isRawPermissionLabel(label)) return label;
  if (perm.type === "api") {
    return t("organization.permission.pluginCatalog.unnamedApi");
  }
  return getPermissionDisplayName(perm);
};

const corePermissionDescription = (perm: Permission) =>
  localeText(perm.meta?.description_i18n as any) ||
  String(perm.description || "").trim();

const pluginAPITitle = (binding: any) => {
  const title = pluginPermissionTitle(binding.permission);
  if (!isRawPermissionLabel(title)) return title;
  return t("organization.permission.pluginCatalog.unnamedApi");
};

const pluginPermissionRegistrationErrorText = (error: string) => {
  const value = String(error || "").trim();
  if (!value) return "";
  if (value.startsWith("menu_path_expected:")) {
    return t("organization.permission.pluginCatalog.errors.menuPathExpected", {
      path: value.slice("menu_path_expected:".length),
    });
  }
  if (value.startsWith("menu_path_actual:")) {
    return t("organization.permission.pluginCatalog.errors.menuPathActual", {
      path: value.slice("menu_path_actual:".length),
    });
  }
  const key = `organization.permission.pluginCatalog.errors.${value}`;
  return te(key) ? t(key) : value;
};

const pluginPermissionRegistrationErrors = (item: any) =>
  (item?.registration_errors || [])
    .map((error: string) => pluginPermissionRegistrationErrorText(error))
    .filter(Boolean);

const isPluginPermissionSelectable = (item: any) =>
  item.status === "active" && item.registration_status === "registered";

const togglePluginPermission = (item: any) => {
  if (!isPluginPermissionSelectable(item)) return;
  togglePermission(Number(item.id));
};

const countInvalidPluginPermissions = (plugin: any) =>
  collectPluginCatalogItems(plugin)
    .filter((item: any) => item.registration_status !== "registered").length;

const cleanupInvalidPluginPermissions = async (pluginID: string) => {
  const invalidCount = countInvalidPluginPermissions(
    pluginPermissionPlugins.value.find((plugin) => plugin.plugin_id === pluginID),
  );
  if (invalidCount <= 0) return;
  const ok = confirm(
    t("organization.permission.pluginCatalog.cleanup.confirm", {
      plugin: pluginID,
      count: invalidCount,
    }),
  );
  if (!ok) return;
  try {
    const result =
      await permissionStore.cleanupInvalidPluginPermissions(pluginID);
    notifyOnce(
      t("organization.permission.pluginCatalog.cleanup.successTitle"),
      t("organization.permission.pluginCatalog.cleanup.successDescription", {
        count: result.deleted_permissions,
      }),
      "success" as const,
      "solid" as const,
    );
  } catch (error) {
    const { title, description } = normalizeApiError(error, {
      meta: "metaText",
    });
    notifyOnce(
      title ||
        t("organization.permission.pluginCatalog.cleanup.errorTitle"),
      description,
      "error" as const,
      "solid" as const,
    );
  }
};

// ✅ 新增：是否有改动（对比初始态）
const dirty = computed(() => {
  const roleId = selectedRole.value?.id;
  if (!roleId) return false;
  const cur = new Set(roleSelection.value[roleId] || []);
  const init = new Set(permissionStore.roleInitialSelection[roleId] || []);
  if (cur.size !== init.size) return true;
  for (const id of cur) if (!init.has(id)) return true;
  return false;
});

/** 勾选单个权限（列表页） */
const togglePermission = (permissionId: number) => {
  const roleId = selectedRole.value?.id;
  if (!roleId) return;
  const set = new Set(roleSelection.value[roleId] || []);
  if (set.has(permissionId)) set.delete(permissionId);
  else set.add(permissionId);
  roleSelection.value[roleId] = Array.from(set);
};

/** 勾选整模块权限（列表页） */
const toggleModulePermissions = (module: string, checked: boolean) => {
  const roleId = selectedRole.value?.id;
  if (!roleId) return;
  const ids = filteredNonMenuPermissions.value
    .filter((p) => p.module === module)
    .map((p) => p.id);
  const set = new Set(roleSelection.value[roleId] || []);
  if (checked) ids.forEach((id) => set.add(id));
  else ids.forEach((id) => set.delete(id));
  roleSelection.value[roleId] = Array.from(set);
};

const isModuleFullySelected = (module: string) => {
  const roleId = selectedRole.value?.id;
  if (!roleId) return false;
  const ids = filteredNonMenuPermissions.value
    .filter((p) => p.module === module)
    .map((p) => p.id);
  const cur = new Set(roleSelection.value[roleId] || []);
  return ids.length > 0 && ids.every((id) => cur.has(id));
};

const isModulePartiallySelected = (module: string) => {
  const roleId = selectedRole.value?.id;
  if (!roleId) return false;
  const ids = filteredNonMenuPermissions.value
    .filter((p) => p.module === module)
    .map((p) => p.id);
  const cur = new Set(roleSelection.value[roleId] || []);
  const picked = ids.filter((id) => cur.has(id)).length;
  return picked > 0 && picked < ids.length;
};

/** 保存按钮：一次性提交 set-ids */
const saving = ref(false);
const saveRolePermissions = async () => {
  const roleId = selectedRole.value?.id;
  if (!roleId) return;
  try {
    saving.value = true;
    const ids = roleSelection.value[roleId] || [];
    await permissionStore.setRolePermissionIDs(roleId, ids);
  } catch (e) {
    console.error(e);
    const { title, description } = normalizeApiError(e, {
      meta: "metaText",
    });
    notifyOnce(
      title || "保存权限失败",
      description,
      "error" as const,
      "solid" as const,
    );
  } finally {
    saving.value = false;
  }
};

/** 标签 & 颜色 */
const getPermissionTypeLabel = (type: string) =>
  (
    ({
      menu: t("organization.permission.pluginCatalog.types.menu"),
      page: t("organization.permission.pluginCatalog.types.page"),
      action: t("organization.permission.pluginCatalog.types.action"),
      admin_action: t("organization.permission.pluginCatalog.types.action"),
      data: t("organization.permission.pluginCatalog.types.data"),
      api: t("organization.permission.pluginCatalog.types.api"),
      api_key: t("organization.permission.pluginCatalog.types.apiKeyGrant"),
    }) as any
  )[type] || type;
const getPermissionTypeColor = (m?: string) =>
  (
    ({
      GET: "success",
      POST: "primary",
      PUT: "warning",
      DELETE: "error",
      PATCH: "warning",
    }) as any
  )[m || ""] || "neutral";
const getDataScopeLabel = (s?: string) =>
  (
    ({
      own: "仅自己",
      department: "本部门",
      company: "本公司",
      all: "全部",
    }) as any
  )[s || ""] ||
  s ||
  "";
const getPermissionTextColor = (type: string) =>
  (
    ({
      menu: "text-blue-600",
      action: "text-emerald-600",
      data: "text-rose-600",
      api: "text-violet-600",
    }) as any
  )[type] || "text-slate-600";
const getTypeOrder = (type: string) =>
  (({ menu: 1, action: 2, api: 3, data: 4 }) as any)[type] || 999;
const getSortedTypes = (types: string[]) =>
  types.sort((a, b) => getTypeOrder(a) - getTypeOrder(b));

/** ====== Nuxt UI / TanStack ====== */
const UButton = resolveComponent("UButton");

/** （如需表格列配置，可保留） */
const roleColumns = computed(() => {
  const _ = locale.value; // 响应式依赖
  return [
    { id: "name", accessorKey: "name", header: "角色名称" },
    { id: "code", accessorKey: "code", header: "角色代码" },
    { id: "description", accessorKey: "description", header: "描述" },
    {
      id: "actions",
      header: "操作",
      cell: ({ row }: any) => {
        const role: Role = row.original;
        return h(
          "div",
          { class: "flex gap-2" },
          [
            h(
              UButton,
              {
                size: "xs",
                variant: "ghost",
                icon: "i-heroicons-pencil-square",
                onClick: () => openEditRoleForm(role),
              },
              { default: () => "编辑" },
            ),
            !role.builtin &&
              h(
                UButton,
                {
                  size: "xs",
                  color: "red",
                  variant: "ghost",
                  icon: "i-heroicons-trash",
                  onClick: () => deleteRole(role.id),
                },
                { default: () => "删除" },
              ),
          ].filter(Boolean),
        );
      },
    },
  ];
});

/** ====== 表单内权限选择（弹窗） ====== */
const formModulePermissionIds = (module: string) =>
  filteredNonMenuPermissions.value
    .filter((p) => p.module === module)
    .map((p) => p.id);
const hasFormPermission = (permissionId: number) =>
  roleForm.permissions.includes(permissionId);
const toggleFormPermission = (permissionId: number) => {
  const i = roleForm.permissions.indexOf(permissionId);
  if (i === -1) roleForm.permissions.push(permissionId);
  else roleForm.permissions.splice(i, 1);
};
const toggleFormPermissionIds = (permissionIds: number[], checked: boolean) => {
  if (checked) {
    for (const id of permissionIds) {
      if (!roleForm.permissions.includes(id)) roleForm.permissions.push(id);
    }
    return;
  }
  roleForm.permissions = roleForm.permissions.filter(
    (id) => !permissionIds.includes(id),
  );
};
const toggleFormModulePermissions = (module: string, checked: boolean) => {
  const ids = formModulePermissionIds(module);
  if (checked) {
    ids.forEach((id) => {
      if (!roleForm.permissions.includes(id)) roleForm.permissions.push(id);
    });
  } else {
    roleForm.permissions = roleForm.permissions.filter(
      (id) => !ids.includes(id),
    );
  }
};
const isFormModuleFullySelected = (module: string) => {
  const ids = formModulePermissionIds(module);
  return ids.length > 0 && ids.every((id) => roleForm.permissions.includes(id));
};
const isFormModulePartiallySelected = (module: string) => {
  const ids = formModulePermissionIds(module);
  const picked = ids.filter((id) => roleForm.permissions.includes(id)).length;
  return picked > 0 && picked < ids.length;
};
</script>

<template>
  <div>
    <!-- 权限管理头部 -->
    <div class="flex justify-between items-center mb-6">
      <div>
        <h2 class="text-xl font-semibold text-gray-800">
          {{ $t("organization.permission.title") }}
        </h2>
        <p class="text-sm text-gray-500 mt-1">
          {{ $t("organization.permission.description") }}
        </p>
      </div>
      <div class="flex gap-3">
        <UButton
          :loading="saving"
          :disabled="!dirty"
          color="primary"
          icon="i-heroicons-check"
          @click="saveRolePermissions"
        >
          保存
        </UButton>
        <UButton
          color="primary"
          icon="i-heroicons-plus"
          @click="openAddRoleForm"
        >
          {{ $t("organization.permission.add") }}
        </UButton>
      </div>
    </div>

    <div class="grid grid-cols-1 gap-6 xl:grid-cols-[320px_minmax(0,1fr)]">
      <!-- 角色列表 -->
      <div class="min-w-0">
        <div class="bg-white rounded-lg shadow">
          <div class="p-4 border-b">
            <h3 class="text-lg font-medium text-gray-900">
              {{ $t("organization.permission.roleList") }}
            </h3>
            <UInput
              v-model="searchQuery"
              icon="i-heroicons-magnifying-glass"
              :placeholder="$t('organization.permission.search')"
              class="mt-2"
            />
          </div>

          <div class="divide-y divide-gray-200 max-h-[600px] overflow-y-auto">
            <div
              v-for="role in filteredRoles"
              :key="role.id"
              @click="selectRole(role)"
              :class="[
                'relative p-3 cursor-pointer transition-colors hover:bg-gray-50 dark:hover:bg-gray-800/40',
                selectedRole && selectedRole.id === role.id
                  ? 'bg-transparent'
                  : '',
              ]"
            >
              <div
                :class="[
                  'flex justify-between items-start rounded-md border p-3 transition-colors',
                  selectedRole && selectedRole.id === role.id
                    ? 'border-primary-300 bg-primary-50 text-primary-700 shadow-sm ring-1 ring-primary-100 dark:border-primary-700 dark:bg-primary-950/45 dark:text-primary-300 dark:ring-primary-900'
                    : 'border-transparent',
                ]"
              >
                <div>
                  <div class="flex items-center">
                    <h4
                      :class="[
                        'font-medium',
                        selectedRole && selectedRole.id === role.id
                          ? 'text-primary-700 dark:text-primary-300'
                          : 'text-gray-900 dark:text-gray-100',
                      ]"
                    >
                      {{ role.name }}
                    </h4>
                    <UBadge
                      v-if="role.builtin"
                      color="primary"
                      variant="subtle"
                      size="sm"
                      class="ml-2"
                    >
                      {{ $t("organization.permission.systemRole") }}
                    </UBadge>
                  </div>
                  <p class="text-sm text-gray-500 dark:text-gray-400 mt-1">{{ role.code }}</p>
                  <p class="text-sm text-gray-600 dark:text-gray-300 mt-1">
                    {{ role.description }}
                  </p>
                  <p class="text-xs text-gray-500 dark:text-gray-400 mt-2">
                    <UIcon
                      name="i-heroicons-users"
                      class="w-4 h-4 inline-block mr-1"
                    />
                    {{ role.userCount ?? 0 }}
                    {{ $t("organization.permission.userCount") }}
                  </p>
                </div>
                <div class="flex space-x-1">
                  <UButton
                    color="neutral"
                    variant="ghost"
                    icon="i-heroicons-pencil-square"
                    size="xs"
                    @click.stop="openEditRoleForm(role)"
                  />
                  <UButton
                    v-if="!role.builtin"
                    color="error"
                    variant="ghost"
                    icon="i-heroicons-trash"
                    size="xs"
                    @click.stop="deleteRole(role.id)"
                  />
                </div>
              </div>
            </div>

            <!-- 空状态 -->
            <div v-if="filteredRoles.length === 0" class="p-8 text-center">
              <UIcon
                name="i-heroicons-user-group"
                class="w-12 h-12 text-gray-400 mx-auto mb-4"
              />
              <h3 class="text-lg font-medium text-gray-900 mb-2">
                {{ $t("organization.permission.empty.title") }}
              </h3>
              <p class="text-gray-500 mb-4">
                {{
                  searchQuery
                    ? $t("organization.permission.empty.noResults")
                    : $t("organization.permission.empty.create")
                }}
              </p>
              <UButton
                v-if="!searchQuery"
                color="primary"
                @click="openAddRoleForm"
              >
                {{ $t("organization.permission.add") }}
              </UButton>
            </div>
          </div>
        </div>
      </div>

      <!-- 权限配置 -->
      <div class="min-w-0">
        <div class="bg-white rounded-lg shadow">
          <div class="p-4 border-b">
            <h3 class="text-lg font-medium text-gray-900">
              {{
                (selectedRole && selectedRole.name) ||
                $t("organization.permission.roleConfig")
              }}
            </h3>
            <p class="text-sm text-gray-500 mt-1">
              {{ $t("organization.permission.configDesc") }}
            </p>
            <UTabs
              v-model="permissionTab"
              :items="permissionTabItems"
              class="permission-tabs mt-4"
            />
            <div class="mt-4 flex flex-col gap-2 sm:flex-row sm:items-center">
              <UInput
                v-model="permissionSearchQuery"
                icon="i-heroicons-magnifying-glass"
                :placeholder="
                  $t('organization.permission.permissionSearch.placeholder')
                "
                class="w-full sm:max-w-xl"
              />
              <UButton
                v-if="permissionSearchQuery"
                color="neutral"
                variant="ghost"
                icon="i-heroicons-x-mark"
                size="sm"
                :aria-label="
                  $t('organization.permission.permissionSearch.clear')
                "
                @click="permissionSearchQuery = ''"
              />
              <div class="text-xs text-gray-500 sm:ml-auto">
                {{
                  $t("organization.permission.permissionSearch.results", {
                    count: visiblePermissionResultCount,
                  })
                }}
              </div>
            </div>
            <div
              v-if="permissionTab === 'capabilities'"
              class="mt-4 grid grid-cols-1 gap-2 md:grid-cols-2 xl:grid-cols-[minmax(280px,2fr)_minmax(220px,1.3fr)_minmax(160px,0.8fr)_minmax(220px,1.1fr)_auto]"
            >
              <USelect
                v-model="pluginPermissionPluginFilter"
                :items="pluginPermissionPluginFilterItems"
                :aria-label="
                  $t('organization.permission.pluginCatalog.filters.source')
                "
                class="w-full min-w-0"
              />
              <USelect
                v-model="pluginPermissionModuleFilter"
                :items="pluginPermissionModuleFilterItems"
                :aria-label="
                  $t('organization.permission.pluginCatalog.filters.module')
                "
                class="w-full min-w-0"
              />
              <USelect
                v-model="pluginPermissionTypeFilter"
                :items="pluginPermissionTypeFilterItems"
                :aria-label="
                  $t('organization.permission.pluginCatalog.filters.type')
                "
                class="w-full min-w-0"
              />
              <USelect
                v-model="pluginPermissionRegistrationFilter"
                :items="pluginPermissionRegistrationFilterItems"
                :aria-label="
                  $t(
                    'organization.permission.pluginCatalog.filters.registration',
                  )
                "
                class="w-full min-w-0"
              />
              <UButton
                color="neutral"
                variant="subtle"
                icon="i-heroicons-arrow-path"
                :disabled="!hasPluginPermissionCatalogFilters"
                :label="
                  $t('organization.permission.pluginCatalog.filters.reset')
                "
                class="w-full justify-center xl:w-auto"
                @click="resetPluginPermissionCatalogFilters"
              />
            </div>
          </div>

          <div
            v-if="permissionTab === 'menus'"
            class="p-4 max-h-[600px] overflow-y-auto"
          >
            <div
              v-if="menuPermissionSections.length"
              class="rounded-lg border border-gray-200 bg-gray-50/60 p-4"
            >
              <div class="flex items-center gap-2 mb-4">
                <UIcon
                  name="i-heroicons-bars-3"
                  class="w-5 h-5 text-primary-600"
                />
                <div>
                  <h4 class="font-bold text-gray-900 text-lg">菜单权限</h4>
                  <p class="text-xs text-gray-500">
                    控制该角色登录后左侧菜单中可见的入口。
                  </p>
                </div>
              </div>

              <div class="space-y-4">
                <div
                  v-for="section in menuPermissionSections"
                  :key="section.key"
                  class="rounded-md border border-gray-200 bg-white p-3"
                >
                  <div class="flex items-center mb-3">
                    <UCheckbox
                      :model-value="menuSectionCheckboxValue(section)"
                      @update:model-value="
                        toggleMenuSectionPermissions(section, $event as boolean)
                      "
                    />
                    <UIcon
                      :name="section.icon"
                      class="ml-2 h-4 w-4 text-gray-500"
                    />
                    <h5 class="ml-2 text-sm font-semibold text-gray-800">
                      {{ section.label }}
                    </h5>
                    <UBadge
                      size="xs"
                      color="neutral"
                      variant="subtle"
                      class="ml-auto"
                    >
                      {{ collectMenuSectionPermissionIds(section).length }}
                    </UBadge>
                  </div>

                  <MenuPermissionTree
                    :nodes="section.nodes"
                    :selected-ids="currentRoleMenuPermissionIds"
                    @toggle="toggleMenuPermissionIds"
                  />
                </div>
              </div>
            </div>
            <div v-else class="py-12 text-center text-sm text-gray-500">
              {{
                hasPermissionSearch
                  ? $t("organization.permission.permissionSearch.empty")
                  : "暂无菜单权限。请先执行 seed 同步系统菜单权限。"
              }}
            </div>
          </div>

          <div v-else class="p-4">
            <div
              v-if="capabilitySourceGroups.length"
              class="grid grid-cols-1 overflow-hidden rounded-lg border border-gray-200 bg-gray-50/60 lg:grid-cols-[180px_200px_minmax(0,1fr)] 2xl:grid-cols-[200px_220px_minmax(0,1fr)]"
            >
              <div class="border-b border-gray-200 bg-white lg:border-b-0 lg:border-r">
                <div class="border-b border-gray-100 px-3 py-2 text-xs font-medium text-gray-500">
                  {{ $t("organization.permission.pluginCatalog.levels.source") }}
                </div>
                <div class="max-h-[480px] overflow-y-auto p-2">
                  <button
                    v-for="source in capabilitySourceGroups"
                    :key="source.key"
                    type="button"
                    class="mb-1 flex w-full items-center gap-2 rounded-md border px-3 py-2 text-left text-sm transition-colors"
                    :class="
                      selectedCapabilitySourceKey === source.key
                        ? 'border-primary-300 bg-primary-50 text-primary-700 shadow-sm ring-1 ring-primary-100 dark:border-primary-700 dark:bg-primary-950/45 dark:text-primary-300 dark:ring-primary-900'
                        : 'border-transparent text-gray-700 hover:bg-gray-50 dark:text-gray-300 dark:hover:bg-gray-800/70'
                    "
                    @click="selectedCapabilitySourceKey = source.key"
                  >
                    <UIcon :name="source.icon" class="h-4 w-4 shrink-0" />
                    <span class="min-w-0 flex-1 truncate">{{ source.label }}</span>
                    <UBadge size="xs" color="neutral" variant="subtle">
                      {{ source.count }}
                    </UBadge>
                  </button>
                </div>
              </div>

              <div class="border-b border-gray-200 bg-white lg:border-b-0 lg:border-r">
                <div class="border-b border-gray-100 px-3 py-2 text-xs font-medium text-gray-500">
                  {{ $t("organization.permission.pluginCatalog.levels.module") }}
                </div>
                <div class="max-h-[480px] overflow-y-auto p-2">
                  <button
                    v-for="module in selectedCapabilitySource?.modules || []"
                    :key="`${selectedCapabilitySourceKey}:${module.key}`"
                    type="button"
                    class="mb-1 flex w-full items-center gap-2 rounded-md border px-3 py-2 text-left text-sm transition-colors"
                    :class="
                      selectedCapabilityModuleKey === module.key
                        ? 'border-primary-300 bg-primary-50 text-primary-700 shadow-sm ring-1 ring-primary-100 dark:border-primary-700 dark:bg-primary-950/45 dark:text-primary-300 dark:ring-primary-900'
                        : 'border-transparent text-gray-700 hover:bg-gray-50 dark:text-gray-300 dark:hover:bg-gray-800/70'
                    "
                    @click="selectedCapabilityModuleKey = module.key"
                  >
                    <span class="min-w-0 flex-1 truncate">{{ module.label }}</span>
                    <UBadge size="xs" color="neutral" variant="subtle">
                      {{ module.count }}
                    </UBadge>
                  </button>
                </div>
              </div>

              <div class="min-w-0 bg-white">
                <div class="flex items-center gap-3 border-b border-gray-100 px-4 py-3">
                  <div class="min-w-0 flex-1">
                    <div class="truncate text-sm font-semibold text-gray-900">
                      {{ selectedCapabilitySource?.label }}
                      <span v-if="selectedCapabilityModule">
                        / {{ selectedCapabilityModule.label }}
                      </span>
                    </div>
                    <div
                      v-if="selectedCapabilitySource?.hint"
                      class="truncate text-xs text-gray-500"
                    >
                      {{ selectedCapabilitySource.hint }}
                    </div>
                  </div>
                  <UButton
                    v-if="
                      selectedCapabilitySource?.plugin &&
                      countInvalidPluginPermissions(selectedCapabilitySource.plugin) > 0
                    "
                    color="error"
                    variant="subtle"
                    size="xs"
                    icon="i-heroicons-trash"
                    :label="
                      $t(
                        'organization.permission.pluginCatalog.cleanup.action',
                      )
                    "
                    @click="
                      cleanupInvalidPluginPermissions(
                        selectedCapabilitySource.plugin.plugin_id,
                      )
                    "
                  />
                </div>

                <div class="max-h-[520px] overflow-y-auto p-4">
                  <template v-if="selectedCapabilitySource && selectedCapabilityModule">
                    <template v-if="selectedCapabilitySource.key === PERMISSION_FILTER_CORE">
                      <div
                        v-for="typeGroup in selectedCapabilityModule.typeGroups"
                        :key="`${selectedCapabilitySource.key}:${selectedCapabilityModule.key}:${typeGroup.key}`"
                        class="mb-5 last:mb-0"
                      >
                        <div class="mb-2 border-b border-gray-100 pb-1 text-xs font-medium text-gray-500">
                          {{ typeGroup.label }}
                        </div>
                        <div class="grid grid-cols-1 gap-2 xl:grid-cols-2">
                          <template
                            v-for="type in getSortedTypes(Object.keys(typeGroup.types))"
                            :key="`${selectedCapabilityModule.key}:${type}`"
                          >
                            <div
                              v-for="perm in typeGroup.types[type]"
                              :key="perm.id"
                              class="flex min-w-0 items-start rounded-md border border-gray-200 bg-gray-50/60 p-3"
                            >
                              <UCheckbox
                                :model-value="hasPermission(perm.id)"
                                @update:model-value="togglePermission(perm.id)"
                              />
                              <div class="ml-2 min-w-0 flex-1">
                                <div class="flex min-w-0 flex-wrap items-center gap-2">
                                  <span class="min-w-0 truncate text-sm font-medium text-gray-900">
                                    {{ corePermissionTitle(perm) }}
                                  </span>
                                  <UBadge size="xs" color="neutral" variant="subtle">
                                    {{ getPermissionTypeLabel(perm.type) }}
                                  </UBadge>
                                </div>
                                <p
                                  v-if="corePermissionDescription(perm)"
                                  class="mt-1 line-clamp-2 text-xs text-gray-500"
                                >
                                  {{ corePermissionDescription(perm) }}
                                </p>
                                <div
                                  v-if="perm.type === 'api'"
                                  class="mt-2 min-w-0 text-xs text-gray-500"
                                >
                                  <UBadge
                                    size="xs"
                                    :color="getPermissionTypeColor(perm.httpMethod)"
                                  >
                                    {{ perm.httpMethod }}
                                  </UBadge>
                                  <code class="ml-1 break-all text-xs text-gray-500">
                                    {{ perm.apiEndpoint }}
                                  </code>
                                </div>
                                <code class="mt-2 block break-all text-xs text-gray-400">
                                  {{ perm.code }}
                                </code>
                              </div>
                            </div>
                          </template>
                        </div>
                      </div>
                    </template>

                    <template v-else>
                      <div
                        v-if="(selectedCapabilityModule.operationResources || []).length"
                        class="mb-5"
                      >
                        <div class="mb-2 border-b border-gray-100 pb-1 text-xs font-medium text-gray-500">
                          {{
                            $t(
                              'organization.permission.pluginCatalog.groups.operation',
                            )
                          }}
                        </div>
                        <div
                          v-for="resource in selectedCapabilityModule.operationResources"
                          :key="`${selectedCapabilitySource.key}:${selectedCapabilityModule.key}:${resource.resource}`"
                          class="mb-4 last:mb-0"
                        >
                          <div class="mb-2 text-xs font-medium text-gray-500">
                            {{ resource.resource }}
                          </div>
                          <div class="grid grid-cols-1 gap-2 xl:grid-cols-2">
                            <div
                              v-for="perm in [
                                ...(resource.pages || []),
                                ...(resource.actions || []),
                              ]"
                              :key="perm.id"
                              class="flex min-w-0 items-start rounded-md border border-gray-200 bg-gray-50/60 p-3"
                              :class="
                                isPluginPermissionSelectable(perm)
                                  ? ''
                                  : 'opacity-60'
                              "
                            >
                              <UCheckbox
                                :model-value="hasPermission(Number(perm.id))"
                                :disabled="!isPluginPermissionSelectable(perm)"
                                @update:model-value="togglePluginPermission(perm)"
                              />
                              <div class="ml-2 min-w-0 flex-1">
                                <div class="flex min-w-0 flex-wrap items-center gap-2">
                                  <span class="min-w-0 truncate text-sm font-medium text-gray-900">
                                    {{ pluginPermissionTitle(perm) }}
                                  </span>
                                  <UBadge
                                    v-if="perm.registration_status && perm.registration_status !== 'registered'"
                                    size="xs"
                                    color="error"
                                    variant="subtle"
                                  >
                                    {{
                                      $t(
                                        'organization.permission.pluginCatalog.invalid',
                                      )
                                    }}
                                  </UBadge>
                                </div>
                                <p
                                  v-if="pluginPermissionDescription(perm)"
                                  class="mt-1 line-clamp-2 text-xs text-gray-500"
                                >
                                  {{ pluginPermissionDescription(perm) }}
                                </p>
                                <code class="mt-2 block break-all text-xs text-gray-400">
                                  {{ perm.permission_code }}
                                </code>
                              </div>
                            </div>
                          </div>
                        </div>
                      </div>

                      <div
                        v-if="(selectedCapabilityModule.apiBindings || []).length"
                      >
                        <div class="mb-2 border-b border-gray-100 pb-1 text-xs font-medium text-gray-500">
                          {{
                            $t(
                              'organization.permission.pluginCatalog.groups.api',
                            )
                          }}
                        </div>
                        <div class="grid grid-cols-1 gap-2 xl:grid-cols-2">
                          <div
                            v-for="binding in selectedCapabilityModule.apiBindings || []"
                            :key="binding.permission.id"
                            class="flex min-w-0 items-start rounded-md border border-gray-200 bg-gray-50/60 p-3"
                            :class="
                              isPluginPermissionSelectable(binding.permission)
                                ? ''
                                : 'opacity-60'
                            "
                          >
                            <UCheckbox
                              v-if="binding.independent"
                              :model-value="
                                hasPermission(Number(binding.permission.id))
                              "
                              :disabled="
                                !isPluginPermissionSelectable(binding.permission)
                              "
                              @update:model-value="
                                togglePluginPermission(binding.permission)
                              "
                            />
                            <div
                              :class="binding.independent ? 'ml-2' : ''"
                              class="min-w-0 flex-1"
                            >
                              <div class="flex min-w-0 flex-wrap items-center gap-2">
                                <span class="min-w-0 truncate text-sm font-medium text-gray-900">
                                  {{ pluginAPITitle(binding) }}
                                </span>
                                <UBadge
                                  v-if="binding.permission.registration_status && binding.permission.registration_status !== 'registered'"
                                  size="xs"
                                  color="error"
                                  variant="subtle"
                                >
                                  {{
                                    $t(
                                      'organization.permission.pluginCatalog.invalid',
                                    )
                                  }}
                                </UBadge>
                              </div>
                              <p
                                v-if="pluginPermissionDescription(binding.permission)"
                                class="mt-1 line-clamp-2 text-xs text-gray-500"
                              >
                                {{
                                  pluginPermissionDescription(binding.permission)
                                }}
                              </p>
                              <code class="mt-2 block break-all text-xs text-gray-400">
                                {{ binding.permission.permission_code }}
                              </code>
                              <p
                                v-if="binding.business_permission_code"
                                class="mt-1 break-all text-xs text-gray-500"
                              >
                                {{
                                  $t(
                                    'organization.permission.pluginCatalog.businessPermission',
                                  )
                                }}:
                                <span class="font-mono">
                                  {{ binding.business_permission_code }}
                                </span>
                              </p>
                            </div>
                          </div>
                        </div>
                      </div>
                    </template>
                  </template>
                </div>
              </div>
            </div>
            <div
              v-if="capabilitySourceGroups.length === 0"
              class="py-12 text-center text-sm text-gray-500"
            >
              {{
                hasPermissionSearch
                  ? $t("organization.permission.permissionSearch.empty")
                  : "暂无能力/API权限。"
              }}
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 角色表单对话框 -->
    <UModal
      v-model:open="showRoleForm"
      :ui="{ content: 'w-full max-w-5xl' }"
      :title="
        isEditing
          ? $t('organization.permission.edit')
          : $t('organization.permission.add')
      "
      :description="$t('organization.permission.configDesc')"
    >
      <template #content>
        <div class="py-12 px-24">
          <h3 class="text-lg font-medium text-gray-900 mb-4">
            {{
              isEditing
                ? $t("organization.permission.edit")
                : $t("organization.permission.add")
            }}
          </h3>

          <form @submit.prevent="saveRole">
            <div class="space-y-4">
              <UFormField
                :label="$t('organization.permission.form.name')"
                required
              >
                <UInput
                  v-model="roleForm.name"
                  :placeholder="
                    $t('organization.permission.form.namePlaceholder')
                  "
                />
              </UFormField>

              <UFormField
                v-if="!isEditing"
                :label="$t('organization.permission.form.code')"
                required
              >
                <UInput
                  v-model="roleForm.code"
                  :placeholder="
                    $t('organization.permission.form.codePlaceholder')
                  "
                />
              </UFormField>

              <UFormField
                :label="$t('organization.permission.form.description')"
              >
                <UTextarea
                  v-model="roleForm.description"
                  :placeholder="
                    $t('organization.permission.form.descriptionPlaceholder')
                  "
                />
              </UFormField>

              <UFormField v-if="!isEditing" label="租户" required>
                <SelectTree
                  v-model="selectedTenant"
                  :items="tenantTreeItems"
                  placeholder="选择租户"
                  searchable
                  clearable
                  class="w-full"
                />
              </UFormField>

              <UFormField
                :label="$t('organization.permission.form.permissions')"
              >
                <div
                  class="border rounded-md p-4 max-h-[300px] overflow-y-auto"
                >
                  <div class="mb-4 flex flex-col gap-2 sm:flex-row sm:items-center">
                    <UInput
                      v-model="permissionSearchQuery"
                      icon="i-heroicons-magnifying-glass"
                      :placeholder="
                        $t(
                          'organization.permission.permissionSearch.placeholder',
                        )
                      "
                      class="w-full"
                    />
                    <UButton
                      v-if="permissionSearchQuery"
                      color="neutral"
                      variant="ghost"
                      icon="i-heroicons-x-mark"
                      size="sm"
                      :aria-label="
                        $t('organization.permission.permissionSearch.clear')
                      "
                      @click="permissionSearchQuery = ''"
                    />
                  </div>
                  <UTabs
                    v-model="roleFormPermissionTab"
                    :items="permissionTabItems"
                    class="mb-4"
                  />

                  <div v-if="roleFormPermissionTab === 'menus'">
                    <div
                      v-if="menuPermissionSections.length"
                      class="rounded-md border border-gray-200 bg-gray-50/60 p-3"
                    >
                      <div class="flex items-center gap-2 mb-3">
                        <UIcon
                          name="i-heroicons-bars-3"
                          class="w-4 h-4 text-primary-600"
                        />
                        <h4 class="font-semibold text-gray-900">菜单权限</h4>
                      </div>
                      <div class="space-y-3">
                        <div
                          v-for="section in menuPermissionSections"
                          :key="`form-${section.key}`"
                          class="rounded-md border border-gray-200 bg-white p-3"
                        >
                          <div class="mb-2 flex items-center gap-2">
                            <UCheckbox
                              :model-value="
                                formMenuSectionCheckboxValue(section)
                              "
                              @update:model-value="
                                toggleFormPermissionIds(
                                  collectMenuSectionPermissionIds(section),
                                  $event as boolean,
                                )
                              "
                            />
                            <UIcon
                              :name="section.icon"
                              class="h-4 w-4 text-gray-500"
                            />
                            <div class="text-xs font-medium text-gray-600">
                              {{ section.label }}
                            </div>
                          </div>
                          <MenuPermissionTree
                            :nodes="section.nodes"
                            :selected-ids="roleForm.permissions"
                            @toggle="toggleFormPermissionIds"
                          />
                        </div>
                      </div>
                    </div>
                    <div v-else class="py-8 text-center text-sm text-gray-500">
                      {{
                        hasPermissionSearch
                          ? $t("organization.permission.permissionSearch.empty")
                          : "暂无菜单权限。请先执行 seed 同步系统菜单权限。"
                      }}
                    </div>
                  </div>

                  <div v-else>
                    <div v-if="capabilitySourceGroups.length" class="space-y-3">
                      <div
                        v-for="source in capabilitySourceGroups"
                        :key="`form-source-${source.key}`"
                        class="rounded-md border border-gray-200 bg-gray-50/60 p-3"
                      >
                        <div class="mb-3 flex items-center gap-2">
                          <UIcon
                            :name="source.icon"
                            class="h-4 w-4 text-primary-600"
                          />
                          <div class="min-w-0">
                            <div class="truncate text-sm font-semibold text-gray-900">
                              {{ source.label }}
                            </div>
                            <div
                              v-if="source.hint"
                              class="truncate text-xs text-gray-500"
                            >
                              {{ source.hint }}
                            </div>
                          </div>
                        </div>

                        <div class="space-y-3">
                          <div
                            v-for="module in source.modules"
                            :key="`form-source-${source.key}-${module.key}`"
                            class="space-y-2"
                          >
                            <div class="text-xs font-medium text-gray-600">
                              {{ module.label }}
                            </div>

                            <template v-if="source.key === PERMISSION_FILTER_CORE">
                              <div
                                v-for="typeGroup in module.typeGroups"
                                :key="`form-core-${module.key}-${typeGroup.key}`"
                                class="ml-3 space-y-2"
                              >
                                <div class="text-xs text-gray-500">
                                  {{ typeGroup.label }}
                                </div>
                                <div
                                  v-for="type in getSortedTypes(
                                    Object.keys(typeGroup.types),
                                  )"
                                  :key="`form-core-${module.key}-${type}`"
                                  class="space-y-2"
                                >
                                  <div
                                    v-for="perm in typeGroup.types[type]"
                                    :key="`form-core-perm-${perm.id}`"
                                    class="flex items-start"
                                  >
                                    <UCheckbox
                                      :model-value="hasFormPermission(perm.id)"
                                      @update:model-value="
                                        toggleFormPermission(perm.id)
                                      "
                                    />
                                    <div class="ml-2 min-w-0 flex-1">
                                      <div
                                        class="flex flex-wrap items-center gap-2"
                                      >
                                        <span
                                          class="text-sm font-medium"
                                          :class="
                                            getPermissionTextColor(perm.type)
                                          "
                                        >
                                          {{ getPermissionDisplayName(perm) }}
                                        </span>
                                        <UBadge
                                          v-if="perm.type === 'api'"
                                          size="xs"
                                          :color="
                                            getPermissionTypeColor(
                                              perm.httpMethod,
                                            )
                                          "
                                        >
                                          {{ perm.httpMethod }}
                                        </UBadge>
                                      </div>
                                      <div class="break-all text-xs text-gray-500">
                                        {{ perm.description }}
                                        <template
                                          v-if="
                                            perm.type === 'api' &&
                                            perm.apiEndpoint
                                          "
                                        >
                                          <code class="ml-1">
                                            {{ perm.apiEndpoint }}
                                          </code>
                                        </template>
                                      </div>
                                    </div>
                                  </div>
                                </div>
                              </div>
                            </template>

                            <template v-else>
                              <div
                                v-for="resource in module.operationResources || []"
                                :key="`form-plugin-resource-${source.key}-${module.key}-${resource.resource}`"
                                class="ml-3 space-y-2"
                              >
                                <div class="text-xs text-gray-500">
                                  {{ resource.resource }}
                                </div>
                                <div
                                  v-for="perm in [
                                    ...(resource.pages || []),
                                    ...(resource.actions || []),
                                  ]"
                                  :key="`form-plugin-perm-${perm.id}`"
                                  class="flex items-start"
                                >
                                  <UCheckbox
                                    :model-value="
                                      hasFormPermission(Number(perm.id))
                                    "
                                    :disabled="
                                      !isPluginPermissionSelectable(perm)
                                    "
                                    @update:model-value="
                                      toggleFormPermission(Number(perm.id))
                                    "
                                  />
                                  <div class="ml-2 min-w-0 flex-1">
                                    <div
                                      class="text-sm font-medium text-gray-900"
                                    >
                                      {{ pluginPermissionTitle(perm) }}
                                    </div>
                                    <div
                                      class="break-all font-mono text-xs text-gray-500"
                                    >
                                      {{ perm.permission_code }}
                                    </div>
                                  </div>
                                </div>
                              </div>

                              <div
                                v-if="(module.apiBindings || []).length"
                                class="ml-3 space-y-2"
                              >
                                <div class="text-xs text-gray-500">
                                  {{
                                    $t(
                                      'organization.permission.pluginCatalog.groups.api',
                                    )
                                  }}
                                </div>
                                <div
                                  v-for="binding in module.apiBindings || []"
                                  :key="`form-plugin-api-${binding.permission.id}`"
                                  class="flex items-start"
                                >
                                  <UCheckbox
                                    v-if="binding.independent"
                                    :model-value="
                                      hasFormPermission(
                                        Number(binding.permission.id),
                                      )
                                    "
                                    :disabled="
                                      !isPluginPermissionSelectable(
                                        binding.permission,
                                      )
                                    "
                                    @update:model-value="
                                      toggleFormPermission(
                                        Number(binding.permission.id),
                                      )
                                    "
                                  />
                                  <div
                                    :class="binding.independent ? 'ml-2' : ''"
                                    class="min-w-0 flex-1"
                                  >
                                    <div
                                      class="text-sm font-medium text-gray-900"
                                    >
                                      {{
                                        pluginPermissionTitle(
                                          binding.permission,
                                        )
                                      }}
                                    </div>
                                    <div
                                      class="break-all font-mono text-xs text-gray-500"
                                    >
                                      {{
                                        binding.business_permission_code ||
                                        binding.permission.permission_code
                                      }}
                                    </div>
                                  </div>
                                </div>
                              </div>
                            </template>
                          </div>
                        </div>
                      </div>
                    </div>
                    <div
                      v-else
                      class="py-8 text-center text-sm text-gray-500"
                    >
                      {{
                        hasPermissionSearch
                          ? $t("organization.permission.permissionSearch.empty")
                          : "暂无能力/API权限。"
                      }}
                    </div>
                  </div>
                </div>
              </UFormField>
            </div>

            <div class="mt-6 flex justify-end gap-3">
              <UButton
                color="neutral"
                variant="outline"
                @click="showRoleForm = false"
              >
                {{ $t("organization.common.cancel") }}
              </UButton>
              <UButton type="submit" color="primary">
                {{ $t("organization.common.save") }}
              </UButton>
            </div>
          </form>
        </div>
      </template>
    </UModal>
  </div>
</template>

<style scoped>
.permission-tabs :deep([role="tablist"]) {
  background: rgba(15, 23, 42, 0.04);
  border: 1px solid rgba(148, 163, 184, 0.22);
  border-radius: 8px;
  gap: 0.25rem;
  padding: 0.25rem;
}

.permission-tabs :deep([role="tab"]) {
  border-radius: 6px;
  color: rgb(71, 85, 105);
  background: transparent;
}

.permission-tabs :deep([role="tab"][aria-selected="true"]),
.permission-tabs :deep([role="tab"][data-state="active"]) {
  color: rgb(30, 64, 175);
  background: rgb(255, 255, 255);
  box-shadow: 0 1px 2px rgba(15, 23, 42, 0.08);
}

.dark .permission-tabs :deep([role="tablist"]) {
  background: rgba(15, 23, 42, 0.7);
  border-color: rgba(148, 163, 184, 0.18);
}

.dark .permission-tabs :deep([role="tab"]) {
  color: rgb(203, 213, 225);
}

.dark .permission-tabs :deep([role="tab"][aria-selected="true"]),
.dark .permission-tabs :deep([role="tab"][data-state="active"]) {
  color: rgb(34, 197, 94);
  background: rgba(15, 23, 42, 0.95);
}
</style>
