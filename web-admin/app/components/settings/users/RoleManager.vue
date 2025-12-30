<script setup lang="ts">
import { ref, reactive, computed, h, resolveComponent, onMounted } from "vue";
import { watchDebounced } from "@vueuse/core";
import { useI18n } from "#imports";
import { useRoleStore } from "~/stores/role";
import { useTenantService } from "~/composables/api/services/tenantService";
import type { Tenant } from "~/composables/api/services/tenantService";
import { useOneShotAlert } from "~/composables/useOneShotAlert";
import type {
  Role,
  RoleCreateParams,
  RoleUpdateParams,
} from "~/composables/api/services/roleService";

const { t, locale } = useI18n();
const roleStore = useRoleStore();
const tenantService = useTenantService();
const { notifyOnce, visible, title, description, color, variant, hide } =
  useOneShotAlert();

/* ================= 租户相关 ================= */
type Option = { label: string; value: string; description?: string };

const tenants = ref<Tenant[]>([]);
const loadingTenants = ref(false);
const isRootUser = ref(true);

// ✅ 对象当值（不再是 number）
const selectedTenant = ref<Option | null>(null);

// 搜索关键词（受控）
const tenantKeyword = ref("");

// ✅ 从后端映射 + 并入当前已选项，避免异步时丢失回显
const tenantOptions = computed<Option[]>(() => {
  const list = tenants.value.map((t) => ({
    label: t.name,
    value: t.uuid || String(t.id),
    description: t.domain || `租户: ${t.name}`,
  }));
  if (
    selectedTenant.value &&
    !list.some((o) => o.value === selectedTenant.value!.value)
  ) {
    list.unshift(selectedTenant.value);
  }
  return list;
});

/* —— 缓冲同步选中值到表单（对象 → id） —— */
watchDebounced(
  selectedTenant,
  (opt) => {
    roleForm.tenant_uuid = opt?.value ?? undefined;
  },
  { debounce: 300 }
);

/* —— 远程搜索（防抖） —— */
watchDebounced(
  tenantKeyword,
  (q) => {
    const s = (q ?? "").trim();
    if (!s) {
      // 推荐：拉默认一页（若后端允许）
      loadTenants();
      return;
    }
    loadTenants(s);
  },
  { debounce: 400 }
);

/* —— 远程搜索（防抖） —— */
watchDebounced(
  tenantKeyword,
  (q) => {
    const s = (q ?? "").trim();
    if (!s) {
      // ✅ 推荐：拉一页默认（如果后端允许）
      loadTenants();
      return;
    }
    loadTenants(s);
  },
  { debounce: 400 }
);

/* ================= 角色相关/状态 ================= */
const roles = computed(() => roleStore.roles);
const loading = computed(() => roleStore.loading);
const error = computed(() => roleStore.error);
const storePagination = computed(() => roleStore.pagination);

const searchQuery = ref("");
const selectedScope = ref<string>("");
const selectedBuiltin = ref<boolean | undefined>(undefined);

/* ================= 分页 ================= */
const pagination = reactive({
  page: 1,
  pageSize: 20,
  total: 0,
  totalPages: 0,
});

const pageSizeOptions = [
  { label: "10", value: 10 },
  { label: "20", value: 20 },
  { label: "50", value: 50 },
  { label: "100", value: 100 },
];

/* ================= 表单 ================= */
const showForm = ref(false);
const isEditing = ref(false);
const editingId = ref<number | null>(null);

const roleForm = reactive<RoleCreateParams & { id?: number }>({
  scope: "tenant",
  tenant_uuid: undefined,
  code: "",
  name: "",
  description: "",
});

/* ================= API ================= */
const loadTenants = async (keyword?: string) => {
  if (!isRootUser.value) return;
  loadingTenants.value = true;
  try {
    // 参数清洗：只在有值时传 q，避免后端解析 undefined/null
    const params: Record<string, any> = { page: 1, page_size: 10 };
    const q = (keyword ?? "").trim();
    if (q) params.q = q;

    const response = await tenantService.getTenants(params);
    if (response?.code === 200 && response.data) {
      tenants.value = response.data.items;
    } else {
      // 可选：对非 200 的后端返回做兜底
      // console.warn("getTenants 非 200：", response);
    }
  } catch (err) {
    console.error("加载租户列表失败:", err);
  } finally {
    loadingTenants.value = false;
  }
};

const loadRoles = async () => {
  try {
    const params: Record<string, any> = {
      page: pagination.page,
      page_size: pagination.pageSize,
    };
    const kw = (searchQuery.value ?? "").trim();
    if (kw) params.keyword = kw;
    if (selectedScope.value) params.scope = selectedScope.value;
    if (typeof selectedBuiltin.value === "boolean")
      params.builtin = selectedBuiltin.value;

    await roleStore.fetchRoles(params);

    pagination.total = storePagination.value.total;
    pagination.totalPages = storePagination.value.pages;
  } catch (err) {
    console.error("加载角色列表失败:", err);
  }
};

/* ================= 动作 ================= */
const resetForm = () => {
  roleForm.scope = "tenant";
  roleForm.tenant_uuid = undefined;
  roleForm.code = "";
  roleForm.name = "";
  roleForm.description = "";
  delete roleForm.id;
  selectedTenant.value = null;
  isEditing.value = false;
  editingId.value = null;
};

const openAddForm = () => {
  resetForm();
  showForm.value = true;
};

const openEditForm = (role: Role) => {
  roleForm.name = role.name;
  roleForm.code = role.code;
  roleForm.description = role.description || "";
  roleForm.scope = role.scope;
  roleForm.id = role.id;
  isEditing.value = true;
  editingId.value = role.id;
  showForm.value = true;
};

const saveRole = async () => {
  if (!roleForm.name || !roleForm.code) {
    notifyOnce("请填写必填字段", "角色名称和代码为必填项", "warning");
    return;
  }
  try {
    if (isEditing.value && editingId.value !== null) {
      const updateData: RoleUpdateParams = {
        name: roleForm.name,
        description: roleForm.description,
      };
      await roleStore.updateRole(editingId.value, updateData);
    } else {
      const createData: RoleCreateParams = {
        scope: roleForm.scope,
        tenant_uuid: roleForm.tenant_uuid,
        code: roleForm.code,
        name: roleForm.name,
        description: roleForm.description,
      };
      await roleStore.createRole(createData);
    }
    showForm.value = false;
    resetForm();
    await loadRoles();

    const action = isEditing.value ? "更新" : "创建";

    notifyOnce(
      `${action}角色成功`,
      `角色 "${roleForm.name}" 已${action}成功`,
      "success"
    );
  } catch (err) {
    console.error("保存角色失败:", err);
    notifyOnce("保存角色失败", "请检查网络连接或联系管理员", "error");
  }
};

const deleteRole = async (id: number) => {
  if (!confirm("确定要删除此角色吗？")) return;
  try {
    await roleStore.deleteRole(id);
    await loadRoles();
    notifyOnce("删除角色成功", "角色已成功删除", "success");
  } catch (err) {
    console.error("删除角色失败:", err);
    notifyOnce("删除角色失败", "请检查网络连接或联系管理员", "error");
  }
};

/* ================= 列表/分页/搜索 ================= */
const handleSearch = async () => {
  pagination.page = 1;
  await loadRoles();
};

// 给搜索也做点缓冲，省接口
watchDebounced(
  searchQuery,
  () => {
    handleSearch();
  },
  { debounce: 300 }
);

const filteredRoles = computed(() => roles.value);
const paginatedRoles = computed(() => roles.value);

const paginationInfo = computed(() => {
  const start = (pagination.page - 1) * pagination.pageSize + 1;
  const end = Math.min(pagination.page * pagination.pageSize, pagination.total);
  return {
    start: pagination.total > 0 ? start : 0,
    end,
    total: pagination.total,
    page: pagination.page,
    totalPages: pagination.totalPages,
  };
});

const changePage = async (page: number) => {
  if (page >= 1 && page <= pagination.totalPages) {
    pagination.page = page;
    await loadRoles();
  }
};

const changePageSize = async (pageSize: number) => {
  pagination.pageSize = pageSize;
  pagination.page = 1;
  await loadRoles();
};

const hasNextPage = computed(() => pagination.page < pagination.totalPages);
const hasPrevPage = computed(() => pagination.page > 1);

const clearError = () => roleStore.clearError();

/* ================= 表格列定义 ================= */
const UButton = resolveComponent("UButton");
const UBadge = resolveComponent("UBadge");

const columns = computed(() => {
  const _ = locale.value; // 显式依赖
  return [
    {
      id: "name",
      accessorKey: "name",
      header: "角色名称",
      cell: ({ row }: any) => {
        const role = row.original as Role;
        return h("div", { class: "flex items-center gap-2" }, [
          h("span", { class: "font-medium" }, role.name),
          role.builtin &&
            h(
              UBadge,
              { color: "blue", variant: "subtle", size: "xs" },
              { default: () => "系统" }
            ),
        ]);
      },
    },
    {
      id: "code",
      accessorKey: "code",
      header: "角色代码",
      cell: ({ row }: any) => {
        const role = row.original as Role;
        return h(
          "code",
          { class: "text-sm bg-gray-100 px-2 py-1 rounded" },
          role.code
        );
      },
    },
    {
      id: "scope",
      accessorKey: "scope",
      header: "作用域",
      cell: ({ row }: any) => {
        const role = row.original as Role;
        return h(
          UBadge,
          {
            color: role.scope === "system" ? "red" : "green",
            variant: "subtle",
            size: "sm",
          },
          { default: () => (role.scope === "system" ? "系统" : "租户") }
        );
      },
    },
    {
      id: "description",
      accessorKey: "description",
      header: "描述",
      cell: ({ row }: any) => {
        const role = row.original as Role;
        return h(
          "span",
          { class: "text-sm text-gray-600" },
          role.description || "-"
        );
      },
    },
    {
      id: "createdAt",
      accessorKey: "createdAt",
      header: "创建时间",
      cell: ({ row }: any) => {
        const role = row.original as Role;
        return h(
          "span",
          { class: "text-sm text-gray-500" },
          new Date(role.createdAt).toLocaleDateString()
        );
      },
    },
    {
      id: "actions",
      header: "操作",
      cell: ({ row }: any) => {
        const role = row.original as Role;
        return h(
          "div",
          { class: "flex gap-1" },
          [
            h(
              UButton,
              {
                size: "xs",
                variant: "ghost",
                icon: "i-heroicons-pencil-square",
                onClick: () => openEditForm(role),
              },
              { default: () => "编辑" }
            ),
            !role.builtin &&
              h(
                UButton,
                {
                  size: "xs",
                  color: "error",
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

/* ================= 生命周期 ================= */
onMounted(() => {
  loadRoles();
  if (isRootUser.value) {
    // 若你的后端允许无关键字的默认列表，这里可以直接 loadTenants()
    // 否则保持空，等待用户输入搜索词
    // loadTenants();
  }
});
</script>

<template>
  <div>
    <!-- 头部 -->
    <div class="flex justify-between items-center mb-6">
      <div>
        <h2 class="text-xl font-semibold text-gray-800">角色管理</h2>
        <p class="text-sm text-gray-500 mt-1">
          基于RBAC模型的角色权限管理，支持层级权限控制
        </p>
      </div>
      <UButton color="primary" icon="i-heroicons-plus" @click="openAddForm">
        新建角色
      </UButton>
    </div>

    <!-- 过滤 -->
    <div class="flex flex-col md:flex-row gap-4 mb-6">
      <UInput
        v-model="searchQuery"
        icon="i-heroicons-magnifying-glass"
        placeholder="搜索角色名称、代码..."
        class="flex-1"
      />
      <USelect
        v-model="selectedScope"
        :items="[
          { label: '全部作用域', value: null },
          { label: '系统角色', value: 'system' },
          { label: '租户角色', value: 'tenant' },
        ]"
        placeholder="全部作用域"
        class="w-full md:w-40"
      />
      <USelect
        v-model="selectedBuiltin"
        :items="[
          { label: '全部类型', value: null },
          { label: '系统内置', value: true },
          { label: '自定义', value: false },
        ]"
        placeholder="全部类型"
        class="w-full md:w-40"
      />
    </div>

    <!-- 加载状态 -->
    <div v-if="loading" class="flex justify-center py-8">
      <UIcon name="i-heroicons-arrow-path" class="w-6 h-6 animate-spin" />
      <span class="ml-2">加载中...</span>
    </div>

    <!-- 表格 -->
    <div v-else class="bg-white rounded-lg shadow-sm">
      <UTable :data="paginatedRoles" :columns="columns" />

      <!-- 分页控件 -->
      <div
        v-if="pagination.totalPages > 1"
        class="px-6 py-4 border-t border-gray-200"
      >
        <div class="flex justify-between items-center">
          <div class="text-sm text-gray-600">
            第 {{ pagination.page }} 页，共 {{ pagination.totalPages }} 页
          </div>
          <div class="flex gap-2">
            <UButton
              :disabled="!hasPrevPage"
              variant="outline"
              size="sm"
              icon="i-heroicons-chevron-left"
              @click="changePage(pagination.page - 1)"
            >
              上一页
            </UButton>

            <template
              v-for="page in Math.min(5, pagination.totalPages)"
              :key="page"
            >
              <UButton
                v-if="
                  Math.abs(page - pagination.page) <= 2 ||
                  page === 1 ||
                  page === pagination.totalPages
                "
                :variant="page === pagination.page ? 'solid' : 'outline'"
                size="sm"
                @click="changePage(page)"
              >
                {{ page }}
              </UButton>
            </template>

            <UButton
              :disabled="!hasNextPage"
              variant="outline"
              size="sm"
              icon="i-heroicons-chevron-right"
              @click="changePage(pagination.page + 1)"
            >
              下一页
            </UButton>
          </div>
        </div>
      </div>
    </div>

    <!-- 空状态 -->
    <div
      v-if="!loading && filteredRoles.length === 0"
      class="text-center py-12 bg-gray-50 rounded-lg mt-4"
    >
      <UIcon
        name="i-heroicons-shield-check"
        class="w-12 h-12 text-gray-400 mx-auto mb-4"
      />
      <h3 class="text-lg font-medium text-gray-900 mb-2">暂无角色数据</h3>
      <p class="text-gray-500 mb-4">
        {{
          searchQuery
            ? "没有找到匹配的角色，请尝试调整筛选条件"
            : "点击「新建角色」按钮创建第一个角色"
        }}
      </p>
      <div class="flex justify-center gap-3">
        <UButton v-if="!searchQuery" color="primary" @click="openAddForm">
          新建角色
        </UButton>
        <UButton
          v-if="searchQuery"
          color="neutral"
          variant="outline"
          @click="searchQuery = ''"
        >
          重置筛选
        </UButton>
      </div>
    </div>

    <!-- 角色表单对话框 -->
    <UModal
      v-model:open="showForm"
      title="role-manager-title"
      description="role-manager-desc"
      :ui="{ content: 'sm:max-w-md' }"
    >
      <template #content>
        <UCard>
          <template #header>
            <h3 class="text-lg font-medium text-gray-900">
              {{ isEditing ? "编辑角色" : "新建角色" }}
            </h3>
          </template>

          <form @submit.prevent="saveRole">
            <div class="space-y-4">
              <UFormField label="角色名称" required>
                <UInput v-model="roleForm.name" placeholder="输入角色名称" />
              </UFormField>

              <UFormField v-if="!isEditing" label="角色代码" required>
                <UInput
                  v-model="roleForm.code"
                  placeholder="输入角色代码（英文，如：sales_manager）"
                />
              </UFormField>

              <!-- <UFormField v-if="!isEditing" label="作用域">
                <USelect
                  v-model="roleForm.scope"
                  :options="[
                    { label: '租户角色', value: 'tenant' },
                    { label: '系统角色', value: 'system' },
                  ]"
                />
              </UFormField> -->

              <UFormField
                v-if="!isEditing && isRootUser && roleForm.scope === 'tenant'"
                label="选择租户"
                required
              >
                <USelectMenu
                  v-model="selectedTenant"
                  :items="tenantOptions"
                  :loading="loadingTenants"
                  searchable
                  v-model:search-term="tenantKeyword"
                  placeholder="选择租户"
                  class="w-full"
                >
                  <template #option="{ option }">
                    <div class="flex flex-col">
                      <span class="font-medium">{{ option.label }}</span>
                      <span class="text-xs text-gray-500">{{
                        option.description
                      }}</span>
                    </div>
                  </template>
                </USelectMenu>
              </UFormField>

              <UFormField label="角色描述">
                <UTextarea
                  v-model="roleForm.description"
                  placeholder="输入角色描述"
                  :rows="3"
                />
              </UFormField>
            </div>

            <div class="mt-6 flex justify-end space-x-3">
              <UButton
                color="neutral"
                variant="outline"
                @click="showForm = false"
              >
                取消
              </UButton>
              <UButton type="submit" color="primary">
                {{ isEditing ? "更新角色" : "创建角色" }}
              </UButton>
            </div>
          </form>
        </UCard>
      </template>
    </UModal>
  </div>
</template>
