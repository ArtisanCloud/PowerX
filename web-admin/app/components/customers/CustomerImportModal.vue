<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue';
import { useCustomerService, type CustomerImportResult } from '~/composables/api/services/customerService';
import { CUSTOMER_IMPORT_TEMPLATE, customerAdminError, customerImportResultCSV, downloadCustomerCSV } from '~/utils/customer-admin';

const props = defineProps<{ open: boolean; allowed: boolean; initialId?: string }>();
const emit = defineEmits<{ 'update:open': [boolean]; completed: []; previewed: [string] }>();
const { t } = useI18n(); const api = useCustomerService();
const file = ref<File>(), result = ref<CustomerImportResult>(), error = ref(''), busy = ref(false), confirmed = ref(false), lookupID = ref('');
const canCommit = computed(() => props.allowed && result.value?.status === 'preview' && result.value.ready > 0 && confirmed.value);
let revision = 0, disposed = false;
watch(() => props.open, async open => { revision++; if (open) { error.value = ''; confirmed.value = false; file.value = undefined; result.value = undefined; lookupID.value = props.initialId || ''; if (lookupID.value && props.allowed) await readResult(); } else { file.value = undefined; result.value = undefined; } });
onBeforeUnmount(() => { disposed = true; revision++; });
const current = (id: number) => !disposed && props.open && id === revision;
function selectFile(event: Event) { revision++; file.value = (event.target as HTMLInputElement).files?.[0]; result.value = undefined; confirmed.value = false; error.value = ''; }
async function preview() {
  if (busy.value || !props.allowed || !file.value) return;
  const stamp = revision, source = file.value; busy.value = true; error.value = '';
  try {
    if (!source.name.toLowerCase().endsWith('.csv') || source.size > 1048576) throw new Error(t('customerAdmin.fileInvalid'));
    let csv: string;
    try { csv = new TextDecoder('utf-8', { fatal: true }).decode(await source.arrayBuffer()); } catch { throw new Error(t('customerAdmin.encodingInvalid')); }
    const preview = await api.previewImport(source.name, csv);
    if (current(stamp)) { result.value = preview; lookupID.value = preview.uuid; emit('previewed', preview.uuid); }
  } catch (e) { if (current(stamp)) error.value = customerAdminError(e, t('customerAdmin.importFailed')); }
  finally { busy.value = false; }
}
async function readResult() {
  if (busy.value || !props.allowed || !lookupID.value.trim()) return;
  const stamp = revision; busy.value = true; error.value = ''; confirmed.value = false;
  try { const value = await api.getImport(lookupID.value.trim()); if (current(stamp)) { result.value = value; emit('previewed', value.uuid); } }
  catch (e) { if (current(stamp)) error.value = customerAdminError(e, t('customerAdmin.importFailed')); }
  finally { busy.value = false; }
}
async function commit() {
  if (busy.value || !canCommit.value || !result.value) return;
  const stamp = revision, snapshot = result.value; busy.value = true; error.value = '';
  try { const value = await api.commitImport(snapshot.uuid, snapshot.checksum); if (current(stamp)) { result.value = value; confirmed.value = false; emit('completed'); } }
  catch (e) { if (current(stamp)) error.value = customerAdminError(e, t('customerAdmin.importRetryHint')); }
  finally { busy.value = false; }
}
</script>

<template>
  <UModal :open="open" :dismissible="!busy" :close="!busy" :title="t('customerAdmin.import')" :description="t('customerAdmin.importHint')" :ui="{ content: 'max-w-6xl w-[95vw]' }" @update:open="emit('update:open', $event)">
    <template #body>
      <div class="space-y-4" data-testid="customer-import-dialog">
        <UButton color="neutral" variant="outline" @click="downloadCustomerCSV('customers-template.csv', CUSTOMER_IMPORT_TEMPLATE)">{{ t('customerAdmin.template') }}</UButton>
        <UFormField :label="t('customerAdmin.csvFile')" :help="t('customerAdmin.fileHelp')"><input type="file" accept=".csv,text/csv" :disabled="busy || !allowed" class="block w-full text-sm" data-testid="customer-import-file" @change="selectFile"></UFormField>
        <UButton :disabled="!file || !allowed || busy" :loading="busy" data-testid="customer-import-preview" @click="preview">{{ t('customerAdmin.preview') }}</UButton>
        <div class="flex flex-wrap gap-2"><UInput v-model="lookupID" :placeholder="t('customerAdmin.lookupPlaceholder')" :disabled="busy" class="min-w-64 flex-1" data-testid="customer-import-lookup-id" /><UButton color="neutral" variant="outline" :disabled="!lookupID.trim() || !allowed || busy" data-testid="customer-import-read" @click="readResult">{{ t('customerAdmin.readResult') }}</UButton></div>
        <UAlert v-if="error" color="error" :title="error" :description="result ? t('customerAdmin.importRetryHint') : undefined" />
        <template v-if="result">
          <UAlert :color="result.status === 'completed' ? (result.failed ? 'warning' : 'success') : 'info'" :title="t(result.status === 'completed' ? 'customerAdmin.completed' : 'customerAdmin.previewReady')" :description="t('customerAdmin.summary', { total: result.total, ready: result.ready, created: result.created, skipped: result.skipped, failed: result.failed })" />
          <p class="break-all text-xs text-gray-600 dark:text-gray-300">{{ t('customerAdmin.importID') }}: {{ result.uuid }} · {{ t('customerAdmin.expires') }}: {{ result.expires_at }}<span v-if="result.trace_id"> · Trace: {{ result.trace_id }}</span></p>
          <div class="max-h-96 overflow-auto rounded-lg border border-gray-200 dark:border-gray-700">
            <table class="min-w-full text-left text-sm"><thead class="sticky top-0 bg-gray-50 text-gray-700 dark:bg-slate-900 dark:text-gray-200"><tr><th class="p-2">{{ t('customerAdmin.row') }}</th><th class="p-2">{{ t('customerMaster.displayName') }}</th><th class="p-2">{{ t('customerAdmin.type') }}</th><th class="p-2">{{ t('customerMaster.email') }} / {{ t('customerMaster.phone') }}</th><th class="p-2">{{ t('customerAdmin.primaryContact') }}</th><th class="p-2">{{ t('common.status') }}</th><th class="p-2">{{ t('customerAdmin.rowResult') }}</th></tr></thead><tbody><tr v-for="row in result.rows" :key="row.row" class="border-t border-gray-200 dark:border-gray-700"><td class="p-2">{{ row.row }}</td><td class="p-2">{{ row.input.display_name }}</td><td class="p-2">{{ t(`customerAdmin.${row.input.type === 'company' ? 'company' : 'person'}`) }}</td><td class="break-all p-2">{{ row.input.primary_email }}<br>{{ row.input.primary_phone }}</td><td class="p-2">{{ row.input.primary_contact_name || (row.input.type === 'person' ? row.input.display_name : '') }}<br>{{ row.input.primary_contact_email }} {{ row.input.primary_contact_phone }}</td><td class="p-2">{{ t(`customerAdmin.rowStatus.${row.status}`) }}</td><td class="break-all p-2"><span v-if="row.error_code">{{ t(`customerAdmin.errors.${row.error_code}`) }} · {{ row.error_code }}</span><NuxtLink v-if="row.customer_uuid" :to="`/customers/${row.customer_uuid}`" class="text-primary-600 dark:text-primary-300">{{ row.customer_uuid }}</NuxtLink></td></tr></tbody></table>
          </div>
          <UButton color="neutral" variant="outline" @click="downloadCustomerCSV(`customers-result-${result.uuid}.csv`, customerImportResultCSV(result))">{{ t('customerAdmin.downloadResult') }}</UButton>
          <UCheckbox v-if="result.status === 'preview'" v-model="confirmed" :label="t('customerAdmin.confirmRows')" :disabled="busy || !allowed || !result.ready" data-testid="customer-import-confirm" />
          <UButton v-if="result.status === 'preview'" :loading="busy" :disabled="!canCommit" data-testid="customer-import-commit" @click="commit">{{ t('customerAdmin.commit') }}</UButton>
        </template>
      </div>
    </template>
  </UModal>
</template>
