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
        <div class="space-y-3">
          <MediaFilterBar
            v-model="filters"
            :view-mode="viewMode"
            @update:viewMode="viewMode = $event"
            @apply="applyFilters"
            @reset="resetFilters"
          />
          <div class="flex flex-wrap items-center justify-between gap-3 border-t border-[var(--border-color)] pt-3">
            <div class="flex flex-wrap items-center gap-2 text-sm text-[var(--text-secondary)]">
              <UCheckbox
                :model-value="allCurrentPageSelected"
                :indeterminate="someCurrentPageSelected && !allCurrentPageSelected"
                label="全选当前页"
                @update:model-value="toggleSelectCurrentPage(Boolean($event))"
              />
              <span>已选 {{ selectedCount }} 个</span>
            </div>
            <div class="flex flex-wrap items-center gap-2">
              <UButton
                size="sm"
                variant="outline"
                color="neutral"
                :disabled="selectedCount === 0 || deletingSelected"
                @click="clearSelection"
              >
                清空选择
              </UButton>
              <UButton
                size="sm"
                color="error"
                icon="i-heroicons-trash"
                :disabled="selectedCount === 0"
                :loading="deletingSelected"
                @click="deleteSelected"
              >
                删除选中
              </UButton>
            </div>
          </div>
        </div>
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
        <MediaGrid
          v-if="viewMode === 'grid'"
          :items="rows"
          :selected-uuids="selectedUUIDs"
          @select="openDetail"
          @toggle-selected="toggleSelected"
        />
        <MediaTable
          v-else
          :items="rows"
          :loading="loading"
          :selected-uuids="selectedUUIDs"
          @select="openDetail"
          @copyLink="copyDownloadLink"
          @toggle-selected="toggleSelected"
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
import { useConfirm } from "~/composables/useConfirm";
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
const { confirm } = useConfirm();
const apiClient = useApiClient();
const media = useMediaAssetService();
const localePath = useLocalePath();

const loading = ref(false);
const error = ref<string | null>(null);
const viewMode = ref<"grid" | "table">("grid");
const showUpload = ref(false);
const deletingSelected = ref(false);
const selectedUUIDs = ref<string[]>([]);

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

const currentPageUUIDs = computed(() =>
  rows.value.map((item) => String(item.uuid || "").trim()).filter(Boolean)
);
const selectedSet = computed(() => new Set(selectedUUIDs.value));
const selectedCount = computed(() => selectedUUIDs.value.length);
const allCurrentPageSelected = computed(() => {
  const ids = currentPageUUIDs.value;
  return ids.length > 0 && ids.every((id) => selectedSet.value.has(id));
});
const someCurrentPageSelected = computed(() =>
  currentPageUUIDs.value.some((id) => selectedSet.value.has(id))
);

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

function toggleSelected(uuid: string, selected: boolean) {
  const id = String(uuid || "").trim();
  if (!id) return;
  const next = new Set(selectedUUIDs.value);
  if (selected) {
    next.add(id);
  } else {
    next.delete(id);
  }
  selectedUUIDs.value = Array.from(next);
}

function toggleSelectCurrentPage(selected: boolean) {
  const next = new Set(selectedUUIDs.value);
  for (const id of currentPageUUIDs.value) {
    if (selected) {
      next.add(id);
    } else {
      next.delete(id);
    }
  }
  selectedUUIDs.value = Array.from(next);
}

function clearSelection() {
  selectedUUIDs.value = [];
}

async function deleteSelected() {
  const ids = selectedUUIDs.value.map((id) => String(id || "").trim()).filter(Boolean);
  if (ids.length === 0) return;
  const ok = await confirm({
    title: "删除媒体资产",
    description: `确认删除选中的 ${ids.length} 个媒体资产？该操作会进入回收站，可通过回收站筛选查看。`,
    confirmLabel: "删除",
    confirmColor: "red",
    cancelLabel: "取消",
  });
  if (!ok) return;
  deletingSelected.value = true;
  let success = 0;
  const failures: string[] = [];
  try {
    for (const id of ids) {
      try {
        await media.deleteAsset(id);
        success += 1;
      } catch (e: any) {
        failures.push(`${id}: ${String(e?.message || "删除失败")}`);
      }
    }
    selectedUUIDs.value = [];
    await fetchList();
    if (failures.length === 0) {
      toast.add({ title: "删除完成", description: `已删除 ${success} 个媒体资产` });
    } else {
      toast.add({
        title: "部分删除失败",
        description: `成功 ${success} 个，失败 ${failures.length} 个`,
        color: "red",
      });
      error.value = failures.slice(0, 5).join("\n");
    }
  } finally {
    deletingSelected.value = false;
  }
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
    const visible = new Set(items.map((item) => String(item.uuid || "").trim()).filter(Boolean));
    selectedUUIDs.value = selectedUUIDs.value.filter((id) => visible.has(id));
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
