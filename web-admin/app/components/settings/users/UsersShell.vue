<!-- /components/settings/users/UsersShell.vue -->
<script setup lang="ts">
import { computed, onMounted } from "vue";
import { storeToRefs } from "pinia";
import UsersRoot from "./UsersRoot.vue";
import UsersTenantAdmin from "./UsersTenantAdmin.vue";
import UsersTenantMember from "./UsersTenantMember.vue";
import { useUserStore } from "~/stores/user";

// 使用用户状态 Store
const userStore = useUserStore();
const {
  isRoot,
  isCurrentTenantAdmin,
  currentTenantUuid,
  isLoading,
  error,
  displayName,
  avatarUrl,
} = storeToRefs(userStore);

// 计算当前视图类型
const view = computed(() => {
  if (isRoot.value) return "root";
  if (isCurrentTenantAdmin.value) return "admin";
  return "member";
});

// 组件挂载时加载用户上下文
onMounted(async () => {
  try {
    await userStore.fetchUserContext();
  } catch (error) {
    console.error("加载用户上下文失败:", error);
  }
});
</script>

<template>
  <div>
    <!-- 加载状态 -->
    <div v-if="isLoading" class="flex items-center justify-center py-12">
      <UIcon
        name="i-heroicons-arrow-path"
        class="animate-spin h-8 w-8 text-blue-600"
      />
      <span class="ml-3 text-gray-600">{{ $t("common.loading") }}</span>
    </div>

    <!-- 错误状态 -->
    <div
      v-else-if="error"
      class="p-6 bg-red-50 border border-red-200 rounded-lg"
    >
      <div class="flex items-center">
        <UIcon
          name="i-heroicons-exclamation-triangle"
          class="h-5 w-5 text-red-600"
        />
        <span class="ml-2 text-red-800 font-medium">{{
          $t("common.error")
        }}</span>
      </div>
      <p class="mt-2 text-red-700">{{ error }}</p>
      <UButton
        class="mt-3"
        color="primary"
        variant="outline"
        size="sm"
        @click="userStore.fetchUserContext"
      >
        {{ $t("common.retry") }}
      </UButton>
    </div>

    <!-- 根据用户角色显示对应视图 -->
    <div v-else>
      <UsersRoot v-if="view === 'root'" />
      <UsersTenantAdmin
        v-else-if="view === 'admin'"
        :tenant-uuid="currentTenantUuid!"
      />
      <UsersTenantMember v-else />
    </div>
  </div>
</template>
