<template>
  <div class="mx-auto max-w-4xl p-6">
    <div class="mb-6">
      <div>
        <h1 class="text-2xl font-semibold text-[var(--text-primary)]">{{ t("profile.title") }}</h1>
        <p class="mt-1 text-sm text-[var(--text-secondary)]">{{ t("profile.description") }}</p>
      </div>
    </div>

    <UCard>
      <template #header>
        <div class="flex items-center justify-between gap-3">
          <div class="flex items-center gap-3">
            <UAvatar :src="userStore.avatarUrl || undefined" :alt="form.display_name || 'user'" size="xl" />
            <div>
              <div class="text-base font-medium text-[var(--text-primary)]">{{ userStore.displayName }}</div>
              <div class="text-sm text-[var(--text-secondary)]">{{ userStore.user?.email || '-' }}</div>
            </div>
          </div>
          <UBadge :color="userStore.isRoot ? 'warning' : userStore.isCurrentTenantAdmin ? 'primary' : 'neutral'" variant="subtle">
            {{ roleLabel }}
          </UBadge>
        </div>
      </template>

      <div v-if="currentTenant" class="mb-6 rounded-lg border border-[var(--border-subtle)] bg-[var(--bg-muted)] p-4">
        <div class="mb-3 flex items-center justify-between gap-3">
          <div>
            <div class="text-sm font-medium text-[var(--text-primary)]">{{ t("profile.currentOrganization") }}</div>
            <div class="text-xs text-[var(--text-secondary)]">{{ t("profile.currentOrganizationHint") }}</div>
          </div>
          <UBadge :color="currentTenant.is_owner ? 'primary' : currentTenant.is_admin ? 'success' : 'neutral'" variant="subtle">
            {{ currentTenantRoleLabel }}
          </UBadge>
        </div>
        <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
          <div>
            <div class="text-xs text-[var(--text-secondary)]">{{ t("profile.organizationName") }}</div>
            <div class="mt-1 text-sm text-[var(--text-primary)]">{{ currentTenant.tenant_name || "-" }}</div>
          </div>
          <div>
            <div class="text-xs text-[var(--text-secondary)]">{{ t("profile.organizationKey") }}</div>
            <div class="mt-1 font-mono text-sm text-[var(--text-primary)]">{{ currentTenant.tenant_key || "-" }}</div>
          </div>
          <div>
            <div class="text-xs text-[var(--text-secondary)]">{{ t("profile.domain") }}</div>
            <div class="mt-1 text-sm text-[var(--text-primary)]">{{ currentTenant.tenant_domain || "-" }}</div>
          </div>
        </div>
      </div>

      <UForm :state="form" @submit="saveProfile">
        <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
          <UFormField :label="t('profile.displayName')" required>
            <UInput v-model="form.display_name" :placeholder="t('profile.displayNamePlaceholder')" />
          </UFormField>

          <UFormField :label="t('profile.email')" required>
            <UInput v-model="form.email" type="email" :placeholder="t('profile.emailPlaceholder')" />
          </UFormField>

          <UFormField :label="t('profile.phone')">
            <UInput v-model="form.phone" :placeholder="t('profile.phonePlaceholder')" />
          </UFormField>

          <UFormField :label="t('profile.avatarUrl')">
            <UInput v-model="form.avatar_url" :placeholder="t('profile.avatarUrlPlaceholder')" />
          </UFormField>
        </div>

        <div class="mt-6 flex items-center justify-end gap-2 border-t border-[var(--border-subtle)] pt-4">
          <UButton size="sm" color="primary" variant="outline" @click="openPasswordModal">
            {{ t("profile.changePassword") }}
          </UButton>
          <UButton size="sm" color="neutral" variant="outline" @click="resetForm">{{ t("common.reset") }}</UButton>
          <UButton size="sm" type="submit" :loading="saving">{{ t("profile.saveProfile") }}</UButton>
        </div>
      </UForm>
    </UCard>

    <UModal
      v-model:open="showPasswordModal"
      :title="t('profile.changePassword')"
      :description="t('profile.changePasswordDescription')"
    >
      <template #body>
        <UForm
          id="profile-password-form"
          :state="passwordForm"
          class="space-y-4"
          @submit="submitPassword"
        >
          <input
            class="sr-only"
            type="text"
            name="username"
            autocomplete="username"
            :value="passwordUsername"
            readonly
            tabindex="-1"
          >
          <UFormField :label="t('profile.currentPassword')" required>
            <UInput
              v-model="passwordForm.current_password"
              type="password"
              autocomplete="current-password"
              :placeholder="t('profile.currentPasswordPlaceholder')"
            />
          </UFormField>
          <UFormField :label="t('profile.newPassword')" required>
            <UInput
              v-model="passwordForm.new_password"
              type="password"
              autocomplete="new-password"
              :placeholder="t('profile.newPasswordPlaceholder')"
            />
          </UFormField>
          <UFormField :label="t('profile.confirmPassword')" required>
            <UInput
              v-model="passwordForm.confirm_password"
              type="password"
              autocomplete="new-password"
              :placeholder="t('profile.confirmPasswordPlaceholder')"
            />
          </UFormField>
        </UForm>
      </template>
      <template #footer>
        <div class="flex w-full justify-end gap-2">
          <UButton
            type="button"
            color="neutral"
            variant="ghost"
            @click="showPasswordModal = false"
          >
            {{ t("common.cancel") }}
          </UButton>
          <UButton
            type="submit"
            form="profile-password-form"
            :loading="savingPassword"
          >
            {{ t("profile.confirmChangePassword") }}
          </UButton>
        </div>
      </template>
    </UModal>
  </div>
</template>

<script setup lang="ts">
import { useUserStore } from "~/stores/user";
import { useMe } from "~/composables/useMe";

definePageMeta({
  title: "profile.title",
});

const { t } = useI18n();
const toast = useToast();
const userStore = useUserStore();
const { updateMyProfile, changeMyPassword } = useMe();

const saving = ref(false);
const savingPassword = ref(false);
const showPasswordModal = ref(false);
const form = reactive({
  display_name: "",
  email: "",
  phone: "",
  avatar_url: "",
});
const passwordForm = reactive({
  current_password: "",
  new_password: "",
  confirm_password: "",
});
const passwordUsername = computed(
  () => userStore.user?.email || userStore.user?.phone || userStore.displayName || ""
);

const roleLabel = computed(() => {
  if (userStore.isRoot) return "Root";
  if (currentTenant.value?.is_owner) return t("profile.roles.owner");
  if (userStore.isCurrentTenantAdmin) return t("profile.roles.admin");
  return t("profile.roles.member");
});
const currentTenant = computed(() => userStore.currentTenant);
const currentTenantRoleLabel = computed(() => {
  if (!currentTenant.value) return "-";
  if (currentTenant.value.is_owner) return t("profile.roles.owner");
  if (currentTenant.value.is_admin) return t("profile.roles.admin");
  return t("profile.roles.member");
});

function syncFromStore() {
  form.display_name = userStore.user?.display_name || "";
  form.email = userStore.user?.email || "";
  form.phone = userStore.user?.phone || "";
  form.avatar_url = userStore.user?.avatar_url || "";
}

function resetForm() {
  syncFromStore();
}

function openPasswordModal() {
  passwordForm.current_password = "";
  passwordForm.new_password = "";
  passwordForm.confirm_password = "";
  showPasswordModal.value = true;
}

async function saveProfile() {
  const displayName = form.display_name.trim();
  const email = form.email.trim();
  const phone = form.phone.trim();
  const avatar = form.avatar_url.trim();

  if (!displayName) {
    toast.add({ title: t("profile.toasts.saveFailed"), description: t("profile.validation.displayNameRequired"), color: "error" });
    return;
  }
  if (!email) {
    toast.add({ title: t("profile.toasts.saveFailed"), description: t("profile.validation.emailRequired"), color: "error" });
    return;
  }

  saving.value = true;
  try {
    await updateMyProfile({
      display_name: displayName,
      email,
      phone,
      avatar_url: avatar,
    });
    await userStore.fetchUserContext({ force: true });
    syncFromStore();
    toast.add({ title: t("profile.toasts.saveSuccess"), description: t("profile.toasts.profileUpdated"), color: "success" });
  } catch (e: any) {
    toast.add({
      title: t("profile.toasts.saveFailed"),
      description: String(e?.data?.message || e?.message || t("common.retry")),
      color: "error",
    });
  } finally {
    saving.value = false;
  }
}

async function submitPassword() {
  const current = passwordForm.current_password.trim();
  const next = passwordForm.new_password.trim();
  const confirm = passwordForm.confirm_password.trim();

  if (!current || !next || !confirm) {
    toast.add({ title: t("profile.toasts.changeFailed"), description: t("profile.validation.passwordRequired"), color: "error" });
    return;
  }
  if (next.length < 6) {
    toast.add({ title: t("profile.toasts.changeFailed"), description: t("profile.validation.passwordTooShort"), color: "error" });
    return;
  }
  if (next !== confirm) {
    toast.add({ title: t("profile.toasts.changeFailed"), description: t("profile.validation.passwordMismatch"), color: "error" });
    return;
  }
  if (current === next) {
    toast.add({ title: t("profile.toasts.changeFailed"), description: t("profile.validation.passwordSame"), color: "error" });
    return;
  }

  savingPassword.value = true;
  try {
    await changeMyPassword({
      current_password: current,
      new_password: next,
    });
    showPasswordModal.value = false;
    toast.add({ title: t("profile.toasts.changeSuccess"), description: t("profile.toasts.passwordUpdated"), color: "success" });
  } catch (e: any) {
    toast.add({
      title: t("profile.toasts.changeFailed"),
      description: String(e?.data?.message || e?.message || t("common.retry")),
      color: "error",
    });
  } finally {
    savingPassword.value = false;
  }
}

onMounted(async () => {
  if (!userStore.user) {
    await userStore.fetchUserContext({ force: true });
  }
  syncFromStore();
});
</script>
