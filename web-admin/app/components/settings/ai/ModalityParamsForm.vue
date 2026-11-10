<template>
  <div class="space-y-4">
    <!-- LLM 参数 -->
    <div v-if="activeModality === 'llm'" class="space-y-4">
      <ModelParameterInfo :profile="parameterProfile" />
      <UButton type="button" variant="ghost" size="xs" data-testid="ai-use-model-defaults" @click="$emit('resetModelParameters')">{{ $t('settings.ai.parameters.useDefaults') }}</UButton>
      <UFormField v-if="parameterProfile?.reasoning_efforts?.length" :label="$t('settings.ai.vision.reasoningEffort')">
        <USelect v-model="llm.reasoningEffort" :items="parameterProfile.reasoning_efforts" class="w-full" data-testid="llm-reasoning" />
      </UFormField>
      <p v-if="!canSample('temperature') || !canSample('top_p')" class="text-xs text-muted" role="status">{{ $t('settings.ai.parameters.samplingUnavailable') }}</p>
      <div class="grid grid-cols-2 gap-4">
        <div>
          <label
            class="block text-sm font-medium text-[var(--text-primary)] mb-2"
          >
            温度 (Temperature)
          </label>
          <USlider
            v-model="llm.temperature"
            :disabled="!canSample('temperature')"
            data-testid="llm-temperature"
            :min="0"
            :max="2"
            :step="0.1"
            class="w-full"
          />
          <div class="text-xs text-[var(--text-secondary)] mt-1">
            当前值: {{ llm.temperature }}
          </div>
        </div>
        <div>
          <label
            class="block text-sm font-medium text-[var(--text-primary)] mb-2"
          >
            最大令牌数
          </label>
          <UInput
            v-model="llm.maxTokens"
            data-testid="llm-max-tokens"
            type="number"
            :min="1"
            :max="parameterProfile?.max_output_tokens"
            placeholder="4096"
          />
          <p class="text-xs text-muted mt-1">{{ $t('settings.ai.parameters.budgetHint') }}</p>
        </div>
      </div>
      <div class="grid grid-cols-2 gap-4">
        <div>
          <label
            class="block text-sm font-medium text-[var(--text-primary)] mb-2"
          >
            Top P
          </label>
          <USlider
            v-model="llm.topP"
            :disabled="!canSample('top_p')"
            data-testid="llm-top-p"
            :min="0"
            :max="1"
            :step="0.01"
            class="w-full"
          />
          <div class="text-xs text-[var(--text-secondary)] mt-1">
            当前值: {{ llm.topP }}
          </div>
        </div>
        <div class="flex items-center">
          <UCheckbox
            v-model="llm.stream"
            label="启用流式输出"
            class="text-sm"
          />
        </div>
        <UFormField :label="$t('settings.ai.modelConcurrency.label')">
          <UInput
            v-model.number="llm.maxConcurrentRequests"
            type="number"
            :min="1"
            :max="1024"
            :placeholder="$t('settings.ai.modelConcurrency.placeholder')"
          />
          <template #hint>
            <span class="text-xs text-[var(--text-secondary)]">{{ $t('settings.ai.modelConcurrency.hint') }}</span>
          </template>
        </UFormField>
      </div>
    </div>

    <!-- 图像生成参数 -->
    <div v-else-if="activeModality === 'vlm' && vlm" class="space-y-4" data-testid="vlm-parameters">
      <p class="text-sm text-muted">{{ $t('settings.ai.vision.hint') }}</p>
      <UFormField :label="$t('settings.ai.vision.maxTokens')">
        <UInput v-model.number="vlm.maxTokens" type="number" :min="1" :max="128000" data-testid="vlm-max-tokens" />
      </UFormField>
      <ModelParameterInfo v-if="parameterProfile" :profile="parameterProfile" />
      <UFormField v-if="visionReasoningOptions.length" :label="$t('settings.ai.vision.reasoningEffort')">
        <USelect v-model="vlm.reasoningEffort" :items="visionReasoningOptions" class="w-full" data-testid="vlm-reasoning" />
      </UFormField>
      <div v-if="visionSamplingAvailable" class="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <UFormField :label="$t('settings.ai.vision.temperature')"><UInput v-model.number="vlm.temperature" type="number" :min="0" :max="2" :step="0.1" :placeholder="$t('settings.ai.vision.providerDefault')" /></UFormField>
        <UFormField label="Top P"><UInput v-model.number="vlm.topP" type="number" :min="0" :max="1" :step="0.1" :placeholder="$t('settings.ai.vision.providerDefault')" /></UFormField>
      </div>
      <UCheckbox v-model="vlm.jsonMode" :label="$t('settings.ai.vision.jsonMode')" />
    </div>
    <div v-else-if="activeModality === 'image'" class="min-w-0 space-y-4" data-testid="image-parameters">
      <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <div>
          <label
            class="block text-sm font-medium text-[var(--text-primary)] mb-2"
          >
            {{ $t('settings.ai.imageParameters.size') }}
          </label>
          <USelect
            v-model="image.size"
            :items="imageSizeOptions"
            class="w-full"
            :placeholder="$t('settings.ai.imageParameters.sizePlaceholder')"
          />
        </div>
        <div v-if="!isAgnesProvider(image.provider) && imageQualityOptions.length">
          <label
            class="block text-sm font-medium text-[var(--text-primary)] mb-2"
          >
            {{ $t('settings.ai.imageParameters.quality') }}
          </label>
          <USelect
            v-model="image.quality"
            :items="imageQualityOptions"
            class="w-full min-w-0"
            :placeholder="$t('settings.ai.imageParameters.qualityPlaceholder')"
            data-testid="image-quality"
          />
        </div>
      </div>
      <div v-if="!isAgnesProvider(image.provider)" class="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <div>
          <label
            class="block text-sm font-medium text-[var(--text-primary)] mb-2"
          >
            {{ $t('settings.ai.imageParameters.format') }}
          </label>
          <USelect
            v-model="image.format"
            :items="imageFormatOptions"
            class="w-full min-w-0"
            :placeholder="$t('settings.ai.imageParameters.formatPlaceholder')"
          />
        </div>
        <div>
          <label
            class="block text-sm font-medium text-[var(--text-primary)] mb-2"
          >
            {{ $t('settings.ai.imageParameters.promptHint') }}
          </label>
          <UInput v-model="image.promptHint" class="w-full" :placeholder="$t('settings.ai.imageParameters.promptHintPlaceholder')" />
        </div>
      </div>
    </div>

    <!-- 向量嵌入参数 -->
    <div v-else-if="activeModality === 'embedding'" class="space-y-4">
      <div class="grid grid-cols-2 gap-4">
        <div>
          <label
            class="block text-sm font-medium text-[var(--text-primary)] mb-2"
          >
            向量维度
          </label>
          <UInput
            v-model="embedding.dimensions"
            type="number"
            :min="1"
            :max="4096"
            placeholder="1536"
          />
        </div>
        <div>
          <label
            class="block text-sm font-medium text-[var(--text-primary)] mb-2"
          >
            截断策略
          </label>
          <USelect
            v-model="embedding.truncate"
            :options="truncateOptions"
            :portal="false"
            v-model:open="truncateOpen"
            @update:open="handleTruncateOpen"
            placeholder="选择截断策略"
          />
        </div>
      </div>
      <div class="grid grid-cols-2 gap-4">
        <div>
          <label
            class="block text-sm font-medium text-[var(--text-primary)] mb-2"
          >
            批处理大小
          </label>
          <UInput
            v-model="embedding.batch"
            type="number"
            :min="1"
            :max="100"
            placeholder="32"
          />
        </div>
        <div>
          <label
            class="block text-sm font-medium text-[var(--text-primary)] mb-2"
          >
            最大输入长度（探测值）
          </label>
          <UInput
            :model-value="embedding.maxInputTokens || '-'"
            disabled
          />
          <div class="mt-1 text-xs text-[var(--text-secondary)]">
            测试连接后自动填充，用于约束分段长度。
          </div>
        </div>
      </div>
    </div>

    <!-- 语音合成参数 -->
    <div v-else-if="activeModality === 'audio_tts'" class="space-y-4">
      <div class="grid grid-cols-2 gap-4">
        <div>
          <label
            class="block text-sm font-medium text-[var(--text-primary)] mb-2"
          >
            语音类型
          </label>
          <UInput
            v-model="audioTts.voice"
            placeholder="如 alloy / qwen-voice / custom-voice-id"
          />
          <div class="mt-1 text-xs text-[var(--text-secondary)]">
            可填写厂商自定义 voice ID，不限制为固定枚举。
          </div>
        </div>
        <div>
          <label
            class="block text-sm font-medium text-[var(--text-primary)] mb-2"
          >
            语速
          </label>
          <USlider
            v-model="audioTts.speed"
            :min="0.25"
            :max="4.0"
            :step="0.25"
            class="w-full"
          />
          <div class="text-xs text-[var(--text-secondary)] mt-1">
            当前值: {{ audioTts.speed }}x
          </div>
        </div>
      </div>
      <div class="grid grid-cols-2 gap-4">
        <div>
          <label
            class="block text-sm font-medium text-[var(--text-primary)] mb-2"
          >
            音频格式
          </label>
          <USelect
            v-model="audioTts.format"
            :options="audioFormatOptions"
            placeholder="选择格式"
          />
        </div>
        <div>
          <label
            class="block text-sm font-medium text-[var(--text-primary)] mb-2"
          >
            音频质量
          </label>
          <USelect
            v-model="audioTts.quality"
            :options="audioQualityOptions"
            placeholder="选择质量"
          />
        </div>
      </div>
    </div>

    <!-- 语音识别参数 -->
    <div v-else-if="activeModality === 'audio_asr'" class="space-y-4">
      <div class="grid grid-cols-2 gap-4">
        <div>
          <label
            class="block text-sm font-medium text-[var(--text-primary)] mb-2"
          >
            识别语言
          </label>
          <USelect
            v-model="audioAsr.language"
            :options="languageOptions"
            placeholder="选择语言"
          />
        </div>
        <div>
          <label
            class="block text-sm font-medium text-[var(--text-primary)] mb-2"
          >
            响应格式
          </label>
          <USelect
            v-model="audioAsr.responseFormat"
            :options="responseFormatOptions"
            placeholder="选择格式"
          />
        </div>
      </div>
      <div class="grid grid-cols-2 gap-4">
        <div>
          <label
            class="block text-sm font-medium text-[var(--text-primary)] mb-2"
          >
            温度
          </label>
          <USlider
            v-model="audioAsr.temperature"
            :min="0"
            :max="1"
            :step="0.1"
            class="w-full"
          />
          <div class="text-xs text-[var(--text-secondary)] mt-1">
            当前值: {{ audioAsr.temperature }}
          </div>
        </div>
        <div>
          <label
            class="block text-sm font-medium text-[var(--text-primary)] mb-2"
          >
            提示词
          </label>
          <UInput v-model="audioAsr.prompt" placeholder="可选的识别提示词" />
        </div>
      </div>
    </div>

    <!-- 视频生成参数 -->
    <div v-else-if="activeModality === 'video'" class="space-y-4">
      <div class="grid grid-cols-2 gap-4">
        <div>
          <label
            class="block text-sm font-medium text-[var(--text-primary)] mb-2"
          >
            视频分辨率
          </label>
          <USelect
            v-model="video.resolution"
            :items="videoResolutionOptions"
            class="w-full"
            placeholder="选择分辨率"
          />
        </div>
        <div v-if="!isAgnesProvider(video.provider)">
          <label
            class="block text-sm font-medium text-[var(--text-primary)] mb-2"
          >
            帧率 (FPS)
          </label>
          <UInput
            v-model="video.fps"
            type="number"
            :min="1"
            :max="60"
            placeholder="24"
          />
        </div>
      </div>
      <div class="grid grid-cols-2 gap-4">
        <div>
          <label
            class="block text-sm font-medium text-[var(--text-primary)] mb-2"
          >
            最大时长 (秒)
          </label>
          <UInput
            v-model="video.maxDurationSec"
            type="number"
            :min="isAgnesProvider(video.provider) ? 4 : 1"
            :max="isAgnesProvider(video.provider) ? 12 : 60"
            placeholder="10"
          />
        </div>
        <div v-if="!isAgnesProvider(video.provider)">
          <label
            class="block text-sm font-medium text-[var(--text-primary)] mb-2"
          >
            提示词增强
          </label>
          <UInput v-model="video.promptHint" placeholder="可选的提示词前缀" />
        </div>
      </div>
    </div>

    <!-- 3D 生成参数 -->
    <div v-else-if="activeModality === 'model3d'" class="space-y-4">
      <div class="grid grid-cols-2 gap-4">
        <div>
          <label
            class="block text-sm font-medium text-[var(--text-primary)] mb-2"
          >
            输出格式
          </label>
          <USelect
            v-model="model3d.outputFormat"
            :options="model3dFormatOptions"
            placeholder="选择格式"
          />
        </div>
        <div>
          <label
            class="block text-sm font-medium text-[var(--text-primary)] mb-2"
          >
            提示词增强
          </label>
          <UInput v-model="model3d.promptHint" placeholder="可选的提示词前缀" />
        </div>
      </div>
    </div>

    <!-- 重排序参数 -->
    <div v-else-if="activeModality === 'rerank'" class="space-y-4">
      <div class="grid grid-cols-2 gap-4">
        <div>
          <label
            class="block text-sm font-medium text-[var(--text-primary)] mb-2"
          >
            Top K
          </label>
          <USelect
            v-model="rerank.topK"
            :options="topKOptions"
            placeholder="选择Top K"
          />
        </div>
        <div>
          <label
            class="block text-sm font-medium text-[var(--text-primary)] mb-2"
          >
            每文档最大块数
          </label>
          <UInput
            v-model="rerank.maxChunksPerDoc"
            type="number"
            :min="1"
            :max="100"
            placeholder="10"
          />
        </div>
      </div>
      <div class="flex items-center">
        <UCheckbox
          v-model="rerank.returnDocuments"
          label="返回原始文档"
          class="text-sm"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { nextTick, ref } from "vue";
import { isAgnesProvider } from "~/utils/agnesMediaSettings";
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

interface Props {
  parameterProfile?: import('~/composables/api/services/aiSettingService').ModelParameterProfile;
  activeModality: Modality;
  llm: any;
  vlm?: any;
  image: any;
  embedding: any;
  audioTts: any;
  audioAsr: any;
  video: any;
  model3d: any;
  rerank: any;
  imageSizeOptions: string[];
  imageQualityOptions: Array<string | { label: string; value: string; disabled?: boolean }>;
  imageFormatOptions: string[];
  truncateOptions: string[];
  videoResolutionOptions: string[];
  model3dFormatOptions: string[];
  audioFormatOptions: string[];
  audioQualityOptions: string[];
  languageOptions: string[];
  responseFormatOptions: string[];
  topKOptions: number[];
}

import ModelParameterInfo from './ModelParameterInfo.vue';
import { computed } from 'vue';
import { modelSupportsParameter } from '~/utils/ai-model-parameters';
const props = defineProps<Props>();
defineEmits<{ (e: 'resetModelParameters'): void }>();
const canSample = (field: string) => modelSupportsParameter(props.parameterProfile, field, props.llm.reasoningEffort);
const visionReasoningOptions = computed(() => props.parameterProfile?.reasoning_efforts || (props.vlm?.provider === 'openai' && props.vlm?.model === 'gpt-6-luna' ? ['none', 'low', 'medium', 'high', 'xhigh', 'max'] : []));
const visionSamplingAvailable = computed(() => props.parameterProfile
  ? modelSupportsParameter(props.parameterProfile, 'temperature', props.vlm?.reasoningEffort)
  : props.vlm?.provider !== 'openai' || props.vlm?.model !== 'gpt-6-luna' || props.vlm?.reasoningEffort === 'none');

const truncateOpen = ref(false);

const handleTruncateOpen = (value: boolean) => {
  truncateOpen.value = value;
  if (value) {
    nextTick(() => {
      const active = document.activeElement as HTMLElement | null;
      active?.blur?.();
    });
  }
};
</script>
