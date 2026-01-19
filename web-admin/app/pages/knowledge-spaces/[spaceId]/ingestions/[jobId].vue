<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useKnowledgeSpaces, type IngestionChunkListResult, type IngestionChunkRecord, type IngestionJobRecord } from "~/composables/useKnowledgeSpaces";
import { useMediaAssetService, type MediaAssetAdminView } from "~/composables/api/services/mediaAssetService";
import { useEmbeddingGuard } from "~/composables/useEmbeddingGuard";
import { useWSBus } from "~/composables/useWSBus";
import { type IngestionProgress } from "~/composables/wsBus";

useHead({ title: "切块预览" });

const route = useRoute();
const router = useRouter();
const api = useKnowledgeSpaces();
const media = useMediaAssetService();
const toast = useToast();
const wsBus = useWSBus();
const { ensureEmbeddingReady } = useEmbeddingGuard();
const embeddingReady = ref(false);

const spaceId = computed(() => String(route.params.spaceId || "").trim());
const jobId = computed(() => String(route.params.jobId || "").trim());

const loading = ref(false);
const error = ref<string | null>(null);
const result = ref<IngestionChunkListResult | null>(null);
const jobInfo = ref<IngestionJobRecord | null>(null);
const wsProgress = ref<IngestionProgress | null>(null);
const spaceName = ref<string>("");
const sourceAsset = ref<MediaAssetAdminView | null>(null);

const page = ref(1);
const pageSize = ref(50);
const keyword = ref("");
const kindFilter = ref<string>("auto");

let pollTimer: ReturnType<typeof setInterval> | null = null;
let progressTimer: ReturnType<typeof setTimeout> | null = null;
let pendingProgress: IngestionProgress | null = null;
let lastJobStatus = "";
let unsubscribeIngestionProgress: (() => void) | null = null;

const editOpen = ref(false);
const editing = ref(false);
const editError = ref<string | null>(null);
const editChunk = ref<IngestionChunkRecord | null>(null);
const editContent = ref("");
const editBy = ref("");
const editReason = ref("");

const deleteOpen = ref(false);
const deleting = ref(false);
const deleteError = ref<string | null>(null);

type ProvenanceRegion = { x1: number; y1: number; x2: number; y2: number; confidence?: number };
type ProvenancePage = { page_number: number; regions?: ProvenanceRegion[] };
type ChunkProvenance = { pages?: ProvenancePage[] };

const previewOpen = ref(false);
const previewLoading = ref(false);
const previewError = ref<string | null>(null);
const previewChunk = ref<IngestionChunkRecord | null>(null);
const previewPageNumber = ref<number | null>(null);
const previewImageUrl = ref<string | null>(null);

const shortId = (raw: string, keep = 8) => {
  const s = String(raw || "").trim();
  if (!s) return "";
  if (s.length <= keep) return s;
  return `${s.slice(0, keep)}…`;
};

const goBackToSpace = async () => {
  if (!spaceId.value) return;
  if (process.client && window.history.length > 1) {
    router.back();
    return;
  }
  await navigateTo(`/knowledge-spaces/${encodeURIComponent(spaceId.value)}`);
};

const isString = (v: unknown): v is string => typeof v === "string";
const normalizeStr = (v: unknown) => (isString(v) ? v.trim() : "");
const isHTTPURL = (v: unknown) => {
  const s = normalizeStr(v);
  return /^https?:\/\//i.test(s);
};
const formatURLLabel = (raw: string) => {
  const s = raw.trim();
  try {
    const u = new URL(s);
    const path = u.pathname || "/";
    // 不展示长 query，避免 presign 链接占满 UI
    return `${u.host}${path}${u.search ? "?…" : ""}`;
  } catch {
    return s;
  }
};
const truncate = (s: string, max = 64) => (s.length > max ? `${s.slice(0, Math.max(0, max - 1))}…` : s);
const extractHTTPURLs = (text: string) => {
  const raw = String(text || "");
  const matches = raw.match(/https?:\/\/[^\s)]+/gi) || [];
  const cleaned = matches
    .map((u) => u.replace(/[),.;]+$/g, "").trim())
    .filter(Boolean);
  const uniq: string[] = [];
  for (const u of cleaned) {
    if (!uniq.includes(u)) uniq.push(u);
    if (uniq.length >= 3) break;
  }
  return uniq;
};
const replaceHTTPURLs = (text: string) => String(text || "").replace(/https?:\/\/[^\s)]+/gi, "【链接】");

const normalizeChunkContentForDisplay = (raw: string) => {
  const s = String(raw || "").trim();
  if (!s) return "";
  // builtin processors 的占位内容：`source=<url> format=<fmt> ...`，对 UI 来说噪声很大
  // 这里仅在匹配到固定前缀时剥离，避免影响真实正文。
  const m = s.match(/^(OCR\s+)?source=\S+\s+format=\S+\s+(.*)$/s);
  if (m && m[2]) return m[2].trim();
  return s;
};

const chunkContentForUI = (item: IngestionChunkRecord) => normalizeChunkContentForDisplay(item?.content || "");
const isSyntheticChunkContent = (raw: string) => /^(OCR\s+)?source=\S+\s+format=\S+\s+/s.test(String(raw || "").trim());

const totalPages = computed(() => {
  const total = Number(result.value?.total ?? 0);
  const size = Number(pageSize.value || 1);
  return Math.max(1, Math.ceil(total / size));
});

const displaySpaceName = computed(() => spaceName.value.trim() || "知识空间");
const displayAssetName = computed(() => sourceAsset.value?.name?.trim() || "原文件");
const segmentStrategyHint = computed(() => {
  const info = jobInfo.value;
  if (!info) return "";
  const mode = String(info.segmentMode || "-");
  const size = Number(info.chunkSize || 0);
  const overlap = Number(info.chunkOverlap || 0);
  const separators = (info.separators || []).length ? info.separators.join(" / ") : "-";
  const pagePriority = info.pagePriority ? "是" : "否";
  const anchors = Object.entries(info.chunkAnchors || {})
    .filter(([, v]) => Boolean(v))
    .map(([k]) => k)
    .join(" / ");
  return `分页优先: ${pagePriority} · 模式: ${mode} · chunk: ${size} / overlap: ${overlap} · 分隔符: ${separators} · anchors: ${
    anchors || "-"
  }`;
});

const isUUIDLike = (v: unknown) => {
  const s = normalizeStr(v).toLowerCase();
  return /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(s);
};

const visibleMetadataEntries = (item: IngestionChunkRecord) => {
  const meta = (item?.metadata || {}) as Record<string, any>;
  const denyKeys = new Set([
    "provenance",
    "source_uri",
    "job_uuid",
    "trace_id",
    "request_id",
  ]);
  const allowKeys = new Set([
    "page",
    "pages",
    "section",
    "segment_mode",
    "chunk_idx",
    "masked",
    "confidence",
  ]);

  const out: Array<[string, any]> = [];
  for (const [k, v] of Object.entries(meta)) {
    if (denyKeys.has(k)) continue;
    if (!allowKeys.has(k)) continue;
    if (typeof v === "string" && isUUIDLike(v)) continue;
    out.push([k, v]);
  }
  return out;
};

const allKinds = computed(() => {
  const items = result.value?.items ?? [];
  const set = new Set<string>();
  for (const it of items) {
    const k = String(it.kind || "").trim();
    if (k) set.add(k);
  }
  return Array.from(set).sort((a, b) => a.localeCompare(b));
});

const resolvedKindFilter = computed(() => {
  const v = String(kindFilter.value || "").trim();
  if (v !== "auto") return v;
  // 默认优先展示“正文块”（chunk/paragraph/content），避免摘要/链接噪声把人看懵
  const kinds = allKinds.value;
  for (const preferred of ["chunk", "paragraph", "content"]) {
    if (kinds.includes(preferred)) return preferred;
  }
  return "all";
});

const filteredItems = computed<IngestionChunkRecord[]>(() => {
  const items = result.value?.items ?? [];
  const q = keyword.value.trim().toLowerCase();
  const kind = resolvedKindFilter.value;
  const byKind = kind === "all" ? items : items.filter((it) => String(it.kind || "").trim() === kind);
  const byQuery = !q
    ? byKind
    : byKind.filter((it) => {
    const hay = `${it.chunkId} ${it.kind} ${it.content}`.toLowerCase();
    return hay.includes(q);
  });

  const metaInt = (it: IngestionChunkRecord, key: string) => {
    const meta = (it?.metadata || {}) as Record<string, any>;
    const v = meta[key];
    if (typeof v === "number" && Number.isFinite(v)) return Math.trunc(v);
    if (typeof v === "string") {
      const n = Number.parseInt(v, 10);
      if (Number.isFinite(n)) return n;
    }
    return Number.MAX_SAFE_INTEGER;
  };

  return byQuery.slice().sort((a, b) => {
    const pa = metaInt(a, "section");
    const pb = metaInt(b, "section");
    if (pa !== pb) return pa - pb;
    const sa = metaInt(a, "segment_part");
    const sb = metaInt(b, "segment_part");
    if (sa !== sb) return sa - sb;
    const ca = metaInt(a, "chunk_idx");
    const cb = metaInt(b, "chunk_idx");
    if (ca !== cb) return ca - cb;
    return String(a.chunkId || "").localeCompare(String(b.chunkId || ""));
  });
});

const filteredCountHint = computed(() => {
  const total = Number(result.value?.total ?? 0);
  const shown = filteredItems.value.length;
  const kind = resolvedKindFilter.value;
  const q = keyword.value.trim();
  const active = kind !== "all" || q.length > 0;
  if (!active) return `共 ${total} 条`;
  const kindHint = kind !== "all" ? (kind === "chunk" ? "正文 chunk" : kind) : "";
  return `显示 ${shown} 条${kindHint ? `（${kindHint}）` : ""} / 共 ${total} 条`;
});

const fetchChunks = async () => {
  if (!embeddingReady.value) return;
  if (!spaceId.value || !jobId.value) return;
  loading.value = true;
  error.value = null;
  try {
    jobInfo.value = await api.getIngestionJob(spaceId.value, jobId.value);
    result.value = await api.listIngestionChunks(spaceId.value, jobId.value, { page: page.value, pageSize: pageSize.value });
  } catch (e: any) {
    error.value = String(e?.message || "加载失败");
    result.value = null;
  } finally {
    loading.value = false;
  }
};

const fetchJobStatus = async () => {
  if (!embeddingReady.value) return;
  if (!spaceId.value || !jobId.value) return;
  try {
    jobInfo.value = await api.getIngestionJob(spaceId.value, jobId.value);
  } catch {
    // ignore
  }
};

const extractMediaUUIDFromURL = (raw: string) => {
  const s = String(raw || "").trim();
  if (!s) return "";
  const m = s.match(/\/media\/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})\/resource/i);
  if (m && m[1]) return m[1];
  const m2 = s.match(/\/media\/assets\/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})\/resource/i);
  if (m2 && m2[1]) return m2[1];
  return "";
};

const fetchSpaceAndAsset = async () => {
  if (!embeddingReady.value) return;
  if (!spaceId.value) return;
  try {
    const space = await api.getSpace(spaceId.value);
    spaceName.value = String(space?.spaceName || "").trim();
  } catch {
    spaceName.value = "";
  }

  const src = String(result.value?.sourceUri || "").trim();
  const assetUUID = extractMediaUUIDFromURL(src);
  if (!assetUUID) {
    sourceAsset.value = null;
    return;
  }
  try {
    sourceAsset.value = await media.getAsset(assetUUID);
  } catch {
    sourceAsset.value = null;
  }
};

onMounted(async () => {
  if (!(await ensureEmbeddingReady())) return;
  embeddingReady.value = true;
  await fetchChunks();
  await fetchSpaceAndAsset();
  if (process.client) {
    wsBus.connect();
    subscribeIngestionProgress();
  }
});
onBeforeUnmount(() => {
  if (pollTimer) {
    clearInterval(pollTimer);
    pollTimer = null;
  }
  if (progressTimer) {
    clearTimeout(progressTimer);
    progressTimer = null;
  }
  if (unsubscribeIngestionProgress) {
    unsubscribeIngestionProgress();
    unsubscribeIngestionProgress = null;
  }
});
watch(() => [spaceId.value, jobId.value, page.value, pageSize.value] as const, () => {
  if (!embeddingReady.value) return;
  fetchChunks();
});
watch(() => [spaceId.value, result.value?.sourceUri] as const, () => {
  if (!embeddingReady.value) return;
  fetchSpaceAndAsset();
});

watch(
  () => [wsBus.connected.value, jobInfo.value?.status] as const,
  () => {
    ensurePolling();
  }
);

watch(
  () => wsBus.activeTenant.value,
  () => {
    if (!process.client) return;
    if (unsubscribeIngestionProgress) {
      unsubscribeIngestionProgress();
      unsubscribeIngestionProgress = null;
    }
    wsBus.connect();
    subscribeIngestionProgress();
  }
);

const sourceLink = computed(() => String(result.value?.sourceUri || "").trim());
const format = computed(() => String(result.value?.format || "").trim());
const wsRealtimeLabel = computed(() => (wsBus.connected.value ? "实时" : "轮询"));
const wsRealtimeDesc = computed(() => (wsBus.connected.value ? "WS 已连接" : "WS 未连接，使用轮询兜底"));
const statusColor = computed(() => {
  const s = String(jobInfo.value?.status || "").toLowerCase();
  if (s === "completed") return "success";
  if (s === "failed") return "error";
  if (s === "blocked") return "warning";
  return "neutral";
});
const showProgress = computed(() => isRunningStatus(jobInfo.value?.status));

const isRunningStatus = (status?: string) => {
  const s = String(status || "").toLowerCase();
  return s === "running" || s === "retrying" || s === "pending";
};

const jobProgressPct = (job?: IngestionJobRecord | null) => {
  if (!job) return 0;
  const pct =
    (typeof job.chunkCoveragePct === "number" ? job.chunkCoveragePct : 0) ||
    (typeof job.embeddingSuccessPct === "number" ? job.embeddingSuccessPct : 0) ||
    (typeof job.maskingCoveragePct === "number" ? job.maskingCoveragePct : 0);
  if (!Number.isFinite(pct) || pct < 0) return 0;
  return Math.min(100, Math.max(0, pct));
};

const displayProgress = computed(() => {
  if (wsProgress.value && Number.isFinite(wsProgress.value.progress)) {
    return Math.min(100, Math.max(0, wsProgress.value.progress));
  }
  return jobProgressPct(jobInfo.value);
});

const stageLabel = computed(() => wsProgress.value?.stage || "");
const statusLabel = computed(() => String(jobInfo.value?.status || "unknown"));

const chunkSourceURL = (item: IngestionChunkRecord) => {
  const v = item?.metadata?.source_uri;
  return isHTTPURL(v) ? String(v).trim() : "";
};

const ensurePolling = () => {
  const running = isRunningStatus(jobInfo.value?.status);
  if (!running || wsBus.connected.value) {
    if (pollTimer) {
      clearInterval(pollTimer);
      pollTimer = null;
    }
    return;
  }
  if (pollTimer) return;
  pollTimer = setInterval(async () => {
    if (!embeddingReady.value) return;
    await fetchJobStatus();
    if (!isRunningStatus(jobInfo.value?.status)) {
      await fetchChunks();
      if (pollTimer) {
        clearInterval(pollTimer);
        pollTimer = null;
      }
    }
  }, 5000);
};

const applyProgressUpdate = async (payload: IngestionProgress) => {
  if (!payload || payload.job_uuid !== jobId.value) return;
  wsProgress.value = payload;
  if (!jobInfo.value) {
    jobInfo.value = {
      jobId: payload.job_uuid,
      status: payload.status,
      retryCount: 0,
      chunkTotal: payload.chunk_total || 0,
      chunkCoveragePct: payload.progress || 0,
      embeddingSuccessPct: payload.embedding_pct || 0,
      maskingCoveragePct: payload.masking_pct || 0,
    };
  } else {
    jobInfo.value = {
      ...jobInfo.value,
      status: payload.status,
      chunkTotal: payload.chunk_total ?? jobInfo.value.chunkTotal,
      chunkCoveragePct: Number.isFinite(payload.progress) ? payload.progress : jobInfo.value.chunkCoveragePct,
      embeddingSuccessPct: Number.isFinite(payload.embedding_pct) ? payload.embedding_pct : jobInfo.value.embeddingSuccessPct,
      maskingCoveragePct: Number.isFinite(payload.masking_pct) ? payload.masking_pct : jobInfo.value.maskingCoveragePct,
    };
  }
  if (!lastJobStatus) lastJobStatus = String(jobInfo.value?.status || "");
  const terminal = !isRunningStatus(payload.status);
  if (terminal && lastJobStatus !== payload.status) {
    lastJobStatus = payload.status;
    await fetchChunks();
  }
};

const scheduleProgressUpdate = (payload: IngestionProgress) => {
  if (!payload) return;
  if (!isRunningStatus(payload.status)) {
    if (progressTimer) {
      clearTimeout(progressTimer);
      progressTimer = null;
    }
    pendingProgress = null;
    void applyProgressUpdate(payload);
    return;
  }
  pendingProgress = payload;
  if (progressTimer) return;
  progressTimer = setTimeout(() => {
    progressTimer = null;
    if (pendingProgress) {
      void applyProgressUpdate(pendingProgress);
      pendingProgress = null;
    }
  }, 1000);
};

const subscribeIngestionProgress = () => {
  if (unsubscribeIngestionProgress) return;
  unsubscribeIngestionProgress = wsBus.subscribe("knowledge.ingestion.job", (payload) => {
    if (!payload) return;
    scheduleProgressUpdate(payload as IngestionProgress);
  });
};

const getProvenancePages = (item: IngestionChunkRecord): ProvenancePage[] => {
  const prov = (item?.metadata?.provenance || {}) as ChunkProvenance;
  const pages = Array.isArray(prov.pages) ? prov.pages : [];
  return pages
    .map((p) => ({
      page_number: Number((p as any).page_number),
      regions: Array.isArray((p as any).regions) ? ((p as any).regions as any[]) : [],
    }))
    .filter((p) => Number.isFinite(p.page_number) && p.page_number > 0);
};

const previewPages = computed(() => {
  const item = previewChunk.value;
  if (!item) return [];
  return getProvenancePages(item).sort((a, b) => a.page_number - b.page_number);
});

const currentRegions = computed<ProvenanceRegion[]>(() => {
  const item = previewChunk.value;
  if (!item || !previewPageNumber.value) return [];
  const pages = getProvenancePages(item);
  const page = pages.find((p) => p.page_number === previewPageNumber.value);
  const regions = page?.regions || [];
  return regions
    .map((r) => ({
      x1: Number((r as any).x1),
      y1: Number((r as any).y1),
      x2: Number((r as any).x2),
      y2: Number((r as any).y2),
      confidence: Number((r as any).confidence),
    }))
    .filter((r) => [r.x1, r.y1, r.x2, r.y2].every((v) => Number.isFinite(v)));
});

const cleanupPreviewImageUrl = () => {
  if (previewImageUrl.value && process.client) {
    try {
      URL.revokeObjectURL(previewImageUrl.value);
    } catch {
      // ignore
    }
  }
  previewImageUrl.value = null;
};

watch(
  () => previewOpen.value,
  (open) => {
    if (!open) {
      previewChunk.value = null;
      previewPageNumber.value = null;
      previewError.value = null;
      cleanupPreviewImageUrl();
    }
  },
);

const copy = async (text: string) => {
  if (!process.client) return;
  try {
    await navigator.clipboard.writeText(text);
    toast.add({ title: "已复制", description: truncate(text, 80) });
  } catch {
    // ignore
  }
};

const closeDeleteModal = () => {
  if (process.client) {
    (document.activeElement as HTMLElement | null)?.blur?.();
  }
  deleteOpen.value = false;
};

const deleteJob = async () => {
  if (!spaceId.value || !jobId.value) return;
  deleting.value = true;
  deleteError.value = null;
  try {
    await api.deleteIngestionJob(spaceId.value, jobId.value);
    toast.add({ title: "已删除", description: `Job ${shortId(jobId.value, 12)}` });
    deleteOpen.value = false;
    navigateTo(`/knowledge-spaces/${encodeURIComponent(spaceId.value)}/ingestions`);
  } catch (e: any) {
    deleteError.value = String(e?.message || "删除失败");
  } finally {
    deleting.value = false;
  }
};

const closeEditModal = () => {
  if (process.client) {
    (document.activeElement as HTMLElement | null)?.blur?.();
  }
  editOpen.value = false;
};

const closePreviewModal = () => {
  if (process.client) {
    (document.activeElement as HTMLElement | null)?.blur?.();
  }
  previewOpen.value = false;
};

const openFeedback = (chunkId?: string) => {
  const chunks = chunkId ? encodeURIComponent(chunkId) : "";
  const qs = `spaceId=${encodeURIComponent(spaceId.value)}${chunks ? `&chunks=${chunks}` : ""}`;
  navigateTo(`/knowledge-spaces/feedback?${qs}`);
};

const openEdit = (item: IngestionChunkRecord) => {
  editChunk.value = item;
  editContent.value = item.content || "";
  editBy.value = "";
  editReason.value = "";
  editError.value = null;
  editOpen.value = true;
};

const openInNewTab = (url: string) => {
  if (!process.client) return;
  try {
    window.open(url, "_blank", "noreferrer");
  } catch {
    // ignore
  }
};

const openPreview = async (item: IngestionChunkRecord) => {
  const pages = getProvenancePages(item);
  if (!pages.length) {
    toast.add({ title: "暂无定位信息", description: "该 Chunk 没有 page+bbox provenance（可能未启用 OCR Plan B）" });
    return;
  }
  previewChunk.value = item;
  previewPageNumber.value = pages[0].page_number;
  previewOpen.value = true;
  await fetchPreviewImage();
};

const fetchPreviewImage = async () => {
  if (!previewChunk.value || !previewOpen.value || !previewPageNumber.value) return;
  previewLoading.value = true;
  previewError.value = null;
  cleanupPreviewImageUrl();
  try {
    const blob = await api.getIngestionPageImageBlob(spaceId.value, jobId.value, previewPageNumber.value);
    previewImageUrl.value = URL.createObjectURL(blob);
  } catch (e: any) {
    previewError.value = String(e?.message || "加载页图片失败");
  } finally {
    previewLoading.value = false;
  }
};

watch(
  () => previewPageNumber.value,
  () => {
    if (!previewOpen.value) return;
    fetchPreviewImage();
  },
);

const saveEdit = async () => {
  if (!editChunk.value) return;
  const content = editContent.value.trim();
  if (!content) {
    editError.value = "内容不能为空";
    return;
  }
  editing.value = true;
  editError.value = null;
  try {
    await api.updateIngestionChunk(spaceId.value, jobId.value, editChunk.value.chunkId, {
      content,
      editedBy: editBy.value.trim() || undefined,
      editReason: editReason.value.trim() || undefined,
    });
    // 乐观更新当前页内容（不强制翻页）
    if (result.value?.items?.length) {
      const idx = result.value.items.findIndex((x) => x.chunkId === editChunk.value?.chunkId);
      if (idx >= 0) {
        result.value.items[idx] = { ...result.value.items[idx], content };
      }
    }
    toast.add({ title: "已保存", description: "已对该 Chunk 重新写入索引" });
    editOpen.value = false;
  } catch (e: any) {
    editError.value = String(e?.message || "保存失败");
  } finally {
    editing.value = false;
  }
};
</script>

<template>
  <div class="p-6 space-y-4">
    <div class="flex items-start justify-between gap-3">
      <div class="flex items-start gap-3 min-w-0">
        <UButton color="neutral" variant="ghost" icon="i-heroicons-arrow-left" @click="goBackToSpace">返回空间</UButton>
        <div class="min-w-0">
        <div class="text-lg font-semibold">切块预览</div>
        <div class="text-sm text-[var(--text-secondary)] truncate">
          {{ displaySpaceName }} · 任务 {{ shortId(jobId, 10) }}
          <UButton size="xs" color="neutral" variant="soft" icon="i-heroicons-clipboard" class="ml-2" @click="copy(jobId)">
            复制任务ID
          </UButton>
        </div>
        <div class="text-xs text-[var(--text-tertiary)] mt-1" v-if="format || sourceLink">
          <span v-if="format">格式：{{ format }}</span>
          <span v-if="format && sourceLink"> · </span>
          <span v-if="sourceLink" class="inline-flex items-center gap-2 min-w-0">
            <span class="shrink-0">原文件：</span>
            <span class="truncate">{{ truncate(displayAssetName, 72) }}</span>
            <UButton
              size="xs"
              color="neutral"
              variant="soft"
              icon="i-heroicons-arrow-top-right-on-square"
              as="a"
              :href="sourceLink"
              target="_blank"
              rel="noreferrer"
            >
              打开
            </UButton>
            <UButton size="xs" color="neutral" variant="soft" icon="i-heroicons-clipboard" @click="copy(sourceLink)">
              复制链接
            </UButton>
          </span>
        </div>
        </div>
      </div>
      <div class="flex items-center gap-2">
        <UButton color="neutral" variant="soft" size="sm" icon="i-heroicons-chat-bubble-left-right" @click="openFeedback()">
          反馈
        </UButton>
        <UButton color="neutral" variant="soft" size="sm" icon="i-heroicons-arrow-path" :loading="loading" @click="fetchChunks">
          刷新
        </UButton>
        <UButton color="error" variant="soft" size="sm" icon="i-heroicons-trash" type="button" :disabled="deleting" @click.stop="deleteOpen = true">
          删除入库
        </UButton>
      </div>
    </div>

    <UCard :ui="{ body: 'p-4 sm:p-5 space-y-3' }">
      <div class="flex flex-wrap items-center gap-2">
        <div class="text-sm font-medium">任务状态</div>
        <UBadge :color="statusColor" variant="soft">{{ statusLabel }}</UBadge>
        <UBadge color="neutral" variant="soft">{{ wsRealtimeLabel }}</UBadge>
        <span class="text-xs text-[var(--text-tertiary)]">{{ wsRealtimeDesc }}</span>
      </div>
      <div v-if="showProgress" class="space-y-2">
        <div class="flex items-center gap-2 text-xs text-[var(--text-secondary)]">
          <span>阶段：{{ stageLabel || "处理中" }}</span>
          <span>·</span>
          <span>进度：{{ displayProgress.toFixed(0) }}%</span>
        </div>
        <UProgress :value="displayProgress" color="primary" />
      </div>
    </UCard>

    <UAlert
      v-if="segmentStrategyHint"
      color="neutral"
      variant="soft"
      title="分段策略"
      :description="segmentStrategyHint"
    />

    <UAlert v-if="error" color="error" variant="soft" title="加载失败" :description="error" />

    <div v-else class="flex flex-col gap-3">
      <div class="flex flex-wrap items-center gap-2">
        <UInput v-model="keyword" class="w-64" placeholder="过滤（chunkId/类型/内容）" />
        <USelect
          v-model="kindFilter"
          :items="[
            { label: '默认（仅正文 chunk）', value: 'auto' },
            { label: '全部类型', value: 'all' },
            ...allKinds.map(k => ({ label: `仅 ${k}`, value: k })),
          ]"
          option-attribute="label"
          value-attribute="value"
          class="w-40"
        />
        <USelect
          v-model="pageSize"
          :items="[
            { label: '50', value: 50 },
            { label: '100', value: 100 },
            { label: '200', value: 200 },
          ]"
          option-attribute="label"
          value-attribute="value"
          class="w-28"
        />
        <div class="text-xs text-[var(--text-secondary)]">
          {{ filteredCountHint }} · 第 {{ page }} / {{ totalPages }} 页
        </div>
        <div class="ml-auto flex items-center gap-2">
          <UButton size="xs" color="neutral" variant="soft" :disabled="page <= 1" @click="page -= 1">上一页</UButton>
          <UButton size="xs" color="neutral" variant="soft" :disabled="page >= totalPages" @click="page += 1">下一页</UButton>
        </div>
      </div>

      <UAlert
        v-if="!loading && (!result || !result.items.length)"
        color="neutral"
        variant="soft"
        title="暂无切块"
        description="如果任务刚完成，产物可能仍在写入；稍后刷新再试。"
      />

      <div v-else class="space-y-2">
        <UCard
          v-for="item in filteredItems"
          :key="item.chunkId"
          class="overflow-hidden"
          :ui="{ body: 'p-4 space-y-2' }"
        >
          <div class="flex items-start justify-between gap-2">
            <div class="min-w-0">
              <div class="text-sm font-medium truncate">
                {{ item.kind || 'content' }} · {{ shortId(item.chunkId, 10) }}
              </div>
              <div class="text-xs text-[var(--text-secondary)]">
                <span>置信度：{{ Number(item.confidence || 0).toFixed(2) }} · Masked：{{ item.masked ? "yes" : "no" }}</span>
                <UBadge v-if="isSyntheticChunkContent(item.content)" class="ml-2" color="warning" variant="soft">
                  占位内容（未抽取正文）
                </UBadge>
              </div>
            </div>
            <div class="flex items-center gap-2">
              <UButton size="xs" color="neutral" variant="soft" icon="i-heroicons-clipboard" @click="copy(item.chunkId)">
                复制块ID
              </UButton>
              <UButton
                size="xs"
                color="neutral"
                variant="soft"
                icon="i-heroicons-square-3-stack-3d"
                :disabled="!item.metadata?.provenance?.pages?.length"
                @click="openPreview(item)"
              >
                叠框预览
              </UButton>
              <UButton size="xs" color="neutral" variant="soft" icon="i-heroicons-pencil-square" @click="openEdit(item)">
                编辑
              </UButton>
              <UButton size="xs" color="primary" variant="soft" @click="openFeedback(item.chunkId)">
                反馈这个 Chunk
              </UButton>
            </div>
          </div>

          <div v-if="chunkSourceURL(item) && chunkSourceURL(item) !== sourceLink" class="flex flex-wrap items-center gap-2 text-xs">
            <UBadge color="neutral" variant="soft">原文件</UBadge>
            <span class="truncate max-w-[50ch]">{{ truncate(displayAssetName, 72) }}</span>
            <UButton
              size="xs"
              color="neutral"
              variant="soft"
              icon="i-heroicons-arrow-top-right-on-square"
              @click="openInNewTab(chunkSourceURL(item))"
            >
              打开
            </UButton>
            <UButton size="xs" color="neutral" variant="soft" icon="i-heroicons-clipboard" @click="copy(chunkSourceURL(item))">
              复制链接
            </UButton>
          </div>

          <div v-if="item.metadata && Object.keys(item.metadata).length" class="flex flex-wrap gap-2 text-xs">
            <UBadge
              v-for="[k, v] in visibleMetadataEntries(item)"
              :key="k"
              color="neutral"
              variant="soft"
            >
              {{ k }}={{ typeof v === "string" ? truncate(String(v), 60) : String(v) }}
            </UBadge>
          </div>

          <div class="text-xs whitespace-pre-wrap break-words bg-gray-50 rounded-lg p-3 border border-gray-200">
            <div v-if="extractHTTPURLs(chunkContentForUI(item)).length" class="flex flex-wrap items-center gap-2 mb-2">
              <UBadge color="neutral" variant="soft">链接</UBadge>
              <template v-for="(u, ui) in extractHTTPURLs(chunkContentForUI(item))" :key="ui">
                <span class="truncate max-w-[44ch] text-xs">{{ truncate(formatURLLabel(u), 60) }}</span>
                <UButton size="xs" color="neutral" variant="soft" icon="i-heroicons-arrow-top-right-on-square" @click="openInNewTab(u)">
                  打开
                </UButton>
                <UButton size="xs" color="neutral" variant="soft" icon="i-heroicons-clipboard" @click="copy(u)">
                  复制
                </UButton>
              </template>
            </div>
            {{ replaceHTTPURLs(chunkContentForUI(item)) }}
          </div>
        </UCard>
      </div>
    </div>
  </div>

  <UModal
    v-model:open="editOpen"
    title="编辑 Chunk"
    description="修正内容后将用于检索与引用"
    :ui="{ width: 'max-w-3xl w-full', body: 'p-4 sm:p-5', footer: 'justify-end' }"
    :close="{ onClick: closeEditModal }"
    prevent-close
  >
    <template #body>
      <div class="space-y-3">
        <div class="text-xs text-[var(--text-secondary)] truncate">
          {{ editChunk?.kind || "content" }} · {{ shortId(editChunk?.chunkId || '', 12) }}
          <UButton size="xs" color="neutral" variant="soft" icon="i-heroicons-clipboard" class="ml-2" @click="copy(editChunk?.chunkId || '')">
            复制块ID
          </UButton>
        </div>

        <UAlert v-if="editError" color="error" variant="soft" title="保存失败" :description="editError" />

        <UFormField label="内容">
          <UTextarea v-model="editContent" :rows="12" placeholder="请输入修正后的内容" />
        </UFormField>

        <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
          <UFormField label="编辑人（可选）">
            <UInput v-model="editBy" placeholder="例如：michael" />
          </UFormField>
          <UFormField label="原因（可选）">
            <UInput v-model="editReason" placeholder="例如：OCR 误识别/分段不合理" />
          </UFormField>
        </div>
      </div>
    </template>
    <template #footer>
      <div class="flex items-center justify-end gap-2">
        <UButton color="neutral" variant="subtle" type="button" :disabled="editing" @click="closeEditModal">取消</UButton>
        <UButton color="primary" type="button" :loading="editing" @click="saveEdit">保存并重建索引</UButton>
      </div>
    </template>
  </UModal>

  <UModal
    v-model:open="deleteOpen"
    title="删除入库任务"
    description="将删除该 Job 的切块、向量记录与本地产物（如存在）。此操作不可撤销。"
    :ui="{ width: 'max-w-xl w-full', body: 'p-4 sm:p-5', footer: 'justify-end' }"
    :close="{ onClick: closeDeleteModal }"
    prevent-close
  >
    <template #body>
      <div class="space-y-3">
        <div class="text-sm">
          确认删除 Job：<span class="font-mono">{{ jobId }}</span>
        </div>
        <UAlert v-if="deleteError" color="error" variant="soft" title="删除失败" :description="deleteError" />
      </div>
    </template>
    <template #footer>
      <div class="flex items-center justify-end gap-2">
        <UButton color="neutral" variant="subtle" type="button" :disabled="deleting" @click="closeDeleteModal">取消</UButton>
        <UButton color="error" type="button" :loading="deleting" @click="deleteJob">确认删除</UButton>
      </div>
    </template>
  </UModal>

  <UModal
    v-model:open="previewOpen"
    title="页预览（bbox 叠框）"
    description="用于核对 Chunk 在原文中的位置"
    :ui="{ width: 'max-w-3xl w-full', body: 'p-4 sm:p-5', footer: 'justify-end' }"
    :close="{ onClick: closePreviewModal }"
    prevent-close
  >
    <template #body>
      <div class="space-y-3">
        <div class="text-xs text-[var(--text-secondary)] truncate">
          {{ previewChunk?.kind || "content" }} · {{ shortId(previewChunk?.chunkId || '', 12) }}
        </div>

        <UAlert v-if="previewError" color="error" variant="soft" title="加载失败" :description="previewError" />

        <div class="flex items-center gap-2">
          <USelect
            v-model="previewPageNumber"
            :items="previewPages.map(p => ({ label: `第 ${p.page_number} 页`, value: p.page_number }))"
            option-attribute="label"
            value-attribute="value"
            class="w-40"
          />
          <div class="text-xs text-[var(--text-secondary)]">
            Regions：{{ currentRegions.length }}
          </div>
          <div class="ml-auto">
            <UButton size="xs" color="neutral" variant="soft" :loading="previewLoading" @click="fetchPreviewImage">
              刷新图片
            </UButton>
          </div>
        </div>

        <div class="rounded-lg border border-gray-200 bg-gray-50 p-3 overflow-auto max-h-[70dvh]">
          <div v-if="previewLoading" class="text-sm text-[var(--text-secondary)]">加载中…</div>
          <div v-else-if="!previewImageUrl" class="text-sm text-[var(--text-secondary)]">暂无图片</div>
          <div v-else class="inline-block relative">
            <img :src="previewImageUrl" class="max-w-full h-auto block rounded-md" />
            <div
              v-for="(r, idx) in currentRegions"
              :key="idx"
              class="absolute border-2 border-amber-500 bg-amber-200/20 pointer-events-none"
              :style="{
                left: `${Math.max(0, Math.min(1, r.x1)) * 100}%`,
                top: `${Math.max(0, Math.min(1, r.y1)) * 100}%`,
                width: `${Math.max(0, Math.min(1, r.x2) - Math.min(1, Math.max(0, r.x1))) * 100}%`,
                height: `${Math.max(0, Math.min(1, r.y2) - Math.min(1, Math.max(0, r.y1))) * 100}%`,
              }"
            />
          </div>
        </div>
      </div>
    </template>

    <template #footer>
      <div class="flex items-center justify-end gap-2">
        <UButton color="neutral" variant="subtle" type="button" @click="closePreviewModal">关闭</UButton>
      </div>
    </template>
  </UModal>
</template>
