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
import { useOneShotAlert } from "~/composables/useOneShotAlert";
import { useTenantService } from "~/composables/api/services/tenantService";
import { normalizeApiError } from "~/composables/api/normalizeApiError";

const { t, locale } = useI18n();
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
  type: "menu" | "action" | "data" | "api";
  apiEndpoint?: string;
  httpMethod?: "GET" | "POST" | "PUT" | "DELETE" | "PATCH";
  dataScope?: "own" | "department" | "company" | "all";
};

/** ====== Pinia Store ====== */
const roleStore = useRoleStore();
const { roles } = storeToRefs(roleStore);
roleStore.ensureInitialized?.();

const permissionStore = usePermissionStore();
const { normalizedList, roleSelection } = storeToRefs(permissionStore);

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
    value: String(t.id),
    icon: "i-heroicons-building-office",
    disabled: t.status === 0, // 假设 status 为 0 表示禁用
  }));
});

// 监听租户选择变化，同步到表单
watch(selectedTenant, (tenantId) => {
  roleForm.tenant_id = tenantId ? parseInt(tenantId) : undefined;
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
const showRoleForm = ref(false);
const isEditing = ref(false);
const editingId = ref<number | null>(null);

const roleForm = reactive({
  name: "",
  code: "",
  description: "",
  scope: "tenant" as "system" | "tenant",
  tenant_id: undefined as number | undefined,
  permissions: [] as number[],
});

/** ====== 首屏加载权限 & 当前角色权限 ====== */
onMounted(async () => {
  await permissionStore.fetchAllActive(); // ← 改成全量拉取
  if (selectedRole.value?.id) {
    await permissionStore.fetchRolePermissionIDs(selectedRole.value.id);
  }
});

/** 分组：module -> type -> Permission[] */
const permissionGroups = computed(() => {
  const groups: Record<string, Record<string, Permission[]>> = {};
  for (const p of permissions.value) {
    const m = p.module;
    const t = p.type;
    if (!groups[m]) groups[m] = {};
    if (!groups[m][t]) groups[m][t] = [];
    groups[m][t].push(p);
  }
  return groups;
});

/** ====== 工具函数 ====== */
const resetRoleForm = () => {
  roleForm.name = "";
  roleForm.code = "";
  roleForm.description = "";
  roleForm.scope = "tenant";
  roleForm.tenant_id = undefined;
  roleForm.permissions = [];
  selectedTenant.value = null;
  isEditing.value = false;
  editingId.value = null;
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
        roleForm.permissions
      );
    } else {
      // 创建角色（直接带权限）
      const result = await roleStore.createRole({
        name: roleForm.name,
        code: roleForm.code,
        description: roleForm.description,
        scope: roleForm.scope,
        tenant_id: roleForm.tenant_id, // 添加租户ID
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
      "solid" as const
    );
  }
};

const deleteRole = async (id: number) => {
  const role = roles.value.find((r: any) => r.id === id) as Role | undefined;
  if (role && role.builtin) {
    notifyOnce(
      "系统角色不能删除",
      "内置角色受系统保护，无法删除",
      "warning" as const
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
        "solid" as const
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
      (r.description || "").toLowerCase().includes(q)
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
  const ids = permissions.value
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
  const ids = permissions.value
    .filter((p) => p.module === module)
    .map((p) => p.id);
  const cur = new Set(roleSelection.value[roleId] || []);
  return ids.length > 0 && ids.every((id) => cur.has(id));
};

const isModulePartiallySelected = (module: string) => {
  const roleId = selectedRole.value?.id;
  if (!roleId) return false;
  const ids = permissions.value
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
      "solid" as const
    );
  } finally {
    saving.value = false;
  }
};

/** 标签 & 颜色 */
const getPermissionTypeLabel = (type: string) =>
  (({ menu: "菜单", action: "操作", data: "数据", api: "API" }) as any)[type] ||
  type;
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
              { default: () => "编辑" }
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
                { default: () => "删除" }
              ),
          ].filter(Boolean)
        );
      },
    },
  ];
});

/** ====== 表单内权限选择（弹窗） ====== */
const formModulePermissionIds = (module: string) =>
  permissions.value.filter((p) => p.module === module).map((p) => p.id);
const hasFormPermission = (permissionId: number) =>
  roleForm.permissions.includes(permissionId);
const toggleFormPermission = (permissionId: number) => {
  const i = roleForm.permissions.indexOf(permissionId);
  if (i === -1) roleForm.permissions.push(permissionId);
  else roleForm.permissions.splice(i, 1);
};
const toggleFormModulePermissions = (module: string, checked: boolean) => {
  const ids = formModulePermissionIds(module);
  if (checked) {
    ids.forEach((id) => {
      if (!roleForm.permissions.includes(id)) roleForm.permissions.push(id);
    });
  } else {
    roleForm.permissions = roleForm.permissions.filter(
      (id) => !ids.includes(id)
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

    <div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
      <!-- 角色列表 -->
      <div class="lg:col-span-1">
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
                'p-4 cursor-pointer hover:bg-gray-50',
                selectedRole && selectedRole.id === role.id
                  ? 'bg-primary-50'
                  : '',
              ]"
            >
              <div class="flex justify-between items-start">
                <div>
                  <div class="flex items-center">
                    <h4 class="font-medium text-gray-900">{{ role.name }}</h4>
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
                  <p class="text-sm text-gray-500 mt-1">{{ role.code }}</p>
                  <p class="text-sm text-gray-600 mt-1">
                    {{ role.description }}
                  </p>
                  <p class="text-xs text-gray-500 mt-2">
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
      <div class="lg:col-span-2">
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
          </div>

          <div class="p-4 max-h-[600px] overflow-y-auto">
            <div
              v-for="(typeGroups, module) in permissionGroups"
              :key="module"
              class="mb-6 border-t border-gray-200 pt-4 first:border-t-0 first:pt-0"
            >
              <div class="flex items-center mb-3">
                <UCheckbox
                  :model-value="isModuleFullySelected(module)"
                  :indeterminate="isModulePartiallySelected(module)"
                  @update:model-value="
                    toggleModulePermissions(module, $event as boolean)
                  "
                />
                <h4 class="ml-2 font-bold text-gray-900 text-lg">
                  {{ module }}
                </h4>
              </div>

              <div class="ml-6 space-y-4">
                <div
                  v-for="type in getSortedTypes(Object.keys(typeGroups))"
                  :key="type"
                  class="space-y-2"
                >
                  <h5
                    class="text-sm font-medium text-gray-600 border-b border-gray-100 pb-1"
                  >
                    {{ getPermissionTypeLabel(type) }}权限
                  </h5>

                  <div class="grid grid-cols-1 md:grid-cols-2 gap-2 ml-4">
                    <div
                      v-for="perm in typeGroups[type]"
                      :key="perm.id"
                      class="flex items-start"
                    >
                      <UCheckbox
                        :model-value="hasPermission(perm.id)"
                        @update:model-value="togglePermission(perm.id)"
                      />
                      <div class="ml-2 flex-1">
                        <div class="flex items-center gap-2">
                          <span
                            class="text-sm font-medium"
                            :class="getPermissionTextColor(perm.type)"
                          >
                            {{ getPermissionDisplayName(perm) }}
                          </span>
                        </div>
                        <UTooltip :text="perm.name">
                          <div class="text-xs text-gray-500">
                            {{ perm.description }}
                          </div>
                        </UTooltip>
                        <div
                          v-if="perm.type === 'api'"
                          class="text-xs text-blue-600 mt-1"
                        >
                          <UBadge
                            size="xs"
                            :color="getPermissionTypeColor(perm.httpMethod)"
                          >
                            {{ perm.httpMethod }}
                          </UBadge>
                          <code class="text-xs ml-1">{{
                            perm.apiEndpoint
                          }}</code>
                        </div>
                        <div
                          v-if="perm.type === 'data'"
                          class="text-xs text-green-600 mt-1"
                        >
                          数据范围: {{ getDataScopeLabel(perm.dataScope) }}
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
                <!-- /type -->
              </div>
            </div>
            <!-- /module -->
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
                  <div
                    v-for="(typeGroups, module) in permissionGroups"
                    :key="module"
                    class="mb-4 border-t border-gray-200 pt-4 first:border-t-0 first:pt-0"
                  >
                    <div class="flex items-center mb-2">
                      <UCheckbox
                        :model-value="isFormModuleFullySelected(module)"
                        :indeterminate="isFormModulePartiallySelected(module)"
                        @update:model-value="
                          toggleFormModulePermissions(module, $event as boolean)
                        "
                      />
                      <h4 class="ml-2 font-semibold text-gray-900">
                        {{ module }}
                      </h4>
                    </div>

                    <div class="ml-6 space-y-3">
                      <div
                        v-for="type in Object.keys(typeGroups).sort(
                          (a, b) => getTypeOrder(a) - getTypeOrder(b)
                        )"
                        :key="type"
                        class="space-y-2"
                      >
                        <h5 class="text-xs font-medium text-gray-600">
                          {{ getPermissionTypeLabel(type) }}
                        </h5>

                        <div class="grid grid-cols-1 md:grid-cols-2 gap-2">
                          <div
                            v-for="perm in typeGroups[type]"
                            :key="perm.id"
                            class="flex items-start"
                          >
                            <UCheckbox
                              :model-value="hasFormPermission(perm.id)"
                              @update:model-value="
                                toggleFormPermission(perm.id)
                              "
                            />
                            <div class="ml-2 flex-1">
                              <div class="flex items-center gap-2">
                                <span
                                  class="text-sm font-medium"
                                  :class="getPermissionTextColor(perm.type)"
                                >
                                  {{ perm.name }}
                                </span>
                                <UBadge
                                  v-if="perm.type === 'api'"
                                  size="xs"
                                  :color="
                                    getPermissionTypeColor(perm.httpMethod)
                                  "
                                >
                                  {{ perm.httpMethod }}
                                </UBadge>
                              </div>
                              <div class="text-xs text-gray-500">
                                {{ perm.description }}
                                <template
                                  v-if="perm.type === 'api' && perm.apiEndpoint"
                                >
                                  · <code>{{ perm.apiEndpoint }}</code>
                                </template>
                                <template
                                  v-if="perm.type === 'data' && perm.dataScope"
                                >
                                  · {{ getDataScopeLabel(perm.dataScope) }}
                                </template>
                              </div>
                            </div>
                          </div>
                        </div>
                      </div>
                      <!-- /type -->
                    </div>
                  </div>
                  <!-- /module -->
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
