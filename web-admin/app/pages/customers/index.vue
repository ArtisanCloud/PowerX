<script setup lang="ts">
import { useCustomerService } from '~/composables/api/services/customerService'

definePageMeta({
  title: 'customers',
  icon: 'i-heroicons-users',
  order: 3
})

const { t } = useI18n()
const { list } = useCustomerService()
const customers = ref<any[]>([])
const loading = ref(false)
const error = ref('')
const search = ref('')
const load = async () => {
  loading.value = true
  error.value = ''
  try { customers.value = (await list(1, 100, search.value)).items } catch { error.value = t('customerMaster.loadFailed') } finally { loading.value = false }
}
onMounted(load)
</script>

<template>
  <div class="space-y-6 p-6">
    <div class="mb-6">
      <h1 class="text-2xl font-bold text-gray-900 dark:text-white">{{ t('customerMaster.title') }}</h1>
      <p class="mt-2 text-gray-600 dark:text-gray-300">{{ t('customerMaster.description') }}</p>
    </div>
    <div class="rounded-lg border border-gray-200 bg-white p-5 shadow-sm dark:border-gray-800 dark:bg-[#111a2b]">
      <div class="mb-4 flex flex-wrap justify-between gap-3">
        <UInput v-model="search" icon="i-heroicons-magnifying-glass" class="w-full sm:w-96" :placeholder="t('customerMaster.search')" @keyup.enter="load" />
        <UButton icon="i-heroicons-arrow-path" :loading="loading" @click="load">{{ t('common.refresh') }}</UButton>
      </div>
      <UAlert v-if="error" color="error" variant="soft" :title="error" class="mb-4" />
      <div v-if="!loading && !error && customers.length === 0" class="rounded-md bg-gray-50 p-5 text-sm text-gray-600 dark:bg-slate-900 dark:text-gray-300">{{ t('customerMaster.empty') }}</div>
      <div v-else class="overflow-x-auto">
        <table class="min-w-full text-left text-sm"><thead class="border-b border-gray-200 text-xs text-gray-500 dark:border-gray-700 dark:text-gray-400"><tr><th class="px-3 py-3">{{ t('customerMaster.customer') }}</th><th class="px-3 py-3">{{ t('customerMaster.customerUuid') }}</th><th class="px-3 py-3">{{ t('customerMaster.email') }}</th><th class="px-3 py-3">{{ t('common.status') }}</th><th class="px-3 py-3 text-right">{{ t('customerMaster.actions') }}</th></tr></thead><tbody class="divide-y divide-gray-100 dark:divide-gray-800"><tr v-for="customer in customers" :key="customer.uuid"><td class="px-3 py-3 font-medium text-gray-900 dark:text-white">{{ customer.display_name || customer.nickname || customer.primary_email || t('common.unknown') }}</td><td class="px-3 py-3 font-mono text-xs text-gray-500">{{ customer.uuid }}</td><td class="px-3 py-3 text-gray-600 dark:text-gray-300">{{ customer.primary_email || '-' }}</td><td class="px-3 py-3"><UBadge variant="soft" :color="customer.status === 'active' ? 'success' : 'neutral'">{{ customer.status }}</UBadge></td><td class="px-3 py-3 text-right"><UButton size="xs" variant="soft" icon="i-heroicons-eye" :to="`/customers/${customer.uuid}`">{{ t('customerMaster.open') }}</UButton></td></tr></tbody></table>
      </div>
    </div>
  </div>
</template>
