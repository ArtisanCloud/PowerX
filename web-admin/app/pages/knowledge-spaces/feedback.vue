<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import {
  useKnowledgeSpaces,
  type FeedbackCasePayload,
  type FeedbackCaseRecord,
} from "~/composables/useKnowledgeSpaces";
import { useKnowledgeSpaceStore } from "~/stores/knowledgeSpaces";

useHead({
  title: "反馈与再加工",
});

const api = useKnowledgeSpaces();
const store = useKnowledgeSpaceStore();

const spaceId = ref("");
const loadingCases = ref(false);
const submitting = ref(false);
const acting = ref(false);
const statusMessage = ref("");
const errorMessage = ref("");
const cases = ref<FeedbackCaseRecord[]>([]);
const chunkInput = ref("");
const selectedCaseId = ref<string>("");
const actionBy = ref<string>("");
const actionNotes = ref<string>("");

const form = reactive<FeedbackCasePayload>({
  severity: "medium",
  issueType: "accuracy",
  linkedChunks: [],
  notes: "",
  reportedBy: "",
  toolTraceRef: "",
});

const severityOptions = [
  { label: "低", value: "low" },
  { label: "中", value: "medium" },
  { label: "高", value: "high" },
  { label: "致命", value: "critical" },
];

const issueOptions = [
  { label: "准确性", value: "accuracy" },
  { label: "时效性", value: "freshness" },
  { label: "合规性", value: "compliance" },
];

const recentSpaceId = computed(() => store.lastSpace?.spaceId ?? "");
const selectedCase = computed(() =>
  cases.value.find(item => item.caseId === selectedCaseId.value),
);

const slaCountdown = computed(() => {
  const due = selectedCase.value?.slaDueAt;
  if (!due) {
    return "";
  }
  const dueAt = new Date(due).getTime();
  const now = Date.now();
  const delta = dueAt - now;
  if (Number.isNaN(dueAt)) {
    return "";
  }
  const hours = Math.floor(delta / 36e5);
  const minutes = Math.floor((delta % 36e5) / 6e4);
  const sign = delta < 0 ? "-" : "";
  return `${sign}${Math.abs(hours)}h ${Math.abs(minutes)}m`;
});

watch(
  () => recentSpaceId.value,
  value => {
    if (!spaceId.value && value) {
      spaceId.value = value;
    }
  },
  { immediate: true },
);

const loadCases = async () => {
  if (!spaceId.value) {
    errorMessage.value = "请先输入空间 ID";
    return;
  }
  errorMessage.value = "";
  loadingCases.value = true;
  try {
    cases.value = await api.listFeedbackCases(spaceId.value);
    if (!selectedCaseId.value && cases.value.length > 0) {
      selectedCaseId.value = cases.value[0].caseId;
    }
  } catch (error) {
    const message = error instanceof Error ? error.message : "加载反馈失败";
    errorMessage.value = message;
  } finally {
    loadingCases.value = false;
  }
};

const submitFeedback = async () => {
  if (!spaceId.value) {
    errorMessage.value = "请先输入空间 ID";
    return;
  }
  const chunks = chunkInput.value
    .split(/[\s,]+/)
    .map(chunk => chunk.trim())
    .filter(Boolean);
  if (chunks.length === 0) {
    errorMessage.value = "请至少填写一个 Chunk ID";
    return;
  }
  form.linkedChunks = chunks;
  errorMessage.value = "";
  statusMessage.value = "";
  submitting.value = true;
  try {
    const record = await api.submitFeedbackCase(spaceId.value, form);
    statusMessage.value = `反馈 ${record.caseId.slice(0, 8)} 已进入再加工`;
    chunkInput.value = "";
    form.notes = "";
    selectedCaseId.value = record.caseId;
    await loadCases();
  } catch (error) {
    const message = error instanceof Error ? error.message : "提交失败";
    errorMessage.value = message;
  } finally {
    submitting.value = false;
  }
};

const doReprocess = async () => {
  if (!spaceId.value || !selectedCase.value) {
    return;
  }
  acting.value = true;
  statusMessage.value = "";
  errorMessage.value = "";
  try {
    const record = await api.reprocessFeedbackCase(spaceId.value, selectedCase.value.caseId, {
      requestedBy: actionBy.value || form.reportedBy,
    });
    statusMessage.value = `已触发再加工：${record.caseId.slice(0, 8)}`;
    await loadCases();
  } catch (error) {
    const message = error instanceof Error ? error.message : "触发再加工失败";
    errorMessage.value = message;
  } finally {
    acting.value = false;
  }
};

const doEscalate = async () => {
  if (!spaceId.value || !selectedCase.value) {
    return;
  }
  acting.value = true;
  statusMessage.value = "";
  errorMessage.value = "";
  try {
    const record = await api.escalateFeedbackCase(spaceId.value, selectedCase.value.caseId, {
      requestedBy: actionBy.value || form.reportedBy,
      reason: actionNotes.value,
    });
    statusMessage.value = `已升级案例：${record.caseId.slice(0, 8)}`;
    await loadCases();
  } catch (error) {
    const message = error instanceof Error ? error.message : "升级失败";
    errorMessage.value = message;
  } finally {
    acting.value = false;
  }
};

const doRollback = async () => {
  if (!spaceId.value || !selectedCase.value) {
    return;
  }
  acting.value = true;
  statusMessage.value = "";
  errorMessage.value = "";
  try {
    const record = await api.rollbackFeedbackCase(spaceId.value, selectedCase.value.caseId, {
      requestedBy: actionBy.value || form.reportedBy,
      reason: actionNotes.value,
    });
    statusMessage.value = `已回滚并关闭案例：${record.caseId.slice(0, 8)}`;
    await loadCases();
  } catch (error) {
    const message = error instanceof Error ? error.message : "回滚失败";
    errorMessage.value = message;
  } finally {
    acting.value = false;
  }
};

const doClose = async () => {
  if (!spaceId.value || !selectedCase.value) {
    return;
  }
  acting.value = true;
  statusMessage.value = "";
  errorMessage.value = "";
  try {
    const record = await api.closeFeedbackCase(spaceId.value, selectedCase.value.caseId, {
      requestedBy: actionBy.value || form.reportedBy,
      resolutionNotes: actionNotes.value,
    });
    statusMessage.value = `已关闭案例：${record.caseId.slice(0, 8)}`;
    await loadCases();
  } catch (error) {
    const message = error instanceof Error ? error.message : "关闭失败";
    errorMessage.value = message;
  } finally {
    acting.value = false;
  }
};

const doExport = async () => {
  if (!spaceId.value) {
    return;
  }
  acting.value = true;
  statusMessage.value = "";
  errorMessage.value = "";
  try {
    const payload = await api.exportFeedbackCases(spaceId.value, { limit: 200 });
    const blob = new Blob([JSON.stringify(payload, null, 2)], { type: "application/json" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `knowledge-feedback-${spaceId.value}.json`;
    a.click();
    URL.revokeObjectURL(url);
    statusMessage.value = "已导出反馈报告";
  } catch (error) {
    const message = error instanceof Error ? error.message : "导出失败";
    errorMessage.value = message;
  } finally {
    acting.value = false;
  }
};
</script>

<template>
  <section class="px-6 py-8 space-y-8">
    <header class="space-y-2">
      <p class="text-sm text-gray-500">Feedback Loop</p>
      <h1 class="text-2xl font-semibold text-gray-900">反馈驱动再加工</h1>
      <p class="text-gray-600">
        管理反馈案例、监控 SLA，并触发再加工 / 热更新流程。
      </p>
    </header>

    <UCard>
      <template #header>
        <div class="flex flex-col gap-3 md:flex-row md:items-end md:justify-between">
          <label class="flex flex-col gap-1 text-sm text-gray-700">
            空间 ID
            <input
              v-model="spaceId"
              type="text"
              placeholder="a25b4e6a-..."
              class="rounded-lg border border-gray-200 px-3 py-2 text-sm shadow-sm focus:border-primary-500 focus:outline-none"
            />
          </label>
          <UButton :loading="loadingCases" @click="loadCases">
            加载反馈列表
          </UButton>
        </div>
      </template>
      <div class="space-y-4">
        <div
          v-for="item in cases"
          :key="item.caseId"
          class="rounded-lg border border-gray-200 p-4 cursor-pointer hover:border-primary-300"
          :class="item.caseId === selectedCaseId ? 'border-primary-400 bg-primary-50/30' : ''"
          @click="selectedCaseId = item.caseId"
        >
          <div class="flex flex-wrap items-center gap-2 text-sm">
            <span class="font-medium text-gray-900">案例 {{ item.caseId.slice(0, 8) }}</span>
            <UBadge color="primary" variant="soft">{{ item.status }}</UBadge>
            <UBadge color="gray" variant="soft">{{ item.severity }}</UBadge>
            <UBadge color="primary" variant="outline">{{ item.issueType }}</UBadge>
          </div>
          <p class="mt-2 text-sm text-gray-600">
            Chunk：{{ item.linkedChunks?.join(", ") || "未提供" }}
          </p>
          <p class="text-xs text-gray-500">
            报告人：{{ item.reportedBy || "未知" }} ·
            SLA 截止：
            {{ item.slaDueAt ? new Date(item.slaDueAt).toLocaleString() : "计算中" }}
          </p>
          <p v-if="item.traceId || item.toolTraceRef" class="text-xs text-gray-500">
            Trace：{{ item.traceId || item.toolTraceRef }}
          </p>
        </div>
        <p v-if="!cases.length" class="text-sm text-gray-500">
          尚未加载反馈或该空间暂无案例。
        </p>
      </div>
    </UCard>

    <UCard v-if="selectedCase">
      <template #header>
        <div class="flex flex-col gap-2 md:flex-row md:items-center md:justify-between">
          <div>
            <h2 class="text-lg font-semibold">案例详情</h2>
            <p class="text-sm text-gray-500">
              Case {{ selectedCase.caseId }} · 状态 {{ selectedCase.status }} · SLA {{ slaCountdown || "N/A" }}
            </p>
          </div>
          <div class="flex flex-wrap gap-2">
            <UButton size="sm" :loading="acting" @click="doReprocess">一键 Reprocess</UButton>
            <UButton size="sm" color="yellow" variant="soft" :loading="acting" @click="doEscalate">升级</UButton>
            <UButton size="sm" color="red" variant="soft" :loading="acting" @click="doRollback">回滚</UButton>
            <UButton size="sm" color="green" variant="soft" :loading="acting" @click="doClose">关闭</UButton>
            <UButton size="sm" color="gray" variant="outline" :loading="acting" @click="doExport">导出</UButton>
          </div>
        </div>
      </template>
      <div class="space-y-3 text-sm">
        <div class="grid gap-2 md:grid-cols-2">
          <div class="rounded-lg border border-gray-200 p-3">
            <p class="text-xs text-gray-500">Trace / Job</p>
            <p class="mt-1 text-gray-900 break-all">
              {{ selectedCase.traceId || selectedCase.toolTraceRef || "未提供" }}
            </p>
            <p v-if="selectedCase.reprocessJobId" class="text-xs text-gray-500 mt-1">
              Reprocess Job：{{ selectedCase.reprocessJobId }}
            </p>
          </div>
          <div class="rounded-lg border border-gray-200 p-3">
            <p class="text-xs text-gray-500">SLA 解释</p>
            <p class="mt-1 text-gray-700">
              以严重级别计算 SLA 截止时间；超时会建议升级并触发告警策略。
            </p>
          </div>
        </div>
        <div class="rounded-lg border border-gray-200 p-3">
          <p class="text-xs text-gray-500">Citations / Chunks</p>
          <p class="mt-1 text-gray-900 break-all">
            {{ selectedCase.linkedChunks?.join(", ") || "未提供" }}
          </p>
        </div>
        <div class="grid gap-4 md:grid-cols-2">
          <label class="flex flex-col gap-1 text-sm text-gray-700">
            操作人（用于审计/通知）
            <input
              v-model="actionBy"
              type="text"
              class="rounded-lg border border-gray-200 px-3 py-2 text-sm shadow-sm focus:border-primary-500 focus:outline-none"
              placeholder="ops@powerx.local"
            />
          </label>
          <label class="flex flex-col gap-1 text-sm text-gray-700 md:col-span-2">
            处理记录 / 升级原因 / 回滚说明（会写入审计）
            <textarea
              v-model="actionNotes"
              rows="3"
              class="rounded-lg border border-gray-200 px-3 py-2 text-sm shadow-sm focus:border-primary-500 focus:outline-none"
              placeholder="例如：已回滚至 bundle-xxx，并通知业务方…"
            />
          </label>
        </div>
        <p class="text-xs text-gray-500">
          通知记录：当前版本仅保留审计条目（export 中 audits）；后续可接入 IM/Webhook。
        </p>
      </div>
    </UCard>

    <UCard>
      <template #header>
        <div>
          <h2 class="text-lg font-semibold">提交新反馈</h2>
          <p class="text-sm text-gray-500">
            填写 Chunk / Severity，即可触发再加工任务。
          </p>
        </div>
      </template>
      <div class="grid gap-4 md:grid-cols-2">
        <label class="flex flex-col gap-1 text-sm text-gray-700">
          严重级别
          <select
            v-model="form.severity"
            class="rounded-lg border border-gray-200 px-3 py-2 text-sm shadow-sm focus:border-primary-500 focus:outline-none"
          >
            <option v-for="opt in severityOptions" :key="opt.value" :value="opt.value">
              {{ opt.label }}
            </option>
          </select>
        </label>
        <label class="flex flex-col gap-1 text-sm text-gray-700">
          问题类型
          <select
            v-model="form.issueType"
            class="rounded-lg border border-gray-200 px-3 py-2 text-sm shadow-sm focus:border-primary-500 focus:outline-none"
          >
            <option v-for="opt in issueOptions" :key="opt.value" :value="opt.value">
              {{ opt.label }}
            </option>
          </select>
        </label>
        <label class="flex flex-col gap-1 text-sm text-gray-700 md:col-span-2">
          关联 Chunk ID（逗号或换行分隔）
          <textarea
            v-model="chunkInput"
            rows="3"
            class="rounded-lg border border-gray-200 px-3 py-2 text-sm shadow-sm focus:border-primary-500 focus:outline-none"
            placeholder="chunk-uuid-1, chunk-uuid-2"
          />
        </label>
        <label class="flex flex-col gap-1 text-sm text-gray-700 md:col-span-2">
          备注
          <textarea
            v-model="form.notes"
            rows="3"
            class="rounded-lg border border-gray-200 px-3 py-2 text-sm shadow-sm focus:border-primary-500 focus:outline-none"
          />
        </label>
        <label class="flex flex-col gap-1 text-sm text-gray-700">
          报告人
          <input
            v-model="form.reportedBy"
            type="email"
            class="rounded-lg border border-gray-200 px-3 py-2 text-sm shadow-sm focus:border-primary-500 focus:outline-none"
            placeholder="ops@powerx.local"
          />
        </label>
        <label class="flex flex-col gap-1 text-sm text-gray-700">
          Tool Trace Ref
          <input
            v-model="form.toolTraceRef"
            type="text"
            class="rounded-lg border border-gray-200 px-3 py-2 text-sm shadow-sm focus:border-primary-500 focus:outline-none"
            placeholder="trace-123"
          />
        </label>
      </div>
      <div class="mt-4 flex flex-wrap items-center gap-3">
        <UButton :loading="submitting" @click="submitFeedback">
          提交反馈
        </UButton>
        <p v-if="statusMessage" class="text-sm text-primary-600">{{ statusMessage }}</p>
        <p v-if="errorMessage" class="text-sm text-red-500">{{ errorMessage }}</p>
      </div>
    </UCard>
  </section>
</template>
