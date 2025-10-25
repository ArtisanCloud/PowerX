<!-- /components/settings/users/PermissionRoot.vue -->
<script setup lang="ts">
import { ref, computed, onMounted, watch } from "vue";
import { storeToRefs } from "pinia";
import { usePermissionStore } from "~/stores/permission";
import { useUserStore } from "~/stores/user";
import type { TableColumn } from "@nuxt/ui";
import type { FormError, FormSubmitEvent } from "@nuxt/ui";
import { useCopy } from "~/composables/useCopy";
import {
  statusText,
  statusColor,
} from "~/composables/api/services/PermissionService";

const { copy } = useCopy({ showToast: true });
const onCopy = (text: string) => copy(text);

// ---- Stores ----
const permissionStore = usePermissionStore();
const { listData, isLoading, error } = storeToRefs(permissionStore);
const userStore = useUserStore();

// ---- 映射后端字段为前端统一模型 ----
const permissions = computed(() => {
  const raw = Array.isArray(listData.value?.items) ? listData.value.items : [];
  return raw.map((p: any) => {
    const code = `${p?.resource ?? ""}.${p?.action ?? ""}`.replace(/^\./, "");
    const codeFull =
      `${p?.plugin ?? "core"}:${p?.resource ?? ""}:${p?.action ?? ""}`.replace(
        /:+$/,
        ""
      );

    // 生命周期：顶层 status 优先，回退 meta.status
    const lifecycle = String(
      p?.status ?? p?.meta?.status ?? "active"
    ).toLowerCase();

    // 启用：后端布尔优先，否则按 lifecycle 推断
    const enabled =
      typeof p?.enabled === "boolean"
        ? p.enabled
        : ["active", "beta", "preview"].includes(lifecycle);

    const category =
      p?.meta?.module ??
      (typeof p?.resource === "string" ? p.resource.split(".")[0] : "") ??
      p?.plugin ??
      "";

    return {
      id: p?.id,
      name: p?.meta?.label ?? code,
      code,
      codeFull,
      description: p?.description ?? "",
      category,
      enabled,
      lifecycle,
      effect: p?.effect ?? "allow",
      deprecated_at: p?.deprecated_at ?? p?.meta?.deprecated_at ?? null,
      created_at: p?.createdAt ?? p?.created_at ?? null,
      tags: [p?.meta?.type, p?.meta?.http_method].filter(Boolean),
      __raw: p, // 保留原始
    };
  });
});

// ---- 分类选项 ----
const categoryOptions = computed(() => {
  if (!permissions.value || !Array.isArray(permissions.value)) {
    return [{ label: "（自定义…）", value: "__custom__" }];
  }
  const base = Array.from(
    new Set(permissions.value.map((p) => p?.category).filter(Boolean))
  ).map((c) => ({ label: c as string, value: c as string }));
  return [...base, { label: "（自定义…）", value: "__custom__" }];
});

// ---- 校验 & 工具 ----
const codePattern =
  /^[a-z][a-z0-9._:-]*\.[a-z][a-z0-9._:-]*$|^[a-z][a-z0-9._:-]*:[a-z][a-z0-9._:-]*$/;

const isCodeUnique = (code: string, ignoreId?: string) => {
  if (!permissions.value || !Array.isArray(permissions.value)) return true;
  const target = (code || "").trim().toLowerCase();
  return !permissions.value.some(
    (p) => p?.code?.toLowerCase() === target && p?.id !== ignoreId
  );
};

function validatePermission(
  state: any,
  { isEdit = false, ignoreId }: { isEdit?: boolean; ignoreId?: string } = {}
): FormError[] {
  const errors: FormError[] = [];
  const add = (path: string, message: string) => errors.push({ path, message });

  if (!state.name || String(state.name).trim().length < 2)
    add("name", "名称至少 2 个字符");
  else if (String(state.name).trim().length > 50)
    add("name", "名称不应超过 50 字");

  if (!state.code || String(state.code).trim().length < 3)
    add("code", "代码至少 3 个字符");
  else if (String(state.code).trim().length > 100)
    add("code", "代码不应超过 100 字");
  else if (!codePattern.test(String(state.code).trim()))
    add("code", "建议形如 module.action 或 module:action（小写/数字/.-_ :）");
  else if (!isCodeUnique(state.code, ignoreId))
    add("code", "代码已存在，请更换");

  if (state.category === "__custom__") {
    if (!state.customCategory || !String(state.customCategory).trim()) {
      add("customCategory", "请输入自定义分类");
    }
  }

  const sortNum = Number(state.sort);
  if (!Number.isInteger(sortNum) || sortNum < 0)
    add("sort", "排序必须是大于等于 0 的整数");
  else if (sortNum > 9999) add("sort", "排序不应超过 9999");

  if (state.description && String(state.description).length > 500)
    add("description", "描述不应超过 500 字");

  if (!["low", "medium", "high"].includes(state.riskLevel))
    add("riskLevel", "请选择风险等级");

  if (!["system", "tenant"].includes(state.scope)) add("scope", "请选择作用域");

  return errors;
}

// 仅编辑模式：只校验描述和生命周期
function validateEditOnly(state: any): FormError[] {
  const errors: FormError[] = [];
  const add = (path: string, message: string) => errors.push({ path, message });
  if (state.description && String(state.description).length > 500)
    add("description", "描述不应超过 500 字");
  if (
    !["active", "beta", "preview", "deprecated", "disabled"].includes(
      String(state.lifecycle)
    )
  )
    add("lifecycle", "请选择有效状态");
  return errors;
}

// 只在创建时发送 code，编辑时不发
const toPayload = (state: any, { includeCode = true } = {}) => {
  const category =
    state.category === "__custom__"
      ? String(state.customCategory || "").trim()
      : String(state.category || "").trim();

  const tags = String(state.tagsInput || "")
    .split(",")
    .map((s: string) => s.trim())
    .filter(Boolean)
    .slice(0, 10);

  const payload: any = {
    name: String(state.name || "").trim(),
    description: String(state.description || "").trim(),
    category,
    enabled: Boolean(state.enabled),
    scope: state.scope,
    risk_level: state.riskLevel,
    sort: Number(state.sort ?? 100),
    tags,
  };

  if (includeCode) {
    payload.code = String(state.code || "").trim();
  }
  return payload;
};

// 仅用于“编辑”的 payload：只提交 description 和 status（顶层）
const toUpdatePayload = (state: any) => {
  return {
    description: String(state.description || "").trim(),
    status: String(state.lifecycle || "active").toLowerCase(),
  };
};

// ---- 表单状态 ----
const newPermissionForm = ref({
  name: "",
  code: "",
  description: "",
  category: "",
  customCategory: "",
  enabled: true,
  scope: "system",
  riskLevel: "low",
  sort: 100,
  tagsInput: "",
});

const editingForm = ref<any | null>(null);
const editingPermission = ref<any | null>(null);

// 状态下拉项（使用服务里的 statusText 作为文案）
const STATUS_OPTIONS = [
  { label: statusText("active"), value: "active" },
  { label: statusText("beta"), value: "beta" },
  { label: statusText("preview"), value: "preview" },
  { label: statusText("deprecated"), value: "deprecated" },
  { label: statusText("disabled"), value: "disabled" },
];

// ---- 提交 ----
const onCreateSubmit = async (e: FormSubmitEvent<any>) => {
  const errs = validatePermission(newPermissionForm.value);
  if (errs.length) return;
  try {
    await permissionStore.createPermission(
      toPayload(newPermissionForm.value, { includeCode: true })
    );
    isCreateModalOpen.value = false;
    Object.assign(newPermissionForm.value, {
      name: "",
      code: "",
      description: "",
      category: "",
      customCategory: "",
      enabled: true,
      scope: "system",
      riskLevel: "low",
      sort: 100,
      tagsInput: "",
    });
    await permissionStore.fetchList();
  } catch (error) {
    console.error("创建权限失败:", error);
  }
};

const onEditSubmit = async (e: FormSubmitEvent<any>) => {
  if (!editingPermission.value?.id) return;
  const errs = validateEditOnly(editingForm.value);
  if (errs.length) return;

  try {
    await permissionStore.updatePermission(
      editingPermission.value.id,
      toUpdatePayload(editingForm.value)
    );
    isEditModalOpen.value = false;
    editingPermission.value = null;
    editingForm.value = null;
    await permissionStore.fetchList();
  } catch (error) {
    console.error("更新权限失败:", error);
  }
};

const editPermission = (permission: any) => {
  editingPermission.value = { ...permission };
  const categoryExists =
    categoryOptions.value &&
    categoryOptions.value.some((o) => o.value === permission?.category);

  editingForm.value = {
    name: permission?.name || "",
    code: permission?.code || "",
    description: permission?.description || "",
    category: categoryExists ? permission?.category : "__custom__",
    customCategory: categoryExists ? "" : permission?.category || "",
    enabled: permission?.enabled ?? true,
    lifecycle: permission?.lifecycle ?? "active",
    scope: permission?.scope || "system",
    riskLevel: permission?.risk_level || "low",
    sort: typeof permission?.sort === "number" ? permission.sort : 100,
    tagsInput: Array.isArray(permission?.tags)
      ? permission.tags.join(", ")
      : "",
  };
  isEditModalOpen.value = true;
};

// ---- 列定义（v3）----
type Row = {
  id: number | string;
  name: string;
  code: string;
  codeFull: string;
  description: string;
  category: string;
  created_at: string | null;

  lifecycle?: string;
  enabled?: boolean;
  effect?: "allow" | "deny" | string;
  deprecated_at?: number | string | null;
};

const columns: TableColumn<Row>[] = [
  { accessorKey: "codeFull", header: "标识" }, // 组合标识主列
  { accessorKey: "description", header: "描述" },
  { accessorKey: "lifecycle", header: "状态" },
  { accessorKey: "category", header: "分类" },
  { accessorKey: "created_at", header: "创建时间" },
  { id: "actions", header: "操作", enableSorting: false },
];

// ---- 搜索/过滤 ----
const searchQuery = ref<string | null>(null);
const selectedCategory = ref<string | null>(null);

const categories = computed(() => {
  if (!permissions.value || !Array.isArray(permissions.value)) return [];
  const cats = new Set(
    permissions.value
      .map((p) => p?.category)
      .filter((v) => v != null && v !== "")
  );
  return Array.from(cats);
});

const filteredPermissions = computed<Row[]>(() => {
  if (!permissions.value || !Array.isArray(permissions.value)) return [];
  let filtered = permissions.value;

  if (searchQuery.value) {
    const query = String(searchQuery.value || "").toLowerCase();
    filtered = filtered.filter((p) => {
      const name = String(p?.name || "").toLowerCase();
      const code = String(p?.code || "").toLowerCase();
      const codeFull = String((p as any)?.codeFull || "").toLowerCase();
      const desc = String(p?.description || "").toLowerCase();
      return (
        name.includes(query) ||
        code.includes(query) ||
        codeFull.includes(query) ||
        desc.includes(query)
      );
    });
  }

  if (selectedCategory.value) {
    filtered = filtered.filter((p) => p?.category === selectedCategory.value);
  }

  return filtered as Row[];
});

// ---- 分页（前端）----
const page = ref(1);
const pageSize = ref<number | string>(10);

const numericPageSize = computed(() => {
  const v: any = pageSize.value as any;
  if (typeof v === "object" && v !== null && "value" in v)
    return Number(v.value) || 10;
  return Number(v) || 10;
});

const total = computed(() => filteredPermissions.value.length);
const pagedPermissions = computed<Row[]>(() => {
  const start = (page.value - 1) * numericPageSize.value;
  const end = start + numericPageSize.value;
  return filteredPermissions.value.slice(start, end);
});

watch([filteredPermissions, numericPageSize], () => {
  page.value = 1;
});

// ---- 生命周期 ----
const isCreateModalOpen = ref(false);
const isEditModalOpen = ref(false);

onMounted(async () => {
  await permissionStore.fetchList();
});

// ---- 日期格式（YYYY-MM-DD）----（自动识别秒/毫秒）
const formatDate = (value?: string | number | Date) => {
  if (!value) return "-";
  let t: number | Date = value as any;
  if (typeof t === "string" && /^\d+$/.test(t)) t = Number(t);
  if (typeof t === "number" && t < 1e12) t = t * 1000; // 秒 → 毫秒
  const d = new Date(t);
  if (isNaN(d.getTime())) return "-";
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
};

// ---- 删除 ----
const deletePermission = async (permission: any) => {
  if (confirm(`确定要删除权限 "${permission?.name || ""}" 吗？`)) {
    try {
      await permissionStore.deletePermission(permission.id);
      await permissionStore.fetchList();
    } catch (error) {
      console.error("删除权限失败:", error);
    }
  }
};
</script>

<template>
  <div class="space-y-6">
    <!-- 标题 -->
    <div class="flex justify-between items-center">
      <div>
        <h1 class="text-2xl font-bold text-gray-900">权限管理</h1>
        <p class="text-gray-600 mt-1">管理系统权限和访问控制</p>
      </div>
      <UButton
        icon="i-heroicons-plus"
        color="primary"
        @click="isCreateModalOpen = true"
      >
        新增权限
      </UButton>
    </div>

    <!-- 搜索与过滤 -->
    <div class="flex gap-4">
      <UInput
        v-model="searchQuery"
        placeholder="搜索名称、代码、标识或描述..."
        icon="i-heroicons-magnifying-glass"
        class="flex-1"
      />
      <USelect
        v-model="selectedCategory"
        :items="[
          { label: '全部分类', value: null },
          ...categories.map((c) => ({ label: c, value: c })),
        ]"
        placeholder="全部分类"
        class="w-48"
      />
    </div>

    <!-- 列表卡片 -->
    <UCard>
      <template #header>
        <div class="flex flex-wrap gap-3 justify-between items-center">
          <h3 class="text-lg font-semibold">权限列表</h3>
          <div class="flex items-center gap-3">
            <span class="text-sm text-gray-500">共 {{ total }} 个</span>
            <USelect
              v-model="pageSize"
              :items="[
                { label: '每页 10 条', value: 10 },
                { label: '每页 20 条', value: 20 },
                { label: '每页 50 条', value: 50 },
              ]"
              class="w-32"
            />
          </div>
        </div>
      </template>

      <UTable
        :data="pagedPermissions"
        :columns="columns"
        :loading="isLoading"
        :meta="{ class: { tr: 'align-middle' } }"
        empty="暂无权限数据"
      >
        <!-- 主列：组合标识 -->
        <template #codeFull-cell="{ row }">
          <div class="flex flex-col gap-1">
            <!-- 上：Badge 拼接 -->
            <div class="flex items-center gap-1 flex-wrap">
              <UBadge variant="outline" size="sm">
                {{ row.original.__raw?.plugin ?? "core" }}
              </UBadge>
              <span class="text-gray-400">/</span>
              <UBadge variant="soft" size="sm" class="max-w-[160px] truncate">
                {{ row.original.__raw?.resource }}
              </UBadge>
              <span class="text-gray-400">/</span>
              <UBadge variant="soft" size="sm">
                {{ row.original.__raw?.action }}
              </UBadge>
            </div>

            <!-- 下：等宽 codeFull + 复制按钮 -->
            <div class="flex items-center gap-2">
              <UTooltip :text="row.original.codeFull">
                <code
                  class="text-xs bg-gray-50 rounded px-1 py-0.5 truncate max-w-[260px]"
                >
                  {{ row.original.codeFull }}
                </code>
              </UTooltip>
              <UButton
                size="2xs"
                variant="ghost"
                icon="i-heroicons-clipboard"
                @click="onCopy(row.original.codeFull)"
              />
            </div>

            <!-- （可选）API 副行：方法 + endpoint -->
            <div
              v-if="
                row.original.__raw?.meta?.http_method ||
                row.original.__raw?.meta?.api_endpoint
              "
              class="flex items-center gap-2"
            >
              <UBadge
                size="xs"
                :color="
                  row.original.__raw?.meta?.http_method === 'GET'
                    ? 'primary'
                    : 'secondary'
                "
              >
                {{ row.original.__raw?.meta?.http_method }}
              </UBadge>
              <UTooltip :text="row.original.__raw?.meta?.api_endpoint">
                <span class="text-xs text-gray-600 truncate max-w-[260px]">
                  {{ row.original.__raw?.meta?.api_endpoint }}
                </span>
              </UTooltip>
            </div>
          </div>
        </template>

        <!-- 描述 -->
        <template #description-cell="{ row }">
          <span class="text-gray-600">
            {{ row.original.description || "-" }}
          </span>
        </template>

        <!-- 分类 -->
        <template #category-cell="{ row }">
          <UBadge v-if="row.original.category" variant="soft">
            {{ row.original.category }}
          </UBadge>
          <span v-else class="text-gray-400">-</span>
        </template>

        <!-- 状态列：生命周期 +（可选）停用 -->
        <template #lifecycle-cell="{ row }">
          <div class="flex items-center gap-2 flex-wrap">
            <UTooltip
              :text="
                row.original.lifecycle === 'deprecated' &&
                row.original.deprecated_at
                  ? `废弃于：${formatDate(row.original.deprecated_at)}`
                  : undefined
              "
            >
              <UBadge
                :color="statusColor(row.original.lifecycle)"
                size="xs"
                variant="soft"
              >
                {{ statusText(row.original.lifecycle) }}
              </UBadge>
            </UTooltip>

            <UBadge
              v-if="row.original.enabled === false"
              color="neutral"
              size="xs"
              variant="outline"
            >
              已停用
            </UBadge>
          </div>
        </template>

        <!-- 创建时间 -->
        <template #created_at-cell="{ row }">
          <span class="text-sm text-gray-500">
            {{ formatDate(row.original.created_at) }}
          </span>
        </template>

        <!-- 操作 -->
        <template #actions-cell="{ row }">
          <div class="flex gap-2">
            <UButton
              icon="i-heroicons-pencil"
              size="sm"
              color="gray"
              variant="ghost"
              @click="editPermission(row.original)"
            />
            <!-- 如需删除，打开下面这段 -->
            <!--
            <UButton
              icon="i-heroicons-trash"
              size="sm"
              color="error"
              variant="ghost"
              @click="deletePermission(row.original)"
            />
            -->
          </div>
        </template>
      </UTable>

      <!-- 分页 -->
      <div class="flex justify-center pt-4" v-if="total > numericPageSize">
        <UPagination
          v-model:page="page"
          :total="total"
          :items-per-page="numericPageSize"
          show-edges
        />
      </div>
    </UCard>

    <!-- 新增权限 -->
    <UModal
      title="新增权限"
      description="新增一个权限"
      v-model:open="isCreateModalOpen"
    >
      <template #content>
        <UCard>
          <template #header>
            <h3 class="text-lg font-semibold">新增权限</h3>
          </template>

          <UForm
            :state="newPermissionForm"
            :validate="(state) => validatePermission(state)"
            @submit="onCreateSubmit"
            class="space-y-4"
          >
            <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
              <UFormField label="权限名称" name="name" required>
                <UInput
                  v-model="newPermissionForm.name"
                  placeholder="例如：用户创建"
                />
              </UFormField>

              <UFormField
                label="权限代码"
                name="code"
                required
                help="建议 module.action 或 module:action"
              >
                <UInput
                  v-model="newPermissionForm.code"
                  placeholder="例如：iam.permission:create"
                />
              </UFormField>

              <UFormField label="分类" name="category">
                <USelect
                  v-model="newPermissionForm.category"
                  :items="[
                    { label: '请选择分类', value: null },
                    ...categoryOptions,
                  ]"
                  placeholder="请选择分类"
                />
              </UFormField>

              <UFormField
                v-if="newPermissionForm.category === '__custom__'"
                label="自定义分类"
                name="customCategory"
                required
              >
                <UInput
                  v-model="newPermissionForm.customCategory"
                  placeholder="输入自定义分类"
                />
              </UFormField>

              <UFormField label="启用" name="enabled">
                <USwitch v-model="newPermissionForm.enabled" />
              </UFormField>

              <UFormField label="风险等级" name="riskLevel">
                <URadioGroup
                  v-model="newPermissionForm.riskLevel"
                  :options="[
                    { label: '低', value: 'low' },
                    { label: '中', value: 'medium' },
                    { label: '高', value: 'high' },
                  ]"
                />
              </UFormField>

              <UFormField label="作用域" name="scope">
                <URadioGroup
                  v-model="newPermissionForm.scope"
                  :options="[
                    { label: '系统级', value: 'system' },
                    { label: '租户级', value: 'tenant' },
                  ]"
                />
              </UFormField>

              <UFormField
                label="排序权重"
                name="sort"
                help="数值越小优先级越高"
              >
                <UInput
                  v-model.number="newPermissionForm.sort"
                  type="number"
                  min="0"
                  max="9999"
                />
              </UFormField>

              <UFormField
                label="标签"
                name="tagsInput"
                help="以逗号分隔，如：用户, 管理"
              >
                <UInput
                  v-model="newPermissionForm.tagsInput"
                  placeholder="例如：用户, 管理"
                />
              </UFormField>
            </div>

            <UFormField label="描述" name="description">
              <UTextarea
                v-model="newPermissionForm.description"
                placeholder="输入权限描述（可选）"
              />
            </UFormField>

            <div class="flex justify-end gap-3 pt-2">
              <UButton
                color="gray"
                variant="ghost"
                @click="isCreateModalOpen = false"
                >取消</UButton
              >
              <UButton color="primary" type="submit">创建</UButton>
            </div>
          </UForm>
        </UCard>
      </template>
    </UModal>

    <!-- 编辑权限：仅可改 状态+描述 -->
    <UModal
      title="编辑权限"
      description="编辑权限"
      v-model:open="isEditModalOpen"
    >
      <template #content>
        <UCard v-if="editingForm">
          <template #header>
            <h3 class="text-lg font-semibold">编辑权限</h3>
          </template>

          <UForm
            :state="editingForm"
            :validate="validateEditOnly"
            @submit="onEditSubmit"
            class="space-y-4"
          >
            <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
              <!-- 名称：只读 -->
              <UFormField label="权限名称" name="name">
                <UInput v-model="editingForm.name" disabled />
              </UFormField>

              <!-- 代码：只读展示 + 复制 -->
              <UFormField label="权限代码" name="code" help="创建后不可修改">
                <div class="flex items-center gap-2">
                  <code
                    class="px-2 py-1 bg-gray-100 rounded text-sm truncate max-w-[320px]"
                  >
                    {{ editingForm.code }}
                  </code>
                  <UButton
                    size="2xs"
                    variant="ghost"
                    icon="i-heroicons-clipboard"
                    @click="onCopy(editingForm.code)"
                  />
                </div>
              </UFormField>

              <!-- 分类：只读 -->
              <UFormField label="分类" name="category">
                <USelect
                  v-model="editingForm.category"
                  :items="[
                    {
                      label: editingForm.category || '-',
                      value: editingForm.category,
                    },
                  ]"
                  disabled
                />
              </UFormField>

              <!-- 自定义分类：只读（仅在是自定义时显示一行只读输入） -->
              <UFormField
                v-if="editingForm.category === '__custom__'"
                label="自定义分类"
                name="customCategory"
              >
                <UInput v-model="editingForm.customCategory" disabled />
              </UFormField>

              <!-- ✅ 状态（可编辑） -->
              <UFormField label="状态" name="lifecycle">
                <USelect
                  v-model="editingForm.lifecycle"
                  :items="STATUS_OPTIONS"
                />
              </UFormField>

              <!-- 风险等级：只读 -->
              <UFormField label="风险等级" name="riskLevel">
                <URadioGroup
                  v-model="editingForm.riskLevel"
                  :options="[
                    { label: '低', value: 'low' },
                    { label: '中', value: 'medium' },
                    { label: '高', value: 'high' },
                  ]"
                  disabled
                />
              </UFormField>

              <!-- 作用域：只读 -->
              <UFormField label="作用域" name="scope">
                <URadioGroup
                  v-model="editingForm.scope"
                  :options="[
                    { label: '系统级', value: 'system' },
                    { label: '租户级', value: 'tenant' },
                  ]"
                  disabled
                />
              </UFormField>

              <!-- 排序：只读 -->
              <UFormField
                label="排序权重"
                name="sort"
                help="数值越小优先级越高"
              >
                <UInput
                  v-model.number="editingForm.sort"
                  type="number"
                  min="0"
                  max="9999"
                  disabled
                />
              </UFormField>

              <!-- 标签：只读 -->
              <UFormField label="标签" name="tagsInput">
                <UInput v-model="editingForm.tagsInput" disabled />
              </UFormField>
            </div>

            <!-- ✅ 描述（可编辑） -->
            <UFormField label="描述" name="description">
              <UTextarea
                v-model="editingForm.description"
                placeholder="输入权限描述（可选）"
              />
            </UFormField>

            <div class="flex justify-end gap-3 pt-2">
              <UButton
                color="gray"
                variant="ghost"
                @click="isEditModalOpen = false"
                >取消</UButton
              >
              <UButton color="primary" type="submit">保存更改</UButton>
            </div>
          </UForm>
        </UCard>
      </template>
    </UModal>
  </div>
</template>
