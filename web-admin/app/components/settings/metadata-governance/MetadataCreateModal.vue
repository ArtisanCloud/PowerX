<template>
  <UModal
    v-model:open="openModel"
    :title="t(`settings.metadataGovernance.create.${target}.title`)"
    :description="t(`settings.metadataGovernance.create.${target}.description`)"
    :ui="{ content: 'max-w-3xl w-full' }"
  >
    <template #body>
      <UAlert
        v-if="error"
        class="mb-4"
        color="error"
        variant="subtle"
        icon="i-lucide-circle-alert"
        :title="t('settings.metadataGovernance.create.errorTitle')"
        :description="error"
      />
      <form class="space-y-4" @submit.prevent="submit">
        <UAlert
          v-if="contextTitle"
          color="neutral"
          variant="subtle"
          icon="i-lucide-map-pin"
          :title="contextTitle"
          :description="contextDescription"
        />

        <div v-if="needsDefinitionFields" class="grid grid-cols-1 gap-4 md:grid-cols-2">
          <UFormField :label="t('settings.metadataGovernance.form.module')" required>
            <USelectMenu
              v-model="form.module"
              value-key="value"
              label-key="label"
              :items="moduleOptions"
              :portal="false"
              :content="selectContent"
              :ui="selectMenuUi"
              class="w-full"
              :placeholder="t('settings.metadataGovernance.form.modulePlaceholder')"
              :search-input="{ placeholder: t('settings.metadataGovernance.form.moduleSearchPlaceholder') }"
            />
          </UFormField>
          <UFormField :label="t('settings.metadataGovernance.form.namespaceSuffix')" required>
            <div class="flex w-full overflow-hidden rounded-md ring ring-inset ring-accented focus-within:ring-2 focus-within:ring-primary">
              <div
                class="flex max-w-[45%] shrink-0 items-center truncate bg-muted px-3 text-sm text-muted"
                :title="namespacePrefix"
              >
                {{ namespacePrefix }}
              </div>
              <UInput
                v-model="form.namespaceSuffix"
                class="min-w-0 flex-1"
                :ui="{ base: 'rounded-l-none ring-0 focus-visible:ring-0' }"
                :placeholder="namespaceSuffixPlaceholder"
              />
            </div>
          </UFormField>
        </div>

        <div v-if="needsResourceTypeField" class="grid grid-cols-1 gap-4 md:grid-cols-2">
          <UFormField :label="t('settings.metadataGovernance.form.resourceType')" required>
            <UInput v-model="form.resourceType" class="w-full" :placeholder="t('settings.metadataGovernance.form.resourceTypePlaceholder')" />
          </UFormField>
          <UFormField :label="t('settings.metadataGovernance.form.module')" required>
            <USelectMenu
              v-model="form.module"
              value-key="value"
              label-key="label"
              :items="moduleOptions"
              :portal="false"
              :content="selectContent"
              :ui="selectMenuUi"
              class="w-full"
              :placeholder="t('settings.metadataGovernance.form.modulePlaceholder')"
              :search-input="{ placeholder: t('settings.metadataGovernance.form.moduleSearchPlaceholder') }"
            />
          </UFormField>
        </div>

        <div v-if="target === 'tag'" class="grid grid-cols-1 gap-4 md:grid-cols-2">
          <UFormField :label="t('settings.metadataGovernance.form.resourceType')" required>
            <USelectMenu
              v-model="form.resourceType"
              value-key="value"
              label-key="label"
              :items="resourceTypeOptions"
              :portal="false"
              :content="selectContent"
              :ui="selectMenuUi"
              class="w-full"
              :placeholder="t('settings.metadataGovernance.form.resourceTypeSelectPlaceholder')"
              :search-input="{ placeholder: t('settings.metadataGovernance.form.resourceTypeSearchPlaceholder') }"
            />
          </UFormField>
          <UFormField :label="t('settings.metadataGovernance.form.namespaceSuffix')" required>
            <div class="flex w-full overflow-hidden rounded-md ring ring-inset ring-accented focus-within:ring-2 focus-within:ring-primary">
              <div
                class="flex max-w-[45%] shrink-0 items-center truncate bg-muted px-3 text-sm text-muted"
                :title="namespacePrefix"
              >
                {{ namespacePrefix }}
              </div>
              <UInput
                v-model="form.namespaceSuffix"
                class="min-w-0 flex-1"
                :ui="{ base: 'rounded-l-none ring-0 focus-visible:ring-0' }"
                :placeholder="namespaceSuffixPlaceholder"
              />
            </div>
          </UFormField>
        </div>

        <div v-if="hasCodeField" class="grid grid-cols-1 gap-4 md:grid-cols-2">
          <UFormField :label="t('settings.metadataGovernance.form.machineIdentifier')" required>
            <UInput v-model="form.code" class="w-full" :placeholder="t('settings.metadataGovernance.form.machineIdentifierPlaceholder')" />
          </UFormField>
          <UFormField v-if="hasSortOrderField" :label="t('settings.metadataGovernance.form.sortOrder')">
            <UInput v-model.number="form.sortOrder" class="w-full" type="number" min="0" />
          </UFormField>
          <UFormField v-if="target === 'tag'" :label="t('settings.metadataGovernance.form.color')">
            <UInput v-model="form.color" class="w-full" type="color" />
          </UFormField>
        </div>

        <div v-if="target === 'taxonomyNode'" class="grid grid-cols-1 gap-4 md:grid-cols-2">
          <UFormField :label="t('settings.metadataGovernance.form.parentNode')">
            <USelect v-model="form.parentUuid" :items="parentNodeItems" class="w-full" />
          </UFormField>
          <UFormField :label="t('settings.metadataGovernance.form.sortOrder')">
            <UInput v-model.number="form.sortOrder" class="w-full" type="number" min="0" />
          </UFormField>
        </div>

        <div v-if="target === 'taxonomy'" class="grid grid-cols-1 gap-4 md:grid-cols-2">
          <UFormField :label="t('settings.metadataGovernance.form.maxDepth')" required>
            <UInput v-model.number="form.maxDepth" class="w-full" type="number" min="1" />
          </UFormField>
        </div>

        <div v-if="target === 'resourceType'" class="grid grid-cols-1 gap-4 md:grid-cols-2">
          <UFormField :label="t('settings.metadataGovernance.form.validatorKey')">
            <UInput v-model="form.validatorKey" class="w-full" :placeholder="t('settings.metadataGovernance.form.validatorKeyPlaceholder')" />
          </UFormField>
          <UFormField :label="t('settings.metadataGovernance.form.bindingEnabled')">
            <USwitch v-model="form.bindingEnabled" />
          </UFormField>
        </div>

        <div class="rounded-lg border border-gray-200 p-4 dark:border-gray-800">
          <div class="mb-3 flex flex-col gap-2 md:flex-row md:items-center md:justify-between">
            <div>
              <div class="text-sm font-medium text-gray-900 dark:text-white">
                {{ t("settings.metadataGovernance.form.requiredLocaleSection") }}
              </div>
              <div class="text-xs text-gray-500">
                {{ t("settings.metadataGovernance.form.requiredLocaleDescription") }}
              </div>
            </div>
            <UBadge color="primary" variant="subtle">
              {{ requiredLocaleLabel }}
            </UBadge>
          </div>
          <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
            <UFormField :label="t('settings.metadataGovernance.form.localizedName')" required>
              <UInput
                v-model="form.names[requiredLocale]"
                class="w-full"
                :placeholder="t('settings.metadataGovernance.form.localizedNamePlaceholder')"
              />
            </UFormField>
            <UFormField :label="t('settings.metadataGovernance.form.localizedDescription')">
              <UTextarea
                v-model="form.descriptions[requiredLocale]"
                class="w-full"
                :rows="3"
                :placeholder="t('settings.metadataGovernance.form.localizedDescriptionPlaceholder')"
              />
            </UFormField>
          </div>
        </div>

        <div class="rounded-lg border border-gray-200 p-4 dark:border-gray-800">
          <div class="mb-3">
            <div>
              <div class="text-sm font-medium text-gray-900 dark:text-white">
                {{ t("settings.metadataGovernance.form.optionalLocaleSection") }}
              </div>
              <div class="text-xs text-gray-500">
                {{ t("settings.metadataGovernance.form.optionalLocaleDescription") }}
              </div>
            </div>
          </div>
          <div class="mb-4 max-w-md">
            <USelectMenu
              v-model="selectedOptionalLocale"
              value-key="value"
              label-key="label"
              :items="optionalLocaleOptions"
              :portal="false"
              :content="optionalLocaleSelectContent"
              :ui="optionalLocaleSelectUi"
              class="w-full"
              :placeholder="t('settings.metadataGovernance.form.localeSelectPlaceholder')"
              :search-input="{ placeholder: t('settings.metadataGovernance.form.localeSearchPlaceholder') }"
            />
          </div>

          <div v-if="selectedOptionalLocale" class="grid grid-cols-1 gap-4 md:grid-cols-2">
            <UFormField
              :label="t('settings.metadataGovernance.form.localizedName')"
              :required="requiresEnglishName && selectedOptionalLocale === englishLocale"
            >
              <UInput
                v-model="form.names[selectedOptionalLocale]"
                class="w-full"
                :placeholder="t('settings.metadataGovernance.form.localizedNamePlaceholder')"
              />
            </UFormField>
            <UFormField :label="t('settings.metadataGovernance.form.localizedDescription')">
              <UTextarea
                v-model="form.descriptions[selectedOptionalLocale]"
                class="w-full"
                :rows="3"
                :placeholder="t('settings.metadataGovernance.form.localizedDescriptionPlaceholder')"
              />
            </UFormField>
          </div>

          <div v-if="filledOptionalLocales.length > 0" class="mt-3 flex flex-wrap gap-2">
            <UBadge
              v-for="localeItem in filledOptionalLocales"
              :key="localeItem.value"
              color="neutral"
              variant="subtle"
            >
              {{ localeItem.label }}
            </UBadge>
          </div>
        </div>

        <div class="flex justify-end gap-2">
          <UButton type="button" color="neutral" variant="subtle" @click="openModel = false">
            {{ t("common.cancel") }}
          </UButton>
          <UButton type="submit" color="primary" :loading="submitting">
            {{ t("common.save") }}
          </UButton>
        </div>
      </form>
    </template>
  </UModal>
</template>

<script setup lang="ts">
import type { MetadataCreateTarget, TaxonomyNode } from "~/types/metadata-governance";

const props = defineProps<{
  open: boolean
  target: MetadataCreateTarget
  taxonomyNodes?: TaxonomyNode[]
  moduleItems?: Array<{ label: string; value: string }>
  resourceTypeItems?: Array<{ label: string; value: string; module?: string }>
  defaultModule?: string
  contextTitle?: string
  contextDescription?: string
  activeLocale?: string
  submitting?: boolean
  errorMessage?: string
}>();

const emit = defineEmits<{
  "update:open": [value: boolean]
  submit: [payload: Record<string, unknown>]
}>();

const { t } = useI18n();
const runtimeConfig = useRuntimeConfig();
const ROOT_NODE = "__root";
const requiredLocale = "zh-CN";
const englishLocale = "en";
const MACHINE_IDENTIFIER_RE = /^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$/;
const optionalLocaleSelectContent = {
  side: "top" as const,
  sideOffset: 8,
  collisionPadding: 12,
  position: "popper" as const,
};
const optionalLocaleSelectUi = {
  content: "z-[90] max-h-56 overflow-y-auto",
};
const selectContent = {
  side: "bottom" as const,
  sideOffset: 8,
  collisionPadding: 12,
  position: "popper" as const,
};
const selectMenuUi = {
  content: "z-[90] max-h-56 overflow-y-auto",
};

const normalizeLocale = (value: string) => {
  const raw = String(value || "").trim();
  if (!raw) return "";
  if (raw === "zh") return "zh-CN";
  return raw;
};

const availableLocaleCodes = computed(() =>
  String(runtimeConfig.public.availableLanguages || "zh,en,ja,ko")
    .split(",")
    .map((item) => normalizeLocale(item))
    .filter(Boolean),
);
const localeOptions = computed(() => {
  const values = new Set<string>(availableLocaleCodes.value);
  values.add(requiredLocale);
  values.add(englishLocale);
  const active = normalizeLocale(props.activeLocale || "");
  if (active) values.add(active);
  return Array.from(values).map((value) => ({
    value,
    label: t(`settings.metadataGovernance.form.locale.${value.replace("-", "_")}`),
  }));
});
const requiredLocaleLabel = computed(() =>
  t(`settings.metadataGovernance.form.locale.${requiredLocale.replace("-", "_")}`),
);
const optionalLocaleOptions = computed(() => localeOptions.value.filter((item) => item.value !== requiredLocale));
const defaultOptionalLocale = () => {
  if (props.target === "dictionaryItem" && optionalLocaleOptions.value.some((item) => item.value === englishLocale)) {
    return englishLocale;
  }
  const active = normalizeLocale(props.activeLocale || "");
  if (active && active !== requiredLocale && optionalLocaleOptions.value.some((item) => item.value === active)) {
    return active;
  }
  return optionalLocaleOptions.value[0]?.value || "";
};
const moduleOptions = computed(() => props.moduleItems?.filter((item) => item.value && item.label) ?? []);
const resourceTypeOptions = computed(() => props.resourceTypeItems?.filter((item) => item.value && item.label) ?? []);
const selectedResourceType = computed(() =>
  resourceTypeOptions.value.find((item) => item.value === form.resourceType),
);

const openModel = computed({
  get: () => props.open,
  set: (value: boolean) => emit("update:open", value),
});

const defaultModuleValue = () => props.defaultModule || moduleOptions.value[0]?.value || "";
const defaultResourceTypeValue = () => resourceTypeOptions.value[0]?.value || "";
const namespacePrefixForModule = (moduleValue: string) => {
  const normalized = String(moduleValue || "").trim();
  return normalized ? `${normalized}.` : "";
};
const namespacePrefixForResourceType = (resourceTypeValue: string) => {
  const moduleValue = String(resourceTypeOptions.value.find((item) => item.value === resourceTypeValue)?.module || "").trim();
  return moduleValue ? `${moduleValue}.` : "";
};
const defaultNamespaceSuffix = () => {
  return "";
};
const emptyForm = () => {
  const moduleValue = defaultModuleValue();
  const resourceTypeValue = defaultResourceTypeValue();
  return {
    module: moduleValue,
    resourceType: resourceTypeValue,
    namespaceSuffix: defaultNamespaceSuffix(),
    code: "",
    names: Object.fromEntries(localeOptions.value.map((item) => [item.value, ""])) as Record<string, string>,
    descriptions: Object.fromEntries(localeOptions.value.map((item) => [item.value, ""])) as Record<string, string>,
    sortOrder: 0,
    maxDepth: 3,
    color: "#2563eb",
    validatorKey: "",
    bindingEnabled: true,
    parentUuid: ROOT_NODE,
  };
};

const form = reactive(emptyForm());
const validationError = ref("");
const selectedOptionalLocale = ref(defaultOptionalLocale());
const error = computed(() => validationError.value || props.errorMessage || "");
const filledOptionalLocales = computed(() =>
  optionalLocaleOptions.value.filter((item) =>
    String(form.names[item.value] || "").trim() || String(form.descriptions[item.value] || "").trim(),
  ),
);

watch(
  () => [
    props.open,
    props.target,
    props.defaultModule,
    localeOptions.value.map((item) => item.value).join(","),
    resourceTypeOptions.value.map((item) => item.value).join(","),
  ],
  () => {
    Object.assign(form, emptyForm());
    selectedOptionalLocale.value = defaultOptionalLocale();
    validationError.value = "";
  },
);

watch(
  () => form.module,
  (moduleValue) => {
    if (!needsDefinitionFields.value) return;
    const trimmed = String(moduleValue || "").trim();
    if (!trimmed) return;
  },
);

const needsDefinitionFields = computed(() => props.target === "dictionaryNamespace" || props.target === "taxonomy");
const needsResourceTypeField = computed(() => props.target === "resourceType");
const hasCodeField = computed(() => props.target === "taxonomyNode" || props.target === "tag");
const hasSortOrderField = computed(() => props.target === "dictionaryItem");
const requiresEnglishName = computed(() => props.target === "dictionaryItem");
const namespacePrefix = computed(() => {
  if (props.target === "tag") return namespacePrefixForResourceType(form.resourceType);
  return namespacePrefixForModule(form.module);
});
const namespaceValue = computed(() => `${namespacePrefix.value}${String(form.namespaceSuffix || "").trim()}`);
const namespaceSuffixPlaceholder = computed(() =>
  props.target === "tag"
    ? t("settings.metadataGovernance.form.namespaceSuffixPlaceholderTag")
    : t("settings.metadataGovernance.form.namespaceSuffixPlaceholder"),
);
const parentNodeItems = computed(() => [
  { label: t("settings.metadataGovernance.form.rootNode"), value: ROOT_NODE },
  ...(props.taxonomyNodes ?? []).map((node) => ({
    label: `${"  ".repeat(Math.max(0, Number(node.depth || 1) - 1))}${node.display_name}`,
    value: node.uuid,
  })),
]);

const i18nMap = (values: Record<string, string>) => {
  const result: Record<string, string> = {};
  for (const localeItem of localeOptions.value) {
    const value = String(values[localeItem.value] || "").trim();
    if (value) result[localeItem.value] = value;
  }
  return result;
};

const optionalI18nMap = (values: Record<string, string>) => {
  const result: Record<string, string> = {};
  for (const localeItem of localeOptions.value) {
    const value = String(values[localeItem.value] || "").trim();
    if (value) result[localeItem.value] = value;
  }
  return Object.keys(result).length > 0 ? result : undefined;
};

const submit = () => {
  validationError.value = "";
  if (!String(form.names[requiredLocale] || "").trim()) {
    validationError.value = t("settings.metadataGovernance.create.validation.localizedNameRequired");
    return;
  }

  const namePayload = {
    name_i18n: i18nMap(form.names),
    description_i18n: optionalI18nMap(form.descriptions),
  };
  const labelPayload = {
    label_i18n: i18nMap(form.names),
    description_i18n: optionalI18nMap(form.descriptions),
  };

  if (props.target === "dictionaryNamespace") {
    const namespace = namespaceValue.value;
    if (!namespace || !form.namespaceSuffix.trim() || !form.module.trim()) return failRequired();
    if (!isMachineIdentifier(namespace) || !isMachineIdentifier(form.module)) return failMachineIdentifierInvalid();
    if (!isNamespaceInModule(namespace, form.module)) return failNamespaceModuleMismatch();
    emit("submit", { namespace, module: form.module.trim(), ...namePayload });
    return;
  }
  if (props.target === "dictionaryItem") {
    const code = generatedCode.value;
    if (!code) return failEnglishNameRequired();
    if (!isMachineIdentifier(code)) return failMachineIdentifierInvalid();
    emit("submit", { code, sort_order: Number(form.sortOrder) || 0, ...labelPayload });
    return;
  }
  if (props.target === "taxonomy") {
    const namespace = namespaceValue.value;
    if (!namespace || !form.namespaceSuffix.trim() || !form.module.trim() || Number(form.maxDepth) < 1) return failRequired();
    if (!isMachineIdentifier(namespace) || !isMachineIdentifier(form.module)) return failMachineIdentifierInvalid();
    if (!isNamespaceInModule(namespace, form.module)) return failNamespaceModuleMismatch();
    emit("submit", { namespace, module: form.module.trim(), max_depth: Number(form.maxDepth), ...namePayload });
    return;
  }
  if (props.target === "taxonomyNode") {
    if (!form.code.trim()) return failRequired();
    if (!isMachineIdentifier(form.code)) return failMachineIdentifierInvalid();
    emit("submit", {
      parent_uuid: form.parentUuid === ROOT_NODE ? null : form.parentUuid,
      code: form.code.trim(),
      sort_order: Number(form.sortOrder) || 0,
      ...labelPayload,
    });
    return;
  }
  if (props.target === "tag") {
    if (resourceTypeOptions.value.length === 0) return failNoResourceType();
    const namespace = namespaceValue.value;
    if (!namespace || !form.namespaceSuffix.trim() || !form.resourceType.trim() || !form.code.trim()) return failRequired();
    if (!isMachineIdentifier(namespace) || !isMachineIdentifier(form.resourceType) || !isMachineIdentifier(form.code)) return failMachineIdentifierInvalid();
    if (!isNamespaceInModule(namespace, String(selectedResourceType.value?.module || ""))) return failNamespaceModuleMismatch();
    emit("submit", {
      namespace,
      resource_type: form.resourceType.trim(),
      code: form.code.trim(),
      color: form.color.trim(),
      ...labelPayload,
    });
    return;
  }
  if (props.target === "resourceType") {
    if (!form.resourceType.trim() || !form.module.trim()) return failRequired();
    if (!isMachineIdentifier(form.resourceType) || !isMachineIdentifier(form.module)) return failMachineIdentifierInvalid();
    emit("submit", {
      resource_type: form.resourceType.trim(),
      module: form.module.trim(),
      validator_key: form.validatorKey.trim(),
      binding_enabled: form.bindingEnabled,
      ...namePayload,
    });
  }
};

const failRequired = () => {
  validationError.value = t("settings.metadataGovernance.create.validation.required");
};
const failNoResourceType = () => {
  validationError.value = t("settings.metadataGovernance.create.validation.resourceTypeRequired");
};
const failEnglishNameRequired = () => {
  validationError.value = t("settings.metadataGovernance.create.validation.englishNameRequired");
};
const failMachineIdentifierInvalid = () => {
  validationError.value = t("settings.metadataGovernance.create.validation.machineIdentifierInvalid");
};
const failNamespaceModuleMismatch = () => {
  validationError.value = t("settings.metadataGovernance.create.validation.namespaceModuleMismatch");
};

const generatedCode = computed(() => {
  const source = String(form.names[englishLocale] || "").trim();
  if (!source) return "";
  const ascii = source
    .normalize("NFKD")
    .toLowerCase()
    .replace(/[^a-z0-9._-]+/g, "_")
    .replace(/^[._-]+|[._-]+$/g, "")
    .replace(/[._-]{2,}/g, "_");
  return ascii.slice(0, 64).replace(/[._-]+$/g, "");
});

const isMachineIdentifier = (value: string) => {
  return MACHINE_IDENTIFIER_RE.test(String(value || "").trim());
};
const isNamespaceInModule = (namespace: string, module: string) => {
  const normalizedNamespace = String(namespace || "").trim();
  const normalizedModule = String(module || "").trim();
  return Boolean(normalizedModule) && (normalizedNamespace === normalizedModule || normalizedNamespace.startsWith(`${normalizedModule}.`));
};
</script>
