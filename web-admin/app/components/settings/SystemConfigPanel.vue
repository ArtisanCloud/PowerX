<template>
  <div class="space-y-6">
    <!-- 系统信息 -->
    <div class="bg-white dark:bg-gray-800 rounded-lg p-6 shadow-sm">
      <h3 class="text-lg font-semibold mb-4 flex items-center gap-2">
        <UIcon name="i-heroicons-cog-6-tooth" class="w-5 h-5" />
        {{ $t("system.info.title") }}
      </h3>

      <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
        <div class="space-y-2">
          <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ $t("system.info.appName") }}
          </label>
          <div class="text-sm text-gray-600 dark:text-gray-400">
            {{ systemConfigStore.config.appName }}
          </div>
        </div>

        <div class="space-y-2">
          <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ $t("system.info.appVersion") }}
          </label>
          <div class="text-sm text-gray-600 dark:text-gray-400">
            {{ systemConfigStore.config.appVersion }}
          </div>
        </div>
      </div>
    </div>

    <!-- 语言设置 -->
    <div class="bg-white dark:bg-gray-800 rounded-lg p-6 shadow-sm">
      <h3 class="text-lg font-semibold mb-4 flex items-center gap-2">
        <UIcon name="i-heroicons-language" class="w-5 h-5" />
        {{ $t("system.language.title") }}
      </h3>

      <div class="space-y-4">
        <div class="flex items-center justify-between">
          <div>
            <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ $t("system.language.current") }}
            </label>
            <div class="text-sm text-gray-600 dark:text-gray-400 mt-1">
              {{ systemConfigStore.currentLanguageInfo.flag }}
              {{ systemConfigStore.currentLanguageInfo.name }}
            </div>
          </div>

          <div class="flex items-center gap-2">
            <UBadge
              v-if="systemConfigStore.config.forceLanguage"
              color="orange"
              variant="soft"
            >
              {{ $t("system.language.forced") }}
            </UBadge>

            <USelect
              v-if="systemConfigStore.canSwitchLanguage"
              :model-value="systemConfigStore.currentLanguage"
              :items="languageOptions"
              @update:model-value="handleLanguageChange"
              class="w-40"
            >
              <template #label>
                <div class="flex items-center gap-2">
                  <span>{{ systemConfigStore.currentLanguageInfo.flag }}</span>
                  <span>{{ systemConfigStore.currentLanguageInfo.name }}</span>
                </div>
              </template>

              <template #option="{ option }">
                <div class="flex items-center gap-2">
                  <span>{{ option.flag }}</span>
                  <span>{{ option.name }}</span>
                </div>
              </template>
            </USelect>
          </div>
        </div>

        <div class="text-xs text-gray-500 dark:text-gray-400">
          {{ $t("system.language.available") }}:
          {{ systemConfigStore.config.availableLanguages.join(", ") }}
        </div>
      </div>
    </div>

    <!-- 主题设置 -->
    <div class="bg-white dark:bg-gray-800 rounded-lg p-6 shadow-sm">
      <h3 class="text-lg font-semibold mb-4 flex items-center gap-2">
        <UIcon
          :name="systemConfigStore.currentThemeInfo.icon"
          class="w-5 h-5"
        />
        {{ $t("system.theme.title") }}
      </h3>

      <div class="space-y-4">
        <div class="flex items-center justify-between">
          <div>
            <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ $t("system.theme.current") }}
            </label>
            <div class="text-sm text-gray-600 dark:text-gray-400 mt-1">
              {{ systemConfigStore.currentThemeInfo.name }}
            </div>
          </div>

          <div class="flex items-center gap-2">
            <UBadge
              v-if="systemConfigStore.config.forceTheme"
              color="orange"
              variant="soft"
            >
              {{ $t("system.theme.forced") }}
            </UBadge>

            <USelect
              v-if="systemConfigStore.canSwitchTheme"
              :model-value="systemConfigStore.currentTheme"
              :options="themeOptions"
              @update:model-value="handleThemeChange"
              class="w-40"
            >
              <template #label>
                <div class="flex items-center gap-2">
                  <UIcon
                    :name="systemConfigStore.currentThemeInfo.icon"
                    class="w-4 h-4"
                  />
                  <span>{{ systemConfigStore.currentThemeInfo.name }}</span>
                </div>
              </template>

              <template #option="{ option }">
                <div class="flex items-center gap-2">
                  <UIcon :name="option.icon" class="w-4 h-4" />
                  <span>{{ option.name }}</span>
                </div>
              </template>
            </USelect>
          </div>
        </div>

        <div class="text-xs text-gray-500 dark:text-gray-400">
          {{ $t("system.theme.available") }}:
          {{ systemConfigStore.config.availableThemes.join(", ") }}
        </div>
      </div>
    </div>

    <!-- 功能开关状态 -->
    <div
      v-if="systemConfigStore.isDebugMode"
      class="bg-yellow-50 dark:bg-yellow-900/20 rounded-lg p-6 border border-yellow-200 dark:border-yellow-800"
    >
      <h3
        class="text-lg font-semibold mb-4 flex items-center gap-2 text-yellow-800 dark:text-yellow-200"
      >
        <UIcon name="i-heroicons-bug-ant" class="w-5 h-5" />
        {{ $t("system.debug.title") }}
      </h3>

      <div class="space-y-2 text-sm">
        <div class="flex justify-between">
          <span>{{ $t("system.debug.languageSwitch") }}:</span>
          <UBadge
            :color="systemConfigStore.canSwitchLanguage ? 'green' : 'red'"
            variant="soft"
          >
            {{
              systemConfigStore.canSwitchLanguage
                ? $t("common.enabled")
                : $t("common.disabled")
            }}
          </UBadge>
        </div>

        <div class="flex justify-between">
          <span>{{ $t("system.debug.themeSwitch") }}:</span>
          <UBadge
            :color="systemConfigStore.canSwitchTheme ? 'green' : 'red'"
            variant="soft"
          >
            {{
              systemConfigStore.canSwitchTheme
                ? $t("common.enabled")
                : $t("common.disabled")
            }}
          </UBadge>
        </div>

        <div class="flex justify-between">
          <span>{{ $t("system.debug.userPreferences") }}:</span>
          <UBadge
            :color="systemConfigStore.userPreferencesEnabled ? 'green' : 'red'"
            variant="soft"
          >
            {{
              systemConfigStore.userPreferencesEnabled
                ? $t("common.enabled")
                : $t("common.disabled")
            }}
          </UBadge>
        </div>
      </div>
    </div>

    <!-- 操作按钮 -->
    <div class="flex justify-end gap-3">
      <UButton
        variant="outline"
        @click="handleReset"
        :disabled="!systemConfigStore.userPreferencesEnabled"
      >
        {{ $t("system.actions.reset") }}
      </UButton>

      <UButton
        v-if="systemConfigStore.isDebugMode"
        variant="outline"
        @click="showConfigInfo"
      >
        {{ $t("system.actions.showConfig") }}
      </UButton>
    </div>
  </div>
</template>

<script setup lang="ts">
const systemConfigStore = useSystemConfigStore();

// 语言选项
const languageOptions = computed(() =>
  systemConfigStore.availableLanguageOptions.map((lang) => ({
    label: `${lang.flag} ${lang.name}`,
    value: lang.code,
    ...lang,
  }))
);

// 主题选项
const themeOptions = computed(() =>
  systemConfigStore.availableThemeOptions.map((theme) => ({
    label: theme.name,
    value: theme.code,
    ...theme,
  }))
);

// 处理语言切换
const handleLanguageChange = async (language: string) => {
  await systemConfigStore.switchLanguage(language);
};

// 处理主题切换
const handleThemeChange = (theme: string) => {
  systemConfigStore.switchTheme(theme);
};

// 重置配置
const handleReset = async () => {
  await systemConfigStore.reset();
};

// 显示配置信息（调试用）
const showConfigInfo = () => {
  console.info("系统配置信息:", systemConfigStore.getConfigInfo());
  alert("配置信息已输出到控制台");
};
</script>
