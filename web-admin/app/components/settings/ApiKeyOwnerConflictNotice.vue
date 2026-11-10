<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useI18n } from '#imports';
import type { ApiKeyOwnerConflictReport } from '~/utils/apiKeyOwnerConflicts';

const props = defineProps<{
  report: ApiKeyOwnerConflictReport;
  operation: 'profile' | 'owners';
}>();
const { t } = useI18n();
const open = ref(false);
const summary = computed(() =>
  t('integrationApiKeyConflicts.summary', {
    count: props.report.conflict_count,
    keys: props.report.conflicting_key_count,
  }),
);
const groups = computed(() => {
  const items = new Map<string, typeof props.report.conflicts>();
  for (const conflict of props.report.conflicts) {
    if (!items.has(conflict.key_id)) items.set(conflict.key_id, []);
    items.get(conflict.key_id)!.push(conflict);
  }
  return [...items].map(([keyID, conflicts]) => ({
    keyID,
    name: conflicts[0]!.key_name,
    conflicts,
  }));
});
watch(
  () => props.report,
  () => {
    open.value = false;
  },
);
</script>

<template>
  <div data-testid="api-key-owner-conflict" class="min-w-0">
    <UAlert
      color="error"
      variant="subtle"
      icon="i-heroicons-exclamation-triangle"
      :title="t('integrationApiKeyConflicts.title')"
      :description="summary"
    >
      <template #actions>
        <UButton
          color="error"
          variant="outline"
          size="sm"
          @click="open = true"
          >{{ t('integrationApiKeyConflicts.details') }}</UButton
        >
      </template>
    </UAlert>
    <p role="status" class="mt-2 text-sm text-muted">
      {{ t(`integrationApiKeyConflicts.${operation}Unchanged`) }}
    </p>
    <UModal
      v-model:open="open"
      :title="t('integrationApiKeyConflicts.title')"
      :description="summary"
      :ui="{ content: 'max-w-3xl', footer: 'justify-end' }"
    >
      <template #body>
        <div class="space-y-4" data-testid="api-key-conflict-details">
          <p class="text-sm text-muted">
            {{ t('integrationApiKeyConflicts.clearHelp') }}
          </p>
          <p v-if="report.profile_name" class="text-sm break-all">
            Profile：{{ report.profile_name }}
          </p>
          <section
            v-for="group in groups"
            :key="group.keyID"
            class="min-w-0 rounded-lg border border-default p-3 space-y-3"
          >
            <header class="min-w-0">
              <h3 class="font-semibold break-all">
                {{ group.name || t('integrationApiKeyConflicts.unnamedKey') }}
              </h3>
              <p class="text-xs font-mono break-all text-muted">
                Key ID：{{ group.keyID }}
              </p>
            </header>
            <article
              v-for="conflict in group.conflicts"
              :key="`${group.keyID}-${conflict.permission_id}-${conflict.scope}-${conflict.action}`"
              class="min-w-0 border-t border-default pt-3 space-y-2"
            >
              <p class="font-medium break-all">
                {{
                  conflict.permission_name ||
                  t('integrationApiKeyConflicts.unnamedPermission')
                }}
                <span class="text-xs text-muted"
                  >#{{ conflict.permission_id }}</span
                >
              </p>
              <p
                v-if="conflict.capability_id"
                class="text-xs font-mono break-all"
              >
                {{ conflict.capability_id }}
              </p>
              <dl
                class="grid grid-cols-1 sm:grid-cols-[9rem_minmax(0,1fr)] gap-x-3 gap-y-1 text-sm"
              >
                <dt class="text-muted">
                  {{ t('integrationApiKeyConflicts.requiredPlugin') }}
                </dt>
                <dd class="font-mono break-all">
                  {{ conflict.required_plugin_id }}
                </dd>
                <dt class="text-muted">
                  {{ t(operation === 'owners' ? 'integrationApiKeyConflicts.submittedPlugins' : 'integrationApiKeyConflicts.configuredPlugins') }}
                </dt>
                <dd class="font-mono break-all">
                  {{
                    conflict.owner_plugin_ids.join(', ') ||
                    t('integrationApiKeyConflicts.unbound')
                  }}
                </dd>
                <template v-if="conflict.binding_mode === 'legacy_permissions'">
                  <dt class="text-muted">
                    {{ t('integrationApiKeyConflicts.permissionOwners') }}
                  </dt>
                  <dd class="font-mono break-all">
                    {{
                      conflict.permission_owner_plugin_ids.join(', ') ||
                      t('integrationApiKeyConflicts.unbound')
                    }}
                  </dd>
                </template>
                <dt class="text-muted">
                  {{ t('integrationApiKeyConflicts.action') }}
                </dt>
                <dd class="font-mono break-all">
                  {{ conflict.action }} · {{ conflict.resource_type }} /
                  {{ conflict.resource_pattern }}
                </dd>
                <dt class="text-muted">scope</dt>
                <dd class="font-mono break-all">{{ conflict.scope }}</dd>
              </dl>
            </article>
          </section>
          <p v-if="report.truncated" class="text-sm text-warning">
            {{
              t('integrationApiKeyConflicts.truncated', {
                count: report.conflicts.length,
                total: report.conflict_count,
              })
            }}
          </p>
          <p
            v-if="report.trace_id"
            class="text-xs text-muted font-mono break-all"
          >
            Trace ID：{{ report.trace_id }}
          </p>
        </div>
      </template>
      <template #footer
        ><UButton color="neutral" variant="outline" @click="open = false">{{
          t('integrationApiKeyConflicts.close')
        }}</UButton></template
      >
    </UModal>
  </div>
</template>
