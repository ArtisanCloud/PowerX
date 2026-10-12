<script setup lang="ts">
import { computed, onBeforeUnmount, reactive, ref, watch } from 'vue';
import * as v from 'valibot';
import { useCustomerService } from '~/composables/api/services/customerService';
import { customerAdminError, customerNameValid } from '~/utils/customer-admin';

const props = defineProps<{ open: boolean; allowed: boolean }>();
const emit = defineEmits<{ 'update:open': [boolean]; created: [string] }>();
const { t } = useI18n();
const api = useCustomerService();
const saving = ref(false), error = ref('');
const empty = () => ({ type: 'person' as 'person' | 'company', display_name: '', primary_email: '', primary_phone: '', contact_name: '', contact_email: '', contact_phone: '' });
const form = reactive(empty());
const optionalEmail = v.union([v.literal(''), v.pipe(v.string(), v.email(), v.maxLength(255))]);
const phone = v.pipe(v.string(), v.maxLength(32), v.regex(/^[0-9+() -]*$/));
const schema = computed(() => v.pipe(v.object({ type: v.picklist(['person', 'company']), display_name: v.pipe(v.string(), v.check(value => customerNameValid(value, true), t('customerMaintenance.nameInvalid'))), primary_email: optionalEmail, primary_phone: phone, contact_name: v.pipe(v.string(), v.check(value => customerNameValid(value), t('customerMaintenance.nameInvalid'))), contact_email: optionalEmail, contact_phone: phone }), v.check(data => data.type !== 'company' || !!data.contact_name.trim(), t('customerAdmin.companyContactRequired'))));
const ready = computed(() => props.allowed && !!form.display_name.trim() && (form.type !== 'company' || !!form.contact_name.trim()));
let disposed = false;
watch(() => props.open, open => { if (open) { Object.assign(form, empty()); error.value = ''; } });
onBeforeUnmount(() => { disposed = true; });
async function submit() {
  if (saving.value || !ready.value) return;
  saving.value = true; error.value = '';
  try {
    const result = await api.create({ type: form.type, display_name: form.display_name.trim(), primary_email: form.primary_email.trim(), primary_phone: form.primary_phone.trim(), ...(form.type === 'company' ? { primary_contact: { display_name: form.contact_name.trim(), email: form.contact_email.trim(), phone: form.contact_phone.trim() } } : {}) });
    if (!disposed && props.open) { emit('created', result.uuid); emit('update:open', false); }
  } catch (e) { if (!disposed) error.value = customerAdminError(e, t('customerAdmin.createFailed')); }
  finally { saving.value = false; }
}
</script>

<template>
  <UModal :open="open" :dismissible="!saving" :close="!saving" :title="t('customerAdmin.create')" :description="t('customerAdmin.createHint')" :ui="{ content: 'max-w-2xl w-full' }" @update:open="emit('update:open', $event)">
    <template #body>
      <UForm :state="form" :schema="schema" class="space-y-4" data-testid="customer-create-form" @submit="submit">
        <UFormField :label="t('customerAdmin.type')" name="type"><USelect v-model="form.type" :items="[{ value: 'person', label: t('customerAdmin.person') }, { value: 'company', label: t('customerAdmin.company') }]" class="w-full" :disabled="saving" /></UFormField>
        <UFormField :label="t('customerMaster.displayName')" name="display_name" required><UInput v-model="form.display_name" class="w-full" :disabled="saving" /></UFormField>
        <div class="grid gap-4 sm:grid-cols-2">
          <UFormField :label="t('customerMaster.email')" name="primary_email"><UInput v-model="form.primary_email" type="email" class="w-full" :disabled="saving" /></UFormField>
          <UFormField :label="t('customerMaster.phone')" name="primary_phone"><UInput v-model="form.primary_phone" class="w-full" :disabled="saving" /></UFormField>
        </div>
        <fieldset v-if="form.type === 'company'" class="space-y-4 rounded-lg border border-gray-200 p-4 dark:border-gray-700">
          <legend class="px-2 text-sm font-medium">{{ t('customerAdmin.primaryContact') }}</legend>
          <UFormField :label="t('customerMaster.displayName')" name="contact_name" required><UInput v-model="form.contact_name" class="w-full" :disabled="saving" /></UFormField>
          <div class="grid gap-4 sm:grid-cols-2">
            <UFormField :label="t('customerMaster.email')" name="contact_email"><UInput v-model="form.contact_email" type="email" class="w-full" :disabled="saving" /></UFormField>
            <UFormField :label="t('customerMaster.phone')" name="contact_phone"><UInput v-model="form.contact_phone" class="w-full" :disabled="saving" /></UFormField>
          </div>
        </fieldset>
        <UAlert v-if="error" color="error" :title="error" :description="t('customerAdmin.createRetryHint')" />
        <div class="flex justify-end gap-2"><UButton color="neutral" variant="ghost" :disabled="saving" @click="emit('update:open', false)">{{ t('common.cancel') }}</UButton><UButton type="submit" :loading="saving" :disabled="!ready" data-testid="customer-create-submit">{{ t('customerAdmin.create') }}</UButton></div>
      </UForm>
    </template>
  </UModal>
</template>
