<template>
  <DatabaseBackupPolicies
    v-if="kind === 'backup'"
    @busy="emit('busy', $event)"
    @attention="emit('attention', $event)"
    @changed="emit('changed')"
  />
  <SystemSnapshotPolicies
    v-else
    @busy="emit('busy', $event)"
    @attention="emit('attention', $event)"
    @changed="emit('changed')"
  />
</template>
<script setup lang="ts">
import DatabaseBackupPolicies from "~/components/ops/backup/DatabaseBackupPolicies.vue";
import SystemSnapshotPolicies from "~/components/ops/backup/SystemSnapshotPolicies.vue";
defineProps<{ kind: "backup" | "snapshot"; revision?: number }>();
const emit = defineEmits<{
  busy: [boolean];
  attention: [boolean];
  changed: [];
}>();
</script>
