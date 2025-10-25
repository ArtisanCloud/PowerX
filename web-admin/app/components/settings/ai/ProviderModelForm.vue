<template>
  <UForm :state="state" class="space-y-4">
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
      };
    } | null;
    state: {
      provider: string;
      model: string;
      apiKey: string;
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

// 动态字段配置
const authFields = computed(() => {
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
      label: "Base URL（可选）",
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
      label: "Region / Location（可选）",
      type: "text",
      placeholder: "如 eastus / us-east-1",
      required: false,
    },
    azure_deployment: {
      label: "Azure Deployment（可选）",
      type: "text",
      placeholder: "Azure OpenAI 的部署名称",
      required: false,
    },
  };

  return props.activeProvider.auth.fields.map((field) => ({
    key: field,
    ...(fieldMap[field] || {
      label: field,
      type: "text",
      placeholder: "",
      required: false,
    }),
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
  };
  return keyMap[backendKey] || backendKey;
};
</script>
