<!-- /components/settings/users/UsersTenantMember.vue -->
<script setup lang="ts">
import { ref, computed, h, resolveComponent, onMounted, watch } from "vue";
import { storeToRefs } from "pinia";
import { useI18n } from "#imports";
import { useUserStore } from "~/stores/user";

// Member 不需要传入 tenantUuid，而是自己选择所属的租户
const { t, locale } = useI18n();

// 使用用户 Store
const userStore = useUserStore();
const {
  memberTenants,
  currentTenantUuid,
  isLoading: userLoading,
  displayName,
} = storeToRefs(userStore);

// 租户相关
interface UserTenant {
  id: string;
  name: string;
  domain: string;
}

// 用户数据结构
interface RowUser {
  id: number;
  name: string;
  username: string;
  email: string;
  department?: string;
  phone?: string;
  status: "active" | "inactive";
  avatar: string;
  joinedAt: string;
}

// 状态管理
const users = ref<RowUser[]>([]);
const searchQuery = ref("");
const isLoading = ref(false);

// 转换租户数据格式
const myTenants = computed(() =>
  memberTenants.value.map((tenant) => ({
    id: tenant.tenant_uuid,
    name: tenant.tenant_name,
    domain: `${tenant.tenant_name.toLowerCase()}.example.com`,
  }))
);

const selectedTenantUuid = ref<string | null>(currentTenantUuid.value);

// 计算属性
const selectedTenant = computed(() =>
  myTenants.value.find((t) => t.id === selectedTenantUuid.value)
);

const filteredUsers = computed(() => {
  const q = searchQuery.value.trim().toLowerCase();
  return users.value.filter(
    (u) =>
      !q ||
      u.name.toLowerCase().includes(q) ||
      u.username.toLowerCase().includes(q) ||
      u.email.toLowerCase().includes(q) ||
      (u.department && u.department.toLowerCase().includes(q))
  );
});

// 表格列定义
const UAvatar = resolveComponent("UAvatar");
const UBadge = resolveComponent("UBadge");

const columns = computed(() => {
  const _ = locale.value;
  return [
    {
      id: "avatar",
      accessorKey: "avatar",
      header: "",
      cell: ({ row }: any) =>
        h(UAvatar, { src: row.original.avatar, size: "sm" }),
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
      id: "department",
      accessorKey: "department",
      header: t("organization.user.table.department").toString(),
    },
    {
      id: "phone",
      accessorKey: "phone",
      header: "联系电话",
    },
    {
      id: "joinedAt",
      accessorKey: "joinedAt",
      header: "加入时间",
    },
    {
      id: "status",
      accessorKey: "status",
      header: t("organization.user.table.status").toString(),
      cell: ({ row }: any) =>
        h(
          UBadge,
          {
            color: row.original.status === "active" ? "success" : "neutral",
            variant: "subtle",
            size: "sm",
          },
          () =>
            row.original.status === "active"
              ? t("organization.user.form.active")
              : t("organization.user.form.inactive")
        ),
    },
  ];
});

// 加载指定租户的用户数据
async function loadUsersForTenant(tenantUuid: string) {
  isLoading.value = true;
  try {
    // TODO: 替换为真实API调用
    // const response = await $fetch(`/api/admin/iam/members`, {
    //   params: { tenant_uuid: tenantUuid, page: 1, page_size: 100 }
    // });
    // users.value = response.data || [];

    // 模拟不同租户的用户数据
    const mockData: Record<string, RowUser[]> = {
      "demo-tenant": [
        {
          id: 1,
          name: "张三",
          username: "zhangsan",
          email: "zhangsan@tech.com",
          department: "技术部",
          phone: "138****1234",
          status: "active",
          avatar: "https://randomuser.me/api/portraits/men/1.jpg",
          joinedAt: "2024-01-15",
        },
        {
          id: 2,
          name: "李四",
          username: "lisi",
          email: "lisi@tech.com",
          department: "产品部",
          phone: "139****5678",
          status: "active",
          avatar: "https://randomuser.me/api/portraits/women/2.jpg",
          joinedAt: "2024-02-20",
        },
      ],
      "ops-tenant": [
        {
          id: 3,
          name: "王五",
          username: "wangwu",
          email: "wangwu@consulting.com",
          department: "咨询部",
          phone: "136****9012",
          status: "active",
          avatar: "https://randomuser.me/api/portraits/men/3.jpg",
          joinedAt: "2024-03-10",
        },
      ],
    };

    users.value = mockData[tenantUuid] || [];
  } catch (error) {
    console.error("加载用户数据失败:", error);
    users.value = [];
  } finally {
    isLoading.value = false;
  }
}

// 监听租户切换
watch(selectedTenantUuid, async (newTenantUuid) => {
  if (newTenantUuid && newTenantUuid !== currentTenantUuid.value) {
    try {
      isLoading.value = true;
      await userStore.switchTenant(newTenantUuid);
      await loadUsersForTenant(newTenantUuid);
    } catch (error: any) {
      console.error("切换租户失败:", error);
      // 切换失败时恢复到原来的租户
      selectedTenantUuid.value = currentTenantUuid.value;
    } finally {
      isLoading.value = false;
    }
  } else if (newTenantUuid) {
    await loadUsersForTenant(newTenantUuid);
  }
});

// 组件挂载时初始化
onMounted(async () => {
  try {
    // 确保用户上下文已加载
    if (!userStore.context) {
      await userStore.fetchUserContext();
    }

    // 设置当前选中的租户
    if (currentTenantUuid.value) {
      selectedTenantUuid.value = currentTenantUuid.value;
      await loadUsersForTenant(currentTenantUuid.value);
    } else if (myTenants.value.length > 0) {
      selectedTenantUuid.value = myTenants.value[0].id;
      await loadUsersForTenant(myTenants.value[0].id);
    }
  } catch (error) {
    console.error("初始化用户上下文失败:", error);
  }
});
</script>

<template>
  <div>
    <!-- 顶部：租户选择和用户信息 -->
    <div class="mb-6">
      <div class="flex justify-between items-start mb-4">
        <div>
          <h2 class="text-xl font-semibold text-gray-800">
            {{ $t("organization.user.title") }}
          </h2>
          <p class="text-sm text-gray-500 mt-1">查看同事信息（只读权限）</p>
        </div>
        <div class="text-right">
          <p class="text-sm text-gray-600">当前用户：{{ displayName }}</p>
        </div>
      </div>

      <!-- 租户选择器 -->
      <div class="bg-white p-4 rounded-lg shadow-sm">
        <div class="flex items-center gap-4">
          <UFormField label="选择租户" class="flex-shrink-0">
            <USelect
              v-model="selectedTenantUuid"
              :options="myTenants"
              option-attribute="name"
              value-attribute="id"
              class="w-64"
              :loading="userLoading"
            />
          </UFormField>

          <!-- 当前租户信息 -->
          <div v-if="selectedTenant" class="flex-1">
            <div class="flex items-center gap-2">
              <UBadge color="primary" variant="subtle">
                {{ selectedTenant.name }}
              </UBadge>
              <span class="text-sm text-gray-500">
                {{ selectedTenant.domain }}
              </span>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 搜索栏 -->
    <div class="mb-6 bg-white p-4 rounded-lg shadow-sm">
      <div class="flex items-center gap-4">
        <div class="flex-1">
          <UInput
            v-model="searchQuery"
            icon="i-heroicons-magnifying-glass"
            placeholder="搜索同事姓名、用户名、邮箱或部门..."
            class="w-full"
          />
        </div>
        <div class="text-sm text-gray-500">
          共 {{ filteredUsers.length }} 位同事
        </div>
      </div>
    </div>

    <!-- 用户列表 -->
    <div class="bg-white rounded-lg shadow-sm">
      <!-- 加载状态 -->
      <div v-if="isLoading" class="flex items-center justify-center py-12">
        <UIcon
          name="i-heroicons-arrow-path"
          class="animate-spin h-6 w-6 text-blue-600"
        />
        <span class="ml-2 text-gray-600">加载中...</span>
      </div>

      <!-- 空状态 -->
      <div
        v-else-if="filteredUsers.length === 0"
        class="text-center py-12 text-gray-500"
      >
        <UIcon name="i-heroicons-users" class="h-12 w-12 mx-auto mb-4" />
        <p class="text-lg font-medium mb-2">暂无同事信息</p>
        <p class="text-sm">
          {{ searchQuery ? "没有找到匹配的同事" : "当前租户暂无其他成员" }}
        </p>
      </div>

      <!-- 用户表格 -->
      <div v-else>
        <UTable :data="filteredUsers" :columns="columns" />
      </div>
    </div>

    <!-- 权限说明 -->
    <div class="mt-6 p-4 bg-blue-50 border border-blue-200 rounded-lg">
      <div class="flex items-start">
        <UIcon
          name="i-heroicons-information-circle"
          class="h-5 w-5 text-blue-600 mt-0.5 mr-3 flex-shrink-0"
        />
        <div class="text-sm">
          <p class="font-medium text-blue-800 mb-1">权限说明</p>
          <ul class="text-blue-700 space-y-1">
            <li>• 您只能查看有权限访问的同事信息</li>
            <li>• 无法编辑、添加或删除用户</li>
            <li>• 可以在您所属的租户间切换查看</li>
            <li>• 如需更多权限，请联系管理员</li>
          </ul>
        </div>
      </div>
    </div>
  </div>
</template>
