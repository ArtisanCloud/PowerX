<template>
  <div
    :class="[
      'overflow-hidden rounded-md bg-black/5 flex items-center justify-center',
      containerClass,
    ]"
  >
    <div
      v-if="!canInlinePreview"
      class="flex flex-col items-center justify-center gap-1 px-2 text-center"
    >
      <UIcon
        :name="fallbackIcon"
        class="h-6 w-6 text-[var(--text-tertiary)]"
      />
      <div v-if="showLabel" class="text-[10px] text-[var(--text-tertiary)]">
        {{ fallbackLabel }}
      </div>
    </div>

    <USkeleton v-else-if="state === 'loading'" class="h-full w-full" />

    <img
      v-else-if="objectUrl"
      :src="objectUrl"
      :alt="asset?.name || 'media'"
      :class="['h-full w-full object-contain', imgClass]"
      :data-media-source="previewSource || undefined"
      loading="lazy"
      decoding="async"
    />

    <div v-else class="flex flex-col items-center justify-center gap-1 px-2 text-center">
      <UIcon
        name="i-heroicons-photo"
        class="h-6 w-6 text-[var(--text-tertiary)]"
      />
      <div v-if="showLabel" class="text-[10px] text-[var(--text-tertiary)]">
        加载失败
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useMediaAssetService } from "~/composables/api/services/mediaAssetService";
import type { MediaAssetAdminView } from "~/composables/api/services/mediaAssetService";

const props = withDefaults(
  defineProps<{
    asset: MediaAssetAdminView;
    containerClass?: string;
    imgClass?: string;
    showLabel?: boolean;
  }>(),
  {
    containerClass: "h-36 w-full",
    imgClass: "",
    showLabel: false,
  }
);

const media = useMediaAssetService();

const objectUrl = ref<string | null>(null);
const state = ref<"idle" | "loading" | "ready" | "error">("idle");
const previewSource = ref<"preview" | "thumbnail" | null>(null);

const mimeType = computed(() => String(props.asset?.mimeType || "").toLowerCase());
const isImage = computed(() => mimeType.value.startsWith("image/"));
const canInlinePreview = computed(() => {
  if (!props.asset?.uuid) return false;
  if (props.asset?.externalUrl) return false;
  return isImage.value;
});

const fallbackIcon = computed(() => {
  if (props.asset?.externalUrl) return "i-lucide-external-link";
  if (mimeType.value.startsWith("video/")) return "i-lucide-video";
  if (mimeType.value.startsWith("audio/")) return "i-lucide-music";
  return "i-heroicons-photo";
});

const fallbackLabel = computed(() => {
  if (props.asset?.externalUrl) return "外链";
  if (!mimeType.value) return "未知";
  return mimeType.value;
});

function revoke() {
  if (!process.client) return;
  if (objectUrl.value) URL.revokeObjectURL(stripObjectUrlFragment(objectUrl.value));
  objectUrl.value = null;
  previewSource.value = null;
  state.value = "idle";
}

function stripObjectUrlFragment(url: string) {
  return String(url || "").split("#", 1)[0];
}

function withMediaSourceFragment(url: string, source: "preview" | "thumbnail") {
  return `${url}#media-source=${encodeURIComponent(source)}`;
}

let loadSeq = 0;
async function load() {
  revoke();
  if (!canInlinePreview.value) return;

  const seq = ++loadSeq;
  state.value = "loading";
  try {
    const blob = await loadPreferredPreviewBlob(props.asset.uuid);
    if (seq !== loadSeq) return;
    const source = media.selectPreviewVariant(props.asset);
    if (!source) throw new Error("missing media asset preview variant");
    previewSource.value = source;
    objectUrl.value = withMediaSourceFragment(URL.createObjectURL(blob), source);
    state.value = "ready";
  } catch {
    if (seq !== loadSeq) return;
    state.value = "error";
  }
}

async function loadPreferredPreviewBlob(uuid: string): Promise<Blob> {
  const id = String(uuid || "").trim();
  if (!id) throw new Error("missing media asset uuid");
  return media.getPreviewResourceBlob(props.asset, "inline");
}

watch(
  () => props.asset?.uuid,
  () => load(),
  { immediate: true }
);

onBeforeUnmount(() => revoke());
</script>
