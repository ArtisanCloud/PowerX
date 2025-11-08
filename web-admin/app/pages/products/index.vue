<script setup lang="ts">
definePageMeta({
  title: '产品管理',
  icon: 'i-heroicons-squares-2x2',
  order: 5
})

const { t } = useI18n()
</script>

<template>
  <div class="p-6">
    <div class="mb-6">
      <h1 class="text-2xl font-bold text-gray-900">{{ t('menu.productList') }}</h1>
      <p class="text-gray-600 mt-2">管理您的产品信息</p>
    </div>
    
    <div class="bg-white rounded-lg shadow p-6">
      <div class="text-center py-12">
                <UIcon class="w-16 h-16 text-gray-400 mx-auto mb-4 inline-block" name="i-heroicons-squares-2x2"  />
        <h3 class="text-lg font-medium text-gray-900 mb-2">产品列表</h3>
        <p class="text-gray-500">产品管理功能正在开发中...</p>
      </div>
    </div>
  </div>
</template>