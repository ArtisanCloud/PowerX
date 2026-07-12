<template>
  <div>
    <div v-if="asset?.externalUrl" class="space-y-3">
      <div class="text-sm text-[var(--text-secondary)]">
        该资源为外链入库，预览/下载将跳转到外部地址。
      </div>
      <UButton
        icon="i-lucide-external-link"
        variant="outline"
        size="sm"
        @click="$emit('openExternal')"
      >
        打开外链
      </UButton>
      <div class="break-all text-xs text-[var(--text-tertiary)]">
        {{ asset.externalUrl }}
      </div>
    </div>

    <div v-else class="space-y-3">
      <div v-if="state === 'loading'" class="flex items-center justify-center py-10">
        <USkeleton class="h-40 w-full" />
      </div>
      <div
        v-else-if="state === 'error'"
        class="rounded-md border border-[var(--border-color)] p-4"
      >
        <div class="text-sm text-[var(--text-secondary)]">
          预览加载失败（可尝试下载）。
        </div>
      </div>
      <div v-else-if="previewUrl" class="space-y-2">
        <div v-if="kind === 'image'" class="flex justify-center">
          <img
            :src="previewUrl"
            class="max-h-[520px] max-w-full h-auto w-auto rounded-md object-contain bg-black/5"
            :data-media-source="previewSource || undefined"
            alt="media preview"
          />
        </div>
        <video
          v-else-if="kind === 'video'"
          :src="previewUrl"
          class="w-full rounded-md bg-black/5"
          :data-media-source="previewSource || undefined"
          controls
        />
        <audio
          v-else-if="kind === 'audio'"
          :src="previewUrl"
          :data-media-source="previewSource || undefined"
          class="w-full"
          controls
        />
        <div v-else class="rounded-md border border-[var(--border-color)] p-4">
          <div class="text-sm text-[var(--text-secondary)]">
            当前类型不支持内嵌预览，请下载查看。
          </div>
        </div>
      </div>
      <div v-else class="rounded-md border border-[var(--border-color)] p-4">
        <div class="text-sm text-[var(--text-secondary)]">
          暂无可用预览。
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { MediaAssetAdminView } from "~/composables/api/services/mediaAssetService";

defineProps<{
  asset: MediaAssetAdminView | null;
  previewUrl: string | null;
  previewSource?: "preview" | "thumbnail" | null;
  state: "idle" | "loading" | "ready" | "error";
  kind: "image" | "video" | "audio" | "unknown";
}>();

defineEmits<{
  (e: "openExternal"): void;
}>();
</script>
