<script setup lang="ts">
const { t } = useI18n()

const githubBase = "https://github.com/ArtisanCloud/PowerX/tree/main"

const links = [
  {
    label: "Runbook",
    href: `${githubBase}/docs/guides/knowledge_space/runbook.md`,
  },
  {
    label: "Smoke Checklist",
    href: `${githubBase}/docs/guides/knowledge_space/smoke_checklist.md`,
  },
  {
    label: "Performance Validation",
    href: `${githubBase}/docs/guides/knowledge_space/perf_validation.md`,
  },
  {
    label: "场景/策略（索引）",
    href: `${githubBase}/docs/guides/knowledge_space/strategy/README.md`,
  },
  {
    label: "SCN-001 默认入库 + Playground",
    href: `${githubBase}/docs/guides/knowledge_space/strategy/scenes/SCN-001_basic_ingestion_and_playground.md`,
  },
  {
    label: "SCN-010 SOP/制度/产品说明",
    href: `${githubBase}/docs/guides/knowledge_space/strategy/scenes/SCN-010_sop_manual.md`,
  },
  {
    label: "SCN-020 合同/报价",
    href: `${githubBase}/docs/guides/knowledge_space/strategy/scenes/SCN-020_contract_quote.md`,
  },
  {
    label: "SCN-030 论文/研究/长报告",
    href: `${githubBase}/docs/guides/knowledge_space/strategy/scenes/SCN-030_research_report.md`,
  },
  {
    label: "SCN-040 台账/清单（表格）",
    href: `${githubBase}/docs/guides/knowledge_space/strategy/scenes/SCN-040_table_ledger.md`,
  },
  {
    label: "SCN-050 SQL/配置/依赖（KG 强）",
    href: `${githubBase}/docs/guides/knowledge_space/strategy/scenes/SCN-050_sql_config_kg.md`,
  },
  {
    label: "SCN-060 自定义（专家）",
    href: `${githubBase}/docs/guides/knowledge_space/strategy/scenes/SCN-060_custom_expert.md`,
  },
  {
    label: "SCN-070 产品库/规格参数查询",
    href: `${githubBase}/docs/guides/knowledge_space/strategy/scenes/SCN-070_product_specs.md`,
  },
  {
    label: "SCN-071 产品库/兼容性与配件关系",
    href: `${githubBase}/docs/guides/knowledge_space/strategy/scenes/SCN-071_product_compat.md`,
  },
  {
    label: "SCN-072 产品库/选型对比与推荐理由",
    href: `${githubBase}/docs/guides/knowledge_space/strategy/scenes/SCN-072_product_selection.md`,
  },
  {
    label: "SCN-080 客服 FAQ / 运营知识",
    href: `${githubBase}/docs/guides/knowledge_space/strategy/scenes/SCN-080_support_faq.md`,
  },
  {
    label: "SCN-081 客服/售后政策与规则",
    href: `${githubBase}/docs/guides/knowledge_space/strategy/scenes/SCN-081_support_policy.md`,
  },
  {
    label: "SCN-082 客服/故障现象与排查",
    href: `${githubBase}/docs/guides/knowledge_space/strategy/scenes/SCN-082_support_troubleshooting.md`,
  },
  {
    label: "SCN-090 工程/运维/故障排查（Runbook）",
    href: `${githubBase}/docs/guides/knowledge_space/strategy/scenes/SCN-090_eng_runbook.md`,
  },
  {
    label: "SCN-091 工程/故障排查与应急响应",
    href: `${githubBase}/docs/guides/knowledge_space/strategy/scenes/SCN-091_eng_incident.md`,
  },
  {
    label: "SCN-092 工程/变更与发布",
    href: `${githubBase}/docs/guides/knowledge_space/strategy/scenes/SCN-092_eng_change.md`,
  },
  {
    label: "SCN-100 API/数据字典/接口文档",
    href: `${githubBase}/docs/guides/knowledge_space/strategy/scenes/SCN-100_api_reference.md`,
  },
  {
    label: "SCN-101 数据字典/表结构与字段口径",
    href: `${githubBase}/docs/guides/knowledge_space/strategy/scenes/SCN-101_data_dictionary.md`,
  },
  {
    label: "SCN-110 销售材料/话术与竞品",
    href: `${githubBase}/docs/guides/knowledge_space/strategy/scenes/SCN-110_sales_enablement.md`,
  },
  {
    label: "SCN-120 市场活动/促销规则",
    href: `${githubBase}/docs/guides/knowledge_space/strategy/scenes/SCN-120_marketing_promo_rules.md`,
  },
  {
    label: "SCN-130 法规/监管政策（口径与约束）",
    href: `${githubBase}/docs/guides/knowledge_space/strategy/scenes/SCN-130_compliance_regulation.md`,
  },
  {
    label: "SCN-140 计费/价格规则（口径与例外）",
    href: `${githubBase}/docs/guides/knowledge_space/strategy/scenes/SCN-140_billing_pricing.md`,
  },
  {
    label: "SCN-150 会议纪要/决议与行动项",
    href: `${githubBase}/docs/guides/knowledge_space/strategy/scenes/SCN-150_meeting_minutes.md`,
  },
  {
    label: "SCN-160 项目方案/交付文档",
    href: `${githubBase}/docs/guides/knowledge_space/strategy/scenes/SCN-160_project_docs.md`,
  },
  {
    label: "SCN-170 工单/聊天记录（问题追踪）",
    href: `${githubBase}/docs/guides/knowledge_space/strategy/scenes/SCN-170_ticket_conversations.md`,
  },
  {
    label: "SCN-180 入职/培训资料（学习路径）",
    href: `${githubBase}/docs/guides/knowledge_space/strategy/scenes/SCN-180_onboarding_training.md`,
  },
]
</script>

<template>
  <div class="p-6 space-y-4">
    <h1 class="text-2xl font-semibold">
      {{ t("knowledgeSpaces.docs.title", "Knowledge Spaces 文档") }}
    </h1>
    <p class="text-sm text-gray-500">
      {{ t("knowledgeSpaces.docs.desc", "这里汇总了知识空间的运行手册与校验清单。") }}
    </p>
    <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
      <UCard v-for="item in links" :key="item.href" class="h-full">
        <template #header>{{ item.label }}</template>
        <p class="text-sm text-gray-500">
          {{ t("knowledgeSpaces.docs.openOnGitHub", "将在 GitHub 中打开") }}
        </p>
        <div class="mt-3">
          <UButton as="a" :href="item.href" target="_blank" icon="i-heroicons-arrow-top-right-on-square">
            {{ t("knowledgeSpaces.docs.view", "查看") }}
          </UButton>
        </div>
      </UCard>
    </div>
    <p class="text-xs text-gray-400">
      {{ t("knowledgeSpaces.docs.note", "如果需要离线版本，可在仓库 docs/guides/knowledge_space/ 下查阅。") }}
    </p>
  </div>
</template>
