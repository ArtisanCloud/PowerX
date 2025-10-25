<template>
  <div class="p-6 prose dark:prose-invert max-w-none space-y-6">
    <!-- 隐私政策内容 -->
    <section>
      <h2 class="text-xl font-semibold text-gray-900 dark:text-white mb-4">
        1. 信息收集
      </h2>
      <div class="space-y-4 text-gray-700 dark:text-gray-300">
        <p>我们收集您提供的信息以及您使用我们服务时自动收集的信息。</p>
        <div class="bg-gray-50 dark:bg-gray-800 rounded-lg p-4 space-y-3">
          <h4 class="font-semibold">我们收集的信息类型：</h4>
          <ul class="list-disc list-inside space-y-1 text-sm">
            <li>账户信息：用户名、邮箱地址、密码（加密存储）</li>
            <li>个人资料：姓名、头像、联系方式等</li>
            <li>使用数据：登录时间、操作记录、偏好设置</li>
            <li>技术信息：IP地址、浏览器类型、设备信息</li>
          </ul>
        </div>
      </div>
    </section>

    <section>
      <h2 class="text-xl font-semibold text-gray-900 dark:text-white mb-4">
        2. 信息使用
      </h2>
      <div class="space-y-4 text-gray-700 dark:text-gray-300">
        <p>我们使用收集的信息来：</p>
        <div class="grid md:grid-cols-2 gap-4">
          <div class="bg-blue-50 dark:bg-blue-900/20 rounded-lg p-4">
            <h4 class="font-semibold text-blue-900 dark:text-blue-100 mb-2">
              服务提供
            </h4>
            <ul class="text-sm text-blue-800 dark:text-blue-200 space-y-1">
              <li>• 提供和维护服务</li>
              <li>• 处理您的请求</li>
              <li>• 个性化体验</li>
            </ul>
          </div>
          <div class="bg-green-50 dark:bg-green-900/20 rounded-lg p-4">
            <h4 class="font-semibold text-green-900 dark:text-green-100 mb-2">
              安全保护
            </h4>
            <ul class="text-sm text-green-800 dark:text-green-200 space-y-1">
              <li>• 防止欺诈行为</li>
              <li>• 确保系统安全</li>
              <li>• 遵守法律要求</li>
            </ul>
          </div>
        </div>
      </div>
    </section>

    <section>
      <h2 class="text-xl font-semibold text-gray-900 dark:text-white mb-4">
        3. 信息共享
      </h2>
      <div class="space-y-4 text-gray-700 dark:text-gray-300">
        <div
          class="bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-800 rounded-lg p-4"
        >
          <p class="text-sm">
            我们不会出售您的个人信息。我们只在以下情况下共享您的信息：
          </p>
        </div>
        <ul class="list-disc list-inside space-y-2">
          <li>经您明确同意</li>
          <li>与可信的服务提供商合作</li>
          <li>遵守法律法规要求</li>
          <li>保护我们或他人的权利和安全</li>
        </ul>
      </div>
    </section>

    <section>
      <h2 class="text-xl font-semibold text-gray-900 dark:text-white mb-4">
        4. 数据安全
      </h2>
      <div class="space-y-4 text-gray-700 dark:text-gray-300">
        <p>我们采用多层安全措施保护您的信息：</p>
        <div class="bg-gray-50 dark:bg-gray-800 rounded-lg p-4 space-y-3">
          <div class="flex items-start space-x-3">
            <div
              class="w-6 h-6 bg-green-500 text-white rounded-full flex items-center justify-center text-xs font-bold"
            >
              ✓
            </div>
            <div>
              <h4 class="font-semibold">加密传输</h4>
              <p class="text-sm">使用SSL/TLS加密保护数据传输</p>
            </div>
          </div>
          <div class="flex items-start space-x-3">
            <div
              class="w-6 h-6 bg-green-500 text-white rounded-full flex items-center justify-center text-xs font-bold"
            >
              ✓
            </div>
            <div>
              <h4 class="font-semibold">访问控制</h4>
              <p class="text-sm">严格限制数据访问权限</p>
            </div>
          </div>
          <div class="flex items-start space-x-3">
            <div
              class="w-6 h-6 bg-green-500 text-white rounded-full flex items-center justify-center text-xs font-bold"
            >
              ✓
            </div>
            <div>
              <h4 class="font-semibold">定期审计</h4>
              <p class="text-sm">定期进行安全审计和评估</p>
            </div>
          </div>
        </div>
      </div>
    </section>

    <section>
      <h2 class="text-xl font-semibold text-gray-900 dark:text-white mb-4">
        5. 您的权利
      </h2>
      <div class="space-y-4 text-gray-700 dark:text-gray-300">
        <p>您对自己的个人信息享有以下权利：</p>
        <div class="grid md:grid-cols-2 gap-4">
          <div class="space-y-2">
            <h4 class="font-semibold">数据权利：</h4>
            <ul class="list-disc list-inside space-y-1 text-sm">
              <li>查看您的个人信息</li>
              <li>更正不准确的信息</li>
              <li>删除您的个人信息</li>
              <li>限制数据处理</li>
            </ul>
          </div>
          <div class="space-y-2">
            <h4 class="font-semibold">控制权利：</h4>
            <ul class="list-disc list-inside space-y-1 text-sm">
              <li>撤回同意</li>
              <li>数据可携带性</li>
              <li>反对数据处理</li>
              <li>投诉监管机构</li>
            </ul>
          </div>
        </div>
      </div>
    </section>

    <!-- 联系信息 -->
    <div
      class="bg-gradient-to-r from-green-50 to-blue-50 dark:from-green-900/20 dark:to-blue-900/20 rounded-lg p-4 border border-green-200 dark:border-green-700"
    >
      <h4
        class="text-lg font-semibold text-gray-900 dark:text-white mb-2 flex items-center"
      >
        <UIcon
          name="i-heroicons-envelope"
          class="w-4 h-4 mr-2 text-green-500"
        />
        隐私问题联系
      </h4>
      <p class="text-gray-700 dark:text-gray-300 text-sm">
        如果您对我们的隐私做法有任何疑问，请联系：privacy@example.com
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
// 纯内容展示组件，无需任何逻辑
</script>
