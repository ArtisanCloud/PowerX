<script setup lang="ts">
definePageMeta({
  layout: false,
});

const { t } = useI18n();

// SEO 设置
useSeoMeta({
  title: "联系我们 - PowerX",
  description:
    "联系 PowerX 团队，获取专业的技术支持和咨询服务。我们随时为您提供帮助。",
});

// 表单数据
const form = reactive({
  name: "",
  email: "",
  company: "",
  phone: "",
  subject: "",
  message: "",
  type: "general",
});

// 表单状态
const loading = ref(false);
const success = ref(false);
const error = ref("");

// 联系类型选项
const contactTypes = [
  { value: "general", label: "一般咨询" },
  { value: "sales", label: "销售咨询" },
  { value: "support", label: "技术支持" },
  { value: "partnership", label: "合作伙伴" },
  { value: "media", label: "媒体合作" },
];

// 表单验证
const validateForm = () => {
  if (!form.name.trim()) {
    error.value = "请输入您的姓名";
    return false;
  }
  if (!form.email.trim()) {
    error.value = "请输入邮箱地址";
    return false;
  }
  if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(form.email)) {
    error.value = "请输入有效的邮箱地址";
    return false;
  }
  if (!form.subject.trim()) {
    error.value = "请输入主题";
    return false;
  }
  if (!form.message.trim()) {
    error.value = "请输入消息内容";
    return false;
  }
  return true;
};

// 提交表单
const handleSubmit = async () => {
  if (!validateForm()) return;

  loading.value = true;
  error.value = "";

  try {
    // 这里添加实际的提交逻辑
    await new Promise((resolve) => setTimeout(resolve, 2000)); // 模拟API调用

    success.value = true;
    // 重置表单
    Object.keys(form).forEach((key) => {
      if (key === "type") {
        form[key] = "general";
      } else {
        form[key] = "";
      }
    });
  } catch (err) {
    error.value = "发送失败，请稍后重试";
  } finally {
    loading.value = false;
  }
};
</script>

<template>
  <div
    class="min-h-screen bg-gradient-to-br from-blue-50 via-white to-purple-50"
  >
    <!-- 页面头部 -->
    <div class="bg-white shadow-sm">
      <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
        <div class="text-center">
          <h1 class="text-4xl font-bold text-gray-900 mb-4">联系我们</h1>
          <p class="text-xl text-gray-600 max-w-3xl mx-auto">
            我们随时为您提供专业的技术支持和咨询服务
          </p>
        </div>
      </div>
    </div>

    <!-- 主要内容 -->
    <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-16">
      <div class="grid lg:grid-cols-2 gap-12">
        <!-- 联系表单 -->
        <div class="bg-white rounded-2xl shadow-xl p-8">
          <h2 class="text-2xl font-bold text-gray-900 mb-6">发送消息</h2>

          <!-- 成功提示 -->
          <UAlert
            v-if="success"
            color="success"
            variant="soft"
            title="消息发送成功！"
            description="我们已收到您的消息，将在24小时内回复您。"
            class="mb-6"
          />

          <form v-else @submit.prevent="handleSubmit" class="space-y-6">
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
              class="mb-4"
            />

            <!-- 联系类型 -->
            <div>
              <label class="block text-sm font-medium text-gray-700 mb-2">
                咨询类型 <span class="text-red-500">*</span>
              </label>
              <USelectMenu
                v-model="form.type"
                :options="contactTypes"
                option-attribute="label"
                value-attribute="value"
                :disabled="loading"
              />
            </div>

            <!-- 姓名和邮箱 -->
            <div class="grid md:grid-cols-2 gap-4">
              <div>
                <label class="block text-sm font-medium text-gray-700 mb-2">
                  姓名 <span class="text-red-500">*</span>
                </label>
                <UInput
                  v-model="form.name"
                  placeholder="请输入您的姓名"
                  :disabled="loading"
                />
              </div>
              <div>
                <label class="block text-sm font-medium text-gray-700 mb-2">
                  邮箱 <span class="text-red-500">*</span>
                </label>
                <UInput
                  v-model="form.email"
                  type="email"
                  placeholder="请输入邮箱地址"
                  :disabled="loading"
                />
              </div>
            </div>

            <!-- 公司和电话 -->
            <div class="grid md:grid-cols-2 gap-4">
              <div>
                <label class="block text-sm font-medium text-gray-700 mb-2">
                  公司名称
                </label>
                <UInput
                  v-model="form.company"
                  placeholder="请输入公司名称（可选）"
                  :disabled="loading"
                />
              </div>
              <div>
                <label class="block text-sm font-medium text-gray-700 mb-2">
                  联系电话
                </label>
                <UInput
                  v-model="form.phone"
                  placeholder="请输入联系电话（可选）"
                  :disabled="loading"
                />
              </div>
            </div>

            <!-- 主题 -->
            <div>
              <label class="block text-sm font-medium text-gray-700 mb-2">
                主题 <span class="text-red-500">*</span>
              </label>
              <UInput
                v-model="form.subject"
                placeholder="请输入消息主题"
                :disabled="loading"
              />
            </div>

            <!-- 消息内容 -->
            <div>
              <label class="block text-sm font-medium text-gray-700 mb-2">
                消息内容 <span class="text-red-500">*</span>
              </label>
              <UTextarea
                v-model="form.message"
                placeholder="请详细描述您的问题或需求..."
                :rows="6"
                :disabled="loading"
              />
            </div>

            <!-- 提交按钮 -->
            <UButton
              type="submit"
              block
              size="lg"
              :loading="loading"
              class="bg-gradient-to-r from-blue-600 to-purple-600 hover:from-blue-700 hover:to-purple-700"
            >
              {{ loading ? "发送中..." : "发送消息" }}
            </UButton>
          </form>
        </div>

        <!-- 联系信息 -->
        <div class="space-y-8">
          <!-- 联系方式 -->
          <div class="bg-white rounded-2xl shadow-xl p-8">
            <h2 class="text-2xl font-bold text-gray-900 mb-6">联系方式</h2>
            <div class="space-y-6">
              <div class="flex items-start space-x-4">
                <div
                  class="w-12 h-12 bg-blue-100 rounded-lg flex items-center justify-center"
                >
                  <UIcon
                    name="i-heroicons-envelope"
                    class="w-6 h-6 text-blue-600"
                  />
                </div>
                <div>
                  <h3 class="font-semibold text-gray-900">邮箱地址</h3>
                  <p class="text-gray-600">contact@powerx.com</p>
                  <p class="text-sm text-gray-500">我们会在24小时内回复</p>
                </div>
              </div>

              <div class="flex items-start space-x-4">
                <div
                  class="w-12 h-12 bg-green-100 rounded-lg flex items-center justify-center"
                >
                  <UIcon
                    name="i-heroicons-phone"
                    class="w-6 h-6 text-green-600"
                  />
                </div>
                <div>
                  <h3 class="font-semibold text-gray-900">客服热线</h3>
                  <p class="text-gray-600">400-888-0000</p>
                  <p class="text-sm text-gray-500">工作日 9:00-18:00</p>
                </div>
              </div>

              <div class="flex items-start space-x-4">
                <div
                  class="w-12 h-12 bg-purple-100 rounded-lg flex items-center justify-center"
                >
                  <UIcon
                    name="i-heroicons-map-pin"
                    class="w-6 h-6 text-purple-600"
                  />
                </div>
                <div>
                  <h3 class="font-semibold text-gray-900">办公地址</h3>
                  <p class="text-gray-600">北京市朝阳区科技园区</p>
                  <p class="text-sm text-gray-500">欢迎预约参观</p>
                </div>
              </div>
            </div>
          </div>

          <!-- 技术支持 -->
          <div
            class="bg-gradient-to-br from-blue-500 to-purple-600 rounded-2xl p-8 text-white"
          >
            <h2 class="text-2xl font-bold mb-4">技术支持</h2>
            <p class="text-blue-100 mb-6">
              遇到技术问题？我们的专业团队随时为您提供帮助。
            </p>
            <div class="space-y-4">
              <div class="flex items-center space-x-3">
                <UIcon
                  name="i-heroicons-check-circle"
                  class="w-5 h-5 text-green-300"
                />
                <span>7x24 在线技术支持</span>
              </div>
              <div class="flex items-center space-x-3">
                <UIcon
                  name="i-heroicons-check-circle"
                  class="w-5 h-5 text-green-300"
                />
                <span>远程协助服务</span>
              </div>
              <div class="flex items-center space-x-3">
                <UIcon
                  name="i-heroicons-check-circle"
                  class="w-5 h-5 text-green-300"
                />
                <span>专业培训服务</span>
              </div>
            </div>
            <UButton
              variant="white"
              size="lg"
              class="mt-6"
              @click="form.type = 'support'"
            >
              获取技术支持
            </UButton>
          </div>

          <!-- 销售咨询 -->
          <div class="bg-white rounded-2xl shadow-xl p-8">
            <h2 class="text-2xl font-bold text-gray-900 mb-4">销售咨询</h2>
            <p class="text-gray-600 mb-6">
              想了解我们的产品和解决方案？联系我们的销售团队。
            </p>
            <div class="space-y-3">
              <div class="flex items-center space-x-3">
                <UIcon
                  name="i-heroicons-check-circle"
                  class="w-5 h-5 text-green-500"
                />
                <span class="text-gray-700">免费产品演示</span>
              </div>
              <div class="flex items-center space-x-3">
                <UIcon
                  name="i-heroicons-check-circle"
                  class="w-5 h-5 text-green-500"
                />
                <span class="text-gray-700">定制化方案设计</span>
              </div>
              <div class="flex items-center space-x-3">
                <UIcon
                  name="i-heroicons-check-circle"
                  class="w-5 h-5 text-green-500"
                />
                <span class="text-gray-700">专业咨询服务</span>
              </div>
            </div>
            <UButton
              variant="outline"
              size="lg"
              class="mt-6"
              @click="form.type = 'sales'"
            >
              咨询销售
            </UButton>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
