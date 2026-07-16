<template>
  <MetadataStatePanel
    v-if="rows.length === 0"
    state="empty"
    :title="emptyTitle"
    :description="emptyDescription"
  />
  <div v-else class="space-y-2">
    <div
      v-for="row in treeRows"
      :key="row.uuid"
      class="rounded-md border border-gray-200 bg-white px-3 py-2 text-sm dark:border-gray-800 dark:bg-gray-950"
    >
      <div class="flex min-w-0 items-start gap-3" :style="{ paddingLeft: `${row.level * 20}px` }">
        <div class="mt-1 flex h-5 w-5 shrink-0 items-center justify-center">
          <UIcon
            v-if="row.level > 0"
            name="i-lucide-corner-down-right"
            class="h-4 w-4 text-gray-400"
          />
          <span v-else class="h-2.5 w-2.5 rounded-full bg-primary-500" />
        </div>
        <div class="min-w-0 flex-1">
          <div class="flex flex-wrap items-center gap-2">
            <span class="font-medium text-gray-900 dark:text-white">{{ row.display_name }}</span>
            <UBadge size="xs" variant="subtle" color="neutral">{{ row.code }}</UBadge>
            <UBadge size="xs" variant="subtle" :color="statusColor(row.status)">
              {{ t(`settings.metadataGovernance.status.${row.status}`) }}
            </UBadge>
          </div>
          <div class="mt-1 flex flex-wrap gap-3 text-xs text-gray-500">
            <span>{{ t("settings.metadataGovernance.taxonomy.depth", { depth: row.depth }) }}</span>
            <span>{{ t("settings.metadataGovernance.taxonomy.children", { count: row.child_count }) }}</span>
            <span>{{ t("settings.metadataGovernance.taxonomy.references", { count: row.reference_count ?? 0 }) }}</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import MetadataStatePanel from "~/components/settings/metadata-governance/MetadataStatePanel.vue";
import type { MetadataStatus, TaxonomyNode } from "~/types/metadata-governance";

const props = defineProps<{
  rows: TaxonomyNode[]
  emptyTitle: string
  emptyDescription: string
}>();

const { t } = useI18n();

type TreeRow = TaxonomyNode & {
  level: number
  child_count: number
}

const treeRows = computed<TreeRow[]>(() => {
  const byParent = new Map<string, TaxonomyNode[]>();
  const childCounts = new Map<string, number>();
  const byUuid = new Set(props.rows.map((row) => row.uuid));

  for (const row of props.rows) {
    const parentUuid = row.parent_uuid && byUuid.has(row.parent_uuid) ? row.parent_uuid : "";
    const siblings = byParent.get(parentUuid) ?? [];
    siblings.push(row);
    byParent.set(parentUuid, siblings);
    if (row.parent_uuid) childCounts.set(row.parent_uuid, (childCounts.get(row.parent_uuid) ?? 0) + 1);
  }

  for (const siblings of byParent.values()) {
    siblings.sort((left, right) => {
      const orderDiff = (left.sort_order ?? 0) - (right.sort_order ?? 0);
      if (orderDiff !== 0) return orderDiff;
      return left.display_name.localeCompare(right.display_name);
    });
  }

  const output: TreeRow[] = [];
  const walk = (parentUuid: string, level: number) => {
    for (const row of byParent.get(parentUuid) ?? []) {
      output.push({ ...row, level, child_count: childCounts.get(row.uuid) ?? 0 });
      walk(row.uuid, level + 1);
    }
  };

  walk("", 0);
  return output;
});

const statusColor = (status: MetadataStatus) =>
  status === "enabled" ? "success" : status === "disabled" ? "warning" : "neutral";
</script>
