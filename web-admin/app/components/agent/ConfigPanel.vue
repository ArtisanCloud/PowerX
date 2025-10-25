<script setup lang="ts">
import { reactive, watch, ref, computed } from "vue";
import type { AgentConfig } from "~/composables/agent/useAgentManager";
import type { Agent } from "~/types/agent";

type AgentConfigEx = AgentConfig & {
  contextWindow?: number;
  responseFormat?: "text" | "markdown" | "json";
  streaming?: boolean;
};

const props = withDefaults(
  defineProps<{
    agent?: Partial<AgentConfigEx> | Agent | null;
    isVisible?: boolean;
  }>(),
  {
    agent: null,
    isVisible: false,
  }
);

const emit = defineEmits<{
  (e: "close"): void;
  (e: "save", config: Partial<AgentConfigEx>): void;
}>();

const { t } = useI18n();

// ------- 选项 -------
const modelOptions = [
  { label: "GPT-4", value: "gpt-4" },
  { label: "GPT-4 Turbo", value: "gpt-4-turbo" },
  { label: "GPT-3.5 Turbo", value: "gpt-3.5-turbo" },
  { label: "Claude-3 Opus", value: "claude-3-opus" },
  { label: "Claude-3 Sonnet", value: "claude-3-sonnet" },
  { label: "Claude-3 Haiku", value: "claude-3-haiku" },
  { label: "Gemini Pro", value: "gemini-pro" },
  { label: "Gemini Pro Vision", value: "gemini-pro-vision" },
];

// 统一的能力字典（用 key 作为 id）
const capabilityDict = [
  {
    key: "text-generation",
    name: t("agent.config.capabilities.textGeneration"),
  },
  {
    key: "code-generation",
    name: t("agent.config.capabilities.codeGeneration"),
  },
  { key: "image-analysis", name: t("agent.config.capabilities.imageAnalysis") },
  { key: "web-search", name: t("agent.config.capabilities.webSearch") },
  {
    key: "file-processing",
    name: t("agent.config.capabilities.fileProcessing"),
  },
  { key: "data-analysis", name: t("agent.config.capabilities.dataAnalysis") },
  { key: "translation", name: t("agent.config.capabilities.translation") },
  { key: "summarization", name: t("agent.config.capabilities.summarization") },
];

// ------- 表单状态（可变副本） -------
const form = reactive<Partial<AgentConfigEx>>({
  id: "",
  name: "",
  description: "",
  avatar: "",
  model: "gpt-4",
  systemPrompt: "",
  temperature: 0.7,
  topP: 1,
  maxTokens: 2000,
  frequencyPenalty: 0,
  presencePenalty: 0,
  isActive: true,
  contextWindow: 4096,
  responseFormat: "markdown",
  streaming: true,
  capabilities: [], // 保存时会被我们重建
});

// 用字符串数组在面板里编辑选中的能力
const selectedCaps = ref<string[]>([]);

// 是否编辑：只看 form.id 是否有值
const isEdit = computed(() => !!form.id && String(form.id).trim() !== "");

// 把 props.agent → 表单
watch(
  () => props.agent,
  (a) => {
    console.log("[config-panel] incoming agent:", a);
    // 1) 先重置为默认
    Object.assign(form, {
      id: "",
      name: "",
      description: "",
      avatar: "",
      model: "gpt-4",
      systemPrompt: "",
      temperature: 0.7,
      topP: 1,
      maxTokens: 2000,
      frequencyPenalty: 0,
      presencePenalty: 0,
      isActive: true,
      contextWindow: 4096,
      responseFormat: "markdown",
      streaming: true,
      capabilities: [],
    });

    // 2) 有传入 agent（编辑态）→ 规范回填
    if (a && typeof a === "object") {
      const anyA = a as any;
      form.id = anyA.id != null ? String(anyA.id) : ""; // 关键：给 id 赋值
      form.name = anyA.name ?? "";
      form.description = anyA.description ?? "";
      form.avatar = anyA.avatar ?? "";
      // 后端 Agent 通常有 status，前端 config 没有：都兜底
      form.isActive =
        typeof anyA.isActive === "boolean"
          ? anyA.isActive
          : anyA.status
            ? anyA.status === "active"
            : true;

      // 可选：如果前端传了高级字段就用前端的
      form.model = anyA.model ?? form.model;
      form.systemPrompt = anyA.systemPrompt ?? form.systemPrompt;
      form.temperature = anyA.temperature ?? form.temperature;
      form.topP = anyA.topP ?? form.topP;
      form.maxTokens = anyA.maxTokens ?? form.maxTokens;
      form.frequencyPenalty = anyA.frequencyPenalty ?? form.frequencyPenalty;
      form.presencePenalty = anyA.presencePenalty ?? form.presencePenalty;
      form.contextWindow = anyA.contextWindow ?? form.contextWindow;
      form.responseFormat = anyA.responseFormat ?? form.responseFormat;
      form.streaming = anyA.streaming ?? form.streaming;

      // 3) 能力：兼容对象数组/字符串数组/无此字段三种情况
      const caps: any[] = anyA.capabilities || [];
      if (Array.isArray(caps)) {
        if (caps.length > 0 && typeof caps[0] === "string") {
          selectedCaps.value = caps as string[];
        } else {
          selectedCaps.value = caps
            .map((c: any) => c?.id || toKeyFromName(c?.name))
            .filter(Boolean);
        }
      } else {
        selectedCaps.value = [];
      }
    } else {
      // 创建态：清空选择
      selectedCaps.value = [];
    }
  },
  { immediate: true }
);

// 名称 → key（兜底用）
function toKeyFromName(name?: string) {
  return (name || "")
    .toLowerCase()
    .replace(/\s+/g, "-")
    .replace(/[^a-z0-9-]/g, "");
}

// 切换能力
function toggleCapability(key: string) {
  const i = selectedCaps.value.indexOf(key);
  if (i > -1) selectedCaps.value.splice(i, 1);
  else selectedCaps.value.push(key);
}

function hasCapability(key: string) {
  return selectedCaps.value.includes(key);
}

// 预设系统提示词
const systemPromptPresets = [
  {
    name: t("agent.config.presets.assistant"),
    prompt: t("agent.config.presets.assistantPrompt"),
  },
  {
    name: t("agent.config.presets.coder"),
    prompt: t("agent.config.presets.coderPrompt"),
  },
  {
    name: t("agent.config.presets.translator"),
    prompt: t("agent.config.presets.translatorPrompt"),
  },
  {
    name: t("agent.config.presets.analyst"),
    prompt: t("agent.config.presets.analystPrompt"),
  },
];
function applyPreset(preset: { name: string; prompt: string }) {
  form.systemPrompt = preset.prompt;
}

// 保存：把 selectedCaps → AgentConfig.capabilities（对象数组）
function saveConfig() {
  if (!form.name?.trim()) return;

  const mappedCaps = selectedCaps.value.map((key) => {
    const dict = capabilityDict.find((c) => c.key === key);
    return {
      id: key,
      name: dict?.name || key,
      description: "",
      enabled: true,
      config: {},
    };
  });

  emit("save", {
    ...form,
    capabilities: mappedCaps,
  });
}

function resetForm() {
  const a = props.agent as any;
  if (a && a.id != null) {
    // 只回填你需要的字段
    form.id = String(a.id);
    form.name = a.name ?? "";
    form.description = a.description ?? "";
    form.isActive = a.status ? a.status === "active" : true;
    // 其余字段按需回填/保持默认
    form.model = a.model ?? "gpt-4";
    form.systemPrompt = a.systemPrompt ?? "";
    form.temperature = a.temperature ?? 0.7;
    form.topP = a.topP ?? 1;
    form.maxTokens = a.maxTokens ?? 2000;
    form.frequencyPenalty = a.frequencyPenalty ?? 0;
    form.presencePenalty = a.presencePenalty ?? 0;
    form.contextWindow = a.contextWindow ?? 4096;
    form.responseFormat = a.responseFormat ?? "markdown";
    form.streaming = a.streaming ?? true;
    form.avatar = a.avatar ?? "";
  } else {
    // 创建态回默认
    Object.assign(form, {
      id: "",
      name: "",
      description: "",
      avatar: "",
      model: "gpt-4",
      systemPrompt: "",
      temperature: 0.7,
      topP: 1,
      maxTokens: 2000,
      frequencyPenalty: 0,
      presencePenalty: 0,
      isActive: true,
      contextWindow: 4096,
      responseFormat: "markdown",
      streaming: true,
      capabilities: [],
    });
    selectedCaps.value = [];
  }
}

function cancelConfig() {
  emit("close");
}
</script>

<template>
  <USlideover
    :title="t('agent.config.title')"
    :description="t('agent.config.description')"
    :open="props.isVisible"
    side="right"
    @update:open="
      (v: boolean) => {
        if (!v) emit('close');
      }
    "
  >
    <template #content>
      <div class="flex flex-col h-full">
        <!-- 头部 -->
        <div
          class="flex items-center justify-between p-6 border-b border-gray-200"
        >
          <h2 class="text-xl font-semibold text-gray-900">
            {{
              isEdit
                ? t("agent.config.editTitle")
                : t("agent.config.createTitle")
            }}
          </h2>
          <UButton
            variant="ghost"
            icon="i-heroicons-x-mark"
            @click="cancelConfig"
          />
        </div>

        <!-- 内容 -->
        <div class="flex-1 overflow-y-auto p-6 space-y-6">
          <!-- 基本信息 -->
          <div class="space-y-4">
            <h3 class="text-lg font-medium text-gray-900">
              {{ t("agent.config.basicInfo") }}
            </h3>

            <UFormField :label="t('agent.config.name')" required>
              <UInput
                v-model="form.name"
                :placeholder="t('agent.config.namePlaceholder')"
                class="w-full"
              />
            </UFormField>

            <UFormField :label="t('agent.config.description')">
              <UTextarea
                v-model="form.description"
                :rows="3"
                :placeholder="t('agent.config.descriptionPlaceholder')"
                class="w-full"
              />
            </UFormField>

            <UFormField :label="t('agent.config.avatar')">
              <UInput
                v-model="form.avatar"
                :placeholder="t('agent.config.avatarPlaceholder')"
                class="w-full"
              />
              <div v-if="form.avatar" class="mt-2">
                <img
                  :src="form.avatar"
                  :alt="form.name"
                  class="w-12 h-12 rounded-full object-cover"
                  @error="form.avatar = ''"
                />
              </div>
            </UFormField>

            <UFormField :label="t('agent.config.status')">
              <USwitch
                v-model="form.isActive"
                :label="
                  form.isActive
                    ? t('agent.config.active')
                    : t('agent.config.inactive')
                "
              />
            </UFormField>
          </div>

          <!-- 模型配置 -->
          <div class="space-y-4">
            <h3 class="text-lg font-medium text-gray-900">
              {{ t("agent.config.modelConfig") }}
            </h3>

            <UFormField :label="t('agent.config.model')">
              <USelect
                v-model="form.model"
                :items="modelOptions"
                :placeholder="t('agent.config.selectModel')"
                class="w-full"
              />
            </UFormField>

            <!-- 温度（用原生 range，避免 URange/USlider 未注册） -->
            <UFormField :label="t('agent.config.temperature')">
              <div class="space-y-2">
                <div class="flex items-center gap-3">
                  <input
                    type="range"
                    min="0"
                    max="2"
                    step="0.1"
                    v-model.number="form.temperature"
                    class="w-full"
                  />
                  <span class="w-10 text-right text-sm text-gray-700">{{
                    (form.temperature ?? 0.7).toFixed(1)
                  }}</span>
                </div>
                <div class="flex justify-between text-sm text-gray-500">
                  <span>{{ t("agent.config.conservative") }}</span>
                  <span>{{ form.temperature ?? 0.7 }}</span>
                  <span>{{ t("agent.config.creative") }}</span>
                </div>
              </div>
            </UFormField>

            <UFormField :label="t('agent.config.maxTokens')">
              <UInput
                v-model.number="form.maxTokens"
                type="number"
                :min="1"
                :max="32000"
                :placeholder="t('agent.config.maxTokensPlaceholder')"
                class="w-full"
              />
            </UFormField>
          </div>

          <!-- 系统提示词 -->
          <div class="space-y-4">
            <div class="flex items-center justify-between">
              <h3 class="text-lg font-medium text-gray-900">
                {{ t("agent.config.systemPrompt") }}
              </h3>
              <UDropdownMenu
                :items="[
                  systemPromptPresets.map((p) => ({
                    label: p.name,
                    click: () => applyPreset(p),
                  })),
                ]"
              >
                <UButton variant="outline" size="sm">
                  {{ t("agent.config.usePreset") }}
                </UButton>
              </UDropdownMenu>
            </div>

            <UFormField>
              <UTextarea
                v-model="form.systemPrompt"
                :rows="6"
                :placeholder="t('agent.config.systemPromptPlaceholder')"
                class="w-full"
              />
            </UFormField>
          </div>

          <!-- 能力配置（用字符串 key 编辑，保存时映射为对象数组） -->
          <div class="space-y-4">
            <h3 class="text-lg font-medium text-gray-900">
              {{ t("agent.config.capability") }}
            </h3>
            <div class="grid grid-cols-2 gap-3">
              <div
                v-for="cap in capabilityDict"
                :key="cap.key"
                class="flex items-center space-x-2"
              >
                <UCheckbox
                  :checked="hasCapability(cap.key)"
                  @change="toggleCapability(cap.key)"
                />
                <label class="text-sm text-gray-700 cursor-pointer">{{
                  cap.name
                }}</label>
              </div>
            </div>
          </div>

          <!-- 高级设置 -->
          <div class="space-y-4">
            <h3 class="text-lg font-medium text-gray-900">
              {{ t("agent.config.advancedSettings") }}
            </h3>

            <UFormField :label="t('agent.config.contextWindow')">
              <UInput
                v-model.number="form.contextWindow"
                type="number"
                :placeholder="t('agent.config.contextWindowPlaceholder')"
                class="w-full"
              />
            </UFormField>

            <UFormField :label="t('agent.config.responseFormat')">
              <USelect
                v-model="form.responseFormat"
                :items="[
                  { label: t('agent.config.formats.text'), value: 'text' },
                  {
                    label: t('agent.config.formats.markdown'),
                    value: 'markdown',
                  },
                  { label: t('agent.config.formats.json'), value: 'json' },
                ]"
                class="w-full"
              />
            </UFormField>

            <UFormField :label="t('agent.config.streaming')">
              <USwitch
                v-model="form.streaming"
                :label="
                  form.streaming
                    ? t('agent.config.streamingEnabled')
                    : t('agent.config.streamingDisabled')
                "
              />
            </UFormField>
          </div>
        </div>

        <!-- 底部 -->
        <div
          class="flex items-center justify-between p-6 border-t border-gray-200 bg-gray-50"
        >
          <UButton variant="outline" @click="resetForm">
            {{ t("agent.config.reset") }}
          </UButton>
          <div class="flex space-x-3">
            <UButton variant="outline" @click="cancelConfig">
              {{ t("agent.config.cancel") }}
            </UButton>
            <UButton @click="saveConfig" :disabled="!form.name?.trim()">
              {{ isEdit ? t("agent.config.update") : t("agent.config.save") }}
            </UButton>
          </div>
        </div>
      </div>
    </template>
  </USlideover>
</template>

<style scoped>
.overflow-y-auto::-webkit-scrollbar {
  width: 6px;
}
.overflow-y-auto::-webkit-scrollbar-track {
  background: #f1f1f1;
}
.overflow-y-auto::-webkit-scrollbar-thumb {
  background: #c1c1c1;
  border-radius: 3px;
}
.overflow-y-auto::-webkit-scrollbar-thumb:hover {
  background: #a8a8a8;
}
</style>
