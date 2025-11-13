<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import { useKnowledgeSpaces } from "~/composables/useKnowledgeSpaces";
import { useKnowledgeSpaceStore } from "~/stores/knowledgeSpaces";

useHead({
  title: "知识空间",
  meta: [{ name: "description", content: "统一管理知识空间的配额、策略与告警入口" }],
});

const api = useKnowledgeSpaces();
const knowledgeStore = useKnowledgeSpaceStore();

const quickActions = [
  {
    icon: "i-heroicons-plus-circle",
    title: "创建知识空间",
    description: "通过向导配置租户、策略、配额与 IAM",
    to: "/knowledge-spaces/create",
    primary: true,
  },
  {
    icon: "i-heroicons-book-open",
    title: "操作指南",
    description: "查看配置流程与准入条件",
    to: "/docs/knowledge-spaces",
  },
];

const placeholders = [
	{
		title: "空间列表",
		description: "即将展示租户维度的空间概览、配额与健康状态。",
	},
	{
		title: "融合策略概览",
		description: "这里会汇总最近的融合任务与冲突提示。",
	},
	{
		title: "反馈与告警",
		description: "追踪 SLA 超时、告警升级与再加工任务。",
	},
];

const ingestionForm = reactive({
	spaceId: "",
	sourceType: "pdf",
	sourceUri: "",
	maskingProfile: "",
	priority: "normal",
});

const ingestionMode = ref<"document" | "api">("document");
const selectedFile = ref<File | null>(null);
const ingestionSubmitting = ref(false);
const ingestionError = ref("");
const ingestionResult = ref<{ jobId: string; status: string; chunkTotal: number; chunkCoveragePct: number; embeddingSuccessPct: number; maskingCoveragePct: number } | null>(null);
const ingestionHistory = ref<Array<{ jobId: string; status: string; completedAt: string }>>([]);
const recentSpaces = computed(() =>
	knowledgeStore.lastSpace ? [knowledgeStore.lastSpace] : [],
);

watch(
	() => recentSpaces.value,
	(spaces) => {
		if (!ingestionForm.spaceId && spaces.length > 0) {
			ingestionForm.spaceId = spaces[0].spaceId;
		}
	},
	{ immediate: true },
);

const sourceOptions = [
	{ label: "PDF", value: "pdf" },
	{ label: "Markdown", value: "markdown" },
	{ label: "表格/Excel", value: "table" },
	{ label: "API", value: "api" },
];

const priorityOptions = [
	{ label: "标准", value: "normal" },
	{ label: "高优先级", value: "high" },
];

const canSubmit = computed(() => {
	if (!ingestionForm.spaceId) return false;
	if (ingestionMode.value === "document") {
		return !!selectedFile.value || Boolean(ingestionForm.sourceUri);
	}
	return Boolean(ingestionForm.sourceUri);
});

const handleFileChange = (event: Event) => {
	const input = event.target as HTMLInputElement;
	const file = input.files?.[0] || null;
	selectedFile.value = file;
	if (file) {
		ingestionForm.sourceUri = `file://${file.name}`;
	}
};

const handleSpaceSelect = (event: Event) => {
	const target = event.target as HTMLSelectElement;
	ingestionForm.spaceId = target.value;
};

const submitIngestion = async () => {
	ingestionError.value = "";
	if (!ingestionForm.spaceId) {
		ingestionError.value = "请输入空间 ID";
		return;
	}
	if (!canSubmit.value) {
		ingestionError.value = "请完善来源信息";
		return;
	}
	ingestionSubmitting.value = true;
	try {
		const resolvedSource =
			ingestionMode.value === "document" && selectedFile.value
				? `file://${selectedFile.value.name}`
				: ingestionForm.sourceUri;
		const payload = {
			sourceType: ingestionForm.sourceType,
			sourceUri: resolvedSource,
			maskingProfile: ingestionForm.maskingProfile,
			priority: ingestionForm.priority,
		};
		const data = await api.triggerIngestion(ingestionForm.spaceId, payload);
		ingestionResult.value = data;
		if (ingestionResult.value) {
			ingestionForm.sourceUri = "";
			ingestionForm.maskingProfile = "";
			selectedFile.value = null;
			ingestionHistory.value.unshift({
				jobId: data.jobId,
				status: data.status,
				completedAt: new Date().toISOString(),
			});
			if (ingestionHistory.value.length > 5) {
				ingestionHistory.value.pop();
			}
		}
	} catch (error) {
		const message = error instanceof Error ? error.message : "触发入库失败";
		ingestionError.value = message;
	} finally {
		ingestionSubmitting.value = false;
	}
};
</script>

<template>
  <section class="px-6 py-8 space-y-8">
    <header class="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
      <div>
        <p class="text-sm text-gray-500">Knowledge Space</p>
        <h1 class="text-2xl font-semibold text-gray-900">知识空间工作台</h1>
        <p class="text-gray-600 mt-2">
          统一创建与管理多租户知识空间，串联入库、融合与反馈闭环。
        </p>
      </div>
      <div class="flex flex-wrap gap-3">
        <NuxtLink
          v-for="action in quickActions"
          :key="action.title"
          :to="action.to"
          class="inline-flex items-center gap-2 rounded-lg border px-4 py-2 text-sm font-medium transition hover:bg-gray-50"
          :class="action.primary ? 'bg-primary-600 text-white border-primary-600 hover:bg-primary-500' : 'border-gray-200 text-gray-700'"
        >
          <UIcon :name="action.icon" class="w-5 h-5" />
          <span>{{ action.title }}</span>
        </NuxtLink>
      </div>
    </header>

    <UCard>
      <template #header>
        <div class="flex items-center justify-between">
          <div>
            <h2 class="text-lg font-semibold">入库 orchestrator</h2>
            <p class="text-gray-500 text-sm">提交文档或 API 源，自动生成 chunk 与 embedding。</p>
          </div>
          <UBadge color="primary" variant="soft">US2</UBadge>
        </div>
      </template>

      <div class="grid gap-4 md:grid-cols-2">
        <label class="flex flex-col gap-2" v-if="recentSpaces.length">
          <span class="text-sm font-medium text-gray-700">最近空间</span>
          <select
            class="rounded-lg border border-gray-200 px-3 py-2 text-sm shadow-sm focus:border-primary-500 focus:outline-none"
            :value="ingestionForm.spaceId"
            @change="handleSpaceSelect"
          >
            <option v-for="space in recentSpaces" :key="space.spaceId" :value="space.spaceId">
              {{ space.spaceName }} · {{ space.spaceId.slice(0, 8) }}
            </option>
          </select>
          <p class="text-xs text-gray-500">同步来自向导的最新空间，快速触发入库。</p>
        </label>
        <label class="flex flex-col gap-2">
          <span class="text-sm font-medium text-gray-700">空间 ID</span>
          <input
            v-model="ingestionForm.spaceId"
            type="text"
            placeholder="a25b4e6a-..."
            class="rounded-lg border border-gray-200 px-3 py-2 text-sm shadow-sm focus:border-primary-500 focus:outline-none"
          />
        </label>
        <label class="flex flex-col gap-2">
          <span class="text-sm font-medium text-gray-700">来源类型</span>
          <select
            v-model="ingestionForm.sourceType"
            class="rounded-lg border border-gray-200 px-3 py-2 text-sm shadow-sm focus:border-primary-500 focus:outline-none"
          >
            <option v-for="opt in sourceOptions" :key="opt.value" :value="opt.value">
              {{ opt.label }}
            </option>
          </select>
        </label>
        <div class="md:col-span-2">
          <span class="text-sm font-medium text-gray-700">入库模式</span>
          <div class="mt-2 flex flex-wrap gap-4 text-sm text-gray-700">
            <label class="inline-flex items-center gap-2">
              <input
                type="radio"
                value="document"
                v-model="ingestionMode"
                class="text-primary-600 focus:ring-primary-500"
              />
              文档上传
            </label>
            <label class="inline-flex items-center gap-2">
              <input
                type="radio"
                value="api"
                v-model="ingestionMode"
                class="text-primary-600 focus:ring-primary-500"
              />
              API / URL
            </label>
          </div>
        </div>

        <div v-if="ingestionMode === 'document'" class="md:col-span-2 space-y-2">
          <span class="text-sm font-medium text-gray-700">上传文件</span>
          <input
            type="file"
            accept=".pdf,.md,.txt,.xlsx"
            @change="handleFileChange"
            class="text-sm"
          />
          <p class="text-xs text-gray-500">
            文件会自动上传到对象存储后进入 chunk pipeline。
          </p>
          <p v-if="selectedFile" class="text-xs text-primary-600">
            已选择：{{ selectedFile.name }}
          </p>
        </div>

        <template v-else>
          <label class="flex flex-col gap-2 md:col-span-2">
            <span class="text-sm font-medium text-gray-700">来源地址</span>
            <input
              v-model="ingestionForm.sourceUri"
              type="text"
              placeholder="https://api.example.com/docs 或 s3://bucket/handbook.pdf"
              class="rounded-lg border border-gray-200 px-3 py-2 text-sm shadow-sm focus:border-primary-500 focus:outline-none"
            />
          </label>
          <label class="flex flex-col gap-2">
            <span class="text-sm font-medium text-gray-700">脱敏策略</span>
            <input
              v-model="ingestionForm.maskingProfile"
              type="text"
              placeholder="masking.profile.v1"
              class="rounded-lg border border-gray-200 px-3 py-2 text-sm shadow-sm focus:border-primary-500 focus:outline-none"
            />
          </label>
        </template>

        <label class="flex flex-col gap-2">
          <span class="text-sm font-medium text-gray-700">优先级</span>
          <select
            v-model="ingestionForm.priority"
            class="rounded-lg border border-gray-200 px-3 py-2 text-sm shadow-sm focus:border-primary-500 focus:outline-none"
          >
            <option v-for="opt in priorityOptions" :key="opt.value" :value="opt.value">
              {{ opt.label }}
            </option>
          </select>
        </label>
      </div>
      <div class="mt-4 flex items-center justify-between">
        <span class="text-sm text-red-500" v-if="ingestionError">{{ ingestionError }}</span>
        <div class="flex items-center gap-2">
          <UButton :loading="ingestionSubmitting" :disabled="!canSubmit" @click="submitIngestion">
            立即入库
          </UButton>
        </div>
      </div>
      <div v-if="ingestionResult" class="mt-4 rounded-lg border border-primary-100 bg-primary-50 p-4 text-sm space-y-1">
        <p class="font-medium text-primary-700">最近一次任务</p>
        <p class="text-primary-600">
          任务 {{ ingestionResult.jobId }} · 状态 {{ ingestionResult.status }} · Chunk {{ ingestionResult.chunkTotal }} 个
        </p>
        <p class="text-primary-600">
          覆盖率 {{ ingestionResult.chunkCoveragePct }}% · Embedding {{ ingestionResult.embeddingSuccessPct }}% · Masking {{ ingestionResult.maskingCoveragePct }}%
        </p>
      </div>
      <div v-if="ingestionHistory.length" class="mt-4 rounded-lg border border-gray-200 p-4">
        <p class="text-sm font-medium text-gray-700">最近任务</p>
        <ul class="mt-2 space-y-1 text-sm text-gray-600">
          <li v-for="task in ingestionHistory" :key="task.jobId">
            {{ task.jobId.slice(0, 8) }} · {{ task.status }} · {{ new Date(task.completedAt).toLocaleTimeString() }}
          </li>
        </ul>
      </div>
    </UCard>

    <UCard>
      <template #header>
        <div class="flex items-center justify-between">
          <div>
            <h2 class="text-lg font-semibold">空间动态</h2>
            <p class="text-gray-500 text-sm">后续将展示真实空间/融合数据。</p>
          </div>
          <UBadge color="orange" variant="soft">规划中</UBadge>
        </div>
      </template>

      <div class="grid gap-6 md:grid-cols-3">
        <article
          v-for="item in placeholders"
          :key="item.title"
          class="rounded-xl border border-dashed border-gray-200 p-4 bg-gray-50"
        >
          <h3 class="font-medium text-gray-900">{{ item.title }}</h3>
          <p class="text-sm text-gray-600 mt-2">{{ item.description }}</p>
        </article>
      </div>
    </UCard>
  </section>
</template>
