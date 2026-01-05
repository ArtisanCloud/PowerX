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

const props = defineProps<{
  items: MediaAssetAdminView[];
  loading?: boolean;
}>();

const emit = defineEmits<{
  (e: "select", uuid: string): void;
  (e: "copyLink", uuid: string): void;
}>();

const columns = [
  {
    accessorKey: "name",
    header: "名称/类型",
    cell: ({ row }: any) =>
      h("div", { class: "space-y-1" }, [
        h(
          "div",
          { class: "font-semibold text-sm text-[var(--text-primary)]" },
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
          () => "复制链接"
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
</script>

