<template>
  <div class="space-y-5 p-6">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <h1 class="text-xl font-semibold text-gray-900">Agent 运行追踪</h1>
        <p class="text-sm text-gray-500">Root 调试 Agent 会话、单轮消息与节点执行链路。</p>
      </div>
      <div class="flex items-center gap-2">
        <UButton v-if="isDetailMode" icon="i-heroicons-arrow-down-tray" variant="soft" :disabled="!report" @click="openDownload(jsonDownloadUrl)">单轮 JSON</UButton>
        <UButton v-if="isDetailMode" icon="i-heroicons-document-arrow-down" variant="soft" :disabled="!report" @click="openDownload(markdownDownloadUrl)">单轮报告</UButton>
        <UButton v-if="query.session_id" icon="i-heroicons-archive-box-arrow-down" variant="soft" :disabled="!sessionMarkdownDownloadUrl" @click="openDownload(sessionMarkdownDownloadUrl)">会话报告</UButton>
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
        <div class="grid gap-3 md:grid-cols-[minmax(260px,1fr)_180px_auto_auto]">
          <UFormField label="租户">
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
          <UFormField v-if="!isDetailMode" label="状态">
            <USelect v-model="runStatus" :items="runStatusItems" class="w-full" @change="handleStatusChange" />
          </UFormField>
          <div class="flex items-end">
            <UButton
              v-if="isDetailMode"
              icon="i-heroicons-arrow-left"
              variant="soft"
              @click="backToSessionRuns"
            >
              返回消息
            </UButton>
            <UButton
              v-else-if="isSessionMode"
              icon="i-heroicons-arrow-left"
              variant="soft"
              @click="backToList"
            >
              返回会话
            </UButton>
            <UButton
              v-else
              icon="i-heroicons-arrow-path"
              variant="soft"
              :loading="sessionsLoading"
              @click="loadSessions"
            >
              刷新会话
            </UButton>
          </div>
          <div class="flex items-end">
            <UButton v-if="isDetailMode" icon="i-heroicons-arrow-path" :loading="loading" @click="loadTrace">刷新详情</UButton>
            <UButton v-else-if="isSessionMode" icon="i-heroicons-arrow-path" :loading="runsLoading" @click="loadRuns">刷新消息</UButton>
          </div>
        </div>
      </div>

      <div v-if="!isDetailMode && !isSessionMode" class="rounded-lg border border-gray-200 bg-white p-4">
        <div class="mb-3 flex items-center justify-between gap-3">
          <div>
            <div class="text-sm font-semibold text-gray-900">会话追踪</div>
            <div class="text-xs text-gray-500">一行代表一个 Agent 会话；点击“查看”进入该会话的消息运行列表。</div>
          </div>
        </div>
        <div v-if="sessions.length" class="overflow-hidden rounded-md border border-gray-200">
          <table class="min-w-full divide-y divide-gray-200 text-sm">
            <thead class="bg-gray-50 text-xs font-medium text-gray-500">
              <tr>
                <th class="px-3 py-2 text-left">最近运行</th>
                <th class="px-3 py-2 text-left">Agent</th>
                <th class="px-3 py-2 text-left">状态</th>
                <th class="px-3 py-2 text-left">消息/节点/错误</th>
                <th class="px-3 py-2 text-left">耗时</th>
                <th class="px-3 py-2 text-left">会话标识</th>
                <th class="px-3 py-2 text-right">操作</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-200 bg-white">
              <tr
                v-for="item in sessions"
                :key="item.session_id"
                class="hover:bg-gray-50"
              >
                <td class="whitespace-nowrap px-3 py-2 text-gray-700">
                  {{ formatTraceTime(item.latest_at) }}
                </td>
                <td class="px-3 py-2 font-medium text-gray-900">{{ item.agent_id || 'Agent' }}</td>
                <td class="px-3 py-2">
                  <UBadge size="xs" :color="item.status === 'failed' ? 'error' : 'success'" variant="soft">
                    {{ item.status === 'failed' ? '失败' : '完成' }}
                  </UBadge>
                </td>
                <td class="px-3 py-2 text-gray-600">
                  {{ item.message_count || 0 }} / {{ item.node_count || 0 }} / {{ item.error_count || 0 }}
                </td>
                <td class="px-3 py-2 text-gray-600">
                  {{ item.duration_ms || 0 }}ms
                </td>
                <td class="px-3 py-2 font-mono text-xs text-gray-500">{{ shortID(item.session_id) }}</td>
                <td class="px-3 py-2 text-right">
                  <UButton size="xs" variant="ghost" icon="i-heroicons-eye" @click="selectSession(item)">查看</UButton>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <div v-if="sessions.length" class="mt-3 flex flex-wrap items-center justify-between gap-3">
          <div class="text-xs text-gray-500">
            共 {{ sessionsTotal }} 个会话，第 {{ sessionsPage }} / {{ sessionsPageCount }} 页
          </div>
          <div class="flex items-center gap-2">
            <USelect v-model="sessionsPageSize" :items="pageSizeItems" class="w-24" @change="handleSessionPageSizeChange" />
            <UPagination
              v-model:page="sessionsPage"
              :total="sessionsTotal"
              :items-per-page="sessionsPageSize"
              :sibling-count="1"
              show-edges
              @update:page="loadSessions"
            />
          </div>
        </div>
        <div v-else class="rounded-md bg-gray-50 p-4 text-sm text-gray-500">
          当前租户暂无可展示会话追踪。切换租户后点“刷新会话”，或从新消息的“运行追踪”入口进入。
        </div>
      </div>

      <div v-if="isSessionMode" class="rounded-lg border border-gray-200 bg-white p-4">
        <div class="mb-3 flex items-center justify-between gap-3">
          <div>
            <div class="text-sm font-semibold text-gray-900">会话消息运行</div>
            <div class="text-xs text-gray-500">当前会话内的每轮消息运行；点击“查看”进入单轮节点详情。</div>
          </div>
          <div class="flex items-center gap-2">
            <UButton size="sm" variant="soft" icon="i-heroicons-arrow-left" @click="backToList">返回会话</UButton>
          </div>
        </div>
        <div v-if="runs.length" class="overflow-hidden rounded-md border border-gray-200">
          <table class="min-w-full divide-y divide-gray-200 text-sm">
            <thead class="bg-gray-50 text-xs font-medium text-gray-500">
              <tr>
                <th class="px-3 py-2 text-left">时间</th>
                <th class="px-3 py-2 text-left">状态</th>
                <th class="px-3 py-2 text-left">消息前缀</th>
                <th class="px-3 py-2 text-left">节点/事件/错误</th>
                <th class="px-3 py-2 text-left">耗时</th>
                <th class="px-3 py-2 text-right">操作</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-200 bg-white">
              <tr
                v-for="item in runs"
                :key="`${item.session_id}-${item.message_id}-${item.run_id}`"
                class="hover:bg-gray-50"
              >
                <td class="whitespace-nowrap px-3 py-2 text-gray-700">
                  {{ formatTraceTime(item.ended_at || item.started_at || item.created_at) }}
                </td>
                <td class="px-3 py-2">
                  <UBadge size="xs" :color="item.status === 'failed' ? 'error' : 'success'" variant="soft">
                    {{ item.status === 'failed' ? '失败' : '完成' }}
                  </UBadge>
                </td>
                <td class="max-w-[520px] px-3 py-2">
                  <div class="truncate text-gray-800">
                    {{ item.message_preview || '旧追踪未关联消息内容' }}
                  </div>
                  <div class="mt-1 flex flex-wrap items-center gap-2 text-xs text-gray-500">
                    <UBadge v-if="item.message_role" size="xs" color="neutral" variant="soft">{{ formatMessageRole(item.message_role) }}</UBadge>
                    <span class="font-mono">{{ shortID(item.message_id) }}</span>
                  </div>
                </td>
                <td class="px-3 py-2 text-gray-600">
                  {{ item.node_count || 0 }} / {{ item.event_count || 0 }} / {{ item.error_count || 0 }}
                </td>
                <td class="px-3 py-2 text-gray-600">{{ item.duration_ms || 0 }}ms</td>
                <td class="px-3 py-2 text-right">
                  <UButton size="xs" variant="ghost" icon="i-heroicons-eye" @click="selectRun(item)">查看</UButton>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <div v-if="runs.length" class="mt-3 flex flex-wrap items-center justify-between gap-3">
          <div class="text-xs text-gray-500">
            共 {{ runsTotal }} 条消息运行，第 {{ runsPage }} / {{ runsPageCount }} 页
          </div>
          <div class="flex items-center gap-2">
            <USelect v-model="runsPageSize" :items="pageSizeItems" class="w-24" @change="handlePageSizeChange" />
            <UPagination
              v-model:page="runsPage"
              :total="runsTotal"
              :items-per-page="runsPageSize"
              :sibling-count="1"
              show-edges
              @update:page="loadRuns"
            />
          </div>
        </div>
        <div v-else class="rounded-md bg-gray-50 p-4 text-sm text-gray-500">
          当前会话没有可展示运行追踪。该会话可能是在 Agent Trace 启用前产生，或当时后端未写入真实
          session/message 追踪上下文；请在当前会话重新发送一条消息后再刷新。
        </div>
      </div>

      <div v-if="isDetailMode && report" class="rounded-lg border border-gray-200 bg-white p-4">
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div>
            <div class="text-sm font-semibold text-gray-900">单轮运行详情</div>
            <div class="mt-1 text-xs text-gray-500">
              会话 {{ shortID(query.session_id) }} · 消息 {{ shortID(query.message_id) }}
            </div>
          </div>
          <UBadge :color="String(report.summary?.status || '').toLowerCase() === 'failed' ? 'error' : 'success'" variant="soft">
            {{ String(report.summary?.status || '-') }}
          </UBadge>
        </div>
      </div>

      <div v-if="isDetailMode" class="grid gap-3 md:grid-cols-4">
        <div v-for="card in metricCards" :key="card.label" class="rounded-lg border border-gray-200 bg-white p-4">
          <div class="text-xs text-gray-500">{{ card.label }}</div>
          <div class="mt-2 truncate text-2xl font-semibold text-gray-900">{{ card.value }}</div>
        </div>
      </div>

      <div v-if="isDetailMode" class="rounded-lg border border-gray-200 bg-white p-4">
        <div class="mb-3 flex flex-wrap items-start justify-between gap-3">
          <div>
            <div class="text-sm font-semibold text-gray-900">上下文状态</div>
            <div class="mt-1 text-xs text-gray-500">
              当前消息的 Response Plan、上下文层、能力范围与模型选择。
            </div>
          </div>
          <UBadge v-if="contextState.responseMode" size="xs" color="primary" variant="soft">
            {{ formatResponseMode(contextState.responseMode) }}
          </UBadge>
        </div>
        <div v-if="hasContextState" class="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
          <div class="rounded-md border border-gray-100 bg-gray-50 p-3">
            <div class="text-[11px] font-medium text-gray-500">Response Mode</div>
            <div class="mt-1 truncate text-sm font-semibold text-gray-900">
              {{ formatResponseMode(contextState.responseMode) || '-' }}
            </div>
            <div v-if="contextState.responsePlanID" class="mt-1 font-mono text-[11px] text-gray-500">
              {{ shortID(contextState.responsePlanID) }}
            </div>
          </div>
          <div class="rounded-md border border-gray-100 bg-gray-50 p-3">
            <div class="text-[11px] font-medium text-gray-500">Context Layers</div>
            <div class="mt-2 flex flex-wrap gap-1">
              <UBadge
                v-for="layer in contextState.usedContextLayers"
                :key="layer"
                size="xs"
                color="neutral"
                variant="soft"
              >
                {{ layer }}
              </UBadge>
              <span v-if="!contextState.usedContextLayers.length" class="text-xs text-gray-500">-</span>
            </div>
          </div>
          <div class="rounded-md border border-gray-100 bg-gray-50 p-3">
            <div class="text-[11px] font-medium text-gray-500">Capabilities</div>
            <div class="mt-2 flex flex-wrap gap-1">
              <UBadge
                v-for="capability in contextState.targetCapabilities"
                :key="capability"
                size="xs"
                color="neutral"
                variant="soft"
              >
                {{ shortID(capability) }}
              </UBadge>
              <span v-if="!contextState.targetCapabilities.length" class="text-xs text-gray-500">-</span>
            </div>
          </div>
          <div class="rounded-md border border-gray-100 bg-gray-50 p-3">
            <div class="text-[11px] font-medium text-gray-500">Model</div>
            <div class="mt-1 truncate text-sm font-semibold text-gray-900">
              {{ contextState.modelLabel || '-' }}
            </div>
            <div v-if="contextState.modelSource" class="mt-1 text-[11px] text-gray-500">
              {{ contextState.modelSource }}
            </div>
          </div>
        </div>
        <div v-if="hasContextState" class="mt-3 grid gap-3 md:grid-cols-2">
          <div class="rounded-md border border-gray-100 bg-gray-50 p-3">
            <div class="text-[11px] font-medium text-gray-500">Missing Fields</div>
            <div class="mt-2 flex flex-wrap gap-1">
              <UBadge
                v-for="field in contextState.missingFields"
                :key="field"
                size="xs"
                color="warning"
                variant="soft"
              >
                {{ field }}
              </UBadge>
              <span v-if="!contextState.missingFields.length" class="text-xs text-gray-500">无</span>
            </div>
          </div>
          <div class="rounded-md border border-gray-100 bg-gray-50 p-3">
            <div class="text-[11px] font-medium text-gray-500">Context Policy</div>
            <div class="mt-2 grid gap-1 text-xs text-gray-700">
              <div>完整介绍：{{ contextState.repeatFullIntro ? '是' : '否' }}</div>
              <div>使用能力上下文：{{ contextState.useCapabilityContext ? '是' : '否' }}</div>
              <div>包含示例：{{ contextState.includeExamples ? '是' : '否' }}</div>
            </div>
          </div>
        </div>
        <div v-if="hasContextState && contextState.rawContext" class="mt-3 rounded-md border border-gray-100 bg-gray-50 p-3">
          <div class="mb-1 text-[11px] font-medium text-gray-500">Raw Context Snapshot</div>
          <pre class="max-h-40 overflow-auto whitespace-pre-wrap text-[11px] text-gray-700">{{ stringifyNode(contextState.rawContext) }}</pre>
        </div>
        <div v-else-if="!hasContextState" class="rounded-md bg-gray-50 p-4 text-sm text-gray-500">
          当前消息没有上下文状态快照。请确认这条消息由新版 Agent Runtime 产生，并且 Agent Trace 已开启。
        </div>
      </div>

      <div v-if="isDetailMode" class="grid gap-5 xl:grid-cols-[minmax(0,1fr)_460px]">
        <div class="space-y-5">
          <TraceTimeline :events="timeline" @select="selectNode" />
          <div class="rounded-lg border border-gray-200 bg-white p-4">
            <div class="mb-3 text-sm font-semibold text-gray-900">节点输入输出</div>
            <div v-if="nodes.length" class="space-y-3">
              <button
                v-for="node in nodes"
                :key="node.node_id"
                type="button"
                class="w-full rounded-md border border-gray-200 p-3 text-left hover:bg-gray-50"
                :class="selectedNode?.node_id === node.node_id ? 'border-primary-500 bg-primary-50/60' : ''"
                @click="selectNode(node.node_id)"
              >
                <div class="flex items-center justify-between gap-3">
                  <div class="min-w-0">
                    <div class="truncate text-sm font-medium text-gray-900">
                      {{ formatNodeKind(node.node_kind) }} · {{ node.node_ref || node.node_id }}
                    </div>
                    <div class="mt-1 text-xs text-gray-500">
                      {{ node.phase_status || 'running' }}
                      <span v-if="node.skill_id"> · {{ node.skill_id }}</span>
                    </div>
                  </div>
                  <UBadge size="xs" :color="node.error_summary ? 'error' : 'success'" variant="soft">
                    {{ node.error_summary ? '失败' : '完成' }}
                  </UBadge>
                </div>
                <div class="mt-2 grid gap-2 md:grid-cols-2">
                  <div class="rounded bg-gray-50 p-2">
                    <div class="mb-1 text-[11px] font-medium text-gray-500">输入</div>
                    <pre class="max-h-28 overflow-auto whitespace-pre-wrap text-[11px] text-gray-700">{{ stringifyNode(node.input_summary) }}</pre>
                  </div>
                  <div class="rounded bg-gray-50 p-2">
                    <div class="mb-1 text-[11px] font-medium text-gray-500">输出</div>
                    <pre class="max-h-28 overflow-auto whitespace-pre-wrap text-[11px] text-gray-700">{{ stringifyNode(node.output_summary || { error: node.error_summary }) }}</pre>
                  </div>
                </div>
              </button>
            </div>
            <div v-else class="rounded-md bg-gray-50 p-4 text-sm text-gray-500">
              当前消息没有节点快照。请确认 Agent Trace 已开启，并从新消息入口进入追踪页。
            </div>
          </div>
        </div>
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
import type { AgentRunListItem, AgentSessionListItem, AgentTraceNode, AgentTraceQuery, AgentTraceReport } from "~/composables/api/types/agentTrace";
import { useUserStore } from "~/stores/user";

const userStore = useUserStore();
const isRoot = computed(() => userStore.isRoot);
const service = useAgentTraceService();
const tenantService = useTenantService();
const route = useRoute();
const router = useRouter();

const query = reactive<AgentTraceQuery>({
  tenant_uuid: String(route.query.tenant_uuid || ""),
  session_id: String(route.query.session_id || ""),
  message_id: String(route.query.message_id || ""),
  trace_id: String(route.query.trace_id || "") || undefined,
});
const loading = ref(false);
const tenantLoading = ref(false);
const tenantSearch = ref("");
const tenants = ref<Tenant[]>([]);
const report = ref<AgentTraceReport | null>(null);
const selectedNodeID = ref("");
const runsLoading = ref(false);
const runs = ref<AgentRunListItem[]>([]);
const sessionsLoading = ref(false);
const sessions = ref<AgentSessionListItem[]>([]);
const sessionsTotal = ref(0);
const sessionsPage = ref(1);
const sessionsPageSize = ref(20);
const runsTotal = ref(0);
const runsPage = ref(1);
const runsPageSize = ref(20);
const runStatus = ref("all");
const runStatusItems = [
  { label: "全部", value: "all" },
  { label: "失败", value: "failed" },
  { label: "完成", value: "completed" },
];
const pageSizeItems = [
  { label: "20 条", value: 20 },
  { label: "50 条", value: 50 },
  { label: "100 条", value: 100 },
];
const isDetailMode = computed(() => Boolean(query.tenant_uuid && query.session_id && query.message_id));
const isSessionMode = computed(() => Boolean(query.tenant_uuid && query.session_id && !query.message_id));
const runsPageCount = computed(() => Math.max(1, Math.ceil(runsTotal.value / runsPageSize.value)));
const sessionsPageCount = computed(() => Math.max(1, Math.ceil(sessionsTotal.value / sessionsPageSize.value)));

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
const responsePlannerNode = computed(() => findNodeByKind("response_planner"));
const contextBuilderNode = computed(() => findNodeByKind("context_builder"));
const finalResponseNode = computed(() => findNodeByKind("final_response"));
const contextState = computed(() => {
  const responsePlanner = mergeNodePayload(responsePlannerNode.value);
  const contextBuilder = mergeNodePayload(contextBuilderNode.value);
  const finalResponse = mergeNodePayload(finalResponseNode.value);
  const modelSelection = readRecord(
    responsePlanner.model_selection || finalResponse.model_selection || contextBuilder.model_selection,
  );
  const modelProvider = readString(modelSelection.provider);
  const modelName = readString(modelSelection.model) || readString(responsePlanner.final_response_model) || readString(finalResponse.final_response_model);
  return {
    responseMode: readString(responsePlanner.response_mode || finalResponse.response_mode || contextBuilder.response_mode),
    responsePlanID: readString(responsePlanner.response_plan_id || finalResponse.response_plan_id),
    targetCapabilities: readList(
      responsePlanner.target_capability_ids
      || responsePlanner.capability_ids
      || finalResponse.target_capability_ids
      || finalResponse.capability_ids,
    ),
    usedContextLayers: readList(contextBuilder.used_context_layers || finalResponse.used_context_layers || responsePlanner.used_context_layers),
    missingFields: readList(responsePlanner.missing_fields || finalResponse.missing_fields),
    repeatFullIntro: readBool(responsePlanner.repeat_full_intro),
    useCapabilityContext: readBool(responsePlanner.use_capability_context),
    includeExamples: readBool(responsePlanner.include_examples),
    modelLabel: [modelProvider, modelName].filter(Boolean).join(" / "),
    modelSource: readString(modelSelection.source),
    rawContext: Object.keys(contextBuilder).length ? contextBuilder : undefined,
  };
});
const hasContextState = computed(() => {
  return Boolean(
    contextState.value.responseMode
    || contextState.value.responsePlanID
    || contextState.value.targetCapabilities.length
    || contextState.value.usedContextLayers.length
    || contextState.value.modelLabel
    || contextState.value.rawContext,
  );
});

const metricCards = computed(() => [
  { label: "状态", value: String(report.value?.summary?.status || "-") },
  { label: "节点", value: String(report.value?.nodes?.length || 0) },
  { label: "事件", value: String(report.value?.timeline?.length || 0) },
  { label: "错误", value: String(report.value?.errors?.length || 0) },
]);

const jsonDownloadUrl = computed(() => (report.value ? service.downloadUrl(query, "json") : undefined));
const markdownDownloadUrl = computed(() => (report.value ? service.downloadUrl(query, "markdown") : undefined));
const sessionMarkdownDownloadUrl = computed(() => (query.tenant_uuid && query.session_id ? service.sessionDownloadUrl(query, "markdown") : undefined));

const openDownload = (url?: string) => {
  if (!url || typeof window === "undefined") return;
  window.open(url, "_blank", "noopener,noreferrer");
};

const loadTrace = async () => {
  if (!query.tenant_uuid || !query.session_id || !query.message_id) return;
  loading.value = true;
  try {
    report.value = await service.getReport({ ...query });
    selectedNodeID.value = report.value.nodes?.[0]?.node_id || "";
  } finally {
    loading.value = false;
  }
};

const loadRuns = async () => {
  if (!query.tenant_uuid) return;
  if (!query.session_id) {
    runs.value = [];
    runsTotal.value = 0;
    return;
  }
  if (runsPage.value < 1) runsPage.value = 1;
  runsLoading.value = true;
  try {
    const res = await service.listRuns({
      tenant_uuid: query.tenant_uuid,
      session_id: query.session_id,
      status: runStatus.value === "all" ? undefined : runStatus.value,
      offset: (runsPage.value - 1) * runsPageSize.value,
      limit: runsPageSize.value,
    });
    runs.value = res.items || [];
    runsTotal.value = Number(res.total || 0);
  } finally {
    runsLoading.value = false;
  }
};

const loadSessions = async () => {
  if (!query.tenant_uuid) return;
  if (sessionsPage.value < 1) sessionsPage.value = 1;
  sessionsLoading.value = true;
  try {
    const res = await service.listSessions({
      tenant_uuid: query.tenant_uuid,
      status: runStatus.value === "all" ? undefined : runStatus.value,
      offset: (sessionsPage.value - 1) * sessionsPageSize.value,
      limit: sessionsPageSize.value,
    });
    sessions.value = res.items || [];
    sessionsTotal.value = Number(res.total || 0);
  } finally {
    sessionsLoading.value = false;
  }
};

const handleStatusChange = async () => {
  sessionsPage.value = 1;
  runsPage.value = 1;
  if (isSessionMode.value) await loadRuns();
  else await loadSessions();
};

const handlePageSizeChange = async () => {
  runsPage.value = 1;
  await loadRuns();
};

const handleSessionPageSizeChange = async () => {
  sessionsPage.value = 1;
  await loadSessions();
};

const selectSession = async (item: AgentSessionListItem) => {
  query.tenant_uuid = item.tenant_uuid;
  query.session_id = item.session_id;
  query.message_id = "";
  query.trace_id = undefined;
  report.value = null;
  selectedNodeID.value = "";
  runsPage.value = 1;
  await router.replace({
    query: {
      tenant_uuid: query.tenant_uuid,
      session_id: query.session_id,
    },
  });
  await loadRuns();
};

const selectRun = async (item: AgentRunListItem) => {
  query.tenant_uuid = item.tenant_uuid;
  query.session_id = item.session_id;
  query.message_id = item.message_id;
  query.trace_id = item.trace_id || undefined;
  await router.replace({
    query: {
      tenant_uuid: query.tenant_uuid,
      session_id: query.session_id,
      message_id: query.message_id,
      ...(query.trace_id ? { trace_id: query.trace_id } : {}),
    },
  });
  await loadTrace();
};

const backToSessionRuns = async () => {
  if (!query.session_id) {
    await backToList();
    return;
  }
  query.message_id = "";
  query.trace_id = undefined;
  report.value = null;
  selectedNodeID.value = "";
  await router.replace({
    query: {
      tenant_uuid: query.tenant_uuid,
      session_id: query.session_id,
    },
  });
  await loadRuns();
};

const backToList = async () => {
  query.session_id = "";
  query.message_id = "";
  query.trace_id = undefined;
  report.value = null;
  selectedNodeID.value = "";
  await router.replace({
    query: {
      ...(query.tenant_uuid ? { tenant_uuid: query.tenant_uuid } : {}),
    },
  });
  sessionsPage.value = 1;
  await loadSessions();
};

const selectNode = (nodeId: string) => {
  selectedNodeID.value = nodeId;
};

const nodeKindMap: Record<string, string> = {
  receive_message: "接收消息",
  intent_recognition: "意图识别",
  planner: "任务规划",
  response_planner: "响应规划",
  context_builder: "上下文构建",
  agent_handoff: "子智能体分发",
  skill: "技能执行",
  tooling: "工具执行",
  workflow: "工作流",
  llm_call: "模型调用",
  final_response: "最终回复",
};

const formatNodeKind = (kind: string) => nodeKindMap[String(kind || "").trim()] || kind || "节点";

const responseModeMap: Record<string, string> = {
  capability_intro: "能力介绍",
  capability_howto: "能力用法",
  skill_execution: "技能执行",
  clarify_params: "参数澄清",
  normal_chat: "普通对话",
  error_explain: "错误解释",
};

const formatResponseMode = (mode: string) => responseModeMap[String(mode || "").trim()] || mode || "";

const messageRoleMap: Record<string, string> = {
  user: "用户",
  assistant: "助手",
  system: "系统",
  tool: "工具",
  summary: "摘要",
};

const formatMessageRole = (role: string) => messageRoleMap[String(role || "").trim()] || role || "消息";

const stringifyNode = (value: unknown) => {
  if (value == null) return "-";
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
};

const findNodeByKind = (kind: string) => nodes.value.find((node) => String(node.node_kind || "").trim() === kind) || null;

const mergeNodePayload = (node: AgentTraceNode | null) => ({
  ...(readRecord(node?.input_summary)),
  ...(readRecord(node?.output_summary)),
  ...(readRecord(node?.attributes)),
});

const readRecord = (value: unknown): Record<string, unknown> => {
  if (!value || typeof value !== "object" || Array.isArray(value)) return {};
  return value as Record<string, unknown>;
};

const readString = (value: unknown) => (typeof value === "string" ? value.trim() : "");

const readList = (value: unknown) => {
  if (!Array.isArray(value)) return [];
  return value.map((item) => String(item).trim()).filter(Boolean);
};

const readBool = (value: unknown) => value === true || value === "true";

const formatTraceTime = (value?: string) => {
  if (!value) return "-";
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  }).format(d);
};

const shortID = (value?: string) => {
  const raw = String(value || "").trim();
  if (!raw) return "-";
  if (raw.length <= 16) return raw;
  return `${raw.slice(0, 8)}...${raw.slice(-6)}`;
};

const loadTenants = async () => {
  tenantLoading.value = true;
  try {
    const params = isRoot.value
      ? { page: 1, page_size: 100 }
      : {
          page: 1,
          page_size: 1,
          tenant_uuid: userStore.currentTenantUuid || "",
        };
    const response = await tenantService.getTenants(params);
    tenants.value = response.data?.items || [];
    if (!query.tenant_uuid) {
      query.tenant_uuid = userStore.currentTenantUuid || tenants.value[0]?.uuid || "";
    }
    if (isDetailMode.value) {
      await loadTrace();
    } else if (isSessionMode.value) {
      await loadRuns();
    } else {
      await loadSessions();
    }
  } finally {
    tenantLoading.value = false;
  }
};

watch(
  () => query.tenant_uuid,
  async (tenantUUID, oldTenantUUID) => {
    if (!tenantUUID || tenantUUID === oldTenantUUID || isDetailMode.value) return;
    query.session_id = "";
    query.message_id = "";
    query.trace_id = undefined;
    report.value = null;
    runsPage.value = 1;
    sessionsPage.value = 1;
    await router.replace({
      query: {
        tenant_uuid: tenantUUID,
      },
    });
    await loadSessions();
  }
);

onMounted(() => {
  void loadTenants();
});
</script>
