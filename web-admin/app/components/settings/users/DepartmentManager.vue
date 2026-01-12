<script setup lang="ts">
import {
  ref,
  reactive,
  computed,
  h,
  resolveComponent,
  watch,
  onMounted,
} from "vue";
import { storeToRefs } from "pinia";
import { useI18n } from "#imports";
import {
  useDepartmentService,
  type Department,
  type DepartmentCreateParams,
  type DepartmentUpdateParams,
} from "~/composables/api/services/departmentService";
import { useDepartmentStore } from "~/stores/department";
import { useUserStore } from "~/stores/user";
import { useMemberService } from "~/composables/api/services/memberService";
import { useOneShotAlert } from "~/composables/useOneShotAlert";
import * as v from "valibot";
import type { FormSubmitEvent } from "@nuxt/ui";

import { normalizeApiError } from "~/composables/api/normalizeApiError";
const { notifyOnce, reset } = useOneShotAlert();

// 字段/表单错误位
const formError = ref<string | null>(null);
const fieldErrors = reactive<Record<string, string>>({});
const clearErrors = () => {
  formError.value = null;
  Object.keys(fieldErrors).forEach((k) => delete fieldErrors[k]);
};

/** ================== UI ================== */
const { t, locale } = useI18n();
const UButton = resolveComponent("UButton");

/** ================== 状态 ================== */
const deptService = useDepartmentService();
const userStore = useUserStore();
const tenantRecovering = ref(false);
const tenantRecoveryAttempted = ref(false);
const formSubmitting = ref(false);

const activeNodeId = ref<number | null>(null); // UTree 当前选中部门 id
const activeNode = computed(
  () => flat.value.find((d) => d.id === activeNodeId.value) || null
);

const memberService = useMemberService();
const members = ref<{ label: string; value: number }[]>([]);

const loadMembers = async () => {
  const list = await memberService.listAll(); // 你实际的 API
  members.value = list.map((m: any) => ({ label: m.name, value: m.id }));
};
onMounted(() => {
  ensureTenantAndTree().catch(() => {});
  loadMembers().catch(() => {});
});

const searchQuery = ref("");

/** 分页 */
const pagination = reactive({
  page: 1,
  pageSize: 10,
  total: 0,
  totalPages: 0,
});
const pageSizeOptions = [
  { label: "10", value: 10 },
  { label: "20", value: 20 },
  { label: "50", value: 50 },
  { label: "100", value: 100 },
];

/** 表单 & 弹窗 */
const showForm = ref(false);
const isEditing = ref(false);
const editingId = ref<number | null>(null);

const originalEditing = ref<
  | (Department & {
      key?: string | null;
      sort?: number | null;
      leader_member_id?: number | null;
      status?: number | null;
      meta?: any;
    })
  | null
>(null);

// 表单模型：新增 key / sort / leader_member_id / status / meta / new_parent_id
const departmentForm = reactive({
  name: "",
  key: "",
  parent_id: undefined as number | undefined, // 仅用于创建 or 选择父级
  new_parent_id: null as number | null, // 更新移动：不传=不移动，null=置空
  sort: undefined as number | undefined,
  leader_member_id: null as number | null,
  status: 1 as number, // 1启用 / 0停用（示例）
  metaText: "" as string, // 文本编辑区，保存时转 JSON
});

const resetForm = () => {
  departmentForm.name = "";
  departmentForm.key = "";
  departmentForm.parent_id = activeNodeId.value ?? undefined;
  departmentForm.new_parent_id = null;
  departmentForm.sort = undefined;
  departmentForm.leader_member_id = null;
  departmentForm.status = 1;
  departmentForm.metaText = "";
  isEditing.value = false;
  editingId.value = null;
  originalEditing.value = null;
};

const openAddForm = () => {
  resetForm();
  clearErrors();
  showForm.value = true;
};

const openEditForm = (dept: Department & any) => {
  // 这里的 dept 建议从 flat 里取全量（你 trailing 里已传 parent_id）
  const full = flat.value.find((d) => d.id === dept.id) || dept;
  departmentForm.name = full.name ?? "";
  departmentForm.key = full.key ?? "";
  departmentForm.parent_id = full.parent_id; // 仅用于展示；实际移动用 new_parent_id
  departmentForm.new_parent_id = null; // 默认不移动
  departmentForm.sort = full.sort;
  departmentForm.leader_member_id = full.leader_member_id ?? null;
  departmentForm.status = full.status ?? 1;
  departmentForm.metaText = full.meta ? JSON.stringify(full.meta, null, 2) : "";

  isEditing.value = true;
  editingId.value = full.id;
  originalEditing.value = JSON.parse(JSON.stringify(full));
  clearErrors();
  showForm.value = true;
};

const closeFormModal = () => {
  if (process.client) {
    (document.activeElement as HTMLElement | null)?.blur?.();
  }
  showForm.value = false;
  resetForm();
  clearErrors();
};

/** ================== 数据获取 & 工具 ================== */
// 使用全局Store
const deptStore = useDepartmentStore();
const {
  tree: storeTree,
  flat: storeFlat,
  status,
  error,
} = storeToRefs(deptStore);

// 计算属性来兼容现有代码
const tree = computed(() => storeTree.value);
const flat = computed(() => storeFlat.value);
const isLoadingTree = computed(() => status.value === "loading");
const loadError = computed(() => error.value);

const fetchTree = async ({ force = false }: { force?: boolean } = {}) => {
  try {
    await deptStore.fetchTree({ force });

    // 默认选择第一个根节点
    if (!activeNodeId.value) {
      const firstRoot = flat.value.find((d) => !d.parent_id);
      activeNodeId.value = firstRoot?.id ?? null;
    }
    selectedValue.value = activeNodeId.value
      ? [String(activeNodeId.value)]
      : [];
  } catch (e: any) {
    console.error("获取部门树失败:", e);
  }
};

/** UTree 数据 */
const treeItems = computed(() => tree.value.map((n) => toTreeItem(n)));

function toTreeItem(n: Department): any {
  const hasChildren = !!(n.children && n.children.length);
  return {
    // ✅ UTree 用 value 作为唯一标识（或 label）
    value: String(n.id),
    label: n.name,
    id: n.id, // 额外带上，方便右侧编辑删除
    hasChildren,
    children: hasChildren ? n.children!.map(toTreeItem) : undefined,
  };
}

const activeNodeActivePath = ref<string[]>([]);

// 当树数据加载完，初始化一次（保持和 activeNodeId 同步）
watch(
  () => activeNodeId.value,
  (id) => {
    activeNodeActivePath.value = id ? [String(id)] : [];
  },
  { immediate: true }
);

// 新增：选中值 & 展开集合（字符串数组）
const selectedValue = ref<string[]>([]);
const expandedValues = ref<string[]>([]);

// 同步：当选择变化时，更新 activeNodeId（右侧列表依赖它）
watch(selectedValue, (vals) => {
  const first = Array.isArray(vals) && vals.length ? vals[0] : null;
  activeNodeId.value = first ? Number(first) : null;
  pagination.page = 1;
});

const pickPreferredTenant = () => {
  const members = userStore.memberTenants || [];
  if (!members.length) return null;
  const byName = members.find((m) => /sme|system/i.test(m.tenant_name || ""));
  return byName?.tenant_uuid || members[0]?.tenant_uuid || null;
};

const ensureTenantAndTree = async () => {
  // db-refresh 后本地 tenant uuid/token 可能仍指向旧租户，导致部门树为空但不报错。
  if (!userStore.isLoggedIn) {
    try {
      await userStore.fetchUserContext({ force: true });
    } catch {
      // ignore
    }
  } else {
    try {
      await userStore.fetchUserContext({ force: true });
    } catch {
      // ignore
    }
  }

  await fetchTree({ force: true });

  if (treeItems.value.length > 0 || tenantRecoveryAttempted.value) {
    return;
  }
  tenantRecoveryAttempted.value = true;

  const preferred = pickPreferredTenant();
  if (!preferred) return;
  if (userStore.currentTenantUuid === preferred) return;

  tenantRecovering.value = true;
  try {
    await userStore.switchTenant(preferred);
    await fetchTree({ force: true });
  } finally {
    tenantRecovering.value = false;
  }
};

const onFormSubmit = async (
  _e: FormSubmitEvent<v.InferOutput<typeof schema>>
) => {
  reset();
  await saveDepartment(); // 仍然走你已经改造过的 saveDepartment（带 notifyOnce）
};

function flattenDepartments(nodes: Department[], result: Department[] = []) {
  for (const n of nodes) {
    result.push(n);
    if (n.children?.length) flattenDepartments(n.children, result);
  }
  return result;
}

/** 右侧表格：显示当前选中节点的“直接子部门”，并支持搜索+分页 */
const childrenOfActive = computed<Department[]>(() => {
  if (!activeNodeId.value) return [];
  const parent = flat.value.find((d) => d.id === activeNodeId.value);
  return parent?.children ?? [];
});

const filteredDepartments = computed(() => {
  const q = searchQuery.value.trim().toLowerCase();
  const list = childrenOfActive.value;
  return q
    ? list.filter((d) => (d.name ?? "").toLowerCase().includes(q))
    : list;
});

watch(
  [filteredDepartments, () => pagination.pageSize],
  () => {
    pagination.total = filteredDepartments.value.length;
    pagination.totalPages = Math.ceil(pagination.total / pagination.pageSize);
    if (pagination.page > pagination.totalPages)
      pagination.page = pagination.totalPages || 1;
  },
  { immediate: true }
);

const paginatedDepartments = computed(() => {
  const start = (pagination.page - 1) * pagination.pageSize;
  return filteredDepartments.value.slice(start, start + pagination.pageSize);
});

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

const changePage = (page: number) => {
  if (page >= 1 && page <= pagination.totalPages) pagination.page = page;
};
const changePageSize = (v: number | string) => {
  pagination.pageSize = Number(v);
  pagination.page = 1;
};
const hasNextPage = computed(() => pagination.page < pagination.totalPages);
const hasPrevPage = computed(() => pagination.page > 1);

/** 选择上级部门（下拉用） */
const parentOptions = computed(() => {
  const selfId = editingId.value;
  return [
    {
      label: t("organization.department.form.noParent") as string,
      value: null as any,
    },
    ...flat.value
      .filter((d) => d.id !== selfId) // 🚫 不能把自己选为上级
      .map((d) => ({ label: d.name, value: d.id })),
  ];
});

/** ================== CRUD（走后端） ================== */
const deleteDepartment = async (id: number) => {
  if (!confirm(t("organization.department.confirmDelete") as string)) return;
  try {
    await deptStore.deleteDepartment(id);
    if (activeNodeId.value === id) {
      const deleted = flat.value.find((d) => d.id === id);
      activeNodeId.value =
        deleted?.parent_id ?? flat.value.find((d) => !d.parent_id)?.id ?? null;
    }
    notifyOnce("部门删除成功", "", "success", "solid");
  } catch (e: any) {
    const { title, description } = normalizeApiError(e, { meta: "metaText" }); // ✨ 统一解析
    reset(); // ✨ 先重置一次 one-shot
    notifyOnce(title || "删除失败", description, "error", "solid"); // ✨ 弹全局 Alert（会在 Modal 之上）
  }
};

/** ============ TanStack 列定义（右侧“子部门列表”） ============ */
const columns = computed(() => {
  const _ = locale.value; // 语言切换依赖
  return [
    {
      id: "name",
      accessorKey: "name",
      header: t("organization.department.table.name"),
      cell: ({ row }: any) => {
        const d: Department = row.original;
        // 高亮当前选择的节点的直接子项名称
        return h("div", { class: "flex items-center gap-2" }, [
          h("span", d.name),
        ]);
      },
    },
    {
      id: "id",
      accessorKey: "id",
      header: "ID",
    },
    {
      id: "parent",
      header: t("organization.department.form.parent") || "上级部门",
      cell: ({ row }: any) => {
        const d: Department = row.original;
        const parentName = d.parent_id
          ? (flat.value.find((x) => x.id === d.parent_id)?.name ?? "-")
          : "-";
        return h("span", parentName);
      },
    },
    {
      id: "sort",
      accessorKey: "sort",
      header: t("organization.department.form.sort") || "排序",
    },
    {
      id: "leader",
      header: t("organization.department.form.leader") || "负责人",
      cell: ({ row }: any) => {
        const d: any = row.original;
        const leaderName =
          d.leader_name || d.leader?.name || d.leader_member_id || "-";
        return h("span", String(leaderName));
      },
    },
    {
      id: "actions",
      header: t("organization.department.table.actions"),
      enableSorting: false,
      cell: ({ row }: any) => {
        const d: Department = row.original;
        return h("div", { class: "flex gap-2" }, [
          h(
            UButton,
            {
              size: "xs",
              variant: "ghost",
              icon: "i-heroicons-chevron-up",
              onClick: async () => {
                const cur = (d.sort ?? 0) - 1;
                await deptService.updateDepartment(d.id, { sort: cur });
                await fetchTree({ force: true });
              },
            },
            { default: () => t("organization.common.up") }
          ),
          h(
            UButton,
            {
              size: "xs",
              variant: "ghost",
              icon: "i-heroicons-chevron-down",
              onClick: async () => {
                const cur = (d.sort ?? 0) + 1;
                await deptService.updateDepartment(d.id, { sort: cur });
                await fetchTree({ force: true });
              },
            },
            { default: () => t("organization.common.down") }
          ),
          h(
            UButton,
            {
              size: "xs",
              variant: "ghost",
              icon: "i-heroicons-pencil-square",
              onClick: () => openEditForm(d),
            },
            { default: () => t("organization.common.edit") }
          ),
          h(
            UButton,
            {
              size: "xs",
              color: "error",
              variant: "ghost",
              icon: "i-heroicons-trash",
              onClick: () => deleteDepartment(d.id),
            },
            { default: () => t("organization.common.delete") }
          ),
        ]);
      },
    },
  ];
});

/** UTree 选择 */
function onSelectNode(payload: any) {
  // 统一把各种形态归一到 string[]
  let arr: string[] = [];

  if (Array.isArray(payload)) {
    arr = payload.map(String);
  } else if (payload && typeof payload === "object") {
    if ("id" in payload) arr = [String((payload as any).id)];
    else if ("value" in payload) arr = [String((payload as any).value)];
  } else if (payload != null) {
    arr = [String(payload)];
  }

  selectedValue.value = arr;
  activeNodeId.value = arr.length ? Number(arr[0]) : null;
  activeNodeActivePath.value = selectedValue.value.slice(0, 1);
  pagination.page = 1;
}

const schema = v.object({
  name: v.pipe(v.string(), v.minLength(1, "部门名称为必填项")),
  // 允许为空/不选
  parent_id: v.nullable(v.optional(v.number())),
  // 仅字母/数字/下划线/短横线；可留空
  key: v.optional(
    v.pipe(
      v.string(),
      v.maxLength(64, "Key 最长 64 个字符"),
      v.regex(/^[A-Za-z0-9_-]*$/, "仅允许字母/数字/下划线/短横线")
    )
  ),
  // 可选；如果填了必须是 >=0 的整数
  sort: v.optional(
    v.pipe(
      v.number(),
      v.integer("排序必须是整数"),
      v.minValue(0, "排序不能为负数")
    )
  ),
  // 允许 null/不选
  leader_member_id: v.nullable(v.optional(v.number())),
  // 只能是 0 或 1
  status: v.union([v.literal(0), v.literal(1)], "状态不合法"),
  // 留空通过；非空必须是合法 JSON
  metaText: v.pipe(
    v.string(),
    v.check((s) => {
      if (!s?.trim()) return true;
      try {
        JSON.parse(s);
        return true;
      } catch {
        return false;
      }
    }, "Meta 必须是合法的 JSON")
  ),
  // 仅编辑时会用到；这里统一允许 null/不传
  new_parent_id: v.nullable(v.optional(v.number())),
});

const saveDepartment = async () => {
  if (formSubmitting.value) return;
  formSubmitting.value = true;
  let success = false; // 标记是否成功
  try {
    if (isEditing.value && editingId.value) {
      const payload = buildUpdatePayload();

      // 没有任何变化：不调接口，直接提示并返回
      if (Object.keys(payload).length === 0) {
        notifyOnce("无变更", "没有检测到修改内容", "warning", "solid");
        return;
      }

      // 如果你的 deptService 要求 meta 为对象，保持不变；若后端要字符串，可在这里 JSON.stringify
      const ok = await deptService.updateDepartment(editingId.value, payload);
      success = !!ok;
    } else {
      // 创建
      const created = await deptService.createDepartment({
        name: departmentForm.name,
        parent_id: departmentForm.parent_id,
        key: departmentForm.key || undefined,
        sort: departmentForm.sort,
        leader_member_id: departmentForm.leader_member_id ?? undefined,
        status: departmentForm.status,
        meta: departmentForm.metaText?.trim()
          ? JSON.parse(departmentForm.metaText)
          : undefined,
      } as DepartmentCreateParams);
      success = !!created;
    }
  } catch (e: any) {
    const { title, description } = normalizeApiError(e, { meta: "metaText" }); // ✨ 统一解析
    reset(); // ✨ 先重置一次 one-shot
    notifyOnce(title || "保存失败", description, "error", "solid"); // ✨ 弹全局 Alert（会在 Modal 之上）
  } finally {
    if (success) {
      reset(); // 允许成功提示出现
      notifyOnce("保存成功", "部门信息已成功保存", "success", "solid");
      showForm.value = false;
      await fetchTree({ force: true });
      resetForm();
    }
    formSubmitting.value = false;
  }
};

function buildUpdatePayload(): DepartmentUpdateParams {
  const orig = originalEditing.value || ({} as any);
  const payload: DepartmentUpdateParams = {};

  // name
  if (departmentForm.name !== orig.name) payload.name = departmentForm.name;

  // key
  if ((departmentForm.key ?? "") !== (orig.key ?? ""))
    payload.key = departmentForm.key || "";

  // new_parent_id：只有当你明确选择了（包含置空）才发送；默认不移动不传
  if (departmentForm.new_parent_id !== null) {
    payload.new_parent_id = departmentForm.new_parent_id;
  }

  // sort
  if (departmentForm.sort !== orig.sort) payload.sort = departmentForm.sort;

  // leader
  if (
    (departmentForm.leader_member_id ?? null) !==
    (orig.leader_member_id ?? null)
  ) {
    payload.leader_member_id = departmentForm.leader_member_id;
  }

  // status
  if ((departmentForm.status ?? null) !== (orig.status ?? null)) {
    payload.status = departmentForm.status;
  }

  // meta：由 metaText 解析
  if (
    departmentForm.metaText !== (orig.meta ? JSON.stringify(orig.meta) : "")
  ) {
    try {
      const parsed = departmentForm.metaText?.trim()
        ? JSON.parse(departmentForm.metaText)
        : null;
      payload.meta = parsed ?? null;
    } catch (e) {
      throw new Error("Meta JSON 非法，请检查 JSON 语法。");
    }
  }

  return payload;
}
</script>

<template>
  <div class="p-4">
    <div class="flex justify-between items-center mb-6">
      <div>
        <h2 class="text-xl font-semibold text-gray-800">
          {{ $t("organization.department.title") }}
        </h2>
        <p class="text-sm text-gray-500 mt-1">
          {{ $t("organization.department.description") }}
        </p>
      </div>
      <UButton color="primary" icon="i-heroicons-plus" @click="openAddForm">
        {{ $t("organization.department.add") }}
      </UButton>
    </div>

    <!-- 搜索 -->
    <UInput
      v-model="searchQuery"
      icon="i-heroicons-magnifying-glass"
      :placeholder="$t('organization.department.search')"
      class="w-full md:w-80 mb-4"
    />

    <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
      <!-- 左侧：组织树 -->
      <UCard class="col-span-1">
        <template #header>
          <div class="flex justify-between items-center">
            <h3 class="text-lg font-medium">
              {{ $t("organization.department.title") }}
            </h3>
            <UButton
              icon="i-heroicons-plus-circle"
              size="sm"
              color="gray"
              variant="ghost"
              @click="openAddForm"
            >
              {{ $t("organization.department.add") }}
            </UButton>
          </div>
        </template>

        <div v-if="isLoadingTree" class="flex justify-center py-4">
          <UIcon name="i-heroicons-arrow-path" class="animate-spin h-6 w-6" />
        </div>
        <div v-else-if="loadError" class="text-center text-red-500 py-4">
          {{ loadError }}
        </div>
        <div
          v-else-if="treeItems.length === 0"
          class="text-center py-4 text-gray-500"
        >
          {{ $t("organization.department.empty.title") }}
        </div>
        <div v-else class="department-tree">
          <UTree
            :items="treeItems"
            v-model="selectedValue"
            v-model:expanded="expandedValues"
            expanded-icon="i-heroicons-folder-open"
            collapsed-icon="i-heroicons-folder"
            @update:model-value="onSelectNode"
          >
            <!-- 左侧图标：三态明确区分 -->
            <template #item-leading="{ item, expanded }">
              <UIcon
                :name="
                  item.hasChildren
                    ? expanded
                      ? 'i-heroicons-folder-open'
                      : 'i-heroicons-folder'
                    : 'i-heroicons-document'
                "
                :class="[
                  'h-4 w-4',
                  item.hasChildren ? 'text-amber-500' : 'text-gray-400',
                ]"
              />
            </template>

            <template #item-label="{ item }">
              <span>{{ item.label }}</span>
            </template>

            <template #item-trailing="{ item }">
              <div class="flex space-x-1">
                <UButton
                  icon="i-heroicons-pencil"
                  size="xs"
                  color="gray"
                  variant="ghost"
                  @click.stop="
                    openEditForm({
                      id: Number(item.id),
                      name: item.label,
                      parent_id: flat.find((x) => x.id === Number(item.id))
                        ?.parent_id,
                    } as any)
                  "
                />
                <UButton
                  icon="i-heroicons-trash"
                  size="xs"
                  color="error"
                  variant="ghost"
                  @click.stop="deleteDepartment(Number(item.id))"
                />
              </div>
            </template>
          </UTree>
        </div>
      </UCard>

      <!-- 右侧：选中节点的直接子部门列表（表格） -->
      <UCard class="col-span-1 md:col-span-2">
        <template #header>
          <div class="flex justify-between items-center">
            <h3 class="text-lg font-medium">
              {{
                activeNode
                  ? `${activeNode.name} - ${$t("organization.department.title")}`
                  : $t("organization.department.title")
              }}
            </h3>
            <div class="flex items-center gap-2">
              <span class="text-sm text-gray-600">{{
                $t("organization.department.table.name")
              }}</span>
            </div>
          </div>
        </template>

        <div class="bg-white rounded-lg">
          <UTable
            :data="paginatedDepartments"
            :columns="columns"
            :row-key="(row) => row.id"
          />

          <div
            v-if="pagination.totalPages > 1"
            class="px-6 py-4 border-t border-gray-200"
          >
            <div class="flex justify-between items-center">
              <div class="text-sm text-gray-600">
                第 {{ pagination.page }} /
                {{ pagination.totalPages }} 页；本级子部门
                {{ pagination.total }} 个
              </div>
              <div class="flex items-center gap-4">
                <div class="flex items-center gap-2">
                  <span class="text-sm text-gray-600">每页：</span>
                  <USelect
                    :model-value="pagination.pageSize"
                    :items="pageSizeOptions"
                    option-attribute="label"
                    value-attribute="value"
                    @update:model-value="changePageSize"
                    class="w-20"
                  />
                </div>
                <div class="flex gap-2">
                  <UButton
                    :disabled="!hasPrevPage"
                    variant="outline"
                    size="sm"
                    icon="i-heroicons-chevron-left"
                    @click="changePage(pagination.page - 1)"
                    >上一页</UButton
                  >
                  <UButton
                    :disabled="!hasNextPage"
                    variant="outline"
                    size="sm"
                    icon="i-heroicons-chevron-right"
                    @click="changePage(pagination.page + 1)"
                    >下一页</UButton
                  >
                </div>
              </div>
            </div>
          </div>
        </div>

        <template #footer>
          <div class="flex justify-between items-center text-sm text-gray-500">
            <span>
              {{
                t("organization.department.pagination.showing", {
                  start: paginationInfo.start,
                  end: paginationInfo.end,
                  total: paginationInfo.total,
                })
              }}
            </span>
          </div>
        </template>
      </UCard>
    </div>

    <!-- 空状态（针对右侧列表） -->
    <div
      v-if="!isLoadingTree && filteredDepartments.length === 0"
      class="text-center py-10 text-gray-500"
    >
      {{ $t("organization.department.empty.noResults") }}
    </div>

    <!-- 表单 -->
    <UModal
      v-model:open="showForm"
      :title="isEditing ? $t('organization.department.edit') : $t('organization.department.add')"
      :description="$t('organization.department.modalDesc', '新增或编辑部门信息')"
      :ui="{ width: 'max-w-3xl w-full', body: 'p-4 sm:p-5', footer: 'justify-end' }"
      :close="{ onClick: closeFormModal }"
      prevent-close
    >
      <template #body>
        <UForm
          id="department-form"
          :schema="schema"
          :state="departmentForm"
          class="space-y-4"
          @submit="onFormSubmit"
        >
          <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
            <UFormField
              name="name"
              :label="$t('organization.department.form.name')"
              required
            >
              <UInput v-model="departmentForm.name" class="w-full" />
            </UFormField>

            <UFormField
              name="parent_id"
              :label="$t('organization.department.form.parent')"
            >
              <USelect
                :model-value="departmentForm.parent_id"
                :items="parentOptions"
                option-attribute="label"
                value-attribute="value"
                class="w-full"
                :placeholder="$t('organization.department.form.noParent')"
                @update:model-value="
                  (v) =>
                    (departmentForm.parent_id =
                      v === undefined || v === null || v === ''
                        ? undefined
                        : Number(v))
                "
              />
            </UFormField>

            <UFormField
              name="key"
              :label="$t('organization.department.form.key') || '唯一键 Key'"
            >
              <UInput
                v-model="departmentForm.key"
                class="w-full"
                placeholder="英文/短横线/下划线"
              />
            </UFormField>

            <UFormField
              v-if="isEditing"
              name="new_parent_id"
              :label="
                $t('organization.department.form.moveParent') ||
                '移动到新上级'
              "
            >
              <USelect
                :model-value="departmentForm.new_parent_id"
                :items="[
                  {
                    label: $t('organization.department.form.noParent'),
                    value: null,
                  },
                  ...parentOptions,
                ]"
                option-attribute="label"
                value-attribute="value"
                class="w-full"
                :placeholder="$t('organization.department.form.noParent')"
                @update:model-value="
                  (v) =>
                    (departmentForm.new_parent_id =
                      v === '' ? null : v === null ? null : Number(v))
                "
              />
              <p class="text-xs text-gray-500 mt-1">
                不选择则不移动；选择“无上级”将把部门提升为根节点。
              </p>
            </UFormField>

            <UFormField
              name="sort"
              :label="$t('organization.department.form.sort') || '排序'"
            >
              <UInput
                type="number"
                :min="0"
                v-model.number="departmentForm.sort"
                class="w-full"
                placeholder="数字越小越靠前"
              />
            </UFormField>

            <UFormField
              name="leader_member_id"
              :label="
                $t('organization.department.form.leader') || '部门负责人'
              "
            >
              <USelect
                :model-value="departmentForm.leader_member_id"
                :items="[
                  {
                    label: $t('organization.common.none') || '无',
                    value: null,
                  },
                  ...members,
                ]"
                option-attribute="label"
                value-attribute="value"
                class="w-full"
                @update:model-value="
                  (v) =>
                    (departmentForm.leader_member_id =
                      v === '' ? null : v === null ? null : Number(v))
                "
              />
            </UFormField>

            <UFormField
              name="status"
              :label="$t('organization.department.form.status') || '状态'"
            >
              <URadioGroup
                v-model="departmentForm.status"
                :items="[
                  {
                    label: $t('organization.common.enabled') || '启用',
                    value: 1,
                  },
                  {
                    label: $t('organization.common.disabled') || '停用',
                    value: 0,
                  },
                ]"
              />
            </UFormField>

            <UFormField
              name="metaText"
              :label="
                $t('organization.department.form.meta') || '扩展 Meta(JSON)'
              "
              help="留空表示不修改；清空并保存表示置空。"
              class="md:col-span-2"
            >
              <UTextarea
                v-model="departmentForm.metaText"
                :rows="6"
                placeholder='{"color":"#fff","bizTag":"x"}'
              />
            </UFormField>
          </div>
        </UForm>
      </template>

      <template #footer>
        <div class="flex justify-end gap-2 w-full">
          <UButton
            color="neutral"
            variant="subtle"
            type="button"
            :disabled="formSubmitting"
            @click="closeFormModal"
          >
            {{ $t("organization.common.cancel") }}
          </UButton>
          <UButton
            color="primary"
            type="submit"
            form="department-form"
            :loading="formSubmitting"
          >
            {{ $t("organization.common.save") }}
          </UButton>
        </div>
      </template>
    </UModal>
  </div>
</template>

<style>
.department-tree :deep(.u-tree-node) {
  padding-top: 0.25rem;
  padding-bottom: 0.25rem;
}
.department-tree :deep(.u-tree-node-content) {
  padding: 0.25rem 0.5rem;
  border-radius: 0.375rem;
}
.department-tree :deep(.u-tree-node-content:hover) {
  background-color: #f3f4f6;
}
.dark .department-tree :deep(.u-tree-node-content:hover) {
  background-color: #1f2937;
}
.department-tree :deep(.u-tree-node-selected) {
  background-color: rgba(var(--color-primary-500), 0.1);
}
.dark .department-tree :deep(.u-tree-node-selected) {
  background-color: rgba(var(--color-primary-500), 0.05);
}
</style>
