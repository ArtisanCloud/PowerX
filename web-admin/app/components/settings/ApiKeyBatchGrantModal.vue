<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useI18n } from '#imports';
import ApiKeyPermissionAccess from '~/components/settings/ApiKeyPermissionAccess.vue';
import {
  useIntegrationGatewayApiKeyService,
  type IntegrationGatewayApiKeyRecord,
  type IntegrationGatewayPermissionCatalogItem,
  type BatchApiKeyGrantResult,
} from '~/composables/api/services/integrationGatewayApiKeyService';

const props = defineProps<{
  open: boolean;
  tenantUuid: string;
  profileId: number;
  profileName: string;
  keys: IntegrationGatewayApiKeyRecord[];
  permissions: IntegrationGatewayPermissionCatalogItem[];
  initialKeyIds?: string[];
  initialPermissionIds?: number[];
}>();
const emit = defineEmits<{ 'update:open': [value: boolean]; saved: [] }>();
const { t } = useI18n();
const svc = useIntegrationGatewayApiKeyService();
const step = ref(0),
  keyIDs = ref<string[]>([]),
  permissionIDs = ref<number[]>([]),
  searchKeys = ref(''),
  searchPermissions = ref('');
const busy = ref(false),
  plan = ref<BatchApiKeyGrantResult | null>(null),
  error = ref(''),
  uncertain = ref(false);
let generation = 0;
const successful = computed(() => plan.value?.committed === true);
const steps = computed(() => [
  t('integrationApiKeyBatch.stepKeys'),
  t('integrationApiKeyBatch.stepPermissions'),
  t('integrationApiKeyBatch.stepReview'),
]);
const canUse = (key: IntegrationGatewayApiKeyRecord) =>
  key.status === 'active' &&
  (!key.expires_at || Date.parse(key.expires_at) > Date.now());
const visibleKeys = computed(() =>
  props.keys.filter((key) =>
    `${key.name} ${key.key_id}`
      .toLowerCase()
      .includes(searchKeys.value.trim().toLowerCase()),
  ),
);
const eligiblePermissions = computed(() =>
  props.permissions.filter(
    (item) =>
      item.meta?.api_key_explicit === true &&
      typeof item.meta?.capability_id === 'string' &&
      item.meta?.api_key?.scope,
  ),
);
const label = (item: IntegrationGatewayPermissionCatalogItem) =>
  String(item.meta?.label || `${item.resource}.${item.action}`);
const visiblePermissions = computed(() =>
  eligiblePermissions.value.filter((item) =>
    `${label(item)} ${item.meta?.capability_id} ${item.meta?.api_key?.plugin_id || 'PowerX'} ${item.action}`
      .toLowerCase()
      .includes(searchPermissions.value.trim().toLowerCase()),
  ),
);
const close = () => {
  if (!busy.value) emit('update:open', false);
};
const reset = () => {
  generation++;
  step.value = 0;
  const candidates = new Set(props.initialKeyIds || []);
  const permissions = new Set(props.initialPermissionIds || []);
  const initialKeys = props.keys
    .filter((key) => candidates.has(key.key_id) && canUse(key))
    .map((key) => key.key_id);
  const initialPermissions = eligiblePermissions.value
    .filter((item) => permissions.has(item.id))
    .map((item) => item.id);
  keyIDs.value = initialKeys.length <= 50 ? initialKeys : [];
  permissionIDs.value =
    initialPermissions.length <= 128 ? initialPermissions : [];
  searchKeys.value = '';
  searchPermissions.value = '';
  busy.value = false;
  plan.value = null;
  error.value = '';
  uncertain.value = false;
};
watch(
  () => props.open,
  (value) => {
    if (value) reset();
    else generation++;
  },
  { immediate: true },
);
watch([() => props.tenantUuid, () => props.profileId], () => {
  reset();
  emit('update:open', false);
});
watch(
  [keyIDs, permissionIDs],
  () => {
    plan.value = null;
    error.value = '';
    uncertain.value = false;
  },
  { deep: true },
);
function toggleKey(id: string, value: unknown) {
  const selected = new Set(keyIDs.value);
  if (value) selected.add(id);
  else selected.delete(id);
  keyIDs.value = [...selected];
}
function togglePermission(id: number, value: unknown) {
  const selected = new Set(permissionIDs.value);
  if (value) selected.add(id);
  else selected.delete(id);
  permissionIDs.value = [...selected];
}
function failureMessage(err: any) {
  const data = err?.data ?? err?.response?._data ?? {},
    details = data.details ?? err?.details ?? {};
  const code = data.error_code ?? err?.errorCode;
  const messages: Record<string, string> = {
    API_KEY_BATCH_PREVIEW_STALE: 'stale',
    API_KEY_BATCH_PREVIEW_REQUIRED: 'reviewFirst',
    API_KEY_BATCH_GRANT_INVALID: 'invalid',
    CAPABILITY_UNKNOWN: 'unpublished',
    CAPABILITY_TENANT_NOT_REGISTERED: 'unregistered',
    API_KEY_INACTIVE: 'inactive',
    API_KEY_PROFILE_DISABLED: 'inactive',
    API_KEY_PROFILE_INACTIVE: 'inactive',
    API_KEY_NOT_FOUND: 'keyMissing',
    API_KEY_OWNER_ADMIN_REQUIRED: 'forbidden',
  };
  const name =
    details.failed_key_name ||
    props.keys.find((key) => key.key_id === details.failed_key_id)?.name ||
    details.failed_permission_name ||
    details.failed_profile_name;
  const message = messages[code]
    ? t(`integrationApiKeyBatch.${messages[code]}`)
    : err?.message || t('integrationApiKeyBatch.failed');
  return name ? `${name}：${message}` : message;
}
function complete(
  result: BatchApiKeyGrantResult | undefined,
  committed: boolean,
): result is BatchApiKeyGrantResult {
  return (
    !!result &&
    result.committed === committed &&
    result.key_count === keyIDs.value.length &&
    Array.isArray(result.keys) &&
    result.keys.length === keyIDs.value.length &&
    new Set(result.keys.map((key) => key.key_id)).size ===
      keyIDs.value.length &&
    result.keys.every((key) => keyIDs.value.includes(key.key_id)) &&
    Array.isArray(result.permissions) &&
    result.permissions.length === permissionIDs.value.length &&
    new Set(result.permissions.map((permission) => permission.permission_id))
      .size === permissionIDs.value.length &&
    result.permissions.every((permission) =>
      permissionIDs.value.includes(permission.permission_id),
    ) &&
    /^[a-f0-9]{64}$/.test(result.plan_hash)
  );
}
async function run(commit = false) {
  if (
    busy.value ||
    !keyIDs.value.length ||
    keyIDs.value.length > 50 ||
    !permissionIDs.value.length ||
    permissionIDs.value.length > 128
  )
    return;
  if (commit && !plan.value) return;
  const request = ++generation;
  busy.value = true;
  error.value = '';
  uncertain.value = false;
  try {
    const response = await svc.batchAppendExplicitGrants({
      key_ids: [...keyIDs.value],
      permission_ids: [...permissionIDs.value],
      dry_run: !commit,
      ...(commit ? { expected_plan_hash: plan.value!.plan_hash } : {}),
    });
    if (request !== generation) return;
    if (!complete(response.data, commit))
      throw new Error('API_KEY_BATCH_RESPONSE_INVALID');
    plan.value = response.data;
    step.value = 2;
    if (commit) emit('saved');
  } catch (err: any) {
    if (request !== generation) return;
    plan.value = null;
    const data = err?.data ?? err?.response?._data ?? {};
    const status = Number(err?.status ?? err?.statusCode ?? data.code);
    uncertain.value =
      commit &&
      !(
        status >= 400 &&
        status < 500 &&
        typeof (data.error_code ?? err?.errorCode) === 'string'
      );
    error.value = uncertain.value
      ? t('integrationApiKeyBatch.unknown')
      : failureMessage(err);
  } finally {
    if (request === generation) busy.value = false;
  }
}
</script>

<template>
  <UModal
    :open="open"
    @update:open="
      (value) => {
        if (!busy) emit('update:open', value);
      }
    "
    :title="t('integrationApiKeyBatch.title')"
    :description="
      t('integrationApiKeyBatch.description', { profile: profileName })
    "
    :dismissible="!busy"
    :close="busy ? false : undefined"
    :ui="{ content: 'max-w-5xl', body: 'p-4 sm:p-5', footer: 'justify-end' }"
  >
    <template #body>
      <div
        class="grid grid-cols-1 gap-5 md:grid-cols-[11rem_minmax(0,1fr)]"
        data-testid="api-key-batch-modal"
      >
        <nav
          :aria-label="t('integrationApiKeyBatch.steps')"
          class="flex gap-2 md:flex-col"
        >
          <UButton
            v-for="(name, index) in steps"
            :key="index"
            :color="step === index ? 'primary' : 'neutral'"
            :variant="step === index ? 'soft' : 'ghost'"
            :disabled="
              busy ||
              successful ||
              (index === 1 && !keyIDs.length) ||
              (index === 2 && !plan)
            "
            class="justify-start"
            @click="step = index"
            >{{ index + 1 }}. {{ name }}</UButton
          >
        </nav>
        <div class="min-w-0 space-y-4">
          <UAlert
            v-if="error"
            role="alert"
            color="error"
            variant="subtle"
            :title="
              uncertain
                ? t('integrationApiKeyBatch.unknownTitle')
                : t('integrationApiKeyBatch.failed')
            "
            :description="error"
          />
          <UAlert
            v-if="successful"
            role="status"
            color="success"
            variant="subtle"
            :title="
              t('integrationApiKeyBatch.success', { count: plan?.key_count })
            "
            :description="t('integrationApiKeyBatch.scopeHelp')"
          />
          <template v-if="step === 0">
            <p class="text-sm text-muted">
              {{ t('integrationApiKeyBatch.keyHelp') }}
            </p>
            <div class="flex flex-wrap gap-2">
              <UInput
                v-model="searchKeys"
                :disabled="busy"
                :placeholder="t('integrationApiKeyBatch.searchKeys')"
                class="grow"
              /><UButton
                color="neutral"
                variant="outline"
                :disabled="busy"
                @click="
                  keyIDs = visibleKeys
                    .filter(canUse)
                    .slice(0, 50)
                    .map((key) => key.key_id)
                "
                >{{ t('integrationApiKeyBatch.selectVisible') }}</UButton
              ><UButton
                color="neutral"
                variant="ghost"
                :disabled="busy"
                @click="keyIDs = []"
                >{{ t('integrationApiKeyBatch.clear') }}</UButton
              >
            </div>
            <p class="text-sm">
              {{
                t('integrationApiKeyBatch.selectedKeys', {
                  count: keyIDs.length,
                })
              }}
            </p>
            <div class="max-h-[50vh] overflow-auto space-y-2">
              <label
                v-for="key in visibleKeys"
                :key="key.key_id"
                class="flex min-w-0 items-start gap-3 rounded-lg border border-default p-3"
              >
                <UCheckbox
                  :model-value="keyIDs.includes(key.key_id)"
                  :disabled="
                    busy ||
                    !canUse(key) ||
                    (!keyIDs.includes(key.key_id) && keyIDs.length >= 50)
                  "
                  @update:model-value="(value) => toggleKey(key.key_id, value)"
                />
                <span class="min-w-0"
                  ><span class="block font-medium break-all">{{
                    key.name
                  }}</span
                  ><span class="block text-xs text-muted font-mono break-all">{{
                    key.plugin_ids?.join(', ') ||
                    t('integrationApiKeyBatch.unbound')
                  }}</span
                  ><span
                    v-if="!canUse(key)"
                    class="block text-xs text-warning"
                    >{{ t('integrationApiKeyBatch.unavailable') }}</span
                  ></span
                >
              </label>
            </div>
          </template>
          <template v-else-if="step === 1">
            <p class="text-sm text-muted">
              {{ t('integrationApiKeyBatch.permissionHelp') }}
            </p>
            <UInput
              v-model="searchPermissions"
              :disabled="busy"
              :placeholder="t('integrationApiKeyBatch.searchPermissions')"
              class="w-full"
            />
            <p class="text-sm">
              {{
                t('integrationApiKeyBatch.selectedPermissions', {
                  count: permissionIDs.length,
                })
              }}
            </p>
            <div class="max-h-[50vh] overflow-auto space-y-2">
              <label
                v-for="permission in visiblePermissions"
                :key="permission.id"
                class="flex min-w-0 items-start gap-3 rounded-lg border border-default p-3"
              >
                <UCheckbox
                  :model-value="permissionIDs.includes(permission.id)"
                  :disabled="
                    busy ||
                    (!permissionIDs.includes(permission.id) &&
                      permissionIDs.length >= 128)
                  "
                  @update:model-value="
                    (value) => togglePermission(permission.id, value)
                  "
                />
                <div class="min-w-0">
                  <span class="block font-medium break-all">{{ label(permission) }}</span>
                  <ApiKeyPermissionAccess :permission="permission" />
                  <span class="block text-xs text-muted font-mono break-all">
                    #{{ permission.id }} · {{ permission.meta?.capability_id }}
                  </span>
                  <span class="block text-xs text-muted">{{ permission.action }}</span>
                </div>
              </label>
              <p v-if="!visiblePermissions.length" class="text-sm text-muted">
                {{ t('integrationApiKeyBatch.noPermissions') }}
              </p>
            </div>
          </template>
          <template v-else>
            <template v-if="plan">
              <p class="font-medium">
                {{
                  t('integrationApiKeyBatch.reviewCount', {
                    keys: plan.key_count,
                    permissions: plan.permissions.length,
                    added: plan.added_count,
                  })
                }}
              </p>
              <p class="text-sm text-muted">
                {{ t('integrationApiKeyBatch.scopeHelp') }}
              </p>
              <section class="space-y-2">
                <h3 class="font-semibold">
                  {{ t('integrationApiKeyBatch.targetPermissions') }}
                </h3>
                <div
                  v-for="permission in plan.permissions"
                  :key="permission.permission_id"
                  class="min-w-0 rounded border border-default p-3"
                >
                  <p class="font-medium break-all">{{ permission.name }}</p>
                  <p class="text-xs font-mono text-muted break-all">
                    {{ permission.capability_id }}
                  </p>
                  <p class="text-xs text-muted break-all">
                    {{ permission.action }} · {{ permission.resource_type }} /
                    {{ permission.resource_pattern }}
                  </p>
                </div>
              </section>
              <section class="space-y-2">
                <h3 class="font-semibold">
                  {{ t('integrationApiKeyBatch.targetKeys') }}
                </h3>
                <ul class="max-h-56 overflow-auto space-y-1 text-sm">
                  <li
                    v-for="key in plan.keys"
                    :key="key.key_id"
                    class="break-all"
                  >
                    {{ key.name }} ·
                    {{
                      t('integrationApiKeyBatch.keyCounts', {
                        added: key.added_count,
                        existing: key.existing_count,
                      })
                    }}
                  </li>
                </ul>
              </section>
            </template>
            <UButton
              v-else
              :loading="busy"
              :disabled="!keyIDs.length || !permissionIDs.length"
              @click="run(false)"
              >{{ t('integrationApiKeyBatch.previewAgain') }}</UButton
            >
          </template>
        </div>
      </div>
    </template>
    <template #footer>
      <div class="flex w-full flex-wrap items-center justify-end gap-2">
        <UButton
          color="neutral"
          variant="outline"
          :disabled="busy"
          @click="close"
          >{{
            successful
              ? t('integrationApiKeyBatch.close')
              : t('integrationApiKeyBatch.cancel')
          }}</UButton
        >
        <template v-if="!successful"
          ><UButton
            v-if="step > 0"
            color="neutral"
            variant="soft"
            :disabled="busy"
            @click="step--"
            >{{ t('integrationApiKeyBatch.back') }}</UButton
          ><UButton
            v-if="step === 0"
            :disabled="busy || !keyIDs.length"
            @click="step = 1"
            >{{ t('integrationApiKeyBatch.next') }}</UButton
          ><UButton
            v-else-if="step === 1"
            :loading="busy"
            :disabled="!permissionIDs.length"
            @click="run(false)"
            >{{ t('integrationApiKeyBatch.preview') }}</UButton
          ><UButton v-else-if="plan" :loading="busy" @click="run(true)">{{
            t('integrationApiKeyBatch.commit')
          }}</UButton></template
        >
      </div>
    </template>
  </UModal>
</template>
