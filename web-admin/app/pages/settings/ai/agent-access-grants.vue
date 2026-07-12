<template>
  <div class="space-y-6 p-4">
    <div class="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
      <div>
        <h1 class="text-lg font-semibold text-[var(--text-primary)]">{{ t("agent.accessGrants.title") }}</h1>
        <p class="text-sm text-[var(--text-secondary)]">{{ t("agent.accessGrants.description") }}</p>
      </div>
      <UButton variant="soft" icon="i-heroicons-arrow-left" :to="localePath('/settings/ai/agents')">
        {{ t("agent.accessGrants.backToAgents") }}
      </UButton>
    </div>

    <UAlert
      v-if="loadError"
      color="error"
      variant="soft"
      icon="i-heroicons-exclamation-triangle"
      :title="loadError"
    />

    <UAlert
      v-else-if="!loading && agents.length === 0"
      color="warning"
      variant="soft"
      icon="i-heroicons-exclamation-circle"
      :title="t('agent.accessGrants.emptyAgents')"
      :description="t('agent.accessGrants.emptyAgentsDescription')"
    />

    <section class="grid grid-cols-1 gap-4 xl:grid-cols-12">
      <div class="space-y-3 rounded-lg border border-[var(--border-color)] bg-[var(--bg-muted)]/20 p-4 xl:col-span-3">
        <div class="space-y-3">
          <div class="flex items-center justify-between gap-3">
            <h2 class="text-sm font-semibold text-[var(--text-primary)]">{{ t("agent.accessGrants.sections.agents") }}</h2>
            <UBadge size="xs" variant="soft" color="neutral">{{ agents.length }}</UBadge>
          </div>
          <USelectMenu
            v-model="selectedAgentUUID"
            :items="agentSelectItems"
            label-key="label"
            value-key="value"
            searchable
            class="w-full"
            icon="i-heroicons-cpu-chip"
            :loading="loading"
            :portal="false"
            :content="agentSelectContent"
            :ui="agentSelectUi"
            :placeholder="t('agent.accessGrants.placeholders.agent')"
            :search-input="{ placeholder: t('agent.accessGrants.placeholders.agentSearch') }"
            @update:model-value="onAgentSelect"
          />
        </div>

        <div v-if="selectedAgent" class="rounded-md border border-primary-500/50 bg-primary-500/10 p-3">
          <div class="text-xs text-[var(--text-secondary)]">{{ t("agent.accessGrants.currentAgent") }}</div>
          <div class="mt-1 truncate text-sm font-medium text-[var(--text-primary)]">{{ agentLabel(selectedAgent) }}</div>
          <div class="mt-2 flex items-center gap-2">
            <UBadge size="xs" variant="soft" :color="selectedAgent.managedByPlugin ? 'warning' : 'primary'">
              {{ selectedAgent.managedByPlugin ? t("agent.management.sources.plugin") : t("agent.management.sources.core") }}
            </UBadge>
            <span v-if="selectedAgent.ownerPluginId" class="truncate text-xs text-[var(--text-secondary)]">{{ selectedAgent.ownerPluginId }}</span>
          </div>
        </div>
        <div v-else class="rounded-md border border-dashed border-[var(--border-color)] p-4 text-sm text-[var(--text-secondary)]">
          <div class="font-medium text-[var(--text-primary)]">{{ t("agent.accessGrants.selectAgentTitle") }}</div>
          <div class="mt-1">{{ t("agent.accessGrants.typeToSearchAgent") }}</div>
        </div>
      </div>

      <div class="space-y-3 rounded-lg border border-[var(--border-color)] bg-[var(--bg-muted)]/20 p-4 xl:col-span-5">
        <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div class="space-y-1">
            <h2 class="text-sm font-semibold text-[var(--text-primary)]">{{ t("agent.accessGrants.sections.subjects") }}</h2>
            <div class="truncate text-xs text-[var(--text-secondary)]">
              {{ selectedAgent ? agentLabel(selectedAgent) : t("agent.accessGrants.emptyAgent") }}
            </div>
          </div>
          <div class="text-xs text-[var(--text-secondary)]">
            {{ t("agent.accessGrants.summary.enabled") }}: {{ enabledCount }} / {{ currentSubjects.length }}
          </div>
        </div>
        <div class="rounded-md border border-[var(--border-color)] bg-[var(--bg-elevated)] p-3 text-xs text-[var(--text-secondary)]">
          {{ activeTab === "member" ? t("agent.accessGrants.memberGrantHint") : t("agent.accessGrants.roleGrantHint") }}
        </div>
        <div class="grid grid-cols-1 gap-2 lg:grid-cols-12">
          <UTabs v-model="activeTab" :items="tabItems" class="lg:col-span-5" />
          <UInput
            v-model="search"
            class="lg:col-span-7"
            icon="i-heroicons-magnifying-glass"
            :placeholder="t('agent.accessGrants.placeholders.search')"
          />
        </div>

        <div class="h-[36rem] overflow-y-auto pr-1">
          <div v-if="!selectedAgentUUID" class="flex h-full items-center justify-center text-sm text-[var(--text-secondary)]">
            {{ t("agent.accessGrants.emptyAgent") }}
          </div>
          <div v-else-if="loading" class="flex h-full items-center justify-center text-sm text-[var(--text-secondary)]">
            {{ t("agent.accessGrants.loading") }}
          </div>
          <div v-else-if="filteredSubjects.length === 0" class="flex h-full items-center justify-center text-sm text-[var(--text-secondary)]">
            {{ t("agent.accessGrants.emptyFiltered") }}
          </div>
          <div v-else class="space-y-2">
            <button
              v-for="subject in filteredSubjects"
              :key="subject.uuid"
              type="button"
              class="w-full rounded-md border border-[var(--border-color)] bg-[var(--bg-elevated)] p-3 text-left transition hover:border-primary-400"
              :class="selectedSubjectUUID === subject.uuid ? 'border-primary-500 ring-1 ring-primary-500' : ''"
              @click="selectSubject(subject)"
            >
              <div class="flex items-center justify-between gap-3">
                <div class="min-w-0">
                  <div class="truncate text-sm font-medium text-[var(--text-primary)]">{{ subject.label }}</div>
                  <div class="truncate text-xs text-[var(--text-secondary)]">{{ subject.secondary || subject.typeLabel }}</div>
                </div>
                <USwitch
                  :model-value="isEnabled(subject.uuid)"
                  :loading="savingUUID === subject.uuid"
                  :aria-label="t('agent.accessGrants.actions.toggle')"
                  @click.stop
                  @update:model-value="(enabled) => toggleGrant(subject, Boolean(enabled))"
                />
              </div>
            </button>
          </div>
        </div>
      </div>

      <div class="space-y-3 rounded-lg border border-[var(--border-color)] bg-[var(--bg-muted)]/20 p-4 xl:col-span-4">
        <div class="flex flex-col gap-3 2xl:flex-row 2xl:items-start 2xl:justify-between">
          <div class="min-w-0">
            <h2 class="text-sm font-semibold text-[var(--text-primary)]">{{ t("agent.accessGrants.preview.title") }}</h2>
            <p class="text-xs text-[var(--text-secondary)]">{{ t("agent.accessGrants.preview.description") }}</p>
          </div>
          <UButton
            size="sm"
            variant="soft"
            icon="i-heroicons-arrow-path"
            class="w-fit shrink-0 whitespace-nowrap"
            :loading="previewLoading"
            :disabled="!canPreview"
            @click="loadPreview"
          >
            {{ t("agent.accessGrants.actions.preview") }}
          </UButton>
        </div>

        <div class="rounded-md border border-primary-500/30 bg-primary-500/10 p-3">
          <div class="text-xs font-medium text-[var(--text-primary)]">{{ t("agent.accessGrants.preview.formulaTitle") }}</div>
          <p class="mt-1 text-xs leading-5 text-[var(--text-secondary)]">
            {{ t("agent.accessGrants.preview.formulaDescription") }}
          </p>
          <div class="mt-3 grid grid-cols-2 gap-2 text-xs 2xl:grid-cols-5">
            <UBadge size="xs" variant="soft" color="neutral">{{ t("agent.accessGrants.preview.formula.user") }}</UBadge>
            <UBadge size="xs" variant="soft" color="neutral">{{ t("agent.accessGrants.preview.formula.useGrant") }}</UBadge>
            <UBadge size="xs" variant="soft" color="neutral">{{ t("agent.accessGrants.preview.formula.agent") }}</UBadge>
            <UBadge size="xs" variant="soft" color="neutral">{{ t("agent.accessGrants.preview.formula.tenant") }}</UBadge>
            <UBadge size="xs" variant="soft" color="neutral">{{ t("agent.accessGrants.preview.formula.policy") }}</UBadge>
          </div>
        </div>

        <UAlert
          v-if="previewResult && !previewResult.agent_access_allowed"
          color="warning"
          variant="soft"
          icon="i-heroicons-lock-closed"
          :title="t('agent.accessGrants.preview.accessDenied')"
        />

        <div v-if="selectedSubject" class="rounded-md border border-[var(--border-color)] bg-[var(--bg-elevated)] p-3">
          <div class="flex items-start justify-between gap-3">
            <div class="min-w-0">
              <div class="text-xs text-[var(--text-secondary)]">{{ t("agent.accessGrants.preview.subject") }}</div>
              <div class="mt-1 truncate text-sm font-medium text-[var(--text-primary)]">{{ selectedSubject.label }}</div>
              <div class="truncate text-xs text-[var(--text-secondary)]">{{ selectedSubject.secondary || selectedSubject.typeLabel }}</div>
            </div>
            <UBadge size="xs" variant="soft" :color="isEnabled(selectedSubject.uuid) ? 'success' : 'warning'">
              {{ isEnabled(selectedSubject.uuid) ? t("agent.accessGrants.preview.granted") : t("agent.accessGrants.preview.notGranted") }}
            </UBadge>
          </div>
        </div>

        <UAlert
          v-if="selectedSubject?.subjectType === 'role'"
          color="neutral"
          variant="soft"
          icon="i-heroicons-information-circle"
          :title="t('agent.accessGrants.preview.rolePreviewUnavailable')"
        />

        <div v-if="selectedSubject && selectedSubject.subjectType === 'member'" class="grid grid-cols-3 gap-2">
          <div class="rounded-md border border-[var(--border-color)] bg-[var(--bg-elevated)] p-3">
            <div class="text-xs text-[var(--text-secondary)]">{{ t("agent.accessGrants.preview.total") }}</div>
            <div class="mt-1 text-xl font-semibold text-[var(--text-primary)]">{{ previewSummary.total }}</div>
          </div>
          <div class="rounded-md border border-[var(--border-color)] bg-[var(--bg-elevated)] p-3">
            <div class="text-xs text-[var(--text-secondary)]">{{ t("agent.accessGrants.preview.allowed") }}</div>
            <div class="mt-1 text-xl font-semibold text-green-500">{{ previewSummary.allowed }}</div>
          </div>
          <div class="rounded-md border border-[var(--border-color)] bg-[var(--bg-elevated)] p-3">
            <div class="text-xs text-[var(--text-secondary)]">{{ t("agent.accessGrants.preview.denied") }}</div>
            <div class="mt-1 text-xl font-semibold text-amber-500">{{ previewSummary.denied }}</div>
          </div>
        </div>

        <div v-if="previewResult" class="rounded-md border border-[var(--border-color)] bg-[var(--bg-elevated)] p-3 text-xs leading-5 text-[var(--text-secondary)]">
          {{ previewInterpretation }}
        </div>

        <div v-if="denyReasonSummary.length > 0" class="rounded-md border border-amber-500/30 bg-amber-500/10 p-3">
          <div class="text-xs font-medium text-[var(--text-primary)]">{{ t("agent.accessGrants.preview.denySummaryTitle") }}</div>
          <div class="mt-2 space-y-1">
            <div
              v-for="reason in denyReasonSummary"
              :key="reason.reason"
              class="flex items-center justify-between gap-3 text-xs"
            >
              <span class="min-w-0 break-words text-[var(--text-secondary)]">{{ reasonLabel(reason.reason) }}</span>
              <span class="shrink-0 font-semibold text-amber-500">{{ reason.count }}</span>
            </div>
          </div>
        </div>

        <UTabs
          v-if="previewResult"
          v-model="previewFilter"
          :items="previewFilterItems"
        />

        <div v-if="previewResult" class="grid grid-cols-1 gap-2 2xl:grid-cols-12">
          <UInput
            v-model="previewSearch"
            class="min-w-0 2xl:col-span-8"
            icon="i-heroicons-magnifying-glass"
            :placeholder="t('agent.accessGrants.preview.searchPlaceholder')"
          />
          <USelect
            v-model="previewPageSize"
            class="min-w-28 2xl:col-span-4"
            :items="previewPageSizeItems"
            :aria-label="t('agent.accessGrants.preview.pageSize')"
          />
        </div>

        <div class="max-h-[27rem] min-h-40 overflow-y-auto pr-1">
          <div v-if="previewLoading" class="flex min-h-40 items-center justify-center text-sm text-[var(--text-secondary)]">
            {{ t("agent.accessGrants.preview.loading") }}
          </div>
          <div v-else-if="!selectedSubject" class="flex min-h-40 flex-col items-center justify-center gap-3 rounded-md border border-dashed border-[var(--border-color)] px-6 py-6 text-center">
            <UIcon name="i-heroicons-user-circle" class="size-10 text-[var(--text-secondary)]" />
            <div>
              <div class="text-sm font-medium text-[var(--text-primary)]">{{ t("agent.accessGrants.preview.selectMemberTitle") }}</div>
              <div class="mt-1 text-sm text-[var(--text-secondary)]">{{ t("agent.accessGrants.preview.selectMemberDescription") }}</div>
            </div>
          </div>
          <div v-else-if="selectedSubject.subjectType === 'role'" class="flex min-h-40 flex-col items-center justify-center gap-3 rounded-md border border-dashed border-[var(--border-color)] px-6 py-6 text-center">
            <UIcon name="i-heroicons-user-group" class="size-10 text-[var(--text-secondary)]" />
            <div>
              <div class="text-sm font-medium text-[var(--text-primary)]">{{ t("agent.accessGrants.preview.roleSelectedTitle") }}</div>
              <div class="mt-1 text-sm text-[var(--text-secondary)]">{{ t("agent.accessGrants.preview.roleSelectedDescription") }}</div>
            </div>
          </div>
          <div v-else-if="!previewResult" class="flex min-h-40 items-center justify-center rounded-md border border-dashed border-[var(--border-color)] text-sm text-[var(--text-secondary)]">
            {{ t("agent.accessGrants.preview.empty") }}
          </div>
          <div v-else-if="pagedPreviewItems.length === 0" class="flex min-h-40 flex-col items-center justify-center gap-3 rounded-md border border-dashed border-[var(--border-color)] px-4 py-6 text-center text-sm text-[var(--text-secondary)]">
            <UIcon name="i-heroicons-lock-closed" class="size-9 text-[var(--text-secondary)]" />
            <div class="font-medium text-[var(--text-primary)]">{{ previewEmptyTitle }}</div>
            <div class="max-w-sm">{{ previewEmptyDescription }}</div>
            <UButton
              v-if="previewFilter === 'allowed' && previewSummary.denied > 0"
              size="sm"
              variant="soft"
              color="warning"
              icon="i-heroicons-list-bullet"
              @click="previewFilter = 'denied'"
            >
              {{ t("agent.accessGrants.preview.actions.viewDeniedDetails") }}
            </UButton>
          </div>
          <div v-else class="space-y-2">
            <div
              v-for="item in pagedPreviewItems"
              :key="item.capability_uuid + item.permission_code"
              class="rounded-md border border-[var(--border-color)] bg-[var(--bg-elevated)] p-3"
            >
              <div class="flex items-start justify-between gap-3">
                <div class="min-w-0">
                  <div class="truncate text-sm font-medium text-[var(--text-primary)]">{{ item.display_name || item.capability_id }}</div>
                  <div class="break-all text-xs text-[var(--text-secondary)]">{{ item.permission_code }}</div>
                </div>
                <UBadge size="xs" variant="soft" :color="item.effective_allowed ? 'success' : 'warning'">
                  {{ item.effective_allowed ? t("agent.accessGrants.status.allowed") : t("agent.accessGrants.status.denied") }}
                </UBadge>
              </div>
              <div class="mt-3 grid grid-cols-2 gap-2 text-xs 2xl:grid-cols-4">
                <div class="rounded border border-[var(--border-color)] px-2 py-1">
                  <span class="text-[var(--text-secondary)]">{{ t("agent.accessGrants.preview.columns.user") }}</span>
                  <span class="ml-1 font-medium" :class="item.user_allowed ? 'text-green-500' : 'text-amber-500'">
                    {{ item.user_allowed ? t("agent.accessGrants.status.allowed") : t("agent.accessGrants.status.denied") }}
                  </span>
                </div>
                <div class="rounded border border-[var(--border-color)] px-2 py-1">
                  <span class="text-[var(--text-secondary)]">{{ t("agent.accessGrants.preview.columns.agent") }}</span>
                  <span class="ml-1 font-medium" :class="item.agent_allowed ? 'text-green-500' : 'text-amber-500'">
                    {{ item.agent_allowed ? t("agent.accessGrants.status.allowed") : t("agent.accessGrants.status.denied") }}
                  </span>
                </div>
                <div class="rounded border border-[var(--border-color)] px-2 py-1">
                  <span class="text-[var(--text-secondary)]">{{ t("agent.accessGrants.preview.columns.tenant") }}</span>
                  <span class="ml-1 font-medium" :class="item.tenant_enabled ? 'text-green-500' : 'text-amber-500'">
                    {{ item.tenant_enabled ? t("agent.accessGrants.status.allowed") : t("agent.accessGrants.status.denied") }}
                  </span>
                </div>
                <div class="rounded border border-[var(--border-color)] px-2 py-1">
                  <span class="text-[var(--text-secondary)]">{{ t("agent.accessGrants.preview.columns.policy") }}</span>
                  <span class="ml-1 font-medium" :class="item.policy_allowed ? 'text-green-500' : 'text-amber-500'">
                    {{ item.policy_allowed ? t("agent.accessGrants.status.allowed") : t("agent.accessGrants.status.denied") }}
                  </span>
                </div>
              </div>
              <div v-if="!item.effective_allowed" class="mt-2 text-xs text-amber-500">
                {{ reasonLabel(item.deny_reason) }}
              </div>
            </div>
          </div>
        </div>

        <div v-if="previewResult && filteredPreviewItems.length > previewPageSize" class="space-y-2 border-t border-[var(--border-color)] pt-3">
          <div class="text-xs leading-5 text-[var(--text-secondary)]">
            {{ t("agent.accessGrants.preview.pageSummary", {
              shown: pagedPreviewItems.length,
              total: filteredPreviewItems.length
            }) }}
          </div>
          <div class="flex min-w-0 items-center justify-between gap-2">
            <div class="shrink-0 text-xs font-medium text-[var(--text-secondary)]">
              {{ previewPage }} / {{ previewTotalPages }}
            </div>
            <div class="flex shrink-0 items-center gap-1">
              <UButton
                size="xs"
                variant="soft"
                color="neutral"
                icon="i-heroicons-chevron-double-left"
                :aria-label="t('agent.accessGrants.preview.pagination.first')"
                :disabled="previewPage <= 1"
                @click="previewPage = 1"
              />
              <UButton
                size="xs"
                variant="soft"
                color="neutral"
                icon="i-heroicons-chevron-left"
                :aria-label="t('agent.accessGrants.preview.pagination.previous')"
                :disabled="previewPage <= 1"
                @click="previewPage = Math.max(1, previewPage - 1)"
              />
              <UButton
                size="xs"
                variant="soft"
                color="neutral"
                icon="i-heroicons-chevron-right"
                :aria-label="t('agent.accessGrants.preview.pagination.next')"
                :disabled="previewPage >= previewTotalPages"
                @click="previewPage = Math.min(previewTotalPages, previewPage + 1)"
              />
              <UButton
                size="xs"
                variant="soft"
                color="neutral"
                icon="i-heroicons-chevron-double-right"
                :aria-label="t('agent.accessGrants.preview.pagination.last')"
                :disabled="previewPage >= previewTotalPages"
                @click="previewPage = previewTotalPages"
              />
            </div>
          </div>
        </div>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import type { Agent, AgentAccessGrant, AgentAccessSubjectType, AgentEffectivePermissions } from "~/types/agent";
import { useAgentManager } from "~/composables/agent/useAgentManager";
import { useMemberService } from "~/composables/api/services/memberService";
import { useRoleService, type Role } from "~/composables/api/services/roleService";

definePageMeta({
  title: "agent.accessGrants.title",
  icon: "i-heroicons-shield-check",
  order: 37,
});

type SubjectRow = {
  uuid: string;
  label: string;
  secondary: string;
  subjectType: AgentAccessSubjectType;
  typeLabel: string;
};

const { t, locale } = useI18n();
const localePath = useLocalePath();
const route = useRoute();
const agentManager = useAgentManager();
const memberService = useMemberService();
const roleService = useRoleService();

const agents = ref<Agent[]>([]);
const members = ref<any[]>([]);
const roles = ref<Role[]>([]);
const grants = ref<AgentAccessGrant[]>([]);
const selectedAgentUUID = ref(String(route.query.agent_uuid || ""));
const selectedSubjectUUID = ref("");
const activeTab = ref<AgentAccessSubjectType>("member");
const search = ref("");
const loading = ref(false);
const savingUUID = ref("");
const loadError = ref("");
const previewLoading = ref(false);
const previewResult = ref<AgentEffectivePermissions | null>(null);
const previewFilter = ref<"allowed" | "denied" | "all">("allowed");
const previewSearch = ref("");
const previewPage = ref(1);
const previewPageSize = ref(10);
const previewPageSizeItems = [10, 25, 50];

const agentSelectContent = {
  side: "bottom" as const,
  sideOffset: 10,
  collisionPadding: 12,
  position: "popper" as const,
};

const agentSelectUi = {
  base: "h-10 bg-[#0f172a] ring-1 ring-[#334155] text-white hover:bg-[#111c30] focus-visible:ring-2 focus-visible:ring-primary",
  leadingIcon: "text-white",
  trailingIcon: "text-white",
  value: "text-white truncate",
  placeholder: "text-slate-400 truncate",
  content:
    "z-50 min-w-[var(--reka-combobox-trigger-width)] bg-[#0f172a] border border-[#334155] ring-0 shadow-xl rounded-md overflow-hidden",
  input:
    "border-b border-[#334155] bg-[#111c30] text-white placeholder:text-slate-500 [&_input]:bg-[#111c30] [&_input]:text-white [&_input::placeholder]:text-slate-500",
  viewport: "max-h-72 overflow-y-auto divide-y-0 py-1",
  group: "p-1",
  item:
    "text-slate-100 data-highlighted:not-data-disabled:text-white data-highlighted:not-data-disabled:before:bg-[#1f2f46]",
  itemLabel: "truncate",
  itemLeadingIcon: "text-slate-300",
  itemTrailingIcon: "text-white",
  empty: "px-3 py-3 text-sm text-slate-400",
};

const localizedText = (values?: Record<string, string>) => {
  if (!values) return "";
  const current = String(locale.value || "").toLowerCase();
  return values[current] || values[current.split("-")[0]] || values.zh || values.en || "";
};

const agentLabel = (agent: Agent) =>
  localizedText(agent.title_i18n) || agent.name || agent.key || t("agent.accessGrants.unnamedAgent");

const selectedAgent = computed(() =>
  agents.value.find((agent) => agent.uuid === selectedAgentUUID.value) || null
);

const agentSelectItems = computed(() =>
  agents.value.map((agent) => ({
    label: agentLabel(agent),
    value: agent.uuid,
  }))
);

const tabItems = computed(() => [
  { label: t("agent.accessGrants.tabs.members"), value: "member" },
  { label: t("agent.accessGrants.tabs.roles"), value: "role" },
]);

const normalizeMember = (item: any): SubjectRow | null => {
  const member = item?.Member || item?.member || item;
  const user = item?.User || item?.user || null;
  const uuid = String(member?.uuid || item?.uuid || "");
  if (!uuid) return null;
  const label =
    member?.display_name ||
    member?.name ||
    user?.display_name ||
    member?.username ||
    user?.email ||
    t("agent.accessGrants.unnamedMember");
  return {
    uuid,
    label,
    secondary: user?.email || user?.phone || member?.username || "",
    subjectType: "member",
    typeLabel: t("agent.accessGrants.tabs.members"),
  };
};

const normalizeRole = (role: Role): SubjectRow | null => {
  const uuid = String(role?.uuid || "");
  if (!uuid) return null;
  return {
    uuid,
    label: role.name || role.code || t("agent.accessGrants.unnamedRole"),
    secondary: role.description || role.code || "",
    subjectType: "role",
    typeLabel: t("agent.accessGrants.tabs.roles"),
  };
};

const memberSubjects = computed(() => members.value.map(normalizeMember).filter(Boolean) as SubjectRow[]);
const roleSubjects = computed(() => roles.value.map(normalizeRole).filter(Boolean) as SubjectRow[]);
const currentSubjects = computed(() => (activeTab.value === "member" ? memberSubjects.value : roleSubjects.value));

const filteredSubjects = computed(() => {
  const keyword = search.value.trim().toLowerCase();
  if (!keyword) return currentSubjects.value;
  return currentSubjects.value.filter((subject) =>
    [subject.label, subject.secondary].some((value) => String(value || "").toLowerCase().includes(keyword))
  );
});

const grantMap = computed(() => {
  const out = new Map<string, AgentAccessGrant>();
  for (const grant of grants.value) {
    out.set(`${grant.subject_type}:${grant.subject_uuid}`, grant);
  }
  return out;
});

const isEnabled = (subjectUUID: string) =>
  grantMap.value.get(`${activeTab.value}:${subjectUUID}`)?.status === "enabled";

const enabledCount = computed(() =>
  currentSubjects.value.filter((subject) => isEnabled(subject.uuid)).length
);

const selectedSubject = computed(() =>
  currentSubjects.value.find((subject) => subject.uuid === selectedSubjectUUID.value) || null
);

const canPreview = computed(() =>
  Boolean(selectedAgentUUID.value && selectedSubject.value?.subjectType === "member")
);

const previewSummary = computed(() => {
  const items = previewResult.value?.items || [];
  const allowed = items.filter((item) => item.effective_allowed).length;
  return { total: items.length, allowed, denied: items.length - allowed };
});

const previewFilterItems = computed(() => [
  { label: t("agent.accessGrants.preview.filters.allowed"), value: "allowed" },
  { label: t("agent.accessGrants.preview.filters.denied"), value: "denied" },
  { label: t("agent.accessGrants.preview.filters.all"), value: "all" },
]);

const filteredPreviewItems = computed(() => {
  const allItems = previewResult.value?.items || [];
  const byStatus = previewFilter.value === "allowed"
    ? allItems.filter((item) => item.effective_allowed)
    : previewFilter.value === "denied"
      ? allItems.filter((item) => !item.effective_allowed)
      : allItems;
  const keyword = previewSearch.value.trim().toLowerCase();
  if (!keyword) return byStatus;
  return byStatus.filter((item) => [
    item.display_name,
    item.capability_id,
    item.plugin_id,
    item.permission_code,
    reasonLabel(item.deny_reason),
  ].some((value) => String(value || "").toLowerCase().includes(keyword)));
});

const pagedPreviewItems = computed(() => {
  const start = (previewPage.value - 1) * previewPageSize.value;
  return filteredPreviewItems.value.slice(start, start + previewPageSize.value);
});

const previewTotalPages = computed(() =>
  Math.max(1, Math.ceil(filteredPreviewItems.value.length / previewPageSize.value))
);

const previewInterpretation = computed(() => t("agent.accessGrants.preview.interpretation", {
  total: previewSummary.value.total,
  allowed: previewSummary.value.allowed,
  denied: previewSummary.value.denied,
}));

const denyReasonSummary = computed(() => {
  const counts = new Map<string, number>();
  for (const item of previewResult.value?.items || []) {
    if (item.effective_allowed) continue;
    const reason = String(item.deny_reason || "none").trim() || "none";
    counts.set(reason, (counts.get(reason) || 0) + 1);
  }
  return Array.from(counts.entries())
    .map(([reason, count]) => ({ reason, count }))
    .sort((a, b) => b.count - a.count)
    .slice(0, 4);
});

const previewEmptyTitle = computed(() => {
  if (previewFilter.value === "allowed") return t("agent.accessGrants.preview.emptyAllowedTitle");
  if (previewFilter.value === "denied") return t("agent.accessGrants.preview.emptyDeniedTitle");
  return t("agent.accessGrants.preview.emptyItemsTitle");
});

const previewEmptyDescription = computed(() => {
  if (previewFilter.value === "allowed") return t("agent.accessGrants.preview.emptyAllowedDescription");
  if (previewFilter.value === "denied") return t("agent.accessGrants.preview.emptyDeniedDescription");
  return t("agent.accessGrants.preview.emptyItems");
});

watch(activeTab, () => {
  selectedSubjectUUID.value = "";
  previewResult.value = null;
  previewFilter.value = "allowed";
  previewSearch.value = "";
  previewPage.value = 1;
});

watch([previewFilter, previewSearch, previewPageSize], () => {
  previewPage.value = 1;
});

watch(filteredPreviewItems, (items) => {
  if (previewPage.value > previewTotalPages.value) {
    previewPage.value = previewTotalPages.value;
  }
});

const reloadGrants = async () => {
  previewResult.value = null;
  previewFilter.value = "allowed";
  selectedSubjectUUID.value = "";
  if (!selectedAgentUUID.value) {
    grants.value = [];
    return;
  }
  loading.value = true;
  loadError.value = "";
  try {
    grants.value = await agentManager.fetchAgentAccessGrants(selectedAgentUUID.value);
  } catch (error: any) {
    loadError.value = error?.message || t("agent.accessGrants.errors.loadFailed");
  } finally {
    loading.value = false;
  }
};

const onAgentSelect = async () => {
  await reloadGrants();
};

const loadOptions = async () => {
  loading.value = true;
  loadError.value = "";
  try {
    await agentManager.fetchAgents();
    agents.value = [...agentManager.agents.value];
    members.value = await memberService.getMemberList({ page: 1, page_size: 100, sort_by: "created_at", sort_order: "asc" });
    const roleResp = await roleService.getRoles({ page: 1, page_size: 100, scope: "tenant" });
    roles.value = roleResp?.data?.items || [];
    if (selectedAgentUUID.value) {
      grants.value = await agentManager.fetchAgentAccessGrants(selectedAgentUUID.value);
    }
  } catch (error: any) {
    loadError.value = error?.message || t("agent.accessGrants.errors.loadFailed");
  } finally {
    loading.value = false;
  }
};

const selectSubject = async (subject: SubjectRow) => {
  selectedSubjectUUID.value = subject.uuid;
  if (subject.subjectType !== "member") {
    previewResult.value = null;
    return;
  }
  await loadPreview();
};

const toggleGrant = async (subject: SubjectRow, enabled: boolean) => {
  if (!selectedAgentUUID.value) return;
  selectedSubjectUUID.value = subject.uuid;
  if (subject.subjectType !== "member") {
    previewResult.value = null;
  }
  savingUUID.value = subject.uuid;
  loadError.value = "";
  try {
    grants.value = await agentManager.updateAgentAccessGrants(selectedAgentUUID.value, [{
      subject_type: subject.subjectType,
      subject_uuid: subject.uuid,
      enabled,
    }]);
    if (subject.subjectType === "member") {
      await loadPreview();
    }
  } catch (error: any) {
    loadError.value = error?.message || t("agent.accessGrants.errors.saveFailed");
  } finally {
    savingUUID.value = "";
  }
};

const loadPreview = async () => {
  if (!selectedAgentUUID.value || !selectedSubject.value || selectedSubject.value.subjectType !== "member") return;
  previewLoading.value = true;
  loadError.value = "";
  try {
    previewResult.value = await agentManager.fetchEffectivePermissions(selectedAgentUUID.value, selectedSubject.value.uuid);
    previewFilter.value = "allowed";
    previewSearch.value = "";
    previewPage.value = 1;
  } catch (error: any) {
    previewResult.value = null;
    loadError.value = error?.message || t("agent.accessGrants.errors.previewFailed");
  } finally {
    previewLoading.value = false;
  }
};

const reasonLabel = (reason?: string) => {
  const key = String(reason || "none").trim() || "none";
  const mapped = `agent.accessGrants.reasons.${key}`;
  const value = t(mapped);
  return value === mapped ? key : value;
};

onMounted(loadOptions);
</script>
