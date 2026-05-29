<template>
  <div class="mx-auto max-w-4xl p-6">
    <div class="mb-6">
      <div>
        <h1 class="text-2xl font-semibold text-[var(--text-primary)]">个人中心</h1>
        <p class="mt-1 text-sm text-[var(--text-secondary)]">管理当前登录账号的个人资料</p>
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
            <div class="text-sm font-medium text-[var(--text-primary)]">当前组织</div>
            <div class="text-xs text-[var(--text-secondary)]">当前 token 对应的租户身份，不属于个人资料字段</div>
          </div>
          <UBadge :color="currentTenant.is_owner ? 'primary' : currentTenant.is_admin ? 'success' : 'neutral'" variant="subtle">
            {{ currentTenantRoleLabel }}
          </UBadge>
        </div>
        <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
          <div>
            <div class="text-xs text-[var(--text-secondary)]">组织名称</div>
            <div class="mt-1 text-sm text-[var(--text-primary)]">{{ currentTenant.tenant_name || "-" }}</div>
          </div>
          <div>
            <div class="text-xs text-[var(--text-secondary)]">组织标识</div>
            <div class="mt-1 font-mono text-sm text-[var(--text-primary)]">{{ currentTenant.tenant_key || "-" }}</div>
          </div>
          <div>
            <div class="text-xs text-[var(--text-secondary)]">租户 UUID</div>
            <div class="mt-1 break-all font-mono text-xs text-[var(--text-primary)]">{{ currentTenant.tenant_uuid }}</div>
          </div>
          <div>
            <div class="text-xs text-[var(--text-secondary)]">域名</div>
            <div class="mt-1 text-sm text-[var(--text-primary)]">{{ currentTenant.tenant_domain || "-" }}</div>
          </div>
        </div>
      </div>

      <UForm :state="form" @submit="saveProfile">
        <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
          <UFormField label="显示名称" required>
            <UInput v-model="form.display_name" placeholder="请输入显示名称" />
          </UFormField>

          <UFormField label="邮箱" required>
            <UInput v-model="form.email" type="email" placeholder="请输入邮箱" />
          </UFormField>

          <UFormField label="手机号">
            <UInput v-model="form.phone" placeholder="请输入手机号" />
          </UFormField>

          <UFormField label="头像 URL">
            <UInput v-model="form.avatar_url" placeholder="https://..." />
          </UFormField>
        </div>

        <div class="mt-6 flex items-center justify-end gap-2 border-t border-[var(--border-subtle)] pt-4">
          <UButton size="sm" color="primary" variant="outline" @click="openPasswordModal">
            修改密码
          </UButton>
          <UButton size="sm" color="neutral" variant="outline" @click="resetForm">重置</UButton>
          <UButton size="sm" type="submit" :loading="saving">保存资料</UButton>
        </div>
      </UForm>
    </UCard>

    <UModal v-model:open="showPasswordModal" title="修改密码">
      <template #body>
        <div class="space-y-4">
          <UFormField label="当前密码" required>
            <UInput v-model="passwordForm.current_password" type="password" placeholder="请输入当前密码" />
          </UFormField>
          <UFormField label="新密码" required>
            <UInput v-model="passwordForm.new_password" type="password" placeholder="至少 6 位" />
          </UFormField>
          <UFormField label="确认新密码" required>
            <UInput v-model="passwordForm.confirm_password" type="password" placeholder="再次输入新密码" />
          </UFormField>
        </div>
      </template>
      <template #footer>
        <div class="flex w-full justify-end gap-2">
          <UButton color="neutral" variant="ghost" @click="showPasswordModal = false">取消</UButton>
          <UButton :loading="savingPassword" @click="submitPassword">确认修改</UButton>
        </div>
      </template>
    </UModal>
  </div>
</template>

<script setup lang="ts">
import { useUserStore } from "~/stores/user";
import { useMe } from "~/composables/useMe";

definePageMeta({
  title: "个人中心",
});

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

const roleLabel = computed(() => {
  if (userStore.isRoot) return "Root";
  if (currentTenant.value?.is_owner) return "租户所有者";
  if (userStore.isCurrentTenantAdmin) return "租户管理员";
  return "普通成员";
});
const currentTenant = computed(() => userStore.currentTenant);
const currentTenantRoleLabel = computed(() => {
  if (!currentTenant.value) return "-";
  if (currentTenant.value.is_owner) return "租户所有者";
  if (currentTenant.value.is_admin) return "租户管理员";
  return "普通成员";
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
    toast.add({ title: "保存失败", description: "显示名称不能为空", color: "error" });
    return;
  }
  if (!email) {
    toast.add({ title: "保存失败", description: "邮箱不能为空", color: "error" });
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
    toast.add({ title: "保存成功", description: "个人资料已更新", color: "success" });
  } catch (e: any) {
    toast.add({
      title: "保存失败",
      description: String(e?.data?.message || e?.message || "请稍后重试"),
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
    toast.add({ title: "修改失败", description: "请填写完整密码信息", color: "error" });
    return;
  }
  if (next.length < 6) {
    toast.add({ title: "修改失败", description: "新密码至少 6 位", color: "error" });
    return;
  }
  if (next !== confirm) {
    toast.add({ title: "修改失败", description: "两次输入的新密码不一致", color: "error" });
    return;
  }
  if (current === next) {
    toast.add({ title: "修改失败", description: "新密码不能与当前密码相同", color: "error" });
    return;
  }

  savingPassword.value = true;
  try {
    await changeMyPassword({
      current_password: current,
      new_password: next,
    });
    showPasswordModal.value = false;
    toast.add({ title: "修改成功", description: "密码已更新，请使用新密码重新登录", color: "success" });
  } catch (e: any) {
    toast.add({
      title: "修改失败",
      description: String(e?.data?.message || e?.message || "请稍后重试"),
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
