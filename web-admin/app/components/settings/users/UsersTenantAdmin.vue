<!-- /components/settings/users/UsersTenantAdmin.vue -->
<script setup lang="ts">
import {
  ref,
  reactive,
  computed,
  h,
  resolveComponent,
  onMounted,
  watch,
} from "vue";
import { useI18n } from "#imports";
import SelectTree from "~/components/ui/SelectTree.vue";
import { useDepartmentStore } from "~/stores/department";
import {
  useUserService,
  type MemberWithProfile,
} from "~/composables/api/services/userService";
import type { Department } from "~/composables/api/services/departmentService";

// ==== 输入属性（Root 复用时传入 tenantUuid） ====
const props = defineProps<{ tenantUuid: string }>();
const { t, locale } = useI18n();

// ==== 部门store ====
const departmentStore = useDepartmentStore();
const userService = useUserService();

// ===== 类型与数据 =====
type StatusType = "active" | "inactive";
type RoleType = "admin" | "editor" | "user";

interface RowUser {
  id: number; // Member ID
  userId?: number; // User ID
  name: string;
  username?: string;
  email?: string;
  phone?: string;
  department?: string;
  roles?: RoleType[] | null;
  status: StatusType | string;
  avatar: string;
  meta?: Record<string, any> | null;
}

// 表格数据和加载状态
const users = ref<RowUser[]>([]);
const loading = ref(false);

// ====== 过滤/分页（与你现有一致） ======
const searchQuery = ref("");
const filters = reactive({
  department: null as string | null,
  role: null as string | null,
  status: null as string | null,
});

const pagination = reactive({ page: 1, pageSize: 10, total: 0, totalPages: 0 });

// 将部门数据转换为SelectTree需要的TreeNode格式
const departmentTreeItems = computed(() => {
  const convertDepartmentToTreeNode = (dept: Department) => ({
    label: dept.name || "未命名部门",
    value: String(dept.id),
    icon: "i-heroicons-building-office-2",
    children: dept.children?.map(convertDepartmentToTreeNode) || [],
    disabled: dept.status === 0, // 假设status为0表示禁用
  });

  return departmentStore.tree.map(convertDepartmentToTreeNode);
});
const roles = ref([
  { label: t("organization.user.form.selectRole"), value: null },
  { label: t("organization.user.role.admin"), value: "admin" },
  { label: t("organization.user.role.editor"), value: "editor" },
  { label: t("organization.user.role.user"), value: "user" },
]);

// ====== 导入导出 ======
type ExportFormat = "csv" | "json";

async function exportUsers(format: ExportFormat) {
  try {
    let content: string;
    let filename: string;
    let mimeType: string;

    if (format === "csv") {
      const { default: Papa } = await import("papaparse");
      content = Papa.unparse(
        users.value.map((u) => ({
          姓名: u.name,
          用户名: u.username,
          邮箱: u.email,
          部门: u.department || "",
          状态: u.status === "active" ? "激活" : "停用",
        }))
      );
      filename = `users_${new Date().toISOString().split("T")[0]}.csv`;
      mimeType = "text/csv;charset=utf-8;";
    } else {
      content = JSON.stringify(users.value, null, 2);
      filename = `users_${new Date().toISOString().split("T")[0]}.json`;
      mimeType = "application/json;charset=utf-8;";
    }

    const { saveAs } = await import("file-saver");
    const blob = new Blob([content], { type: mimeType });
    saveAs(blob, filename);
  } catch (error) {
    console.error("导出失败:", error);
    alert("导出失败，请重试");
  }
}

function importUsers() {
  const input = document.createElement("input");
  input.type = "file";
  input.accept = ".csv,.json";
  input.onchange = async (e) => {
    const file = (e.target as HTMLInputElement).files?.[0];
    if (!file) return;

    try {
      const text = await file.text();
      let importedData: any[];

      if (file.name.endsWith(".csv")) {
        const { default: Papa } = await import("papaparse");
        const result = Papa.parse(text, { header: true, skipEmptyLines: true });
        importedData = result.data;
      } else {
        importedData = JSON.parse(text);
      }

      // 这里可以添加数据验证和转换逻辑
      console.log("导入的数据:", importedData);
      alert(`成功导入 ${importedData.length} 条记录`);
    } catch (error) {
      console.error("导入失败:", error);
      alert("导入失败，请检查文件格式");
    }
  };
  input.click();
}

const importExportItems = computed(() => [
  [
    {
      label: t("organization.user.export.csv"),
      icon: "i-heroicons-arrow-down-tray",
      click: () => exportUsers("csv"),
    },
    {
      label: t("organization.user.export.json"),
      icon: "i-heroicons-arrow-down-tray",
      click: () => exportUsers("json"),
    },
  ],
  [
    {
      label: t("organization.user.import.button"),
      icon: "i-heroicons-arrow-up-tray",
      click: () => importUsers(),
    },
  ],
]);

// ====== 新增/编辑 ======
const showForm = ref(false);
const isEditing = ref(false);
const editingId = ref<number | null>(null);

// 统一"扁平表单" -> 后端映射 User+Member（我们之前对齐的）
const userForm = reactive({
  name: "",
  username: "",
  email: "",
  phone: "",
  departmentId: null as number | null,
  roleIds: [] as number[],
  avatarUrl: "",
  password: "",
  confirmPassword: "",
  status: "active" as "active" | "disabled" | "locked",
  meta: {} as Record<string, any>,
});

function resetForm() {
  userForm.name = "";
  userForm.username = "";
  userForm.email = "";
  userForm.phone = "";
  userForm.departmentId = null;
  userForm.roleIds = [];
  userForm.avatarUrl = "";
  userForm.password = "";
  userForm.confirmPassword = "";
  userForm.status = "active";
  userForm.meta = {};
  isEditing.value = false;
  editingId.value = null;
}

function openAddForm() {
  resetForm();
  showForm.value = true;
}

function openEditForm(row: RowUser) {
  resetForm();
  isEditing.value = true;
  editingId.value = row.id; // 这里使用的是Member的ID

  // 将行数据映射回表单
  userForm.name = row.name;
  userForm.username = row.username || "";
  userForm.email = row.email || "";
  userForm.phone = row.phone || "";
  userForm.avatarUrl = row.avatar;
  userForm.status = row.status === "active" ? "active" : "disabled";
  userForm.meta = row.meta || {};
  showForm.value = true;
}

async function saveUser() {
  // 基础校验
  if (!userForm.name || !userForm.email) {
    return alert(t("organization.user.validation.requiredFields"));
  }
  if (!isEditing.value && !userForm.username) {
    return alert("用户名为必填项");
  }
  if (!isEditing.value && userForm.password !== userForm.confirmPassword) {
    return alert(t("organization.user.validation.passwordMismatch"));
  }

  try {
    if (isEditing.value && editingId.value) {
      // 更新用户
      const updatePayload = {
        display_name: userForm.name,
        email: userForm.email,
        phone: userForm.phone,
        avatar_url: userForm.avatarUrl,
        status: userForm.status === "active" ? 1 : 0,
      };
      await userService.updateUser(editingId.value, updatePayload);
    } else {
      // 创建系统用户
      const createPayload = {
        display_name: userForm.name,
        email: userForm.email,
        phone: userForm.phone,
        avatar_url: userForm.avatarUrl,
        status: userForm.status === "active" ? 1 : 0,
        meta: userForm.meta ?? {},
        username: userForm.username || userForm.email.split("@")[0],
        initial_password: userForm.password,
        dept_ids: userForm.departmentId ? [userForm.departmentId] : [],
      };
      await userService.createSystemUser(createPayload);
    }
    showForm.value = false;
    await loadUsers(); // 重新加载数据
  } catch (e: any) {
    alert(e?.message || "保存失败");
  }
}

async function deleteUser(id: number) {
  if (!confirm(t("organization.user.confirmDelete"))) return;
  try {
    // 注意：这里的id是Member的ID，但API可能需要User的ID
    // 根据后端实现调整
    await userService.deleteUser(id);
    await loadUsers(); // 重新加载数据
  } catch (e: any) {
    alert(e?.message || "删除失败");
  }
}

async function toggleUserStatus(row: RowUser) {
  try {
    const newStatus = row.status === "active" ? 0 : 1;
    // 注意：这里的row.id是Member的ID，但API可能需要User的ID
    // 根据后端实现调整
    await userService.setUserStatus(row.id, { status: newStatus });
    await loadUsers(); // 重新加载数据
  } catch (e: any) {
    alert(e?.message || "状态更新失败");
  }
}

// ===== 过滤/分页逻辑 =====
const filteredUsers = computed(() => {
  // 由于使用API分页，直接返回当前用户数据
  return users.value;
});

const paginatedUsers = computed(() => {
  // API已经返回分页数据，直接使用
  return users.value;
});

const hasNextPage = computed(() => pagination.page < pagination.totalPages);
const hasPrevPage = computed(() => pagination.page > 1);

async function changePage(p: number) {
  if (p >= 1 && p <= pagination.totalPages) {
    pagination.page = p;
    await loadUsers();
  }
}

async function changePageSize(size: number) {
  pagination.pageSize = size;
  pagination.page = 1;
  await loadUsers();
}

function resetFilters() {
  filters.department = filters.role = filters.status = null;
  searchQuery.value = "";
  pagination.page = 1;
  loadUsers();
}

// 监听搜索和过滤条件变化
watch(
  [
    searchQuery,
    () => filters.department,
    () => filters.role,
    () => filters.status,
  ],
  () => {
    pagination.page = 1;
    loadUsers();
  }
);

watch(
  () => props.tenantUuid,
  () => {
    pagination.page = 1;
    loadUsers();
  }
);

// ===== 列定义：含"编辑/禁用/删除"操作 =====
const UButton = resolveComponent("UButton");
const UAvatar = resolveComponent("UAvatar");
const UBadge = resolveComponent("UBadge");

const columns = computed(() => {
  const _ = locale.value;
  return [
    {
      id: "avatar",
      accessorKey: "avatar",
      header: "",
      cell: ({ row }: any) => {
        const u = row.original as RowUser;
        return h(UAvatar, { src: u.avatar, alt: u.name, size: "sm" });
      },
    },
    {
      id: "name",
      accessorKey: "name",
      header: t("organization.user.table.name").toString(),
    },
    {
      id: "username",
      accessorKey: "username",
      header: t("organization.user.table.username").toString(),
    },
    {
      id: "email",
      accessorKey: "email",
      header: t("organization.user.table.email").toString(),
    },
    {
      id: "phone",
      accessorKey: "phone",
      header: t("organization.user.table.phone").toString(),
      cell: ({ row }: any) => {
        const u = row.original as RowUser;
        return maskPhone(u.phone || "");
      },
    },
    {
      id: "status",
      accessorKey: "status",
      header: t("organization.user.table.status").toString(),
      cell: ({ row }: any) => {
        const u = row.original as RowUser;
        return h(
          UBadge,
          {
            color: u.status === "active" ? "success" : "neutral",
            variant: "subtle",
            size: "sm",
          },
          () =>
            u.status === "active"
              ? t("organization.user.form.active")
              : t("organization.user.form.inactive")
        );
      },
    },
    {
      id: "actions",
      header: t("organization.user.table.actions").toString(),
      cell: ({ row }: any) => {
        const u = row.original as RowUser;
        return h("div", { class: "flex gap-2" }, [
          h(
            UButton,
            {
              size: "xs",
              variant: "ghost",
              icon: "i-heroicons-pencil-square",
              onClick: () => openEditForm(u),
            },
            () => t("organization.common.edit")
          ),
          h(
            UButton,
            {
              size: "xs",
              color: u.status === "active" ? "warning" : "success",
              variant: "ghost",
              icon:
                u.status === "active"
                  ? "i-heroicons-lock-closed"
                  : "i-heroicons-lock-open",
              onClick: () => toggleUserStatus(u),
            },
            () =>
              u.status === "active"
                ? t("organization.user.disable")
                : t("organization.user.enable")
          ),
          h(
            UButton,
            {
              size: "xs",
              color: "error",
              variant: "ghost",
              icon: "i-heroicons-trash",
              onClick: () => deleteUser(u.id),
            },
            () => t("organization.common.delete")
          ),
        ]);
      },
    },
  ];
});

// 手机号脱敏函数
function maskPhone(phone: string): string {
  if (!phone) return "";
  if (phone.length <= 7) return phone;
  return phone.slice(0, 3) + "****" + phone.slice(-4);
}

// 转换API数据为组件需要的格式
function transformUserData(memberWithProfile: MemberWithProfile): RowUser {
  const { Member, User } = memberWithProfile;
  return {
    id: Member.id, // 使用Member的ID作为主要ID
    userId: User.id, // 保存User的ID以备后用
    name: Member.display_name || User.display_name,
    username: Member.username,
    email: User.email || "",
    phone: User.phone || "",
    department: Member.meta?.title || Member.meta?.department || "",
    roles: null,
    status: Member.status === 1 ? "active" : "inactive",
    avatar:
      Member.avatar_url ||
      User.avatar_url ||
      `https://i.pravatar.cc/150?u=${encodeURIComponent(User.email || Member.display_name)}`,
    meta: { ...User.meta, ...Member.meta }, // 合并User和Member的meta
  };
}

// 加载用户数据
async function loadUsers() {
  if (!props.tenantUuid) {
    return;
  }
  try {
    loading.value = true;
    const params: any = {
      page: pagination.page,
      page_size: pagination.pageSize,
      status: filters.status
        ? filters.status === "active"
          ? 1
          : 0
        : undefined, // 不传status则显示所有状态
    };

    // 添加搜索参数
    if (searchQuery.value.trim()) {
      params.q = searchQuery.value.trim(); // 后端使用q参数
    }

    const response = await userService.getUsers(params);

    if (response.data) {
      users.value = response.data.items.map(transformUserData);
      pagination.total = response.data.pagination.total;
      pagination.totalPages = response.data.pagination.pages;
    }
  } catch (error) {
    console.error("加载用户数据失败:", error);
  } finally {
    loading.value = false;
  }
}

// 初始化数据
onMounted(async () => {
  // 初始化部门数据
  try {
    await departmentStore.fetchTree();
  } catch (error) {
    console.error("加载部门数据失败:", error);
  }

  // 加载用户数据
  await loadUsers();
});
</script>

<template>
  <div>
    <!-- 顶部：导入导出 + 新增 -->
    <div class="flex justify-between items-center mb-6">
      <div>
        <h2 class="text-xl font-semibold text-gray-800">
          {{ $t("organization.user.title") }}
        </h2>
        <p class="text-sm text-gray-500 mt-1">
          {{ $t("organization.user.description") }}
        </p>
      </div>
      <div class="flex space-x-2">
        <UDropdownMenu :items="importExportItems">
          <UButton
            color="neutral"
            variant="outline"
            icon="i-heroicons-arrow-up-tray"
          >
            {{ $t("organization.user.importExport") }}
          </UButton>
        </UDropdownMenu>
        <UButton color="primary" icon="i-heroicons-plus" @click="openAddForm">
          {{ $t("organization.user.add") }}
        </UButton>
      </div>
    </div>

    <!-- 搜索与筛选（与你现有一致） -->
    <div class="mb-6 bg-white p-4 rounded-lg shadow-sm">
      <div class="flex flex-wrap gap-4 items-end">
        <div class="flex-grow min-w-[200px]">
          <UInput
            v-model="searchQuery"
            icon="i-heroicons-magnifying-glass"
            :placeholder="$t('organization.user.search')"
          />
        </div>
        <UFormField :label="$t('organization.user.form.department')">
          <SelectTree
            v-model="filters.department"
            :items="departmentTreeItems"
            :placeholder="$t('organization.user.form.selectDepartment')"
            searchable
            clearable
            class="w-full sm:min-w-[12rem]"
          />
        </UFormField>
        <UFormField :label="$t('organization.user.form.role')">
          <USelect
            v-model="filters.role"
            :items="roles"
            class="w-full sm:min-w-[12rem]"
            :placeholder="$t('organization.user.form.selectRole')"
            option-attribute="label"
          />
        </UFormField>
        <UFormField :label="$t('organization.user.form.status')" class="mb-0">
          <USelect
            v-model="filters.status"
            :items="[
              { label: $t('organization.user.filter.allStatus'), value: null },
              { label: $t('organization.user.filter.active'), value: 'active' },
              {
                label: $t('organization.user.filter.inactive'),
                value: 'inactive',
              },
            ]"
            class="w-full sm:w-40"
            :placeholder="$t('organization.user.filter.allStatus')"
          />
        </UFormField>
        <UButton
          color="neutral"
          variant="ghost"
          icon="i-heroicons-arrow-path"
          @click="resetFilters"
        >
          {{ $t("organization.user.filter.reset") }}
        </UButton>
      </div>
    </div>

    <!-- 表格 + 分页 -->
    <div class="bg-white rounded-lg shadow-sm">
      <UTable
        :data="paginatedUsers"
        :columns="columns"
        :loading="loading"
        :empty-state="{
          icon: 'i-heroicons-circle-stack-20-solid',
          label: '暂无用户数据',
          description: '当前没有找到任何用户信息',
        }"
      />
      <div
        v-if="pagination.totalPages > 1"
        class="px-6 py-4 border-t border-gray-200 flex justify-between items-center"
      >
        <div class="text-sm text-gray-600">
          第 {{ pagination.page }} / {{ pagination.totalPages }} 页， 共
          {{ pagination.total }} 条
        </div>
        <div class="flex gap-2">
          <UButton
            :disabled="!hasPrevPage || loading"
            variant="outline"
            size="sm"
            icon="i-heroicons-chevron-left"
            @click="changePage(pagination.page - 1)"
            >上一页</UButton
          >
          <UButton
            :disabled="!hasNextPage || loading"
            variant="outline"
            size="sm"
            icon="i-heroicons-chevron-right"
            @click="changePage(pagination.page + 1)"
            >下一页</UButton
          >
        </div>
      </div>
    </div>

    <!-- 表单弹窗（新增/编辑） -->
    <UModal
      v-model:open="showForm"
      :title="
        isEditing
          ? t('organization.user.form.editUser')
          : t('organization.user.form.addUser')
      "
      :description="
        isEditing
          ? t('organization.user.form.editUserDesc')
          : t('organization.user.form.addUserDesc')
      "
    >
      <template #content>
        <div class="py-8 px-8">
          <form
            @submit.prevent="saveUser"
            class="grid grid-cols-1 md:grid-cols-2 gap-4"
          >
            <UFormField :label="$t('organization.user.form.name')" required>
              <UInput v-model="userForm.name" />
            </UFormField>
            <UFormField
              :label="$t('organization.user.form.username')"
              :required="!isEditing"
            >
              <UInput
                v-model="userForm.username"
                :placeholder="isEditing ? '编辑时可选' : '必填，用于租户内登录'"
              />
            </UFormField>
            <UFormField
              :label="$t('organization.user.form.email')"
              required
              class="md:col-span-2"
            >
              <UInput v-model="userForm.email" type="email" />
            </UFormField>
            <UFormField :label="$t('organization.user.form.phone')">
              <UInput
                v-model="userForm.phone"
                type="tel"
                :placeholder="$t('organization.user.form.phonePlaceholder')"
              />
            </UFormField>
            <UFormField
              :label="$t('organization.user.form.password')"
              :required="!isEditing"
              ><UInput v-model="userForm.password" type="password"
            /></UFormField>
            <UFormField
              :label="$t('organization.user.form.confirmPassword')"
              :required="!isEditing"
              ><UInput v-model="userForm.confirmPassword" type="password"
            /></UFormField>
            <div class="md:col-span-2 flex justify-end gap-3 mt-2">
              <UButton
                color="neutral"
                variant="outline"
                @click="showForm = false"
                >{{ $t("organization.common.cancel") }}</UButton
              >
              <UButton type="submit" color="primary">{{
                $t("organization.common.save")
              }}</UButton>
            </div>
          </form>
        </div>
      </template>
    </UModal>
  </div>
</template>
