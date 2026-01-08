<template>
  <div class="p-6 space-y-6">
    <div class="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
      <div>
        <h1 class="text-2xl font-bold text-gray-900 dark:text-white">异步任务（Event Fabric）</h1>
        <p class="text-gray-600 dark:text-gray-400">
          统一监管投递、重试队列与 DLQ；用于知识库反馈再加工等异步链路。
        </p>
      </div>
      <div class="flex flex-wrap gap-2">
        <UButton icon="i-heroicons-arrow-path" :loading="loading" @click="refresh">
          刷新
        </UButton>
      </div>
    </div>

    <UAlert
      v-if="!allowAccess"
      icon="i-heroicons-lock-closed"
      color="amber"
      variant="subtle"
      title="无权限"
      description="仅 Root 管理员可查看 Event Fabric 监管面板。"
    />

    <div v-else class="space-y-4">
      <UCard>
        <template #header>
          <div class="flex flex-wrap items-center gap-3">
            <div class="text-sm text-gray-600 dark:text-gray-400">
              当前租户：
              <span class="font-mono">{{ overview?.tenant_uuid || "-" }}</span>
            </div>
            <div class="text-sm text-gray-600 dark:text-gray-400">
              更新时间：
              <span class="font-mono">{{ overview?.now || "-" }}</span>
            </div>
          </div>
        </template>

        <div class="grid grid-cols-1 gap-3 md:grid-cols-4">
          <UInput v-model="filters.namespace" icon="i-heroicons-tag" placeholder="namespace（默认 knowledge.space.feedback）" />
          <UInput v-model="filters.name" icon="i-heroicons-hashtag" placeholder="name（默认 reprocess）" />
          <UInput v-model="filters.subscriberId" icon="i-heroicons-identification" placeholder="subscriber_id（默认 core.knowledge_space.reprocess）" />
          <UButton variant="outline" @click="refresh">应用筛选</UButton>
        </div>
      </UCard>

      <div class="grid grid-cols-1 gap-4 md:grid-cols-3">
        <UCard>
          <template #header>
            <div class="flex items-center justify-between">
              <div class="font-semibold">DLQ</div>
              <UBadge variant="subtle" color="red">{{ overview?.stats.dlq.total ?? 0 }}</UBadge>
            </div>
          </template>
          <div class="text-sm text-gray-600 dark:text-gray-400">
            进入死信队列的消息总数（按当前筛选 topic 聚合）。
          </div>
        </UCard>
        <UCard>
          <template #header>
            <div class="flex items-center justify-between">
              <div class="font-semibold">投递 Attempts</div>
              <UBadge variant="subtle" color="blue">{{ overview?.stats.delivery_attempts.total ?? 0 }}</UBadge>
            </div>
          </template>
          <div class="text-sm text-gray-600 dark:text-gray-400">
            subscriber：
            <span class="font-mono">{{ overview?.stats.delivery_attempts.subscriber_id || "-" }}</span>
          </div>
        </UCard>
        <UCard>
          <template #header>
            <div class="flex items-center justify-between">
              <div class="font-semibold">回放任务</div>
              <UBadge variant="subtle" color="gray">{{ overview?.stats.replay_tasks.recent?.length ?? 0 }}</UBadge>
            </div>
          </template>
          <div class="text-sm text-gray-600 dark:text-gray-400">
            最近 {{ replayLimit }} 条 replay task（按提交时间倒序）。
          </div>
        </UCard>
      </div>

      <UCard>
        <template #header>
          <div class="flex flex-wrap items-center justify-between gap-3">
            <div class="font-semibold">Topics</div>
            <div class="text-xs text-gray-500">
              仅展示当前租户下、满足筛选条件的 Topics。
            </div>
          </div>
        </template>

        <UTable :columns="topicColumns" :data="topicRows" :loading="loading" row-key="uuid" />
      </UCard>

      <UCard>
        <template #header>
          <div class="flex flex-wrap items-center justify-between gap-3">
            <div class="font-semibold">DLQ 消息</div>
            <div class="flex flex-wrap gap-2">
              <USelect
                v-model="dlq.topic"
                :items="dlqTopicOptions"
                option-attribute="label"
                value-attribute="value"
                class="min-w-[260px]"
                placeholder="请选择 topic"
              />
              <UButton variant="outline" :loading="dlqLoading" @click="loadDlq(1)">加载</UButton>
            </div>
          </div>
        </template>

        <UAlert
          v-if="dlq.items.length === 0 && !dlqLoading"
          icon="i-heroicons-information-circle"
          variant="subtle"
          class="border-dashed"
          title="暂无 DLQ 消息"
          description="选择 topic 后点击“加载”查看死信队列消息。"
        />

        <div v-else class="space-y-3">
          <UTable
            :columns="dlqColumns"
            :data="dlq.items"
            :loading="dlqLoading"
            row-key="id"
          />

          <div class="flex items-center justify-between">
            <div class="text-xs text-gray-500">
              共 {{ dlq.pagination.total }} 条 · 第 {{ dlq.pagination.page }} 页
            </div>
            <div class="flex gap-2">
              <UButton size="sm" variant="outline" :disabled="dlq.pagination.page <= 1" @click="loadDlq(dlq.pagination.page - 1)">
                上一页
              </UButton>
              <UButton
                size="sm"
                variant="outline"
                :disabled="dlq.pagination.page * dlq.pagination.page_size >= dlq.pagination.total"
                @click="loadDlq(dlq.pagination.page + 1)"
              >
                下一页
              </UButton>
            </div>
          </div>
        </div>
      </UCard>

      <UCard>
        <template #header>
          <div class="font-semibold">Replay 任务（最近）</div>
        </template>
        <UAlert
          v-if="(overview?.stats.replay_tasks.recent?.length ?? 0) === 0"
          icon="i-heroicons-information-circle"
          variant="subtle"
          class="border-dashed"
          title="暂无回放任务"
          description="当你通过 DLQ replay 或手动创建 replay task 后，这里会显示最近任务。"
        />
        <UTable
          v-else
          :columns="replayColumns"
          :data="overview?.stats.replay_tasks.recent || []"
          row-key="id"
        />
      </UCard>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, h, onMounted, reactive, ref, resolveComponent } from "vue";
import { storeToRefs } from "pinia";
import { useUserStore } from "~/stores/user";
import { useEventFabricService, type EventFabricOverview, type EventFabricDlqMessage } from "~/composables/api/services/eventFabricService";

definePageMeta({
  title: "异步任务（Event Fabric）",
  layout: "default",
});

const userStore = useUserStore();
const { isRoot } = storeToRefs(userStore);
const allowAccess = computed(() => isRoot.value);

const svc = useEventFabricService();
const toast = useToast();

const loading = ref(false);
const overview = ref<EventFabricOverview | null>(null);

const replayLimit = 20;

const filters = reactive({
  namespace: "knowledge.space.feedback",
  name: "reprocess",
  subscriberId: "core.knowledge_space.reprocess",
});

const topicColumns = [
  {
    accessorKey: "full_topic",
    header: "Full Topic",
    cell: ({ row }: any) =>
      h(
        "span",
        { class: "font-mono text-xs text-gray-700 dark:text-gray-200" },
        row.original.full_topic || "-"
      ),
  },
  { accessorKey: "lifecycle", header: "Lifecycle" },
  { accessorKey: "max_retry", header: "MaxRetry" },
  { accessorKey: "ack_timeout_sec", header: "AckTimeout(s)" },
  { accessorKey: "dlq", header: "DLQ" },
  { accessorKey: "attempts", header: "Attempts" },
];

const topicRows = computed(() => {
  const topics = overview.value?.topics || [];
  const dlq = overview.value?.stats.dlq.by_topic || [];
  const attempts = overview.value?.stats.delivery_attempts.by_topic || [];

  const dlqMap = new Map(dlq.map((x) => [x.topic_uuid, x]));
  const attemptMap = new Map(attempts.map((x) => [x.topic_uuid, x]));

  return topics.map((t) => {
    const dlqStats = dlqMap.get(t.uuid);
    const attemptStats = attemptMap.get(t.uuid);
    return {
      ...t,
      dlq: dlqStats ? formatByStatus(dlqStats.by_status) : "-",
      attempts: attemptStats ? formatByStatus(attemptStats.by_status) : "-",
    };
  });
});

function formatByStatus(by: Record<string, number>) {
  const keys = Object.keys(by || {}).sort();
  if (keys.length === 0) return "0";
  return keys.map((k) => `${k}:${by[k]}`).join(" · ");
}

async function refresh() {
  if (!allowAccess.value) return;
  loading.value = true;
  try {
    const res = await svc.getOverview({
      namespace: filters.namespace || undefined,
      name: filters.name || undefined,
      subscriber_id: filters.subscriberId || undefined,
      limit: replayLimit,
    });
    overview.value = res.data.data;
  } catch (e: any) {
    toast.add({
      title: "加载失败",
      description: e?.message || "无法获取 Event Fabric overview",
      color: "error",
    });
  } finally {
    loading.value = false;
  }
}

const dlqLoading = ref(false);
const replayLoadingId = ref<string | null>(null);
const dlq = reactive<{
  topic: string | null;
  items: EventFabricDlqMessage[];
  pagination: { total: number; page: number; page_size: number };
}>({
  topic: null,
  items: [],
  pagination: { total: 0, page: 1, page_size: 20 },
});

const dlqTopicOptions = computed(() => {
  const topics = overview.value?.topics || [];
  const opts = topics.map((t) => ({ label: t.full_topic, value: t.uuid }));
  return opts;
});

const dlqColumns = [
  {
    accessorKey: "id",
    header: "Message ID",
    cell: ({ row }: any) =>
      h(
        "span",
        { class: "font-mono text-xs text-gray-700 dark:text-gray-200" },
        row.original.id
      ),
  },
  {
    accessorKey: "event_id",
    header: "Event ID",
    cell: ({ row }: any) =>
      h(
        "span",
        { class: "font-mono text-xs text-gray-700 dark:text-gray-200" },
        row.original.event_id || "-"
      ),
  },
  { accessorKey: "retry_count", header: "Retry" },
  { accessorKey: "failed_at", header: "FailedAt" },
  {
    accessorKey: "actions",
    header: "操作",
    cell: ({ row }: any) => {
      const UButton = resolveComponent("UButton") as any;
      const messageId = row.original.id as string;
      return h(
        UButton,
        {
          size: "xs",
          color: "primary",
          variant: "soft",
          loading: replayLoadingId.value === messageId,
          onClick: () => replayOne(messageId),
        },
        () => "Replay"
      );
    },
  },
];

async function loadDlq(page: number) {
  if (!dlq.topic) return;
  dlqLoading.value = true;
  try {
    const res = await svc.listDlqMessages({
      topic: dlq.topic,
      page,
      page_size: dlq.pagination.page_size,
    });
    dlq.items = res.data.data.items;
    dlq.pagination = res.data.data.pagination;
  } catch (e: any) {
    toast.add({
      title: "加载失败",
      description: e?.message || "无法获取 DLQ messages",
      color: "error",
    });
  } finally {
    dlqLoading.value = false;
  }
}

async function replayOne(messageId: string) {
  replayLoadingId.value = messageId;
  try {
    const res = await svc.replayDlqMessages({
      message_ids: [messageId],
      operator_id: "web-admin",
      notes: "replay from settings/event-fabric",
    });
    toast.add({
      title: "已提交 Replay",
      description: `replayed: ${res.data.data.replayed}`,
      color: "success",
    });
    await loadDlq(dlq.pagination.page);
    await refresh();
  } catch (e: any) {
    toast.add({
      title: "Replay 失败",
      description: e?.message || "无法 replay DLQ message",
      color: "error",
    });
  } finally {
    replayLoadingId.value = null;
  }
}

const replayColumns = [
  {
    accessorKey: "id",
    header: "Task ID",
    cell: ({ row }: any) =>
      h(
        "span",
        { class: "font-mono text-xs text-gray-700 dark:text-gray-200" },
        row.original.id
      ),
  },
  {
    accessorKey: "full_topic",
    header: "Topic",
    cell: ({ row }: any) =>
      h(
        "span",
        { class: "font-mono text-xs text-gray-700 dark:text-gray-200" },
        row.original.full_topic || row.original.topic_uuid || "-"
      ),
  },
  {
    accessorKey: "status",
    header: "Status",
    cell: ({ row }: any) => {
      const UBadge = resolveComponent("UBadge") as any;
      const status = row.original.status || "-";
      return h(
        UBadge,
        {
          size: "xs",
          variant: "subtle",
          color: replayStatusColor(status),
        },
        () => status
      );
    },
  },
  { accessorKey: "submitted_at", header: "SubmittedAt" },
  { accessorKey: "completed_at", header: "CompletedAt" },
  { accessorKey: "result_count", header: "Result" },
];

function replayStatusColor(status: string) {
  const s = (status || "").toLowerCase();
  if (s === "completed") return "green";
  if (s === "failed") return "red";
  if (s === "running") return "blue";
  return "gray";
}

onMounted(async () => {
  await refresh();
});
</script>
