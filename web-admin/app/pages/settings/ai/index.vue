<template>
  <div class="space-y-6 p-4">
    <!-- 标题 -->
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-lg font-semibold text-[var(--text-primary)]">
          大模型设置
        </h1>
        <p class="text-sm text-[var(--text-secondary)]">
          配置系统使用的多模态模型供应商与参数（LLM / 图像 / 向量 / 视频）
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
          保存
        </UButton>
        <UButton
          variant="ghost"
          icon="i-heroicons-arrow-path"
          class="whitespace-nowrap"
          @click="resetSettings"
        >
          重置
        </UButton>
      </div>
    </div>

    <div class="grid grid-cols-1 lg:grid-cols-4 gap-6">
      <!-- 左侧：垂直Tab导航 -->
      <div class="lg:col-span-1">
        <div class="space-y-4">
          <!-- 环境选择 -->
          <div
            class="rounded-lg border border-[var(--border-color)] bg-[var(--card-bg)] p-4"
          >
            <div class="mb-3 text-sm font-medium text-[var(--text-primary)]">
              环境配置
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

          <!-- 垂直Tab -->
          <div
            class="rounded-lg border border-[var(--border-color)] bg-[var(--card-bg)] p-4"
          >
            <div class="mb-3 text-sm font-medium text-[var(--text-primary)]">
              模态类型
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

      <!-- 中间：表单 -->
      <div class="lg:col-span-2 space-y-6">
        <!-- Provider/Model/凭证（随当前模态绑定） -->
        <div
          class="rounded-lg border border-[var(--border-color)] bg-[var(--card-bg)] p-4"
        >
          <div class="mb-4 font-medium text-[var(--text-primary)]">
            {{ currentTitle }} - 通用
          </div>
          <ProviderModelForm
            :provider-options="providerOptions"
            :model-options="modelOptions"
            :state="currentState"
            @provider-changed="onProviderChanged"
          />
        </div>

        <!-- 参数配置（随模态切换） -->
        <div
          class="rounded-lg border border-[var(--border-color)] bg-[var(--card-bg)] p-4"
        >
          <div class="mb-4 font-medium text-[var(--text-primary)]">
            {{ currentTitle }} - 参数
          </div>
          <ModalityParamsForm
            :active-modality="modality"
            :llm="llm"
            :image="image"
            :embedding="embedding"
            :audio-tts="audioTTS"
            :audio-asr="audioASR"
            :video="video"
            :rerank="rerank"
            :image-size-options="imageSizeOptions"
            :image-quality-options="imageQualityOptions"
            :image-format-options="imageFormatOptions"
            :truncate-options="truncateOptions"
            :video-resolution-options="videoResolutionOptions"
            :voice-options="voiceOptions"
            :audio-format-options="audioFormatOptions"
            :audio-quality-options="audioQualityOptions"
            :language-options="languageOptions"
            :response-format-options="responseFormatOptions"
            :top-k-options="topKOptions"
          />
        </div>
      </div>

      <!-- 右侧：测试 -->
      <div class="lg:col-span-1 space-y-6">
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
import ProviderModelForm from "~/components/settings/ai/ProviderModelForm.vue";
import ModalityParamsForm from "~/components/settings/ai/ModalityParamsForm.vue";
import TestPanel from "~/components/settings/ai/TestPanel.vue";
import { useAISettingsStore } from "~/stores/aiSettings";
import { useEnvStore, ENV_OPTIONS } from "~/stores/envStore";
import type {
  Provider,
  SaveSettingsPayload,
} from "~/composables/api/services/aiSettingService";
import type { SelectOption } from "~/composables/api/types/select";

type Modality =
  | "llm"
  | "image"
  | "embedding"
  | "audio_tts"
  | "audio_asr"
  | "video"
  | "rerank";

// 使用 AI 设置 store
const aiSettingsStore = useAISettingsStore();

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
  { key: "rerank", label: "重排序", icon: "i-heroicons-arrows-up-down" },
] as const;

const modality = ref<Modality>("llm");

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
  model: string | null;
  apiKey: string;
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
  model: null,
  apiKey: "",
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
  provider: "OpenAI",
  model: "gpt-image-1",
  apiKey: "",
  baseURL: "",
  region: "",
  organization: "",
  size: "1024x1024",
  quality: "standard",
  format: "png",
  promptHint: "",
});

const embedding = reactive<
  BaseConn & { dimensions: number; truncate: string; batch: number }
>({
  provider: "OpenAI",
  model: "text-embedding-3-small",
  apiKey: "",
  baseURL: "",
  region: "",
  organization: "",
  dimensions: 1536,
  truncate: "none",
  batch: 32,
});

const audioTTS = reactive<
  BaseConn & {
    voice: string;
    speed: number;
    format: string;
    quality: string;
  }
>({
  provider: "OpenAI",
  model: "tts-1",
  apiKey: "",
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
  provider: "OpenAI",
  model: "whisper-1",
  apiKey: "",
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
  provider: "OpenAI",
  model: "sora-preview",
  apiKey: "",
  baseURL: "",
  region: "",
  organization: "",
  resolution: "1080p",
  fps: 24,
  maxDurationSec: 10,
  promptHint: "",
});

const rerank = reactive<
  BaseConn & {
    topK: number;
    returnDocuments: boolean;
    maxChunksPerDoc: number;
  }
>({
  provider: "OpenAI",
  model: "text-embedding-3-large",
  apiKey: "",
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

async function onProviderChanged(nextProvider?: string) {
  const rawProvider = nextProvider ?? currentState.value.provider;
  const currentModality = modality.value;

  // 关键：参数不全就短路，但不清空 models
  if (!rawProvider || !currentModality) {
    return;
  }

  try {
    // ✅ 直接传原始值，让 store 内部处理规范化
    await aiSettingsStore.fetchModels(rawProvider, currentModality, env.value);
    const models = aiSettingsStore.models ?? [];
    if (models.length && !models.includes(currentState.value.model)) {
      currentState.value.model = models[0];
    }
  } catch (error) {
    console.error("获取模型列表失败:", error);
    // 这里不清空，保持上一次成功值
  }
}

/**
 * 选项集合（传给 ModalityParamsForm）
 */
const imageSizeOptions = ["256x256", "512x512", "1024x1024"];
const imageQualityOptions = ["standard", "hd"];
const imageFormatOptions = ["png", "jpeg", "webp"];
const truncateOptions = ["none", "start", "end"];
const videoResolutionOptions = ["720p", "1080p", "4k"];

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
    model: currentState.value.model ?? "",
    apiKey: currentState.value.apiKey ?? "",
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
const lastTestMessage = computed(() => aiSettingsStore.lastTestMessage);

async function saveSettings() {
  try {
    const payload = buildPayloadForCurrentModality();
    await aiSettingsStore.saveSettings(payload);
  } catch (error) {
    console.error("保存设置失败:", error);
  }
}

async function resetSettings() {
  const resetMap: Record<Modality, { provider: string; model: string }> = {
    llm: { provider: "OpenAI", model: "gpt-4o-mini" },
    image: { provider: "OpenAI", model: "dall-e-3" },
    embedding: { provider: "OpenAI", model: "text-embedding-3-small" },
    audio_tts: { provider: "OpenAI", model: "tts-1" },
    audio_asr: { provider: "OpenAI", model: "whisper-1" },
    video: { provider: "OpenAI", model: "sora-preview" },
    rerank: { provider: "OpenAI", model: "text-embedding-3-large" },
  };
  const cur = currentState.value as BaseConn;
  const def = resetMap[modality.value];

  // 先设置 provider
  cur.provider = def.provider;
  // 拉取对应的模型列表
  await onProviderChanged(def.provider);
  // 最后设置默认模型
  cur.model = def.model;

  aiSettingsStore.lastTestMessage = "已恢复默认（当前模态）。";
}

async function testConnection() {
  try {
    const payload = buildPayloadForCurrentModality();
    await aiSettingsStore.testConnection(
      currentState.value.provider || "",
      payload
    );
  } catch (error) {
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
  } catch (error) {
    console.error("快速调用测试失败:", error);
  }
}
async function refreshStateForEnvAndModality() {
  await loadActiveConfiguration();
  if (!currentState.value.provider) {
    loadExistingConfiguration();
  }
  if (currentState.value.provider) {
    await onProviderChanged(currentState.value.provider);
  }
}

// 页面初始化
onMounted(async () => {
  try {
    envStore.initialize();
    await aiSettingsStore.initialize();
    await refreshStateForEnvAndModality();
  } catch (error) {
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
  currentState.value.model = profile.model ?? currentState.value.model;
  currentState.value.maxTokens =
    d.maxTokens ?? currentState.value.maxTokens ?? 4096;
  currentState.value.stream = d.stream ?? currentState.value.stream ?? true;
  currentState.value.temperature =
    d.temperature ?? currentState.value.temperature ?? 0.7;
  currentState.value.topP = d.topP ?? currentState.value.topP ?? 1;

  // credential.data 也兜底
  const cd = credential.data ?? {};
  currentState.value.apiKey = cd.api_key ?? currentState.value.apiKey ?? "";
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
      config.model = profile.model;

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
    }
  } catch (error) {
    console.error("加载激活配置失败", error);
  }
}

// 监听 provider 改变（初始化已手动调用过）
watch(
  () => currentState.value.provider,
  (p) => {
    if (p) onProviderChanged(p);
  },
  { immediate: false }
);

// 监听模态切换，重新加载配置
watch(modality, async () => {
  await refreshStateForEnvAndModality();
});

watch(
  () => env.value,
  async () => {
    await refreshStateForEnvAndModality();
  }
);
</script>
