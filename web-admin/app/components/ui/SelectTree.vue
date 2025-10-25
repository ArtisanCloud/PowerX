<!-- components/ui/SelectTree.vue -->
<script setup lang="ts">
import { ref, computed, watch } from "vue";

/** ==== 类型 ==== */
interface TreeNode {
  label: string;
  value: string;
  defaultExpanded?: boolean;
  children?: TreeNode[];
  disabled?: boolean;
  icon?: string;
}
type ModelInput = string | string[] | TreeNode | TreeNode[] | null;

interface Props {
  id?: string;
  modelValue?: ModelInput;
  items: TreeNode[];
  placeholder?: string;
  disabled?: boolean;
  size?: "sm" | "md" | "lg";
  variant?: "outline" | "subtle" | "ghost" | "link";
  color?:
    | "primary"
    | "success"
    | "neutral"
    | "warning"
    | "secondary"
    | "info"
    | "error";
  icon?: string;
  loading?: boolean;
  clearable?: boolean;
  searchable?: boolean;
  multiple?: boolean;
  popoverClass?: string;
  treeClass?: string;
  buttonClass?: string;
}

const props = withDefaults(defineProps<Props>(), {
  placeholder: "请选择",
  size: "md",
  variant: "outline",
  disabled: false,
  loading: false,
  clearable: false,
  searchable: false,
  multiple: false,
  popoverClass: "",
  treeClass: "w-64 max-h-64 overflow-auto",
  buttonClass: "",
});

const emit = defineEmits<{
  "update:modelValue": [value: string | string[] | null];
  change: [value: string | string[] | null];
  clear: [];
  open: [];
  close: [];
}>();

/** ==== 工具函数 ==== */
// 把对象/对象数组归一化成 value/value[]
const normalizeToValue = (
  v: ModelInput,
  multiple = false
): string | string[] | null => {
  if (v == null) return multiple ? [] : null;
  if (Array.isArray(v))
    return v.map((x) => (typeof x === "object" ? (x?.value ?? "") : String(x)));
  return typeof v === "object" ? (v?.value ?? null) : String(v);
};

/** ==== 状态 ==== */
const open = ref(false);
const searchQuery = ref("");
const internalValue = ref<string | string[] | null>(
  normalizeToValue(props.modelValue ?? null, !!props.multiple)
);
const expanded = ref<string[]>([]);

/** ==== 依赖数据 ==== */
// 扁平索引，便于快速找节点/叶子判断/取 label
const flatMap = computed(() => {
  const map = new Map<string, TreeNode & { parent?: string }>();
  const walk = (items: TreeNode[], parent?: string) => {
    for (const it of items) {
      map.set(it.value, { ...it, parent });
      if (it.children?.length) walk(it.children, it.value);
    }
  };
  walk(props.items);
  return map;
});
const isLeaf = (val: string | null) => {
  if (!val) return false;
  const n = flatMap.value.get(val);
  return !n?.children?.length;
};

// 默认展开：收集一次（或当 items 变化时）
const collectExpanded = (items: TreeNode[], acc: string[] = []) => {
  for (const it of items) {
    if (it.defaultExpanded && it.value) acc.push(it.value);
    if (it.children?.length) collectExpanded(it.children, acc);
  }
  return acc;
};
watch(
  () => props.items,
  (v) => {
    expanded.value = collectExpanded(v);
  },
  { immediate: true }
);

/** ==== 搜索过滤（不再动 defaultExpanded，展开全由 expanded 受控） ==== */
const filteredItems = computed(() => {
  if (!props.searchable || !searchQuery.value.trim()) return props.items;
  const q = searchQuery.value.toLowerCase();

  const filterTree = (items: TreeNode[]): TreeNode[] =>
    items
      .map((it) => {
        const match = it.label.toLowerCase().includes(q);
        const kids = it.children ? filterTree(it.children) : [];
        if (match || kids.length) {
          return {
            ...it,
            // 命中父：保留原 children；只命中子孙：保留过滤后的
            children: match ? it.children : kids.length ? kids : undefined,
          };
        }
        return null;
      })
      .filter(Boolean) as TreeNode[];

  return filterTree(props.items);
});

/** ==== 父节点文本=选中；箭头=展开/收起 ==== */
const selectById = (id: string) => {
  internalValue.value = props.multiple ? [id] : id;
};
// 包装 items：给“有子节点”的项挂 onSelect，拦截默认并改为“选中自身”
const interactiveItems = computed(() => {
  const wrap = (items: TreeNode[]): any[] =>
    items.map((it) => {
      const hasChildren = !!(it.children && it.children.length);
      return {
        ...it,
        onSelect: hasChildren
          ? (e: Event) => {
              e.preventDefault();
              selectById(it.value);
            }
          : undefined,
        children: hasChildren ? wrap(it.children!) : undefined,
      };
    });
  return wrap(filteredItems.value);
});

/** ==== 显示文本 ==== */
const displayLabel = computed(() => {
  const v = internalValue.value;
  if (!v || (Array.isArray(v) && !v.length)) return props.placeholder;
  const first = Array.isArray(v) ? v[0] : v;
  return flatMap.value.get(first)?.label ?? props.placeholder;
});

/** ==== 同步外部值 → 内部 ==== */
watch(
  () => props.modelValue,
  (val) =>
    (internalValue.value = normalizeToValue(val ?? null, !!props.multiple))
);

/** ==== UTree 回传归一化（避免对象直接进 internalValue） ==== */
const onTreeUpdate = (value: any) => {
  internalValue.value = normalizeToValue(value, !!props.multiple);
};

/** ==== 内部值变化 → 对外派发；仅“选中叶子”时关闭 Popover（单选） ==== */
watch(internalValue, (val) => {
  const out = val ?? (props.multiple ? [] : null);
  emit("update:modelValue", out);
  emit("change", out);

  if (!props.multiple) {
    const first = Array.isArray(val) ? (val[0] ?? null) : (val ?? null);
    if (isLeaf(first)) {
      open.value = false;
      emit("close");
    }
  }
});

/** ==== 其它操作 ==== */
const handleClear = (e: Event) => {
  e.stopPropagation();
  internalValue.value = props.multiple ? [] : null;
  emit("clear");
};
const handleClose = () => {
  open.value = false;
  searchQuery.value = "";
  emit("close");
};
</script>

<template>
  <UPopover
    v-model:open="open"
    @open="emit('open')"
    @close="handleClose"
    :class="popoverClass"
  >
    <UButton
      :id="props.id"
      :label="displayLabel"
      :variant="variant"
      :size="size"
      :color="color"
      :disabled="disabled"
      :loading="loading"
      :icon="icon"
      :class="[
        'justify-between min-w-0',
        buttonClass,
        {
          'text-gray-400':
            !internalValue ||
            (Array.isArray(internalValue) && !internalValue.length),
          'pr-8':
            clearable &&
            internalValue &&
            (!Array.isArray(internalValue) || internalValue.length),
        },
      ]"
    >
      <template #trailing>
        <div class="flex items-center gap-1">
          <UButton
            v-if="
              clearable &&
              internalValue &&
              (!Array.isArray(internalValue) || internalValue.length) &&
              !disabled &&
              !loading
            "
            icon="i-heroicons-x-mark-20-solid"
            size="xs"
            color="neutral"
            variant="ghost"
            @click="handleClear"
          />
          <UIcon
            name="i-heroicons-chevron-down-20-solid"
            class="w-4 h-4 transition-transform duration-200"
            :class="{ 'rotate-180': open }"
          />
        </div>
      </template>
    </UButton>

    <template #content>
      <div class="p-2">
        <div v-if="searchable" class="mb-2">
          <UInput
            v-model="searchQuery"
            icon="i-heroicons-magnifying-glass-20-solid"
            placeholder="搜索..."
            size="sm"
          />
        </div>

        <!-- 用受控 expanded；避免 v-model 直接塞入对象，改为 :model-value + @update -->
        <UTree
          :model-value="internalValue"
          @update:modelValue="onTreeUpdate"
          v-model:expanded="expanded"
          :items="interactiveItems"
          :multiple="multiple"
          value-key="value"
          label-key="label"
          :class="treeClass"
        />

        <div
          v-if="searchable && searchQuery && filteredItems.length === 0"
          class="text-center py-4 text-gray-500 text-sm"
        >
          <UIcon
            name="i-heroicons-magnifying-glass-20-solid"
            class="w-8 h-8 mx-auto mb-2 text-gray-300"
          />
          <p>未找到匹配项</p>
        </div>
      </div>
    </template>
  </UPopover>
</template>
