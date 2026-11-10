<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch, nextTick } from 'vue'
import type { FormError } from '@nuxt/ui'
import { useClientAppService, type ClientApplication, type ClientAppCatalog, type LegacyClientApplication } from '~/composables/api/services/clientAppService'
import { useUserContext } from '~/composables/api/services/meService'
import { useUserStore } from '~/stores/user'
import SecretInput from '~/components/SecretInput.vue'

definePageMeta({ title: 'clientApps', icon: 'i-heroicons-device-phone-mobile', order: 7 })
const { t } = useI18n()
const api = useClientAppService(), access = useUserContext(), user = useUserStore()
const identity = computed(() => `${user.user?.id || ''}:${user.currentTenantUuid || ''}`)
const tenantName = computed(() => user.currentTenant?.tenant_name || user.currentTenantUuid || '')
const apps = ref<ClientApplication[]>([]), catalog = ref<ClientAppCatalog | null>(null)
const loading = ref(false), error = ref(''), notice = ref(''), total = ref(0), page = ref(1)
const kind = ref(''), provider = ref('all'), search = ref('')
const canCreate = ref(false), canEdit = ref(false), canImport = ref(false), permissionsLoading = ref(true)
const open = ref(false), saving = ref(false), opening = ref(false), importing = ref(''), formError = ref('')
type AppStep = 'overview' | 'credentials' | 'plugins'
const stepOrder: AppStep[] = ['overview', 'credentials', 'plugins']
const fieldStep: Record<string, AppStep> = { name: 'overview', kind: 'overview', provider: 'overview', appID: 'credentials', secret: 'credentials', pluginIDs: 'plugins' }
const selected = ref<ClientApplication | null>(null), section = ref<AppStep>('overview')
const form = ref<{ loading: boolean; clear(): void; setErrors(errors: FormError[]): void } | null>(null)
const busy = computed(() => saving.value || !!form.value?.loading)
const stepIndex = computed(() => stepOrder.indexOf(section.value))
const nextStepLabel = computed(() => t('clientApps.nextStep', { step: sections.value[stepIndex.value + 1]?.label || '' }))
const draft = reactive({ name: '', kind: 'mini_program', status: 'disabled' as 'active' | 'disabled', provider: '', appID: '', secret: '', pluginIDs: [] as string[] })
const pages = computed(() => Math.max(1, Math.ceil(total.value / 20)))
const sections = computed(() => [
  { value: 'overview', label: t('clientApps.overview'), icon: 'i-heroicons-squares-2x2' },
  { value: 'credentials', label: t('clientApps.credentials'), icon: 'i-heroicons-key' },
  { value: 'plugins', label: t('clientApps.plugins'), icon: 'i-heroicons-puzzle-piece' },
])
const kindOptions = computed(() => (catalog.value?.kinds || []).map(k => ({ value: k.id, label: t(`clientApps.kinds.${k.id}`) + (k.supported ? '' : ` (${t('clientApps.comingSoon')})`), disabled: !k.supported })))
const providerOptions = computed(() => (catalog.value?.providers || []).map(p => ({ value: p.id, label: t(`clientApps.providers.${p.id}`) + (p.supported ? '' : ` (${t('clientApps.comingSoon')})`), disabled: !p.supported })))
const providerFilters = computed(() => [{ value: 'all', label: t('clientApps.allProviders') }, ...(catalog.value?.providers || []).filter(p => p.supported).map(p => ({ value: p.id, label: t(`clientApps.providers.${p.id}`) }))])
// Inline poppers stay inside the dialog focus scope and above subsequent fields.
const modalSelectUi = { content: 'z-[60]' }
const allowedToSave = computed(() => selected.value ? canEdit.value : canCreate.value)
let disposed = false, loadVersion = 0, scopeVersion = 0, loadController: AbortController | undefined, writeController: AbortController | undefined
function errorText(e: any) {
  const code = e?.data?.error_code || e?.data?.code || e?.response?._data?.error_code || e?.response?._data?.code || e?.code
  const keys: Record<string, string> = {
    CLIENT_APP_NOT_FOUND: 'notFound', CLIENT_APP_DUPLICATE: 'duplicate', CLIENT_APP_VERSION_CONFLICT: 'conflict',
    CLIENT_APP_PLUGIN_FORBIDDEN: 'pluginForbidden', CLIENT_APP_PROVIDER_UNSUPPORTED: 'unsupported',
    CLIENT_APP_STORAGE_UNAVAILABLE: 'storageUnavailable', CLIENT_APP_SECRET_UNAVAILABLE: 'secretUnavailable',
    CLIENT_APP_SECRET_REQUIRED: 'secretRequired', CLIENT_APP_IDENTITY_IMMUTABLE: 'identityImmutable',
    CLIENT_APP_INVALID_ARGUMENT: 'invalid', CLIENT_APP_SECRET_INVALID: 'invalid', CLIENT_APP_VERSION_REQUIRED: 'conflict',
    CUSTOMER_ADMIN_FORBIDDEN: 'forbidden', CUSTOMER_TENANT_REQUIRED: 'tenantRequired'
  }
  const status = e?.statusCode || e?.status || e?.response?.status
  return t(`clientApps.errors.${keys[code] || (status === 403 ? 'forbidden' : 'failed')}`)
}
async function load() {
  loadController?.abort()
  const controller = loadController = new AbortController(), stamp = ++loadVersion, scope = scopeVersion
  loading.value = true; error.value = ''
  try {
    const [result, meta] = await Promise.all([api.list({ page: page.value, kind: kind.value, provider: provider.value === 'all' ? '' : provider.value, q: search.value.trim() }, controller.signal), api.catalog(controller.signal)])
    if (!disposed && stamp === loadVersion && scope === scopeVersion) { apps.value = result.items; total.value = result.pagination.total; catalog.value = meta }
  } catch (e) { if (!disposed && stamp === loadVersion && scope === scopeVersion) { apps.value = []; catalog.value = null; error.value = errorText(e) } }
  finally { if (stamp === loadVersion) loading.value = false }
}
async function loadAccess() {
  const scope = scopeVersion
  permissionsLoading.value = true; canCreate.value = false; canEdit.value = false; canImport.value = false
  const result = await Promise.all(['customer.admin_customers_client_apps:create', 'customer.admin_customers_client_apps_app_uuid:update', 'customer.admin_customers_client_apps_import:create'].map(p => access.hasPermission(p).catch(() => false)))
  if (!disposed && scope === scopeVersion) { canCreate.value = result[0] === true; canEdit.value = result[1] === true; canImport.value = canCreate.value && result[2] === true; permissionsLoading.value = false }
}
function filter(nextKind = kind.value) { kind.value = nextKind; page.value = 1; void load() }
function close() { (document.activeElement as HTMLElement | null)?.blur(); open.value = false }
function clearDraft() { draft.secret = ''; draft.pluginIDs = []; selected.value = null; formError.value = '' }
function fillDraft(app: ClientApplication | null) {
  selected.value = app; section.value = 'overview'; formError.value = ''
  const binding = app?.connections[0]
  Object.assign(draft, { name: app?.name || '', kind: app?.kind || 'mini_program', status: app?.status || 'disabled', provider: binding?.provider || '', appID: binding?.app_id || '', secret: '', pluginIDs: [...(app?.plugin_ids || [])] })
  open.value = true
}
async function edit(app: ClientApplication) {
  const scope = scopeVersion
  opening.value = true; error.value = ''
  try { const fresh = await api.get(app.uuid); if (!disposed && scope === scopeVersion) fillDraft(fresh) }
  catch (e) { if (!disposed && scope === scopeVersion) error.value = errorText(e) }
  finally { if (scope === scopeVersion) opening.value = false }
}
function validationErrors(): FormError[] {
  const errors: FormError[] = []
  if (!draft.name.trim() || [...draft.name.trim()].length > 128) errors.push({ name: 'name', message: t('clientApps.errors.nameRequired') })
  if (!catalog.value?.providers.some(p => p.id === draft.provider && p.supported)) errors.push({ name: 'provider', message: t('clientApps.selectProvider') })
  if (!draft.appID.trim() || /[\s/\\]/.test(draft.appID) || draft.appID.length > 128) errors.push({ name: 'appID', message: t('clientApps.errors.appIDRequired') })
  if (draft.status === 'active' && !draft.secret.trim() && !selected.value?.connections[0]?.secret_configured) errors.push({ name: 'secret', message: t('clientApps.errors.secretRequired') })
  if (draft.secret && (draft.secret.trim().length > 64 || /[^!-~]/.test(draft.secret.trim()))) errors.push({ name: 'secret', message: t('clientApps.errors.invalid') })
  if (!draft.pluginIDs.length) errors.push({ name: 'pluginIDs', message: t('clientApps.errors.pluginsRequired') })
  return errors
}
// Only validate the visible step on Next/Enter; final save checks the entire draft.
function validate(): FormError[] {
  return allowedToSave.value ? validationErrors().filter(e => fieldStep[e.name || ''] === section.value) : []
}
function canVisitStep(index: number) {
  return !allowedToSave.value || index <= stepIndex.value || !validationErrors().some(e => stepOrder.indexOf(fieldStep[e.name || '']) < index)
}
function setStep(target: AppStep) {
  section.value = target; form.value?.clear()
  void nextTick(() => {
    if (open.value && section.value === target) document.getElementById(`app-panel-${target}`)?.focus({ preventScroll: true })
  })
}
function selectStep(index: number) { if (!busy.value && canVisitStep(index)) setStep(stepOrder[index]!) }
async function focusError(error?: FormError) {
  if (!error?.name) return
  await nextTick()
  const fields: Record<string, string> = { name: 'name', provider: 'provider', kind: 'kind', appID: 'id', secret: 'secret' }
  const selector = fields[error.name] ? `[data-testid="client-app-${fields[error.name]}"]` : '[data-testid="client-app-plugin-field"] button'
  document.querySelector<HTMLElement>(selector)?.focus()
}
function invalidForm() { void focusError(validate()[0]) }
async function submitStep(event: Event) {
  if (saving.value) return
  // Distinct submitters prevent a Next/Enter event from becoming a save if the
  // current step changes during async form validation.
  const action = ((event as SubmitEvent).submitter as HTMLElement | null)?.dataset.stepAction
  if (action === 'save') { if (section.value === 'plugins') await save(); return }
  if (section.value === 'plugins') return
  const errors = validate()
  if (errors.length) { form.value?.setErrors(errors); await focusError(errors[0]); return }
  setStep(stepOrder[stepIndex.value + 1]!)
}
async function save() {
  if (section.value !== 'plugins' || !allowedToSave.value || saving.value) return
  const errors = validationErrors()
  if (errors.length) {
    setStep(fieldStep[errors[0]!.name || ''] || 'overview')
    await nextTick()
    form.value?.setErrors(errors.filter(e => fieldStep[e.name || ''] === section.value))
    await focusError(errors[0]); return
  }
  const scope = scopeVersion, controller = writeController = new AbortController()
  saving.value = true; formError.value = ''; notice.value = ''
  const secret = draft.secret.trim()
  try {
    await api.save(selected.value?.uuid || '', { expected_version: selected.value?.version || 0, name: draft.name.trim(), kind: draft.kind, status: draft.status, plugin_ids: [...draft.pluginIDs], connections: [{ uuid: selected.value?.connections[0]?.uuid, provider: draft.provider, app_id: draft.appID.trim(), ...(secret ? { app_secret: secret } : {}) }] }, controller.signal)
    if (!disposed && scope === scopeVersion) { close(); notice.value = t('clientApps.saved'); await load() }
  } catch (e) { if (!disposed && scope === scopeVersion) formError.value = errorText(e) }
  finally { if (scope === scopeVersion) saving.value = false }
}
async function importLegacy(app: LegacyClientApplication) {
  if (!canImport.value || importing.value) return
  const scope = scopeVersion, controller = writeController = new AbortController()
  importing.value = app.app_id; error.value = ''; notice.value = ''
  try { await api.importLegacy(app, controller.signal); if (!disposed && scope === scopeVersion) { notice.value = t('clientApps.imported'); await load() } }
  catch (e) { if (!disposed && scope === scopeVersion) error.value = errorText(e) }
  finally { if (scope === scopeVersion) importing.value = '' }
}
watch(open, value => { if (!value) clearDraft() })
watch(identity, () => {
  scopeVersion++; loadVersion++; loadController?.abort(); writeController?.abort(); close(); clearDraft()
  apps.value = []; catalog.value = null; total.value = 0; page.value = 1; error.value = ''; notice.value = ''; saving.value = false; opening.value = false; importing.value = ''
  if (user.currentTenantUuid) void Promise.all([load(), loadAccess()])
}, { flush: 'sync' })
onMounted(() => { if (user.currentTenantUuid) void Promise.all([load(), loadAccess()]) })
onBeforeUnmount(() => { disposed = true; scopeVersion++; loadController?.abort(); writeController?.abort(); clearDraft() })
</script>

<template>
  <div class="space-y-5 p-4 md:p-6" data-testid="client-apps-page">
    <div class="flex flex-wrap items-start justify-between gap-4">
      <div><h1 class="text-2xl font-bold text-gray-900 dark:text-white">{{ t('clientApps.title') }}</h1><p class="mt-1 text-sm text-gray-600 dark:text-gray-300">{{ t('clientApps.description') }}</p><p class="mt-2 text-sm font-medium">{{ t('clientApps.tenant') }}: {{ tenantName }}</p></div>
      <div class="flex gap-2"><UButton color="neutral" variant="outline" icon="i-heroicons-arrow-path" :loading="loading" @click="load">{{ t('clientApps.refresh') }}</UButton><UButton icon="i-heroicons-plus" :disabled="permissionsLoading || !canCreate || !catalog || !!importing" data-testid="client-app-new" @click="fillDraft(null)">{{ t('clientApps.create') }}</UButton></div>
    </div>
    <UAlert v-if="error" color="error" :title="error" data-testid="client-apps-error" />
    <UAlert v-if="notice" color="success" :title="notice" />
    <UAlert v-if="!user.currentTenantUuid" color="warning" :title="t('clientApps.errors.tenantRequired')" />
    <div class="rounded-xl border border-gray-200 bg-white dark:border-white/10 dark:bg-[#0f192a]">
      <div class="flex gap-1 overflow-x-auto border-b border-gray-200 p-2 dark:border-white/10" role="tablist" :aria-label="t('clientApps.kind')">
        <button v-for="tab in [{ id: '', label: t('clientApps.allKinds') }, ...(catalog?.kinds || []).map(k => ({ id: k.id, label: t(`clientApps.kinds.${k.id}`) }))]" :key="tab.id" type="button" role="tab" :aria-selected="kind === tab.id" class="shrink-0 rounded-lg px-4 py-2 text-sm font-medium" :class="kind === tab.id ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-200' : 'text-gray-600 dark:text-gray-300'" @click="filter(tab.id)">{{ tab.label }}</button>
      </div>
      <form class="flex flex-wrap gap-2 p-4" @submit.prevent="filter()"><UInput v-model="search" class="min-w-40 flex-1" :placeholder="t('clientApps.search')" :aria-label="t('clientApps.search')" /><USelect v-model="provider" :items="providerFilters" class="w-44" :aria-label="t('clientApps.provider')" @update:model-value="filter()" /><UButton type="submit" color="neutral" variant="outline">{{ t('clientApps.searchAction') }}</UButton></form>
      <p v-if="loading" class="p-8 text-center text-gray-600 dark:text-gray-300" role="status">{{ t('clientApps.loading') }}</p>
      <div v-else-if="apps.length" class="overflow-x-auto">
        <table class="w-full text-left text-sm"><thead class="border-y border-gray-200 bg-gray-50 dark:border-white/10 dark:bg-white/5"><tr><th class="px-4 py-3">{{ t('clientApps.name') }}</th><th class="px-4 py-3">{{ t('clientApps.connection') }}</th><th class="px-4 py-3">{{ t('clientApps.status') }}</th><th class="px-4 py-3">{{ t('clientApps.plugins') }}</th><th class="px-4 py-3"><span class="sr-only">{{ t('clientApps.manage') }}</span></th></tr></thead>
          <tbody><tr v-for="app in apps" :key="app.uuid" class="border-b border-gray-100 last:border-0 dark:border-white/10"><td class="px-4 py-4"><p class="font-medium">{{ app.name }}</p><p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t(`clientApps.kinds.${app.kind}`) }}</p></td><td class="px-4 py-4"><div v-for="binding in app.connections" :key="binding.uuid"><p>{{ t(`clientApps.providers.${binding.provider}`) }}</p><p class="break-all font-mono text-xs">{{ binding.app_id }}</p><span class="text-xs text-gray-500 dark:text-gray-400">{{ t(binding.secret_configured ? 'clientApps.secretConfigured' : 'clientApps.secretMissing') }}</span></div></td><td class="px-4 py-4"><UBadge :color="app.status === 'active' ? 'success' : 'neutral'" variant="subtle">{{ t(`clientApps.statuses.${app.status}`) }}</UBadge></td><td class="px-4 py-4">{{ t('clientApps.pluginCount', { count: app.plugin_ids.length }) }}</td><td class="px-4 py-4"><UButton color="neutral" variant="ghost" :loading="opening" data-testid="client-app-edit" @click="edit(app)">{{ t('clientApps.manage') }}</UButton></td></tr></tbody>
        </table>
      </div>
      <div v-else class="px-6 py-12 text-center"><UIcon name="i-heroicons-device-phone-mobile" class="mx-auto size-9 text-primary-600 dark:text-primary-200" /><p class="mt-3 font-medium">{{ t('clientApps.empty') }}</p><p class="mt-1 text-sm text-gray-600 dark:text-gray-300">{{ t('clientApps.supportHint') }}</p></div>
      <div class="flex items-center justify-between gap-3 border-t border-gray-200 p-4 text-sm dark:border-white/10"><span>{{ t('clientApps.total', { count: total }) }}</span><div class="flex items-center gap-3"><UButton color="neutral" variant="outline" :disabled="page <= 1 || loading" @click="page--; load()">{{ t('clientApps.previous') }}</UButton><span>{{ page }} / {{ pages }}</span><UButton color="neutral" variant="outline" :disabled="page >= pages || loading" @click="page++; load()">{{ t('clientApps.next') }}</UButton></div></div>
    </div>
    <details v-if="catalog?.legacy_applications.length" class="rounded-xl border border-gray-200 p-4 dark:border-white/10"><summary class="cursor-pointer font-medium">{{ t('clientApps.legacyTitle') }} ({{ catalog.legacy_applications.length }})</summary><p class="my-3 text-sm text-gray-600 dark:text-gray-300">{{ t('clientApps.legacyHint') }}</p><div v-for="app in catalog.legacy_applications" :key="`${app.platform}:${app.app_id}`" class="flex flex-wrap items-center justify-between gap-2 border-t border-gray-100 py-3 dark:border-white/10"><div><p>{{ app.name || app.app_id }}</p><p class="font-mono text-xs">{{ app.app_id }}</p></div><UButton color="neutral" variant="outline" :loading="importing === app.app_id" :disabled="!canImport || !!importing || open" @click="importLegacy(app)">{{ t('clientApps.import') }}</UButton></div></details>
    <UModal v-model:open="open" :title="selected ? t('clientApps.manage') : t('clientApps.create')" :description="t('clientApps.scopeHint')" :dismissible="!saving" :close="!saving" :ui="{ content: 'max-w-4xl' }">
      <template #body>
        <UAlert v-if="formError" color="error" :title="formError" class="mb-4" data-testid="client-app-form-error" />
        <UAlert v-if="!allowedToSave" color="warning" :title="t('clientApps.readOnly')" class="mb-4" />
        <UForm ref="form" id="client-app-form" :state="draft" :validate="validate" class="grid gap-5 md:grid-cols-[11rem_minmax(0,1fr)]" @submit="submitStep" @error="invalidForm">
          <nav :aria-label="t('clientApps.steps')">
            <ol class="flex gap-1 overflow-x-auto md:flex-col">
              <li v-for="(item, index) in sections" :key="item.value" class="shrink-0">
                <button :id="`app-tab-${item.value}`" type="button" :aria-current="section === item.value ? 'step' : undefined" :disabled="busy || !canVisitStep(index)" :title="canVisitStep(index) ? item.label : t('clientApps.completePrevious')" :data-testid="`client-app-section-${item.value}`" class="flex w-full items-center gap-2 rounded-lg px-2 py-3 text-left text-sm font-medium md:px-3 disabled:cursor-not-allowed disabled:opacity-50" :class="section === item.value ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-200' : 'text-gray-600 hover:bg-gray-50 dark:text-gray-300 dark:hover:bg-white/5'" @click="selectStep(index)">
                  <span aria-hidden="true" class="flex size-6 shrink-0 items-center justify-center rounded-full border border-current text-xs"><UIcon v-if="index < stepIndex" name="i-heroicons-check" class="size-4" /><span v-else>{{ index + 1 }}</span></span>{{ item.label }}
                </button>
              </li>
            </ol>
          </nav>
          <div :id="`app-panel-${section}`" role="region" tabindex="-1" :aria-labelledby="`app-tab-${section}`" class="min-h-72 min-w-0 focus:outline-none">
            <div v-if="section === 'overview'" class="space-y-5">
              <UFormField :label="t('clientApps.name')" name="name" required><UInput v-model="draft.name" class="w-full" :maxlength="128" :disabled="saving || !allowedToSave" data-testid="client-app-name" /></UFormField>
              <div class="grid gap-5 sm:grid-cols-2">
                <UFormField :label="t('clientApps.kind')" name="kind" required><USelect v-model="draft.kind" :items="kindOptions" :portal="false" :ui="modalSelectUi" class="w-full" :disabled="!!selected || saving || !allowedToSave" data-testid="client-app-kind" /></UFormField>
                <UFormField :label="t('clientApps.provider')" name="provider" required><USelect v-model="draft.provider" :items="providerOptions" :placeholder="t('clientApps.selectProvider')" :portal="false" :ui="modalSelectUi" class="w-full" :disabled="!!selected || saving || !allowedToSave" data-testid="client-app-provider" /></UFormField>
              </div>
              <UFormField :label="t('clientApps.status')" name="status" :description="t('clientApps.statusHint')"><USelect v-model="draft.status" :items="[{ value: 'disabled', label: t('clientApps.statuses.disabled') }, { value: 'active', label: t('clientApps.statuses.active') }]" :portal="false" :ui="modalSelectUi" class="w-full" :disabled="saving || !allowedToSave" data-testid="client-app-status" /></UFormField>
            </div>
            <div v-if="section === 'credentials'" class="space-y-5">
              <div v-if="draft.provider" class="flex flex-wrap gap-2"><UBadge color="neutral" variant="subtle">{{ t(`clientApps.kinds.${draft.kind}`) }}</UBadge><UBadge color="primary" variant="subtle">{{ t(`clientApps.providers.${draft.provider}`) }}</UBadge></div>
              <UButton v-else color="neutral" variant="outline" @click="selectStep(0)">{{ t('clientApps.selectProvider') }}</UButton>
              <UFormField label="AppID" name="appID" required :description="selected ? t('clientApps.identityHint') : ''"><UInput v-model="draft.appID" class="w-full font-mono" :disabled="!!selected || saving || !allowedToSave || !draft.provider" :maxlength="128" autocomplete="off" data-testid="client-app-id" /></UFormField>
              <UFormField label="AppSecret" name="secret" :required="draft.status === 'active' && !selected?.connections[0]?.secret_configured" :description="selected?.connections[0]?.secret_configured ? t('clientApps.rotateHint') : t('clientApps.secretHint')"><SecretInput v-model="draft.secret" :disabled="saving || !allowedToSave || !draft.provider" :maxlength="64" autocomplete="new-password" data-testid="client-app-secret" /></UFormField>
              <p class="text-sm text-gray-600 dark:text-gray-300">{{ t('clientApps.supportHint') }}</p>
            </div>
            <div v-if="section === 'plugins'" class="space-y-4">
              <p class="text-sm text-gray-600 dark:text-gray-300">{{ t('clientApps.pluginHint') }}</p>
              <UFormField :label="t('clientApps.plugins')" name="pluginIDs" required data-testid="client-app-plugin-field"><UCheckboxGroup v-model="draft.pluginIDs" :items="[...new Set([...(catalog?.plugin_ids || []), ...draft.pluginIDs])].map(id => ({ value: id, label: id }))" :disabled="saving || !allowedToSave" /></UFormField>
              <UAlert v-if="!catalog?.plugin_ids.length" color="warning" :title="t('clientApps.noPlugins')" />
            </div>
          </div>
        </UForm>
      </template>
      <template #footer>
        <div class="flex w-full flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <p class="text-sm text-gray-600 dark:text-gray-300" aria-live="polite" data-testid="client-app-step-progress">{{ t('clientApps.stepProgress', { current: stepIndex + 1, total: stepOrder.length }) }}</p>
          <div class="flex flex-wrap justify-end gap-2">
            <UButton color="neutral" variant="outline" :disabled="busy" @click="close">{{ t('clientApps.cancel') }}</UButton>
            <UButton v-if="stepIndex > 0" color="neutral" variant="outline" type="button" :disabled="busy" data-testid="client-app-previous-step" @click="selectStep(stepIndex - 1)">{{ t('clientApps.previousStep') }}</UButton>
            <UButton v-if="section !== 'plugins'" key="next-step" type="submit" form="client-app-form" :disabled="busy" data-step-action="next" data-testid="client-app-next-step">{{ nextStepLabel }}</UButton>
            <UButton v-else-if="allowedToSave" key="save-app" type="submit" form="client-app-form" :loading="saving" :disabled="busy || !catalog?.plugin_ids.length" data-step-action="save" data-testid="client-app-save">{{ t('clientApps.save') }}</UButton>
          </div>
        </div>
      </template>
    </UModal>
  </div>
</template>
