<script setup lang="ts">
definePageMeta({
  layout: false, // 禁用layout
});

const { t } = useI18n();

// ========== 强制阅读功能开关 ==========
// 设置为 false 可以关闭强制阅读功能，用户可以直接勾选同意
// const ENABLE_FORCED_READING = ref(true);
const ENABLE_FORCED_READING = ref(false);
// =====================================

// 表单数据
const form = reactive({
  username: "",
  email: "",
  password: "",
  confirmPassword: "",
  agree: false,
});

// 表单验证状态
const loading = ref(false);
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
  if (!form.username.trim()) {
    error.value = t("auth.required");
    return false;
  }
  if (!form.email.trim()) {
    error.value = t("auth.required");
    return false;
  }
  if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(form.email)) {
    error.value = t("auth.invalidEmail");
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

    const registerData = {
      username: form.username,
      email: form.email,
      password: form.password,
      displayName: form.username, // 使用用户名作为显示名称
    };

    const response = await authService.registerFromForm(registerData);

    if (response.success) {
      success.value = true;
      countdown.value = 3;

      // 开始倒计时
      const timer = setInterval(() => {
        countdown.value--;
        if (countdown.value <= 0) {
          clearInterval(timer);
          // 跳转到首页
          const localePath = useLocalePath();
          navigateTo(localePath("/"));
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
                恭喜您注册成功！{{ countdown }}秒后将自动跳转到首页...
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

            <!-- 用户名输入 -->
            <div class="mb-6">
              <label
                for="username"
                class="block text-sm font-medium text-gray-700 mb-3"
              >
                {{ $t("auth.register.username") }}
                <span class="text-red-500">*</span>
              </label>
              <UInput
                id="username"
                name="username"
                v-model="form.username"
                :placeholder="$t('auth.register.username')"
                size="lg"
                :disabled="loading"
                class="w-full"
              />
            </div>

            <!-- 邮箱输入 -->
            <div class="mb-6">
              <label
                for="email"
                class="block text-sm font-medium text-gray-700 mb-3"
              >
                {{ $t("auth.email") }} <span class="text-red-500">*</span>
              </label>
              <UInput
                id="email"
                name="email"
                v-model="form.email"
                type="email"
                :placeholder="$t('auth.email')"
                size="lg"
                :disabled="loading"
                class="w-full"
              />
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
