<!-- /components/settings/users/UsersTenantMember.vue -->
<script setup lang="ts">
import { ref, computed, h, resolveComponent, onMounted, watch } from "vue";
import { storeToRefs } from "pinia";
import { useI18n } from "#imports";
import { useUserStore } from "~/stores/user";
import { useMemberService } from "~/composables/api/services/memberService";

// Member 不需要传入 tenantUuid，而是自己选择所属的租户
const { t, locale } = useI18n();

// 使用用户 Store
const userStore = useUserStore();
const {
  memberTenants,
  currentTenantUuid,
  displayName,
} = storeToRefs(userStore);
const memberService = useMemberService();
const toast = useToast();

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
const showDetail = ref(false);
const selectedUser = ref<RowUser | null>(null);

const currentTenant = computed(() =>
  memberTenants.value.find((tenant) => tenant.tenant_uuid === currentTenantUuid.value)
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
const UButton = resolveComponent("UButton");

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
    {
      id: "actions",
      header: "操作",
      cell: ({ row }: any) =>
        h(
          UButton,
          {
            size: "xs",
            variant: "ghost",
            icon: "i-heroicons-eye",
            onClick: () => openDetail(row.original as RowUser),
          },
          () => "详情"
        ),
    },
  ];
});

function openDetail(user: RowUser) {
  selectedUser.value = user;
  showDetail.value = true;
}

// 加载当前租户用户数据（普通成员只读）
async function loadUsersForTenant() {
  isLoading.value = true;
  try {
    const list = await memberService.getMemberList({
      page: 1,
      page_size: 100,
      sort_by: "created_at",
      sort_order: "asc",
    });
    const items = Array.isArray(list) ? list : [];

    users.value = items.map((item: any) => {
      const member = item?.Member || item?.member || item;
      const user = item?.User || item?.user || null;
      const rawPhone = user?.phone || user?.mobile || member?.phone || member?.mobile || "";
      const maskedPhone =
        rawPhone.length > 7
          ? `${rawPhone.slice(0, 3)}****${rawPhone.slice(-4)}`
          : rawPhone;
      const joinedAtRaw = member?.createdAt || member?.created_at;
      return {
        id: Number(member?.id || 0),
        name: member?.display_name || member?.name || user?.display_name || member?.username || "",
        username: member?.username || "",
        email: user?.email || member?.email || "",
        department: (member?.meta as any)?.department || "",
        phone: maskedPhone,
        status: Number(member?.status) === 1 ? "active" : "inactive",
        avatar:
          member?.avatar_url ||
          user?.avatar_url ||
          `https://i.pravatar.cc/150?u=${encodeURIComponent(
            user?.email || member?.username || String(member?.id || 0)
          )}`,
        joinedAt: joinedAtRaw
          ? new Date(joinedAtRaw).toLocaleDateString("zh-CN")
          : "",
      } as RowUser;
    });
  } catch (error) {
    console.error("加载用户数据失败:", error);
    users.value = [];
    toast.add({
      title: "加载同事列表失败",
      description: "请刷新重试，或联系管理员排查权限/接口配置",
      color: "error",
    });
  } finally {
    isLoading.value = false;
  }
}

watch(currentTenantUuid, async (uuid) => {
  if (uuid) {
    await loadUsersForTenant();
  } else {
    users.value = [];
  }
});

// 组件挂载时初始化
onMounted(async () => {
  try {
    // 确保用户上下文已加载
    if (!userStore.context) {
      await userStore.fetchUserContext();
    }

    // 普通成员只读取当前租户
    if (currentTenantUuid.value) {
      await loadUsersForTenant();
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

      <!-- 当前租户（只读） -->
      <div class="bg-white p-4 rounded-lg shadow-sm">
        <div class="flex items-center gap-2">
          <span class="text-sm text-gray-500">当前租户</span>
          <UBadge color="primary" variant="subtle">
            {{ currentTenant?.tenant_name || "Unknown" }}
          </UBadge>
          <span class="text-xs text-gray-500">{{ currentTenantUuid || "-" }}</span>
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
    <div class="mt-6 rounded-lg border border-slate-300 bg-slate-50 p-4 dark:border-slate-600 dark:bg-slate-800/70">
      <div class="flex items-start">
        <UIcon
          name="i-heroicons-information-circle"
          class="mt-0.5 mr-3 h-5 w-5 flex-shrink-0 text-slate-700 dark:text-slate-200"
        />
        <div class="text-sm leading-6 text-slate-700 dark:text-slate-200">
          <p class="mb-1 font-semibold text-slate-900 dark:text-slate-100">权限说明</p>
          <ul class="space-y-1">
            <li>• 您只能查看有权限访问的同事信息</li>
            <li>• 无法编辑、添加或删除用户</li>
            <li>• 当前页面仅展示您当前租户内的成员信息</li>
            <li>• 如需更多权限，请联系管理员</li>
          </ul>
        </div>
      </div>
    </div>

    <UModal v-model:open="showDetail" title="用户详情">
      <template #body>
        <div v-if="selectedUser" class="space-y-3 text-sm">
          <div><span class="text-gray-500">姓名：</span>{{ selectedUser.name || "-" }}</div>
          <div><span class="text-gray-500">用户名：</span>{{ selectedUser.username || "-" }}</div>
          <div><span class="text-gray-500">邮箱：</span>{{ selectedUser.email || "-" }}</div>
          <div><span class="text-gray-500">联系电话：</span>{{ selectedUser.phone || "-" }}</div>
          <div><span class="text-gray-500">部门：</span>{{ selectedUser.department || "-" }}</div>
          <div><span class="text-gray-500">加入时间：</span>{{ selectedUser.joinedAt || "-" }}</div>
          <div><span class="text-gray-500">状态：</span>{{ selectedUser.status === "active" ? "启用" : "停用" }}</div>
        </div>
      </template>
    </UModal>
  </div>
</template>
