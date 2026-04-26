<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from "vue";
import { storeToRefs } from "pinia";
import { useUserStore } from "~/stores/user";
import {
  useIntegrationGatewayApiKeyService,
  type IntegrationGatewayApiKeyRecord,
  type IntegrationGatewayApiKeyProfile,
  type IntegrationGatewayPermissionCatalogItem,
} from "~/composables/api/services/integrationGatewayApiKeyService";

definePageMeta({
  title: "API Key 管理",
  icon: "i-heroicons-key",
  order: 12,
});

type PermissionActionGroup = {
  actionName: string;
  permissions: IntegrationGatewayPermissionCatalogItem[];
};

type PermissionResourceGroup = {
  resourceName: string;
  actions: PermissionActionGroup[];
};

const userStore = useUserStore();
const { isRoot, isCurrentTenantAdmin, currentTenantUuid, currentTenant, memberTenants } = storeToRefs(userStore);
const allowAccess = computed(() => Boolean(isRoot.value || isCurrentTenantAdmin.value));
const canSwitchTenant = computed(() => Boolean(isRoot.value));

const toast = useToast();
const svc = useIntegrationGatewayApiKeyService();

const loading = ref(false);
const switchingTenant = ref(false);
const creatingProfile = ref(false);
const updatingProfileID = ref<number | null>(null);
const creatingKey = ref(false);
const rotatingKeyID = ref("");
const revokingKeyID = ref("");
const deletingKeyID = ref("");
const savingPermissions = ref(false);
const loadingPermissions = ref(false);
let refreshInFlight: Promise<void> | null = null;

const tenantUUID = ref("");
const selectedTenantUUID = ref("");
const searchProfile = ref("");
const searchPermission = ref("");

const apiKeyProfiles = ref<IntegrationGatewayApiKeyProfile[]>([]);
const apiKeys = ref<IntegrationGatewayApiKeyRecord[]>([]);
const permissionCatalog = ref<IntegrationGatewayPermissionCatalogItem[]>([]);

const selectedProfileID = ref<number | null>(null);
const selectedPermissionIDs = ref<number[]>([]);
const initialPermissionIDs = ref<number[]>([]);

const latestPlainKey = ref("");
const latestPlainKeyLabel = ref("");

const createProfileOpen = ref(false);
const renameProfileOpen = ref(false);
const createKeyOpen = ref(false);
const rotateOpen = ref(false);
const rotateTarget = ref<IntegrationGatewayApiKeyRecord | null>(null);

const createProfileForm = reactive({
  name: "",
  key: "",
});

const renameProfileForm = reactive({
  name: "",
});

const createKeyForm = reactive({
  name: "",
  description: "",
  expiresAt: "",
});

const rotateForm = reactive({
  name: "",
  description: "",
  expiresAt: "",
});

const tenantOptions = computed(() =>
  (memberTenants.value || []).map((item) => ({
    label: item.tenant_name || item.tenant_uuid,
    value: item.tenant_uuid,
    description: item.is_admin ? "admin" : "member",
  }))
);

const effectiveTenantUUID = computed(() => {
  const fromContext = String(currentTenantUuid.value || "").trim();
  if (fromContext) return fromContext;
  return String(tenantUUID.value || "").trim();
});

const hasValidTenant = computed(() => /^[0-9a-fA-F-]{36}$/.test(effectiveTenantUUID.value));

const selectedProfile = computed(
  () => apiKeyProfiles.value.find((item) => item.id === selectedProfileID.value) || null
);

const filteredProfiles = computed(() => {
  const keyword = searchProfile.value.trim().toLowerCase();
  if (!keyword) return apiKeyProfiles.value;
  return apiKeyProfiles.value.filter((item) => {
    const text = `${item.name} ${item.key} #${item.id}`.toLowerCase();
    return text.includes(keyword);
  });
});

const keyCountMap = computed<Record<number, number>>(() => {
  const out: Record<number, number> = {};
  for (const item of apiKeys.value) {
    const profileID = Number(item.profile_id || 0);
    if (!profileID) continue;
    out[profileID] = (out[profileID] || 0) + 1;
  }
  return out;
});

const profileKeys = computed(() => {
  if (!selectedProfileID.value) return [];
  return apiKeys.value.filter((item) => Number(item.profile_id) === Number(selectedProfileID.value));
});

const permissionGroups = computed<PermissionResourceGroup[]>(() => {
  const grouped = new Map<string, Map<string, IntegrationGatewayPermissionCatalogItem[]>>();
  for (const item of permissionCatalog.value) {
    const resourceName = String(item.resource || "unknown_resource");
    const actionName = String(item.action || "unknown_action");
    if (!grouped.has(resourceName)) grouped.set(resourceName, new Map());
    const actionMap = grouped.get(resourceName)!;
    if (!actionMap.has(actionName)) actionMap.set(actionName, []);
    actionMap.get(actionName)!.push(item);
  }
  return Array.from(grouped.entries())
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([resourceName, actions]) => ({
      resourceName,
      actions: Array.from(actions.entries())
        .sort(([left], [right]) => left.localeCompare(right))
        .map(([actionName, permissions]) => ({
          actionName,
          permissions: permissions.sort((a, b) => Number(a.id) - Number(b.id)),
        })),
    }));
});

function metaString(value: unknown): string {
  if (!value) return "";
  if (typeof value === "string") return value;
  if (typeof value === "number" || typeof value === "boolean") return String(value);
  return "";
}

function permissionCapabilityID(item: IntegrationGatewayPermissionCatalogItem): string {
  return String(
    metaString(item.meta?.capability_id) ||
      metaString(item.meta?.permission_code) ||
      metaString(item.meta?.code)
  ).trim();
}

function permissionSearchText(item: IntegrationGatewayPermissionCatalogItem): string {
  const title = permissionTitle(item);
  const capabilityID = permissionCapabilityID(item);
  const metaText = item.meta ? JSON.stringify(item.meta) : "";
  return [
    title,
    capabilityID,
    item.module,
    item.resource,
    item.action,
    item.description || "",
    metaText,
  ]
    .join(" ")
    .toLowerCase();
}

const filteredPermissionGroups = computed<PermissionResourceGroup[]>(() => {
  const keyword = searchPermission.value.trim().toLowerCase();
  if (!keyword) return permissionGroups.value;
  const groups: PermissionResourceGroup[] = [];
  for (const resourceGroup of permissionGroups.value) {
    const actions: PermissionActionGroup[] = [];
    for (const actionGroup of resourceGroup.actions) {
      const permissions = actionGroup.permissions.filter((item) => permissionSearchText(item).includes(keyword));
      if (permissions.length > 0) {
        actions.push({
          actionName: actionGroup.actionName,
          permissions,
        });
      }
    }
    if (actions.length > 0) {
      groups.push({
        resourceName: resourceGroup.resourceName,
        actions,
      });
    }
  }
  return groups;
});

const allPermissionItems = computed<IntegrationGatewayPermissionCatalogItem[]>(() => permissionCatalog.value || []);

const totalPermissionCount = computed(() => allPermissionItems.value.length);

const totalCheckedPermissionCount = computed(() => checkedCount(allPermissionItems.value));

function permissionTitle(item: IntegrationGatewayPermissionCatalogItem) {
  const label = String(item.meta?.label || "").trim();
  if (label) return label;
  return `${item.resource}.${item.action}`;
}

const permissionDirty = computed(() => {
  const current = Array.from(new Set(selectedPermissionIDs.value)).sort((a, b) => a - b);
  const initial = Array.from(new Set(initialPermissionIDs.value)).sort((a, b) => a - b);
  if (current.length !== initial.length) return true;
  return current.some((item, index) => item !== initial[index]);
});

function normalizeSelectNumber(raw: unknown): number {
  if (typeof raw === "number") return raw;
  if (typeof raw === "string") return Number(raw) || 0;
  if (raw && typeof raw === "object") {
    const value = (raw as any).value;
    if (typeof value === "number") return value;
    if (typeof value === "string") return Number(value) || 0;
  }
  return 0;
}

function normalizeSelectString(raw: unknown): string {
  if (typeof raw === "string") return raw;
  if (raw && typeof raw === "object" && typeof (raw as any).value === "string") return (raw as any).value;
  return "";
}

function isChecked(permissionID: number) {
  return selectedPermissionIDs.value.includes(permissionID);
}

function setPermission(permissionID: number, checked: boolean) {
  const current = new Set(selectedPermissionIDs.value);
  if (checked) current.add(permissionID);
  else current.delete(permissionID);
  selectedPermissionIDs.value = Array.from(current);
}

function togglePermissions(items: IntegrationGatewayPermissionCatalogItem[], checked: boolean) {
  const current = new Set(selectedPermissionIDs.value);
  for (const item of items) {
    const permissionID = Number(item.id);
    if (!permissionID) continue;
    if (checked) current.add(permissionID);
    else current.delete(permissionID);
  }
  selectedPermissionIDs.value = Array.from(current);
}

function toggleAllPermissions(checked: boolean) {
  togglePermissions(allPermissionItems.value, checked);
}

function checkedCount(items: IntegrationGatewayPermissionCatalogItem[]) {
  return items.filter((item) => isChecked(Number(item.id))).length;
}

async function refreshAll() {
  if (!allowAccess.value || !hasValidTenant.value) return;
  if (refreshInFlight) {
    await refreshInFlight;
    return;
  }
  refreshInFlight = (async () => {
    loading.value = true;
    try {
      const [profileResp, keyResp, catalogResp] = await Promise.all([
        svc.listApiKeyProfiles(effectiveTenantUUID.value),
        svc.listApiKeys({ tenant_uuid: effectiveTenantUUID.value, page: 1, page_size: 200 }),
        svc.listPermissionCatalog(),
      ]);
      apiKeyProfiles.value = Array.isArray(profileResp?.data?.items) ? profileResp.data.items : [];
      apiKeys.value = Array.isArray(keyResp?.data?.items) ? keyResp.data.items : [];
      permissionCatalog.value = Array.isArray(catalogResp?.data?.items) ? catalogResp.data.items : [];

      const currentSelected = selectedProfileID.value || 0;
      const exists = apiKeyProfiles.value.some((item) => item.id === currentSelected);
      if (!exists) {
        const active = apiKeyProfiles.value.find((item) => item.status === 1);
        selectedProfileID.value = active?.id || apiKeyProfiles.value[0]?.id || null;
      }
    } catch (err: any) {
      toast.add({
        title: "加载失败",
        description: err?.message || "无法加载 API Key 数据",
        color: "error",
      });
    } finally {
      loading.value = false;
      refreshInFlight = null;
    }
  })();
  await refreshInFlight;
}

async function loadSelectedProfilePermissions() {
  const profile = selectedProfile.value;
  if (!profile?.id) {
    selectedPermissionIDs.value = [];
    initialPermissionIDs.value = [];
    return;
  }
  loadingPermissions.value = true;
  try {
    const resp = await svc.getProfilePermissions(profile.id);
    const ids = Array.isArray(resp?.data?.permission_ids) ? resp.data.permission_ids.map((id) => Number(id)) : [];
    selectedPermissionIDs.value = ids;
    initialPermissionIDs.value = [...ids];
  } catch (err: any) {
    toast.add({
      title: "加载 Profile 权限失败",
      description: err?.message || "请稍后重试",
      color: "error",
    });
    selectedPermissionIDs.value = [];
    initialPermissionIDs.value = [];
  } finally {
    loadingPermissions.value = false;
  }
}

async function handleTenantSelect(raw: unknown) {
  if (!canSwitchTenant.value) return;
  const value = normalizeSelectString(raw);
  if (!value || value === tenantUUID.value) return;
  switchingTenant.value = true;
  try {
    await userStore.switchTenant(value);
    tenantUUID.value = value;
    selectedTenantUUID.value = value;
    await refreshAll();
    await loadSelectedProfilePermissions();
    toast.add({ title: "已切换租户", color: "success" });
  } catch (err: any) {
    selectedTenantUUID.value = tenantUUID.value;
    toast.add({
      title: "切换租户失败",
      description: err?.message || "请检查权限",
      color: "error",
    });
  } finally {
    switchingTenant.value = false;
  }
}

function openCreateProfile() {
  createProfileForm.name = "";
  createProfileForm.key = "";
  createProfileOpen.value = true;
}

async function submitCreateProfile() {
  const name = String(createProfileForm.name || "").trim();
  if (!name) {
    toast.add({ title: "请输入 Profile 名称", color: "warning" });
    return;
  }
  creatingProfile.value = true;
  try {
    const resp = await svc.createApiKeyProfile({
      tenant_uuid: effectiveTenantUUID.value,
      name,
      key: String(createProfileForm.key || "").trim() || undefined,
    });
    const createdID = Number(resp?.data?.id || 0);
    createProfileOpen.value = false;
    await refreshAll();
    if (createdID) selectedProfileID.value = createdID;
    await loadSelectedProfilePermissions();
    toast.add({ title: "Profile 创建成功", color: "success" });
  } catch (err: any) {
    toast.add({
      title: "创建 Profile 失败",
      description: err?.message || "请稍后重试",
      color: "error",
    });
  } finally {
    creatingProfile.value = false;
  }
}

function openRenameProfile() {
  if (!selectedProfile.value) return;
  renameProfileForm.name = selectedProfile.value.name || "";
  renameProfileOpen.value = true;
}

async function submitRenameProfile() {
  const profile = selectedProfile.value;
  if (!profile?.id) return;
  const name = String(renameProfileForm.name || "").trim();
  if (!name) {
    toast.add({ title: "请输入 Profile 名称", color: "warning" });
    return;
  }
  updatingProfileID.value = profile.id;
  try {
    await svc.updateApiKeyProfile(profile.id, {
      tenant_uuid: effectiveTenantUUID.value,
      name,
    });
    renameProfileOpen.value = false;
    await refreshAll();
    toast.add({ title: "Profile 已更新", color: "success" });
  } catch (err: any) {
    toast.add({
      title: "更新失败",
      description: err?.message || "请稍后重试",
      color: "error",
    });
  } finally {
    updatingProfileID.value = null;
  }
}

async function toggleSelectedProfileStatus(nextStatus: number) {
  const profile = selectedProfile.value;
  if (!profile?.id) return;
  updatingProfileID.value = profile.id;
  try {
    await svc.updateApiKeyProfile(profile.id, {
      tenant_uuid: effectiveTenantUUID.value,
      status: nextStatus,
    });
    await refreshAll();
    toast.add({
      title: nextStatus === 1 ? "Profile 已启用" : "Profile 已停用",
      color: "success",
    });
  } catch (err: any) {
    toast.add({
      title: "更新状态失败",
      description: err?.message || "请稍后重试",
      color: "error",
    });
  } finally {
    updatingProfileID.value = null;
  }
}

async function savePermissions() {
  const profile = selectedProfile.value;
  if (!profile?.id) return;
  savingPermissions.value = true;
  try {
    const ids = Array.from(new Set(selectedPermissionIDs.value)).filter((item) => Number(item) > 0);
    await svc.setProfilePermissions(profile.id, ids);
    initialPermissionIDs.value = [...ids];
    await refreshAll();
    toast.add({ title: "权限保存成功", color: "success" });
  } catch (err: any) {
    toast.add({
      title: "保存失败",
      description: err?.message || "请稍后重试",
      color: "error",
    });
  } finally {
    savingPermissions.value = false;
  }
}

function openCreateKey() {
  if (!selectedProfile.value?.id) {
    toast.add({ title: "请先选择 Profile", color: "warning" });
    return;
  }
  if (selectedProfile.value.status !== 1) {
    toast.add({ title: "当前 Profile 未启用", color: "warning" });
    return;
  }
  createKeyForm.name = "";
  createKeyForm.description = "";
  createKeyForm.expiresAt = "";
  createKeyOpen.value = true;
}

async function submitCreateKey() {
  const profile = selectedProfile.value;
  if (!profile?.id) return;
  const name = String(createKeyForm.name || "").trim();
  if (!name) {
    toast.add({ title: "请输入 Key 名称", color: "warning" });
    return;
  }
  creatingKey.value = true;
  try {
    const resp = await svc.createApiKey({
      tenant_uuid: effectiveTenantUUID.value,
      profile_id: profile.id,
      name,
      description: String(createKeyForm.description || "").trim() || undefined,
      expires_at: String(createKeyForm.expiresAt || "").trim() || undefined,
    });
    latestPlainKey.value = String(resp?.data?.plain_key || "");
    latestPlainKeyLabel.value = String(resp?.data?.api_key?.name || name);
    createKeyOpen.value = false;
    await refreshAll();
    toast.add({ title: "API Key 创建成功", color: "success" });
  } catch (err: any) {
    toast.add({
      title: "创建失败",
      description: err?.message || "请稍后重试",
      color: "error",
    });
  } finally {
    creatingKey.value = false;
  }
}

function openRotate(record: IntegrationGatewayApiKeyRecord) {
  rotateTarget.value = record;
  rotateForm.name = record.name || "";
  rotateForm.description = record.description || "";
  rotateForm.expiresAt = record.expires_at || "";
  rotateOpen.value = true;
}

async function submitRotate() {
  const target = rotateTarget.value;
  if (!target?.key_id) return;
  rotatingKeyID.value = target.key_id;
  try {
    const resp = await svc.rotateApiKey(target.key_id, {
      tenant_uuid: effectiveTenantUUID.value,
      name: String(rotateForm.name || "").trim() || undefined,
      description: String(rotateForm.description || "").trim() || undefined,
      expires_at: String(rotateForm.expiresAt || "").trim() || undefined,
    });
    latestPlainKey.value = String(resp?.data?.plain_key || "");
    latestPlainKeyLabel.value = String(resp?.data?.api_key?.name || target.name);
    rotateOpen.value = false;
    await refreshAll();
    toast.add({ title: "轮换成功", color: "success" });
  } catch (err: any) {
    toast.add({
      title: "轮换失败",
      description: err?.message || "请稍后重试",
      color: "error",
    });
  } finally {
    rotatingKeyID.value = "";
  }
}

async function revokeKey(record: IntegrationGatewayApiKeyRecord) {
  if (!record?.key_id || record.status !== "active") return;
  const confirmed = window.confirm(`确认吊销 API Key：${record.name}（${record.key_prefix}）？`);
  if (!confirmed) return;
  revokingKeyID.value = record.key_id;
  try {
    await svc.revokeApiKey(record.key_id, { tenant_uuid: effectiveTenantUUID.value });
    await refreshAll();
    toast.add({ title: "已吊销", color: "success" });
  } catch (err: any) {
    toast.add({
      title: "吊销失败",
      description: err?.message || "请稍后重试",
      color: "error",
    });
  } finally {
    revokingKeyID.value = "";
  }
}

async function deleteKey(record: IntegrationGatewayApiKeyRecord) {
  if (!record?.key_id) return;
  const confirmed = window.confirm(`确认删除 API Key：${record.name}（${record.key_prefix}）？删除后将不再显示。`);
  if (!confirmed) return;
  deletingKeyID.value = record.key_id;
  try {
    await svc.deleteApiKey(record.key_id);
    await refreshAll();
    toast.add({ title: "已删除", color: "success" });
  } catch (err: any) {
    toast.add({
      title: "删除失败",
      description: err?.message || "请稍后重试",
      color: "error",
    });
  } finally {
    deletingKeyID.value = "";
  }
}

async function copyPlainKey() {
  if (!latestPlainKey.value) return;
  try {
    await navigator.clipboard.writeText(latestPlainKey.value);
    toast.add({ title: "已复制明文 Key", color: "success" });
  } catch {
    toast.add({ title: "复制失败，请手动复制", color: "warning" });
  }
}

watch(
  () => currentTenantUuid.value,
  async (tenant) => {
    if (!tenant) return;
    const changed = tenantUUID.value !== tenant;
    tenantUUID.value = tenant;
    selectedTenantUUID.value = tenant;
    if (changed) {
      await refreshAll();
      await loadSelectedProfilePermissions();
    }
  }
);

watch(selectedProfileID, async (raw) => {
  if (raw === null || raw === undefined) {
    selectedPermissionIDs.value = [];
    initialPermissionIDs.value = [];
    return;
  }
  const normalized = normalizeSelectNumber(raw);
  if (normalized !== raw) {
    selectedProfileID.value = normalized;
    return;
  }
  await loadSelectedProfilePermissions();
});

onMounted(async () => {
  try {
    await userStore.fetchUserContext({ force: true });
  } catch {}
  tenantUUID.value = currentTenantUuid.value || "";
  selectedTenantUUID.value = tenantUUID.value;
  await refreshAll();
  await loadSelectedProfilePermissions();
});
</script>

<template>
  <div class="p-6 space-y-4">
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div>
        <h1 class="text-2xl font-bold text-gray-900 dark:text-white">API Key 权限管理</h1>
        <p class="text-sm text-gray-600 dark:text-gray-400">
          左侧选 Profile，右侧按权限目录勾选并保存；Key 自动继承 Profile 权限。
        </p>
      </div>
      <div class="flex flex-wrap gap-2">
        <UButton icon="i-heroicons-arrow-path" :loading="loading" @click="refreshAll">刷新</UButton>
        <UButton color="neutral" variant="soft" icon="i-heroicons-plus-circle" :disabled="!hasValidTenant" @click="openCreateProfile">
          新建 Profile
        </UButton>
      </div>
    </div>

    <UAlert
      v-if="!allowAccess"
      icon="i-heroicons-lock-closed"
      color="warning"
      variant="subtle"
      title="无权限"
      description="仅 Root 或当前租户管理员可访问 API Key 配置。"
    />

    <template v-else>
      <UCard>
        <div class="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
          <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
            <div class="rounded border border-gray-200 dark:border-gray-700 px-3 py-2">
              <div class="text-xs text-gray-500">当前租户</div>
              <div class="text-sm font-medium">{{ currentTenant?.tenant_name || tenantUUID || "-" }}</div>
            </div>
            <div class="rounded border border-gray-200 dark:border-gray-700 px-3 py-2">
              <div class="text-xs text-gray-500">tenant_uuid</div>
              <div class="text-sm font-mono break-all">{{ effectiveTenantUUID || "-" }}</div>
            </div>
          </div>
          <div class="w-full lg:w-[320px]">
            <UFormField :label="canSwitchTenant ? '切换租户（Root）' : '租户（当前上下文）'">
              <USelectMenu
                v-if="canSwitchTenant"
                v-model="selectedTenantUUID"
                :items="tenantOptions"
                value-key="value"
                label-key="label"
                :portal="false"
                class="w-full"
                placeholder="选择租户"
                :disabled="switchingTenant || tenantOptions.length === 0"
                @update:model-value="handleTenantSelect"
              />
              <div v-else class="rounded border border-gray-200 dark:border-gray-700 px-3 py-2 text-sm">
                {{ currentTenant?.tenant_name || "-" }}
              </div>
            </UFormField>
          </div>
        </div>
      </UCard>

      <div class="grid grid-cols-1 gap-4 xl:grid-cols-[320px_minmax(0,1fr)]">
        <UCard>
          <template #header>
            <div class="space-y-2">
              <div class="font-semibold">API Key Profile 列表</div>
              <UInput
                v-model="searchProfile"
                icon="i-heroicons-magnifying-glass"
                placeholder="搜索名称 / key / #id"
              />
            </div>
          </template>

          <div class="space-y-2 max-h-[72vh] overflow-auto pr-1">
            <button
              v-for="profile in filteredProfiles"
              :key="profile.id"
              type="button"
              class="w-full rounded border p-3 text-left transition"
              :class="selectedProfileID === profile.id
                ? 'border-primary-400 bg-primary-50 dark:border-primary-500 dark:bg-primary-500/10'
                : 'border-gray-200 hover:border-primary-300 dark:border-gray-700 dark:hover:border-primary-600'"
              @click="selectedProfileID = profile.id"
            >
              <div class="flex items-start justify-between gap-2">
                <div class="min-w-0">
                  <div class="font-medium truncate">{{ profile.name }}</div>
                  <div class="text-xs text-gray-500 font-mono truncate">{{ profile.key }}</div>
                </div>
                <UBadge :color="profile.status === 1 ? 'success' : 'error'" variant="subtle">
                  {{ profile.status === 1 ? "active" : "disabled" }}
                </UBadge>
              </div>
              <div class="mt-2 text-xs text-gray-500">
                #{{ profile.id }} · Key {{ keyCountMap[profile.id] || 0 }} 个
              </div>
            </button>

            <div v-if="filteredProfiles.length === 0" class="rounded border border-dashed p-4 text-sm text-gray-500">
              暂无 Profile，先创建一个再配置权限。
            </div>
          </div>
        </UCard>

        <UCard>
          <template #header>
            <div class="flex flex-wrap items-center justify-between gap-2">
              <div>
                <div class="font-semibold">权限配置</div>
                <div v-if="selectedProfile" class="text-xs text-gray-500">
                  Profile：{{ selectedProfile.name }} · <span class="font-mono">#{{ selectedProfile.id }}</span>
                </div>
                <div v-if="selectedProfile" class="text-xs text-gray-500">
                  全模块：已选 {{ totalCheckedPermissionCount }}/{{ totalPermissionCount }}
                </div>
              </div>
              <div class="flex flex-wrap gap-2">
                <UButton
                  size="xs"
                  variant="soft"
                  :disabled="!selectedProfile"
                  @click="openRenameProfile"
                >
                  重命名
                </UButton>
                <UButton
                  v-if="selectedProfile?.status === 1"
                  size="xs"
                  color="error"
                  variant="soft"
                  :loading="updatingProfileID === selectedProfile?.id"
                  @click="toggleSelectedProfileStatus(0)"
                >
                  停用
                </UButton>
                <UButton
                  v-else-if="selectedProfile?.status === 0"
                  size="xs"
                  color="success"
                  variant="soft"
                  :loading="updatingProfileID === selectedProfile?.id"
                  @click="toggleSelectedProfileStatus(1)"
                >
                  启用
                </UButton>
                <UButton
                  size="xs"
                  variant="soft"
                  :disabled="!selectedProfile || selectedProfile.status !== 1 || totalPermissionCount === 0"
                  @click="toggleAllPermissions(true)"
                >
                  全模块全选
                </UButton>
                <UButton
                  size="xs"
                  color="neutral"
                  variant="soft"
                  :disabled="!selectedProfile || selectedProfile.status !== 1 || totalPermissionCount === 0"
                  @click="toggleAllPermissions(false)"
                >
                  全模块清空
                </UButton>
                <UButton
                  size="xs"
                  color="primary"
                  :loading="savingPermissions"
                  :disabled="!selectedProfile || selectedProfile.status !== 1 || !permissionDirty"
                  @click="savePermissions"
                >
                  保存权限
                </UButton>
              </div>
            </div>
          </template>

          <div v-if="!selectedProfile" class="rounded border border-dashed p-4 text-sm text-gray-500">
            请选择左侧 Profile 后再配置权限。
          </div>

          <template v-else>
            <UAlert
              v-if="selectedProfile.status !== 1"
              color="warning"
              variant="subtle"
              icon="i-heroicons-exclamation-triangle"
              title="当前 Profile 已停用"
              description="可查看权限，但不能保存修改，也不能新建 Key。"
            />

            <div
              v-if="loadingPermissions"
              class="mt-3 rounded border border-gray-200 dark:border-gray-700 p-4 text-sm text-gray-500"
            >
              加载权限中...
            </div>

            <div v-else class="mt-3 space-y-3 max-h-[72vh] overflow-auto pr-1">
              <UInput
                v-model="searchPermission"
                icon="i-heroicons-magnifying-glass"
                placeholder="搜索权限（支持 capability_id / 路径 / resource / action）"
              />
              <div
                v-for="resourceGroup in filteredPermissionGroups"
                :key="`resource-${resourceGroup.resourceName}`"
                class="rounded border border-gray-200 dark:border-gray-700 p-3"
              >
                <div class="flex flex-wrap items-center justify-between gap-2">
                  <div class="font-medium break-all">resource：{{ resourceGroup.resourceName }}</div>
                  <div class="flex items-center gap-2 text-xs">
                    <span class="text-gray-500">
                      已选 {{ checkedCount(resourceGroup.actions.flatMap((item) => item.permissions)) }}/{{ resourceGroup.actions.flatMap((item) => item.permissions).length }}
                    </span>
                    <UButton
                      size="xs"
                      variant="soft"
                      :disabled="selectedProfile.status !== 1"
                      @click="togglePermissions(resourceGroup.actions.flatMap((item) => item.permissions), true)"
                    >
                      全选
                    </UButton>
                    <UButton
                      size="xs"
                      color="neutral"
                      variant="soft"
                      :disabled="selectedProfile.status !== 1"
                      @click="togglePermissions(resourceGroup.actions.flatMap((item) => item.permissions), false)"
                    >
                      清空
                    </UButton>
                  </div>
                </div>

                <div class="mt-2 grid grid-cols-1 gap-2 xl:grid-cols-2">
                  <div
                    v-for="actionGroup in resourceGroup.actions"
                    :key="`action-${resourceGroup.resourceName}-${actionGroup.actionName}`"
                    class="rounded border border-gray-100 dark:border-gray-800 p-2"
                  >
                    <div class="flex items-center justify-between gap-2 text-xs text-gray-500">
                      <span class="font-medium">{{ actionGroup.actionName }}</span>
                      <span>{{ checkedCount(actionGroup.permissions) }}/{{ actionGroup.permissions.length }}</span>
                    </div>

                    <div class="mt-2 space-y-2">
                      <label
                        v-for="item in actionGroup.permissions"
                        :key="`perm-${item.id}`"
                        class="flex items-start gap-2 rounded border border-gray-100 dark:border-gray-800 p-2"
                      >
                        <UCheckbox
                          :model-value="isChecked(Number(item.id))"
                          :disabled="selectedProfile.status !== 1"
                          @update:model-value="setPermission(Number(item.id), Boolean($event))"
                        />
                        <div class="min-w-0 text-xs">
                          <div class="font-medium break-all">
                            {{ permissionTitle(item) }}
                          </div>
                          <div class="font-mono text-[11px] text-gray-500 break-all">
                            #{{ item.id }} · {{ item.resource }} · {{ item.action }}
                          </div>
                          <div
                            v-if="permissionCapabilityID(item)"
                            class="font-mono text-[11px] text-gray-500 break-all"
                          >
                            capability: {{ permissionCapabilityID(item) }}
                          </div>
                        </div>
                      </label>
                    </div>
                  </div>
                </div>
              </div>

              <div v-if="permissionCatalog.length === 0" class="rounded border border-dashed p-4 text-sm text-gray-500">
                权限目录为空，请先检查后端权限初始化。
              </div>
              <div
                v-else-if="filteredPermissionGroups.length === 0"
                class="rounded border border-dashed p-4 text-sm text-gray-500"
              >
                未找到匹配权限，请换一个关键词（例如：`POST /api/v1/admin/agents`、`admin_agents`）。
              </div>
            </div>

            <div class="mt-4 space-y-2">
              <div class="flex items-center justify-between gap-2">
                <div class="font-semibold">当前 Profile 的 API Key</div>
                <UButton
                  size="xs"
                  color="primary"
                  icon="i-heroicons-plus"
                  :disabled="!selectedProfile || selectedProfile.status !== 1"
                  @click="openCreateKey"
                >
                  新建 API Key
                </UButton>
              </div>
              <div class="overflow-x-auto">
                <table class="min-w-full text-sm border border-gray-200 dark:border-gray-700 rounded">
                  <thead class="bg-gray-50 dark:bg-gray-800/50">
                    <tr>
                      <th class="px-3 py-2 text-left whitespace-nowrap">名称</th>
                      <th class="px-3 py-2 text-left whitespace-nowrap">Key 前缀</th>
                      <th class="px-3 py-2 text-left whitespace-nowrap">状态</th>
                      <th class="px-3 py-2 text-left whitespace-nowrap">过期时间</th>
                      <th class="px-3 py-2 text-left whitespace-nowrap">更新时间</th>
                      <th class="px-3 py-2 text-left whitespace-nowrap">操作</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="item in profileKeys" :key="item.key_id" class="border-t border-gray-200 dark:border-gray-700">
                      <td class="px-3 py-2">{{ item.name }}</td>
                      <td class="px-3 py-2 font-mono">{{ item.key_prefix }}</td>
                      <td class="px-3 py-2">
                        <UBadge :color="item.status === 'active' ? 'success' : 'neutral'" variant="subtle">
                          {{ item.status }}
                        </UBadge>
                      </td>
                      <td class="px-3 py-2 font-mono">{{ item.expires_at || "-" }}</td>
                      <td class="px-3 py-2 font-mono">{{ item.updated_at }}</td>
                      <td class="px-3 py-2">
                        <div class="flex flex-nowrap gap-2">
                          <UButton size="xs" variant="soft" :loading="rotatingKeyID === item.key_id" @click="openRotate(item)">
                            轮换
                          </UButton>
                          <UButton
                            size="xs"
                            color="error"
                            variant="soft"
                            :loading="revokingKeyID === item.key_id"
                            :disabled="item.status !== 'active'"
                            @click="revokeKey(item)"
                          >
                            吊销
                          </UButton>
                          <UButton
                            size="xs"
                            color="error"
                            variant="outline"
                            :loading="deletingKeyID === item.key_id"
                            @click="deleteKey(item)"
                          >
                            删除
                          </UButton>
                        </div>
                      </td>
                    </tr>
                    <tr v-if="profileKeys.length === 0">
                      <td colspan="6" class="px-3 py-4 text-center text-gray-500">当前 Profile 暂无 API Key</td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </div>
          </template>
        </UCard>
      </div>

      <UAlert
        v-if="latestPlainKey"
        color="warning"
        variant="subtle"
        icon="i-heroicons-exclamation-triangle"
        title="请立即保存明文 API Key"
      >
        <template #description>
          <div class="space-y-2">
            <div class="text-xs">名称：{{ latestPlainKeyLabel }}</div>
            <div class="font-mono text-xs break-all rounded bg-white/80 px-2 py-1 dark:bg-gray-900">
              {{ latestPlainKey }}
            </div>
            <div class="flex gap-2">
              <UButton size="xs" variant="outline" icon="i-heroicons-clipboard" @click="copyPlainKey">复制</UButton>
              <UButton size="xs" variant="ghost" @click="latestPlainKey = ''">隐藏</UButton>
            </div>
          </div>
        </template>
      </UAlert>
    </template>

    <UModal v-model:open="createProfileOpen" title="新建 API Key Profile" :ui="{ content: 'max-w-lg' }">
      <template #body>
        <div class="space-y-3">
          <UFormField label="Profile 名称">
            <UInput v-model="createProfileForm.name" placeholder="例如：Plugin Runtime Profile" />
          </UFormField>
          <UFormField label="Profile Key（可选）">
            <UInput v-model="createProfileForm.key" placeholder="例如：plugin.runtime.default" />
          </UFormField>
        </div>
      </template>
      <template #footer>
        <div class="flex justify-end gap-2 w-full">
          <UButton color="neutral" variant="subtle" @click="createProfileOpen = false">取消</UButton>
          <UButton color="primary" :loading="creatingProfile" @click="submitCreateProfile">创建</UButton>
        </div>
      </template>
    </UModal>

    <UModal v-model:open="renameProfileOpen" title="重命名 Profile" :ui="{ content: 'max-w-lg' }">
      <template #body>
        <UFormField label="Profile 名称">
          <UInput v-model="renameProfileForm.name" />
        </UFormField>
      </template>
      <template #footer>
        <div class="flex justify-end gap-2 w-full">
          <UButton color="neutral" variant="subtle" @click="renameProfileOpen = false">取消</UButton>
          <UButton color="primary" :loading="updatingProfileID === selectedProfile?.id" @click="submitRenameProfile">
            保存
          </UButton>
        </div>
      </template>
    </UModal>

    <UModal
      v-model:open="createKeyOpen"
      title="新建 API Key"
      description="权限自动继承当前 Profile 的勾选项"
      :ui="{ content: 'max-w-2xl' }"
    >
      <template #body>
        <div class="space-y-3">
          <UFormField label="Key 名称">
            <UInput v-model="createKeyForm.name" placeholder="例如：plugin-runtime-key" />
          </UFormField>
          <UFormField label="描述（可选）">
            <UInput v-model="createKeyForm.description" />
          </UFormField>
          <UFormField label="过期时间（可选）">
            <UInput v-model="createKeyForm.expiresAt" placeholder="RFC3339，例如 2026-12-31T23:59:59Z" />
          </UFormField>
        </div>
      </template>
      <template #footer>
        <div class="flex justify-end gap-2 w-full">
          <UButton color="neutral" variant="subtle" @click="createKeyOpen = false">取消</UButton>
          <UButton color="primary" :loading="creatingKey" @click="submitCreateKey">创建</UButton>
        </div>
      </template>
    </UModal>

    <UModal v-model:open="rotateOpen" title="轮换 API Key" :ui="{ content: 'max-w-2xl' }">
      <template #body>
        <div class="space-y-3">
          <UFormField label="Key 名称">
            <UInput v-model="rotateForm.name" />
          </UFormField>
          <UFormField label="描述">
            <UInput v-model="rotateForm.description" />
          </UFormField>
          <UFormField label="过期时间（可选）">
            <UInput v-model="rotateForm.expiresAt" placeholder="RFC3339，可选" />
          </UFormField>
        </div>
      </template>
      <template #footer>
        <div class="flex justify-end gap-2 w-full">
          <UButton color="neutral" variant="subtle" @click="rotateOpen = false">取消</UButton>
          <UButton color="primary" :loading="Boolean(rotatingKeyID)" @click="submitRotate">确认轮换</UButton>
        </div>
      </template>
    </UModal>
  </div>
</template>
