<!-- /components/settings/users/PermissionTenant.vue -->
<script setup lang="ts">
import { ref, computed, onMounted } from "vue";
import { storeToRefs } from "pinia";
import { usePermissionStore } from "~/stores/permission";
import { useUserStore } from "~/stores/user";

// Props
const props = defineProps<{
  tenantUuid: string;
}>();

// 权限 Store
const permissionStore = usePermissionStore();
const { permissions, tenantPermissions, isLoading, error } =
  storeToRefs(permissionStore);

// 用户 Store
const userStore = useUserStore();

// 表格列定义
const columns = [
  { key: "name", label: "权限名称", sortable: true },
  { key: "code", label: "权限代码", sortable: true },
  { key: "description", label: "描述" },
  { key: "category", label: "分类", sortable: true },
  { key: "enabled", label: "启用状态" },
  { key: "actions", label: "操作" },
];

// 搜索和过滤
const searchQuery = ref("");
const selectedCategory = ref("");
const statusFilter = ref(""); // all, enabled, disabled

const categories = computed(() => {
  const cats = new Set(
    permissions.value.map((p) => p.category).filter(Boolean)
  );
  return Array.from(cats);
});

// 过滤后的权限列表
const filteredPermissions = computed(() => {
  let filtered = permissions.value.map((permission) => {
    const tenantPerm = tenantPermissions.value.find(
      (tp) => tp.permission_id === permission.id
    );
    return {
      ...permission,
      enabled: tenantPerm?.enabled || false,
      tenantPermissionId: tenantPerm?.id,
    };
  });

  if (searchQuery.value) {
    const query = searchQuery.value.toLowerCase();
    filtered = filtered.filter(
      (p) =>
        p.name.toLowerCase().includes(query) ||
        p.code.toLowerCase().includes(query) ||
        p.description?.toLowerCase().includes(query)
    );
  }

  if (selectedCategory.value) {
    filtered = filtered.filter((p) => p.category === selectedCategory.value);
  }

  if (statusFilter.value === "enabled") {
    filtered = filtered.filter((p) => p.enabled);
  } else if (statusFilter.value === "disabled") {
    filtered = filtered.filter((p) => !p.enabled);
  }

  return filtered;
});

// 统计信息
const stats = computed(() => {
  const total = permissions.value.length;
  const enabled = tenantPermissions.value.filter((tp) => tp.enabled).length;
  return { total, enabled, disabled: total - enabled };
});

// 切换权限状态
const togglePermission = async (permission) => {
  try {
    if (permission.enabled) {
      // 禁用权限
      await permissionStore.disableTenantPermission(
        props.tenantUuid,
        permission.id
      );
    } else {
      // 启用权限
      await permissionStore.enableTenantPermission(
        props.tenantUuid,
        permission.id
      );
    }
    // 重新加载租户权限
    await permissionStore.fetchTenantPermissions(props.tenantUuid);
  } catch (error) {
    console.error("切换权限状态失败:", error);
  }
};

// 批量操作
const selectedPermissions = ref([]);
const isSelectAll = computed({
  get: () =>
    selectedPermissions.value.length === filteredPermissions.value.length,
  set: (value) => {
    selectedPermissions.value = value
      ? filteredPermissions.value.map((p) => p.id)
      : [];
  },
});

const batchEnable = async () => {
  try {
    await permissionStore.batchEnableTenantPermissions(
      props.tenantUuid,
      selectedPermissions.value
    );
    await permissionStore.fetchTenantPermissions(props.tenantUuid);
    selectedPermissions.value = [];
  } catch (error) {
    console.error("批量启用权限失败:", error);
  }
};

const batchDisable = async () => {
  try {
    await permissionStore.batchDisableTenantPermissions(
      props.tenantUuid,
      selectedPermissions.value
    );
    await permissionStore.fetchTenantPermissions(props.tenantUuid);
    selectedPermissions.value = [];
  } catch (error) {
    console.error("批量禁用权限失败:", error);
  }
};

// 组件挂载时加载数据
onMounted(async () => {
  await Promise.all([
    permissionStore.fetchList(),
    permissionStore.fetchTenantPermissions(props.tenantUuid),
  ]);
});
</script>

<template>
  <div class="space-y-6">
    <!-- 页面标题 -->
    <div>
      <h1 class="text-2xl font-bold text-gray-900">权限配置</h1>
      <p class="text-gray-600 mt-1">配置租户可用的系统权限</p>
    </div>

    <!-- 统计卡片 -->
    <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
      <UCard>
        <div class="flex items-center">
          <div class="p-2 bg-blue-100 rounded-lg">
            <UIcon name="i-heroicons-key" class="h-6 w-6 text-blue-600" />
          </div>
          <div class="ml-4">
            <p class="text-sm text-gray-600">总权限数</p>
            <p class="text-2xl font-bold text-gray-900">{{ stats.total }}</p>
          </div>
        </div>
      </UCard>

      <UCard>
        <div class="flex items-center">
          <div class="p-2 bg-green-100 rounded-lg">
            <UIcon
              name="i-heroicons-check-circle"
              class="h-6 w-6 text-green-600"
            />
          </div>
          <div class="ml-4">
            <p class="text-sm text-gray-600">已启用</p>
            <p class="text-2xl font-bold text-green-600">{{ stats.enabled }}</p>
          </div>
        </div>
      </UCard>

      <UCard>
        <div class="flex items-center">
          <div class="p-2 bg-gray-100 rounded-lg">
            <UIcon name="i-heroicons-x-circle" class="h-6 w-6 text-gray-600" />
          </div>
          <div class="ml-4">
            <p class="text-sm text-gray-600">已禁用</p>
            <p class="text-2xl font-bold text-gray-600">{{ stats.disabled }}</p>
          </div>
        </div>
      </UCard>
    </div>

    <!-- 搜索和过滤 -->
    <div class="flex gap-4">
      <UInput
        v-model="searchQuery"
        placeholder="搜索权限名称、代码或描述..."
        icon="i-heroicons-magnifying-glass"
        class="flex-1"
      />
      <USelect
        v-model="selectedCategory"
        :options="[
          { label: '全部分类', value: '' },
          ...categories.map((c) => ({ label: c, value: c })),
        ]"
        placeholder="选择分类"
        class="w-48"
      />
      <USelect
        v-model="statusFilter"
        :options="[
          { label: '全部状态', value: '' },
          { label: '已启用', value: 'enabled' },
          { label: '已禁用', value: 'disabled' },
        ]"
        placeholder="选择状态"
        class="w-32"
      />
    </div>

    <!-- 批量操作 -->
    <div
      v-if="selectedPermissions.length > 0"
      class="flex items-center gap-3 p-4 bg-blue-50 rounded-lg"
    >
      <span class="text-sm text-blue-700">
        已选择 {{ selectedPermissions.length }} 个权限
      </span>
      <div class="flex gap-2">
        <UButton size="sm" color="success" @click="batchEnable">
          批量启用
        </UButton>
        <UButton size="sm" color="neutral" @click="batchDisable">
          批量禁用
        </UButton>
        <UButton
          size="sm"
          color="neutral"
          variant="ghost"
          @click="selectedPermissions = []"
        >
          取消选择
        </UButton>
      </div>
    </div>

    <!-- 权限列表 -->
    <UCard>
      <template #header>
        <div class="flex justify-between items-center">
          <h3 class="text-lg font-semibold">权限列表</h3>
          <div class="flex items-center gap-3">
            <UCheckbox v-model="isSelectAll" label="全选" />
            <span class="text-sm text-gray-500">
              共 {{ filteredPermissions.length }} 个权限
            </span>
          </div>
        </div>
      </template>

      <UTable
        v-model="selectedPermissions"
        :rows="filteredPermissions"
        :columns="columns"
        :loading="isLoading"
        :empty-state="{
          icon: 'i-heroicons-key',
          label: '暂无权限数据',
          description: '请联系系统管理员添加权限',
        }"
      >
        <template #name-data="{ row }">
          <div class="flex items-center gap-3">
            <UCheckbox
              :model-value="selectedPermissions.includes(row.id)"
              @update:model-value="
                (checked) => {
                  if (checked) {
                    selectedPermissions.push(row.id);
                  } else {
                    selectedPermissions = selectedPermissions.filter(
                      (id) => id !== row.id
                    );
                  }
                }
              "
            />
            <div class="font-medium text-gray-900">{{ row.name }}</div>
          </div>
        </template>

        <template #code-data="{ row }">
          <code class="px-2 py-1 bg-gray-100 rounded text-sm">{{
            row.code
          }}</code>
        </template>

        <template #description-data="{ row }">
          <span class="text-gray-600">{{ row.description || "-" }}</span>
        </template>

        <template #category-data="{ row }">
          <UBadge v-if="row.category" variant="soft">{{ row.category }}</UBadge>
          <span v-else class="text-gray-400">-</span>
        </template>

        <template #enabled-data="{ row }">
          <UBadge :color="row.enabled ? 'green' : 'gray'" variant="soft">
            {{ row.enabled ? "已启用" : "已禁用" }}
          </UBadge>
        </template>

        <template #actions-data="{ row }">
          <UToggle
            :model-value="row.enabled"
            @update:model-value="togglePermission(row)"
          />
        </template>
      </UTable>
    </UCard>
  </div>
</template>
