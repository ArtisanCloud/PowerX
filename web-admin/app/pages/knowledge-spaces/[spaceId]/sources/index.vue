<script setup lang="ts">
import { useKnowledgeSpaceSources } from "~/composables/useKnowledgeSpaceSources";

const { t } = useI18n();
const route = useRoute();
const toast = useToast();

const spaceId = computed(() => String(route.params.spaceId || "").trim());

useHead(() => ({
  title: t("knowledgeSpaces.sources.head.title", "数据源连接"),
  meta: [
    {
      name: "description",
      content: t(
        "knowledgeSpaces.sources.head.description",
        "为知识空间连接外部数据源并创建同步任务",
      ),
    },
  ],
}));

const goBack = () => navigateTo("/knowledge-spaces");
const goConnect = () => navigateTo(`/knowledge-spaces/${encodeURIComponent(spaceId.value)}/sources/connect`);

const sources = useKnowledgeSpaceSources();
const connections = ref<ReturnType<typeof sources.listSpaceConnections>>([]);

const loadConnections = () => {
  try {
    connections.value = sources.listSpaceConnections(spaceId.value);
  } catch (err: any) {
    connections.value = [];
    toast.add({
      color: "warning",
      title: t("knowledgeSpaces.sources.loadFailed", "无法加载数据源"),
      description: err?.message || "缺少租户上下文或本地存储不可用",
    });
  }
};

onMounted(loadConnections);
watch(spaceId, loadConnections);
</script>

<template>
  <section class="mx-auto max-w-5xl space-y-6 px-6 py-8">
    <header class="rounded-2xl border border-[var(--border-color)] bg-[var(--card-bg)] p-6 shadow-sm">
      <div class="flex items-start justify-between gap-4">
        <div>
          <p class="text-sm text-[var(--text-secondary)]">
            {{ t("knowledgeSpaces.sources.badge", "Data Sources") }}
          </p>
          <h1 class="mt-1 text-2xl font-semibold text-[var(--text-primary)]">
            {{ t("knowledgeSpaces.sources.title", "连接数据源") }}
          </h1>
          <p class="mt-2 text-sm text-[var(--text-secondary)]">
            {{
              t(
                "knowledgeSpaces.sources.subtitle",
                "用于对接 Notion、飞书等需要鉴权的系统，并以“增量同步任务”的方式持续更新内容。",
              )
            }}
          </p>
          <p class="mt-2 text-xs text-[var(--text-secondary)]">
            {{ t("knowledgeSpaces.sources.spaceHint", "当前空间 ID：{id}", { id: spaceId.slice(0, 8) + "…" }) }}
          </p>
        </div>
        <UButton color="neutral" variant="subtle" icon="i-heroicons-arrow-left" @click="goBack">
          {{ t("common.back", "返回") }}
        </UButton>
      </div>
    </header>

      <UCard :ui="{ body: { padding: 'p-6' } }">
      <template #header>
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h2 class="text-lg font-semibold text-[var(--text-primary)]">
              {{ t("knowledgeSpaces.sources.list.title", "已连接的数据源") }}
            </h2>
            <p class="text-sm text-[var(--text-secondary)]">
              {{ t("knowledgeSpaces.sources.list.desc", "这里将展示连接器实例、同步范围与最近同步状态。") }}
            </p>
          </div>
          <UButton color="primary" icon="i-heroicons-plus-circle" @click="goConnect">
            {{ t("knowledgeSpaces.sources.actions.connect", "新增连接") }}
          </UButton>
        </div>
      </template>

      <div v-if="connections.length === 0" class="space-y-4">
        <UAlert
          color="warning"
          variant="soft"
          icon="i-heroicons-information-circle"
          :title="t('knowledgeSpaces.sources.stub.title', '尚未连接任何数据源')"
          :description="
            t(
              'knowledgeSpaces.sources.stub.desc',
              '建议从 Notion/飞书开始：先完成租户级授权，再为当前空间创建增量同步任务。',
            )
          "
        />
      </div>

      <div v-else class="space-y-4">
        <UCard
          v-for="item in connections"
          :key="item.connector.id"
          :ui="{ body: { padding: 'p-5' } }"
        >
          <div class="flex flex-wrap items-start justify-between gap-3">
            <div class="min-w-0">
              <div class="flex items-center gap-2">
                <UBadge color="neutral" variant="soft">
                  {{ item.provider === "notion" ? "Notion" : "飞书" }}
                </UBadge>
                <span class="text-sm font-semibold text-[var(--text-primary)]">
                  {{ item.credential.label }}
                </span>
              </div>
              <p class="mt-1 text-xs text-[var(--text-secondary)]">
                {{ t("knowledgeSpaces.sources.tenantReuseHint", "租户级凭据复用：同一租户可被多个空间共享。") }}
              </p>
              <p v-if="item.credential.maskedHint" class="mt-1 text-xs text-[var(--text-secondary)]">
                {{ t("knowledgeSpaces.sources.maskedHint", "凭据提示：{hint}", { hint: item.credential.maskedHint }) }}
              </p>
            </div>
            <div class="flex gap-2">
              <UButton
                color="neutral"
                variant="soft"
                icon="i-heroicons-plus"
                @click="goConnect"
              >
                {{ t("knowledgeSpaces.sources.actions.addJob", "新增同步任务") }}
              </UButton>
            </div>
          </div>

          <UDivider class="my-4" />

          <div class="space-y-2">
            <div
              v-for="job in item.jobs"
              :key="job.id"
              class="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-[var(--border-color)] bg-[var(--card-bg)] px-4 py-3"
            >
              <div class="min-w-0">
                <div class="flex items-center gap-2">
                  <UBadge :color="job.status === 'active' ? 'success' : job.status === 'failed' ? 'error' : 'neutral'" variant="soft">
                    {{ job.status }}
                  </UBadge>
                  <span class="text-sm text-[var(--text-primary)]">
                    {{ t("knowledgeSpaces.sources.job.schedule", "计划：{cron}", { cron: job.schedule }) }}
                  </span>
                </div>
                <p class="mt-1 text-xs text-[var(--text-secondary)]">
                  {{ t("knowledgeSpaces.sources.job.mode", "模式：{mode}", { mode: job.syncMode }) }}
                </p>
              </div>
              <div class="text-xs text-[var(--text-secondary)]">
                <span v-if="job.lastOkAt">{{ t("knowledgeSpaces.sources.job.lastOk", "最近成功：{t}", { t: job.lastOkAt }) }}</span>
                <span v-else>{{ t("knowledgeSpaces.sources.job.neverRun", "尚未运行") }}</span>
              </div>
            </div>
          </div>
        </UCard>
      </div>
    </UCard>
  </section>
</template>
