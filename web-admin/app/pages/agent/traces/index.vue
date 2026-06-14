<template>
  <div class="space-y-5 p-6">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <h1 class="text-xl font-semibold text-gray-900">Agent 运行追踪</h1>
        <p class="text-sm text-gray-500">Root 调试 Agent Session、Message 与节点链路。</p>
      </div>
      <div class="flex items-center gap-2">
        <UButton icon="i-heroicons-arrow-down-tray" variant="soft" :disabled="!report" :to="jsonDownloadUrl" target="_blank">JSON</UButton>
        <UButton icon="i-heroicons-document-arrow-down" variant="soft" :disabled="!report" :to="markdownDownloadUrl" target="_blank">报告</UButton>
      </div>
    </div>

    <UAlert
      v-if="!isRoot"
      color="error"
      variant="soft"
      title="仅 Root 可查看 Agent 运行追踪"
      description="当前账号没有 Root 权限。"
    />

    <div v-else class="grid gap-5">
      <div class="rounded-lg border border-gray-200 bg-white p-4">
        <div class="grid gap-3 md:grid-cols-[1fr_1fr_1fr_auto]">
          <UFormField label="Tenant UUID">
            <USelectMenu
              v-model="query.tenant_uuid"
              v-model:search-term="tenantSearch"
              :items="tenantOptions"
              searchable
              value-key="value"
              label-key="label"
              :loading="tenantLoading"
              :portal="false"
              class="w-full"
              placeholder="搜索并选择租户"
            />
          </UFormField>
          <UFormField label="Session ID">
            <UInput v-model="query.session_id" placeholder="session id" />
          </UFormField>
          <UFormField label="Message ID">
            <UInput v-model="query.message_id" placeholder="message id" />
          </UFormField>
          <div class="flex items-end">
            <UButton icon="i-heroicons-magnifying-glass" :loading="loading" @click="loadTrace">查询</UButton>
          </div>
        </div>
      </div>

      <div class="grid gap-3 md:grid-cols-4">
        <div v-for="card in metricCards" :key="card.label" class="rounded-lg border border-gray-200 bg-white p-4">
          <div class="text-xs text-gray-500">{{ card.label }}</div>
          <div class="mt-2 truncate text-2xl font-semibold text-gray-900">{{ card.value }}</div>
        </div>
      </div>

      <div class="grid gap-5 xl:grid-cols-[minmax(0,1fr)_420px]">
        <TraceTimeline :events="timeline" @select="selectNode" />
        <TraceNodeDetails :node="selectedNode" />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import TraceNodeDetails from "~/components/agent/trace/TraceNodeDetails.vue";
import TraceTimeline from "~/components/agent/trace/TraceTimeline.vue";
import { useAgentTraceService } from "~/composables/api/services/agentTraceService";
import { useTenantService, type Tenant } from "~/composables/api/services/tenantService";
import type { AgentTraceNode, AgentTraceQuery, AgentTraceReport } from "~/composables/api/types/agentTrace";
import { useUserStore } from "~/stores/user";

const userStore = useUserStore();
const isRoot = computed(() => userStore.isRoot);
const service = useAgentTraceService();
const tenantService = useTenantService();

const query = reactive<AgentTraceQuery>({
  tenant_uuid: "",
  session_id: "",
  message_id: "",
});
const loading = ref(false);
const tenantLoading = ref(false);
const tenantSearch = ref("");
const tenants = ref<Tenant[]>([]);
const report = ref<AgentTraceReport | null>(null);
const selectedNodeID = ref("");

const tenantOptions = computed(() => {
  const keyword = tenantSearch.value.trim().toLowerCase();
  return tenants.value
    .filter((tenant) => {
      if (!keyword) return true;
      return [
        tenant.name,
        tenant.uuid,
        tenant.domain,
      ]
        .filter(Boolean)
        .some((value) => String(value).toLowerCase().includes(keyword));
    })
    .map((tenant) => ({
      label: tenant.name ? `${tenant.name} (${tenant.uuid.slice(0, 8)})` : tenant.uuid,
      value: tenant.uuid,
    }));
});

const timeline = computed(() => report.value?.timeline || []);
const nodes = computed(() => report.value?.nodes || []);
const selectedNode = computed<AgentTraceNode | null>(() => nodes.value.find((node) => node.node_id === selectedNodeID.value) || nodes.value[0] || null);

const metricCards = computed(() => [
  { label: "状态", value: String(report.value?.summary?.status || "-") },
  { label: "节点", value: String(report.value?.nodes?.length || 0) },
  { label: "事件", value: String(report.value?.timeline?.length || 0) },
  { label: "错误", value: String(report.value?.errors?.length || 0) },
]);

const jsonDownloadUrl = computed(() => (report.value ? service.downloadUrl(query, "json") : undefined));
const markdownDownloadUrl = computed(() => (report.value ? service.downloadUrl(query, "markdown") : undefined));

const loadTrace = async () => {
  loading.value = true;
  try {
    report.value = await service.getReport({ ...query });
    selectedNodeID.value = report.value.nodes?.[0]?.node_id || "";
  } finally {
    loading.value = false;
  }
};

const selectNode = (nodeId: string) => {
  selectedNodeID.value = nodeId;
};

const loadTenants = async () => {
  tenantLoading.value = true;
  try {
    const response = await tenantService.getTenants({ page: 1, page_size: 100 });
    tenants.value = response.data?.items || [];
    if (!query.tenant_uuid) {
      query.tenant_uuid = userStore.currentTenantUuid || tenants.value[0]?.uuid || "";
    }
  } finally {
    tenantLoading.value = false;
  }
};

onMounted(() => {
  void loadTenants();
});
</script>
