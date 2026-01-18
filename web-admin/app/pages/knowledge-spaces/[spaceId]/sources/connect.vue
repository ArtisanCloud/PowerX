<script setup lang="ts">
import {
  useKnowledgeSpaceSources,
  type KnowledgeCredentialAuthType,
  type KnowledgeSourceProvider,
  type KnowledgeSyncMode,
} from "~/composables/useKnowledgeSpaceSources";
import { useEmbeddingGuard } from "~/composables/useEmbeddingGuard";

const { t } = useI18n();
const route = useRoute();
const toast = useToast();
const { ensureEmbeddingReady } = useEmbeddingGuard();
const embeddingReady = ref(false);

const spaceId = computed(() => String(route.params.spaceId || "").trim());

useHead(() => ({
  title: t("knowledgeSpaces.sources.connect.head.title", "新增数据源连接"),
}));

const step = ref<1 | 2 | 3 | 4>(1);
const provider = ref<KnowledgeSourceProvider | "">("");

const providerItems = computed(() => [
  { label: t("knowledgeSpaces.sources.providers.notion", "Notion"), value: "notion" },
  { label: t("knowledgeSpaces.sources.providers.feishu", "飞书"), value: "feishu" },
]);

const goBack = () => navigateTo(`/knowledge-spaces/${encodeURIComponent(spaceId.value)}/sources`);

const sources = useKnowledgeSpaceSources();

const authType = ref<KnowledgeCredentialAuthType>("token");
const reuseCredentialId = ref<string>("");
const newCredentialLabel = ref<string>("");
const newCredentialHint = ref<string>("");

const availableCredentials = computed(() => {
  if (!embeddingReady.value) return [];
  if (!provider.value) return [];
  try {
    return sources.listTenantCredentials(provider.value as KnowledgeSourceProvider);
  } catch {
    return [];
  }
});

onMounted(async () => {
  if (!(await ensureEmbeddingReady())) return;
  embeddingReady.value = true;
});

const scopeForm = reactive<Record<string, any>>({
  notion: { workspace: "", databaseId: "", pageId: "" },
  feishu: { wikiSpaceId: "", folderToken: "", docToken: "" },
});

const syncMode = ref<KnowledgeSyncMode>("full_then_incremental");
const schedule = ref<string>("@hourly");

const next = () => {
  if (step.value === 1 && !provider.value) return;
  if (step.value === 2) {
    if (!reuseCredentialId.value && !newCredentialLabel.value.trim()) return;
  }
  if (step.value === 3) {
    if (provider.value === "notion") {
      const v = scopeForm.notion || {};
      if (!String(v.databaseId || v.pageId || v.workspace || "").trim()) return;
    }
    if (provider.value === "feishu") {
      const v = scopeForm.feishu || {};
      if (!String(v.docToken || v.folderToken || v.wikiSpaceId || "").trim()) return;
    }
  }
  if (step.value < 4) step.value = (step.value + 1) as any;
};

const prev = () => {
  if (step.value > 1) step.value = (step.value - 1) as any;
};

const finish = () => {
  if (!provider.value) return;
  let credentialId = reuseCredentialId.value;
  if (!credentialId) {
    const cred = sources.upsertTenantCredential({
      provider: provider.value as KnowledgeSourceProvider,
      authType: authType.value,
      label: newCredentialLabel.value.trim() || `${provider.value}-credential`,
      maskedHint: newCredentialHint.value.trim() || undefined,
    });
    credentialId = cred.id;
  }
  const connector = sources.ensureConnector({
    provider: provider.value as KnowledgeSourceProvider,
    credentialId,
  });
  const scope = provider.value === "notion" ? { ...scopeForm.notion } : { ...scopeForm.feishu };
  sources.createSpaceSyncJob({
    spaceId: spaceId.value,
    provider: provider.value as KnowledgeSourceProvider,
    connectorId: connector.id,
    syncMode: syncMode.value,
    schedule: schedule.value.trim() || "@hourly",
    scope,
  });
  toast.add({
    color: "success",
    title: t("knowledgeSpaces.sources.connect.doneTitle", "已创建连接"),
    description: t("knowledgeSpaces.sources.connect.doneDesc", "已为当前空间创建增量同步任务（本地草稿）。"),
  });
  return goBack();
};
</script>

<template>
  <section class="mx-auto max-w-5xl space-y-6 px-6 py-8">
    <header class="rounded-2xl border border-[var(--border-color)] bg-[var(--card-bg)] p-6 shadow-sm">
      <div class="flex items-start justify-between gap-4">
        <div>
          <p class="text-sm text-[var(--text-secondary)]">
            {{ t("knowledgeSpaces.sources.badge", "Data Sources") }}
          </p>
          <h1 class="mt-1 text-2xl font-semibold text-[var(--text-primary)]">
            {{ t("knowledgeSpaces.sources.connect.title", "新增连接") }}
          </h1>
          <p class="mt-2 text-sm text-[var(--text-secondary)]">
            {{
              t(
                "knowledgeSpaces.sources.connect.subtitle",
                "按向导完成：选择数据源 → 授权（租户级复用）→ 选择同步范围 → 创建增量同步任务。",
              )
            }}
          </p>
        </div>
        <UButton color="neutral" variant="subtle" icon="i-heroicons-arrow-left" @click="goBack">
          {{ t("common.back", "返回") }}
        </UButton>
      </div>
    </header>

    <UCard :ui="{ body: { padding: 'p-6' } }">
      <div class="mb-4 flex items-center justify-between text-sm">
        <span class="text-[var(--text-secondary)]">
          {{ t("knowledgeSpaces.sources.connect.wizard.step", "步骤：{n}/4", { n: step }) }}
        </span>
        <UBadge color="neutral" variant="soft">
          {{ t("knowledgeSpaces.sources.connect.wizard.stepBadge", "第 {n} 步", { n: step }) }}
        </UBadge>
      </div>

      <div v-if="step === 1" class="space-y-4">
        <UFormField :label="t('knowledgeSpaces.sources.connect.provider', '数据源类型')" required>
          <USelect v-model="provider" :items="providerItems" class="w-full" />
          <template #help>
            <span class="text-xs text-[var(--text-secondary)]">
              {{
                t(
                  "knowledgeSpaces.sources.connect.providerHint",
                  "Notion/飞书属于“鉴权 API 接入”，会以连接器 + 同步任务的方式导入内容。",
                )
              }}
            </span>
          </template>
        </UFormField>
      </div>

      <div v-else-if="step === 2" class="space-y-3">
        <UAlert
          color="info"
          variant="soft"
          icon="i-heroicons-lock-closed"
          :title="t('knowledgeSpaces.sources.connect.auth.title', '授权（租户级复用）')"
          :description="t('knowledgeSpaces.sources.connect.auth.desc', '优先复用租户已有授权；没有则新建一个授权草稿。')"
        />

        <UFormField :label="t('knowledgeSpaces.sources.connect.auth.reuse', '复用已有凭据（可选）')">
          <USelect
            v-model="reuseCredentialId"
            :items="availableCredentials.map((c) => ({ label: `${c.label}${c.maskedHint ? `（${c.maskedHint}）` : ''}`, value: c.id }))"
            class="w-full"
            placeholder="选择一个租户级凭据"
          />
        </UFormField>

        <div class="rounded-xl border border-[var(--border-color)] bg-[var(--card-bg)] p-4">
          <div class="text-sm font-semibold text-[var(--text-primary)]">
            {{ t("knowledgeSpaces.sources.connect.auth.createNew", "或新建凭据") }}
          </div>
          <div class="mt-3 grid gap-3 sm:grid-cols-2">
            <UFormField :label="t('knowledgeSpaces.sources.connect.auth.type', '授权方式')">
              <USelect
                v-model="authType"
                :items="[
                  { label: 'Token', value: 'token' },
                  { label: 'OAuth', value: 'oauth' },
                ]"
              />
            </UFormField>
            <UFormField :label="t('knowledgeSpaces.sources.connect.auth.label', '凭据名称')" required>
              <UInput v-model="newCredentialLabel" placeholder="例如：IT 部门 Notion" />
            </UFormField>
            <UFormField :label="t('knowledgeSpaces.sources.connect.auth.hint', '提示信息（可选）')">
              <UInput v-model="newCredentialHint" placeholder="例如：token ****a12f" />
            </UFormField>
          </div>
          <p class="mt-2 text-xs text-[var(--text-secondary)]">
            {{ t("knowledgeSpaces.sources.connect.auth.note", "当前实现为本地草稿存储；后端接入后会改为安全密钥引用。") }}
          </p>
        </div>
      </div>

      <div v-else-if="step === 3" class="space-y-3">
        <UAlert
          color="info"
          variant="soft"
          icon="i-heroicons-folder-open"
          :title="t('knowledgeSpaces.sources.connect.scope.title', '选择同步范围')"
          :description="t('knowledgeSpaces.sources.connect.scope.desc', '先选择一个最小范围（建议从一个数据库/文档开始）。')"
        />

        <div v-if="provider === 'notion'" class="grid gap-3 sm:grid-cols-2">
          <UFormField label="Workspace（可选）">
            <UInput v-model="scopeForm.notion.workspace" placeholder="workspace id / slug" />
          </UFormField>
          <UFormField label="Database ID / URL（推荐）">
            <UInput v-model="scopeForm.notion.databaseId" placeholder="database id 或 URL" />
          </UFormField>
          <UFormField label="Page ID / URL（可选）" class="sm:col-span-2">
            <UInput v-model="scopeForm.notion.pageId" placeholder="page id 或 URL" />
          </UFormField>
        </div>

        <div v-else-if="provider === 'feishu'" class="grid gap-3 sm:grid-cols-2">
          <UFormField label="知识库 Space ID（可选）">
            <UInput v-model="scopeForm.feishu.wikiSpaceId" placeholder="wiki space id" />
          </UFormField>
          <UFormField label="目录 Token（可选）">
            <UInput v-model="scopeForm.feishu.folderToken" placeholder="folder token" />
          </UFormField>
          <UFormField label="文档 Token（推荐）" class="sm:col-span-2">
            <UInput v-model="scopeForm.feishu.docToken" placeholder="doc token" />
          </UFormField>
        </div>
      </div>

      <div v-else class="space-y-3">
        <UAlert
          color="warning"
          variant="soft"
          icon="i-heroicons-clock"
          :title="t('knowledgeSpaces.sources.connect.job.title', '创建增量同步任务')"
          :description="t('knowledgeSpaces.sources.connect.job.desc', '推荐：先全量一次，再按计划做增量；失败自动重试并记录审计。')"
        />

        <div class="grid gap-3 sm:grid-cols-2">
          <UFormField :label="t('knowledgeSpaces.sources.connect.job.mode', '同步模式')">
            <USelect
              v-model="syncMode"
              :items="[
                { label: 'Full → Incremental', value: 'full_then_incremental' },
                { label: 'Incremental only', value: 'incremental' },
              ]"
            />
          </UFormField>
          <UFormField :label="t('knowledgeSpaces.sources.connect.job.schedule', '同步计划（cron/@hourly）')" required>
            <UInput v-model="schedule" placeholder="@hourly / @daily / */15 * * * *" />
          </UFormField>
        </div>
      </div>

      <div class="mt-6 flex items-center justify-between gap-2">
        <UButton color="neutral" variant="subtle" :disabled="step === 1" @click="prev">
          {{ t("knowledgeSpaces.sources.connect.wizard.prev", "上一步") }}
        </UButton>
        <div class="flex gap-2">
          <UButton v-if="step < 4" color="primary" variant="soft" :disabled="step === 1 && !provider" @click="next">
            {{ t("knowledgeSpaces.sources.connect.wizard.next", "下一步") }}
          </UButton>
          <UButton v-else color="primary" @click="finish">
            {{ t("knowledgeSpaces.sources.connect.wizard.finish", "完成") }}
          </UButton>
        </div>
      </div>
    </UCard>
  </section>
</template>
