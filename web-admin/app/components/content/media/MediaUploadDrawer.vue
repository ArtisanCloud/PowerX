<template>
  <UModal
    v-model:open="open"
    title="上传/入库"
    description="预签名上传（推荐）或外链入库"
    :close="{ onClick: close }"
    :ui="{ footer: 'justify-end' }"
    prevent-close
  >
    <template #body>
      <div class="p-4 sm:p-5 max-w-3xl w-full space-y-4 max-h-[75vh] overflow-y-auto">
        <UTabs v-model="tab" :items="tabs" />

        <UAlert
          v-if="error"
          color="red"
          icon="i-heroicons-exclamation-triangle"
          :title="error"
        />

        <div v-if="busy" class="space-y-2">
          <div class="flex items-center justify-between text-xs text-[var(--text-secondary)]">
            <span>{{ progressLabel }}</span>
            <span>{{ Math.round(progress) }}%</span>
          </div>
          <UProgress :value="progress" color="primary" />
        </div>

        <div v-if="tab === 'presign'" class="space-y-4">
          <div class="space-y-2">
            <div class="text-xs text-[var(--text-tertiary)]">文件</div>
            <input
              class="block w-full rounded-md border border-[var(--border-color)] bg-transparent px-3 py-2 text-sm"
              type="file"
              :disabled="busy"
              @change="onFileChange"
            />
            <div v-if="file" class="text-xs text-[var(--text-tertiary)] break-all">
              {{ file.name }} · {{ formatBytes(file.size) }} · {{ file.type || "unknown" }}
            </div>
          </div>

          <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
            <div class="space-y-1">
              <div class="text-xs text-[var(--text-tertiary)]">名称</div>
              <UInput v-model="presignForm.name" :disabled="busy" placeholder="默认取文件名" />
            </div>
            <div class="space-y-1">
              <div class="text-xs text-[var(--text-tertiary)]">驱动（可选）</div>
              <USelect
                v-model="presignForm.driver"
                :disabled="busy"
                :items="driverOptions"
                placeholder="默认驱动"
                class="w-full"
              />
            </div>
          </div>

          <div class="space-y-1">
            <div class="text-xs text-[var(--text-tertiary)]">标签（可选）</div>
            <UInput
              v-model="presignForm.tagsText"
              :disabled="busy"
              placeholder="逗号分隔，例如: marketing,hero"
            />
          </div>

          <div class="flex items-center justify-end gap-2">
            <UButton
              color="neutral"
              variant="subtle"
              type="button"
              :disabled="busy"
              @click="resetPresign"
            >
              重置
            </UButton>
            <UButton
              color="primary"
              type="button"
              :loading="busy"
              :disabled="!file"
              @click="startPresignUpload"
            >
              开始上传
            </UButton>
          </div>
        </div>

        <div v-else-if="tab === 'external'" class="space-y-4">
          <div class="space-y-1">
            <div class="text-xs text-[var(--text-tertiary)]">名称</div>
            <UInput v-model="externalForm.name" :disabled="busy" placeholder="例如：官网 Logo" />
          </div>
          <div class="space-y-1">
            <div class="text-xs text-[var(--text-tertiary)]">外链 URL</div>
            <UInput
              v-model="externalForm.externalUrl"
              :disabled="busy"
              placeholder="https://example.com/file.png"
            />
          </div>
          <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
            <div class="space-y-1">
              <div class="text-xs text-[var(--text-tertiary)]">驱动（可选）</div>
              <USelect
                v-model="externalForm.driver"
                :disabled="busy"
                :items="driverOptions"
                placeholder="默认驱动"
                class="w-full"
              />
            </div>
            <div class="space-y-1">
              <div class="text-xs text-[var(--text-tertiary)]">标签（可选）</div>
              <UInput
                v-model="externalForm.tagsText"
                :disabled="busy"
                placeholder="逗号分隔，例如: marketing,hero"
              />
            </div>
          </div>

          <div class="flex items-center justify-end gap-2">
            <UButton
              color="neutral"
              variant="subtle"
              type="button"
              :disabled="busy"
              @click="resetExternal"
            >
              重置
            </UButton>
            <UButton
              color="primary"
              type="button"
              :loading="busy"
              :disabled="busy"
              @click="createExternalAsset"
            >
              创建
            </UButton>
          </div>
        </div>
      </div>
    </template>

    <template #footer>
      <div class="flex justify-end gap-2 w-full">
        <UButton
          color="neutral"
          variant="subtle"
          type="button"
          :disabled="busy"
          @click="close"
        >
          关闭
        </UButton>
      </div>
    </template>
  </UModal>
</template>

<script setup lang="ts">
import { useToast } from "#imports";
import { useApiClient } from "~/composables/api";
import { useMediaAssetService } from "~/composables/api/services/mediaAssetService";
import type {
  MediaAssetAdminView,
  PresignMediaAssetResult,
} from "~/composables/api/services/mediaAssetService";

const props = defineProps<{
  modelValue: boolean;
}>();

const emit = defineEmits<{
  (e: "update:modelValue", value: boolean): void;
  (e: "done", asset: MediaAssetAdminView): void;
}>();

const open = computed({
  get: () => props.modelValue,
  set: (v: boolean) => emit("update:modelValue", v),
});

const toast = useToast();
const apiClient = useApiClient();
const media = useMediaAssetService();

const tabs = [
  { label: "预签名上传（推荐）", value: "presign" },
  { label: "外链入库", value: "external" },
];

const tab = ref<"presign" | "external">("presign");

const driverOptions = [
  // reka-ui 的 SelectItem 禁止 value=""，空值用 null 表示
  { label: "默认驱动", value: null },
  { label: "local", value: "local" },
  { label: "s3", value: "s3" },
];

const busy = ref(false);
const progress = ref(0);
const progressLabel = ref("");
const error = ref<string | null>(null);

const file = ref<File | null>(null);

const presignForm = reactive({
  name: "",
  driver: null as string | null,
  tagsText: "",
});

const externalForm = reactive({
  name: "",
  externalUrl: "",
  driver: null as string | null,
  tagsText: "",
});

function close(force = false) {
  if (!force && busy.value) {
    toast.add({ title: "上传中，暂不可关闭", color: "warning" });
    return;
  }
  if (process.client) {
    const active = document.activeElement as HTMLElement | null;
    active?.blur?.();
  }
  open.value = false;
}

function setBusy(label: string, value: number) {
  progressLabel.value = label;
  progress.value = value;
}

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

function onFileChange(e: Event) {
  const input = e.target as HTMLInputElement;
  const f = input.files && input.files[0] ? input.files[0] : null;
  file.value = f;
  if (f && !presignForm.name) {
    presignForm.name = f.name;
  }
}

function resetPresign() {
  file.value = null;
  presignForm.name = "";
  presignForm.driver = null;
  presignForm.tagsText = "";
  error.value = null;
  progress.value = 0;
  progressLabel.value = "";
}

function resetExternal() {
  externalForm.name = "";
  externalForm.externalUrl = "";
  externalForm.driver = null;
  externalForm.tagsText = "";
  error.value = null;
  progress.value = 0;
  progressLabel.value = "";
}

function toInternalApiPath(url: string): string {
  const raw = String(url || "").trim();
  if (!raw) return "";
  if (raw.startsWith("http://") || raw.startsWith("https://")) return raw;
  // strip /api or /api/vN because apiClient 会再拼 baseURL=/api
  const m = raw.match(/^\/api(?:\/v\d+)?(\/.*)$/i);
  if (m && m[1]) return m[1];
  return raw;
}

async function uploadWithPresign(presign: PresignMediaAssetResult, body: Blob) {
  const target = String(presign.url || "").trim();
  const method = (presign.method || "PUT").toUpperCase();
  const headers = presign.headers || {};

  if (target.startsWith("http://") || target.startsWith("https://")) {
    // 外部对象存储直传：必须原样透传 headers（可能包含签名约束）
    const resp = await fetch(target, {
      method,
      headers,
      body,
    });
    if (!resp.ok) {
      throw new Error(`上传失败：${resp.status} ${resp.statusText}`);
    }
    return;
  }

  // 内部写入端点：通过 apiClient（自动带 Authorization + X-Tenant-UUID）
  const path = toInternalApiPath(target);
  if (!path.startsWith("/")) {
    throw new Error("预签名 URL 非法");
  }
  await apiClient.request(method as any, path, body, {
    headers: {
      ...headers,
      // Content-Type 对本地上传非必须；保留也会被 apiClient 对 Blob 删除，不影响 token headers
    },
    useGlobalLoading: false,
  } as any);
}

async function startPresignUpload() {
  if (!file.value) return;
  error.value = null;
  busy.value = true;
  progress.value = 0;
  progressLabel.value = "";

  try {
    const name = (presignForm.name || file.value.name || "").trim();
    if (!name) {
      throw new Error("名称不能为空");
    }

    setBusy("创建资产记录...", 10);
    const created = await media.createAsset({
      name,
      driver: presignForm.driver || undefined,
      tags: normalizeTags(presignForm.tagsText),
      uploadMethod: "presign_upload",
      mimeType: file.value.type || undefined,
      sizeBytes: file.value.size || undefined,
    });

    setBusy("生成上传预签名...", 35);
    const presign = await media.presign(created.uuid, {
      action: "upload",
      method: "PUT",
      content_type: file.value.type || "application/octet-stream",
    });

    setBusy("上传中...", 70);
    await uploadWithPresign(presign, file.value);

    setBusy("完成", 100);
    toast.add({ title: "上传成功" });
    emit("done", created);
    close(true);
    resetPresign();
  } catch (e: any) {
    const msg = String(e?.message || "上传失败");
    error.value = msg;
  } finally {
    busy.value = false;
  }
}

async function createExternalAsset() {
  error.value = null;
  busy.value = true;
  progress.value = 0;
  progressLabel.value = "";

  try {
    const name = externalForm.name.trim();
    const externalUrl = externalForm.externalUrl.trim();
    if (!name) throw new Error("名称不能为空");
    if (!externalUrl) throw new Error("外链 URL 不能为空");

    setBusy("创建外链资产...", 40);
    const created = await media.createAsset({
      name,
      driver: externalForm.driver || undefined,
      tags: normalizeTags(externalForm.tagsText),
      uploadMethod: "external_link",
      externalUrl,
    });

    setBusy("完成", 100);
    toast.add({ title: "创建成功" });
    emit("done", created);
    close(true);
    resetExternal();
  } catch (e: any) {
    error.value = String(e?.message || "创建失败");
  } finally {
    busy.value = false;
  }
}

watch(
  () => open.value,
  (v) => {
    if (!v) {
      error.value = null;
      progress.value = 0;
      progressLabel.value = "";
      busy.value = false;
    }
  }
);
</script>
