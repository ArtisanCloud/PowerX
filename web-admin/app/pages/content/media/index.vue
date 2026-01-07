<template>
  <div class="space-y-6 p-6">
    <div class="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
      <div>
        <h1 class="text-2xl font-bold text-[var(--text-primary)]">媒体库</h1>
        <p class="text-sm text-[var(--text-secondary)]">
          管理图片、视频等媒体文件（租户隔离）
        </p>
      </div>
      <div class="flex items-center gap-2">
        <UButton
          icon="i-heroicons-arrow-path"
          size="sm"
          variant="outline"
          :loading="loading"
          @click="fetchList"
        >
          刷新
        </UButton>
        <UButton icon="i-lucide-upload" size="sm" @click="showUpload = true">
          上传
        </UButton>
      </div>
    </div>

    <UCard>
      <template #header>
        <MediaFilterBar
          v-model="filters"
          :view-mode="viewMode"
          @update:viewMode="viewMode = $event"
          @apply="applyFilters"
          @reset="resetFilters"
        />
      </template>

      <div v-if="error" class="mb-4">
        <UAlert color="red" icon="i-heroicons-exclamation-triangle" :title="error" />
      </div>

      <div v-if="!loading && rows.length === 0" class="py-10 text-center">
        <div class="text-sm text-[var(--text-secondary)]">暂无媒体资产</div>
        <div class="mt-2 text-xs text-[var(--text-tertiary)]">
          可通过后续“预签名上传/外链入库”创建资产
        </div>
      </div>

      <div v-else>
        <MediaGrid v-if="viewMode === 'grid'" :items="rows" @select="openDetail" />
        <MediaTable
          v-else
          :items="rows"
          :loading="loading"
          @select="openDetail"
          @copyLink="copyDownloadLink"
        />
      </div>

      <template #footer>
        <div class="flex items-center justify-between">
          <div class="text-xs text-[var(--text-secondary)]">
            共 {{ pagination.total }} 条 · 第 {{ pagination.page }} 页
          </div>
          <UPagination
            v-model:page="pagination.page"
            :total="pagination.total"
            :items-per-page="pagination.pageSize"
            :max="7"
            @update:page="handlePageChange"
          />
        </div>
      </template>
    </UCard>

    <MediaUploadDrawer v-model="showUpload" @done="handleUploadDone" />
  </div>
</template>

<script setup lang="ts">
import { useToast } from "#imports";
import { useApiClient } from "~/composables/api";
import { useMediaAssetService } from "~/composables/api/services/mediaAssetService";
import type { MediaAssetAdminView } from "~/composables/api/services/mediaAssetService";
import MediaFilterBar from "~/components/content/media/MediaFilterBar.vue";
import MediaGrid from "~/components/content/media/MediaGrid.vue";
import MediaTable from "~/components/content/media/MediaTable.vue";
import MediaUploadDrawer from "~/components/content/media/MediaUploadDrawer.vue";

definePageMeta({
  title: "媒体库",
  layout: "default",
});

type MediaAssetBusinessStatus =
  | "draft"
  | "under_review"
  | "published"
  | "archived"
  | string;

type MediaFilterState = {
  keyword: string;
  businessStatus: "" | MediaAssetBusinessStatus;
  driver: "" | "local" | "s3" | string;
  tagsText: string;
  onlyDeleted: boolean;
};

const toast = useToast();
const apiClient = useApiClient();
const media = useMediaAssetService();
const localePath = useLocalePath();

const loading = ref(false);
const error = ref<string | null>(null);
const viewMode = ref<"grid" | "table">("grid");
const showUpload = ref(false);

const filters = ref<MediaFilterState>({
  keyword: "",
  businessStatus: "" as "" | MediaAssetBusinessStatus,
  driver: "" as "" | "local" | "s3" | string,
  tagsText: "",
  onlyDeleted: false,
});

const pagination = reactive({
  page: 1,
  pageSize: 20,
  total: 0,
});

const rows = ref<MediaAssetAdminView[]>([]);

function normalizeTags(input: string): string[] | undefined {
  const parts = input
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean);
  return parts.length ? Array.from(new Set(parts)) : undefined;
}

function formatBytes(value: number) {
  const size = Number(value);
  if (!Number.isFinite(size) || size <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const idx = Math.min(Math.floor(Math.log(size) / Math.log(1024)), units.length - 1);
  const num = size / Math.pow(1024, idx);
  return `${num.toFixed(num >= 10 || idx === 0 ? 0 : 1)} ${units[idx]}`;
}

function formatTime(value: string) {
  if (!value) return "-";
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  return d.toLocaleString();
}

function openDetail(uuid: string) {
  if (!uuid) return;
  navigateTo(localePath(`/content/media/${uuid}`));
}

async function copyDownloadLink(uuid: string) {
  if (!process.client || !uuid) return;
  try {
    const cfg = useRuntimeConfig();
    const upstreamOrigin = String(cfg.public?.upstreamOrigin || "").replace(/\/+$/, "");
    const presign = await media.presign(uuid, {
      action: "download",
      method: "GET",
      expiresInSeconds: 3600,
    });
    let url = String(presign?.url || "").trim();
    if (!url) throw new Error("预签名返回空链接");
    if (url.startsWith("/")) {
      const base = upstreamOrigin || location.origin;
      url = `${base}${url}`;
    }
    await navigator.clipboard.writeText(url);
    toast.add({ title: "已复制下载链接", description: url });
  } catch (e: any) {
    toast.add({ title: "复制失败", description: String(e?.message || ""), color: "red" });
  }
}

async function fetchList() {
  loading.value = true;
  error.value = null;
  try {
    const resp = await apiClient.get("/admin/media/assets", {
      params: {
        page: pagination.page,
        pageSize: pagination.pageSize,
        keyword: filters.value.keyword?.trim() || undefined,
        driver: filters.value.driver || undefined,
        businessStatus: filters.value.businessStatus ? [filters.value.businessStatus] : undefined,
        tags: normalizeTags(filters.value.tagsText),
        onlyDeleted: filters.value.onlyDeleted ? true : undefined,
      },
    });
    const { items, pagination: pageInfo } = apiClient.unwrapList<MediaAssetAdminView>(resp);
    rows.value = items;
    const total = Number(pageInfo?.total ?? 0);
    pagination.total = Number.isFinite(total) ? total : 0;
  } catch (e: any) {
    const msg = String(e?.message || "加载失败");
    error.value = msg;
  } finally {
    loading.value = false;
  }
}

function applyFilters() {
  pagination.page = 1;
  fetchList();
}

function resetFilters() {
  filters.value = {
    keyword: "",
    businessStatus: "",
    driver: "",
    tagsText: "",
    onlyDeleted: false,
  };
  pagination.page = 1;
  fetchList();
}

function handlePageChange() {
  fetchList();
}

function handleUploadDone(asset: MediaAssetAdminView) {
  // 先刷新列表，再可选跳转到详情
  fetchList();
  if (asset && asset.uuid) {
    toast.add({ title: "已创建资产", description: asset.name || asset.uuid });
  }
}

onMounted(() => {
  fetchList();
});
</script>
