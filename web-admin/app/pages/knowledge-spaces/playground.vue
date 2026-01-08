<script setup lang="ts">
import { h } from "vue";
import { useKnowledgeSpaces, type ProfileVersionRecord, type RetrievalPlaygroundRecord } from "~/composables/useKnowledgeSpaces";

definePageMeta({
  layout: "default",
  title: "Retrieval Playground",
});

const api = useKnowledgeSpaces();
const toast = useToast();

const spaceId = ref("");
const query = ref("");
const profileKey = ref("default");
const profiles = ref<ProfileVersionRecord[]>([]);
const selectedProfile = ref<string>("");

const loadingProfiles = ref(false);
const loadingRun = ref(false);

const resultDefault = ref<RetrievalPlaygroundRecord | null>(null);
const resultSelected = ref<RetrievalPlaygroundRecord | null>(null);

const profileOptions = computed(() =>
  profiles.value.map((p) => ({
    label: `${p.displayName || p.profileKey} · v${p.version} · ${p.status}`,
    value: p.uuid,
  })),
);

const loadProfiles = async () => {
  loadingProfiles.value = true;
  try {
    profiles.value = await api.listRagProfiles(profileKey.value);
    if (!selectedProfile.value) {
      const published = profiles.value.find((p) => p.status === "published");
      if (published) selectedProfile.value = published.uuid;
    }
  } catch (e: any) {
    toast.add({ title: "加载失败", description: e?.message || "无法获取 RAG Profiles", color: "error" });
  } finally {
    loadingProfiles.value = false;
  }
};

const runCompare = async () => {
  if (!spaceId.value || !query.value) return;
  loadingRun.value = true;
  try {
    resultDefault.value = await api.retrievalPlayground(spaceId.value, {
      query: query.value,
    });
    resultSelected.value = selectedProfile.value
      ? await api.retrievalPlayground(spaceId.value, {
          query: query.value,
          ragProfileUuid: selectedProfile.value,
        })
      : null;
  } catch (e: any) {
    toast.add({ title: "运行失败", description: e?.message || "检索失败", color: "error" });
  } finally {
    loadingRun.value = false;
  }
};

onMounted(async () => {
  await loadProfiles();
});

const candidateColumns = [
  { accessorKey: "chunkId", header: "Chunk ID" },
  { accessorKey: "score", header: "Score" },
  {
    accessorKey: "text",
    header: "Preview",
    cell: ({ row }: any) => h("div", { class: "text-xs text-gray-600 line-clamp-2" }, row.original.text || "-"),
  },
];

const stageColumns = [
  { accessorKey: "name", header: "Stage" },
  { accessorKey: "candidateCount", header: "Candidates" },
  { accessorKey: "latencyMs", header: "Latency(ms)" },
  { accessorKey: "degradeReason", header: "Degrade" },
];
</script>

<template>
  <section class="space-y-6 p-6">
    <header class="space-y-2">
      <h1 class="text-2xl font-semibold text-gray-900">Retrieval Playground</h1>
      <p class="text-sm text-gray-500">
        基于空间 + RAG Profile 做检索 A/B 对比，展示候选、阶段耗时与 trace_id。
      </p>
    </header>

    <UCard :ui="{ body: { padding: 'p-5 space-y-4' } }">
      <div class="grid grid-cols-1 gap-3 md:grid-cols-3">
        <UInput v-model="spaceId" placeholder="spaceId（UUID）" icon="i-heroicons-circle-stack" />
        <UInput v-model="profileKey" placeholder="profileKey（默认 default）" icon="i-heroicons-tag" @blur="loadProfiles" />
        <USelect
          v-model="selectedProfile"
          :items="profileOptions"
          :loading="loadingProfiles"
          placeholder="选择 RAG Profile 版本"
        />
      </div>
      <div class="flex flex-col gap-3 md:flex-row md:items-center">
        <UInput v-model="query" class="flex-1" placeholder="输入查询（query）" icon="i-heroicons-magnifying-glass" />
        <UButton color="primary" :loading="loadingRun" :disabled="!spaceId || !query" @click="runCompare">
          运行 A/B
        </UButton>
      </div>
    </UCard>

    <div class="grid grid-cols-1 gap-4 lg:grid-cols-2">
      <UCard>
        <template #header>
          <div class="flex items-center justify-between">
            <div class="font-semibold">默认（空间绑定 / latest published）</div>
            <UBadge variant="subtle">{{ resultDefault?.traceId || "-" }}</UBadge>
          </div>
        </template>
        <UTable v-if="resultDefault" :columns="candidateColumns" :data="resultDefault.candidates" row-key="chunkId" />
        <div v-if="resultDefault" class="mt-4 grid gap-3 md:grid-cols-2">
          <UCard :ui="{ body: { padding: 'p-3 space-y-2' } }">
            <p class="text-sm font-medium text-gray-800">Stages</p>
            <UTable :columns="stageColumns" :data="resultDefault.stages" />
          </UCard>
          <UCard :ui="{ body: { padding: 'p-3 space-y-2' } }">
            <p class="text-sm font-medium text-gray-800">Context Pack</p>
            <pre class="max-h-48 overflow-auto rounded bg-gray-50 p-2 text-xs text-gray-700">{{ JSON.stringify(resultDefault.context_pack, null, 2) }}</pre>
          </UCard>
        </div>
        <UAlert v-else variant="subtle" title="暂无结果" description="运行后这里显示默认 profile 的候选列表。" />
      </UCard>

      <UCard>
        <template #header>
          <div class="flex items-center justify-between">
            <div class="font-semibold">选中 Profile</div>
            <UBadge variant="subtle">{{ resultSelected?.traceId || "-" }}</UBadge>
          </div>
        </template>
        <UTable v-if="resultSelected" :columns="candidateColumns" :data="resultSelected.candidates" row-key="chunkId" />
        <div v-if="resultSelected" class="mt-4 grid gap-3 md:grid-cols-2">
          <UCard :ui="{ body: { padding: 'p-3 space-y-2' } }">
            <p class="text-sm font-medium text-gray-800">Stages</p>
            <UTable :columns="stageColumns" :data="resultSelected.stages" />
          </UCard>
          <UCard :ui="{ body: { padding: 'p-3 space-y-2' } }">
            <p class="text-sm font-medium text-gray-800">Context Pack</p>
            <pre class="max-h-48 overflow-auto rounded bg-gray-50 p-2 text-xs text-gray-700">{{ JSON.stringify(resultSelected.context_pack, null, 2) }}</pre>
          </UCard>
        </div>
        <UAlert v-else variant="subtle" title="暂无结果" description="选择 profile 版本并运行后这里显示候选列表。" />
      </UCard>
    </div>
  </section>
</template>
