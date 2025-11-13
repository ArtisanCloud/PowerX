<template>
  <div class="space-y-6 p-4">
    <div
      class="flex flex-col gap-4 md:flex-row md:items-center md:justify-between"
    >
      <div>
        <h1 class="text-lg font-semibold text-[var(--text-primary)]">
          {{ $t("settings.ai.costGuardPageTitle") }}
        </h1>
        <p class="text-sm text-[var(--text-secondary)]">
          {{ $t("settings.ai.costGuardPageDesc") }}
        </p>
      </div>
      <div class="flex flex-wrap gap-2">
        <UButton
          variant="ghost"
          icon="i-heroicons-arrow-left"
          :to="modelSettingsLink"
        >
          {{ $t("settings.ai.actions.backToModels") }}
        </UButton>
      </div>
    </div>

    <UCard>
      <template #header>
        <div class="flex flex-col gap-1">
          <span class="font-medium text-[var(--text-primary)]">
            {{ $t("settings.ai.costGuard.panelTitle") }}
          </span>
          <span class="text-sm text-[var(--text-secondary)]">
            {{ $t("settings.ai.costGuard.panelDesc") }}
          </span>
        </div>
      </template>
      <CostQuotaPanel />
    </UCard>

    <UCard>
      <template #header>
        {{ $t("settings.ai.costGuard.playbookTitle") }}
      </template>
      <ul class="space-y-3">
        <li
          v-for="playbook in playbooks"
          :key="playbook.key"
          class="flex items-start gap-3"
        >
          <UIcon :name="playbook.icon" class="w-5 h-5 text-primary-500" />
          <div>
            <p class="font-medium text-[var(--text-primary)]">
              {{ playbook.title }}
            </p>
            <p class="text-sm text-[var(--text-secondary)]">
              {{ playbook.description }}
            </p>
          </div>
        </li>
      </ul>
    </UCard>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import CostQuotaPanel from "~/components/settings/ai/cost/CostQuotaPanel.vue";

const localePath = useLocalePath();
const modelSettingsLink = computed(() => localePath("/settings/ai"));
const { t } = useI18n({ useScope: "global" });

const playbooks = computed(() => [
  {
    key: "throttle",
    icon: "i-heroicons-arrow-down-right",
    title: t("settings.ai.costGuard.playbooks.throttle.title"),
    description: t("settings.ai.costGuard.playbooks.throttle.desc"),
  },
  {
    key: "degrade",
    icon: "i-heroicons-arrow-path-rounded-square",
    title: t("settings.ai.costGuard.playbooks.degrade.title"),
    description: t("settings.ai.costGuard.playbooks.degrade.desc"),
  },
  {
    key: "disable",
    icon: "i-heroicons-no-symbol",
    title: t("settings.ai.costGuard.playbooks.disable.title"),
    description: t("settings.ai.costGuard.playbooks.disable.desc"),
  },
]);
</script>
