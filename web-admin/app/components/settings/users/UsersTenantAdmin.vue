<!-- /components/settings/users/UsersTenantAdmin.vue -->
<script setup lang="ts">
import {
  ref,
  reactive,
  computed,
  h,
  resolveComponent,
  onMounted,
  nextTick,
  watch,
} from "vue";
import { storeToRefs } from "pinia";
import { useI18n, useToast } from "#imports";
import SelectTree from "~/components/ui/SelectTree.vue";
import { useDepartmentStore } from "~/stores/department";
import { useUserStore } from "~/stores/user";
import { useRoleService } from "~/composables/api/services/roleService";
import {
  useUserService,
  type MemberWithProfile,
} from "~/composables/api/services/userService";
import type { Department } from "~/composables/api/services/departmentService";

// ==== 输入属性（Root 复用时传入 tenantUuid） ====
const props = defineProps<{ tenantUuid: string }>();
const { t, locale } = useI18n();
const toast = useToast();

// ==== 部门store ====
const departmentStore = useDepartmentStore();
const userService = useUserService();
const roleService = useRoleService();
const userStore = useUserStore();
const { user: currentUser, isRoot, isCurrentTenantAdmin } = storeToRefs(userStore);

// ===== 类型与数据 =====
type StatusType = "active" | "inactive";

interface RowUser {
  id: string; // Member UUID
  userUuid?: string; // User UUID
  name: string;
  username?: string;
  email?: string;
  phone?: string;
  department?: string;
  roles?: string[] | null;
  status: StatusType | string;
  isRoot?: boolean;
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

const roleFilterItems = ref([
  { label: t("organization.user.form.selectRole"), value: null },
  { label: t("organization.user.role.admin"), value: "admin" },
  { label: t("organization.user.role.editor"), value: "editor" },
  { label: t("organization.user.role.user"), value: "user" },
]);

type RoleOption = { label: string; value: string; code?: string };
const tenantRoleOptions = ref<RoleOption[]>([]);
const loadingRoles = ref(false);
const usernamePattern = /^[a-z][a-z0-9._-]{2,63}$/;
const roleSelectContent = {
  side: "bottom" as const,
  sideOffset: 8,
  collisionPadding: 16,
  position: "popper" as const,
};
const roleSelectUi = {
  content:
    "z-[120] min-w-[var(--reka-combobox-trigger-width)] max-h-72 overflow-y-auto",
};

const currentUserUuid = computed(() => String(currentUser.value?.uuid || ""));
const canViewRootUsers = computed(() => Boolean(isCurrentTenantAdmin.value));

function canOperateRootUser(row: RowUser): boolean {
  if (!row.isRoot) return true;
  return Boolean(
    isRoot.value && row.userUuid && currentUserUuid.value === row.userUuid
  );
}

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
    notifyError("导出失败", error, "请重试");
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
      console.info("导入的数据:", importedData);
      toast.add({
        title: "导入成功",
        description: `成功导入 ${importedData.length} 条记录`,
        color: "success",
      });
    } catch (error) {
      console.error("导入失败:", error);
      notifyError("导入失败", error, "请检查文件格式");
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
const editingUserUuid = ref<string | null>(null);

// 统一"扁平表单" -> 后端映射 User+Member（我们之前对齐的）
const userForm = reactive({
  name: "",
  username: "",
  email: "",
  phone: "",
  departmentId: null as number | null,
  roleUuids: [] as string[],
  avatarUrl: "",
  password: "",
  confirmPassword: "",
  status: "active" as "active" | "disabled" | "locked",
  meta: {} as Record<string, any>,
});
const selectedRoleOptions = computed<RoleOption[]>({
  get() {
    const selected = new Set(normalizeRoleUUIDs(userForm.roleUuids));
    return tenantRoleOptions.value.filter((option) =>
      selected.has(option.value)
    );
  },
  set(options) {
    userForm.roleUuids = normalizeRoleUUIDs(options);
  },
});

const showResetPassword = ref(false);
const resetPasswordTarget = ref<RowUser | null>(null);
const resetPasswordForm = reactive({
  password: "",
  confirmPassword: "",
});

function normalizeRoleUUIDs(input: unknown): string[] {
  if (!Array.isArray(input)) return [];
  const out: string[] = [];
  for (const item of input) {
    let raw: unknown = item;
    if (item && typeof item === "object" && "value" in (item as any)) {
      raw = (item as any).value;
    }
    const value = String(raw || "").trim();
    if (value) out.push(value);
  }
  return Array.from(new Set(out));
}

function parseApiMessage(error: any, fallback: string): string {
  const msg =
    error?.response?._data?.error ||
    error?.response?._data?.message ||
    error?.data?.error ||
    error?.data?.message ||
    error?.message ||
    fallback;
  const text = String(msg || fallback);
  if (text.includes("uk_user_phone")) return "手机号已被占用，请更换后重试";
  if (text.includes("uk_user_email")) return "邮箱已被占用，请更换后重试";
  return text;
}

function notifyError(title: string, error: any, fallback: string) {
  toast.add({
    title,
    description: parseApiMessage(error, fallback),
    color: "error",
  });
}

function resetForm() {
  userForm.name = "";
  userForm.username = "";
  userForm.email = "";
  userForm.phone = "";
  userForm.departmentId = null;
  userForm.roleUuids = [];
  userForm.avatarUrl = "";
  userForm.password = "";
  userForm.confirmPassword = "";
  userForm.status = "active";
  userForm.meta = {};
  isEditing.value = false;
  editingUserUuid.value = null;
}

async function openAddForm() {
  resetForm();
  await loadTenantRoles();
  if (tenantRoleOptions.value.length > 0) {
    const roleUser = tenantRoleOptions.value.find(
      (role) => role.code === "role_user"
    );
    userForm.roleUuids = [roleUser?.value || tenantRoleOptions.value[0].value];
  }
  showForm.value = true;
  await nextTick();
  userForm.username = "";
  userForm.email = "";
  userForm.phone = "";
  if (typeof window !== "undefined") {
    window.requestAnimationFrame(() => {
      userForm.username = "";
      userForm.email = "";
      userForm.phone = "";
    });
  }
}

async function openEditForm(row: RowUser) {
  if (!canOperateRootUser(row)) {
    toast.add({
      title: String(t("organization.user.toast.editDenied")),
      description: String(t("organization.user.validation.rootEditSelfOnly")),
      color: "warning",
    });
    return;
  }
  if (!row.userUuid) {
    toast.add({
      title: String(t("organization.user.toast.openEditFailed")),
      description: String(t("organization.user.validation.missingUserId")),
      color: "error",
    });
    return;
  }
  resetForm();
  isEditing.value = true;
  editingUserUuid.value = row.userUuid;

  // 将行数据映射回表单
  userForm.name = row.name;
  userForm.username = row.username || "";
  userForm.email = row.email || "";
  userForm.phone = row.phone || "";
  userForm.avatarUrl = row.avatar;
  userForm.status = row.status === "active" ? "active" : "disabled";
  userForm.meta = row.meta || {};
  await loadTenantRoles();
  await loadUserRoles(editingUserUuid.value);
  showForm.value = true;
}

async function saveUser() {
  userForm.roleUuids = normalizeRoleUUIDs(userForm.roleUuids);
  const displayName = userForm.name.trim();
  const email = userForm.email.trim();
  const username = userForm.username.trim().toLowerCase();
  const phone = userForm.phone.trim();

  // 基础校验
  if (!displayName || !email) {
    toast.add({
      title: String(t("organization.user.toast.validationFailed")),
      description: String(t("organization.user.validation.requiredFields")),
      color: "warning",
    });
    return;
  }
  if (!username) {
    toast.add({
      title: String(t("organization.user.toast.validationFailed")),
      description: String(t("organization.user.validation.usernameRequired")),
      color: "warning",
    });
    return;
  }
  if (!usernamePattern.test(username)) {
    toast.add({
      title: String(t("organization.user.toast.validationFailed")),
      description: String(t("organization.user.validation.usernameFormat")),
      color: "warning",
    });
    return;
  }
  if (!isEditing.value && userForm.password !== userForm.confirmPassword) {
    toast.add({
      title: String(t("organization.user.toast.validationFailed")),
      description: String(t("organization.user.validation.passwordMismatch")),
      color: "warning",
    });
    return;
  }
  if (userForm.roleUuids.length === 0) {
    toast.add({
      title: String(t("organization.user.toast.validationFailed")),
      description: String(t("organization.user.validation.roleRequired")),
      color: "warning",
    });
    return;
  }

  try {
    if (isEditing.value && editingUserUuid.value) {
      // 更新用户
      const updatePayload = {
        tenant_uuid: props.tenantUuid,
        username,
        display_name: displayName,
        email,
        phone,
        avatar_url: userForm.avatarUrl.trim(),
        status: userForm.status === "active" ? 1 : 0,
      };
      await userService.updateUser(editingUserUuid.value, updatePayload);
      await userService.setUserRoles(editingUserUuid.value, {
        tenant_uuid: props.tenantUuid,
        role_uuids: userForm.roleUuids,
      });
    } else {
      // 创建系统用户
      const createPayload = {
        tenant_uuid: props.tenantUuid,
        display_name: displayName,
        email,
        phone: phone || undefined,
        avatar_url: userForm.avatarUrl.trim(),
        status: userForm.status === "active" ? 1 : 0,
        meta: userForm.meta ?? {},
        username,
        initial_password: userForm.password,
        dept_ids: userForm.departmentId ? [userForm.departmentId] : [],
        role_uuids: userForm.roleUuids,
      };
      await userService.createSystemUser(createPayload);
    }
    showForm.value = false;
    await loadUsers(); // 重新加载数据
    toast.add({
      title: isEditing.value
        ? String(t("organization.user.toast.updated"))
        : String(t("organization.user.toast.created")),
      color: "success",
    });
  } catch (e: any) {
    notifyError(
      String(t("organization.user.toast.saveFailed")),
      e,
      String(t("organization.user.toast.saveFailed"))
    );
  }
}

async function loadTenantRoles() {
  if (!props.tenantUuid) return;
  try {
    loadingRoles.value = true;
    const response = await roleService.getRoles({
      scope: "tenant",
      tenant_uuid: props.tenantUuid,
      page: 1,
      size: 200,
    });
    tenantRoleOptions.value = (response.data?.items || []).map((role: any) => {
      return {
        label: role.name,
        value: role.uuid,
        code: role.code,
      };
    });
  } catch (error) {
    console.error("加载角色失败:", error);
    tenantRoleOptions.value = [];
  } finally {
    loadingRoles.value = false;
  }
}

async function loadUserRoles(userUuid: string | null) {
  if (!userUuid) return;
  try {
    const response = await userService.getUserRoles(userUuid, {
      tenant_uuid: props.tenantUuid,
    });
    userForm.roleUuids = normalizeRoleUUIDs(response.data?.role_uuids || []);
  } catch (error) {
    console.error("加载用户角色失败:", error);
    userForm.roleUuids = [];
  }
}

const roleNameByUUID = computed(() => {
  const map = new Map<string, string>();
  tenantRoleOptions.value.forEach((role) => map.set(role.value, role.label));
  return map;
});

async function hydrateRowsRoles(rows: RowUser[]) {
  await Promise.all(
    rows.map(async (row) => {
      if (!row.userUuid) {
        row.roles = [];
        return;
      }
      try {
        const response = await userService.getUserRoles(row.userUuid, {
          tenant_uuid: props.tenantUuid,
        });
        const roleUUIDs = normalizeRoleUUIDs(response.data?.role_uuids || []);
        row.roles = roleUUIDs.map(
          (roleUUID) => roleNameByUUID.value.get(roleUUID) || "-"
        );
      } catch (error) {
        console.error("加载用户角色失败:", error);
        row.roles = [];
      }
    })
  );
}

async function deleteUser(userUuid: string) {
  if (!confirm(t("organization.user.confirmDelete"))) return;
  try {
    await userService.deleteUser(userUuid);
    await loadUsers(); // 重新加载数据
    toast.add({
      title: String(t("organization.user.toast.deleted")),
      color: "success",
    });
  } catch (e: any) {
    notifyError(
      String(t("organization.user.toast.deleteFailed")),
      e,
      String(t("organization.user.toast.deleteFailed"))
    );
  }
}

async function toggleUserStatus(row: RowUser) {
  if (!row.userUuid) {
    toast.add({
      title: String(t("organization.user.toast.statusUpdateFailed")),
      description: String(t("organization.user.validation.missingUserId")),
      color: "error",
    });
    return;
  }
  try {
    const newStatus = row.status === "active" ? 0 : 1;
    await userService.setUserStatus(row.userUuid, { status: newStatus });
    await loadUsers(); // 重新加载数据
    toast.add({
      title: String(t("organization.user.toast.statusUpdated")),
      color: "success",
    });
  } catch (e: any) {
    notifyError(
      String(t("organization.user.toast.statusUpdateFailed")),
      e,
      String(t("organization.user.toast.statusUpdateFailed"))
    );
  }
}

function openResetPassword(row: RowUser) {
  if (!canOperateRootUser(row)) {
    toast.add({
      title: String(t("organization.user.toast.resetPasswordDenied")),
      description: String(t("organization.user.validation.rootResetSelfOnly")),
      color: "warning",
    });
    return;
  }
  if (!row.userUuid) {
    toast.add({
      title: String(t("organization.user.toast.operationFailed")),
      description: String(t("organization.user.validation.missingUserId")),
      color: "error",
    });
    return;
  }
  resetPasswordTarget.value = row;
  resetPasswordForm.password = "";
  resetPasswordForm.confirmPassword = "";
  showResetPassword.value = true;
}

async function submitResetPassword() {
  const target = resetPasswordTarget.value;
  if (!target) return;
  if (!resetPasswordForm.password || resetPasswordForm.password.length < 6) {
    toast.add({
      title: String(t("organization.user.toast.validationFailed")),
      description: String(t("organization.user.validation.passwordMinLength")),
      color: "warning",
    });
    return;
  }
  if (resetPasswordForm.password !== resetPasswordForm.confirmPassword) {
    toast.add({
      title: String(t("organization.user.toast.validationFailed")),
      description: String(t("organization.user.validation.passwordMismatch")),
      color: "warning",
    });
    return;
  }
  try {
    await userService.resetUserPassword(target.userUuid || "", {
      new_password: resetPasswordForm.password,
    });
    showResetPassword.value = false;
    toast.add({
      title: String(t("organization.user.toast.passwordReset")),
      description: String(
        t("organization.user.toast.passwordResetDescription", {
          name: target.name,
        })
      ),
      color: "success",
    });
  } catch (e: any) {
    notifyError(
      String(t("organization.user.toast.resetPasswordFailed")),
      e,
      String(t("organization.user.toast.retryLater"))
    );
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
    loadTenantRoles();
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
      id: "roles",
      accessorKey: "roles",
      header: t("organization.user.form.role").toString(),
      cell: ({ row }: any) => {
        const u = row.original as RowUser;
        if (!u.roles || u.roles.length === 0) return "-";
        return h(
          "div",
          { class: "flex flex-wrap gap-1 max-w-[220px]" },
          u.roles.map((role) =>
            h(
              UBadge,
              {
                color: "success",
                variant: "subtle",
                size: "xs",
                class: "max-w-[160px] truncate",
                title: role,
              },
              () => role
            )
          )
        );
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
        const actions: any[] = [];
        if (canOperateRootUser(u)) {
          actions.push(
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
                variant: "ghost",
                color: "primary",
                icon: "i-heroicons-key",
                onClick: () => openResetPassword(u),
              },
              () => t("organization.user.resetPassword.action")
            )
          );
        }
        if (!u.isRoot) {
          actions.push(
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
                onClick: () => {
                  if (!u.userUuid) {
                    toast.add({
                      title: String(t("organization.user.toast.deleteFailed")),
                      description: String(
                        t("organization.user.validation.missingUserId")
                      ),
                      color: "error",
                    });
                    return;
                  }
                  deleteUser(u.userUuid);
                },
              },
              () => t("organization.common.delete")
            )
          );
        }
        return h("div", { class: "flex gap-2" }, actions);
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
  const { Member, User } = memberWithProfile as any;
  return {
    id: Member.uuid,
    userUuid: User?.uuid,
    name: Member.display_name || User?.display_name,
    username: Member.username,
    email: User?.email || "",
    phone: User?.phone || "",
    department: Member.meta?.title || Member.meta?.department || "",
    roles: null,
    status: Member.status === 1 ? "active" : "inactive",
    isRoot: Boolean(User?.is_root),
    avatar:
      Member.avatar_url ||
      User?.avatar_url ||
      `https://i.pravatar.cc/150?u=${encodeURIComponent(
        User?.email || Member.display_name
      )}`,
    meta: { ...(User?.meta || {}), ...(Member?.meta || {}) }, // 合并User和Member的meta
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
      tenant_uuid: props.tenantUuid,
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

    if (tenantRoleOptions.value.length === 0) {
      await loadTenantRoles();
    }
    const response = await userService.getUsers(params);

    if (response.data) {
      const rows = response.data.items.map(transformUserData);
      await hydrateRowsRoles(rows);
      const visibleRows = canViewRootUsers.value
        ? rows
        : rows.filter((row: RowUser) => !row.isRoot);
      users.value = visibleRows;
      pagination.total = visibleRows.length;
      pagination.totalPages = Math.max(
        1,
        Math.ceil(visibleRows.length / pagination.pageSize)
      );
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

  await loadTenantRoles();
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
            :items="roleFilterItems"
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
            autocomplete="off"
          >
            <UFormField :label="$t('organization.user.form.name')" required>
              <UInput
                v-model="userForm.name"
                name="px-user-display-name"
                autocomplete="off"
              />
            </UFormField>
            <UFormField
              :label="$t('organization.user.form.username')"
              required
            >
              <UInput
                v-model="userForm.username"
                :placeholder="$t('organization.user.form.usernamePlaceholder')"
                name="px-tenant-username"
                autocomplete="new-password"
              />
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                {{ $t("organization.user.form.usernameHint") }}
              </p>
            </UFormField>
            <UFormField
              :label="$t('organization.user.form.email')"
              required
              class="md:col-span-2"
            >
              <UInput
                v-model="userForm.email"
                type="email"
                name="px-user-email"
                autocomplete="off"
              />
            </UFormField>
            <UFormField :label="$t('organization.user.form.phone')">
              <UInput
                v-model="userForm.phone"
                type="tel"
                :placeholder="$t('organization.user.form.phonePlaceholder')"
                name="px-user-phone"
                autocomplete="new-password"
              />
            </UFormField>
            <UFormField :label="$t('organization.user.form.role')" required>
              <USelectMenu
                v-model="selectedRoleOptions"
                :items="tenantRoleOptions"
                multiple
                searchable
                label-key="label"
                :loading="loadingRoles"
                :placeholder="$t('organization.user.form.selectRole')"
                :search-input="{
                  placeholder: $t('organization.user.form.searchRole'),
                }"
                :portal="true"
                :content="roleSelectContent"
                :ui="roleSelectUi"
                class="w-full"
              />
            </UFormField>
            <UFormField
              v-if="!isEditing"
              :label="$t('organization.user.form.password')"
              :required="!isEditing"
              ><UInput
                v-model="userForm.password"
                type="password"
                name="px-user-initial-password"
                autocomplete="new-password"
            /></UFormField>
            <UFormField
              v-if="!isEditing"
              :label="$t('organization.user.form.confirmPassword')"
              :required="!isEditing"
              ><UInput
                v-model="userForm.confirmPassword"
                type="password"
                name="px-user-confirm-password"
                autocomplete="new-password"
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

    <UModal
      v-model:open="showResetPassword"
      :title="t('organization.user.resetPassword.title')"
      :description="
        t('organization.user.resetPassword.description', {
          name: resetPasswordTarget?.name || t('organization.user.resetPassword.defaultName'),
        })
      "
    >
      <template #content>
        <div class="py-6 px-6">
          <form @submit.prevent="submitResetPassword" class="space-y-4">
            <UFormField :label="t('organization.user.resetPassword.newPassword')" required>
              <UInput v-model="resetPasswordForm.password" type="password" />
            </UFormField>
            <UFormField :label="t('organization.user.resetPassword.confirmPassword')" required>
              <UInput
                v-model="resetPasswordForm.confirmPassword"
                type="password"
              />
            </UFormField>
            <div class="flex justify-end gap-3 pt-2">
              <UButton
                color="neutral"
                variant="outline"
                @click="showResetPassword = false"
              >
                {{ $t("organization.common.cancel") }}
              </UButton>
              <UButton type="submit" color="primary">
                {{ t("organization.user.resetPassword.submit") }}
              </UButton>
            </div>
          </form>
        </div>
      </template>
    </UModal>
  </div>
</template>
