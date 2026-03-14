<template>
  <div class="space-y-6 p-4">
    <div class="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
      <div>
        <h1 class="text-lg font-semibold text-[var(--text-primary)]">Skills</h1>
        <p class="text-sm text-[var(--text-secondary)]">
          管理 Skills 导入、发布与回滚；区分系统固有技能目录与已导入 Registry。
        </p>
      </div>
      <div class="flex flex-wrap gap-2">
        <UButton v-if="allowAccess && activeTab === 'catalog'" color="neutral" variant="soft" icon="i-heroicons-plus" @click="openCatalogEditor()">
          新增目录项
        </UButton>
        <UButton icon="i-heroicons-arrow-up-tray" @click="importModalOpen = true">导入 Skill</UButton>
      </div>
    </div>

    <UAlert
      v-if="!allowAccess"
      color="warning"
      variant="soft"
      icon="i-heroicons-lock-closed"
      title="无权限访问"
      description="当前页面仅 admin root 可访问。"
    />

    <template v-else>
      <SettingsAiSkillsImportForm v-model:open="importModalOpen" @imported="onImported" />
      <SettingsAiSkillsAuditDrawer v-model="auditDrawerOpen" :skill-id="selectedSkillId" />
      <UModal
        v-model:open="catalogEditorOpen"
        :title="catalogEditorMode === 'create' ? '新增官方目录项' : '编辑官方目录项'"
        :ui="{ content: 'max-w-3xl w-full' }"
      >
        <template #body>
          <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
            <UInput v-model="catalogForm.catalog_skill_id" label="Catalog Skill ID" placeholder="catalog.github" :disabled="catalogEditorMode === 'edit'" />
            <UInput v-model="catalogForm.skill_id" label="Skill ID" placeholder="skill.builtin.github" />
            <UInput v-model="catalogForm.recommended_version" label="推荐版本" placeholder="1.0.0" />
            <USelect v-model="catalogForm.risk_level" :items="riskLevelOptions" label="风险等级" />
            <USelect v-model="catalogForm.category" :items="categoryOptions" label="分类" />
            <UInput v-model="catalogForm.maintainer" label="维护者（可选）" placeholder="powerx-core" />
            <UInput v-model="catalogForm.bundle_uri" label="Bundle URI（可选）" placeholder="builtin://skills/github/1.0.0" />
            <UInput v-model="catalogForm.checksum" label="Checksum（可选）" placeholder="sha256:..." />
            <UTextarea v-model="catalogForm.summary" class="md:col-span-2" label="说明（可选）" />
            <UTextarea v-model="catalogForm.official_release_note" class="md:col-span-2" label="发布说明（可选）" />
          </div>
        </template>
        <template #footer>
          <div class="flex items-center justify-end gap-2">
            <UButton color="neutral" variant="soft" :disabled="catalogActionLoading" @click="catalogEditorOpen = false">取消</UButton>
            <UButton :loading="catalogActionLoading" @click="submitCatalogEditor">保存</UButton>
          </div>
        </template>
      </UModal>

      <UCard>
        <template #header>
          <div class="flex items-center justify-between">
            <UTabs v-model="activeTab" :items="tabItems" />
            <span class="text-xs text-[var(--text-secondary)]">
              <template v-if="activeTab === 'registry'">共 {{ registryTotal }} 条</template>
              <template v-else>共 {{ catalogFilteredTotal }} 条</template>
            </span>
          </div>
        </template>
        <p class="mb-4 text-xs text-[var(--text-secondary)]">
          系统固有技能目录用于推荐与基线能力展示；Registry 列表用于管理已导入技能版本（含 third_party/plugin）及其发布状态。
        </p>

        <template v-if="activeTab === 'registry'">
          <div class="mb-4 grid grid-cols-1 gap-3 md:grid-cols-4">
            <UInput v-model="filters.skillId" placeholder="按 Skill ID 过滤" />
            <USelect v-model="filters.status" :items="statusOptions" placeholder="状态" />
            <USelect v-model="filters.source" :items="sourceOptions" placeholder="来源" />
            <UButton :loading="loadingRegistry" @click="fetchRegistry">查询</UButton>
          </div>

          <div v-if="loadingRegistry" class="text-sm text-[var(--text-secondary)]">加载中...</div>
          <div v-else-if="registryItems.length === 0" class="text-sm text-[var(--text-secondary)]">暂无数据</div>
          <UTable
            v-else
            :columns="registryColumns"
            :data="registryRows"
            row-key="row_key"
            :ui="{ divide: 'divide-y divide-[var(--border-color)]' }"
          />
        </template>

        <template v-else>
          <div class="mb-4 grid grid-cols-1 gap-3 md:grid-cols-5">
            <UInput v-model="catalogFilters.keyword" placeholder="按 Catalog ID/Skill ID/说明 关键词搜索" />
            <USelect v-model="catalogFilters.riskLevel" :items="catalogRiskFilterOptions" placeholder="风险等级" />
            <USelect v-model="catalogFilters.category" :items="catalogCategoryFilterOptions" placeholder="分类" />
            <USelect v-model="catalogFilters.active" :items="catalogActiveFilterOptions" placeholder="状态" />
            <USelect v-model="catalogPagination.pageSize" :items="catalogPageSizeOptions" placeholder="每页数量" />
          </div>
          <div v-if="loadingCatalog" class="text-sm text-[var(--text-secondary)]">加载中...</div>
          <div v-else-if="catalogFilteredTotal === 0" class="text-sm text-[var(--text-secondary)]">暂无数据</div>
          <UTable
            v-else
            :columns="catalogColumns"
            :data="catalogPagedItems"
            row-key="catalog_skill_id"
            :ui="{ divide: 'divide-y divide-[var(--border-color)]' }"
          />
          <div v-if="!loadingCatalog && catalogFilteredTotal > 0" class="mt-4 flex items-center justify-between">
            <span class="text-xs text-[var(--text-secondary)]">
              显示 {{ catalogPageStart }}-{{ catalogPageEnd }} / 共 {{ catalogFilteredTotal }} 条
            </span>
            <UPagination
              v-model:page="catalogPagination.page"
              :total="catalogFilteredTotal"
              :items-per-page="catalogPagination.pageSize"
            />
          </div>
        </template>
      </UCard>
    </template>
  </div>
</template>

<script setup lang="ts">
import { h, resolveComponent } from "vue";
import { storeToRefs } from "pinia";
import { useSkillsService, type SkillCatalogItem, type SkillRecord } from "~/composables/api/services";
import { useUserStore } from "~/stores/user";

const toast = useToast();
const skillsService = useSkillsService();
const userStore = useUserStore();
const { isRoot } = storeToRefs(userStore);

definePageMeta({
  title: "Skills",
  layout: "default",
});

type CatalogItem = SkillCatalogItem;

const allowAccess = computed(() => isRoot.value);
const loadingCatalog = ref(false);
const loadingRegistry = ref(false);
const actionLoading = ref(false);
const catalogItems = ref<CatalogItem[]>([]);
const registryItems = ref<SkillRecord[]>([]);
const registryRows = computed(() => registryItems.value.map((item) => ({ ...item, row_key: rowKey(item) })));
const registryTotal = ref(0);
const publishDraft = reactive<Record<string, string>>({});
const rollbackDraft = reactive<Record<string, string>>({});
const auditDrawerOpen = ref(false);
const importModalOpen = ref(false);
const selectedSkillId = ref("");
const catalogEditorOpen = ref(false);
const catalogEditorMode = ref<"create" | "edit">("create");
const catalogActionLoading = ref(false);
const catalogForm = reactive({
  catalog_skill_id: "",
  skill_id: "",
  recommended_version: "",
  risk_level: "L2",
  category: "",
  summary: "",
  maintainer: "",
  official_release_note: "",
  bundle_uri: "",
  checksum: "",
});
const FILTER_ALL = "__all__";
const activeTab = ref<"registry" | "catalog">("registry");
const tabItems = [
  { label: "已导入技能（Registry）", value: "registry", icon: "i-heroicons-rectangle-stack" },
  { label: "系统固有技能目录", value: "catalog", icon: "i-heroicons-book-open" },
];

const filters = reactive({
  skillId: "",
  status: FILTER_ALL,
  source: FILTER_ALL,
});

const statusOptions = [
  { label: "全部状态", value: FILTER_ALL },
  { label: "draft", value: "draft" },
  { label: "published", value: "published" },
  { label: "deprecated", value: "deprecated" },
  { label: "disabled", value: "disabled" },
];

const sourceOptions = [
  { label: "全部来源", value: FILTER_ALL },
  { label: "builtin", value: "builtin" },
  { label: "plugin", value: "plugin" },
  { label: "third_party", value: "third_party" },
];

const riskLevelOptions = [
  { label: "L1", value: "L1" },
  { label: "L2", value: "L2" },
  { label: "L3", value: "L3" },
];
const categoryOptions = [
  { label: "platform", value: "platform" },
  { label: "dev", value: "dev" },
  { label: "doc", value: "doc" },
  { label: "knowledge", value: "knowledge" },
  { label: "comm", value: "comm" },
  { label: "pm", value: "pm" },
  { label: "media", value: "media" },
  { label: "device", value: "device" },
  { label: "channel", value: "channel" },
  { label: "security", value: "security" },
];
const FILTER_ALL_CATALOG = "__all_catalog__";
const catalogFilters = reactive({
  keyword: "",
  riskLevel: FILTER_ALL_CATALOG,
  category: FILTER_ALL_CATALOG,
  active: FILTER_ALL_CATALOG,
});
const catalogRiskFilterOptions = computed(() => [
  { label: "全部风险等级", value: FILTER_ALL_CATALOG },
  ...riskLevelOptions,
]);
const catalogCategoryFilterOptions = computed(() => [
  { label: "全部分类", value: FILTER_ALL_CATALOG },
  ...categoryOptions,
]);
const catalogActiveFilterOptions = [
  { label: "全部状态", value: FILTER_ALL_CATALOG },
  { label: "active", value: "active" },
  { label: "disabled", value: "disabled" },
];
const catalogPageSizeOptions = [
  { label: "10 / 页", value: 10 },
  { label: "20 / 页", value: 20 },
  { label: "50 / 页", value: 50 },
];
const catalogPagination = reactive({
  page: 1,
  pageSize: 10,
});
const catalogFilteredItems = computed(() => {
  const keyword = catalogFilters.keyword.trim().toLowerCase();
  return catalogItems.value.filter((item) => {
    if (catalogFilters.riskLevel !== FILTER_ALL_CATALOG && item.risk_level !== catalogFilters.riskLevel) {
      return false;
    }
    if (catalogFilters.category !== FILTER_ALL_CATALOG && item.category !== catalogFilters.category) {
      return false;
    }
    if (catalogFilters.active !== FILTER_ALL_CATALOG) {
      const expected = catalogFilters.active === "active";
      if (Boolean(item.active) !== expected) return false;
    }
    if (!keyword) return true;
    const haystack = [item.catalog_skill_id, item.skill_id, item.summary, item.category].map((v) => String(v || "").toLowerCase()).join(" ");
    return haystack.includes(keyword);
  });
});
const catalogFilteredTotal = computed(() => catalogFilteredItems.value.length);
const catalogTotalPages = computed(() => Math.max(1, Math.ceil(catalogFilteredTotal.value / catalogPagination.pageSize)));
const catalogPagedItems = computed(() => {
  const start = (catalogPagination.page - 1) * catalogPagination.pageSize;
  return catalogFilteredItems.value.slice(start, start + catalogPagination.pageSize);
});
const catalogPageStart = computed(() => {
  if (catalogFilteredTotal.value === 0) return 0;
  return (catalogPagination.page - 1) * catalogPagination.pageSize + 1;
});
const catalogPageEnd = computed(() => Math.min(catalogPagination.page * catalogPagination.pageSize, catalogFilteredTotal.value));

watch(
  () => [catalogFilters.keyword, catalogFilters.riskLevel, catalogFilters.category, catalogFilters.active, catalogPagination.pageSize],
  () => {
    catalogPagination.page = 1;
  },
);

watch(
  () => [catalogFilteredTotal.value, catalogTotalPages.value],
  () => {
    if (catalogPagination.page > catalogTotalPages.value) {
      catalogPagination.page = catalogTotalPages.value;
    }
  },
);

const catalogColumns = computed(() => {
  const UButton = resolveComponent("UButton");
  return [
    { accessorKey: "catalog_skill_id", header: "Catalog ID" },
    { accessorKey: "skill_id", header: "Skill ID" },
    { accessorKey: "recommended_version", header: "推荐版本" },
    { accessorKey: "risk_level", header: "风险等级" },
    { accessorKey: "category", header: "分类" },
    {
      accessorKey: "active",
      header: "状态",
      cell: ({ row }: any) => (row?.original?.active ? "active" : "disabled"),
    },
    { accessorKey: "summary", header: "说明" },
    {
      id: "actions",
      header: "操作",
      cell: ({ row }: any) => {
        const item = row.original as CatalogItem;
        return h("div", { class: "flex items-center gap-2" }, [
          h(
            UButton as any,
            { size: "xs", variant: "soft", onClick: () => openCatalogEditor(item) },
            () => "编辑",
          ),
          h(
            UButton as any,
            {
              size: "xs",
              color: item.active ? "warning" : "primary",
              loading: catalogActionLoading.value,
              onClick: () => onToggleCatalogActive(item),
            },
            () => (item.active ? "停用" : "启用"),
          ),
        ]);
      },
    },
  ];
});

const registryColumns = computed(() => {
  const UInput = resolveComponent("UInput");
  const UButton = resolveComponent("UButton");
  return [
    { accessorKey: "skill_id", header: "Skill ID" },
    { accessorKey: "version", header: "版本" },
    { accessorKey: "source", header: "来源" },
    { accessorKey: "status", header: "状态" },
    {
      id: "publish",
      header: "发布",
      cell: ({ row }: any) => {
        const item = row.original as SkillRecord;
        const key = rowKey(item);
        if (!publishDraft[key]) publishDraft[key] = item.version;
        return h("div", { class: "flex items-center gap-2" }, [
          h(UInput as any, {
            modelValue: publishDraft[key],
            "onUpdate:modelValue": (v: string) => (publishDraft[key] = v),
            placeholder: "版本",
            class: "w-32",
            size: "xs",
          }),
          h(
            UButton as any,
            {
              size: "xs",
              color: "primary",
              loading: actionLoading.value,
              onClick: () => onPublish(item.skill_id, publishDraft[key]),
            },
            () => "发布",
          ),
        ]);
      },
    },
    {
      id: "rollback",
      header: "回滚",
      cell: ({ row }: any) => {
        const item = row.original as SkillRecord;
        const key = rowKey(item);
        if (!rollbackDraft[key]) rollbackDraft[key] = item.version;
        return h("div", { class: "flex items-center gap-2" }, [
          h(UInput as any, {
            modelValue: rollbackDraft[key],
            "onUpdate:modelValue": (v: string) => (rollbackDraft[key] = v),
            placeholder: "目标版本",
            class: "w-32",
            size: "xs",
          }),
          h(
            UButton as any,
            {
              size: "xs",
              color: "neutral",
              loading: actionLoading.value,
              onClick: () => onRollback(item.skill_id, rollbackDraft[key]),
            },
            () => "回滚",
          ),
        ]);
      },
    },
    {
      id: "audit",
      header: "审计",
      cell: ({ row }: any) => {
        const item = row.original as SkillRecord;
        return h(
          UButton as any,
          {
            size: "xs",
            variant: "ghost",
            onClick: () => openAuditDrawer(item.skill_id),
          },
          () => "查看",
        );
      },
    },
  ];
});

function rowKey(item: SkillRecord) {
  return `${item.skill_id}@${item.version}`;
}

function resetCatalogForm() {
  catalogForm.catalog_skill_id = "";
  catalogForm.skill_id = "";
  catalogForm.recommended_version = "";
  catalogForm.risk_level = "L2";
  catalogForm.category = "";
  catalogForm.summary = "";
  catalogForm.maintainer = "";
  catalogForm.official_release_note = "";
  catalogForm.bundle_uri = "";
  catalogForm.checksum = "";
}

function openCatalogEditor(item?: CatalogItem) {
  if (!item) {
    catalogEditorMode.value = "create";
    resetCatalogForm();
    catalogEditorOpen.value = true;
    return;
  }
  catalogEditorMode.value = "edit";
  catalogForm.catalog_skill_id = item.catalog_skill_id || "";
  catalogForm.skill_id = item.skill_id || "";
  catalogForm.recommended_version = item.recommended_version || "";
  catalogForm.risk_level = item.risk_level || "L2";
  catalogForm.category = item.category || "";
  catalogForm.summary = item.summary || "";
  catalogForm.maintainer = item.maintainer || "";
  catalogForm.official_release_note = item.official_release_note || "";
  catalogForm.bundle_uri = "";
  catalogForm.checksum = "";
  catalogEditorOpen.value = true;
}

async function submitCatalogEditor() {
  catalogActionLoading.value = true;
  try {
    await skillsService.upsertCatalog({
      catalog_skill_id: catalogForm.catalog_skill_id.trim(),
      skill_id: catalogForm.skill_id.trim(),
      recommended_version: catalogForm.recommended_version.trim(),
      risk_level: catalogForm.risk_level,
      category: catalogForm.category.trim(),
      summary: catalogForm.summary.trim() || undefined,
      maintainer: catalogForm.maintainer.trim() || undefined,
      official_release_note: catalogForm.official_release_note.trim() || undefined,
      bundle_uri: catalogForm.bundle_uri.trim() || undefined,
      checksum: catalogForm.checksum.trim() || undefined,
      active: true,
    });
    toast.add({ title: "目录项保存成功", color: "success" });
    catalogEditorOpen.value = false;
    await Promise.all([fetchCatalog(), fetchRegistry()]);
  } catch (error: any) {
    toast.add({ title: "目录项保存失败", description: error?.message || "请求失败", color: "error" });
  } finally {
    catalogActionLoading.value = false;
  }
}

async function onToggleCatalogActive(item: CatalogItem) {
  if (!item.catalog_skill_id) return;
  catalogActionLoading.value = true;
  try {
    await skillsService.setCatalogActive(item.catalog_skill_id, !item.active);
    toast.add({ title: !item.active ? "目录项已启用" : "目录项已停用", color: "success" });
    await fetchCatalog();
  } catch (error: any) {
    toast.add({ title: "更新状态失败", description: error?.message || "请求失败", color: "error" });
  } finally {
    catalogActionLoading.value = false;
  }
}

async function fetchCatalog() {
  if (!allowAccess.value) return;
  loadingCatalog.value = true;
  try {
    const resp = await skillsService.listCatalog();
    let items = (resp?.data?.items || []) as CatalogItem[];
    if (items.length === 0) {
      const builtinResp = await skillsService.list({
        source: "builtin",
        page: 1,
        page_size: 200,
      });
      const builtinItems = builtinResp?.data?.items || [];
      items = builtinItems.map((item) => ({
        catalog_skill_id: `auto.${item.skill_id}`,
        skill_id: item.skill_id,
        recommended_version: item.version,
        risk_level: "unknown",
        category: "native",
        summary: "来自本地 builtin registry",
        active: true,
      }));
    }
    catalogItems.value = items;
  } catch (error: any) {
    toast.add({ title: "加载目录失败", description: error?.message || "请求失败", color: "error" });
  } finally {
    loadingCatalog.value = false;
  }
}

async function fetchRegistry() {
  if (!allowAccess.value) return;
  loadingRegistry.value = true;
  try {
    const resp = await skillsService.list({
      skill_id: filters.skillId || undefined,
      status: filters.status === FILTER_ALL ? undefined : filters.status,
      source: filters.source === FILTER_ALL ? undefined : filters.source,
      page: 1,
      page_size: 50,
    });
    registryItems.value = resp?.data?.items || [];
    registryTotal.value = resp?.data?.total || 0;
  } catch (error: any) {
    toast.add({ title: "加载 registry 失败", description: error?.message || "请求失败", color: "error" });
  } finally {
    loadingRegistry.value = false;
  }
}

async function onPublish(skillId: string, version: string) {
  actionLoading.value = true;
  try {
    await skillsService.publish(skillId, version, "manual publish from admin ui")
    toast.add({ title: "发布成功", color: "success" });
    await fetchRegistry();
  } catch (error: any) {
    toast.add({ title: "发布失败", description: error?.message || "请求失败", color: "error" });
  } finally {
    actionLoading.value = false;
  }
}

async function onRollback(skillId: string, targetVersion: string) {
  actionLoading.value = true;
  try {
    await skillsService.rollback(skillId, targetVersion, "manual rollback from admin ui");
    toast.add({ title: "回滚成功", color: "success" });
    await fetchRegistry();
  } catch (error: any) {
    toast.add({ title: "回滚失败", description: error?.message || "请求失败", color: "error" });
  } finally {
    actionLoading.value = false;
  }
}

async function onImported() {
  await fetchCatalog();
  await fetchRegistry();
}

function openAuditDrawer(skillId: string) {
  selectedSkillId.value = skillId;
  auditDrawerOpen.value = true;
}

onMounted(async () => {
  await Promise.all([fetchCatalog(), fetchRegistry()]);
});
</script>
