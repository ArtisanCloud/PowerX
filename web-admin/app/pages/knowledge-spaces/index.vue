<script setup lang="ts">
import { reactive, ref } from "vue";

useHead({
  title: "知识空间",
  meta: [{ name: "description", content: "统一管理知识空间的配额、策略与告警入口" }],
});

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

const ingestionSubmitting = ref(false);
const ingestionError = ref("");
const ingestionResult = ref<{ jobId: string; status: string; chunkTotal: number } | null>(null);

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

const submitIngestion = async () => {
	ingestionError.value = "";
	if (!ingestionForm.spaceId) {
		ingestionError.value = "请输入空间 ID";
		return;
	}
	ingestionSubmitting.value = true;
	try {
		const payload = {
			sourceType: ingestionForm.sourceType,
			sourceUri: ingestionForm.sourceUri,
			maskingProfile: ingestionForm.maskingProfile,
			priority: ingestionForm.priority,
		};
		const data = await $fetch<{ data: { jobId: string; status: string; chunkTotal: number } }>(
			`/api/admin/knowledge-spaces/${ingestionForm.spaceId}/ingestion-jobs`,
			{
				method: "POST",
				body: payload,
			},
		);
		ingestionResult.value = data?.data ?? null;
		if (ingestionResult.value) {
			ingestionForm.sourceUri = "";
			ingestionForm.maskingProfile = "";
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
        <label class="flex flex-col gap-2 md:col-span-2">
          <span class="text-sm font-medium text-gray-700">来源地址</span>
          <input
            v-model="ingestionForm.sourceUri"
            type="text"
            placeholder="s3://bucket/handbook.pdf 或 https://api.example.com/docs"
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
          <UButton :loading="ingestionSubmitting" @click="submitIngestion">
            立即入库
          </UButton>
        </div>
      </div>
      <div v-if="ingestionResult" class="mt-4 rounded-lg border border-primary-100 bg-primary-50 p-4 text-sm">
        <p class="font-medium text-primary-700">最近一次任务</p>
        <p class="text-primary-600 mt-1">
          任务 {{ ingestionResult.jobId }} · 状态 {{ ingestionResult.status }} · Chunk {{ ingestionResult.chunkTotal }} 个
        </p>
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
