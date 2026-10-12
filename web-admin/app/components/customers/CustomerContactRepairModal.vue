<script setup lang="ts">
import { computed, onBeforeUnmount, reactive, ref, watch } from 'vue'
import { useCustomerService, type CustomerAccount, type CustomerContact, type ContactRepairInput, type ContactRepairResult } from '~/composables/api/services/customerService'
import { customerAdminError } from '~/utils/customer-admin'
import { useUserStore } from '~/stores/user'
const props = defineProps<{ open: boolean; allowed: boolean; account: CustomerAccount; contacts: CustomerContact[]; initialId?: string }>()
const emit = defineEmits<{ 'update:open': [boolean]; previewed: [string]; completed: [] }>()
const { t } = useI18n(), api = useCustomerService()
const user = useUserStore()
const opened = computed({ get: () => props.open, set: value => emit('update:open', value) })
const form = reactive({ type: '' as '' | 'person' | 'company', contact: '', name: '', email: '', phone: '' })
const result = ref<ContactRepairResult | null>(null), confirmed = ref(false), busy = ref(false), error = ref('')
const activeContacts = computed(() => props.contacts.filter(c => c.status === 'active'))
const options = computed(() => [{ label: t('customerMaintenance.createNaturalPerson'), value: '' }, ...activeContacts.value.map(c => ({ label: c.display_name, value: c.uuid }))])
let revision = 0, disposed = false
async function execute(work: () => Promise<ContactRepairResult>, completed = false) {
  if (!props.allowed || busy.value) return
  const stamp = ++revision, tenant = user.currentTenantUuid, customer = props.account.uuid
  const live = () => !disposed && stamp === revision && props.open && user.currentTenantUuid === tenant && props.account.uuid === customer
  busy.value = true; error.value = ''
  try {
    const response = await work()
    if (!live()) return
    if (response.proposal.customer_uuid !== props.account.uuid) throw new Error(t('customerMaintenance.repairScopeMismatch'))
    result.value = response; confirmed.value = false
    emit('previewed', response.uuid)
    if (completed || response.status === 'completed') emit('completed')
  } catch (cause) { if (live()) error.value = customerAdminError(cause, t('customerMaster.saveFailed')) }
  finally { if (stamp === revision) busy.value = false }
}
async function preview() {
  if (!form.type || result.value) return
  const input: ContactRepairInput = { customer_uuid: props.account.uuid, type: form.type }
  if (form.contact) input.contact_uuid = form.contact
  else input.primary_contact = { display_name: form.name.trim(), email: form.email.trim(), phone: form.phone.trim() }
  await execute(() => api.previewContactRepair(input))
}
async function commit() {
  const snapshot = result.value
  if (!snapshot || !confirmed.value || snapshot.status !== 'preview') return
  await execute(() => api.commitContactRepair(snapshot.uuid, snapshot.checksum), true)
}
function resetPreview() { revision++; result.value = null; confirmed.value = false; error.value = ''; busy.value = false }
watch(() => [props.open, props.account.uuid, props.allowed] as const, () => {
  resetPreview()
  Object.assign(form, { type: props.account.type || '', contact: activeContacts.value.length === 1 ? activeContacts.value[0]!.uuid : '', name: props.account.type === 'person' ? props.account.display_name || '' : '', email: props.account.type === 'person' ? props.account.primary_email || '' : '', phone: props.account.type === 'person' ? props.account.primary_phone || '' : '' })
  if (props.open && props.allowed && props.initialId) void execute(() => api.getContactRepair(props.initialId!))
}, { immediate: true, flush: 'sync' })
onBeforeUnmount(() => { disposed = true; revision++ })
</script>

<template>
  <UModal v-model:open="opened" :title="t('customerMaintenance.repair')" :dismissible="!busy" :close="!busy" :ui="{ content: 'w-full max-w-2xl' }">
    <template #body><div class="space-y-4">
      <UAlert color="info" :description="t('customerMaintenance.repairNotice')" />
      <UAlert v-if="error" color="error" :title="error" />
      <div v-if="!result" class="space-y-3">
        <UFormField :label="t('customerAdmin.type')" required><USelect v-model="form.type" :disabled="Boolean(account.type) || busy" :items="[{ label: t('customerAdmin.person'), value: 'person' }, { label: t('customerAdmin.company'), value: 'company' }]" class="w-full" /></UFormField>
        <UFormField :label="t('customerMaintenance.targetContact')"><USelectMenu v-model="form.contact" :items="options" value-key="value" label-key="label" :portal="false" :disabled="busy" class="w-full" /></UFormField>
        <fieldset v-if="!form.contact" :disabled="busy" class="space-y-3">
          <UFormField :label="t('customerMaster.displayName')" required><UInput v-model="form.name" class="w-full" /></UFormField>
          <UFormField :label="t('customerMaster.email')"><UInput v-model="form.email" type="email" class="w-full" /></UFormField>
          <UFormField :label="t('customerMaster.phone')"><UInput v-model="form.phone" class="w-full" /></UFormField>
        </fieldset>
        <UButton data-testid="repair-preview" :loading="busy" :disabled="!allowed || !form.type || (!form.contact && !form.name.trim())" @click="preview">{{ t('customerAdmin.preview') }}</UButton>
      </div>
      <div v-else class="space-y-3">
        <dl class="space-y-2 break-all text-sm"><dt>{{ t('customerMaintenance.repairID') }}</dt><dd class="font-mono">{{ result.uuid }}</dd><dt>{{ t('customerMaintenance.proposal') }}</dt><dd>{{ t('customerMaintenance.' + result.proposal.action) }} · {{ result.proposal.type }} · {{ result.proposal.primary_contact?.display_name || contacts.find(c => c.uuid === result?.proposal.contact_uuid)?.display_name || result.proposal.contact_uuid }}</dd><dt>{{ t('common.status') }}</dt><dd>{{ result.status }}</dd><dt>{{ t('customerMaintenance.checksum') }}</dt><dd class="font-mono">{{ result.checksum }}</dd><dt v-if="result.trace_id">Trace</dt><dd v-if="result.trace_id">{{ result.trace_id }}</dd></dl>
        <UAlert v-if="result.status === 'completed'" color="success" :title="t('customerMaintenance.completed')" :description="result.primary_contact_uuid" />
        <template v-else><UCheckbox v-model="confirmed" data-testid="repair-confirm" :disabled="busy || !allowed" :label="t('customerMaintenance.confirmRepair')" /><div class="flex justify-end gap-2"><UButton variant="ghost" :disabled="busy" @click="resetPreview">{{ t('customerMaintenance.newPreview') }}</UButton><UButton data-testid="repair-commit" :loading="busy" :disabled="!allowed || !confirmed" @click="commit">{{ t('common.confirm') }}</UButton></div></template>
      </div>
    </div></template>
  </UModal>
</template>
