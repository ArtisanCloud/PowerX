<template>
  <UForm :state="state" class="space-y-4" data-testid="ai-provider-model-form">
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
          :disabled="providersLoading || !providerOptions.length"
          :aria-busy="providersLoading"
          data-testid="ai-provider-select"
          icon="i-heroicons-building-library"
          class="w-full"
          :ui="searchableSelectUi"
          :search-input="{ placeholder: '搜索 Provider...' }"
          :placeholder="$t('agent.config.selectProvider')"
          @update:model-value="emit('providerChanged', $event)"
        />
        <p v-if="providersLoading" role="status" class="mt-2 text-xs text-muted">
          {{ $t('settings.ai.catalog.providersLoading') }}
        </p>
        <p v-if="providersError" role="alert" class="mt-2 text-sm text-error">
          {{ $t('settings.ai.catalog.providersFailed') }}
          <UButton type="button" size="xs" variant="link" :loading="providersLoading" @click="emit('retryProviders')">{{ $t('settings.ai.catalog.retry') }}</UButton>
        </p>
        <p v-else-if="!providersLoading && !providerOptions.length" role="status" class="mt-2 text-sm text-muted">
          {{ $t('settings.ai.catalog.noProviders') }}
        </p>
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
        <label class="mb-1 block text-sm text-[var(--text-secondary)]">Model</label>
        <USelectMenu
          v-model="state.model"
          :items="visibleModelOptions"
          data-testid="ai-model-select"
          value-key="value"
          label-key="label"
          :disabled="modelsLoading || (!visionMode && !visibleModelOptions.length)"
          icon="i-heroicons-cpu-chip"
          class="w-full"
          :ui="modelSelectUi"
          :search-input="{ placeholder: '搜索 Model...' }"
          :placeholder="$t('agent.config.selectModel')"
          :aria-busy="modelsLoading"
        >
          <template v-if="visionMode" #content-top>
            <div role="group" :aria-label="$t('settings.ai.visionModels.groups')" class="border-b border-default p-2" data-testid="ai-vision-model-groups">
              <div class="grid grid-cols-2 gap-1 rounded-lg bg-muted/40 p-1">
                <button v-for="group in ['supported', 'other'] as const" :key="group" type="button"
                  :aria-pressed="visionGroup === group" :data-testid="`ai-vision-models-${group}`"
                  class="flex min-w-0 items-center justify-center gap-2 rounded-md px-2 py-1.5 text-xs font-medium whitespace-nowrap"
                  :class="visionGroup === group ? 'bg-primary/15 text-primary' : 'text-muted hover:bg-elevated'"
                  @click.stop="visionGroup = group">
                  <span>{{ $t(`settings.ai.visionModels.${group === 'supported' ? 'confirmedShort' : 'otherShort'}`) }}</span>
                  <span class="tabular-nums opacity-70">{{ group === 'supported' ? supportedModels.length : otherModels.length }}</span>
                </button>
              </div>
              <p v-if="visionGroup === 'supported' && !supportedModels.length && !modelsLoading" role="status" class="px-1 pt-2 text-xs text-muted">{{ $t('settings.ai.visionModels.noneConfirmed') }}</p>
            </div>
          </template>
          <template #item-label="{ item }">
            <span class="block truncate">{{ item.label }}</span>
            <span v-if="visionMode && item.vision?.status === 'unsupported'" class="block whitespace-normal text-xs text-error">{{ $t(visionUsageKey(item.vision)) }}</span>
          </template>
        </USelectMenu>
        <p v-if="modelsLoading" role="status" class="mt-2 text-xs text-muted">{{ $t('settings.ai.catalog.modelsLoading') }}</p>
        <p v-if="modelsError" role="alert" class="mt-2 text-sm text-error">
          {{ $t(dynamicModels ? 'settings.ai.catalog.instanceModelsFailed' : 'settings.ai.catalog.modelsFailed') }}
          <UButton type="button" size="xs" variant="link" :loading="modelsLoading" @click="emit('retryModels')">{{ $t('settings.ai.catalog.retry') }}</UButton>
        </p>
        <p v-if="modelsError && !dynamicModels" class="mt-1 text-xs text-muted">{{ $t('settings.ai.onlineModels.setupConnection') }}</p>
        <p v-else-if="!modelsLoading && !modelOptions.length && !visionMode" role="status" class="mt-2 text-sm text-muted">{{ $t(dynamicModels ? 'settings.ai.catalog.noInstanceModels' : 'settings.ai.catalog.noModels') }}</p>
      </div>
    </div>

    <div v-if="visionMode || hasCatalogInfo || dynamicModels || catalogSource !== 'local'" class="flex flex-wrap items-center justify-between gap-x-3 gap-y-2 text-xs" data-testid="ai-model-tools">
      <div class="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1">
        <span v-if="visionMode && selectedVision && !modelsLoading" class="inline-flex items-center gap-1.5"
          :class="selectedVision.status === 'unsupported' ? 'text-error' : selectedVision.status === 'supported' ? 'text-primary' : 'text-warning'"
          :role="selectedVision.status === 'unsupported' ? 'alert' : 'status'" data-testid="ai-vision-model-status">
          <UIcon :name="selectedVision.status === 'supported' ? 'i-heroicons-check-circle' : 'i-heroicons-exclamation-triangle'" class="size-3.5 shrink-0" />
          {{ $t(selectedVision.status === 'supported' ? 'settings.ai.visionModels.supportedShort' : selectedVision.status === 'unknown' ? 'settings.ai.visionModels.unknownShort' : visionUsageKey(selectedVision)) }}
        </span>
        <span v-else-if="usageUnspecified && !modelsLoading" class="text-warning" data-testid="ai-model-usage-unspecified">{{ $t('settings.ai.onlineModels.usageUnspecified') }}</span>
        <span v-if="hasCatalogInfo" class="rounded bg-elevated px-2 py-0.5 text-muted" data-testid="ai-model-source">{{ $t(`settings.ai.modelDetails.source_${catalogSource}`) }}</span>
      </div>
      <div class="ml-auto flex shrink-0 items-center gap-2">
        <UButton v-if="hasCatalogInfo || selectedVision" type="button" size="xs" color="neutral" variant="link" data-testid="ai-model-details-toggle" :aria-expanded="showModelDetails" aria-controls="ai-model-details" :trailing-icon="showModelDetails ? 'i-heroicons-chevron-up' : 'i-heroicons-chevron-down'" @click="showModelDetails = !showModelDetails">{{ $t('settings.ai.modelDetails.title') }}</UButton>
        <UButton v-if="catalogSource !== 'local' && !dynamicModels" type="button" size="xs" color="neutral" variant="link" icon="i-heroicons-arrow-path" data-testid="ai-online-models" :disabled="modelsLoading || !state.provider" @click="emit('updateOnlineModels')">{{ $t('settings.ai.onlineModels.update') }}</UButton>
        <UButton v-if="dynamicModels" type="button" size="xs" color="neutral" variant="link" icon="i-heroicons-arrow-path" data-testid="ai-instance-models-refresh" :loading="modelsLoading" :disabled="modelsLoading" @click="emit('retryModels')">{{ $t('settings.ai.catalog.refreshModels') }}</UButton>
      </div>
    </div>

    <div v-if="showModelDetails" id="ai-model-details" role="region" :aria-label="$t('settings.ai.modelDetails.title')" class="rounded-lg bg-muted/30 p-3 text-xs" data-testid="ai-model-details">
      <dl class="grid min-w-0 grid-cols-1 gap-3 sm:grid-cols-2">
        <div class="sm:col-span-2"><dt class="text-muted">Model</dt><dd class="mt-1 break-all font-mono">{{ state.model || '—' }}</dd></div>
        <div v-if="hasCatalogInfo"><dt class="text-muted">{{ $t('settings.ai.modelDetails.sourceLabel') }}</dt><dd class="mt-1">{{ $t(`settings.ai.onlineModels.source_${catalogSource}`) }}</dd></div>
        <div v-if="catalogSource === 'online' && catalogUpdatedAt"><dt class="text-muted">{{ $t('settings.ai.onlineModels.updatedAt') }}</dt><dd class="mt-1">{{ new Date(catalogUpdatedAt).toLocaleString() }}</dd></div>
        <div v-if="visionMode && selectedVision" class="sm:col-span-2"><dt class="text-muted">{{ $t('settings.ai.modelDetails.usage') }}</dt><dd class="mt-1">{{ $t(visionUsageKey(selectedVision)) }}</dd></div>
        <div v-if="visionMode && selectedVision?.source !== 'unknown' && selectedVision" class="sm:col-span-2"><dt class="text-muted">{{ $t('settings.ai.modelDetails.evidence') }}</dt><dd class="mt-1 flex flex-wrap items-center gap-2">
          <a v-if="visionDocumentationURL(selectedVision.source_url)" :href="visionDocumentationURL(selectedVision.source_url)" target="_blank" rel="noopener noreferrer" class="text-primary underline">{{ $t('settings.ai.visionModels.officialDocs') }}</a>
          <span v-else>{{ $t('settings.ai.visionModels.providerAPI') }}</span>
          <span v-if="selectedVision.checked_at" class="whitespace-nowrap text-muted">{{ selectedVision.checked_at }}</span>
        </dd></div>
      </dl>
      <p v-if="usageUnspecified" class="mt-3 text-muted" data-testid="ai-model-usage-detail">{{ $t('settings.ai.onlineModels.unspecifiedSelection') }}</p>
      <p v-else-if="visionMode && selectedVision?.status === 'supported'" class="mt-3 text-muted">{{ $t('settings.ai.visionModels.trialHint') }}</p>
      <p v-if="dynamicModels" class="mt-3 text-muted">{{ $t('settings.ai.catalog.instanceModelsHint') }}</p>
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
          <span v-if="field.required && !(field.type === 'password' && hasSavedCredentials)" class="text-red-500">*</span>
        </label>
        <SecretInput
          v-if="field.type === 'password'"
          :key="`${state.provider}:${state.authMode}:${field.key}`"
          v-model="state[getStateKey(field.key)]"
          :placeholder="hasSavedCredentials ? $t('settings.ai.sharedCredential.reusePlaceholder') : field.placeholder"
          :required="field.required && !hasSavedCredentials"
          :disabled="providersLoading"
          :data-testid="getStateKey(field.key) === 'apiKey' ? 'ai-api-key' : undefined"
        />
        <UInput
          v-else
          class="w-full"
          v-model="state[getStateKey(field.key)]"
          :type="field.type"
          :placeholder="field.placeholder"
          :required="field.required"
          :disabled="providersLoading"
          :data-testid="field.key === 'base_url' ? 'ai-base-url' : undefined"
          @change="field.key === 'base_url' && emit('baseUrlChanged')"
        />
        <p v-if="field.key === 'base_url' && !String(state.baseURL || '').trim() && defaultAuthValues.baseURL" class="mt-1 truncate text-xs text-muted" :title="defaultAuthValues.baseURL" data-testid="ai-default-base-url-hint">
          {{ $t('settings.ai.connectionTest.emptyBaseUsesDefault') }} {{ defaultAuthValues.baseURL }}
        </p>
        <p v-if="field.type === 'password' && (hasSavedCredentials || String(state[getStateKey(field.key)] || '').trim())" class="mt-1 text-xs leading-relaxed text-muted" data-testid="ai-shared-credential-hint">
          {{ $t(String(state[getStateKey(field.key)] || '').trim() ? 'settings.ai.sharedCredential.replaceHint' : 'settings.ai.sharedCredential.reuseHint', { provider: activeProvider?.name || state.provider }) }}
        </p>
      </div>
    </div>
  </UForm>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import type { SelectOption } from "~/composables/api/types/select";
import { visionDocumentationURL, visionUsageKey, type VisionModelOption } from '~/utils/ai-vision-model-choice';
import { defaultAIProviderBaseURL } from "~/utils/ai-provider-endpoint";

const props = withDefaults(
  defineProps<{
    providerOptions?: SelectOption[];
    appOptions?: SelectOption[];
    modelOptions?: VisionModelOption[];
    visionMode?: boolean;
    modelContext?: string;
    providersLoading?: boolean;
    providersError?: boolean;
    modelsLoading?: boolean;
    modelsError?: boolean;
    dynamicModels?: boolean;
    catalogSource?: string;
    catalogUpdatedAt?: string;
    usageUnspecified?: boolean;
    hasSavedCredentials?: boolean;
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
    providersLoading: false,
    providersError: false,
    modelsLoading: false,
    modelsError: false,
    dynamicModels: false,
    hasSavedCredentials: false,
    activeProvider: null,
  }
);

const { t } = useI18n();
const visionGroup = ref<'supported' | 'other'>('supported');
const showModelDetails = ref(false);
const hasCatalogInfo = computed(() => ['online', 'local', 'instance'].includes(props.catalogSource || ''));
watch(() => props.modelContext, () => { showModelDetails.value = false; });
const supportedModels = computed(() => props.modelOptions.filter(item => item.vision?.status === 'supported'));
const otherModels = computed(() => props.modelOptions.filter(item => item.vision?.status !== 'supported'));
const selectedVision = computed(() => props.modelOptions.find(item => item.value === props.state.model)?.vision);
const visibleModelOptions = computed(() => {
  if (!props.visionMode) return props.modelOptions;
  const list = visionGroup.value === 'supported' ? supportedModels.value : otherModels.value;
  const selected = props.modelOptions.find(item => item.value === props.state.model);
  // 分组不修改绑定；始终可见当前值，包括升级前保存的不适用型号。
  return selected && !list.includes(selected) ? [{ ...selected, label: `${selected.label} · ${t('settings.ai.visionModels.currentBinding')}` }, ...list] : list;
});
watch([() => props.modelContext, () => props.state.model, () => selectedVision.value?.status], () => {
  visionGroup.value = selectedVision.value?.status === 'unknown' ? 'other' : 'supported';
}, { immediate: true });

const emit = defineEmits<{
  (e: "providerChanged", provider: string): void;
  (e: "appChanged", app: string): void;
  (e: "retryProviders"): void;
  (e: "retryModels"): void;
  (e: "baseUrlChanged"): void;
  (e: "updateOnlineModels"): void;
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

const modelSelectUi = {
  ...searchableSelectUi,
  content: `${searchableSelectUi.content} min-w-[min(22rem,calc(100vw-2rem))] max-w-[calc(100vw-2rem)]`,
};

const hasApp = computed(() => Boolean(props.appOptions?.length));
const gridClass = computed(() =>
  hasApp.value
    ? "grid grid-cols-1 md:grid-cols-12 gap-4 items-start"
    : "grid grid-cols-1 md:grid-cols-2 gap-4 items-start"
);
const providerColClass = computed(() =>
  hasApp.value ? "min-w-0 md:col-span-3" : "min-w-0"
);
const appColClass = computed(() =>
  hasApp.value ? "min-w-0 md:col-span-3" : "min-w-0"
);
const modelColClass = computed(() =>
  hasApp.value ? "min-w-0 md:col-span-6" : "min-w-0"
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
    baseURL: defaultAIProviderBaseURL(props.activeProvider, props.state.authMode),
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
    // 默认地址通过占位和说明展示，保留用户清空地址的意图。
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
