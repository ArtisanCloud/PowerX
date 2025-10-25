<script setup lang="ts">
definePageMeta({
  title: '订单管理',
  icon: 'i-heroicons-shopping-cart',
  order: 7
})

const { t } = useI18n()
</script>

<template>
  <div class="p-6">
    <div class="mb-6">
      <h1 class="text-2xl font-bold text-gray-900">{{ t('menu.orders') }}</h1>
      <p class="text-gray-600 mt-2">管理订单和交易记录</p>
    </div>
    
    <div class="bg-white rounded-lg shadow p-6">
      <div class="text-center py-12">
                  <UIcon class="w-16 h-16 text-gray-400 mx-auto mb-4 inline-block" name="i-heroicons-shopping-cart"  />

        <h3 class="text-lg font-medium text-gray-900 mb-2">订单列表</h3>
        <p class="text-gray-500">订单管理功能正在开发中...</p>
        <UBadge color="primary" class="mt-2">新功能</UBadge>
      </div>
    </div>
  </div>
</template>