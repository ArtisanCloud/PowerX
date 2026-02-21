<script setup lang="ts">
import { computed, onActivated, onMounted, reactive, ref, watch } from "vue";
import { storeToRefs } from "pinia";
import { useUserStore } from "~/stores/user";
import {
  useEventFabricService,
  type EventFabricAclTopicMatrixResult,
  type EventFabricAclPrincipalMatrixResult,
} from "~/composables/api/services/eventFabricService";

definePageMeta({
  title: "事件权限",
  icon: "i-heroicons-shield-check",
  order: 11,
});

interface TopicOption {
  label: string;
  value: string;
  namespace: string;
  name: string;
  scopeType: string;
  scopeId: string;
}

const route = useRoute();
const userStore = useUserStore();
const { isRoot } = storeToRefs(userStore);
const allowAccess = computed(() => Boolean(isRoot.value));
const svc = useEventFabricService();
const toast = useToast();

const loading = ref(false);
const topicLoading = ref(false);
const topicOptions = ref<TopicOption[]>([]);
const totalTopics = ref(0);
const selectedTopic = ref("");

const topicMatrix = ref<EventFabricAclTopicMatrixResult | null>(null);
const principalMatrix = ref<EventFabricAclPrincipalMatrixResult | null>(null);

const filters = reactive({
  namespace: "",
  name: "",
  principalId: "",
});

const grantForm = reactive({
  principalType: "role",
  principalId: "",
  action: "replay",
  operatorId: "web-admin",
});

const topicFullName = computed(() => {
  const selected = String(selectedTopic.value || "").trim();
  if (selected) return selected;
  const full = String(topicMatrix.value?.topic?.full_topic || "").trim();
  return full;
});

const principalRows = computed(() => topicMatrix.value?.principals || []);
const principalTopicRows = computed(() => principalMatrix.value?.topics || []);
const selectedTopicMeta = computed(() => topicOptions.value.find((opt) => opt.value === selectedTopic.value));

function scopeTypeLabel(scopeType: string): string {
  switch (String(scopeType || "").trim().toLowerCase()) {
    case "system":
      return "系统级";
    case "tenant":
      return "租户级";
    case "plugin":
      return "插件级";
    case "third_party":
      return "第三方级";
    default:
      return "未定义";
  }
}

function parseSemanticTopic(topicKey: string) {
  const value = String(topicKey || "").trim();
  if (!value) return { namespace: "", name: "" };
  const parts = value.split(".").map((part) => part.trim()).filter(Boolean);
  if (parts.length < 2) return { namespace: "", name: "" };
  return {
    namespace: parts.slice(0, parts.length - 1).join("."),
    name: parts[parts.length - 1],
  };
}

function inferPrincipalType(principalId: string): string {
  const value = String(principalId || "").trim();
  if (!value.includes(":")) return "role";
  const token = value.split(":", 1)[0]?.trim().toLowerCase();
  if (["role", "member", "user", "service"].includes(token)) return token;
  return "role";
}

function applyTopicSelection(fullTopic: string) {
  const value = String(fullTopic || "").trim();
  selectedTopic.value = value;
  if (!value) {
    filters.namespace = "";
    filters.name = "";
    return;
  }
  const parts = value.split(".").map((part) => part.trim()).filter(Boolean);
  if (parts.length >= 3) {
    filters.namespace = parts.slice(1, parts.length - 1).join(".");
    filters.name = parts[parts.length - 1];
    return;
  }
  const parsed = parseSemanticTopic(value);
  filters.namespace = parsed.namespace;
  filters.name = parsed.name;
}

function ensureTopicSelected(): boolean {
  if (!filters.namespace || !filters.name) {
    toast.add({ title: "请先选择 Topic", description: "先从下拉框选择一个已注册 Topic", color: "warning" });
    return false;
  }
  return true;
}

async function loadTopics() {
  topicLoading.value = true;
  try {
    const toOptions = (rows: any[]): TopicOption[] => {
      return rows
        .map((item: any) => {
          const full = String(item?.full_topic || "").trim();
          const namespace = String(item?.namespace || "").trim();
          const name = String(item?.name || "").trim();
          const scopeType = String(item?.scope_type || "").trim().toLowerCase() || "tenant";
          const scopeId = String(item?.scope_id || item?.tenant_key || "").trim();
          if (!full || !namespace || !name) return null;
          return {
            label: `[${scopeTypeLabel(scopeType)}] ${full}`,
            value: full,
            namespace,
            name,
            scopeType,
            scopeId,
          } as TopicOption;
        })
        .filter(Boolean) as TopicOption[];
    };

    const res = await svc.listTopics({ page: 1, page_size: 200 });
    const data = res?.data || {};
    const items = Array.isArray(data?.items) ? data.items : [];
    let options = toOptions(items);
    let total = Number(data?.total || options.length || 0);

    // 兜底：部分环境 /topics 受租户过滤影响返回 0，改从 overview 同步 Topic。
    if (options.length === 0) {
      try {
        const ov = await svc.getOverview({ limit: 1 });
        const topics = Array.isArray(ov?.data?.topics) ? ov.data.topics : [];
        const fallback = toOptions(topics);
        if (fallback.length > 0) {
          options = fallback;
          total = fallback.length;
        }
      } catch {
      }
    }

    totalTopics.value = total;
    topicOptions.value = options;
    if (options.length === 0) {
      toast.add({
        title: "未读取到 Topic",
        description: "后端返回 0 条 Topic，请检查 /admin/event-fabric/topics 与当前租户上下文。",
        color: "warning",
      });
    }
  } catch (err: any) {
    toast.add({ title: "加载 Topic 列表失败", description: err?.message || "未知错误", color: "error" });
  } finally {
    topicLoading.value = false;
  }
}

async function refreshTopicMatrix() {
  if (topicOptions.value.length === 0) {
    await loadTopics();
  }
  if (!ensureTopicSelected()) return;
  loading.value = true;
  try {
    const res = await svc.getAclTopicMatrix({
      namespace: filters.namespace,
      name: filters.name,
    });
    topicMatrix.value = res.data;
  } catch (err: any) {
    toast.add({
      title: "加载 Topic 权限失败",
      description: err?.message || "未知错误",
      color: "error",
    });
  } finally {
    loading.value = false;
  }
}

async function refreshPrincipalMatrix() {
  if (!ensureTopicSelected()) return;
  if (!filters.principalId.trim()) {
    toast.add({ title: "请先输入主体ID", color: "warning" });
    return;
  }
  loading.value = true;
  try {
    const res = await svc.getAclPrincipalMatrix({
      principal_id: filters.principalId,
      namespace: filters.namespace,
      name: filters.name,
    });
    principalMatrix.value = res.data;
  } catch (err: any) {
    toast.add({
      title: "加载主体权限失败",
      description: err?.message || "未知错误",
      color: "error",
    });
  } finally {
    loading.value = false;
  }
}

async function grantAction() {
  if (!ensureTopicSelected()) return;
  const principalId = grantForm.principalId.trim();
  if (!principalId) {
    toast.add({ title: "请先输入主体ID", color: "warning" });
    return;
  }
  if (!topicFullName.value) {
    toast.add({ title: "缺少 Topic", description: "请先选择 Topic", color: "warning" });
    return;
  }
  loading.value = true;
  try {
    await svc.upsertAclBindings({
      topic_full_name: topicFullName.value,
      grants: [
        {
          principal_type: grantForm.principalType,
          principal_id: principalId,
          action: grantForm.action,
          operator_id: grantForm.operatorId,
          justification: "event acl governance ui",
        },
      ],
    });
    toast.add({ title: "授权成功", color: "success" });
    if (!filters.principalId) {
      filters.principalId = principalId;
    }
    await refreshTopicMatrix();
    await refreshPrincipalMatrix();
  } catch (err: any) {
    toast.add({ title: "授权失败", description: err?.message || "未知错误", color: "error" });
  } finally {
    loading.value = false;
  }
}

async function revokeAction(principalId: string, action: string) {
  if (!topicFullName.value) {
    toast.add({ title: "缺少 Topic", description: "请先选择 Topic", color: "warning" });
    return;
  }
  loading.value = true;
  try {
    await svc.upsertAclBindings({
      topic_full_name: topicFullName.value,
      revokes: [
        {
          principal_type: inferPrincipalType(principalId),
          principal_id: principalId,
          action,
          operator_id: grantForm.operatorId,
        },
      ],
    });
    toast.add({ title: "撤销成功", color: "success" });
    await refreshTopicMatrix();
    await refreshPrincipalMatrix();
  } catch (err: any) {
    toast.add({ title: "撤销失败", description: err?.message || "未知错误", color: "error" });
  } finally {
    loading.value = false;
  }
}

function actionEnabled(actions: Record<string, boolean>, action: string): boolean {
  return Boolean(actions?.[action]);
}

async function handleTopicChange(value: string) {
  applyTopicSelection(value);
  topicMatrix.value = null;
  principalMatrix.value = null;
  if (!filters.namespace || !filters.name) return;
  await refreshTopicMatrix();
  if (filters.principalId.trim()) {
    await refreshPrincipalMatrix();
  }
}

const initialized = ref(false);

async function bootstrapTopicData() {
  if (!allowAccess.value || initialized.value) return;
  await loadTopics();

  const fromTopic = String(route.query.topic_key || "").trim();
  if (fromTopic) {
    const semantic = parseSemanticTopic(fromTopic);
    const matched = topicOptions.value.find((opt) => opt.namespace === semantic.namespace && opt.name === semantic.name);
    if (matched) {
      await handleTopicChange(matched.value);
    } else {
      filters.namespace = semantic.namespace;
      filters.name = semantic.name;
    }
  } else if (topicOptions.value.length > 0) {
    await handleTopicChange(topicOptions.value[0].value);
  }
  initialized.value = true;
}

watch(
  allowAccess,
  async (enabled) => {
    if (!enabled) return;
    await bootstrapTopicData();
  },
  { immediate: true }
);

onMounted(async () => {
  try {
    await userStore.fetchUserContext({ force: true });
  } catch {
  }
  await bootstrapTopicData();
});

onActivated(async () => {
  await bootstrapTopicData();
});
</script>

<template>
  <div class="p-6 space-y-4">
    <div class="flex items-start justify-between gap-3">
      <div>
        <h1 class="text-2xl font-bold text-gray-900 dark:text-white">事件权限（Event ACL）</h1>
        <p class="text-sm text-gray-600 dark:text-gray-400">二级页面：按 Topic 维护角色/主体 ACL 授权。</p>
      </div>
      <UButton variant="outline" icon="i-heroicons-arrow-left" @click="navigateTo('/settings/event-fabric')">返回事件管理</UButton>
    </div>

    <UAlert
      v-if="!allowAccess"
      icon="i-heroicons-lock-closed"
      color="amber"
      variant="subtle"
      title="无权限"
      description="仅 Root 管理员可管理事件权限。"
    />

    <template v-else>
      <UCard>
        <div class="grid grid-cols-1 gap-3 md:grid-cols-4">
          <USelect
            v-model="selectedTopic"
            :items="topicOptions"
            :loading="topicLoading"
            label="Topic"
            placeholder="请选择 Topic"
            @update:model-value="handleTopicChange"
          />
          <UInput v-model="filters.principalId" label="主体ID" placeholder="role:role_admin" />
          <div class="flex items-end text-xs text-gray-500">已注册 Topic：{{ totalTopics }}</div>
          <div class="flex items-end text-xs text-gray-500">页面自动同步 Topic 与权限数据</div>
        </div>
      </UCard>

      <UCard>
        <template #header>
          <div class="flex items-center justify-between gap-2">
            <div class="font-semibold">当前 Topic</div>
            <div v-if="topicFullName" class="flex items-center gap-2">
              <UBadge
                v-if="selectedTopicMeta"
                variant="subtle"
                color="warning"
              >
                {{ scopeTypeLabel(selectedTopicMeta.scopeType) }} / {{ selectedTopicMeta.scopeId || "-" }}
              </UBadge>
              <UBadge variant="subtle" color="primary">{{ topicFullName }}</UBadge>
            </div>
            <UBadge v-else variant="subtle" color="neutral">未选择</UBadge>
          </div>
        </template>

        <div class="grid grid-cols-1 gap-3 md:grid-cols-5">
          <div class="space-y-1">
            <div class="text-xs text-gray-500">主体类型（不是 Topic 类型）</div>
            <USelect
              v-model="grantForm.principalType"
              :items="[
                { label: '角色', value: 'role' },
                { label: '成员', value: 'member' },
                { label: '用户', value: 'user' },
                { label: '服务', value: 'service' },
              ]"
              placeholder="请选择主体类型"
            />
          </div>
          <div class="space-y-1">
            <div class="text-xs text-gray-500">主体ID</div>
            <UInput v-model="grantForm.principalId" placeholder="如：role:role_admin" />
          </div>
          <div class="space-y-1">
            <div class="text-xs text-gray-500">授权动作</div>
            <USelect
              v-model="grantForm.action"
              :items="[
                { label: 'publish', value: 'publish' },
                { label: 'subscribe', value: 'subscribe' },
                { label: 'replay', value: 'replay' },
              ]"
              placeholder="请选择动作"
            />
          </div>
          <div class="space-y-1">
            <div class="text-xs text-gray-500">操作人</div>
            <UInput v-model="grantForm.operatorId" placeholder="如：web-admin / role:role_admin" />
          </div>
          <div class="flex items-end">
            <UButton class="w-full" :loading="loading" color="primary" @click="grantAction">授予权限</UButton>
          </div>
        </div>
      </UCard>

      <UCard>
        <template #header><div class="font-semibold">Topic 权限矩阵</div></template>
        <div class="overflow-x-auto">
          <table class="w-full text-sm">
            <thead>
              <tr class="border-b border-gray-200 dark:border-gray-700">
                <th class="text-left px-3 py-2">主体</th>
                <th class="text-left px-3 py-2">publish</th>
                <th class="text-left px-3 py-2">subscribe</th>
                <th class="text-left px-3 py-2">replay</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="row in principalRows"
                :key="row.principal_id"
                class="border-t border-gray-200 dark:border-gray-700"
              >
                <td class="px-3 py-2 font-mono">{{ row.principal_id }}</td>
                <td class="px-3 py-2">
                  <UButton
                    size="xs"
                    :variant="actionEnabled(row.actions, 'publish') ? 'soft' : 'outline'"
                    :color="actionEnabled(row.actions, 'publish') ? 'success' : 'gray'"
                    @click="revokeAction(row.principal_id, 'publish')"
                  >
                    {{ actionEnabled(row.actions, 'publish') ? '已授权（点撤销）' : '未授权' }}
                  </UButton>
                </td>
                <td class="px-3 py-2">
                  <UButton
                    size="xs"
                    :variant="actionEnabled(row.actions, 'subscribe') ? 'soft' : 'outline'"
                    :color="actionEnabled(row.actions, 'subscribe') ? 'success' : 'gray'"
                    @click="revokeAction(row.principal_id, 'subscribe')"
                  >
                    {{ actionEnabled(row.actions, 'subscribe') ? '已授权（点撤销）' : '未授权' }}
                  </UButton>
                </td>
                <td class="px-3 py-2">
                  <UButton
                    size="xs"
                    :variant="actionEnabled(row.actions, 'replay') ? 'soft' : 'outline'"
                    :color="actionEnabled(row.actions, 'replay') ? 'success' : 'gray'"
                    @click="revokeAction(row.principal_id, 'replay')"
                  >
                    {{ actionEnabled(row.actions, 'replay') ? '已授权（点撤销）' : '未授权' }}
                  </UButton>
                </td>
              </tr>
              <tr v-if="principalRows.length === 0">
                <td class="px-3 py-3 text-gray-500" colspan="4">暂无授权记录</td>
              </tr>
            </tbody>
          </table>
        </div>
      </UCard>

      <UCard>
        <template #header><div class="font-semibold">主体反查视图</div></template>
        <div class="text-xs text-gray-500 mb-2">按主体查看其在当前筛选 Topic 上的权限。</div>
        <div class="overflow-x-auto">
          <table class="w-full text-sm">
            <thead>
              <tr class="border-b border-gray-200 dark:border-gray-700">
                <th class="text-left px-3 py-2">主体</th>
                <th class="text-left px-3 py-2">Topic</th>
                <th class="text-left px-3 py-2">Actions</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="row in principalTopicRows"
                :key="`${principalMatrix?.principal_id}-${row.topic_uuid}`"
                class="border-t border-gray-200 dark:border-gray-700"
              >
                <td class="px-3 py-2 font-mono">{{ principalMatrix?.principal_id }}</td>
                <td class="px-3 py-2 font-mono">{{ row.topic }}</td>
                <td class="px-3 py-2">{{ row.actions.join(', ') || '-' }}</td>
              </tr>
              <tr v-if="principalTopicRows.length === 0">
                <td class="px-3 py-3 text-gray-500" colspan="3">该主体在当前筛选下暂无权限</td>
              </tr>
            </tbody>
          </table>
        </div>
      </UCard>
    </template>
  </div>
</template>
