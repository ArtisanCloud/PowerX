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
    <div class="bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 rounded-lg p-4">
      <p class="text-sm">
        <strong>{{ copy.lastUpdatedLabel }}</strong>{{ copy.lastUpdatedValue }}
      </p>
      <p class="text-sm mt-2">{{ copy.contact }}</p>
    </div>
  </div>
</template>

<script setup lang="ts">
type TermsSection = {
  title: string;
  paragraphs: string[];
  bullets?: string[];
};

type TermsCopy = {
  sections: TermsSection[];
  lastUpdatedLabel: string;
  lastUpdatedValue: string;
  contact: string;
};

const { locale } = useI18n();

const zhCN: TermsCopy = {
  sections: [
    {
      title: "1. 服务说明",
      paragraphs: [
        "欢迎使用我们的服务！本条款规定了您使用本系统的条件。",
        "通过访问或使用本系统，表示您同意受本条款约束。",
      ],
      bullets: [
        "提供稳定可靠的在线服务平台",
        "保护用户数据安全和隐私",
        "持续改进和优化用户体验",
      ],
    },
    {
      title: "2. 用户责任",
      paragraphs: ["作为本系统用户，您需要遵守以下要求："],
      bullets: [
        "妥善保管账户凭证，不得与他人共享",
        "不得发布违法或侵权内容",
        "不得进行恶意攻击或破坏系统行为",
      ],
    },
    {
      title: "3. 数据与安全",
      paragraphs: [
        "我们将采取合理的技术与管理措施保障数据安全。",
        "您同意在法律允许范围内配合必要的安全审计与合规要求。",
      ],
    },
    {
      title: "4. 免责声明",
      paragraphs: [
        "服务按“现状”提供，不保证持续无中断或完全无误。",
        "在法律允许范围内，我们不对间接损失承担责任。",
      ],
    },
  ],
  lastUpdatedLabel: "最后更新：",
  lastUpdatedValue: new Date().toLocaleDateString("zh-CN"),
  contact: "如有疑问，请联系：support@example.com",
};

const enUS: TermsCopy = {
  sections: [
    {
      title: "1. Service Description",
      paragraphs: [
        "Welcome! These terms define the conditions for using this system.",
        "By accessing or using the system, you agree to be bound by these terms.",
      ],
      bullets: [
        "Provide a stable and reliable service platform",
        "Protect user data and privacy",
        "Continuously improve product experience",
      ],
    },
    {
      title: "2. User Responsibilities",
      paragraphs: ["As a user, you must comply with the following requirements:"],
      bullets: [
        "Keep account credentials secure and do not share them",
        "Do not publish illegal or infringing content",
        "Do not perform malicious attacks or disruptive actions",
      ],
    },
    {
      title: "3. Data and Security",
      paragraphs: [
        "We apply reasonable technical and organizational measures to protect data.",
        "You agree to cooperate with necessary security and compliance checks where legally required.",
      ],
    },
    {
      title: "4. Disclaimer",
      paragraphs: [
        "The service is provided \"as is\" without guarantee of uninterrupted or error-free operation.",
        "To the extent permitted by law, we are not liable for indirect damages.",
      ],
    },
  ],
  lastUpdatedLabel: "Last updated: ",
  lastUpdatedValue: new Date().toLocaleDateString("en-US"),
  contact: "For questions, contact: support@example.com",
};

const copy = computed<TermsCopy>(() =>
  String(locale.value || "").startsWith("zh") ? zhCN : enUS
);
</script>
