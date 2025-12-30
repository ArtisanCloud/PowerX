<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import { useKnowledgeSpaces } from "~/composables/useKnowledgeSpaces";
import { createQaBridgeClient } from "~/composables/api/services/knowledge-spaces/qaBridgeClient";
import QaBridgeStatusCard from "~/components/knowledge-spaces/QaBridgeStatusCard.vue";
import { useKnowledgeSpaceStore } from "~/stores/knowledgeSpaces";

const { t } = useI18n();

useHead(() => ({
  title: t("knowledgeSpaces.head.title"),
  meta: [{ name: "description", content: t("knowledgeSpaces.head.description") }],
}));

const api = useKnowledgeSpaces();
const qaClient = createQaBridgeClient();
const knowledgeStore = useKnowledgeSpaceStore();

const quickActions = computed(() => [
  {
    icon: "i-heroicons-plus-circle",
    title: t("knowledgeSpaces.hero.actions.create"),
    description: t("knowledgeSpaces.hero.actions.createDesc"),
    to: "/knowledge-spaces/create",
    primary: true,
  },
  {
    icon: "i-heroicons-book-open",
    title: t("knowledgeSpaces.hero.actions.docs"),
    description: t("knowledgeSpaces.hero.actions.docsDesc"),
    to: "/docs/knowledge-spaces",
  },
]);

const placeholders = computed(() => [
	{
		title: t("knowledgeSpaces.placeholders.spaces.title"),
		description: t("knowledgeSpaces.placeholders.spaces.description"),
	},
	{
		title: t("knowledgeSpaces.placeholders.fusion.title"),
		description: t("knowledgeSpaces.placeholders.fusion.description"),
	},
	{
		title: t("knowledgeSpaces.placeholders.feedback.title"),
		description: t("knowledgeSpaces.placeholders.feedback.description"),
	},
]);

interface QaDashboardStatus {
  latencyMsP95: number;
  citationCoverage: number;
  toolSuccessRate: number;
  degradeCount: number;
  lastAuditId?: string;
  lastUpdatedAt?: string;
}

const qaStatus = ref<QaDashboardStatus>({
  latencyMsP95: 0,
  citationCoverage: 0,
  toolSuccessRate: 0,
  degradeCount: 0,
});

const refreshQaStatus = async () => {
  const tenantUuid = knowledgeStore.lastSpace?.tenantUuid;
  if (!tenantUuid) {
    return;
  }
  try {
    const plan = await qaClient.plan({
      tenantUuid,
      intent: "dashboard-health-check",
      domainTags: ["ops"],
      sessionId: "knowledge-dashboard",
      latencyBudgetMs: 2000,
    });
    qaStatus.value = {
      latencyMsP95: plan.latencyBudgetMs ?? 2000,
      citationCoverage: plan.candidateSpaces[0]?.citationCoverage ?? 0,
      toolSuccessRate: plan.tooling.length > 0 ? 0.99 : 0.9,
      degradeCount: plan.degradeCount ?? 0,
      lastAuditId: plan.telemetry?.traceId,
      lastUpdatedAt: plan.telemetry?.recordedAt,
    };
  } catch (error) {
    console.error(t("knowledgeSpaces.qaCard.loadError"), error);
  }
};

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
  () => knowledgeStore.lastSpace,
  () => {
    if (knowledgeStore.lastSpace?.spaceId && !ingestionForm.spaceId) {
      ingestionForm.spaceId = knowledgeStore.lastSpace.spaceId;
    }
    refreshQaStatus();
  },
  { immediate: true },
);

const sourceOptions = computed(() => [
	{ label: t("knowledgeSpaces.ingestion.sourceOptions.pdf"), value: "pdf" },
	{ label: t("knowledgeSpaces.ingestion.sourceOptions.markdown"), value: "markdown" },
	{ label: t("knowledgeSpaces.ingestion.sourceOptions.table"), value: "table" },
	{ label: t("knowledgeSpaces.ingestion.sourceOptions.api"), value: "api" },
]);

const priorityOptions = computed(() => [
	{ label: t("knowledgeSpaces.ingestion.priorityOptions.normal"), value: "normal" },
	{ label: t("knowledgeSpaces.ingestion.priorityOptions.high"), value: "high" },
]);

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
			ingestionError.value = t("knowledgeSpaces.ingestion.errors.missingSpaceId");
			return;
		}
		if (!canSubmit.value) {
			ingestionError.value = t("knowledgeSpaces.ingestion.errors.missingSource");
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
		const message = error instanceof Error ? error.message : t("knowledgeSpaces.ingestion.errors.runFailed");
		ingestionError.value = message;
	} finally {
		ingestionSubmitting.value = false;
	}
};
</script>

<template>
  <section class="px-6 py-8 space-y-8 lg:px-10">
    <header class="rounded-2xl border border-gray-200 bg-white p-6 shadow-sm flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
      <div>
        <p class="text-sm text-gray-500">{{ t("knowledgeSpaces.hero.badge") }}</p>
        <h1 class="text-2xl font-semibold text-gray-900">{{ t("knowledgeSpaces.hero.title") }}</h1>
        <p class="text-gray-600 mt-2">
          {{ t("knowledgeSpaces.hero.description") }}
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

    <section class="grid gap-4 md:grid-cols-2">
      <QaBridgeStatusCard :status="qaStatus" @refresh="refreshQaStatus" />
      <div class="rounded-lg border border-dashed border-gray-200 p-4 text-sm text-gray-500">
        <p class="font-medium text-gray-700 mb-1">{{ t("knowledgeSpaces.governance.title") }}</p>
        <p>{{ t("knowledgeSpaces.governance.description") }}</p>
      </div>
    </section>

    <UCard>
      <template #header>
        <div class="flex items-center justify-between">
          <div>
            <h2 class="text-lg font-semibold">{{ t("knowledgeSpaces.ingestion.title") }}</h2>
            <p class="text-gray-500 text-sm">{{ t("knowledgeSpaces.ingestion.subtitle") }}</p>
          </div>
          <UBadge color="primary" variant="soft">US2</UBadge>
        </div>
      </template>

      <div class="grid gap-4 md:grid-cols-2">
        <label class="flex flex-col gap-2" v-if="recentSpaces.length">
          <span class="text-sm font-medium text-gray-700">{{ t("knowledgeSpaces.ingestion.recentSpace") }}</span>
          <select
            class="rounded-lg border border-gray-200 px-3 py-2 text-sm shadow-sm focus:border-primary-500 focus:outline-none"
            :value="ingestionForm.spaceId"
            @change="handleSpaceSelect"
          >
            <option v-for="space in recentSpaces" :key="space.spaceId" :value="space.spaceId">
              {{ space.spaceName }} · {{ space.spaceId.slice(0, 8) }}
            </option>
          </select>
          <p class="text-xs text-gray-500">{{ t("knowledgeSpaces.ingestion.recentSpaceHint") }}</p>
        </label>
        <label class="flex flex-col gap-2">
          <span class="text-sm font-medium text-gray-700">{{ t("knowledgeSpaces.ingestion.spaceId") }}</span>
          <input
            v-model="ingestionForm.spaceId"
            type="text"
            placeholder="a25b4e6a-..."
            class="rounded-lg border border-gray-200 px-3 py-2 text-sm shadow-sm focus:border-primary-500 focus:outline-none"
          />
        </label>
        <label class="flex flex-col gap-2">
          <span class="text-sm font-medium text-gray-700">{{ t("knowledgeSpaces.ingestion.sourceType") }}</span>
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
          <span class="text-sm font-medium text-gray-700">{{ t("knowledgeSpaces.ingestion.mode") }}</span>
          <div class="mt-2 flex flex-wrap gap-4 text-sm text-gray-700">
            <label class="inline-flex items-center gap-2">
              <input
                type="radio"
                value="document"
                v-model="ingestionMode"
                class="text-primary-600 focus:ring-primary-500"
              />
              {{ t("knowledgeSpaces.ingestion.modeDocument") }}
            </label>
            <label class="inline-flex items-center gap-2">
              <input
                type="radio"
                value="api"
                v-model="ingestionMode"
                class="text-primary-600 focus:ring-primary-500"
              />
              {{ t("knowledgeSpaces.ingestion.modeApi") }}
            </label>
          </div>
        </div>

        <div v-if="ingestionMode === 'document'" class="md:col-span-2 space-y-2">
          <span class="text-sm font-medium text-gray-700">{{ t("knowledgeSpaces.ingestion.uploadFile") }}</span>
          <input
            type="file"
            accept=".pdf,.md,.txt,.xlsx"
            @change="handleFileChange"
            class="text-sm"
          />
          <p class="text-xs text-gray-500">
            {{ t("knowledgeSpaces.ingestion.uploadHint") }}
          </p>
          <p v-if="selectedFile" class="text-xs text-primary-600">
            {{ t("knowledgeSpaces.ingestion.selectedFile", { name: selectedFile.name }) }}
          </p>
        </div>

        <template v-else>
          <label class="flex flex-col gap-2 md:col-span-2">
            <span class="text-sm font-medium text-gray-700">{{ t("knowledgeSpaces.ingestion.sourceUri") }}</span>
            <input
              v-model="ingestionForm.sourceUri"
              type="text"
              placeholder="https://api.example.com/docs 或 s3://bucket/handbook.pdf"
              class="rounded-lg border border-gray-200 px-3 py-2 text-sm shadow-sm focus:border-primary-500 focus:outline-none"
            />
          </label>
          <label class="flex flex-col gap-2">
            <span class="text-sm font-medium text-gray-700">{{ t("knowledgeSpaces.ingestion.maskingProfile") }}</span>
            <input
              v-model="ingestionForm.maskingProfile"
              type="text"
              placeholder="masking.profile.v1"
              class="rounded-lg border border-gray-200 px-3 py-2 text-sm shadow-sm focus:border-primary-500 focus:outline-none"
            />
          </label>
        </template>

        <label class="flex flex-col gap-2">
          <span class="text-sm font-medium text-gray-700">{{ t("knowledgeSpaces.ingestion.priority") }}</span>
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
            {{ t("knowledgeSpaces.ingestion.actions.ingestNow") }}
          </UButton>
        </div>
      </div>
      <div v-if="ingestionResult" class="mt-4 rounded-lg border border-primary-100 bg-primary-50 p-4 text-sm space-y-1">
        <p class="font-medium text-primary-700">{{ t("knowledgeSpaces.ingestion.result.latestTitle") }}</p>
        <p class="text-primary-600">
          {{
            t("knowledgeSpaces.ingestion.result.jobSummary", {
              jobId: ingestionResult.jobId,
              status: ingestionResult.status,
              chunkTotal: ingestionResult.chunkTotal,
            })
          }}
        </p>
        <p class="text-primary-600">
          {{
            t("knowledgeSpaces.ingestion.result.metricSummary", {
              coverage: ingestionResult.chunkCoveragePct,
              embedding: ingestionResult.embeddingSuccessPct,
              masking: ingestionResult.maskingCoveragePct,
            })
          }}
        </p>
      </div>
      <div v-if="ingestionHistory.length" class="mt-4 rounded-lg border border-gray-200 p-4">
        <p class="text-sm font-medium text-gray-700">{{ t("knowledgeSpaces.ingestion.history") }}</p>
        <ul class="mt-2 space-y-1 text-sm text-gray-600">
          <li v-for="task in ingestionHistory" :key="task.jobId">
            {{
              t("knowledgeSpaces.ingestion.historyEntry", {
                job: task.jobId.slice(0, 8),
                status: task.status,
                time: new Date(task.completedAt).toLocaleTimeString(),
              })
            }}
          </li>
        </ul>
      </div>
    </UCard>

    <UCard>
      <template #header>
        <div class="flex items-center justify-between">
          <div>
            <h2 class="text-lg font-semibold">{{ t("knowledgeSpaces.timeline.title") }}</h2>
            <p class="text-gray-500 text-sm">{{ t("knowledgeSpaces.timeline.description") }}</p>
          </div>
          <UBadge color="orange" variant="soft">{{ t("knowledgeSpaces.timeline.badge") }}</UBadge>
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
