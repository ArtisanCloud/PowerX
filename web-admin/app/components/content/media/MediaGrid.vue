<template>
  <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
    <UCard
      v-for="asset in items"
      :key="asset.uuid"
      class="relative cursor-pointer hover:shadow-md transition-shadow"
      @click="$emit('select', asset.uuid)"
    >
      <div class="absolute left-3 top-3 z-10 rounded-md bg-[var(--bg-primary)]/90 px-1 py-0.5 shadow-sm">
        <UCheckbox
          :model-value="isSelected(asset.uuid)"
          :aria-label="`选择 ${asset.name || asset.uuid}`"
          @click.stop
          @update:model-value="$emit('toggleSelected', asset.uuid, Boolean($event))"
        />
      </div>
      <div class="space-y-3">
        <MediaThumbnail :asset="asset" container-class="h-36 w-full" />

        <div class="flex items-start justify-between gap-2">
          <div class="min-w-0">
            <div class="truncate text-sm font-semibold text-[var(--text-primary)]">
              {{ asset.name || "-" }}
            </div>
            <div class="mt-1 flex flex-wrap items-center gap-1 text-xs text-[var(--text-secondary)]">
              <span>{{ asset.mimeType || "unknown" }}</span>
              <span v-if="asset.sizeBytes != null">· {{ formatBytes(asset.sizeBytes) }}</span>
            </div>
          </div>
          <UBadge :color="statusColor(asset.businessStatus)" variant="soft" class="shrink-0">
            {{ asset.businessStatus || "-" }}
          </UBadge>
        </div>

        <div class="flex items-center justify-between text-xs text-[var(--text-tertiary)]">
          <span>{{ asset.driver || "-" }}</span>
          <span>{{ formatTime(asset.updatedAt) }}</span>
        </div>
      </div>
    </UCard>
  </div>
</template>

<script setup lang="ts">
import type { MediaAssetAdminView } from "~/composables/api/services/mediaAssetService";
import MediaThumbnail from "~/components/content/media/MediaThumbnail.vue";

const props = defineProps<{
  items: MediaAssetAdminView[];
  selectedUuids?: string[];
}>();

defineEmits<{
  (e: "select", uuid: string): void;
  (e: "toggleSelected", uuid: string, selected: boolean): void;
}>();

const selectedSet = computed(() => new Set((props.selectedUuids || []).map((id) => String(id || "").trim()).filter(Boolean)));

function isSelected(uuid: string) {
  return selectedSet.value.has(String(uuid || "").trim());
}

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
</script>
