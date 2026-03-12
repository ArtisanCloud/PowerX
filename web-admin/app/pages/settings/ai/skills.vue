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

      <UCard>
        <template #header>
          <div class="flex items-center justify-between">
            <UTabs v-model="activeTab" :items="tabItems" />
            <span class="text-xs text-[var(--text-secondary)]">
              <template v-if="activeTab === 'registry'">共 {{ registryTotal }} 条</template>
              <template v-else>共 {{ catalogItems.length }} 条</template>
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
          <div v-if="loadingCatalog" class="text-sm text-[var(--text-secondary)]">加载中...</div>
          <div v-else-if="catalogItems.length === 0" class="text-sm text-[var(--text-secondary)]">暂无数据</div>
          <UTable
            v-else
            :columns="catalogColumns"
            :data="catalogItems"
            row-key="skill_id"
            :ui="{ divide: 'divide-y divide-[var(--border-color)]' }"
          />
        </template>
      </UCard>
    </template>
  </div>
</template>

<script setup lang="ts">
import { h, resolveComponent } from "vue";
import { storeToRefs } from "pinia";
import { useSkillsService, type SkillRecord } from "~/composables/api/services";
import { useUserStore } from "~/stores/user";

const toast = useToast();
const skillsService = useSkillsService();
const userStore = useUserStore();
const { isRoot } = storeToRefs(userStore);

definePageMeta({
  title: "Skills",
  layout: "default",
});

type CatalogItem = {
  skill_id: string;
  recommended_version: string;
  risk_level: string;
  category?: string;
  summary?: string;
};

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

const catalogColumns = [
  { accessorKey: "skill_id", header: "Skill ID" },
  { accessorKey: "recommended_version", header: "推荐版本" },
  { accessorKey: "risk_level", header: "风险等级" },
  { accessorKey: "category", header: "分类" },
  { accessorKey: "summary", header: "说明" },
];

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
        skill_id: item.skill_id,
        recommended_version: item.version,
        risk_level: "unknown",
        category: "native",
        summary: "来自本地 builtin registry",
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
