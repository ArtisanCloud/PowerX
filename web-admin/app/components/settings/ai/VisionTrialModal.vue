<template>
  <UModal :open="open" :title="$t('settings.ai.visionTrial.title')" :description="`${context.provider} / ${context.model} · ${context.env}`"
    :ui="{ content: 'w-[calc(100vw-2rem)] max-w-4xl max-h-[calc(100dvh-2rem)]' }" @update:open="close">
    <template #body>
      <div class="space-y-3 min-w-0">
        <p class="text-sm text-muted">{{ $t('settings.ai.visionTrial.hint') }}</p>
        <p v-if="blockedReason" role="alert" class="rounded-lg bg-warning/10 p-3 text-sm text-warning">{{ blockedReason }}</p>
        <div v-if="errorKey" role="alert" class="rounded-lg border border-error/40 bg-error/10 p-3 text-sm space-y-2" data-testid="vision-trial-error">
          <p>{{ $t(errorKey) }}</p>
          <details v-if="errorDetails"><summary class="cursor-pointer">{{ $t('settings.ai.connectionTest.details') }}</summary><pre class="mt-2 text-xs whitespace-pre-wrap break-all">{{ errorDetails }}</pre></details>
        </div>
        <div v-if="busy" role="status" class="text-sm text-primary">{{ $t('settings.ai.visionTrial.running') }}</div>
        <div class="flex flex-col sm:flex-row gap-4 min-w-0">
          <div role="tablist" :aria-label="$t('settings.ai.visionTrial.sections')" class="flex sm:flex-col gap-2 sm:w-32 shrink-0">
            <button v-for="item in tabs" :id="`vision-trial-tab-${item.value}`" :key="item.value" type="button" role="tab" :aria-selected="tab === item.value"
              :aria-controls="`vision-trial-panel-${item.value}`" :tabindex="tab === item.value ? 0 : -1" :class="['rounded-lg px-3 py-2 text-sm text-left', tab === item.value ? 'bg-primary/15 text-primary' : 'hover:bg-elevated']"
              @click="tab = item.value" @keydown="switchTab">{{ item.label }}</button>
          </div>
          <section :id="`vision-trial-panel-${tab}`" role="tabpanel" :aria-labelledby="`vision-trial-tab-${tab}`" class="flex-1 min-w-0">
            <UForm v-if="tab === 'input'" :state="form" class="space-y-4" @submit="run">
              <fieldset :disabled="busy || reading" class="space-y-4 min-w-0">
                <URadioGroup v-model="mode" :items="[{ label: $t('settings.ai.visionTrial.upload'), value: 'file' }, { label: $t('settings.ai.visionTrial.url'), value: 'url' }]" orientation="horizontal" />
                <UFormField v-if="mode === 'file'" :label="$t('settings.ai.visionTrial.image')" :help="$t('settings.ai.visionTrial.fileHint')">
                  <input :key="fileVersion" type="file" accept="image/png,image/jpeg,image/webp" class="block w-full min-w-0 text-sm" data-testid="vision-trial-file" @change="chooseFile" />
                  <p v-if="fileName" class="mt-2 text-xs text-muted break-all">{{ fileName }}</p>
                </UFormField>
                <UFormField v-else :label="$t('settings.ai.visionTrial.url')">
                  <div class="flex flex-wrap gap-2"><UInput v-model="form.url" type="url" placeholder="https://…" class="flex-1 min-w-0" data-testid="vision-trial-url" />
                    <UButton type="button" variant="soft" :disabled="!validURL" @click="urlPreview = form.url.trim()">{{ $t('settings.ai.visionTrial.preview') }}</UButton></div>
                </UFormField>
                <div v-if="preview" class="rounded-lg border border-default p-2"><img :src="preview" :alt="$t('settings.ai.visionTrial.preview')" referrerpolicy="no-referrer" class="max-h-44 max-w-full mx-auto object-contain" data-testid="vision-trial-preview" /></div>
                <UFormField :label="$t('settings.ai.visionTrial.prompt')" required>
                  <UTextarea v-model="form.prompt" :rows="3" :maxlength="2000" class="w-full" data-testid="vision-trial-prompt" />
                </UFormField>
              </fieldset>
            </UForm>
            <div v-else class="space-y-3" data-testid="vision-trial-result">
              <template v-if="result">
                <div class="text-xs text-muted break-all">{{ result.provider }} / {{ result.model }} · {{ $t('settings.ai.visionTrial.elapsed', { seconds: (result.elapsed_ms / 1000).toFixed(2) }) }}</div>
                <pre class="max-h-80 overflow-y-auto rounded-lg border border-default p-3 text-sm whitespace-pre-wrap break-words" data-testid="vision-trial-analysis">{{ result.text }}</pre>
                <details class="text-xs text-muted"><summary class="cursor-pointer">{{ $t('settings.ai.connectionTest.details') }}</summary><pre class="mt-2 whitespace-pre-wrap break-all">{{ JSON.stringify({ trace_id: result.trace_id, env: result.env, usage: result.usage }, null, 2) }}</pre></details>
              </template>
              <p v-else class="rounded-lg border border-default p-4 text-sm text-muted">{{ $t(busy ? 'settings.ai.visionTrial.running' : 'settings.ai.visionTrial.empty') }}</p>
            </div>
          </section>
        </div>
      </div>
    </template>
    <template #footer>
      <div class="flex w-full flex-wrap justify-end gap-2">
        <UButton v-if="busy" type="button" color="neutral" variant="soft" @click="stop">{{ $t('settings.ai.visionTrial.stop') }}</UButton>
        <UButton type="button" color="neutral" variant="soft" @click="close">{{ $t('settings.ai.onlineModels.close') }}</UButton>
        <UButton type="button" icon="i-heroicons-eye" :loading="busy" :disabled="!canRun || busy || reading" data-testid="vision-trial-run" @click="run">{{ $t('settings.ai.visionTrial.run') }}</UButton>
      </div>
    </template>
  </UModal>
</template>

<script setup lang="ts">
import { computed, ref, reactive, watch, onMounted, onUnmounted } from 'vue';
import { AISettingService, type VisionTrialContext, type VisionTrialImage, type VisionTrialResult } from '~/composables/api/services/aiSettingService';
import { AUTH_SESSION_EVENT, AUTH_SESSION_KEY, readAuthSession } from '~/utils/auth-session';

const props = defineProps<{ open: boolean; context: VisionTrialContext; scope: string; blockedReason?: string }>();
const emit = defineEmits<{ (e: 'update:open', value: boolean): void }>();
const { t } = useI18n();
const mode = ref('file'), tab = ref<'input' | 'result'>('input');
const tabs = computed(() => [{ value: 'input' as const, label: t('settings.ai.visionTrial.inputTab') }, { value: 'result' as const, label: t('settings.ai.visionTrial.resultTab') }]);
const form = reactive({ url: '', prompt: t('settings.ai.visionTrial.defaultPrompt') });
const fileData = ref(''), fileName = ref(''), urlPreview = ref(''), fileVersion = ref(0), reading = ref(false);
const result = ref<VisionTrialResult | null>(null), errorKey = ref(''), errorDetails = ref(''), busy = ref(false);
let controller: AbortController | undefined, reader: FileReader | undefined, revision = 0;
const validURL = computed(() => {
  try { const u = new URL(form.url.trim()); return ['http:', 'https:'].includes(u.protocol) && !u.username && !u.password && form.url.length <= 4096; } catch { return false; }
});
const preview = computed(() => mode.value === 'file' ? fileData.value : urlPreview.value);
const canRun = computed(() => !props.blockedReason && !!form.prompt.trim() && (mode.value === 'file' ? !!fileData.value : validURL.value));
function stop() { revision++; controller?.abort(); controller = undefined; busy.value = false; }
function close() { stop(); emit('update:open', false); }
function resetScope() {
  close(); const previous = reader; reader = undefined; previous?.abort(); reading.value = false; fileData.value = ''; fileName.value = ''; fileVersion.value++;
  form.url = ''; form.prompt = t('settings.ai.visionTrial.defaultPrompt'); urlPreview.value = ''; result.value = null; errorKey.value = ''; errorDetails.value = ''; tab.value = 'input';
}
const identity = () => { const session = readAuthSession(); return `${session?.sessionId || ''}/${session?.tenantUuid || ''}/${session?.status || ''}`; };
let lastIdentity = '';
function sessionChanged(event: Event) {
  if (event instanceof StorageEvent && event.key !== AUTH_SESSION_KEY) return;
  const next = identity(); if (next !== lastIdentity) { lastIdentity = next; resetScope(); }
}
function chooseFile(event: Event) {
  reader?.abort(); reading.value = false; fileData.value = ''; fileName.value = ''; errorKey.value = ''; errorDetails.value = '';
  const file = (event.target as HTMLInputElement).files?.[0]; if (!file) return;
  if (!['image/png', 'image/jpeg', 'image/webp'].includes(file.type)) { errorKey.value = 'settings.ai.visionTrial.fileTypeError'; return; }
  if (!file.size || file.size > 5 * 1024 * 1024) { errorKey.value = 'settings.ai.visionTrial.fileSizeError'; return; }
  const current = new FileReader(); reader = current; reading.value = true;
  current.onload = () => { if (reader !== current) return; reading.value = false; fileData.value = String(current.result || ''); fileName.value = file.name; };
  current.onerror = () => { if (reader === current) { reading.value = false; errorKey.value = 'settings.ai.visionTrial.fileReadError'; } };
  current.readAsDataURL(file);
}
async function run() {
  if (!canRun.value || busy.value || reading.value) return;
  busy.value = true; errorKey.value = ''; errorDetails.value = ''; result.value = null;
  const current = ++revision; controller = new AbortController();
  const image: VisionTrialImage = mode.value === 'file' ? { type: 'image_base64', content: fileData.value } : { type: 'image_url', url: form.url.trim() };
  try {
    const value = await AISettingService.trialVision(props.context, image, form.prompt.trim(), controller.signal);
    if (current !== revision) return;
    result.value = value; tab.value = 'result';
  } catch (error: any) {
    if (current !== revision) return;
    const data = error?.data || error?.response?._data || {};
    let code = data.error_code || error?.errorCode || error?.code;
    if (props.context.connection && ['AI_VISION_TRIAL_SAVED_CONNECTION_REQUIRED', 'AI_VISION_TRIAL_REQUEST_INVALID', 'AI_MODEL_NOT_CONFIGURED'].includes(code)) code = 'AI_VISION_TRIAL_NOT_SUPPORTED';
    const known = new Set(['AI_VISION_TRIAL_NOT_SUPPORTED', 'AI_CONNECTION_CONFIG_INVALID', 'AI_FORBIDDEN', 'AI_UNAUTHORIZED', 'AI_MODEL_NOT_CONFIGURED', 'AI_PROVIDER_REJECTED', 'AI_PROVIDER_TIMEOUT', 'AI_PARAMETER_INVALID', 'AI_INPUT_INVALID', 'AI_OUTPUT_INVALID', 'AI_MODEL_CAPABILITY_UNSUPPORTED', 'AI_MODEL_NOT_IN_ONLINE_CATALOG']);
    errorKey.value = `settings.ai.visionTrial.errors.${known.has(code) ? code : 'generic'}`;
    const details = data.details || error?.details || {};
    errorDetails.value = JSON.stringify({ error_code: typeof code === 'string' ? code : undefined, trace_id: data.request_id || details.trace_id, upstream_status: details.upstream_status, provider_trace_id: details.provider_trace_id, parameter: details.parameter, reason: details.reason }, null, 2);
  } finally { if (current === revision) { busy.value = false; controller = undefined; } }
}
function switchTab(event: KeyboardEvent) {
  if (!['ArrowDown', 'ArrowUp', 'ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return;
  event.preventDefault(); tab.value = event.key === 'Home' ? 'input' : event.key === 'End' ? 'result' : tab.value === 'input' ? 'result' : 'input';
  document.getElementById(`vision-trial-tab-${tab.value}`)?.focus();
}
watch(() => props.open, value => { if (!value) stop(); });
watch(() => props.scope, resetScope);
watch(() => props.context, () => { stop(); result.value = null; errorKey.value = ''; errorDetails.value = ''; tab.value = 'input'; });
watch(() => form.url, () => { urlPreview.value = ''; });
onMounted(() => { lastIdentity = identity(); window.addEventListener(AUTH_SESSION_EVENT, sessionChanged); window.addEventListener('storage', sessionChanged); });
onUnmounted(() => { stop(); const previous = reader; reader = undefined; previous?.abort(); window.removeEventListener(AUTH_SESSION_EVENT, sessionChanged); window.removeEventListener('storage', sessionChanged); });
</script>
