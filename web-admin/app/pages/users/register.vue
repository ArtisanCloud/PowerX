<script setup lang="ts">
definePageMeta({
  layout: false, // 禁用layout
});

const { t } = useI18n();
const runtimeConfig = useRuntimeConfig();
const verificationEnabled = computed(
  () => String(runtimeConfig.public.saasSignupVerificationEnabled) === "true"
);

// ========== 强制阅读功能开关 ==========
// 设置为 false 可以关闭强制阅读功能，用户可以直接勾选同意
// const ENABLE_FORCED_READING = ref(true);
const ENABLE_FORCED_READING = ref(false);
// =====================================

// 表单数据
const form = reactive({
  tenantName: "",
  tenantKey: "",
  contact: "",
  verificationCode: "",
  password: "",
  confirmPassword: "",
  agree: false,
});
const tenantKeyTouched = ref(false);

// 表单验证状态
const loading = ref(false);
const sendingCode = ref(false);
const error = ref("");
const success = ref(false);
const countdown = ref(3);

// 阅读状态
const hasReadTerms = ref(false);
const hasReadPrivacy = ref(false);

// 计算是否可以勾选同意
const canAgree = computed(() => {
  // 如果关闭了强制阅读功能，直接返回 true
  if (!ENABLE_FORCED_READING.value) {
    return true;
  }
  // 否则需要阅读完条款和隐私政策
  return hasReadTerms.value && hasReadPrivacy.value;
});

// 弹层控制
const showTermsModal = ref(false);
const showPrivacyModal = ref(false);
const canCloseTerms = ref(false);
const canClosePrivacy = ref(false);

// 强制阅读逻辑
const termsReadingTime = ref(0);
const privacyReadingTime = ref(0);
const termsScrolledToBottom = ref(false);
const privacyScrolledToBottom = ref(false);
const termsTimer = ref<NodeJS.Timeout | null>(null);
const privacyTimer = ref<NodeJS.Timeout | null>(null);

const MIN_READING_TIME = 5; // 最少阅读5秒

// 打开条款弹层
const openTermsModal = (e: Event) => {
  e.preventDefault();
  // 重置阅读状态
  canCloseTerms.value = false;
  termsReadingTime.value = 0;
  termsScrolledToBottom.value = false;
  showTermsModal.value = true;

  // 开始计时
  startTermsTimer();
};

// 打开隐私政策弹层
const openPrivacyModal = (e: Event) => {
  e.preventDefault();
  // 重置阅读状态
  canClosePrivacy.value = false;
  privacyReadingTime.value = 0;
  privacyScrolledToBottom.value = false;
  showPrivacyModal.value = true;

  // 开始计时
  startPrivacyTimer();
};

// 开始条款阅读计时
const startTermsTimer = () => {
  if (termsTimer.value) {
    clearInterval(termsTimer.value);
  }

  termsTimer.value = setInterval(() => {
    termsReadingTime.value++;

    // 检查是否满足解锁条件
    if (
      termsReadingTime.value >= MIN_READING_TIME &&
      termsScrolledToBottom.value
    ) {
      canCloseTerms.value = true;
      if (termsTimer.value) {
        clearInterval(termsTimer.value);
        termsTimer.value = null;
      }
    }
  }, 1000);
};

// 开始隐私政策阅读计时
const startPrivacyTimer = () => {
  if (privacyTimer.value) {
    clearInterval(privacyTimer.value);
  }

  privacyTimer.value = setInterval(() => {
    privacyReadingTime.value++;

    // 检查是否满足解锁条件
    if (
      privacyReadingTime.value >= MIN_READING_TIME &&
      privacyScrolledToBottom.value
    ) {
      canClosePrivacy.value = true;
      if (privacyTimer.value) {
        clearInterval(privacyTimer.value);
        privacyTimer.value = null;
      }
    }
  }, 1000);
};

// 处理条款滚动
const handleTermsScroll = (event: Event) => {
  const target = event.target as HTMLElement;
  const scrollTop = target.scrollTop;
  const scrollHeight = target.scrollHeight;
  const clientHeight = target.clientHeight;

  // 检查是否滚动到底部（允许10px的误差）
  if (scrollTop + clientHeight >= scrollHeight - 10) {
    termsScrolledToBottom.value = true;
  }
};

// 处理隐私政策滚动
const handlePrivacyScroll = (event: Event) => {
  const target = event.target as HTMLElement;
  const scrollTop = target.scrollTop;
  const scrollHeight = target.scrollHeight;
  const clientHeight = target.clientHeight;

  // 检查是否滚动到底部（允许10px的误差）
  if (scrollTop + clientHeight >= scrollHeight - 10) {
    privacyScrolledToBottom.value = true;
  }
};

// 计算剩余时间
const termsRemainingTime = computed(() => {
  return Math.max(0, MIN_READING_TIME - termsReadingTime.value);
});

const privacyRemainingTime = computed(() => {
  return Math.max(0, MIN_READING_TIME - privacyReadingTime.value);
});

// 处理条款同意
const handleTermsAgree = (agreed: boolean) => {
  // 清理计时器
  if (termsTimer.value) {
    clearInterval(termsTimer.value);
    termsTimer.value = null;
  }

  showTermsModal.value = false;
  if (agreed) {
    hasReadTerms.value = true;
    if (canAgree.value && !form.agree) {
      form.agree = true;
    }
  }
};

// 处理隐私政策同意
const handlePrivacyAgree = (agreed: boolean) => {
  // 清理计时器
  if (privacyTimer.value) {
    clearInterval(privacyTimer.value);
    privacyTimer.value = null;
  }

  showPrivacyModal.value = false;
  if (agreed) {
    hasReadPrivacy.value = true;
    if (canAgree.value && !form.agree) {
      form.agree = true;
    }
  }
};

// 组件卸载时清理计时器
onUnmounted(() => {
  if (termsTimer.value) {
    clearInterval(termsTimer.value);
  }
  if (privacyTimer.value) {
    clearInterval(privacyTimer.value);
  }
});

// 密码强度检查
const passwordStrength = computed(() => {
  const password = form.password;
  if (!password) return { level: 0, text: "", color: "gray" };

  let score = 0;
  if (password.length >= 8) score++;
  if (/[A-Z]/.test(password)) score++;
  if (/[a-z]/.test(password)) score++;
  if (/[0-9]/.test(password)) score++;
  if (/[^A-Za-z0-9]/.test(password)) score++;

  if (score <= 2)
    return {
      level: score,
      text: t("auth.passwordStrength.weak"),
      color: "red",
    };
  if (score <= 3)
    return {
      level: score,
      text: t("auth.passwordStrength.medium"),
      color: "yellow",
    };
  return {
    level: score,
    text: t("auth.passwordStrength.strong"),
    color: "green",
  };
});

// 表单验证
const validateForm = () => {
  if (!form.tenantName.trim()) {
    error.value = "请填写租户名称";
    return false;
  }
  if (!form.tenantKey.trim()) {
    error.value = "请填写组织标识";
    return false;
  }
  if (!/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(form.tenantKey.trim())) {
    error.value = "组织标识只能使用小写字母、数字和中横线";
    return false;
  }
  if (!form.contact.trim()) {
    error.value = t("auth.required");
    return false;
  }
  if (form.contact.includes("@") && !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(form.contact)) {
    error.value = t("auth.invalidEmail");
    return false;
  }
  if (verificationEnabled.value && !form.verificationCode.trim()) {
    error.value = t("auth.required");
    return false;
  }
  if (verificationEnabled.value && !/^\d{6}$/.test(form.verificationCode.trim())) {
    error.value = "请输入 6 位验证码";
    return false;
  }
  if (!form.password) {
    error.value = t("auth.required");
    return false;
  }
  if (form.password.length < 6) {
    error.value = t("auth.passwordTooShort");
    return false;
  }
  if (form.password !== form.confirmPassword) {
    error.value = t("auth.passwordMismatch");
    return false;
  }
  if (!form.agree) {
    error.value = t("auth.mustAgreeTerms");
    return false;
  }
  return true;
};

const sendVerificationCode = async () => {
  const contact = form.contact.trim();
  if (!contact) {
    error.value = "请先填写邮箱或手机号";
    return;
  }
  if (contact.includes("@") && !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(contact)) {
    error.value = t("auth.invalidEmail");
    return;
  }
  sendingCode.value = true;
  error.value = "";
  try {
    const { useAuthService } = await import(
      "~/composables/api/services/authService"
    );
    const authService = useAuthService();
    await authService.sendSignupVerificationCode(contact);
  } catch (err: any) {
    error.value = err.response?.data?.message || err.message || "验证码发送失败";
  } finally {
    sendingCode.value = false;
  }
};

// 注册处理
const handleRegister = async () => {
  if (!validateForm()) return;

  loading.value = true;
  error.value = "";

  try {
    // 调用注册接口
    const { useAuthService } = await import(
      "~/composables/api/services/authService"
    );
    const authService = useAuthService();
    const { setAuth } = useAuth();

    const registerData = {
      tenantName: form.tenantName,
      tenantKey: form.tenantKey,
      contact: form.contact,
      password: form.password,
      confirmPassword: form.confirmPassword,
      verificationCode: verificationEnabled.value ? form.verificationCode : "",
      displayName: form.contact,
    };

    const response = await authService.registerFromForm(registerData);

    if (response.code === 200 && response.data?.access_token) {
      setAuth({
        token_type: response.data.token_type || "Bearer",
        access_token: response.data.access_token,
        refresh_token: response.data.refresh_token,
        expires_in: response.data.expires_in || 3600,
        scope: response.data.scope || "access",
      });
      if (process.client && response.data.context?.current_tenant_uuid) {
        localStorage.setItem(
          "px_current_tenant_uuid",
          String(response.data.context.current_tenant_uuid)
        );
      }
      success.value = true;
      countdown.value = 3;

      // 开始倒计时
      const timer = setInterval(() => {
        countdown.value--;
        if (countdown.value <= 0) {
          clearInterval(timer);
          const localePath = useLocalePath();
          navigateTo(localePath("/agent"));
        }
      }, 1000);
    } else {
      error.value = response.message || t("auth.registerFailed");
    }
  } catch (err: any) {
    console.error("注册失败:", err);

    // 处理不同类型的错误
    if (err.response?.data?.message) {
      error.value = err.response.data.message;
    } else if (err.message) {
      error.value = err.message;
    } else {
      error.value = t("auth.registerFailed");
    }
  } finally {
    loading.value = false;
  }
};

const slugifyTenantKey = (value: string) =>
  value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .replace(/-{2,}/g, "-");

watch(
  () => form.tenantName,
  (value) => {
    if (!tenantKeyTouched.value) {
      form.tenantKey = slugifyTenantKey(value);
    }
  }
);

const handleTenantKeyInput = (value: string) => {
  tenantKeyTouched.value = true;
  form.tenantKey = slugifyTenantKey(value);
};
</script>

<template>
  <div
    class="min-h-screen bg-gradient-to-br from-blue-50 via-white to-purple-50 flex items-center justify-center p-4"
  >
    <div class="max-w-md w-full">
      <!-- 返回首页链接和语言切换器 -->
      <div class="mb-6 flex justify-between items-center">
        <NuxtLink
          :to="$localePath('/')"
          class="inline-flex items-center text-gray-600 hover:text-blue-600 transition-colors text-sm"
        >
          <svg
            class="w-4 h-4 mr-2"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M15 19l-7-7 7-7"
            ></path>
          </svg>
          {{ $t("auth.backToHome") }}
        </NuxtLink>
      </div>

      <!-- 注册卡片 -->
      <UCard class="shadow-xl border-0">
        <template #header>
          <div class="text-center py-6">
            <!-- Logo -->
            <h1
              class="text-3xl font-bold bg-gradient-to-r from-blue-600 to-purple-600 bg-clip-text text-transparent mb-3"
            >
              PowerX
            </h1>
            <h2 class="text-xl font-semibold text-gray-900 mb-2">
              {{ $t("auth.createAccount") }}
            </h2>
            <p class="text-gray-600 text-sm">
              {{ $t("auth.registerSubtitle") }}
            </p>
          </div>
        </template>

        <div class="px-6 pb-6">
          <!-- 成功提示 -->
          <UAlert v-if="success" color="success" variant="soft" class="mb-6">
            <template #title>
              <div class="flex items-center space-x-2">
                <UIcon
                  name="i-heroicons-check-circle"
                  class="w-5 h-5 text-green-500"
                />
                <span class="text-green-800 font-semibold">注册成功！</span>
              </div>
            </template>
            <template #description>
              <p class="text-green-700">
                恭喜您注册成功！{{ countdown }}秒后将自动进入工作台...
              </p>
            </template>
          </UAlert>

          <form v-else @submit.prevent="handleRegister" class="space-y-5">
            <!-- 错误提示 -->
            <UAlert
              v-if="error"
              color="error"
              variant="soft"
              :title="error"
              :close-button="{
                icon: 'i-heroicons-x-mark-20-solid',
                color: 'gray',
                variant: 'link',
                padded: false,
              }"
              @close="error = ''"
              class="mb-1"
            />

            <!-- 租户名称输入 -->
            <div class="mb-6">
              <label
                for="tenantName"
                class="block text-sm font-medium text-gray-700 mb-3"
              >
                租户名称 <span class="text-red-500">*</span>
              </label>
              <UInput
                id="tenantName"
                name="tenantName"
                v-model="form.tenantName"
                placeholder="Acme Inc"
                size="lg"
                :disabled="loading"
                class="w-full"
              />
            </div>

            <!-- 联系方式输入 -->
            <div class="mb-6">
              <label
                for="tenantKey"
                class="block text-sm font-medium text-gray-700 mb-3"
              >
                组织标识 <span class="text-red-500">*</span>
              </label>
              <UInput
                id="tenantKey"
                name="tenantKey"
                :model-value="form.tenantKey"
                placeholder="acme-inc"
                size="lg"
                :disabled="loading"
                class="w-full font-mono"
                @update:model-value="handleTenantKeyInput(String($event || ''))"
              />
              <p class="mt-2 text-xs text-gray-500">
                用于生成系统唯一标识，只能使用小写字母、数字和中横线。
              </p>
            </div>

            <!-- 联系方式输入 -->
            <div class="mb-6">
              <label
                for="contact"
                class="block text-sm font-medium text-gray-700 mb-3"
              >
                邮箱或手机号
                <span class="text-red-500">*</span>
              </label>
              <UInput
                id="contact"
                name="contact"
                v-model="form.contact"
                placeholder="owner@example.com / 13800000000"
                size="lg"
                :disabled="loading"
                class="w-full"
              />
            </div>

            <!-- 验证码输入 -->
            <div v-if="verificationEnabled" class="mb-6">
              <label
                for="verificationCode"
                class="block text-sm font-medium text-gray-700 mb-3"
              >
                验证码 <span class="text-red-500">*</span>
              </label>
              <div class="flex gap-2">
                <UInput
                  id="verificationCode"
                  name="verificationCode"
                  v-model="form.verificationCode"
                  placeholder="6 位验证码"
                  size="lg"
                  :disabled="loading"
                  class="min-w-0 flex-1"
                />
                <UButton
                  type="button"
                  color="primary"
                  variant="soft"
                  :loading="sendingCode"
                  :disabled="loading || sendingCode"
                  @click="sendVerificationCode"
                >
                  发送验证码
                </UButton>
              </div>
            </div>

            <!-- 密码输入 -->
            <div class="mb-6">
              <label
                for="password"
                class="block text-sm font-medium text-gray-700 mb-3"
              >
                {{ $t("auth.password") }} <span class="text-red-500">*</span>
              </label>
              <UInput
                id="password"
                name="password"
                v-model="form.password"
                type="password"
                :placeholder="$t('auth.password')"
                size="lg"
                :disabled="loading"
                class="w-full"
              />
              <!-- 密码强度指示器 -->
              <div v-if="form.password" class="mt-3">
                <div class="flex items-center space-x-2">
                  <div class="flex-1 bg-gray-200 rounded-full h-2">
                    <div
                      class="h-2 rounded-full transition-all duration-300"
                      :class="{
                        'bg-red-500': passwordStrength.color === 'red',
                        'bg-yellow-500': passwordStrength.color === 'yellow',
                        'bg-green-500': passwordStrength.color === 'green',
                      }"
                      :style="{
                        width: `${(passwordStrength.level / 5) * 100}%`,
                      }"
                    ></div>
                  </div>
                  <span
                    class="text-xs font-medium"
                    :class="{
                      'text-red-600': passwordStrength.color === 'red',
                      'text-yellow-600': passwordStrength.color === 'yellow',
                      'text-green-600': passwordStrength.color === 'green',
                    }"
                  >
                    {{ passwordStrength.text }}
                  </span>
                </div>
              </div>
            </div>

            <!-- 确认密码输入 -->
            <div class="mb-6">
              <label
                for="confirmPassword"
                class="block text-sm font-medium text-gray-700 mb-3"
              >
                {{ $t("auth.register.confirmPassword") }}
                <span class="text-red-500">*</span>
              </label>
              <UInput
                id="confirmPassword"
                name="confirmPassword"
                v-model="form.confirmPassword"
                type="password"
                :placeholder="$t('auth.register.confirmPassword')"
                size="lg"
                :disabled="loading"
                class="w-full"
              />
            </div>

            <!-- 同意条款 -->
            <div class="flex items-start space-x-3 pt-1">
              <UCheckbox
                v-model="form.agree"
                :disabled="loading || !canAgree"
                class="mt-0.5"
              />
              <p class="text-sm text-gray-600 leading-relaxed">
                {{ $t("auth.agreeTerms") }}
                <a
                  href="#"
                  role="button"
                  aria-controls="terms-modal"
                  @click="openTermsModal"
                  class="text-blue-600 hover:text-blue-700 underline"
                  >{{ $t("termsOfService") }}</a
                >
                {{ $t("auth.and") }}
                <a
                  href="#"
                  role="button"
                  aria-controls="privacy-modal"
                  @click="openPrivacyModal"
                  class="text-blue-600 hover:text-blue-700 underline"
                  >{{ $t("privacyPolicy") }}</a
                >
              </p>
            </div>

            <!-- 阅读状态提示 -->
            <div
              v-if="ENABLE_FORCED_READING && !canAgree"
              class="text-xs text-amber-600 dark:text-amber-400 flex items-center space-x-1"
            >
              <UIcon name="i-heroicons-information-circle" class="w-4 h-4" />
              <span>请先阅读并同意服务条款和隐私政策</span>
            </div>
            <div
              v-else-if="ENABLE_FORCED_READING && canAgree && !form.agree"
              class="text-xs text-green-600 dark:text-green-400 flex items-center space-x-1"
            >
              <UIcon name="i-heroicons-check-circle" class="w-4 h-4" />
              <span>您已完成阅读，现在可以勾选同意</span>
            </div>
            <div
              v-else-if="!ENABLE_FORCED_READING"
              class="text-xs text-blue-600 dark:text-blue-400 flex items-center space-x-1"
            >
              <UIcon name="i-heroicons-information-circle" class="w-4 h-4" />
              <span>{{ $t("auth.termsInfo") }}</span>
            </div>

            <!-- 注册按钮 -->
            <div class="pt-2">
              <UButton
                type="submit"
                block
                size="lg"
                :loading="loading"
                :disabled="!form.agree"
                class="bg-gradient-to-r from-blue-600 to-purple-600 hover:from-blue-700 hover:to-purple-700"
              >
                {{
                  loading
                    ? $t("auth.creatingAccount")
                    : $t("auth.createAccount")
                }}
              </UButton>
            </div>
          </form>

          <!-- 登录链接 -->
          <div class="text-center mt-6 pt-4 border-t border-gray-200">
            <p class="text-gray-600 text-sm">
              {{ $t("auth.hasAccount") }}
              <NuxtLink
                :to="$localePath('/users/login')"
                class="text-blue-600 hover:text-blue-700 font-medium"
              >
                {{ $t("auth.signInNow") }}
              </NuxtLink>
            </p>
          </div>
        </div>
      </UCard>
    </div>

    <!-- 条款 -->
    <UModal
      v-model:open="showTermsModal"
      title="register-terms-title"
      description="register-terms-desc"
      :prevent-close="!canCloseTerms"
      :ui="{
        panel: 'w-full max-w-3xl max-h-[85dvh] overflow-hidden flex flex-col',
      }"
    >
      <template #content>
        <header
          class="px-4 py-3 border-b shrink-0 flex justify-between items-center"
        >
          <h3 class="text-base font-semibold">服务条款</h3>
          <UButton
            variant="ghost"
            icon="i-heroicons-x-mark"
            :disabled="!canCloseTerms"
            @click="handleTermsAgree(false)"
          />
        </header>

        <div
          class="flex-1 min-h-0 overflow-y-auto overscroll-contain"
          @scroll="handleTermsScroll"
        >
          <!-- 阅读进度提示 -->
          <div
            class="p-4 bg-blue-50 dark:bg-blue-900/20 border-b border-blue-200 dark:border-blue-800"
          >
            <div class="flex items-center justify-between text-sm">
              <div class="flex items-center space-x-4">
                <div class="flex items-center space-x-2">
                  <UIcon
                    :name="
                      termsScrolledToBottom
                        ? 'i-heroicons-check-circle'
                        : 'i-heroicons-arrow-down'
                    "
                    :class="
                      termsScrolledToBottom ? 'text-green-500' : 'text-gray-400'
                    "
                    class="w-4 h-4"
                  />
                  <span
                    :class="
                      termsScrolledToBottom ? 'text-green-600' : 'text-gray-500'
                    "
                  >
                    {{
                      termsScrolledToBottom ? "已阅读完整内容" : "请滚动到底部"
                    }}
                  </span>
                </div>

                <div class="flex items-center space-x-2">
                  <UIcon
                    :name="
                      termsRemainingTime === 0
                        ? 'i-heroicons-check-circle'
                        : 'i-heroicons-clock'
                    "
                    :class="
                      termsRemainingTime === 0
                        ? 'text-green-500'
                        : 'text-gray-400'
                    "
                    class="w-4 h-4"
                  />
                  <span
                    :class="
                      termsRemainingTime === 0
                        ? 'text-green-600'
                        : 'text-gray-500'
                    "
                  >
                    {{
                      termsRemainingTime === 0
                        ? "阅读时间充足"
                        : `还需 ${termsRemainingTime} 秒`
                    }}
                  </span>
                </div>
              </div>

              <!-- 解锁状态 -->
              <div
                v-if="canCloseTerms"
                class="flex items-center space-x-2 text-green-600"
              >
                <UIcon name="i-heroicons-check-circle" class="w-4 h-4" />
                <span class="text-sm font-medium">可以同意条款</span>
              </div>
            </div>
          </div>

          <UsersTermsModal />
        </div>

        <footer class="px-4 py-3 border-t shrink-0 flex justify-end gap-2">
          <UButton
            variant="soft"
            :disabled="!canCloseTerms"
            @click="handleTermsAgree(false)"
            >关闭</UButton
          >
          <UButton
            color="primary"
            :disabled="!canCloseTerms"
            @click="handleTermsAgree(true)"
            >我已阅读并同意</UButton
          >
        </footer>
      </template>
    </UModal>

    <!-- 隐私 -->
    <UModal
      title="register-privacy-title"
      description="register-privacy-desc"
      v-model:open="showPrivacyModal"
      :prevent-close="!canClosePrivacy"
      :ui="{
        panel: 'w-full max-w-3xl max-h-[85dvh] overflow-hidden flex flex-col',
      }"
    >
      <template #content>
        <header
          class="px-4 py-3 border-b shrink-0 flex justify-between items-center"
        >
          <h3 class="text-base font-semibold">隐私政策</h3>
          <UButton
            variant="ghost"
            icon="i-heroicons-x-mark"
            :disabled="!canClosePrivacy"
            @click="handlePrivacyAgree(false)"
          />
        </header>

        <div
          class="flex-1 min-h-0 overflow-y-auto overscroll-contain"
          @scroll="handlePrivacyScroll"
        >
          <!-- 阅读进度提示 -->
          <div
            class="p-4 bg-blue-50 dark:bg-blue-900/20 border-b border-blue-200 dark:border-blue-800"
          >
            <div class="flex items-center justify-between text-sm">
              <div class="flex items-center space-x-4">
                <div class="flex items-center space-x-2">
                  <UIcon
                    :name="
                      privacyScrolledToBottom
                        ? 'i-heroicons-check-circle'
                        : 'i-heroicons-arrow-down'
                    "
                    :class="
                      privacyScrolledToBottom
                        ? 'text-green-500'
                        : 'text-gray-400'
                    "
                    class="w-4 h-4"
                  />
                  <span
                    :class="
                      privacyScrolledToBottom
                        ? 'text-green-600'
                        : 'text-gray-500'
                    "
                  >
                    {{
                      privacyScrolledToBottom
                        ? "已阅读完整内容"
                        : "请滚动到底部"
                    }}
                  </span>
                </div>

                <div class="flex items-center space-x-2">
                  <UIcon
                    :name="
                      privacyRemainingTime === 0
                        ? 'i-heroicons-check-circle'
                        : 'i-heroicons-clock'
                    "
                    :class="
                      privacyRemainingTime === 0
                        ? 'text-green-500'
                        : 'text-gray-400'
                    "
                    class="w-4 h-4"
                  />
                  <span
                    :class="
                      privacyRemainingTime === 0
                        ? 'text-green-600'
                        : 'text-gray-500'
                    "
                  >
                    {{
                      privacyRemainingTime === 0
                        ? "阅读时间充足"
                        : `还需 ${privacyRemainingTime} 秒`
                    }}
                  </span>
                </div>
              </div>

              <!-- 解锁状态 -->
              <div
                v-if="canClosePrivacy"
                class="flex items-center space-x-2 text-green-600"
              >
                <UIcon name="i-heroicons-check-circle" class="w-4 h-4" />
                <span class="text-sm font-medium">可以同意政策</span>
              </div>
            </div>
          </div>

          <UsersPrivacyModal />
        </div>

        <footer class="px-4 py-3 border-t shrink-0 flex justify-end gap-2">
          <UButton
            variant="soft"
            :disabled="!canClosePrivacy"
            @click="handlePrivacyAgree(false)"
            >关闭</UButton
          >
          <UButton
            color="primary"
            :disabled="!canClosePrivacy"
            @click="handlePrivacyAgree(true)"
            >我已阅读并同意</UButton
          >
        </footer>
      </template>
    </UModal>
  </div>
</template>
