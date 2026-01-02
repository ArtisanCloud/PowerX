<template>
  <div class="space-y-4">
    <!-- LLM 参数 -->
    <div v-if="activeModality === 'llm'" class="space-y-4">
      <div class="grid grid-cols-2 gap-4">
        <div>
          <label
            class="block text-sm font-medium text-[var(--text-primary)] mb-2"
          >
            温度 (Temperature)
          </label>
          <USlider
            v-model="llm.temperature"
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
            type="number"
            :min="1"
            :max="32000"
            placeholder="4096"
          />
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
      </div>
    </div>

    <!-- 图像生成参数 -->
    <div v-else-if="activeModality === 'image'" class="space-y-4">
      <div class="grid grid-cols-2 gap-4">
        <div>
          <label
            class="block text-sm font-medium text-[var(--text-primary)] mb-2"
          >
            图像尺寸
          </label>
          <USelect
            v-model="image.size"
            :options="imageSizeOptions"
            placeholder="选择尺寸"
          />
        </div>
        <div>
          <label
            class="block text-sm font-medium text-[var(--text-primary)] mb-2"
          >
            图像质量
          </label>
          <USelect
            v-model="image.quality"
            :options="imageQualityOptions"
            placeholder="选择质量"
          />
        </div>
      </div>
      <div class="grid grid-cols-2 gap-4">
        <div>
          <label
            class="block text-sm font-medium text-[var(--text-primary)] mb-2"
          >
            输出格式
          </label>
          <USelect
            v-model="image.format"
            :options="imageFormatOptions"
            placeholder="选择格式"
          />
        </div>
        <div>
          <label
            class="block text-sm font-medium text-[var(--text-primary)] mb-2"
          >
            提示词增强
          </label>
          <UInput v-model="image.promptHint" placeholder="可选的提示词前缀" />
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
            placeholder="选择截断策略"
          />
        </div>
      </div>
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
          <USelect
            v-model="audioTts.voice"
            :options="voiceOptions"
            placeholder="选择语音"
          />
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
            :options="videoResolutionOptions"
            placeholder="选择分辨率"
          />
        </div>
        <div>
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
            :min="1"
            :max="60"
            placeholder="10"
          />
        </div>
        <div>
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
type Modality =
  | "llm"
  | "image"
  | "embedding"
  | "audio_tts"
  | "audio_asr"
  | "video"
  | "model3d"
  | "rerank";

interface Props {
  activeModality: Modality;
  llm: any;
  image: any;
  embedding: any;
  audioTts: any;
  audioAsr: any;
  video: any;
  model3d: any;
  rerank: any;
  imageSizeOptions: string[];
  imageQualityOptions: string[];
  imageFormatOptions: string[];
  truncateOptions: string[];
  videoResolutionOptions: string[];
  model3dFormatOptions: string[];
  voiceOptions: string[];
  audioFormatOptions: string[];
  audioQualityOptions: string[];
  languageOptions: string[];
  responseFormatOptions: string[];
  topKOptions: number[];
}

defineProps<Props>();
</script>
