<script setup lang="ts">
import { useKnowledgeSpaceStore } from "~/stores/knowledgeSpaces";
import PolicySelector from "~/components/knowledge-spaces/PolicySelector.vue";
import QuotaForm from "~/components/knowledge-spaces/QuotaForm.vue";
import AuditPreview from "~/components/knowledge-spaces/AuditPreview.vue";
import IamStatusBadge from "~/components/knowledge-spaces/IamStatusBadge.vue";
import { useKnowledgeSpaces } from "~/composables/useKnowledgeSpaces";

const store = useKnowledgeSpaceStore();
const { fetchStatus } = useKnowledgeSpaces();
const statusSnapshot = ref<{ pendingIam: number; active: number; retired: number } | null>(null);

const policyOptions = [
  {
    label: "默认模版 v1",
    value: "default-v1",
    description: "启用必需的 RAG / Masking / Alerting 组合。",
  },
  {
    label: "严苛模版 v2",
    value: "strict-v2",
    description: "针对合规租户，强化 IAM 审批与脱敏审计。",
  },
];

const scenarioOptions = [
  {
    label: "默认式（Default）",
    value: "default",
    description: "沿用默认 ProfileKey（default），适合大多数通用空间。",
  },
  {
    label: "引导式（Guided）",
    value: "guided",
    description: "绑定 ragProfileKey=guided，用于更强的引导/约束策略（可在 Profiles/Playground 中继续调参）。",
  },
];

const loadStatus = async () => {
  try {
    statusSnapshot.value = await fetchStatus();
  } catch (error) {
    console.warn("failed to fetch status", error);
  }
};

onMounted(() => {
  loadStatus();
});

const stepTitle = computed(() => {
  switch (store.step) {
    case 1:
      return "基础信息";
    case 2:
      return "策略模版";
    case 3:
      return "配额与 IAM";
    default:
      return "审阅与创建";
  }
});

const canSubmit = computed(
  () =>
    store.isBasicInfoValid &&
    store.isPolicyStepValid &&
    store.isQuotaStepValid &&
    !store.loading,
);

const submitWizard = async () => {
  await store.submit();
};
</script>

<template>
  <section class="space-y-6 px-6 py-8">
    <header class="space-y-2">
      <p class="text-sm text-gray-500">Knowledge Space</p>
      <h1 class="text-2xl font-semibold text-gray-900">创建知识空间</h1>
      <p class="text-gray-600">
        通过下列四个步骤完成租户级空间的配置、策略绑定与配额校验。
      </p>
    </header>

    <div class="grid gap-4 md:grid-cols-3">
      <UCard>
        <div class="space-y-1">
          <p class="text-sm text-gray-500">当前步骤</p>
          <p class="text-lg font-semibold text-gray-900">
            第 {{ store.step }} 步 · {{ stepTitle }}
          </p>
        </div>
      </UCard>
      <UCard v-if="statusSnapshot">
        <p class="text-sm text-gray-500">全局概览</p>
        <p class="text-lg font-semibold text-gray-900">
          等待 IAM {{ statusSnapshot.pendingIam }} · 运行中
          {{ statusSnapshot.active }} · 已退役 {{ statusSnapshot.retired }}
        </p>
      </UCard>
      <UCard v-else>
        <p class="text-sm text-gray-500">全局概览</p>
        <p class="text-lg font-semibold text-gray-900">加载中…</p>
      </UCard>
    </div>

    <UCard :ui="{ body: { padding: 'p-6 space-y-6' } }">
      <template #header>
        <div class="flex items-center justify-between">
          <div>
            <h2 class="text-xl font-semibold text-gray-900">{{ stepTitle }}</h2>
            <p class="text-sm text-gray-500">完成所有字段以进入下一步</p>
          </div>
          <div class="flex items-center gap-2 text-sm text-gray-500">
            <span>步骤 {{ store.step }}/4</span>
          </div>
        </div>
      </template>

      <div v-if="store.step === 1" class="space-y-4">
        <label class="flex flex-col gap-2">
          <span class="text-sm font-medium text-gray-800">租户 UUID</span>
          <input
            type="text"
            class="rounded-lg border border-gray-200 px-3 py-2 text-sm shadow-sm focus:border-primary-500 focus:outline-none"
            placeholder="d86c5da9-35f4-4db8-9c2e-d879ed2b9e10"
            :value="store.form.tenantUuid"
            @input="store.form.tenantUuid = String(($event.target as HTMLInputElement).value)"
          />
        </label>
        <label class="flex flex-col gap-2">
          <span class="text-sm font-medium text-gray-800">空间名称</span>
          <input
            type="text"
            class="rounded-lg border border-gray-200 px-3 py-2 text-sm shadow-sm focus:border-primary-500 focus:outline-none"
            placeholder="ops-handbook"
            :value="store.form.spaceName"
            @input="store.form.spaceName = String(($event.target as HTMLInputElement).value)"
          />
        </label>
        <label class="flex flex-col gap-2">
          <span class="text-sm font-medium text-gray-800">部门编码</span>
          <input
            type="text"
            class="rounded-lg border border-gray-200 px-3 py-2 text-sm shadow-sm focus:border-primary-500 focus:outline-none"
            placeholder="OPS-01"
            :value="store.form.departmentCode"
            @input="
              store.form.departmentCode = String(
                ($event.target as HTMLInputElement).value,
              )
            "
          />
        </label>
      </div>

      <div v-else-if="store.step === 2">
        <PolicySelector
          :model-value="store.form.policyTemplateVersionId"
          :feature-flags="store.form.featureFlags"
          :options="policyOptions"
          @update:model-value="store.form.policyTemplateVersionId = $event"
          @update:feature-flags="store.form.featureFlags = $event"
        />

        <div class="mt-6 grid gap-3 md:grid-cols-2">
          <UCard :ui="{ body: { padding: 'p-4 space-y-3' } }">
            <p class="text-sm font-medium text-gray-800">场景模板 / 默认策略</p>
            <USelect
              :items="scenarioOptions"
              :model-value="store.scenarioTemplate"
              @update:model-value="store.setScenarioTemplate($event)"
            />
            <p class="text-xs text-gray-500">
              当前绑定：ingestion={{ store.form.ingestionProfileKey }} · index={{ store.form.indexProfileKey }} · rag={{ store.form.ragProfileKey }}
            </p>
          </UCard>
          <UCard :ui="{ body: { padding: 'p-4 space-y-2' } }">
            <p class="text-sm font-medium text-gray-800">提示</p>
            <p class="text-xs text-gray-500">
              创建后可在 Retrieval Playground 中对比“空间默认 profile”与“草稿/其他版本”，并结合 Corpus Check 推荐卡片做调参。
            </p>
          </UCard>
        </div>
      </div>

      <div v-else-if="store.step === 3">
        <QuotaForm
          :quotas="store.form.quotas"
          :iam-email="store.iamEmail"
          @update:quotas="store.form.quotas = $event"
          @update:iam-email="store.iamEmail = $event"
        />
      </div>

      <div v-else>
        <AuditPreview
          :payload="store.form"
          :iam-email="store.iamEmail"
          :sla-remaining="store.slaRemaining"
        />
        <div class="mt-4 space-y-3 rounded-lg border border-dashed border-gray-200 p-4">
          <div class="flex items-center justify-between">
            <p class="text-sm font-medium text-gray-800">导入样本文档（可选）</p>
            <UCheckbox v-model="store.sampleDoc.enabled">启用</UCheckbox>
          </div>
          <div v-if="store.sampleDoc.enabled" class="grid grid-cols-1 gap-3 md:grid-cols-3">
            <USelect
              v-model="store.sampleDoc.format"
              :items="[
                { label: 'PDF', value: 'pdf' },
                { label: 'DOCX', value: 'docx' },
                { label: 'XLSX', value: 'xlsx' },
                { label: 'CSV', value: 'csv' },
                { label: 'Markdown', value: 'markdown' },
                { label: 'HTML', value: 'html' },
                { label: 'SQL', value: 'sql' },
                { label: 'Image', value: 'image' },
                { label: 'Table', value: 'table' },
              ]"
              placeholder="格式"
            />
            <UInput
              v-model="store.sampleDoc.sourceUri"
              class="md:col-span-2"
              placeholder="sourceUri（用于生成样本入库记录，随后触发 Corpus Check）"
              icon="i-heroicons-link"
            />
            <div class="md:col-span-3">
              <UCheckbox v-model="store.sampleDoc.ocrRequired">强制 OCR（用于验证 blocked/degraded 指引）</UCheckbox>
            </div>
          </div>
          <p class="text-xs text-gray-500">
            若启用：提交创建后会先触发一次样本入库，再运行 Corpus Check 生成推荐策略卡片。
          </p>
        </div>
        <div class="mt-4 space-y-2 rounded-lg border border-dashed border-gray-200 p-4">
          <UCheckbox v-model="store.runCorpusCheckAfterCreate">
            创建后立即运行语料体检（Corpus Check）
          </UCheckbox>
          <p class="text-xs text-gray-500">
            体检会统计 OCR/格式分布，并给出推荐策略与插件提示（如扫描件占比高将建议启用 OCR 插件）。
          </p>
        </div>
      </div>

      <div class="flex items-center justify-between border-t border-gray-100 pt-4">
        <UButton
          variant="ghost"
          :disabled="store.step === 1 || store.loading"
          @click="store.prevStep()"
        >
          上一步
        </UButton>
        <div class="flex items-center gap-3">
          <p v-if="store.error" class="text-sm text-red-500">{{ store.error }}</p>
          <UButton
            v-if="store.step < 4"
            color="primary"
            :disabled="
              store.loading ||
              (store.step === 1 && !store.isBasicInfoValid) ||
              (store.step === 2 && !store.isPolicyStepValid) ||
              (store.step === 3 && !store.isQuotaStepValid)
            "
            @click="store.nextStep()"
          >
            下一步
          </UButton>
          <UButton
            v-else
            color="primary"
            :loading="store.loading"
            :disabled="!canSubmit"
            @click="submitWizard"
          >
            提交创建
          </UButton>
        </div>
      </div>
    </UCard>

    <div v-if="store.wizardCompleted" class="space-y-4">
      <UAlert
        color="primary"
        variant="soft"
        icon="i-heroicons-check-circle"
        title="空间创建成功"
        description="正在等待 IAM 确认，完成后可继续入库和融合。"
      />
      <IamStatusBadge
        status="pending_iam"
        :audit-token="store.lastSpace?.auditToken"
      />
      <UAlert
        v-if="store.lastIngestionJob"
        :color="store.lastIngestionJob.status === 'blocked' ? 'red' : store.lastIngestionJob.status === 'failed' ? 'red' : 'green'"
        variant="subtle"
        icon="i-heroicons-document-text"
        :title="`样本入库：${store.lastIngestionJob.status}`"
        :description="store.lastIngestionJob.reason ? store.lastIngestionJob.reason : '样本文档已生成入库记录，可用于 Corpus Check 统计与 Playground 对比。'"
      />
      <UAlert
        v-if="store.lastCorpusCheckJob"
        :color="store.lastCorpusCheckJob.status === 'completed' ? 'green' : 'amber'"
        variant="subtle"
        icon="i-heroicons-sparkles"
        :title="`Corpus Check：${store.lastCorpusCheckJob.status}`"
        :description="store.lastCorpusCheckJob.trace_id ? `trace_id: ${store.lastCorpusCheckJob.trace_id}` : '已提交语料体检任务，可在 Playground 中做检索对比。'"
      />
      <UCard v-if="store.lastCorpusCheckJob?.status === 'completed'" :ui="{ body: { padding: 'p-4 space-y-3' } }">
        <div class="flex items-center justify-between">
          <p class="text-sm font-medium text-gray-800">推荐策略卡片</p>
          <UButton to="/knowledge-spaces/playground" variant="ghost" size="xs">打开 Playground</UButton>
        </div>
        <div v-if="Array.isArray(store.lastCorpusCheckJob.recommendations) && store.lastCorpusCheckJob.recommendations.length" class="space-y-2">
          <div
            v-for="(rec, idx) in store.lastCorpusCheckJob.recommendations"
            :key="idx"
            class="rounded-lg border border-gray-100 bg-white p-3"
          >
            <p class="text-sm font-medium text-gray-900">{{ rec.title || rec.key }}</p>
            <p v-if="rec.risk" class="text-xs text-gray-500">风险：{{ rec.risk }}</p>
            <p v-if="rec.cost" class="text-xs text-gray-500">成本：{{ rec.cost }}</p>
            <p v-if="rec.plugin" class="text-xs text-amber-700">插件：{{ rec.plugin }}</p>
          </div>
        </div>
        <UAlert v-else variant="subtle" title="暂无推荐卡片" description="当前样本不足或未触发体检建议。" />
      </UCard>
      <UAlert
        v-if="store.lastCorpusCheckJob?.recommendations?.some((r: any) => r?.plugin === 'com.powerx.plugin.data_forge')"
        color="amber"
        variant="soft"
        icon="i-heroicons-exclamation-triangle"
        title="建议启用 OCR 扩展"
        description="Corpus Check 检测到扫描件占比较高：推荐安装/启用 com.powerx.plugin.data_forge（OCR/Processor），否则可能出现入库 blocked/degraded 与召回下降。"
      />
    </div>
  </section>
</template>
