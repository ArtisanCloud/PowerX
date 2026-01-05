<template>
  <div class="space-y-6 p-6">
    <div class="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
      <div class="flex items-center gap-3">
        <UButton
          icon="i-heroicons-arrow-left"
          size="sm"
          variant="ghost"
          @click="backToList"
        >
          返回
        </UButton>
        <div>
          <h1 class="text-xl font-semibold text-[var(--text-primary)]">
            {{ asset?.name || "媒体详情" }}
          </h1>
          <p class="text-xs text-[var(--text-tertiary)]">
            {{ uuid || "-" }}
          </p>
        </div>
      </div>

      <div class="flex flex-wrap items-center gap-2">
        <UButton
          icon="i-lucide-link"
          size="sm"
          variant="outline"
          :disabled="!uuid"
          @click="copyResourceLink"
        >
          复制资源链接
        </UButton>
        <UButton
          icon="i-lucide-download"
          size="sm"
          variant="outline"
          :disabled="!asset"
          :loading="downloading"
          @click="downloadResource"
        >
          下载
        </UButton>
        <UButton
          icon="i-lucide-trash-2"
          size="sm"
          color="red"
          variant="soft"
          :disabled="!asset"
          :loading="deleting"
          @click="deleteAsset"
        >
          删除
        </UButton>
      </div>
    </div>

    <div v-if="error" class="mb-4">
      <UAlert
        color="red"
        icon="i-heroicons-exclamation-triangle"
        :title="error"
      />
    </div>

    <div v-if="loading">
      <UCard>
        <div class="space-y-3">
          <USkeleton class="h-6 w-2/3" />
          <USkeleton class="h-48 w-full" />
          <USkeleton class="h-6 w-1/2" />
        </div>
      </UCard>
    </div>

    <div v-else-if="asset" class="grid grid-cols-1 gap-6 lg:grid-cols-3">
      <div class="space-y-6 lg:col-span-2">
        <UCard>
          <template #header>
            <div class="flex items-center justify-between">
              <div class="text-sm font-medium text-[var(--text-primary)]">
                预览
              </div>
              <div class="flex items-center gap-2">
                <UBadge
                  :color="statusColor(asset.businessStatus)"
                  variant="soft"
                >
                  {{ asset.businessStatus || "-" }}
                </UBadge>
                <UBadge color="gray" variant="soft">
                  {{ asset.driver || "-" }}
                </UBadge>
              </div>
            </div>
          </template>

          <MediaPreview
            :asset="asset"
            :preview-url="previewUrl"
            :state="previewState"
            :kind="previewKind"
            @openExternal="openExternal"
          />
        </UCard>
      </div>

      <div class="space-y-6">
        <MediaAssetDetailPanel
          :asset="asset"
          v-model="form"
          :saving="saving"
          @save="save"
          @reset="resetForm"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useToast } from "#imports";
import { useApiClient } from "~/composables/api";

definePageMeta({
  title: "媒体详情",
  layout: "default",
});

type MediaAssetBusinessStatus =
  | "draft"
  | "under_review"
  | "published"
  | "archived"
  | string;

interface MediaAssetAdminView {
  uuid: string;
  tenant_uuid: string;
  name: string;
  description?: string;
  driver: string;
  folder?: string;
  objectKey: string;
  externalUrl?: string;
  sizeBytes?: number | null;
  mimeType?: string;
  ownerSubjectType?: string;
  ownerSubjectId?: string;
  tags?: string[];
  businessStatus: MediaAssetBusinessStatus;
  downloadUrl?: string;
  downloadExpiredAt?: string;
  createdAt: string;
  updatedAt: string;
  deleted?: boolean;
  metadata?: Record<string, any>;
}

const toast = useToast();
const apiClient = useApiClient();
const localePath = useLocalePath();
const route = useRoute();

const uuid = computed(() => String(route.params.uuid || "").trim());

const loading = ref(false);
const saving = ref(false);
const deleting = ref(false);
const downloading = ref(false);
const error = ref<string | null>(null);

const asset = ref<MediaAssetAdminView | null>(null);

const form = ref({
  name: "",
  description: "",
  businessStatus: "" as "" | MediaAssetBusinessStatus,
  tagsText: "",
});

const previewUrl = ref<string | null>(null);
const previewState = ref<"idle" | "loading" | "ready" | "error">("idle");

const isImage = computed(() => (asset.value?.mimeType || "").toLowerCase().startsWith("image/"));
const isVideo = computed(() => (asset.value?.mimeType || "").toLowerCase().startsWith("video/"));
const isAudio = computed(() => (asset.value?.mimeType || "").toLowerCase().startsWith("audio/"));

const previewKind = computed<"image" | "video" | "audio" | "unknown">(() => {
  if (isImage.value) return "image";
  if (isVideo.value) return "video";
  if (isAudio.value) return "audio";
  return "unknown";
});

function statusColor(status: MediaAssetBusinessStatus) {
  switch (String(status || "").toLowerCase()) {
    case "draft":
      return "gray";
    case "under_review":
      return "amber";
    case "published":
      return "green";
    case "archived":
      return "blue";
    default:
      return "gray";
  }
}

function formatTime(value: string) {
  if (!value) return "-";
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  return d.toLocaleString();
}

function normalizeTags(input: string): string[] {
  const parts = input
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean);
  return Array.from(new Set(parts));
}

function backToList() {
  navigateTo(localePath("/content/media"));
}

function resetForm() {
  const a = asset.value;
  form.value = {
    name: a?.name || "",
    description: a?.description || "",
    businessStatus: (a?.businessStatus || "") as any,
    tagsText: (a?.tags || []).join(", "),
  };
}

async function fetchDetail() {
  if (!uuid.value) return;

  loading.value = true;
  error.value = null;
  try {
    const resp = await apiClient.get(`/admin/media/assets/${encodeURIComponent(uuid.value)}`);
    const data = apiClient.unwrap<MediaAssetAdminView>(resp);
    asset.value = data;
    resetForm();
    await loadPreview();
  } catch (e: any) {
    const msg = String(e?.message || "加载失败");
    error.value = msg;
  } finally {
    loading.value = false;
  }
}

function revokePreviewUrl() {
  if (previewUrl.value && process.client) {
    URL.revokeObjectURL(previewUrl.value);
  }
  previewUrl.value = null;
  previewState.value = "idle";
}

async function fetchResourceBlob(disposition: "inline" | "attachment" = "inline") {
  const id = uuid.value;
  if (!id) throw new Error("missing uuid");
  const url = `/admin/media/assets/${encodeURIComponent(id)}/resource?disposition=${encodeURIComponent(disposition)}`;
  // $fetch 支持 responseType，但 ApiRequestConfig 未声明，这里用 any 透传即可
  return apiClient.request<Blob>("GET", url, undefined, {
    responseType: "blob",
    headers: { Accept: "*/*" },
    useGlobalLoading: false,
  } as any);
}

async function loadPreview() {
  revokePreviewUrl();
  if (!asset.value || asset.value.externalUrl) return;

  const mt = (asset.value.mimeType || "").toLowerCase();
  const canPreview = mt.startsWith("image/") || mt.startsWith("video/") || mt.startsWith("audio/");
  if (!canPreview) return;

  previewState.value = "loading";
  try {
    const blob = await fetchResourceBlob("inline");
    if (!blob) throw new Error("empty blob");
    previewUrl.value = URL.createObjectURL(blob);
    previewState.value = "ready";
  } catch {
    previewState.value = "error";
  }
}

function copyResourceLink() {
  if (!process.client || !uuid.value) return;
  const url = `${location.origin}/api/admin/media/assets/${encodeURIComponent(uuid.value)}/resource`;
  navigator.clipboard
    .writeText(url)
    .then(() => toast.add({ title: "已复制资源链接", description: url }))
    .catch(() => toast.add({ title: "复制失败", color: "red" }));
}

function openExternal() {
  if (!asset.value?.externalUrl || !process.client) return;
  window.open(asset.value.externalUrl, "_blank", "noopener,noreferrer");
}

function fileNameForDownload() {
  const a = asset.value;
  const base = (a?.name || a?.uuid || "media").trim() || "media";
  const mt = (a?.mimeType || "").toLowerCase();
  const ext =
    mt === "image/png"
      ? ".png"
      : mt === "image/jpeg"
        ? ".jpg"
        : mt === "image/webp"
          ? ".webp"
          : mt === "image/gif"
            ? ".gif"
            : mt.startsWith("video/")
              ? ".mp4"
              : mt.startsWith("audio/")
                ? ".mp3"
                : "";
  return base.endsWith(ext) || !ext ? base : `${base}${ext}`;
}

async function downloadResource() {
  if (!asset.value) return;
  if (asset.value.externalUrl) {
    openExternal();
    return;
  }

  downloading.value = true;
  try {
    const blob = await fetchResourceBlob("attachment");
    const { saveAs } = await import("file-saver");
    saveAs(blob, fileNameForDownload());
    toast.add({ title: "下载已开始" });
  } catch (e: any) {
    toast.add({ title: "下载失败", description: String(e?.message || ""), color: "red" });
  } finally {
    downloading.value = false;
  }
}

async function save() {
  if (!uuid.value || !asset.value) return;

  const name = form.value.name.trim();
  if (!name) {
    toast.add({ title: "名称不能为空", color: "red" });
    return;
  }

  saving.value = true;
  try {
    const payload: any = {
      name,
      description: form.value.description,
      tags: normalizeTags(form.value.tagsText),
    };
    if (form.value.businessStatus) {
      payload.businessStatus = form.value.businessStatus;
    }
    const resp = await apiClient.patch(`/admin/media/assets/${encodeURIComponent(uuid.value)}`, payload);
    const updated = apiClient.unwrap<MediaAssetAdminView>(resp);
    asset.value = updated;
    resetForm();
    toast.add({ title: "保存成功" });
  } catch (e: any) {
    toast.add({ title: "保存失败", description: String(e?.message || ""), color: "red" });
  } finally {
    saving.value = false;
  }
}

async function deleteAsset() {
  if (!uuid.value || !asset.value) return;
  if (!process.client) return;

  const ok = window.confirm("确认删除该媒体资产？（软删除）");
  if (!ok) return;

  deleting.value = true;
  try {
    await apiClient.delete(`/admin/media/assets/${encodeURIComponent(uuid.value)}`);
    toast.add({ title: "已删除（软删除）" });
    navigateTo(localePath("/content/media?onlyDeleted=1"));
  } catch (e: any) {
    toast.add({ title: "删除失败", description: String(e?.message || ""), color: "red" });
  } finally {
    deleting.value = false;
  }
}

watch(
  () => uuid.value,
  async (next, prev) => {
    if (!next || next === prev) return;
    revokePreviewUrl();
    await fetchDetail();
  },
  { immediate: true }
);

onBeforeUnmount(() => {
  revokePreviewUrl();
});
</script>
