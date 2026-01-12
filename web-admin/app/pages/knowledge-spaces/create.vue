<script setup lang="ts">
import { useKnowledgeSpaceStore } from "~/stores/knowledgeSpaces";
import { useUserStore } from "~/stores/user";
import { useDepartmentService, type Department } from "~/composables/api/services/departmentService";
import { useKnowledgeSpaces, type StrategyValidationResult } from "~/composables/useKnowledgeSpaces";
import { SCENE_STRATEGY_CATALOG, type SceneKey, type StrategyBundleKey } from "~/constants/sceneStrategyCatalog";

const store = useKnowledgeSpaceStore();
const userStore = useUserStore();
const toast = useToast();

const api = useKnowledgeSpaces();
const strategyValidation = ref<StrategyValidationResult | null>(null);
const strategyValidationLoading = ref(false);
const strategyValidationError = ref<string | null>(null);


const recommendationApplying = ref(false);
const recommendationApplyError = ref<string | null>(null);
const lastApplied = ref<{ sceneKey: SceneKey; bundleKey: StrategyBundleKey } | null>(null);

const corpusRecommendations = computed(() => {
  const list = (store.lastCorpusCheckJob as any)?.recommendations;
  return Array.isArray(list) ? list : [];
});

const corpusSceneBundleRec = computed(() =>
  corpusRecommendations.value.find((r: any) => r && r.type === "scene_bundle" && r.sceneKey && r.bundleKey),
);

const applyCorpusRecommendation = async (rec: any) => {
  if (!rec?.sceneKey || !rec?.bundleKey) return;
  if (!store.lastSpace?.spaceId) return;
  recommendationApplying.value = true;
  recommendationApplyError.value = null;
  lastApplied.value = { sceneKey: store.sceneKey, bundleKey: store.bundleKey };
  try {
    store.setSceneAndBundle(rec.sceneKey as SceneKey, rec.bundleKey as StrategyBundleKey);
    const updated = await api.updateSpace(store.lastSpace.spaceId, {
      ingestionProfileKey: store.form.ingestionProfileKey,
      indexProfileKey: store.form.indexProfileKey,
      ragProfileKey: store.form.ragProfileKey,
      featureFlags: store.form.featureFlags,
      updatedBy: store.iamEmail || userStore.user?.email || "ops@powerx.local",
    });
    store.lastSpace = updated as any;
    toast.add({
      color: "success",
      title: "已应用推荐策略",
      description: "已将推荐的场景/策略包写入该空间配置。",
    });
  } catch (e: any) {
    recommendationApplyError.value = e?.message || "应用失败";
    toast.add({ color: "error", title: "应用推荐失败", description: recommendationApplyError.value });
  } finally {
    recommendationApplying.value = false;
  }
};

const rollbackCorpusRecommendation = async () => {
  if (!lastApplied.value) return;
  if (!store.lastSpace?.spaceId) return;
  recommendationApplying.value = true;
  recommendationApplyError.value = null;
  try {
    store.setSceneAndBundle(lastApplied.value.sceneKey, lastApplied.value.bundleKey);
    const updated = await api.updateSpace(store.lastSpace.spaceId, {
      ingestionProfileKey: store.form.ingestionProfileKey,
      indexProfileKey: store.form.indexProfileKey,
      ragProfileKey: store.form.ragProfileKey,
      featureFlags: store.form.featureFlags,
      updatedBy: store.iamEmail || userStore.user?.email || "ops@powerx.local",
    });
    store.lastSpace = updated as any;
    toast.add({ color: "success", title: "已回滚", description: "已回滚到应用推荐前的选择。" });
    lastApplied.value = null;
  } catch (e: any) {
    recommendationApplyError.value = e?.message || "回滚失败";
    toast.add({ color: "error", title: "回滚失败", description: recommendationApplyError.value });
  } finally {
    recommendationApplying.value = false;
  }
};

const refreshStrategyValidation = async () => {
  strategyValidationLoading.value = true;
  strategyValidationError.value = null;
  try {
    strategyValidation.value = await api.validateStrategy({
      sceneKey: store.sceneKey,
      bundleKey: store.bundleKey,
    });
  } catch (e: any) {
    strategyValidationError.value = e?.message || "策略依赖校验失败";
    strategyValidation.value = null;
  } finally {
    strategyValidationLoading.value = false;
  }
};

type MyDepartment = { id: number; name: string; code: string; parent_id?: number };
const departments = ref<MyDepartment[]>([]);
const departmentsLoading = ref(false);
const departmentsError = ref<string | null>(null);

const departmentItems = computed(() => {
  const byParent = new Map<number | null, MyDepartment[]>();
  for (const dept of departments.value) {
    const parent = typeof dept.parent_id === "number" ? dept.parent_id : null;
    const list = byParent.get(parent) ?? [];
    list.push(dept);
    byParent.set(parent, list);
  }
  const sortByName = (a: MyDepartment, b: MyDepartment) => a.name.localeCompare(b.name);
  for (const [k, list] of byParent) {
    list.sort(sortByName);
    byParent.set(k, list);
  }

  const out: Array<{ label: string; value: string }> = [];
  const walk = (parent: number | null, depth: number) => {
    const children = byParent.get(parent) ?? [];
    for (const child of children) {
      const prefix = depth > 0 ? `${"—".repeat(Math.min(depth, 4))} ` : "";
      out.push({ label: `${prefix}${child.name}`, value: child.code });
      walk(child.id, depth + 1);
    }
  };
  walk(null, 0);
  return out;
});

const canSubmit = computed(() => store.isBasicInfoValid && !store.loading);

const sceneItems = computed(() =>
  (Object.entries(SCENE_STRATEGY_CATALOG.scenes) as Array<[SceneKey, any]>).map(([key, scene]) => ({
    label: scene.label,
    value: key,
  })),
);

const bundleItems = computed(() => {
  const scene = SCENE_STRATEGY_CATALOG.scenes[store.sceneKey];
  const allowed = scene?.allowedBundles ?? [];
  return allowed.map((key: StrategyBundleKey) => ({
    label: SCENE_STRATEGY_CATALOG.bundles[key].label,
    value: key,
  }));
});

const selectedScene = computed(() => SCENE_STRATEGY_CATALOG.scenes[store.sceneKey]);
const selectedBundle = computed(() => SCENE_STRATEGY_CATALOG.bundles[store.bundleKey]);

const enabledIndexChannels = computed(() => store.computeEnabledIndexChannels());

watch([() => store.sceneKey, () => store.bundleKey], async () => {
  await refreshStrategyValidation();
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

const loadDepartments = async () => {
  departmentsLoading.value = true;
  departmentsError.value = null;
  try {
    const svc = useDepartmentService();
    const tree = await svc.getDepartmentTree();
    const flat: MyDepartment[] = [];
    const walk = (nodes: Department[], parentId?: number) => {
      for (const node of nodes) {
        flat.push({
          id: Number(node.id),
          name: String(node.name || ""),
          code: String(node.key || node.id),
          parent_id: typeof parentId === "number" ? parentId : node.parent_id ?? undefined,
        });
        if (node.children?.length) {
          walk(node.children, Number(node.id));
        }
      }
    };
    walk(tree);
    departments.value = flat;
    if (!store.form.departmentCode && departmentItems.value.length) {
      store.form.departmentCode = departmentItems.value[0].value;
    }
  } catch (e: any) {
    departmentsError.value = e?.message || "加载部门失败";
    departments.value = [];
  } finally {
    departmentsLoading.value = false;
  }
};

onMounted(async () => {
  try {
    await userStore.fetchUserContext();
    if (!store.iamEmail && userStore.user?.email) {
      store.iamEmail = userStore.user.email;
    }
  } catch {
    // ignore
  }
  store.setSceneAndBundle(store.sceneKey, store.bundleKey);
  await refreshStrategyValidation();
  await loadDepartments();
});

const submit = async () => {
  try {
    await store.submit();
    if (process.client && store.lastSpace?.spaceId) {
      localStorage.setItem("px_last_space_id", store.lastSpace.spaceId);
    }
    toast.add({
      color: "success",
      title: "空间创建成功",
      description: "下一步：回到总览页打开入库，导入文档/URL。",
    });
  } catch {
    // store.error already set
  }
};

const goIngestion = async (opts?: { ocr?: boolean }) => {
  // 通过 query 触发总览页自动打开入库 modal
  await navigateTo({
    path: "/knowledge-spaces",
    query: {
      openIngestion: "1",
      spaceId: store.lastSpace?.spaceId || "",
      ocr: opts?.ocr ? "1" : undefined,
    },
  });
};

const openPluginMarket = (pluginId: string) => {
  navigateTo(`/plugins/market?pluginId=${encodeURIComponent(pluginId)}`);
};
</script>

<template>
  <section class="mx-auto max-w-5xl space-y-6 px-6 py-8">
    <header class="rounded-2xl border border-[var(--border-color)] bg-[var(--card-bg)] p-6 shadow-sm">
      <p class="text-sm text-[var(--text-secondary)]">Knowledge Space</p>
      <h1 class="mt-1 text-2xl font-semibold text-[var(--text-primary)]">创建知识空间</h1>
      <p class="mt-2 text-sm text-[var(--text-secondary)]">
        空间用于承载你的知识内容（文档/URL/API）。创建空间后，再回到“知识空间总览”进行入库与检索验证。
      </p>
    </header>

    <UCard :ui="{ body: { padding: 'p-6' } }">
      <template #header>
        <div class="flex items-center justify-between">
          <div>
            <h2 class="text-lg font-semibold text-[var(--text-primary)]">基本信息</h2>
            <p class="text-sm text-[var(--text-secondary)]">无需填写租户 UUID，系统会使用当前登录租户上下文。</p>
          </div>
          <UButton color="neutral" variant="subtle" to="/knowledge-spaces">
            返回列表
          </UButton>
        </div>
      </template>

      <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
        <UFormField label="空间名称" required>
          <UInput
            v-model="store.form.spaceName"
            class="w-full"
            placeholder="例如：运营手册 / 产品知识库"
            icon="i-heroicons-rectangle-stack"
          />
        </UFormField>

        <UFormField label="所属部门" required>
          <USelectMenu
            v-model="store.form.departmentCode"
            :items="departmentItems"
            value-key="value"
            label-key="label"
            class="w-full"
            :loading="departmentsLoading"
            placeholder="请选择部门"
          />
          <template #help>
            <span v-if="departmentsError" class="text-red-500">{{ departmentsError }}</span>
            <span v-else-if="!departmentsLoading && departmentItems.length === 0">当前租户暂无部门，请先在“组织架构”中配置。</span>
          </template>
        </UFormField>
      </div>

      <div class="mt-6 flex items-center justify-end gap-2">
        <span v-if="store.error" class="mr-auto text-sm text-red-500">{{ store.error }}</span>
        <UButton color="neutral" variant="subtle" type="button" @click="store.reset()" :disabled="store.loading">
          重置
        </UButton>
        <UButton color="primary" :loading="store.loading" :disabled="!canSubmit" @click="submit">
          创建空间
        </UButton>
      </div>
    </UCard>

    <UCard :ui="{ body: { padding: 'p-6' } }">
      <template #header>
        <div>
          <h2 class="text-lg font-semibold text-[var(--text-primary)]">场景与策略包</h2>
          <p class="text-sm text-[var(--text-secondary)]">
            先选择你的业务场景（L1），再选择该场景允许的策略包（L2）。系统会自动绑定空间的 Ingestion/Index/RAG Profile。
          </p>
        </div>
      </template>

      <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
        <UFormField label="场景（L1）" required>
          <USelect
            :model-value="store.sceneKey"
            :items="sceneItems"
            class="w-full"
            @update:model-value="(v) => store.setSceneAndBundle(v as SceneKey)"
          />
          <template #help>
            <span class="text-xs text-[var(--text-secondary)]">{{ selectedScene?.description }}</span>
          </template>
        </UFormField>

        <UFormField label="策略包（L2）" required>
          <USelect
            :model-value="store.bundleKey"
            :items="bundleItems"
            class="w-full"
            @update:model-value="(v) => store.setSceneAndBundle(store.sceneKey, v as StrategyBundleKey)"
          />
          <template #help>
            <span class="text-xs text-[var(--text-secondary)]">{{ selectedBundle?.description }}</span>
          </template>
        </UFormField>
      </div>

      <div class="mt-4 rounded-xl border border-[var(--border-color)] bg-[var(--card-bg)] p-4">
        <div class="mb-3">
          <UAlert
            v-if="strategyValidationError"
            color="error"
            variant="soft"
            title="策略依赖校验失败"
            :description="strategyValidationError"
          />
          <UAlert
            v-else-if="strategyValidation && !strategyValidation.ok"
            color="warning"
            variant="soft"
            title="当前选择的策略依赖未满足"
            description="请按下方提示补齐能力，或切换到不依赖该能力的策略包。"
          >
            <template #description>
              <div class="space-y-2">
                <div
                  v-for="item in strategyValidation.missing"
                  :key="item.code + item.key"
                  class="rounded-lg border border-[var(--border-color)] bg-[var(--card-bg)] p-3"
                >
                  <div class="font-medium text-[var(--text-primary)]">{{ item.message }}</div>
                  <div class="mt-2 text-sm text-[var(--text-secondary)]">
                    <ul class="list-disc pl-5">
                      <li v-for="(r, i) in item.remediation" :key="i">{{ r }}</li>
                    </ul>
                  </div>
                </div>
                <div class="flex flex-wrap gap-2">
                  <UButton
                    v-if="strategyValidation.missing.some((m) => m.code === 'evidence_checker_required')"
                    color="primary"
                    size="sm"
                    to="/settings/ai"
                    icon="i-heroicons-cog-6-tooth"
                  >
                    去配置 AI Provider
                  </UButton>
                </div>
              </div>
            </template>
          </UAlert>
          <UAlert
            v-else-if="strategyValidation && strategyValidation.ok"
            color="success"
            variant="soft"
            title="策略依赖校验通过"
          />
          <UAlert
            v-else
            color="neutral"
            variant="soft"
            title="正在校验策略依赖"
            :description="strategyValidationLoading ? '加载中…' : ''"
          />
        </div>
        <div class="flex flex-wrap items-center gap-2">
          <span class="text-sm font-medium text-[var(--text-primary)]">将启用的索引通道：</span>
          <UBadge
            v-for="ch in enabledIndexChannels"
            :key="ch"
            color="neutral"
            variant="soft"
          >
            {{ channelLabel(ch) }}
          </UBadge>
        </div>

        <div class="mt-3 grid grid-cols-1 gap-2 text-sm md:grid-cols-3">
          <div class="text-[var(--text-secondary)]">
            IngestionProfileKey：<span class="text-[var(--text-primary)]">{{ store.form.ingestionProfileKey }}</span>
          </div>
          <div class="text-[var(--text-secondary)]">
            IndexProfileKey：<span class="text-[var(--text-primary)]">{{ store.form.indexProfileKey }}</span>
          </div>
          <div class="text-[var(--text-secondary)]">
            RAGProfileKey：<span class="text-[var(--text-primary)]">{{ store.form.ragProfileKey }}</span>
          </div>
        </div>
      </div>
    </UCard>

    <div v-if="store.wizardCompleted" class="space-y-4">
      <UAlert
        color="primary"
        variant="soft"
        icon="i-heroicons-check-circle"
        title="空间创建成功"
        description="下一步：点击“立即入库”，导入文档/URL/API。"
      />
      <div class="flex flex-wrap gap-2">
        <UButton color="primary" icon="i-heroicons-arrow-up-tray" @click="goIngestion()">立即入库</UButton>
        <UButton color="neutral" variant="subtle" to="/knowledge-spaces" icon="i-heroicons-arrow-left">
          稍后再说
        </UButton>
      </div>
    
      <UCard v-if="store.lastCorpusCheckJob" :ui="{ body: { padding: 'p-6' } }">
        <template #header>
          <div class="flex items-start justify-between gap-4">
            <div>
              <h2 class="text-lg font-semibold text-[var(--text-primary)]">Corpus Check 推荐</h2>
              <p class="text-sm text-[var(--text-secondary)]">基于最近入库样本的分布，给出场景/策略包与成本/风险提示。</p>
            </div>
            <UBadge color="neutral" variant="soft">{{ (store.lastCorpusCheckJob as any)?.status || 'unknown' }}</UBadge>
          </div>
        </template>

        <UAlert
          v-if="recommendationApplyError"
          color="error"
          variant="soft"
          title="操作失败"
          :description="recommendationApplyError"
          class="mb-4"
        />

        <div class="space-y-3">
          <div
            v-for="rec in corpusRecommendations"
            :key="rec.key || rec.title"
            class="rounded-xl border border-[var(--border-color)] bg-[var(--card-bg)] p-4"
          >
            <div class="flex items-start justify-between gap-3">
              <div>
                <div class="font-medium text-[var(--text-primary)]">{{ rec.title }}</div>
                <div v-if="rec.risk" class="mt-1 text-sm text-[var(--text-secondary)]">风险：{{ rec.risk }}</div>
                <div v-if="rec.cost" class="mt-1 text-sm text-[var(--text-secondary)]">成本：{{ rec.cost }}</div>
              </div>
              <div class="flex flex-wrap gap-2">
                <UButton
                  v-if="rec.type === 'scene_bundle' && rec.sceneKey && rec.bundleKey"
                  color="primary"
                  size="sm"
                  :loading="recommendationApplying"
                  :disabled="recommendationApplying"
                  @click="applyCorpusRecommendation(rec)"
                >
                  应用推荐
                </UButton>
                <UButton
                  v-if="rec.key === 'enable_ocr' && rec.plugin"
                  color="primary"
                  size="sm"
                  variant="soft"
                  icon="i-heroicons-shopping-bag"
                  @click="openPluginMarket(rec.plugin)"
                >
                  安装 OCR 插件
                </UButton>
                <UButton
                  v-if="rec.key === 'enable_ocr'"
                  color="neutral"
                  variant="soft"
                  size="sm"
                  @click="goIngestion({ ocr: true })"
                >
                  打开入库并启用 OCR
                </UButton>
                <UButton
                  v-if="rec.type === 'scene_bundle' && lastApplied"
                  color="neutral"
                  variant="soft"
                  size="sm"
                  :loading="recommendationApplying"
                  :disabled="recommendationApplying"
                  @click="rollbackCorpusRecommendation"
                >
                  回滚
                </UButton>
              </div>
            </div>
            <div v-if="rec.type === 'scene_bundle'" class="mt-3 flex flex-wrap items-center gap-2 text-sm">
              <UBadge color="primary" variant="soft">场景：{{ rec.sceneLabel || rec.sceneKey }}</UBadge>
              <UBadge color="primary" variant="soft">策略包：{{ rec.bundleLabel || rec.bundleKey }}</UBadge>
            </div>
          </div>
        </div>
      </UCard>
</div>
  </section>
</template>
