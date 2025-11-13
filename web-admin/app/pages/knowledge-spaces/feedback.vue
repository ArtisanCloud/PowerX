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
const statusMessage = ref("");
const errorMessage = ref("");
const cases = ref<FeedbackCaseRecord[]>([]);
const chunkInput = ref("");

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
    await loadCases();
  } catch (error) {
    const message = error instanceof Error ? error.message : "提交失败";
    errorMessage.value = message;
  } finally {
    submitting.value = false;
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
          class="rounded-lg border border-gray-200 p-4"
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
        </div>
        <p v-if="!cases.length" class="text-sm text-gray-500">
          尚未加载反馈或该空间暂无案例。
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
