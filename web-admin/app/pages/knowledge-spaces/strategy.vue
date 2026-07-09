<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useKnowledgeSpaces, type KnowledgeSpaceRecord, type StrategyValidationResult, type VectorIndexStatus } from "~/composables/useKnowledgeSpaces";
import {
  SCENE_CATALOG,
  STRATEGY_PACKAGE_CATALOG,
  STRATEGY_PACKAGE_ORDER,
  type SceneKey,
  type StrategyPackageKey,
} from "~/constants/strategyPackageCatalog";
import { useKnowledgeSpaceStore } from "~/stores/knowledgeSpaces";
import { useEmbeddingGuard } from "~/composables/useEmbeddingGuard";
import { useUserStore } from "~/stores/user";

const { t } = useI18n();
const route = useRoute();
const router = useRouter();

useHead(() => ({
  title: t("knowledgeSpaces.strategy.head.title", "策略配置"),
  meta: [
    {
      name: "description",
      content: t("knowledgeSpaces.strategy.head.description", "选择策略包并将配置应用到指定知识空间。"),
    },
  ],
}));

const api = useKnowledgeSpaces();
const wizardStore = useKnowledgeSpaceStore();
const userStore = useUserStore();
const toast = useToast();
const { ensureEmbeddingReady } = useEmbeddingGuard();
const embeddingReady = ref(false);

const spacesLoading = ref(false);
const spacesError = ref<string | null>(null);
const spaces = ref<KnowledgeSpaceRecord[]>([]);

const selectedSpaceId = ref<string>("");
const selectedSpace = computed(() => spaces.value.find((s) => s.spaceId === selectedSpaceId.value) ?? null);

const querySpaceId = computed(() => String(route.query.spaceId || "").trim());
const spaceLocked = computed(() => Boolean(querySpaceId.value));

const strategyPackageKey = ref<StrategyPackageKey>("H_fusion");

const saving = ref(false);
const savingError = ref<string | null>(null);

const strategyValidation = ref<StrategyValidationResult | null>(null);
const strategyValidationLoading = ref(false);
const strategyValidationError = ref<string | null>(null);

const vectorIndexStatus = ref<VectorIndexStatus | null>(null);
const vectorIndexLoading = ref(false);
const vectorIndexError = ref<string | null>(null);
const embeddingProfileKeyInput = ref<string>("openai/text-embedding-3-small");

const shortId = (id: string | null | undefined) => (id ? String(id).slice(0, 8) : "-");

const spaceLabel = (s: KnowledgeSpaceRecord | null) => {
  if (!s) return "-";
  const name = String(s.spaceName || "").trim() || t("knowledgeSpaces.strategy.space.unnamed", "未命名空间");
  return `${name}（${shortId(s.spaceId)}）`;
};

const spaceItems = computed(() =>
  spaces.value.map((s) => ({
    label: spaceLabel(s),
    value: s.spaceId,
  })),
);

const packageItems = computed(() =>
  STRATEGY_PACKAGE_ORDER.map((key) => ({
    label: STRATEGY_PACKAGE_CATALOG[key]?.label || key,
    value: key,
  })),
);

const selectedPackage = computed(() => STRATEGY_PACKAGE_CATALOG[strategyPackageKey.value]);
const packageSceneLabels = computed(() =>
  packageScenes.value.map((key) => ({
    key,
    label: SCENE_CATALOG[key]?.label || key,
    category: SCENE_CATALOG[key]?.category || "",
  })),
);
const defaultSceneKey = computed<SceneKey>(() => packageScenes.value[0] ?? "custom_expert");
const packageCouplingLabel = computed(() => (selectedPackage.value?.coupling === "strong" ? "强关联" : "弱关联"));
const corpusCheckJob = computed(() => {
  if (!selectedSpace.value) return null;
  const job = wizardStore.lastCorpusCheckJob as any;
  if (!job || job.space_uuid !== selectedSpace.value.spaceId) return null;
  return job;
});
const strategyPackageRecommendation = computed(() => {
  const job = corpusCheckJob.value;
  if (!job || !Array.isArray(job.recommendations)) return null;
  const hit = job.recommendations.find((item: any) => {
    const key =
      item?.strategyPackageKey ||
      item?.strategy_package ||
      item?.packageKey ||
      item?.package_key ||
      item?.value ||
      item?.key;
    return Boolean(key && STRATEGY_PACKAGE_CATALOG[key as StrategyPackageKey]);
  });
  if (!hit) return null;
  const key =
    hit?.strategyPackageKey ||
    hit?.strategy_package ||
    hit?.packageKey ||
    hit?.package_key ||
    hit?.value ||
    hit?.key;
  if (!key || !STRATEGY_PACKAGE_CATALOG[key as StrategyPackageKey]) return null;
  return {
    key: key as StrategyPackageKey,
    reason: hit?.reason || hit?.message || hit?.desc || "",
    risk: hit?.risk || "",
    cost: hit?.cost || "",
    scenes: Array.isArray(hit?.scenes) ? hit.scenes : [],
  };
});

type IndexChannel = "dense" | "sparse" | "hier" | "kg" | "time" | "structured";

const mapIndexPrereqToChannel = (key: string): IndexChannel | null => {
  switch (key) {
    case "index.dense":
      return "dense";
    case "index.sparse":
      return "sparse";
    case "index.hier":
      return "hier";
    case "index.kg":
      return "kg";
    case "index.time_fields":
      return "time";
    case "index.structured_fields":
      return "structured";
    default:
      return null;
  }
};

const indexChannelOrder: IndexChannel[] = ["dense", "sparse", "hier", "kg", "time", "structured"];

const packageIndexChannels = computed<IndexChannel[]>(() => {
  const set = new Set<IndexChannel>();
  for (const k of selectedPackage.value?.dependencies.index ?? []) {
    const ch = mapIndexPrereqToChannel(k);
    if (ch) set.add(ch);
  }
  return indexChannelOrder.filter((x) => set.has(x));
});

const packageRuntimePrereqs = computed<string[]>(() => selectedPackage.value?.dependencies.runtime ?? []);
const packageAssetPrereqs = computed<string[]>(() => selectedPackage.value?.dependencies.assets ?? []);
const packageScenes = computed<SceneKey[]>(() => selectedPackage.value?.recommendedScenes ?? []);

const runtimeLabel = (key: string) => {
  switch (key) {
    case "runtime.evidence_checker":
      return "证据校验器（Evidence Checker）";
    case "routing_policy":
      return "路由策略（Routing Policy）";
    case "query_rewrite":
      return "查询重写（Query Rewrite）";
    case "reranker_model":
      return "重排模型（Reranker）";
    case "score_normalizer":
      return "融合归一化（Score Normalizer）";
    case "llm_generate":
      return "LLM 生成（HyDE）";
    case "graph_query":
      return "图谱查询（KG Query）";
    case "policy_router":
      return "策略路由器（Adaptive）";
    case "consistency_checker":
      return "一致性校验器（Self/CRAG）";
    case "offline_pipeline":
      return "离线管线（Augmentation）";
    case "feedback_workflow":
      return "反馈闭环（Feedback）";
    case "versioning_policy":
      return "版本/时间策略（Time-aware）";
    case "acl_enforcer":
      return "权限过滤（ACL Enforcer）";
    default:
      return key;
  }
};

const channelLabel = (ch: string) => {
  switch (ch) {
    case "dense":
      return "向量（Dense）";
    case "sparse":
      return "稀疏（BM25）";
    case "hier":
      return "层次（Hier）";
    case "kg":
      return "知识图谱（KG）";
    case "time":
      return "时间字段（Time）";
    case "structured":
      return "结构化字段（Structured）";
    default:
      return ch;
  }
};

const profileLabel = (profileKey: string | null | undefined) => {
  const key = String(profileKey || "").trim();
  if (!key) return "-";
  switch (key) {
    case "p0_basic":
      return `P0 基础（${key}）`;
    case "p1_general":
      return `P1 通用推荐（${key}）`;
    case "p2_high_accuracy":
      return `P2 高准确/合规（${key}）`;
    case "p3_kg_strong":
      return `P3 KG 约束（${key}）`;
    default:
      return key || "-";
  }
};

const derivePackageFromProfile = (profileKey: string | null | undefined): StrategyPackageKey => {
  switch (profileKey) {
    case "p0_basic":
      return "A_simple";
    case "p2_high_accuracy":
      return "O_crag";
    case "p3_kg_strong":
      return "K_kg";
    case "p1_general":
    default:
      return "H_fusion";
  }
};

const goBack = async () => {
  if (process.client && window.history.length > 1) {
    router.back();
    return;
  }
  await navigateTo("/knowledge-spaces");
};

const applyStrategyRecommendation = () => {
  const rec = strategyPackageRecommendation.value;
  if (!rec) return;
  strategyPackageKey.value = rec.key;
  toast.add({
    color: "primary",
    title: "已应用推荐策略包",
    description: `${STRATEGY_PACKAGE_CATALOG[rec.key]?.label || rec.key}`,
  });
};

const inferFromSpace = (space: KnowledgeSpaceRecord | null) => {
  if (!space) return;
  const flags = (space.featureFlags ?? []).map((f) => String(f || "").trim().toLowerCase());
  const rawPackage = flags.find((f) => f.startsWith("rag.strategy_package:"))?.slice("rag.strategy_package:".length);
  if (rawPackage && STRATEGY_PACKAGE_CATALOG[rawPackage as StrategyPackageKey]) {
    strategyPackageKey.value = rawPackage as StrategyPackageKey;
    return;
  }
  const profileFallback = space.ragProfileKey || space.indexProfileKey || space.ingestionProfileKey;
  strategyPackageKey.value = derivePackageFromProfile(profileFallback);
};

const refreshStrategyValidation = async () => {
  strategyValidationLoading.value = true;
  strategyValidationError.value = null;
  try {
  const pkg = selectedPackage.value;
  const sceneKey = defaultSceneKey.value;
  const bundleKey = pkg?.recommendedProfileKey ?? "p1_general";
  strategyValidation.value = await api.validateStrategy({ sceneKey, bundleKey });
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
    const pkg = selectedPackage.value;
    const sceneKey = defaultSceneKey.value;
    const profileKey = pkg?.recommendedProfileKey ?? "p1_general";
    const existingFlags = (selectedSpace.value.featureFlags ?? []).map((f) => String(f || "").trim().toLowerCase());
    const kept = existingFlags.filter(
      (f) =>
        !f.startsWith("rag.scene:") &&
        !f.startsWith("rag.bundle:") &&
        !f.startsWith("rag.strategy_package:") &&
        !f.startsWith("rag.primary:") &&
        f !== "rag.guided",
    );
    const nextFlags = [
      ...kept,
      `rag.strategy_package:${strategyPackageKey.value}`,
      `rag.scene:${sceneKey}`,
      `rag.bundle:${profileKey}`,
    ];
    if (sceneKey === "custom_expert") nextFlags.push("rag.guided");

    const updated = await api.updateSpace(selectedSpace.value.spaceId, {
      ingestionProfileKey: profileKey,
      indexProfileKey: profileKey,
      ragProfileKey: profileKey,
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

const resetToSpace = () => {
  if (!selectedSpace.value) return;
  inferFromSpace(selectedSpace.value);
  toast.add({
    color: "neutral",
    title: "已回滚到空间当前配置",
    description: "已恢复为该空间已保存的策略包设置。",
  });
};

const refreshVectorIndex = async () => {
  if (!selectedSpace.value) {
    vectorIndexStatus.value = null;
    return;
  }
  vectorIndexLoading.value = true;
  vectorIndexError.value = null;
  try {
    vectorIndexStatus.value = await api.getVectorIndexStatus(selectedSpace.value.spaceId);
  } catch (e: any) {
    vectorIndexError.value = e?.message || "获取向量索引状态失败";
    vectorIndexStatus.value = null;
  } finally {
    vectorIndexLoading.value = false;
  }
};

const activateDenseIndex = async () => {
  if (!selectedSpace.value) return;
  const key = String(embeddingProfileKeyInput.value || "").trim();
  if (!key) {
    toast.add({ color: "error", title: "参数错误", description: "embeddingProfileKey 不能为空" });
    return;
  }
  saving.value = true;
  savingError.value = null;
  try {
    await api.activateVectorIndex(selectedSpace.value.spaceId, {
      embeddingProfileKey: key,
      requestedBy: userStore.user?.email || wizardStore.iamEmail || "ops@powerx.local",
    });
    await loadSpaces();
    await refreshVectorIndex();
    await refreshStrategyValidation();
    toast.add({ color: "success", title: "向量索引已激活", description: "已为该空间绑定 embedding profile 并创建/启用向量表。" });
  } catch (e: any) {
    savingError.value = e?.message || "激活失败";
    toast.add({ color: "error", title: "激活失败", description: savingError.value });
  } finally {
    saving.value = false;
  }
};

const loadSpaces = async () => {
  spacesLoading.value = true;
  spacesError.value = null;
  try {
    spaces.value = await api.listSpaces({ limit: 200 });
    const preferred = querySpaceId.value;
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
    if (!embeddingReady.value) return;
    inferFromSpace(space);
    embeddingProfileKeyInput.value = String(space?.embeddingProfileKey || "").trim() || "openai/text-embedding-3-small";
    refreshVectorIndex();
  },
  { immediate: true },
);

watch(strategyPackageKey, async () => {
  if (!embeddingReady.value) return;
  await refreshStrategyValidation();
});

onMounted(async () => {
  if (!(await ensureEmbeddingReady())) return;
  embeddingReady.value = true;
  try {
    await userStore.fetchUserContext();
  } catch {
    // ignore
  }
  await loadSpaces();
  await refreshStrategyValidation();
  await refreshVectorIndex();
});
</script>

<template>
  <section class="px-6 py-8 space-y-8 lg:px-10">
    <header class="rounded-2xl border border-gray-200 bg-white p-6 shadow-sm">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div class="flex items-start gap-3">
          <UButton color="neutral" variant="ghost" icon="i-heroicons-arrow-left" @click="goBack">
            {{ t("common.backToList", "返回列表") }}
          </UButton>
          <div>
            <h1 class="text-2xl font-semibold text-gray-900">{{ t("knowledgeSpaces.strategy.title", "策略配置") }}</h1>
          </div>
        </div>
      </div>
    </header>

    <UCard>
      <template #header>
        <div class="flex items-center justify-between gap-3">
          <div>
            <h2 class="text-lg font-semibold">{{ t("knowledgeSpaces.strategy.title", "策略配置") }}</h2>
            <p class="text-sm text-[var(--text-secondary)]">{{ t("knowledgeSpaces.strategy.subtitle") }}</p>
          </div>
          <div class="flex items-center gap-2">
            <UButton color="neutral" variant="soft" icon="i-heroicons-arrow-path" :loading="spacesLoading" @click="loadSpaces">
              {{ t("common.refresh", "刷新") }}
            </UButton>
            <UButton color="neutral" variant="subtle" icon="i-heroicons-arrow-uturn-left" :disabled="!selectedSpace" @click="resetToSpace">
              {{ t("common.rollback", "回滚") }}
            </UButton>
            <UButton color="primary" icon="i-heroicons-check" :loading="saving" :disabled="!selectedSpace" @click="persistToSpace">
              {{ t("knowledgeSpaces.strategy.actions.save", "保存到空间") }}
            </UButton>
          </div>
        </div>
      </template>

      <UAlert
        v-if="!spaceLocked"
        color="warning"
        variant="soft"
        :title="t('knowledgeSpaces.strategy.space.title', '选择空间')"
        description="请从空间列表点击“策略”进入（URL 需要携带 spaceId）。"
      />
      <div v-else-if="spacesError" class="text-sm text-red-500">{{ spacesError }}</div>
      <div v-else class="space-y-4">
        <div class="grid gap-4 md:grid-cols-2">
          <div class="rounded-lg border border-[var(--border-color)] p-4 text-sm">
            <div class="font-medium text-[var(--text-primary)]">{{ t("knowledgeSpaces.strategy.space.label", "空间") }}</div>
            <div class="mt-1 text-[var(--text-secondary)]">
              {{ spaceLabel(selectedSpace) }}
            </div>
            <div class="mt-2 text-[var(--text-secondary)]">
              {{ t("knowledgeSpaces.strategy.space.id", "空间 ID") }}：{{ shortId(selectedSpace?.spaceId) }}…
            </div>

            <div class="mt-3 pt-3 border-t border-[var(--border-color)] space-y-1 text-[var(--text-secondary)]">
              <div class="font-medium text-[var(--text-primary)]">当前（已写入空间）</div>
              <div>入库 Profile：{{ profileLabel(selectedSpace?.ingestionProfileKey) }}</div>
              <div>索引 Profile：{{ profileLabel(selectedSpace?.indexProfileKey) }}</div>
              <div>RAG Profile：{{ profileLabel(selectedSpace?.ragProfileKey) }}</div>
              <div class="pt-2 mt-2 border-t border-[var(--border-color)]">
                <div class="font-medium text-[var(--text-primary)]">向量索引（Dense）</div>
                <div>Embedding Profile：{{ selectedSpace?.embeddingProfileKey || "-" }}</div>
                <div>Active Index Key：{{ selectedSpace?.activeVectorIndexKey || "-" }}</div>
                <div v-if="vectorIndexLoading" class="text-xs text-[var(--text-secondary)]">加载中…</div>
                <div v-else-if="vectorIndexError" class="text-xs text-red-500">{{ vectorIndexError }}</div>
                <div v-else-if="vectorIndexStatus?.active" class="text-xs text-[var(--text-secondary)]">
                  dims={{ vectorIndexStatus.active.dimensions }} · table={{ vectorIndexStatus.active.table_name }}
                </div>
              </div>
            </div>
          </div>

          <div class="rounded-lg border border-[var(--border-color)] p-4">
            <UFormField :label="t('knowledgeSpaces.strategy.package', '策略包（A0–O）')" required>
              <USelectMenu
                v-model="strategyPackageKey"
                :items="packageItems"
                value-key="value"
                label-key="label"
                :search-input="{ placeholder: '搜索策略包' }"
                :portal="true"
                class="w-full"
                :ui="{ content: 'z-[80] min-w-[min(90vw,900px)] max-h-72 overflow-y-auto', item: 'whitespace-normal break-words' }"
              />
              <template #help>
                <div class="text-[var(--text-secondary)]">{{ selectedPackage?.summary }}</div>
              </template>
            </UFormField>

            <div class="mt-3 grid gap-3 md:grid-cols-2 text-sm">
              <div class="rounded-lg border border-[var(--border-color)] p-3">
                <div class="font-medium text-[var(--text-primary)]">策略要点</div>
                <div class="mt-2 text-[var(--text-secondary)]">
                  <div>阶段：{{ selectedPackage?.phase || "-" }}</div>
                  <div>联动强度：{{ packageCouplingLabel }}</div>
                  <div>
                    推荐 Profile：
                    {{ profileLabel(selectedPackage?.recommendedProfileKey) }}
                  </div>
                </div>
              </div>
              <div class="rounded-lg border border-[var(--border-color)] p-3">
                <div class="font-medium text-[var(--text-primary)]">适用场景（映射说明）</div>
                <div class="mt-2 flex flex-wrap gap-2">
                  <UBadge
                    v-for="scene in packageSceneLabels"
                    :key="scene.key"
                    color="neutral"
                    variant="soft"
                  >
                    {{ scene.label }}
                  </UBadge>
                  <span v-if="!packageSceneLabels.length" class="text-[var(--text-secondary)]">无</span>
                </div>
              </div>
            </div>

            <div class="mt-3 pt-3 border-t border-[var(--border-color)] text-sm text-[var(--text-secondary)] space-y-1">
              <div class="font-medium text-[var(--text-primary)]">将写入（点击“保存到空间”后）</div>
              <div>策略包：{{ selectedPackage?.label }}（{{ strategyPackageKey }}）</div>
              <div>推荐 Profile：{{ profileLabel(selectedPackage?.recommendedProfileKey) }}</div>
              <div>兼容场景标签：{{ SCENE_CATALOG[defaultSceneKey]?.label || defaultSceneKey }}</div>
            </div>

            <div v-if="corpusCheckJob" class="mt-4 rounded-lg border border-[var(--border-color)] p-3 text-sm">
              <div class="flex flex-wrap items-center justify-between gap-2">
                <div class="font-medium text-[var(--text-primary)]">Corpus Check 推荐</div>
                <UBadge color="primary" variant="soft">{{ corpusCheckJob.status || "unknown" }}</UBadge>
              </div>
              <div class="mt-2 text-[var(--text-secondary)]">
                Trace：{{ corpusCheckJob.trace_id || "-" }}
              </div>
              <div v-if="strategyPackageRecommendation" class="mt-3 flex flex-wrap items-center gap-3">
                <div class="text-[var(--text-secondary)]">
                  推荐策略包：
                  <span class="font-medium text-[var(--text-primary)]">
                    {{ STRATEGY_PACKAGE_CATALOG[strategyPackageRecommendation.key]?.label || strategyPackageRecommendation.key }}
                  </span>
                </div>
                <UButton size="xs" color="primary" variant="soft" @click="applyStrategyRecommendation">
                  应用推荐
                </UButton>
              </div>
              <div v-if="strategyPackageRecommendation?.reason" class="mt-2 text-[var(--text-secondary)]">
                {{ strategyPackageRecommendation.reason }}
              </div>
              <div v-if="strategyPackageRecommendation?.risk" class="mt-2 text-[var(--text-secondary)]">
                风险提示：{{ strategyPackageRecommendation.risk }}
              </div>
              <div v-if="strategyPackageRecommendation?.cost" class="mt-1 text-[var(--text-secondary)]">
                成本提示：{{ strategyPackageRecommendation.cost }}
              </div>
              <div v-if="strategyPackageRecommendation?.scenes?.length" class="mt-2 flex flex-wrap gap-2">
                <UBadge
                  v-for="scene in strategyPackageRecommendation.scenes"
                  :key="scene"
                  color="neutral"
                  variant="soft"
                >
                  {{ SCENE_CATALOG[scene as SceneKey]?.label || scene }}
                </UBadge>
              </div>
              <div v-else-if="!strategyPackageRecommendation" class="mt-2 text-[var(--text-secondary)]">
                暂无可用的策略包推荐。
              </div>
            </div>

            <div class="mt-4 pt-4 border-t border-[var(--border-color)]">
              <div class="flex items-center justify-between gap-3">
                <div>
                  <div class="font-medium text-[var(--text-primary)]">激活向量索引（Dense）</div>
                  <div class="text-sm text-[var(--text-secondary)]">
                    只在空间层面绑定/激活（不会在 AI Settings 测试时建表）。
                  </div>
                </div>
                <UButton
                  color="primary"
                  icon="i-heroicons-bolt"
                  :loading="saving"
                  :disabled="!selectedSpace"
                  @click="activateDenseIndex"
                >
                  激活
                </UButton>
              </div>
              <div class="mt-3 grid gap-3 md:grid-cols-2">
                <UFormField label="EmbeddingProfileKey（provider/model）" required>
                  <UInput v-model="embeddingProfileKeyInput" placeholder="openai/text-embedding-3-small" />
                  <template #help>
                    <div class="text-[var(--text-secondary)]">示例：openai/text-embedding-3-small</div>
                  </template>
                </UFormField>
                <div class="text-sm text-[var(--text-secondary)] rounded-lg border border-[var(--border-color)] p-3">
                  <div class="font-medium text-[var(--text-primary)]">说明</div>
                  <div class="mt-1">激活时后端会：probe 维度 → `CREATE TABLE IF NOT EXISTS` → 写入索引登记表 → 更新 space 绑定。</div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <div class="rounded-lg border border-[var(--border-color)] p-4">
          <div class="flex flex-wrap items-center justify-between gap-3">
            <div>
              <div class="font-medium text-[var(--text-primary)]">依赖摘要</div>
              <div class="text-sm text-[var(--text-secondary)]">提示该策略包需要的索引通道、运行时能力与产物。</div>
            </div>
            <div class="flex flex-wrap gap-2" />
          </div>
          <div class="mt-3 grid gap-3 md:grid-cols-3 text-sm">
            <div class="rounded-lg border border-[var(--border-color)] p-3">
              <div class="font-medium text-[var(--text-primary)]">索引通道依赖</div>
              <div class="mt-2 flex flex-wrap gap-2">
                <UBadge v-for="ch in packageIndexChannels" :key="ch" color="primary" variant="soft">
                  {{ channelLabel(ch) }}
                </UBadge>
                <span v-if="!packageIndexChannels.length" class="text-[var(--text-secondary)]">无</span>
              </div>
            </div>
            <div class="rounded-lg border border-[var(--border-color)] p-3">
              <div class="font-medium text-[var(--text-primary)]">运行时能力</div>
              <div class="mt-2 flex flex-wrap gap-2">
                <UBadge v-for="k in packageRuntimePrereqs" :key="k" color="neutral" variant="soft">
                  {{ runtimeLabel(k) }}
                </UBadge>
                <span v-if="!packageRuntimePrereqs.length" class="text-[var(--text-secondary)]">无</span>
              </div>
            </div>
            <div class="rounded-lg border border-[var(--border-color)] p-3">
              <div class="font-medium text-[var(--text-primary)]">离线/索引产物</div>
              <div class="mt-2 flex flex-wrap gap-2">
                <UBadge v-for="k in packageAssetPrereqs" :key="k" color="neutral" variant="soft">
                  {{ k }}
                </UBadge>
                <span v-if="!packageAssetPrereqs.length" class="text-[var(--text-secondary)]">无</span>
              </div>
            </div>
          </div>
        </div>

        <div>
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
      </div>
    </UCard>
  </section>
</template>
