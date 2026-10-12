<template>
  <div class="space-y-6 p-4">
    <div class="flex items-start justify-between gap-3">
      <div>
        <h1 class="text-lg font-semibold text-[var(--text-primary)]">{{ t('agentCapabilityApprovals.title') }}</h1>
        <p class="text-sm text-[var(--text-secondary)]">{{ t('agentCapabilityApprovals.description') }}</p>
      </div>
      <UButton variant="soft" icon="i-heroicons-arrow-left" :to="localePath('/settings/ai/agents')">{{ t('agentCapabilityApprovals.back') }}</UButton>
    </div>
    <UAlert v-if="loadError" color="error" variant="soft" :title="t('agentCapabilityApprovals.loadFailed')" />
    <div v-else-if="loading" class="text-sm text-[var(--text-secondary)]">{{ t('agentCapabilityApprovals.loading') }}</div>
    <UEmpty v-else-if="items.length === 0" icon="i-heroicons-check-circle" :title="t('agentCapabilityApprovals.emptyTitle')" :description="t('agentCapabilityApprovals.emptyDescription')" />
    <UCard v-for="item in items" :key="item.approval_uuid" class="border border-[var(--border-color)]">
      <div class="flex items-center justify-between gap-4">
        <div>
          <p class="font-medium text-[var(--text-primary)]">{{ t('agentCapabilityApprovals.requestTitle') }}</p>
          <p class="text-sm text-[var(--text-secondary)]">{{ t('agentCapabilityApprovals.requestedAt', { time: formatTime(item.created_at) }) }}</p>
        </div>
        <div class="flex gap-2">
          <UButton color="error" variant="soft" :loading="deciding === item.approval_uuid" @click="reject(item.approval_uuid)">{{ t('agentCapabilityApprovals.reject') }}</UButton>
          <UButton color="primary" :loading="deciding === item.approval_uuid" @click="approve(item.approval_uuid)">{{ t('agentCapabilityApprovals.approve') }}</UButton>
        </div>
      </div>
    </UCard>
  </div>
</template>

<script setup lang="ts">
const { t, locale } = useI18n()
const localePath = useLocalePath()
const service = useAgentCapabilityApprovalService()
const toast = useToast()
const items = ref<AgentCapabilityApproval[]>([])
const loading = ref(true)
const loadError = ref(false)
const deciding = ref('')
const formatTime = (value: string) => new Intl.DateTimeFormat(locale.value, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value))
const load = async () => { loading.value = true; loadError.value = false; try { items.value = (await service.listPending()).items || [] } catch { loadError.value = true } finally { loading.value = false } }
const decide = async (approvalUUID: string, action: 'approve' | 'reject') => { deciding.value = approvalUUID; try { await service[action](approvalUUID); items.value = items.value.filter(item => item.approval_uuid !== approvalUUID); toast.add({ title: t(action === 'approve' ? 'agentCapabilityApprovals.approved' : 'agentCapabilityApprovals.rejected'), color: 'success' }) } catch { toast.add({ title: t(action === 'approve' ? 'agentCapabilityApprovals.approveFailed' : 'agentCapabilityApprovals.rejectFailed'), color: 'error' }) } finally { deciding.value = '' } }
const approve = (approvalUUID: string) => decide(approvalUUID, 'approve')
const reject = (approvalUUID: string) => decide(approvalUUID, 'reject')
onMounted(load)
</script>
