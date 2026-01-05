<template>
  <div class="flex flex-col gap-3 md:flex-row md:items-end md:justify-between">
    <div class="grid grid-cols-1 gap-3 md:grid-cols-4">
      <UInput v-model="local.keyword" placeholder="搜索名称（keyword）" />
      <USelect v-model="local.businessStatus" :items="statusOptions" placeholder="业务状态" />
      <USelect v-model="local.driver" :items="driverOptions" placeholder="存储驱动" />
      <UInput v-model="local.tagsText" placeholder="标签（逗号分隔，AND）" />
    </div>

    <div class="flex flex-wrap items-center gap-2">
      <UCheckbox v-model="local.onlyDeleted" label="回收站（仅软删）" />
      <UButton size="sm" @click="emitApply">应用筛选</UButton>
      <UButton size="sm" variant="ghost" @click="emitReset">重置</UButton>
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

watch(
  () => props.modelValue,
  (next) => {
    Object.assign(local, next || {});
  },
  { immediate: true, deep: true }
);

watch(
  local,
  () => {
    emit("update:modelValue", { ...local });
  },
  { deep: true }
);

const statusOptions = [
  { label: "全部状态", value: "" },
  { label: "draft", value: "draft" },
  { label: "under_review", value: "under_review" },
  { label: "published", value: "published" },
  { label: "archived", value: "archived" },
];

const driverOptions = [
  { label: "全部驱动", value: "" },
  { label: "local", value: "local" },
  { label: "s3", value: "s3" },
];

const viewMode = computed(() => props.viewMode);

const emitApply = () => emit("apply");
const emitReset = () => emit("reset");
</script>
