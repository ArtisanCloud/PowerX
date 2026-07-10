<template>
  <div class="space-y-6 p-4">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-lg font-semibold text-[var(--text-primary)]">{{ t('agent.management.title') }}</h1>
        <p class="text-sm text-[var(--text-secondary)]">
          {{ t('agent.management.description') }}
        </p>
      </div>
      <div class="flex items-center gap-2">
        <UButton variant="soft" icon="i-heroicons-chat-bubble-left-right" :to="localePath('/agent/sessions')">
          {{ t('agent.management.enterChat') }}
        </UButton>
        <UButton color="primary" icon="i-heroicons-plus" @click="createAgentQuick">
          {{ t('agent.management.create') }}
        </UButton>
      </div>
    </div>

    <div class="grid grid-cols-1 gap-3 rounded-lg border border-[var(--border-color)] bg-[var(--bg-muted)]/20 p-3 lg:grid-cols-12">
      <UInput
        v-model="agentSearch"
        class="lg:col-span-6"
        icon="i-heroicons-magnifying-glass"
        :placeholder="t('agent.management.filters.searchPlaceholder')"
      />
      <USelect v-model="agentSourceFilter" class="lg:col-span-3" :items="agentSourceItems" />
      <USelect v-model="agentStatusFilter" class="lg:col-span-3" :items="agentStatusItems" />
    </div>

    <div class="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
      <UCard v-for="agent in paginatedAgents" :key="agent.uuid" class="border border-[var(--border-color)]">
        <div class="space-y-3">
          <div class="flex items-start justify-between gap-3">
            <div class="flex min-w-0 items-start gap-3">
              <UAvatar
                :src="agentAvatar(agent)"
                :alt="agent.name"
                :text="agentInitials(agent)"
                size="lg"
                class="shrink-0"
              />
              <div class="min-w-0">
                <div class="truncate text-base font-medium text-[var(--text-primary)]">{{ agent.name }}</div>
                <div class="break-all text-xs text-[var(--text-secondary)]">{{ agent.key }}</div>
              </div>
            </div>
            <UBadge class="shrink-0" :color="agent.status === 'active' ? 'success' : 'neutral'" variant="soft">
              {{ agentStatusLabel(agent.status) }}
            </UBadge>
          </div>

          <div class="flex flex-wrap items-center gap-2">
            <UBadge size="xs" variant="soft" :color="agentSourceColor(agent)">
              {{ agentSourceLabel(agent) }}
            </UBadge>
            <span v-if="agentPluginID(agent)" class="break-all text-xs text-[var(--text-muted)]">
              {{ agentPluginID(agent) }}
            </span>
          </div>

          <p class="line-clamp-2 text-sm text-[var(--text-secondary)]">
            {{ agent.description || t('agent.management.emptyDescription') }}
          </p>

          <div class="flex items-center gap-1">
            <UButton
              size="xs"
              variant="soft"
              icon="i-heroicons-chat-bubble-left-right"
              :to="`${localePath('/agent/sessions')}?agent_uuid=${encodeURIComponent(agent.uuid)}`"
              :title="t('agent.management.actions.chat')"
              :aria-label="t('agent.management.actions.chat')"
            />
            <UButton
              size="xs"
              variant="soft"
              icon="i-heroicons-user-group"
              :to="`${localePath('/settings/ai/agent-teams')}?parent_agent_uuid=${encodeURIComponent(agent.uuid)}`"
              :title="t('agent.management.actions.team')"
              :aria-label="t('agent.management.actions.team')"
            />
            <UButton
              size="xs"
              variant="soft"
              icon="i-heroicons-shield-check"
              :title="t('agent.management.actions.permissions')"
              :aria-label="t('agent.management.actions.permissions')"
              @click="openPermissionForm(agent)"
            />
            <UButton
              size="xs"
              variant="ghost"
              icon="i-heroicons-pencil-square"
              :title="t('agent.management.actions.edit')"
              :aria-label="t('agent.management.actions.edit')"
              @click="openEditForm(agent)"
            />
          </div>
        </div>
      </UCard>
    </div>

    <div v-if="filteredAgents.length > pageSize" class="flex items-center justify-between gap-3">
      <div class="text-sm text-[var(--text-secondary)]">
        {{ t('agent.management.pagination.total', { total: filteredAgents.length }) }}
      </div>
      <UPagination
        v-model:page="currentPage"
        :items-per-page="pageSize"
        :total="filteredAgents.length"
        :sibling-count="1"
        show-edges
      />
    </div>

    <UAlert
      v-if="errorText"
      color="error"
      variant="soft"
      icon="i-heroicons-exclamation-triangle"
      :title="errorText"
    />

    <UModal
      v-model:open="editOpen"
      :title="t('agent.management.edit.title')"
      :description="t('agent.management.edit.description')"
      :ui="{ content: 'sm:max-w-5xl' }"
    >
      <template #body>
        <div class="pr-1">
          <div class="mx-auto max-w-4xl space-y-5">
            <section class="space-y-3 rounded-lg border border-[var(--border-color)] bg-[var(--bg-muted)]/20 p-4">
              <div class="text-sm font-semibold text-[var(--text-primary)]">{{ t('agent.management.edit.basicInfo') }}</div>
              <div class="grid grid-cols-1 gap-4 lg:grid-cols-12">
                <UFormField class="lg:col-span-5" :label="t('agent.management.edit.name')" required>
                  <UInput v-model="editForm.name" class="w-full" :placeholder="t('agent.management.edit.namePlaceholder')" />
                </UFormField>
                <UFormField class="lg:col-span-3" :label="t('agent.management.edit.typeId')">
                  <UInput v-model="editForm.typeId" class="w-full" :placeholder="t('agent.management.edit.typeIdPlaceholder')" />
                </UFormField>
                <UFormField class="lg:col-span-2" :label="t('agent.management.edit.scene')">
                  <UInput v-model="editForm.scene" class="w-full" :placeholder="t('agent.management.edit.scenePlaceholder')" />
                </UFormField>
                <UFormField class="lg:col-span-2" :label="t('agent.management.edit.status')">
                  <USelect v-model="editForm.status" class="w-full" :items="statusItems" />
                </UFormField>
                <UFormField class="lg:col-span-12" :label="t('agent.management.edit.publicDescription')" :description="t('agent.management.edit.publicDescriptionHelp')">
                  <UInput v-model="editForm.description" class="w-full" :placeholder="t('agent.management.edit.publicDescriptionPlaceholder')" />
                </UFormField>
                <UFormField class="lg:col-span-12" :label="t('agent.management.edit.systemKey')">
                  <UInput class="w-full" :model-value="editForm.key" disabled />
                </UFormField>
              </div>
            </section>

            <section class="space-y-3 rounded-lg border border-[var(--border-color)] bg-[var(--bg-muted)]/20 p-4">
              <div class="text-sm font-semibold text-[var(--text-primary)]">{{ t('agent.management.edit.generationPolicy') }}</div>
              <div class="grid grid-cols-1 gap-4 lg:grid-cols-12">
                <UFormField class="lg:col-span-12" :label="t('agent.management.edit.promptSeed')" :description="t('agent.management.edit.promptSeedHelp')">
                  <UTextarea v-model="editForm.promptSeed" class="w-full min-h-56" :rows="8" :placeholder="t('agent.management.edit.promptSeedPlaceholder')" />
                </UFormField>
                <UFormField class="lg:col-span-12" :label="t('agent.management.edit.persona')" :description="t('agent.management.edit.personaHelp')">
                  <UTextarea v-model="editForm.persona" class="w-full min-h-40" :rows="5" :placeholder="t('agent.management.edit.personaPlaceholder')" />
                </UFormField>
              </div>
            </section>

            <section class="space-y-3 rounded-lg border border-[var(--border-color)] bg-[var(--bg-muted)]/20 p-4">
              <div class="text-sm font-semibold text-[var(--text-primary)]">{{ t('agent.management.edit.resourceBinding') }}</div>
              <div class="grid grid-cols-1 gap-4 lg:grid-cols-12">
                <UFormField class="lg:col-span-12" :label="t('agent.management.edit.skillIds')" :description="t('agent.management.edit.commaSeparated')">
                  <UInput v-model="editForm.skillIdsText" class="w-full" :placeholder="t('agent.management.edit.skillIdsPlaceholder')" />
                </UFormField>
                <UFormField class="lg:col-span-12" :label="t('agent.management.edit.knowledgeBaseIds')" :description="t('agent.management.edit.uuidCommaSeparated')">
                  <UInput v-model="editForm.knowledgeBaseIdsText" class="w-full" :placeholder="t('agent.management.edit.knowledgeBaseIdsPlaceholder')" />
                </UFormField>
              </div>
            </section>

            <section class="space-y-3 rounded-lg border border-[var(--border-color)] bg-[var(--bg-muted)]/20 p-4">
              <div class="flex items-center justify-between gap-3">
                <div>
                  <div class="text-sm font-semibold text-[var(--text-primary)]">
                    {{ t('agent.management.grants.title') }}
                  </div>
                  <p class="text-xs text-[var(--text-secondary)]">
                    {{ t('agent.management.grants.description') }}
                  </p>
                </div>
                <UBadge variant="soft" color="neutral">
                  {{ selectedVisibleGrantCount }}/{{ visibleGrantableCapabilities.length }}
                </UBadge>
              </div>

              <UTabs v-model="grantCategoryTab" :items="grantCategoryItems" />

              <div class="grid grid-cols-1 gap-2 lg:grid-cols-12">
                <UInput
                  v-model="grantSearch"
                  class="lg:col-span-6"
                  icon="i-heroicons-magnifying-glass"
                  :placeholder="t('agent.management.grants.searchPlaceholder')"
                />
                <USelect v-model="grantPluginFilter" class="lg:col-span-2" :items="grantPluginItems" />
                <USelect v-model="grantAvailabilityFilter" class="lg:col-span-2" :items="grantAvailabilityItems" />
                <USelect v-model="grantSelectionFilter" class="lg:col-span-2" :items="grantSelectionItems" />
              </div>

              <UAlert
                v-if="showGrantCategoryHint"
                color="warning"
                variant="soft"
                icon="i-heroicons-information-circle"
                :title="grantCategoryHint"
              />

              <UAlert
                v-if="grantError"
                color="error"
                variant="soft"
                icon="i-heroicons-exclamation-triangle"
                :title="grantError"
              />

              <div class="h-80 overflow-y-auto pr-1">
                <div v-if="grantLoading" class="flex h-full items-center justify-center text-sm text-[var(--text-secondary)]">
                  {{ t('agent.management.grants.loading') }}
                </div>
                <div v-else-if="grantableCapabilities.length === 0" class="flex h-full items-center justify-center text-sm text-[var(--text-secondary)]">
                  {{ t('agent.management.grants.empty') }}
                </div>
                <div v-else-if="visibleGrantableCapabilities.length === 0" class="flex h-full items-center justify-center text-sm text-[var(--text-secondary)]">
                  {{ t('agent.management.grants.filteredEmpty') }}
                </div>
                <div v-else class="space-y-2">
                  <div
                    v-for="item in visibleGrantableCapabilities"
                    :key="grantKey(item.capability_uuid, item.permission_code)"
                    class="flex items-start justify-between gap-4 rounded-md border border-[var(--border-color)] bg-[var(--bg-elevated)] p-3"
                  >
                    <div class="min-w-0 space-y-1">
                      <div class="flex flex-wrap items-center gap-2">
                        <span class="font-medium text-[var(--text-primary)]">{{ item.display_name || item.capability_id }}</span>
                        <UBadge size="xs" variant="soft" color="neutral">{{ item.risk_level || t('common.unknown') }}</UBadge>
                        <UBadge
                          size="xs"
                          variant="soft"
                          :color="item.tenant_enabled && item.agent_usable ? 'success' : 'warning'"
                        >
                          {{ item.tenant_enabled && item.agent_usable ? t('agent.management.grants.available') : t('agent.management.grants.unavailable') }}
                        </UBadge>
                      </div>
                      <div class="break-all font-mono text-xs text-[var(--text-secondary)]">
                        {{ item.permission_code }}
                      </div>
                      <div class="break-all text-xs text-[var(--text-muted)]">
                        {{ item.plugin_id }} · {{ item.capability_id }}
                      </div>
                    </div>
                    <USwitch
                      :model-value="selectedGrantKeys.has(grantKey(item.capability_uuid, item.permission_code))"
                      :disabled="!item.tenant_enabled || !item.agent_usable"
                      @update:model-value="setGrantSelected(item, Boolean($event))"
                    />
                  </div>
                </div>
              </div>
            </section>
          </div>
        </div>
      </template>
      <template #footer>
        <div class="sticky bottom-0 z-10 -mx-6 -mb-6 flex justify-end gap-2 border-t border-[var(--border-color)] bg-[var(--bg-elevated)] px-6 py-4">
          <UButton variant="soft" @click="editOpen = false">{{ t('common.cancel') }}</UButton>
          <UButton color="primary" :loading="submitting" @click="submitEdit">{{ t('common.save') }}</UButton>
        </div>
      </template>
    </UModal>

    <UModal
      v-model:open="permissionOpen"
      :title="t('agent.management.grants.title')"
      :description="permissionAgentName"
      :ui="{ content: 'sm:max-w-4xl' }"
    >
      <template #body>
        <section class="space-y-3">
          <div class="flex items-center justify-between gap-3">
            <div class="min-w-0 space-y-2">
              <p class="text-sm text-[var(--text-secondary)]">
                {{ t('agent.management.grants.description') }}
              </p>
              <div class="flex flex-wrap items-center gap-2 text-xs">
                <span class="text-[var(--text-secondary)]">{{ t('agent.management.grants.ownerPlugin') }}</span>
                <UBadge v-if="activeAgentPluginID" color="warning" variant="soft" class="break-all">
                  {{ activeAgentPluginID }}
                </UBadge>
                <UBadge v-else color="info" variant="soft">
                  {{ t('agent.management.sources.core') }}
                </UBadge>
              </div>
            </div>
            <UBadge variant="soft" color="neutral">
              {{ selectedVisibleGrantCount }}/{{ visibleGrantableCapabilities.length }}
            </UBadge>
          </div>

          <UTabs v-model="grantCategoryTab" :items="grantCategoryItems" />

          <div class="grid grid-cols-1 gap-2 lg:grid-cols-12">
            <UInput
              v-model="grantSearch"
              class="lg:col-span-6"
              icon="i-heroicons-magnifying-glass"
              :placeholder="t('agent.management.grants.searchPlaceholder')"
            />
            <USelect v-model="grantPluginFilter" class="lg:col-span-2" :items="grantPluginItems" />
            <USelect v-model="grantAvailabilityFilter" class="lg:col-span-2" :items="grantAvailabilityItems" />
            <USelect v-model="grantSelectionFilter" class="lg:col-span-2" :items="grantSelectionItems" />
          </div>

          <UAlert
            v-if="showGrantCategoryHint"
            color="warning"
            variant="soft"
            icon="i-heroicons-information-circle"
            :title="grantCategoryHint"
          />

          <UAlert
            v-if="grantError"
            color="error"
            variant="soft"
            icon="i-heroicons-exclamation-triangle"
            :title="grantError"
          />

          <div class="h-[28rem] overflow-y-auto pr-1">
            <div v-if="grantLoading" class="flex h-full items-center justify-center text-sm text-[var(--text-secondary)]">
              {{ t('agent.management.grants.loading') }}
            </div>
            <div v-else-if="grantableCapabilities.length === 0" class="flex h-full items-center justify-center text-sm text-[var(--text-secondary)]">
              {{ t('agent.management.grants.empty') }}
            </div>
            <div v-else-if="visibleGrantableCapabilities.length === 0" class="flex h-full items-center justify-center text-sm text-[var(--text-secondary)]">
              {{ t('agent.management.grants.filteredEmpty') }}
            </div>
            <div v-else class="space-y-2">
              <div
                v-for="item in visibleGrantableCapabilities"
                :key="grantKey(item.capability_uuid, item.permission_code)"
                class="flex items-start justify-between gap-4 rounded-md border border-[var(--border-color)] bg-[var(--bg-elevated)] p-3"
              >
                <div class="min-w-0 space-y-1">
                  <div class="flex flex-wrap items-center gap-2">
                    <span class="font-medium text-[var(--text-primary)]">{{ item.display_name || item.capability_id }}</span>
                    <UBadge size="xs" variant="soft" color="neutral">{{ item.risk_level || t('common.unknown') }}</UBadge>
                    <UBadge
                      size="xs"
                      variant="soft"
                      :color="item.tenant_enabled && item.agent_usable ? 'success' : 'warning'"
                    >
                      {{ item.tenant_enabled && item.agent_usable ? t('agent.management.grants.available') : t('agent.management.grants.unavailable') }}
                    </UBadge>
                  </div>
                  <div class="break-all font-mono text-xs text-[var(--text-secondary)]">
                    {{ item.permission_code }}
                  </div>
                  <div class="break-all text-xs text-[var(--text-muted)]">
                    {{ item.plugin_id }} · {{ item.capability_id }}
                  </div>
                </div>
                <USwitch
                  :model-value="selectedGrantKeys.has(grantKey(item.capability_uuid, item.permission_code))"
                  :disabled="!item.tenant_enabled || !item.agent_usable"
                  @update:model-value="setGrantSelected(item, Boolean($event))"
                />
              </div>
            </div>
          </div>
        </section>
      </template>
      <template #footer>
        <div class="flex justify-end gap-2">
          <UButton variant="soft" @click="permissionOpen = false">{{ t('common.cancel') }}</UButton>
          <UButton color="primary" :loading="permissionSubmitting" @click="submitPermissions">
            {{ t('common.save') }}
          </UButton>
        </div>
      </template>
    </UModal>
  </div>
</template>

<script setup lang="ts">
import { useAgentManager } from '~/composables/agent/useAgentManager'
import type { Agent, AgentGrantableCapability } from '~/types/agent'

const localePath = useLocalePath()
const { t } = useI18n()
const toast = useToast()
const {
  agents,
  fetchAgents,
  createAgent,
  updateAgent,
  fetchGrantableCapabilities,
  fetchAgentGrants,
  updateAgentGrants,
} = useAgentManager()

const errorText = ref('')
const editOpen = ref(false)
const permissionOpen = ref(false)
const submitting = ref(false)
const permissionSubmitting = ref(false)
const grantLoading = ref(false)
const grantError = ref('')
const grantableCapabilities = ref<AgentGrantableCapability[]>([])
const selectedGrantKeys = reactive(new Set<string>())
const grantSearch = ref('')
const grantCategoryTab = ref<'own' | 'core' | 'other'>('own')
const grantPluginFilter = ref('__all__')
const grantAvailabilityFilter = ref<'all' | 'available' | 'unavailable'>('all')
const grantSelectionFilter = ref<'all' | 'selected' | 'unselected'>('all')
const activeGrantAgent = ref<Agent | null>(null)
const editUUID = ref('')
const permissionUUID = ref('')
const permissionAgentName = ref('')
const currentPage = ref(1)
const pageSize = 9
const agentSearch = ref('')
const agentSourceFilter = ref<'all' | 'core' | 'plugin'>('all')
const agentStatusFilter = ref<'all' | 'active' | 'draft' | 'disabled'>('all')
const editForm = reactive({
  key: '',
  name: '',
  description: '',
  status: 'draft',
  typeId: '',
  scene: '',
  promptSeed: '',
  persona: '',
  skillIdsText: '',
  knowledgeBaseIdsText: '',
})
const statusItems = computed(() => [
  { label: t('agent.management.status.draft'), value: 'draft' },
  { label: t('agent.management.status.active'), value: 'active' },
  { label: t('agent.management.status.disabled'), value: 'disabled' },
])
const agentSourceItems = computed(() => [
  { label: t('agent.management.filters.allSources'), value: 'all' },
  { label: t('agent.management.sources.core'), value: 'core' },
  { label: t('agent.management.sources.plugin'), value: 'plugin' },
])
const agentStatusItems = computed(() => [
  { label: t('agent.management.filters.allStatuses'), value: 'all' },
  { label: t('agent.management.status.active'), value: 'active' },
  { label: t('agent.management.status.draft'), value: 'draft' },
  { label: t('agent.management.status.disabled'), value: 'disabled' },
])
const filteredAgents = computed(() => {
  const keyword = agentSearch.value.trim().toLowerCase()
  return agents.value.filter((agent) => {
    if (agentSourceFilter.value === 'core' && agentSourceType(agent) !== 'core') return false
    if (agentSourceFilter.value === 'plugin' && agentSourceType(agent) !== 'plugin') return false
    if (agentStatusFilter.value !== 'all' && agent.status !== agentStatusFilter.value) return false
    if (!keyword) return true
    return [
      agent.name,
      agent.key,
      agent.description,
      agent.source,
      agent.ownerPluginId,
      agentPluginID(agent),
    ].some((value) => String(value || '').toLowerCase().includes(keyword))
  })
})
const paginatedAgents = computed(() => {
  const start = (currentPage.value - 1) * pageSize
  return filteredAgents.value.slice(start, start + pageSize)
})
const activeAgentPluginID = computed(() => agentPluginID(activeGrantAgent.value))
const ownPluginGrantCapabilities = computed(() => {
  const pluginID = activeAgentPluginID.value
  if (!pluginID) return []
  return grantableCapabilities.value.filter((item) => item.plugin_id === pluginID)
})
const coreGrantCapabilities = computed(() =>
  grantableCapabilities.value.filter((item) => isCoreCapabilityPlugin(item.plugin_id))
)
const otherPluginGrantCapabilities = computed(() => {
  const pluginID = activeAgentPluginID.value
  return grantableCapabilities.value.filter((item) => {
    if (isCoreCapabilityPlugin(item.plugin_id)) return false
    if (pluginID && item.plugin_id === pluginID) return false
    return true
  })
})
const grantCategoryItems = computed(() => [
  { label: t('agent.management.grants.tabs.own', { count: ownPluginGrantCapabilities.value.length }), value: 'own' },
  { label: t('agent.management.grants.tabs.core', { count: coreGrantCapabilities.value.length }), value: 'core' },
  { label: t('agent.management.grants.tabs.other', { count: otherPluginGrantCapabilities.value.length }), value: 'other' },
])
const categorizedGrantCapabilities = computed(() => {
  if (grantCategoryTab.value === 'own') return ownPluginGrantCapabilities.value
  if (grantCategoryTab.value === 'core') return coreGrantCapabilities.value
  return otherPluginGrantCapabilities.value
})
const grantCategoryHint = computed(() => {
  if (grantCategoryTab.value === 'own') {
    return activeAgentPluginID.value
      ? t('agent.management.grants.hints.ownEmpty', { pluginId: activeAgentPluginID.value })
      : t('agent.management.grants.hints.noOwnerPlugin')
  }
  if (grantCategoryTab.value === 'core') return t('agent.management.grants.hints.coreEmpty')
  return t('agent.management.grants.hints.otherEmpty')
})
const showGrantCategoryHint = computed(() =>
  grantableCapabilities.value.length > 0 && categorizedGrantCapabilities.value.length === 0
)
const grantPluginItems = computed(() => {
  const plugins = Array.from(new Set(categorizedGrantCapabilities.value.map((item) => item.plugin_id).filter(Boolean))).sort()
  return [
    { label: t('agent.management.grants.filters.allPlugins'), value: '__all__' },
    ...plugins.map((plugin) => ({ label: plugin, value: plugin })),
  ]
})
const grantAvailabilityItems = computed(() => [
  { label: t('agent.management.grants.filters.anyAvailability'), value: 'all' },
  { label: t('agent.management.grants.filters.available'), value: 'available' },
  { label: t('agent.management.grants.filters.unavailable'), value: 'unavailable' },
])
const grantSelectionItems = computed(() => [
  { label: t('agent.management.grants.filters.anySelection'), value: 'all' },
  { label: t('agent.management.grants.filters.selected'), value: 'selected' },
  { label: t('agent.management.grants.filters.unselected'), value: 'unselected' },
])
const visibleGrantableCapabilities = computed(() => {
  const keyword = grantSearch.value.trim().toLowerCase()
  return categorizedGrantCapabilities.value.filter((item) => {
    if (grantPluginFilter.value !== '__all__' && item.plugin_id !== grantPluginFilter.value) return false
    const available = Boolean(item.tenant_enabled && item.agent_usable)
    if (grantAvailabilityFilter.value === 'available' && !available) return false
    if (grantAvailabilityFilter.value === 'unavailable' && available) return false
    const selected = selectedGrantKeys.has(grantKey(item.capability_uuid, item.permission_code))
    if (grantSelectionFilter.value === 'selected' && !selected) return false
    if (grantSelectionFilter.value === 'unselected' && selected) return false
    if (!keyword) return true
    return [
      item.display_name,
      item.capability_id,
      item.permission_code,
      item.plugin_id,
      item.description,
    ].some((value) => String(value || '').toLowerCase().includes(keyword))
  })
})
const selectedVisibleGrantCount = computed(() =>
  visibleGrantableCapabilities.value.filter((item) => selectedGrantKeys.has(grantKey(item.capability_uuid, item.permission_code))).length
)

watch(
  () => filteredAgents.value.length,
  (total) => {
    const maxPage = Math.max(1, Math.ceil(total / pageSize))
    if (currentPage.value > maxPage) currentPage.value = maxPage
  }
)

watch([agentSearch, agentSourceFilter, agentStatusFilter], () => {
  currentPage.value = 1
})

const agentAvatar = (agent: Agent) => {
  const meta = agent.meta || {}
  return String(meta.avatar || meta.avatar_url || meta.icon_url || '').trim() || undefined
}

const agentInitials = (agent: Agent) => {
  const name = String(agent.name || agent.key || t('agent.management.defaultInitial')).trim()
  return name.slice(0, 1).toUpperCase()
}

const agentStatusLabel = (status?: string) => {
  const normalized = String(status || '').trim()
  if (normalized === 'draft') return t('agent.management.status.draft')
  if (normalized === 'active') return t('agent.management.status.active')
  if (normalized === 'disabled') return t('agent.management.status.disabled')
  return t('common.unknown')
}

const agentSourceType = (agent: Agent) => agentPluginID(agent) ? 'plugin' : 'core'

const agentSourceLabel = (agent: Agent) => {
  if (agentSourceType(agent) === 'plugin') return t('agent.management.sources.plugin')
  return t('agent.management.sources.core')
}

const agentSourceColor = (agent: Agent) => agentSourceType(agent) === 'plugin' ? 'warning' : 'info'

const load = async () => {
  try {
    errorText.value = ''
    await fetchAgents()
  } catch (e: any) {
    errorText.value = e?.message || t('agent.list.loadFailed')
  }
}

const createAgentQuick = async () => {
  const key = `agent_${Date.now().toString().slice(-6)}`
  const suffix = key.slice(-4)
  try {
    await createAgent({
      key,
      name: t('agent.management.quickCreate.name', { suffix }),
      description: t('agent.management.quickCreate.description'),
      status: 'active',
      meta: {},
    })
    toast.add({
      title: t('agent.management.quickCreate.success'),
      description: t('agent.management.quickCreate.successDescription'),
      color: 'success',
    })
    await load()
  } catch (e: any) {
    toast.add({ title: t('agent.management.quickCreate.failed'), description: e?.message || t('common.unknown'), color: 'error' })
  }
}

const openEditForm = async (agent: Agent) => {
  resetGrantFilters(agent)
  editUUID.value = agent.uuid
  editForm.key = agent.key || ''
  editForm.name = agent.name || ''
  editForm.description = agent.description || ''
  editForm.status = agent.status || 'draft'
  editForm.typeId = (agent as any).typeId || ''
  editForm.scene = (agent as any).scene || ''
  editForm.promptSeed = (agent as any).promptSeed || ''
  editForm.persona = (agent as any).persona || ''
  editForm.skillIdsText = normalizeArrayText((agent as any).skillIds)
  editForm.knowledgeBaseIdsText = normalizeArrayText((agent as any).knowledgeBaseIds)
  editOpen.value = true
  await loadAgentGrantState(agent.uuid)
}

const openPermissionForm = async (agent: Agent) => {
  resetGrantFilters(agent)
  permissionUUID.value = agent.uuid
  permissionAgentName.value = agent.name || agent.key || ''
  permissionOpen.value = true
  await loadAgentGrantState(agent.uuid)
}

const submitPermissions = async () => {
  if (!permissionUUID.value) return
  try {
    permissionSubmitting.value = true
    await updateAgentGrants(permissionUUID.value, grantPayload())
    toast.add({ title: t('agent.management.grants.saved'), color: 'success' })
    permissionOpen.value = false
  } catch (e: any) {
    toast.add({
      title: t('agent.management.grants.saveFailed'),
      description: e?.message || '',
      color: 'error',
    })
  } finally {
    permissionSubmitting.value = false
  }
}

const submitEdit = async () => {
  if (!editUUID.value) return
  if (!editForm.name.trim()) {
    toast.add({ title: t('agent.management.edit.nameRequired'), color: 'warning' })
    return
  }
  try {
    submitting.value = true
    await updateAgent(editUUID.value, {
      name: editForm.name.trim(),
      description: editForm.description.trim(),
      status: editForm.status,
      typeId: editForm.typeId.trim(),
      scene: editForm.scene.trim(),
      promptSeed: editForm.promptSeed.trim(),
      persona: editForm.persona.trim(),
      skillIds: parseCommaValues(editForm.skillIdsText),
      knowledgeBaseIds: parseCommaValues(editForm.knowledgeBaseIdsText),
    })
    await updateAgentGrants(editUUID.value, grantPayload())
    toast.add({ title: t('agent.management.edit.updated'), color: 'success' })
    editOpen.value = false
    await load()
  } catch (e: any) {
    toast.add({ title: t('agent.management.edit.updateFailed'), description: e?.message || t('common.unknown'), color: 'error' })
  } finally {
    submitting.value = false
  }
}

const loadAgentGrantState = async (agentUUID: string) => {
  try {
    grantLoading.value = true
    grantError.value = ''
    selectedGrantKeys.clear()
    const [catalog, grants] = await Promise.all([
      fetchGrantableCapabilities(),
      fetchAgentGrants(agentUUID),
    ])
    grantableCapabilities.value = catalog
    for (const grant of grants) {
      if (grant.status === 'enabled') {
        selectedGrantKeys.add(grantKey(grant.capability_uuid, grant.permission_code))
      }
    }
  } catch (e: any) {
    grantError.value = e?.message || t('agent.management.grants.loadFailed')
  } finally {
    grantLoading.value = false
  }
}

const resetGrantFilters = (agent: Agent) => {
  activeGrantAgent.value = agent
  grantSearch.value = ''
  grantCategoryTab.value = agentPluginID(agent) ? 'own' : 'core'
  grantPluginFilter.value = '__all__'
  grantAvailabilityFilter.value = 'all'
  grantSelectionFilter.value = 'all'
}

const agentPluginID = (agent?: Agent | null) => {
  if (!agent) return ''
  if (agent.ownerPluginId) return String(agent.ownerPluginId).trim()
  const source = String(agent.source || '').trim()
  if (source.toLowerCase().startsWith('plugin:')) return source.slice('plugin:'.length).trim()
  return ''
}

const isCoreCapabilityPlugin = (pluginID?: string) => {
  const normalized = String(pluginID || '').trim().toLowerCase()
  if (!normalized) return true
  return normalized === 'core' ||
    normalized === 'corex' ||
    normalized === 'powerx' ||
    normalized.startsWith('com.powerx.core') ||
    normalized.startsWith('com.corex.')
}

watch(categorizedGrantCapabilities, () => {
  if (grantPluginFilter.value !== '__all__' && !categorizedGrantCapabilities.value.some((item) => item.plugin_id === grantPluginFilter.value)) {
    grantPluginFilter.value = '__all__'
  }
})

const grantKey = (capabilityUUID: string, permissionCode: string) =>
  `${String(capabilityUUID || '').toLowerCase()}|${String(permissionCode || '').toLowerCase()}`

const setGrantSelected = (item: AgentGrantableCapability, selected: boolean) => {
  const key = grantKey(item.capability_uuid, item.permission_code)
  if (selected) selectedGrantKeys.add(key)
  else selectedGrantKeys.delete(key)
}

const grantPayload = () =>
  grantableCapabilities.value.map((item) => ({
    capability_uuid: item.capability_uuid,
    permission_code: item.permission_code,
    enabled: selectedGrantKeys.has(grantKey(item.capability_uuid, item.permission_code)),
  }))

onMounted(load)

const parseCommaValues = (raw: string): string[] => {
  if (!raw) return []
  const parts = raw.split(',').map((item) => item.trim()).filter(Boolean)
  return Array.from(new Set(parts))
}

const normalizeArrayText = (value: unknown): string => {
  if (!Array.isArray(value)) return ''
  return value.map((item) => String(item || '').trim()).filter(Boolean).join(',')
}
</script>
