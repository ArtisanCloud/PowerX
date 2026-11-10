<template>
  <div class="space-y-6 p-4">
    <!-- 标题 -->
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <h1 class="text-lg font-semibold text-[var(--text-primary)]">
          {{ $t("menu.aiSettingsModel") }}
        </h1>
        <p class="text-sm text-[var(--text-secondary)]">
          {{ $t("settings.ai.description") }}
        </p>
      </div>
      <div class="flex flex-wrap items-center gap-2">
        <UButton
          color="primary"
          icon="i-heroicons-cloud-arrow-up"
          class="whitespace-nowrap"
          :loading="saving"
          :disabled="!String(currentState.model || '').trim() || selectedVisionUnsupported || imageParameterErrors.length > 0"
          data-testid="ai-save-settings"
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
          variant="soft"
          icon="i-heroicons-adjustments-horizontal"
          class="whitespace-nowrap"
          :to="contextOptimizerLink"
        >
          {{ $t("settings.ai.actions.openContextOptimizer") }}
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
      <div class="lg:col-span-1 min-w-0">
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
            <div class="flex gap-2 overflow-x-auto lg:flex-col lg:overflow-x-visible" :aria-label="$t('settings.ai.modalityTabsLabel')">
              <button
                v-for="tab in modalityTabs"
                :key="tab.key"
                class="shrink-0 lg:w-full flex items-center gap-3 px-3 py-2 text-left rounded-md transition-colors whitespace-nowrap"
                :aria-pressed="modality === tab.key"
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
          <ProviderModelForm
            :provider-options="providerOptions"
            :app-options="appOptions"
            :model-options="modelOptions"
            :active-provider="activeProviderForForm"
            :providers-loading="aiSettingsStore.providersLoading || configurationLoads > 0"
            :providers-error="aiSettingsStore.providersError"
            :models-loading="aiSettingsStore.modelsLoading || configurationLoads > 0"
            :models-error="aiSettingsStore.modelsError"
            :dynamic-models="isOllamaProvider(currentState.provider)"
            :catalog-source="aiSettingsStore.modelsCatalog?.catalog_source"
            :catalog-updated-at="aiSettingsStore.modelsCatalog?.updated_at"
            :usage-unspecified="selectedModelUsageUnspecified"
            :vision-mode="modality === 'vlm' && aiSettingsStore.modelsCatalog?.catalog_source === 'online'"
            :model-context="`${env}/${modality}/${currentState.provider}/${currentState.app || ''}`"
            :state="currentState"
            :has-saved-credentials="hasSavedProviderCredentials"
            @retry-providers="retryProviders"
            @retry-models="retryModels"
            @base-url-changed="retryModels"
            @update-online-models="openOnlineModels"
            @app-changed="onAppChanged"
          />
          <p v-if="savedModelMissing" role="status" class="mt-2 text-sm text-warning" data-testid="ai-saved-model-review">{{ $t('settings.ai.onlineModels.savedModelMissing') }}</p>
          <OnlineModelCatalogModal
            v-if="onlineModelsContext"
            v-bind="onlineModelsContext"
            @close="onlineModelsContext = null"
            @saved="onOnlineModelsSaved"
          />
        </div>

        <div
          class="rounded-lg border border-[var(--border-color)] bg-[var(--card-bg)] p-4"
        >
          <div class="mb-4 font-medium text-[var(--text-primary)]">
            {{ currentTitle }} - {{ $t("settings.ai.sections.parameters") }}
          </div>
          <fieldset class="min-w-0 border-0 m-0 p-0" :disabled="configurationLoads > 0 || aiSettingsStore.loading || !aiSettingsStore.initialized" :aria-busy="configurationLoads > 0 || aiSettingsStore.loading || !aiSettingsStore.initialized">
          <p v-for="error in imageParameterErrors" :key="error.parameter" role="alert" class="mb-3 text-sm text-error" data-testid="image-parameter-error">{{ $t('settings.ai.imageParameters.invalid', { parameter: error.parameter, value: error.value, allowed: error.allowed.join(' / ') }) }}</p>
          <ModalityParamsForm
            :parameter-profile="selectedParameterProfile"
            @reset-model-parameters="resetModelParameters"
            :active-modality="modality"
            :llm="llm"
            :vlm="vlm"
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
            :audio-format-options="audioFormatOptions"
            :audio-quality-options="audioQualityOptions"
            :language-options="languageOptions"
            :response-format-options="responseFormatOptions"
            :top-k-options="topKOptions"
          />
          </fieldset>
        </div>
      </div>

      <div class="space-y-6 lg:col-span-1">
        <TestPanel
          data-testid="ai-test-panel"
          :current-title="currentTitle"
          :current-state="currentState"
          :last-test-message="lastTestMessage"
          :last-test-detail="lastTestDetail"
          :connection-testing="aiSettingsStore.testingAction === 'connection'"
          :quick-testing="aiSettingsStore.testingAction === 'quick'"
          :quick-call-validates="modality !== 'llm'"
          :disabled="configurationLoads > 0 || aiSettingsStore.loading || !aiSettingsStore.initialized"
          :on-test-connection="testConnection"
          :on-test-quick-call="testQuickCall"
          :on-test-vision="modality === 'vlm' ? openVisionTrial : undefined"
        />

        <div
          class="rounded-lg border border-[var(--border-color)] bg-[var(--card-bg)] p-4 space-y-3"
        >
          <div class="flex items-center justify-between">
            <div class="font-medium text-[var(--text-primary)]">
              {{ $t("settings.ai.skillsSourcePolicy.title") }}
            </div>
            <UBadge
              :color="skillsPolicyEffectiveSource === 'tenant' ? 'primary' : 'neutral'"
              variant="soft"
            >
              {{
                skillsPolicyEffectiveSource === "tenant"
                  ? $t("settings.ai.skillsSourcePolicy.effectiveSource.tenant")
                  : $t("settings.ai.skillsSourcePolicy.effectiveSource.default")
              }}
            </UBadge>
          </div>
          <p class="text-xs text-[var(--text-secondary)]">
            {{ $t("settings.ai.skillsSourcePolicy.description") }}
          </p>

          <div class="space-y-2">
            <UCheckbox
              :model-value="hasSource('builtin')"
              :label="$t('settings.ai.skillsSourcePolicy.sources.builtin')"
              @update:model-value="toggleSource('builtin', $event)"
            />
            <UCheckbox
              :model-value="hasSource('plugin')"
              :label="$t('settings.ai.skillsSourcePolicy.sources.plugin')"
              @update:model-value="toggleSource('plugin', $event)"
            />
            <UCheckbox
              :model-value="hasSource('third_party')"
              :label="$t('settings.ai.skillsSourcePolicy.sources.thirdParty')"
              @update:model-value="toggleSource('third_party', $event)"
            />
          </div>

          <div class="text-xs text-[var(--text-secondary)]">
            {{ $t("settings.ai.skillsSourcePolicy.updatedAt") }}：{{
              skillsPolicyUpdatedAtText ||
              $t("settings.ai.skillsSourcePolicy.notConfigured")
            }}
          </div>

          <UButton
            color="primary"
            block
            :loading="skillsPolicySaving"
            :disabled="skillsPolicyLoading"
            @click="saveSkillsSourcePolicy"
          >
            {{ $t("settings.ai.skillsSourcePolicy.actions.save") }}
          </UButton>
        </div>
      </div>
    </div>
    <VisionTrialModal v-if="visionTrialContext" v-model:open="visionTrialOpen" :context="visionTrialContext"
      :scope="`${env}/${userStore.currentTenantUuid || ''}`" :blocked-reason="visionTrialBlockedReason" />
  </div>
</template>

<script setup lang="ts">
import { storeToRefs } from "pinia";
import ProviderModelForm from "~/components/settings/ai/ProviderModelForm.vue";
import OnlineModelCatalogModal from "~/components/settings/ai/OnlineModelCatalogModal.vue";
import ModalityParamsForm from "~/components/settings/ai/ModalityParamsForm.vue";
import TestPanel from "~/components/settings/ai/TestPanel.vue";
import VisionTrialModal from "~/components/settings/ai/VisionTrialModal.vue";
import { useAISettingsStore } from "~/stores/aiSettings";
import { useEnvStore, ENV_OPTIONS } from "~/stores/envStore";
import { agnesImageSizes, agnesVideoResolutions, isAgnesProvider, syncAgnesMediaSettings } from "~/utils/agnesMediaSettings";
import { useUserStore } from "~/stores/user";
import {
  AISettingService,
  type Provider,
  type SaveSettingsPayload,
  type OnlineModelCatalog,
  type VisionTrialContext,
} from "~/composables/api/services/aiSettingService";
import type { SelectOption } from "~/composables/api/types/select";
import { visionModelOptions, visionUsageKey, type VisionModelOption } from '~/utils/ai-vision-model-choice';
import { splitAIModelKey } from "~/utils/ai-model-key";
import { resolveAIFormBaseURL } from "~/utils/ai-provider-endpoint";
import { aiProviderErrorMessage } from "~/utils/ai-provider-error";
import { imageParameterOptions, invalidImageParameters, modelParameterDefaults, modelParameterKey, modelParameterPresets, modelSupportsParameter } from '~/utils/ai-model-parameters';

definePageMeta({
  title: "menu.aiSettingsModel",
  layout: "default",
});

type Modality =
  | "llm"
  | "vlm"
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
const { t } = useI18n({ useScope: "global" });
const localePath = useLocalePath();
const costGuardLink = computed(() => localePath("/settings/ai/cost"));
const connectorsLink = computed(() => localePath("/settings/ai/connectors"));
const contextOptimizerLink = computed(() =>
  localePath("/settings/ai/context-optimizer")
);
const registryLink = computed(() =>
  localePath("/settings/ai/capability-registry")
);

const route = useRoute();
const userStore = useUserStore();
const { isRoot, isCurrentTenantAdmin } = storeToRefs(userStore);
const allowTenantAISettings = computed(() => isRoot.value || isCurrentTenantAdmin.value);

/**
 * Tab & 环境
 */
const modalityTabs = [
  { key: "llm", label: "LLM 文本", icon: "i-heroicons-bars-3-bottom-left" },
  { key: "vlm", label: t("settings.ai.vision.title"), icon: "i-heroicons-eye" },
  { key: "image", label: t("settings.ai.catalog.imageModality"), icon: "i-heroicons-photo" },
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
  const compact = (raw: unknown) => {
    const message = String(raw ?? "").trim();
    if (!message) return "未知错误";
    const connectionCode = message.match(/AI_CONNECTION_[A-Z_]+/)?.[0];
    if (connectionCode) return t(`settings.ai.connectionTest.errors.${connectionCode}`);
    const ollamaNotFound = message.match(/OLLAMA_MODEL_NOT_FOUND\s+model=([^:\s]+)/i);
    if (ollamaNotFound?.[1]) {
      const model = ollamaNotFound[1];
      return t("settings.ai.errors.ollamaModelNotFound", { model });
    }
    const lower = message.toLowerCase();
    if (
      lower.includes("accessdenied.unpurchased") ||
      lower.includes("access to model denied")
    ) {
      return "模型未开通或无权限（AccessDenied.Unpurchased），请在官方控制台开通该模型后重试。";
    }
    if (
      lower.includes("invalid_api_key") ||
      lower.includes("incorrect api key provided")
    ) {
      return "API Key 无效，请确认使用的是对应站点/地域的官方 Key。";
    }
    const bodyPos = message.indexOf("body=");
    const brief = (aiProviderErrorMessage(message) || (bodyPos >= 0 ? message.slice(0, bodyPos) : message))
      .replace(/\s+/g, " ")
      .trim();
    return brief.length > 180 ? `${brief.slice(0, 180)}...` : brief;
  };

  if (!error) return "未知错误";
  if (typeof error === "string") {
    return compact(error);
  }
  if (typeof error === "object") {
    const anyError = error as Record<string, any>;
    return compact(
      anyError?.data?.message ||
      anyError?.response?.statusMessage ||
      anyError?.message ||
      anyError?.error ||
      "未知错误"
    );
  }
  return compact(error);
};

/**
 * Provider 列表与模型目录（从 store 获取）
 */
const providerOptions = computed<SelectOption[]>(() => {
  const providers = aiSettingsStore.providers;
  return providers.map((p: Provider) => ({
    label: p.name,
    value: p.id,
  }));
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
      (p: Provider) => p.id.toLowerCase() === pid.toLowerCase()
    ) ?? null;
  if (!hit) return null;
  return {
    id: hit.id,
    name: hit.name,
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
    maxConcurrentRequests: number;
    reasoningEffort: string;
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
  maxConcurrentRequests: 2,
  reasoningEffort: '',
});

const vlm = reactive<BaseConn & { maxTokens: number; reasoningEffort: string; temperature: number | null; topP: number | null; jsonMode: boolean }>({
 provider: "openai", app: "", model: null, authMode: "", apiKey: "", secretId: "", secretKey: "", baseURL: "", region: "", organization: "",
 maxTokens: 2048, reasoningEffort: "none", temperature: null, topP: null, jsonMode: false,
});
function loadVisionDefaults(defaults: Record<string, any>) {
 vlm.maxTokens = defaults.max_tokens ?? defaults.maxTokens ?? 2048;
 vlm.reasoningEffort = defaults.reasoning_effort ?? "none";
 vlm.temperature = defaults.temperature ?? null;
 vlm.topP = defaults.top_p ?? defaults.topP ?? null;
 vlm.jsonMode = defaults.json_mode ?? false;
}

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
  quality: "",
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
    case "vlm":
      return t("settings.ai.vision.title");
    case "image":
      return t("settings.ai.catalog.imageModality");
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

function stateForModality(value: string) {
  switch (value) {
    case "llm":
      return llm;
    case "vlm":
      return vlm;
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
}

const currentState = computed<any>({
  get() {
    return stateForModality(modality.value);
  },
  set(_v) {
    // 保持为对象引用，不整体替换
  },
});

const hasSavedProviderCredentials = computed(() => {
  if (aiSettingsStore.currentEnv !== env.value) return false;
  const provider = String(currentState.value.provider || "").trim();
  if (!provider) return false;
  const credential = aiSettingsStore.getCredentialByProvider(provider);
  if (!credential?.has_credentials) return false;
  const savedMode = String((credential.data as Record<string, unknown>)?.auth_mode || "").trim();
  return !activeProviderForForm.value?.auth?.modes?.length || savedMode === String(currentState.value.authMode || "").trim();
});

/**
 * ProviderModelForm 的 Model 下拉选项：从后端获取
 */
const loadedSavedModel = ref<{ env: string; modality: string; provider: string; model: string; app: string } | null>(null);
function rememberSavedModel(profile: { provider: string; model: string }, environment: string, mod: string) {
  const parsed = splitAppModelKey(profile.model, profile.provider);
  loadedSavedModel.value = { env: environment, modality: mod, provider: profile.provider, model: parsed.model, app: parsed.app };
}
const savedModelMissing = computed(() => {
  const state = currentState.value;
  const active = loadedSavedModel.value;
  if (!active || active.env !== env.value || active.modality !== modality.value || active.provider !== state.provider || isOllamaProvider(state.provider)) return false;
  return Boolean(state.model && active.model === state.model && active.app === String(state.app || '') && !aiSettingsStore.models.includes(state.model));
});
const modelOptions = computed<VisionModelOption[]>(() => {
  const models = aiSettingsStore.models;

  // 可选：去重 + 过滤空值
  const uniq = Array.from(new Set(models)).filter((m) => !!m);

  const options = modality.value === 'vlm' && aiSettingsStore.modelsCatalog?.catalog_source === 'online'
    ? visionModelOptions(uniq, aiSettingsStore.modelsCatalog.items)
    : uniq.map((model) => ({
    label: model,
    value: model,
  }));

  // 已保存配置不因目录复核被自动替换；仅作为当前选中值展示，不开放为新候选。
  return savedModelMissing.value ? [{ label: currentState.value.model, value: currentState.value.model, disabled: true }, ...options] : options;
});

const selectedVisionUsage = computed(() => modality.value === 'vlm'
  ? modelOptions.value.find(item => item.value === currentState.value.model)?.vision
  : undefined);
const selectedVisionUnsupported = computed(() => selectedVisionUsage.value?.status === 'unsupported');
const selectedParameterProfile = computed(() => {
  const state = currentState.value;
  const item = aiSettingsStore.modelsCatalog?.items?.find(item => item.id === state.model)?.parameter_profile;
  return item?.provider === state.provider && item.model === state.model ? item : undefined;
});
const selectedImageOperation = computed(() => modality.value === 'image'
  ? aiSettingsStore.modelsCatalog?.items?.find(item => item.id === image.model)?.operations?.image_generation
  : undefined);
const imageParameterErrors = computed(() => modality.value === 'image' ? invalidImageParameters(selectedImageOperation.value, image) : []);
const selectedModelUsageUnspecified = computed(() => {
  const catalog = aiSettingsStore.modelsCatalog;
  if (catalog?.catalog_source !== 'online' || !currentState.value.model) return false;
  if (modality.value === 'vlm') return selectedVisionUsage.value?.status !== 'supported' && !selectedVisionUnsupported.value;
  const item = catalog.items?.find(item => item.id === currentState.value.model);
  return !item?.reported_modalities?.length;
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
      (p: Provider) => p.id.toLowerCase() === pid.toLowerCase()
    ) ?? null;
  return hit?.apps ?? [];
}

function buildAppOptions(apps?: { id: string; name: string }[]) {
  if (!Array.isArray(apps) || apps.length === 0) return [];
  return apps.map((app) => ({
    label: app.name || app.id,
    value: app.id,
  }));
}

function isOllamaProvider(provider?: string | null) {
  return normalizeProviderKey(provider) === "ollama";
}

function splitAppModelKey(raw?: string | null, provider = currentState.value.provider) {
  return splitAIModelKey(raw, resolveAppsForProvider(provider));
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
      return [...base, "temperature", "maxTokens", "topP", "stream", "maxConcurrentRequests", "reasoningEffort"];
    case "vlm":
      return [...base, "maxTokens", "reasoningEffort", "temperature", "topP", "jsonMode"];
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
      state.maxConcurrentRequests = 2;
      if ("authMode" in state) state.authMode = "";
      break;
    case "vlm":
      state.maxTokens = 2048; state.reasoningEffort = "none"; state.temperature = null; state.topP = null; state.jsonMode = false;
      break;
    case "image":
      state.size = "1024x1024";
      state.quality = "";
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
  const savedVision = modalityVal === "vlm"
    ? aiSettingsStore.profiles.find(profile => {
        const model = splitAppModelKey(profile.model, profile.provider);
        return profile.modality === "vlm" && normalizeProviderKey(profile.provider) === p && model.model === state.model && model.app === String(state.app || "");
      })
    : null;
  applyModalityDefaults(state, modalityVal);
  // 激活画像先于供应商目录载入；不能再把它的视觉参数重置为清单默认值。
  if (savedVision) {
    loadVisionDefaults(savedVision.defaults ?? {});
    state.app = splitAppModelKey(savedVision.model, savedVision.provider).app;
  }

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
    if (!currentApp || !appIds.has(currentApp)) {
      currentState.value.app = String(apps[0]?.id || "").trim();
    }
  }

  const appValue = String(currentState.value.app || "").trim();
  try {
    // ✅ 直接传原始值，让 store 内部处理规范化
    const accepted = await aiSettingsStore.fetchModels(
      rawProvider,
      currentModality,
      env.value,
      appValue || undefined,
      resolveAIFormBaseURL(currentState.value.baseURL, activeProviderForForm.value, currentState.value.authMode)
    );
    if (!accepted) return;
  } catch (error) {
    console.error("获取模型列表失败:", error);
    // 读取失败不覆盖当前型号草稿，行内提示提供重试。
    return;
  }

  if (env.value !== envSnapshot || modality.value !== modalitySnapshot || currentState.value.provider !== rawProvider) return;

  reconcileModelSelection(currentState.value);
  if (modalitySnapshot === "embedding") {
    syncEmbeddingProbeMessageFromProfiles();
  }
  if (env.value === envSnapshot && modality.value === modalitySnapshot && currentState.value.provider === rawProvider) {
    syncAgnesMediaSettings(modalitySnapshot, currentState.value);
  }
}

async function onAppChanged(nextApp?: string) {
  const rawProvider = currentState.value.provider;
  const currentModality = modality.value;
  const envSnapshot = env.value;
  if (!rawProvider || !currentModality) return;
  if (nextApp !== undefined) {
    currentState.value.app = nextApp ?? "";
  }
  const apps = resolveAppsForProvider(rawProvider);
  if (apps.length) {
    const appIds = new Set(apps.map((app) => String(app.id)));
    const curApp = String(currentState.value.app || "").trim();
    if (!curApp || !appIds.has(curApp)) {
      currentState.value.app = String(apps[0]?.id || "").trim();
    }
  }
  const appValue = String(currentState.value.app || "").trim();
  try {
    const accepted = await aiSettingsStore.fetchModels(
      rawProvider,
      currentModality,
      env.value,
      appValue || undefined,
      resolveAIFormBaseURL(currentState.value.baseURL, activeProviderForForm.value, currentState.value.authMode)
    );
    if (!accepted) return;
  } catch (error) {
    console.error("获取模型列表失败:", error);
    return;
  }

  if (env.value !== envSnapshot || modality.value !== currentModality || currentState.value.provider !== rawProvider || currentState.value.app !== appValue) return;

  reconcileModelSelection(currentState.value);
  if (currentModality === "embedding") {
    syncEmbeddingProbeMessageFromProfiles();
  }
}

// 旧实现：syncCredentialFieldsForProvider（已用 restoreProviderDraft 替代，避免 apiKey/参数串台）

/**
 * 选项集合（传给 ModalityParamsForm）
 */
const imageSizeOptions = computed(() => imageParameterOptions(selectedImageOperation.value, 'size').length
  ? imageParameterOptions(selectedImageOperation.value, 'size') : isAgnesProvider(image.provider) ? agnesImageSizes : ['1024x1024']);
const imageQualityOptions = computed(() => {
  const options = imageParameterOptions(selectedImageOperation.value, 'quality');
  return imageParameterErrors.value.some(error => error.parameter === 'quality')
    ? [{ label: t('settings.ai.imageParameters.invalidOption', { value: image.quality }), value: image.quality, disabled: true }, ...options]
    : options;
});
const imageFormatOptions = computed(() => imageParameterOptions(selectedImageOperation.value, 'format').length
  ? imageParameterOptions(selectedImageOperation.value, 'format') : ['png', 'jpeg', 'webp']);
const truncateOptions = ["none", "start", "end"];
const videoResolutionOptions = computed(() => isAgnesProvider(video.provider) ? agnesVideoResolutions(video.model) : ["720p", "1080p", "4k"]);
const model3dFormatOptions = ["glb", "gltf", "obj", "fbx"];

// 新增音频TTS选项
const audioFormatOptions = ["mp3", "wav", "pcm", "opus", "aac", "flac"];
const audioQualityOptions = ["standard", "hd"];

// 新增音频ASR选项
const languageOptions = ["auto", "zh", "en", "ja", "ko", "es", "fr", "de"];
const responseFormatOptions = ["json", "text", "srt", "verbose_json", "vtt"];

// 新增重排序选项
const topKOptions = [5, 10, 20, 50, 100];

function buildPayloadForCurrentModality(promptOverride?: string, connectionProbe = false) {
  const clean = (value: unknown) => String(value ?? "").trim();
  const cleanSecret = (value: unknown) => {
    const text = clean(value);
    return /^[*•]{6,}$/.test(text) ? "" : text;
  };
  const baseConn = {
    provider: clean(currentState.value.provider),
    app: clean(currentState.value.app),
    model: clean(currentState.value.model),
    authMode: clean(currentState.value.authMode),
    apiKey: cleanSecret(currentState.value.apiKey),
    secretId: cleanSecret(currentState.value.secretId),
    secretKey: cleanSecret(currentState.value.secretKey),
    baseURL: clean(currentState.value.baseURL),
    organization: clean(currentState.value.organization),
    region: clean(currentState.value.region),
    azureDeployment: clean(currentState.value.azureDeployment),
  };

  let body: Record<string, any> = { ...baseConn };
  switch (modality.value) {
    case "llm":
      body = {
        ...baseConn,
        ...(modelSupportsParameter(selectedParameterProfile.value, 'temperature', llm.reasoningEffort) ? { temperature: llm.temperature } : {}),
        ...(llm.reasoningEffort ? { reasoningEffort: llm.reasoningEffort } : {}),
        maxTokens: currentState.value.maxTokens ?? 4096,
        ...(modelSupportsParameter(selectedParameterProfile.value, 'top_p', llm.reasoningEffort) ? { topP: llm.topP } : {}),
        stream:
          currentState.value.stream !== undefined
            ? currentState.value.stream
            : true,
        maxConcurrentRequests: currentState.value.maxConcurrentRequests,
      };
      break;
    case "vlm": {
      const saved = aiSettingsStore.getCredentialByProvider(vlm.provider)?.data as Record<string, any> | undefined;
      const sameURL = (value: unknown) => resolveAIFormBaseURL(value, activeProviderForForm.value, vlm.authMode).replace(/\/+$/, "");
      const reuse = !connectionProbe && !!saved && !baseConn.apiKey && !baseConn.secretId && !baseConn.secretKey && sameURL(baseConn.baseURL) === sameURL(saved.base_url) && baseConn.organization === clean(saved.organization) && baseConn.region === clean(saved.region);
      const modern = Boolean(selectedParameterProfile.value?.reasoning_efforts?.length) || vlm.provider === "openai" && vlm.model === "gpt-6-luna";
      body = {
        ...(reuse ? { provider: baseConn.provider, app: baseConn.app, model: baseConn.model, reuseCredential: true } : baseConn),
        maxTokens: vlm.maxTokens, jsonMode: vlm.jsonMode,
        ...(modern ? { reasoningEffort: vlm.reasoningEffort } : {}),
        ...((!modern || vlm.reasoningEffort === "none") && clean(vlm.temperature) !== "" ? { temperature: Number(vlm.temperature) } : {}),
        ...((!modern || vlm.reasoningEffort === "none") && clean(vlm.topP) !== "" ? { topP: Number(vlm.topP) } : {}),
      };
      break;
    }
    case "image":
      body = {
        ...baseConn,
        size: image.size,
        ...(isAgnesProvider(image.provider) ? {} : { ...(image.quality ? { quality: image.quality } : {}), format: image.format, promptHint: image.promptHint }),
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
        maxDurationSec: video.maxDurationSec,
        ...(isAgnesProvider(video.provider) ? {} : { fps: video.fps, promptHint: video.promptHint }),
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
const lastTestDetail = computed(() => aiSettingsStore.lastTestDetail || "");
// A test belongs to one form snapshot. Switching context, editing the form or
// leaving the page invalidates both displayed and in-flight feedback.
let testContextRevision = 0;
function invalidateTestFeedback() {
  testContextRevision++;
  aiSettingsStore.clearTestMessage();
  lastProbeMessage.value = "";
}
invalidateTestFeedback();
watch([env, modality, () => currentState.value], invalidateTestFeedback, { deep: true, flush: "sync" });
onUnmounted(invalidateTestFeedback);
const skillsPolicyLoading = ref(false);
const skillsPolicySaving = ref(false);
const skillsSourceAllowlist = ref<string[]>([]);
const skillsPolicyEffectiveSource = ref<"tenant" | "default" | string>("default");
const skillsPolicyUpdatedAt = ref<string>("");
const skillsPolicyUpdatedAtText = computed(() => {
  const raw = String(skillsPolicyUpdatedAt.value || "").trim();
  if (!raw) return "";
  const ts = Date.parse(raw);
  if (Number.isNaN(ts)) return raw;
  return new Date(ts).toLocaleString();
});

const normalizeSkillsAllowlist = (values: string[]) => {
  const allowed = new Set(["builtin", "plugin", "third_party"]);
  const out: string[] = [];
  const seen = new Set<string>();
  for (const item of values || []) {
    const value = String(item || "").trim().toLowerCase();
    if (!allowed.has(value) || seen.has(value)) continue;
    seen.add(value);
    out.push(value);
  }
  return out;
};

const hasSource = (source: string) =>
  skillsSourceAllowlist.value.includes(String(source || "").trim().toLowerCase());

const toggleSource = (source: string, checked: boolean | "indeterminate") => {
  const key = String(source || "").trim().toLowerCase();
  const next = new Set(skillsSourceAllowlist.value);
  if (checked === true) {
    next.add(key);
  } else {
    next.delete(key);
  }
  skillsSourceAllowlist.value = normalizeSkillsAllowlist(Array.from(next));
};

const loadSkillsSourcePolicy = async () => {
  skillsPolicyLoading.value = true;
  try {
    const view = await AISettingService.getSkillsSourcePolicy();
    skillsSourceAllowlist.value = normalizeSkillsAllowlist(view.allowlist || []);
    skillsPolicyEffectiveSource.value = view.effective_source || "default";
    skillsPolicyUpdatedAt.value = view.updated_at || "";
    if (skillsSourceAllowlist.value.length === 0) {
      skillsSourceAllowlist.value = ["builtin", "plugin", "third_party"];
    }
  } catch (error) {
    toast.add({
      title: t("settings.ai.skillsSourcePolicy.toasts.loadFailed"),
      description: getErrorMessage(error),
      color: "error",
    });
  } finally {
    skillsPolicyLoading.value = false;
  }
};

const saveSkillsSourcePolicy = async () => {
  const allowlist = normalizeSkillsAllowlist(skillsSourceAllowlist.value);
  if (allowlist.length === 0) {
    toast.add({
      title: t("settings.ai.skillsSourcePolicy.toasts.saveFailed"),
      description: t("settings.ai.skillsSourcePolicy.toasts.saveValidationFailed"),
      color: "error",
    });
    return;
  }
  skillsPolicySaving.value = true;
  try {
    const view = await AISettingService.setSkillsSourcePolicy(allowlist);
    skillsSourceAllowlist.value = normalizeSkillsAllowlist(view.allowlist || allowlist);
    skillsPolicyEffectiveSource.value = view.effective_source || "tenant";
    skillsPolicyUpdatedAt.value = view.updated_at || "";
    toast.add({
      title: t("settings.ai.skillsSourcePolicy.toasts.saveSuccess"),
      description: t("settings.ai.skillsSourcePolicy.toasts.saveSuccessDesc"),
      color: "success",
    });
  } catch (error) {
    toast.add({
      title: t("settings.ai.skillsSourcePolicy.toasts.saveFailed"),
      description: getErrorMessage(error),
      color: "error",
    });
  } finally {
    skillsPolicySaving.value = false;
  }
};

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
  if (selectedVisionUnsupported.value || imageParameterErrors.value.length) return;
  const targetState = currentState.value;
  const targetProvider = targetState.provider;
  const targetEnv = env.value;
  const targetModality = modality.value;
  const targetTitle = currentTitle.value;
  const submittedSecrets = { apiKey: targetState.apiKey, secretKey: targetState.secretKey };
  const submittedModel = { env: targetEnv, modality: targetModality, provider: targetProvider, model: String(targetState.model || ''), app: String(targetState.app || '') };
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
    const result = await aiSettingsStore.saveSettings(payload);
    if (!result?.ok) throw new Error("保存接口未确认成功，请重试。");
    // Store 已完成保存及凭据回读；已提交的密钥不再作为未保存草稿。
    // 只清除同一上下文、且仍与提交时一致的值，保留等待期间的新输入。
    const clearSubmittedSecrets = (state: Record<string, any> | undefined) => {
      if (!state) return;
      for (const field of ["apiKey", "secretKey"] as const) {
        if (state[field] === submittedSecrets[field]) state[field] = "";
      }
    };
    clearSubmittedSecrets(draftByProviderKey[draftKey(targetProvider, targetEnv, targetModality)]);
    if (env.value === targetEnv && modality.value === targetModality && targetState.provider === targetProvider) {
      clearSubmittedSecrets(targetState);
      loadedSavedModel.value = submittedModel;
    }
    toast.add({
      title: "保存成功",
      description: `${targetTitle} 配置已更新`,
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
    vlm: { provider: "openai", model: "gpt-6-luna" },
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
  cur.model = aiSettingsStore.models.includes(def.model) ? def.model : aiSettingsStore.models[0] || "";
  resetModelParameters();

  aiSettingsStore.lastTestMessage = "已恢复默认（当前模态）。";
}

async function testConnection() {
  if (aiSettingsStore.testing) return;
  const targetEnv = env.value;
  const targetTitle = currentTitle.value;
  const targetRevision = testContextRevision;
  try {
    const result = await aiSettingsStore.testConnection(
      currentState.value.provider || "",
      buildPayloadForCurrentModality(undefined, true)
    );
    if (targetRevision !== testContextRevision) return;
    const message = t(result.verification?.api_key_verified
      ? "settings.ai.connectionTest.successDetail"
      : "settings.ai.connectionTest.unauthenticatedDetail");
    aiSettingsStore.lastTestMessage = message;
    // 仅刷新非敏感凭据元数据，不遮挡页面，不激活配置、不探测向量维度。
    if (env.value === targetEnv) {
      try {
        await aiSettingsStore.fetchCredentials(targetEnv, { useGlobalLoading: false });
        if (aiSettingsStore.modelsError || !aiSettingsStore.models.length) await retryModels();
      } catch {}
    }
    if (targetRevision === testContextRevision) {
      toast.add({ title: t("settings.ai.connectionTest.success"), description: `${targetTitle}: ${message}`, color: "success" });
    }
  } catch (error) {
    if (targetRevision !== testContextRevision) return;
    const message = getErrorMessage(error);
    aiSettingsStore.lastTestMessage = message;
    toast.add({ title: t("settings.ai.connectionTest.failure"), description: message, color: "error" });
  }
}

async function testQuickCall() {
  if (aiSettingsStore.testing) return;
  const validates = modality.value !== "llm";
  const targetRevision = testContextRevision;
  try {
    const payload = buildPayloadForCurrentModality("Hello, this is a test message.");
    const result = await aiSettingsStore.testQuickCall(
      currentState.value.provider || "",
      currentState.value.model || "",
      payload,
      "Hello, this is a test message."
    );
    if (targetRevision !== testContextRevision) return;
    const message = validates
      ? t("settings.ai.connectionTest.validateDetail")
      : String(result?.result || "");
    aiSettingsStore.lastTestMessage = message;
    toast.add({
      title: t(validates ? "settings.ai.connectionTest.validateSuccess" : "settings.ai.connectionTest.quickSuccess"),
      description: message,
      color: "success",
    });
  } catch (error) {
    if (targetRevision !== testContextRevision) return;
    const message = getErrorMessage(error);
    aiSettingsStore.lastTestMessage = message;
    toast.add({ title: t("settings.ai.connectionTest.failure"), description: message, color: "error" });
  }
}
const configurationLoads = ref(0);
const visionTrialOpen = ref(false);
const visionTrialContext = ref<VisionTrialContext | null>(null);
const visionTrialBlockedReason = computed(() => {
  if (selectedVisionUnsupported.value) return t(visionUsageKey(selectedVisionUsage.value));
  return !vlm.provider || !vlm.model ? t('settings.ai.visionTrial.selectModel') : '';
});
function openVisionTrial() {
  const params = buildPayloadForCurrentModality().vlm!;
  const next: VisionTrialContext = { env: env.value, provider: vlm.provider, model: vlm.model, app: String(vlm.app || ''),
    connection: { baseURL: String(vlm.baseURL || '').trim(), authMode: String(vlm.authMode || '').trim(), organization: String(vlm.organization || '').trim(), region: String(vlm.region || '').trim(), secretId: String(vlm.secretId || '').trim(), azureDeployment: String(vlm.azureDeployment || '').trim(),
      ...(String(vlm.apiKey || '').trim() ? { apiKey: String(vlm.apiKey).trim() } : {}), ...(String(vlm.secretKey || '').trim() ? { secretKey: String(vlm.secretKey).trim() } : {}) },
    params: { maxTokens: params.maxTokens, jsonMode: Boolean(params.jsonMode),
      ...(params.reasoningEffort ? { reasoningEffort: params.reasoningEffort } : {}),
      ...(params.temperature != null ? { temperature: params.temperature } : {}),
      ...(params.topP != null ? { topP: params.topP } : {}) } };
  if (JSON.stringify(next) !== JSON.stringify(visionTrialContext.value)) visionTrialContext.value = next;
  visionTrialOpen.value = true;
}
watch([env, modality, () => currentState.value, () => userStore.currentTenantUuid], () => { visionTrialOpen.value = false; }, { deep: true, flush: 'sync' });
let configurationRequestID = 0;
async function refreshStateForEnvAndModality() {
  const requestID = ++configurationRequestID;
  configurationLoads.value++;
  try {
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
    if (requestID !== configurationRequestID) return;
    await loadActiveConfiguration();
    if (requestID !== configurationRequestID) return;
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
  } finally {
    configurationLoads.value--;
  }
}

async function retryProviders() {
  try {
    await aiSettingsStore.fetchProviders(modality.value, env.value);
  } catch {
    // 行内失败状态保留，用户可以再次重试。
  }
}

function reconcileModelSelection(state: Record<string, any>) {
  const models = aiSettingsStore.models ?? [];
  if (state.model && models.includes(state.model)) return;
  if (savedModelMissing.value) return;
  // 私有实例成功返回后，已消失的型号清空，交由用户重新选择。
  state.model = isOllamaProvider(state.provider) ? "" : modality.value === 'vlm' && aiSettingsStore.modelsCatalog?.catalog_source === 'online'
    ? String(visionModelOptions(models, aiSettingsStore.modelsCatalog.items).find(item => item.vision?.status === 'supported')?.value || '')
    : models[0] || "";
}

async function retryModels() {
  const state = currentState.value;
  const provider = state.provider;
  const mod = modality.value;
  const environment = env.value;
  try {
    const accepted = await aiSettingsStore.fetchModels(provider, mod, environment, state.app, resolveAIFormBaseURL(state.baseURL, activeProviderForForm.value, state.authMode));
    if (accepted && state === currentState.value && provider === state.provider && mod === modality.value && environment === env.value) {
      reconcileModelSelection(state);
    }
  } catch {
    // 重试目录读取不覆盖当前表单和凭据。
  }
}

const onlineModelsContext = ref<{ provider: string; providerName: string; env: string; app: string; modality: string; connectionDirty: boolean } | null>(null);

function openOnlineModels() {
  const state = currentState.value;
  const saved = aiSettingsStore.getCredentialByProvider(state.provider)?.data as Record<string, any> | undefined;
  const effectiveBase = (value: unknown) => resolveAIFormBaseURL(value, activeProviderForForm.value, state.authMode).replace(/\/+$/, '');
  const connectionDirty = Boolean(String(state.apiKey || '').trim()) ||
    effectiveBase(state.baseURL) !== effectiveBase(saved?.base_url) ||
    String(state.organization || '').trim() !== String(saved?.organization || '').trim();
  onlineModelsContext.value = { provider: state.provider, providerName: activeProviderForForm.value?.name || state.provider, env: env.value, app: String(state.app || ''), modality: modality.value, connectionDirty };
}

async function onOnlineModelsSaved(result: OnlineModelCatalog) {
  const context = onlineModelsContext.value;
  if (!context || result.provider !== currentState.value.provider || result.env !== env.value || result.app !== String(currentState.value.app || '') || context.modality !== modality.value) return;
  // 同步目录不自动换型号、不清空参数或用户草稿。
  try { await aiSettingsStore.fetchModels(result.provider, context.modality, result.env, result.app, currentState.value.baseURL); } catch { /* 已保存结果保留在弹窗中，目录读取可重试。 */ }
}

watch([env, modality, () => currentState.value.provider, () => currentState.value.app], () => { onlineModelsContext.value = null; });

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
    const queryModality = resolveModalityFromQuery(route.query?.modality);
    if (queryModality) {
      modality.value = queryModality;
    }
    if (!userStore.context) {
      await userStore.fetchUserContext();
    }
    if (!allowTenantAISettings.value) {
      await navigateTo("/dashboard");
      return;
    }
    envStore.initialize();
    await aiSettingsStore.initialize(env.value);
    await refreshStateForEnvAndModality();
    await loadSkillsSourcePolicy();
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
  rememberSavedModel(profile, env.value, modality.value);

  // profile.defaults 可能不存在，全部兜底
  const d = profile.defaults ?? {};
  currentState.value.provider = profile.provider ?? currentState.value.provider;
  if (profile.model != null) {
    const parsed = splitAppModelKey(profile.model, profile.provider);
    currentState.value.app = parsed.app;
    currentState.value.model = parsed.model;
  }
  currentState.value.maxTokens =
    d.maxTokens ?? currentState.value.maxTokens ?? 4096;
  currentState.value.stream = d.stream ?? currentState.value.stream ?? true;
  currentState.value.temperature =
    d.temperature ?? currentState.value.temperature ?? 0.7;
  currentState.value.topP = d.topP ?? currentState.value.topP ?? 1;
  currentState.value.maxConcurrentRequests = d.max_concurrent_requests ?? currentState.value.maxConcurrentRequests ?? 2;
  if (modality.value === "vlm") loadVisionDefaults(d);

  // credential.data 也兜底
  const cd = credential.data ?? {};
  // 密钥只在后端解析；页面保留当前草稿，使用状态提示表达凭据复用。
  currentState.value.apiKey = currentState.value.apiKey ?? "";
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
  const environment = env.value;
  const mod = modality.value;
  // console.info("加载激活配置", { env: env.value, modality: modality.value });
  try {
    const activeData = await aiSettingsStore.fetchActiveProfile(
      environment,
      mod
    );
    if (environment !== env.value || mod !== modality.value) return;
    if (activeData && activeData.profile) {
      const profile = activeData.profile;
      rememberSavedModel(profile, environment, mod);
      const config = currentState.value;

      // 更新当前状态
      config.provider = profile.provider;
      if (profile.model != null) {
        const parsed = splitAppModelKey(profile.model, profile.provider);
        config.app = parsed.app;
        config.model = parsed.model;
      }

      // 更新默认参数
      if (profile.defaults) {
        config.maxTokens = profile.defaults.maxTokens ?? config.maxTokens;
        config.stream = profile.defaults.stream ?? config.stream;
        config.temperature = profile.defaults.temperature ?? config.temperature;
        config.topP = profile.defaults.topP ?? config.topP;
        config.maxConcurrentRequests = profile.defaults.max_concurrent_requests ?? config.maxConcurrentRequests;
        if (mod === "vlm") loadVisionDefaults(profile.defaults);
      }

      // console.info("激活配置加载成功", profile);

      if (config.provider === profile.provider && config.model === splitAppModelKey(profile.model).model) {
        syncAgnesMediaSettings(modality.value, config, profile.defaults ?? {});
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
// 参数草稿以型号为单位；连接草稿继续由 Provider 管理，密钥不进入型号缓存。
const parameterDrafts = new Map<string, Record<string, any>>();
let activeParameterKey = '';
let applyingParameters = false;
const currentParameterKey = computed(() => modelParameterKey(env.value, modality.value, String(currentState.value.provider || ''), String(currentState.value.app || ''), String(currentState.value.model || '')));
const parameterFields = computed(() => Object.keys(modelParameterPresets[modality.value] || {}));
function resetModelParameters() {
  applyingParameters = true;
  try {
    parameterDrafts.delete(currentParameterKey.value);
    Object.assign(currentState.value, modelParameterDefaults(modality.value, selectedParameterProfile.value, undefined, selectedImageOperation.value));
  } finally { applyingParameters = false; }
}
watch(() => parameterFields.value.map(key => (currentState.value as any)[key]), () => {
  if (!applyingParameters && !configurationLoads.value && activeParameterKey === currentParameterKey.value) {
    parameterDrafts.set(activeParameterKey, Object.fromEntries(parameterFields.value.map(key => [key, (currentState.value as any)[key]])));
  }
}, { flush: 'sync' });
watch([currentParameterKey, selectedParameterProfile, selectedImageOperation, configurationLoads], () => {
  if (configurationLoads.value || !currentState.value.model || !parameterFields.value.length) return;
  const key = currentParameterKey.value;
  if (key === activeParameterKey && parameterDrafts.has(key)) return;
  const state = currentState.value;
  const saved = aiSettingsStore.profiles.find(profile => profile.modality === modality.value && profile.provider === state.provider && matchProfileModel(profile.model, state.app, state.model));
  applyingParameters = true;
  try {
    Object.assign(state, parameterDrafts.get(key) || modelParameterDefaults(modality.value, selectedParameterProfile.value, saved?.defaults, selectedImageOperation.value));
    activeParameterKey = key;
  } finally { applyingParameters = false; }
}, { flush: 'post' });
watch(() => userStore.currentTenantUuid, () => { parameterDrafts.clear(); activeParameterKey = ''; });

watch(
  () => [currentState.value.provider, currentState.value.model, modality.value] as const,
  () => syncAgnesMediaSettings(modality.value, currentState.value),
);

watch(
  () => [currentState.value.provider, modality.value, env.value] as const,
  ([next, mod, environment], [prev, previousMod, previousEnv]) => {
    // 模态／环境切换由配置载入流程处理；不能把新分区的默认值存入旧 Provider 草稿。
    if (mod !== previousMod || environment !== previousEnv || configurationLoads.value) return;
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
    if ((modeId === "openai" && (isTC3 || missingV1)) || needSwitchToTC3) {
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
watch(modality, async (_next, previous) => {
  const previousState = stateForModality(previous);
  persistProviderDraft(previousState.provider, env.value, previous, previousState);
  await refreshStateForEnvAndModality();
}, { flush: "sync" });

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
    persistProviderDraft(currentState.value.provider, prev, modality.value);
    await handleEnvChange(next);
  }
);
</script>
