<script setup lang="ts">
import { computed, onMounted } from "vue";
import { storeToRefs } from "pinia";
import DepartmentManager from "@/components/settings/users/DepartmentManager.vue";
import UserShell from "@/components/settings/users/UsersShell.vue";
import PermissionShell from "@/components/settings/users/PermissionShell.vue";
import { useUserStore } from "~/stores/user";

definePageMeta({
  title: "用户管理",
  icon: "i-heroicons-user",
  order: 8,
});

const { t } = useI18n();
const activeTab = ref("departments");

// 使用用户状态 Store
const userStore = useUserStore();
const { isRoot, isCurrentTenantAdmin, isLoading, error } =
  storeToRefs(userStore);

// 根据用户角色动态生成选项卡
const tabs = computed(() => {
  const baseTabs = [
    {
      id: "departments",
      name: t("organization.tabs.departments"),
      icon: "i-heroicons-building-office",
    },
    {
      id: "users",
      name: t("organization.tabs.users"),
      icon: "i-heroicons-users",
    },
  ];

  // 只有 root 用户或租户管理员才能看到权限管理
  if (isRoot.value || isCurrentTenantAdmin.value) {
    baseTabs.push({
      id: "permissions",
      name: t("organization.tabs.permissions"),
      icon: "i-heroicons-lock-closed",
    });
  }

  return baseTabs;
});

// 组件挂载时加载用户上下文
onMounted(async () => {
  try {
    await userStore.fetchUserContext({ force: true });
  } catch (error) {
    console.error("加载用户上下文失败:", error);
  }
});
</script>

<template>
  <div class="p-6">
    <div class="mb-6">
      <h1 class="text-2xl font-bold text-gray-900">
        {{ t("menu.userManagement") }}
      </h1>
      <p class="text-gray-600 mt-2">{{ t("organization.description") }}</p>
    </div>

    <div class="bg-white rounded-lg shadow">
      <!-- 选项卡导航 -->
      <div class="border-b border-gray-200">
        <nav class="flex -mb-px">
          <button
            v-for="tab in tabs"
            :key="tab.id"
            @click="activeTab = tab.id"
            :class="[
              'px-6 py-4 text-sm font-medium flex items-center',
              activeTab === tab.id
                ? 'border-b-2 border-primary-500 text-primary-600'
                : 'text-gray-500 hover:text-gray-700 hover:border-b-2 hover:border-gray-300',
            ]"
          >
            <UIcon :name="tab.icon" class="w-5 h-5 mr-2" />
            {{ tab.name }}
          </button>
        </nav>
      </div>

      <!-- 选项卡内容 -->
      <div class="p-6">
        <!-- 部门管理 -->
        <div v-if="activeTab === 'departments'">
          <DepartmentManager />
        </div>

        <!-- 用户管理 -->
        <!-- 用户管理 -->
        <div v-if="activeTab === 'users'">
          <UserShell />
        </div>

        <!-- 权限管理 -->
        <div v-if="activeTab === 'permissions'">
          <PermissionShell />
        </div>
      </div>
    </div>
  </div>
</template>
