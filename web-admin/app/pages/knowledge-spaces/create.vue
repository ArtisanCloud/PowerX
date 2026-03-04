<script setup lang="ts">
import { useKnowledgeSpaceStore } from "~/stores/knowledgeSpaces";
import { useEmbeddingGuard } from "~/composables/useEmbeddingGuard";
import { useUserStore } from "~/stores/user";
import { useDepartmentService, type Department } from "~/composables/api/services/departmentService";
import { useKnowledgeSpaces, type StrategyValidationResult } from "~/composables/useKnowledgeSpaces";
import type { EmbeddingGuardResult } from "~/composables/useEmbeddingGuard";
import {
  SCENE_CATALOG,
  STRATEGY_PACKAGE_CATALOG,
  STRATEGY_PACKAGE_ORDER,
  type SceneKey,
  type StrategyPackageKey,
} from "~/constants/strategyPackageCatalog";

const store = useKnowledgeSpaceStore();
const { ensureEmbeddingReady } = useEmbeddingGuard();
const userStore = useUserStore();
const toast = useToast();

const api = useKnowledgeSpaces();
const strategyValidation = ref<StrategyValidationResult | null>(null);
const strategyValidationLoading = ref(false);
const strategyValidationError = ref<string | null>(null);

const refreshStrategyValidation = async () => {
  strategyValidationLoading.value = true;
  strategyValidationError.value = null;
  try {
    const pkg = STRATEGY_PACKAGE_CATALOG[store.strategyPackageKey];
    const sceneKey: SceneKey = pkg?.recommendedScenes?.[0] ?? "custom_expert";
    const bundleKey = pkg?.recommendedProfileKey ?? "p1_general";
    strategyValidation.value = await api.validateStrategy({
      sceneKey,
      bundleKey,
    });
  } catch (e: any) {
    strategyValidationError.value = e?.message || "策略依赖校验失败";
    strategyValidation.value = null;
  } finally {
    strategyValidationLoading.value = false;
  }
};

const autoActivateVectorIndex = async (spaceId: string, guard: EmbeddingGuardResult) => {
  const embeddingProfileKey = guard.embeddingProfileKey;
  if (!embeddingProfileKey) return;
  await api.activateVectorIndex(spaceId, {
    embeddingProfileKey,
    requestedBy: userStore.user?.email || store.iamEmail || "ops@powerx.local",
  });
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

const packageItems = computed(() =>
  STRATEGY_PACKAGE_ORDER.map((key) => ({
    label: STRATEGY_PACKAGE_CATALOG[key]?.label || key,
    value: key,
  })),
);

const selectedPackage = computed(() => STRATEGY_PACKAGE_CATALOG[store.strategyPackageKey]);
const defaultSceneKey = computed<SceneKey>(() => selectedPackage.value?.recommendedScenes?.[0] ?? "custom_expert");
const sceneLabel = computed(() => SCENE_CATALOG[defaultSceneKey.value]?.label || defaultSceneKey.value);

const profileLabel = (key: string | null | undefined) => {
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

const enabledIndexChannels = computed(() => store.computeEnabledIndexChannels());

watch(() => store.strategyPackageKey, async () => {
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
  store.setStrategyPackage(store.strategyPackageKey);
  await refreshStrategyValidation();
  await loadDepartments();
});

const submit = async () => {
  try {
    const guard = await ensureEmbeddingReady();
    if (!guard) return;
    await store.submit();
    if (process.client && store.lastSpace?.spaceId) {
      localStorage.setItem("px_last_space_id", store.lastSpace.spaceId);
    }
    if (store.lastSpace?.spaceId) {
      try {
        await autoActivateVectorIndex(store.lastSpace.spaceId, guard);
        toast.add({
          color: "success",
          title: "向量索引已激活",
          description: "已自动绑定 embedding profile 并创建向量表。",
        });
      } catch (e: any) {
        toast.add({
          color: "warning",
          title: "向量索引未自动激活",
          description: e?.message || "请前往“策略配置”手动激活向量索引。",
        });
      }
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
          <h2 class="text-lg font-semibold text-[var(--text-primary)]">策略包（A0–O）</h2>
          <p class="text-sm text-[var(--text-secondary)]">
            先选策略包，再由系统映射适用场景与 Profile 预设。场景只是说明，不再作为必选项。
          </p>
        </div>
      </template>

      <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
        <UFormField label="策略包（A0–O）" required>
          <USelect
            :model-value="store.strategyPackageKey"
            :items="packageItems"
            class="w-full"
            @update:model-value="(v) => store.setStrategyPackage(v as StrategyPackageKey)"
          />
          <template #help>
            <span class="text-xs text-[var(--text-secondary)]">{{ selectedPackage?.summary }}</span>
          </template>
        </UFormField>

        <div class="rounded-lg border border-[var(--border-color)] p-4 text-sm text-[var(--text-secondary)]">
          <div class="font-medium text-[var(--text-primary)]">自动映射</div>
          <div class="mt-2">推荐 Profile：{{ profileLabel(selectedPackage?.recommendedProfileKey) }}</div>
          <div class="mt-1">适用场景：{{ sceneLabel }}</div>
        </div>
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
    
	      <UAlert
	        color="neutral"
	        variant="soft"
	        icon="i-heroicons-information-circle"
	        title="提示"
	        description="Corpus Check 推荐依赖入库样本文档；空间创建本身不会触发。首次入库后将自动生成（可在“策略配置”查看）。"
	      />
	</div>
  </section>
</template>
