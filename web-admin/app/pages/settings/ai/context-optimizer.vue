<template>
  <div class="space-y-6 p-4">
    <div
      class="flex flex-col gap-4 md:flex-row md:items-center md:justify-between"
    >
      <div>
        <h1 class="text-lg font-semibold text-[var(--text-primary)]">
          {{ t("settings.ai.contextOptimizer.title") }}
        </h1>
        <p class="text-sm text-[var(--text-secondary)]">
          {{ t("settings.ai.contextOptimizer.description") }}
        </p>
      </div>
      <div class="flex flex-wrap gap-2">
        <UButton
          variant="ghost"
          icon="i-heroicons-arrow-left"
          :to="modelSettingsLink"
        >
          {{ t("settings.ai.actions.backToModels") }}
        </UButton>
        <UButton
          color="neutral"
          variant="soft"
          icon="i-heroicons-arrow-path"
          :loading="loading"
          @click="reloadAll"
        >
          {{ t("common.refresh") }}
        </UButton>
      </div>
    </div>

    <UCard>
      <template #header>
        <div class="flex flex-col gap-3 md:flex-row md:items-end md:justify-between">
          <div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
            <div>
              <div class="mb-1 text-xs text-[var(--text-secondary)]">{{ t("settings.ai.environment") }}</div>
              <USelect v-model="env" :items="envOptions" class="w-44" />
            </div>
            <div>
              <div class="mb-1 text-xs text-[var(--text-secondary)]">{{ t("settings.ai.contextOptimizer.scope") }}</div>
              <USelect v-model="scope" :items="scopeOptions" class="w-36" />
            </div>
            <div>
              <div class="mb-1 text-xs text-[var(--text-secondary)]">{{ t("settings.ai.contextOptimizer.activeSource") }}</div>
              <UBadge :color="activeSourceColor" variant="soft">
                {{ activeSourceText }}
              </UBadge>
            </div>
          </div>
          <div class="text-xs text-[var(--text-secondary)]">
            {{ t("settings.ai.contextOptimizer.activeVersion", { version: activeVersion }) }} · {{ t("settings.ai.contextOptimizer.lastUpdated") }}：{{ activeUpdatedAtText }}
          </div>
        </div>
      </template>

      <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
        <UCheckbox v-model="form.enabled" :label="t('settings.ai.contextOptimizer.enableOptimizer')" />
        <UCheckbox v-model="form.debug_trace_enabled" :label="t('settings.ai.contextOptimizer.enableDebugTrace')" />

        <UFormField :label="t('settings.ai.contextOptimizer.fields.maxPromptTokens')">
          <UInput v-model.number="form.max_prompt_tokens" type="number" />
        </UFormField>
        <UFormField :label="t('settings.ai.contextOptimizer.fields.reservedCompletionTokens')">
          <UInput v-model.number="form.reserved_completion_tokens" type="number" />
        </UFormField>

        <UFormField :label="t('settings.ai.contextOptimizer.fields.recentMessages')">
          <UInput v-model.number="form.recent_messages" type="number" />
        </UFormField>
        <UFormField :label="t('settings.ai.contextOptimizer.fields.retrievalTopK')">
          <UInput v-model.number="form.retrieval_top_k" type="number" />
        </UFormField>

        <UFormField :label="t('settings.ai.contextOptimizer.fields.summaryRefreshIntervalSec')">
          <UInput v-model.number="form.summary_refresh_interval_sec" type="number" />
        </UFormField>
        <UFormField :label="t('settings.ai.contextOptimizer.fields.cacheMode')">
          <USelect
            v-model="form.cache_mode"
            :items="cacheModeOptions"
            class="w-40"
            :ui="{ content: 'min-w-[140px]' }"
          />
        </UFormField>

        <div class="col-span-1 md:col-span-2 mt-2 border-t border-[var(--border-color)] pt-3">
          <div class="text-sm font-medium text-[var(--text-primary)]">
            {{ t("settings.ai.contextOptimizer.sections.plannerOptimizer") }}
          </div>
        </div>

        <UCheckbox v-model="form.planner_enabled" :label="t('settings.ai.contextOptimizer.fields.plannerEnabled')" />
        <UCheckbox v-model="form.planner_decision_cache_enabled" :label="t('settings.ai.contextOptimizer.fields.plannerDecisionCacheEnabled')" />

        <UFormField :label="t('settings.ai.contextOptimizer.fields.plannerCandidateTopK')">
          <UInput v-model.number="form.planner_candidate_top_k" type="number" />
        </UFormField>
        <UFormField :label="t('settings.ai.contextOptimizer.fields.plannerDecisionCacheTtlSec')">
          <UInput v-model.number="form.planner_decision_cache_ttl_sec" type="number" />
        </UFormField>

        <UFormField :label="t('settings.ai.contextOptimizer.fields.plannerPromptSlimMode')">
          <USelect
            v-model="form.planner_prompt_slim_mode"
            :items="plannerPromptSlimOptions"
            class="w-40"
            :ui="{ content: 'min-w-[140px]' }"
          />
        </UFormField>
        <div />

        <UFormField :label="t('settings.ai.contextOptimizer.fields.plannerQuotaWorkflow')">
          <UInput v-model.number="form.planner_quota_workflow" type="number" />
        </UFormField>
        <UFormField :label="t('settings.ai.contextOptimizer.fields.plannerQuotaSkill')">
          <UInput v-model.number="form.planner_quota_skill" type="number" />
        </UFormField>
        <UFormField :label="t('settings.ai.contextOptimizer.fields.plannerQuotaTooling')">
          <UInput v-model.number="form.planner_quota_tooling" type="number" />
        </UFormField>
        <UFormField :label="t('settings.ai.contextOptimizer.fields.plannerQuotaLlm')">
          <UInput v-model.number="form.planner_quota_llm" type="number" />
        </UFormField>
      </div>

      <template #footer>
        <div class="flex flex-wrap items-center gap-2">
          <UButton color="primary" :loading="savingDraft" @click="saveDraft">
            {{ t("settings.ai.contextOptimizer.actions.saveDraft") }}
          </UButton>
          <UButton
            color="primary"
            variant="soft"
            :disabled="!selectedVersion"
            :loading="publishing"
            @click="publishSelected"
          >
            {{ t("settings.ai.contextOptimizer.actions.publishSelected") }}
          </UButton>
          <UButton
            color="warning"
            variant="soft"
            :disabled="!selectedVersion"
            :loading="rollingBack"
            @click="rollbackSelected"
          >
            {{ t("settings.ai.contextOptimizer.actions.rollbackSelected") }}
          </UButton>
        </div>
      </template>
    </UCard>

    <UCard>
      <template #header>
        <div class="flex items-center justify-between">
          <div class="font-medium text-[var(--text-primary)]">{{ t("settings.ai.contextOptimizer.versionHistory") }}</div>
          <div class="text-xs text-[var(--text-secondary)]">{{ t("settings.ai.contextOptimizer.versionHint") }}</div>
        </div>
      </template>

      <div class="space-y-2">
        <button
          v-for="item in versions"
          :key="item.uuid"
          class="w-full rounded-md border px-3 py-2 text-left transition-colors"
          :class="selectedVersion === item.version ? 'border-primary-500 bg-primary-50/30' : 'border-[var(--border-color)] hover:bg-[var(--hover-bg)]'"
          @click="selectedVersion = item.version"
        >
          <div class="flex items-center justify-between gap-3">
            <div class="flex items-center gap-2">
              <span class="font-medium">v{{ item.version }}</span>
              <UBadge :color="item.status === 'published' ? 'success' : 'neutral'" variant="soft">
                {{ statusText(item.status) }}
              </UBadge>
            </div>
            <span class="text-xs text-[var(--text-secondary)]">{{ formatTime(item.updated_at) }}</span>
          </div>
          <div class="mt-1 text-xs text-[var(--text-secondary)]">
            {{ item.change_reason || t("settings.ai.contextOptimizer.noChangeReason") }}
          </div>
        </button>
        <div v-if="!versions.length" class="text-sm text-[var(--text-secondary)]">
          {{ t("settings.ai.contextOptimizer.noVersions") }}
        </div>
      </div>
    </UCard>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from "vue";
import { storeToRefs } from "pinia";
import { ENV_OPTIONS, useEnvStore } from "~/stores/envStore";
import { useUserStore } from "~/stores/user";
import {
  AISettingService,
  type ContextOptimizerConfig,
  type ContextOptimizerVersionItem,
} from "~/composables/api/services/aiSettingService";

const toast = useToast();
const localePath = useLocalePath();
const { t } = useI18n({ useScope: "global" });
const envStore = useEnvStore();
const userStore = useUserStore();
const { isRoot } = storeToRefs(userStore);

const modelSettingsLink = computed(() => localePath("/settings/ai"));
const envOptions = computed(() =>
  ENV_OPTIONS.map((option) => ({
    label: option.label,
    value: option.value,
  }))
);
const cacheModeOptions = [
  { label: t("settings.ai.contextOptimizer.cacheModes.auto"), value: "auto" },
  { label: t("settings.ai.contextOptimizer.cacheModes.forceOn"), value: "force_on" },
  { label: t("settings.ai.contextOptimizer.cacheModes.forceOff"), value: "force_off" },
];
const plannerPromptSlimOptions = [
  { label: t("settings.ai.contextOptimizer.plannerPromptSlimModes.compact"), value: "compact" },
  { label: t("settings.ai.contextOptimizer.plannerPromptSlimModes.verbose"), value: "verbose" },
];
const scopeOptions = computed(() => {
  const base = [{ label: t("settings.ai.contextOptimizer.scopeTenant"), value: "tenant" }];
  if (isRoot.value) {
    base.push({ label: t("settings.ai.contextOptimizer.scopeSystem"), value: "system" });
  }
  return base;
});

const env = ref(envStore.currentEnv || "dev");
const scope = ref<"tenant" | "system">("tenant");
const loading = ref(false);
const savingDraft = ref(false);
const publishing = ref(false);
const rollingBack = ref(false);
const selectedVersion = ref<number | null>(null);
const versions = ref<ContextOptimizerVersionItem[]>([]);
const activeVersion = ref(0);
const activeSource = ref("yaml_default");
const activeUpdatedAt = ref("");

const form = reactive<ContextOptimizerConfig>({
  enabled: true,
  max_prompt_tokens: 12000,
  reserved_completion_tokens: 1200,
  recent_messages: 8,
  retrieval_top_k: 6,
  cache_mode: "auto",
  summary_refresh_interval_sec: 300,
  debug_trace_enabled: false,
  planner_enabled: true,
  planner_candidate_top_k: 32,
  planner_prompt_slim_mode: "compact",
  planner_decision_cache_enabled: true,
  planner_decision_cache_ttl_sec: 60,
  planner_quota_workflow: 8,
  planner_quota_skill: 16,
  planner_quota_tooling: 16,
  planner_quota_llm: 8,
});

const activeSourceColor = computed(() => {
  if (activeSource.value === "tenant") return "primary";
  if (activeSource.value === "system") return "warning";
  return "neutral";
});
const activeSourceText = computed(() => {
  if (activeSource.value === "tenant") return t("settings.ai.contextOptimizer.sources.tenant");
  if (activeSource.value === "system") return t("settings.ai.contextOptimizer.sources.system");
  if (activeSource.value === "yaml_default") return t("settings.ai.contextOptimizer.sources.yamlDefault");
  return activeSource.value || t("settings.ai.contextOptimizer.sources.unknown");
});
const activeUpdatedAtText = computed(() =>
  activeUpdatedAt.value ? formatTime(activeUpdatedAt.value) : "-"
);

function formatTime(v?: string) {
  if (!v) return "-";
  const d = new Date(v);
  if (Number.isNaN(d.getTime())) return v;
  return d.toLocaleString();
}

function statusText(status?: string) {
  if (status === "published") return t("settings.ai.contextOptimizer.status.published");
  if (status === "draft") return t("settings.ai.contextOptimizer.status.draft");
  return status || "-";
}

function applyForm(cfg?: ContextOptimizerConfig) {
  if (!cfg) return;
  form.enabled = Boolean(cfg.enabled);
  form.max_prompt_tokens = Number(cfg.max_prompt_tokens || 12000);
  form.reserved_completion_tokens = Number(cfg.reserved_completion_tokens || 1200);
  form.recent_messages = Number(cfg.recent_messages || 8);
  form.retrieval_top_k = Number(cfg.retrieval_top_k || 6);
  form.cache_mode = (cfg.cache_mode || "auto") as any;
  form.summary_refresh_interval_sec = Number(cfg.summary_refresh_interval_sec || 300);
  form.debug_trace_enabled = Boolean(cfg.debug_trace_enabled);
  form.planner_enabled = Boolean(cfg.planner_enabled ?? true);
  form.planner_candidate_top_k = Number(cfg.planner_candidate_top_k || 32);
  form.planner_prompt_slim_mode = (cfg.planner_prompt_slim_mode || "compact") as any;
  form.planner_decision_cache_enabled = Boolean(cfg.planner_decision_cache_enabled ?? true);
  form.planner_decision_cache_ttl_sec = Number(cfg.planner_decision_cache_ttl_sec || 60);
  form.planner_quota_workflow = Number(cfg.planner_quota_workflow || 8);
  form.planner_quota_skill = Number(cfg.planner_quota_skill || 16);
  form.planner_quota_tooling = Number(cfg.planner_quota_tooling || 16);
  form.planner_quota_llm = Number(cfg.planner_quota_llm || 8);
}

async function loadActive() {
  const res = await AISettingService.getContextOptimizerActive({
    env: env.value,
    scope: scope.value,
  });
  activeSource.value = String(res.active?.source || "yaml_default");
  activeVersion.value = Number(res.active?.version || 0);
  activeUpdatedAt.value = String(res.active?.updated_at || "");
  applyForm(res.active?.config);
}

async function loadVersions() {
  const res = await AISettingService.listContextOptimizerVersions({
    env: env.value,
    scope: scope.value,
    limit: 100,
  });
  versions.value = Array.isArray(res.versions) ? res.versions : [];
  if (!selectedVersion.value && versions.value.length) {
    selectedVersion.value = versions.value[0].version;
  }
}

async function reloadAll() {
  loading.value = true;
  try {
    await Promise.all([loadActive(), loadVersions()]);
  } catch (err: any) {
    toast.add({
      title: t("settings.ai.contextOptimizer.errors.loadFailed"),
      description: String(err?.message || err),
      color: "error",
    });
  } finally {
    loading.value = false;
  }
}

async function saveDraft() {
  savingDraft.value = true;
  try {
    const res = await AISettingService.saveContextOptimizerDraft({
      env: env.value,
      scope: scope.value,
      config: { ...form },
      change_reason: "UI 保存草稿",
    });
    selectedVersion.value = res?.draft?.version || selectedVersion.value;
    toast.add({ title: t("settings.ai.contextOptimizer.toasts.draftSaved"), color: "success" });
    await reloadAll();
  } catch (err: any) {
    toast.add({
      title: t("settings.ai.contextOptimizer.errors.saveFailed"),
      description: String(err?.message || err),
      color: "error",
    });
  } finally {
    savingDraft.value = false;
  }
}

async function publishSelected() {
  if (!selectedVersion.value) return;
  publishing.value = true;
  try {
    await AISettingService.publishContextOptimizer({
      env: env.value,
      scope: scope.value,
      version: selectedVersion.value,
      change_reason: "UI 发布版本",
    });
    toast.add({ title: t("settings.ai.contextOptimizer.toasts.publishSuccess"), color: "success" });
    await reloadAll();
  } catch (err: any) {
    toast.add({
      title: t("settings.ai.contextOptimizer.errors.publishFailed"),
      description: String(err?.message || err),
      color: "error",
    });
  } finally {
    publishing.value = false;
  }
}

async function rollbackSelected() {
  if (!selectedVersion.value) return;
  rollingBack.value = true;
  try {
    await AISettingService.rollbackContextOptimizer({
      env: env.value,
      scope: scope.value,
      target_version: selectedVersion.value,
      change_reason: "UI 回滚版本",
    });
    toast.add({ title: t("settings.ai.contextOptimizer.toasts.rollbackSuccess"), color: "success" });
    await reloadAll();
  } catch (err: any) {
    toast.add({
      title: t("settings.ai.contextOptimizer.errors.rollbackFailed"),
      description: String(err?.message || err),
      color: "error",
    });
  } finally {
    rollingBack.value = false;
  }
}

watch(env, (v) => {
  envStore.setCurrentEnv(v);
  void reloadAll();
});

watch(scope, () => {
  void reloadAll();
});

onMounted(async () => {
  if (!userStore.context) {
    await userStore.fetchUserContext();
  }
  await reloadAll();
});
</script>
