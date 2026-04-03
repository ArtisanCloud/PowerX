<template>
  <div class="p-6 prose dark:prose-invert max-w-none space-y-6">
    <section v-for="section in copy.sections" :key="section.title">
      <h2 class="text-xl font-semibold text-gray-900 dark:text-white mb-4">
        {{ section.title }}
      </h2>
      <div class="space-y-3 text-gray-700 dark:text-gray-300">
        <p v-for="p in section.paragraphs" :key="p">{{ p }}</p>
        <ul v-if="section.bullets?.length" class="list-disc list-inside space-y-1">
          <li v-for="b in section.bullets" :key="b">{{ b }}</li>
        </ul>
      </div>
    </section>
    <div class="bg-green-50 dark:bg-green-900/20 border border-green-200 dark:border-green-700 rounded-lg p-4">
      <h4 class="text-lg font-semibold text-gray-900 dark:text-white mb-2 flex items-center">
        <UIcon name="i-heroicons-envelope" class="w-4 h-4 mr-2 text-green-500" />
        {{ copy.contactTitle }}
      </h4>
      <p class="text-gray-700 dark:text-gray-300 text-sm">
        {{ copy.contactText }}
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
type PrivacySection = {
  title: string;
  paragraphs: string[];
  bullets?: string[];
};

type PrivacyCopy = {
  sections: PrivacySection[];
  contactTitle: string;
  contactText: string;
};

const { locale } = useI18n();

const zhCN: PrivacyCopy = {
  sections: [
    {
      title: "1. 信息收集",
      paragraphs: ["我们收集您主动提供的信息，以及您使用系统时产生的必要技术信息。"],
      bullets: [
        "账户信息：用户名、邮箱、加密后的凭证",
        "业务信息：配置、操作日志与偏好设置",
        "技术信息：IP、设备与浏览器信息",
      ],
    },
    {
      title: "2. 信息使用",
      paragraphs: ["我们仅将信息用于提供服务、安全保障与合规要求。"],
      bullets: [
        "提供和维护核心功能",
        "进行安全审计与故障排查",
        "在授权范围内进行体验优化",
      ],
    },
    {
      title: "3. 信息共享",
      paragraphs: ["我们不会出售个人信息，仅在以下场景共享必要数据："],
      bullets: ["经您明确授权", "法律法规要求", "保护系统与用户安全"],
    },
    {
      title: "4. 您的权利",
      paragraphs: ["您可依法申请访问、更正、删除或限制处理您的个人信息。"],
    },
  ],
  contactTitle: "隐私问题联系",
  contactText: "如对隐私处理有疑问，请联系：privacy@example.com",
};

const enUS: PrivacyCopy = {
  sections: [
    {
      title: "1. Information Collection",
      paragraphs: ["We collect information you provide and necessary technical data generated during system use."],
      bullets: [
        "Account data: username, email, encrypted credentials",
        "Business data: configuration, operation logs, preferences",
        "Technical data: IP, device, and browser information",
      ],
    },
    {
      title: "2. Information Use",
      paragraphs: ["We use information only for service delivery, security protection, and compliance obligations."],
      bullets: [
        "Provide and maintain core features",
        "Perform security auditing and troubleshooting",
        "Improve product experience within authorized scope",
      ],
    },
    {
      title: "3. Information Sharing",
      paragraphs: ["We do not sell personal data. We share only when necessary in the following scenarios:"],
      bullets: ["With your explicit consent", "When required by law", "To protect platform and user security"],
    },
    {
      title: "4. Your Rights",
      paragraphs: ["Subject to applicable law, you may request access, correction, deletion, or restriction of your personal data."],
    },
  ],
  contactTitle: "Privacy Contact",
  contactText: "For privacy-related questions, contact: privacy@example.com",
};

const copy = computed<PrivacyCopy>(() =>
  String(locale.value || "").startsWith("zh") ? zhCN : enUS
);
</script>
