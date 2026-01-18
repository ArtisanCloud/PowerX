<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useKnowledgeSpaces, type KnowledgeSpaceRecord, type StrategyValidationResult, type VectorIndexStatus } from "~/composables/useKnowledgeSpaces";
import { SCENE_STRATEGY_CATALOG, type SceneKey, type StrategyBundleKey } from "~/constants/sceneStrategyCatalog";
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
      content: t("knowledgeSpaces.strategy.head.description", "选择场景与策略包，并将配置应用到指定知识空间。"),
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

const sceneKey = ref<SceneKey>("sop");
const bundleKey = ref<StrategyBundleKey>("p1_general");

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

const sceneIndexChannels = computed<IndexChannel[]>(() => {
  const scene = SCENE_STRATEGY_CATALOG.scenes[sceneKey.value];
  const set = new Set<IndexChannel>();
  for (const k of scene?.prerequisites.index ?? []) {
    const ch = mapIndexPrereqToChannel(k);
    if (ch) set.add(ch);
  }
  return indexChannelOrder.filter((x) => set.has(x));
});

const bundleIndexChannels = computed<IndexChannel[]>(() => {
  const bundle = SCENE_STRATEGY_CATALOG.bundles[bundleKey.value];
  const set = new Set<IndexChannel>();
  for (const k of bundle?.prerequisites ?? []) {
    if (!String(k).startsWith("index.")) continue;
    const ch = mapIndexPrereqToChannel(k);
    if (ch) set.add(ch);
  }
  return indexChannelOrder.filter((x) => set.has(x));
});

const extraIndexChannels = computed<IndexChannel[]>(() => {
  const base = new Set(sceneIndexChannels.value);
  return bundleIndexChannels.value.filter((x) => !base.has(x));
});

const bundleRuntimePrereqs = computed<string[]>(() => {
  const bundle = SCENE_STRATEGY_CATALOG.bundles[bundleKey.value];
  return (bundle?.prerequisites ?? []).filter((k) => String(k).startsWith("runtime."));
});

const runtimeLabel = (key: string) => {
  switch (key) {
    case "runtime.evidence_checker":
      return "证据校验器（Evidence Checker）";
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
  const bundle = (SCENE_STRATEGY_CATALOG.bundles as any)?.[key];
  if (bundle?.label) return `${bundle.label}（${key}）`;
  return key;
};

const goBack = async () => {
  if (process.client && window.history.length > 1) {
    router.back();
    return;
  }
  await navigateTo("/knowledge-spaces");
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

watch([sceneKey, bundleKey], async () => {
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
            <div class="grid gap-4 md:grid-cols-2">
              <UFormField :label="t('knowledgeSpaces.strategy.scene', '业务场景（L1）')" required>
                <USelectMenu
                  v-model="sceneKey"
                  :items="sceneItems"
                  value-key="value"
                  label-key="label"
                  class="w-full"
                />
                <template #help>
                  <div class="text-[var(--text-secondary)]">{{ selectedScene?.description }}</div>
                </template>
              </UFormField>
              <UFormField :label="t('knowledgeSpaces.strategy.bundle', '策略包（L2）')" required>
                <USelectMenu
                  v-model="bundleKey"
                  :items="bundleItems"
                  value-key="value"
                  label-key="label"
                  class="w-full"
                />
                <template #help>
                  <div class="text-[var(--text-secondary)]">{{ selectedBundle?.description }}</div>
                </template>
              </UFormField>
            </div>

            <div class="mt-3 pt-3 border-t border-[var(--border-color)] text-sm text-[var(--text-secondary)] space-y-1">
              <div class="font-medium text-[var(--text-primary)]">将写入（点击“保存到空间”后）</div>
              <div>业务场景（L1）：{{ selectedScene?.label }}（{{ sceneKey }}）</div>
              <div>策略包（L2）：{{ selectedBundle?.label }}（{{ bundleKey }}）</div>
              <div>三类 Profile：{{ selectedBundle?.label }}（{{ bundleKey }}）</div>
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
            <div class="text-sm text-[var(--text-secondary)]">用于提示该组合需要的索引通道/运行时能力，并影响下方校验结果。</div>
            </div>
            <div class="flex flex-wrap gap-2" />
        </div>
          <div class="mt-3 grid gap-3 md:grid-cols-3 text-sm">
            <div class="rounded-lg border border-[var(--border-color)] p-3">
              <div class="font-medium text-[var(--text-primary)]">L1 场景基线索引</div>
              <div class="mt-2 flex flex-wrap gap-2">
                <UBadge v-for="ch in sceneIndexChannels" :key="ch" color="primary" variant="soft">
                  {{ channelLabel(ch) }}
                </UBadge>
              </div>
            </div>
            <div class="rounded-lg border border-[var(--border-color)] p-3">
              <div class="font-medium text-[var(--text-primary)]">L2 额外索引</div>
              <div class="mt-2 flex flex-wrap gap-2">
                <UBadge v-for="ch in extraIndexChannels" :key="ch" color="primary" variant="soft">
                  {{ channelLabel(ch) }}
                </UBadge>
                <span v-if="!extraIndexChannels.length" class="text-[var(--text-secondary)]">无</span>
              </div>
            </div>
            <div class="rounded-lg border border-[var(--border-color)] p-3">
              <div class="font-medium text-[var(--text-primary)]">L2 运行时依赖</div>
              <div class="mt-2 flex flex-wrap gap-2">
                <UBadge v-for="k in bundleRuntimePrereqs" :key="k" color="neutral" variant="soft">
                  {{ runtimeLabel(k) }}
                </UBadge>
                <span v-if="!bundleRuntimePrereqs.length" class="text-[var(--text-secondary)]">无</span>
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
