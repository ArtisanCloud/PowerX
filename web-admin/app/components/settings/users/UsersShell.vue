<!-- /components/settings/users/UsersShell.vue -->
<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
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

const rootCrossTenantMode = ref(false);

// 计算当前视图类型
const view = computed(() => {
  if (isRoot.value) return "root";
  if (isCurrentTenantAdmin.value) return "admin";
  return "member";
});

const canRenderRootTenantAdmin = computed(
  () => !!currentTenantUuid.value && !rootCrossTenantMode.value
);

watch(isRoot, (next) => {
  if (!next) {
    rootCrossTenantMode.value = false;
  }
});

// 组件挂载时加载用户上下文
onMounted(async () => {
  try {
    // 用户管理页需要基于最新身份/租户上下文做视图分流（root/admin/member），
    // 这里强制刷新，避免命中 store 的 5 分钟缓存导致展示旧权限视图。
    await userStore.fetchUserContext({ force: true });
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
        @click="() => userStore.fetchUserContext({ force: true })"
      >
        {{ $t("common.retry") }}
      </UButton>
    </div>

    <!-- 根据用户角色显示对应视图 -->
    <div v-else>
      <template v-if="view === 'root'">
        <div class="mb-4 flex items-center justify-between">
          <p class="text-sm text-gray-500">
            {{
              rootCrossTenantMode
                ? "跨租户管理模式：选择租户后再进入用户管理。"
                : "当前租户管理模式：可直接新增和管理当前租户用户。"
            }}
          </p>
          <UButton
            size="sm"
            variant="outline"
            @click="rootCrossTenantMode = !rootCrossTenantMode"
          >
            {{ rootCrossTenantMode ? "返回当前租户管理" : "跨租户管理" }}
          </UButton>
        </div>

        <UsersTenantAdmin
          v-if="canRenderRootTenantAdmin"
          :tenant-uuid="currentTenantUuid!"
        />
        <UsersRoot v-else />
      </template>
      <UsersTenantAdmin v-else-if="view === 'admin'" :tenant-uuid="currentTenantUuid!" />
      <UsersTenantMember v-else />
    </div>
  </div>
</template>
