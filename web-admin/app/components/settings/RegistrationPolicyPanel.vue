<script setup lang="ts">
import {
  registrationPolicyModes,
  type RegistrationPolicyMode,
} from "~/composables/domain/registrationPolicy";
import {
  useRegistrationPolicyService,
  type RegistrationPolicy,
  type RegistrationInviteCodeRecord,
} from "~/composables/api/services/registrationPolicyService";

const { t } = useI18n();
const service = useRegistrationPolicyService();

const activeRegistrationTab = ref("active");
const updatePolicyOpen = ref(false);
const loading = ref(false);
const saving = ref(false);
const generating = ref(false);
const error = ref("");
const generatedCodes = ref<string[]>([]);
const generatedBatchName = ref("");
const generatedCodesOpen = ref(false);
const selectedBatch = ref<any | null>(null);
const batchDetailOpen = ref(false);
const batchCodesLoading = ref(false);
const resettingMissingPlainCodes = ref(false);
const deleteInviteBatchesOpen = ref(false);
const deletingInviteBatches = ref(false);
const selectedBatchCodes = ref<RegistrationInviteCodeRecord[]>([]);
const selectedInviteBatchUUIDs = ref<string[]>([]);
const copiedInviteCodeUUID = ref("");
const activePolicy = ref<any | null>(null);
const policyHistory = ref<RegistrationPolicy[]>([]);
const inviteBatches = ref<any[]>([]);
const requests = ref<any[]>([]);
let copiedInviteCodeTimer: ReturnType<typeof setTimeout> | undefined;

const policyForm = reactive({
  mode: "closed" as RegistrationPolicyMode,
  requires_verification: false,
  requires_invite_code: false,
  requires_root_approval: false,
  daily_tenant_quota: undefined as number | undefined,
  total_tenant_quota: undefined as number | undefined,
});

const inviteForm = reactive({
  name: "",
  max_codes: 20,
  max_uses_per_code: 1,
  allowed_email_domains: "",
  allowed_channels: "web",
  generate_count: 20,
});

const modeOptions = computed(() =>
  registrationPolicyModes.map((mode) => ({
    label: t(`registration.policy.mode.${mode}`),
    value: mode,
  }))
);
const registrationAdminTabItems = computed(() => [
  {
    label: t("registration.admin.tabs.activePolicy"),
    value: "active",
    icon: "i-heroicons-shield-check",
  },
  {
    label: t("registration.admin.tabs.inviteManagement"),
    value: "invites",
    icon: "i-heroicons-ticket",
  },
  {
    label: t("registration.admin.tabs.requests"),
    value: "requests",
    icon: "i-heroicons-inbox-stack",
  },
]);

const isClosedMode = computed(() => policyForm.mode === "closed");
const isInviteOnlyMode = computed(() => policyForm.mode === "invite_only");
const isRequestMode = computed(() =>
  policyForm.mode === "waitlist" || policyForm.mode === "approval_required"
);
const isApprovalMode = computed(() => policyForm.mode === "approval_required");
const canEditVerification = computed(() =>
  !isClosedMode.value && !isRequestMode.value
);
const canEditInvite = computed(() =>
  !isClosedMode.value && !isInviteOnlyMode.value && !isRequestMode.value
);
const canEditApproval = computed(() =>
  !isClosedMode.value && !isApprovalMode.value
);
const canEditQuota = computed(() => !isClosedMode.value);
const modeHintKey = computed(
  () => `registration.admin.modeHint.${policyForm.mode}`
);
const defaultInviteBatchName = computed(() => {
  const now = new Date();
  const pad = (n: number) => String(n).padStart(2, "0");
  const timestamp = [
    now.getFullYear(),
    pad(now.getMonth() + 1),
    pad(now.getDate()),
  ].join("") + `-${pad(now.getHours())}${pad(now.getMinutes())}`;
  return t("registration.admin.defaultBatchName", {
    mode: t(`registration.policy.mode.${policyForm.mode}`),
    timestamp,
  });
});
const generatedCodesText = computed(() => generatedCodes.value.join("\n"));
const generatedCodesCSV = computed(() => {
  const rows = [["index", "invite_code"]];
  generatedCodes.value.forEach((code, index) => {
    rows.push([String(index + 1), code]);
  });
  return rows
    .map((row) =>
      row
        .map((cell) => `"${String(cell).replaceAll('"', '""')}"`)
        .join(",")
    )
    .join("\n");
});
const selectedBatchPlainCodes = computed(() =>
  selectedBatchCodes.value
    .map((item) => item.plain_code || "")
    .filter(Boolean)
);
const selectedBatchHasMissingPlainCodes = computed(() =>
  selectedBatchCodes.value.some((item) => !String(item.plain_code || "").trim())
);
const selectedBatchCodesText = computed(() =>
  selectedBatchPlainCodes.value.join("\n")
);
const selectedBatchCodesCSV = computed(() => {
  const rows = [
    [
      "index",
      "invite_code",
      "status",
      "use_count",
      "max_uses",
      "last_used_at",
      "consumed_tenant_uuid",
    ],
  ];
  selectedBatchCodes.value.forEach((item, index) => {
    rows.push([
      String(index + 1),
      item.plain_code || "",
      inviteCodeUsageStatusLabel(item),
      String(item.use_count ?? 0),
      String(item.max_uses ?? 0),
      item.last_used_at || "",
      item.consumed_tenant_uuid || "",
    ]);
  });
  return rows
    .map((row) =>
      row
        .map((cell) => `"${String(cell).replaceAll('"', '""')}"`)
        .join(",")
    )
    .join("\n");
});
const selectedInviteBatchCount = computed(() => selectedInviteBatchUUIDs.value.length);
const allInviteBatchesSelected = computed(
  () =>
    inviteBatches.value.length > 0 &&
    inviteBatches.value.every((batch) =>
      selectedInviteBatchUUIDs.value.includes(String(batch.uuid || ""))
    )
);
const latestPolicyHistory = computed(() => policyHistory.value.slice(0, 20));

const normalizePolicyFormForMode = () => {
  if (isClosedMode.value) {
    policyForm.requires_verification = false;
    policyForm.requires_invite_code = false;
    policyForm.requires_root_approval = false;
    policyForm.daily_tenant_quota = undefined;
    policyForm.total_tenant_quota = undefined;
    return;
  }
  if (isRequestMode.value) {
    policyForm.requires_verification = false;
    policyForm.requires_invite_code = false;
  }
  if (isInviteOnlyMode.value) {
    policyForm.requires_invite_code = true;
  }
  if (isApprovalMode.value) {
    policyForm.requires_root_approval = true;
  }
};

watch(() => policyForm.mode, normalizePolicyFormForMode);

const isActivePolicyMissing = (err: any) => {
  const status = Number(err?.status || err?.statusCode || err?.cause?.status || 0);
  const message = String(err?.message || err?.data?.error || err?.data?.message || "");
  return status === 404 && message.includes("active registration policy missing");
};

const pruneInviteBatchSelection = () => {
  const visible = new Set(inviteBatches.value.map((batch) => String(batch.uuid || "")));
  selectedInviteBatchUUIDs.value = selectedInviteBatchUUIDs.value.filter((batchUUID) =>
    visible.has(batchUUID)
  );
};

const setInviteBatchSelected = (batchUUID: string, checked: boolean) => {
  const normalized = String(batchUUID || "").trim();
  if (!normalized) return;
  const next = new Set(selectedInviteBatchUUIDs.value);
  if (checked) {
    next.add(normalized);
  } else {
    next.delete(normalized);
  }
  selectedInviteBatchUUIDs.value = [...next];
};

const toggleAllInviteBatches = (checked: boolean) => {
  selectedInviteBatchUUIDs.value = checked
    ? inviteBatches.value.map((batch) => String(batch.uuid || "")).filter(Boolean)
    : [];
};

const loadAll = async () => {
  loading.value = true;
  error.value = "";
  try {
    const [policyResult, historyResult, batchesResult, requestsResult] = await Promise.allSettled([
      service.getPolicy(),
      service.listPolicyHistory(),
      service.listInviteBatches(),
      service.listRequests(),
    ]);

    if (policyResult.status === "fulfilled") {
      activePolicy.value = policyResult.value?.data || null;
    } else if (isActivePolicyMissing(policyResult.reason)) {
      activePolicy.value = null;
    } else {
      throw policyResult.reason;
    }

    if (historyResult.status === "fulfilled") {
      policyHistory.value = historyResult.value?.data?.items || [];
    } else {
      throw historyResult.reason;
    }

    if (batchesResult.status === "fulfilled") {
      inviteBatches.value = batchesResult.value?.data?.items || [];
      pruneInviteBatchSelection();
    } else {
      throw batchesResult.reason;
    }

    if (requestsResult.status === "fulfilled") {
      requests.value = requestsResult.value?.data?.items || [];
    } else {
      throw requestsResult.reason;
    }

    if (activePolicy.value?.mode) {
      policyForm.mode = activePolicy.value.mode;
      policyForm.requires_verification = Boolean(activePolicy.value.requires_verification);
      policyForm.requires_invite_code = Boolean(activePolicy.value.requires_invite_code);
      policyForm.requires_root_approval = Boolean(activePolicy.value.requires_root_approval);
      policyForm.daily_tenant_quota = activePolicy.value.daily_tenant_quota || undefined;
      policyForm.total_tenant_quota = activePolicy.value.total_tenant_quota || undefined;
    }
  } catch (err: any) {
    error.value = err.response?.data?.message || err.message || t("registration.admin.loadFailed");
  } finally {
    loading.value = false;
  }
};

const savePolicy = async () => {
  saving.value = true;
  error.value = "";
  try {
    normalizePolicyFormForMode();
    const draft: any = await service.createPolicyDraft({
      mode: policyForm.mode,
      requires_verification: policyForm.requires_verification,
      requires_invite_code:
        policyForm.requires_invite_code || policyForm.mode === "invite_only",
      requires_root_approval:
        policyForm.requires_root_approval || policyForm.mode === "approval_required",
      daily_tenant_quota: policyForm.daily_tenant_quota,
      total_tenant_quota: policyForm.total_tenant_quota,
      rules: [],
    });
    const policyUUID = draft?.data?.uuid;
    if (!policyUUID) {
      throw new Error(t("registration.admin.draftUuidMissing"));
    }
    await service.activatePolicy(policyUUID);
    await loadAll();
    updatePolicyOpen.value = false;
  } catch (err: any) {
    error.value = err.response?.data?.message || err.message || t("registration.admin.saveFailed");
  } finally {
    saving.value = false;
  }
};

const createInviteBatch = async () => {
  generating.value = true;
  error.value = "";
  generatedCodes.value = [];
  generatedBatchName.value = "";
  try {
    const batchName = inviteForm.name.trim() || defaultInviteBatchName.value;
    const batch: any = await service.createInviteBatch({
      name: batchName,
      max_codes: inviteForm.max_codes,
      max_uses_per_code: inviteForm.max_uses_per_code,
      allowed_email_domains: splitCSV(inviteForm.allowed_email_domains),
      allowed_channels: splitCSV(inviteForm.allowed_channels),
    });
    const batchUUID = batch?.data?.uuid;
    if (!batchUUID) {
      throw new Error(t("registration.admin.batchUuidMissing"));
    }
    const codes: any = await service.generateInviteCodes(
      batchUUID,
      inviteForm.generate_count
    );
    generatedCodes.value = codes?.data?.plain_codes || [];
    generatedBatchName.value = batchName;
    generatedCodesOpen.value = generatedCodes.value.length > 0;
    inviteForm.name = "";
    await loadAll();
  } catch (err: any) {
    error.value = err.response?.data?.message || err.message || t("registration.admin.inviteFailed");
  } finally {
    generating.value = false;
  }
};

const confirmDeleteInviteBatches = () => {
  if (!selectedInviteBatchUUIDs.value.length) return;
  deleteInviteBatchesOpen.value = true;
};

const deleteSelectedInviteBatches = async () => {
  if (!selectedInviteBatchUUIDs.value.length) return;
  deletingInviteBatches.value = true;
  error.value = "";
  const deletingUUIDs = [...selectedInviteBatchUUIDs.value];
  try {
    await service.deleteInviteBatches(deletingUUIDs);
    selectedInviteBatchUUIDs.value = [];
    if (selectedBatch.value?.uuid && deletingUUIDs.includes(String(selectedBatch.value.uuid))) {
      batchDetailOpen.value = false;
      selectedBatch.value = null;
      selectedBatchCodes.value = [];
    }
    deleteInviteBatchesOpen.value = false;
    await loadAll();
  } catch (err: any) {
    error.value = err.response?.data?.message || err.message || t("registration.admin.deleteInviteBatchesFailed");
  } finally {
    deletingInviteBatches.value = false;
  }
};

const openBatchDetail = async (batch: any) => {
  selectedBatch.value = batch;
  selectedBatchCodes.value = [];
  batchDetailOpen.value = true;
  batchCodesLoading.value = true;
  error.value = "";
  try {
    const result = await service.listInviteCodes(batch.uuid);
    selectedBatchCodes.value = result?.data?.items || [];
  } catch (err: any) {
    error.value = err.response?.data?.message || err.message || t("registration.admin.loadCodesFailed");
  } finally {
    batchCodesLoading.value = false;
  }
};

const resetMissingPlainCodes = async () => {
  if (!selectedBatch.value?.uuid) return;
  resettingMissingPlainCodes.value = true;
  error.value = "";
  try {
    const result = await service.resetMissingInviteCodePlaintext(selectedBatch.value.uuid);
    selectedBatchCodes.value = result?.data?.items || [];
    await loadAll();
  } catch (err: any) {
    error.value = err.response?.data?.message || err.message || t("registration.admin.resetMissingPlainCodesFailed");
  } finally {
    resettingMissingPlainCodes.value = false;
  }
};

const copyGeneratedCodes = async () => {
  if (!generatedCodesText.value || !process.client) return;
  await navigator.clipboard.writeText(generatedCodesText.value);
};

const downloadGeneratedCodes = () => {
  if (!generatedCodesCSV.value || !process.client) return;
  const safeName = (generatedBatchName.value || "invite-codes")
    .replace(/[^\p{L}\p{N}._-]+/gu, "-")
    .replace(/^-+|-+$/g, "");
  const blob = new Blob([generatedCodesCSV.value], {
    type: "text/csv;charset=utf-8",
  });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = `${safeName || "invite-codes"}.csv`;
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
};

const copySelectedBatchCodes = async () => {
  if (!selectedBatchCodesText.value || !process.client) return;
  await navigator.clipboard.writeText(selectedBatchCodesText.value);
};

const copyInviteCode = async (item: RegistrationInviteCodeRecord) => {
  const code = String(item.plain_code || "").trim();
  if (!code || !process.client) return;
  await navigator.clipboard.writeText(code);
  copiedInviteCodeUUID.value = item.uuid;
  if (copiedInviteCodeTimer) {
    clearTimeout(copiedInviteCodeTimer);
  }
  copiedInviteCodeTimer = setTimeout(() => {
    if (copiedInviteCodeUUID.value === item.uuid) {
      copiedInviteCodeUUID.value = "";
    }
  }, 1400);
};

const downloadSelectedBatchCodes = () => {
  if (!selectedBatchCodesCSV.value || !process.client) return;
  const safeName = (selectedBatch.value?.name || "invite-codes")
    .replace(/[^\p{L}\p{N}._-]+/gu, "-")
    .replace(/^-+|-+$/g, "");
  const blob = new Blob([selectedBatchCodesCSV.value], {
    type: "text/csv;charset=utf-8",
  });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = `${safeName || "invite-codes"}.csv`;
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
};

const formatListValue = (value: any) => {
  if (Array.isArray(value)) return value.join(", ") || "-";
  if (typeof value === "string") {
    const trimmed = value.trim();
    if (!trimmed) return "-";
    try {
      const parsed = JSON.parse(trimmed);
      if (Array.isArray(parsed)) return parsed.join(", ") || "-";
    } catch {
      return trimmed;
    }
    return trimmed;
  }
  return "-";
};

const formatDateTime = (value: any) => {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "-";
  return date.toLocaleString();
};

const formatShortUUID = (value: any) => {
  const raw = String(value || "").trim();
  if (!raw) return "-";
  return raw.length > 12 ? `${raw.slice(0, 8)}...${raw.slice(-4)}` : raw;
};

const inviteBatchStatusLabel = (status: string) =>
  t(`registration.admin.inviteStatus.${status || "unknown"}`);

const policyStatusLabel = (status: string) =>
  t(`registration.admin.policyStatus.${status || "unknown"}`);

const inviteCodeUsageStatusKey = (item: RegistrationInviteCodeRecord) => {
  if (item.status === "revoked") return "revoked";
  if (item.status === "consumed") return "used";
  const useCount = Number(item.use_count || 0);
  const maxUses = Number(item.max_uses || 0);
  if (maxUses > 0 && useCount >= maxUses) return "used";
  if (useCount > 0) return "partiallyUsed";
  return "unused";
};

const inviteCodeUsageStatusLabel = (item: RegistrationInviteCodeRecord) =>
  t(`registration.admin.codeUsageStatus.${inviteCodeUsageStatusKey(item)}`);

const rejectRequest = async (requestUUID: string) => {
  await service.rejectRequest(requestUUID, "root_rejected");
  await loadAll();
};

const splitCSV = (value: string) =>
  value
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);

onMounted(loadAll);

onBeforeUnmount(() => {
  if (copiedInviteCodeTimer) {
    clearTimeout(copiedInviteCodeTimer);
  }
});
</script>

<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between gap-4">
      <div>
        <h2 class="text-xl font-semibold text-gray-900 dark:text-white">
          {{ $t("registration.admin.title") }}
        </h2>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
          {{ $t("registration.admin.subtitle") }}
        </p>
      </div>
      <UButton
        icon="i-heroicons-arrow-path"
        variant="soft"
        :loading="loading"
        @click="loadAll"
      >
        {{ $t("common.refresh") }}
      </UButton>
    </div>

    <UAlert
      v-if="error"
      color="error"
      variant="soft"
      :title="error"
      class="mb-2"
    />

    <UTabs v-model="activeRegistrationTab" :items="registrationAdminTabItems" class="w-full" />

    <section
      v-if="activeRegistrationTab === 'active'"
      class="rounded-lg border border-gray-200 bg-white p-5 dark:border-gray-800 dark:bg-gray-900"
    >
      <div class="flex items-start justify-between gap-3">
        <div>
          <h3 class="text-base font-semibold text-gray-900 dark:text-white">
            {{ $t("registration.admin.activePolicy") }}
          </h3>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            {{ $t("registration.admin.activePolicyDescription") }}
          </p>
        </div>
        <UButton
          icon="i-heroicons-pencil-square"
          variant="soft"
          @click="updatePolicyOpen = true"
        >
          {{ $t("registration.admin.updatePolicy") }}
        </UButton>
      </div>
        <dl class="mt-4 grid grid-cols-2 gap-3 text-sm">
          <div>
            <dt class="text-gray-500">{{ $t("registration.admin.mode") }}</dt>
            <dd class="font-medium text-gray-900 dark:text-white">
              {{ activePolicy ? $t(`registration.policy.mode.${activePolicy.mode}`) : $t("registration.admin.notInitialized") }}
            </dd>
          </div>
          <div>
            <dt class="text-gray-500">{{ $t("registration.admin.version") }}</dt>
            <dd class="font-medium text-gray-900 dark:text-white">
              {{ activePolicy?.version || "-" }}
            </dd>
          </div>
        </dl>
        <div class="mt-6">
          <h4 class="text-sm font-semibold text-gray-900 dark:text-white">
            {{ $t("registration.admin.policyHistory") }}
          </h4>
          <div class="mt-3 overflow-x-auto rounded-lg border border-gray-200 dark:border-gray-800">
            <table class="min-w-full text-sm">
              <thead class="bg-gray-50 text-left text-xs uppercase tracking-wide text-gray-500 dark:bg-gray-950 dark:text-gray-400">
                <tr>
                  <th class="px-3 py-2">{{ $t("registration.admin.policyHistoryTable.version") }}</th>
                  <th class="px-3 py-2">{{ $t("registration.admin.policyHistoryTable.mode") }}</th>
                  <th class="px-3 py-2">{{ $t("registration.admin.policyHistoryTable.status") }}</th>
                  <th class="px-3 py-2">{{ $t("registration.admin.policyHistoryTable.gates") }}</th>
                  <th class="px-3 py-2">{{ $t("registration.admin.policyHistoryTable.activatedAt") }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 dark:divide-gray-800">
                <tr v-if="!latestPolicyHistory.length">
                  <td class="px-3 py-5 text-center text-gray-500" colspan="5">
                    {{ $t("registration.admin.noPolicyHistory") }}
                  </td>
                </tr>
                <tr v-for="item in latestPolicyHistory" :key="item.uuid">
                  <td class="px-3 py-2 font-medium text-gray-900 dark:text-white">
                    {{ item.version }}
                  </td>
                  <td class="px-3 py-2 text-gray-600 dark:text-gray-300">
                    {{ $t(`registration.policy.mode.${item.mode}`) }}
                  </td>
                  <td class="px-3 py-2 text-gray-600 dark:text-gray-300">
                    {{ policyStatusLabel(item.status) }}
                  </td>
                  <td class="px-3 py-2 text-gray-600 dark:text-gray-300">
                    <span class="inline-flex flex-wrap gap-1">
                      <span
                        v-if="item.requires_verification"
                        class="rounded bg-gray-100 px-1.5 py-0.5 text-xs dark:bg-gray-800"
                      >
                        {{ $t("registration.admin.requiresVerification") }}
                      </span>
                      <span
                        v-if="item.requires_invite_code"
                        class="rounded bg-gray-100 px-1.5 py-0.5 text-xs dark:bg-gray-800"
                      >
                        {{ $t("registration.admin.requiresInvite") }}
                      </span>
                      <span
                        v-if="item.requires_root_approval"
                        class="rounded bg-gray-100 px-1.5 py-0.5 text-xs dark:bg-gray-800"
                      >
                        {{ $t("registration.admin.requiresApproval") }}
                      </span>
                      <span
                        v-if="!item.requires_verification && !item.requires_invite_code && !item.requires_root_approval"
                        class="text-gray-500"
                      >
                        -
                      </span>
                    </span>
                  </td>
                  <td class="px-3 py-2 text-gray-600 dark:text-gray-300">
                    {{ formatDateTime((item as any).activated_at) }}
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
    </section>

    <section
      v-else-if="activeRegistrationTab === 'invites'"
      class="rounded-lg border border-gray-200 bg-white p-5 dark:border-gray-800 dark:bg-gray-900"
    >
      <h3 class="text-base font-semibold text-gray-900 dark:text-white">
        {{ $t("registration.admin.inviteBatches") }}
      </h3>
      <form class="mt-4 grid gap-3 lg:grid-cols-5" @submit.prevent="createInviteBatch">
        <UInput v-model="inviteForm.name" :placeholder="defaultInviteBatchName" />
        <UInput v-model.number="inviteForm.max_codes" type="number" min="1" />
        <UInput v-model="inviteForm.allowed_email_domains" :placeholder="$t('registration.admin.allowedDomains')" />
        <UInput v-model="inviteForm.allowed_channels" :placeholder="$t('registration.admin.allowedChannels')" />
        <UButton type="submit" icon="i-heroicons-ticket" :loading="generating">
          {{ $t("registration.admin.createCodes") }}
        </UButton>
      </form>
      <div class="mt-4 flex items-center justify-between gap-3">
        <p class="text-sm text-gray-500 dark:text-gray-400">
          {{ $t("registration.admin.selectedInviteBatches", { count: selectedInviteBatchCount }) }}
        </p>
        <UButton
          color="error"
          variant="soft"
          icon="i-heroicons-trash"
          :disabled="!selectedInviteBatchCount"
          @click="confirmDeleteInviteBatches"
        >
          {{ $t("registration.admin.deleteSelectedInviteBatches") }}
        </UButton>
      </div>
      <div class="mt-4 overflow-x-auto rounded-lg border border-gray-200 dark:border-gray-800">
        <table class="min-w-full text-sm">
          <thead class="bg-gray-50 text-left text-xs uppercase tracking-wide text-gray-500 dark:bg-gray-950 dark:text-gray-400">
            <tr>
              <th class="w-12 px-4 py-3">
                <UCheckbox
                  :model-value="allInviteBatchesSelected"
                  :disabled="!inviteBatches.length"
                  :aria-label="$t('registration.admin.selectAllInviteBatches')"
                  @update:model-value="toggleAllInviteBatches(Boolean($event))"
                />
              </th>
              <th class="px-4 py-3">{{ $t("registration.admin.batchTable.name") }}</th>
              <th class="px-4 py-3">{{ $t("registration.admin.batchTable.status") }}</th>
              <th class="px-4 py-3">{{ $t("registration.admin.batchTable.maxCodes") }}</th>
              <th class="px-4 py-3">{{ $t("registration.admin.batchTable.maxUses") }}</th>
              <th class="px-4 py-3">{{ $t("registration.admin.batchTable.channels") }}</th>
              <th class="px-4 py-3 text-right">{{ $t("registration.admin.batchTable.actions") }}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100 dark:divide-gray-800">
            <tr v-if="!inviteBatches.length">
              <td class="px-4 py-6 text-center text-gray-500" colspan="7">
                {{ $t("registration.admin.noInviteBatches") }}
              </td>
            </tr>
            <tr v-for="batch in inviteBatches" :key="batch.uuid">
              <td class="px-4 py-3">
                <UCheckbox
                  :model-value="selectedInviteBatchUUIDs.includes(String(batch.uuid || ''))"
                  :aria-label="$t('registration.admin.selectInviteBatch', { name: batch.name })"
                  @update:model-value="setInviteBatchSelected(batch.uuid, Boolean($event))"
                />
              </td>
              <td class="px-4 py-3 font-medium text-gray-900 dark:text-white">
                {{ batch.name }}
              </td>
              <td class="px-4 py-3 text-gray-600 dark:text-gray-300">
                {{ inviteBatchStatusLabel(batch.status) }}
              </td>
              <td class="px-4 py-3 text-gray-600 dark:text-gray-300">
                {{ batch.max_codes ?? "-" }}
              </td>
              <td class="px-4 py-3 text-gray-600 dark:text-gray-300">
                {{ batch.max_uses_per_code ?? "-" }}
              </td>
              <td class="px-4 py-3 text-gray-600 dark:text-gray-300">
                {{ formatListValue(batch.allowed_channels) }}
              </td>
              <td class="px-4 py-3 text-right">
                <UButton
                  size="xs"
                  variant="soft"
                  icon="i-heroicons-eye"
                  @click="openBatchDetail(batch)"
                >
                  {{ $t("registration.admin.details") }}
                </UButton>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <section
      v-else-if="activeRegistrationTab === 'requests'"
      class="rounded-lg border border-gray-200 bg-white p-5 dark:border-gray-800 dark:bg-gray-900"
    >
      <h3 class="text-base font-semibold text-gray-900 dark:text-white">
        {{ $t("registration.admin.requests") }}
      </h3>
      <div class="mt-4 divide-y divide-gray-100 dark:divide-gray-800">
        <div v-for="item in requests" :key="item.uuid" class="flex items-center justify-between gap-3 py-3 text-sm">
          <div>
            <div class="font-medium text-gray-900 dark:text-white">{{ item.tenant_name }}</div>
            <div class="text-gray-500">{{ item.mode }} · {{ item.status }}</div>
          </div>
          <div v-if="item.status === 'submitted'" class="flex gap-2">
            <UButton size="sm" variant="soft" @click="service.approveRequest(item.uuid).then(loadAll)">
              {{ $t("registration.admin.approve") }}
            </UButton>
            <UButton size="sm" color="error" variant="soft" @click="rejectRequest(item.uuid)">
              {{ $t("registration.admin.reject") }}
            </UButton>
          </div>
        </div>
      </div>
    </section>

    <UModal
      v-model:open="updatePolicyOpen"
      :title="$t('registration.admin.updatePolicy')"
      :description="$t('registration.admin.updatePolicyDescription')"
      :ui="{ content: 'max-w-3xl w-[90vw]' }"
    >
      <template #body>
        <form id="registration-policy-update-form" class="space-y-4" @submit.prevent="savePolicy">
          <UFormField :label="$t('registration.admin.mode')">
            <USelect v-model="policyForm.mode" :items="modeOptions" class="w-full" />
          </UFormField>
          <div class="grid gap-3 sm:grid-cols-3">
            <UCheckbox
              v-model="policyForm.requires_verification"
              :label="$t('registration.admin.requiresVerification')"
              :disabled="!canEditVerification"
            />
            <UCheckbox
              v-model="policyForm.requires_invite_code"
              :label="$t('registration.admin.requiresInvite')"
              :disabled="!canEditInvite"
            />
            <UCheckbox
              v-model="policyForm.requires_root_approval"
              :label="$t('registration.admin.requiresApproval')"
              :disabled="!canEditApproval"
            />
          </div>
          <p class="text-xs text-gray-500 dark:text-gray-400">
            {{ $t(modeHintKey) }}
          </p>
          <div class="grid gap-3 sm:grid-cols-2">
            <UFormField :label="$t('registration.admin.dailyQuota')">
              <UInput
                v-model.number="policyForm.daily_tenant_quota"
                type="number"
                min="0"
                :disabled="!canEditQuota"
                class="w-full"
              />
            </UFormField>
            <UFormField :label="$t('registration.admin.totalQuota')">
              <UInput
                v-model.number="policyForm.total_tenant_quota"
                type="number"
                min="0"
                :disabled="!canEditQuota"
                class="w-full"
              />
            </UFormField>
          </div>
        </form>
      </template>
      <template #footer>
        <div class="flex w-full justify-end gap-2">
          <UButton
            color="neutral"
            variant="subtle"
            :disabled="saving"
            @click="updatePolicyOpen = false"
          >
            {{ $t("common.cancel") }}
          </UButton>
          <UButton
            type="submit"
            form="registration-policy-update-form"
            icon="i-heroicons-check"
            :loading="saving"
          >
            {{ $t("registration.admin.saveAndActivate") }}
          </UButton>
        </div>
      </template>
    </UModal>

    <UModal
      v-model:open="generatedCodesOpen"
      :title="$t('registration.admin.generatedCodesTitle')"
      :description="$t('registration.admin.generatedCodesDescription')"
      :ui="{ content: 'max-w-2xl' }"
    >
      <template #body>
        <div class="rounded-md bg-gray-50 p-3 font-mono text-sm dark:bg-gray-950">
          <div v-for="code in generatedCodes" :key="code">{{ code }}</div>
        </div>
      </template>
      <template #footer>
        <div class="flex w-full justify-end gap-2">
          <UButton color="neutral" variant="subtle" @click="generatedCodesOpen = false">
            {{ $t("common.close") }}
          </UButton>
          <UButton icon="i-heroicons-arrow-down-tray" variant="soft" @click="downloadGeneratedCodes">
            {{ $t("registration.admin.downloadCodes") }}
          </UButton>
          <UButton icon="i-heroicons-clipboard" @click="copyGeneratedCodes">
            {{ $t("registration.admin.copyCodes") }}
          </UButton>
        </div>
      </template>
    </UModal>

    <UModal
      v-model:open="batchDetailOpen"
      :title="selectedBatch?.name || $t('registration.admin.batchDetailTitle')"
      :ui="{ content: 'max-w-5xl w-[92vw]' }"
    >
      <template #body>
        <div v-if="selectedBatch" class="space-y-5">
          <dl class="grid grid-cols-1 gap-4 text-sm sm:grid-cols-2">
            <div>
              <dt class="text-gray-500">{{ $t("registration.admin.batchTable.status") }}</dt>
              <dd class="font-medium text-gray-900 dark:text-white">
                {{ inviteBatchStatusLabel(selectedBatch.status) }}
              </dd>
            </div>
            <div>
              <dt class="text-gray-500">{{ $t("registration.admin.batchTable.maxCodes") }}</dt>
              <dd class="font-medium text-gray-900 dark:text-white">
                {{ selectedBatch.max_codes ?? "-" }}
              </dd>
            </div>
            <div>
              <dt class="text-gray-500">{{ $t("registration.admin.batchTable.maxUses") }}</dt>
              <dd class="font-medium text-gray-900 dark:text-white">
                {{ selectedBatch.max_uses_per_code ?? "-" }}
              </dd>
            </div>
            <div>
              <dt class="text-gray-500">{{ $t("registration.admin.allowedPlan") }}</dt>
              <dd class="font-medium text-gray-900 dark:text-white">
                {{ selectedBatch.allowed_plan || "-" }}
              </dd>
            </div>
            <div>
              <dt class="text-gray-500">{{ $t("registration.admin.batchTable.channels") }}</dt>
              <dd class="font-medium text-gray-900 dark:text-white">
                {{ formatListValue(selectedBatch.allowed_channels) }}
              </dd>
            </div>
            <div>
              <dt class="text-gray-500">{{ $t("registration.admin.batchTable.emailDomains") }}</dt>
              <dd class="font-medium text-gray-900 dark:text-white">
                {{ formatListValue(selectedBatch.allowed_email_domains) }}
              </dd>
            </div>
            <div>
              <dt class="text-gray-500">{{ $t("registration.admin.batchTable.startsAt") }}</dt>
              <dd class="font-medium text-gray-900 dark:text-white">
                {{ formatDateTime(selectedBatch.starts_at) }}
              </dd>
            </div>
            <div>
              <dt class="text-gray-500">{{ $t("registration.admin.batchTable.expiresAt") }}</dt>
              <dd class="font-medium text-gray-900 dark:text-white">
                {{ formatDateTime(selectedBatch.expires_at) }}
              </dd>
            </div>
          </dl>

          <div class="rounded-lg border border-gray-200 p-4 dark:border-gray-800">
            <div class="mb-3 flex items-center justify-between gap-3">
              <h4 class="text-sm font-semibold text-gray-900 dark:text-white">
                {{ $t("registration.admin.batchCodes") }}
              </h4>
              <div class="flex gap-2">
                <UButton
                  v-if="selectedBatchHasMissingPlainCodes"
                  size="xs"
                  variant="soft"
                  icon="i-heroicons-arrow-path"
                  :loading="resettingMissingPlainCodes"
                  @click="resetMissingPlainCodes"
                >
                  {{ $t("registration.admin.resetMissingPlainCodes") }}
                </UButton>
                <UButton
                  size="xs"
                  icon="i-heroicons-clipboard"
                  :disabled="!selectedBatchPlainCodes.length"
                  @click="copySelectedBatchCodes"
                >
                  {{ $t("registration.admin.copyCodes") }}
                </UButton>
                <UButton
                  size="xs"
                  variant="soft"
                  icon="i-heroicons-arrow-down-tray"
                  :disabled="!selectedBatchCodes.length"
                  @click="downloadSelectedBatchCodes"
                >
                  {{ $t("registration.admin.downloadCodes") }}
                </UButton>
              </div>
            </div>
            <div
              v-if="selectedBatchHasMissingPlainCodes"
              class="mb-3 rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800 dark:border-amber-900/60 dark:bg-amber-950/30 dark:text-amber-200"
            >
              {{ $t("registration.admin.missingPlainCodeNotice") }}
            </div>
            <div v-if="batchCodesLoading" class="py-6 text-center text-sm text-gray-500">
              {{ $t("registration.admin.loadingCodes") }}
            </div>
            <div v-else class="max-h-72 overflow-auto rounded-lg border border-gray-200 dark:border-gray-800">
              <table class="min-w-full text-sm">
                <thead class="bg-gray-50 text-left text-xs uppercase tracking-wide text-gray-500 dark:bg-gray-950 dark:text-gray-400">
                  <tr>
                    <th class="px-3 py-2">{{ $t("registration.admin.codeTable.code") }}</th>
                    <th class="px-3 py-2">{{ $t("registration.admin.codeTable.status") }}</th>
                    <th class="px-3 py-2">{{ $t("registration.admin.codeTable.uses") }}</th>
                    <th class="px-3 py-2">{{ $t("registration.admin.codeTable.lastUsedAt") }}</th>
                    <th class="px-3 py-2">{{ $t("registration.admin.codeTable.consumedTenant") }}</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-gray-100 dark:divide-gray-800">
                  <tr v-if="!selectedBatchCodes.length">
                    <td class="px-3 py-6 text-center text-gray-500" colspan="5">
                      {{ $t("registration.admin.noInviteCodes") }}
                    </td>
                  </tr>
                  <tr v-for="item in selectedBatchCodes" :key="item.uuid">
                    <td class="px-3 py-2">
                      <button
                        v-if="item.plain_code"
                        type="button"
                        class="group flex w-full items-center justify-between gap-2 rounded px-1 py-1 text-left font-mono text-gray-900 transition hover:bg-emerald-50 hover:text-emerald-700 focus:outline-none focus:ring-2 focus:ring-emerald-500 dark:text-white dark:hover:bg-emerald-950/40 dark:hover:text-emerald-300"
                        :title="$t('registration.admin.copySingleCode')"
                        :aria-label="$t('registration.admin.copySingleCode')"
                        @click="copyInviteCode(item)"
                      >
                        <span>{{ item.plain_code }}</span>
                        <span class="shrink-0 text-xs font-sans text-emerald-600 opacity-0 transition group-hover:opacity-100 group-focus:opacity-100 dark:text-emerald-300">
                          {{ copiedInviteCodeUUID === item.uuid ? $t("registration.admin.codeCopied") : $t("registration.admin.clickToCopy") }}
                        </span>
                      </button>
                      <span v-else class="font-mono text-gray-500">
                        {{ $t("registration.admin.missingPlainCode") }}
                      </span>
                    </td>
                    <td class="px-3 py-2 text-gray-600 dark:text-gray-300">
                      {{ inviteCodeUsageStatusLabel(item) }}
                    </td>
                    <td class="px-3 py-2 text-gray-600 dark:text-gray-300">
                      {{ item.use_count ?? 0 }} / {{ item.max_uses ?? 0 }}
                    </td>
                    <td class="px-3 py-2 text-gray-600 dark:text-gray-300">
                      {{ formatDateTime(item.last_used_at) }}
                    </td>
                    <td class="px-3 py-2 text-gray-600 dark:text-gray-300">
                      {{ formatShortUUID(item.consumed_tenant_uuid) }}
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>
        </div>
      </template>
      <template #footer>
        <div class="flex w-full justify-end">
          <UButton color="neutral" variant="subtle" @click="batchDetailOpen = false">
            {{ $t("common.close") }}
          </UButton>
        </div>
      </template>
    </UModal>

    <UModal
      v-model:open="deleteInviteBatchesOpen"
      :title="$t('registration.admin.deleteInviteBatchesTitle')"
      :description="$t('registration.admin.deleteInviteBatchesDescription', { count: selectedInviteBatchCount })"
      :ui="{ content: 'max-w-lg' }"
    >
      <template #body>
        <div class="space-y-3">
          <UAlert
            color="warning"
            variant="soft"
            :title="$t('registration.admin.deleteInviteBatchesWarning')"
          />
          <ul class="max-h-48 space-y-2 overflow-auto rounded-md border border-gray-200 p-3 text-sm dark:border-gray-800">
            <li
              v-for="batch in inviteBatches.filter((item) => selectedInviteBatchUUIDs.includes(String(item.uuid || '')))"
              :key="batch.uuid"
              class="font-medium text-gray-900 dark:text-white"
            >
              {{ batch.name }}
            </li>
          </ul>
        </div>
      </template>
      <template #footer>
        <div class="flex w-full justify-end gap-2">
          <UButton
            color="neutral"
            variant="subtle"
            :disabled="deletingInviteBatches"
            @click="deleteInviteBatchesOpen = false"
          >
            {{ $t("common.cancel") }}
          </UButton>
          <UButton
            color="error"
            icon="i-heroicons-trash"
            :loading="deletingInviteBatches"
            @click="deleteSelectedInviteBatches"
          >
            {{ $t("registration.admin.confirmDeleteInviteBatches") }}
          </UButton>
        </div>
      </template>
    </UModal>
  </div>
</template>
