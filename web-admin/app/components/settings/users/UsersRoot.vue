<!-- /components/settings/users/UsersRoot.vue -->
<script setup lang="ts">
import { ref, reactive, computed, watch, onMounted } from "vue";
import UsersTenantAdmin from "./UsersTenantAdmin.vue";
import { useI18n } from "#imports";
import {
  useTenantService,
  type Tenant,
} from "~/composables/api/services/tenantService";
import { useUserStore } from "~/stores/user";
import { useToast } from "#imports";

import {
  TenantStatus,
  getTenantStatusColor,
  getTenantStatusDisplayKey,
} from "~/composables/api/types/tenant";

const { t } = useI18n();

// 依赖注入的服务
const tenantService = useTenantService();

// 转换后的租户数据结构
interface DisplayTenant {
  id: number;
  uuid: string;
  name: string;
  domain: string;
  status: TenantStatus;
  userCount: number;
  createdAt: string;
  plan: string;
}

// 状态管理
const tenants = ref<DisplayTenant[]>([]);
const selectedTenant = ref<DisplayTenant | null>(null);
const detailModalOpen = ref(false);
const managingTenant = ref<DisplayTenant | null>(null);
const searchQuery = ref("");

// 分页和筛选
const pagination = reactive({
  page: 1,
  pageSize: 10,
  total: 0,
  totalPages: 0,
});

const filters = reactive({
  status: null as TenantStatus | null,
  plan: null as string | null,
});

// 筛选选项
const statusOptions = computed(() => [
  { label: t("organization.user.filter.allStatus"), value: null },
  { label: t("organization.user.status.active"), value: TenantStatus.Active },
  {
    label: t("organization.user.status.inactive"),
    value: TenantStatus.Inactive,
  },
  {
    label: t("organization.user.status.suspended"),
    value: TenantStatus.Suspended,
  },
]);

const planOptions = computed(() => [
  { label: t("organization.user.filter.allPlans"), value: null },
  { label: t("organization.user.plan.free"), value: "free" },
  { label: t("organization.user.plan.pro"), value: "pro" },
  { label: t("organization.user.plan.enterprise"), value: "enterprise" },
]);

// 计算属性 - 使用服务端分页，不需要客户端过滤
const paginatedTenants = computed(() => tenants.value);

// 方法
const userStore = useUserStore();
const toast = useToast();

async function loadTenants() {
  try {
    const response = await tenantService.getTenants({
      page: pagination.page,
      page_size: pagination.pageSize,
      status: filters.status ?? undefined,
      plan: filters.plan ?? undefined,
      search: searchQuery.value.trim() || undefined,
    });

    if (response.code === 200) {
      // 转换API数据为显示格式
      tenants.value = response.data.items.map(
        (tenant: Tenant): DisplayTenant => ({
          id: tenant.id,
          uuid: tenant.uuid,
          name: tenant.name,
          domain: tenant.domain,
          status: tenant.status as TenantStatus,
          userCount: tenant.user_count,
          createdAt: new Date(tenant.createdAt).toLocaleDateString("zh-CN"),
          plan: tenant.plan,
        })
      );

      // 更新分页信息
      pagination.total = response.data.pagination.total;
      pagination.totalPages = response.data.pagination.pages;
    }
  } catch (error) {
    console.error(t("organization.user.loadFailed"), error);
    // 错误提示已在tenantService中处理
  }
}

function selectTenant(tenant: DisplayTenant) {
  selectedTenant.value = tenant;
}

function openTenantDetails(tenant: DisplayTenant) {
  selectedTenant.value = tenant;
  detailModalOpen.value = true;
}

function closeTenantDetails() {
  detailModalOpen.value = false;
}

async function switchAndManageTenant(tenant: DisplayTenant) {
  try {
    await userStore.switchTenant(tenant.uuid);
    selectedTenant.value = tenant;
    detailModalOpen.value = false;
    managingTenant.value = tenant;
  } catch (error: any) {
    console.error("切换租户失败:", error);
    toast.add({
      color: "red",
      title: t("common.error"),
      description: error?.message || t("common.retry"),
    });
  }
}

function backToTenantList() {
  managingTenant.value = null;
}

function resetFilters() {
  searchQuery.value = "";
  filters.status = null;
  filters.plan = null;
  pagination.page = 1;
  loadTenants();
}

function changePage(page: number) {
  if (page >= 1 && page <= pagination.totalPages) {
    pagination.page = page;
    loadTenants();
  }
}

// 状态显示 - 使用导入的工具函数
// getStatusColor 已从 user.ts 导入

const getStatusText = (status: TenantStatus) => {
  const statusKey = getTenantStatusDisplayKey(status);
  const translationStatus = `organization.user.status.${statusKey}`;
  return t(translationStatus);
};

const getPlanText = (plan: string) => {
  return t(`organization.user.plan.${plan}`);
};

// 防抖定时器
let searchDebounceTimer: ReturnType<typeof setTimeout> | null = null;

// 监听筛选变化，立即触发重新加载
watch([() => filters.status, () => filters.plan], () => {
  pagination.page = 1;
  loadTenants();
});

// 监听搜索变化，使用防抖延迟触发
watch(searchQuery, () => {
  if (searchDebounceTimer) {
    clearTimeout(searchDebounceTimer);
  }

  searchDebounceTimer = setTimeout(() => {
    pagination.page = 1;
    loadTenants();
  }, 800);
});

onMounted(() => {
  loadTenants();
});
</script>

<template>
  <div>
    <!-- 租户选择界面 -->
    <div v-if="!managingTenant">
      <div class="flex items-center justify-between mb-6">
        <div>
          <h2 class="text-xl font-semibold">
            {{ t("organization.user.title") }} ·
            {{ t("organization.user.rootManagement") }}
          </h2>
          <p class="text-sm text-gray-500">
            {{ t("organization.user.selectTenantDesc") }}
          </p>
        </div>
        <div class="flex gap-2">
          <UTooltip :text="$t('tenant.tooltips.saasSupport')">
            <UButton icon="i-lucide-plus" variant="outline" disabled>
              {{ $t("tenant.add") }}
            </UButton>
          </UTooltip>
          <UButton
            icon="i-heroicons-arrow-path"
            variant="outline"
            @click="loadTenants"
          >
            {{ t("common.reload") }}
          </UButton>
        </div>
      </div>

      <!-- 搜索和筛选 -->
      <div class="mb-6 bg-white p-4 rounded-lg shadow-sm">
        <div class="flex flex-wrap gap-4 items-end">
          <div class="flex-grow min-w-[300px]">
            <UInput
              v-model="searchQuery"
              icon="i-heroicons-magnifying-glass"
              :placeholder="t('organization.user.searchTenants')"
            />
          </div>
          <UFormField :label="t('organization.user.filter.status')">
            <USelect
              v-model="filters.status"
              :items="statusOptions"
              :placeholder="t('organization.user.filter.allStatus')"
              class="w-32"
            />
          </UFormField>
          <UFormField :label="t('organization.user.filter.plan')">
            <USelect
              v-model="filters.plan"
              :items="planOptions"
              :placeholder="t('organization.user.filter.allPlans')"
              class="w-32"
            />
          </UFormField>
          <UButton
            icon="i-heroicons-arrow-path"
            variant="ghost"
            @click="resetFilters"
          >
            {{ t("organization.common.reset") }}
          </UButton>
        </div>
      </div>

      <!-- 租户列表 -->
      <div class="bg-white rounded-lg shadow-sm">
        <div
          v-if="paginatedTenants.length === 0"
          class="p-8 text-center text-gray-500"
        >
          <UIcon
            name="i-heroicons-building-office"
            class="h-12 w-12 mx-auto mb-4 text-gray-300"
          />
          <p>{{ t("organization.user.noTenantsFound") }}</p>
        </div>

        <div v-else class="divide-y divide-gray-200">
          <div
            v-for="tenant in paginatedTenants"
            :key="tenant.id"
            :class="[
              'p-4 cursor-pointer transition-colors',
              selectedTenant?.id === tenant.id
                ? 'bg-primary-50/40'
                : 'hover:bg-gray-50',
            ]"
            @click="selectTenant(tenant)"
          >
            <div class="flex items-center justify-between">
              <div class="flex-1">
                <div class="flex items-center gap-3">
                  <div
                    class="w-10 h-10 bg-blue-100 rounded-lg flex items-center justify-center"
                  >
                    <UIcon
                      name="i-heroicons-building-office"
                      class="h-5 w-5 text-blue-600"
                    />
                  </div>
                  <div>
                    <h3 class="font-medium text-gray-900">{{ tenant.name }}</h3>
                    <p class="text-sm text-gray-500">
                      {{ tenant.domain || t("organization.user.noDomain") }}
                    </p>
                  </div>
                </div>
              </div>

              <div class="flex items-center gap-4 text-sm">
                <div class="text-center">
                  <p class="font-medium text-gray-900">
                    {{ tenant.userCount }}
                  </p>
                  <p class="text-gray-500">
                    {{ t("organization.user.userCount") }}
                  </p>
                </div>

                <div class="text-center">
                  <UBadge
                    :color="getTenantStatusColor(tenant.status)"
                    variant="subtle"
                  >
                    {{ getStatusText(tenant.status) }}
                  </UBadge>
                  <p class="text-gray-500 mt-1">
                    {{ getPlanText(tenant.plan) }}
                  </p>
                </div>

                <div class="text-center">
                  <p class="text-gray-500">{{ tenant.createdAt }}</p>
                  <p class="text-gray-400 text-xs">
                    {{ t("organization.user.createdAt") }}
                  </p>
                </div>

                <div class="flex items-center gap-2">
                  <UButton
                    size="xs"
                    variant="outline"
                    @click.stop="openTenantDetails(tenant)"
                  >
                    查看详情
                  </UButton>
                  <UButton
                    size="xs"
                    color="primary"
                    @click.stop="switchAndManageTenant(tenant)"
                  >
                    切换并管理
                  </UButton>
                  <UIcon
                    name="i-heroicons-chevron-right"
                    class="h-5 w-5 text-gray-400"
                  />
                </div>
              </div>
            </div>
          </div>
        </div>

        <UModal
          v-model:open="detailModalOpen"
          title="租户详情"
          description="查看租户基础信息，再决定是否切换到该租户进行用户管理。"
          :ui="{ content: 'max-w-2xl' }"
        >
          <template #content>
            <UCard v-if="selectedTenant">
              <template #header>
                <div class="flex items-center gap-3">
                  <div
                    class="w-10 h-10 bg-blue-100 rounded-lg flex items-center justify-center"
                  >
                    <UIcon
                      name="i-heroicons-building-office"
                      class="h-5 w-5 text-blue-600"
                    />
                  </div>
                  <div>
                    <h3 class="text-lg font-semibold text-gray-900">
                      {{ selectedTenant.name }}
                    </h3>
                    <p class="text-sm text-gray-500">
                      {{
                        selectedTenant.domain ||
                        t("organization.user.noDomain")
                      }}
                    </p>
                  </div>
                </div>
              </template>

              <div class="grid grid-cols-1 md:grid-cols-2 gap-4 text-sm">
                <div>
                  <p class="text-gray-500">租户 UUID</p>
                  <p class="font-mono break-all text-gray-900">
                    {{ selectedTenant.uuid }}
                  </p>
                </div>
                <div>
                  <p class="text-gray-500">状态</p>
                  <UBadge
                    :color="getTenantStatusColor(selectedTenant.status)"
                    variant="subtle"
                  >
                    {{ getStatusText(selectedTenant.status) }}
                  </UBadge>
                </div>
                <div>
                  <p class="text-gray-500">套餐</p>
                  <p class="text-gray-900">
                    {{ getPlanText(selectedTenant.plan) }}
                  </p>
                </div>
                <div>
                  <p class="text-gray-500">用户数</p>
                  <p class="text-gray-900">{{ selectedTenant.userCount }}</p>
                </div>
                <div>
                  <p class="text-gray-500">创建时间</p>
                  <p class="text-gray-900">{{ selectedTenant.createdAt }}</p>
                </div>
                <div>
                  <p class="text-gray-500">域名</p>
                  <p class="text-gray-900 break-all">
                    {{
                      selectedTenant.domain ||
                      t("organization.user.noDomain")
                    }}
                  </p>
                </div>
              </div>

              <template #footer>
                <div class="flex justify-end">
                  <UButton variant="ghost" @click="closeTenantDetails">
                    关闭
                  </UButton>
                </div>
              </template>
            </UCard>
          </template>
        </UModal>

        <!-- 分页 -->
        <div
          v-if="pagination.totalPages > 1"
          class="px-6 py-4 border-t border-gray-200"
        >
          <div class="flex justify-between items-center">
            <div class="text-sm text-gray-600">
              {{
                t("organization.user.pagination.showing", {
                  start: (pagination.page - 1) * pagination.pageSize + 1,
                  end: Math.min(
                    pagination.page * pagination.pageSize,
                    pagination.total
                  ),
                  total: pagination.total,
                })
              }}
            </div>
            <div class="flex gap-2">
              <UButton
                :disabled="pagination.page <= 1"
                variant="outline"
                size="sm"
                icon="i-heroicons-chevron-left"
                @click="changePage(pagination.page - 1)"
              >
                {{ t("organization.user.pagination.previous") }}
              </UButton>
              <UButton
                :disabled="pagination.page >= pagination.totalPages"
                variant="outline"
                size="sm"
                icon="i-heroicons-chevron-right"
                @click="changePage(pagination.page + 1)"
              >
                {{ t("organization.user.pagination.next") }}
              </UButton>
            </div>
          </div>
        </div>
      </div>

    </div>

    <!-- 选中租户后的用户管理界面 -->
    <div v-else>
      <div class="flex items-center gap-3 mb-6">
        <UButton
          icon="i-heroicons-arrow-left"
          variant="ghost"
          @click="backToTenantList"
        >
          {{ t("organization.user.backToTenantList") }}
        </UButton>
        <div class="h-6 w-px bg-gray-300"></div>
        <div class="flex items-center gap-3">
          <div
            class="w-8 h-8 bg-blue-100 rounded-lg flex items-center justify-center"
          >
            <UIcon
              name="i-heroicons-building-office"
              class="h-4 w-4 text-blue-600"
            />
          </div>
          <div>
            <h2 class="text-xl font-semibold">{{ managingTenant.name }}</h2>
            <p class="text-sm text-gray-500">
              {{ managingTenant.domain || t("organization.user.noDomain") }} ·
              {{
                t("organization.user.userCountText", {
                  count: managingTenant.userCount,
                })
              }}
            </p>
          </div>
        </div>
      </div>

      <UsersTenantAdmin :tenant-uuid="managingTenant.uuid" />
    </div>
  </div>
</template>
