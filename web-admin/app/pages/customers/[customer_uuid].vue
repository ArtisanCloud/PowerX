<script setup lang="ts">
import { computed, onBeforeUnmount, reactive, ref, watch } from 'vue'
import { useCustomerService, type CustomerAccount, type CustomerAuthIdentity, type CustomerContact, type CustomerLoginEvent, type CustomerMembership, type MiniAppEntry, type ContactIdentity } from '~/composables/api/services/customerService'
import { useUserContext } from '~/composables/api/services/meService'
import { useUserStore } from '~/stores/user'
import { useMetadataGovernanceService } from '~/composables/api/services/metadataGovernanceService'
import CustomerContactRepairModal from '~/components/customers/CustomerContactRepairModal.vue'
import CustomerEditModal from '~/components/customers/CustomerEditModal.vue'
import { customerAdminError, customerNameValid } from '~/utils/customer-admin'
import type { DictionaryItem } from '~/types/metadata-governance'
definePageMeta({ title: 'customers', icon: 'i-heroicons-users' })
const { t, locale } = useI18n()
const route = useRoute(), router = useRouter(), api = useCustomerService(), access = useUserContext(), user = useUserStore(), metadataApi = useMetadataGovernanceService()
const customerUUID = computed(() => String(route.params.customer_uuid || '').trim())
const tenantKey = computed(() => user.currentTenantUuid || '')
const scopeKey = computed(() => tenantKey.value + '/' + customerUUID.value)
const activeTab = ref('basic'), loading = ref(false), error = ref(''), account = ref<CustomerAccount | null>(null)
const identities = ref<CustomerAuthIdentity[]>([]), memberships = ref<CustomerMembership[]>([]), loginEvents = ref<CustomerLoginEvent[]>([]), entries = ref<MiniAppEntry[]>([]), contacts = ref<CustomerContact[]>([])
const contactTotal = ref(0)
const selectedContact = ref<CustomerContact | null>(null), contactIdentities = ref<ContactIdentity[]>([])
const contactModalOpen = ref(false), identityModalOpen = ref(false), primaryModalOpen = ref(false), repairOpen = ref(false)
const profileEditOpen = ref(false)
const saving = ref(false), actionError = ref(''), resolveResult = ref<any>(null)
const contactChannelItems = ref<DictionaryItem[]>([]), contactChannelsUnavailable = ref(false)
const contactForm = reactive({ display_name: '', given_name: '', family_name: '', email: '', phone: '', legal: false, tags: '' })
const identityForm = reactive({ channel_dictionary_item_uuid: '', external_subject: '' })
const primaryExpected = ref('')
const permissionCodes = {
  profileUpdate: 'customer.admin_customers_accounts_customer_uuid:update',
  read: 'customer.admin_customers_customer_uuid_contacts:read',
  create: 'customer.admin_customers_customer_uuid_contacts:create',
  update: 'customer.admin_customers_customer_uuid_contacts_contact_uuid:update',
  identityRead: 'customer.admin_customers_customer_uuid_contacts_contact_uuid_identities:read',
  bind: 'customer.admin_customers_customer_uuid_contacts_contact_uuid_identities:create',
  resolve: 'customer.admin_customers_customer_uuid_contactsresolve_identity:create',
  primary: 'customer.admin_customers_customer_uuid_contacts_contact_uuid_primary:create',
  repairPreview: 'customer.admin_customers_contact_repairs_preview:create',
  repairRead: 'customer.admin_customers_contact_repairs_repair_uuid:read',
  repairCommit: 'customer.admin_customers_contact_repairs_repair_uuid_commit:create',
  authRead: 'customer.admin_customers_accounts_customer_uuid_auth_identities:read',
  memberRead: 'customer.admin_customers_accounts_customer_uuid_tenant_memberships:read',
  eventsRead: 'customer.admin_customers_accounts_customer_uuid_login_events:read',
  entriesRead: 'customer.admin_customers_mini_app_entries:list'
}
const allowed = reactive(Object.fromEntries(Object.keys(permissionCodes).map(k => [k, false])) as Record<keyof typeof permissionCodes, boolean>)
const canRepair = computed(() => allowed.repairPreview && allowed.repairRead && allowed.repairCommit)
const needsRepair = computed(() => !account.value?.type || !account.value?.primary_contact_uuid || (allowed.read && (contacts.value.length >= contactTotal.value || contacts.value.some(c => c.uuid === account.value?.primary_contact_uuid)) && !contacts.value.some(c => c.uuid === account.value?.primary_contact_uuid && c.status === 'active')))
const repairID = computed(() => typeof route.query.repair_uuid === 'string' ? route.query.repair_uuid : '')
const tabs = computed(() => [
  { id: 'basic', label: t('customerMaster.basic') },
  ...(allowed.authRead ? [{ id: 'identities', label: t('customerMaster.authIdentities') }] : []),
  ...(allowed.memberRead ? [{ id: 'memberships', label: t('customerMaster.memberships') }] : []),
  ...(allowed.eventsRead ? [{ id: 'events', label: t('customerMaster.loginEvents') }] : []),
  ...(allowed.entriesRead ? [{ id: 'entries', label: t('customerMaster.miniAppEntries') }] : []),
  ...(allowed.read ? [{ id: 'contacts', label: t('customerMaster.contacts') }] : [])
])
const formatTime = (value?: string) => value ? new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value)) : '-'
const short = (value?: string) => value && value.length > 20 ? value.slice(0, 8) + '…' + value.slice(-4) : value || '-'
const roleLabel = (role: string) => role === 'primary' ? t('customerMaster.rolePrimary') : role === 'legal_representative' ? t('customerMaster.roleLegalRepresentative') : role
const channelOptions = computed(() => contactChannelItems.value.map(item => ({ label: item.display_name || item.code, value: item.uuid })))
const channelLabel = (id: string) => contactChannelItems.value.find(item => item.uuid === id)?.display_name || t('common.unknown')
const isPrimary = (contact: CustomerContact) => contact.uuid === account.value?.primary_contact_uuid
let scopeVersion = 0, loadVersion = 0, actionVersion = 0, disposed = false
const validScope = (stamp: number) => !disposed && stamp === scopeVersion
async function fetchChannels() {
  const namespaces = await metadataApi.listDictionaries({ module: 'corex.customer', q: 'corex.customer.contact_identity_channel', status: 'enabled', locale: String(locale.value), page_size: 20 })
  const ns = namespaces.items.find(item => item.namespace === 'corex.customer.contact_identity_channel')
  if (!ns) throw new Error('contact_channel_dictionary_missing')
  return (await metadataApi.listDictionaryItems(ns.uuid, { status: 'enabled', locale: String(locale.value), page_size: 100 })).items
}
async function load() {
  const stamp = scopeVersion, request = ++loadVersion, id = customerUUID.value
  if (!id || !tenantKey.value) return
  loading.value = true; error.value = ''
  try {
    const keys = Object.keys(permissionCodes) as (keyof typeof permissionCodes)[]
    const grants = await Promise.all(keys.map(key => access.hasPermission(permissionCodes[key]).catch(() => false)))
    if (!validScope(stamp) || request !== loadVersion) return
    keys.forEach((key, i) => { allowed[key] = grants[i] === true })
    const empty = Promise.resolve({ items: [], pagination: { total: 0 } })
    const channelRequest = allowed.identityRead || allowed.bind || allowed.resolve
    const [customer, auth, member, events, miniApp, contactPage, channels] = await Promise.all([
      api.get(id), allowed.authRead ? api.listAuthIdentities(id) : empty,
      allowed.memberRead ? api.listMemberships(id) : empty, allowed.eventsRead ? api.listLoginEvents(id) : empty,
      allowed.entriesRead ? api.listMiniAppEntries() : empty, allowed.read ? api.listContacts(id) : empty,
      channelRequest ? fetchChannels().then(items => ({ items, unavailable: false })).catch(() => ({ items: [], unavailable: true })) : Promise.resolve({ items: [], unavailable: false })
    ])
    if (!validScope(stamp) || request !== loadVersion) return
    account.value = customer; identities.value = auth.items; memberships.value = member.items; loginEvents.value = events.items
    entries.value = miniApp.items; contacts.value = contactPage.items; contactChannelItems.value = channels.items
    contactTotal.value = contactPage.pagination?.total ?? contactPage.items.length
    contactChannelsUnavailable.value = channels.unavailable
  } catch (cause) { if (validScope(stamp) && request === loadVersion) error.value = customerAdminError(cause, t('customerMaster.loadFailed')) }
  finally { if (validScope(stamp) && request === loadVersion) loading.value = false }
}
function resetContactForm(contact?: CustomerContact) {
  if (saving.value || (contact ? !allowed.update : !allowed.create)) return
  actionVersion++; selectedContact.value = contact || null; actionError.value = ''
  Object.assign(contactForm, { display_name: contact?.display_name || '', given_name: contact?.given_name || '', family_name: contact?.family_name || '', email: contact?.email || '', phone: contact?.phone || '', legal: Boolean(contact?.roles?.includes('legal_representative')), tags: contact?.tags?.join(', ') || '' })
  contactModalOpen.value = true
}
async function action(work: (id: string, live: () => boolean) => Promise<void>) {
  if (saving.value) return
  const stamp = scopeVersion, request = ++actionVersion, id = customerUUID.value
  const live = () => validScope(stamp) && request === actionVersion
  saving.value = true; actionError.value = ''
  try { await work(id, live) } catch (cause) { if (live()) actionError.value = customerAdminError(cause, t('customerMaster.saveFailed')) }
  finally { if (live()) saving.value = false }
}
async function saveContact(temporary = false) {
  const selected = selectedContact.value
  if (selected ? !allowed.update : !allowed.create) return
  const fields = [contactForm.display_name, contactForm.given_name, contactForm.family_name]
  if (!customerNameValid(contactForm.display_name, true) || fields.some(v => !customerNameValid(v))) { actionError.value = t('customerMaintenance.nameInvalid'); return }
  const roles = [...(selected?.roles || []).filter(role => role !== 'primary' && role !== 'legal_representative'), ...(selected && isPrimary(selected) ? ['primary'] : []), ...(contactForm.legal ? ['legal_representative'] : [])]
  const input = { display_name: contactForm.display_name.trim(), given_name: contactForm.given_name.trim(), family_name: contactForm.family_name.trim(), email: contactForm.email.trim(), phone: contactForm.phone.trim(), roles, tags: contactForm.tags.split(',').map(v => v.trim()).filter(Boolean) }
  await action(async (id, live) => {
    if (selected) await api.updateContact(id, selected.uuid, input)
    else await api.createContact(id, { ...input, status: temporary ? 'temporary' : 'active', creation_intent: temporary ? 'explicit_temporary' : 'explicit_create' })
    if (!live()) return
    void load(); contactModalOpen.value = false
  })
}
async function deactivate(contact: CustomerContact) {
  if (!allowed.update || isPrimary(contact)) return
  await action(async (id, live) => { await api.updateContact(id, contact.uuid, { status: 'inactive' }); if (live()) void load() })
}
async function openIdentities(contact: CustomerContact) {
  if (!allowed.identityRead || saving.value) return
  selectedContact.value = contact; contactIdentities.value = []; resolveResult.value = null
  Object.assign(identityForm, { channel_dictionary_item_uuid: '', external_subject: '' })
  identityModalOpen.value = true
  await action(async (id, live) => { const page = await api.listContactIdentities(id, contact.uuid); if (live()) contactIdentities.value = page.items })
}
async function bindIdentity() {
  const selected = selectedContact.value, channel = identityForm.channel_dictionary_item_uuid, subject = identityForm.external_subject.trim()
  if (!allowed.bind || !selected || !channel || !subject || contactChannelsUnavailable.value) return
  await action(async (id, live) => {
    await api.bindContactIdentity(id, selected.uuid, channel, subject)
    if (!live()) return
    const page = await api.listContactIdentities(id, selected.uuid)
    if (!live()) return
    contactIdentities.value = page.items; resolveResult.value = null
    Object.assign(identityForm, { channel_dictionary_item_uuid: '', external_subject: '' })
  })
}
async function resolveIdentity() {
  const channel = identityForm.channel_dictionary_item_uuid, subject = identityForm.external_subject.trim()
  if (!allowed.resolve || !channel || !subject || contactChannelsUnavailable.value) return
  await action(async (id, live) => { const result = await api.resolveContactIdentity(id, channel, subject); if (live()) resolveResult.value = result })
}
function openPrimary(contact: CustomerContact) {
  if (!allowed.primary || contact.status !== 'active' || saving.value) return
  selectedContact.value = contact; primaryExpected.value = account.value?.primary_contact_uuid || ''; actionError.value = ''; primaryModalOpen.value = true
}
async function setPrimary() {
  const selected = selectedContact.value, expected = primaryExpected.value
  if (!allowed.primary || !selected) return
  await action(async (id, live) => { await api.setPrimaryContact(id, selected.uuid, expected); if (live()) { void load(); primaryModalOpen.value = false } })
}
function rememberRepair(id: string) { void router.replace({ query: { ...route.query, repair_uuid: id } }) }
function profileUpdated(value: CustomerAccount) { account.value = value }
watch([contactModalOpen, identityModalOpen, primaryModalOpen], (now, old) => { if (old.some((open, i) => open && !now[i])) { actionVersion++; saving.value = false } }, { flush: 'sync' })
watch(scopeKey, (_key, old) => {
  scopeVersion++; loadVersion++; actionVersion++; loading.value = false; saving.value = false
  account.value = null; identities.value = []; memberships.value = []; loginEvents.value = []; entries.value = []; contacts.value = []
  contactTotal.value = 0
  selectedContact.value = null; contactIdentities.value = []; contactChannelItems.value = []; contactChannelsUnavailable.value = false
  contactModalOpen.value = false; identityModalOpen.value = false; primaryModalOpen.value = false; repairOpen.value = false; profileEditOpen.value = false
  actionError.value = ''; error.value = ''; resolveResult.value = null; primaryExpected.value = ''; activeTab.value = 'basic'
  Object.assign(contactForm, { display_name: '', given_name: '', family_name: '', email: '', phone: '', legal: false, tags: '' })
  Object.assign(identityForm, { channel_dictionary_item_uuid: '', external_subject: '' })
  Object.keys(allowed).forEach(k => { allowed[k as keyof typeof allowed] = false })
  if (old && route.query.repair_uuid) { const { repair_uuid, ...query } = route.query; void router.replace({ query }) }
  void load()
}, { immediate: true, flush: 'sync' })
onBeforeUnmount(() => { disposed = true; scopeVersion++; loadVersion++; actionVersion++ })
</script>

<template>
  <div class="space-y-6 p-6">
    <div class="flex flex-wrap items-start justify-between gap-3"><div><UButton variant="link" icon="i-heroicons-arrow-left" to="/customers">{{ t('customerMaster.back') }}</UButton><h1 class="mt-2 text-2xl font-bold text-gray-900 dark:text-white">{{ account?.display_name || account?.nickname || t('customerMaster.customer') }}</h1><p class="mt-1 font-mono text-xs text-gray-500">{{ customerUUID }}</p></div><UButton icon="i-heroicons-arrow-path" :loading="loading" @click="load">{{ t('common.refresh') }}</UButton></div>
    <UButton v-if="account && needsRepair && canRepair" data-testid="contact-repair" color="warning" variant="soft" @click="repairOpen = true">{{ t('customerMaintenance.repair') }}</UButton>
    <UButton v-if="account && allowed.profileUpdate" data-testid="customer-profile-edit" icon="i-heroicons-pencil-square" @click="profileEditOpen = true">{{ t('customerAdmin.editProfile') }}</UButton>
    <UAlert v-if="error" color="error" variant="soft" :title="error" />
    <div v-else-if="account" class="rounded-lg border border-gray-200 bg-white shadow-sm dark:border-gray-800 dark:bg-[#111a2b]"><div class="flex flex-wrap gap-1 border-b border-gray-200 p-3 dark:border-gray-800"><UButton v-for="tab in tabs" :key="tab.id" size="sm" :color="activeTab === tab.id ? 'primary' : 'neutral'" :variant="activeTab === tab.id ? 'soft' : 'ghost'" @click="activeTab = tab.id as any">{{ tab.label }}</UButton></div><div class="p-5">
      <section v-if="activeTab === 'basic'" class="grid gap-4 md:grid-cols-2"><div v-for="item in [{label:t('customerMaster.customer'), value:account.display_name || account.nickname},{label:t('customerMaster.email'), value:account.primary_email},{label:t('customerMaster.phone'), value:account.primary_phone},{label:t('common.status'), value:account.status},{label:t('customerMaster.source'), value:account.member_source},{label:t('customerMaster.updatedAt'), value:formatTime(account.updated_at)}]" :key="item.label" class="rounded-md border border-gray-200 p-3 dark:border-gray-800"><p class="text-xs text-gray-500">{{ item.label }}</p><p class="mt-1 break-all text-sm font-medium text-gray-900 dark:text-white">{{ item.value || '-' }}</p></div></section>
      <section v-else-if="activeTab === 'identities'" class="overflow-x-auto"><table class="min-w-full text-left text-sm"><thead><tr><th>{{ t('customerMaster.provider') }}</th><th>{{ t('customerMaster.subject') }}</th><th>{{ t('common.status') }}</th></tr></thead><tbody><tr v-for="identity in identities" :key="identity.uuid"><td>{{ identity.provider }}</td><td class="font-mono text-xs">{{ identity.provider_subject || identity.email || identity.phone || '-' }}</td><td>{{ identity.status }}</td></tr></tbody></table></section>
      <section v-else-if="activeTab === 'memberships'" class="overflow-x-auto"><table class="min-w-full text-left text-sm"><thead><tr><th>{{ t('customerMaster.customerUuid') }}</th><th>{{ t('customerMaster.source') }}</th><th>{{ t('common.status') }}</th><th>{{ t('customerMaster.expiresAt') }}</th></tr></thead><tbody><tr v-for="membership in memberships" :key="membership.uuid"><td class="font-mono text-xs">{{ short(membership.tenant_uuid) }}</td><td>{{ membership.source }}</td><td>{{ membership.status }}</td><td>{{ formatTime(membership.expires_at) }}</td></tr></tbody></table></section>
      <section v-else-if="activeTab === 'events'" class="overflow-x-auto"><table class="min-w-full text-left text-sm"><thead><tr><th>{{ t('customerMaster.provider') }}</th><th>{{ t('customerMaster.event') }}</th><th>{{ t('customerMaster.result') }}</th><th>{{ t('customerMaster.updatedAt') }}</th></tr></thead><tbody><tr v-for="event in loginEvents" :key="event.id"><td>{{ event.identity_provider || '-' }}</td><td>{{ event.event_type }}</td><td>{{ event.ok ? t('customerMaster.loginSuccess') : t('customerMaster.loginFailure') }}</td><td>{{ formatTime(event.created_at) }}</td></tr></tbody></table></section>
      <section v-else-if="activeTab === 'entries'"><UAlert color="info" variant="soft" :description="t('customerMaster.tenantEntryDescription')" class="mb-4" /><div class="overflow-x-auto"><table class="min-w-full text-left text-sm"><thead><tr><th>{{ t('customerMaster.entryCode') }}</th><th>{{ t('customerMaster.entryType') }}</th><th>{{ t('customerMaster.channel') }}</th><th>{{ t('common.status') }}</th></tr></thead><tbody><tr v-for="entry in entries" :key="entry.uuid"><td class="font-mono text-xs">{{ entry.entry_code }}</td><td>{{ entry.entry_type }}</td><td>{{ entry.channel || '-' }}</td><td>{{ entry.status }}</td></tr></tbody></table></div></section>
<section v-else><div class="mb-4 flex flex-wrap justify-between gap-2"><p class="text-sm text-gray-600 dark:text-gray-300">{{ t('customerMaster.contacts') }}</p><UButton data-testid="contact-create" :disabled="!allowed.create || saving" icon="i-heroicons-plus" @click="resetContactForm()">{{ t('customerMaster.createContact') }}</UButton></div><p v-if="actionError" class="mb-3 text-sm text-error-600">{{ actionError }}</p><div v-if="contacts.length === 0" class="rounded-md bg-gray-50 p-4 text-sm text-gray-500 dark:bg-slate-900">{{ t('customerMaster.contactEmpty') }}</div><div v-else class="space-y-2"><div v-for="contact in contacts" :key="contact.uuid" class="flex flex-wrap items-center justify-between gap-3 rounded-md border border-gray-200 p-3 dark:border-gray-800"><div><p class="font-medium text-gray-900 dark:text-white">{{ contact.display_name }}</p><p class="mt-1 text-xs text-gray-500">{{ contact.tags?.join(', ') || '-' }}</p><div v-if="contact.roles?.some(role => role !== 'primary')" class="mt-2 flex flex-wrap items-center gap-1"><span class="text-xs text-gray-500">{{ t('customerMaster.roles') }}:</span><UBadge v-for="role in contact.roles.filter(role => role !== 'primary')" :key="role" size="xs" color="info" variant="soft">{{ roleLabel(role) }}</UBadge></div></div><div class="flex flex-wrap items-center gap-2"><UBadge v-if="isPrimary(contact)" color="success" variant="soft">{{ t('customerMaster.rolePrimary') }}</UBadge><UBadge variant="soft">{{ contact.status }}</UBadge><UButton v-if="allowed.primary && contact.status === 'active' && !isPrimary(contact)" data-testid="contact-set-primary" size="xs" variant="soft" :disabled="saving" @click="openPrimary(contact)">{{ t('customerMaintenance.setPrimary') }}</UButton><UButton v-if="allowed.identityRead" data-testid="contact-identities" size="xs" :disabled="saving" variant="ghost" @click="openIdentities(contact)">{{ t('common.contactChannelsTitle') }}</UButton><UButton v-if="allowed.update" data-testid="contact-edit" size="xs" :disabled="saving" variant="ghost" @click="resetContactForm(contact)">{{ t('customerMaster.edit') }}</UButton><UButton v-if="allowed.update && contact.status !== 'inactive'" data-testid="contact-disable" :disabled="isPrimary(contact)" :title="isPrimary(contact) ? t('customerMaintenance.replaceBeforeDisable') : undefined" size="xs" color="error" variant="ghost" :loading="saving" @click="deactivate(contact)">{{ t('customerMaster.disable') }}</UButton></div></div></div></section>
    </div></div>
    <UModal v-model:open="contactModalOpen" :dismissible="!saving" :close="!saving" :title="selectedContact ? t('customerMaster.edit') : t('customerMaster.createContact')"><template #body><div class="space-y-4"><UFormField :label="t('customerMaster.displayName')" required><UInput v-model="contactForm.display_name" /></UFormField><div class="grid grid-cols-2 gap-3"><UFormField :label="t('customerMaster.givenName')"><UInput v-model="contactForm.given_name" /></UFormField><UFormField :label="t('customerMaster.familyName')"><UInput v-model="contactForm.family_name" /></UFormField></div><div class="grid grid-cols-2 gap-3"><UFormField :label="t('customerMaster.email')"><UInput v-model="contactForm.email" type="email" /></UFormField><UFormField :label="t('customerMaster.phone')"><UInput v-model="contactForm.phone" /></UFormField></div><UFormField :label="t('customerMaster.tags')" :help="t('customerMaster.tagsHint')"><UInput v-model="contactForm.tags" /></UFormField><label class="flex gap-2 text-sm"><input v-model="contactForm.legal" type="checkbox">{{ t('customerMaster.roleLegalRepresentative') }}</label><UAlert v-if="!selectedContact" color="warning" variant="soft" :description="t('customerMaster.temporaryNotice')" /><p v-if="actionError" class="text-sm text-error-600">{{ actionError }}</p><div class="flex justify-end gap-2"><UButton :disabled="saving" variant="ghost" @click="contactModalOpen = false">{{ t('common.cancel') }}</UButton><UButton v-if="!selectedContact" variant="soft" :loading="saving" @click="saveContact(true)">{{ t('customerMaster.saveTemporary') }}</UButton><UButton data-testid="contact-save" :loading="saving" @click="saveContact(false)">{{ t('common.save') }}</UButton></div></div></template></UModal>
    <UModal v-model:open="identityModalOpen" :dismissible="!saving" :close="!saving" :title="t('common.contactChannelsTitle')" :ui="{ content: 'w-full max-w-2xl' }"><template #body><div class="space-y-5"><UAlert color="info" variant="soft" :description="t('common.contactChannelsDescription', { name: selectedContact?.display_name || '' })" /><UAlert v-if="contactChannelsUnavailable" color="error" variant="soft" :title="t('common.contactChannelDictionaryUnavailable')" /><div><h3 class="font-medium">{{ t('common.boundContactChannels') }}</h3><div v-if="contactIdentities.length" class="mt-2 space-y-2"><div v-for="identity in contactIdentities" :key="identity.uuid" class="rounded-md border border-gray-200 p-3 dark:border-gray-800"><p class="font-medium">{{ channelLabel(identity.channel_dictionary_item_uuid) }}</p><p class="font-mono text-xs text-gray-500">{{ identity.external_subject }}</p></div></div><p v-else class="mt-2 text-sm text-gray-500">{{ t('customerMaster.identityEmpty') }}</p></div><div class="border-t border-gray-200 pt-4 dark:border-gray-800"><h3 class="font-medium">{{ t('common.contactChannelAccount') }}</h3><p class="mt-1 text-sm text-gray-500">{{ t('common.contactChannelRelationship', { name: selectedContact?.display_name || '' }) }}</p><UFormField :label="t('customerMaster.channel')" class="mt-3"><USelectMenu v-model="identityForm.channel_dictionary_item_uuid" :items="channelOptions" value-key="value" label-key="label" :disabled="contactChannelsUnavailable || saving" :portal="false" class="w-full" :ui="{ content: 'min-w-[22rem] max-w-[calc(100vw-3rem)]', item: 'whitespace-normal break-words' }" /></UFormField><UFormField :label="t('customerMaster.externalSubject')" class="mt-3"><UInput v-model="identityForm.external_subject" :disabled="contactChannelsUnavailable" class="w-full" /></UFormField><UAlert v-if="resolveResult" class="mt-3" color="success" variant="soft" :description="t('common.resolvedContactChannel', { name: resolveResult.contact?.display_name || '' })" /><div class="mt-4 flex flex-wrap justify-end gap-2"><UButton v-if="allowed.resolve" data-testid="contact-resolve" variant="soft" :loading="saving" :disabled="contactChannelsUnavailable" @click="resolveIdentity">{{ t('customerMaster.resolveIdentity') }}</UButton><UButton v-if="allowed.bind" data-testid="contact-bind" :loading="saving" :disabled="contactChannelsUnavailable" @click="bindIdentity">{{ t('customerMaster.bindIdentity') }}</UButton></div></div><p v-if="actionError" class="text-sm text-error-600">{{ actionError }}</p></div></template></UModal>

    <UModal v-model:open="primaryModalOpen" :dismissible="!saving" :close="!saving" :title="t('customerMaintenance.setPrimary')"><template #body><div class="space-y-4"><p>{{ t('customerMaintenance.primaryConfirm', { name: selectedContact?.display_name || '' }) }}</p><UAlert v-if="actionError" color="error" :title="actionError"/><div class="flex justify-end gap-2"><UButton variant="ghost" :disabled="saving" @click="primaryModalOpen = false">{{ t('common.cancel') }}</UButton><UButton data-testid="primary-confirm" :loading="saving" @click="setPrimary">{{ t('common.confirm') }}</UButton></div></div></template></UModal>
    <CustomerContactRepairModal v-if="account" :key="scopeKey" v-model:open="repairOpen" :allowed="canRepair" :account="account" :contacts="contacts" :initial-id="repairID" @previewed="rememberRepair" @completed="load" />
    <CustomerEditModal :key="`profile-${scopeKey}`" v-model:open="profileEditOpen" :allowed="allowed.profileUpdate" :account="account" @updated="profileUpdated" />
  </div>
</template>
