<script setup lang="ts">
import { computed, onBeforeUnmount, reactive, ref, watch } from 'vue';
import * as v from 'valibot';
import { useCustomerService, type CustomerAccount } from '~/composables/api/services/customerService';
import { useUserStore } from '~/stores/user';
import { customerAdminError, customerNameValid } from '~/utils/customer-admin';

const props = defineProps<{ open: boolean; allowed: boolean; account: CustomerAccount | null }>();
const emit = defineEmits<{ 'update:open': [boolean]; updated: [CustomerAccount] }>();
const { t } = useI18n(), api = useCustomerService(), user = useUserStore();
const form = reactive({ display_name: '', nickname: '', primary_email: '', primary_phone: '' });
const expected = ref(''), saving = ref(false), error = ref(''), conflict = ref(false);
const schema = computed(() => v.object({
  display_name: v.pipe(v.string(), v.check(value => customerNameValid(value, true), t('customerMaintenance.nameInvalid'))),
  nickname: v.pipe(v.string(), v.check(value => customerNameValid(value), t('customerMaintenance.nameInvalid'))),
  primary_email: v.pipe(v.string(), v.trim(), v.union([v.literal(''), v.pipe(v.string(), v.email(), v.maxLength(255))])),
  primary_phone: v.pipe(v.string(), v.trim(), v.maxLength(32), v.regex(/^[0-9+() -]*$/), v.check(value => !value || /[0-9]/.test(value))),
}));
const values = () => Object.fromEntries(Object.entries(form).map(([key, value]) => [key, value.trim()])) as typeof form;
const initial = ref('');
const ready = computed(() => props.open && props.allowed && !!props.account && !!expected.value && JSON.stringify(values()) !== initial.value && v.safeParse(schema.value, values()).success);
let revision = 0, disposed = false, controller: AbortController | null = null;
function invalidate() { revision++; controller?.abort(); controller = null; saving.value = false; }
function fill(account: CustomerAccount) {
  Object.assign(form, { display_name: account.display_name || '', nickname: account.nickname || '', primary_email: account.primary_email || '', primary_phone: account.primary_phone || '' });
  expected.value = account.profile_version || ''; initial.value = JSON.stringify(values()); error.value = ''; conflict.value = false;
}
watch([() => props.open, () => props.account?.uuid, () => user.currentTenantUuid, () => props.allowed], (now, previous) => {
  invalidate(); error.value = ''; conflict.value = false;
  if (props.open && previous?.[0] && (previous[1] !== now[1] || previous[2] !== now[2])) { expected.value = ''; emit('update:open', false); return; }
  if (props.open && props.account && props.allowed) fill(props.account);
  else if (props.open) emit('update:open', false);
}, { immediate: true });
onBeforeUnmount(() => { disposed = true; invalidate(); });
async function submit() {
  if (saving.value || !ready.value || conflict.value) return;
  const stamp = revision, id = props.account!.uuid, tenant = user.currentTenantUuid;
  const live = () => !disposed && stamp === revision && props.open && props.allowed && props.account?.uuid === id && user.currentTenantUuid === tenant;
  controller = new AbortController(); saving.value = true; error.value = '';
  try {
    const result = await api.updateProfile(id, { ...values(), expected_profile_version: expected.value }, controller.signal);
    if (live()) { emit('updated', result); emit('update:open', false); }
  } catch (cause) {
    if (live()) { error.value = customerAdminError(cause, t('customerAdmin.editFailed')); conflict.value = error.value.includes('CUSTOMER_PROFILE_CHANGED'); }
  } finally { if (stamp === revision) { saving.value = false; controller = null; } }
}
async function reloadLatest() {
  if (saving.value || !props.allowed || !props.account) return;
  const stamp = revision, id = props.account.uuid, tenant = user.currentTenantUuid; saving.value = true;
  const live = () => !disposed && stamp === revision && props.open && props.allowed && props.account?.uuid === id && user.currentTenantUuid === tenant;
  try { const result = await api.get(id); if (live()) fill(result); }
  catch (cause) { if (live()) error.value = customerAdminError(cause, t('customerMaster.loadFailed')); }
  finally { if (stamp === revision) saving.value = false; }
}
</script>

<template>
  <UModal :open="open" :dismissible="!saving" :close="!saving" :title="t('customerAdmin.editProfile')" :description="t('customerAdmin.editHint')" :ui="{ content: 'max-w-2xl w-full' }" @update:open="emit('update:open', $event)">
    <template #body>
      <UForm :state="form" :schema="schema" data-testid="customer-edit-form" class="space-y-4" @submit="submit">
        <UFormField :label="t('customerMaster.displayName')" name="display_name" required><UInput v-model="form.display_name" class="w-full" :disabled="saving" data-testid="customer-edit-name" /></UFormField>
        <UFormField :label="t('customerAdmin.nickname')" name="nickname"><UInput v-model="form.nickname" class="w-full" :disabled="saving" data-testid="customer-edit-nickname" /></UFormField>
        <div class="grid gap-4 sm:grid-cols-2">
          <UFormField :label="t('customerMaster.email')" name="primary_email"><UInput v-model="form.primary_email" type="email" class="w-full" :disabled="saving" data-testid="customer-edit-email" /></UFormField>
          <UFormField :label="t('customerMaster.phone')" name="primary_phone"><UInput v-model="form.primary_phone" class="w-full" :disabled="saving" data-testid="customer-edit-phone" /></UFormField>
        </div>
        <UAlert v-if="!expected" color="warning" :title="t('customerAdmin.editReloadRequired')" />
        <UAlert v-if="error" color="error" :title="error" />
        <div class="flex justify-end gap-2">
          <UButton v-if="conflict || !expected" data-testid="customer-edit-reload" color="neutral" variant="outline" :loading="saving" @click="reloadLatest">{{ t('customerAdmin.reloadProfile') }}</UButton>
          <UButton color="neutral" variant="ghost" :disabled="saving" @click="emit('update:open', false)">{{ t('common.cancel') }}</UButton>
          <UButton type="submit" data-testid="customer-edit-save" :loading="saving" :disabled="!ready || conflict">{{ t('common.save') }}</UButton>
        </div>
      </UForm>
    </template>
  </UModal>
</template>
