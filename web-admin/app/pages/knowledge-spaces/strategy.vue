<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useKnowledgeSpaces, type CorpusCheckJobRecord, type KnowledgeSpaceRecord, type StrategyValidationResult } from "~/composables/useKnowledgeSpaces";
import { SCENE_STRATEGY_CATALOG, type SceneKey, type StrategyBundleKey } from "~/constants/sceneStrategyCatalog";
import { useKnowledgeSpaceStore } from "~/stores/knowledgeSpaces";
import { useUserStore } from "~/stores/user";

const { t } = useI18n();
const route = useRoute();

useHead(() => ({
  title: t("knowledgeSpaces.strategy.head.title", "策略配置"),
  meta: [
    {
      name: "description",
      content: t("knowledgeSpaces.strategy.head.description", "选择场景与策略包，并将配置应用到指定知识空间。"),
    },
  ],
}));

const api = useKnowledgeSpaces();
const wizardStore = useKnowledgeSpaceStore();
const userStore = useUserStore();
const toast = useToast();

const spacesLoading = ref(false);
const spacesError = ref<string | null>(null);
const spaces = ref<KnowledgeSpaceRecord[]>([]);

const selectedSpaceId = ref<string>("");
const selectedSpace = computed(() => spaces.value.find((s) => s.spaceId === selectedSpaceId.value) ?? null);

const sceneKey = ref<SceneKey>("sop");
const bundleKey = ref<StrategyBundleKey>("p1_general");

const saving = ref(false);
const savingError = ref<string | null>(null);

const strategyValidation = ref<StrategyValidationResult | null>(null);
const strategyValidationLoading = ref(false);
const strategyValidationError = ref<string | null>(null);

const corpusCheckJob = ref<CorpusCheckJobRecord | null>(null);
const corpusCheckLoading = ref(false);
const corpusCheckError = ref<string | null>(null);

const spaceItems = computed(() =>
  spaces.value.map((s) => ({
    label: s.spaceName ? `${s.spaceName}（${s.departmentCode}）` : s.spaceId,
    value: s.spaceId,
  })),
);

const sceneItems = computed(() =>
  (Object.entries(SCENE_STRATEGY_CATALOG.scenes) as Array<[SceneKey, any]>).map(([key, scene]) => ({
    label: scene.label,
    value: key,
  })),
);

const bundleItems = computed(() => {
  const scene = SCENE_STRATEGY_CATALOG.scenes[sceneKey.value];
  const allowed = scene?.allowedBundles ?? [];
  return allowed.map((key: StrategyBundleKey) => ({
    label: SCENE_STRATEGY_CATALOG.bundles[key].label,
    value: key,
  }));
});

const selectedScene = computed(() => SCENE_STRATEGY_CATALOG.scenes[sceneKey.value]);
const selectedBundle = computed(() => SCENE_STRATEGY_CATALOG.bundles[bundleKey.value]);

const enabledIndexChannels = computed(() => {
  const scene = SCENE_STRATEGY_CATALOG.scenes[sceneKey.value];
  const bundle = SCENE_STRATEGY_CATALOG.bundles[bundleKey.value];
  const idx = new Set<string>();
  for (const k of scene?.prerequisites.index ?? []) idx.add(k);
  for (const k of bundle?.prerequisites ?? []) idx.add(k);

  const out: Array<"dense" | "sparse" | "hier" | "kg" | "time" | "structured"> = [];
  const mapKey = (key: string) => {
    switch (key) {
      case "index.dense":
        out.push("dense");
        break;
      case "index.sparse":
        out.push("sparse");
        break;
      case "index.hier":
        out.push("hier");
        break;
      case "index.kg":
        out.push("kg");
        break;
      case "index.time_fields":
        out.push("time");
        break;
      case "index.structured_fields":
        out.push("structured");
        break;
    }
  };
  for (const k of idx) {
    if (k.startsWith("index.")) mapKey(k);
  }
  const order = ["dense", "sparse", "hier", "kg", "time", "structured"] as const;
  return order.filter((x) => out.includes(x));
});

const channelLabel = (ch: string) => {
  switch (ch) {
    case "dense":
      return "Dense";
    case "sparse":
      return "Sparse(BM25)";
    case "hier":
      return "Hier";
    case "kg":
      return "KG";
    case "time":
      return "Time";
    case "structured":
      return "Structured";
    default:
      return ch;
  }
};

const inferFromSpace = (space: KnowledgeSpaceRecord | null) => {
  if (!space) return;
  const flags = (space.featureFlags ?? []).map((f) => String(f || "").trim().toLowerCase());
  const rawScene = flags.find((f) => f.startsWith("rag.scene:"))?.slice("rag.scene:".length) as SceneKey | undefined;
  const rawBundle = flags.find((f) => f.startsWith("rag.bundle:"))?.slice("rag.bundle:".length) as StrategyBundleKey | undefined;

  const resolvedScene: SceneKey = rawScene && SCENE_STRATEGY_CATALOG.scenes[rawScene] ? rawScene : "sop";
  const scene = SCENE_STRATEGY_CATALOG.scenes[resolvedScene];
  const resolvedBundle: StrategyBundleKey =
    rawBundle && scene.allowedBundles.includes(rawBundle) ? rawBundle : scene.defaultBundle;

  sceneKey.value = resolvedScene;
  bundleKey.value = resolvedBundle;
};

const refreshStrategyValidation = async () => {
  strategyValidationLoading.value = true;
  strategyValidationError.value = null;
  try {
    strategyValidation.value = await api.validateStrategy({
      sceneKey: sceneKey.value,
      bundleKey: bundleKey.value,
    });
  } catch (e: any) {
    strategyValidationError.value = e?.message || t("knowledgeSpaces.strategy.validation.failed", "策略依赖校验失败");
    strategyValidation.value = null;
  } finally {
    strategyValidationLoading.value = false;
  }
};

const persistToSpace = async () => {
  if (!selectedSpace.value) return;
  saving.value = true;
  savingError.value = null;
  try {
    const existingFlags = (selectedSpace.value.featureFlags ?? []).map((f) => String(f || "").trim().toLowerCase());
    const kept = existingFlags.filter(
      (f) => !f.startsWith("rag.scene:") && !f.startsWith("rag.bundle:") && f !== "rag.guided",
    );
    const nextFlags = [...kept, `rag.scene:${sceneKey.value}`, `rag.bundle:${bundleKey.value}`];
    if (sceneKey.value === "custom_expert") nextFlags.push("rag.guided");

    const updated = await api.updateSpace(selectedSpace.value.spaceId, {
      ingestionProfileKey: bundleKey.value,
      indexProfileKey: bundleKey.value,
      ragProfileKey: bundleKey.value,
      featureFlags: nextFlags,
      updatedBy: userStore.user?.email || wizardStore.iamEmail || "ops@powerx.local",
    });

    const idx = spaces.value.findIndex((s) => s.spaceId === updated.spaceId);
    if (idx >= 0) spaces.value[idx] = updated;
    toast.add({
      color: "success",
      title: t("knowledgeSpaces.strategy.toast.savedTitle", "策略已保存"),
      description: t("knowledgeSpaces.strategy.toast.savedDesc", "已将场景/策略包与三类 Profile 写入该空间。"),
    });
  } catch (e: any) {
    savingError.value = e?.message || t("knowledgeSpaces.strategy.toast.savedFailed", "保存失败");
    toast.add({ color: "error", title: t("knowledgeSpaces.strategy.toast.savedFailed", "保存失败"), description: savingError.value });
  } finally {
    saving.value = false;
  }
};

const runCorpusCheck = async () => {
  if (!selectedSpace.value) return;
  corpusCheckLoading.value = true;
  corpusCheckError.value = null;
  try {
    const created = await api.startCorpusCheck(selectedSpace.value.spaceId, userStore.user?.email || wizardStore.iamEmail || "");
    corpusCheckJob.value = created;
    wizardStore.lastCorpusCheckJob = created as any;
    for (let i = 0; i < 12; i++) {
      const latest = await api.getCorpusCheckJob(selectedSpace.value.spaceId, created.uuid);
      corpusCheckJob.value = latest;
      wizardStore.lastCorpusCheckJob = latest as any;
      if (latest?.status === "completed" || latest?.status === "failed") break;
      await new Promise((r) => setTimeout(r, 1000));
    }
    toast.add({
      color: "success",
      title: t("knowledgeSpaces.strategy.corpus.toast.title", "Corpus Check 已完成"),
      description: t("knowledgeSpaces.strategy.corpus.toast.desc", "已生成推荐卡片，可一键应用到当前空间。"),
    });
  } catch (e: any) {
    corpusCheckError.value = e?.message || t("knowledgeSpaces.strategy.corpus.toast.failed", "启动 Corpus Check 失败");
    toast.add({ color: "error", title: t("knowledgeSpaces.strategy.corpus.toast.failed", "启动 Corpus Check 失败"), description: corpusCheckError.value });
  } finally {
    corpusCheckLoading.value = false;
  }
};

const applyRecommendation = async (rec: any) => {
  if (!rec?.sceneKey || !rec?.bundleKey) return;
  sceneKey.value = rec.sceneKey as SceneKey;
  bundleKey.value = rec.bundleKey as StrategyBundleKey;
  await refreshStrategyValidation();
  await persistToSpace();
};

const openPluginMarket = (pluginId: string) => {
  navigateTo(`/plugins/market?pluginId=${encodeURIComponent(pluginId)}`);
};

const openIngestionWithOcr = () => {
  if (!selectedSpace.value?.spaceId) return;
  navigateTo({ path: "/knowledge-spaces", query: { openIngestion: "1", spaceId: selectedSpace.value.spaceId, ocr: "1" } });
};

const recommendations = computed(() => {
  const list = corpusCheckJob.value?.recommendations ?? (wizardStore.lastCorpusCheckJob as any)?.recommendations;
  return Array.isArray(list) ? list : [];
});

const loadSpaces = async () => {
  spacesLoading.value = true;
  spacesError.value = null;
  try {
    spaces.value = await api.listSpaces({ limit: 200 });
    const preferred = String(route.query.spaceId || "").trim();
    if (preferred && spaces.value.some((s) => s.spaceId === preferred)) {
      selectedSpaceId.value = preferred;
    } else if (!selectedSpaceId.value && spaces.value[0]?.spaceId) {
      selectedSpaceId.value = spaces.value[0].spaceId;
    }
  } catch (e: any) {
    spacesError.value = e?.message || t("knowledgeSpaces.strategy.loadFailed", "加载空间失败");
    spaces.value = [];
  } finally {
    spacesLoading.value = false;
  }
};

watch(
  () => selectedSpace.value,
  (space) => {
    inferFromSpace(space);
    const last = wizardStore.lastCorpusCheckJob as any;
    if (space && last?.space_uuid === space.spaceId) {
      corpusCheckJob.value = last;
    } else {
      corpusCheckJob.value = null;
    }
  },
  { immediate: true },
);

watch([sceneKey, bundleKey], async () => {
  await refreshStrategyValidation();
});

onMounted(async () => {
  try {
    await userStore.fetchUserContext();
  } catch {
    // ignore
  }
  await loadSpaces();
  await refreshStrategyValidation();
});
</script>

<template>
  <section class="px-6 py-8 space-y-8 lg:px-10">
    <header class="rounded-2xl border border-gray-200 bg-white p-6 shadow-sm">
      <h1 class="text-2xl font-semibold text-gray-900">{{ t("knowledgeSpaces.strategy.title", "策略配置") }}</h1>
      <p class="text-gray-600 mt-2">
        {{ t("knowledgeSpaces.strategy.subtitle", "先选择空间，再选择业务场景（L1）与策略包（L2）。保存后会同步更新该空间的 Ingestion/Index/RAG Profile。") }}
      </p>
    </header>

    <UCard>
      <template #header>
        <div class="flex items-center justify-between gap-3">
          <div>
            <h2 class="text-lg font-semibold">{{ t("knowledgeSpaces.strategy.space.title", "选择空间") }}</h2>
            <p class="text-sm text-[var(--text-secondary)]">{{ t("knowledgeSpaces.strategy.space.desc", "策略配置是租户级复用，但会写入到某个具体空间。") }}</p>
          </div>
          <UButton color="neutral" variant="soft" icon="i-heroicons-arrow-path" :loading="spacesLoading" @click="loadSpaces">
            {{ t("common.refresh", "刷新") }}
          </UButton>
        </div>
      </template>

      <div v-if="spacesError" class="text-sm text-red-500">{{ spacesError }}</div>
      <div v-else class="grid gap-4 md:grid-cols-2">
        <UFormField :label="t('knowledgeSpaces.strategy.space.label', '空间')" required>
          <USelectMenu v-model="selectedSpaceId" :items="spaceItems" class="w-full" />
        </UFormField>
        <div class="rounded-lg border border-[var(--border-color)] p-4 text-sm">
          <div class="font-medium text-[var(--text-primary)]">{{ t("knowledgeSpaces.strategy.space.current", "当前空间配置") }}</div>
          <div class="mt-2 text-[var(--text-secondary)]">
            <div>IngestionProfileKey：{{ selectedSpace?.ingestionProfileKey || "-" }}</div>
            <div>IndexProfileKey：{{ selectedSpace?.indexProfileKey || "-" }}</div>
            <div>RAGProfileKey：{{ selectedSpace?.ragProfileKey || "-" }}</div>
            <div class="mt-2">
              {{ t("knowledgeSpaces.strategy.space.id", "空间 ID") }}：{{ selectedSpace?.spaceId?.slice(0, 8) }}…
            </div>
          </div>
        </div>
      </div>
    </UCard>

    <UCard>
      <template #header>
        <div class="flex items-center justify-between gap-3">
          <div>
            <h2 class="text-lg font-semibold">{{ t("knowledgeSpaces.strategy.l1l2.title", "场景（L1）→ 策略包（L2）") }}</h2>
            <p class="text-sm text-[var(--text-secondary)]">{{ t("knowledgeSpaces.strategy.l1l2.desc", "先选场景，再只展示该场景允许的策略包。") }}</p>
          </div>
          <UButton color="primary" icon="i-heroicons-check" :loading="saving" :disabled="!selectedSpace" @click="persistToSpace">
            {{ t("knowledgeSpaces.strategy.actions.save", "保存到空间") }}
          </UButton>
        </div>
      </template>

      <div class="grid gap-4 md:grid-cols-2">
        <UFormField :label="t('knowledgeSpaces.strategy.scene', '业务场景（L1）')" required>
          <USelectMenu v-model="sceneKey" :items="sceneItems" class="w-full" />
          <template #help>
            <div class="text-[var(--text-secondary)]">{{ selectedScene?.description }}</div>
          </template>
        </UFormField>
        <UFormField :label="t('knowledgeSpaces.strategy.bundle', '策略包（L2）')" required>
          <USelectMenu v-model="bundleKey" :items="bundleItems" class="w-full" />
          <template #help>
            <div class="text-[var(--text-secondary)]">{{ selectedBundle?.description }}</div>
          </template>
        </UFormField>
      </div>

      <div class="mt-4 rounded-lg border border-[var(--border-color)] p-4">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div>
            <div class="font-medium text-[var(--text-primary)]">{{ t("knowledgeSpaces.strategy.channels.title", "将启用的索引通道") }}</div>
            <div class="text-sm text-[var(--text-secondary)]">{{ t("knowledgeSpaces.strategy.channels.desc", "用于判断依赖是否满足，以及后续策略推荐的成本/风险。") }}</div>
          </div>
          <div class="flex flex-wrap gap-2">
            <UBadge v-for="ch in enabledIndexChannels" :key="ch" color="primary" variant="soft">
              {{ channelLabel(ch) }}
            </UBadge>
          </div>
        </div>
      </div>

      <div class="mt-4">
        <div v-if="strategyValidationLoading" class="text-sm text-[var(--text-secondary)]">
          {{ t("knowledgeSpaces.strategy.validation.loading", "正在校验策略依赖…") }}
        </div>
        <div v-else-if="strategyValidationError" class="text-sm text-red-500">{{ strategyValidationError }}</div>
        <div v-else-if="strategyValidation" class="space-y-3">
          <div v-if="strategyValidation.ok" class="rounded-lg border border-green-200 bg-green-50 p-4 text-sm text-green-700">
            {{ t("knowledgeSpaces.strategy.validation.ok", "依赖满足：可以发布/激活。") }}
          </div>
          <div v-else class="rounded-lg border border-yellow-200 bg-yellow-50 p-4 text-sm text-yellow-800 space-y-2">
            <div class="font-medium">{{ t("knowledgeSpaces.strategy.validation.blocked", "依赖不满足：将阻止激活/发布。") }}</div>
            <ul class="list-disc pl-5 space-y-1">
              <li v-for="m in strategyValidation.missing" :key="m.code">
                <span class="font-medium">{{ m.message }}</span>
                <div v-if="m.remediation?.length" class="text-[var(--text-secondary)] mt-1">
                  {{ t("knowledgeSpaces.strategy.validation.remediation", "修复建议：") }}
                  <span v-for="(r, i) in m.remediation" :key="r">
                    {{ r }}<span v-if="i < m.remediation.length - 1">；</span>
                  </span>
                </div>
              </li>
            </ul>
          </div>
        </div>

        <div v-if="savingError" class="mt-3 text-sm text-red-500">{{ savingError }}</div>
      </div>
    </UCard>

    <UCard>
      <template #header>
        <div class="flex items-center justify-between gap-3">
          <div>
            <h2 class="text-lg font-semibold">{{ t("knowledgeSpaces.strategy.corpus.title", "Corpus Check 推荐") }}</h2>
            <p class="text-sm text-[var(--text-secondary)]">
              {{ t("knowledgeSpaces.strategy.corpus.desc", "导入首批样本文档后建议跑一次体检，系统会给出推荐的场景/策略包（非全量映射）。") }}
            </p>
          </div>
          <UButton color="secondary" variant="soft" icon="i-heroicons-beaker" :loading="corpusCheckLoading" :disabled="!selectedSpace" @click="runCorpusCheck">
            {{ t("knowledgeSpaces.strategy.corpus.actions.run", "开始体检") }}
          </UButton>
        </div>
      </template>

      <div v-if="corpusCheckError" class="text-sm text-red-500">{{ corpusCheckError }}</div>
      <div v-else-if="!selectedSpace" class="text-sm text-[var(--text-secondary)]">
        {{ t("knowledgeSpaces.strategy.corpus.hint.noSpace", "请先选择空间。") }}
      </div>
      <div v-else-if="!recommendations.length" class="text-sm text-[var(--text-secondary)]">
        {{ t("knowledgeSpaces.strategy.corpus.hint.empty", "暂无推荐。你可以先入库一份样本文档，再点击“开始体检”。") }}
      </div>
      <div v-else class="space-y-3">
        <div
          v-for="rec in recommendations"
          :key="rec.type + ':' + (rec.sceneKey || '') + ':' + (rec.bundleKey || '')"
          class="rounded-lg border border-[var(--border-color)] p-4"
        >
          <div class="flex flex-wrap items-start justify-between gap-3">
            <div class="space-y-2">
              <div class="flex flex-wrap gap-2">
                <UBadge v-if="rec.type" color="neutral" variant="soft">{{ rec.type }}</UBadge>
                <UBadge v-if="rec.sceneKey" color="primary" variant="soft">场景：{{ rec.sceneLabel || rec.sceneKey }}</UBadge>
                <UBadge v-if="rec.bundleKey" color="primary" variant="soft">策略包：{{ rec.bundleLabel || rec.bundleKey }}</UBadge>
              </div>
              <div v-if="rec.reason" class="text-sm text-[var(--text-secondary)]">{{ rec.reason }}</div>
              <div v-if="rec.cost || rec.risk" class="text-xs text-[var(--text-secondary)]">
                <span v-if="rec.cost">成本：{{ rec.cost }}</span>
                <span v-if="rec.cost && rec.risk">｜</span>
                <span v-if="rec.risk">风险：{{ rec.risk }}</span>
              </div>
            </div>
            <UButton
              v-if="rec.type === 'scene_bundle' && rec.sceneKey && rec.bundleKey"
              size="sm"
              color="primary"
              variant="soft"
              icon="i-heroicons-sparkles"
              :disabled="saving || corpusCheckLoading"
              @click="applyRecommendation(rec)"
            >
              {{ t("knowledgeSpaces.strategy.corpus.actions.apply", "一键应用") }}
            </UButton>
            <div v-else-if="rec.key === 'enable_ocr'" class="flex flex-wrap gap-2">
              <UButton
                v-if="rec.plugin"
                size="sm"
                color="primary"
                variant="soft"
                icon="i-heroicons-shopping-bag"
                @click="openPluginMarket(rec.plugin)"
              >
                去安装 OCR 插件
              </UButton>
              <UButton size="sm" color="neutral" variant="soft" @click="openIngestionWithOcr">
                打开入库并启用 OCR
              </UButton>
            </div>
          </div>
        </div>
      </div>
    </UCard>
  </section>
</template>
