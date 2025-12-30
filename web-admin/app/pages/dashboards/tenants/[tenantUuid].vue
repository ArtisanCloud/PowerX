<template>
  <div class="p-6 space-y-6">
    <div
      class="flex flex-col gap-4 md:flex-row md:items-center md:justify-between"
    >
      <div>
        <h1 class="text-lg font-semibold text-[var(--text-primary)]">
          {{ t("dashboards.tenantCost.title", { tenant: tenantUuid }) }}
        </h1>
        <p class="text-sm text-[var(--text-secondary)]">
          {{ t("dashboards.tenantCost.environmentLabel", { env: envStore.currentEnvLabel }) }}
        </p>
      </div>
      <div class="flex items-center gap-3">
        <UBadge color="gray" v-if="lastSynced">
          {{ t("dashboards.tenantCost.lastSynced", { time: lastSynced }) }}
        </UBadge>
        <UButton
          icon="i-heroicons-arrow-path"
          :loading="loading"
          @click="refresh"
        >
          {{ t("common.refresh") }}
        </UButton>
      </div>
    </div>

    <UAlert
      v-if="error"
      icon="i-heroicons-exclamation-triangle"
      color="red"
      :title="t('dashboards.tenantCost.errorTitle')"
      :description="error || t('dashboards.tenantCost.errorDesc')"
    />

    <div class="grid gap-4 md:grid-cols-3">
      <UCard>
        <p class="text-xs text-[var(--text-secondary)]">
          {{ t("dashboards.tenantCost.cards.limit") }}
        </p>
        <p class="text-2xl font-semibold text-[var(--text-primary)]">
          <USkeleton v-if="loading" class="h-6 w-24" />
          <span v-else>{{ formatCurrency(totalLimit) }}</span>
        </p>
      </UCard>
      <UCard>
        <p class="text-xs text-[var(--text-secondary)]">
          {{ t("dashboards.tenantCost.cards.usage") }}
        </p>
        <p class="text-2xl font-semibold text-[var(--text-primary)]">
          <USkeleton v-if="loading" class="h-6 w-24" />
          <span v-else>{{ formatCurrency(totalUsage) }}</span>
        </p>
      </UCard>
      <UCard>
        <p class="text-xs text-[var(--text-secondary)]">
          {{ t("dashboards.tenantCost.cards.remaining") }}
        </p>
        <p class="text-2xl font-semibold text-[var(--text-primary)]">
          <USkeleton v-if="loading" class="h-6 w-24" />
          <span v-else>{{ formatCurrency(remainingBudget) }}</span>
        </p>
      </UCard>
    </div>

    <div class="grid gap-4 md:grid-cols-2">
      <UCard>
        <template #header>
          <div class="flex items-center justify-between">
            <span>{{ t("dashboards.tenantCost.tables.quota") }}</span>
            <UBadge color="red" v-if="breachedCount > 0">
              {{ t("dashboards.tenantCost.badges.breached", { count: breachedCount }) }}
            </UBadge>
            <UBadge color="orange" v-else-if="warningCount > 0">
              {{ t("dashboards.tenantCost.badges.warning", { count: warningCount }) }}
            </UBadge>
          </div>
        </template>
        <div v-if="loading" class="space-y-2">
          <USkeleton class="h-10" v-for="n in 3" :key="n" />
        </div>
        <UTable v-else :rows="quotaRows" :columns="quotaColumns">
          <template #status-data="{ row }">
            <UBadge :color="statusColor(row.status)">
              {{ statusLabel(row.status) }}
            </UBadge>
          </template>
        </UTable>
      </UCard>

      <UCard>
        <template #header>
          {{ t("dashboards.tenantCost.tables.history") }}
        </template>
        <div v-if="loading" class="space-y-2">
          <USkeleton class="h-10" v-for="n in 4" :key="n" />
        </div>
        <UTable
          v-else-if="historyRows.length"
          :rows="historyRows"
          :columns="historyColumns"
        />
        <div v-else class="text-sm text-[var(--text-secondary)]">
          {{ t("dashboards.tenantCost.historyEmpty") }}
        </div>
      </UCard>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useRoute } from "vue-router";
import { storeToRefs } from "pinia";
import { useDateFormat } from "@vueuse/core";
import { useCostQuotaStore } from "~/stores/costQuota";
import { useEnvStore } from "~/stores/envStore";

definePageMeta({
  layout: "admin",
  middleware: "admin-only",
});

const route = useRoute();
const envStore = useEnvStore();
const toast = useToast();
const costStore = useCostQuotaStore();
const { quotas, loading, error, lastFetchedAt } = storeToRefs(costStore);
const { t, locale } = useI18n({ useScope: "global" });

const tenantUuid = computed(() => String(route.params.tenantUuid || ""));

const quotaColumns = computed(() => [
  { key: "provider", label: t("dashboards.tenantCost.columns.provider") },
  { key: "limit", label: t("dashboards.tenantCost.columns.limit") },
  { key: "usage", label: t("dashboards.tenantCost.columns.usage") },
  { key: "status", label: t("dashboards.tenantCost.columns.status") },
]);

const quotaRows = computed(() =>
  quotas.value.map((quota) => ({
    provider: quota.providerId,
    limit: formatCurrency(quota.limit),
    usage: formatCurrency(quota.usage),
    status: quota.status,
  }))
);

const historyColumns = computed(() => [
  { key: "timestamp", label: t("dashboards.tenantCost.columns.timestamp") },
  { key: "action", label: t("dashboards.tenantCost.columns.action") },
  { key: "reason", label: t("dashboards.tenantCost.columns.reason") },
  { key: "actor", label: t("dashboards.tenantCost.columns.actor") },
]);

const historyRows = computed(() =>
  quotas.value
    .flatMap((quota) => quota.enforcement?.history || [])
    .map((item) => ({
      timestamp: formatDate(item.timestamp),
      action: statusLabel(item.action),
      reason: item.reason || t("dashboards.tenantCost.reasonFallback"),
      actor: item.requestedBy || t("dashboards.tenantCost.actorFallback"),
    }))
    .sort((a, b) => (a.timestamp > b.timestamp ? -1 : 1))
);

const totalLimit = computed(() =>
  quotas.value.reduce((sum, q) => sum + (q.limit || 0), 0)
);
const totalUsage = computed(() =>
  quotas.value.reduce((sum, q) => sum + (q.usage || 0), 0)
);
const remainingBudget = computed(() =>
  Math.max(totalLimit.value - totalUsage.value, 0)
);
const breachedCount = computed(
  () => quotas.value.filter((q) => q.status === "breached").length
);
const warningCount = computed(
  () => quotas.value.filter((q) => q.status === "warning").length
);

const lastSynced = computed(() => {
  if (!lastFetchedAt.value) return null;
  return useDateFormat(lastFetchedAt.value, "MM-DD HH:mm").value;
});

const refresh = async () => {
  if (!tenantUuid.value) return;
  try {
    await costStore.fetchQuotas(envStore.currentEnv, tenantUuid.value);
  } catch (err: any) {
    toast.add({
      color: "red",
      title: t("dashboards.tenantCost.errorTitle"),
      description: err?.message || t("dashboards.tenantCost.errorDesc"),
    });
  }
};

watch(
  () => [envStore.currentEnv, tenantUuid.value],
  () => {
    refresh();
  },
  { immediate: true }
);

const currencyFormatter = computed(
  () =>
    new Intl.NumberFormat(
      locale.value === "zh" ? "zh-CN" : locale.value === "en" ? "en-US" : locale.value,
      { style: "currency", currency: "CNY", minimumFractionDigits: 2 }
    )
);

const formatCurrency = (value?: number) =>
  currencyFormatter.value.format(typeof value === "number" ? value : 0);

const formatDate = (value?: string) => {
  if (!value) return "--";
  return useDateFormat(value, "MM-DD HH:mm").value;
};

const statusLabel = (status?: string) => {
  switch ((status || "").toLowerCase()) {
    case "breached":
      return t("dashboards.tenantCost.status.breached");
    case "warning":
      return t("dashboards.tenantCost.status.warning");
    case "throttle":
      return t("dashboards.tenantCost.status.throttle");
    case "degrade":
      return t("dashboards.tenantCost.status.degrade");
    case "disable":
      return t("dashboards.tenantCost.status.disable");
    default:
      return t("dashboards.tenantCost.status.healthy");
  }
};

const statusColor = (status?: string) => {
  switch ((status || "").toLowerCase()) {
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
</script>
