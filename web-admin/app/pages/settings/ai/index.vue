<template>
  <div class="space-y-6 p-4">
    <!-- 标题 -->
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-lg font-semibold text-[var(--text-primary)]">
          {{ $t("settings.ai.title") }}
        </h1>
        <p class="text-sm text-[var(--text-secondary)]">
          {{ $t("settings.ai.description") }}
        </p>
      </div>
      <div class="flex items-center gap-2">
        <UButton
          color="primary"
          icon="i-heroicons-cloud-arrow-up"
          class="whitespace-nowrap"
          :loading="saving"
          @click="saveSettings"
        >
          {{ $t("common.save") }}
        </UButton>
        <UButton
          variant="ghost"
          icon="i-heroicons-arrow-path"
          class="whitespace-nowrap"
          @click="resetSettings"
        >
          {{ $t("common.reset") }}
        </UButton>
        <UButton
          variant="soft"
          icon="i-heroicons-banknotes"
          class="whitespace-nowrap"
          :to="costGuardLink"
        >
          {{ $t("settings.ai.actions.openCostGuard") }}
        </UButton>
        <UButton
          variant="soft"
          icon="i-heroicons-link"
          class="whitespace-nowrap"
          :to="connectorsLink"
        >
          {{ $t("settings.ai.actions.openConnectors") }}
        </UButton>
        <UButton
          v-if="isRoot"
          variant="soft"
          icon="i-heroicons-table-cells"
          class="whitespace-nowrap"
          :to="registryLink"
        >
          {{ $t("settings.ai.actions.openCapabilityRegistry") }}
        </UButton>
      </div>
    </div>

    <div class="grid grid-cols-1 gap-6 lg:grid-cols-4">
      <div class="lg:col-span-1">
        <div class="space-y-4">
          <div
            class="rounded-lg border border-[var(--border-color)] bg-[var(--card-bg)] p-4"
          >
            <div class="mb-3 text-sm font-medium text-[var(--text-primary)]">
              {{ $t("settings.ai.environment") }}
            </div>
            <USelect
              v-model="env"
              :items="envOptions"
              class="w-full"
              icon="i-heroicons-circle-stack"
            >
              <template #leading>
                <div
                  class="w-2 h-2 rounded-full"
                  :class="`bg-${envStore.currentEnvColor}-500`"
                ></div>
              </template>
            </USelect>
          </div>

          <div
            class="rounded-lg border border-[var(--border-color)] bg-[var(--card-bg)] p-4"
          >
            <div class="mb-3 text-sm font-medium text-[var(--text-primary)]">
              {{ $t("settings.ai.modalityTabsLabel") }}
            </div>
            <div class="space-y-2">
              <button
                v-for="tab in modalityTabs"
                :key="tab.key"
                class="w-full flex items-center gap-3 px-3 py-2 text-left rounded-md transition-colors"
                :class="[
                  modality === tab.key
                    ? 'bg-primary-500 text-white'
                    : 'text-[var(--text-secondary)] hover:bg-[var(--hover-bg)] hover:text-[var(--text-primary)]',
                ]"
                @click="modality = tab.key as any"
              >
                <UIcon :name="tab.icon" class="w-4 h-4 flex-shrink-0" />
                <span class="text-sm font-medium">{{ tab.label }}</span>
              </button>
            </div>
          </div>
        </div>
      </div>

      <div class="lg:col-span-2 space-y-6">
        <div
          class="rounded-lg border border-[var(--border-color)] bg-[var(--card-bg)] p-4"
        >
          <div class="mb-4 font-medium text-[var(--text-primary)]">
            {{ currentTitle }} - {{ $t("settings.ai.sections.general") }}
          </div>
          <p
            v-if="modality === 'image' || modality === 'video'"
            class="mb-3 text-xs text-[var(--text-secondary)]"
          >
            图像/视频的 Provider 列表已对齐；若某 Provider 在当前模态暂无专用模型，会自动回退展示另一模态的模型（占位）。
          </p>
          <ProviderModelForm
            :provider-options="providerOptions"
            :app-options="appOptions"
            :model-options="modelOptions"
            :active-provider="activeProviderForForm"
            :state="currentState"
            @provider-changed="onProviderChanged"
            @app-changed="onAppChanged"
          />
        </div>

        <div
          class="rounded-lg border border-[var(--border-color)] bg-[var(--card-bg)] p-4"
        >
          <div class="mb-4 font-medium text-[var(--text-primary)]">
            {{ currentTitle }} - {{ $t("settings.ai.sections.parameters") }}
          </div>
          <ModalityParamsForm
            :active-modality="modality"
            :llm="llm"
            :image="image"
            :embedding="embedding"
            :audio-tts="audioTTS"
            :audio-asr="audioASR"
            :video="video"
            :model3d="model3d"
            :rerank="rerank"
            :image-size-options="imageSizeOptions"
            :image-quality-options="imageQualityOptions"
            :image-format-options="imageFormatOptions"
            :truncate-options="truncateOptions"
            :video-resolution-options="videoResolutionOptions"
            :model3d-format-options="model3dFormatOptions"
            :voice-options="voiceOptions"
            :audio-format-options="audioFormatOptions"
            :audio-quality-options="audioQualityOptions"
            :language-options="languageOptions"
            :response-format-options="responseFormatOptions"
            :top-k-options="topKOptions"
          />
        </div>
      </div>

      <div class="space-y-6 lg:col-span-1">
        <TestPanel
          :current-title="currentTitle"
          :current-state="currentState"
          :last-test-message="lastTestMessage"
          :on-test-connection="testConnection"
          :on-test-quick-call="testQuickCall"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { storeToRefs } from "pinia";
import ProviderModelForm from "~/components/settings/ai/ProviderModelForm.vue";
import ModalityParamsForm from "~/components/settings/ai/ModalityParamsForm.vue";
import TestPanel from "~/components/settings/ai/TestPanel.vue";
import { useAISettingsStore } from "~/stores/aiSettings";
import { useEnvStore, ENV_OPTIONS } from "~/stores/envStore";
import { useUserStore } from "~/stores/user";
import {
  AISettingService,
  type Provider,
  type SaveSettingsPayload,
} from "~/composables/api/services/aiSettingService";
import type { SelectOption } from "~/composables/api/types/select";

type Modality =
  | "llm"
  | "image"
  | "embedding"
  | "audio_tts"
  | "audio_asr"
  | "video"
  | "model3d"
  | "rerank";

// 使用 AI 设置 store
const aiSettingsStore = useAISettingsStore();
const toast = useToast();
const localePath = useLocalePath();
const costGuardLink = computed(() => localePath("/settings/ai/cost"));
const connectorsLink = computed(() => localePath("/settings/ai/connectors"));
const registryLink = computed(() =>
  localePath("/settings/ai/capability-registry")
);

const route = useRoute();
const userStore = useUserStore();
const { isRoot } = storeToRefs(userStore);

onMounted(async () => {
  try {
    const queryModality = resolveModalityFromQuery(route.query?.modality);
    if (queryModality) {
      modality.value = queryModality;
    }
    if (!userStore.context) {
      await userStore.fetchUserContext();
    }
  } catch (error) {
    console.error("加载用户上下文失败:", error);
  }
});

/**
 * Tab & 环境
 */
const modalityTabs = [
  { key: "llm", label: "LLM 文本", icon: "i-heroicons-bars-3-bottom-left" },
  { key: "image", label: "图像生成", icon: "i-heroicons-photo" },
  {
    key: "embedding",
    label: "向量嵌入",
    icon: "i-heroicons-square-3-stack-3d",
  },
  { key: "audio_tts", label: "语音合成", icon: "i-heroicons-speaker-wave" },
  { key: "audio_asr", label: "语音识别", icon: "i-heroicons-microphone" },
  { key: "video", label: "视频生成", icon: "i-heroicons-video-camera" },
  { key: "model3d", label: "3D 生成", icon: "i-heroicons-cube" },
  { key: "rerank", label: "重排序", icon: "i-heroicons-arrows-up-down" },
] as const;

const modality = ref<Modality>("llm");

const resolveModalityFromQuery = (value: unknown): Modality | null => {
  const raw = String(value || "").trim();
  if (!raw) return null;
  const hit = modalityTabs.find((tab) => tab.key === raw);
  return hit ? (hit.key as Modality) : null;
};

// 使用环境store
const envStore = useEnvStore();
const envOptions = computed(() =>
  ENV_OPTIONS.map((option) => ({
    label: option.label,
    value: option.value,
  }))
);
const env = computed({
  get: () => envStore.currentEnv,
  set: (value: string) => envStore.setCurrentEnv(value),
});

const getErrorMessage = (error: unknown) => {
  if (!error) return "未知错误";
  if (typeof error === "string") {
    return error;
  }
  if (typeof error === "object") {
    const anyError = error as Record<string, any>;
    return (
      anyError?.data?.message ||
      anyError?.response?.statusMessage ||
      anyError?.message ||
      anyError?.error ||
      "未知错误"
    );
  }
  return "未知错误";
};

/**
 * Provider 列表与模型目录（从 store 获取）
 */
const providerOptions = computed<SelectOption[]>(() => {
  const providers = aiSettingsStore.providers;
  const placeholder: SelectOption = {
    label: $t("agent.config.selectProvider"),
    value: null,
  };

  if (!Array.isArray(providers)) {
    console.warn("providers 不是数组:", providers);
    return [placeholder];
  }

  const options = providers.map((p: Provider) => ({
    label: p.Name,
    value: p.ID!,
  }));

  return [placeholder, ...options];
});

const appOptions = computed<SelectOption[]>(() => {
  const apps = resolveAppsForProvider(currentState.value.provider);
  return buildAppOptions(apps);
});

const activeProviderForForm = computed(() => {
  const pid = String(currentState.value.provider ?? "").trim();
  if (!pid) return null;
  const hit =
    (aiSettingsStore.providers ?? []).find(
      (p: Provider) => String(p.ID ?? "").trim().toLowerCase() === pid.toLowerCase()
    ) ?? null;
  if (!hit) return null;
  return {
    id: hit.ID,
    name: hit.Name,
    auth: hit.auth ?? undefined,
  };
});

// 当前选中的 Provider
const activeProvider = computed(
  () => aiSettingsStore.activeProfile?.provider || null
);

const activeModel = computed(
  () => aiSettingsStore.activeProfile?.model || null
);

/**
 * 各模态的 state（包含 Provider/Model/凭证 + 模态参数）
 */
type BaseConn = {
  provider: string | null;
  app?: string | null;
  model: string | null;
  authMode: string;
  apiKey: string;
  secretId: string;
  secretKey: string;
  baseURL: string;
  region: string;
  organization: string;
  azureDeployment?: string;
};

const llm = reactive<
  BaseConn & {
    temperature: number;
    maxTokens: number;
    topP: number;
    stream: boolean;
  }
>({
  provider: null,
  app: "",
  model: null,
  authMode: "",
  apiKey: "",
  secretId: "",
  secretKey: "",
  baseURL: "",
  region: "",
  organization: "",
  temperature: 0.7,
  maxTokens: 4096,
  topP: 1,
  stream: true,
});

const image = reactive<
  BaseConn & {
    size: string;
    quality: string;
    format: string;
    promptHint: string;
  }
>({
  provider: "openai",
  app: "",
  model: "gpt-image-1",
  authMode: "",
  apiKey: "",
  secretId: "",
  secretKey: "",
  baseURL: "",
  region: "",
  organization: "",
  size: "1024x1024",
  quality: "standard",
  format: "png",
  promptHint: "",
});

const embedding = reactive<
  BaseConn & { dimensions: number; truncate: string; batch: number; maxInputTokens?: number }
>({
  provider: "openai",
  app: "",
  model: "text-embedding-3-small",
  authMode: "",
  apiKey: "",
  secretId: "",
  secretKey: "",
  baseURL: "",
  region: "",
  organization: "",
  dimensions: 1536,
  truncate: "none",
  batch: 32,
  maxInputTokens: undefined,
});

const audioTTS = reactive<
  BaseConn & {
    voice: string;
    speed: number;
    format: string;
    quality: string;
  }
>({
  provider: "openai",
  app: "",
  model: "gpt-4o-mini-tts",
  authMode: "",
  apiKey: "",
  secretId: "",
  secretKey: "",
  baseURL: "",
  region: "",
  organization: "",
  voice: "alloy",
  speed: 1.0,
  format: "mp3",
  quality: "standard",
});

const audioASR = reactive<
  BaseConn & {
    language: string;
    responseFormat: string;
    temperature: number;
    prompt: string;
  }
>({
  provider: "openai",
  app: "",
  model: "whisper-1",
  authMode: "",
  apiKey: "",
  secretId: "",
  secretKey: "",
  baseURL: "",
  region: "",
  organization: "",
  language: "auto",
  responseFormat: "json",
  temperature: 0,
  prompt: "",
});

const video = reactive<
  BaseConn & {
    resolution: string;
    fps: number;
    maxDurationSec: number;
    promptHint: string;
  }
>({
  provider: "openai",
  app: "",
  model: "sora-preview",
  authMode: "",
  apiKey: "",
  secretId: "",
  secretKey: "",
  baseURL: "",
  region: "",
  organization: "",
  resolution: "1080p",
  fps: 24,
  maxDurationSec: 10,
  promptHint: "",
});

const model3d = reactive<
  BaseConn & {
    outputFormat: string;
    promptHint: string;
  }
>({
  provider: "hunyuan",
  app: "",
  model: "HY-3D-Express",
  authMode: "openai",
  apiKey: "",
  secretId: "",
  secretKey: "",
  baseURL: "",
  region: "",
  organization: "",
  outputFormat: "glb",
  promptHint: "",
});

const rerank = reactive<
  BaseConn & {
    topK: number;
    returnDocuments: boolean;
    maxChunksPerDoc: number;
  }
>({
  provider: "openai",
  app: "",
  model: "text-embedding-3-large",
  authMode: "",
  apiKey: "",
  secretId: "",
  secretKey: "",
  baseURL: "",
  region: "",
  organization: "",
  topK: 10,
  returnDocuments: true,
  maxChunksPerDoc: 10,
});

/**
 * 计算当前标题与当前 state
 */
const currentTitle = computed(() => {
  switch (modality.value) {
    case "llm":
      return "LLM 文本";
    case "image":
      return "图像生成";
    case "embedding":
      return "向量嵌入";
    case "audio_tts":
      return "语音合成";
    case "audio_asr":
      return "语音识别";
    case "video":
      return "视频生成";
    case "model3d":
      return "3D 生成";
    case "rerank":
      return "重排序";
    default:
      return "未知模态";
  }
});

const currentState = computed<any>({
  get() {
    switch (modality.value) {
      case "llm":
        return llm;
      case "image":
        return image;
      case "embedding":
        return embedding;
      case "audio_tts":
        return audioTTS;
      case "audio_asr":
        return audioASR;
      case "video":
        return video;
      case "model3d":
        return model3d;
      case "rerank":
        return rerank;
      default:
        return llm;
    }
  },
  set(_v) {
    // 保持为对象引用，不整体替换
  },
});

/**
 * ProviderModelForm 的 Model 下拉选项：从后端获取
 */
const modelOptions = computed<SelectOption[]>(() => {
  const models = aiSettingsStore.models;
  const placeholder: SelectOption = {
    label: $t("agent.config.selectModel"),
    value: null,
  };

  if (!Array.isArray(models)) {
    console.warn("models 不是数组:", models);
    return [placeholder];
  }

  // 可选：去重 + 过滤空值
  const uniq = Array.from(new Set(models)).filter((m) => !!m);

  const options = uniq.map((model) => ({
    label: model,
    value: model,
  }));

  return [placeholder, ...options];
});

const draftByProviderKey = reactive<Record<string, Record<string, any>>>({});

function normalizeProviderKey(provider?: string | null) {
  return String(provider ?? "")
    .trim()
    .toLowerCase();
}

function isHuggingFaceProvider(provider?: string | null) {
  const p = normalizeProviderKey(provider);
  return p === "huggingface" || p === "hf";
}

function resolveHuggingFaceBaseURL(_model?: string | null) {
  return "https://router.huggingface.co/v1";
}

function resolveAppsForProvider(provider?: string | null) {
  const pid = String(provider ?? "").trim();
  if (!pid) return [];
  const hit =
    (aiSettingsStore.providers ?? []).find(
      (p: Provider) => String(p.ID ?? "").trim().toLowerCase() === pid.toLowerCase()
    ) ?? null;
  return hit?.apps ?? [];
}

function buildAppOptions(apps?: { id: string; name: string }[]) {
  if (!Array.isArray(apps) || apps.length === 0) return [];
  const options = apps.map((app) => ({
    label: app.name || app.id,
    value: app.id,
  }));
  return [{ label: "不区分 App", value: "" }, ...options];
}

function splitAppModelKey(raw?: string | null) {
  const value = String(raw || "").trim();
  if (!value) return { app: "", model: "" };
  const idx = value.indexOf(":");
  if (idx <= 0) return { app: "", model: value };
  return { app: value.slice(0, idx), model: value.slice(idx + 1) };
}

function matchProfileModel(
  profileModel: string | null | undefined,
  app: string | null | undefined,
  model: string | null | undefined
) {
  const parsed = splitAppModelKey(profileModel);
  return (
    parsed.model === String(model || "").trim() &&
    parsed.app === String(app || "").trim()
  );
}

function draftKey(
  provider?: string | null,
  envVal?: string | null,
  modalityVal?: string | null
) {
  const p = normalizeProviderKey(provider);
  const e = String(envVal || "default").trim();
  const m = String(modalityVal || "llm").trim();
  return `${e}::${m}::${p}`;
}

function getDraftableFields(modalityVal?: string | null): string[] {
  const m = String(modalityVal || "").trim();
  const base = [
    "app",
    "model",
    "authMode",
    "apiKey",
    "secretId",
    "secretKey",
    "baseURL",
    "organization",
    "region",
    "azureDeployment",
  ];
  switch (m) {
    case "llm":
      return [...base, "temperature", "maxTokens", "topP", "stream"];
    case "image":
      return [...base, "size", "quality", "format", "promptHint"];
    case "embedding":
      return [...base, "truncate", "batch"];
    case "audio_tts":
      return [...base, "voice", "speed", "format", "quality"];
    case "audio_asr":
      return [...base, "language", "responseFormat", "temperature", "prompt"];
    case "video":
      return [...base, "resolution", "fps", "maxDurationSec", "promptHint"];
    case "model3d":
      return [...base, "outputFormat", "promptHint"];
    case "rerank":
      return [...base, "topK", "returnDocuments", "maxChunksPerDoc"];
    default:
      return base;
  }
}

function applyModalityDefaults(state: Record<string, any>, modalityVal?: string | null) {
  const m = String(modalityVal || "").trim();
  if ("app" in state) state.app = "";
  switch (m) {
    case "llm":
      state.temperature = 0.7;
      state.maxTokens = 4096;
      state.topP = 1;
      state.stream = true;
      if ("authMode" in state) state.authMode = "";
      break;
    case "image":
      state.size = "1024x1024";
      state.quality = "standard";
      state.format = "png";
      state.promptHint = "";
      if ("authMode" in state) state.authMode = "";
      break;
    case "embedding":
      state.dimensions = 0;
      state.truncate = "none";
      state.batch = 1;
      if ("authMode" in state) state.authMode = "";
      break;
    case "audio_tts":
      state.voice = "alloy";
      state.speed = 1.0;
      state.format = "mp3";
      state.quality = "standard";
      if ("authMode" in state) state.authMode = "";
      break;
    case "audio_asr":
      state.language = "auto";
      state.responseFormat = "json";
      state.temperature = 0;
      state.prompt = "";
      if ("authMode" in state) state.authMode = "";
      break;
    case "video":
      state.resolution = "1080p";
      state.fps = 24;
      state.maxDurationSec = 10;
      state.promptHint = "";
      if ("authMode" in state) state.authMode = "";
      break;
    case "model3d":
      state.outputFormat = "glb";
      state.promptHint = "";
      if ("authMode" in state) state.authMode = "openai";
      break;
    case "rerank":
      state.topK = 10;
      state.returnDocuments = true;
      state.maxChunksPerDoc = 10;
      if ("authMode" in state) state.authMode = "";
      break;
  }
}

function persistProviderDraft(
  provider?: string | null,
  envVal?: string | null,
  modalityVal?: string | null,
  stateOverride?: Record<string, any>
) {
  const p = normalizeProviderKey(provider);
  if (!p) return;
  const state = (stateOverride || (currentState.value as any)) as Record<string, any>;
  const k = draftKey(p, envVal, modalityVal);
  const fields = getDraftableFields(modalityVal);
  const snapshot: Record<string, any> = {};
  for (const f of fields) {
    if (f === "dimensions") {
      const dim = Number.parseInt(String(state[f] ?? "0"), 10);
      if (!Number.isFinite(dim) || dim <= 0) continue;
    }
    snapshot[f] = state[f];
  }
  draftByProviderKey[k] = snapshot;
}

function restoreProviderDraft(
  provider?: string | null,
  envVal?: string | null,
  modalityVal?: string | null,
  stateOverride?: Record<string, any>
) {
  const p = normalizeProviderKey(provider);
  const state = (stateOverride || (currentState.value as any)) as Record<string, any>;
  if (!p) return;

  // 1) 优先使用“该 provider 的草稿”（避免切换 provider 时把别人的 key 带过去）
  const draft = draftByProviderKey[draftKey(p, envVal, modalityVal)];
  if (draft) {
    const fields = getDraftableFields(modalityVal);
    for (const f of fields) {
      if (f in draft) state[f] = draft[f];
    }
    return;
  }

  // 2) 没有草稿：重置为默认参数 + 回填已保存的非敏感字段（注意：后端会脱敏，不会返回 api_key）
  applyModalityDefaults(state, modalityVal);

  const getter =
    typeof aiSettingsStore.getCredentialByProvider === "function"
      ? aiSettingsStore.getCredentialByProvider
      : null;
  const data = (getter ? getter(p)?.data ?? {} : {}) as Record<string, any>;
  const getString = (key: string) => (typeof data[key] === "string" ? data[key] : "");

  // 由 ProviderModelForm 兜底选择默认 authMode；这里仅在后端返回时回填
  state.authMode = getString("auth_mode");
  state.apiKey = ""; // 关键：不同 provider 的 key 必须隔离
  state.secretKey = ""; // 关键：敏感字段不从后端回填
  state.secretId = getString("secret_id");
  state.baseURL = getString("base_url");
  state.organization = getString("organization");
  state.region = getString("region");
  if ("azureDeployment" in state) state.azureDeployment = getString("azure_deployment");
}

async function onProviderChanged(nextProvider?: string) {
  const rawProvider = nextProvider ?? currentState.value.provider;
  const currentModality = modality.value;
  const envSnapshot = env.value;
  const modalitySnapshot = modality.value;

  // 关键：参数不全就短路，但不清空 models
  if (!rawProvider || !currentModality) {
    restoreProviderDraft(rawProvider, envSnapshot, modalitySnapshot);
    return;
  }

  restoreProviderDraft(rawProvider, envSnapshot, modalitySnapshot);
  const apps = resolveAppsForProvider(rawProvider);
  if (!apps.length) {
    currentState.value.app = "";
  } else {
    const appIds = new Set(apps.map((app) => String(app.id)));
    const currentApp = String(currentState.value.app || "").trim();
    if (currentApp && !appIds.has(currentApp)) {
      currentState.value.app = "";
    }
  }

  const appValue = String(currentState.value.app || "").trim();
  try {
    // ✅ 直接传原始值，让 store 内部处理规范化
    await aiSettingsStore.fetchModels(
      rawProvider,
      currentModality,
      env.value,
      appValue || undefined
    );
  } catch (error) {
    console.error("获取模型列表失败:", error);
    // 这里不清空，保持上一次成功值
  }

  // restoreDraft 可能会把旧 model 带回来；这里再做一次兜底校验
  const models = aiSettingsStore.models ?? [];
  if (models.length) {
    const curModel = currentState.value.model;
    if (!curModel || !models.includes(curModel)) {
      currentState.value.model = models[0];
    }
  } else {
    // ✅ 该 provider 在当前模态下没有可用模型：清空，避免出现 provider=Coze 但 model=OpenAI 的错配显示
    currentState.value.model = "";
  }
  if (modalitySnapshot === "embedding") {
    syncEmbeddingProbeMessageFromProfiles();
  }
}

async function onAppChanged(nextApp?: string) {
  const rawProvider = currentState.value.provider;
  const currentModality = modality.value;
  if (!rawProvider || !currentModality) return;
  if (nextApp !== undefined) {
    currentState.value.app = nextApp ?? "";
  }
  const appValue = String(currentState.value.app || "").trim();
  try {
    await aiSettingsStore.fetchModels(
      rawProvider,
      currentModality,
      env.value,
      appValue || undefined
    );
  } catch (error) {
    console.error("获取模型列表失败:", error);
  }

  const models = aiSettingsStore.models ?? [];
  if (models.length) {
    const curModel = currentState.value.model;
    if (!curModel || !models.includes(curModel)) {
      currentState.value.model = models[0];
    }
  } else {
    currentState.value.model = "";
  }
  if (currentModality === "embedding") {
    syncEmbeddingProbeMessageFromProfiles();
  }
}

// 旧实现：syncCredentialFieldsForProvider（已用 restoreProviderDraft 替代，避免 apiKey/参数串台）

/**
 * 选项集合（传给 ModalityParamsForm）
 */
const imageSizeOptions = [
  "auto",
  "256x256",
  "512x512",
  "1024x1024",
  "1536x1024",
  "1024x1536",
  "1792x1024",
  "1024x1792",
];
const imageQualityOptions = [
  "auto",
  "low",
  "medium",
  "high",
  "standard",
  "hd",
];
const imageFormatOptions = ["png", "jpeg", "webp"];
const truncateOptions = ["none", "start", "end"];
const videoResolutionOptions = ["720p", "1080p", "4k"];
const model3dFormatOptions = ["glb", "gltf", "obj", "fbx"];

// 新增音频TTS选项
const voiceOptions = ["alloy", "echo", "fable", "onyx", "nova", "shimmer"];
const audioFormatOptions = ["mp3", "opus", "aac", "flac"];
const audioQualityOptions = ["standard", "hd"];

// 新增音频ASR选项
const languageOptions = ["auto", "zh", "en", "ja", "ko", "es", "fr", "de"];
const responseFormatOptions = ["json", "text", "srt", "verbose_json", "vtt"];

// 新增重排序选项
const topKOptions = [5, 10, 20, 50, 100];

function buildPayloadForCurrentModality(promptOverride?: string) {
  const baseConn = {
    provider: currentState.value.provider ?? "",
    app: currentState.value.app ?? "",
    model: currentState.value.model ?? "",
    authMode: currentState.value.authMode ?? "",
    apiKey: currentState.value.apiKey ?? "",
    secretId: currentState.value.secretId ?? "",
    secretKey: currentState.value.secretKey ?? "",
    baseURL: currentState.value.baseURL ?? "",
    organization: currentState.value.organization ?? "",
    region: currentState.value.region ?? "",
    azureDeployment: currentState.value.azureDeployment ?? "",
  };

  let body: Record<string, any> = { ...baseConn };
  switch (modality.value) {
    case "llm":
      body = {
        ...baseConn,
        temperature: currentState.value.temperature ?? 0.7,
        maxTokens: currentState.value.maxTokens ?? 4096,
        topP: currentState.value.topP ?? 1,
        stream:
          currentState.value.stream !== undefined
            ? currentState.value.stream
            : true,
      };
      break;
    case "image":
      body = {
        ...baseConn,
        size: image.size,
        quality: image.quality,
        format: image.format,
        promptHint: image.promptHint,
      };
      break;
    case "embedding":
      body = {
        ...baseConn,
        dimensions: embedding.dimensions,
        truncate: embedding.truncate,
        batch: embedding.batch,
      };
      break;
    case "audio_tts":
      body = {
        ...baseConn,
        voice: audioTTS.voice,
        speed: audioTTS.speed,
        format: audioTTS.format,
        quality: audioTTS.quality,
      };
      break;
    case "audio_asr":
      body = {
        ...baseConn,
        language: audioASR.language,
        responseFormat: audioASR.responseFormat,
        temperature: audioASR.temperature,
        prompt: audioASR.prompt,
      };
      break;
    case "video":
      body = {
        ...baseConn,
        resolution: video.resolution,
        fps: video.fps,
        maxDurationSec: video.maxDurationSec,
        promptHint: video.promptHint,
      };
      break;
    case "model3d":
      body = {
        ...baseConn,
        outputFormat: model3d.outputFormat,
        promptHint: model3d.promptHint,
      };
      break;
    case "rerank":
      body = {
        ...baseConn,
        topK: rerank.topK,
        returnDocuments: rerank.returnDocuments,
        maxChunksPerDoc: rerank.maxChunksPerDoc,
      };
      break;
  }

  const payload: Record<string, any> = {
    env: env.value,
    modality: modality.value,
    [modality.value]: body,
  };

  if (promptOverride) {
    payload.prompt = promptOverride;
  }

  return payload;
}

/**
 * 保存/重置/测试（接入后端 API）
 */
const saving = computed(() => aiSettingsStore.saving);
const lastProbeMessage = ref("");
const lastTestMessage = computed(() => aiSettingsStore.lastTestMessage || lastProbeMessage.value);

const resolveEmbeddingMaxInputTokens = (profile: any) => {
  if (!profile) return 0;
  const defaults = profile?.defaults || {};
  const capCache = (profile as any)?.capCache || {};
  const keys = [
    "max_input_tokens",
    "max_tokens",
    "context_length",
    "context_window",
    "context_size",
    "max_context_tokens",
  ];
  for (const key of keys) {
    const v = defaults?.[key];
    const n = Number.parseInt(String(v ?? "0"), 10);
    if (Number.isFinite(n) && n > 0) return n;
  }
  for (const key of keys) {
    const v = capCache?.[key];
    const n = Number.parseInt(String(v ?? "0"), 10);
    if (Number.isFinite(n) && n > 0) return n;
  }
  return 0;
};

const resolveEmbeddingDimensionsFromProfiles = () => {
  if (modality.value !== "embedding") return 0;
  const curProvider = String(currentState.value.provider || "").trim().toLowerCase();
  const curModel = String(currentState.value.model || "").trim();
  if (!curProvider || !curModel) return 0;
  const profile = (aiSettingsStore.profiles || []).find(
    (p) =>
      String(p?.modality || "").toLowerCase() === "embedding" &&
      String(p?.provider || "").trim().toLowerCase() === curProvider &&
      matchProfileModel(p?.model, currentState.value.app, curModel)
  );
  if (!profile) return 0;
  const capCache = (profile as any).capCache || {};
  const dimRaw = capCache?.dimensions ?? profile?.defaults?.dimensions ?? profile?.defaults?.dim;
  const dim = Number.parseInt(String(dimRaw || "0"), 10);
  return Number.isFinite(dim) && dim > 0 ? dim : 0;
};

const syncEmbeddingProbeMessageFromProfiles = () => {
  if (modality.value !== "embedding") return;
  const curProvider = String(currentState.value.provider || "").trim().toLowerCase();
  const curModel = String(currentState.value.model || "").trim();
  if (!curProvider || !curModel) return;
  const profile = (aiSettingsStore.profiles || []).find(
    (p) =>
      String(p?.modality || "").toLowerCase() === "embedding" &&
      String(p?.provider || "").trim().toLowerCase() === curProvider &&
      matchProfileModel(p?.model, currentState.value.app, curModel)
  );
  if (!profile) {
    return;
  }
  const capCache = (profile as any).capCache || {};
  const probedAt = String(capCache?.probed_at || "").trim();
  const dimRaw = capCache?.dimensions ?? profile?.defaults?.dimensions ?? profile?.defaults?.dim;
  const dim = Number.parseInt(String(dimRaw || "0"), 10);
  const maxInputTokens = resolveEmbeddingMaxInputTokens(profile);
  if (Number.isFinite(dim) && dim > 0) {
    currentState.value.dimensions = dim;
  }
  if (Number.isFinite(maxInputTokens) && maxInputTokens > 0) {
    currentState.value.maxInputTokens = maxInputTokens;
  }
  if (probedAt) {
    const ts = Number.isNaN(Date.parse(probedAt)) ? probedAt : new Date(probedAt).toLocaleString();
    lastProbeMessage.value = `上次测试通过时间：${ts}${Number.isFinite(dim) && dim > 0 ? `；向量维度：${dim}` : ""}${
      Number.isFinite(maxInputTokens) && maxInputTokens > 0 ? `；最大输入：${maxInputTokens}` : ""
    }`;
  }
};

async function saveSettings() {
  try {
    if (modality.value === "embedding") {
      const curDim = Number.parseInt(String(currentState.value.dimensions || "0"), 10);
      if (!Number.isFinite(curDim) || curDim <= 0) {
        const fallbackDim = resolveEmbeddingDimensionsFromProfiles();
        if (fallbackDim > 0) {
          currentState.value.dimensions = fallbackDim;
        }
      }
    }
    const payload = buildPayloadForCurrentModality();
    await aiSettingsStore.saveSettings(payload);
    toast.add({
      title: "保存成功",
      description: `${currentTitle.value} 配置已更新`,
      color: "success",
    });
  } catch (error) {
    const message = getErrorMessage(error);
    toast.add({
      title: "保存失败",
      description: message,
      color: "error",
    });
    console.error("保存设置失败:", error);
  }
}

async function resetSettings() {
  const resetMap: Record<Modality, { provider: string; model: string }> = {
    llm: { provider: "openai", model: "gpt-4o-mini" },
    image: { provider: "openai", model: "gpt-image-1" },
    embedding: { provider: "openai", model: "text-embedding-3-small" },
    audio_tts: { provider: "openai", model: "gpt-4o-mini-tts" },
    audio_asr: { provider: "openai", model: "whisper-1" },
    video: { provider: "openai", model: "sora-preview" },
    model3d: { provider: "hunyuan", model: "HY-3D-Express" },
    rerank: { provider: "openai", model: "text-embedding-3-large" },
  };
  const cur = currentState.value as BaseConn;
  const def = resetMap[modality.value];

  // 先设置 provider
  cur.provider = def.provider;
  cur.app = "";
  // 拉取对应的模型列表
  await onProviderChanged(def.provider);
  // 最后设置默认模型
  cur.model = def.model;

  aiSettingsStore.lastTestMessage = "已恢复默认（当前模态）。";
}

async function testConnection() {
  try {
    const payload = buildPayloadForCurrentModality();
    const result = await aiSettingsStore.testConnection(
      currentState.value.provider || "",
      payload
    );
    if (modality.value === "embedding") {
      const dim = Number.parseInt(String(result?.dimensions || "0"), 10);
      if (Number.isFinite(dim) && dim > 0) {
        currentState.value.dimensions = dim;
      }
      // 测试通过后清空草稿，避免旧草稿覆盖探测值
      const draftKeyValue = draftKey(
        currentState.value.provider,
        env.value,
        modality.value
      );
      delete draftByProviderKey[draftKeyValue];
      // 记录“本次测试通过”的配置，保证刷新后仍能读取到探测结果
      const p = String(currentState.value.provider || "").trim();
      const m = String(currentState.value.model || "").trim();
      const a = String(currentState.value.app || "").trim();
      if (p && m) {
        try {
          await AISettingService.setActiveProfile({
            env: env.value,
            modality: "embedding",
            provider: p,
            model: a ? `${a}:${m}` : m,
          });
        } catch (error) {
          console.warn("测试连接后设置激活配置失败", error);
        }
      }
      try {
        await aiSettingsStore.fetchProfiles(env.value, ["embedding"]);
        const curProvider = String(currentState.value.provider || "").trim().toLowerCase();
        const curModel = String(currentState.value.model || "").trim();
        const profile = (aiSettingsStore.profiles || []).find(
          (p) =>
            String(p?.modality || "").toLowerCase() === "embedding" &&
            String(p?.provider || "").trim().toLowerCase() === curProvider &&
            matchProfileModel(p?.model, currentState.value.app, curModel)
        );
        if (profile) {
          const capCache = (profile as any).capCache || {};
          const probedAt = String(capCache?.probed_at || "").trim();
          const dimRaw = capCache?.dimensions ?? profile?.defaults?.dimensions ?? profile?.defaults?.dim;
          const dim = Number.parseInt(String(dimRaw || "0"), 10);
          const maxInputTokens = resolveEmbeddingMaxInputTokens(profile);
          if (Number.isFinite(dim) && dim > 0) {
            currentState.value.dimensions = dim;
          }
          if (Number.isFinite(maxInputTokens) && maxInputTokens > 0) {
            currentState.value.maxInputTokens = maxInputTokens;
          }
          if (profile?.defaults?.truncate) {
            currentState.value.truncate = String(profile.defaults.truncate);
          }
          if (profile?.defaults?.batch != null) {
            const batch = Number.parseInt(String(profile.defaults.batch), 10);
            if (Number.isFinite(batch) && batch > 0) currentState.value.batch = batch;
          }
          if (probedAt) {
            const ts = Number.isNaN(Date.parse(probedAt)) ? probedAt : new Date(probedAt).toLocaleString();
            lastProbeMessage.value = `上次测试通过时间：${ts}${Number.isFinite(dim) && dim > 0 ? `；向量维度：${dim}` : ""}${
              Number.isFinite(maxInputTokens) && maxInputTokens > 0 ? `；最大输入：${maxInputTokens}` : ""
            }`;
          }
        }
      } catch {}
    }
    // 测试成功后后端会自动保存该 provider 的凭据（不激活默认路由），这里刷新一下非敏感凭据元数据
    try {
      await aiSettingsStore.fetchCredentials(env.value);
    } catch {}
    toast.add({
      title: "连接测试成功",
      description: `${currentTitle.value} 连接正常（凭据已保存，未改变默认路由）`,
      color: "success",
    });
  } catch (error) {
    const message = getErrorMessage(error);
    toast.add({
      title: "连接测试失败",
      description: message,
      color: "error",
    });
    console.error("连接测试失败:", error);
  }
}

async function testQuickCall() {
  try {
    const payload = buildPayloadForCurrentModality(
      "Hello, this is a test message."
    );
    await aiSettingsStore.testQuickCall(
      currentState.value.provider || "",
      currentState.value.model || "",
      payload,
      "Hello, this is a test message."
    );
    toast.add({
      title: "快速调用成功",
      description: `${currentTitle.value} 已返回测试结果`,
      color: "success",
    });
  } catch (error) {
    const message = getErrorMessage(error);
    toast.add({
      title: "快速调用失败",
      description: message,
      color: "error",
    });
    console.error("快速调用测试失败:", error);
  }
}
async function refreshStateForEnvAndModality() {
  aiSettingsStore.setCurrentEnv(env.value);
  try {
    await aiSettingsStore.fetchProviders(modality.value, env.value);
  } catch (error) {
    toast.add({
      title: "加载 Provider 列表失败",
      description: getErrorMessage(error),
      color: "error",
    });
  }
  await loadActiveConfiguration();
  if (!currentState.value.provider) {
    loadExistingConfiguration();
  }
  if (currentState.value.provider) {
    await onProviderChanged(currentState.value.provider);
  }
  if (modality.value === "embedding") {
    try {
      await aiSettingsStore.fetchProfiles(env.value, ["embedding"]);
      syncEmbeddingProbeMessageFromProfiles();
    } catch {}
  }
}

async function handleEnvChange(nextEnv: string) {
  aiSettingsStore.setCurrentEnv(nextEnv);
  try {
    await aiSettingsStore.refreshEnvData(nextEnv);
  } catch (error) {
    toast.add({
      title: "环境加载失败",
      description: getErrorMessage(error),
      color: "error",
    });
  }
  await refreshStateForEnvAndModality();
}

// 页面初始化
onMounted(async () => {
  try {
    envStore.initialize();
    await aiSettingsStore.initialize(env.value);
    await refreshStateForEnvAndModality();
  } catch (error) {
    toast.add({
      title: "初始化失败",
      description: getErrorMessage(error),
      color: "error",
    });
    console.error("初始化AI设置页面失败:", error);
  }
});

// 加载现有配置
function loadExistingConfiguration() {
  const profile =
    aiSettingsStore.getProfileByModality?.(modality.value) ?? null;
  const credential = profile
    ? (aiSettingsStore.getCredentialByProvider?.(profile.provider) ?? null)
    : null;

  if (!profile || !credential) return; // 数据不齐就直接返回

  // profile.defaults 可能不存在，全部兜底
  const d = profile.defaults ?? {};
  currentState.value.provider = profile.provider ?? currentState.value.provider;
  if (profile.model != null) {
    const parsed = splitAppModelKey(profile.model);
    currentState.value.app = parsed.app;
    currentState.value.model = parsed.model;
  }
  currentState.value.maxTokens =
    d.maxTokens ?? currentState.value.maxTokens ?? 4096;
  currentState.value.stream = d.stream ?? currentState.value.stream ?? true;
  currentState.value.temperature =
    d.temperature ?? currentState.value.temperature ?? 0.7;
  currentState.value.topP = d.topP ?? currentState.value.topP ?? 1;

  // credential.data 也兜底
  const cd = credential.data ?? {};
  currentState.value.apiKey = cd.api_key ?? currentState.value.apiKey ?? "";
  currentState.value.secretId = cd.secret_id ?? currentState.value.secretId ?? "";
  // SecretKey 不会从后端回填（脱敏/不回传）
  currentState.value.secretKey = "";
  currentState.value.baseURL = cd.base_url ?? currentState.value.baseURL ?? "";
  currentState.value.organization =
    cd.organization ?? currentState.value.organization ?? "";
  currentState.value.region = cd.region ?? currentState.value.region ?? "";
  currentState.value.azureDeployment =
    cd.azure_deployment ?? currentState.value.azureDeployment;
}

// 获取当前激活的配置
async function loadActiveConfiguration() {
  // console.log("加载激活配置", { env: env.value, modality: modality.value });
  try {
    const activeData = await aiSettingsStore.fetchActiveProfile(
      env.value,
      modality.value
    );
    if (activeData && activeData.profile) {
      const profile = activeData.profile;
      const config = currentState.value;

      // 更新当前状态
      config.provider = profile.provider;
      if (profile.model != null) {
        const parsed = splitAppModelKey(profile.model);
        config.app = parsed.app;
        config.model = parsed.model;
      }

      // 更新默认参数
      if (profile.defaults) {
        config.maxTokens = profile.defaults.maxTokens ?? config.maxTokens;
        config.stream = profile.defaults.stream ?? config.stream;
        config.temperature = profile.defaults.temperature ?? config.temperature;
        config.topP = profile.defaults.topP ?? config.topP;
      }

      // console.log("激活配置加载成功", profile);

      // 加载对应的模型列表
      if (profile.provider) {
        await onProviderChanged(profile.provider);
      }

      if (modality.value === "embedding") {
        const capCache = (profile as any).capCache || {};
        const probedAt = String(capCache?.probed_at || "").trim();
        const dimRaw = capCache?.dimensions ?? profile?.defaults?.dimensions ?? profile?.defaults?.dim;
        const dim = Number.parseInt(String(dimRaw || "0"), 10);
        const maxInputTokens = resolveEmbeddingMaxInputTokens(profile);
        if (Number.isFinite(dim) && dim > 0) {
          currentState.value.dimensions = dim;
        }
        if (Number.isFinite(maxInputTokens) && maxInputTokens > 0) {
          currentState.value.maxInputTokens = maxInputTokens;
        }
        if (profile?.defaults?.truncate) {
          currentState.value.truncate = String(profile.defaults.truncate);
        }
        if (profile?.defaults?.batch != null) {
          const batch = Number.parseInt(String(profile.defaults.batch), 10);
          if (Number.isFinite(batch) && batch > 0) currentState.value.batch = batch;
        }
        if (probedAt) {
          const ts = Number.isNaN(Date.parse(probedAt)) ? probedAt : new Date(probedAt).toLocaleString();
          lastProbeMessage.value = `上次测试通过时间：${ts}${Number.isFinite(dim) && dim > 0 ? `；向量维度：${dim}` : ""}${
            Number.isFinite(maxInputTokens) && maxInputTokens > 0 ? `；最大输入：${maxInputTokens}` : ""
          }`;
        } else {
          lastProbeMessage.value = "";
        }
      } else {
        lastProbeMessage.value = "";
      }
    }
  } catch (error) {
    console.error("加载激活配置失败", error);
  }
}

// 监听 provider 改变（初始化已手动调用过）
watch(
  () => currentState.value.provider,
  (next, prev) => {
    const envSnapshot = env.value;
    const modalitySnapshot = modality.value;
    // 在 provider 切换前，先把旧 provider 的输入保存为草稿（避免切回时丢失）
    if (prev) {
      persistProviderDraft(prev, envSnapshot, modalitySnapshot);
    }
    if (next) onProviderChanged(next);
  },
  { immediate: false }
);

watch(
  () => [currentState.value.provider, modality.value, aiSettingsStore.providers] as const,
  () => {
    const ap = activeProviderForForm.value;
    const modes = ap?.auth?.modes ?? [];
    if (!Array.isArray(modes) || modes.length === 0) return;
    if (!currentState.value.authMode) {
      // 默认优先第一个（hunyuan.yaml 里把 openai 放第一个）
      currentState.value.authMode = modes[0].id;
    }
  }
);

watch(
  () => [currentState.value.provider, currentState.value.authMode, modality.value, aiSettingsStore.providers] as const,
  () => {
    const ap = activeProviderForForm.value;
    const modes = ap?.auth?.modes ?? [];
    if (!Array.isArray(modes) || modes.length === 0) return;

    const modeId = String(currentState.value.authMode || "").trim();
    if (!modeId) return;
    const hit = modes.find((m: any) => String(m?.id || "").trim() === modeId);
    const defBase = String(hit?.defaults?.base_url || "").trim();
    if (!defBase) return;

    const curBase = String(currentState.value.baseURL || "").trim();
    // 仅在明显需要时覆盖：空 / 误填 tc3 endpoint / 忘了 /v1
    const lower = curBase.toLowerCase();
    const isTC3 = lower.includes("tencentcloudapi.com");
    const isHunyuanOpenAIHost = lower.includes("hunyuan.cloud.tencent.com");
    const missingV1 = modeId === "openai" && curBase && !lower.includes("/v1");
    const needSwitchToTC3 = modeId === "tc3" && curBase && (isHunyuanOpenAIHost || lower.includes("/v1"));
    if (!curBase || (modeId === "openai" && (isTC3 || missingV1)) || needSwitchToTC3) {
      currentState.value.baseURL = defBase;
    }
  }
);

watch(
  () =>
    [currentState.value.provider, currentState.value.app, currentState.value.model, modality.value] as const,
  ([provider, _app, model, mod]) => {
    if (mod !== "embedding") return;
    lastProbeMessage.value = "";
    if (!provider || !model) return;
    aiSettingsStore
      .fetchProfiles(env.value, ["embedding"])
      .then(() => syncEmbeddingProbeMessageFromProfiles())
      .catch(() => {});
  }
);

watch(
  () => [currentState.value.provider, currentState.value.model, modality.value] as const,
  ([provider, model, mod]) => {
    if (mod !== "embedding" || !isHuggingFaceProvider(provider)) return;
    const resolved = resolveHuggingFaceBaseURL(model);
    if (!resolved) return;
    const curBase = String(currentState.value.baseURL || "").trim();
    const lower = curBase.toLowerCase();
    const isLegacyRouter = lower === "https://router.huggingface.co";
    const isDeprecatedInference = lower.includes("api-inference.huggingface.co");
    const isTemplate = curBase.includes("{model}");
    if (!curBase || isLegacyRouter || isDeprecatedInference || isTemplate) {
      currentState.value.baseURL = resolved;
    }
  }
);

// 监听模态切换，重新加载配置
watch(modality, async () => {
  await refreshStateForEnvAndModality();
});

watch(
  () => route.query?.modality,
  (next) => {
    const queryModality = resolveModalityFromQuery(next);
    if (queryModality) {
      modality.value = queryModality;
    }
  }
);

watch(
  () => env.value,
  async (next, prev) => {
    if (!next || next === prev) {
      return;
    }
    await handleEnvChange(next);
  }
);
</script>
