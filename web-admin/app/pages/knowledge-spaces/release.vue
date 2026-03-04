<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { useKnowledgeSpaces, type ReleasePolicyRecord, type ReleaseStatusView } from "~/composables/useKnowledgeSpaces";
import { useEmbeddingGuard } from "~/composables/useEmbeddingGuard";

useHead(() => ({
  title: "租户灰度发布",
  meta: [{ name: "description", content: "租户灰度发布与治理（US9）" }],
}));

const api = useKnowledgeSpaces();
const { ensureEmbeddingReady } = useEmbeddingGuard();
const embeddingReady = ref(false);

const loading = ref(false);
const errorText = ref("");
const successText = ref("");

const policies = ref<ReleasePolicyRecord[]>([]);
const selectedPolicyId = ref<string>("");
const versionId = ref<string>("");
const rollbackReason = ref<string>("metrics breached");
const matrixText = ref<string>("");

const status = ref<ReleaseStatusView | null>(null);
const selectedBatchToken = ref<string>("");

const selectedPolicy = computed(() => {
  const id = Number.parseInt(selectedPolicyId.value, 10);
  return policies.value.find((p) => p.id === id);
});

const hydrateMatrixFromPolicy = () => {
  if (!selectedPolicy.value) {
    matrixText.value = "";
    return;
  }
  const p = selectedPolicy.value;
  matrixText.value = JSON.stringify(
    {
      matrixVersion: p.matrixVersion,
      pilotTenants: p.pilotTenants || [],
      batches: (p as any).batches || [],
      guardrails: (p as any).guardrails || {},
      approvedBy: p.approvedBy || "",
      createdBy: p.createdBy || "",
    },
    null,
    2,
  );
};

const refreshPolicies = async () => {
  if (!embeddingReady.value) return;
  loading.value = true;
  errorText.value = "";
  successText.value = "";
  try {
    policies.value = await api.listReleasePolicies(20);
    if (!selectedPolicyId.value && policies.value.length > 0) {
      selectedPolicyId.value = String(policies.value[0].id);
      hydrateMatrixFromPolicy();
    }
  } catch (err: any) {
    errorText.value = err?.message || "加载策略失败";
  } finally {
    loading.value = false;
  }
};

const upsertPolicy = async () => {
  loading.value = true;
  errorText.value = "";
  successText.value = "";
  try {
    const payload = JSON.parse(matrixText.value || "{}");
    const resp = await api.upsertReleasePolicy({
      matrixVersion: payload.matrixVersion,
      pilotTenants: payload.pilotTenants || [],
      batches: payload.batches || [],
      guardrails: payload.guardrails || {},
      approvedBy: payload.approvedBy || "web-admin",
      createdBy: payload.createdBy || "web-admin",
    });
    successText.value = `策略已保存：policyId=${resp.policyId} status=${resp.status}`;
    await refreshPolicies();
    selectedPolicyId.value = String(resp.policyId);
    hydrateMatrixFromPolicy();
  } catch (err: any) {
    errorText.value = err?.message || "保存策略失败";
  } finally {
    loading.value = false;
  }
};

const refreshStatus = async () => {
  if (!selectedPolicyId.value) {
    errorText.value = "请先选择 policyId";
    return;
  }
  loading.value = true;
  errorText.value = "";
  successText.value = "";
  try {
    status.value = await api.getReleaseStatus(selectedPolicyId.value, versionId.value || undefined);
    selectedBatchToken.value = status.value?.batches?.[0]?.batchToken || "";
  } catch (err: any) {
    errorText.value = err?.message || "加载状态失败";
  } finally {
    loading.value = false;
  }
};

const publish = async () => {
  if (!selectedPolicyId.value || !versionId.value) {
    errorText.value = "publish 需要 policyId 与 versionId";
    return;
  }
  loading.value = true;
  errorText.value = "";
  successText.value = "";
  try {
    const resp = await api.publishRelease({
      policyId: selectedPolicyId.value,
      versionId: versionId.value,
      requestedBy: "web-admin",
    });
    successText.value = `已发布：batchToken=${resp.batchToken} tenants=${resp.tenants?.length || 0}`;
    selectedBatchToken.value = resp.batchToken;
    await refreshStatus();
  } catch (err: any) {
    errorText.value = err?.message || "发布失败";
  } finally {
    loading.value = false;
  }
};

const promote = async (alerts: string[] = []) => {
  if (!selectedPolicyId.value || !versionId.value || !selectedBatchToken.value) {
    errorText.value = "promote 需要 policyId/versionId/batchToken";
    return;
  }
  loading.value = true;
  errorText.value = "";
  successText.value = "";
  try {
    const resp = await api.promoteRelease({
      policyId: selectedPolicyId.value,
      versionId: versionId.value,
      batchToken: selectedBatchToken.value,
      alerts,
      requestedBy: "web-admin",
    });
    successText.value = `推进结果：state=${resp.state} coverage=${resp.tenantCoverage}`;
    selectedBatchToken.value = resp.batchToken || "";
    await refreshStatus();
  } catch (err: any) {
    errorText.value = err?.message || "推进失败";
  } finally {
    loading.value = false;
  }
};

const rollback = async () => {
  if (!selectedPolicyId.value || !versionId.value) {
    errorText.value = "rollback 需要 policyId/versionId";
    return;
  }
  loading.value = true;
  errorText.value = "";
  successText.value = "";
  try {
    const resp = await api.rollbackRelease({
      policyId: selectedPolicyId.value,
      versionId: versionId.value,
      reason: rollbackReason.value,
      requestedBy: "web-admin",
    });
    successText.value = `回滚已触发：status=${resp.status}`;
    await refreshStatus();
  } catch (err: any) {
    errorText.value = err?.message || "回滚失败";
  } finally {
    loading.value = false;
  }
};

onMounted(async () => {
  if (!(await ensureEmbeddingReady())) return;
  embeddingReady.value = true;
  await refreshPolicies();
});
</script>

<template>
  <section class="space-y-6 px-6 py-8">
    <header class="space-y-2">
      <p class="text-sm text-gray-500">Knowledge Space</p>
      <h1 class="text-2xl font-semibold text-gray-900">租户灰度发布与治理</h1>
      <p class="text-gray-600">
        管理租户发布矩阵，发布/推进/暂停/回滚批次，并查看版本 drift 与覆盖率。
      </p>
    </header>

    <div class="grid gap-4 md:grid-cols-3">
      <UCard>
        <p class="text-sm text-gray-500">策略数量</p>
        <p class="text-lg font-semibold text-gray-900">{{ policies.length }}</p>
      </UCard>
      <UCard>
        <p class="text-sm text-gray-500">当前 policyId</p>
        <p class="text-lg font-semibold text-gray-900">{{ selectedPolicyId || "未选择" }}</p>
      </UCard>
      <UCard>
        <p class="text-sm text-gray-500">当前 versionId</p>
        <p class="text-lg font-semibold text-gray-900">{{ versionId || "未设置" }}</p>
      </UCard>
    </div>

    <div v-if="errorText" class="rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-700">
      {{ errorText }}
    </div>
    <div v-if="successText" class="rounded-lg border border-emerald-200 bg-emerald-50 p-3 text-sm text-emerald-700">
      {{ successText }}
    </div>

    <UCard :ui="{ body: { padding: 'p-6 space-y-4' } }">
      <template #header>
        <div class="flex items-center justify-between">
          <div>
            <h2 class="text-xl font-semibold text-gray-900">策略矩阵</h2>
            <p class="text-sm text-gray-500">选择已有策略或编辑 JSON 后保存</p>
          </div>
          <div class="flex items-center gap-2">
            <UButton :loading="loading" variant="ghost" @click="refreshPolicies">刷新</UButton>
            <UButton :loading="loading" color="primary" @click="upsertPolicy">保存策略</UButton>
          </div>
        </div>
      </template>

      <div class="grid gap-4 md:grid-cols-3">
        <label class="flex flex-col gap-2">
          <span class="text-sm font-medium text-gray-800">选择 policyId</span>
          <select
            class="rounded-lg border border-gray-200 px-3 py-2 text-sm shadow-sm focus:border-primary-500 focus:outline-none"
            :value="selectedPolicyId"
            @change="
              selectedPolicyId = String(($event.target as HTMLSelectElement).value);
              hydrateMatrixFromPolicy();
            "
          >
            <option value="">未选择</option>
            <option v-for="p in policies" :key="p.id" :value="String(p.id)">
              {{ p.id }} · {{ p.matrixVersion }} · {{ p.status }}
            </option>
          </select>
        </label>

        <label class="flex flex-col gap-2">
          <span class="text-sm font-medium text-gray-800">versionId</span>
          <input
            type="text"
            class="rounded-lg border border-gray-200 px-3 py-2 text-sm shadow-sm focus:border-primary-500 focus:outline-none"
            placeholder="ver-2025.02"
            :value="versionId"
            @input="versionId = String(($event.target as HTMLInputElement).value)"
          />
        </label>

        <label class="flex flex-col gap-2">
          <span class="text-sm font-medium text-gray-800">回滚原因</span>
          <input
            type="text"
            class="rounded-lg border border-gray-200 px-3 py-2 text-sm shadow-sm focus:border-primary-500 focus:outline-none"
            placeholder="metrics breached"
            :value="rollbackReason"
            @input="rollbackReason = String(($event.target as HTMLInputElement).value)"
          />
        </label>
      </div>

      <label class="flex flex-col gap-2">
        <span class="text-sm font-medium text-gray-800">矩阵 JSON</span>
        <textarea
          class="min-h-[220px] rounded-lg border border-gray-200 px-3 py-2 font-mono text-xs shadow-sm focus:border-primary-500 focus:outline-none"
          spellcheck="false"
          :value="matrixText"
          @input="matrixText = String(($event.target as HTMLTextAreaElement).value)"
        />
      </label>
    </UCard>

    <UCard :ui="{ body: { padding: 'p-6 space-y-4' } }">
      <template #header>
        <div class="flex items-center justify-between">
          <div>
            <h2 class="text-xl font-semibold text-gray-900">灰度执行</h2>
            <p class="text-sm text-gray-500">发布 → 推进批次 → 回滚，并查看状态</p>
          </div>
          <div class="flex items-center gap-2">
            <UButton :loading="loading" variant="ghost" @click="refreshStatus">刷新状态</UButton>
            <UButton :loading="loading" color="primary" @click="publish">发布</UButton>
            <UButton :loading="loading" color="gray" @click="promote()">推进</UButton>
            <UButton :loading="loading" color="red" @click="rollback">回滚</UButton>
          </div>
        </div>
      </template>

      <div v-if="status" class="grid gap-4 md:grid-cols-3">
        <UCard>
          <p class="text-sm text-gray-500">grayState</p>
          <p class="text-lg font-semibold text-gray-900">{{ status.grayState }}</p>
        </UCard>
        <UCard>
          <p class="text-sm text-gray-500">tenantCoverage</p>
          <p class="text-lg font-semibold text-gray-900">{{ status.tenantCoverage }}</p>
        </UCard>
        <UCard>
          <p class="text-sm text-gray-500">versionDrift</p>
          <p class="text-lg font-semibold text-gray-900">{{ status.versionDrift }}</p>
        </UCard>
      </div>

      <div v-if="status?.alerts?.length" class="rounded-lg border border-amber-200 bg-amber-50 p-3 text-sm text-amber-700">
        Alerts：{{ status.alerts.join(", ") }}
      </div>

      <div v-if="status?.batches?.length" class="space-y-2">
        <p class="text-sm font-medium text-gray-800">批次列表（点击选择 batchToken）</p>
        <div class="grid gap-2">
          <button
            v-for="b in status.batches"
            :key="b.batchToken"
            type="button"
            class="w-full rounded-lg border px-3 py-2 text-left text-sm shadow-sm"
            :class="b.batchToken === selectedBatchToken ? 'border-primary-300 bg-primary-50' : 'border-gray-200 bg-white'"
            @click="selectedBatchToken = b.batchToken"
          >
            <div class="flex items-center justify-between">
              <div class="font-semibold text-gray-900">
                #{{ b.batchIndex }} · {{ b.state }}
              </div>
              <div class="text-xs text-gray-500">{{ b.batchToken.slice(0, 8) }}…</div>
            </div>
            <div class="mt-1 text-xs text-gray-600">
              tenants={{ b.tenants?.length || 0 }}
              <span v-if="b.alerts?.length"> · alerts={{ b.alerts.join(";") }}</span>
            </div>
          </button>
        </div>
      </div>
    </UCard>
  </section>
</template>
