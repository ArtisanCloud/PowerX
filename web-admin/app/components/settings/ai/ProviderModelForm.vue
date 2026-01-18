<template>
  <UForm :state="state" class="space-y-4">
    <div v-if="authModeOptions.length" class="grid grid-cols-1 md:grid-cols-2 gap-4">
      <div>
        <label class="block text-sm mb-1 text-[var(--text-secondary)]">接入方式</label>
        <USelect
          v-model="state.authMode"
          :items="authModeOptions"
          icon="i-heroicons-adjustments-horizontal"
          class="w-full"
          placeholder="选择接入方式"
        />
      </div>
    </div>
    <!-- Provider 与 Model 行 -->
    <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
      <div>
        <label class="block text-sm mb-1 text-[var(--text-secondary)]"
          >Provider</label
        >
        <USelect
          v-model="state.provider"
          :items="providerOptions"
          :disabled="!providerOptions?.length"
          :loading="!providerOptions?.length"
          icon="i-heroicons-building-library"
          class="w-full"
          :placeholder="$t('agent.config.selectProvider')"
          @update:model-value="emit('providerChanged', $event)"
        />
      </div>
      <div>
        <label class="block text-sm mb-1 text-[var(--text-secondary)]"
          >Model</label
        >
        <USelect
          v-model="state.model"
          :items="modelOptions"
          :disabled="!modelOptions?.length"
          icon="i-heroicons-cpu-chip"
          class="w-full"
          :placeholder="$t('agent.config.selectModel')"
          :loading="!modelOptions?.length"
        />
      </div>
    </div>

    <!-- 动态认证字段 -->
    <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
      <div
        v-for="field in authFields"
        :key="field.key"
        :class="{ 'md:col-span-2': field.key === 'azure_deployment' }"
      >
        <label class="block text-sm mb-1 text-[var(--text-secondary)]">
          {{ field.label }}
          <span v-if="field.required" class="text-red-500">*</span>
        </label>
        <UInput
          v-model="state[getStateKey(field.key)]"
          :type="field.type"
          :placeholder="field.placeholder"
          :required="field.required"
        />
      </div>
    </div>
  </UForm>
</template>

<script setup lang="ts">
import type { SelectOption } from "~/composables/api/types/select";

const props = withDefaults(
  defineProps<{
    providerOptions?: SelectOption[];
    modelOptions?: SelectOption[];
    activeProvider?: {
      id: string;
      name: string;
      auth?: {
        scheme: string;
        fields: string[];
        defaults?: Record<string, string>;
      };
    } | null;
    state: {
      provider: string;
      model: string;
      authMode?: string;
      apiKey: string;
      secretId?: string;
      secretKey?: string;
      baseURL: string;
      region: string;
      organization: string;
      azureDeployment?: string;
    };
  }>(),
  {
    providerOptions: () => [],
    modelOptions: () => [],
    activeProvider: null,
  }
);

const emit = defineEmits<{
  (e: "providerChanged", provider: string): void;
}>();

const isAzure = computed(() => props.state.provider === "Azure OpenAI");

const authModeOptions = computed(() => {
  const modes = props.activeProvider?.auth?.modes ?? [];
  if (!Array.isArray(modes) || modes.length === 0) return [];
  return modes.map((m) => ({
    label: m.label || m.id,
    value: m.id,
  }));
});

const selectedAuthMode = computed(() => {
  const modes = props.activeProvider?.auth?.modes ?? [];
  if (!Array.isArray(modes) || modes.length === 0) return null;
  const selected = String(props.state.authMode || "").trim().toLowerCase();
  return (
    modes.find((m) => String(m.id || "").trim().toLowerCase() === selected) ||
    modes[0] ||
    null
  );
});

const baseURLPlaceholder = computed(() => {
  const provider = String(props.state.provider || "").trim().toLowerCase();
  if (provider === "huggingface" || provider === "hf") {
    return "https://router.huggingface.co/v1";
  }
  return "https://api.example.com";
});

watch(
  authModeOptions,
  (opts) => {
    if (!Array.isArray(opts) || opts.length === 0) return;
    if (!String(props.state.authMode || "").trim()) {
      props.state.authMode = String(opts[0].value || "");
    }
  },
  { immediate: true }
);

watch(
  selectedAuthMode,
  (next) => {
    if (!next?.defaults) return;
    // 仅在字段为空时用默认值回填，避免覆盖用户输入
    const defBaseURL = String(next.defaults.base_url || "").trim();
    if (defBaseURL && !String(props.state.baseURL || "").trim()) {
      props.state.baseURL = defBaseURL;
    }
    const defRegion = String(next.defaults.region || "").trim();
    if (defRegion && !String(props.state.region || "").trim()) {
      props.state.region = defRegion;
    }
  },
  { immediate: true }
);

// 字段映射
const fieldMap: Record<
  string,
  { label: string; type: string; placeholder: string; required: boolean }
> = {
  api_key: {
    label: "API Key",
    type: "password",
    placeholder: "••••••••",
    required: true,
  },
  base_url: {
    label: "Base URL",
    type: "text",
    placeholder: "https://api.example.com",
    required: false,
  },
  organization: {
    label: "Organization / Project（可选）",
    type: "text",
    placeholder: "组织或订阅ID（可选）",
    required: false,
  },
  region: {
    label: "Region / Location",
    type: "text",
    placeholder: "如 ap-guangzhou / eastus",
    required: false,
  },
  azure_deployment: {
    label: "Azure Deployment（可选）",
    type: "text",
    placeholder: "Azure OpenAI 的部署名称",
    required: false,
  },
  secret_id: {
    label: "SecretId",
    type: "text",
    placeholder: "腾讯云 SecretId",
    required: true,
  },
  secret_key: {
    label: "SecretKey",
    type: "password",
    placeholder: "腾讯云 SecretKey",
    required: true,
  },
};

// 动态字段配置
const authFields = computed(() => {
  const modes = props.activeProvider?.auth?.modes ?? [];
  if (Array.isArray(modes) && modes.length) {
    const hit = selectedAuthMode.value;
    const fields = hit?.fields ?? [];
    return fields.map((field) => ({
      key: field,
      ...(() => {
        const base =
          fieldMap[field] ||
          ({
            label: field,
            type: "text",
            placeholder: "",
            required: false,
          } as const);
        const defaults = hit?.defaults ?? {};
        if (field === "base_url") {
          const required =
            base.required || !String((defaults as any).base_url || "").trim();
          return {
            ...base,
            required,
            placeholder: baseURLPlaceholder.value,
            label: required ? "Base URL" : "Base URL（可选）",
          };
        }
        if (field === "region") {
          const required =
            base.required || !String((defaults as any).region || "").trim();
          return {
            ...base,
            required,
            label: required ? "Region / Location" : "Region / Location（可选）",
          };
        }
        return base;
      })(),
    }));
  }

  if (!props.activeProvider?.auth?.fields) {
    // 默认字段
    return [
      {
        key: "apiKey",
        label: "API Key",
        type: "password",
        placeholder: "••••••••",
        required: true,
      },
      {
        key: "baseURL",
        label: "Base URL（可选）",
        type: "text",
        placeholder: "https://api.example.com",
        required: false,
      },
    ];
  }

  return props.activeProvider.auth.fields.map((field) => ({
    key: field,
    ...(() => {
      const base =
        fieldMap[field] ||
        ({
          label: field,
          type: "text",
          placeholder: "",
          required: false,
        } as const);
      const defaults = props.activeProvider?.auth?.defaults ?? {};
      if (field === "base_url") {
        const required =
          base.required || !String((defaults as any).base_url || "").trim();
        return {
          ...base,
          required,
          placeholder: baseURLPlaceholder.value,
          label: required ? "Base URL" : "Base URL（可选）",
        };
      }
      if (field === "region") {
        const required =
          base.required || !String((defaults as any).region || "").trim();
        return {
          ...base,
          required,
          label: required ? "Region / Location" : "Region / Location（可选）",
        };
      }
      return base;
    })(),
  }));
});

// 字段映射函数
const getStateKey = (backendKey: string) => {
  const keyMap: Record<string, string> = {
    api_key: "apiKey",
    base_url: "baseURL",
    organization: "organization",
    region: "region",
    azure_deployment: "azureDeployment",
    secret_id: "secretId",
    secret_key: "secretKey",
  };
  return keyMap[backendKey] || backendKey;
};
</script>
