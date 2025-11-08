<template>
  <div class="min-h-screen bg-gray-50 p-8">
    <div class="max-w-4xl mx-auto">
      <h1 class="text-4xl font-bold text-gray-900 mb-8 text-center">
        Nuxt UI 组件测试页面
      </h1>

      <!-- 按钮测试 -->
      <UCard class="mb-6">
        <template #header>
          <h2 class="text-2xl font-semibold">按钮组件测试</h2>
        </template>

        <div class="space-y-4">
          <div class="flex flex-wrap gap-4">
            <UButton>默认按钮</UButton>
            <UButton color="primary">主要按钮</UButton>
            <UButton color="error">红色按钮</UButton>
            <UButton color="success">绿色按钮</UButton>
            <UButton variant="outline">轮廓按钮</UButton>
            <UButton variant="ghost">幽灵按钮</UButton>
          </div>

          <div class="flex flex-wrap gap-4">
            <UButton size="xs">超小按钮</UButton>
            <UButton size="sm">小按钮</UButton>
            <UButton size="md">中等按钮</UButton>
            <UButton size="lg">大按钮</UButton>
            <UButton size="xl">超大按钮</UButton>
          </div>

          <div class="flex flex-wrap gap-4">
            <UButton icon="i-heroicons-heart" />
            <UButton icon="i-heroicons-star" trailing>带图标</UButton>
            <UButton loading>加载中</UButton>
            <UButton disabled>禁用按钮</UButton>
          </div>
        </div>
      </UCard>

      <!-- 表单组件测试 -->
      <UCard class="mb-6">
        <template #header>
          <h2 class="text-2xl font-semibold">表单组件测试</h2>
        </template>

        <div class="space-y-4">
          <UInput placeholder="输入框测试" />
          <UTextarea placeholder="文本域测试" />
          <USelect
            :items="[
              { label: '选项1', value: '1' },
              { label: '选项2', value: '2' },
              { label: '选项3', value: '3' },
            ]"
            placeholder="选择一个选项"
          />
          <UCheckbox label="复选框测试" />
          <USwitch label="单选框测试" />
          <USwitch
            unchecked-icon="i-lucide-x"
            checked-icon="i-lucide-check"
            label="开关测试"
          />
        </div>
      </UCard>

      <!-- 反馈组件测试 -->
      <UCard class="mb-6">
        <template #header>
          <h2 class="text-2xl font-semibold">反馈组件测试</h2>
        </template>

        <div class="space-y-4">
          <div class="flex flex-wrap gap-4">
            <UBadge>默认徽章</UBadge>
            <UBadge color="error">红色徽章</UBadge>
            <UBadge color="success">绿色徽章</UBadge>
            <UBadge color="primanry">蓝色徽章</UBadge>
            <UBadge variant="outline">轮廓徽章</UBadge>
          </div>

          <UAlert
            icon="i-heroicons-information-circle"
            title="信息提示"
            description="这是一个信息提示框"
          />

          <UAlert
            color="error"
            icon="i-heroicons-exclamation-triangle"
            title="错误提示"
            description="这是一个错误提示框"
          />

          <UProgress :value="75" />

          <div class="flex items-center space-x-4">
            <UAvatar
              src="https://avatars.githubusercontent.com/u/739984?v=4"
              alt="头像"
            />
            <UAvatar
              size="sm"
              src="https://avatars.githubusercontent.com/u/739985?v=4"
              alt="小头像"
            />
            <UAvatar
              size="lg"
              src="https://avatars.githubusercontent.com/u/739986?v=4"
              alt="大头像"
            />
          </div>
        </div>
      </UCard>

      <!-- 图标测试 -->
      <UCard class="mb-6">
        <template #header>
          <h2 class="text-2xl font-semibold">图标测试</h2>
        </template>

        <div class="grid grid-cols-6 gap-4">
          <div class="text-center">
            <UIcon
              class="w-8 h-8 mx-auto mb-2 inline-block"
              name="i-heroicons-home"
            />
            <p class="text-sm">首页</p>
          </div>
          <div class="text-center">
            <UIcon
              class="w-8 h-8 mx-auto mb-2 inline-block"
              name="i-heroicons-user"
            />
            <p class="text-sm">用户</p>
          </div>
          <div class="text-center">
            <UIcon
              class="w-8 h-8 mx-auto mb-2 inline-block"
              name="i-heroicons-cog-6-tooth"
            />
            <p class="text-sm">设置</p>
          </div>
          <div class="text-center">
            <UIcon
              class="w-8 h-8 mx-auto mb-2 text-red-500 inline-block"
              name="i-heroicons-heart"
            />
            <p class="text-sm">喜欢</p>
          </div>
          <div class="text-center">
            <UIcon
              class="w-8 h-8 mx-auto mb-2 text-yellow-500 inline-block"
              name="i-heroicons-star"
            />
            <p class="text-sm">收藏</p>
          </div>
          <div class="text-center">
            <UIcon
              class="w-8 h-8 mx-auto mb-2 text-blue-500 inline-block"
              name="i-heroicons-bell"
            />
            <p class="text-sm">通知</p>
          </div>
        </div>
      </UCard>

      <!-- 导航测试 -->
      <div class="text-center">
        <div class="space-x-4">
          <NuxtLink to="/test-tailwind">
            <UButton color="primary">测试 Tailwind CSS</UButton>
          </NuxtLink>
          <NuxtLink to="/">
            <UButton variant="outline">返回首页</UButton>
          </NuxtLink>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
useHead({
  title: "Nuxt UI 组件测试页面",
  meta: [{ name: "description", content: "测试Nuxt UI组件是否正常工作" }],
});
</script>
