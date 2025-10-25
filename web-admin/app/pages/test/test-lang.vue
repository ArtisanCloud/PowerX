<template>
  <div class="p-8">
    <h1 class="text-2xl mb-4">语言切换器测试</h1>
    <p class="mb-4">当前语言: {{ $i18n.locale }}</p>
    <LanguageSwitcher />
    <div class="mt-4">
      <p>{{ $t('welcome') }}</p>
      <p>{{ $t('login') }}</p>
      <p>{{ $t('register') }}</p>
    </div>
  </div>
</template>