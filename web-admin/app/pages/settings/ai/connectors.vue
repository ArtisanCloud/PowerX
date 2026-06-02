<template>
  <div class="space-y-6 p-4">
    <div class="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
      <div>
        <h1 class="text-lg font-semibold text-[var(--text-primary)]">
          {{ $t("settings.ai.connectorsPageTitle") }}
        </h1>
        <p class="text-sm text-[var(--text-secondary)]">
          {{ $t("settings.ai.connectorsPageDesc") }}
        </p>
      </div>
      <div class="flex flex-wrap gap-2">
        <UButton variant="ghost" icon="i-heroicons-arrow-left" :to="modelSettingsLink">
          {{ $t("settings.ai.actions.backToModels") }}
        </UButton>
      </div>
    </div>

    <UAlert
      color="info"
      variant="soft"
      icon="i-heroicons-link"
      :title="$t('settings.ai.connectors.noticeTitle')"
      :description="$t('settings.ai.connectors.noticeDesc')"
    />

    <div class="grid gap-4 md:grid-cols-2">
      <UCard>
        <template #header>
          <div class="flex items-center gap-2">
            <UIcon name="i-heroicons-bolt" class="w-5 h-5 text-primary-500" />
            <span class="font-medium text-[var(--text-primary)]">Coze</span>
          </div>
        </template>
        <p class="text-sm text-[var(--text-secondary)]">
          {{ $t("settings.ai.connectors.platformDesc", { platform: "Coze" }) }}
        </p>
        <div class="mt-4 rounded-lg border border-[var(--border-color)] bg-[var(--card-bg)] p-3 text-xs text-[var(--text-secondary)]">
          <div class="font-medium text-[var(--text-primary)]">API</div>
          <div class="mt-1">POST /internal/connector-platforms/coze/instances</div>
        </div>
      </UCard>

      <UCard>
        <template #header>
          <div class="flex items-center gap-2">
            <UIcon name="i-heroicons-squares-plus" class="w-5 h-5 text-primary-500" />
            <span class="font-medium text-[var(--text-primary)]">n8n</span>
          </div>
        </template>
        <p class="text-sm text-[var(--text-secondary)]">
          {{ $t("settings.ai.connectors.platformDesc", { platform: "n8n" }) }}
        </p>
        <div class="mt-4 rounded-lg border border-[var(--border-color)] bg-[var(--card-bg)] p-3 text-xs text-[var(--text-secondary)]">
          <div class="font-medium text-[var(--text-primary)]">API</div>
          <div class="mt-1">POST /internal/connector-platforms/n8n/instances</div>
        </div>
      </UCard>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";

const localePath = useLocalePath();
const modelSettingsLink = computed(() => localePath("/settings/ai"));
const userStore = useUserStore();
const { isRoot, isCurrentTenantAdmin } = storeToRefs(userStore);

onMounted(async () => {
  if (!userStore.context) {
    await userStore.fetchUserContext();
  }
  if (isRoot.value || !isCurrentTenantAdmin.value) {
    await navigateTo("/dashboard");
  }
});
</script>
