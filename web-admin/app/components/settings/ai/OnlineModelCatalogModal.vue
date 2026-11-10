<template>
  <UModal :open="true" :title="$t('settings.ai.onlineModels.title')" :description="`${providerName} · ${env} · ${modality}`" :dismissible="!busy" :close="!busy" :ui="{ content: 'w-[calc(100vw-2rem)] max-w-4xl' }" @update:open="value => { if (!value) emit('close'); }">
    <template #body>
      <div class="space-y-4 min-w-0">
        <p class="text-sm text-muted">{{ $t('settings.ai.onlineModels.connectionHint') }}</p>
        <div v-if="connectionDirty" role="alert" class="rounded-lg bg-warning/10 p-3 text-sm text-warning">{{ $t('settings.ai.onlineModels.saveConnectionFirst') }}</div>
        <div v-if="busy" role="status" class="flex items-center gap-2 text-sm"><UIcon name="i-heroicons-arrow-path" class="animate-spin" />{{ $t('settings.ai.onlineModels.working') }}</div>
        <div v-if="errorCode" role="alert" class="rounded-lg border border-error/40 bg-error/10 p-3 space-y-2">
          <p>{{ errorMessage }}</p>
          <p class="text-xs text-muted break-all">{{ errorCode }}<span v-if="traceId"> · {{ traceId }}</span></p>
          <p v-if="attemptSource" class="text-xs text-muted break-all">{{ $t('settings.ai.onlineModels.source') }}：{{ attemptSource }}<span v-if="upstreamStatus"> · HTTP {{ upstreamStatus }}</span></p>
          <UButton type="button" size="sm" variant="soft" :disabled="busy || connectionDirty" @click="update">{{ $t('settings.ai.catalog.retry') }}</UButton>
        </div>
        <template v-if="catalog">
          <div class="rounded-lg bg-primary/10 p-3 space-y-1" role="status" data-testid="online-model-summary">
            <p class="font-medium">{{ $t('settings.ai.onlineModels.onlineSummary', { total: catalog.models.length, declared: declared.length, unknown: undeclared.length }) }}</p>
            <p class="text-xs text-muted break-all">{{ $t('settings.ai.onlineModels.source') }}：{{ catalog.source }}</p>
            <p class="text-xs text-muted">{{ $t('settings.ai.onlineModels.updatedAt') }}：{{ new Date(catalog.updated_at).toLocaleString() }} · {{ catalog.protocol }}</p>
          </div>
          <div class="flex flex-col sm:flex-row gap-4 min-w-0">
            <div role="tablist" :aria-label="$t('settings.ai.onlineModels.results')" class="flex sm:flex-col gap-2 sm:w-36 shrink-0 max-w-full overflow-x-auto sm:overflow-x-visible">
              <button v-for="item in tabs" :id="`online-tab-${item.value}`" :key="item.value" type="button" role="tab" :aria-selected="tab === item.value" :aria-controls="`online-panel-${item.value}`" :tabindex="tab === item.value ? 0 : -1" :class="['rounded-lg px-3 py-2 text-sm text-left', tab === item.value ? 'bg-primary/15 text-primary' : 'hover:bg-elevated']" @click="tab = item.value" @keydown="switchTab($event)">{{ item.label }}</button>
            </div>
            <section :id="`online-panel-${tab}`" role="tabpanel" :aria-labelledby="`online-tab-${tab}`" class="min-w-0 flex-1 space-y-3">
              <p class="text-sm text-muted">{{ $t('settings.ai.onlineModels.onlineSelectionHint') }}</p>
              <UInput v-model="query" icon="i-heroicons-magnifying-glass" :placeholder="$t('settings.ai.onlineModels.search')" class="w-full" />
              <div class="max-h-80 overflow-y-auto rounded-lg border border-default divide-y divide-default" data-testid="online-model-list">
                <p v-if="!visible.length" class="p-4 text-sm text-muted">{{ $t('settings.ai.onlineModels.empty') }}</p>
                <div v-for="item in visible" :key="item.id" class="flex items-start gap-3 p-3 min-w-0" :data-model-id="item.id">
                  <div class="min-w-0 flex-1">
                    <p class="text-sm break-all">{{ item.label || item.id }}</p>
                    <p class="font-mono text-xs text-muted break-all">{{ item.id }}</p>
                    <ModelParameterInfo :profile="item.parameter_profile" class="mt-2" />
                    <p class="text-xs" :class="item.reported_modalities?.length ? 'text-muted' : 'text-warning'">{{ item.reported_modalities?.join(', ') || $t('settings.ai.onlineModels.usageUnspecified') }}</p>
                    <p v-if="item.input_modalities?.length || item.output_modalities?.length" class="text-xs text-muted">{{ $t('settings.ai.onlineModels.ioTypes', { input: item.input_modalities?.join(', ') || '—', output: item.output_modalities?.join(', ') || '—' }) }}</p>
                    <p v-if="modality === 'vlm' && item.vision?.status !== 'unknown' && item.vision" class="text-xs" :class="item.vision.status === 'supported' ? 'text-primary' : 'text-error'">
                      {{ $t(visionUsageKey(item.vision)) }}
                      <a v-if="visionDocumentationURL(item.vision.source_url)" :href="visionDocumentationURL(item.vision.source_url)" target="_blank" rel="noopener noreferrer" class="ml-1 underline">{{ $t('settings.ai.visionModels.officialDocs') }} · {{ item.vision.checked_at }}</a>
                    </p>
                  </div>
                </div>
              </div>
            </section>
          </div>
        </template>
      </div>
    </template>
    <template #footer>
      <div class="flex w-full flex-wrap justify-end gap-2">
        <UButton type="button" color="neutral" variant="soft" :disabled="busy" @click="emit('close')">{{ $t('settings.ai.onlineModels.close') }}</UButton>
        <UButton type="button" icon="i-heroicons-cloud-arrow-down" :loading="busy" :disabled="busy || connectionDirty" @click="update">{{ $t('settings.ai.onlineModels.update') }}</UButton>
      </div>
    </template>
  </UModal>
</template>

<script setup lang="ts">
import { computed, ref, onMounted, onUnmounted } from 'vue';
import { AISettingService, type OnlineModelCatalog } from '~/composables/api/services/aiSettingService';
import { visionDocumentationURL, visionUsageKey } from '~/utils/ai-vision-model-choice';
import ModelParameterInfo from './ModelParameterInfo.vue';

const props = defineProps<{ provider: string; providerName: string; env: string; app: string; modality: string; connectionDirty: boolean }>();
const emit = defineEmits<{ (e: 'close'): void; (e: 'saved', catalog: OnlineModelCatalog): void }>();
const { t } = useI18n();
const catalog = ref<OnlineModelCatalog | null>(null);
const busy = ref(false), errorCode = ref(''), traceId = ref(''), query = ref('');
const attemptSource = ref(''), upstreamStatus = ref<number | undefined>();
const upstreamReason = ref('');
const tab = ref<'all' | 'declared' | 'undeclared'>('all');
let active = true;
const allModels = computed(() => catalog.value?.models || []);
const declared = computed(() => allModels.value.filter(item => item.reported_modalities?.length));
const undeclared = computed(() => allModels.value.filter(item => !item.reported_modalities?.length));
const tabs = computed(() => [{ value: 'all' as const, label: `${t('settings.ai.onlineModels.allOnline')} (${allModels.value.length})` }, { value: 'declared' as const, label: `${t('settings.ai.onlineModels.declared')} (${declared.value.length})` }, { value: 'undeclared' as const, label: `${t('settings.ai.onlineModels.usageUnspecified')} (${undeclared.value.length})` }]);
const visible = computed(() => ({ all: allModels.value, declared: declared.value, undeclared: undeclared.value }[tab.value]).filter(item => `${item.id} ${item.label}`.toLowerCase().includes(query.value.toLowerCase())));
const knownErrors = new Set(['AI_MODEL_SYNC_UNSUPPORTED', 'AI_SETTINGS_PROVIDER_NOT_CONFIGURED', 'AI_MODEL_SYNC_AUTH_FAILED', 'AI_MODEL_DISCOVERY_TIMEOUT', 'AI_MODEL_DISCOVERY_UNREACHABLE', 'AI_MODEL_DISCOVERY_UPSTREAM_FAILED', 'AI_MODEL_DISCOVERY_RESPONSE_INVALID', 'AI_MODEL_SYNC_STALE', 'AI_MODEL_SYNC_CONNECTION_CHANGED', 'AI_MODEL_SYNC_ADMIN_REQUIRED', 'AI_MODEL_SYNC_ASSIGNMENT_INVALID', 'AI_MODEL_SYNC_CONFIRMATION_REQUIRED', 'AI_MODEL_DISCOVERY_CONFIG_INVALID']);
const errorMessage = computed(() => t(errorCode.value === 'AI_MODEL_SYNC_AUTH_FAILED' && upstreamReason.value === 'legacy_free_key_retired'
  ? 'settings.ai.onlineModels.errors.legacyFreeKeyRetired'
  : `settings.ai.onlineModels.errors.${knownErrors.has(errorCode.value) ? errorCode.value : 'generic'}`));
function switchTab(event: KeyboardEvent) {
  if (!['ArrowDown', 'ArrowUp', 'ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return;
  event.preventDefault();
  const values = ['all', 'declared', 'undeclared'] as const;
  const step = ['ArrowUp', 'ArrowLeft'].includes(event.key) ? -1 : 1;
  tab.value = event.key === 'Home' ? 'all' : event.key === 'End' ? 'undeclared' : values[(values.indexOf(tab.value) + step + values.length) % values.length]!;
  document.getElementById(`online-tab-${tab.value}`)?.focus();
}
async function update() {
  if (busy.value || props.connectionDirty) return;
  busy.value = true; errorCode.value = ''; traceId.value = '';
  attemptSource.value = ''; upstreamStatus.value = undefined; upstreamReason.value = '';
  try {
    const result = await AISettingService.syncOnlineModels({ env: props.env, provider: props.provider, app: props.app, modality: props.modality, action: 'refresh' });
    if (!active) return;
    catalog.value = result; emit('saved', result);
  } catch (error: any) {
    if (!active) return;
    const data = error?.data || error?.response?._data || {};
    const code = data?.error_code || error?.errorCode || error?.message;
    errorCode.value = typeof code === 'string' && /^AI_[A-Z_]+$/.test(code) ? code : 'AI_MODEL_SYNC_FAILED';
    traceId.value = data?.request_id || data?.details?.trace_id || '';
    attemptSource.value = typeof data?.details?.source === 'string' ? data.details.source : '';
    upstreamStatus.value = typeof data?.details?.upstream_status === 'number' ? data.details.upstream_status : undefined;
    upstreamReason.value = typeof data?.details?.upstream_reason === 'string' ? data.details.upstream_reason : '';
  } finally { if (active) busy.value = false; }
}
onMounted(() => { void update(); });
onUnmounted(() => { active = false; });
</script>
