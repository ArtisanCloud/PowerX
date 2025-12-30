<template>
  <div class="space-y-6">
    <div class="flex flex-col gap-4 md:flex-row md:items-end">
      <div class="flex-1 space-y-1">
        <label class="text-sm font-medium text-[var(--text-secondary)]">
          {{ t("settings.ai.costGuard.tenantLabel") }}
        </label>
        <USelectMenu
          v-model="tenantUuid"
          :options="tenantOptions"
          :placeholder="t('settings.ai.costGuard.tenantPlaceholder')"
        />
      </div>
      <div class="flex flex-wrap items-center gap-3">
        <UButton
          icon="i-heroicons-arrow-path"
          variant="soft"
          :loading="quotaStore.loading"
          @click="refresh"
        >
          {{ t("settings.ai.costGuard.refresh") }}
        </UButton>
        <UButton
          v-if="dashboardLink"
          icon="i-heroicons-presentation-chart-line"
          variant="ghost"
          :to="dashboardLink"
        >
          {{ t("settings.ai.costGuard.openDashboard") }}
        </UButton>
        <span class="text-xs text-[var(--text-secondary)]" v-if="lastUpdated">
          {{ t("settings.ai.costGuard.lastUpdated") }}：
          {{ useDateFormat(lastUpdated, "HH:mm:ss").value }}
        </span>
      </div>
    </div>

    <div
      v-if="quotaStore.loading"
      class="grid gap-4 md:grid-cols-2 lg:grid-cols-3"
    >
      <USkeleton class="h-44" v-for="n in 3" :key="n" />
    </div>

    <UAlert
      v-else-if="error"
      color="red"
      icon="i-heroicons-exclamation-triangle"
      :title="t('settings.ai.costGuard.errorTitle')"
      :description="error"
    />

    <UAlert
      v-else-if="!quotas.length"
      color="gray"
      icon="i-heroicons-information-circle"
      :title="t('settings.ai.costGuard.emptyTitle')"
      :description="t('settings.ai.costGuard.emptyDesc')"
    />

    <div v-else class="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
      <UCard
        v-for="quota in quotas"
        :key="quota.providerId"
        class="flex flex-col gap-4"
      >
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm text-[var(--text-secondary)]">
              {{ quota.providerId }}
            </p>
            <p class="text-lg font-semibold text-[var(--text-primary)]">
              {{ formatCurrency(quota.usage) }} / {{ formatCurrency(quota.limit) }}
            </p>
          </div>
          <UBadge :color="statusColor(quota.status)">
            {{ statusLabel(quota.status) }}
          </UBadge>
        </div>

        <UProgress
          :value="progressPct(quota)"
          size="sm"
          :color="statusColor(quota.status)"
        />

        <div class="space-y-2">
          <USelectMenu
            v-model="selectedAction[quota.providerId]"
            :options="actionOptions"
            :placeholder="t('settings.ai.costGuard.actionPlaceholder')"
          />
          <UInput
            v-model="reasonInputs[quota.providerId]"
            :placeholder="t('settings.ai.costGuard.reasonPlaceholder')"
          />
          <UButton
            size="sm"
            icon="i-heroicons-shield-check"
            block
            :loading="pendingAction === quota.providerId"
            @click="applyAction(quota)"
          >
            {{ t("settings.ai.costGuard.executeAction") }}
          </UButton>
        </div>

        <div v-if="historyRows(quota).length" class="border-t pt-2">
          <p class="mb-1 text-xs text-[var(--text-secondary)]">
            {{ t("settings.ai.costGuard.historyTitle") }}
          </p>
          <ul class="space-y-1">
            <li
              v-for="row in historyRows(quota)"
              :key="row.timestamp"
              class="text-xs text-[var(--text-primary)]"
            >
              <span class="font-medium">{{ statusLabel(row.action) }}</span>
              ·
              {{ row.reason || t("settings.ai.costGuard.reasonFallback") }}
              <span class="text-[var(--text-secondary)]">
                ({{ formatTime(row.timestamp) }})
              </span>
            </li>
          </ul>
        </div>
      </UCard>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useDateFormat } from "@vueuse/core";
import { storeToRefs } from "pinia";
import { useCostQuotaStore } from "~/stores/costQuota";
import { useEnvStore } from "~/stores/envStore";

const quotaStore = useCostQuotaStore();
const envStore = useEnvStore();
const toast = useToast();
const { t, locale } = useI18n({ useScope: "global" });
const localePath = useLocalePath();

const currencyFormatter = computed(
  () =>
    new Intl.NumberFormat(
      locale.value === "zh" ? "zh-CN" : locale.value === "en" ? "en-US" : locale.value,
      {
        style: "currency",
        currency: "CNY",
        minimumFractionDigits: 2,
      }
    )
);

const tenantOptions = computed(() => {
  const fromConfig = Array.isArray(envStore.envConfig?.tenants)
    ? envStore.envConfig.tenants
    : [];
  if (fromConfig.length > 0) {
    return fromConfig
      .map((item: any) => {
        const value =
          item?.value ||
          item?.tenant_uuid ||
          item?.tenantUuid ||
          item?.id ||
          item?.slug ||
          "";
        if (!value) {
          return null;
        }
        return {
          label: item?.label || item?.name || value,
          value,
        };
      })
      .filter(Boolean) as { label: string; value: string }[];
  }
  return [
    {
      label: t("settings.ai.costGuard.defaultTenants.demo"),
      value: "demo-tenant",
    },
    {
      label: t("settings.ai.costGuard.defaultTenants.ops"),
      value: "ops-tenant",
    },
  ];
});

const tenantUuid = ref<string | null>(null);
const selectedAction = reactive<Record<string, string>>({});
const reasonInputs = reactive<Record<string, string>>({});
const pendingAction = ref<string | null>(null);

const { quotas, lastFetchedAt, error } = storeToRefs(quotaStore);
const lastUpdated = computed(() => lastFetchedAt.value);

const actionOptions = computed(() => [
  { label: t("settings.ai.costGuard.actions.throttle"), value: "throttle" },
  { label: t("settings.ai.costGuard.actions.degrade"), value: "degrade" },
  { label: t("settings.ai.costGuard.actions.disable"), value: "disable" },
]);

const resolvedTenantUuid = computed(
  () => tenantUuid.value || tenantOptions.value[0]?.value || ""
);

watchEffect(() => {
  const options = tenantOptions.value;
  if (!options.length) {
    tenantUuid.value = null;
    return;
  }
  if (
    !tenantUuid.value ||
    !options.some((opt) => opt.value === tenantUuid.value)
  ) {
    tenantUuid.value = options[0].value;
  }
});

const dashboardLink = computed(() =>
  resolvedTenantUuid.value
    ? localePath(
        `/dashboards/tenants/${encodeURIComponent(resolvedTenantUuid.value)}`
      )
    : null
);

const refresh = async () => {
  if (!resolvedTenantUuid.value) {
    return;
  }
  try {
    await quotaStore.fetchQuotas(
      envStore.currentEnv,
      resolvedTenantUuid.value
    );
  } catch (err: any) {
    toast.add({
      color: "red",
      title: t("settings.ai.costGuard.toast.loadFailedTitle"),
      description:
        err?.message || t("settings.ai.costGuard.toast.loadFailedDesc"),
    });
  }
};

const applyAction = async (quota: (typeof quotas.value)[number]) => {
  const action = selectedAction[quota.providerId] || "throttle";
  pendingAction.value = quota.providerId;
  try {
    await quotaStore.enforceAction({
      env: envStore.currentEnv,
      tenantUuid: resolvedTenantUuid.value,
      providerId: quota.providerId === "tenant" ? undefined : quota.providerId,
      action,
      reason: reasonInputs[quota.providerId],
      requestedBy: "web-admin",
    });
    toast.add({
      color: "green",
      title: t("settings.ai.costGuard.toast.actionSuccessTitle"),
      description: t("settings.ai.costGuard.toast.actionSuccessDesc", {
        provider: quota.providerId,
        action: statusLabel(action),
      }),
    });
    await refresh();
  } catch (err: any) {
    toast.add({
      color: "red",
      title: t("settings.ai.costGuard.toast.actionFailedTitle"),
      description:
        err?.message || t("settings.ai.costGuard.toast.actionFailedDesc"),
    });
  } finally {
    pendingAction.value = null;
  }
};

const statusLabel = (status: string) => {
  switch ((status || "").toLowerCase()) {
    case "breached":
    case "disable":
      return t("settings.ai.costGuard.status.breached");
    case "warning":
    case "degrade":
      return t("settings.ai.costGuard.status.warning");
    case "throttle":
      return t("settings.ai.costGuard.status.throttle");
    case "healthy":
      return t("settings.ai.costGuard.status.healthy");
    default:
      return t("settings.ai.costGuard.status.default");
  }
};

const statusColor = (status: string) => {
  switch (status) {
    case "breached":
    case "disable":
      return "red";
    case "warning":
    case "degrade":
      return "orange";
    case "throttle":
      return "yellow";
    default:
      return "green";
  }
};

const progressPct = (quota: (typeof quotas.value)[number]) => {
  if (!quota.limit || quota.limit <= 0) {
    return 0;
  }
  return Math.min(100, Number(((quota.usage / quota.limit) * 100).toFixed(2)));
};

const historyRows = (quota: (typeof quotas.value)[number]) => {
  return quota.enforcement?.history || [];
};

const formatCurrency = (value?: number) => {
  const amount = typeof value === "number" ? value : 0;
  return currencyFormatter.value.format(amount);
};

const formatTime = (value?: string) => {
  if (!value) return "";
  return useDateFormat(value, "MM-DD HH:mm").value;
};

watch(
  () => [envStore.currentEnv, resolvedTenantUuid.value],
  ([, tenant]) => {
    if (tenant) {
      refresh();
    }
  },
  { immediate: true }
);
</script>
