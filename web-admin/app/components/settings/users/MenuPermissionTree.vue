<script setup lang="ts">
type MenuPermissionNode = {
  key: string;
  label: string;
  icon: string;
  hint?: string;
  permission?: { id: number; code?: string; resource?: string };
  children: MenuPermissionNode[];
};

const props = defineProps<{
  nodes: MenuPermissionNode[];
  selectedIds: number[];
  level?: number;
}>();

const emit = defineEmits<{
  toggle: [ids: number[], checked: boolean];
}>();

const collectExpandableKeys = (nodes: MenuPermissionNode[] = []): string[] => {
  const keys: string[] = [];
  for (const node of nodes) {
    if (node.children?.length) {
      keys.push(node.key, ...collectExpandableKeys(node.children));
    }
  }
  return keys;
};

const expandedKeys = ref<Set<string>>(new Set());

watch(
  () => props.nodes,
  (nodes) => {
    expandedKeys.value = new Set(collectExpandableKeys(nodes));
  },
  { immediate: true },
);

const collectIds = (node: MenuPermissionNode): number[] => {
  const ids = new Set<number>();
  if (node.permission) ids.add(node.permission.id);
  for (const child of node.children || []) {
    for (const id of collectIds(child)) ids.add(id);
  }
  return Array.from(ids);
};

const collectVisibleStateIds = (node: MenuPermissionNode): number[] => {
  if (node.children?.length) {
    return node.children.flatMap((child) => collectVisibleStateIds(child));
  }
  return node.permission ? [node.permission.id] : [];
};

const selectedSet = computed(() => new Set(props.selectedIds || []));

const isFullySelected = (node: MenuPermissionNode) => {
  const ids = collectVisibleStateIds(node);
  return ids.length > 0 && ids.every((id) => selectedSet.value.has(id));
};

const isPartiallySelected = (node: MenuPermissionNode) => {
  const ids = collectVisibleStateIds(node);
  const picked = ids.filter((id) => selectedSet.value.has(id)).length;
  return picked > 0 && picked < ids.length;
};

const checkboxValue = (node: MenuPermissionNode) => {
  if (isFullySelected(node)) return true;
  if (isPartiallySelected(node)) return "indeterminate";
  return false;
};

const toggleNode = (node: MenuPermissionNode, checked: boolean) => {
  emit("toggle", collectIds(node), checked === true);
};

const isExpandable = (node: MenuPermissionNode) => (node.children || []).length > 0;

const isExpanded = (node: MenuPermissionNode) => {
  if (!isExpandable(node)) return false;
  return expandedKeys.value.has(node.key);
};

const toggleExpanded = (node: MenuPermissionNode) => {
  if (!isExpandable(node)) return;
  const next = new Set(expandedKeys.value);
  if (next.has(node.key)) next.delete(node.key);
  else next.add(node.key);
  expandedKeys.value = next;
};
</script>

<template>
  <div class="space-y-1">
    <div v-for="node in nodes" :key="node.key">
      <div
        class="flex items-center gap-2 rounded-md border border-gray-100 px-3 py-2 hover:bg-gray-50"
        :style="{ marginLeft: `${(level || 0) * 18}px` }"
      >
        <UButton
          v-if="isExpandable(node)"
          color="neutral"
          variant="ghost"
          size="xs"
          square
          :icon="
            isExpanded(node)
              ? 'i-heroicons-chevron-down'
              : 'i-heroicons-chevron-right'
          "
          @click.stop="toggleExpanded(node)"
        />
        <span v-else class="h-6 w-6 shrink-0" />
        <UCheckbox
          :model-value="checkboxValue(node)"
          @update:model-value="toggleNode(node, $event as boolean)"
        />
        <UIcon :name="node.icon" class="h-4 w-4 shrink-0 text-gray-500" />
        <div class="min-w-0 flex-1">
          <div
            class="truncate text-sm font-medium text-gray-900"
            :title="node.permission?.code || node.permission?.resource || node.key"
          >
            {{ node.label }}
          </div>
          <div
            v-if="node.hint"
            class="truncate text-xs text-gray-500"
            :title="node.permission?.code || node.permission?.resource"
          >
            {{ node.hint }}
          </div>
        </div>
        <UBadge
          v-if="node.children?.length"
          size="xs"
          color="neutral"
          variant="subtle"
        >
          {{ node.children.length }}
        </UBadge>
      </div>

      <MenuPermissionTree
        v-if="node.children?.length && isExpanded(node)"
        class="mt-1"
        :nodes="node.children"
        :selected-ids="selectedIds"
        :level="(level || 0) + 1"
        @toggle="(ids, checked) => emit('toggle', ids, checked)"
      />
    </div>
  </div>
</template>
