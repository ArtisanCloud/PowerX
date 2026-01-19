<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import { useKnowledgeSpaces, type IngestionJobRecord } from "~/composables/useKnowledgeSpaces";
import { useEmbeddingGuard } from "~/composables/useEmbeddingGuard";

useHead({ title: "入库记录" });

const route = useRoute();
const router = useRouter();
const api = useKnowledgeSpaces();
const toast = useToast();
const { ensureEmbeddingReady } = useEmbeddingGuard();
const embeddingReady = ref(false);

const spaceId = computed(() => String(route.params.spaceId || "").trim());
const loading = ref(false);
const error = ref<string | null>(null);
const jobs = ref<IngestionJobRecord[]>([]);
const deleting = ref(false);
const deleteOpen = ref(false);
const deleteTargetJobId = ref<string>("");
const deleteError = ref<string | null>(null);

const fetchJobs = async () => {
  if (!embeddingReady.value) return;
  if (!spaceId.value) return;
  loading.value = true;
  error.value = null;
  try {
    jobs.value = await api.listIngestionJobs(spaceId.value, 50);
  } catch (e: any) {
    error.value = String(e?.message || "加载失败");
    jobs.value = [];
  } finally {
    loading.value = false;
  }
};

let pollingTimer: ReturnType<typeof setInterval> | null = null;

const startPolling = () => {
  if (pollingTimer) return;
  pollingTimer = setInterval(async () => {
    if (!jobs.value.length) return;
    const hasRunning = jobs.value.some((j) => isRunningStatus(j.status));
    if (!hasRunning) return;
    await fetchJobs();
  }, 5000);
};

const stopPolling = () => {
  if (!pollingTimer) return;
  clearInterval(pollingTimer);
  pollingTimer = null;
};

onMounted(async () => {
  if (!(await ensureEmbeddingReady())) return;
  embeddingReady.value = true;
  await fetchJobs();
  startPolling();
});
watch(() => spaceId.value, fetchJobs);
onUnmounted(() => stopPolling());

const goBack = async () => {
  if (process.client && window.history.length > 1) {
    router.back();
    return;
  }
  await navigateTo("/knowledge-spaces");
};

const goJob = (jobId: string) => {
  if (!jobId) return;
  navigateTo(`/knowledge-spaces/${encodeURIComponent(spaceId.value)}/ingestions/${encodeURIComponent(jobId)}`);
};

const goFeedback = () => {
  navigateTo(`/knowledge-spaces/feedback?spaceId=${encodeURIComponent(spaceId.value)}`);
};

const askDelete = (jobId: string) => {
  deleteTargetJobId.value = jobId;
  deleteError.value = null;
  deleteOpen.value = true;
};

const closeDelete = () => {
  if (process.client) {
    (document.activeElement as HTMLElement | null)?.blur?.();
  }
  deleteOpen.value = false;
};

const confirmDelete = async () => {
  if (!spaceId.value || !deleteTargetJobId.value) return;
  deleting.value = true;
  deleteError.value = null;
  try {
    await api.deleteIngestionJob(spaceId.value, deleteTargetJobId.value);
    toast.add({ title: "已删除", description: `Job ${deleteTargetJobId.value}` });
    deleteOpen.value = false;
    await fetchJobs();
  } catch (e: any) {
    deleteError.value = String(e?.message || "删除失败");
  } finally {
    deleting.value = false;
  }
};

const copy = async (text: string) => {
  if (!process.client) return;
  try {
    await navigator.clipboard.writeText(text);
    toast.add({ title: "已复制", description: text });
  } catch {
    // ignore
  }
};

const isRunningStatus = (status?: string) => {
  const s = String(status || "").toLowerCase();
  return s === "running" || s === "retrying" || s === "pending";
};

const jobProgressPct = (job: IngestionJobRecord) => {
  const pct =
    (typeof job.chunkCoveragePct === "number" ? job.chunkCoveragePct : 0) ||
    (typeof job.embeddingSuccessPct === "number" ? job.embeddingSuccessPct : 0) ||
    (typeof job.maskingCoveragePct === "number" ? job.maskingCoveragePct : 0);
  if (!Number.isFinite(pct) || pct < 0) return 0;
  return Math.min(100, Math.max(0, pct));
};
</script>

<template>
  <div class="p-6 space-y-4">
    <div class="flex items-center justify-between gap-2">
      <div class="flex items-start gap-3">
        <UButton color="neutral" variant="ghost" icon="i-heroicons-arrow-left" @click="goBack">返回列表</UButton>
        <div>
          <div class="text-lg font-semibold">入库记录</div>
          <div class="text-sm text-[var(--text-secondary)]">Space：{{ spaceId }}</div>
        </div>
      </div>
      <div class="flex items-center gap-2">
        <UButton color="neutral" variant="soft" size="sm" icon="i-heroicons-chat-bubble-left-right" @click="goFeedback">
          反馈
        </UButton>
        <UButton color="neutral" variant="soft" size="sm" icon="i-heroicons-arrow-path" :loading="loading" @click="fetchJobs">
          刷新
        </UButton>
      </div>
    </div>

    <UAlert
      v-if="error"
      color="error"
      variant="soft"
      title="加载失败"
      :description="error"
    />

    <UAlert
      v-else-if="!loading && !jobs.length"
      color="neutral"
      variant="soft"
      title="暂无入库记录"
      description="触发入库后，这里会显示每一次入库作业。"
    />

    <div v-else class="grid gap-3">
      <div
        v-for="job in jobs"
        :key="job.jobId"
        class="rounded-xl border border-gray-200 bg-white p-4 space-y-2"
      >
        <div class="flex items-start justify-between gap-3">
          <div class="min-w-0">
            <div class="font-medium truncate">任务：{{ job.jobId }}</div>
            <div class="text-xs text-[var(--text-secondary)]">
              类型：{{ job.sourceType || "-" }} · Chunks：{{ job.chunkTotal }}
            </div>
          </div>
          <div class="flex items-center gap-2">
            <UBadge
              :color="job.status === 'completed' ? 'success' : job.status === 'failed' ? 'error' : job.status === 'blocked' ? 'warning' : 'neutral'"
              variant="soft"
            >
              {{ job.status }}
            </UBadge>
            <UButton size="xs" color="primary" variant="soft" @click="goJob(job.jobId)">
              预览切块
            </UButton>
            <UButton size="xs" color="error" variant="soft" :disabled="deleting" type="button" @click.stop="askDelete(job.jobId)">
              删除入库
            </UButton>
          </div>
        </div>

        <div class="flex items-center gap-2 text-xs text-[var(--text-tertiary)]">
          <span>重试：{{ job.retryCount }}</span>
          <span v-if="job.errorCode">· 错误：{{ job.errorCode }}</span>
          <span v-if="job.reason" class="truncate">· 原因：{{ job.reason }}</span>
        </div>

        <div v-if="isRunningStatus(job.status)" class="space-y-2">
          <div class="text-xs text-[var(--text-secondary)]">
            进度：{{ jobProgressPct(job).toFixed(0) }}%
          </div>
          <UProgress :value="jobProgressPct(job)" color="primary" />
        </div>

        <div class="flex items-center gap-2">
          <UButton size="xs" color="neutral" variant="soft" icon="i-heroicons-clipboard" @click="copy(job.jobId)">
            复制 Job ID
          </UButton>
        </div>
      </div>
    </div>

    <UModal
      v-model:open="deleteOpen"
      title="删除入库任务"
      description="将删除该 Job 的切块、向量记录与本地产物（如存在）。此操作不可撤销。"
      :ui="{ width: 'max-w-xl w-full', body: 'p-4 sm:p-5', footer: 'justify-end' }"
      :close="{ onClick: closeDelete }"
      prevent-close
    >
      <template #body>
        <div class="space-y-3">
          <div class="text-sm">
            确认删除 Job：<span class="font-mono">{{ deleteTargetJobId }}</span>
          </div>
          <UAlert v-if="deleteError" color="error" variant="soft" title="删除失败" :description="deleteError" />
        </div>
      </template>
      <template #footer>
        <div class="flex items-center justify-end gap-2">
          <UButton color="neutral" variant="subtle" type="button" :disabled="deleting" @click="closeDelete">取消</UButton>
          <UButton color="error" type="button" :loading="deleting" @click="confirmDelete">确认删除</UButton>
        </div>
      </template>
    </UModal>
  </div>
</template>
