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
            <UInput
              v-model="form.module"
              class="w-full"
              :placeholder="t('settings.metadataGovernance.form.modulePlaceholder')"
            />
            <div v-if="moduleOptions.length > 0" class="mt-2 flex flex-wrap gap-2">
              <UButton
                v-for="moduleItem in moduleOptions"
                :key="moduleItem.value"
                type="button"
                size="xs"
                variant="soft"
                color="neutral"
                @click="form.module = moduleItem.value"
              >
                {{ moduleItem.label }}
              </UButton>
            </div>
          </UFormField>
          <UFormField :label="namespaceLabel" required>
            <UInput v-model="form.namespace" class="w-full" :placeholder="namespacePlaceholder" />
          </UFormField>
        </div>

        <div v-if="needsResourceTypeField" class="grid grid-cols-1 gap-4 md:grid-cols-2">
          <UFormField :label="t('settings.metadataGovernance.form.resourceType')" required>
            <UInput v-model="form.resourceType" class="w-full" :placeholder="t('settings.metadataGovernance.form.resourceTypePlaceholder')" />
          </UFormField>
          <UFormField :label="t('settings.metadataGovernance.form.module')" required>
            <UInput
              v-model="form.module"
              class="w-full"
              :placeholder="t('settings.metadataGovernance.form.modulePlaceholder')"
            />
            <div v-if="moduleOptions.length > 0" class="mt-2 flex flex-wrap gap-2">
              <UButton
                v-for="moduleItem in moduleOptions"
                :key="moduleItem.value"
                type="button"
                size="xs"
                variant="soft"
                color="neutral"
                @click="form.module = moduleItem.value"
              >
                {{ moduleItem.label }}
              </UButton>
            </div>
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
              class="w-full"
              :placeholder="t('settings.metadataGovernance.form.resourceTypeSelectPlaceholder')"
            />
          </UFormField>
          <UFormField :label="t('settings.metadataGovernance.form.namespace')" required>
            <UInput v-model="form.namespace" class="w-full" :placeholder="t('settings.metadataGovernance.form.namespacePlaceholder')" />
          </UFormField>
        </div>

        <div v-if="hasCodeField" class="grid grid-cols-1 gap-4 md:grid-cols-2">
          <UFormField :label="t('settings.metadataGovernance.form.code')" required>
            <UInput v-model="form.code" class="w-full" :placeholder="t('settings.metadataGovernance.form.codePlaceholder')" />
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
          <div class="mb-3 flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
            <div>
              <div class="text-sm font-medium text-gray-900 dark:text-white">
                {{ t("settings.metadataGovernance.form.optionalLocaleSection") }}
              </div>
              <div class="text-xs text-gray-500">
                {{ t("settings.metadataGovernance.form.optionalLocaleDescription") }}
              </div>
            </div>
            <div class="w-full md:w-72">
              <UInput
                v-model="localeSearch"
                icon="i-lucide-search"
                class="w-full"
                :placeholder="t('settings.metadataGovernance.form.localeSearchPlaceholder')"
              />
              <div
                v-if="filteredOptionalLocaleOptions.length > 0"
                class="mt-2 flex max-h-28 flex-wrap gap-2 overflow-y-auto rounded-md border border-gray-200 p-2 dark:border-gray-800"
              >
                <UButton
                  v-for="localeItem in filteredOptionalLocaleOptions"
                  :key="localeItem.value"
                  type="button"
                  size="xs"
                  :variant="localeItem.value === selectedOptionalLocale ? 'solid' : 'soft'"
                  :color="localeItem.value === selectedOptionalLocale ? 'primary' : 'neutral'"
                  @click="selectedOptionalLocale = localeItem.value"
                >
                  {{ localeItem.label }}
                </UButton>
              </div>
            </div>
          </div>

          <div v-if="selectedOptionalLocale" class="grid grid-cols-1 gap-4 md:grid-cols-2">
            <UFormField :label="t('settings.metadataGovernance.form.localizedName')">
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

const emptyForm = () => ({
  namespace: "",
  module: props.defaultModule || moduleOptions.value[0]?.value || "",
  resourceType: resourceTypeOptions.value[0]?.value || "",
  code: "",
  names: Object.fromEntries(localeOptions.value.map((item) => [item.value, ""])) as Record<string, string>,
  descriptions: Object.fromEntries(localeOptions.value.map((item) => [item.value, ""])) as Record<string, string>,
  sortOrder: 0,
  maxDepth: 3,
  color: "#2563eb",
  validatorKey: "",
  bindingEnabled: true,
  parentUuid: ROOT_NODE,
});
const defaultTagNamespace = () => String(resourceTypeOptions.value[0]?.module || "").trim();

const form = reactive(emptyForm());
const validationError = ref("");
const selectedOptionalLocale = ref(defaultOptionalLocale());
const localeSearch = ref("");
const error = computed(() => validationError.value || props.errorMessage || "");
const filledOptionalLocales = computed(() =>
  optionalLocaleOptions.value.filter((item) =>
    String(form.names[item.value] || "").trim() || String(form.descriptions[item.value] || "").trim(),
  ),
);
const filteredOptionalLocaleOptions = computed(() => {
  const query = localeSearch.value.trim().toLowerCase();
  if (!query) return optionalLocaleOptions.value;
  return optionalLocaleOptions.value.filter((item) =>
    item.label.toLowerCase().includes(query) || item.value.toLowerCase().includes(query),
  );
});

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
    localeSearch.value = "";
    if (props.target === "tag" && !form.namespace.trim()) {
      form.namespace = defaultTagNamespace();
    }
    validationError.value = "";
  },
);

watch(
  () => form.module,
  (moduleValue, oldModuleValue) => {
    if (!needsDefinitionFields.value) return;
    const trimmed = String(moduleValue || "").trim();
    if (!trimmed) return;
    if (!form.namespace.trim() || form.namespace.trim() === `${oldModuleValue}.`) {
      form.namespace = `${trimmed}.`;
    }
  },
);

watch(
  () => form.resourceType,
  () => {
    if (props.target !== "tag") return;
    const moduleValue = String(selectedResourceType.value?.module || "").trim();
    if (moduleValue && !form.namespace.trim()) {
      form.namespace = moduleValue;
    }
  },
);

const needsDefinitionFields = computed(() => props.target === "dictionaryNamespace" || props.target === "taxonomy");
const needsResourceTypeField = computed(() => props.target === "resourceType");
const hasCodeField = computed(() => props.target === "dictionaryItem" || props.target === "taxonomyNode" || props.target === "tag");
const hasSortOrderField = computed(() => props.target === "dictionaryItem");
const namespaceLabel = computed(() => t("settings.metadataGovernance.form.namespace"));
const namespacePlaceholder = computed(() => t("settings.metadataGovernance.form.namespacePlaceholder"));
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
    if (!form.namespace.trim() || !form.module.trim()) return failRequired();
    emit("submit", { namespace: form.namespace.trim(), module: form.module.trim(), ...namePayload });
    return;
  }
  if (props.target === "dictionaryItem") {
    if (!form.code.trim()) return failRequired();
    emit("submit", { code: form.code.trim(), sort_order: Number(form.sortOrder) || 0, ...labelPayload });
    return;
  }
  if (props.target === "taxonomy") {
    if (!form.namespace.trim() || !form.module.trim() || Number(form.maxDepth) < 1) return failRequired();
    emit("submit", { namespace: form.namespace.trim(), module: form.module.trim(), max_depth: Number(form.maxDepth), ...namePayload });
    return;
  }
  if (props.target === "taxonomyNode") {
    if (!form.code.trim()) return failRequired();
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
    if (!form.namespace.trim() || !form.resourceType.trim() || !form.code.trim()) return failRequired();
    emit("submit", {
      namespace: form.namespace.trim(),
      resource_type: form.resourceType.trim(),
      code: form.code.trim(),
      color: form.color.trim(),
      ...labelPayload,
    });
    return;
  }
  if (props.target === "resourceType") {
    if (!form.resourceType.trim() || !form.module.trim()) return failRequired();
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
</script>
