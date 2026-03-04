<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from "vue";
import { storeToRefs } from "pinia";
import { useUserStore } from "~/stores/user";
import {
  useEventFabricService,
  type EventFabricTopic,
} from "~/composables/api/services/eventFabricService";

definePageMeta({
  title: "事件管理",
  icon: "i-heroicons-queue-list",
  order: 10,
});

const userStore = useUserStore();
const { isRoot } = storeToRefs(userStore);
const allowAccess = computed(() => Boolean(isRoot.value));
const toast = useToast();
const svc = useEventFabricService();
const router = useRouter();
const localePath = useLocalePath();

const loading = ref(false);
const creating = ref(false);
const lifecycleLoadingId = ref("");
const createModalOpen = ref(false);

const topics = ref<EventFabricTopic[]>([]);
const total = ref(0);

const filters = reactive({
  namespace: "",
  lifecycle: "__all__",
  tenantUUID: "",
});

const createForm = reactive({
  namespace: "",
  name: "",
  payloadFormat: "json",
  maxRetry: 5,
  ackTimeoutSec: 30,
  versioningMode: "strict",
  retentionPolicy: '{"mode":"standard"}',
  createdBy: "web-admin",
});

const lifecycleOptions = [
  { label: "全部状态", value: "__all__" },
  { label: "active", value: "active" },
  { label: "deprecated", value: "deprecated" },
  { label: "retired", value: "retired" },
];

function normalizeLifecycleFilterValue(raw: unknown): string {
  if (typeof raw === "string") {
    return raw.trim();
  }
  if (raw && typeof raw === "object") {
    const value = (raw as any).value;
    if (typeof value === "string") {
      return value.trim();
    }
  }
  return "";
}

const canSearchByTenant = computed(() => allowAccess.value);

function toTopicKey(topic: EventFabricTopic): string {
  return `${topic.namespace}.${topic.name}`;
}

function lifecycleLabel(lifecycle: string): string {
  const value = String(lifecycle || "").trim().toLowerCase();
  if (value === "active") return "启用";
  if (value === "deprecated") return "弃用";
  if (value === "retired") return "退役";
  return value || "-";
}

function scopeTypeLabel(scopeType?: string): string {
  const value = String(scopeType || "").trim().toLowerCase();
  if (value === "system") return "系统";
  if (value === "tenant") return "租户";
  if (value === "plugin") return "插件";
  if (value === "third_party") return "第三方";
  return value || "-";
}

async function loadTopics(force = false) {
  if (!force && !allowAccess.value) return;
  loading.value = true;
  try {
    const lifecycleFilter = normalizeLifecycleFilterValue(filters.lifecycle);
    const response = await svc.listTopics({
      namespace: String(filters.namespace || "").trim() || undefined,
      lifecycle: lifecycleFilter === "__all__" ? undefined : lifecycleFilter || undefined,
      tenant_uuid:
        canSearchByTenant.value && String(filters.tenantUUID || "").trim()
          ? String(filters.tenantUUID).trim()
          : undefined,
      page: 1,
      page_size: 200,
    });
    const data = response?.data || {};
    topics.value = Array.isArray(data?.items) ? data.items : [];
    total.value = Number(data?.total || topics.value.length || 0);
  } catch (err: any) {
    toast.add({
      title: "加载 Topic 失败",
      description: err?.message || "未知错误",
      color: "error",
    });
  } finally {
    loading.value = false;
  }
}

function resetFilters() {
  filters.namespace = "";
  filters.lifecycle = "__all__";
  filters.tenantUUID = "";
  loadTopics();
}

async function createTopic() {
  const namespace = String(createForm.namespace || "").trim();
  const name = String(createForm.name || "").trim();
  if (!namespace || !name) {
    toast.add({ title: "请填写 namespace 和 name", color: "warning" });
    return;
  }

  creating.value = true;
  try {
    await svc.createTopic({
      namespace,
      name,
      payload_format: String(createForm.payloadFormat || "json").trim() || "json",
      max_retry: Number(createForm.maxRetry || 0) || 5,
      ack_timeout_sec: Number(createForm.ackTimeoutSec || 0) || 30,
      versioning_mode:
        String(createForm.versioningMode || "strict").trim() || "strict",
      retention_policy:
        String(createForm.retentionPolicy || '{"mode":"standard"}').trim() ||
        '{"mode":"standard"}',
      created_by: String(createForm.createdBy || "web-admin").trim() || "web-admin",
    });

    toast.add({ title: "Topic 创建成功", color: "success" });
    createForm.namespace = "";
    createForm.name = "";
    createModalOpen.value = false;
    await loadTopics();
  } catch (err: any) {
    toast.add({
      title: "Topic 创建失败",
      description: err?.message || "未知错误",
      color: "error",
    });
  } finally {
    creating.value = false;
  }
}

async function changeLifecycle(topic: EventFabricTopic, target: "active" | "deprecated" | "retired") {
  if (!topic?.id) return;
  if (topic.lifecycle === target) return;

  if (target === "retired") {
    const confirmed = window.confirm(`确认将 Topic ${topic.full_topic} 退役吗？`);
    if (!confirmed) return;
  }

  lifecycleLoadingId.value = topic.id;
  try {
    await svc.updateTopicLifecycle(topic.id, {
      target_state: target,
      change_reason: "web-admin topic lifecycle update",
    });
    toast.add({ title: `已更新为 ${target}`, color: "success" });
    await loadTopics();
  } catch (err: any) {
    toast.add({
      title: "更新生命周期失败",
      description: err?.message || "未知错误",
      color: "error",
    });
  } finally {
    lifecycleLoadingId.value = "";
  }
}

async function goAcl(topic: EventFabricTopic) {
  try {
    const target = localePath({
      path: "/settings/event-acl",
      query: { topic_key: toTopicKey(topic) },
    } as any);
    await router.push(target as any);
  } catch (err: any) {
    toast.add({
      title: "跳转权限配置失败",
      description: err?.message || "未知错误",
      color: "error",
    });
  }
}

onMounted(async () => {
  try {
    await userStore.fetchUserContext({ force: true });
  } catch {
  }
  await loadTopics(true);
});

watch(
  () => allowAccess.value,
  (enabled) => {
    if (enabled) {
      void loadTopics();
    }
  },
  { immediate: true }
);
</script>

<template>
  <div class="p-6 space-y-4">
    <div class="flex items-start justify-between gap-3">
      <div>
        <h1 class="text-2xl font-bold text-gray-900 dark:text-white">事件管理</h1>
        <p class="text-sm text-gray-600 dark:text-gray-400">管理 Topic 目录（新增、查询、生命周期变更），权限授权在二级页面维护。</p>
      </div>
      <UButton v-if="allowAccess" color="primary" icon="i-heroicons-plus" @click="createModalOpen = true">新建 Topic</UButton>
    </div>

    <UAlert
      v-if="!allowAccess"
      icon="i-heroicons-lock-closed"
      color="amber"
      variant="subtle"
      title="无权限"
      description="仅 Root 管理员可管理事件目录。"
    />

    <template v-else>
      <UModal v-model:open="createModalOpen" title="新建 Topic" description="创建新的事件 Topic">
        <template #content>
          <UCard>
            <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
              <UInput v-model="createForm.namespace" label="namespace" placeholder="_topic.knowledge.space.feedback" />
              <UInput v-model="createForm.name" label="name" placeholder="reprocess" />
              <UInput v-model="createForm.payloadFormat" label="payload_format" placeholder="json" />
              <UInput v-model="createForm.versioningMode" label="versioning_mode" placeholder="strict" />
              <UInput v-model="createForm.maxRetry" type="number" label="max_retry" />
              <UInput v-model="createForm.ackTimeoutSec" type="number" label="ack_timeout_sec" />
              <UInput v-model="createForm.createdBy" label="created_by" placeholder="web-admin" />
              <UInput v-model="createForm.retentionPolicy" label="retention_policy(JSON)" placeholder='{"mode":"standard"}' />
            </div>
            <div class="mt-4 flex justify-end gap-2">
              <UButton variant="ghost" @click="createModalOpen = false">取消</UButton>
              <UButton color="primary" :loading="creating" @click="createTopic">创建 Topic</UButton>
            </div>
          </UCard>
        </template>
      </UModal>

      <UCard>
        <template #header>
          <div class="flex items-center justify-between gap-2">
            <div class="font-semibold">Topic 列表</div>
            <UBadge variant="subtle" color="primary">{{ total }}</UBadge>
          </div>
        </template>

        <div class="grid grid-cols-1 gap-3 md:grid-cols-4">
          <UInput v-model="filters.namespace" label="namespace" placeholder="按事件域筛选" />
          <USelect v-model="filters.lifecycle" :items="lifecycleOptions" label="lifecycle" />
          <UInput v-model="filters.tenantUUID" :disabled="!canSearchByTenant" label="tenant_uuid（可选）" placeholder="root 可按租户查询" />
          <div class="flex items-end gap-2">
            <UButton variant="outline" :loading="loading" @click="loadTopics">查询</UButton>
            <UButton variant="ghost" @click="resetFilters">清空</UButton>
          </div>
        </div>

        <div class="mt-2 text-xs text-gray-500">操作说明：当前为“启用”仅可“弃用”；当前非“启用”仅可“启用”。“权限配置”进入 ACL 页面。</div>

        <div class="mt-3 overflow-x-auto">
          <table class="w-full text-sm">
            <thead>
              <tr class="border-b border-gray-200 dark:border-gray-700">
                <th class="px-3 py-2 text-left">Topic</th>
                <th class="px-3 py-2 text-left">Tenant</th>
                <th class="px-3 py-2 text-left">ScopeType</th>
                <th class="px-3 py-2 text-left">Lifecycle</th>
                <th class="px-3 py-2 text-left">策略</th>
                <th class="px-3 py-2 text-left">操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="topic in topics" :key="topic.id" class="border-t border-gray-200 dark:border-gray-700">
                <td class="px-3 py-2 font-mono">{{ topic.full_topic }}</td>
                <td class="px-3 py-2 font-mono">{{ topic.tenant_key || topic.tenant_uuid || '-' }}</td>
                <td class="px-3 py-2">{{ scopeTypeLabel(topic.scope_type) }}</td>
                <td class="px-3 py-2">
                  <UBadge
                    :color="topic.lifecycle === 'active' ? 'success' : topic.lifecycle === 'deprecated' ? 'warning' : 'neutral'"
                    variant="subtle"
                  >
                    {{ lifecycleLabel(topic.lifecycle) }}
                  </UBadge>
                </td>
                <td class="px-3 py-2 text-xs text-gray-500">
                  retry={{ topic.max_retry }} / ack={{ topic.ack_timeout_sec }}s / {{ topic.versioning_mode }}
                </td>
                <td class="px-3 py-2">
                  <div class="flex flex-wrap gap-1">
                    <UButton
                      v-if="topic.lifecycle === 'active'"
                      size="xs"
                      variant="outline"
                      color="warning"
                      :loading="lifecycleLoadingId === topic.id"
                      title="将生命周期设置为 deprecated"
                      @click="changeLifecycle(topic, 'deprecated')"
                    >
                      弃用
                    </UButton>
                    <UButton
                      v-else
                      size="xs"
                      variant="outline"
                      :loading="lifecycleLoadingId === topic.id"
                      title="将生命周期设置为 active"
                      @click="changeLifecycle(topic, 'active')"
                    >
                      启用
                    </UButton>
                    <UButton size="xs" color="primary" title="进入该 Topic 的权限配置页面" @click="goAcl(topic)">权限配置</UButton>
                  </div>
                </td>
              </tr>
              <tr v-if="topics.length === 0">
                <td colspan="6" class="px-3 py-3 text-gray-500">暂无 Topic 记录</td>
              </tr>
            </tbody>
          </table>
        </div>
      </UCard>
    </template>
  </div>
</template>
