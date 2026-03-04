<template>
  <div class="space-y-6">
    <UCard>
      <template #header>
        <div class="text-sm font-medium text-[var(--text-primary)]">基本信息</div>
      </template>

      <div class="space-y-4">
        <div class="grid grid-cols-1 gap-3">
          <div class="space-y-1">
            <div class="text-xs text-[var(--text-tertiary)]">名称</div>
            <UInput v-model="local.name" placeholder="请输入名称" />
          </div>

          <div class="space-y-1">
            <div class="text-xs text-[var(--text-tertiary)]">描述</div>
            <UTextarea v-model="local.description" :rows="3" placeholder="可选" />
          </div>

          <div class="space-y-1">
            <div class="text-xs text-[var(--text-tertiary)]">业务状态</div>
            <USelect v-model="local.businessStatus" :items="statusOptions" placeholder="选择状态" />
          </div>

          <div class="space-y-1">
            <div class="text-xs text-[var(--text-tertiary)]">标签</div>
            <UInput v-model="local.tagsText" placeholder="逗号分隔，例如: marketing,hero" />
          </div>
        </div>

        <div class="flex items-center justify-end gap-2">
          <UButton size="sm" variant="ghost" @click="$emit('reset')">重置</UButton>
          <UButton size="sm" :loading="saving" @click="emitSave">保存</UButton>
        </div>
      </div>
    </UCard>

    <UCard>
      <template #header>
        <div class="text-sm font-medium text-[var(--text-primary)]">存储信息</div>
      </template>

      <div class="space-y-3 text-sm">
        <div class="flex items-center justify-between">
          <span class="text-[var(--text-secondary)]">类型</span>
          <span class="text-[var(--text-primary)]">{{ asset?.mimeType || "unknown" }}</span>
        </div>
        <div class="flex items-center justify-between">
          <span class="text-[var(--text-secondary)]">大小</span>
          <span class="text-[var(--text-primary)]">
            {{ asset?.sizeBytes != null ? formatBytes(asset.sizeBytes) : "-" }}
          </span>
        </div>
        <div class="flex items-center justify-between">
          <span class="text-[var(--text-secondary)]">驱动</span>
          <span class="text-[var(--text-primary)]">{{ asset?.driver || "-" }}</span>
        </div>
        <div class="space-y-1">
          <div class="text-[var(--text-secondary)]">ObjectKey</div>
          <div class="break-all text-xs text-[var(--text-tertiary)]">
            {{ asset?.objectKey || "-" }}
          </div>
        </div>
        <div class="space-y-1">
          <div class="text-[var(--text-secondary)]">更新时间</div>
          <div class="text-xs text-[var(--text-tertiary)]">
            {{ formatTime(asset?.updatedAt || "") }}
          </div>
        </div>
      </div>
    </UCard>

    <UCard>
      <template #header>
        <div class="text-sm font-medium text-[var(--text-primary)]">扩展元数据</div>
      </template>

      <div class="space-y-2">
        <div v-if="metadataEntries.length === 0" class="text-sm text-[var(--text-secondary)]">-</div>
        <div v-else class="space-y-2">
          <div
            v-for="(item, idx) in metadataEntries"
            :key="idx"
            class="rounded-md border border-[var(--border-color)] p-2"
          >
            <div class="text-xs text-[var(--text-tertiary)] break-all">{{ item[0] }}</div>
            <div class="mt-1 text-xs text-[var(--text-secondary)] break-all">{{ item[1] }}</div>
          </div>
        </div>
      </div>
    </UCard>
  </div>
</template>

<script setup lang="ts">
import type { MediaAssetAdminView } from "~/composables/api/services/mediaAssetService";

export interface MediaAssetDetailFormState {
  name: string;
  description: string;
  businessStatus: string;
  tagsText: string;
}

const props = defineProps<{
  asset: MediaAssetAdminView | null;
  modelValue: MediaAssetDetailFormState;
  saving?: boolean;
}>();

const emit = defineEmits<{
  (e: "update:modelValue", value: MediaAssetDetailFormState): void;
  (e: "save"): void;
  (e: "reset"): void;
}>();

const local = reactive<MediaAssetDetailFormState>({
  name: "",
  description: "",
  businessStatus: "",
  tagsText: "",
});

watch(
  () => props.modelValue,
  (next) => {
    Object.assign(local, next || {});
  },
  { immediate: true, deep: true }
);

watch(
  local,
  () => {
    emit("update:modelValue", { ...local });
  },
  { deep: true }
);

const saving = computed(() => !!props.saving);

const statusOptions = [
  { label: "draft", value: "draft" },
  { label: "under_review", value: "under_review" },
  { label: "published", value: "published" },
  { label: "archived", value: "archived" },
];

const metadataEntries = computed(() => Object.entries(props.asset?.metadata || {}));

function emitSave() {
  emit("save");
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
