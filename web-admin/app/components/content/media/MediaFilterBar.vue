<template>
  <div class="flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between">
    <div class="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-4 xl:gap-4 flex-1">
      <UInput v-model="local.keyword" placeholder="搜索名称（keyword）" class="w-full" />
      <USelect
        v-model="local.businessStatus"
        :items="statusOptions"
        placeholder="业务状态"
        class="w-full"
      />
      <USelect v-model="local.driver" :items="driverOptions" placeholder="存储驱动" class="w-full" />
      <UInput v-model="local.tagsText" placeholder="标签（逗号分隔，AND）" class="w-full" />
    </div>

    <div class="flex flex-col gap-2 sm:flex-row sm:flex-wrap sm:items-center sm:justify-end">
      <UCheckbox v-model="local.onlyDeleted" label="回收站（仅软删）" />
      <UButton color="primary" size="sm" @click="emitApply">应用筛选</UButton>
      <UButton color="neutral" variant="subtle" size="sm" type="button" @click="emitReset">
        重置
      </UButton>
      <UButtonGroup size="sm" orientation="horizontal">
        <UButton :variant="viewMode === 'grid' ? 'solid' : 'outline'" @click="$emit('update:viewMode', 'grid')">
          网格
        </UButton>
        <UButton :variant="viewMode === 'table' ? 'solid' : 'outline'" @click="$emit('update:viewMode', 'table')">
          表格
        </UButton>
      </UButtonGroup>
    </div>
  </div>
</template>

<script setup lang="ts">
type ViewMode = "grid" | "table";

export interface MediaFilterState {
  keyword: string;
  businessStatus: string;
  driver: string;
  tagsText: string;
  onlyDeleted: boolean;
}

const props = defineProps<{
  modelValue: MediaFilterState;
  viewMode: ViewMode;
}>();

const emit = defineEmits<{
  (e: "update:modelValue", value: MediaFilterState): void;
  (e: "update:viewMode", value: ViewMode): void;
  (e: "apply"): void;
  (e: "reset"): void;
}>();

const local = reactive<MediaFilterState>({
  keyword: "",
  businessStatus: "",
  driver: "",
  tagsText: "",
  onlyDeleted: false,
});

function toInternalSelectValue(v: string | null | undefined) {
  // reka-ui 的 SelectItem 禁止 value=""，空值用 null 表示
  const s = String(v ?? "").trim();
  return s === "" ? null : s;
}

function toExternalSelectValue(v: unknown) {
  if (v == null) return "";
  return String(v);
}

watch(
  () => props.modelValue,
  (next) => {
    const n = next || ({} as any);
    local.keyword = String(n.keyword ?? "");
    local.businessStatus = toExternalSelectValue(toInternalSelectValue(n.businessStatus));
    local.driver = toExternalSelectValue(toInternalSelectValue(n.driver));
    local.tagsText = String(n.tagsText ?? "");
    local.onlyDeleted = !!n.onlyDeleted;
  },
  { immediate: true, deep: true }
);

watch(
  local,
  () => {
    emit("update:modelValue", {
      ...local,
      businessStatus: toExternalSelectValue(toInternalSelectValue(local.businessStatus)),
      driver: toExternalSelectValue(toInternalSelectValue(local.driver)),
    });
  },
  { deep: true }
);

const statusOptions = [
  { label: "全部状态", value: null },
  { label: "draft", value: "draft" },
  { label: "under_review", value: "under_review" },
  { label: "published", value: "published" },
  { label: "archived", value: "archived" },
];

const driverOptions = [
  { label: "全部驱动", value: null },
  { label: "local", value: "local" },
  { label: "s3", value: "s3" },
];

const viewMode = computed(() => props.viewMode);

const emitApply = () => emit("apply");
const emitReset = () => emit("reset");
</script>
