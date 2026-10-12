<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useCustomerService, type CustomerAccount } from '~/composables/api/services/customerService'
import { useUserContext } from '~/composables/api/services/meService'
import { useUserStore } from '~/stores/user'
import CustomerCreateModal from '~/components/customers/CustomerCreateModal.vue'
import CustomerImportModal from '~/components/customers/CustomerImportModal.vue'
import CustomerEditModal from '~/components/customers/CustomerEditModal.vue'
import { customerAdminError } from '~/utils/customer-admin'

definePageMeta({
  title: 'customers',
  icon: 'i-heroicons-users',
  order: 3
})

const { t } = useI18n()
const { list } = useCustomerService()
const access = useUserContext()
const userStore = useUserStore()
const route = useRoute(), router = useRouter()
const customers = ref<CustomerAccount[]>([])
const loading = ref(false)
const error = ref('')
const search = ref('')
const page = ref(1), pageSize = 20, total = ref(0)
const canCreate = ref(false), canImport = ref(false), permissionsLoading = ref(true)
const canEdit = ref(false), editOpen = ref(false), editingAccount = ref<CustomerAccount | null>(null)
const createOpen = ref(false), importOpen = ref(false)
const initialImportID = computed(() => typeof route.query.import_uuid === 'string' ? route.query.import_uuid : '')
const tenantKey = computed(() => userStore.currentTenantUuid || '')
const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize)))
let revision = 0, accessRevision = 0, disposed = false
const load = async () => {
  const stamp = ++revision
  loading.value = true
  error.value = ''
  try {
    const result = await list(page.value, pageSize, search.value.trim())
    if (!disposed && stamp === revision) { customers.value = result.items; total.value = result.pagination.total }
  } catch (e) { if (!disposed && stamp === revision) error.value = customerAdminError(e, t('customerMaster.loadFailed')) }
  finally { if (stamp === revision) loading.value = false }
}
async function loadAccess() {
  const stamp = ++accessRevision
  canCreate.value = false; canImport.value = false; canEdit.value = false; permissionsLoading.value = true
  try {
    const permissions = await Promise.all(['customer.admin_customers_accounts:create', 'customer.admin_customers_imports_preview:create', 'customer.admin_customers_imports_import_uuid:read', 'customer.admin_customers_imports_import_uuid_commit:create', 'customer.admin_customers_accounts_customer_uuid:update'].map(permission => access.hasPermission(permission).catch(() => false)))
    if (!disposed && stamp === accessRevision) { canCreate.value = permissions[0] === true; canImport.value = permissions.slice(1, 4).every(Boolean); canEdit.value = permissions[4] === true }
  } finally { if (stamp === accessRevision) permissionsLoading.value = false }
}
function searchCustomers() { page.value = 1; void load() }
function changePage(next: number) { page.value = next; void load() }
function rememberImport(id: string) { void router.replace({ query: { ...route.query, import_uuid: id } }) }
function customerCreated() { page.value = 1; search.value = ''; void load() }
function editCustomer(account: CustomerAccount) { if (canEdit.value) { editingAccount.value = account; editOpen.value = true } }
function customerUpdated() { void load() }
onMounted(async () => { await Promise.all([load(), loadAccess()]); if (!disposed && canImport.value && initialImportID.value) importOpen.value = true })
watch(tenantKey, (_value, previous) => { if (previous) { revision++; customers.value = []; createOpen.value = false; importOpen.value = false; editOpen.value = false; editingAccount.value = null; page.value = 1; const { import_uuid, ...query } = route.query; void router.replace({ query }); void Promise.all([load(), loadAccess()]) } })
onBeforeUnmount(() => { disposed = true; revision++; accessRevision++ })
</script>

<template>
  <div class="space-y-6 p-6">
    <div class="mb-6">
      <h1 class="text-2xl font-bold text-gray-900 dark:text-white">{{ t('customerMaster.title') }}</h1>
      <p class="mt-2 text-gray-600 dark:text-gray-300">{{ t('customerMaster.description') }}</p>
      <div class="mt-4 flex flex-wrap gap-2">
        <UButton icon="i-heroicons-plus" :disabled="permissionsLoading || !canCreate" data-testid="customer-new" @click="createOpen = true">{{ t('customerAdmin.create') }}</UButton>
        <UButton icon="i-heroicons-arrow-up-tray" color="neutral" variant="outline" :disabled="permissionsLoading || !canImport" data-testid="customer-import" @click="importOpen = true">{{ t('customerAdmin.import') }}</UButton>
      </div>
      <p v-if="!permissionsLoading && (!canCreate || !canImport)" class="mt-2 text-sm text-gray-600 dark:text-gray-300">{{ t('customerAdmin.permissionHint') }}</p>
    </div>
    <div class="rounded-lg border border-gray-200 bg-white p-5 shadow-sm dark:border-gray-800 dark:bg-[#111a2b]">
      <div class="mb-4 flex flex-wrap justify-between gap-3">
        <UInput v-model="search" icon="i-heroicons-magnifying-glass" class="w-full sm:w-96" :placeholder="t('customerMaster.search')" @keyup.enter="searchCustomers" />
        <UButton icon="i-heroicons-arrow-path" :loading="loading" @click="load">{{ t('common.refresh') }}</UButton>
      </div>
      <UAlert v-if="error" color="error" variant="soft" :title="error" class="mb-4" />
      <div v-if="!loading && !error && customers.length === 0" class="rounded-md bg-gray-50 p-5 text-sm text-gray-600 dark:bg-slate-900 dark:text-gray-300">{{ t('customerMaster.empty') }}</div>
      <div v-else class="overflow-x-auto">
        <table class="min-w-full text-left text-sm"><thead class="border-b border-gray-200 text-xs text-gray-500 dark:border-gray-700 dark:text-gray-400"><tr><th class="px-3 py-3">{{ t('customerMaster.customer') }}</th><th class="px-3 py-3">{{ t('customerMaster.customerUuid') }}</th><th class="px-3 py-3">{{ t('customerMaster.email') }}</th><th class="px-3 py-3">{{ t('common.status') }}</th><th class="px-3 py-3 text-right">{{ t('customerMaster.actions') }}</th></tr></thead><tbody class="divide-y divide-gray-100 dark:divide-gray-800"><tr v-for="customer in customers" :key="customer.uuid"><td class="px-3 py-3 font-medium text-gray-900 dark:text-white">{{ customer.display_name || customer.nickname || customer.primary_email || t('common.unknown') }}</td><td class="px-3 py-3 font-mono text-xs text-gray-500">{{ customer.uuid }}</td><td class="px-3 py-3 text-gray-600 dark:text-gray-300">{{ customer.primary_email || '-' }}</td><td class="px-3 py-3"><UBadge variant="soft" :color="customer.status === 'active' ? 'success' : 'neutral'">{{ customer.status }}</UBadge></td><td class="px-3 py-3 text-right"><UButton v-if="canEdit" size="xs" variant="outline" icon="i-heroicons-pencil-square" class="mr-2" data-testid="customer-edit" @click="editCustomer(customer)">{{ t('customerAdmin.editProfile') }}</UButton><UButton size="xs" variant="soft" icon="i-heroicons-eye" :to="`/customers/${customer.uuid}`">{{ t('customerMaster.open') }}</UButton></td></tr></tbody></table>
      </div>
      <div class="mt-4 flex flex-wrap items-center justify-between gap-3"><p class="text-sm text-gray-600 dark:text-gray-300">{{ t('customerAdmin.pagination', { total, page, pages: totalPages }) }}</p><div class="flex gap-2"><UButton color="neutral" variant="outline" :disabled="loading || page <= 1" data-testid="customer-prev" @click="changePage(page - 1)">{{ t('customerAdmin.previous') }}</UButton><UButton color="neutral" variant="outline" :disabled="loading || page >= totalPages" data-testid="customer-next" @click="changePage(page + 1)">{{ t('customerAdmin.next') }}</UButton></div></div>
    </div>
    <CustomerCreateModal :key="`create-${tenantKey}`" v-model:open="createOpen" :allowed="canCreate" @created="customerCreated" />
    <CustomerImportModal :key="`import-${tenantKey}`" v-model:open="importOpen" :allowed="canImport" :initial-id="initialImportID" @previewed="rememberImport" @completed="load" />
    <CustomerEditModal :key="`edit-${tenantKey}`" v-model:open="editOpen" :allowed="canEdit" :account="editingAccount" @updated="customerUpdated" />
  </div>
</template>
