<template>
  <UTable
    :columns="columns"
    :data="items"
    :loading="loading"
    row-key="uuid"
    :ui="{ divide: 'divide-y divide-[var(--border-color)]' }"
  />
</template>

<script setup lang="ts">
import { h, resolveComponent } from "vue";
import type { MediaAssetAdminView } from "~/composables/api/services/mediaAssetService";
import MediaThumbnail from "~/components/content/media/MediaThumbnail.vue";

const props = defineProps<{
  items: MediaAssetAdminView[];
  loading?: boolean;
  selectedUuids?: string[];
}>();

const emit = defineEmits<{
  (e: "select", uuid: string): void;
  (e: "copyLink", uuid: string): void;
  (e: "toggleSelected", uuid: string, selected: boolean): void;
}>();

const columns = [
  {
    accessorKey: "selected",
    header: "",
    cell: ({ row }: any) =>
      h(resolveComponent("UCheckbox"), {
        modelValue: isSelected(row.original.uuid),
        "aria-label": `选择 ${row.original.name || row.original.uuid}`,
        onClick: (event: Event) => event.stopPropagation(),
        "onUpdate:modelValue": (value: boolean) => emit("toggleSelected", row.original.uuid, Boolean(value)),
      }),
  },
  {
    accessorKey: "name",
    header: "名称/类型",
    cell: ({ row }: any) =>
      h("div", { class: "flex items-center gap-3" }, [
        h(MediaThumbnail, {
          asset: row.original as MediaAssetAdminView,
          containerClass: "h-10 w-10 shrink-0",
        }),
        h("div", { class: "min-w-0 space-y-1" }, [
          h(
            "div",
            { class: "truncate font-semibold text-sm text-[var(--text-primary)]" },
            row.original.name || "-"
          ),
          h(
            "div",
            { class: "text-xs text-[var(--text-secondary)]" },
            `${row.original.mimeType || "unknown"} · ${
              row.original.sizeBytes != null ? formatBytes(row.original.sizeBytes) : "-"
            }`
          ),
        ]),
      ]),
  },
  {
    accessorKey: "driver",
    header: "驱动",
    cell: ({ row }: any) =>
      h(
        "div",
        { class: "text-sm text-[var(--text-secondary)]" },
        row.original.driver || "-"
      ),
  },
  {
    accessorKey: "businessStatus",
    header: "状态",
    cell: ({ row }: any) =>
      h(
        resolveComponent("UBadge"),
        { color: statusColor(row.original.businessStatus), variant: "soft" },
        () => row.original.businessStatus || "-"
      ),
  },
  {
    accessorKey: "updatedAt",
    header: "更新时间",
    cell: ({ row }: any) =>
      h(
        "div",
        { class: "text-sm text-[var(--text-secondary)]" },
        formatTime(row.original.updatedAt)
      ),
  },
  {
    accessorKey: "actions",
    header: "操作",
    cell: ({ row }: any) =>
      h("div", { class: "flex items-center gap-2" }, [
        h(
          resolveComponent("UButton"),
          { size: "xs", variant: "outline", onClick: () => emit("select", row.original.uuid) },
          () => "详情"
        ),
        h(
          resolveComponent("UButton"),
          { size: "xs", variant: "ghost", onClick: () => emit("copyLink", row.original.uuid) },
          () => "复制下载链接"
        ),
      ]),
  },
];

function statusColor(status: string) {
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

const loading = computed(() => !!props.loading);
const selectedSet = computed(() => new Set((props.selectedUuids || []).map((id) => String(id || "").trim()).filter(Boolean)));

function isSelected(uuid: string) {
  return selectedSet.value.has(String(uuid || "").trim());
}
</script>
