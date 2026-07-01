<template>
  <UForm :state="state" class="space-y-4">
    <!-- Provider / App / Model 行 -->
    <div :class="gridClass">
      <div :class="providerColClass">
        <label class="block text-sm mb-1 text-[var(--text-secondary)]"
          >Provider</label
        >
        <USelectMenu
          v-model="state.provider"
          :items="providerOptions"
          value-key="value"
          label-key="label"
          :disabled="!providerOptions?.length"
          :loading="!providerOptions?.length"
          icon="i-heroicons-building-library"
          class="w-full"
          :ui="searchableSelectUi"
          :search-input="{ placeholder: '搜索 Provider...' }"
          :placeholder="$t('agent.config.selectProvider')"
          @update:model-value="emit('providerChanged', $event)"
        />
      </div>
      <div v-if="appOptions?.length" :class="appColClass">
        <label class="block text-sm mb-1 text-[var(--text-secondary)]"
          >App</label
        >
        <USelect
          v-model="state.app"
          :items="appOptions"
          icon="i-heroicons-squares-2x2"
          class="w-full"
          placeholder="选择 App"
          @update:model-value="emit('appChanged', $event)"
        />
      </div>
      <div :class="modelColClass">
        <label class="block text-sm mb-1 text-[var(--text-secondary)]"
          >Model</label
        >
        <USelectMenu
          v-model="state.model"
          :items="modelOptions"
          value-key="value"
          label-key="label"
          :disabled="!modelOptions?.length"
          icon="i-heroicons-cpu-chip"
          class="w-full"
          :ui="searchableSelectUi"
          :search-input="{ placeholder: '搜索 Model...' }"
          :placeholder="$t('agent.config.selectModel')"
          :loading="!modelOptions?.length"
        />
        <p
          v-if="String(state.model || '').trim()"
          class="mt-1 text-xs text-[var(--text-secondary)] break-all leading-5"
        >
          {{ state.model }}
        </p>
      </div>
    </div>

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
    appOptions?: SelectOption[];
    modelOptions?: SelectOption[];
    activeProvider?: {
      id: string;
      name: string;
      auth?: {
        scheme: string;
        fields: string[];
        defaults?: Record<string, string>;
        modes?: Array<{
          id: string;
          label?: string;
          scheme?: string;
          fields?: string[];
          defaults?: Record<string, string>;
        }>;
      };
    } | null;
    state: {
      provider: string;
      app?: string;
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
    appOptions: () => [],
    modelOptions: () => [],
    activeProvider: null,
  }
);

const emit = defineEmits<{
  (e: "providerChanged", provider: string): void;
  (e: "appChanged", app: string): void;
}>();

const searchableSelectUi = {
  base: "h-10 bg-[#0f172a] ring-1 ring-[#334155] text-white hover:bg-[#111c30] focus-visible:ring-2 focus-visible:ring-primary",
  leadingIcon: "text-white",
  trailingIcon: "text-white",
  value: "text-white truncate",
  placeholder: "text-slate-400 truncate",
  content:
    "bg-[#0f172a] border border-[#334155] ring-0 shadow-xl rounded-md overflow-hidden",
  input:
    "border-b border-[#334155] bg-[#111c30] text-white placeholder:text-slate-500 [&_input]:bg-[#111c30] [&_input]:text-white [&_input::placeholder]:text-slate-500",
  viewport: "max-h-72 overflow-y-auto divide-y-0 py-1",
  group: "p-1",
  item:
    "text-slate-100 data-highlighted:not-data-disabled:text-white data-highlighted:not-data-disabled:before:bg-[#1f2f46]",
  itemLabel: "truncate",
  itemLeadingIcon: "text-slate-300",
  itemTrailingIcon: "text-white",
  empty: "px-3 py-3 text-sm text-slate-400",
};

const hasApp = computed(() => Boolean(props.appOptions?.length));
const gridClass = computed(() =>
  hasApp.value
    ? "grid grid-cols-1 md:grid-cols-12 gap-4"
    : "grid grid-cols-1 md:grid-cols-2 gap-4"
);
const providerColClass = computed(() =>
  hasApp.value ? "md:col-span-3" : ""
);
const appColClass = computed(() =>
  hasApp.value ? "md:col-span-3" : ""
);
const modelColClass = computed(() =>
  hasApp.value ? "md:col-span-6" : ""
);

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

const defaultAuthValues = computed(() => {
  const modeDefaults = selectedAuthMode.value?.defaults ?? {};
  const providerDefaults = props.activeProvider?.auth?.defaults ?? {};
  return {
    baseURL: String((modeDefaults as any).base_url || (providerDefaults as any).base_url || "").trim(),
    region: String((modeDefaults as any).region || (providerDefaults as any).region || "").trim(),
  };
});

const baseURLPlaceholder = computed(() => {
  if (defaultAuthValues.value.baseURL) {
    return defaultAuthValues.value.baseURL;
  }
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
  defaultAuthValues,
  (next) => {
    // 仅在字段为空时用默认值回填，避免覆盖用户输入
    const defBaseURL = String(next.baseURL || "").trim();
    if (defBaseURL && !String(props.state.baseURL || "").trim()) {
      props.state.baseURL = defBaseURL;
    }
    const defRegion = String(next.region || "").trim();
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
    label: "AccessKeyId",
    type: "text",
    placeholder: "AccessKeyId",
    required: true,
  },
  secret_key: {
    label: "SecretAccessKey",
    type: "password",
    placeholder: "SecretAccessKey",
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
