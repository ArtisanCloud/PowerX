<template>
  <div class="space-y-6 p-4">
    <div
      class="flex flex-col gap-4 md:flex-row md:items-center md:justify-between"
    >
      <div>
        <h1 class="text-lg font-semibold text-[var(--text-primary)]">
          {{ $t("settings.ai.registryPageTitle") }}
        </h1>
        <p class="text-sm text-[var(--text-secondary)]">
          {{ $t("settings.ai.registryPageDesc") }}
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
        <UButton
          icon="i-heroicons-arrow-path"
          :loading="loading || jobsLoading"
          @click="refreshAll"
        >
          {{ $t("common.reload") }}
        </UButton>
      </div>
    </div>

    <UAlert
      v-if="!allowAccess"
      icon="i-heroicons-lock-closed"
      color="amber"
      variant="subtle"
      :title="$t('settings.ai.registry.noAccess.title')"
      :description="$t('settings.ai.registry.noAccess.description')"
    />

    <div v-else class="space-y-6">
      <div
        class="rounded-lg border border-[var(--border-color)] bg-[var(--card-bg)] p-4 space-y-3"
      >
        <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-5 gap-3">
          <UInput
            v-model="filters.search"
            icon="i-heroicons-magnifying-glass"
            :placeholder="$t('settings.ai.registry.filters.searchPlaceholder')"
          />
          <UInput
            v-model="filters.pluginId"
            icon="i-heroicons-puzzle-piece"
            :placeholder="$t('settings.ai.registry.filters.pluginPlaceholder')"
          />
          <USelect
            v-model="filters.status"
            :items="statusOptions"
            :placeholder="$t('settings.ai.registry.filters.statusAll')"
          />
          <USelect
            v-model="filters.protocol"
            :items="protocolOptions"
            :placeholder="$t('settings.ai.registry.filters.protocolAll')"
          />
          <USelect
            v-model="filters.source"
            :items="sourceOptions"
            :placeholder="$t('settings.ai.registry.filters.sourcePlugin')"
          />
        </div>
        <div class="flex gap-2">
          <UButton size="sm" @click="applyFilters">
            {{ $t("settings.ai.registry.filters.apply") }}
          </UButton>
          <UButton size="sm" variant="ghost" @click="resetFilters">
            {{ $t("settings.ai.registry.filters.reset") }}
          </UButton>
        </div>
      </div>

      <UCard>
        <template #header>
          <div class="flex items-center justify-between">
            <span class="text-sm text-[var(--text-secondary)]">
              {{
                $t("settings.ai.registry.summary", {
                  total: pagination.total,
                })
              }}
            </span>
            <span class="text-xs text-[var(--text-tertiary)]">
              {{ pagination.page }} / {{ pagination.pages || 1 }}
            </span>
          </div>
        </template>

        <UTable
          :columns="columns"
          :data="rows"
          :loading="loading"
          row-key="capabilityId"
          :ui="{
            divide: 'divide-y divide-[var(--border-color)]',
          }"
        />

        <template #footer>
          <div class="flex items-center justify-between">
            <div class="text-xs text-[var(--text-secondary)]">
              {{ $t("settings.ai.registry.summary", { total: pagination.total })
              }}
            </div>
            <UPagination
              v-model:page="pagination.page"
              :total="pagination.total"
              :items-per-page="pagination.pageSize"
              :max="7"
              @update:page="handlePageChange"
            />
          </div>
        </template>
      </UCard>

      <UCard>
        <template #header>
          <div class="flex items-center justify-between">
            <span class="font-medium text-[var(--text-primary)]">
              {{ $t("settings.ai.registry.syncJobs.title") }}
            </span>
            <UButton
              icon="i-heroicons-arrow-path"
              variant="ghost"
              size="xs"
              :loading="jobsLoading"
              @click="fetchSyncJobs"
            >
              {{ $t("common.reload") }}
            </UButton>
          </div>
        </template>

        <div v-if="jobsLoading" class="flex items-center gap-2 text-sm">
          <UIcon
            name="i-heroicons-arrow-path"
            class="h-4 w-4 animate-spin text-primary-500"
          />
          <span>{{ $t("common.reload") }}...</span>
        </div>
        <div
          v-else-if="jobs.length === 0"
          class="text-sm text-[var(--text-secondary)]"
        >
          {{ $t("settings.ai.registry.syncJobs.empty") }}
        </div>
        <ul v-else class="space-y-3">
          <li
            v-for="job in jobs"
            :key="job.jobId"
            class="rounded-lg border border-[var(--border-color)] p-3"
          >
            <div class="flex flex-wrap items-center gap-2">
              <span class="font-medium text-[var(--text-primary)]">
                {{ job.pluginId }} · {{ job.pluginVersion }}
              </span>
              <UBadge
                :color="statusColor(job.status)"
                variant="subtle"
                size="xs"
              >
                {{ jobStatusLabel(job.status) }}
              </UBadge>
              <span
                v-if="job.capabilityId"
                class="text-xs text-[var(--text-secondary)]"
              >
                {{ job.capabilityId }}
              </span>
            </div>
            <div
              class="mt-1 text-xs text-[var(--text-secondary)] space-y-1 md:flex md:items-center md:justify-between md:space-y-0"
            >
              <span>
                {{ formatDate(job.startedAt) }}
                <template v-if="job.finishedAt">
                  → {{ formatDate(job.finishedAt) }}
                </template>
              </span>
              <span v-if="job.errorSummary" class="text-red-500">
                {{ job.errorSummary }}
              </span>
            </div>
          </li>
        </ul>
        <p
          v-if="jobs.length > 0"
          class="mt-2 text-xs text-[var(--text-tertiary)]"
        >
          {{
            $t("settings.ai.registry.syncJobs.more", {
              count: maxJobDisplay,
            })
          }}
        </p>
      </UCard>
    </div>
  </div>
</template>

<script setup lang="ts">
import { h, resolveComponent } from "vue";
import { storeToRefs } from "pinia";
import {
  CapabilityRegistryService,
  type CapabilityRecord,
  type CapabilitySyncJob,
  type ProtocolBinding,
} from "~/composables/api/services/capabilityRegistryService";
import { useUserStore } from "~/stores/user";

definePageMeta({
  title: "能力注册表",
  layout: "default",
});

const { t, locale } = useI18n({ useScope: "global" });
const localePath = useLocalePath();
const toast = useToast();

const userStore = useUserStore();
const { isRoot } = storeToRefs(userStore);

const modelSettingsLink = computed(() => localePath("/settings/ai"));
const allowAccess = computed(() => isRoot.value);

const loading = ref(false);
const jobsLoading = ref(false);
const rows = ref<CapabilityRecord[]>([]);
const jobs = ref<CapabilitySyncJob[]>([]);
const pagination = reactive({
  page: 1,
  pageSize: 20,
  total: 0,
  pages: 0,
});
const maxJobDisplay = 10;

const filters = reactive({
  search: "",
  pluginId: "",
  status: "all",
  protocol: "all",
  source: "plugin",
});

const statusOptions = computed(() => [
  {
    label: t("settings.ai.registry.filters.statusAll"),
    value: "all",
  },
  { label: "published", value: "published" },
  { label: "disabled", value: "disabled" },
  { label: "draft", value: "draft" },
]);

const protocolOptions = computed(() => [
  {
    label: t("settings.ai.registry.filters.protocolAll"),
    value: "all",
  },
  { label: "mcp", value: "mcp" },
  { label: "grpc", value: "grpc" },
  { label: "rest", value: "rest" },
  { label: "workflow", value: "workflow" },
  { label: "composite", value: "composite" },
]);

const sourceOptions = computed(() => [
  {
    label: t("settings.ai.registry.filters.sourcePlugin"),
    value: "plugin",
  },
  {
    label: t("settings.ai.registry.filters.sourcePlatform"),
    value: "corex",
  },
  {
    label: t("settings.ai.registry.filters.sourceAll"),
    value: "all",
  },
]);

const UBadge = resolveComponent("UBadge");

const columns = computed(() => {
  locale.value;
  return [
    {
    id: "capability",
    header: t("settings.ai.registry.columns.capability").toString(),
    cell: ({ row }: any) =>
      h("div", { class: "space-y-1" }, [
        h(
          "div",
          { class: "text-sm font-semibold text-[var(--text-primary)]" },
          row.original.title || row.original.capabilityId
        ),
        h(
          "div",
          { class: "text-xs text-[var(--text-secondary)]" },
          row.original.capabilityId
        ),
        row.original.intents?.length
          ? h(
              "div",
              { class: "text-[11px] text-[var(--text-tertiary)]" },
              `Intent: ${row.original.intents.slice(0, 3).join(", ")}`
            )
          : null,
      ]),
    },
    {
    id: "plugin",
    header: t("settings.ai.registry.columns.plugin").toString(),
    cell: ({ row }: any) =>
      h("div", { class: "space-y-1" }, [
        h(
          "div",
          { class: "text-sm font-medium text-[var(--text-primary)]" },
          row.original.pluginId
        ),
        h(
          "div",
          { class: "text-xs text-[var(--text-secondary)]" },
          row.original.pluginVersion || "-"
        ),
        h(
          UBadge,
          {
            color: statusColor(row.original.status),
            variant: "subtle",
            size: "xs",
          },
          () => row.original.status || "-"
        ),
      ]),
    },
    {
    id: "policy",
    header: t("settings.ai.registry.columns.policy").toString(),
    cell: ({ row }: any) => {
      const policy = row.original.policy;
      if (!policy) {
        return h(
          "span",
          { class: "text-xs text-[var(--text-tertiary)]" },
          "-"
        );
      }
      return h("div", { class: "text-xs space-y-1" }, [
        h(
          "div",
          { class: "text-[var(--text-secondary)]" },
          `Prefer: ${policy.prefer || "-"}`
        ),
        policy.fallback?.length
          ? h(
              "div",
              { class: "text-[var(--text-tertiary)]" },
              `Fallback: ${policy.fallback.join(", ")}`
            )
          : null,
      ]);
    },
    },
    {
    id: "protocols",
    header: t("settings.ai.registry.columns.protocols").toString(),
    cell: ({ row }: any) =>
      h(
        "div",
        { class: "flex flex-wrap gap-1" },
        (row.original.protocols || []).map((binding: ProtocolBinding) =>
          h(
            UBadge,
            {
              key: `${row.original.capabilityId}-${binding.channel}`,
              variant: "outline",
              size: "xs",
            },
            () => binding.channel
          )
        )
      ),
    },
    {
    id: "hash",
    header: t("settings.ai.registry.columns.hash").toString(),
    cell: ({ row }: any) =>
      h("div", { class: "text-xs space-y-1" }, [
        h(
          "div",
          { class: "text-[var(--text-secondary)]" },
          truncateHash(row.original.capabilitiesHash)
        ),
        h(
          "div",
          { class: "text-[var(--text-tertiary)]" },
          truncateHash(row.original.protocolHash)
        ),
      ]),
    },
    {
    id: "publishedAt",
    header: t("settings.ai.registry.columns.publishedAt").toString(),
    cell: ({ row }: any) =>
      h("div", { class: "text-xs text-[var(--text-secondary)] space-y-1" }, [
        h("div", null, formatDate(row.original.publishedAt)),
        needsManualUpgrade(row.original)
          ? h(
              UBadge,
              { color: "amber", variant: "subtle", size: "xs" },
              () => t("settings.ai.registry.manualUpgrade")
            )
          : null,
      ]),
    },
  ];
});

const truncateHash = (value?: string) => {
  if (!value) return "-";
  return value.length > 12 ? `${value.slice(0, 12)}…` : value;
};

const formatDate = (value?: string) => {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
};

const needsManualUpgrade = (record: CapabilityRecord) =>
  !!record.workflowTemplates?.some((tpl) => tpl.requiresManualUpgrade);

const statusColor = (status?: string) => {
  switch (status?.toLowerCase()) {
    case "published":
      return "success";
    case "disabled":
      return "gray";
    case "failed":
      return "error";
    default:
      return "neutral";
  }
};

const jobStatusLabel = (status: string) => {
  const key = status.toLowerCase() as keyof typeof jobStatusMap;
  return (
    jobStatusMap[key] ||
    t(`settings.ai.registry.syncJobs.status.${key}`, status)
  );
};

const jobStatusMap: Record<string, string> = {
  pending: "pending",
  running: "running",
  succeeded: "succeeded",
  failed: "failed",
};

const handlePageChange = (page: number) => {
  pagination.page = page;
  fetchCapabilities();
};

const applyFilters = () => {
  pagination.page = 1;
  fetchCapabilities();
  fetchSyncJobs();
};

const resetFilters = () => {
  filters.search = "";
  filters.pluginId = "";
  filters.protocol = "all";
  filters.status = "all";
  filters.source = "plugin";
  applyFilters();
};

const refreshAll = () => {
  fetchCapabilities();
  fetchSyncJobs();
};

const listParams = computed(() => ({
  page: pagination.page,
  pageSize: pagination.pageSize,
  pluginId: filters.pluginId?.trim() || undefined,
  search: filters.search?.trim() || undefined,
  protocol: filters.protocol !== "all" ? filters.protocol : undefined,
  source: filters.source !== "all" ? filters.source : undefined,
  status:
    filters.status !== "all" && filters.status
      ? [filters.status]
      : undefined,
}));

async function fetchCapabilities() {
  if (!allowAccess.value) return;
  loading.value = true;
  try {
    const result = await CapabilityRegistryService.listCapabilities({
      ...listParams.value,
      includeWorkflows: true,
    });
    rows.value = result.items;
    if (result.pagination) {
      pagination.total = result.pagination.total;
      pagination.pages = result.pagination.pages || 0;
    }
  } catch (error: any) {
    console.error("load capabilities failed", error);
    toast.add({
      title: t("settings.ai.registry.toast.loadFailed"),
      description: error?.message,
      color: "red",
    });
  } finally {
    loading.value = false;
  }
}

async function fetchSyncJobs() {
  if (!allowAccess.value) return;
  jobsLoading.value = true;
  try {
    const result = await CapabilityRegistryService.listSyncJobs({
      page: 1,
      pageSize: maxJobDisplay,
      pluginId: filters.pluginId?.trim() || undefined,
      capabilityId: undefined,
    });
    jobs.value = result.items;
  } catch (error: any) {
    console.error("load sync jobs failed", error);
    toast.add({
      title: t("settings.ai.registry.toast.jobsFailed"),
      description: error?.message,
      color: "red",
    });
  } finally {
    jobsLoading.value = false;
  }
}

onMounted(async () => {
  if (!userStore.context) {
    try {
      await userStore.fetchUserContext();
    } catch (error) {
      console.error("fetch user context failed", error);
    }
  }
  if (allowAccess.value) {
    refreshAll();
  }
});

watch(
  () => allowAccess.value,
  (val) => {
    if (val) {
      refreshAll();
    }
  }
);
</script>
